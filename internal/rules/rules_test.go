package rules

import (
	"path/filepath"
	"testing"
)

func TestStore_AddRemovePersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if _, err := s.Add(Allow, "Good.COM."); err != nil {
		t.Fatalf("Add allow: %v", err)
	}
	if _, err := s.Add(Deny, "bad.com"); err != nil {
		t.Fatalf("Add deny: %v", err)
	}
	got, err := s.Add(Deny, "bad.com")
	if err != nil {
		t.Fatalf("duplicate Add: %v", err)
	}
	if len(got.Deny) != 1 {
		t.Errorf("duplicate add should be a no-op, deny = %v", got.Deny)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	r := reopened.Rules()
	if len(r.Allow) != 1 || r.Allow[0] != "good.com" || len(r.Deny) != 1 || r.Deny[0] != "bad.com" {
		t.Errorf("persisted rules = %+v", r)
	}

	after, err := reopened.Remove(Deny, "bad.com")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(after.Deny) != 0 {
		t.Errorf("deny after remove = %v", after.Deny)
	}
}

func TestStore_RejectsInvalid(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "rules.json"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		kind   Kind
		domain string
	}{
		{Allow, "not a domain"},
		{Allow, ""},
		{Kind("other"), "ok.com"},
		{Deny, "1.2.3.4"},
	}
	for _, c := range cases {
		if _, err := s.Add(c.kind, c.domain); err == nil {
			t.Errorf("Add(%q, %q) should fail", c.kind, c.domain)
		}
	}
}

func TestStore_RulesReturnsCopy(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "rules.json"))
	_, _ = s.Add(Deny, "bad.com")

	r := s.Rules()
	r.Deny[0] = "mutated.com"

	if s.Rules().Deny[0] != "bad.com" {
		t.Error("Rules() must return a copy")
	}
}
