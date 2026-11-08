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

function renderQueries(qs) {
  renderRows($("queries"), ["시간", "도메인", "타입", "클라이언트", "결과", "ms"], qs.map((q) => {
    const status = cell("");
    const pill = document.createElement("span");
    pill.className = "pill " + q.status;
    pill.textContent = q.status;
    status.append(pill);
    return [cell(new Date(q.time).toLocaleTimeString("ko-KR")), cell(q.domain, "domain"), cell(q.type),
            cell(q.client), status, cell(q.latency_ms.toFixed(1), "num")];
  }));
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
  try {
    renderRules(await api("/api/rules", { method: "POST", body: JSON.stringify({ kind: $("rule-kind").value, domain }) }));
    $("rule-domain").value = "";
    toast(`${domain} 추가됨`);
  } catch (e) { toast("실패: " + e.message); }
};

api("/api/rules").then(renderRules).catch(() => {});
poll();
setInterval(poll, POLL_MS);
