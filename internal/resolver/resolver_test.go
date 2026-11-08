package resolver

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/zihado/hole/internal/blocklist"
	"github.com/zihado/hole/internal/cache"
	"github.com/zihado/hole/internal/stats"
)

// startDNS runs h on a random local UDP port and returns its address.
func startDNS(t *testing.T, h dns.Handler) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	started := make(chan struct{})
	srv := &dns.Server{PacketConn: pc, Handler: h, NotifyStartedFunc: func() { close(started) }}
	go func() { _ = srv.ActivateAndServe() }()
	<-started
	t.Cleanup(func() { _ = srv.Shutdown() })
	return pc.LocalAddr().String()
}

// fakeUpstream answers every A query with 9.9.9.9 and counts requests.
func fakeUpstream(t *testing.T) (string, *atomic.Int32) {
	var hits atomic.Int32
	addr := startDNS(t, dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		hits.Add(1)
		m := new(dns.Msg)
		m.SetReply(r)
		rr, _ := dns.NewRR(r.Question[0].Name + " 300 IN A 9.9.9.9")
		m.Answer = []dns.RR{rr}
		_ = w.WriteMsg(m)
	}))
	return addr, &hits
}

type fixture struct {
	handler *Handler
	addr    string
	hits    *atomic.Int32
	stats   *stats.Recorder
}

func newFixture(t *testing.T, block, allow []string, upstreams ...string) fixture {
	t.Helper()
	upAddr, hits := fakeUpstream(t)
	if len(upstreams) == 0 {
		upstreams = []string{upAddr}
	}
	rec := stats.NewRecorder(100)
	h := New(Config{
		Upstreams: upstreams,
		Timeout:   500 * time.Millisecond,
		Cache:     cache.New(100, time.Now),
		Stats:     rec,
		Matcher:   blocklist.NewMatcher(block, allow),
	})
	return fixture{handler: h, addr: startDNS(t, h), hits: hits, stats: rec}
}

func query(t *testing.T, addr, name string, qtype uint16) *dns.Msg {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	c := &dns.Client{Timeout: 2 * time.Second}
	resp, _, err := c.Exchange(m, addr)
	if err != nil {
		t.Fatalf("exchange %s: %v", name, err)
	}
	if resp.Id != m.Id {
		t.Fatalf("response id %d != request id %d", resp.Id, m.Id)
	}
	return resp
}

func firstIP(t *testing.T, m *dns.Msg) string {
	t.Helper()
	for _, rr := range m.Answer {
		switch rr := rr.(type) {
		case *dns.A:
			return rr.A.String()
		case *dns.AAAA:
			return rr.AAAA.String()
		}
	}
	t.Fatalf("no A/AAAA answer in %v", m)
	return ""
}

func TestHandler_BlocksListedDomain(t *testing.T) {
	f := newFixture(t, []string{"ads.com"}, nil)

	if ip := firstIP(t, query(t, f.addr, "x.ads.com", dns.TypeA)); ip != "0.0.0.0" {
		t.Errorf("A answer = %s, want 0.0.0.0", ip)
	}
	if ip := firstIP(t, query(t, f.addr, "ads.com", dns.TypeAAAA)); ip != "::" {
		t.Errorf("AAAA answer = %s, want ::", ip)
	}
	if resp := query(t, f.addr, "ads.com", dns.TypeTXT); len(resp.Answer) != 0 || resp.Rcode != dns.RcodeSuccess {
		t.Errorf("TXT for blocked domain should be empty NOERROR, got %v", resp)
	}
	if f.hits.Load() != 0 {
		t.Errorf("blocked queries must not reach upstream, got %d hits", f.hits.Load())
	}
	if s := f.stats.Snapshot(1); s.Blocked != 3 {
		t.Errorf("Blocked = %d, want 3", s.Blocked)
	}
}

func TestHandler_ForwardsAndCaches(t *testing.T) {
	f := newFixture(t, []string{"ads.com"}, nil)

	if ip := firstIP(t, query(t, f.addr, "good.com", dns.TypeA)); ip != "9.9.9.9" {
		t.Errorf("answer = %s, want 9.9.9.9", ip)
	}
	_ = query(t, f.addr, "good.com", dns.TypeA)

	if f.hits.Load() != 1 {
		t.Errorf("upstream hits = %d, want 1 (second served from cache)", f.hits.Load())
	}
	s := f.stats.Snapshot(1)
	if s.Forwarded != 1 || s.Cached != 1 {
		t.Errorf("stats = %+v, want 1 forwarded and 1 cached", s)
	}
}

func TestHandler_AllowlistBypassesBlock(t *testing.T) {
	f := newFixture(t, []string{"example.com"}, []string{"ok.example.com"})

	if ip := firstIP(t, query(t, f.addr, "ok.example.com", dns.TypeA)); ip != "9.9.9.9" {
		t.Errorf("allowlisted answer = %s, want 9.9.9.9", ip)
	}
}

func TestHandler_DisabledForwardsEverything(t *testing.T) {
	f := newFixture(t, []string{"ads.com"}, nil)
	f.handler.SetEnabled(false)

	if ip := firstIP(t, query(t, f.addr, "ads.com", dns.TypeA)); ip != "9.9.9.9" {
		t.Errorf("answer while disabled = %s, want 9.9.9.9", ip)
	}
}

func TestHandler_SetMatcherSwapsRules(t *testing.T) {
	f := newFixture(t, nil, nil)
	f.handler.SetMatcher(blocklist.NewMatcher([]string{"new.com"}, nil))

	if ip := firstIP(t, query(t, f.addr, "new.com", dns.TypeA)); ip != "0.0.0.0" {
		t.Errorf("answer = %s, want 0.0.0.0 after matcher swap", ip)
	}
}

func TestHandler_FallsBackToNextUpstream(t *testing.T) {
	good, hits := fakeUpstream(t)
	dead := deadAddr(t)
	f := newFixture(t, nil, nil, dead, good)

	if ip := firstIP(t, query(t, f.addr, "good.com", dns.TypeA)); ip != "9.9.9.9" {
		t.Errorf("answer = %s, want 9.9.9.9 from fallback", ip)
	}
	if hits.Load() != 1 {
		t.Errorf("fallback upstream hits = %d, want 1", hits.Load())
	}
}

func TestHandler_ServfailWhenAllUpstreamsFail(t *testing.T) {
	f := newFixture(t, nil, nil, deadAddr(t))

	resp := query(t, f.addr, "good.com", dns.TypeA)
	if resp.Rcode != dns.RcodeServerFailure {
		t.Errorf("rcode = %s, want SERVFAIL", dns.RcodeToString[resp.Rcode])
	}
	if s := f.stats.Snapshot(1); s.Errors != 1 {
		t.Errorf("Errors = %d, want 1", s.Errors)
	}
}

// cnameUpstream answers every query with "<name> CNAME <target>" plus an A
// record for target, like a CDN-cloaked ad server.
func cnameUpstream(t *testing.T, target string) string {
	return startDNS(t, dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		cname, _ := dns.NewRR(r.Question[0].Name + " 600 IN CNAME " + dns.Fqdn(target))
		a, _ := dns.NewRR(dns.Fqdn(target) + " 30 IN A 121.53.223.45")
		m.Answer = []dns.RR{cname, a}
		_ = w.WriteMsg(m)
	}))
}

func TestHandler_BlocksCNAMECloakedDomain(t *testing.T) {
	up := cnameUpstream(t, "display-ad.cdn.example")
	f := newFixture(t, []string{"display-ad.cdn.example"}, nil, up)

	resp := query(t, f.addr, "serv.ds.example.com", dns.TypeA)
	if ip := firstIP(t, resp); ip != "0.0.0.0" {
		t.Errorf("answer = %s, want 0.0.0.0 for CNAME pointing at blocked domain", ip)
	}
	if s := f.stats.Snapshot(1); s.Blocked != 1 {
		t.Errorf("Blocked = %d, want 1", s.Blocked)
	}

	// Second lookup must stay blocked (not served from cache as the real answer).
	if ip := firstIP(t, query(t, f.addr, "serv.ds.example.com", dns.TypeA)); ip != "0.0.0.0" {
		t.Errorf("repeat answer = %s, want 0.0.0.0", ip)
	}
}

func TestHandler_CNAMEToAllowedDomainPassesThrough(t *testing.T) {
	up := cnameUpstream(t, "cdn.example")
	f := newFixture(t, []string{"ads.example"}, nil, up)

	if ip := firstIP(t, query(t, f.addr, "www.example.com", dns.TypeA)); ip != "121.53.223.45" {
		t.Errorf("answer = %s, want upstream IP for harmless CNAME", ip)
	}
}

func TestHandler_CNAMEInspectionRespectsDisabled(t *testing.T) {
	up := cnameUpstream(t, "display-ad.cdn.example")
	f := newFixture(t, []string{"display-ad.cdn.example"}, nil, up)
	f.handler.SetEnabled(false)

	if ip := firstIP(t, query(t, f.addr, "serv.ds.example.com", dns.TypeA)); ip != "121.53.223.45" {
		t.Errorf("answer = %s, want upstream IP while blocking disabled", ip)
	}
}

// deadAddr returns a UDP address nobody is listening on.
func deadAddr(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := pc.LocalAddr().String()
	_ = pc.Close()
	return addr
}
