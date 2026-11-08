// Package stats records DNS query outcomes for the dashboard.
package stats

import (
	"sort"
	"sync"
	"time"
)

// Status is the outcome of a DNS query.
type Status string

const (
	StatusBlocked   Status = "blocked"
	StatusForwarded Status = "forwarded"
	StatusCached    Status = "cached"
	StatusError     Status = "error"
)

// Query is one logged DNS lookup.
type Query struct {
	Time      time.Time `json:"time"`
	Domain    string    `json:"domain"`
	Type      string    `json:"type"`
	Client    string    `json:"client"`
	Status    Status    `json:"status"`
	LatencyMS float64   `json:"latency_ms"`
}

// Count is a name with its occurrence count.
type Count struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Snapshot is a point-in-time copy of aggregate stats.
type Snapshot struct {
	Total          int     `json:"total"`
	Blocked        int     `json:"blocked"`
	Forwarded      int     `json:"forwarded"`
	Cached         int     `json:"cached"`
	Errors         int     `json:"errors"`
	BlockedPercent float64 `json:"blocked_percent"`
	TopBlocked     []Count `json:"top_blocked"`
	TopAllowed     []Count `json:"top_allowed"`
	TopClients     []Count `json:"top_clients"`
}

// maxTracked bounds per-domain counters so memory cannot grow without limit.
const maxTracked = 10000

// Recorder aggregates queries. It is safe for concurrent use.
type Recorder struct {
	mu       sync.Mutex
	byStatus map[Status]int
	blocked  map[string]int
	allowed  map[string]int
	clients  map[string]int
	recent   []Query // ring buffer
	next     int
	full     bool
}

// NewRecorder keeps the last recentSize queries for the query log.
func NewRecorder(recentSize int) *Recorder {
	return &Recorder{
		byStatus: map[Status]int{},
		blocked:  map[string]int{},
		allowed:  map[string]int{},
		clients:  map[string]int{},
		recent:   make([]Query, max(recentSize, 1)),
	}
}

// Record adds a query to the stats.
func (r *Recorder) Record(q Query) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.byStatus[q.Status]++
	if q.Status == StatusBlocked {
		bump(r.blocked, q.Domain)
	} else {
		bump(r.allowed, q.Domain)
	}
	bump(r.clients, q.Client)

	r.recent[r.next] = q
	r.next = (r.next + 1) % len(r.recent)
	if r.next == 0 {
		r.full = true
	}
}

func bump(m map[string]int, k string) {
	if _, ok := m[k]; ok || len(m) < maxTracked {
		m[k]++
	}
}

// Snapshot returns aggregate counters and the top n entries of each list.
func (r *Recorder) Snapshot(n int) Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	total := 0
	for _, c := range r.byStatus {
		total += c
	}
	s := Snapshot{
		Total:      total,
		Blocked:    r.byStatus[StatusBlocked],
		Forwarded:  r.byStatus[StatusForwarded],
		Cached:     r.byStatus[StatusCached],
		Errors:     r.byStatus[StatusError],
		TopBlocked: top(r.blocked, n),
		TopAllowed: top(r.allowed, n),
		TopClients: top(r.clients, n),
	}
	if total > 0 {
		s.BlockedPercent = float64(s.Blocked) * 100 / float64(total)
	}
	return s
}

func top(m map[string]int, n int) []Count {
	out := make([]Count, 0, len(m))
	for k, v := range m {
		out = append(out, Count{Name: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// Recent returns logged queries, newest first.
func (r *Recorder) Recent() []Query {
	r.mu.Lock()
	defer r.mu.Unlock()

	size := r.next
	if r.full {
		size = len(r.recent)
	}
	out := make([]Query, 0, size)
	for i := 1; i <= size; i++ {
		idx := (r.next - i + len(r.recent)) % len(r.recent)
		out = append(out, r.recent[idx])
	}
	return out
}
