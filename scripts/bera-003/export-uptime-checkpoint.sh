#!/usr/bin/env bash
# Export ciphertext and its separately captured recovery binding, never a key.
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
python3 - "$1" <<'PY'
import hashlib,json,os,pathlib,re,shutil,sys
source=pathlib.Path(sys.argv[1]);assert re.fullmatch(r'/home/wazi-dev/wazi-phase2/\d{8}T\d{6}Z',str(source))
receipt=json.loads((source/'uptime-sequential-test.json').read_bytes());assert receipt['state']=='passed' and receipt['postgresqlRestored']
payload=pathlib.Path(receipt['checkpointFile']);sidecar=pathlib.Path(str(payload)+'.json')
assert payload.parent==pathlib.Path('/srv/wazi-runtime/phase2/wazi-uptime-kuma-01/backups') and payload.name.startswith('bak_uptime_') and payload.suffix=='.age'
assert payload.resolve()==payload and sidecar.resolve()==sidecar and payload.is_file() and 0<payload.stat().st_size<3*1024**3
with payload.open('rb') as handle:assert 'sha256:'+hashlib.file_digest(handle,'sha256').hexdigest()==receipt['ciphertextDigest']
metadata=json.loads(sidecar.read_bytes());assert metadata['ManifestDigest']==receipt['manifestDigest'] and metadata['PayloadDigest']==receipt['ciphertextDigest'] and metadata['Format']=='age-v1'
directory=source/'offhost-transport';directory.mkdir(mode=0o700,exist_ok=False);owner=source.stat()
os.chown(directory,owner.st_uid,owner.st_gid)
for path in (payload,sidecar):
    target=directory/path.name;shutil.copyfile(path,target);os.chmod(target,0o600);os.chown(target,owner.st_uid,owner.st_gid)
card=directory/'recovery-card.json'
card.write_text(json.dumps({'project':'wazi-uptime-kuma-01','manifestDigest':receipt['manifestDigest'],'ciphertextDigest':receipt['ciphertextDigest'],'checkpointName':payload.name,'payloadFormat':metadata['PayloadFormat'],'maxPlaintextMb':2048,'applicationRestorePerformed':False},sort_keys=True,indent=2))
os.chmod(card,0o600);os.chown(card,owner.st_uid,owner.st_gid)
print('Encrypted Uptime checkpoint and recovery card staged; identity material remains on its original host')
PY
