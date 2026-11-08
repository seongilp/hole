package engine

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zihado/hole/internal/cache"
	"github.com/zihado/hole/internal/lists"
	"github.com/zihado/hole/internal/resolver"
	"github.com/zihado/hole/internal/rules"
	"github.com/zihado/hole/internal/stats"
)

func newEngine(t *testing.T, listContent string) (*Engine, string) {
	t.Helper()
	dir := t.TempDir()
	listPath := filepath.Join(dir, "list.txt")
	if err := os.WriteFile(listPath, []byte(listContent), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := rules.Open(filepath.Join(dir, "rules.json"))
	if err != nil {
		t.Fatal(err)
	}
	rec := stats.NewRecorder(10)
	c := cache.New(10, time.Now)
	h := resolver.New(resolver.Config{Cache: c, Stats: rec})
	e := New(Config{
		Handler: h,
		Rules:   store,
		Fetcher: lists.NewFetcher(http.DefaultClient),
		Sources: []string{listPath},
		Stats:   rec,
		Cache:   c,
	})
	return e, listPath
}

func TestEngine_RefreshLoadsLists(t *testing.T) {
	e, _ := newEngine(t, "ads.com\ntrack.com\n")
	if err := e.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	m := e.handler.Matcher()
	if !m.Blocked("ads.com") || !m.Blocked("track.com") {
		t.Error("domains from list should be blocked")
	}
	st := e.Status()
	if st.ListDomains != 2 || st.LastRefresh.IsZero() {
		t.Errorf("status = %+v", st)
	}
}

func TestEngine_RulesMergeWithLists(t *testing.T) {
	e, _ := newEngine(t, "ads.com\n")
	if err := e.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := e.AddRule(rules.Deny, "custom.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.AddRule(rules.Allow, "ads.com"); err != nil {
		t.Fatal(err)
	}
	m := e.handler.Matcher()
	if !m.Blocked("custom.com") {
		t.Error("deny rule should block")
	}
	if m.Blocked("ads.com") {
		t.Error("allow rule should override list")
	}

	if _, err := e.RemoveRule(rules.Allow, "ads.com"); err != nil {
		t.Fatal(err)
	}
	if !e.handler.Matcher().Blocked("ads.com") {
		t.Error("removing allow rule should re-block")
	}
}

func TestEngine_RefreshFailureKeepsPreviousLists(t *testing.T) {
	e, listPath := newEngine(t, "ads.com\n")
	if err := e.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(listPath); err != nil {
		t.Fatal(err)
	}
	if err := e.Refresh(context.Background()); err == nil {
		t.Error("expected error when all sources fail")
	}
	if !e.handler.Matcher().Blocked("ads.com") {
		t.Error("previous list should remain active after failed refresh")
	}
}
