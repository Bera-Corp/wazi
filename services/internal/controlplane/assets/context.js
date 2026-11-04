'use strict';
(() => {
  let csrf = '';
  let verifiedJobText = '';
  let sessionExpiresAt = 0;
  let sessionTimer = 0;
  let selectedCandidate = null;
  const el = id => document.getElementById(id);
  const protectedControls = ['package', 'operation', 'backup-id', 'propose', 'job', 'retrieve',
    'context-id', 'inspect', 'confirm', 'approve-run', 'deploy', 'job-id', 'refresh',
    'candidate-packet', 'candidate-import', 'candidate-list', 'qualification-receipt',
    'candidate-qualify', 'candidate-confirm-review', 'candidate-review', 'signed-manifest',
    'candidate-confirm-promotion', 'candidate-promote', 'alpha-refresh'];

  function setStatus(message, state = 'idle') {
    el('status').textContent = message;
    el('status').dataset.state = state;
  }
  function setSessionState(message, state) {
    el('session-state').textContent = message;
    el('session-state').dataset.state = state;
  }
  function setSessionControls(active) {
    for (const id of protectedControls) el(id).disabled = !active;
    el('propose').disabled = true;
    el('approve-run').disabled = true;
    el('refresh').disabled = !active || !el('job-id').value.trim();
    el('inspect').disabled = !active || !el('context-id').value.trim();
    el('code').disabled = active;
    el('login').querySelector('button').disabled = active;
    setCandidateControls();
  }
  function setCandidateControls() {
    const state = selectedCandidate ? selectedCandidate.state : '';
    const active = Boolean(csrf);
    const canReview = active && state === 'tested';
    const canPromote = active && state === 'reviewed';
    el('candidate-list').disabled = !active || el('candidate-list').options.length <= 1;
    el('candidate-import').disabled = !active;
    el('candidate-packet').disabled = !active;
    const canQualify = active && (state === 'candidate' || state === 'failed');
    el('qualification-receipt').disabled = !canQualify;
    el('candidate-qualify').disabled = !canQualify || !el('qualification-receipt').value.trim();
    el('candidate-confirm-review').disabled = !canReview;
    el('candidate-review').disabled = !canReview || !el('candidate-confirm-review').checked;
    el('signed-manifest').disabled = !canPromote;
    el('candidate-confirm-promotion').disabled = !canPromote;
    el('candidate-promote').disabled = !canPromote || !el('candidate-confirm-promotion').checked || !el('signed-manifest').value.trim();
  }
  function setCandidateStages(current = -1, failed = false, completeAll = false) {
    for (const [index, step] of [...el('candidate-steps').children].entries()) {
      const state = completeAll || index < current ? 'complete'
        : index === current ? (failed ? 'error' : 'current') : 'pending';
      const spokenState = state === 'complete' ? 'complete'
        : state === 'current' ? 'in progress'
          : state === 'error' ? 'failed; retry required' : 'pending';
      step.dataset.state = state;
      step.setAttribute('aria-label', step.textContent.trim() + ': ' + spokenState);
      if (index === current && !completeAll) step.setAttribute('aria-current', 'step');
      else step.removeAttribute('aria-current');
    }
  }
  function clearSessionData() {
    verifiedJobText = '';
    el('package').replaceChildren(new Option('Open a session to load the private catalog', ''));
    el('catalog-state').textContent = 'Sign in to load the private catalog.';
    el('catalog-state').dataset.state = 'idle';
    el('job').value = '';
    el('context-id').value = '';
    el('job-id').value = '';
    el('confirm').checked = false;
    el('facts').replaceChildren();
    el('steps').replaceChildren();
    el('explanation').replaceChildren();
    el('records').textContent = '';
    el('result').hidden = true;
    el('review-empty').hidden = false;
    setReviewState('No proposal loaded', 'Open a session and prepare an operation to see returned context and evidence.', 'empty');
    el('progress-summary').textContent = 'No job has been submitted in this session.';
    el('progress-summary').dataset.state = 'empty';
    el('progress-events').replaceChildren();
    el('empty-events').hidden = false;
    el('empty-events').textContent = 'Execution events appear after a job response is returned.';
    el('progress').textContent = '';
    el('alpha-metrics').replaceChildren();
    el('alpha-summary').textContent = 'Open a private session to view your cohort.';
    clearCandidateData();
  }
  function clearCandidateData() {
    selectedCandidate = null;
    el('candidate-list').replaceChildren(new Option('No candidate selected', ''));
    el('candidate-packet').value = '';
    el('qualification-receipt').value = '';
    el('signed-manifest').value = '';
    el('candidate-record').textContent = '';
    el('candidate-digest').textContent = 'Select a stored candidate to inspect its digest and evidence.';
    el('candidate-state').textContent = 'Open a maintainer session to load tenant candidates.';
    el('candidate-state').dataset.state = 'idle';
    el('candidate-confirm-review').checked = false;
    el('candidate-confirm-promotion').checked = false;
    setCandidateStages();
    setCandidateControls();
  }
  function showCandidate(candidate) {
    selectedCandidate = candidate || null;
    el('candidate-confirm-review').checked = false;
    el('candidate-confirm-promotion').checked = false;
    if (!candidate) {
      el('candidate-record').textContent = '';
      el('candidate-digest').textContent = 'Select a stored candidate to inspect its digest and evidence.';
      setCandidateStages();
      setCandidateControls();
      return;
    }
    const stages = ['candidate', 'tested', 'reviewed', 'signed'];
    const current = candidate.state === 'failed' ? 1 : stages.indexOf(candidate.state);
    setCandidateStages(candidate.state === 'signed' ? -1 : current, candidate.state === 'failed', candidate.state === 'signed');
    el('candidate-digest').textContent = 'Candidate ' + candidate.candidateId + ' / ' + candidate.state +
      ' / content ' + candidate.candidateDigest + ' / authoring packet ' + candidate.packetDigest;
    el('candidate-record').textContent = JSON.stringify(candidate, null, 2);
    el('qualification-receipt').value = candidate.qualification && candidate.qualification.receipt
      ? JSON.stringify(candidate.qualification.receipt, null, 2) : '';
    el('candidate-state').textContent = candidate.state === 'failed'
      ? 'The latest signed qualification attempt failed and is retained in the record. Review stays blocked until a passing retry. Candidate digest: ' + candidate.candidateDigest
      : 'Selected candidate is ' + candidate.state + '. Every transition is bound to ' + candidate.candidateDigest + '.';
    el('candidate-state').dataset.state = candidate.state === 'signed' ? 'ready'
      : candidate.state === 'failed' ? 'error' : 'idle';
    setCandidateControls();
  }
  async function loadPackageCandidates() {
    el('candidate-state').textContent = 'Loading tenant-scoped candidates…';
    el('candidate-state').dataset.state = 'loading';
    const candidates = await api('/v1/package-candidates');
    el('candidate-list').replaceChildren(new Option('Select a stored candidate', ''));
    for (const candidate of candidates) {
      const option = document.createElement('option');
      option.value = candidate.candidateId;
      option.textContent = candidate.state + ' / ' + candidate.candidateId + ' / ' + candidate.candidateDigest;
      el('candidate-list').append(option);
    }
    el('candidate-state').textContent = candidates.length
      ? candidates.length + ' tenant candidate' + (candidates.length === 1 ? '' : 's') + ' returned by the service.'
      : 'No candidates are stored for this tenant.';
    el('candidate-state').dataset.state = candidates.length ? 'ready' : 'empty';
    el('candidate-list').disabled = candidates.length === 0;
    setCandidateControls();
  }
  async function refreshCandidate(candidate) {
    await loadPackageCandidates();
    if (!candidate) return;
    el('candidate-list').value = candidate.candidateId;
    showCandidate(await api('/v1/package-candidates/' + encodeURIComponent(candidate.candidateId)));
  }
  function setReviewState(title, message, state) {
    verifiedJobText = '';
    el('confirm').checked = false;
    el('approve-run').disabled = true;
    el('result').hidden = true;
    el('review-empty').hidden = false;
    el('review-empty').dataset.state = state;
    el('review-empty').querySelector('h2').textContent = title;
    el('review-empty').querySelector('p').textContent = message;
    el('facts').replaceChildren();
    el('steps').replaceChildren();
    el('explanation').replaceChildren();
    el('records').textContent = '';
    el('inspect').disabled = !csrf || !el('context-id').value.trim();
  }
  function executionPending(message, reset = false) {
    if (reset) {
      el('progress-events').replaceChildren();
      el('progress').textContent = '';
    }
    el('progress-summary').textContent = message;
    el('progress-summary').dataset.state = 'pending';
    el('empty-events').textContent = 'Waiting for returned execution events.';
    el('empty-events').hidden = el('progress-events').children.length > 0;
  }
  function executionFailure(error) {
    el('progress-summary').textContent = 'Request ended without a current execution response. ' +
      (error.message || 'Refresh by job ID to confirm server state.');
    el('progress-summary').dataset.state = 'error';
    if (!el('progress-events').children.length) {
      el('empty-events').textContent = 'No new execution events returned. Refresh by job ID to confirm server state.';
      el('empty-events').hidden = false;
    }
  }
  function expireSession(message) {
    if (sessionTimer) window.clearInterval(sessionTimer);
    sessionTimer = 0;
    sessionExpiresAt = 0;
    csrf = '';
    setSessionControls(false);
    clearSessionData();
    setSessionState('Session expired', 'expired');
    setStatus(message || 'Session expired. Open a new operator session to continue.', 'expired');
  }
  function armSessionExpiry(expiresIn) {
    if (sessionTimer) window.clearInterval(sessionTimer);
    const match = /^\s*(\d+)\s*(seconds?|minutes?)\s*$/i.exec(expiresIn || '');
    const amount = match ? Number(match[1]) : 15;
    const unit = match && match[2].toLowerCase().startsWith('second') ? 1000 : 60000;
    sessionExpiresAt = Date.now() + amount * unit;
    const update = () => {
      const remaining = sessionExpiresAt - Date.now();
      if (remaining <= 0) {
        expireSession('Session expired. Open a new operator session to continue.');
        return;
      }
      const minutes = Math.max(1, Math.ceil(remaining / 60000));
      setSessionState('Session active, expires in ' + minutes + ' min', 'active');
    };
    update();
    sessionTimer = window.setInterval(update, 15000);
  }
  async function api(path, body) {
    const sessionAtStart = csrf;
    const headers = {};
    if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
      headers['X-Wazi-CSRF'] = csrf;
    }
    const response = await fetch(path, {
      method: body === undefined ? 'GET' : 'POST',
      headers,
      credentials: 'same-origin',
      cache: 'no-store',
      redirect: 'error',
      body: body === undefined ? undefined : JSON.stringify(body)
    });
    const text = await response.text();
    if (csrf !== sessionAtStart) throw new Error('The session changed while this request was pending.');
    let value;
    try { value = JSON.parse(text); }
    catch { throw new Error('Service did not return a context response.'); }
    if (!response.ok) {
      if (response.status === 401 && path !== '/v1/context-sessions/exchange' && csrf) {
        expireSession('The server ended this operator session. Open a new session to continue.');
      }
      throw new Error((value.error || 'Request rejected.') + (value.reason ? ': ' + value.reason : '') + (value.detail ? ': ' + value.detail : ''));
    }
    return value;
  }
  function show(value) {
    if (!value) throw new Error('Context not found.');
    verifiedJobText = '';
    el('confirm').checked = false;
    el('approve-run').disabled = true;
    el('result').hidden = false;
    el('review-empty').hidden = true;
    el('context-id').value = value.contextId || '';
    el('inspect').disabled = !csrf || !el('context-id').value;
    const decision = value.decision || 'unavailable';
    el('decision-label').textContent = decision;
    el('decision-label').dataset.state = decision;
    el('facts').replaceChildren();
    for (const [label, key] of [['Decision', 'decision'], ['Observed', 'evaluatedAt'],
      ['Expires', 'expiresAt'], ['Tenant', 'tenantScope'], ['Host', 'hostId'],
      ['Application', 'applicationId'], ['Operation', 'operation']]) {
      const dt = document.createElement('dt');
      const dd = document.createElement('dd');
      dt.textContent = label;
      dd.textContent = value[key] || 'Unavailable';
      el('facts').append(dt, dd);
    }
    const snapshot = value.snapshot || {};
    const resources = (snapshot.localHostObservation || {}).resources;
    const owner = (snapshot.budget || {}).ownerBudget;
    for (const [label, item] of [['Current host', resources], ['Owner cap', owner]]) {
      if (!item) continue;
      const dt = document.createElement('dt');
      const dd = document.createElement('dd');
      dt.textContent = label;
      dd.textContent = item.cpuCores + ' CPU / ' + item.ramMb + ' MiB RAM / ' +
        (item.storageMb ?? item.storageFreeMb) + ' MiB storage';
      el('facts').append(dt, dd);
    }
    for (const [label, text] of [
      ['Selected procedure', value.selected ? value.selected.procedureId + ' @ ' + value.selected.version : 'Unavailable'],
      ['Relevant successes', String((value.successCaseRefs || []).length)],
      ['Relevant failures', String((value.failureCaseRefs || []).length)]
    ]) {
      const dt = document.createElement('dt');
      const dd = document.createElement('dd');
      dt.textContent = label;
      dd.textContent = text;
      el('facts').append(dt, dd);
    }
    el('steps').replaceChildren();
    const steps = (snapshot.plan || {}).steps || [];
    if (!steps.length) {
      const li = document.createElement('li');
      li.textContent = 'No plan steps were returned.';
      el('steps').append(li);
    }
    for (const step of steps) {
      const li = document.createElement('li');
      const capability = document.createElement('span');
      capability.className = 'capability';
      capability.textContent = step.capability || 'Step';
      li.append(capability, document.createTextNode(': ' + JSON.stringify(step.parameters || {})));
      el('steps').append(li);
    }
    el('explanation').replaceChildren();
    const explanation = value.explanation || [];
    if (!explanation.length) {
      const li = document.createElement('li');
      li.textContent = 'No explanation was returned.';
      el('explanation').append(li);
    }
    for (const line of explanation) {
      const li = document.createElement('li');
      li.textContent = line;
      el('explanation').append(li);
    }
    el('records').textContent = JSON.stringify(value, null, 2);
    const accepted = decision === 'accept';
    setStatus(
      accepted
        ? 'Context accepted for review. This does not approve execution.'
        : 'Context loaded with decision ' + decision + '. No execution approval was granted.',
      accepted ? 'success' : 'error'
    );
  }
  function showProgress(value) {
    const operation = ((value.operation || {}).Operation) || {};
    const state = operation.state || value.state || 'unavailable';
    el('progress-summary').textContent = 'State: ' + state + ', ' + (value.jobId || 'job ID unavailable');
    el('progress-summary').dataset.state = String(state).toLowerCase();
    el('progress-events').replaceChildren();
    const events = operation.events || [];
    el('empty-events').hidden = events.length > 0;
    if (!events.length) el('empty-events').textContent = 'The service returned no journal events for this job.';
    for (const event of events) {
      const li = document.createElement('li');
      li.className = 'timeline-event';
      li.dataset.state = String(event.result || event.kind || '').toLowerCase();
      const content = document.createElement('div');
      const label = document.createElement('div');
      label.className = 'timeline-event-label';
      label.textContent = [event.kind, event.capability || event.stepId, event.result].filter(Boolean).join(', ') || 'Journal event';
      content.append(label);
      if (event.at) {
        const time = document.createElement('time');
        const parsed = Date.parse(event.at);
        if (!Number.isNaN(parsed)) time.dateTime = new Date(parsed).toISOString();
        time.textContent = event.at;
        content.append(time);
      }
      li.append(document.createElement('span'), content);
      el('progress-events').append(li);
    }
    el('progress').textContent = JSON.stringify(value, null, 2);
    if (value.jobId) {
      el('job-id').value = value.jobId;
      el('refresh').disabled = !csrf;
    }
  }
  async function run(action, pending = 'Checking current state…', onFailure) {
    setStatus(pending, 'working');
    try { await action(); }
    catch (error) {
      if (onFailure) onFailure(error);
      const state = el('session-state').dataset.state === 'expired' ? 'expired' : 'error';
      setStatus(error.message || 'Request could not be completed.', state);
    }
  }

  async function loadWorkspace() {
      el('catalog-state').textContent = 'Loading enabled application instances…';
      el('catalog-state').dataset.state = 'loading';
      let catalog;
      try { catalog = await api('/v1/context-catalog'); }
      catch (error) {
        el('catalog-state').textContent = 'Catalog could not be loaded. ' + error.message;
        el('catalog-state').dataset.state = 'error';
        throw error;
      }
      el('package').replaceChildren();
      for (const item of catalog) {
        const option = document.createElement('option');
        option.value = item.project;
        option.textContent = item.profile + ' / ' + item.project + ' / ' + item.releaseVersion + ' (pilot)';
        el('package').append(option);
      }
      const hasCatalog = catalog.length > 0;
      el('package').disabled = !hasCatalog;
      el('propose').disabled = !hasCatalog;
      el('catalog-state').textContent = hasCatalog
        ? catalog.length + ' enabled instance' + (catalog.length === 1 ? '' : 's') + ' returned by the service.'
        : 'Session open. No package instances are enabled; operator catalog setup is required.';
      el('catalog-state').dataset.state = hasCatalog ? 'ready' : 'empty';
      try { await loadPackageCandidates(); }
      catch (error) {
        el('candidate-state').textContent = 'Candidate workflow is unavailable. ' + error.message;
        el('candidate-state').dataset.state = 'error';
      }
      setStatus(hasCatalog ? 'Operator session open.' : 'Session open. No application instance is enabled.', 'success');
  }
  setSessionControls(false);
  setSessionState('Session closed', 'closed');
  if (document.body.dataset.mode !== 'preview') {
    (async () => {
      try {
        const value = await api('/v1/context-session');
        const remaining = Math.floor((Date.parse(value.expiresAt) - Date.now()) / 1000);
        if (!value.csrf || !Number.isFinite(remaining) || remaining <= 0) return;
        csrf = value.csrf;
        setSessionControls(true);
        armSessionExpiry(remaining + ' seconds');
        await loadWorkspace();
      } catch (error) {
        if (csrf) setStatus(error.message || 'The workspace could not be loaded.', 'error');
      }
    })();
  }
  el('login').addEventListener('submit', event => {
    event.preventDefault();
    run(async () => {
      const code = el('code').value;
      el('code').value = '';
      el('code').disabled = true;
      el('login').querySelector('button').disabled = true;
      const value = await api('/v1/context-sessions/exchange', {code});
      csrf = value.csrf;
      setSessionControls(true);
      armSessionExpiry(value.expiresIn);
      await loadWorkspace();
    }, 'Opening the operator session…').finally(() => {
      if (!csrf) {
        el('code').disabled = false;
        el('login').querySelector('button').disabled = false;
      }
    });
  });
  el('retrieve').onclick = () => run(async () => {
    setReviewState('Verifying supplied job', 'Waiting for current context verification from the service…', 'loading');
    const text = el('job').value;
    const value = await api('/v1/contexts/retrieve', JSON.parse(text));
    show(value);
    verifiedJobText = value.decision === 'accept' ? text : '';
    el('approve-run').disabled = !verifiedJobText;
  }, 'Verifying the supplied job against current context…', error => {
    setReviewState('Verification failed', error.message || 'The service did not return a verification result.', 'error');
  });
  el('propose').onclick = () => run(async () => {
    el('job').value = '';
    setReviewState('Checking current host', 'Waiting for current host and signed context from the service…', 'loading');
    const value = await api('/v1/context-proposals', {
      project: el('package').value,
      operation: el('operation').value,
      backupId: el('backup-id').value
    });
    el('job').value = JSON.stringify(value.job, null, 2);
    show(value.context);
    if (value.context.decision === 'accept') {
      verifiedJobText = el('job').value;
      el('approve-run').disabled = false;
    }
  }, 'Checking the host and preparing a proposed plan…', error => {
    setReviewState('Proposal unavailable', error.message || 'The service did not return a proposal.', 'error');
  });
  el('approve-run').onclick = () => run(async () => {
    if (!el('confirm').checked || !verifiedJobText || verifiedJobText !== el('job').value) {
      throw new Error('Verify and review this exact plan first.');
    }
    const job = JSON.parse(verifiedJobText);
    const required = (job.envelope || {}).requiredApprovals || [];
    if (!required.length) throw new Error('The job has no declared approval actions.');
    verifiedJobText = '';
    el('approve-run').disabled = true;
    el('confirm').checked = false;
    executionPending('Recording declared approvals for the reviewed job…', true);
    const approvals = [];
    for (const action of required) {
      const record = await api('/v1/context-approvals', {
        caseId: job.caseId,
        plan: job.plan,
        envelope: job.envelope,
        hostId: job.envelope.hostProfileRef.recordId,
        action,
        decision: 'approved',
        validForSeconds: 600
      });
      approvals.push({approvalId: record.approvalId, action});
    }
    job.approvals = approvals;
    el('job').value = JSON.stringify(job, null, 2);
    el('job-id').value = job.jobId;
    el('refresh').disabled = !csrf || !job.jobId;
    executionPending('Submitting job ' + (job.jobId || '') + ' to the service…');
    const value = await api('/v1/context-deployments', job);
    showProgress(value);
    setStatus('Approved job submitted. Refresh to observe actual execution.', 'success');
  }, 'Recording the declared approvals and submitting the job…', executionFailure);
  el('inspect').onclick = () => run(async () => {
    const contextID = el('context-id').value.trim();
    setReviewState('Loading stored snapshot', 'Waiting for the stored context from the service…', 'loading');
    show(await api('/v1/contexts/' + encodeURIComponent(contextID)));
  }, 'Loading the stored snapshot…', error => {
    setReviewState('Snapshot unavailable', error.message || 'The service did not return the stored context.', 'error');
  });
  el('deploy').onclick = () => run(async () => {
    if (!el('confirm').checked) throw new Error('Review and confirm the exact job first.');
    const job = JSON.parse(el('job').value);
    el('confirm').checked = false;
    el('job-id').value = job.jobId || '';
    el('refresh').disabled = !csrf || !el('job-id').value;
    executionPending('Submitting job ' + (job.jobId || '') + ' to the service…', true);
    const value = await api('/v1/context-deployments', job);
    showProgress(value);
    setStatus('Job submitted. Refresh to read returned execution state.', 'success');
  }, 'Submitting the confirmed job…', executionFailure);
  el('job').addEventListener('input', () => {
    el('confirm').checked = false;
    verifiedJobText = '';
    el('approve-run').disabled = true;
  });
  el('job-id').addEventListener('input', () => {
    el('refresh').disabled = !csrf || !el('job-id').value.trim();
  });
  el('context-id').addEventListener('input', () => {
    el('inspect').disabled = !csrf || !el('context-id').value.trim();
  });
  el('candidate-list').addEventListener('change', () => run(async () => {
    const id = el('candidate-list').value;
    if (!id) { showCandidate(null); return; }
    el('candidate-state').textContent = 'Loading exact candidate and evidence…';
    el('candidate-state').dataset.state = 'loading';
    showCandidate(await api('/v1/package-candidates/' + encodeURIComponent(id)));
  }, 'Loading the selected package candidate…', error => {
    el('candidate-state').textContent = 'Candidate could not be loaded. ' + error.message;
    el('candidate-state').dataset.state = 'error';
  }));
  el('candidate-import').onclick = () => run(async () => {
    const authored = JSON.parse(el('candidate-packet').value);
    const packet = authored.packet || authored;
    el('candidate-state').textContent = 'Validating and storing unsigned candidate…';
    el('candidate-state').dataset.state = 'loading';
    const candidate = await api('/v1/package-candidates', packet);
    await refreshCandidate(candidate);
    setStatus('Unsigned package candidate imported. Qualification, maintainer review and signing are separate steps.', 'success');
  }, 'Importing the unsigned candidate…', error => {
    el('candidate-state').textContent = 'Candidate import rejected. ' + error.message;
    el('candidate-state').dataset.state = 'error';
  });
  el('qualification-receipt').addEventListener('input', setCandidateControls);
  el('candidate-qualify').onclick = () => run(async () => {
    if (!selectedCandidate) throw new Error('Select a candidate first.');
    const receipt = JSON.parse(el('qualification-receipt').value);
    const candidate = await api('/v1/package-candidates/' + encodeURIComponent(selectedCandidate.candidateId) + '/qualification', {
      candidateDigest: selectedCandidate.candidateDigest, receipt
    });
    await refreshCandidate(candidate);
    if (candidate.state === 'failed') {
      setStatus('Failed qualification attempt retained. Review remains blocked until a passing retry.', 'error');
    } else {
      setStatus('Passing signed qualification receipt recorded for the exact candidate digest.', 'success');
    }
  }, 'Verifying signed isolation and qualification evidence…', error => {
    el('candidate-state').textContent = 'Qualification receipt rejected. ' + error.message;
    el('candidate-state').dataset.state = 'error';
  });
  el('candidate-confirm-review').addEventListener('change', setCandidateControls);
  el('candidate-review').onclick = () => run(async () => {
    if (!selectedCandidate || !el('candidate-confirm-review').checked) throw new Error('Confirm review of the selected candidate digest.');
    const candidate = await api('/v1/package-candidates/' + encodeURIComponent(selectedCandidate.candidateId) + '/review', {
      candidateDigest: selectedCandidate.candidateDigest
    });
    await refreshCandidate(candidate);
    setStatus('Maintainer review recorded for the exact tested candidate.', 'success');
  }, 'Recording maintainer review…', error => {
    el('candidate-state').textContent = 'Maintainer review rejected. ' + error.message;
    el('candidate-state').dataset.state = 'error';
  });
  el('signed-manifest').addEventListener('input', setCandidateControls);
  el('candidate-confirm-promotion').addEventListener('change', setCandidateControls);
  el('candidate-promote').onclick = () => run(async () => {
    if (!selectedCandidate || !el('candidate-confirm-promotion').checked) throw new Error('Confirm promotion of the reviewed candidate.');
    const signedManifest = JSON.parse(el('signed-manifest').value);
    const candidate = await api('/v1/package-candidates/' + encodeURIComponent(selectedCandidate.candidateId) + '/promotion', {
      candidateDigest: selectedCandidate.candidateDigest, signedManifest
    });
    await refreshCandidate(candidate);
    setStatus('Already-signed release registered. The control plane did not sign it.', 'success');
  }, 'Verifying and registering the signed release…', error => {
    el('candidate-state').textContent = 'Release promotion rejected. ' + error.message;
    el('candidate-state').dataset.state = 'error';
  });
  el('alpha-refresh').onclick = () => run(async () => {
    const session = csrf;
    const value = await api('/v1/alpha-report');
    if (!csrf || csrf !== session) throw new Error('The private session changed. Refresh after signing in.');
    const labels = {firstAttemptSuccess:'First-attempt deployment',rollbackSuccess:'Rollback success',restoreVerification:'Restore verification',plannerSchemaAfterRetry:'Planner schema after retry',deterministicDeployments:'Deterministic deployments',falseCompatible:'False-compatible decisions','secret-exposure':'Secret exposure','uncited-action':'Uncited actions','arbitrary-shell':'Arbitrary shell execution'};
    el('alpha-metrics').replaceChildren();
    for (const [name, metric] of Object.entries(value.metrics)) {
      const row = document.createElement('tr');
      for (const text of [labels[name] || name, metric.state.replaceAll('-', ' '), metric.numerator + ' / ' + metric.denominator]) {
        const cell = document.createElement('td');
        cell.textContent = text;
        row.appendChild(cell);
      }
      el('alpha-metrics').appendChild(row);
    }
    el('alpha-summary').textContent = (value.alphaThresholdsMet ? 'Measured thresholds met. ' : 'More evidence is needed. ') + value.appsWithSameReleaseLocalAndVpsRecovery.length + ' apps have matching-release local and VPS recovery observations. ' + value.observations + ' observations recorded.';
    setStatus('Private alpha progress refreshed.', 'success');
  }, 'Loading private alpha progress…');
  el('refresh').onclick = () => run(async () => {
    const jobID = el('job-id').value.trim();
    executionPending('Refreshing status for job ' + jobID + '…');
    const value = await api('/v1/context-deployments/' + encodeURIComponent(jobID));
    showProgress(value);
    setStatus('Actual job status refreshed from the service.', 'success');
  }, 'Refreshing actual job status…', executionFailure);
})();
