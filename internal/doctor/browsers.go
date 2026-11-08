package doctor

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// chromium browsers keep secure DNS settings in "Local State".
var chromiumBrowsers = []struct{ name, dir string }{
	{"Chrome", "Google/Chrome"},
	{"Edge", "Microsoft Edge"},
	{"Brave", "BraveSoftware/Brave-Browser"},
	{"Arc", "Arc/User Data"},
}

func checkBrowsers(home string) []Result {
	if home == "" {
		return nil
	}
	support := filepath.Join(home, "Library", "Application Support")
	var results []Result
	for _, b := range chromiumBrowsers {
		if r, ok := checkChromium(b.name, filepath.Join(support, b.dir, "Local State")); ok {
			results = append(results, r)
		}
	}
	if r, ok := checkFirefox(filepath.Join(support, "Firefox", "Profiles")); ok {
		results = append(results, r)
	}
	return results
}

// checkChromium reports ok=false when the browser is not installed.
func checkChromium(name, localState string) (Result, bool) {
	data, err := os.ReadFile(localState)
	if err != nil {
		return Result{}, false
	}
	var st struct {
		DNSOverHTTPS struct {
			Mode      string `json:"mode"`
			Templates string `json:"templates"`
		} `json:"dns_over_https"`
	}
	r := Result{Name: name + " 보안 DNS"}
	if err := json.Unmarshal(data, &st); err != nil {
		r.Level, r.Detail = Warn, "설정 파일을 읽지 못함"
		return r, true
	}
	if st.DNSOverHTTPS.Mode == "secure" {
		r.Level, r.Detail = Fail, "켜짐 ("+st.DNSOverHTTPS.Templates+"): Hole을 우회합니다"
		r.Fix = name + " 설정 → 개인 정보 보호 및 보안 → 보안 → '보안 DNS 사용' 끄기"
		return r, true
	}
	// "automatic" (default) only upgrades to DoH when the system DNS supports it; 127.0.0.1 does not.
	r.Level, r.Detail = OK, "Hole을 우회하지 않음"
	return r, true
}

func checkFirefox(profiles string) (Result, bool) {
	prefs, _ := filepath.Glob(filepath.Join(profiles, "*", "prefs.js"))
	if len(prefs) == 0 {
		return Result{}, false
	}
	r := Result{Name: "Firefox DNS over HTTPS", Level: OK, Detail: "Hole을 우회하지 않음"}
	for _, p := range prefs {
		if mode := firefoxTRRMode(p); mode == "2" || mode == "3" {
			r.Level, r.Detail = Fail, "켜짐 (network.trr.mode="+mode+"): Hole을 우회합니다"
			r.Fix = "Firefox 설정 → 개인 정보 및 보안 → DNS over HTTPS → '끄기'"
		}
	}
	return r, true
}

func firefoxTRRMode(prefsPath string) string {
	f, err := os.Open(prefsPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, `user_pref("network.trr.mode",`) {
			v := strings.TrimPrefix(line, `user_pref("network.trr.mode",`)
			return strings.Trim(strings.TrimSpace(v), ");\" ")
		}
	}
	return ""
}
