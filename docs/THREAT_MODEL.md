# Threat Model

## Protected assets

- host control capabilities;
- user and tenant data;
- credentials and generated secrets;
- signed manifests and catalogue trust;
- deployment, backup, restore, and rollback evidence;
- application data and infrastructure metadata.

## Main adversaries

- malicious upstream repository or compromised maintainer;
- malicious catalogue contributor;
- attacker controlling a container image or dependency;
- tenant attempting cross-tenant context access;
- prompt injection embedded in documentation or logs;
- compromised host or control-plane account;
- insider abusing deployment privileges.

## Critical controls

- treat upstream text as untrusted data;
- pin image digests and capture SBOMs;
- scan candidates in isolated environments;
- sign and revoke manifests;
- enforce tenant-scoped context retrieval;
- keep secret values outside prompts and telemetry;
- expose typed capabilities instead of a generic shell;
- require approvals for public exposure and destructive actions;
- create backup checkpoints before migrations;
- verify restore and rollback procedures;
- preserve tamper-evident audit evidence;
- expire host observations and version-sensitive context.

## Failure posture

When context is missing, contradictory, stale, unsigned, or incompatible, Wazi must stop rather than guess. AI confidence never overrides policy or compatibility failures.
