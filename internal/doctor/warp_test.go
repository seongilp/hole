package doctor

import (
	"context"
	"strings"
	"testing"
)

func TestCheckWARP(t *testing.T) {
	tests := []struct {
		name    string
		info    WARPInfo
		servers []string
		want    Level
		absent  bool
	}{
		{name: "not installed", servers: []string{"127.0.0.1"}, absent: true},
		{name: "stub holds DNS", info: WARPInfo{Installed: true, Connected: true, Mode: "warp"}, servers: []string{"127.0.2.2"}, want: Fail},
		{name: "stub without CLI", servers: []string{"FD01:DB8:1111::2"}, want: Fail},
		{name: "DNS mode connected", info: WARPInfo{Installed: true, Connected: true, Mode: "warp"}, servers: []string{"127.0.0.1"}, want: Warn},
		{name: "traffic only", info: WARPInfo{Installed: true, Connected: true, Mode: "warptunnelonly"}, servers: []string{"127.0.0.1"}, want: OK},
		{name: "proxy mode", info: WARPInfo{Installed: true, Connected: true, Mode: "proxy"}, servers: []string{"127.0.0.1"}, want: OK},
		{name: "disconnected", info: WARPInfo{Installed: true, Mode: "warp"}, servers: []string{"127.0.0.1"}, want: OK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig(t)
			cfg.WARPStatus = func(context.Context) WARPInfo { return tt.info }
			cfg.SystemNameservers = func(context.Context) ([]string, error) { return tt.servers, nil }
			r := checkWARP(context.Background(), cfg)
			if tt.absent {
				if r != nil {
					t.Fatalf("want no result, got %+v", r)
				}
				return
			}
			if r == nil || r.Level != tt.want {
				t.Fatalf("want level %d, got %+v", tt.want, r)
			}
			if tt.want != OK && !strings.Contains(r.Fix, "tunnel_only") {
				t.Fatalf("missing fix: %+v", r)
			}
		})
	}
}

func TestCheckSystemDNS_WARPStub(t *testing.T) {
	cfg := baseConfig(t)
	cfg.SystemNameservers = func(context.Context) ([]string, error) { return []string{"127.0.2.2", "127.0.2.3"}, nil }
	if r := checkSystemDNS(context.Background(), cfg); r.Level != Fail || !strings.Contains(r.Detail, "WARP") {
		t.Fatalf("WARP stub must not count as Hole: %+v", r)
	}
}

func TestRun_IncludesWARP(t *testing.T) {
	cfg := baseConfig(t)
	cfg.WARPStatus = func(context.Context) WARPInfo { return WARPInfo{Installed: true, Connected: true, Mode: "warp"} }
	if got := levels(Run(context.Background(), cfg)); got["Cloudflare WARP"] != Warn {
		t.Fatalf("want WARP Warn, got %v", got)
	}
}

func TestParseWARP(t *testing.T) {
	if !parseWARPStatus("Status update: Connected\nNetwork: healthy\n") {
		t.Error("connected not parsed")
	}
	if parseWARPStatus("Status update: Disconnected\nReason: Manual\n") || parseWARPStatus("") {
		t.Error("disconnected parsed as connected")
	}
	settings := "(not set)\tCompliance Environment: Normal\n(default)\tMode: Warp\n(default)\tPacket Tunnel: false\n"
	if got := parseWARPMode(settings); got != "warp" {
		t.Errorf("mode = %q", got)
	}
	if got := parseWARPMode("(user set)\tMode: WarpTunnelOnly\n"); got != "warptunnelonly" {
		t.Errorf("mode = %q", got)
	}
	if got := parseWARPMode("nothing"); got != "" {
		t.Errorf("mode = %q", got)
	}
}
