# Wazi pilot release: Gitea 28.2.0

`manifest.json` is a canonical Wazi app manifest signed with the local Phase 1 pilot signer. The signature was checked by both Cosign v3.1.4 and Wazi's Go verifier; the manifest also passes the embedded app-manifest schema.

The exact image digest, upstream tag object and peeled commit, verified source-binding workflow run, GitHub release-attestation result, and remote services CI run are recorded in `build-evidence.json`.

This is a single-approver demo-pilot signing key. Its encrypted private key remains in the local operator profile and is not in the repository or on bera-003. `signing-policy.json` is the public key pin. The host control plane and agent are not yet configured to trust this signer, and no Gitea workload has been created. The record does not represent a live deployment, lifecycle, owner pairing sign-off, or production trust root.