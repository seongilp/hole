// Package resolver implements the sinkhole DNS handler: blocked domains get a
// null answer, everything else is forwarded upstream and cached.
package resolver

import (
	"fmt"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"

	"github.com/zihado/hole/internal/blocklist"
	"github.com/zihado/hole/internal/cache"
	"github.com/zihado/hole/internal/stats"
)

// blockedTTL is the TTL of synthesized answers for blocked domains.
const blockedTTL = 60

// Config holds the Handler's dependencies.
type Config struct {
	Upstreams []string // "host:port", tried in order
	Timeout   time.Duration
	Cache     *cache.Cache
	Stats     *stats.Recorder
	Matcher   *blocklist.Matcher
	Logger    *slog.Logger
}

// Handler is a dns.Handler that sinkholes blocked domains.
type Handler struct {
	upstreams []string
	udp       *dns.Client
	tcp       *dns.Client
	cache     *cache.Cache
	stats     *stats.Recorder
	log       *slog.Logger
	matcher   atomic.Pointer[blocklist.Matcher]
	enabled   atomic.Bool
}

// New creates a Handler with blocking enabled.
func New(cfg Config) *Handler {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{
		upstreams: cfg.Upstreams,
		udp:       &dns.Client{Net: "udp", Timeout: cfg.Timeout},
		tcp:       &dns.Client{Net: "tcp", Timeout: cfg.Timeout},
		cache:     cfg.Cache,
		stats:     cfg.Stats,
		log:       logger,
	}
	matcher := cfg.Matcher
	if matcher == nil {
		matcher = blocklist.NewMatcher(nil, nil)
	}
	h.matcher.Store(matcher)
	h.enabled.Store(true)
	return h
}

// SetMatcher atomically replaces the active block/allow rules.
func (h *Handler) SetMatcher(m *blocklist.Matcher) { h.matcher.Store(m) }

// Matcher returns the active rules.
func (h *Handler) Matcher() *blocklist.Matcher { return h.matcher.Load() }

// SetEnabled turns blocking on or off. When off, all queries are forwarded.
func (h *Handler) SetEnabled(on bool) { h.enabled.Store(on) }

// Enabled reports whether blocking is on.
func (h *Handler) Enabled() bool { return h.enabled.Load() }

// ServeDNS implements dns.Handler.
func (h *Handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	start := time.Now()
	if len(r.Question) != 1 {
		h.write(w, refuse(r, dns.RcodeFormatError))
		return
	}
	q := r.Question[0]
	resp, status := h.resolve(r, q)
	resp.Id = r.Id
	h.write(w, resp)

	h.stats.Record(stats.Query{
		Time:      start,
		Domain:    blocklist.Normalize(q.Name),
		Type:      dns.TypeToString[q.Qtype],
		Client:    clientIP(w.RemoteAddr()),
		Status:    status,
		LatencyMS: float64(time.Since(start).Microseconds()) / 1000,
	})
}

func (h *Handler) resolve(r *dns.Msg, q dns.Question) (*dns.Msg, stats.Status) {
	if h.Enabled() && h.Matcher().Blocked(q.Name) {
		return blockedReply(r, q), stats.StatusBlocked
	}
	if cached, ok := h.cache.Get(q.Name, q.Qtype); ok {
		if h.cnameBlocked(cached) {
			return blockedReply(r, q), stats.StatusBlocked
		}
		return cached, stats.StatusCached
	}
	resp, err := h.forward(r)
	if err != nil {
		h.log.Warn("upstream lookup failed", "domain", q.Name, "err", err)
		return refuse(r, dns.RcodeServerFailure), stats.StatusError
	}
	// Cache the real answer; the CNAME check runs on every hit so rule
	// changes apply immediately.
	h.cache.Set(q.Name, q.Qtype, resp)
	if h.cnameBlocked(resp) {
		return blockedReply(r, q), stats.StatusBlocked
	}
	return resp, stats.StatusForwarded
}

// cnameBlocked reports whether any CNAME target in the answer is blocked.
// This defeats CNAME cloaking, where an unlisted name (serv.ds.kakao.com)
// aliases a listed ad server (display-ad-*.kgslb.com).
func (h *Handler) cnameBlocked(resp *dns.Msg) bool {
	if !h.Enabled() {
		return false
	}
	m := h.Matcher()
	for _, rr := range resp.Answer {
		if c, ok := rr.(*dns.CNAME); ok && m.Blocked(c.Target) {
			return true
		}
	}
	return false
}

// forward tries each upstream in order, retrying over TCP on truncation.
func (h *Handler) forward(r *dns.Msg) (*dns.Msg, error) {
	var lastErr error
	for _, up := range h.upstreams {
		resp, _, err := h.udp.Exchange(r, up)
		if err == nil && resp.Truncated {
			resp, _, err = h.tcp.Exchange(r, up)
		}
		if err != nil {
			lastErr = fmt.Errorf("upstream %s: %w", up, err)
			continue
		}
		return resp, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no upstreams configured")
	}
	return nil, lastErr
}

// blockedReply answers A with 0.0.0.0, AAAA with ::, and other types with an
// empty NOERROR response (Pi-hole's "NULL" blocking mode).
func blockedReply(r *dns.Msg, q dns.Question) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(r)
	m.RecursionAvailable = true
	hdr := dns.RR_Header{Name: q.Name, Rrtype: q.Qtype, Class: dns.ClassINET, Ttl: blockedTTL}
	switch q.Qtype {
	case dns.TypeA:
		m.Answer = []dns.RR{&dns.A{Hdr: hdr, A: net.IPv4zero}}
	case dns.TypeAAAA:
		m.Answer = []dns.RR{&dns.AAAA{Hdr: hdr, AAAA: net.IPv6zero}}
	}
	return m
}

func refuse(r *dns.Msg, rcode int) *dns.Msg {
	m := new(dns.Msg)
	m.SetRcode(r, rcode)
	m.RecursionAvailable = true
	return m
}

func (h *Handler) write(w dns.ResponseWriter, m *dns.Msg) {
	if err := w.WriteMsg(m); err != nil {
		h.log.Debug("write response failed", "err", err)
	}
}

func clientIP(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}
