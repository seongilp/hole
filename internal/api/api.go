// Package api serves the JSON API and the embedded web dashboard.
package api

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zihado/hole/internal/engine"
	"github.com/zihado/hole/internal/rules"
	"github.com/zihado/hole/internal/stats"
)

//go:embed web
var webFS embed.FS

const (
	topN          = 10
	maxBodyBytes  = 4 << 10
	refreshTimout = 2 * time.Minute
)

// Backend is what the API needs from the engine.
type Backend interface {
	Stats(n int) stats.Snapshot
	Recent() []stats.Query
	Status() engine.Status
	SetEnabled(on bool)
	Rules() rules.Rules
	AddRule(kind rules.Kind, domain string) (rules.Rules, error)
	RemoveRule(kind rules.Kind, domain string) (rules.Rules, error)
	Refresh(ctx context.Context) error
}

type response struct {
	Success bool    `json:"success"`
	Data    any     `json:"data"`
	Error   *string `json:"error"`
}

type server struct {
	b     Backend
	token string
}

// New returns the HTTP handler. If token is non-empty, mutating requests must
// send "Authorization: Bearer <token>".
func New(b Backend, token string) http.Handler {
	s := &server{b: b, token: token}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/stats", s.getStats)
	mux.HandleFunc("GET /api/queries", s.getQueries)
	mux.HandleFunc("GET /api/status", s.getStatus)
	mux.HandleFunc("POST /api/status", s.auth(s.postStatus))
	mux.HandleFunc("GET /api/rules", s.getRules)
	mux.HandleFunc("POST /api/rules", s.auth(s.postRule))
	mux.HandleFunc("DELETE /api/rules", s.auth(s.deleteRule))
	mux.HandleFunc("POST /api/lists/refresh", s.auth(s.postRefresh))
	mux.HandleFunc("GET /api/probes", s.getProbes)

	static, err := fs.Sub(webFS, "web")
	if err != nil {
		panic("embedded web assets missing: " + err.Error())
	}
	mux.Handle("GET /", http.FileServerFS(static))
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	csp := contentSecurityPolicy()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}

func (s *server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				fail(w, http.StatusUnauthorized, "missing or invalid API token")
				return
			}
		}
		next(w, r)
	}
}

func (s *server) getStats(w http.ResponseWriter, _ *http.Request) {
	ok(w, s.b.Stats(topN))
}

func (s *server) getStatus(w http.ResponseWriter, _ *http.Request) {
	ok(w, s.b.Status())
}

func (s *server) getQueries(w http.ResponseWriter, r *http.Request) {
	qs := s.b.Recent()
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			fail(w, http.StatusBadRequest, "limit must be a non-negative integer")
			return
		}
		qs = qs[:min(n, len(qs))]
	}
	ok(w, qs)
}

func (s *server) postStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Enabled == nil {
		fail(w, http.StatusBadRequest, `field "enabled" is required`)
		return
	}
	s.b.SetEnabled(*body.Enabled)
	ok(w, s.b.Status())
}

func (s *server) getRules(w http.ResponseWriter, _ *http.Request) {
	ok(w, s.b.Rules())
}

func (s *server) postRule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind   rules.Kind `json:"kind"`
		Domain string     `json:"domain"`
	}
	if !decode(w, r, &body) {
		return
	}
	s.applyRule(w, s.b.AddRule, body.Kind, body.Domain)
}

func (s *server) deleteRule(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	s.applyRule(w, s.b.RemoveRule, rules.Kind(q.Get("kind")), q.Get("domain"))
}

func (s *server) applyRule(w http.ResponseWriter, op func(rules.Kind, string) (rules.Rules, error), kind rules.Kind, domain string) {
	updated, err := op(kind, domain)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	ok(w, updated)
}

func (s *server) postRefresh(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), refreshTimout)
	defer cancel()
	if err := s.b.Refresh(ctx); err != nil {
		slog.Error("manual refresh failed", "err", err)
		fail(w, http.StatusBadGateway, "block list refresh failed; see server logs")
		return
	}
	ok(w, s.b.Status())
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func ok(w http.ResponseWriter, data any) {
	write(w, http.StatusOK, response{Success: true, Data: data})
}

func fail(w http.ResponseWriter, code int, msg string) {
	write(w, code, response{Success: false, Error: &msg})
}

func write(w http.ResponseWriter, code int, body response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Debug("write response failed", "err", err)
	}
}
