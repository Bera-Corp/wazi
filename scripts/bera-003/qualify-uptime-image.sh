#!/usr/bin/env bash
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
export WAZI_UPTIME_SOURCE="$1" WAZI_RUNNER_RELEASE="$2"
python3 - <<'PY'
import datetime,hashlib,json,os,pathlib,shutil,subprocess,time,uuid
source=pathlib.Path(os.environ['WAZI_UPTIME_SOURCE']);release=pathlib.Path(os.environ['WAZI_RUNNER_RELEASE'])
assert source.parent.parent==pathlib.Path('/home/wazi-dev/wazi-phase2') and source.name=='uptime'
assert release.parent==pathlib.Path('/opt/wazi-dev/releases')
config=json.loads(pathlib.Path('/etc/wazi-phase1/agent.json').read_bytes());runtime=config['Runtime'];uid=runtime['RuntimeUID']
meta=json.loads((source/'package-export.json').read_text());suite=json.loads((source/'qualification-image.json').read_text())
with (source/'package-image.tar').open('rb') as image:assert 'sha256:'+hashlib.file_digest(image,'sha256').hexdigest()==meta['archive']['digest']
raw=(source/'uptime_kuma_qualification.mjs').read_bytes();local_suite_digest='sha256:'+hashlib.sha256(raw).hexdigest()
prefix=['sudo','-u','wazi-runtime','env','HOME='+runtime['Home'],'XDG_RUNTIME_DIR=/run/user/'+str(uid),'DOCKER_HOST='+runtime['Endpoint'],'docker']
def docker(*args,**kwargs):return subprocess.check_output(prefix+list(args),timeout=180,**kwargs)
with (source/'package-image.tar').open('rb') as image:docker('load',stdin=image)
base=meta['image']['configDigest'];actual=json.loads(docker('image','inspect',base))[0];assert actual['Id']==base and not actual['Config'].get('Volumes')
parent=pathlib.Path('/srv/wazi-runtime/qualification');parent.mkdir(mode=0o710,exist_ok=True);os.chmod(parent,0o710);os.chown(parent,0,uid)
subprocess.run(['chattr','-p','30000','+P',str(parent)],check=True)
root=parent/('uptime-'+uuid.uuid4().hex);root.mkdir(mode=0o700);os.chown(root,uid,uid)
script=root/'qualify';script.write_bytes(raw);os.chmod(script,0o555);os.chown(script,uid,uid)
# Runner entrypoint is fixed at /wazi/qualify; assembly must preserve that path.
# A stopped container is modified without executing candidate/application code.
name='wazi-suite-layout-'+uuid.uuid4().hex
try:
    container=docker('create','--name',name,'--network','none','--entrypoint','/bin/true',base).decode().strip()
    # Docker cp creates the directory from an operator-owned tree.
    folder=root/'wazi';folder.mkdir(mode=0o755);os.chmod(folder,0o755);os.chown(folder,uid,uid)
    shutil.copyfile(script,folder/'qualify');os.chmod(folder/'qualify',0o555);os.chown(folder/'qualify',uid,uid)
    docker('cp',str(folder),container+':/wazi')
    suite_id=docker('commit','--change','USER 65534:65534',container).decode().strip()
finally:
    subprocess.run(prefix+['rm','--force',name],check=False,stdout=subprocess.DEVNULL)
manifest=json.loads((source/'manifest.json').read_text());manifest.pop('signature',None);manifest['reviewState']='candidate'
runner=root/'qualification_runner.py';runner.write_bytes((source/'qualification_runner.py').read_bytes());os.chmod(runner,0o555);os.chown(runner,uid,uid)
policy={'image':suite_id,'tenantScope':runtime['TenantScope'],'runnerId':'bera003-uptime-local-suite','runnerVersion':'1.0.1','suiteDigest':local_suite_digest,'testIds':suite['testIds'],'cpuMilli':1000,'ramMb':512,'storageMb':256,'timeoutSeconds':180,'endpoint':runtime['Endpoint']}
for file,value in ((root/'manifest.json',manifest),(root/'policy.json',policy),(root/'local-suite-build.json',{'baseImageConfig':base,'applicationImage':meta['image'],'applicationArchiveDigest':meta['archive']['digest'],'suiteDigest':local_suite_digest,'registrySuiteDigest':suite['suiteDigest'],'localSuiteImageConfig':suite_id,'method':'stopped-container-copy-and-commit','registryQualificationImageUsed':False})):
    file.write_text(json.dumps(value,sort_keys=True));os.chmod(file,0o600);os.chown(file,uid,uid)
pg=docker('ps','--filter','label=com.docker.compose.project=wazi-postgresql-02','--format','{{.ID}}').decode().split();assert len(pg)==1
result=None
try:
    docker('stop','--time','30',pg[0])
    result=subprocess.run(['sudo','-u','wazi-runtime','env','HOME='+runtime['Home'],'PYTHONDONTWRITEBYTECODE=1','PYTHONPATH='+str(release/'scripts'),'python3',str(runner),'--policy',str(root/'policy.json'),'--manifest',str(root/'manifest.json'),'--output',str(root/'receipt')],check=False,timeout=240)
finally:
    docker('start',pg[0])
    for attempt in range(30):
        try:
            assert docker('exec','--env','PGSERVICEFILE=/run/secrets/client.conf',pg[0],'psql','--dbname=service=wazi','--no-password','-Atc','SELECT 1').strip()==b'1'
            break
        except Exception:
            if attempt==29:raise
            time.sleep(1)
record={'runnerExitCode':None if result is None else result.returncode,'receiptDirectory':str(root/'receipt'),'localSuiteBuild':str(root/'local-suite-build.json'),'postgresqlRestored':True,'completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
(source/'live-qualification.json').write_text(json.dumps(record,indent=2));print(json.dumps(record,indent=2))
if result is None or result.returncode:raise SystemExit('Uptime qualification failed; retain receipt and original application state')
PY
