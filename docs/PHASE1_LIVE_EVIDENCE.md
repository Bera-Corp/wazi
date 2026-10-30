# Phase 1 Live Closure Evidence — bera-003 Gitea Pilot

Recorded 30–31 October 2026 (Africa/Nairobi, UTC+03:00). Tenant
`tenant_live_20261030T064834Z`, host `host_bera-003`, project
`wazi-gitea-phase1`, loopback `127.0.0.1:13000`. Approver is 1-of-1
(`pilot_maintainer`); this does not establish independent-human quorum.

## Releases (Wazi-signed, Cosign-verified locally, private keys off-host)

| Release | Image digest | Config digest | Manifest canonical digest |
|---|---|---|---|
| A 28.1.0 (`28.1.0+wazipilot.2`) | `sha256:afd05701641f238dc198f98d7733b1d3b09cb6ae1a3f1cd9b3bc2058e706570b` | `sha256:07a1cbdc452fe6ba4984b8733874c4ff729a29a5c318c073911c0fc5a39e2bae` | `sha256:abd097a7b30a5115a05e3bd9061946591bc43651c146e91382e0c9c537c42d85` |
| B 28.2.0 (`28.2.0+wazipilot.2`) | `sha256:7b7f6f93ac9f4d0a50abd1c0c5f204631f0813723e13529d5e14754288d53ce2` | `sha256:1b13bfb21089cb8c893f0fb5217c3f1d96238e0128ad60d5bcb189021b01b23a` | `sha256:47abc1396ef567a92bc1f87cc6c269a3299e0eab25b994bf865af30ed1ff802a` |

Rollback procedure `proc_gitea_rollback_28_2_0` v1.0.2 (digest
`sha256:bb066cf745f053e33a370b25ee147847de31e80d1b1b49c697303654a8966688`)
adds `volume.backup` compensation steps for restores. Policy bundle
`pol_gitea_pilot` digest
`sha256:f9c0aa4f7d64d8eb6e4225dd731e949da40ebc269d043382a9cfde8d0648e3fb`
adds the `backup` approval action (schema enum extended in both
`schemas/common.schema.json` and the embedded Go copy).

## Lifecycle (all jobs terminal `completed` in the durable journal)

| Step | Job / case | Evidence |
|---|---|---|
| Install B (first) | `job_gitea_install_16` / `case_gitea_pilot_5` | health gate passed after bounded retry |
| Uninstall B | `job_gitea_uninstall_3` / `case_gitea_pilot_6` | compose.down only, volumes retained |
| Install A | `job_gitea_install_17` / `case_gitea_pilot_7` | `GET /api/v1/version` → `v28.1.0` |
| Known data | operator via Gitea API | repo `wazi-pilot/lifecycle-proof`, `proof.txt` sha256 `728836b371192e623885770f40710bbe833d0f76d6cdbadfd9495621c25dc3e6`, blob `8eb1dd982924f80bf5915397144a6017c0ce6fa9` |
| Upgrade A→B | `job_gitea_upgrade_3` / `case_gitea_pilot_10` | backup + pull + up + health; version `v28.2.0`; proof sha unchanged |
| Backup (both volumes) | `job_gitea_backup_2` / `case_gitea_pilot_12` | data `sha256:f6088c568d9c32dd5842acf14d11d0016b9bdd1fc0ec0641bd67c9ee2e6e433b` (2,396,672 B); config `sha256:64c8edee7cddd695aa5147622ac3074b6fa92ef52172fb1f5f4a96553d5f1bc3` (3,584 B) |
| Post-backup mutation | operator via Gitea API | `proof2.txt` created (blob `964e9930d7982e67c972f45425ee24c2d010e220`) |
| Restore | `job_gitea_restore_4` / `case_gitea_pilot_16` | both volumes restored; proof sha exact; `proof2.txt` → HTTP 404; pre-restore checkpoints retained |
| Rollback B→A | `job_gitea_rollback_2` / `case_gitea_pilot_18` | version `v28.1.0`; proof sha unchanged |

Resource enforcement observed live: project quotas `#31001` 18,432 MiB
and `#31002` 2,048 MiB applied and verified via the root-owned helper;
container limits 1 GiB / 2 CPUs inspected after every `compose.up`.

## Adversarial matrix (live, zero unauthorized mutation)

State before and after the matrix: version `v28.1.0`, proof sha
`728836b3…`, both checkpoint digests unchanged.

| Case | Job | Result |
|---|---|---|
| Tampered plan after approval (backupId rewritten) | `job_adv_tamper_1` | rejected in preflight: `approval_binding_mismatch` (403), no journal operation |
| Exact job replay | resubmit of `job_adv_tamper_1` | `already-queued`; original result retained, no re-execution |
| Revoked release B (digest entry, seq 35) | `job_adv_revoked_2` | rejected in preflight: `requested application release is revoked` |
| Version-mismatched revocation entry (seq 34) | `job_adv_revoked_1` | correctly non-matching; job completed as checkpoint no-op |
| Daemon loss | `job_adv_daemon_1` | rejected: rootless Docker socket absent; no journal operation |
| Service restart recovery | `job_gitea_backup_3`, `job_gitea_rollback_3` | controlplane + pilot-agent restarted; jobs complete; rollback recovered the app after daemon restart |

Covered by the Go contract/adversarial suites instead of live runs:
untrusted signer, stale revocation sequence, stale host profile,
wrong architecture, incompatible resources, disk-full, evidence-write
failure.

## Reconciliation

Ambiguous operations from the failure-driven buildout were reconciled
through the pilot agent's `POST /reconcile` endpoint with digest-bound
operator observations; all journal entries are now terminal:

`op_gitea_install_13`, `op_gitea_upgrade_1`, `op_gitea_upgrade_2`,
`op_gitea_restore_2`, `op_gitea_restore_3` → `blocked`. Reconciliation
never resumes a job; further mutation requires a newly approved case.

## Pairing acceptance (formal, 2026-10-30)

Acceptance record `artifacts/phase1-closure/pairing-acceptance.json`,
canonical SHA-256
`sha256:b0cc0f413bc90a6daf242d5ccfab10f0fe4788093fd876aa96809ced6a166696`,
accepted 2026-10-30T22:15:37Z by `pilot_maintainer` (1-of-1). Digest-bound
pairing facts observed live on bera-003:

| Fact | Value |
|---|---|
| Node certificate SHA-256 | `sha256:a951efa5763e34f47596a92c52309990c06d1772461f2870dd833a9129138c8c` |
| Pairing CA SHA-256 | `sha256:6827f47dc080bff8a27a40961e859b629611e9f4c02f7d6a6c3fa60650ebd497` |
| Issuer CA SHA-256 | `sha256:f304f65a4f3556ad9f960aa8572352d3d53edabaed910b50dc8bcba83ac23ea5` |
| Certificate subject / issuer | `CN=host_bera-003` / `CN=wazi-bera003-dev-pairing-ca` |
| Certificate SAN URI | `urn:wazi:tenant:tenant_live_20261030T064834Z:node:host_bera-003` |
| Registry node fingerprint | `34546c8241357c8868bc7ec255e952120a70295190c61302bd252c9f787619ef` |
| Pairing registry SHA-256 | `sha256:4478801ad97247302e190835a0cdc95c1c1eb99ab083ac710d35be4d24e258b3` |
| Enrolled / expires | 2026-10-30T09:49:22+03:00 / 2026-10-31T06:49:22Z, not revoked |

At acceptance the journal held 18 terminal operations (12 completed, 5
blocked after reconciliation, 1 failed) with zero pending reconciliation,
all executed over this paired identity.

**Concrete blocker:** the approver set is a single configured operator
(`pilot_maintainer`, 1-of-1). Independent-human quorum cannot be met on
this pilot and remains a production blocker; certificate renewal must
also complete before 2026-10-31T06:49:22Z.

## Unattended recovery — service restart and full reboot

- `wazi-pilot-agent` was found `disabled` at boot during this pass and is
  now `enabled`; `wazi-controlplane-dev` and the rootless `docker.service`
  (user, linger on) were already enabled.
- Full host reboot 2026-10-31T01:16+03: boot ID changed
  `eae1e4f0-91c1-4b3c-8ac1-2b548f612323` →
  `f364e12c-a52a-415a-a1d3-c58a5a38b6e4`. With no operator action, the
  control plane, pilot agent (socket `/run/wazi-phase1/agent.sock`), and
  rootless Docker returned `active`; all 18 journal files were
  byte-identical to the pre-reboot snapshot (SHA-256 compared per file)
  and the queue held 38 requests / 38 results.
- The application container does not auto-start (no restart policy by
  design). Post-reboot recovery ran as a fresh approved job:
  `job_gitea_rollback_4` / `case_gitea_pilot_22` completed; version
  `v28.1.0`, health `pass`, `proof.txt` SHA-256
  `728836b371192e623885770f40710bbe833d0f76d6cdbadfd9495621c25dc3e6`
  unchanged. No host-side repair was needed beyond the runner staging
  its helper into the fresh `/tmp`.

## Failure-driven fixes proven by this closure

- Executor `compose.up` step binds the canonical artifact record digest
  (builder previously emitted the rendered-compose digest).
- Envelope allowlist includes rollback-procedure capabilities.
- Health gate polls the approved target for up to 90 s (fresh Gitea
  needs ~5 s); redirects and non-200 never pass.
- Quota helper accepts the rootless-mapped application UID as volume
  owner after first start; pilot unit keeps `NoNewPrivileges=false`
  only so the sudoers-pinned fixed helper can run.
- Backup/restore stream volume bytes through a one-off helper container
  running the pinned image (rootless UID mapping blocks host reads);
  restore clears the volume before extraction, retains a digest-bound
  pre-restore checkpoint, and never renames the daemon-managed volume
  directory. Helper stdin is forwarded (`docker run -i`).
- Go binding compares parameter-contract coverage as a capability set;
  a sequence may repeat a capability (two volume backups).
- Case IDs are single-use by journal design; every operation runs under
  a fresh approved case.

## Identity and resource follow-up — 31 October 2026

The node certificate was renewed at 2026-10-30T22:24:13Z (01:24 EAT on
31 October). Its new expiry is 2026-10-31T22:24:13Z (01:24 EAT on
1 November). The current DER fingerprint is
`sha256:a421f34b6486258b9d135c28588103378342f78da331818c18cd7e2ee9ab2f21`;
the PEM SHA-256 is
`sha256:5e72c046462e871bb28df3d837a25117b26e97fd052382eced6cf925773b499b`.
The server anchor was unchanged, all 19 journal files were byte-identical
through renewal, the worker restarted successfully and Gitea remained healthy.
This supersedes the expiry warning above without changing the historical
pairing-acceptance record.

The worker's earlier 512 MiB / 0.5 CPU ceiling was reduced to a conservative
192 MiB / 0.25 CPU ceiling. Both it and rootless Docker
now sit beneath `user-1002.slice`, whose combined hard ceiling is 1,536 MiB
and 2.625 CPUs. These were inspected live after restarting only the worker;
application health remained passing. The persistent aggregate boundary is
configured with `systemctl set-property user-1002.slice MemoryMax=1536M CPUQuota=262.5%`.

Sanitized public acceptance and follow-up receipts are retained under
`releases/phase1/gitea/pilot-evidence/`; private runtime state, credentials
and application backups remain off Git.

Closure scope remains the single approved Gitea pilot and the explicitly
listed live denial subset. Reboot evidence establishes unattended service
startup and journal preservation; application recovery still requires a new
approved job. Automatic certificate renewal and automatic application recovery
have not been demonstrated. Independent-human quorum remains a production
blocker.

Remote [CI run 41138899498](https://github.com/Bera-Corp/wazi/actions/runs/41138899498)
passed for source commit `144e967907c9afe92d4e8159bf8ca16d8ca13cb3`.
Contracts, secret scanning, Go tests/vet/formatting, Linux builds, canonical
round-trip schema validation and native Linux race checks passed. Race checks
include controlplane, pairing, agentclient, operationjournal and pilotqueue.
This qualifies the committed pilot implementation; the limits above remain.
