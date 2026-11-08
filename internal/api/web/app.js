"use strict";

const POLL_MS = 3000;
const $ = (id) => document.getElementById(id);
const fmt = new Intl.NumberFormat("ko-KR");

function token() {
  try { return localStorage.getItem("hole-token") || ""; } catch { return ""; }
}

async function api(path, options = {}) {
  const headers = { "Content-Type": "application/json" };
  const t = token();
  if (t) headers.Authorization = "Bearer " + t;
  const res = await fetch(path, { ...options, headers });
  if (res.status === 401) {
    const entered = prompt("API 토큰을 입력하세요");
    if (entered) {
      try { localStorage.setItem("hole-token", entered); } catch {}
      return api(path, options);
    }
  }
  const body = await res.json();
  if (!body.success) throw new Error(body.error || res.statusText);
  return body.data;
}

function toast(msg) {
  const el = $("toast");
  el.textContent = msg;
  el.style.display = "block";
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => (el.style.display = "none"), 2500);
}

function cell(text, cls) {
  const td = document.createElement("td");
  td.textContent = text;
  if (cls) td.className = cls;
  return td;
}

function renderRows(table, headers, rows, emptyText = "아직 데이터 없음") {
  table.replaceChildren();
  if (rows.length === 0) {
    const tr = document.createElement("tr");
    const td = cell(emptyText, "empty");
    td.colSpan = headers.length;
    tr.append(td);
    table.append(tr);
    return;
  }
  const head = document.createElement("tr");
  headers.forEach((h) => { const th = document.createElement("th"); th.textContent = h; head.append(th); });
  table.append(head);
  rows.forEach((cells) => { const tr = document.createElement("tr"); tr.append(...cells); table.append(tr); });
}

function renderTop(table, items, label = "도메인") {
  renderRows(table, [label, "횟수"], (items || []).map((c) => [cell(c.name, "domain"), cell(fmt.format(c.count), "num")]));
}

function renderStatus(st) {
  $("dot").className = "dot " + (st.enabled ? "on" : "off");
  $("state").textContent = st.enabled ? "차단 중" : "차단 꺼짐";
  $("toggle").textContent = st.enabled ? "차단 끄기" : "차단 켜기";
  $("toggle").dataset.enabled = String(st.enabled);
  $("t-domains").textContent = fmt.format(st.total_blocked_domains);
  const last = st.last_refresh && !st.last_refresh.startsWith("0001") ? new Date(st.last_refresh).toLocaleString("ko-KR") : "아직 없음";
  $("meta").textContent = `목록 마지막 갱신: ${last} · 캐시 ${fmt.format(st.cache_entries)}개` + (st.last_error ? ` · 오류: ${st.last_error}` : "");
}

function renderStats(s) {
  $("t-total").textContent = fmt.format(s.total);
  $("t-blocked").textContent = fmt.format(s.blocked);
  $("t-pct").textContent = s.blocked_percent.toFixed(1) + "%";
  $("t-cached").textContent = fmt.format(s.cached);
  renderTop($("top-blocked"), s.top_blocked);
  renderTop($("top-allowed"), s.top_allowed);
  renderTop($("top-clients"), s.top_clients, "클라이언트");
}

function ruleButton(q) {
  const td = cell("", "action");
  if (q.domain.endsWith(".arpa")) return td; // reverse lookups, not worth a rule
  const blocked = q.status === "blocked";
  const btn = document.createElement("button");
  btn.className = blocked ? "small" : "small danger";
  btn.textContent = blocked ? "허용 추가" : "차단 추가";
  btn.title = blocked ? `${q.domain} 허용 규칙 추가` : `${q.domain} 차단 규칙 추가`;
  btn.onclick = () => addRule(blocked ? "allow" : "deny", q.domain);
  td.append(btn);
  return td;
}

function renderQueries(qs) {
  const table = $("queries");
  // Don't swap rows out from under the pointer: a click would land on a stale row.
  if (table.matches(":hover")) return;
  renderRows(table, ["시간", "도메인", "타입", "클라이언트", "결과", "ms", ""], qs.map((q) => {
    const status = cell("");
    const pill = document.createElement("span");
    pill.className = "pill " + q.status;
    pill.textContent = q.status;
    status.append(pill);
    return [cell(new Date(q.time).toLocaleTimeString("ko-KR")), cell(q.domain, "domain"), cell(q.type),
            cell(q.client), status, cell(q.latency_ms.toFixed(1), "num"), ruleButton(q)];
  }));
}

async function addRule(kind, domain) {
  try {
    renderRules(await api("/api/rules", { method: "POST", body: JSON.stringify({ kind, domain }) }));
    toast(`${domain} ${kind === "deny" ? "차단" : "허용"} 규칙 추가 · 기기 DNS 캐시가 남아 있으면 몇 분 뒤 적용`);
    return true;
  } catch (e) {
    toast("실패: " + e.message);
    return false;
  }
}

function renderRules(r) {
  const rows = [];
  for (const [kind, label] of [["deny", "차단"], ["allow", "허용"]]) {
    for (const d of r[kind]) {
      const del = document.createElement("button");
      del.className = "danger";
      del.textContent = "삭제";
      del.onclick = () => removeRule(kind, d);
      const actions = cell("");
      actions.append(del);
      rows.push([cell(label), cell(d, "domain"), actions]);
    }
  }
  renderRows($("rules"), ["종류", "도메인", ""], rows, "추가한 규칙이 없습니다");
}

async function removeRule(kind, domain) {
  try {
    const params = new URLSearchParams({ kind, domain });
    renderRules(await api("/api/rules?" + params, { method: "DELETE" }));
    toast(`${domain} 규칙 삭제`);
  } catch (e) { toast("실패: " + e.message); }
}

async function poll() {
  try {
    const [st, s, qs] = await Promise.all([api("/api/status"), api("/api/stats"), api("/api/queries?limit=100")]);
    renderStatus(st); renderStats(s); renderQueries(qs);
  } catch (e) {
    $("state").textContent = "서버 연결 실패";
    $("dot").className = "dot off";
  }
}

$("toggle").onclick = async () => {
  try {
    const enabled = $("toggle").dataset.enabled !== "true";
    renderStatus(await api("/api/status", { method: "POST", body: JSON.stringify({ enabled }) }));
  } catch (e) { toast("실패: " + e.message); }
};

$("refresh").onclick = async () => {
  toast("목록 갱신 중…");
  try { renderStatus(await api("/api/lists/refresh", { method: "POST" })); toast("목록 갱신 완료"); }
  catch (e) { toast("실패: " + e.message); }
};

$("rule-form").onsubmit = async (ev) => {
  ev.preventDefault();
  const domain = $("rule-domain").value.trim();
  if (await addRule($("rule-kind").value, domain)) $("rule-domain").value = "";
};

api("/api/rules").then(renderRules).catch(() => {});
poll();
setInterval(poll, POLL_MS);

// --- Browser block test -------------------------------------------------
// A blocked host resolves to 0.0.0.0, so the connection fails at once. An opaque
// (no-cors) success means the request left the machine: the browser bypassed Hole.
const PROBE_TIMEOUT_MS = 4000;

async function reach(host) {
  const ctl = new AbortController();
  const timer = setTimeout(() => ctl.abort(), PROBE_TIMEOUT_MS);
  try {
    await fetch(`https://${host}/?hole-test=${Date.now()}`, { mode: "no-cors", cache: "no-store", signal: ctl.signal });
    return "loaded";
  } catch (e) {
    return e.name === "AbortError" ? "timeout" : "failed";
  } finally {
    clearTimeout(timer);
  }
}

function pill(text, cls) {
  const td = document.createElement("td");
  const span = document.createElement("span");
  span.className = "pill " + cls;
  span.textContent = text;
  td.append(span);
  return td;
}

async function runBlockTest() {
  const button = $("run-test");
  const summary = $("test-summary");
  const hint = $("test-hint");
  button.disabled = true;
  summary.className = "";
  summary.textContent = "테스트 중…";
  hint.textContent = "";
  try {
    const { control, probes } = await api("/api/probes");
    const [controlResult, ...results] = await Promise.all([control, ...probes.map((p) => p.host)].map(reach));
    const passed = probes.filter((_, i) => results[i] === "loaded");
    renderRows($("test-results"), ["분류", "대상", "결과"], probes.map((p, i) => [
      cell(p.category),
      cell(`${p.name} (${p.host})`, "domain"),
      results[i] === "loaded" ? pill("통과됨", "pass") : pill("차단됨", "stopped"),
    ]));
    if (controlResult !== "loaded") {
      summary.className = "bad";
      summary.textContent = "인터넷에 연결되지 않아 테스트할 수 없습니다.";
      hint.textContent = "일반 사이트도 열리지 않습니다. 네트워크 연결을 확인하세요.";
    } else if (passed.length === 0) {
      summary.className = "good";
      summary.textContent = `✓ ${probes.length}개 모두 차단됨. 이 브라우저는 Hole로 보호되고 있습니다.`;
    } else {
      summary.className = "bad";
      summary.textContent = `✗ ${probes.length}개 중 ${passed.length}개가 차단되지 않았습니다.`;
      hint.textContent = "이 브라우저가 Hole을 우회하고 있습니다. 보안 DNS(DoH)와 iCloud 비공개 릴레이를 끄고 브라우저를 다시 시작하세요. 터미널에서 hole doctor 로 원인을 볼 수 있습니다.";
    }
  } catch (e) {
    summary.className = "bad";
    summary.textContent = "테스트 실패: " + e.message;
  } finally {
    button.disabled = false;
  }
}

$("run-test").addEventListener("click", runBlockTest);
runBlockTest();
