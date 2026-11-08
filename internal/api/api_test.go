package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zihado/hole/internal/engine"
	"github.com/zihado/hole/internal/rules"
	"github.com/zihado/hole/internal/stats"
)

type fakeBackend struct {
	enabled    bool
	rules      rules.Rules
	refreshErr error
	refreshed  int
}

func (f *fakeBackend) Stats(n int) stats.Snapshot { return stats.Snapshot{Total: 10, Blocked: 3} }
func (f *fakeBackend) Recent() []stats.Query {
	return []stats.Query{{Domain: "a.com"}, {Domain: "b.com"}, {Domain: "c.com"}}
}
func (f *fakeBackend) Status() engine.Status { return engine.Status{Enabled: f.enabled} }
func (f *fakeBackend) SetEnabled(on bool)    { f.enabled = on }
func (f *fakeBackend) Rules() rules.Rules    { return f.rules }
func (f *fakeBackend) Refresh(context.Context) error {
	f.refreshed++
	return f.refreshErr
}
func (f *fakeBackend) AddRule(k rules.Kind, d string) (rules.Rules, error) {
	if d == "bad" {
		return rules.Rules{}, errors.New("invalid domain")
	}
	f.rules.Deny = append(f.rules.Deny, d)
	return f.rules, nil
}
func (f *fakeBackend) RemoveRule(k rules.Kind, d string) (rules.Rules, error) {
	f.rules.Deny = nil
	return f.rules, nil
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *string         `json:"error"`
}

func do(t *testing.T, h http.Handler, method, path, body, token string) (int, envelope) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var env envelope
	if strings.HasPrefix(path, "/api/") {
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("%s %s: invalid JSON %q: %v", method, path, rec.Body.String(), err)
		}
	}
	return rec.Code, env
}

func TestAPI_ReadEndpoints(t *testing.T) {
	h := New(&fakeBackend{enabled: true}, "")

	tests := []struct {
		path    string
		contain string
	}{
		{"/api/stats", `"total":10`},
		{"/api/status", `"enabled":true`},
		{"/api/queries?limit=2", `"b.com"`},
		{"/api/rules", `"allow"`},
	}
	for _, tt := range tests {
		code, env := do(t, h, http.MethodGet, tt.path, "", "")
		if code != http.StatusOK || !env.Success {
			t.Errorf("GET %s: code=%d env=%+v", tt.path, code, env)
		}
		if !strings.Contains(string(env.Data), tt.contain) {
			t.Errorf("GET %s: data %s missing %s", tt.path, env.Data, tt.contain)
		}
	}
}

func TestAPI_QueriesLimit(t *testing.T) {
	h := New(&fakeBackend{}, "")
	_, env := do(t, h, http.MethodGet, "/api/queries?limit=2", "", "")
	var qs []stats.Query
	if err := json.Unmarshal(env.Data, &qs); err != nil {
		t.Fatal(err)
	}
	if len(qs) != 2 {
		t.Errorf("len = %d, want 2", len(qs))
	}
	if code, _ := do(t, h, http.MethodGet, "/api/queries?limit=abc", "", ""); code != http.StatusBadRequest {
		t.Errorf("invalid limit code = %d, want 400", code)
	}
}

func TestAPI_ToggleBlocking(t *testing.T) {
	b := &fakeBackend{enabled: true}
	h := New(b, "")

	code, _ := do(t, h, http.MethodPost, "/api/status", `{"enabled":false}`, "")
	if code != http.StatusOK || b.enabled {
		t.Errorf("code=%d enabled=%v, want 200 and false", code, b.enabled)
	}
	if code, _ := do(t, h, http.MethodPost, "/api/status", `{bad json`, ""); code != http.StatusBadRequest {
		t.Errorf("bad json code = %d, want 400", code)
	}
	if code, _ := do(t, h, http.MethodPost, "/api/status", `{}`, ""); code != http.StatusBadRequest {
		t.Errorf("missing field code = %d, want 400", code)
	}
}

func TestAPI_Rules(t *testing.T) {
	b := &fakeBackend{}
	h := New(b, "")

	code, env := do(t, h, http.MethodPost, "/api/rules", `{"kind":"deny","domain":"x.com"}`, "")
	if code != http.StatusOK || !strings.Contains(string(env.Data), "x.com") {
		t.Errorf("add: code=%d data=%s", code, env.Data)
	}
	code, env = do(t, h, http.MethodPost, "/api/rules", `{"kind":"deny","domain":"bad"}`, "")
	if code != http.StatusBadRequest || env.Success || env.Error == nil {
		t.Errorf("invalid add: code=%d env=%+v", code, env)
	}
	code, _ = do(t, h, http.MethodDelete, "/api/rules?kind=deny&domain=x.com", "", "")
	if code != http.StatusOK || len(b.rules.Deny) != 0 {
		t.Errorf("delete: code=%d deny=%v", code, b.rules.Deny)
	}
}

func TestAPI_Refresh(t *testing.T) {
	b := &fakeBackend{}
	h := New(b, "")
	if code, _ := do(t, h, http.MethodPost, "/api/lists/refresh", "", ""); code != http.StatusOK || b.refreshed != 1 {
		t.Errorf("code=%d refreshed=%d", code, b.refreshed)
	}
	b.refreshErr = errors.New("boom")
	if code, _ := do(t, h, http.MethodPost, "/api/lists/refresh", "", ""); code != http.StatusBadGateway {
		t.Errorf("failing refresh code = %d, want 502", code)
	}
}

func TestAPI_TokenProtectsMutations(t *testing.T) {
	b := &fakeBackend{enabled: true}
	h := New(b, "secret")

	if code, _ := do(t, h, http.MethodGet, "/api/stats", "", ""); code != http.StatusOK {
		t.Errorf("reads should not need a token, got %d", code)
	}
	if code, _ := do(t, h, http.MethodPost, "/api/status", `{"enabled":false}`, ""); code != http.StatusUnauthorized {
		t.Errorf("no token code = %d, want 401", code)
	}
	if code, _ := do(t, h, http.MethodPost, "/api/status", `{"enabled":false}`, "wrong"); code != http.StatusUnauthorized {
		t.Errorf("wrong token code = %d, want 401", code)
	}
	if code, _ := do(t, h, http.MethodPost, "/api/status", `{"enabled":false}`, "secret"); code != http.StatusOK || b.enabled {
		t.Errorf("valid token code = %d enabled=%v", code, b.enabled)
	}
}

func TestAPI_ServesDashboard(t *testing.T) {
	h := New(&fakeBackend{}, "")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("dashboard code=%d", rec.Code)
	}
}
