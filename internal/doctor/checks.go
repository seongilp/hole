package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/miekg/dns"
)

func query(addr, name string, timeout time.Duration) (*dns.Msg, error) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), dns.TypeA)
	c := &dns.Client{Timeout: timeout}
	resp, _, err := c.Exchange(m, addr)
	return resp, err
}

func firstA(m *dns.Msg) string {
	for _, rr := range m.Answer {
		if a, ok := rr.(*dns.A); ok {
			return a.A.String()
		}
	}
	return ""
}

func checkDNSServer(cfg Config) Result {
	r := Result{Name: "Hole DNS 서버 (" + cfg.DNSAddr + ")"}
	blocked, err := query(cfg.DNSAddr, BlockedProbe, cfg.Timeout)
	if err != nil {
		r.Level, r.Detail = Fail, "응답 없음: "+err.Error()
		r.Fix = "Hole 앱을 실행하거나 메뉴에서 '내장 서버 시작'을 누르세요. 로그: ~/Library/Logs/Hole/hole.log"
		return r
	}
	if ip := firstA(blocked); ip != "0.0.0.0" {
		r.Level, r.Detail = Warn, fmt.Sprintf("%s → %q (차단 안 됨)", BlockedProbe, ip)
		r.Fix = "차단이 꺼져 있거나 목록을 아직 못 받았습니다. 메뉴에서 '차단 켜기' 또는 '차단 목록 갱신'."
		return r
	}
	allowed, err := query(cfg.DNSAddr, AllowedProbe, cfg.Timeout)
	if err != nil || firstA(allowed) == "" {
		r.Level, r.Detail = Fail, "광고는 막지만 일반 도메인이 풀리지 않음"
		r.Fix = "업스트림 DNS 항목을 확인하세요."
		return r
	}
	r.Level, r.Detail = OK, fmt.Sprintf("%s 차단, %s 정상 응답", BlockedProbe, AllowedProbe)
	return r
}

type statusEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Enabled             bool      `json:"enabled"`
		TotalBlockedDomains int       `json:"total_blocked_domains"`
		LastRefresh         time.Time `json:"last_refresh"`
		LastError           string    `json:"last_error"`
	} `json:"data"`
}

func checkAPI(ctx context.Context, cfg Config) Result {
	r := Result{Name: "대시보드/API (" + cfg.HTTPAddr + ")"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+cfg.HTTPAddr+"/api/status", nil)
	if err != nil {
		r.Level, r.Detail = Fail, err.Error()
		return r
	}
	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		r.Level, r.Detail = Fail, "연결 실패: "+err.Error()
		r.Fix = "Hole 서버가 실행 중인지 확인하세요."
		return r
	}
	defer resp.Body.Close()
	var env statusEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil || !env.Success {
		r.Level, r.Detail = Fail, fmt.Sprintf("예상치 못한 응답 (HTTP %d)", resp.StatusCode)
		r.Fix = "이 포트를 다른 프로그램이 쓰고 있지 않은지 확인하세요."
		return r
	}
	d := env.Data
	switch {
	case !d.Enabled:
		r.Level, r.Detail = Warn, "차단이 꺼져 있음"
		r.Fix = "메뉴에서 '차단 켜기' (⌘B)."
	case d.TotalBlockedDomains == 0:
		r.Level, r.Detail = Warn, "차단 목록이 비어 있음"
		r.Fix = "인터넷 연결 후 '차단 목록 갱신' (⌘R)."
		if d.LastError != "" {
			r.Detail += ": " + d.LastError
		}
	default:
		r.Level = OK
		r.Detail = fmt.Sprintf("차단 켜짐, 도메인 %d개", d.TotalBlockedDomains)
		if !d.LastRefresh.IsZero() {
			r.Detail += ", 목록 갱신 " + d.LastRefresh.Local().Format("01-02 15:04")
		}
	}
	return r
}

func checkUpstreams(cfg Config) Result {
	r := Result{Name: "업스트림 DNS"}
	var ok, bad []string
	for _, up := range cfg.Upstreams {
		if m, err := query(up, AllowedProbe, cfg.Timeout); err == nil && firstA(m) != "" {
			ok = append(ok, up)
		} else {
			bad = append(bad, up)
		}
	}
	switch {
	case len(ok) == 0:
		r.Level, r.Detail = Fail, "모두 응답 없음: "+strings.Join(bad, ", ")
		r.Fix = "인터넷 연결이나 방화벽을 확인하세요. 회사망이면 -upstream 으로 사내 DNS를 지정하세요."
	case len(bad) > 0:
		r.Level, r.Detail = Warn, "응답 없음: "+strings.Join(bad, ", ")+" (나머지는 정상)"
	default:
		r.Level, r.Detail = OK, strings.Join(ok, ", ")+" 응답"
	}
	return r
}

func isLocal(ns string) bool {
	ip := net.ParseIP(ns)
	return ip != nil && ip.IsLoopback()
}

func checkSystemDNS(ctx context.Context, cfg Config) Result {
	r := Result{Name: "시스템 DNS 설정"}
	servers, err := cfg.SystemNameservers(ctx)
	if err != nil {
		r.Level, r.Detail = Warn, "확인 불가: "+err.Error()
		return r
	}
	if len(servers) == 0 {
		r.Level, r.Detail = Fail, "DNS 서버 없음"
		return r
	}
	if !isLocal(servers[0]) {
		r.Level, r.Detail = Fail, "광고 차단 안 됨: 이 Mac은 "+strings.Join(servers, ", ")+"를 사용 중"
		r.Fix = "Hole 메뉴에서 '이 Mac의 DNS를 Hole로 지정'을 켜세요."
		return r
	}
	r.Level, r.Detail = OK, "1순위 DNS가 "+servers[0]
	return r
}

func checkSystemResolution(ctx context.Context, cfg Config) Result {
	r := Result{Name: "실제 조회 경로"}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	addrs, err := cfg.SystemLookup(ctx, BlockedProbe)
	if err != nil {
		r.Level, r.Detail = Warn, "조회 실패: "+err.Error()
		return r
	}
	for _, a := range addrs {
		if a != "0.0.0.0" && a != "::" {
			r.Level, r.Detail = Fail, fmt.Sprintf("시스템이 %s 를 %s 로 풀고 있음", BlockedProbe, a)
			r.Fix = "DNS 캐시가 남았을 수 있습니다: sudo killall -HUP mDNSResponder"
			return r
		}
	}
	r.Level, r.Detail = OK, "시스템 조회가 Hole을 거쳐 차단됨"
	return r
}

func systemNameservers(ctx context.Context) ([]string, error) {
	out, err := exec.CommandContext(ctx, "scutil", "--dns").Output()
	if err != nil {
		return nil, err
	}
	return parseScutil(string(out)), nil
}

// parseScutil returns the nameservers of the first resolver in `scutil --dns` output.
func parseScutil(out string) []string {
	var servers []string
	inFirst := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "resolver #") {
			if inFirst {
				break
			}
			inFirst = line == "resolver #1"
			continue
		}
		if inFirst && strings.HasPrefix(line, "nameserver[") {
			if _, v, ok := strings.Cut(line, ":"); ok {
				servers = append(servers, strings.TrimSpace(v))
			}
		}
	}
	return servers
}

func systemLookup(ctx context.Context, host string) ([]string, error) {
	// The cgo resolver goes through mDNSResponder, like browsers and apps do.
	r := &net.Resolver{PreferGo: false}
	return r.LookupHost(ctx, host)
}
