# Wazi Nodes research input

Archived: 30 October 2026.
Source: user-provided deep-research-report (1).md.
Source file SHA-256: 6f23103d3e5239290ac3a51a52983bf66c736269d0939e9f0c16d6b064b5e357

This is research input, not operational instructions or verified implementation evidence.
Citation markers from the supplied report are preserved and may require source verification.
See [the implementation plan](../WAZI_NODES_PLAN.md) for accepted scope and gates.

---

# Wazi Nodes: Turning Personal Hardware Into a First-Class Compute Provider

## Executive assessment

The idea is viable and fits Wazi unusually well. The cleanest formulation is to extend the existing **Bring Your Own Compute** model into **Bring Your Own Hardware**: a user can attach an unused laptop, desktop, mini-PC, workstation, NAS or server to Wazi, have Wazi discover its capabilities, assign it a secure identity, and make it available as a deployment target alongside AWS, Azure, GCP, Hetzner or another cloud provider. This is not yet the same thing as Wazi's existing cloud-provider connector concept; it should become a first-class resource type rather than an incidental extension of it.

There is precedent for every major part of the architecture, but the combination is differentiated. BOINC has allowed individuals to contribute idle personal computers to distributed scientific workloads for decades, selecting work according to the machine's available RAM and capabilities. SaladCloud operates a distributed compute network whose providers contribute spare machines and GPUs, treating them as interruptible resources and matching workloads to compatible nodes. Akash allows operators to contribute their own Linux compute capacity to a marketplace. K3s explicitly targets edge devices, homelabs, IoT, ARM systems and single-board computers. citeturn587739search1turn587739search5turn587739search3turn587739search6

What appears less common is a **consumer application marketplace in which the same manifest can select between a user's old laptop, their existing VPS, a Kubernetes cluster, or purchased cloud compute while Wazi handles capacity matching, isolation, networking, persistence, permissions and lifecycle management**. That is the architectural opportunity.

I would call this subsystem **Wazi Nodes**, with **Wazi Fabric** representing the combined pool of local, edge and cloud resources.

A user could therefore see:

| Resource | Location | CPU | RAM | Storage | Accelerator | Reliability | Suitable workloads |
|---|---|---:|---:|---:|---|---|---|
| Old laptop | Home | 4 cores | 16 GB | 380 GB | Intel iGPU | Best effort | Automation, small web apps, databases |
| Gaming PC | Home | 12 cores | 64 GB | 1.6 TB | RTX-class GPU | Best effort | Local AI, image generation |
| VPS | Hetzner | 4 vCPU | 8 GB | 80 GB | None | Always-on | Public services |
| Kubernetes | Work | Variable | Variable | Variable | GPU nodes | Managed | Production applications |

The important product abstraction becomes **capacity rather than infrastructure provider**. Applications specify what they need; Wazi determines which authorised node can satisfy those requirements.

## How Wazi Nodes should work

The user experience should be closer to adding a device to an account than administering a server.

A user would install a small open-source **Wazi Node Agent** on the computer they want to contribute. The agent should generate a device key locally, authenticate through a short-lived pairing process, report hardware capabilities, run a qualification test and wait for an explicit approval from the account owner. Tailscale already demonstrates a similar operational pattern: devices can be enrolled through one-use authentication credentials, device approval can be required, and machine identities can be tagged separately from human identities. citeturn856543search0turn856543search1

The onboarding sequence should therefore look roughly like this:

**Install Wazi Node → Pair device → Inspect hardware → Run qualification checks → Define resource contribution → Select security profile → Approve node → Available to scheduler.**

The agent should report facts rather than user-entered guesses. A node inventory could contain:

```yaml
node:
  id: wazi-node-7f42
  owner: user_123
  architecture: amd64
  operating_system: linux
  kernel: "..."
  trusted_state: verified

compute:
  cpu:
    cores: 8
    allocatable: 6
    features:
      - avx2

  memory:
    physical_gb: 32
    allocatable_gb: 24

  gpu:
    vendor: nvidia
    model: "..."
    vram_gb: 12
    runtime: cuda

storage:
  total_gb: 1000
  allocatable_gb: 600
  type: nvme

network:
  connectivity: residential
  inbound_public_ip: false
  measured_upload_mbps: 85
  measured_latency_ms: 21

availability:
  mode: best_effort
  allowed_hours:
    - "00:00-07:00"

capabilities:
  containers: true
  kvm: true
  tpm: true
```

Kubernetes already has mechanisms for exposing hardware capabilities through node labels, Node Feature Discovery and device plugins; GPU-enabled nodes can be discovered and advertised to the scheduler rather than manually hard-coded. Nomad similarly fingerprints CPU, memory, disk and networking information when clients join. These provide useful design references for Wazi's hardware-discovery layer. citeturn311773search9turn311773search13turn311773search32turn856543search8

Wazi should distinguish **physical capacity** from **allocatable capacity**. A laptop with 16 GB of RAM should not advertise all 16 GB to applications. The owner might reserve 6 GB for the operating system and normal use, leaving only 10 GB for Wazi. CPU, storage and GPU resources should work the same way.

That also lets someone continue using the machine normally while Wazi runs low-priority services in the background.

## Resource orchestration and isolation

Wazi should not build its own container runtime initially. It should define a higher-level isolation contract and use proven mechanisms underneath it.

For a first production architecture, I would use **OCI containers + Linux cgroups + namespaces** on ordinary nodes. Docker can enforce explicit CPU and memory ceilings, and Linux cgroups expose CPU, memory and block-I/O accounting. Without those constraints a container can consume whatever resources the host scheduler permits, so Wazi should never deploy a workload without generated limits. citeturn311773search0turn311773search22

A Wazi application manifest could consequently express reservations and limits:

```yaml
resources:
  minimum:
    cpu: 1
    memory: 1Gi
    storage: 5Gi

  recommended:
    cpu: 2
    memory: 4Gi

  limits:
    cpu: 3
    memory: 6Gi

placement:
  availability:
    - best_effort
    - always_on

  requires:
    architecture:
      - amd64
      - arm64

  prefers:
    storage: ssd
```

Wazi's scheduler then solves a placement problem:

**application requirements + user policy + node capabilities + current usage + security requirements → eligible deployment targets.**

This makes the same `wazi.yaml` useful for local hardware and cloud infrastructure.

Kubernetes is appropriate once a user has several machines or needs higher-level orchestration, but it should not be the mandatory runtime for one old laptop. K3s is lighter than a traditional Kubernetes distribution and targets edge and homelab environments; its documented minimum for an agent is one CPU core and 512 MB RAM, while a K3s server requires at least two cores and 2 GB RAM. citeturn587739search6turn587739search7

I would therefore create three execution modes:

**Wazi Node Runtime** for one machine, using containerd/Docker-compatible OCI execution.

**Wazi Cluster Runtime** for several personal machines, potentially using K3s underneath.

**External Cluster Runtime** for existing Kubernetes infrastructure.

The user should never need to understand which implementation is being used unless they open the advanced view.

### Security profiles

Not every application deserves the same isolation level. Wazi should expose something like:

| Profile | Isolation | Intended use |
|---|---|---|
| Standard | Rootless/container isolation | Trusted open-source services |
| Restricted | Strong capabilities/seccomp/network restrictions | Unfamiliar applications |
| Hardened | MicroVM or VM boundary | High-risk or untrusted workloads |
| Dedicated | Whole machine/VM reserved | Sensitive data or intensive workloads |

Containers are useful resource and process boundaries, but they are not equivalent to a separate VM. For higher-risk packages, Wazi should eventually support microVMs or full virtual machines instead of claiming container isolation provides complete tenant separation.

This is also where an old Mac becomes interesting. Apple's Virtualization framework can create and manage Linux and macOS virtual machines on supported Macs, so a future macOS Wazi Node could host Linux workloads inside a VM rather than modifying the user's macOS environment directly. citeturn311773search1turn311773search14

For the initial product, however, **Linux should be the canonical Wazi Node host**. Supporting Windows and macOS as resource providers is possible later, but Linux gives Wazi far better control over containers, cgroups, networking, GPUs and storage.

## Networking, access and the home-network problem

This part is more important than raw compute.

Most unused laptops sit behind home routers, NAT and sometimes carrier-grade NAT. Wazi should therefore avoid requiring users to forward ports manually or obtain a static public IP.

The correct model is **outbound-initiated connectivity**.

The Wazi Node Agent establishes an authenticated encrypted connection to Wazi's control/network layer. Applications can then be reached through that connection even though the device has no publicly reachable address. Cloudflare Tunnel demonstrates this pattern: `cloudflared` creates outbound-only connections, allowing HTTP, SSH, remote desktop and other services to be reached without opening inbound firewall ports or requiring a public origin IP. citeturn149719search0turn149719search1turn149719search4

Tailscale demonstrates the complementary peer-to-peer model. Devices join a private overlay network and can expose selected services either privately through Serve or publicly through Funnel. Funnel itself routes public traffic through encrypted tunnels and relay infrastructure. citeturn856543search2turn856543search5

Wazi therefore needs two access modes.

**Private access** would be for dashboards, databases, development tools, AI systems and other services intended only for the owner or authorised collaborators.

```text
Laptop / phone
      │
      │ encrypted Wazi mesh
      ▼
Home Wazi Node
      │
      └── private application
```

**Published access** would be for an application that needs a public URL.

```text
Internet
   │
wazi.app gateway
   │
authenticated tunnel
   │
Home Wazi Node
   │
Application container
```

The user's home IP does not need to be publicly exposed in either model.

This also provides an answer to your earlier idea about applications feeling almost local. Wazi Connect could eventually establish direct peer-to-peer paths where possible and fall back to relays when NAT traversal fails. Latency will still depend on physical distance and the user's internet connection, so it cannot legitimately promise native speeds, but local-network clients could communicate directly with a nearby Wazi Node rather than round-trip through a distant cloud region.

## Reliability, persistence and scheduling

Personal hardware behaves differently from cloud servers. That needs to be embraced rather than hidden.

SaladCloud's own developer documentation describes distributed consumer hardware as interruptible: machines may disappear, network performance varies, cold starts can be longer and replicas should tolerate interruptions. BOINC's architecture likewise assumes heterogeneous devices and schedules work according to available resources. citeturn587739search5turn587739search8

Wazi should therefore assign each node an **availability class**.

**Always-on** — user commits the system to continuous operation.

**Scheduled** — machine is available during declared hours.

**Best effort** — Wazi can use it while online but must expect interruption.

**Interactive** — Wazi reduces or suspends workloads when somebody starts actively using the computer.

This becomes another field in the application's requirements. A photo-generation worker can tolerate a best-effort node. A password manager or business database probably should not.

The scheduler should measure actual availability over time rather than trusting a declaration:

```text
Wazi Node Reliability

Online last 30 days       96.7%
Unexpected disconnects    3
Median restart time       41 sec
Disk health               Good
Network stability         Good
Last verified backup      2 hours ago
```

This information should **not become a public reputation system for personal users**; it is operational metadata for their own placement decisions.

Persistence must also be separated from containers. Application data should live in defined volumes, with snapshot/backup policies that survive container replacement. A workload should be disposable; its state should not be.

For particularly important applications, Wazi could offer a hybrid arrangement:

```text
Primary workload
Home Node

        ├── encrypted backup → user's cloud storage
        │
        └── standby deployment definition → cloud VM
```

If the user's home machine becomes unavailable, Wazi could eventually offer migration or failover onto cloud infrastructure. That transforms cloud providers from the only computing option into a **reliability layer around user-owned hardware**.

That is a compelling product position.

## Device identity, attestation and trust

Physical hardware introduces a security problem that cloud VMs partially hide: Wazi needs to know whether a machine claiming to be a previously approved node is actually that machine and whether its execution environment has changed materially.

The first version only needs strong cryptographic device identity.

The Wazi agent generates a private key locally. Pairing associates the public key with the user's account. Subsequent control-plane messages are mutually authenticated and signed. Credentials should be rotatable and revocable.

A stronger future tier could use hardware-backed identity through TPM 2.0. Keylime, a CNCF-hosted project, provides remote boot attestation and runtime integrity measurements using TPM-based roots of trust. This illustrates that Wazi could later verify aspects of a node's measured boot/runtime state instead of merely trusting what its software agent reports. citeturn311773search27turn311773search38

This could create Wazi trust levels:

```text
Node verified
✓ Account ownership verified
✓ Device key verified
✓ Hardware inventory verified
✓ Secure boot detected
✓ TPM attestation valid
✓ Runtime integrity policy passed
```

Again, these checks have different meanings. A TPM attestation does not prove an application is harmless, and the UI must not imply otherwise.

The user should also be able to revoke Wazi completely. Removing a node should revoke its credentials, stop workloads according to policy, erase Wazi-managed secrets and optionally preserve or delete application data.

## How this extends the existing Wazi architecture

This feature should not be implemented as another separate product stack. It completes the infrastructure abstraction already emerging from Wazi.

The architecture becomes:

```text
                        WAZI CONTROL PLANE
 ┌────────────────────────────────────────────────────────┐
 │ Identity │ Store │ Policies │ Scheduler │ Security     │
 │ Updates  │ MCP   │ S-prints │ Billing   │ Telemetry    │
 └─────────────────────────┬──────────────────────────────┘
                           │
                       Wazi Fabric
                           │
       ┌───────────────────┼───────────────────┐
       │                   │                   │
   Wazi Nodes          BYO Cloud          Managed Cloud
       │                   │                   │
 Home laptop          AWS / Azure         Partner VM
 Desktop              GCP / VPS           GPU provider
 Mini PC              Kubernetes
 NAS / Server
```

Every compute resource produces the same conceptual capability descriptor. Every application produces the same requirement descriptor.

That means the marketplace can offer an unusually simple installation decision.

**Install Immich**

Available destinations:

- Home Server — Recommended · 24 GB RAM free · 620 GB storage
- Old Laptop — Compatible · 7 GB RAM free · insufficient recommended storage
- Hetzner VPS — Compatible · estimated external cost shown
- AWS Account — Compatible · requires provisioning

The user selects a destination; Wazi handles the underlying deployment adapter.

This architecture also fits the future S-print concept. An AI or world-model workload could request an NVIDIA GPU, 24 GB of VRAM, local camera access and 100 GB of model storage. Wazi's scheduler could identify which user-owned or cloud node can satisfy those capabilities.

MCP becomes particularly valuable here. An authorised AI agent could request something like:

```text
wazi.resources.find({
  gpu_memory_gb: 12,
  memory_gb: 16,
  persistence_gb: 80,
  availability: "best_effort"
})
```

Wazi returns eligible resources without giving the agent raw SSH credentials or unrestricted cloud access.

## What is already done elsewhere — and what Wazi could uniquely own

The idea is not unprecedented. The constituent technologies are proven.

BOINC demonstrates **consumer-device computing**. citeturn587739search0turn587739search2

SaladCloud demonstrates **commercial scheduling on heterogeneous contributed hardware**, including GPU qualification, compatibility matching and handling nodes that can disappear without warning. citeturn587739search5

Akash demonstrates **user-supplied infrastructure offered as deployable compute**, although its current provider guidance is oriented towards Linux/Kubernetes operators with substantially more infrastructure knowledge and hardware than the ordinary laptop owner. citeturn587739search3turn587739search4

K3s demonstrates that **production-compatible orchestration can be brought down to edge and homelab hardware**. citeturn587739search6

Cloudflare Tunnel and Tailscale demonstrate **secure access to machines behind residential networks without requiring normal inbound exposure**. citeturn149719search3turn856543search2

Docker, cgroups and Kubernetes demonstrate **capacity limits and hardware-aware workload scheduling**. citeturn311773search0turn311773search13

The area Wazi can own is the **unified application-level experience**:

> Install an application once and decide whether it should run on this laptop, that home server, my existing cloud account or rented infrastructure—without changing the application package.

That complements Wazi's marketplace instead of turning Wazi into another infrastructure marketplace.

### I would not initially allow strangers to rent users' machines

There are actually two possible businesses hidden inside this idea.

**Personal compute federation** means Alice uses Alice's laptop to run Alice's applications.

**Distributed compute marketplace** means Alice allows Bob's workloads to execute on Alice's laptop in exchange for money.

The second brings significantly greater security, abuse, tax, payment, bandwidth, content moderation, ISP-policy and isolation concerns. Akash and Salad demonstrate that such marketplaces can exist, but they require an entirely different trust and operational model. citeturn587739search3turn587739search5

Wazi should begin with the first model.

The user owns the hardware, owns the workload and chooses where it executes.

Once the security model is mature, Wazi could later experiment with trusted organisational pools such as:

```text
Company Wazi Fabric

Employee workstation
Office mini-server
GPU workstation
AWS account
Azure account
```

Only much later should third-party compute resale be considered.

## Recommended implementation roadmap

For the current hackathon, this feature is too important to ignore but too large to build comprehensively. A very small implementation could materially improve the submission.

**Hackathon prototype**

Build a Linux `wazi-node` agent that can be installed on an old laptop or VM. The agent pairs through a one-time code, reports CPU/RAM/storage/architecture, registers itself with the Wazi dashboard and runs one OCI application with enforced CPU and memory limits. Docker already provides the necessary controls for a prototype. citeturn311773search0

Then make the demonstration concrete:

```text
Wazi discovers machine:

ThinkPad
4 cores
15.5 GB RAM
412 GB free storage

Application requirement:

2 CPU
4 GB RAM
20 GB persistent storage

✓ Compatible

Deploy?
```

Run the application, demonstrate the resource limits, restart its container and show that data persists.

Then unplug the machine.

Wazi should report:

```text
Node unavailable
Last seen: 12 seconds ago
Application state: unreachable
Persistent data: retained on node
```

Plug it back in and have the agent reconnect automatically.

That five-minute sequence proves the central architecture better than adding another cloud-provider integration.

**Early alpha**

Add persistent volumes, encrypted Wazi secrets, private network access, device health, scheduling policies, application CPU/RAM/storage reservations and automatic updates.

**Beta**

Add Tailscale/WireGuard-style private networking or an equivalent Wazi networking layer, backup destinations, hardware accelerators, multi-node scheduling and K3s cluster support. Kubernetes' device-plugin model provides a mature pattern for advertising GPU and other specialised hardware. citeturn311773search32

**Production**

Add TPM-backed device attestation, hardened sandbox options, remote backup, policy-controlled failover to cloud resources, Windows/macOS provider support and organisational compute pools.

**Long-term**

Introduce workload migration, local/cloud optimisation, predictive scheduling based on electricity/network availability, S-print/world-model accelerator matching and eventually optional trusted third-party compute sharing.

The strategic effect is larger than simply supporting old laptops. It changes Wazi's core abstraction from **“an app store that can deploy software”** into **“a universal application and compute fabric.”**

A developer publishes one Wazi application. The user chooses where it lives. Wazi discovers the available hardware, verifies that the target can run it, isolates its resources, connects it securely, preserves its state and manages its lifecycle. Cloud hardware and a ten-year-old laptop become different implementations of the same deployment contract.