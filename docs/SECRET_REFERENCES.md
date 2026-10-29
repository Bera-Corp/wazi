# Secret References — Phase 0 Design

## Rule

Secret **values** never enter a manifest, context envelope, deployment
plan, deployment case, prompt, log, or telemetry record. Only references
do. This is enforced, not just documented:

- every schema that can carry a secret uses the `secretRef` pattern
  `^secret://[a-z0-9][a-z0-9/_-]*$`;
- `scripts/validate.py` scans examples AND fixtures and rejects
  secret-bearing keys (`password`, `token`, `api_key`, `secretValue`, …)
  and strings matching synthetic secret patterns (assignments, bearer
  tokens, cloud/GitHub key shapes, PEM headers). `secret://` references
  are explicitly not matched. This scan is a backstop only: it cannot
  prove an arbitrary string contains no secret. The primary control is
  structural — no contract field can carry a value, sensitive-named
  environment entries must use `secretRef` (enforced by the manifest
  schema), and plan parameters are closed per capability;
- host profiles, evidence records, and deployment cases must declare
  `"secretValuesPresent": false`.

## Reference scheme

```text
secret://deployment/<name>     generated or supplied for one deployment
secret://host/<name>           host-scoped credential (e.g. registry pull)
secret://tenant/<name>         tenant-scoped credential managed by the broker
```

A reference records only: the reference itself, the injection point
(e.g. `compose environment APP_SECRET`), the expected format, and
whether rotation is required. See the `secretReferences` array in
[the execution-context envelope example](../context/examples/execution-context-envelope.json).

## Broker boundary

1. The planner and context engine see references and formats only.
2. At execution time the host agent asks the secret broker for the value
   behind a reference, scoped to that deployment and host.
3. The value is injected directly into the target (Compose environment,
   config file with restricted permissions) by the `secret.inject`
   capability.
4. The value is never returned to the control plane, never written to
   evidence, and logs pass through redaction before storage.

## Rotation and revocation

- Rotation creates a new broker value behind the same reference; the
  manifest and envelope do not change.
- Revoking a reference blocks new `secret.inject` calls for it and is
  recorded as an evidence event.
- A deployment case records which references were used, never values.

## Open decisions (not Phase 0 blockers)

- Broker implementation (Vault, cloud KMS, or self-hosted minimal
  broker) is a Phase 1 decision.
- Per-tenant key isolation and audit format need an ADR before the
  control plane stores any tenant secret material.
