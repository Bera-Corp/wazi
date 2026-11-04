#!/usr/bin/env bash
# Refresh protected volume evidence using only a disposable quota-probe volume.
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
python3 - <<'PY'
import datetime,json,os,pathlib,stat,subprocess,uuid
config=json.loads(pathlib.Path('/etc/wazi-phase1/agent.json').read_bytes());runtime=config['Runtime'];uid=runtime['RuntimeUID']
prefix=['sudo','-u','wazi-runtime','env','HOME='+runtime['Home'],'XDG_RUNTIME_DIR=/run/user/'+str(uid),'DOCKER_HOST='+runtime['Endpoint'],'LC_ALL=C']
docker=prefix+['docker']
def run(args,**kw):return subprocess.run(args,capture_output=True,timeout=60,**kw)
def checked(args):return run(args,check=True).stdout
volume='wz-qualification-'+uuid.uuid4().hex;project='32001';mount='/srv/wazi-runtime'
report=checked(['repquota','-P',mount]).decode()
assert not any(row.split() and row.split()[0]=='#'+project and any(int(v) for v in (row.split()[2],row.split()[4])) for row in report.splitlines())
created=False;quota=False
try:
    checked(docker+['volume','create',volume]);created=True
    info=json.loads(checked(docker+['volume','inspect',volume]))[0]
    target=pathlib.Path(info['Mountpoint']);expected=pathlib.Path(runtime['DataRoot'])/'volumes'/volume/'_data'
    assert target==expected and target.resolve()==expected and not list(target.iterdir())
    assert checked(['findmnt','-T',str(target),'-n','-o','FSTYPE']).strip()==b'ext4'
    assert b'prjquota' in checked(['findmnt','-T',str(target),'-n','-o','OPTIONS'])
    checked(['chattr','-p',project,'+P',str(target)])
    checked(['setquota','-P',project,'0','8192','0','0',mount]);quota=True
    probe=target/'bounded.bin'
    checked(prefix+['dd','if=/dev/zero','of='+str(probe),'bs=1M','count=7','conv=fsync','status=none'])
    denied=run(prefix+['dd','if=/dev/zero','of='+str(probe),'bs=1M','count=2','oflag=append','conv=notrunc,fsync','status=none'])
    assert denied.returncode and b'Disk quota exceeded' in denied.stderr and probe.stat().st_size<=8*1048576
finally:
    if created:checked(docker+['volume','rm',volume])
    if quota:checked(['setquota','-P',project,'0','0','0','0',mount])
daemon=json.loads(checked(docker+['info','--format','{{json .}}']))
filesystem=checked(['findmnt','-T',daemon['DockerRootDir'],'-n','-o','TARGET']).decode().strip()
assert filesystem and pathlib.Path(filesystem).is_absolute()
now=datetime.datetime.now(datetime.timezone.utc)
files={p['QualificationFile'] for p in config['Packages']}
for name in files:
    path=pathlib.Path(name);assert path.parent==pathlib.Path('/etc/wazi-phase2') and not path.is_symlink()
    info=path.stat();record=json.loads(path.read_bytes())
    record.update(observedAt=now.isoformat(timespec='seconds').replace('+00:00','Z'),expiresAt=(now+datetime.timedelta(minutes=30)).isoformat(timespec='seconds').replace('+00:00','Z'))
    record['daemon'].update(serverVersion=daemon['ServerVersion'],rootDir=daemon['DockerRootDir'])
    record['storage']['storageFreeMb']=__import__('shutil').disk_usage(filesystem).free//1048576
    temporary=path.with_name(path.name+'.refresh-next')
    with temporary.open('xb') as output:output.write(json.dumps(record,sort_keys=True,separators=(',',':')).encode());output.flush();os.fsync(output.fileno())
    os.chown(temporary,info.st_uid,info.st_gid);os.chmod(temporary,stat.S_IMODE(info.st_mode));os.replace(temporary,path)
print(json.dumps({'quotaOverflowDenied':True,'disposableVolumeRemoved':True,'qualificationFilesRefreshed':len(files),'completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}))
PY
