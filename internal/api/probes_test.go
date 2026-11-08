package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPI_Probes(t *testing.T) {
	h := New(&fakeBackend{enabled: true}, "")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/probes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Data struct {
			Control string  `json:"control"`
			Probes  []Probe `json:"probes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Control != controlHost || len(body.Data.Probes) != len(probes) {
		t.Fatalf("got %+v", body.Data)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "connect-src 'self' https:") || !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP = %q", csp)
	}
}

func TestProbes_UniqueHosts(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range probes {
		if seen[p.Host] {
			t.Errorf("duplicate probe %s", p.Host)
		}
		seen[p.Host] = true
	}
}
