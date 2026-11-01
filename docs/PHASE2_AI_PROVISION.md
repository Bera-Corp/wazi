# Deferred AI provider connection

The owner deferred connection until the end of the project on 1 November 2026.
The provision is disabled; no real model call or spending has been authorized.
Provider connection is not part of current live acceptance evidence.

Copy releases/phase2/ai-provider.provision.json to a private local setup file.
When ready, select the endpoint, exact model, allowed hostname and credential
environment variable. Keep the credential value in the environment only.
Verify current official pricing, record its HTTPS source and UTC observation,
and set a positive total run cap. The runner rejects stale pricing observations.

The explicit runner is scripts/author_package_candidate.py. It requires setup,
request, ledger, output and run-id arguments, plus --authorize-spend. Leave the
provision disabled until actual provider access and spending are approved.
The request supplies applicationId, releaseVersion, hash-pinned sources and
approvedImages. Neither credentials nor secret values belong in that packet.

Requests reserve worst-case cost durably before calling. Failed calls keep
their reservation and are never automatically retried. Outputs remain unsigned
candidates. The adapter cannot sign, promote or deploy them. A later real-call
receipt, human review and qualification are required before an AI-authored
package can enter the reviewed catalog.
