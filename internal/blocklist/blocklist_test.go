package blocklist

import (
	"strings"
	"testing"
)

func TestParse_HostsAndDomainFormats(t *testing.T) {
	input := `
# comment line
0.0.0.0 ads.example.com
127.0.0.1 tracker.example.net # trailing comment
0.0.0.0 localhost
plain-domain.org
||adblock-style.com^
   UPPER.Example.COM.
0.0.0.0 0.0.0.0
not a valid line with spaces
`
	got := Parse(strings.NewReader(input))

	want := []string{
		"ads.example.com",
		"tracker.example.net",
		"plain-domain.org",
		"adblock-style.com",
		"upper.example.com",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d domains %v, want %d %v", len(got), got, len(want), want)
	}
	for i, d := range want {
		if got[i] != d {
			t.Errorf("domain[%d] = %q, want %q", i, got[i], d)
		}
	}
}

func TestMatcher_ExactAndSubdomain(t *testing.T) {
	m := NewMatcher([]string{"doubleclick.net", "ads.example.com"}, nil)

	cases := map[string]bool{
		"doubleclick.net":         true,
		"doubleclick.net.":        true,
		"stats.g.doubleclick.net": true,
		"DoubleClick.NET":         true,
		"ads.example.com":         true,
		"x.ads.example.com":       true,
		"example.com":             false,
		"notdoubleclick.net":      false,
		"google.com":              false,
		"":                        false,
	}
	for domain, want := range cases {
		if got := m.Blocked(domain); got != want {
			t.Errorf("Blocked(%q) = %v, want %v", domain, got, want)
		}
	}
}

func TestMatcher_AllowlistOverridesBlock(t *testing.T) {
	m := NewMatcher([]string{"example.com"}, []string{"safe.example.com"})

	if m.Blocked("safe.example.com") {
		t.Error("allowlisted domain should not be blocked")
	}
	if m.Blocked("cdn.safe.example.com") {
		t.Error("subdomain of allowlisted domain should not be blocked")
	}
	if !m.Blocked("ads.example.com") {
		t.Error("non-allowlisted subdomain should still be blocked")
	}
}

func TestMatcher_Size(t *testing.T) {
	m := NewMatcher([]string{"a.com", "b.com", "a.com"}, nil)
	if m.Size() != 2 {
		t.Errorf("Size() = %d, want 2 (deduplicated)", m.Size())
	}
}
