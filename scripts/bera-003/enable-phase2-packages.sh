#!/usr/bin/env bash
# Applies only the owner-approved smaller-package budget and protected registry.
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
python3 - "$1" <<'PY'
import concurrent.futures, copy, datetime, hashlib, json, os, pathlib, secrets, shutil, subprocess, sys, time, urllib.request, zipfile

source = pathlib.Path(sys.argv[1])
assert source.parent == pathlib.Path('/home/wazi-dev/wazi-phase2') and source.is_dir()
config_path = pathlib.Path('/etc/wazi-phase1/agent.json')
config_raw = config_path.read_bytes()
config = json.loads(config_raw)
runtime = config['Runtime']
uid = runtime['RuntimeUID']
assert uid == 1002 and runtime['Project'] == 'wazi-gitea-phase1'
state = pathlib.Path(runtime['StateDirectory'])
assert {p.name for p in (state/'queue/requests').glob('*.json')} == {p.name for p in (state/'queue/results').glob('*.json')}, 'queue must be idle'
before = {p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in (state/'journal').glob('*.json')}

def run(args, timeout=60, **kwargs):
    return subprocess.run(args, check=True, capture_output=True, timeout=timeout, **kwargs)

def health():
    with urllib.request.urlopen('http://127.0.0.1:13000/api/healthz',timeout=5) as response:
        assert json.load(response)['status'] == 'pass'

def canonical(value):
    return json.dumps(value,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode()

def digest(value):
    return 'sha256:'+hashlib.sha256(canonical(value)).hexdigest()

def protected(path, raw, mode=0o640, group=uid):
    path = pathlib.Path(path)
    temporary = path.with_name('.'+path.name+'.phase2-tmp')
    with temporary.open('xb') as handle:
        handle.write(raw); handle.flush(); os.fsync(handle.fileno())
    os.chown(temporary,0,group); os.chmod(temporary,mode)
    os.replace(temporary,path)
    descriptor = os.open(path.parent,os.O_RDONLY|os.O_DIRECTORY)
    try: os.fsync(descriptor)
    finally: os.close(descriptor)

health()
prefix=['sudo','-n','-u','wazi-runtime','env','HOME='+runtime['Home'],'XDG_RUNTIME_DIR=/run/user/'+str(uid),'PATH=/usr/bin:/bin']
docker=prefix+['docker','--host',runtime['Endpoint']]
now=datetime.datetime.now(datetime.timezone.utc)
date=lambda value:value.isoformat(timespec='seconds').replace('+00:00','Z')
root=pathlib.Path('/etc/wazi-phase2')
root.mkdir(mode=0o750,exist_ok=True); os.chown(root,0,uid); os.chmod(root,0o750)
proposal=json.loads((source/'releases/phase2/package-budget-proposal.json').read_bytes())
assert proposal['proposedOwnerBudget']=={'cpuCores':4.5,'ramMb':4096,'storageMb':49152}
assert proposal['aggregateRuntimeLimit']=={'cpuCores':4,'ramMb':3072}
old_job=json.loads((state/'queue/requests/job_gitea_rollback_4.json').read_bytes())
budget=copy.deepcopy(old_job['budget'])
budget.update(budgetId='budget_phase2_smaller_packages',ownerBudget=proposal['proposedOwnerBudget'],allocations=proposal['allocations'],systemReserve=proposal['protectedSystemReserve'],approvedAt=date(now),approvedBy='pilot-owner (explicit Phase 2 budget approval; 1-of-1 pilot)',validFrom=date(now-datetime.timedelta(seconds=1)),validUntil=date(now+datetime.timedelta(days=7)))
ram=int(next(line.split()[1] for line in pathlib.Path('/proc/meminfo').read_text().splitlines() if line.startswith('MemTotal:')))//1024
budget['physical']={'cpuCores':os.cpu_count(),'ramMb':ram,'storageMb':shutil.disk_usage('/srv/wazi-runtime').total//1048576}
assert ram>=6144 and shutil.disk_usage('/srv/wazi-runtime').free>=48*1024**3
protected(root/'budget.json',canonical(budget))
protected(root/'owner-approval.json',canonical({'scope':proposal['scope'],'approvedBudget':proposal['proposedOwnerBudget'],'runtimeLimit':proposal['aggregateRuntimeLimit'],'approvalText':'Approve this Phase 2 budget','recordedAt':date(now),'productionQuorum':False}))

# Native quota helpers are fixed-name/limit programs; requests cannot choose paths.
for local,target in [('phase1-volume-quota.py','wazi-phase1-quota'),('phase2-volume-quota.py','wazi-phase2-quota')]:
    protected('/usr/local/libexec/'+target,(source/'scripts/bera-003'/local).read_bytes(),0o755,0)
sudoers=pathlib.Path('/etc/sudoers.d/wazi-phase2-quota')
protected(sudoers,b'wazi-runtime ALL=(root) NOPASS'+b'WD: /usr/local/libexec/wazi-phase2-quota\n',0o440,0)
run(['/usr/sbin/visudo','-cf',str(sudoers)])

# Existing application data stays on its own project IDs. Only journals,
# checkpoints and new package state join the bounded cache/scratch pool.
new_state=pathlib.Path('/srv/wazi-runtime/phase2')
new_state.mkdir(mode=0o700,exist_ok=True); os.chown(new_state,uid,uid)
for directory in (state,new_state):
    assert directory.resolve()==directory and directory.is_dir()
    paths=[directory]+list(directory.rglob('*'))
    assert all(not path.is_symlink() for path in paths), 'unexpected state symlink'
    for path in paths:
        if path.is_dir(): run(['chattr','-p','30000','+P',str(path)])
        elif path.is_file(): run(['chattr','-p','30000',str(path)])
run(['setquota','-P','30000','0',str(13*1024*1024),'0','0','/srv/wazi-runtime'])
run(['sudo','-n','/usr/local/libexec/wazi-phase2-quota','capacity'])
slice_dir=pathlib.Path('/etc/systemd/system/user-1002.slice.d')
slice_dir.mkdir(mode=0o755,exist_ok=True); os.chmod(slice_dir,0o755)
protected(slice_dir/'90-phase2-budget.conf',b'[Slice]\nMemoryMax=3072M\nCPUQuota=400%\n',0o644,0)
run(['systemctl','daemon-reload'])
run(['systemctl','set-property','--runtime','user-1002.slice','MemoryMax=3072M','CPUQuota=400%'])

# Authenticate bytes again on the host before loading any image.
packages=[]
secret_root=root/'secrets'; secret_root.mkdir(mode=0o750,exist_ok=True); os.chown(secret_root,0,uid); os.chmod(secret_root,0o750)
for profile,version in [('postgresql','17.11.0+wazi.1'),('mysql','8.4.13+wazi.1'),('nginx','1.30.6+wazi.1')]:
    release=source/'releases/phase2'/profile
    metadata=json.loads((release/'image-export.json').read_bytes())
    for name in ('manifest','procedure-install','procedure-upgrade','procedure-backup','procedure-restore','procedure-uninstall'):
        run([str(source/'bin/wazi-recordtool'),'verify',str(release/(name+'.json')),config['SigningPolicyFile']])
    image=metadata['image']
    manifest=json.loads((release/'manifest.json').read_bytes())
    declared,=manifest['release']['images']
    assert declared['image']==image['name'] and declared['digest']==image['digest']
    archive=source/'packages'/profile/'package-image.tar'
    archive.parent.mkdir(parents=True,exist_ok=True)
    def archive_valid():
        if not archive.is_file(): return False
        with archive.open('rb') as handle:
            return 'sha256:'+hashlib.file_digest(handle,'sha256').hexdigest()==metadata['archive']['digest']
    if not archive_valid():
        urls=json.loads((source/'artifact-transports.private.json').read_bytes())
        request=urllib.request.Request(urls[profile])
        # No credential is sent to this short-lived storage URL. Never log it.
        class NoRedirect(urllib.request.HTTPRedirectHandler):
            def redirect_request(self,*args,**kwargs): return None
        opener=urllib.request.build_opener(NoRedirect)
        transport=archive.with_suffix('.zip')
        head=urllib.request.Request(urls[profile],method='HEAD')
        with opener.open(head,timeout=30) as response:
            size=int(response.headers['Content-Length'])
        assert 0<size<1024**3
        with transport.open('wb') as output: output.truncate(size)
        def part(index):
            first=index*size//8; last=(index+1)*size//8-1
            segment=urllib.request.Request(urls[profile],headers={'Range':'bytes='+str(first)+'-'+str(last)})
            with urllib.request.build_opener(NoRedirect).open(segment,timeout=120) as response:
                assert response.status==206
                data=response.read(last-first+2)
                assert len(data)==last-first+1
            descriptor=os.open(transport,os.O_WRONLY)
            try:
                written=os.pwrite(descriptor,data,first)
                assert written==len(data)
            finally: os.close(descriptor)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
            list(workers.map(part,range(8)))
        with zipfile.ZipFile(transport) as bundle:
            member=bundle.getinfo('package-image.tar')
            assert member.file_size<2*1024**3
            with bundle.open(member) as input_bytes,archive.open('wb') as output:
                shutil.copyfileobj(input_bytes,output)
        transport.unlink()
        assert archive_valid(), 'archive digest mismatch'
    with archive.open('rb') as image_bytes:
        run(docker+['load'],stdin=image_bytes,timeout=180)
    assert run(docker+['image','inspect',image['configDigest'],'--format','{{.Id}}']).stdout.decode().strip()==image['configDigest']
    assert manifest['version']==version
    for index in ('01','02'):
        project='wazi-'+profile+'-'+index
        folder=secret_root/project; folder.mkdir(mode=0o750,exist_ok=True); os.chown(folder,0,uid); os.chmod(folder,0o750)
        secret_files={}
        if profile!='nginx':
            names=('password',) if profile=='postgresql' else ('password','root-password')
            for name in names:
                path=folder/name
                if not path.exists(): protected(path,secrets.token_urlsafe(36).encode())
                assert path.is_file() and not path.is_symlink() and path.stat().st_uid==0 and path.stat().st_mode & 0o027==0
                secret_files[name]=str(path)
            password=(folder/'password').read_text()
            client=('[wazi]\nhost=127.0.0.1\nport=5432\nuser=wazi\ndbname=wazi\npassword='+password+'\n') if profile=='postgresql' else ('[client]\nhost=127.0.0.1\nport=3306\nuser=wazi\npassword='+password+'\n')
            protected(folder/'client',client.encode()); secret_files['client']=str(folder/'client')
        directory=new_state/project; directory.mkdir(mode=0o700,exist_ok=True); os.chown(directory,uid,uid); os.chmod(directory,0o700)
        instance=copy.deepcopy(runtime)
        instance.update(Project=project,StateDirectory=str(directory),QuotaHelper='/usr/local/libexec/wazi-phase2-quota',AllowedManifests={digest(manifest):True},ImageConfigDigests={image['digest']:image['configDigest']})
        packages.append({'Profile':profile,'ReleaseVersion':version,'BudgetDigest':digest(budget),'BudgetFile':str(root/'budget.json'),'QualificationFile':str(root/'qualification.json'),'ApplicationStorageBudgetMb':25600,'Runtime':instance,'SecretFiles':secret_files})

# Protected qualification is produced only after an actual kernel quota probe.
volume='wz-phase2-enable-quota'
assert not run(docker+['volume','ls','--filter','name=^'+volume+'$','--format','{{.Name}}']).stdout.strip()
run(docker+['volume','create',volume])
path=pathlib.Path(runtime['DataRoot'])/'volumes'/volume/'_data'
assert path.resolve()==path
try:
    run(['chattr','-p','32001','+P',str(path)])
    run(['setquota','-P','32001','0','8192','0','0','/srv/wazi-runtime'])
    run(prefix+['dd','if=/dev/zero','of='+str(path/'probe'),'bs=1M','count=7','conv=fsync','status=none'])
    denied=subprocess.run(prefix+['dd','if=/dev/zero','of='+str(path/'probe'),'bs=1M','count=2','oflag=append','conv=notrunc,fsync','status=none'],capture_output=True,timeout=30)
    assert denied.returncode!=0 and b'Disk quota exceeded' in denied.stderr
finally:
    run(docker+['volume','rm',volume]); run(['setquota','-P','32001','0','0','0','0','/srv/wazi-runtime'])
daemon=json.loads(run(docker+['info','--format','{{json .}}']).stdout)
assert daemon['DockerRootDir']==runtime['DataRoot']
assert run(['findmnt','-T',runtime['DataRoot'],'-n','-o','FSTYPE']).stdout.strip()==b'ext4'
assert 'prjquota' in run(['findmnt','-T',runtime['DataRoot'],'-n','-o','OPTIONS']).stdout.decode()
now=datetime.datetime.now(datetime.timezone.utc)
qualification=copy.deepcopy(old_job['qualification'])
qualification.update(qualificationId='volqual_phase2_packages',observedAt=date(now),expiresAt=date(now+datetime.timedelta(minutes=30)),volumes=[{'name':'data','purpose':'application-data'},{'name':'config','purpose':'application-config'}])
qualification['storage']['storageFreeMb']=shutil.disk_usage(runtime['DataRoot']).free//1048576
qualification['daemon'].update(serverVersion=daemon['ServerVersion'],rootDir=daemon['DockerRootDir'])
protected(root/'qualification.json',canonical(qualification))

policy_path=pathlib.Path('/etc/wazi-controlplane-dev/policy-bundle.json')
policy_raw=policy_path.read_bytes()
policy=json.loads(policy_raw)
policy.update(version='1.1.0')
policy['licensing']['allowedLicenses']=['MIT','PostgreSQL','GPL-2.0-only','BSD-2-Clause']
for path in (config_path,policy_path):
    backup=path.with_name(path.name+'.before-packages-'+source.name)
    if not backup.exists(): shutil.copy2(path,backup)
try:
    run(['systemctl','stop','wazi-pilot-agent.service'])
    config.update(Packages=packages)
    config['CapabilityPolicy'].update(Version='1.1.0',Digest=digest(policy))
    protected(policy_path,canonical(policy),0o640,policy_path.stat().st_gid)
    protected(config_path,canonical(config),0o640,config_path.stat().st_gid)
    run(['systemctl','restart','wazi-controlplane-dev.service'])
    run(['systemctl','start','wazi-pilot-agent.service'])
    time.sleep(1)
    run(['systemctl','is-active','--quiet','wazi-controlplane-dev.service','wazi-pilot-agent.service'])
    health()
    assert before=={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in (state/'journal').glob('*.json')}
except Exception:
    protected(policy_path,policy_raw,0o640,policy_path.stat().st_gid)
    protected(config_path,config_raw,0o640,config_path.stat().st_gid)
    run(['systemctl','restart','wazi-controlplane-dev.service','wazi-pilot-agent.service'])
    raise
record={'operation':'enable-reviewed-phase2-packages','budgetDigest':digest(budget),'runtimeLimit':proposal['aggregateRuntimeLimit'],'applicationStorageMb':25600,'sharedStorageMb':13312,'protectedReserveMb':10240,'instances':[entry['Runtime']['Project'] for entry in packages],'journalFilesUnchanged':len(before),'quotaOverflowDenied':True,'giteaHealth':'pass','completedAt':date(datetime.datetime.now(datetime.timezone.utc))}
(source/'packages-enabled.json').write_bytes(canonical(record))
print(json.dumps(record))
PY
