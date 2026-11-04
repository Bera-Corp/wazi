#!/usr/bin/env bash
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
export WAZI_KEYGEN_RELEASE="$1" WAZI_RECOVERY_RECIPIENT="$2"
python3 - <<'PY'
import datetime,json,os,pathlib,re,socket,stat,subprocess,time
release=pathlib.Path(os.environ['WAZI_KEYGEN_RELEASE']);recovery=os.environ['WAZI_RECOVERY_RECIPIENT']
assert release.parent==pathlib.Path('/opt/wazi-dev/releases')
assert re.fullmatch('age1[a-z0-9]{58}',recovery)
path=pathlib.Path('/etc/wazi-phase1/agent.json');original=path.read_bytes();config=json.loads(original)
state=pathlib.Path(config['Runtime']['StateDirectory']);uid=config['Runtime']['RuntimeUID']
assert {p.name for p in (state/'queue/requests').glob('*.json')}=={p.name for p in (state/'queue/results').glob('*.json')}
records=[]
for package in config['Packages']:
    project=package['Runtime']['Project']
    assert re.fullmatch('wazi-(postgresql|mysql|nginx|uptime-kuma)-0[12]',project)
    directory=pathlib.Path('/srv/wazi-runtime/secrets')/project
    assert directory.resolve()==directory and directory.is_dir()
    identity=directory/'backup-identity.txt';recipient=directory/'backup-recipient.txt'
    if not identity.exists() and not recipient.exists():
        subprocess.run([str(release/'wazi-backup-keygen'),'-identity-file',str(identity),'-recipient-file',str(recipient)],check=True)
    assert identity.is_file() and not identity.is_symlink() and recipient.is_file() and not recipient.is_symlink()
    for file in (identity,recipient):os.chown(file,uid,uid);os.chmod(file,0o600)
    public=recipient.read_text().strip();assert re.fullmatch('age1[a-z0-9]{58}',public)
    package['BackupEncryption']={'Recipients':[public,recovery],'IdentityFile':str(identity)}
    records.append({'project':project,'runtimeRecipient':public,'offhostRecipientConfigured':True})
backup=path.with_name('agent.json.before-backup-identities-'+datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ'))
backup.write_bytes(original);os.chmod(backup,0o600)
def replace(raw):
    info=path.stat();temporary=path.with_name('agent.json.backup-identities-next')
    with temporary.open('xb') as output:output.write(raw);output.flush();os.fsync(output.fileno())
    os.chown(temporary,info.st_uid,info.st_gid);os.chmod(temporary,stat.S_IMODE(info.st_mode));os.replace(temporary,path)
subprocess.run(['systemctl','stop','wazi-pilot-agent.service'],check=True)
try:
    replace(json.dumps(config,sort_keys=True,separators=(',',':')).encode())
    subprocess.run(['systemctl','start','wazi-pilot-agent.service'],check=True)
    for attempt in range(30):
        try:
            with socket.socket(socket.AF_UNIX) as client:client.settimeout(2);client.connect('/run/wazi-phase1/agent.sock')
            break
        except OSError:
            if attempt==29:raise
            time.sleep(1)
except Exception:
    replace(original);subprocess.run(['systemctl','restart','wazi-pilot-agent.service'],check=False);raise
print(json.dumps({'state':'backup-identities-configured','instances':records,'completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()},indent=2))
PY
