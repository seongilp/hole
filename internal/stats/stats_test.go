package stats

import (
	"sync"
	"testing"
	"time"
)

func q(domain, client string, status Status) Query {
	return Query{Time: time.Unix(0, 0), Domain: domain, Type: "A", Client: client, Status: status}
}

func TestRecorder_Counters(t *testing.T) {
	r := NewRecorder(10)
	r.Record(q("a.com", "10.0.0.1", StatusForwarded))
	r.Record(q("ads.com", "10.0.0.1", StatusBlocked))
	r.Record(q("ads.com", "10.0.0.2", StatusBlocked))
	r.Record(q("a.com", "10.0.0.2", StatusCached))

	s := r.Snapshot(5)
	if s.Total != 4 || s.Blocked != 2 || s.Cached != 1 || s.Forwarded != 1 {
		t.Fatalf("counters = %+v", s)
	}
	if s.BlockedPercent != 50 {
		t.Errorf("BlockedPercent = %v, want 50", s.BlockedPercent)
	}
}

func TestRecorder_TopLists(t *testing.T) {
	r := NewRecorder(10)
	for i := 0; i < 3; i++ {
		r.Record(q("ads.com", "10.0.0.1", StatusBlocked))
	}
	r.Record(q("track.com", "10.0.0.2", StatusBlocked))
	r.Record(q("ok.com", "10.0.0.2", StatusForwarded))

	s := r.Snapshot(1)
	if len(s.TopBlocked) != 1 || s.TopBlocked[0].Name != "ads.com" || s.TopBlocked[0].Count != 3 {
		t.Errorf("TopBlocked = %+v", s.TopBlocked)
	}
	if len(s.TopClients) != 1 || s.TopClients[0].Name != "10.0.0.1" {
		t.Errorf("TopClients = %+v", s.TopClients)
	}
}

func TestRecorder_RecentIsBoundedNewestFirst(t *testing.T) {
	r := NewRecorder(3)
	for _, d := range []string{"1.com", "2.com", "3.com", "4.com", "5.com"} {
		r.Record(q(d, "c", StatusForwarded))
	}
	got := r.Recent()
	want := []string{"5.com", "4.com", "3.com"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Domain != want[i] {
			t.Errorf("recent[%d] = %s, want %s", i, got[i].Domain, want[i])
		}
	}
}

func TestRecorder_ConcurrentRecord(t *testing.T) {
	r := NewRecorder(100)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Record(q("a.com", "c", StatusForwarded))
			_ = r.Snapshot(3)
			_ = r.Recent()
		}()
	}
	wg.Wait()
	if s := r.Snapshot(1); s.Total != 50 {
		t.Errorf("Total = %d, want 50", s.Total)
	}
}
