package doctor

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// fakeDNS answers BlockedProbe with blockedIP and everything else with 93.184.216.34.
func fakeDNS(t *testing.T, blockedIP string) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		ip := "93.184.216.34"
		if r.Question[0].Name == dns.Fqdn(BlockedProbe) {
			ip = blockedIP
		}
		rr, _ := dns.NewRR(r.Question[0].Name + " 60 IN A " + ip)
		m.Answer = append(m.Answer, rr)
		_ = w.WriteMsg(m)
	})}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })
	return pc.LocalAddr().String()
}

func deadAddr(t *testing.T) string {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := pc.LocalAddr().String()
	pc.Close()
	return addr
}

func apiServer(t *testing.T, body string) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/status" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

const healthyStatus = `{"success":true,"data":{"enabled":true,"total_blocked_domains":78000,"last_refresh":"2026-11-08T10:00:00Z"}}`

func baseConfig(t *testing.T) Config {
	dnsAddr := fakeDNS(t, "0.0.0.0")
	return Config{
		DNSAddr:           dnsAddr,
		HTTPAddr:          apiServer(t, healthyStatus),
		Upstreams:         []string{fakeDNS(t, "1.2.3.4")},
		Home:              t.TempDir(),
		Timeout:           500 * time.Millisecond,
		SystemNameservers: func(context.Context) ([]string, error) { return []string{"127.0.0.1"}, nil },
		SystemLookup:      func(context.Context, string) ([]string, error) { return []string{"0.0.0.0"}, nil },
	}
}

func levels(rs []Result) map[string]Level {
	out := map[string]Level{}
	for _, r := range rs {
		out[strings.SplitN(r.Name, " (", 2)[0]] = r.Level
	}
	return out
}

func TestRun_AllHealthy(t *testing.T) {
	rs := Run(context.Background(), baseConfig(t))
	for _, r := range rs {
		if r.Level != OK {
			t.Errorf("%s: level %d (%s)", r.Name, r.Level, r.Detail)
		}
	}
	var buf bytes.Buffer
	if !Print(&buf, rs) || !strings.Contains(buf.String(), "문제 없음") {
		t.Fatalf("expected healthy output, got:\n%s", buf.String())
	}
}

func TestRun_ServerDown(t *testing.T) {
	cfg := baseConfig(t)
	cfg.DNSAddr = deadAddr(t)
	cfg.HTTPAddr = "127.0.0.1:1"
	got := levels(Run(context.Background(), cfg))
	if got["Hole DNS 서버"] != Fail || got["대시보드/API"] != Fail {
		t.Fatalf("want server+API Fail, got %v", got)
	}
	var buf bytes.Buffer
	if Print(&buf, Run(context.Background(), cfg)) {
		t.Fatal("Print must report unhealthy")
	}
}

func TestCheckDNSServer_NotBlocking(t *testing.T) {
	cfg := baseConfig(t)
	cfg.DNSAddr = fakeDNS(t, "142.250.1.1")
	if r := checkDNSServer(cfg); r.Level != Warn || r.Fix == "" {
		t.Fatalf("want Warn with fix, got %+v", r)
	}
}

func TestCheckAPI_DisabledAndEmpty(t *testing.T) {
	cfg := baseConfig(t)
	cfg.HTTPClient = http.DefaultClient
	cfg.HTTPAddr = apiServer(t, `{"success":true,"data":{"enabled":false,"total_blocked_domains":5}}`)
	if r := checkAPI(context.Background(), cfg); r.Level != Warn || !strings.Contains(r.Detail, "꺼져") {
		t.Fatalf("disabled: %+v", r)
	}
	cfg.HTTPAddr = apiServer(t, `{"success":true,"data":{"enabled":true,"total_blocked_domains":0,"last_error":"offline"}}`)
	if r := checkAPI(context.Background(), cfg); r.Level != Warn || !strings.Contains(r.Detail, "offline") {
		t.Fatalf("empty: %+v", r)
	}
	cfg.HTTPAddr = apiServer(t, `<html>not hole</html>`)
	if r := checkAPI(context.Background(), cfg); r.Level != Fail {
		t.Fatalf("foreign server: %+v", r)
	}
}

func TestCheckUpstreams(t *testing.T) {
	cfg := baseConfig(t)
	cfg.Upstreams = []string{deadAddr(t), fakeDNS(t, "1.1.1.1")}
	if r := checkUpstreams(cfg); r.Level != Warn {
		t.Fatalf("partial: %+v", r)
	}
	cfg.Upstreams = []string{deadAddr(t)}
	if r := checkUpstreams(cfg); r.Level != Fail {
		t.Fatalf("none: %+v", r)
	}
}

func TestCheckSystemDNS(t *testing.T) {
	cfg := baseConfig(t)
	cfg.SystemNameservers = func(context.Context) ([]string, error) { return []string{"192.168.0.1"}, nil }
	if r := checkSystemDNS(context.Background(), cfg); r.Level != Fail || !strings.Contains(r.Fix, "DNS를 Hole로") {
		t.Fatalf("ISP DNS: %+v", r)
	}
	cfg.SystemNameservers = func(context.Context) ([]string, error) { return nil, errors.New("boom") }
	if r := checkSystemDNS(context.Background(), cfg); r.Level != Warn {
		t.Fatalf("error: %+v", r)
	}
}

func TestCheckSystemResolution_Cached(t *testing.T) {
	cfg := baseConfig(t)
	cfg.SystemLookup = func(context.Context, string) ([]string, error) { return []string{"142.250.1.1"}, nil }
	if r := checkSystemResolution(context.Background(), cfg); r.Level != Fail || !strings.Contains(r.Fix, "mDNSResponder") {
		t.Fatalf("got %+v", r)
	}
}

func TestParseScutil(t *testing.T) {
	out := `DNS configuration

resolver #1
  search domain[0] : lan
  nameserver[0] : 127.0.0.1
  nameserver[1] : ::1
  flags    : Request A records

resolver #2
  domain   : local
  nameserver[0] : 8.8.8.8
`
	got := parseScutil(out)
	if len(got) != 2 || got[0] != "127.0.0.1" || got[1] != "::1" {
		t.Fatalf("got %v", got)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCheckBrowsers(t *testing.T) {
	home := t.TempDir()
	support := filepath.Join(home, "Library", "Application Support")
	write(t, filepath.Join(support, "Google/Chrome/Local State"),
		`{"dns_over_https":{"mode":"secure","templates":"https://dns.google/dns-query"}}`)
	write(t, filepath.Join(support, "BraveSoftware/Brave-Browser/Local State"), `{"dns_over_https":{"mode":"automatic"}}`)
	write(t, filepath.Join(support, "Firefox/Profiles/abc.default/prefs.js"), `user_pref("network.trr.mode", 3);`)

	got := map[string]Result{}
	for _, r := range checkBrowsers(home) {
		got[r.Name] = r
	}
	if len(got) != 3 {
		t.Fatalf("want Chrome, Brave, Firefox only (Edge/Arc not installed), got %v", got)
	}
	if got["Chrome 보안 DNS"].Level != Fail || got["Brave 보안 DNS"].Level != OK || got["Firefox DNS over HTTPS"].Level != Fail {
		t.Fatalf("got %+v", got)
	}
}
