"""Validate a decrypted Uptime archive in an exclusive disposable directory.

This verifies SQLite integrity; it does not install an application or restore a
live volume. Authenticate and bind ciphertext with stage_offhost_recovery first.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import sqlite3
import tarfile


def verify(stage, output):
    stage=Path(stage);output=Path(output)
    record=json.loads((stage/'recovery-stage.json').read_text(encoding='utf-8'))
    if record.get('state')!='decrypted-staging-only' or record.get('project')!='wazi-uptime-kuma-01' or record.get('payloadFormat')!='volume-tar':
        raise ValueError('uptime-staging-binding-required')
    payload=stage/'payload'
    if payload.is_symlink() or not 0<payload.stat().st_size<=2048*1048576:
        raise ValueError('bounded-regular-staging-required')
    with payload.open('rb') as source:
        if 'sha256:'+hashlib.file_digest(source,'sha256').hexdigest()!=record['plaintextDigest']:
            raise ValueError('staging-payload-digest-mismatch')
    if os.name=='posix':os.umask(0o077)
    output.mkdir(mode=0o700,exist_ok=False)
    selected={};names=set();total=0
    # Inspect the complete archive before writing any database bytes. No
    # archive path is ever passed to an extraction API or joined to the output.
    with tarfile.open(payload,'r:') as archive:
        for index,item in enumerate(archive):
            path=PurePosixPath(item.name)
            name=str(path)
            if index>=10000 or len(item.name)>1024 or path.is_absolute() or '..' in path.parts or ':' in item.name or '\\' in item.name or name in names or not (item.isdir() or item.isfile()):
                raise ValueError('unsafe-or-duplicate-archive-entry')
            names.add(name);total+=item.size
            if item.size<0 or total>2048*1048576:raise ValueError('archive-content-bound')
            if name in ('kuma.db','kuma.db-wal','kuma.db-shm'):
                if not item.isfile():raise ValueError('database-must-be-regular')
                selected[name]=item
        if 'kuma.db' not in selected:raise ValueError('sqlite-database-missing')
        for name,item in selected.items():
            with archive.extractfile(item) as source,(output/name).open('xb') as target:
                os.chmod(output/name,0o600)
                while chunk:=source.read(65536):target.write(chunk)
    database=sqlite3.connect((output/'kuma.db').resolve().as_uri()+'?mode=ro',uri=True,timeout=5)
    try:
        database.execute('PRAGMA query_only=ON');database.execute('PRAGMA trusted_schema=OFF')
        ticks=0
        def progress():
            nonlocal ticks
            ticks+=1
            return ticks>100000
        database.set_progress_handler(progress,1000)
        if database.execute('PRAGMA integrity_check').fetchall()!=[('ok',)]:raise ValueError('sqlite-integrity-failed')
        tables={row[0] for row in database.execute("SELECT name FROM sqlite_master WHERE type='table'")}
        if not {'monitor','setting','user'}.issubset(tables):raise ValueError('uptime-schema-missing')
    finally:database.close()
    result={'state':'offhost-sqlite-integrity-verified','ciphertextDigest':record['ciphertextDigest'],'manifestDigest':record['manifestDigest'],'sqliteIntegrity':'ok','applicationRestorePerformed':False}
    with (output/'verification.json').open('x',encoding='utf-8') as target:json.dump(result,target,sort_keys=True,indent=2)
    return result


if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--stage',required=True);parser.add_argument('--output',required=True)
    args=parser.parse_args();print(json.dumps(verify(args.stage,args.output)))
