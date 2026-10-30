#!/usr/bin/env bash
# Rotate the pilot node identity while idle. Never copies private keys off-host.
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
python3 - <<'PY'
import hashlib,json,os,pathlib,ssl,subprocess,datetime,urllib.request

def run(args):
    return subprocess.run(args,check=True,capture_output=True,text=True,timeout=60).stdout

config=json.loads(pathlib.Path('/etc/wazi-phase1/agent.json').read_text())
identity=config['Identity']
state=pathlib.Path(config['Runtime']['StateDirectory'])
requests={p.name for p in (state/'queue'/'requests').glob('*.json')}
results={p.name for p in (state/'queue'/'results').glob('*.json')}
if requests != results:
    raise SystemExit('refusing renewal while queue work is pending')
journal={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in (state/'journal').glob('*.json')}
for name in journal:
    entry=json.loads((state/'journal'/name).read_text())
    if entry.get('state') not in ('completed','failed','blocked','cancelled'):
        raise SystemExit('refusing renewal with nonterminal journal work')
pid=run(['systemctl','show','wazi-pilot-agent.service','-p','MainPID','--value']).strip()
worker=pathlib.Path(os.readlink('/proc/'+pid+'/exe'))
binary=worker.with_name('wazi-agent')
if not str(binary).startswith('/opt/wazi-dev/') or not binary.is_file():
    raise SystemExit('verified installed inventory agent unavailable')
cert=pathlib.Path(identity['CertFile'])
anchor=pathlib.Path(identity['CAFile'])
before=cert.read_bytes()
anchor_digest=hashlib.sha256(anchor.read_bytes()).hexdigest()
old_fingerprint=hashlib.sha256(ssl.PEM_cert_to_DER_cert(before.decode())).hexdigest()
run(['systemctl','stop','wazi-pilot-agent.service'])
try:
    run(['sudo','-n','-u','wazi-runtime','env','HOME=/srv/wazi-runtime/home',
         'XDG_RUNTIME_DIR=/run/user/1002','DOCKER_CONTEXT=rootless','PATH=/usr/bin:/bin',str(binary),
         '-tenant',config['Runtime']['TenantScope'],'-host-id',config['Runtime']['HostID'],
         '-state-dir',str(cert.parent),'-cert',str(cert),'-key',identity['KeyFile'],
         '-ca',str(anchor),'-issuer-ca',identity['IssuerCAFile'],
         '-controlplane',config['ControlPlaneURL'],'-server-name',config['ServerName'],'-renew'])
finally:
    run(['systemctl','start','wazi-pilot-agent.service'])
run(['systemctl','is-active','--quiet','wazi-pilot-agent.service'])
after=cert.read_bytes()
if before==after or hashlib.sha256(anchor.read_bytes()).hexdigest()!=anchor_digest:
    raise SystemExit('renewal did not rotate the certificate or altered the server anchor')
after_journal={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in (state/'journal').glob('*.json')}
if journal!=after_journal:
    raise SystemExit('journal changed during identity-only maintenance')
with urllib.request.urlopen('http://127.0.0.1:13000/api/healthz',timeout=5) as response:
    health=json.load(response)
if health.get('status')!='pass':
    raise SystemExit('application health did not remain passing')
record={
    'operation':'paired-node-renewal','tenantScope':config['Runtime']['TenantScope'],
    'hostId':config['Runtime']['HostID'],'previousCertificateFingerprint':'sha256:'+old_fingerprint,
    'newCertificateFingerprint':'sha256:'+hashlib.sha256(ssl.PEM_cert_to_DER_cert(after.decode())).hexdigest(),
    'newCertificatePemSha256':'sha256:'+hashlib.sha256(after).hexdigest(),
    'certificateDates':run(['openssl','x509','-in',str(cert),'-noout','-dates']).splitlines(),
    'serverAnchorUnchanged':True,'journalFilesUnchanged':len(journal),'applicationHealth':'pass',
    'workerActive':True,'verifiedAt':datetime.datetime.now(datetime.timezone.utc).isoformat(timespec='seconds')
}
print(json.dumps(record,sort_keys=True,indent=2))
PY
