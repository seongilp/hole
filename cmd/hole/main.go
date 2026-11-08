// Command hole is a network-wide ad blocking DNS sinkhole.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/miekg/dns"

	"github.com/zihado/hole/internal/api"
	"github.com/zihado/hole/internal/cache"
	"github.com/zihado/hole/internal/engine"
	"github.com/zihado/hole/internal/lists"
	"github.com/zihado/hole/internal/resolver"
	"github.com/zihado/hole/internal/rules"
	"github.com/zihado/hole/internal/stats"
)

type options struct {
	dnsAddr   string
	httpAddr  string
	upstreams []string
	sources   []string
	dataDir   string
	refresh   time.Duration
	cacheSize int
	logSize   int
	token     string
}

func parseFlags() (options, error) {
	var o options
	var upstreams, sources string
	flag.StringVar(&o.dnsAddr, "dns", ":53", "DNS listen address (UDP and TCP)")
	flag.StringVar(&o.httpAddr, "http", "127.0.0.1:8080", "dashboard/API listen address")
	flag.StringVar(&upstreams, "upstream", "1.1.1.1:53,8.8.8.8:53", "comma-separated upstream DNS servers, tried in order")
	flag.StringVar(&sources, "lists", strings.Join(lists.DefaultSources, ","), "comma-separated block list URLs or file paths")
	flag.StringVar(&o.dataDir, "data", "data", "directory for persisted user rules")
	flag.DurationVar(&o.refresh, "refresh", 24*time.Hour, "block list refresh interval")
	flag.IntVar(&o.cacheSize, "cache", 10000, "max cached DNS responses")
	flag.IntVar(&o.logSize, "log-size", 1000, "number of recent queries kept for the query log")
	flag.Parse()

	o.upstreams = splitList(upstreams)
	o.sources = splitList(sources)
	o.token = os.Getenv("HOLE_API_TOKEN")

	if len(o.upstreams) == 0 {
		return o, errors.New("at least one -upstream is required")
	}
	if o.refresh < time.Minute {
		return o, errors.New("-refresh must be at least 1m")
	}
	if o.cacheSize < 1 || o.logSize < 1 {
		return o, errors.New("-cache and -log-size must be positive")
	}
	return o, nil
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	opts, err := parseFlags()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		flag.Usage()
		os.Exit(2)
	}
	if err := run(opts, logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(o options, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := rules.Open(filepath.Join(o.dataDir, "rules.json"))
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}
	rec := stats.NewRecorder(o.logSize)
	c := cache.New(o.cacheSize, time.Now)
	handler := resolver.New(resolver.Config{
		Upstreams: o.upstreams,
		Timeout:   3 * time.Second,
		Cache:     c,
		Stats:     rec,
		Logger:    logger,
	})
	eng := engine.New(engine.Config{
		Handler: handler,
		Rules:   store,
		Fetcher: lists.NewFetcher(&http.Client{Timeout: time.Minute}),
		Sources: o.sources,
		Stats:   rec,
		Cache:   c,
		Logger:  logger,
	})

	// Serve DNS immediately; lists load in the background so startup is fast.
	go func() {
		if err := eng.Refresh(ctx); err != nil {
			logger.Error("initial block list load failed", "err", err)
		}
	}()
	go eng.RunRefreshLoop(ctx, o.refresh)

	errCh := make(chan error, 3)
	dnsServers := []*dns.Server{
		{Addr: o.dnsAddr, Net: "udp", Handler: handler},
		{Addr: o.dnsAddr, Net: "tcp", Handler: handler},
	}
	for _, s := range dnsServers {
		go func(s *dns.Server) {
			if err := s.ListenAndServe(); err != nil {
				errCh <- fmt.Errorf("dns %s %s: %w", s.Net, s.Addr, err)
			}
		}(s)
	}

	httpSrv := &http.Server{
		Addr:              o.httpAddr,
		Handler:           api.New(eng, o.token),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http %s: %w", o.httpAddr, err)
		}
	}()

	logger.Info("hole started", "dns", o.dnsAddr, "dashboard", "http://"+o.httpAddr,
		"upstreams", o.upstreams, "auth", o.token != "")

	var runErr error
	select {
	case <-ctx.Done():
		logger.Info("shutting down")
	case runErr = <-errCh:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	for _, s := range dnsServers {
		_ = s.ShutdownContext(shutdownCtx)
	}
	return runErr
}
