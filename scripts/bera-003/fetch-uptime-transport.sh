#!/usr/bin/env bash
set -euo pipefail
test "$(hostname)" = bera-003
umask 077
python3 - "$1" <<'PY'
import hashlib,json,pathlib,sys,urllib.request,zipfile
root=pathlib.Path(sys.argv[1]);assert root.parent.parent==pathlib.Path('/home/wazi-dev/wazi-phase2') and root.name=='uptime'
url=json.loads((root/'transport.private.json').read_text())['uptime-kuma']
assert url.startswith('https://')
archive=root/'package-transport.zip'
with urllib.request.urlopen(url,timeout=120) as response, archive.open('xb') as output:
    total=0
    while chunk:=response.read(1<<20):
        total+=len(chunk)
        if total>512<<20:raise ValueError('transport-size-bound')
        output.write(chunk)
allowed={'package-image.tar','package-export.json','verified-archive-attestation.json','qualification-image.json','package-source-provenance.json'}
with zipfile.ZipFile(archive) as package:
    assert set(package.namelist())==allowed
    assert sum(item.file_size for item in package.infolist())<=2<<30
    for name in allowed:
        with package.open(name) as source,(root/name).open('xb') as output:
            while chunk:=source.read(1<<20):output.write(chunk)
meta=json.loads((root/'package-export.json').read_text())
with (root/'package-image.tar').open('rb') as image:
    assert 'sha256:'+hashlib.file_digest(image,'sha256').hexdigest()==meta['archive']['digest']
print(json.dumps({'state':'transport-downloaded-digest-matched','image':meta['image'],'archiveDigest':meta['archive']['digest'],'archiveBytes':(root/'package-image.tar').stat().st_size},indent=2))
PY
