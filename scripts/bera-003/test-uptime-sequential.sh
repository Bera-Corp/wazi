#!/usr/bin/env bash
# Owner-approved sequential live test. Preserve both applications' data.
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
python3 - "$@" <<'PY'
import datetime,hashlib,json,pathlib,re,subprocess,sys,time,urllib.request
source=pathlib.Path(sys.argv[1]);assert re.fullmatch(r'/home/wazi-dev/wazi-phase2/\d{8}T\d{6}Z',str(source))
mode=sys.argv[2] if len(sys.argv)>2 else 'qualify';assert mode in ('qualify','restore')
config=json.loads(pathlib.Path('/etc/wazi-phase1/agent.json').read_bytes());runtime=config['Runtime']
prefix=['sudo','-u','wazi-runtime','env','HOME='+runtime['Home'],'XDG_RUNTIME_DIR=/run/user/'+str(runtime['RuntimeUID']),'DOCKER_HOST='+runtime['Endpoint'],'docker']
def docker(*args):return subprocess.check_output(prefix+list(args),timeout=60)
def instances(project):return docker('ps','--filter','label=com.docker.compose.project='+project,'--format','{{.ID}}').decode().split()
pg,=instances('wazi-postgresql-02');assert not instances('wazi-uptime-kuma-01')
test=source/'scripts/bera-003/test-phase2-package.sh';backup='bak_uptime_'+source.name.lower();succeeded=False
operation='install'
anchor_path=pathlib.Path('/srv/wazi-runtime/phase2/wazi-uptime-kuma-01/instance-release.json')
if anchor_path.exists():
    anchor=json.loads(anchor_path.read_bytes())
    policy=next(p for p in config['Packages'] if p['Runtime']['Project']=='wazi-uptime-kuma-01')
    assert anchor['manifestDigest'] in policy['Runtime']['AllowedManifests'] and anchor['project']==policy['Runtime']['Project']
    assert not anchor_path.with_name('instance-transition.json').exists()
    operation='upgrade'
if mode=='restore':
    previous=json.loads((source/'uptime-sequential-test.json').read_bytes())
    assert previous['state']=='passed' and previous['manifestDigest']==anchor['manifestDigest']
    restored=pathlib.Path(previous['checkpointFile']);assert restored.parent==anchor_path.parent/'backups' and restored.suffix=='.age'
    restore_id=restored.stem;operation='restore';backup+='_after_restore'
try:
    docker('stop','--time','30',pg)
    if operation in ('upgrade','restore'):
        container,=docker('ps','-a','--filter','label=com.docker.compose.project=wazi-uptime-kuma-01','--format','{{.ID}}').decode().split()
        inspected=json.loads(docker('inspect',container))[0]
        assert inspected['Image']==anchor['imageConfig'] and inspected['HostConfig']['Memory']==512*1048576 and inspected['HostConfig']['NanoCpus']==500000000
        docker('start',container)
    input_id=restore_id if operation=='restore' else (backup+'_upgrade' if operation=='upgrade' else '')
    subprocess.run(['bash',str(test),'execute',str(source),'wazi-uptime-kuma-01',operation,input_id],check=True,timeout=300)
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
    record={'state':'passed' if succeeded else 'failed','operation':operation,'postgresqlRestored':True,'uptimeStoppedAfterSequentialTest':not instances('wazi-uptime-kuma-01'),'volumesPreserved':True,'completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
    if succeeded:record.update(checkpointFile=str(payload),ciphertextDigest=actual,manifestDigest=metadata['ManifestDigest'])
    (source/'uptime-sequential-test.json').write_text(json.dumps(record,sort_keys=True,indent=2));print(json.dumps(record))
PY
