'use strict';
(() => {
  let csrf = '';
  const el = id => document.getElementById(id);
  const status = (message, error = false) => { el('status').textContent = message; el('status').className = error ? 'error' : ''; };
  async function api(path, body) {
    const headers = {};
    if (body !== undefined) { headers['Content-Type'] = 'application/json'; headers['X-Wazi-CSRF'] = csrf; }
    const response = await fetch(path, {method: body === undefined ? 'GET' : 'POST', headers,
      credentials: 'same-origin', cache: 'no-store', redirect: 'error',
      body: body === undefined ? undefined : JSON.stringify(body)});
    const text = await response.text();
    let value; try { value = JSON.parse(text); } catch { throw new Error('Service did not return a context response.'); }
    if (!response.ok) throw new Error(value.error || 'Request rejected.');
    return value;
  }
  function show(value) {
    if (!value) throw new Error('Context not found.');
    el('result').hidden = false;
    el('context-id').value = value.contextId || '';
    el('facts').replaceChildren();
    for (const [label, key] of [['Decision', 'decision'], ['Observed', 'evaluatedAt'], ['Expires', 'expiresAt'], ['Tenant', 'tenantScope'], ['Host', 'hostId'], ['Application', 'applicationId'], ['Operation', 'operation']]) {
      const dt = document.createElement('dt'), dd = document.createElement('dd');
      dt.textContent = label; dd.textContent = value[key] || 'Unavailable'; el('facts').append(dt, dd);
    }
    el('explanation').replaceChildren();
    for (const line of value.explanation || []) { const li = document.createElement('li'); li.textContent = line; el('explanation').append(li); }
    el('records').textContent = JSON.stringify(value, null, 2);
    status('Context loaded. This is not execution approval.');
  }
  async function run(action) { try { status('Checking…'); await action(); } catch (error) { status(error.message, true); } }
  el('login').addEventListener('submit', event => { event.preventDefault(); run(async () => {
    const code = el('code').value; el('code').value = '';
    const value = await api('/v1/context-sessions/exchange', {code}); csrf = value.csrf;
    for (const id of ['retrieve', 'inspect', 'deploy', 'refresh']) el(id).disabled = false;
    status('Operator session open for 15 minutes.');
  }); });
  el('retrieve').onclick = () => run(async () => { show(await api('/v1/contexts/retrieve', JSON.parse(el('job').value))); });
  el('inspect').onclick = () => run(async () => { show(await api('/v1/contexts/' + encodeURIComponent(el('context-id').value))); });
  el('deploy').onclick = () => run(async () => {
    if (!el('confirm').checked) throw new Error('Review and confirm the exact job first.');
    const job = JSON.parse(el('job').value);
    el('confirm').checked = false;
    const value = await api('/v1/context-deployments', job);
    el('job-id').value = job.jobId; el('progress').textContent = JSON.stringify(value, null, 2);
    status('Job submitted. Refresh to read actual progress.');
  });
  el('job').addEventListener('input', () => { el('confirm').checked = false; });
  el('refresh').onclick = () => run(async () => {
    const value = await api('/v1/context-deployments/' + encodeURIComponent(el('job-id').value));
    el('progress').textContent = JSON.stringify(value, null, 2); status('Actual job status refreshed.');
  });
})();
