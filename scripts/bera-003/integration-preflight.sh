#!/usr/bin/env bash
set -euo pipefail
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
python3 - <<'PY'
import datetime,json,os,pathlib,ssl,subprocess,urllib.request
def run(args):
    return subprocess.check_output(args,text=True,timeout=20).strip()
report={'services':{}}
for name in ('wazi-controlplane-dev.service','wazi-pilot-agent.service'):
    report['services'][name]=run(['systemctl','show',name,'--property=ActiveState,SubState,MainPID,FragmentPath,DropInPaths'])
report['serverCertificate']=run(['openssl','x509','-in','/etc/wazi-controlplane-dev/server.crt','-noout','-dates'])
identity=json.loads(pathlib.Path('/etc/wazi-phase1/agent.json').read_text())['Identity']
report['nodeCertificate']=run(['openssl','x509','-in',identity['CertFile'],'-noout','-dates'])
context=ssl.create_default_context(cafile='/etc/wazi-controlplane-dev/server-ca.crt')
try:
    with urllib.request.urlopen('https://127.0.0.1:9443/healthz',context=context,timeout=5) as response:
        report['privateHealthStatus']=response.status
except Exception as error:
    report['privateHealthError']=type(error).__name__
prefix=['sudo','-u','wazi-runtime','env','XDG_RUNTIME_DIR=/run/user/1002','DOCKER_HOST=unix:///run/user/1002/docker.sock','docker']
report['containers']=[{'project':item['Config']['Labels'].get('com.docker.compose.project'),'running':item['State']['Running'],'imageConfig':item['Image']} for item in (json.loads(run(prefix+['inspect',identifier]))[0] for identifier in run(prefix+['ps','--format','{{.ID}}']).splitlines())]
fs=os.statvfs('/srv/wazi-runtime')
report['runtimeStorageAvailableBytes']=fs.f_bavail*fs.f_frsize
report['runtimeStorageTotalBytes']=fs.f_blocks*fs.f_frsize
report['runtimeMemoryMax']=run(['systemctl','show','user-1002.slice','--property=MemoryMax,CPUQuotaPerSecUSec'])
report['verifiedAt']=datetime.datetime.now(datetime.timezone.utc).isoformat()
print(json.dumps(report,indent=2))
PY
