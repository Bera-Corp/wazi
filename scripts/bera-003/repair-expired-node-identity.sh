#!/usr/bin/env bash
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
export WAZI_IDENTITY_RELEASE="$1"
python3 - <<'PY'
import datetime,json,os,pathlib,shutil,socket,ssl,subprocess,time,urllib.request
release=pathlib.Path(os.environ['WAZI_IDENTITY_RELEASE']);assert release.parent==pathlib.Path('/opt/wazi-dev/releases')
config=json.loads(pathlib.Path('/etc/wazi-phase1/agent.json').read_bytes());runtime=config['Runtime'];identity=config['Identity']
expiry=subprocess.run(['openssl','x509','-in',identity['CertFile'],'-checkend','0','-noout'],capture_output=True)
assert expiry.returncode==1,'only expired identity repair is allowed by this procedure'
state=pathlib.Path(runtime['StateDirectory'])
assert {p.name for p in (state/'queue/requests').glob('*.json')}=={p.name for p in (state/'queue/results').glob('*.json')}
directory=pathlib.Path(identity['CertFile']).parent
subprocess.run(['sudo','-u','wazi-runtime','test','-w',str(directory)],check=True)
backup=pathlib.Path('/etc/wazi-phase2')/('identity-repair-'+datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ'));backup.mkdir(mode=0o700)
for key in ('CertFile','KeyFile','IssuerCAFile'):
    if identity.get(key) and pathlib.Path(identity[key]).is_file():shutil.copyfile(identity[key],backup/key);os.chmod(backup/key,0o600)
operator=None
for line in pathlib.Path('/etc/wazi-controlplane-dev/operator.env').read_text().splitlines():
    if line.startswith('WAZI_OPERATOR_TOKEN='):
        operator=line.split('=',1)[1].strip().strip('"').strip("'")
assert operator
context=ssl.create_default_context(cafile=identity['CAFile'])
request=urllib.request.Request('https://localhost:9443/v1/pairing/codes',data=json.dumps({'tenantScope':runtime['TenantScope'],'hostId':runtime['HostID']}).encode(),method='POST')
request.add_header('Content-Type','application/json');request.add_header('X-Wazi-Operator-Token',operator)
with urllib.request.urlopen(request,context=context,timeout=20) as response:code=json.load(response)['pairingCode']
subprocess.run(['systemctl','stop','wazi-pilot-agent.service'],check=True)
try:
    args=['sudo','-u','wazi-runtime','env','HOME='+runtime['Home'],'XDG_RUNTIME_DIR=/run/user/'+str(runtime['RuntimeUID']),'DOCKER_HOST='+runtime['Endpoint'],str(release/'wazi-agent'),'-controlplane','https://localhost:9443','-tenant',runtime['TenantScope'],'-host-id',runtime['HostID'],'-state-dir',str(directory),'-cert',identity['CertFile'],'-key',identity['KeyFile'],'-ca',identity['CAFile'],'-issuer-ca',identity.get('IssuerCAFile') or str(directory/'issuer.crt'),'-pairing-code',code]
    result=subprocess.run(args,capture_output=True,timeout=45)
    (backup/'agent.stdout').write_bytes(result.stdout);(backup/'agent.stderr').write_bytes(result.stderr)
    if result.returncode:raise SystemExit('node enrollment failed; protected operator logs retained')
finally:subprocess.run(['systemctl','start','wazi-pilot-agent.service'],check=True)
for attempt in range(30):
    try:
        with socket.socket(socket.AF_UNIX) as client:client.settimeout(2);client.connect('/run/wazi-phase1/agent.sock')
        break
    except OSError:
        if attempt==29:raise
        time.sleep(1)
expires=subprocess.check_output(['openssl','x509','-in',identity['CertFile'],'-noout','-enddate'],text=True).strip()
print(json.dumps({'state':'same-host-identity-repaired','certificateExpiry':expires,'serverTrustAnchorChanged':False,'protectedBackupRetained':True,'completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()},indent=2))
PY
