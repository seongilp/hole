package lists

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetch_HTTPAndFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0.0.0.0 remote-ad.com\n"))
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "local.txt")
	if err := os.WriteFile(path, []byte("local-ad.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	f := NewFetcher(srv.Client())
	for src, want := range map[string]string{srv.URL: "remote-ad.com", path: "local-ad.com"} {
		got, err := f.Fetch(context.Background(), src)
		if err != nil {
			t.Fatalf("Fetch(%s): %v", src, err)
		}
		if len(got) != 1 || got[0] != want {
			t.Errorf("Fetch(%s) = %v, want [%s]", src, got, want)
		}
	}
}

func TestFetch_HTTPErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := NewFetcher(srv.Client()).Fetch(context.Background(), srv.URL); err == nil {
		t.Error("expected error for 404")
	}
}

func TestFetchAll_CombinesAndReportsFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("a.com\nb.com\n"))
	}))
	defer srv.Close()

	got, errs := NewFetcher(srv.Client()).FetchAll(context.Background(), []string{srv.URL, "/does/not/exist"})
	if len(got) != 2 {
		t.Errorf("domains = %v, want 2", got)
	}
	if len(errs) != 1 {
		t.Errorf("errors = %v, want 1", errs)
	}
}
