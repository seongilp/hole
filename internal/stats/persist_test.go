package stats

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPersist_SaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	src := NewRecorder(10)
	src.Record(q("ads.example", "10.0.0.1", StatusBlocked))
	src.Record(q("ads.example", "10.0.0.1", StatusBlocked))
	src.Record(q("good.example", "10.0.0.2", StatusForwarded))
	src.Record(q("good.example", "10.0.0.2", StatusCached))
	if err := src.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	dst := NewRecorder(10)
	if err := dst.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, want := dst.Snapshot(5), src.Snapshot(5)
	if got.Total != 4 || got.Blocked != 2 || got.Cached != 1 || got.Forwarded != 1 {
		t.Fatalf("counters not restored: %+v", got)
	}
	if len(got.TopBlocked) != 1 || got.TopBlocked[0] != want.TopBlocked[0] {
		t.Fatalf("top blocked = %v, want %v", got.TopBlocked, want.TopBlocked)
	}
	if len(got.TopClients) != 2 {
		t.Fatalf("top clients = %v", got.TopClients)
	}
	recent := dst.Recent()
	if len(recent) != 4 || recent[0].Status != StatusCached || recent[3].Domain != "ads.example" {
		t.Fatalf("recent not restored newest-first: %+v", recent)
	}

	// New queries keep accumulating on top of restored state.
	dst.Record(q("ads.example", "10.0.0.1", StatusBlocked))
	if s := dst.Snapshot(5); s.Total != 5 || s.TopBlocked[0].Count != 3 {
		t.Fatalf("after restore+record: %+v", s)
	}
}

func TestPersist_LoadMissingFileIsNoop(t *testing.T) {
	r := NewRecorder(10)
	if err := r.Load(filepath.Join(t.TempDir(), "nope.json")); err != nil {
		t.Fatalf("missing file should be ignored, got %v", err)
	}
	if r.Snapshot(5).Total != 0 {
		t.Fatal("expected empty stats")
	}
}

func TestPersist_LoadCorruptFileFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := NewRecorder(10)
	if err := r.Load(path); err == nil {
		t.Fatal("expected error for corrupt file")
	}
	if r.Snapshot(5).Total != 0 {
		t.Fatal("corrupt load must not change stats")
	}
}

func TestPersist_RecentTrimmedToSmallerLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	src := NewRecorder(10)
	for _, d := range []string{"a", "b", "c", "d", "e"} {
		src.Record(q(d, "c", StatusForwarded))
	}
	if err := src.Save(path); err != nil {
		t.Fatal(err)
	}
	dst := NewRecorder(3)
	if err := dst.Load(path); err != nil {
		t.Fatal(err)
	}
	recent := dst.Recent()
	if len(recent) != 3 || recent[0].Domain != "e" || recent[2].Domain != "c" {
		t.Fatalf("want newest 3 (e,d,c), got %+v", recent)
	}
	dst.Record(q("f", "c", StatusForwarded))
	if r := dst.Recent(); r[0].Domain != "f" || len(r) != 3 {
		t.Fatalf("ring broken after trimmed restore: %+v", r)
	}
	if dst.Snapshot(5).Total != 6 {
		t.Fatal("counters must not be trimmed with the log")
	}
}

func TestPersist_LoadCapsTrackedEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	big := map[string]int{}
	for i := 0; i < maxTracked+50; i++ {
		big[fmt.Sprintf("d%d.example", i)] = 1
	}
	st := state{Version: stateVersion, Allowed: big}
	if err := writeState(path, st); err != nil {
		t.Fatal(err)
	}
	r := NewRecorder(10)
	if err := r.Load(path); err != nil {
		t.Fatal(err)
	}
	if n := len(r.allowed); n > maxTracked {
		t.Fatalf("allowed map has %d entries, cap is %d", n, maxTracked)
	}
}
