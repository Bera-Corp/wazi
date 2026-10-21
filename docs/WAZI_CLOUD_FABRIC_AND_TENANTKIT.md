# Wazi Cloud — Customer-Owned Compute, Shared Application Runtime and Developer Silo SDK

> **Status:** Research-backed architectural proposal; **not implemented**.  
> **Date:** 21 October 2026. **Maintainer:** Bera-Corp / Wazi.  
> **Design authority:** Wazi original security, context, signed-manifest and non-generic-shell requirements remain binding.  
> **Related:** [Wazi master plan](WAZI_MASTER_PLAN.md) · [Hackathon execution plan](HACKATHON_2026_EXECUTION_PLAN.md) · [Architecture](ARCHITECTURE.md) · [Context](CONTEXT_ENGINEERING_PLAN.md) · [Threat model](THREAT_MODEL.md) · [Preview program](PREVIEW_AND_INSIDER_PROGRAM.md).

## 1. Executive recommendation

Build **Wazi Cloud Fabric** as a later phase of Wazi: **one logical customer compute workspace running several compatible applications**, with per-app limits, isolated data volumes, verified deployment recipes and a customer-controlled infrastructure profile. Wazi's shared control plane coordinates catalogue, identity, subscriptions, policy, manifests, updates, evidence, monitoring and provider adapters. Application data and main compute remain within each customer's runtime.

**Wazi TenantKit** is a complementary developer-facing toolkit: it helps developers distribute self-hostable or isolated-customer editions of their applications without having to build the entire shared application-level multi-tenant data plane. It cannot magically convert any existing SaaS into a silo deployment or remove developer responsibilities for authentication, security, licensing, upgrades, support, and data compliance.

**Wazi Connect** is a longer-term optional access layer for secure browser, API, private-network, selective filesystem sync and remote-workstation use cases. It must use established transport and network security mechanisms and **must never promise LAN/native speeds across the Internet**.

**Central differentiation:** "One customer's compute, many trusted applications, one governed control plane" plus an app-store experience and a publisher bridge. This concept extends, rather than replaces, Wazi's existing local/BYO VPS Docker Compose wedge.

### Important economic qualification

SaaS fees cover more than compute and storage: product R&D and maintenance, licensed IP, security response, enterprise sales, customer support, payment processing, compliance, uptime, global delivery, identity, abuse prevention and profit. Shared customer compute could reduce *duplicated application infrastructure overhead*, not erase the value or cost of software licenses and human operations. Some centrally managed services (e.g., collaborative network products, cloud AI APIs, proprietary Adobe-like services) may not be legally licensed or technically packaged for customer-hosted instances at all. Wazi must not imply every subscription can be replaced.

## 2. Existing concepts and competitive validation

The direction is **not fundamentally unprecedented**, which validates demand but limits novelty claims.

| Established model/product | Demonstrated capability | Wazi's possible distinction / gap |
| --- | --- | --- |
| AWS SaaS Lens silo/pool/bridge | Tenant-dedicated and hybrid application topologies with centralized management | Consumer-oriented cross-vendor app catalogue and one customer compute subscription |
| Northflank BYOC | Centrally managed workloads in customer-owned VPC/cloud account | Multi-publisher marketplace, per-customer sharing across unrelated applications, consented lifecycle intelligence |
| Cloudron, Umbrel, YunoHost and Coolify | Self-hosted app deployment, catalogue/templates, reverse proxy and upgrades in different forms | Declarative shared compute contract, independent publisher SDK, compatible cloud-provider bridge, portable trust/evidence |
| Fly.io Machines | MicroVM-based hosting, independently billed compute/volumes and network | Customer-level capacity aggregation across marketplace apps, unified licensing and support model |
| WireGuard/Tailscale | Encrypted network access, direct peers and relay fallback | Wazi UX, identity/authorization bridge, purpose-specific application connectors; **not** a new VPN protocol |
| Syncthing, rclone, WebDAV, RDP/WebRTC | Different file synchronization, remote data and screen access patterns | Context-aware connector configuration and app integration, not a claim to replace all transport stacks |

Sources: [AWS SaaS Lens](https://docs.aws.amazon.com/wellarchitected/latest/saas-lens/silo-pool-and-bridge-models.html), [Northflank BYOC](https://northflank.com/features/bring-your-own-cloud), [Fly updated Oct 2026 pricing](https://fly.io/pricing-update/), [Tailscale connection types](https://tailscale.com/docs/reference/connection-types), [Cloudron](https://www.cloudron.io/), [Coolify](https://coolify.io/), [Umbrel](https://umbrel.com/), [YunoHost](https://yunohost.org/).

These vendors solve overlapping problems; Wazi must validate with actual users whether combining BYOC + cross-publisher app licensing + familiar installation UX is an attractive distinct product.

## 3. Product concepts and definitions

| Concept | Definition | Boundary |
| --- | --- | --- |
| **Customer Workspace** | A single customer/organization's application, data, policy, identity and billing scope | Cannot be silently shared with other customers |
| **Compute Instance** | VM belonging to a customer workspace; may share the provider's physical hardware | Logical dedicated VM is **not** dedicated bare metal |
| **Application Instance** | One versioned and policy-managed install of a publisher's app inside a workspace | Shares VM with other apps only if trust/capacity policies allow |
| **Control Plane** | Wazi's centralized tenant-aware metadata, scheduling, receipt and entitlement service | Must not hold arbitrary customer data or privileged remote shell |
| **Data Plane** | Customer workloads, containers, volumes, databases, queues and backups | Owned/billed according to explicit hosting model |
| **Wazi TenantKit** | SDK/manifest contract for developers shipping isolated-customer editions | Does **not** bypass application security or eliminate the need for tenancy in Wazi control plane |
| **Wazi Connect** | Authorized client connectivity and sync adapters | No default access to arbitrary host files, ports or raw disks |

**Terminology:** Multi-tenancy still exists at Wazi control-plane level. Per-customer applications may run in *siloed* instances instead of one large pooled publisher-hosted application. One customer's VM running multiple different apps is **per-customer service consolidation**, not pooling multiple customers in one unisolated server.

## 4. Three infrastructure ownership modes

| Mode | VM/cloud account ownership | Customer powers | Wazi powers and revenue |
| --- | --- | --- | --- |
| **Local / BYO server** | Customer | Host admin, physical capacity and keys | Guided provisioning and optional management subscription |
| **BYOC account** | Customer's cloud provider account | Cloud bills, provider IAM, network policy; optional root via provider | Delegated narrowly scoped deploy/manage role, management fee |
| **Wazi-managed private VM** | Wazi or infrastructure partner's billing account; one logical VM per customer | Start/stop, quotas, volume policy, app lifecycle, backups, export and restore; root optional by plan | Compute/storage/network pass-through plus explicit managed service fee/margin |

**Ownership transparency:** A provider-billed VM and a Wazi-managed VM differ materially in actual control and exit rights. Do not claim that a Wazi-owned VM is "customer-owned infrastructure" just because it is logically dedicated. Clearly label data export rights, encryption-key custody, who can access snapshots, support override access, provider outage responsibilities and billing liabilities.

For *no-root* customers, Wazi exposes bounded capabilities, log access, safe shell alternatives where appropriate, resource monitoring, snapshots and backup/export APIs. Never falsely advertise "full VM control." For developer-managed apps, publishers get scoped ability to update or troubleshoot only **their application** after workspace owner authorization, not administrator control over a customer's other services.

## 5. Reference architecture

~~~text
                       WAZI CONTROL PLANE
                 identity / billing / marketplace
             manifests / policies / approval / audit
                tenant inventory / license broker
                         | signed typed intents
                         | mutually authenticated
                         v
                    PROVIDER ADAPTER
             local | BYOC | managed VM / VPS
                         |
    +--------------------+----------------------------+
    |            CUSTOMER WORKSPACE A                 |
    |    Dedicated logical VM or authorized server     |
    |  +---------------------------------------------+ |
    |  | Wazi scoped host agent + capability sandbox  | |
    |  +---------------------------------------------+ |
    |  | Ingress/Auth gateway (workspace scoped)      | |
    |  | App 1: documents + its private data volume   | |
    |  | App 2: automation + its private data volume  | |
    |  | App 3: notes/CRM + its private data volume   | |
    |  | Optional per-app DB/queue and connectors      | |
    |  | Backup/restore and health receipts            | |
    |  +---------------------------------------------+ |
    +--------------------+----------------------------+
                         | encrypted authenticated access
                         v
              CUSTOMER DEVICES / BROWSERS
         HTTPS / native client / private overlay
              sync cache / remote application
~~~

A second customer gets a separate VM and secret domain. No cross-customer storage sharing by default. A customer can optionally choose dedicated app VMs if trust, compliance, threat model or sustained resource use requires it.

**Potential implementation stack** (proposed, not installed): Existing Wazi React/Vite/TypeScript UI, Go control-plane and host agent, PostgreSQL metadata, object storage for immutable signed manifests and evidence, Docker Compose on Linux x86_64, mutually authenticated agent connections, narrowly scoped provider adapter, one local capacity scheduler with cgroup v2 quotas, rootless containers where feasible, encrypted volumes/backups, HTTPS/TLS proxy and optional private overlay. The provider integration boundary may later support OpenTofu or provider APIs for provisioning, with human policy approval. No Kubernetes, distributed cluster scheduler or autoscaler in first version.

## 6. How a developer ships an application without pooled SaaS multi-tenancy

### Application deployment contract

A developer publishes an installable **customer-isolated edition** with explicit prerequisites:
- immutable image digest or verifiable reproducible build and SBOM;
- private storage/dependency declarations and database migration policy;
- resource envelopes, quotas, ports, domain/TLS and data export support;
- inbound/outbound network permissions and data residency requirements;
- independent app configuration, secrets references and health/backup/recovery procedures;
- upgrade/downgrade compatibility and channel-specific release contracts;
- auth strategy: workspace SSO/OIDC or application-local accounts, with explicit permission boundaries;
- licensing: open-source terms or publisher subscription/license entitlement;
- support path and optional publisher diagnostics consent.

This lets a developer focus on **one customer's app instance** while Wazi handles repeatable provisioning across customers. It does not erase complexity in updates, version skew, tenant authorization, compliance, app support, replication, backups, regional outages or identity federation.

### Hypothetical developer manifest excerpt — not a published schema

~~~yaml
schema: wazi.cloud-app/v0-draft
id: org.example.crm
release: 0.3.0
distribution:
  mode: customer-isolated
  license: AGPL-3.0-only  # example only; publisher chooses real licence
  publisher_entitlement: optional
artifact:
  image: registry.example.com/crm@sha256:REPLACE_REAL_DIGEST
  sbom: cyclonedx:REPLACE_REAL_REF
runtime:
  min_arch: [x86_64]
  os: linux
  engine: docker-compose
  resources:
    cpu_reservation: "0.25"
    cpu_limit: "1.0"
    memory_reservation_mb: 384
    memory_limit_mb: 768
    disk_gb: 5
  storage:
    - id: appdata
      mode: private-volume
      backup: required
      encryption: required
  network:
    ingress: workspace-authenticated-https
    egress: [publisher_updates]
  secrets: [db_password, app_secret]
  allow_privileged: false
lifecycle:
  health: /health
  backup: required
  restore_test: required
  upgrade: reviewed
  downgrade: only_if_proven
identity:
  oidc: optional
  tenant_id_source: workspace
  developer_support_access: requires_owner_approval
data:
  export: supported
  deletion: supported
~~~

**Security interpretation:** The manifest is a candidate until schema checks, image attestation, policy, license review, sandbox tests, backup and restore validation and human approval promote it. The developer's claimed entitlements cannot override Wazi host policy; image digest and package signing are mandatory for promoted builds. Some arbitrary Docker workloads require capabilities that the first Wazi host agent should refuse; those remain ineligible until securely supported.

### TenantKit SDK (possible future)

Proposed API: request a workspace-scoped app identity, resolve secrets by opaque references, register health, provide canonical export/backup hooks, expose structured usage meters, verify publisher entitlements without centralizing app user data, publish capabilities/permissions in manifest, and return sanitized support diagnostics only with customer consent.

The toolkit should be language-agnostic at the core via versioned HTTP/JSON/OpenAPI and a local agent socket; TypeScript/Node and Python bindings are optional adapters. Fleet supplies governance/verification; it is not a substitute for application authorization. MissionKit and Meciria may integrate via versioned optional APIs, not cross-product table writes.

## 7. Sharing resources safely within one customer

Three compatible small applications might share one VM more economically than three separately provisioned VMs, **only if** aggregate memory, peak CPU, I/O and persistent storage do not exhaust that VM. RAM is generally committed even while many apps are idle; low average CPU alone does not imply easy consolidation. Databases, search indexes, AI inference and image processing can spike memory, CPU and disk unexpectedly.

Introduce a **Capacity Planner**:
- inspect app minimum and recommended CPU/memory/storage and peak measured/estimated usage;
- define separate required *reservation* and enforced *limit* per app;
- reserve OS/host-agent/reverse-proxy headroom, plus margins for backups and upgrades;
- reject manifests with insufficient guaranteed memory, storage or I/O isolation;
- set per-app cgroup CPU/memory/PIDs, volume quotas where supported, timeouts and rate limits;
- show burst headroom and "noisy neighbor" risk inside the customer's VM;
- allow upgrades to a larger VM, moving one app to a second VM, or advanced per-app VM isolation.

**Do not overcommit RAM in the MVP.** CPU sharing is generally more tolerant; storage capacity and I/O contention need independent controls and alerts.

### Economic modeling: separate three cost streams

Customer monthly total =
1. **Infrastructure:** running VM + block/object storage + backup snapshots + network transfer + IP, DNS, provider-specific fees;
2. **Wazi operations:** provisioning, patch management, support, security monitoring, identity/control plane, payment costs, incident response, backup validation; and
3. **Application rights:** paid publisher license/support, paid API/model usage or free/open-source rights.

A shared customer VM can reduce duplicated infrastructure **but does not make software licensing or AI inference free**. A previously discussed 5% AI API resale uplift is a separate *hypothesis*, not a viable default markup for every cloud service; it may fail to cover payment, support and fraud costs.

**Illustrative model—not a vendor quotation or forecast:**

| Monthly category | Hypothetical USD |
| --- | ---: |
| Customer VM compute | $20 |
| Persistent storage | $5 |
| Backup snapshots | $3 |
| Network/other overhead | $4 |
| Wazi management | $8 |
| **Total before app licenses** | **$40** |

If three legally self-hostable services would otherwise cost $18 each monthly, their $54 gross subscription total compares to this **hypothetical** $40 base only before considering publisher fees, migration, storage growth, support, taxes, uptime/HA expectations and application equivalence. The gross difference of $14 is not guaranteed savings. A redundant HA setup, high-egress file use, paid models, managed databases or high support burden can reverse the comparison. A 4 GB VM can also be too small for some combinations.

**Current external pricing validation:** Fly announced pricing effective October 1, 2026 with running machine compute billed per second, separately billed provisioned volumes and region-dependent egress; volume pricing is $0.15/GB-month and snapshots $0.08/GB-month (first 10 GB snapshots free), illustrating why a simple "one VM price" is incomplete. Prices vary substantially by region/size and provider. See https://fly.io/pricing-update/ .

**Research tests:** Collect 10 consenting pilot deployment cases, measure actual VM P50/P95 CPU/RAM/IO/egress, installation outcome, restore success, operational labor and full bill. Compare to representative alternatives of equivalent functionality and reliability, not merely list prices from unrelated SaaS products.

### Pricing offer hypotheses to validate

- **Wazi Free/BYO:** open catalogue + local install; customer pays own infrastructure. Wazi optional management fee.
- **Wazi Private Compute:** provider VM/storage billed transparently plus tiered Wazi operations fee; customer sees breakdown.
- **Wazi Publisher:** free listing/installer; advanced support automation, managed updates and optional analytics billed to publisher.
- **Per-app publisher license:** if paid, kept explicit and separately priced; Wazi must enforce licensing and remit appropriate revenue.
- **Metered models/API:** separate pass-through meter with budgeting, model authorization, privacy controls and clearly stated fees.
- **Enterprise:** BYOC/provider-owned accounts, tenant-level isolated stacks, audit and compliance add-ons.

Avoid assuming subscription elimination: Wazi itself has continuing cloud/platform operations costs. Be explicit about termination, data export, backup recovery, retention and delinquent-account lifecycle.

## 8. Developer and customer user journeys

### Customer

1. Explore Wazi Store and see a vetted app with host resource demands and exact publisher license.
2. Choose Local, BYO-VPS/cloud or Wazi Managed Private Compute.
3. See a provision plan: total estimated VM/storage/bandwidth, required RAM and predicted headroom.
4. Review data/control permissions, support access terms and backup policy; explicitly approve.
5. Wazi provisions or pairs with selected host, verifies signed app package, installs and checks health.
6. Customer adds a second app; Wazi capacity planner checks coexistence instead of provisioning an unrelated VM by default.
7. Customer sees each app's isolated storage, CPU/memory usage, permissions, updates, backups and developer access.
8. Customer can revoke publisher access, export supported data, move compatible apps to another provider and cancel with a documented offboarding process.

### Indie developer

1. Publish an isolated-customer app variant, an actual license, support policy and the Wazi cloud manifest.
2. Use TenantKit or simple Wazi execution contract; no need to build its own pooled multi-customer database.
3. CI verifies exact app/build, health, permissions, migration, license, SBOM and restoration on supported host matrix.
4. Wazi marketplace distributes and provisions approved packages on customer-controlled workspaces.
5. Developer gets authorized coarse aggregate usage and opt-in sanitized bug data; never unrestricted access to customer app data.
6. Publisher submits new immutable build; Wazi policy/version gates determine staged channel and per-customer rollout, with owner-configurable schedule.

## 9. Wazi Connect: secure local-feeling access, not impossible networking

The user experience may look local, but *actual network physics* remains. A remote VM does not offer local SSD latency or native PCIe bandwidth on commodity home internet. Performance depends on physical round-trip time, direct vs relayed connection, available upload/download, packet loss, client CPU/GPU, codec, encryption overhead, and workload design.

**Four connector classes, each with different behavior:**

| Connector | Initial technology | Typical use | Realistic limitation |
| --- | --- | --- | --- |
| Web application gateway | HTTPS, WebSockets, OAuth/OIDC, CDN/static asset cache | Browser-first apps, dashboards, CRM | Every interactive uncached action still incurs network latency |
| Private application tunnel | WireGuard-compatible overlay or customer-controlled VPN; identity-scoped access | Internal apps, SSH-like admin capability, private APIs | Direct P2P not guaranteed; relay can reduce throughput and add RTT |
| Selective file sync | Existing tools/protocols such as WebDAV, Syncthing and cached mounts with conflict strategy | Recent working files, offline access | First sync can be large; two-way edits and data leaks require careful policy |
| Remote desktop/application streaming | Established RDP/SPICE/WebRTC stack, adaptive codecs | GUI-heavy apps where processing remains in VM | Latency, GPU encoding, bandwidth, licensing and input security; not native-speed promise |

Tailscale documents direct UDP, DERP relay and peer-relay paths with different performance, despite all being end-to-end encrypted. This reinforces testing direct/relayed measurements before making speed claims: https://tailscale.com/docs/reference/connection-types .

**Suggested future Wazi client:** small desktop tray (Windows/macOS/Linux), list customer workspaces/apps, negotiate authenticated connection, offer browser/native remote access per app, display connection mode/RTT and quota, mount only user-approved sync folders, support offline cache with safe conflict resolution, and provide revoke/disconnect. Never silently mount the whole server drive or grant broad LAN access.

**Acceptance measurements:** P50/P95 input-to-response latency, throughput in Mbps, TTFF (time to first frame), 95th-percentile frame stutter, percent direct vs relayed, transfer accuracy, sync conflict rate, power consumption and monthly egress. Report by region/network; no universal promise of "almost native speeds." Native-like *interface experience* is a design goal, not a measurable bandwidth guarantee.

## 10. Security, privacy and operations

This model has new risk boundaries: independent publisher containers share one customer's VM, potentially allowing a compromised app to inspect other apps or exploit the kernel. VM-level per-customer isolation does **not** automatically provide sufficient per-app security. Container isolation is weaker than running each untrusted app on its own VM.

**Launch policy:** Only curated, licensable, reproducibly packaged and reviewed apps can cohabit. Enforce user namespaces/rootless where possible, dropped Linux capabilities, seccomp/AppArmor, read-only filesystem where possible, separate secrets/volumes/network policy, per-app allowlisted outbound connections, no host Docker socket mounts, no privileged containers, no host PID/network namespace. Reject apps that demand broad host privileges or shared credentials. Require a VM/container sandbox for high-risk untrusted code rather than pretending hardening makes it safe.

**Control plane compromise** must not grant unrestricted customer root shell. All actions are signed, version-bound, scoped, auditable and separately authorized. Root/BYOC admin access is customer-owned; Wazi operations get limited capability grants with expiries and owner approval.

**Data ownership & incident response:** encryption in transit/at rest, secrets broker and rotation, independent backup/key handling, data residency, evidence minimization and retention, deletion/export, maintainer support consent, tenant isolation tests, emergency revocation, supply-chain attestation and disaster recovery independent of Wazi dashboard availability. Existing Wazi \`THREAT_MODEL.md\` and \`CONTEXT_ENGINEERING_PLAN.md\` outrank convenience. Data processing roles differ among Wazi, cloud provider and individual software publishers; contracts and user notice must reflect real access.

**Lifecycle guarantees:** Support tested backup and restore before declaring an app protected. A backup image is not the same as successful restore. Encrypted backup storage and clear recovery steps must be available when the Wazi central control plane is unreachable; customer must retain a viable export path.

## 11. Product engineering interfaces and dependencies

### Subsystems

- **cloud-workspace registry:** tenant-scope, requested provider/region, selected operational mode, spend policy, data-residency flags.
- **provider adapters:** one Linux VPS first, later BYOC accounts via short-lived provider credentials and least-privilege provisioning roles.
- **workspace agent:** existing Wazi Go typed capability surface, scoped deployment intents and immutable receipts.
- **capacity planner:** deterministic scheduling on reservations/limits and compatibility policy.
- **app tenancy contract:** TenantKit adapter and versioned application manifest; app identities and publisher auth.
- **entitlement service:** customer/app permissions and license receipts; no control of unrelated VM data.
- **backup coordinator:** policy + restore tests + export flow + off-platform recovery data.
- **metering + bill explainer:** per customer VM and app usage/resource estimates, provider pass-through and SaaS license components.
- **connection broker (future):** identity-scoped endpoints for Tailscale/WireGuard or similar external infrastructure, HTTPS gateway and streaming/sync adapters.
- **observability:** scoped health and safe metrics, alerting and opted-in bug feedback.

Separate **control-plane tenant isolation** from **application runtime isolation**; do not mix authentication authority for unrelated apps. Shared customer VM needs per-app OIDC claims and secret namespaces.

### Example proposed API surface

~~~text
POST /v1/workspaces/{id}/compute-plans          # deterministic, read-only proposal
POST /v1/workspaces/{id}/compute                # owner-approved provision
GET  /v1/workspaces/{id}/capacity               # actual headroom + reservations
POST /v1/workspaces/{id}/apps/install-plans     # compatibility and risk preview
POST /v1/workspaces/{id}/apps                   # owner-approved typed deployment
GET  /v1/workspaces/{id}/apps/{app}/status
POST /v1/workspaces/{id}/apps/{app}/export-plan
POST /v1/workspaces/{id}/apps/{app}/backup-test
POST /v1/workspaces/{id}/connect/sessions       # future short-lived app-scoped grant
~~~

All mutating requests require authenticated scoped principals, explicit policy and idempotency keys, optimistic version checks, traceable approval evidence and tenant binding. Public URLs do not authenticate privileged commands. API names are proposals only.

## 12. Phased roadmap and hackathon cut-line

### For the October–November 2026 Meta Indie hackathon: **concept in UI, not a distributed cloud product**

Do **not** make this a new dependency for the current working demo. The approved critical path remains GitHub repo → Fleet audit → Wazi manifest → real OSV evidence → Meta AI proposed repair → tests and human review → a *single controlled Linux Docker Compose deployment* → marketplace listing. This controlled target may be presented as a **future Customer Workspace reference**, but do not claim an independent customer billing service, multi-cloud orchestration, true hardware isolation, portable shared capacity or real direct-connect performance.

Only if the critical path is done and robust, add an illustrative read-only "Your Compute" capacity dashboard showing one VM and two known safe sample services. Must use actual VM measurements or clearly label mock values.

### Phase 1 — post-hackathon foundation (0–30 days)

Define ADRs for logical tenancy, provider ownership, isolation guarantees, data migration, app support roles and licensing. Implement machine-checkable workspace/app/resource manifests with fake provider fixture and tests. Extend existing Wazi controlled runtime to explicitly model **one customer + one VM + two compatible apps**, no external cloud account provisioning. Unit-test co-location rejection for insufficient memory, dangerous privilege requests, port collisions, incompatible licenses and cross-app secret boundaries.

**Exit:** Two vetted Docker Compose apps deploy to one disposable Linux VM, with separate persistent volumes, network scopes and health/rollback reports, no app root privileges.

### Phase 2 — controlled private alpha (30–90 days)

Add BYO-VPS provider pairing to scoped agent, resource monitor, capacity planner, per-app backups and restore verification, cost estimate breakdown. Support customer role separation from publisher and break-glass audited support mode. User can add app, see headroom and export/delete data without depending on Wazi retaining root access.

**Exit:** At least three non-production pilot customers, reproducible two-app lifecycle, tested restore and cross-tenant denial, full cost accounting including Wazi overhead.

### Phase 3 — provider-backed workspace (3–6 months)

Add first cloud provisioning provider with approval and least-privilege IAM, quotas, durable billing/metering, spend caps, export workflow and provisioning recovery. Public/private network modes, OAuth/OIDC identity gateway and secure DNS/TLS; no unsupported platform promises. One provider, one region, one supported Linux image and one VM family at first.

**Exit:** Provision, resize, update, migrate/export, suspend, terminate and restore supported customer workspace entirely through audited typed operations; no leaked credentials; measured provider bill matches Wazi breakdown.

### Phase 4 — developer adoption (6–9 months)

Release early TenantKit contract/SDK and conformance fixtures, publisher documentation, entitlements and optional paid licenses, developer issue bridges and consented diagnostics. Support a handful of independent publisher-built self-hostable editions.

**Exit:** External developer ships app through Wazi without building a pooled tenant-data backend, with tested versioned upgrade and per-customer data export.

### Phase 5 — connectivity and providers (9–15+ months)

Pilot Wazi Connect with one secure private app tunnel and optional selective file sync; record direct/relay performance by region. Later investigate adaptive remote GUI streaming only when supported use case justifies its high engineering and GPU cost. Expand BYOC/provider options and advanced isolation after measured demand.

**Exit:** Independent connectivity and support security review, measured P95 performance against ordinary HTTPS/VPN baselines, no unapproved raw file access and tested disconnected recovery.

## 13. Core risk register

| Risk | Severity | Mitigation |
| --- | --- | --- |
| Developer mistakes customer-isolated deployment for zero-tenancy responsibility | High | Honest TenantKit scope and threat-model templates |
| VM consolidation masks RAM/disk and noisy-neighbor spikes | High | Reservations, hard limits, headroom checks and safe split migration |
| Compromised app escapes its container to other apps in same VM | Critical | Curated trust tiers, sandbox hardening, reject privileged apps, dedicated VM fallback |
| Wrong license or proprietary service cannot be mirrored/self-hosted | High | Publisher permission, license conformance and opt-in partner contracts |
| Backups untested or app downgrade corrupts data | Critical | Restore tests, version-aware migrations, owner approval and recovery |
| Wazi central control plane has excessive customer-provider IAM | Critical | Least privilege, short-lived scoped credentials, no general shell |
| "Almost-native" speed claim is physically untrue | Medium | Per-network benchmarking, local cache and direct/relay disclosure |
| Billing subsidy or 5% markup fails to cover support/egress | High | Separate provider cost/ops margin/API metering; pilots and burn caps |
| Customer thinks Wazi-managed VM is owned in their cloud account | High | Contractual mode labeling and data/key/backup custody disclosure |
| Loss of Wazi dashboard prevents recovery | High | Local signed desired state, customer export/restore and offline runbook |
| Many independent app developers add support complexity | High | Eligibility gate, tested recipes, rollback, support boundaries |
| Building Cloud Fabric derails hackathon demo | Critical | Explicit scope gate and a separate roadmap and reference architecture |

## 14. Priority backlog and first technical spike

1. **CLOUD-ADR-001:** Define Local/BYO/Wazi-managed VM ownership, admin powers, recovery rights and application isolation modes.
2. **CLOUD-ADR-002:** Define customer workspace, per-app namespace, identities, network and secret separation. Map to original Wazi threat model.
3. **CLOUD-SCHEMA-003:** Draft \`wazi.cloud-app/v0\` declarative contract, validation and malicious manifest test fixtures.
4. **CLOUD-CAPACITY-004:** CPU/RAM/storage and headroom calculator with failure on overcommit and port collision.
5. **CLOUD-AGENT-005:** Pair one disposable VM, install two curated Compose apps on separated volumes/networks.
6. **CLOUD-BACKUP-006:** Backup/restore and off-platform recovery tests for both apps.
7. **CLOUD-BILLING-007:** Transparent cost model including egress, backup, licensing, tax/support, and model usage.
8. **CLOUD-AUTH-008:** Workspace customer vs publisher roles, timed owner approval and revocation.
9. **CLOUD-DEVSDK-009:** TenantKit SDK draft and reference minimal isolated app.
10. **CLOUD-CONNECT-010:** Benchmark direct vs relay private access and HTTPS/file sync under reproducible network profiles (research spike only).

**Smallest credible spike after the core hackathon demo:** Start an Ubuntu VM, deploy two real curated low-resource apps via separate Wazi-style signed manifests and Docker Compose project names, set cgroup limits, verify they cannot read each other's secrets or volumes, inspect actual peak RAM/CPU, and demonstrate backup/restore and safe stop/start. No credit card, no provider partnerships, no new VPN, no multi-region Kubernetes.

## 15. Primary sources and evidence caveats (21 October 2026)

1. **AWS SaaS Lens: Silo, Pool and Bridge Models** — https://docs.aws.amazon.com/wellarchitected/latest/saas-lens/silo-pool-and-bridge-models.html (validates dedicated tenant stack plus shared control-plane concept, not Wazi's economics).
2. **AWS SaaS Tenant Isolation Strategies** — https://docs.aws.amazon.com/whitepapers/latest/saas-tenant-isolation-strategies/silo-isolation.html (explains strong isolation vs cost/agility tradeoffs).
3. **AWS SaaS Storage Strategies** — https://docs.aws.amazon.com/whitepapers/latest/multi-tenant-saas-storage-strategies/saas-partitioning-models.html (describes partitioning models, tradeoffs).
4. **Northflank BYOC product docs** — https://northflank.com/features/bring-your-own-cloud (current real-world control-plane/BYOC precedent; vendor claims require own testing).
5. **Northflank June 2026 BYOC analysis** — https://northflank.com/blog/what-is-byoc-in-cloud-computing (architecture patterns and business limitations).
6. **Fly.io updated pricing effective October 1, 2026** — https://fly.io/pricing-update/ (compute, storage, snapshot and egress line items; vary by region).
7. **Tailscale connection types** — https://tailscale.com/docs/reference/connection-types (direct/relay effects and encryption).
8. **Tailscale app connectors** — https://tailscale.com/docs/features/app-connectors (application-scoped network routing patterns).
9. **Syncthing network/synchronization tuning** — https://docs.syncthing.net/users/tuning.html (peer-to-peer file sync and relay tradeoffs).
10. **Wazi original repo architecture and context control** — https://github.com/Bera-Corp/wazi/blob/main/README.md and https://github.com/Bera-Corp/wazi/blob/main/docs/CONTEXT_ENGINEERING_PLAN.md .

**Research limitations:** The price example is illustrative, not a quote; the competitive differentiation, product demand, cost saving and customer willingness-to-pay are hypotheses to test. This document is not an assertion of service deployment, available cloud-provider commercial agreements or near-local WAN transport speeds. Reverify third-party prices, rules and policies before implementation.
