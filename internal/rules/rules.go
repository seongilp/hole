// Package rules persists user-defined allow and deny domains.
package rules

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/zihado/hole/internal/blocklist"
)

// Kind selects the allow or deny list.
type Kind string

const (
	Allow Kind = "allow"
	Deny  Kind = "deny"
)

// Rules is a snapshot of user-defined domains.
type Rules struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

func (r Rules) clone() Rules {
	return Rules{Allow: slices.Clone(r.Allow), Deny: slices.Clone(r.Deny)}
}

// Store keeps Rules in a JSON file. It is safe for concurrent use.
type Store struct {
	mu    sync.Mutex
	path  string
	rules Rules
}

// Open loads rules from path, starting empty if the file does not exist.
func Open(path string) (*Store, error) {
	s := &Store{path: path, rules: Rules{Allow: []string{}, Deny: []string{}}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read rules %s: %w", path, err)
	}
	var loaded Rules
	if err := json.Unmarshal(data, &loaded); err != nil {
		return nil, fmt.Errorf("parse rules %s: %w", path, err)
	}
	s.rules = Rules{Allow: orEmpty(loaded.Allow), Deny: orEmpty(loaded.Deny)}
	return s, nil
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Rules returns a copy of the current rules.
func (s *Store) Rules() Rules {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rules.clone()
}

// Add inserts domain into the given list and persists the result.
func (s *Store) Add(kind Kind, domain string) (Rules, error) {
	return s.update(kind, domain, func(list []string, d string) []string {
		if slices.Contains(list, d) {
			return list
		}
		return append(slices.Clone(list), d)
	})
}

// Remove deletes domain from the given list and persists the result.
func (s *Store) Remove(kind Kind, domain string) (Rules, error) {
	return s.update(kind, domain, func(list []string, d string) []string {
		return slices.DeleteFunc(slices.Clone(list), func(x string) bool { return x == d })
	})
}

func (s *Store) update(kind Kind, domain string, op func([]string, string) []string) (Rules, error) {
	d := blocklist.Normalize(domain)
	if !blocklist.ValidDomain(d) {
		return Rules{}, fmt.Errorf("invalid domain %q", domain)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.rules.clone()
	switch kind {
	case Allow:
		next.Allow = op(next.Allow, d)
	case Deny:
		next.Deny = op(next.Deny, d)
	default:
		return Rules{}, fmt.Errorf("invalid rule kind %q (want allow or deny)", kind)
	}
	if err := writeAtomic(s.path, next); err != nil {
		return Rules{}, err
	}
	s.rules = next
	return next.clone(), nil
}

// writeAtomic writes to a temp file and renames it so a crash never leaves a
// half-written rules file.
func writeAtomic(path string, r Rules) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode rules: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create rules dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write rules: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("save rules: %w", err)
	}
	return nil
}
