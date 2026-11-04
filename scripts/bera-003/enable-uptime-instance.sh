#!/usr/bin/env bash
# Enable only the qualified Uptime instance; execution still uses approved jobs.
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
python3 - "$1" "$2" <<'PY'
import copy,datetime,hashlib,json,os,pathlib,re,shutil,socket,stat,subprocess,sys,time
source=pathlib.Path(sys.argv[1]);recovery=sys.argv[2]
assert re.fullmatch(r'/home/wazi-dev/wazi-phase2/\d{8}T\d{6}Z',str(source))
assert re.fullmatch(r'age1[a-z0-9]{58}',recovery)
qualified=json.loads((source/'qualification.json').read_text());assert qualified['state']=='passed'
release=pathlib.Path('/opt/wazi-dev/releases')/(source.name+'-platform')
pid=subprocess.check_output(['systemctl','show','wazi-pilot-agent.service','-p','MainPID','--value'],text=True).strip()
assert pathlib.Path(os.readlink('/proc/'+pid+'/exe'))==release/'wazi-pilot-agent','deploy this qualified source first'
folder=source/'releases/phase2/uptime-kuma';tool=str(release/'wazi-recordtool')
path=pathlib.Path('/etc/wazi-phase1/agent.json');original=path.read_bytes();config=json.loads(original)
runtime=config['Runtime'];uid=runtime['RuntimeUID'];project='wazi-uptime-kuma-01'
assert not any(p['Profile']=='uptime-kuma' for p in config['Packages'])
assert all(p['ApplicationStorageBudgetMb']==27648 for p in config['Packages'])
state=pathlib.Path(runtime['StateDirectory'])
assert {p.name for p in (state/'queue/requests').glob('*.json')}=={p.name for p in (state/'queue/results').glob('*.json')}
before={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in (state/'journal').glob('*.json')}
for record in [folder/'manifest.json',*sorted(folder.glob('procedure-*.json'))]:
    subprocess.run([tool,'verify',str(record),config['SigningPolicyFile']],check=True)
qdir=folder/'qualification-20261104'
subprocess.run([tool,'verify',str(qdir/'qualification.json'),str(qdir/'signing-policy.json')],check=True)
manifest=json.loads((folder/'manifest.json').read_text());candidate=copy.deepcopy(manifest);candidate.pop('signature');candidate['reviewState']='candidate'
canonical=lambda value:json.dumps(value,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode()
digest=lambda value:'sha256:'+hashlib.sha256(canonical(value)).hexdigest()
receipt=json.loads((qdir/'qualification.json').read_text())
assert receipt['candidateDigest']==digest(candidate) and receipt['result']=='passed' and all(t['result']=='passed' for t in receipt['tests'])
assert receipt['tenantScope']==runtime['TenantScope']
completed=datetime.datetime.fromisoformat(receipt['completedAt'].replace('Z','+00:00'))
assert datetime.timedelta(0)<=datetime.datetime.now(datetime.timezone.utc)-completed<datetime.timedelta(days=1)
exported=json.loads((folder/'image-export.json').read_text());image,=manifest['release']['images']
assert image['digest']==exported['image']['digest']
prefix=['sudo','-u','wazi-runtime','env','HOME='+runtime['Home'],'XDG_RUNTIME_DIR=/run/user/'+str(uid),'DOCKER_HOST='+runtime['Endpoint'],'docker']
assert subprocess.check_output(prefix+['image','inspect',exported['image']['configDigest'],'--format','{{.Id}}'],text=True).strip()==exported['image']['configDigest']
directory=pathlib.Path('/srv/wazi-runtime/secrets')/project
directory.mkdir(mode=0o750,exist_ok=False);os.chmod(directory,0o750);os.chown(directory,0,uid)
identity=directory/'backup-identity.txt';recipient=directory/'backup-recipient.txt'
subprocess.run([str(release/'wazi-backup-keygen'),'-identity-file',str(identity),'-recipient-file',str(recipient)],check=True)
for file in (identity,recipient):os.chown(file,uid,uid);os.chmod(file,0o600)
public=recipient.read_text().strip();assert re.fullmatch(r'age1[a-z0-9]{58}',public)
instance=copy.deepcopy(next(p for p in config['Packages'] if p['Profile']=='nginx'))
instance.update(Profile='uptime-kuma',ReleaseVersion=manifest['version'],ProcedureVersion='1.5.0',SecretFiles={},BackupEncryption={'Recipients':[public,recovery],'IdentityFile':str(identity)})
instance.pop('Transitions',None);instance.pop('RecoveryReleaseVersion',None);instance.pop('RecoveryProcedureVersion',None)
instance['Runtime'].update(Project=project,StateDirectory='/srv/wazi-runtime/phase2/'+project,AllowedManifests={digest(manifest):True},ImageConfigDigests={image['digest']:exported['image']['configDigest']})
data=pathlib.Path(instance['Runtime']['StateDirectory']);data.mkdir(mode=0o700,exist_ok=False);os.chown(data,uid,uid)
config['Packages'].append(instance)
helper=pathlib.Path('/usr/local/libexec/wazi-phase2-quota');helper_before=helper.read_bytes()
backup=path.with_name('agent.json.before-uptime-'+source.name);shutil.copy2(path,backup)
def replace(raw):
    info=path.stat();temporary=path.with_name('agent.json.uptime-next')
    with temporary.open('xb') as output:output.write(raw);output.flush();os.fsync(output.fileno())
    os.chown(temporary,info.st_uid,info.st_gid);os.chmod(temporary,stat.S_IMODE(info.st_mode));os.replace(temporary,path)
subprocess.run(['systemctl','stop','wazi-pilot-agent.service'],check=True)
try:
    helper.write_bytes((source/'scripts/bera-003/phase2-volume-quota.py').read_bytes());os.chown(helper,0,0);os.chmod(helper,0o755)
    replace(canonical(config));subprocess.run(['systemctl','start','wazi-pilot-agent.service'],check=True)
    for attempt in range(30):
        try:
            raw=subprocess.check_output(['sudo','-u','wazi-runtime','curl','--fail','--silent','--unix-socket','/run/wazi-phase1/agent.sock','http://localhost/contexts/catalog'],timeout=5)
            catalog=json.loads(raw);assert len(catalog)==len(config['Packages'])
            break
        except Exception:
            if attempt==29:raise
            time.sleep(1)
    assert before=={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in (state/'journal').glob('*.json')}
except Exception:
    replace(original);helper.write_bytes(helper_before);subprocess.run(['systemctl','restart','wazi-pilot-agent.service'],check=False);raise
record={'state':'uptime-instance-enabled','project':project,'release':manifest['version'],'installed':False,'journalFilesUnchanged':len(before),'runtimeAndOffhostRecipientsConfigured':True,'completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
(source/'uptime-enabled.json').write_text(json.dumps(record,sort_keys=True));print(json.dumps(record))
PY
