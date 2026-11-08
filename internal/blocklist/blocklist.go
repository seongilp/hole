// Package blocklist parses ad/tracker domain lists and matches queried
// domains (including subdomains) against them.
package blocklist

import (
	"bufio"
	"io"
	"net"
	"strings"
)

// hostnames that commonly appear in hosts files but must never be blocked.
var ignoredHosts = map[string]struct{}{
	"localhost":             {},
	"localhost.localdomain": {},
	"local":                 {},
	"broadcasthost":         {},
	"ip6-localhost":         {},
	"ip6-loopback":          {},
	"0.0.0.0":               {},
}

// Parse reads a block list in hosts format ("0.0.0.0 domain"), plain domain
// format ("domain") or simple adblock format ("||domain^") and returns the
// normalized domains in order of appearance. Invalid lines are skipped.
func Parse(r io.Reader) []string {
	var out []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if d, ok := parseLine(sc.Text()); ok {
			out = append(out, d)
		}
	}
	return out
}

func parseLine(line string) (string, bool) {
	if i := strings.IndexAny(line, "#!"); i >= 0 {
		line = line[:i]
	}
	fields := strings.Fields(line)
	var candidate string
	switch len(fields) {
	case 1:
		candidate = strings.TrimSuffix(strings.TrimPrefix(fields[0], "||"), "^")
	case 2:
		if net.ParseIP(fields[0]) == nil {
			return "", false
		}
		candidate = fields[1]
	default:
		return "", false
	}
	d := Normalize(candidate)
	if _, ignored := ignoredHosts[d]; ignored || !ValidDomain(d) {
		return "", false
	}
	return d, true
}

// Normalize lowercases a domain and strips surrounding whitespace and the
// trailing root dot.
func Normalize(domain string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
}

// ValidDomain reports whether d (already normalized) is a syntactically valid,
// non-IP domain name with at least two labels.
func ValidDomain(d string) bool {
	if d == "" || len(d) > 253 || !strings.Contains(d, ".") || net.ParseIP(d) != nil {
		return false
	}
	for _, label := range strings.Split(d, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, c := range label {
			isAlnum := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
			if !isAlnum && c != '-' && c != '_' {
				return false
			}
		}
	}
	return true
}

// Matcher is an immutable set of block and allow rules. It is safe for
// concurrent use; build a new Matcher to change rules.
type Matcher struct {
	block map[string]struct{}
	allow map[string]struct{}
}

// NewMatcher builds a Matcher from block and allow domain lists.
func NewMatcher(block, allow []string) *Matcher {
	return &Matcher{block: toSet(block), allow: toSet(allow)}
}

func toSet(domains []string) map[string]struct{} {
	set := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		if n := Normalize(d); n != "" {
			set[n] = struct{}{}
		}
	}
	return set
}

// Blocked reports whether domain or any of its parent domains is on the block
// list, unless domain or one of its parents is allowlisted.
func (m *Matcher) Blocked(domain string) bool {
	d := Normalize(domain)
	if d == "" {
		return false
	}
	if matchesSuffix(m.allow, d) {
		return false
	}
	return matchesSuffix(m.block, d)
}

// matchesSuffix checks d, then each parent ("a.b.c" -> "b.c" -> "c").
func matchesSuffix(set map[string]struct{}, d string) bool {
	for {
		if _, ok := set[d]; ok {
			return true
		}
		i := strings.IndexByte(d, '.')
		if i < 0 {
			return false
		}
		d = d[i+1:]
	}
}

// Size returns the number of unique blocked domains.
func (m *Matcher) Size() int { return len(m.block) }
