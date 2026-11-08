// Package doctor diagnoses why ads might not be blocked on this machine.
package doctor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Level is a check outcome.
type Level int

const (
	OK Level = iota
	Warn
	Fail
)

func (l Level) symbol() string {
	switch l {
	case OK:
		return "✓"
	case Warn:
		return "!"
	default:
		return "✗"
	}
}

// Result is the outcome of one check.
type Result struct {
	Name   string
	Level  Level
	Detail string
	Fix    string // what the user can do; empty when OK
}

// Config tells the doctor where Hole and its upstreams live.
type Config struct {
	DNSAddr   string // e.g. 127.0.0.1:53
	HTTPAddr  string // e.g. 127.0.0.1:4653
	Upstreams []string
	Home      string // user home, for browser settings
	Timeout   time.Duration

	// Hooks for tests; nil means the real system.
	SystemNameservers func(ctx context.Context) ([]string, error)
	SystemLookup      func(ctx context.Context, host string) ([]string, error)
	HTTPClient        *http.Client
}

// BlockedProbe is a domain on every default block list.
const BlockedProbe = "googleads.g.doubleclick.net"

// AllowedProbe must always resolve.
const AllowedProbe = "example.com"

// Run executes all checks in order.
func Run(ctx context.Context, cfg Config) []Result {
	if cfg.Timeout == 0 {
		cfg.Timeout = 2 * time.Second
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: cfg.Timeout}
	}
	if cfg.SystemNameservers == nil {
		cfg.SystemNameservers = systemNameservers
	}
	if cfg.SystemLookup == nil {
		cfg.SystemLookup = systemLookup
	}
	results := []Result{
		checkDNSServer(cfg),
		checkAPI(ctx, cfg),
		checkUpstreams(cfg),
		checkSystemDNS(ctx, cfg),
		checkSystemResolution(ctx, cfg),
	}
	return append(results, checkBrowsers(cfg.Home)...)
}

// Print writes results for humans and reports whether everything is fine (no Fail).
func Print(w io.Writer, results []Result) bool {
	healthy := true
	for _, r := range results {
		fmt.Fprintf(w, "%s %s: %s\n", r.Level.symbol(), r.Name, r.Detail)
		if r.Fix != "" {
			fmt.Fprintf(w, "    → %s\n", r.Fix)
		}
		if r.Level == Fail {
			healthy = false
		}
	}
	fmt.Fprintln(w)
	if healthy {
		fmt.Fprintln(w, "문제 없음.")
	} else {
		fmt.Fprintln(w, "위의 ✗ 항목을 먼저 해결하세요.")
	}
	return healthy
}
