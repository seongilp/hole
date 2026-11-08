package stats

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const stateVersion = 1

// state is the on-disk form of a Recorder.
type state struct {
	Version  int            `json:"version"`
	ByStatus map[Status]int `json:"by_status"`
	Blocked  map[string]int `json:"blocked"`
	Allowed  map[string]int `json:"allowed"`
	Clients  map[string]int `json:"clients"`
	Recent   []Query        `json:"recent"` // oldest first
}

// Save writes the recorder's counters and query log to path atomically.
func (r *Recorder) Save(path string) error {
	return writeState(path, r.export())
}

// Load replaces the recorder's state with the one saved at path.
// A missing file is not an error: the recorder simply starts empty.
func (r *Recorder) Load(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read stats: %w", err)
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return fmt.Errorf("parse stats %s: %w", path, err)
	}
	if st.Version != stateVersion {
		return fmt.Errorf("stats %s: unsupported version %d", path, st.Version)
	}
	r.restore(st)
	return nil
}

func (r *Recorder) export() state {
	recent := r.Recent()
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return state{
		Version:  stateVersion,
		ByStatus: copyMap(r.byStatus, -1),
		Blocked:  copyMap(r.blocked, -1),
		Allowed:  copyMap(r.allowed, -1),
		Clients:  copyMap(r.clients, -1),
		Recent:   recent,
	}
}

func (r *Recorder) restore(st state) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.byStatus = copyMap(st.ByStatus, -1)
	r.blocked = copyMap(st.Blocked, maxTracked)
	r.allowed = copyMap(st.Allowed, maxTracked)
	r.clients = copyMap(st.Clients, maxTracked)

	recent := st.Recent
	if len(recent) > len(r.recent) {
		recent = recent[len(recent)-len(r.recent):]
	}
	r.recent = make([]Query, len(r.recent))
	copy(r.recent, recent)
	r.next = len(recent) % len(r.recent)
	r.full = len(recent) == len(r.recent)
}

// copyMap copies m, keeping at most limit entries (limit < 0 means no limit).
func copyMap[K comparable](m map[K]int, limit int) map[K]int {
	out := make(map[K]int, len(m))
	for k, v := range m {
		if limit >= 0 && len(out) >= limit {
			break
		}
		if v > 0 {
			out[k] = v
		}
	}
	return out
}

func writeState(path string, st state) error {
	data, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("encode stats: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create stats dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".stats-*.json")
	if err != nil {
		return fmt.Errorf("create temp stats: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write stats: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close stats: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace stats: %w", err)
	}
	return nil
}
