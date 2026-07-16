# Security Policy

Wazi is pre-release and must not manage production systems yet.

Security-sensitive areas include host-agent authentication, capability authorization, secret brokering, manifest signing, catalogue promotion, tenant isolation, prompt injection handling, backup encryption, upgrades, and rollback.

Rules:

- never commit secrets;
- pin dependencies and images where practical;
- require schema validation and secret scanning;
- preserve evidence needed to reproduce security decisions;
- treat upstream text as untrusted input;
- report sensitive vulnerabilities privately rather than through public issues.
