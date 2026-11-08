// Package engine wires block lists, user rules, the resolver and stats
// together and exposes the operations the HTTP API needs.
package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/zihado/hole/internal/blocklist"
	"github.com/zihado/hole/internal/cache"
	"github.com/zihado/hole/internal/lists"
	"github.com/zihado/hole/internal/resolver"
	"github.com/zihado/hole/internal/rules"
	"github.com/zihado/hole/internal/stats"
)

// Config holds Engine dependencies.
type Config struct {
	Handler *resolver.Handler
	Rules   *rules.Store
	Fetcher *lists.Fetcher
	Sources []string
	Stats   *stats.Recorder
	Cache   *cache.Cache
	Logger  *slog.Logger
}

// Status describes the engine for the dashboard.
type Status struct {
	Enabled      bool      `json:"enabled"`
	Sources      []string  `json:"sources"`
	ListDomains  int       `json:"list_domains"`
	TotalBlocked int       `json:"total_blocked_domains"`
	CacheEntries int       `json:"cache_entries"`
	LastRefresh  time.Time `json:"last_refresh"`
	LastError    string    `json:"last_error,omitempty"`
}

// Engine coordinates the sinkhole.
type Engine struct {
	handler *resolver.Handler
	rules   *rules.Store
	fetcher *lists.Fetcher
	sources []string
	stats   *stats.Recorder
	cache   *cache.Cache
	log     *slog.Logger

	refreshMu   sync.Mutex // serializes Refresh calls
	mu          sync.RWMutex
	listDomains []string
	lastRefresh time.Time
	lastError   string
}

// New creates an Engine. Call Refresh to load the block lists.
func New(cfg Config) *Engine {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	e := &Engine{
		handler: cfg.Handler,
		rules:   cfg.Rules,
		fetcher: cfg.Fetcher,
		sources: slices.Clone(cfg.Sources),
		stats:   cfg.Stats,
		cache:   cfg.Cache,
		log:     logger,
	}
	e.rebuild()
	return e
}

// Refresh downloads all sources and swaps in the new rules. If every source
// fails, the previous lists stay active and an error is returned.
func (e *Engine) Refresh(ctx context.Context) error {
	e.refreshMu.Lock()
	defer e.refreshMu.Unlock()

	domains, errs := e.fetcher.FetchAll(ctx, e.sources)
	for _, err := range errs {
		e.log.Warn("block list failed", "err", err)
	}
	if len(errs) > 0 && len(errs) == len(e.sources) {
		err := fmt.Errorf("all %d block list sources failed: %w", len(errs), errors.Join(errs...))
		e.setError(err.Error())
		return err
	}

	e.mu.Lock()
	e.listDomains = domains
	e.lastRefresh = time.Now()
	e.lastError = joinErrors(errs)
	e.mu.Unlock()

	e.rebuild()
	e.log.Info("block lists loaded", "sources", len(e.sources), "failed", len(errs),
		"domains", e.handler.Matcher().Size())
	return nil
}

func joinErrors(errs []error) string {
	if len(errs) == 0 {
		return ""
	}
	return errors.Join(errs...).Error()
}

func (e *Engine) setError(msg string) {
	e.mu.Lock()
	e.lastError = msg
	e.mu.Unlock()
}

// rebuild combines list domains with user rules into a fresh Matcher.
func (e *Engine) rebuild() {
	r := e.rules.Rules()
	e.mu.RLock()
	block := make([]string, 0, len(e.listDomains)+len(r.Deny))
	block = append(append(block, e.listDomains...), r.Deny...)
	e.mu.RUnlock()
	e.handler.SetMatcher(blocklist.NewMatcher(block, r.Allow))
}

// Rules returns the user-defined rules.
func (e *Engine) Rules() rules.Rules { return e.rules.Rules() }

// AddRule adds a user rule and applies it immediately.
func (e *Engine) AddRule(kind rules.Kind, domain string) (rules.Rules, error) {
	r, err := e.rules.Add(kind, domain)
	if err != nil {
		return rules.Rules{}, err
	}
	e.rebuild()
	return r, nil
}

// RemoveRule removes a user rule and applies the change immediately.
func (e *Engine) RemoveRule(kind rules.Kind, domain string) (rules.Rules, error) {
	r, err := e.rules.Remove(kind, domain)
	if err != nil {
		return rules.Rules{}, err
	}
	e.rebuild()
	return r, nil
}

// SetEnabled toggles blocking.
func (e *Engine) SetEnabled(on bool) { e.handler.SetEnabled(on) }

// Stats returns aggregate stats with top-n lists.
func (e *Engine) Stats(n int) stats.Snapshot { return e.stats.Snapshot(n) }

// Recent returns the query log, newest first.
func (e *Engine) Recent() []stats.Query { return e.stats.Recent() }

// Status returns engine status.
func (e *Engine) Status() Status {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return Status{
		Enabled:      e.handler.Enabled(),
		Sources:      slices.Clone(e.sources),
		ListDomains:  len(e.listDomains),
		TotalBlocked: e.handler.Matcher().Size(),
		CacheEntries: e.cache.Len(),
		LastRefresh:  e.lastRefresh,
		LastError:    e.lastError,
	}
}

// RunRefreshLoop refreshes lists every interval until ctx is cancelled.
func (e *Engine) RunRefreshLoop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := e.Refresh(ctx); err != nil {
				e.log.Error("scheduled refresh failed", "err", err)
			}
		}
	}
}
