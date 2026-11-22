package doctor

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"unicode"
)

// WARPInfo describes the Cloudflare WARP client, if installed.
type WARPInfo struct {
	Installed bool
	Connected bool
	Mode      string // normalized: "warp", "tunnelonly", "doh", ...
}

// warpStubs are the local DNS proxy addresses WARP installs while it owns DNS.
var warpStubs = map[string]bool{
	"127.0.2.2": true, "127.0.2.3": true,
	"fd01:db8:1111::2": true, "fd01:db8:1111::3": true,
}

const warpFix = "WARP 모드를 'Traffic only'로 바꾸세요: warp-cli mode tunnel_only " +
	"(앱 Mode → Traffic only). 회사가 관리하는 WARP라면 관리자 정책을 따라야 합니다."

// proxiesDNS reports whether a WARP mode takes over the system resolver.
func (w WARPInfo) proxiesDNS() bool {
	return !strings.Contains(w.Mode, "tunnelonly") && !strings.Contains(w.Mode, "proxy")
}

func isWARPStub(ns string) bool { return warpStubs[strings.ToLower(ns)] }

// checkWARP returns nil when WARP is neither installed nor holding DNS.
func checkWARP(ctx context.Context, cfg Config) *Result {
	wctx, cancel := context.WithTimeout(ctx, 2*cfg.Timeout)
	defer cancel()
	info := cfg.WARPStatus(wctx)
	servers, _ := cfg.SystemNameservers(ctx)
	stub := len(servers) > 0 && isWARPStub(servers[0])
	if !info.Installed && !stub {
		return nil
	}
	r := &Result{Name: "Cloudflare WARP"}
	switch {
	case stub:
		r.Level, r.Detail = Fail, "WARP가 DNS를 가로채는 중 ("+servers[0]+")"
		if info.Mode != "" {
			r.Detail += ", 모드: " + info.Mode
		}
		r.Fix = warpFix
	case info.Connected && info.proxiesDNS():
		r.Level = Warn
		r.Detail = "WARP가 DNS 모드(" + info.Mode + ")로 연결됨: Hole과 같이 쓰면 WARP가 'DNS proxy failure'로 끊깁니다"
		r.Fix = warpFix
	case info.Connected:
		r.Level, r.Detail = OK, "Traffic only 모드: Hole과 함께 사용 가능"
	default:
		r.Level, r.Detail = OK, "설치됨, 연결 안 됨"
	}
	return r
}

// warpStatus queries warp-cli; a missing CLI means WARP is not installed.
func warpStatus(ctx context.Context) WARPInfo {
	bin := findWARPCLI()
	if bin == "" {
		return WARPInfo{}
	}
	info := WARPInfo{Installed: true}
	if out, err := exec.CommandContext(ctx, bin, "--accept-tos", "status").Output(); err == nil {
		info.Connected = parseWARPStatus(string(out))
	}
	if out, err := exec.CommandContext(ctx, bin, "--accept-tos", "settings").Output(); err == nil {
		info.Mode = parseWARPMode(string(out))
	}
	return info
}

// warpCLIPaths covers a PATH without /usr/local/bin (e.g. when launched from the app).
var warpCLIPaths = []string{
	"/usr/local/bin/warp-cli",
	"/Applications/Cloudflare WARP.app/Contents/Resources/warp-cli",
}

func findWARPCLI() string {
	if bin, err := exec.LookPath("warp-cli"); err == nil {
		return bin
	}
	for _, p := range warpCLIPaths {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// parseWARPStatus reads `warp-cli status` ("Status update: Connected").
func parseWARPStatus(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		if _, v, ok := strings.Cut(line, "Status update:"); ok {
			return strings.TrimSpace(v) == "Connected"
		}
	}
	return false
}

// parseWARPMode reads the "Mode:" line of `warp-cli settings`, e.g. "(default)\tMode: Warp".
func parseWARPMode(out string) string {
	for _, line := range strings.Split(out, "\n") {
		_, v, ok := strings.Cut(line, "\tMode:")
		if !ok {
			continue
		}
		return strings.Map(func(r rune) rune {
			switch {
			case r == '+':
				return r
			case unicode.IsLetter(r):
				return unicode.ToLower(r)
			}
			return -1
		}, v)
	}
	return ""
}
