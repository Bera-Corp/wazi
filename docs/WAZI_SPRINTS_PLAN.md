# Wazi Worlds and S-Prints

Status: experimental extension, version 0.1.0, integrated on 29 October
2026. Contracts and an offline resolver exist. Model execution, installation,
marketplace UI and measured model improvements remain buildout work.

## Product definition

An S-Print is a versioned capability package for a compatible world-model
or simulation runtime. It can describe perception, action adapters, skills,
environments, planners, evaluators, weights or training recipes. Instructions
alone do not imply a change to learned dynamics.

Wazi's product areas become Apps, Compute, Agents and Worlds. Worlds reuses
registry, identity, artifact storage, provenance, approval and BYOC concepts.
World interfaces describe data flow; they never grant host-agent privileges.

| Term | Meaning |
|---|---|
| World asset | Geometry, scene content and appearance |
| Simulator | Executable transition rules |
| World model | Learned prediction of state transitions |
| Policy | Action selection |
| Skill | Action with initiation, termination and expected effects |
| S-Print | Package connecting one or more of these components |

## Feature catalogue

| Feature | Implemented locally | Buildout acceptance gate |
|---|---|---|
| Package format | Closed experimental JSON Schema, asset digests and provenance | Signed inventory and verified artifact bytes |
| Capability negotiation | Exact interface ID, version, contract digest and base-model hash | Probe tensor shapes, units, coordinate frames, timing and action limits |
| Composition | Exact dependency pins, provider ordering, cycle/ambiguity rejection | Immutable graph lock, atomic activation, reversible deactivation |
| BYOC resources | Conservative sum of base and package RAM/VRAM reservations | Quotas, GPU/driver/runtime checks and exhaustion handling |
| Permissions | Simulation allowlist; requests must fit runtime grants | Brokered observation, filesystem, network, weights, compute, training and device access |
| World Capability Card | Requirements, provenance, evaluation thresholds and limitations in metadata | UI shows verified test evidence, supported scope and publisher identity |
| Evaluation | Suite digest and numeric thresholds required | Before/after runs, held-out tasks, uncertainty and unrelated-behaviour regressions |
| Model specialisation | Exact immutable base-model binding | Compatible adapter loader, original checkpoint unchanged, verified rollback |
| Distribution | Local JSON only | Registry signing/revocation, licence review, publisher workflow |
| Agent access | Read-only inspect/resolve planned | MCP inspection first; separately authorised action interfaces |

Classes: perception, world-conditioning, action-adapter, skill, planner,
policy-adapter, world-adapter, environment, reward, evaluator, dataset,
guardrail and renderer. `training` is reserved. Depth is independently
instruction, adapter, skill, environment or training. Multiple classes may
coexist. Training packages are metadata-only and rejected by this resolver.

## Run the implemented prototype

```powershell
.\scripts\validate.ps1
.\.venv\Scripts\python.exe scripts/sprints.py context/worlds/simulation-composition.json
```

The synthetic example selects a warehouse environment/action adapter,
navigation skill and planner. Input order is arbitrary; the resolver derives
warehouse -> navigation -> planner and returns package pins, a composition
digest and resource reservations. Rejections include explainable reasons.

`compatible-offline` means metadata agrees. Every result has
`executionAuthorized: false` and `evaluationStatus: not-run`. Asset paths,
hashes and thresholds are synthetic; no package assets, trained weights,
measured scores, verified publishers or physical certification are supplied.
This demonstrates composition, not acquired model ability.

## Runtime architecture

```text
Store / CLI / read-only MCP
          |
Registry + signature/revocation verification
          |
Runtime profile + resolver -> immutable graph lock
          |
Policy and approval -> isolated component loader
          |
Observation -> state -> dynamics -> planner -> bounded actions
          |                                  |
          +------------ simulator <----------+
                          |
                   evaluation evidence
```

Only offline resolution is implemented. Future loaders must verify asset
bytes, signatures, licences, isolation and evidence independently of
compatibility. Package README, AGENTS.md, SKILL.md and scripts are untrusted
content; none can override policy. Base weights remain immutable.

Local hashes use Python sorted compact UTF-8 JSON, matching the Phase 0
validator. This convention needs an ADR and cross-language vectors before
being used as a public signing or registry standard.

## Delivery sequence

These are dependency gates, not extra promises within the twelve-month
estimate. The deterministic application slice remains the core delivery path.

| Epic | Status | Prerequisites and exit evidence |
|---|---|---|
| WS-000 Contracts and offline resolution | Implemented locally | Schemas, synthetic three-package graph and negative tests in normal validation |
| WS-001 Executable simulation proof | Next Worlds experiment after Phase 1 gate | One small deterministic simulator; separately load/unload three packages; pinned runtime; reproducible before/after task evidence. Simulator success does not prove learned-model gain |
| WS-002 Model adapter integration | Later, after WS-001 and Phase 2 | One licensed model/release; pinned base weights and interfaces; GPU probe; measured gain with uncertainty and no unrelated regression |
| WS-003 Isolated activation lifecycle | Later, after WS-002 and Phase 4 | Quotas, sandbox tests, package signing/revocation, atomic graph swap, secret brokering and rollback evidence |
| WS-004 Worlds preview and cards | Later, after WS-003 and core private alpha | Opt-in preview, independent test status, simulation scope, read-only MCP, verified publisher and model/data/adapter licences |
| WS-005 Training and broader bridges | Research only | Reproducible data/recipe provenance, cost limits, checkpoints and training consent; physical control requires separate design and authorisation |

Physical actuation and safety-critical deployment are outside the prototype.
No current model family is declared supported merely because it appears in
the research. Network and physical-device permissions cannot be requested
by the current experimental contracts.

## Sources and unresolved research

The [supplied report](research/WAZI_SPRINTS_RESEARCH_2026-10-29.md) is archived
with its source hash. Its viewer citation identifiers do not resolve here.
Market uniqueness, universal portability, sample scores, model sizes and
naming clearance remain research assumptions.

Primary references checked on 29 October 2026:

- [NVIDIA Cosmos](https://github.com/NVIDIA/cosmos): world-model tooling and
  adaptation/evaluation infrastructure motivate packaged components.
- [Meta V-JEPA 2](https://github.com/facebookresearch/vjepa2): separate video
  representation and action-conditioned post-training motivate explicit
  capability interfaces. Review each component's licence.

These sources motivate the direction; they do not establish Wazi support.
Model selection must recheck exact code, weights, licences and hardware.
S-Print naming/trademark clearance remains open before public launch.
