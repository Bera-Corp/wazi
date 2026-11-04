"""Create inert Uptime Kuma instance policy from a private existing runtime policy.

Does not sign, publish, provision identities, reserve capacity or install anything.
"""
import argparse
import copy
import json
import os
from pathlib import Path
import re

import validate


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('template', 'manifest', 'export', 'budget-file', 'budget-digest', 'qualification-file', 'identity-file', 'output'):
        parser.add_argument('--'+name, required=True)
    parser.add_argument('--recipient', action='append', required=True)
    parser.add_argument('--slot', choices=('01','02'), default='01')
    args = parser.parse_args()
    manifest = json.loads(Path(args.manifest).read_text(encoding='utf-8'))
    exported = json.loads(Path(args.export).read_text(encoding='utf-8'))
    if manifest.get('appId') != 'pet.kuma.uptime' or manifest.get('reviewState') != 'signed' or not manifest.get('signature'):
        raise ValueError('signed-uptime-manifest-required')
    image, = manifest['release']['images']
    if image['image'] != 'ghcr.io/bera-corp/wazi-phase2-uptime-kuma' or exported['image']['name'] != image['image'] or exported['image']['digest'] != image['digest']:
        raise ValueError('uptime-image-binding-mismatch')
    if not re.fullmatch(r'sha256:[0-9a-f]{64}', exported['image']['configDigest']) or not re.fullmatch(r'sha256:[0-9a-f]{64}', args.budget_digest):
        raise ValueError('immutable-image-and-budget-required')
    project = 'wazi-uptime-kuma-'+args.slot
    if not args.identity_file.startswith('/srv/wazi-runtime/secrets/'+project+'/'):
        raise ValueError('instance-specific-protected-identity-required')
    config = copy.deepcopy(json.loads(Path(args.template).read_text(encoding='utf-8')))
    config.update(Profile='uptime-kuma', ReleaseVersion=manifest['version'], ProcedureVersion='1.2.0', BudgetDigest=args.budget_digest,
                  BudgetFile=args.budget_file, QualificationFile=args.qualification_file, SecretFiles={})
    config['BackupEncryption'] = {'Recipients':args.recipient,'IdentityFile':args.identity_file}
    runtime = config['Runtime']
    runtime.update(Project=project, StateDirectory='/srv/wazi-runtime/phase2/'+project,
                   AllowedManifests={validate.canonical_digest(manifest):True},
                   ImageConfigDigests={image['digest']:exported['image']['configDigest']})
    # Credentials and capacity stay in the operator's existing template. This
    # generated record must pass the Go constructor and signed preflight later.
    if os.name == 'posix':
        os.umask(0o077)
    with Path(args.output).open('x',encoding='utf-8') as output:
        json.dump(config,output,sort_keys=True,indent=2)
    print('Inert instance policy written; signature trust, capacity and live enablement remain separate gates')


if __name__ == '__main__':
    main()
