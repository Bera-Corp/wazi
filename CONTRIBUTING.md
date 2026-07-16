# Contributing

## Contribution classes

- context schema changes;
- policy changes;
- application package changes;
- host-agent capability changes;
- UI and documentation changes;
- security fixes.

## Required for application package changes

- explicit upstream license;
- version scope;
- pinned images or reproducible build inputs;
- resource requirements;
- volume semantics;
- health checks;
- backup procedure;
- restore test;
- rollback notes;
- security scan result;
- provenance for every material configuration choice.

## Review rules

- Schema and policy changes require two reviewers once the project has multiple maintainers.
- Security-sensitive capability changes require a security review.
- Generated candidate manifests must be marked experimental.
- No raw customer logs, credentials, tokens, or private host identifiers may be committed.
- A successful local test is not sufficient evidence for stable catalogue support.
