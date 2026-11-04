#!/usr/bin/env bash
# Owner-approved sequential live test. Preserve both applications' data.
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
python3 - "$1" <<'PY'
import datetime,hashlib,json,pathlib,re,subprocess,sys,time,urllib.request
source=pathlib.Path(sys.argv[1]);assert re.fullmatch(r'/home/wazi-dev/wazi-phase2/\d{8}T\d{6}Z',str(source))
config=json.loads(pathlib.Path('/etc/wazi-phase1/agent.json').read_bytes());runtime=config['Runtime']
prefix=['sudo','-u','wazi-runtime','env','HOME='+runtime['Home'],'XDG_RUNTIME_DIR=/run/user/'+str(runtime['RuntimeUID']),'DOCKER_HOST='+runtime['Endpoint'],'docker']
def docker(*args):return subprocess.check_output(prefix+list(args),timeout=60)
def instances(project):return docker('ps','--filter','label=com.docker.compose.project='+project,'--format','{{.ID}}').decode().split()
pg,=instances('wazi-postgresql-02');assert not instances('wazi-uptime-kuma-01')
test=source/'scripts/bera-003/test-phase2-package.sh';backup='bak_uptime_'+source.name.lower();succeeded=False
try:
    docker('stop','--time','30',pg)
    subprocess.run(['bash',str(test),'execute',str(source),'wazi-uptime-kuma-01','install'],check=True,timeout=300)
    with urllib.request.urlopen('http://127.0.0.1:13001/',timeout=10) as response:assert response.status==200
    subprocess.run(['bash',str(test),'execute',str(source),'wazi-uptime-kuma-01','backup',backup],check=True,timeout=300)
    payload=pathlib.Path('/srv/wazi-runtime/phase2/wazi-uptime-kuma-01/backups')/(backup+'.age')
    metadata=json.loads(pathlib.Path(str(payload)+'.json').read_bytes())
    actual='sha256:'+hashlib.file_digest(payload.open('rb'),'sha256').hexdigest()
    assert metadata['Format']=='age-v1' and metadata['PayloadFormat']=='volume-tar' and metadata['PayloadDigest']==actual
    succeeded=True
finally:
    for container in instances('wazi-uptime-kuma-01'):docker('stop','--time','30',container)
    docker('start',pg)
    for attempt in range(30):
        try:
            assert docker('exec','--env','PGSERVICEFILE=/run/secrets/client.conf',pg,'psql','--dbname=service=wazi','--no-password','-Atc','SELECT 1').strip()==b'1'
            break
        except Exception:
            if attempt==29:raise
            time.sleep(1)
    record={'state':'passed' if succeeded else 'failed','postgresqlRestored':True,'uptimeStoppedAfterSequentialTest':not instances('wazi-uptime-kuma-01'),'volumesPreserved':True,'completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
    if succeeded:record.update(checkpointFile=str(payload),ciphertextDigest=actual,manifestDigest=metadata['ManifestDigest'])
    (source/'uptime-sequential-test.json').write_text(json.dumps(record,sort_keys=True,indent=2));print(json.dumps(record))
PY
