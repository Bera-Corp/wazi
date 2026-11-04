"""Stage an encrypted checkpoint on a recovery host without touching an app.

Expected ciphertext identity comes from the separately retained recovery card,
not a digest read from the same potentially damaged backup directory.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('checkpoint','identity-file','expected-digest','manifest-digest','project','output'):
        parser.add_argument('--'+name,required=True)
    parser.add_argument('--age',default='age')
    parser.add_argument('--max-mb',type=int,required=True)
    parser.add_argument('--timeout-seconds',type=int,default=300)
    args = parser.parse_args()
    if not 1 <= args.max_mb <= 10240 or not 1 <= args.timeout_seconds <= 1800:
        raise ValueError('recovery-bound-invalid')
    for value in (args.expected_digest,args.manifest_digest):
        if not re.fullmatch(r'sha256:[0-9a-f]{64}',value):
            raise ValueError('recovery-digest-required')
    source=Path(args.checkpoint)
    metadata_path=Path(str(source)+'.json')
    if source.is_symlink() or metadata_path.is_symlink() or metadata_path.stat().st_size > 8192:
        raise ValueError('recovery-input-invalid')
    metadata=json.loads(metadata_path.read_text(encoding='utf-8'))
    if metadata.get('Format') != 'age-v1' or metadata.get('ManifestDigest') != args.manifest_digest or metadata.get('Project') != args.project or metadata.get('PayloadDigest') != args.expected_digest:
        raise ValueError('recovery-binding-mismatch')
    limit=args.max_mb*1024*1024
    if source.stat().st_size > limit + ((limit+65535)//65536)*16 + 65536:
        raise ValueError('ciphertext-bound-exceeded')
    if os.name == 'posix':
        os.umask(0o077)
    directory=Path(args.output)
    directory.mkdir(mode=0o700,exist_ok=False)
    with source.open('rb') as encrypted:
        if 'sha256:'+hashlib.file_digest(encrypted,'sha256').hexdigest() != args.expected_digest:
            raise ValueError('ciphertext-digest-mismatch')
        encrypted.seek(0)
        with tempfile.TemporaryFile(dir=directory) as staged:
            process=subprocess.Popen([args.age,'--decrypt','--identity',args.identity_file],stdin=encrypted,stdout=staged,stderr=subprocess.DEVNULL)
            deadline=time.monotonic()+args.timeout_seconds
            try:
                while process.poll() is None:
                    if time.monotonic() > deadline or os.fstat(staged.fileno()).st_size > limit:
                        raise ValueError('recovery-time-or-plaintext-bound')
                    time.sleep(0.1)
                size=os.fstat(staged.fileno()).st_size
                if process.returncode != 0 or not 0 < size <= limit:
                    raise ValueError('decryption-or-authentication-failed')
                staged.seek(0)
                plaintext_digest='sha256:'+hashlib.file_digest(staged,'sha256').hexdigest()
                staged.seek(0)
                with (directory/'payload').open('xb') as output:
                    while chunk:=staged.read(65536):
                        output.write(chunk)
                    output.flush()
                    os.fsync(output.fileno())
            finally:
                if process.poll() is None:
                    process.kill()
                    process.wait()
    with (directory/'recovery-stage.json').open('x',encoding='utf-8') as output:
        json.dump({'state':'decrypted-staging-only','project':args.project,'manifestDigest':args.manifest_digest,'ciphertextDigest':args.expected_digest,'plaintextDigest':plaintext_digest,'bytes':size,'payloadFormat':metadata.get('PayloadFormat'),'applicationRestorePerformed':False},output,sort_keys=True,indent=2)


if __name__=='__main__':
    main()
