'use strict';
(() => {
  let csrf = '';
  let verifiedJobText = '';
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
    if (!response.ok) throw new Error((value.error || 'Request rejected.') + (value.reason ? ' · ' + value.reason : ''));
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

    const snapshot = value.snapshot || {};
    const resources = (snapshot.localHostObservation || {}).resources;
    const owner = (snapshot.budget || {}).ownerBudget;
    for (const [label, item] of [['Current host', resources], ['Owner cap', owner]]) {
      if (!item) continue;
      const dt = document.createElement('dt'), dd = document.createElement('dd');
      dt.textContent = label;
      dd.textContent = item.cpuCores + ' CPU · ' + item.ramMb + ' MiB RAM · ' + (item.storageMb ?? item.storageFreeMb) + ' MiB storage';
      el('facts').append(dt, dd);
    }
    el('steps').replaceChildren();
    for (const step of (snapshot.plan || {}).steps || []) {
      const li = document.createElement('li');
      li.textContent = step.capability + ' · ' + JSON.stringify(step.parameters);
      el('steps').append(li);
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
    for (const id of ['retrieve', 'inspect', 'deploy', 'refresh', 'propose']) el(id).disabled = false;
    const catalog = await api('/v1/context-catalog');
    el('package').replaceChildren();
    for (const item of catalog) {
      const option = document.createElement('option'); option.value = item.project;
      option.textContent = item.profile + ' · ' + item.project + ' · ' + item.releaseVersion + ' (pilot)';
      el('package').append(option);
    }
    status('Operator session open for 15 minutes.');
  }); });
  el('retrieve').onclick = () => run(async () => {
    const text = el('job').value;
    const value = await api('/v1/contexts/retrieve', JSON.parse(text)); show(value);
    verifiedJobText = value.decision === 'accept' ? text : '';
    el('approve-run').disabled = !verifiedJobText;
  });
  el('propose').onclick = () => run(async () => {
    verifiedJobText = ''; el('approve-run').disabled = true;
    const value = await api('/v1/context-proposals', {project: el('package').value, operation: el('operation').value, backupId: el('backup-id').value});
    el('job').value = JSON.stringify(value.job, null, 2);
    el('confirm').checked = false; show(value.context);
    if (value.context.decision === 'accept') { verifiedJobText = el('job').value; el('approve-run').disabled = false; }
  });
  el('approve-run').onclick = () => run(async () => {
    if (!el('confirm').checked || !verifiedJobText || verifiedJobText !== el('job').value) throw new Error('Verify and review this exact plan first.');
    const job = JSON.parse(verifiedJobText);
    verifiedJobText = ''; el('approve-run').disabled = true; el('confirm').checked = false;
    const approvals = [];
    for (const action of job.envelope.requiredApprovals) {
      const record = await api('/v1/context-approvals', {caseId: job.caseId, plan: job.plan, envelope: job.envelope,
        hostId: job.envelope.hostProfileRef.recordId, action, decision: 'approved', validForSeconds: 600});
      approvals.push({approvalId: record.approvalId, action});
    }
    job.approvals = approvals; el('job').value = JSON.stringify(job, null, 2);
    const value = await api('/v1/context-deployments', job);
    el('job-id').value = job.jobId; el('progress').textContent = JSON.stringify(value, null, 2);
    status('Approved job submitted. Refresh to observe actual execution.');
  });
  el('inspect').onclick = () => run(async () => { show(await api('/v1/contexts/' + encodeURIComponent(el('context-id').value))); });
  el('deploy').onclick = () => run(async () => {
    if (!el('confirm').checked) throw new Error('Review and confirm the exact job first.');
    const job = JSON.parse(el('job').value);
    el('confirm').checked = false;
    const value = await api('/v1/context-deployments', job);
    el('job-id').value = job.jobId; el('progress').textContent = JSON.stringify(value, null, 2);
    status('Job submitted. Refresh to read actual progress.');
  });
  el('job').addEventListener('input', () => { el('confirm').checked = false; verifiedJobText = ''; el('approve-run').disabled = true; });
  el('refresh').onclick = () => run(async () => {
    const value = await api('/v1/context-deployments/' + encodeURIComponent(el('job-id').value));
    el('progress').textContent = JSON.stringify(value, null, 2); status('Actual job status refreshed.');
  });
})();
