# Phase 2 measurement protocol

Status: protocol prepared; no human-time savings or model benefit measured yet.

Use the same pinned release, private port, CPU/RAM limits, storage quota and
synthetic dataset for both paths. Preserve Gitea throughout. Record source
commit, image and contract digests, host facts and the evidence window.

The operator baseline must include setup, secret references, readiness,
seeded data, checkpoint, mutation, restore and proof verification. Start the
timer before the first operator action; stop after proof verification. Count
commands requiring decisions, edits, failures and recovery interventions.
Label automated scripts as a scripted operator baseline, not observed human
manual work. Keep download/build time separate from warm-cache installation.

For Wazi, record proposal preparation, review/approval, queue/execution and
health durations separately. The package test script records monotonic job
duration and terminal receipts. A fast install without a verified restore is
not a successful lifecycle. Record every denied or failed attempt; never
discard failures to improve the headline number.

After provider authorization, record the provider, exact model, pricing
source, hard spending cap, request count, input/output usage, latency and
charged/reserved cost. Capture one candidate's deterministic validation,
isolated qualification, review and signing. Prove that a second installation
uses the same signed package with no new inference request.

Report single-host, small-sample results as observations. Do not infer
production reliability, general user time savings or causal AI improvement.
The final demo should allocate evidence to Impact 30%, Meta tooling 25%,
Engineering 20% and Demo 25%, with the pilot quorum and Immich deferral visible.
