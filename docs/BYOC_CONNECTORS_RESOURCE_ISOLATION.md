# Wazi BYOC Connector, Authorization and Resource Isolation Architecture

> **Status:** Proposed ADR / technical specification (not implemented).  
> **Date:** 21 October 2026. **Owner:** Wazi / Bera-Corp.  
> **Decision:** Make **customer-supplied compute** the default Wazi Cloud operating model, with provider-specific authorization and a provider-neutral deployment contract.  
> **Related:** [Cloud Fabric & TenantKit](WAZI_CLOUD_FABRIC_AND_TENANTKIT.md) · [Master Plan](WAZI_MASTER_PLAN.md) · [Hackathon Plan](HACKATHON_2026_EXECUTION_PLAN.md) · [Original Architecture](ARCHITECTURE.md) · [Context](CONTEXT_ENGINEERING_PLAN.md) · [Threat Model](THREAT_MODEL.md)

## 1. Executive conclusion

**Wazi is the orchestration, security, identity and distribution layer; the customer brings the infrastructure.** Wazi need not become an infrastructure reseller, hypervisor vendor or SaaS-style pooled data host for applications that can run in a customer's own provisioned environment. An external provider handles physical facilities, base virtualization, account billing and underlying capacity. Wazi provides a consistent user experience and policy engine across Linux servers, major cloud accounts and supported Kubernetes clusters.

Customer value: reuse existing capacity for multiple compatible applications, maintain control over infrastructure and data, receive a clear install/upgrade/backup and recovery workflow without mastering orchestration commands.

Indie developer value: ship an explicit customer-isolated deployment package through Wazi's manifest and marketplace, rather than build an application-wide pooled hosting backend. The developer still owns software quality, licensing, authentication design, release discipline, support and relevant data protection obligations.

**Priority:** deliver a credible Docker Compose-on-one-Linux-VPS connector first, but make provider, host and runtime adapters versioned from day one. Add cloud IAM federation and Kubernetes gradually. Do **not** build a custom container runtime, kernel sandbox or distributed scheduler until a documented problem cannot be met by OCI, cgroups, namespaces, LSM controls, Kubernetes, gVisor/Kata or the provider's VM boundary.

## 2. Key distinctions

| Term | Meaning | What a Wazi UI must not imply |
| --- | --- | --- |
| Existing *provision* | User-controlled VPS, pre-existing VM, cloud resources, quotas/credits, or a cluster namespace, depending on provider | An invoice, subscription, credit balance or login does not prove that deployment will succeed |
| Authorized account | Provider confirms an authenticated principal may access a specified account/resource scope | Does **not** necessarily prove legal ownership; delegated authority must be revocable |
| Provider quota | API/service limit, such as vCPU or volume count | Is **not** guaranteed VM stock or runtime spare CPU/RAM |
| Allocatable resource | Currently available host/cluster capability after reservations | Is **not** a permanent guarantee; admission/allocations can race |
| Per-customer VM isolation | Different customers use distinct logical VMs | Apps co-located inside one customer VM still share host kernel; high-risk apps require stronger separation |
| Non-volatile application | Persistent app data is held outside replaceable process/container writable layers | Does **not** mean zero downtime, no data corruption, infinite durability or automatically valid backups |
| Provider-neutral manifest | One intent/constraints schema maps to supported adapters | Does not mean every provider/runtime supports every feature |

Wazi should show *observed*, *verified*, *estimated*, *unsupported* and *unknown* as distinct status types and attach timestamps. A successful read-only preflight is not permission to execute destructive changes.

## 3. Logical planes

~~~text
                   WAZI HOST-INDEPENDENT CONTROL PLANE
     marketplace / release / tenant / identity / consent / policy
         manifest validation / capacity planning / approval
                   receipts / lifecycle / observability
                                 |
                       typed signed intent
                                 |
                 +---------------+----------------+
                 |          CONNECTOR HUB          |
                 | discover / attest / scope /      |
                 | quota / reserve / revoke         |
                 +---------------+----------------+
                 |               |                |
         account providers   existing Linux     Kubernetes
         AWS / Azure / GCP   VPS / VM / NAS     namespace
                 |               |                |
         scoped APIs & IAM     paired Wazi      scoped API &
         temporary tokens      Host Agent       RBAC identity
                 |               |                |
                 +---------------+----------------+
                                 |
                     WAZI RUNTIME ADAPTER
             Compose / OCI container / future Kubernetes
                       policy + bounded actions
                                 |
                 app A / app B / app C data planes
                    CPU / RAM / PIDs / networking
                   volumes / secrets / backup keys
~~~

**Boundary 1 (Wazi central metadata):** customer-facing account names, connection IDs, scoped identities and coarse resource inventory, install intent, approval receipts, release digests and redacted health status. No arbitrary app database data or long-lived cloud admin passwords.

**Boundary 2 (provider account):** customer retains billing and cloud ownership; Wazi only obtains the narrow management role or customer-authorized host agent needed for the requested operations. Never require subscription Owner/global administrator for an ordinary application install.

**Boundary 3 (runtime):** app images and source material are untrusted even if a source repository is public or a manifest is signed. Deployment agents must apply independent policy and hardware/resource access checks before start. Third-party developer support does not gain access to unrelated customer apps or volumes.

**Boundary 4 (customer recovery):** customer can revoke Wazi, access their provider account/host independently, export data, and restore supported applications without relying solely on Wazi's dashboard.

## 4. Connector onboarding, proof and secure credentials

Wazi should offer **Connect Existing Infrastructure**, then prompt for provider and the intended resource scope. Initial connection is read-only discovery; execute roles or agent operations require an explicit additional grant.

### Proposed universal connector handshake

1. Customer signs into Wazi; Wazi creates a one-use, short-lived connection challenge tied to the customer workspace, provider, selected resource scope and session.
2. Customer authorizes or deploys provider-specific scoped identity/agent *through the real provider authority*, never by supplying cloud root credentials to Wazi.
3. Provider API or paired agent proves control of the intended scope. Wazi verifies resource/account ID, principal identity, allowed operations, scope, expiry and nonce/session binding.
4. Wazi calls a small allowlisted read-only inventory set, then verifies host/cluster capabilities and available quotas/capacity, classifying what was actually observable.
5. Wazi constructs an immutable, versioned deployment plan with resource request, data locations, required approvals, ongoing access requirements and estimated provider cost or "unknown".
6. Customer separately approves the scoped execution role or mutation request. A fresh preflight and policy check occur immediately before apply; make execution idempotent and rollback-aware.
7. Wazi records a revocable connection, audits operations and uses short-lived credentials where possible. Revoking credentials must prevent new operations even if marketplace UI still contains cached inventory.

**Security:** Provider credentials issued for the browser must not be persisted in browser storage; Wazi's privileged backend obtains/rotates short-lived tokens only when needed. Never ask for AWS root keys, Azure subscription Owner by default, raw GCP service account private keys or unrestricted kubeconfig. Do not store static SSH private keys as a convenience path.

### Provider integration matrix (proposed rollout)

| Connector | Preferred authorized access | Ownership/scope evidence | Preflight checks |
| --- | --- | --- | --- |
| Linux VPS / home server | Wazi Host Agent pair via owner-confirmed one-time code, mTLS and rotated short-lived certs | Agent challenge/response + operator confirmation; server fingerprint | OS/arch/patch level, storage free/FS, container runtime, cgroups, RAM headroom, port collisions, backup path, network |
| AWS account / pre-existing EC2 | Cross-account IAM role via STS AssumeRole, distinct Wazi-generated ExternalId per customer; minimal policies and region scope | STS GetCallerIdentity, role policy/resource scope, read-only Describe calls | Instance state/metadata, EC2 quotas vs region capacity, EBS/storage, VPC reachability, role rights, estimated bill |
| Azure subscription/resource group | Customer-consented Entra application/service principal or federated workload identity, Azure RBAC at smallest required resource-group/resource scope | Azure subscription/tenant ID, principal/object ID and effective RBAC scope, read-only Resource Manager APIs | VM inventory, resource group, disks, regional quotas, network security rules and provider rights |
| Google Cloud project | Workload Identity Federation / short-lived impersonated identity with fine-grained IAM binding; no service account keys by default | Resource-project identity, workload principal and authorized permissions | VM state, project/region quotas, attached disks, storage and required APIs |
| Existing Kubernetes cluster | Dedicated namespace-scoped ServiceAccount, RBAC/short-lived bounded token or cluster-supported OIDC auth; optional in-cluster scoped agent | Verified namespace identity, API server credentials, SelfSubjectAccessReview/authorization checks | Allocatable/requested resources, ResourceQuota/LimitRange, PSS admission, CNI NetworkPolicy support, StorageClass, PVC/backup, Ingress |
| Other providers | Provider API adapter or authorized Linux Host Agent on already-existing VM | Provider-specific signed API challenge or successful agent pairing | Return documented unsupported/unknown capability flags; no implicit broad credentials |

**AWS confused deputy:** An ExternalId mitigates but does not replace short-lived credentials, precise resource role policies and tenant-binding checks. See AWS official guidance: https://docs.aws.amazon.com/wellarchitected/2024-06-27/framework/sec_permissions_share_securely_third_party.html and https://aws.amazon.com/blogs/security/how-to-improve-cross-account-access-for-saas-applications-accessing-customer-accounts/ .

**GCP:** Official Workload Identity Federation allows short-lived token exchange from external identities: https://docs.cloud.google.com/iam/docs/workload-identity-federation .

**Azure:** RBAC assignments should apply at the smallest resource/resource-group scope: https://learn.microsoft.com/en-us/azure/role-based-access-control/role-assignments .

**Important limitation:** For a VPS with no first-party provider API, the agent proves *operational authority on the host*, not contractual or legal account ownership. For Kubernetes, ability to read resources does not establish permission to create resources. Provider billing balances and prepaid credits may not be exposed through APIs. Where quota/credit/exact available capacity cannot be verified, report unknown rather than green.

## 5. Capacity and entitlement decision engine

Admission should model both **what the customer is allowed to use** and **what their provision can actually run**.

### Required inputs

- Wazi manifest: app identity, license, immutable images, CPU reservation/limits, memory reservation/limits, disk volume requirements and growth, port/ingress/egress, security trust tier, architecture, OS/runtime versions, backup/restore/migration compatibility.
- Actual host/cluster/provider facts: authorized scope, provider quota, existing workloads, allocatable memory/CPU, filesystem/disk free vs volume quotas, volume IOPS where available, network readiness, available IPs/ports, credentials/certs state and region/storage class.
- Customer policy: data region, spend cap, maintenance window, per-app access, allowed networks, max CPU/RAM/volume usage, allowed security profiles, manual approval for destructive actions.

### Admit only when

1. Verified authorized resource scope and read/write capabilities exactly match the plan.
2. Host OS/CPU architecture/runtime meets declared and measured requirements.
3. Reservations fit available capacity *after* subtracting OS/agent/network/backup reserve, other apps and safety headroom; hard limits are supported by the actual runtime.
4. Persistent storage and backup location meet required retention/export policy; available storage is not confused with reserved quota.
5. Ports, domain, TLS, ingress/egress and secret scopes do not conflict.
6. Image provenance/security policy passes; app trust tier is compatible with requested isolation level.
7. Required upgrade, data migration, recovery, authorization and license constraints pass.
8. Current inputs are fresh enough. After reserving resources optimistically, repeat final relevant checks immediately before apply to avoid TOCTOU conditions.

Produce one of: **ADMISSIBLE**, **ADMISSIBLE_WITH_APPROVAL**, **INSUFFICIENT**, **UNSUPPORTED** or **UNKNOWN_NEEDS_INSPECTION**. The UI shows why and what the customer can change. Never guarantee provider capacity or performance from a quota query alone.

### Deterministic pseudo-contract

~~~yaml
# Proposed, not a shipped manifest or binding
schema: wazi.compute-target/v0-draft
customer_workspace: ws-example
provider_connection: opaque-connection-id
ownership_mode: customer-account
target:
  kind: linux-vps      # or existing-cloud-vm, kubernetes-namespace
  resource_ref: opaque-provider-resource-id
required_permissions:
  - inspect_host
  - deploy_signed_application
  - inspect_deployment
isolation:
  tier: hardened-container
  prohibit_privileged: true
  permit_host_docker_socket: false
  deny_inter_app_volume_mounts: true
resource_policy:
  cpu_request_cores: 0.25
  cpu_limit_cores: 1.0
  memory_request_mb: 384
  memory_limit_mb: 768
  disk_persistent_gb: 8
  pid_limit: 256
  reserves_host_headroom: true
persistence:
  private_volume: true
  backup_required: true
  restore_test_required: true
  off_host_backup_preferred: true
network:
  ingress: workspace_auth_https
  default_egress: deny
  egress_allowlist: [example-updates-endpoint]
credentials:
  type: paired-agent
  ttl: short-lived
execution:
  typed_only: true
  owner_approval: required
  immutable_artifact_digest: sha256:REPLACE-WITH-REAL-DIGEST
~~~

This intent model is independent of Docker/Kubernetes; the adapter must declare accurately which controls it actually enforces. Do not pretend Docker Compose natively provides Kubernetes storage quotas or that a Linux filesystem necessarily supports per-app volume quotas. Unsupported required controls make the deployment ineligible.

## 6. Wazi Isolation Profiles

Wazi should **invent a consistent application isolation policy/profile**, *not* a new security primitive or kernel runtime. Start by configuring and testing existing substrates.

| Profile | Intended trust | Executor | Minimum enforceable controls |
| --- | --- | --- | --- |
| **Restricted Container** | Curated, audited, comparatively low-risk open-source app in one customer workspace | OCI/Docker rootless or daemon with policy-bound typed agent | Separate app UID, userns if supported, no privileged mode/Docker socket, dropped caps, seccomp/AppArmor/SELinux where available, network scoping, independent secrets/volumes, resource and PID caps |
| **Managed Kubernetes Namespace** | Trusted enterprise team/apps; moderate app separation, not hostile tenant boundary | Customer K8s cluster with enforced policy | Namespace RBAC, ResourceQuota, LimitRange, Pod Security Restricted or approved Baseline, NetworkPolicy supported by CNI, PVC access and controlled ingress, deny hostPath/privileged workloads |
| **Hardened Sandbox** | Higher-risk or less-trusted workloads | RuntimeClass with Kata Containers/microVM or equivalent vetted virtualization | VM/kernel separation (subject to concrete runtime verification), same strict storage/network/policy controls; explicit host virtualization prerequisite |
| **Dedicated App VM** | Highly sensitive or mutually distrustful publishers/app instances | Separate customer-controlled VM | Independent kernel and disk/credentials/network IAM; still needs OS patching/host firewall/backups |

**Caution:** Two Docker containers on the same VM share a kernel. Kubernetes namespaces alone are **not** equivalent to tenant-hard isolation; Kubernetes's own security docs recommend stronger controls when trust is weak: https://kubernetes.io/docs/concepts/security/rbac-good-practices/ . Kata Containers can provide a guest kernel boundary: https://kata-containers.github.io/kata-containers/design/virtualization/ .

### Resource limits

Docker Engine supports CPU and memory constraints only when the kernel features are present: https://docs.docker.com/engine/containers/resource_constraints . Wazi preflight must check enforcement support and fail closed when the manifest requires a control unavailable on the target.

Kubernetes uses ResourceQuota and LimitRange to bound namespace and per-container allocations. See https://kubernetes.io/docs/concepts/policy/limit-range/ . Default-deny network isolation works only with CNI support for NetworkPolicy: https://kubernetes.io/docs/concepts/services-networking/network-policies/ . Pod Security Admission uses Restricted/Baseline/Privileged policies; a weakly configured cluster cannot truthfully be rated hardened: https://kubernetes.io/docs/setup/best-practices/enforcing-pod-security-standards/ .

### Trust escalation

Privilege requests, mounting host files, accessing other app volumes, egress to restricted networks and executing unknown build steps must require explicit security review, not merely customer acceptance of a dialog. Mutually hostile unvetted publishers should not be cohosted in one container kernel solely for cost savings.

**No generic remote shell** in Wazi Host Agent, no unvetted Docker socket passthrough, and no default kubeconfig cluster-admin. The Wazi execution engine only calls allowlisted typed actions and records immutable receipts.

## 7. Persistent and resilient application data

"Non-volatile" in this proposal means **application data survives container termination, upgrades and ordinary runtime replacement**. It is not a guarantee against VM termination, storage loss or disaster.

### Persistence requirements

- Declare all stateful paths, expected growth and ownership, filesystem permissions, backup consistency hooks and schema migration gates in Wazi manifest.
- Docker: use dedicated named volumes or explicit host mount with verified ownership; volumes can outlive container removal. Docker documents this separation: https://docs.docker.com/engine/storage/volumes/ .
- Kubernetes: use PVC/StorageClass and check volume binding, snapshot capability, access modes and reclaim policies. A dynamic StorageClass may default to **Delete** on reclaim, so retention must be explicit and tested where required: https://kubernetes.io/docs/concepts/storage/storage-classes/ .
- Backups: encrypt off-instance if possible, maintain proper key custody, retention, integrity checks, and complete end-to-end restore drills, including app metadata/databases.
- In-place updates: stage deployment, back up state, verify migration/downward compatibility, perform health checks, and roll back application binaries on failure. Some database migrations are irreversible: mark downgrade unsupported rather than claim rollback.
- Control plane outage: host agent retains signed desired-state/repair evidence for allowed local recovery; automatic retries cannot silently delete storage.
- No complete guarantee of uptime: if host/VM fails, running processes stop; higher availability requires replication, more VMs, provider SLAs and additional cost.

### Data deletion controls

An application uninstall has separate choices: Remove binaries only, Remove app and retain private data, or Delete app and data. The last requires explicit independent consent, confirmed backup/export alternatives and enforceable retention policy. No silent volume prune with all data.

## 8. Provider-neutral runtime API

Avoid exposing raw Kubernetes YAML or Docker/SSH privileges to Wazi clients or model tools. Standardize a small set of operations:

~~~text
connect.challenger(provider, desired_scope)
connect.verify(connection_id, challenge)
connect.revoke(connection_id)
resources.inspect(connection_id, scope)
resources.verify_capability(connection_id, operation)
resources.estimate_capacity(connection_id, signed_app_manifest)
deploy.plan(connection_id, app_manifest, target)
deploy.approve(plan_id, owner_consent)
deploy.apply(plan_id, expected_revision, idempotency_key)
deploy.status(deployment_id)
deploy.backup(deployment_id)
deploy.verify_restore(deployment_id)
deploy.rollback(deployment_id, backup_checkpoint)
deploy.export(deployment_id)
deploy.terminate(deployment_id, retain_data=true)
~~~

Each call has server-side tenant scope, token audience, explicit provider resource ID, operation allowlist, timeout, idempotency, audit receipt and policy checks. Mutating calls are separated from inspection to minimize implied authority. MCP integrations (future) are thin wrappers around these typed APIs, not alternate routes around authorization.

### Provider capability declaration

~~~json
{
  "schema": "wazi.connector-capabilities/v0-draft",
  "connector": "linux-host-agent",
  "version": "0.1-draft",
  "supports": {
    "auth": "mtls-short-lived",
    "discover": true,
    "deploy_compose": true,
    "deploy_kubernetes": false,
    "enforce_cpu_limit": "requires-preflight",
    "enforce_memory_limit": "requires-preflight",
    "enforce_volume_quota": "unknown-until-filesystem-check",
    "private_network_policy": "requires-preflight",
    "backup_restore": "requires-configured-adapter",
    "data_export": true
  }
}
~~~

Capabilities are assertions that require **runtime verification** before use. Marking a feature supported in JSON is not evidence it functions or is safe.

## 9. Recommended first engineering spike and hackathon fit

**Within the existing October/November 2026 Meta Indie hackathon scope**, add only:
1. A read-only "Connect Existing Linux VPS" capacity/status UI with real Wazi Host Agent pairing on a controlled demo VM.
2. A real preflight screen showing app minimums vs measured CPU/RAM/storage headroom, available ports and Docker constraints.
3. One explicit user approval and a signed/pinned Compose deployment using typed operations, evidence and health.
4. If feasible, demonstrate a **second small vetted app** successfully coexisting with separate named volumes/resource limits; otherwise keep the original one-app demo.
5. A negative test: reject a manifest requesting more RAM than available, a privilege escalation or another app's storage access.

**Do not build during hackathon:** AWS/Azure/GCP cross-account IAM federation, Kubernetes admission, Terraform/OpenTofu orchestration, cross-cloud billing, per-app microVM sandboxing, globally distributed storage, native desktop runtime, or Wazi's own custom kernel/container technology. These should appear as verifiable ADRs and roadmap entries.

### 30/60/90 day plan after hackathon

| Period | Focus | Exit gate |
| --- | --- | --- |
| 0–30 days | Linux Host Agent trust pairing, read-only inventory, capacity policy, typed Compose and persistent volume isolation | Two supported apps with independent resource/secret/volume boundaries, tested deny paths |
| 31–60 days | BYOC connector interface, first provider IAM adapter (choose based on pilots), more robust audit and revocation; live quota versus capacity classification | Customer revokes Wazi permissions and new operations immediately fail; cross-tenant isolation verified |
| 61–90 days | Namespace-scoped Kubernetes beta or second provider adapter **based on user demand**, backups and restore testing, constrained capacity/metering dashboard | Three pilot customers, measured use/savings and completed restore/denied privilege tests |
| 3–12 months | Provider adapter conformance suite, hardened sandbox RuntimeClass/Kata or dedicated VM target, portability and publisher isolated-customer SDK | One independent developer and multi-provider integration validated |
| Long term | More providers, application portability, Wazi Connect performance, optional managed-host offers | Repeatable customer migration, measured reliability, no unsupported speed/security claims |

## 10. Security/quality acceptance tests

- **AuthN/AuthZ:** forged connect challenges rejected; credentials scoped to intended project/account/resource; no shared tenant external ID; revoked role/certificate denies new operations; read-only connection cannot deploy.
- **Isolation:** app A cannot read app B volume or secrets; app network cannot reach internal host control plane outside allowlist; privileged and Docker-socket workloads rejected; Kubernetes namespace scope prevents changes elsewhere; error logs cannot contain credentials.
- **Resources:** insufficient measured memory/disk blocked; CPU/memory limits genuinely enforced by host; conflicting ports blocked; quota unknown not mislabeled enough; race between plan and apply rechecks capacity.
- **Persistence:** app data survives container replacement, upgrade and rollback; restores work to a clean host where supported; deletion requires explicit data-destruction approval.
- **Failure:** provider API unavailable returns no destructive retry; signed plan stays immutable; stale capacity/credential/manifest checks fail closed; Wazi dashboard outage does not make independent recovery impossible.
- **Supply chain:** only signed/reviewed immutable artifacts for promoted apps; SBOM and license evidence; prompt injection in upstream manifests/docs cannot override role, policy or approvals.
- **Economics:** show actual provider-billed compute/storage/egress and software license charges separately; don't claim free storage because capacity looks idle.

## 11. Immediate issue backlog (proposed)

1. **BYOC-ADR-001**: Accept user-provided capacity as default Wazi Cloud mode; document ownership and liability.
2. **BYOC-ADR-002**: Provider connection proof, short-lived credentials and authorization escalation model.
3. **BYOC-SCHEMA-003**: Versioned provider-neutral capacity/manifest contracts and capabilities matrix.
4. **BYOC-AGENT-004**: Controlled VPS pairing, mTLS rotation and no-shell typed operations.
5. **BYOC-CAPACITY-005**: Real host resource inventory and deterministic admission engine.
6. **BYOC-RUNTIME-006**: Compose adapter with per-app limits/volumes/network/secret boundaries.
7. **BYOC-PERSIST-007**: Separate lifecycle: reinstall/upgrade without deletion; backup and restore test.
8. **BYOC-SECURITY-008**: Deny unsafe privilege, volume, network requests and test logs for secrets.
9. **BYOC-OCI-009**: OCI manifest/builder conformance fixtures, adapter runtime support capability negotiation.
10. **BYOC-CLOUD-010**: AWS cross-account scoped role proof-of-concept after pilots.
11. **BYOC-K8S-011**: Namespace-scoped Kubernetes adapter with RBAC/quotas/PSS/NetworkPolicy/PVC validation.
12. **BYOC-DR-012**: Independent signed recovery bundle and provider revocation drill.

## 12. Critical decisions and non-goals

Decide: Which compute proof level appears in UI? Is Wazi allowed to install only Wazi-reviewed images, or can advanced users self-assume risk under separate trust tiers? Which provider adapter pilots first? Does Wazi ever store app data? Is remote support access enabled only by time-bound explicit grants? How will privacy, jurisdictional retention and off-platform backup keys work?

Non-goals: a generalized hypervisor scheduler in v1, replacing Docker/Kubernetes, guaranteed access to unused cloud credits, ability to deploy any unknown SaaS without adaptation, perpetual no-downtime workloads, provable physical hardware isolation for ordinary VMs, or direct access to provider billing that isn't exposed/authorized.

## Sources: official primary technical references

- [AWS Well Architected: share resources securely with third parties](https://docs.aws.amazon.com/wellarchitected/2024-06-27/framework/sec_permissions_share_securely_third_party.html)
- [AWS security team: cross-account SaaS access](https://aws.amazon.com/blogs/security/how-to-improve-cross-account-access-for-saas-applications-accessing-customer-accounts/)
- [Google Cloud workload identity federation](https://docs.cloud.google.com/iam/docs/workload-identity-federation)
- [Azure RBAC resource scope](https://learn.microsoft.com/en-us/azure/role-based-access-control/role-assignments)
- [Kubernetes: least-privilege RBAC guidance](https://kubernetes.io/docs/concepts/security/rbac-good-practices/)
- [Kubernetes: LimitRange and ResourceQuota](https://kubernetes.io/docs/concepts/policy/limit-range/)
- [Kubernetes: NetworkPolicy implementation requirement](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [Kubernetes: enforcing Pod Security Standards](https://kubernetes.io/docs/setup/best-practices/enforcing-pod-security-standards/)
- [Kubernetes: StorageClass reclaim policies](https://kubernetes.io/docs/concepts/storage/storage-classes/)
- [Docker: resource limits and kernel prerequisites](https://docs.docker.com/engine/containers/resource_constraints)
- [Docker: persistent volumes and lifecycle](https://docs.docker.com/engine/storage/volumes/)
- [Kata Containers: virtualization boundaries](https://kata-containers.github.io/kata-containers/design/virtualization/)

**Validation note:** This is a standards-grounded proposal, not a claim that Wazi has live provider accounts, delegated IAM, host agents, sandboxed workloads or production SLAs. Before release, evaluate actual versions/provider limits with real pilot resources and security tests.
