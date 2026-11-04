"use strict";
(() => {
  const el = id => document.getElementById(id);
  const preview = document.body.dataset.mode === "preview";
  const transitions = {open: ["triaged"], triaged: ["resolved", "open"], resolved: ["closed", "open"], closed: ["open"]};
  const metricNames = {firstAttemptSuccess: "First-attempt deployment", rollbackSuccess: "Rollback success", restoreVerification: "Restore verification", plannerSchemaAfterRetry: "Schema-valid planner output", deterministicDeployments: "Deterministic deployments", falseCompatible: "False-compatible decisions", "secret-exposure": "Secret exposure", "uncited-action": "Uncited material actions", "arbitrary-shell": "Arbitrary shell execution"};
  let csrf = "", epoch = 0, busy = false, expiry = 0, timer = 0, report = null, selected = null;
  const requests = new Set();
  const actionIds = new Map();
  const writes = [...document.querySelectorAll("#import-form input, #import-form button, #ticket-form input, #ticket-form select, #ticket-form button, #transition-form input, #transition-form select, #transition-form button, #toggle-import, #toggle-ticket, #feedback-consent")];
  const text = (id, value) => { el(id).textContent = value; };
  const node = (tag, value, className) => { const n = document.createElement(tag); if (value !== undefined) n.textContent = String(value); if (className) n.className = className; return n; };
  const human = value => String(value || "not-recorded").replaceAll("-", " ");
  const date = value => { const d = new Date(value); return Number.isNaN(d.getTime()) ? "Time unavailable" : d.toLocaleString(); };
  const percent = value => Number.isFinite(value) ? (value * 100).toLocaleString(undefined, {maximumFractionDigits: 1}) + "%" : "Not measured";
  function message(value, state = "idle") { text("alpha-message", value); el("alpha-message").dataset.state = state; }
  function error(value) { el("alpha-error").hidden = !value; text("alpha-error", value || ""); }
  function controls() {
    const writable = Boolean(csrf) && !preview && !busy;
    writes.forEach(control => { control.disabled = !writable; });
    el("alpha-code").disabled = preview || busy || Boolean(csrf);
    el("alpha-login").querySelector("button").disabled = preview || busy || Boolean(csrf);
    el("alpha-reload").disabled = busy || (!preview && !csrf);
    el("alpha-signout").hidden = preview || !csrf;
    el("alpha-signout").disabled = !csrf;
    el("feedback-download").disabled = !writable || !selected || !el("feedback-consent").checked;
    el("transition-form").querySelector("button").disabled = !writable || !selected || !(transitions[selected.state] || []).length;
    el("alpha-content").setAttribute("aria-busy", String(busy));
    document.querySelectorAll(".alpha-view").forEach(view => view.setAttribute("aria-busy", String(busy)));
    document.querySelectorAll("[data-ticket]").forEach(button => { button.disabled = busy; });
  }
  function clearPrivate() {
    report = null; selected = null; actionIds.clear();
    ["coverage-rows", "measure-rows", "observation-rows", "support-rows", "next-ticket-state"].forEach(id => el(id).replaceChildren());
    ["cohort-total", "cohort-ready", "observation-total", "support-total"].forEach(id => text(id, "—"));
    ["ticket-detail-title", "ticket-detail-meta", "sample-note", "ledger-updated", "next-gap"].forEach(id => text(id, ""));
    text("coverage-status", "Evidence needed"); el("coverage-status").dataset.state = "not-measured";
    el("alpha-content").hidden = true;
    ["ticket-detail", "import-form", "ticket-form"].forEach(id => { el(id).hidden = true; });
    ["import-form", "ticket-form", "transition-form", "alpha-login"].forEach(id => el(id).reset());
    el("feedback-consent").checked = false;
  }
  function invalidate(reason, state = "expired") {
    epoch += 1;
    requests.forEach(controller => controller.abort()); requests.clear();
    clearInterval(timer); timer = 0; expiry = 0; csrf = ""; busy = false;
    clearPrivate(); el("alpha-connect").hidden = preview;
    text("alpha-session", state === "expired" ? "Session expired" : "Session closed");
    message(reason, state); error(""); controls();
  }
  function armSession(value) {
    clearInterval(timer);
    if (value.expiresAt) expiry = Date.parse(value.expiresAt);
    else {
      const duration = /^\s*(\d+)\s*(seconds?|minutes?)\s*$/i.exec(value.expiresIn || "");
      expiry = Date.now() + (duration ? Number(duration[1]) * (duration[2].toLowerCase().startsWith("second") ? 1000 : 60000) : 15 * 60000);
    }
    if (!Number.isFinite(expiry)) { invalidate("The session expiry was unavailable. Open a fresh operator session."); return; }
    const tick = () => { if (Date.now() >= expiry) invalidate("Session expired. Open a new operator session to continue."); else text("alpha-session", "Session active · " + Math.max(1, Math.ceil((expiry - Date.now()) / 60000)) + " min left"); };
    tick(); if (csrf) timer = window.setInterval(tick, 10000);
  }
  async function api(path, {body, method, sessionEpoch = epoch} = {}) {
    const controller = new AbortController(); requests.add(controller);
    try {
      const headers = {};
      if (body !== undefined) headers["Content-Type"] = "application/json";
      if ((method && method !== "GET") || body !== undefined) headers["X-Wazi-CSRF"] = csrf;
      const response = await fetch(path, {method: method || (body === undefined ? "GET" : "POST"), headers, body: body === undefined ? undefined : JSON.stringify(body), credentials: "same-origin", cache: "no-store", redirect: "error", signal: controller.signal});
      let value;
      try { value = await response.json(); } catch { throw new Error("The service did not return a JSON response."); }
      if (sessionEpoch !== epoch) throw new DOMException("Session changed", "AbortError");
      if (!response.ok) {
        if (response.status === 401 && path !== "/v1/context-sessions/exchange") {
          if (!csrf && path === "/v1/context-session") invalidate("Connect to load your cohort and measured evidence.", "closed");
          else invalidate("The server ended this session. Open a new operator session.");
        }
        throw new Error([value.error || "Request rejected", value.reason, value.detail].filter(Boolean).join(": "));
      }
      return value;
    } finally { requests.delete(controller); }
  }
  async function run(work, pending) {
    if (busy) return;
    const started = epoch; busy = true; error(""); message(pending, "loading"); controls();
    try { await work(started); }
    catch (problem) { if (started === epoch && problem.name !== "AbortError") { error(problem.message || "The request could not be completed."); message("Could not complete this request. Correct the issue or refresh evidence.", "error"); } }
    finally { if (started === epoch) { busy = false; controls(); } }
  }
  function badge(value, state = value) { const n = node("span", human(value), "wazi-badge"); n.dataset.state = state; return n; }
  function cell(row, label, value) { const td = node("td"); td.dataset.label = label; td.append(value instanceof Node ? value : document.createTextNode(String(value))); row.append(td); return td; }
  function showTicket(ticket) {
    selected = ticket; el("feedback-consent").checked = false;
    text("ticket-detail-title", ticket.ticketId);
    text("ticket-detail-meta", [ticket.applicationId, ticket.severity, human(ticket.state), ticket.releaseDigest].filter(Boolean).join(" · "));
    el("next-ticket-state").replaceChildren(...(transitions[ticket.state] || []).map(value => new Option(human(value), value)));
    el("transition-form").reset(); el("ticket-detail").hidden = false; controls();
  }
  function render(value) {
    if (!value || !Array.isArray(value.cohort) || !value.metrics || typeof value.metrics !== "object") throw new Error("The service did not return an alpha report.");
    report = value;
    const cohort = value.cohort, observations = value.recentObservations || [], tickets = value.supportTickets || [];
    text("cohort-total", cohort.length);
    text("cohort-ready", (value.appsWithSameReleaseLocalAndVpsRecovery || []).length);
    text("observation-total", value.observations);
    text("support-total", (value.support || []).filter(item => ["open", "triaged"].includes(item.state)).reduce((total, item) => total + item.count, 0));
    el("coverage-rows").replaceChildren();
    for (const app of cohort) {
      const row = node("tr"), appName = node("div");
      appName.append(node("span", app.name || app.applicationId, "app-name"), node("span", app.applicationId, "app-id"));
      cell(row, "Application", appName);
      const release = node("span", app.releaseDigest || "No release evidence", "alpha-release");
      if (app.releaseDigest) release.title = app.releaseDigest;
      cell(row, "Release evidence", release);
      for (const target of ["local", "vps"]) cell(row, target === "local" ? "Local" : "VPS", badge(app.targets?.[target] || "not-recorded"));
      el("coverage-rows").append(row);
    }
    el("coverage-empty").hidden = cohort.length > 0;
    text("coverage-status", value.alphaThresholdsMet ? "Measured gates met" : "Evidence needed");
    el("coverage-status").dataset.state = value.alphaThresholdsMet ? "pass" : "not-measured";
    const gap = cohort.find(app => app.targets?.local !== "observed" || app.targets?.vps !== "observed");
    text("next-gap", gap ? (gap.name || gap.applicationId) + " needs matching deployment and restore evidence on " + ["local", "vps"].filter(target => gap.targets?.[target] !== "observed").map(target => target === "vps" ? "VPS" : "local").join(" and ") + "." : cohort.length ? "Recovery coverage is recorded. Check the reliability measures and outstanding support work before declaring the alpha complete." : "Configure the alpha cohort, then record completed or failed jobs from the workbench.");
    text("sample-note", "Minimum " + value.minimumSamples + " observations per measure");
    el("measure-rows").replaceChildren();
    if (!Object.keys(value.metrics).length) el("measure-rows").append(node("p", "No reliability measurements supplied. Load an exported report or connect to the private service.", "wazi-empty"));
    for (const [name, metric] of Object.entries(value.metrics)) {
      const section = node("article", undefined, "alpha-measure");
      section.append(node("h3", metricNames[name] || human(name)));
      const ratio = node("div", undefined, "alpha-measure-main");
      ratio.append(node("strong", percent(metric.rate)), node("span", metric.numerator + " / " + metric.denominator));
      const sign = name === "falseCompatible" ? "Below " : ["secret-exposure", "uncited-action", "arbitrary-shell"].includes(name) ? "At most " : "At least ";
      section.append(ratio, badge(metric.state), node("span", "Target: " + sign.toLowerCase() + percent(metric.threshold), "alpha-measure-target"));
      el("measure-rows").append(section);
    }
    el("observation-rows").replaceChildren();
    for (const observation of observations) {
      const row = node("tr");
      cell(row, "Application", observation.applicationId); cell(row, "Operation", human(observation.kind));
      cell(row, "Result", badge(observation.outcome, ["success", "valid", "clear", "compatible"].includes(observation.outcome) ? "pass" : "fail"));
      cell(row, "Attempt", observation.attempt); cell(row, "Target", observation.target === "vps" ? "VPS" : human(observation.target));
      const time = node("time", date(observation.recordedAt)); if (Number.isFinite(Date.parse(observation.recordedAt))) time.dateTime = observation.recordedAt;
      cell(row, "Captured", time); row.title = [observation.trialId, observation.releaseDigest].filter(Boolean).join(" · ");
      el("observation-rows").append(row);
    }
    el("observation-empty").hidden = observations.length > 0;
    el("support-rows").replaceChildren();
    for (const ticket of tickets) {
      const article = node("article", undefined, "alpha-ticket"), info = node("div"), button = node("button", "View issue", "wazi-button-secondary");
      info.append(node("h3", ticket.applicationId + " · " + human(ticket.category)), node("p", ticket.severity + " · " + ticket.ticketId));
      button.type = "button"; button.dataset.ticket = ticket.ticketId; button.addEventListener("click", () => showTicket(ticket));
      article.append(info, badge(ticket.state), button); el("support-rows").append(article);
    }
    el("support-empty").hidden = tickets.length > 0;
    if (selected) { const updated = tickets.find(ticket => ticket.ticketId === selected.ticketId); if (updated) showTicket(updated); else { selected = null; el("ticket-detail").hidden = true; } }
    text("ledger-updated", value.generatedAt ? (preview ? "Captured preview · " : "Report generated · ") + date(value.generatedAt) : "No evidence snapshot supplied");
    el("alpha-content").hidden = false; controls();
  }
  async function load(started) {
    const value = await api(preview ? "/preview-data" : "/v1/alpha-report", {sessionEpoch: started});
    render(value); message(preview ? value.snapshotSupplied === false ? "Read-only interface preview. No evidence snapshot supplied. Writes are disabled." : "Read-only captured evidence. Writes are disabled." : "Private evidence refreshed. Missing observations remain unmeasured.", "ready");
  }
  function selectView() {
    const requested = new URL(window.location.href).searchParams.get("view");
    const view = ["readiness", "observations", "support"].includes(requested) ? requested : "readiness";
    document.querySelectorAll(".alpha-tabs [data-view]").forEach(link => { if (link.dataset.view === view) link.setAttribute("aria-current", "page"); else link.removeAttribute("aria-current"); });
    document.querySelectorAll(".alpha-view").forEach(section => { section.hidden = section.id !== "view-" + view; });
  }
  document.querySelectorAll(".alpha-tabs [data-view]").forEach(link => link.addEventListener("click", event => {
    if (event.button || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault(); const url = new URL(window.location.href); url.searchParams.set("view", link.dataset.view); history.pushState(null, "", url); selectView();
  }));
  window.addEventListener("popstate", selectView);
  const action = async (body, started) => {
    if (preview || !csrf) throw new Error("A live private session is required.");
    const key = JSON.stringify(body);
    if (!actionIds.has(key)) actionIds.set(key, "request_" + crypto.randomUUID());
    const result = await api("/v1/alpha-actions", {body: {...body, requestId: actionIds.get(key)}, sessionEpoch: started});
    actionIds.delete(key);
    return result;
  };
  el("alpha-login").addEventListener("submit", event => {
    event.preventDefault(); if (preview || busy) return;
    const code = el("alpha-code").value; el("alpha-code").value = "";
    run(async started => {
      const value = await api("/v1/context-sessions/exchange", {body: {code}, sessionEpoch: started});
      if (typeof value.csrf !== "string" || !value.csrf) throw new Error("The service did not return an operator session.");
      csrf = value.csrf; armSession(value); if (!csrf) return;
      el("alpha-connect").hidden = true; await load(started);
    }, "Opening the private workspace…");
  });
  el("alpha-reload").onclick = () => run(load, "Refreshing private evidence…");
  el("alpha-signout").onclick = () => {
    if (!csrf || preview) return;
    // Invalidate prior requests before sending the cookie-authenticated deletion.
    const previous = csrf; invalidate("Ending the operator session…", "closed");
    const started = epoch; busy = true; controls();
    fetch("/v1/context-session", {method: "DELETE", headers: {"X-Wazi-CSRF": previous}, credentials: "same-origin", cache: "no-store", redirect: "error"}).then(response => { if (!response.ok) throw new Error("The server could not confirm sign-out. This page cleared private data; the operator cookie may remain valid until expiry."); if (epoch === started) message("Operator session ended.", "closed"); }).catch(problem => { if (epoch === started) error(problem.message); }).finally(() => { if (epoch === started) { busy = false; controls(); } });
  };
  for (const [toggle, form] of [["toggle-import", "import-form"], ["toggle-ticket", "ticket-form"]]) el(toggle).onclick = () => { if (preview || !csrf || busy) return; el(form).hidden = !el(form).hidden; if (!el(form).hidden) el(form).querySelector("input").focus(); };
  el("import-form").addEventListener("submit", event => {
    event.preventDefault(); if (preview || !csrf || busy) return;
    const fields = new FormData(event.currentTarget);
    const body = {action: "import-job", jobId: fields.get("jobId").trim(), trialId: fields.get("trialId").trim(), attempt: Number(fields.get("attempt")), interventions: Number(fields.get("interventions")), deterministic: fields.has("deterministic")};
    run(async started => { const result = await action(body, started); await load(started); el("import-form").reset(); el("import-form").hidden = true; message(result.state === "no-new-observation" ? "No new observation was imported. The service retained the existing evidence." : "Observation recorded from the private job receipt.", "ready"); }, "Recording the returned job evidence…");
  });
  el("ticket-form").addEventListener("submit", event => {
    event.preventDefault(); if (preview || !csrf || busy) return;
    const fields = new FormData(event.currentTarget), body = {action: "open-ticket", jobId: fields.get("jobId").trim(), category: fields.get("category"), severity: fields.get("severity")};
    run(async started => { const result = await action(body, started); await load(started); el("ticket-form").reset(); el("ticket-form").hidden = true; const ticket = report.supportTickets.find(item => item.ticketId === result.ticketId); if (ticket) showTicket(ticket); message("Support issue recorded with its release evidence.", "ready"); }, "Creating the support issue…");
  });
  el("transition-form").addEventListener("submit", event => {
    event.preventDefault(); if (preview || !csrf || busy || !selected) return;
    const fields = new FormData(event.currentTarget), body = {action: "transition-ticket", ticketId: selected.ticketId, from: selected.state, to: fields.get("to"), jobId: fields.get("jobId").trim()};
    run(async started => { await action(body, started); await load(started); message("Support status updated from bound job evidence.", "ready"); }, "Updating support status…");
  });
  el("close-ticket-detail").onclick = () => { selected = null; el("ticket-detail").hidden = true; el("feedback-consent").checked = false; controls(); };
  el("feedback-consent").onchange = controls;
  el("feedback-download").onclick = () => {
    if (preview || !csrf || busy || !selected || !el("feedback-consent").checked) return;
    const ticketId = selected.ticketId;
    run(async started => {
      const value = await action({action: "feedback-export", ticketId, consent: true}, started);
      if (!value.feedback || value.sent !== false) throw new Error("The service did not return a local feedback export.");
      const url = URL.createObjectURL(new Blob([JSON.stringify(value.feedback, null, 2) + "\n"], {type: "application/json"}));
      const link = node("a"); link.href = url; link.download = "wazi-feedback-" + ticketId.replace(/[^A-Za-z0-9_-]/g, "") + ".json"; document.body.append(link); link.click(); link.remove(); window.setTimeout(() => URL.revokeObjectURL(url), 1000);
      el("feedback-consent").checked = false; message("Redacted feedback file prepared for local review. Nothing was sent.", "ready");
    }, "Preparing the consented feedback file…");
  };
  selectView(); controls();
  if (preview) {
    el("preview-notice").hidden = false; el("alpha-connect").hidden = true; text("alpha-session", "Read-only preview");
    run(load, "Loading captured pilot evidence…");
  } else run(async started => {
    let value;
    try { value = await api("/v1/context-session", {sessionEpoch: started}); }
    catch (problem) { if (problem.name === "AbortError" || started !== epoch) return; throw problem; }
    if (typeof value.csrf !== "string" || !value.csrf) throw new Error("The service did not return an operator session.");
    csrf = value.csrf; armSession(value); if (!csrf) return;
    el("alpha-connect").hidden = true; await load(started);
  }, "Checking the existing operator session…");
})();
