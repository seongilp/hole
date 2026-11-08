package api

import "net/http"

// Probe is a host the dashboard's browser test tries to reach. Every probe is on the
// default block lists, so a working setup blocks all of them.
type Probe struct {
	Category string `json:"category"`
	Name     string `json:"name"`
	Host     string `json:"host"`
}

// controlHost must load; if it fails the browser is offline and the test is meaningless.
const controlHost = "www.gstatic.com"

var probes = []Probe{
	{"광고", "Google AdSense", "pagead2.googlesyndication.com"},
	{"광고", "Google 광고 (DoubleClick)", "googleads.g.doubleclick.net"},
	{"광고", "Google Ad Manager", "securepubads.g.doubleclick.net"},
	{"광고", "Google 광고 서비스", "adservice.google.com"},
	{"광고", "카카오 광고 (다음)", "display.ad.daum.net"},
	{"광고", "모비온", "www.mediacategory.com"},
	{"광고", "데이블", "static.dable.io"},
	{"광고", "와이더플래닛", "adtg.widerplanet.com"},
	{"분석", "Google Analytics", "www.google-analytics.com"},
	{"분석", "Firebase Analytics", "app-measurement.com"},
	{"분석", "TikTok", "analytics.tiktok.com"},
	{"분석", "X(Twitter) 광고", "static.ads-twitter.com"},
	{"수집", "카카오 통계 수집", "tr.ds.kakao.com"},
	{"수집", "카카오 앱 수집", "aem-kakao-collector.onkakao.net"},
	{"수집", "Bugsnag 오류 보고", "notify.bugsnag.com"},
}

func (s *server) getProbes(w http.ResponseWriter, _ *http.Request) {
	ok(w, map[string]any{"control": controlHost, "probes": probes})
}

// contentSecurityPolicy lets the dashboard script fetch any HTTPS URL: probe hosts often
// redirect elsewhere, and CSP checks every redirect hop. Scripts, styles and frames stay 'self'.
func contentSecurityPolicy() string {
	return "default-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self' https:"
}
