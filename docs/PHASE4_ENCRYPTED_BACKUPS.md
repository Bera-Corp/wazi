# Phase 4: encrypted database checkpoint implementation

## Scope and status

This change adds an offline encrypted checkpoint path for the reviewed
PostgreSQL and MySQL package profiles. It does not configure keys on Bera-003,
encrypt existing checkpoints, prove recovery from an off-host copy, or close
Phase 4 live acceptance.

New database backups stream their database output through age encryption into
a private checkpoint file with an .age suffix. The sidecar identifies the
format as age-v1, records the database payload format, and binds the SHA-256
digest to the ciphertext. PostgreSQL uses its custom dump format; MySQL uses a
logical SQL dump. Nginx configuration checkpoints retain their existing JSON
format.

Restore verifies the checkpoint bindings and ciphertext digest, then decrypts
and authenticates the entire file into a private temporary file before it
mutates database state. Plaintext is bounded by the reviewed storage limit and
the temporary file is removed after use. PostgreSQL additionally runs
pg_restore --list against the staged plaintext before dropping or recreating
the target database. The MySQL path does not yet import into a disposable
database as a semantic preflight; a failed import can leave a partial target
restore. Its pre-restore checkpoint remains available for an explicitly
approved recovery, but automatic rollback is not claimed.

Older PostgreSQL and MySQL .dump checkpoints remain readable for
compatibility. New database backup operations fail closed unless the instance
has a matching encryption recipient and protected identity configured.

## Operator configuration

The reviewed instance configuration accepts public age recipients and a
protected file reference to the private restore identity. For example:

~~~json
{
  "BackupEncryption": {
    "Recipients": ["age1...public-recipient..."],
    "IdentityFile": "/srv/wazi-runtime/secrets/wazi-postgresql-01/backup-identity.txt"
  }
}
~~~

This is an illustrative shape; the recipient placeholder is not a usable key.
The identity file contains one or more age X25519 private identities. Keep
private identity bytes out of contracts, manifests, plans, receipts, source
control and chat. Provision the identity through the host's protected operator
secret process. The configured path must be inside that package instance's
protected secret directory, be a regular file and pass the platform-specific
permission checks.

Include a separately protected off-host recovery recipient before treating
these checkpoints as durable backups. The runtime identity stored beside the
application is not sufficient protection against loss of the host. Record and
test the recovery procedure from an isolated system before relying on it.

The implementation supports native age X25519 recipients only. It does not
enable hybrid post-quantum recipients or claim sender authentication: anyone
with a public recipient can create ciphertext that the identity can decrypt.
The authenticated age stream protects ciphertext integrity, while the local
checkpoint sidecar digest detects accidental changes to the stored bytes.

## Qualification still required

- Configure and protect an instance-specific identity and recipient set using
  the operator secret process; do not put the private key in this repository.
- Build and run the current source on the pinned Linux Go runtime.
- Create a new PostgreSQL and MySQL checkpoint, confirm the stored files are
  ciphertext, and restore each from the configured identity.
- Repeat restore with a wrong identity, modified ciphertext, truncated
  ciphertext and a checkpoint exceeding the configured bound; each must be
  rejected before target mutation.
- Test recovery using the off-host recipient and document measured restore
  time and any manual steps.
- Add a disposable-database MySQL import preflight before describing malformed
  dump handling as non-destructive.

The code currently preserves legacy checkpoint compatibility and adds local
regressions for age round trips and pre-mutation rejection. No live encrypted
database backup or restore has been performed.

The 3 Nov adversarial pass corrected the encrypted checkpoint size bound to
include age authentication tags for every 64 KiB plaintext chunk. Boundary
regressions verify real encrypted payloads; this remains local test evidence,
not live encrypted recovery acceptance.
