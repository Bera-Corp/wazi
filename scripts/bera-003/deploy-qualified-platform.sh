#!/usr/bin/env bash
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
source_dir="$1"
case "$source_dir" in /home/wazi-dev/wazi-phase2/20*T*Z) ;; *) exit 1 ;; esac
cd "$source_dir"
test -s qualification.json
python3 - <<'PY'
import hashlib,json,pathlib
receipt=json.loads(pathlib.Path('qualification.json').read_text())
assert receipt['state']=='passed'
assert hashlib.file_digest(open('source.tar.gz','rb'),'sha256').hexdigest()==receipt['source']['sourceSha256']
PY
sha256sum -c binary-sha256.txt
export WAZI_QUALIFIED_SOURCE="$source_dir"
python3 - <<'PY'
import datetime,hashlib,json,os,pathlib,re,shutil,socket,ssl,subprocess,time,urllib.request
source=pathlib.Path(os.environ['WAZI_QUALIFIED_SOURCE'])
release=pathlib.Path('/opt/wazi-dev/releases')/(source.name+'-platform')
units={'wazi-controlplane-dev.service':'wazi-controlplane','wazi-pilot-agent.service':'wazi-pilot-agent'}
def command(args):return subprocess.check_output(args,text=True,timeout=30)
config_path=pathlib.Path('/etc/wazi-phase1/agent.json')
original_config=config_path.read_bytes()
config=json.loads(original_config)
def replace_config(raw):
    info=config_path.stat();temporary=config_path.with_name('agent.json.qualified-next')
    with temporary.open('xb') as output:output.write(raw);output.flush();os.fsync(output.fileno())
    os.chown(temporary,info.st_uid,info.st_gid);os.chmod(temporary,info.st_mode & 0o777);os.replace(temporary,config_path)
state=pathlib.Path(config['StateDirectory']) if 'StateDirectory' in config else pathlib.Path(config['Runtime']['StateDirectory'])
def hashes():return {p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in (state/'journal').glob('*.json')}
requests={p.name for p in (state/'queue/requests').glob('*.json')}
results={p.name for p in (state/'queue/results').glob('*.json')}
if requests!=results:raise SystemExit('pending jobs prevent deployment')
before=hashes()
if not before or not (state/'queue/requests').is_dir() or not (state/'queue/results').is_dir():raise SystemExit('established runtime journal and queue required')
release.mkdir(mode=0o755,exist_ok=False)
os.chmod(release,0o755)
for name in ('wazi-controlplane','wazi-pilot-agent','wazi-agent','wazi-recordtool','wazi-backup-keygen'):
    shutil.copyfile(source/'bin'/name,release/name);os.chmod(release/name,0o555)
for name in ('scripts','schemas','community'):
    shutil.copytree(source/name,release/name,ignore=shutil.ignore_patterns('__pycache__','.hallmark'))
    for root,dirs,files in os.walk(release/name):
        os.chmod(root,0o755)
        for file in files:os.chmod(pathlib.Path(root)/file,0o644)
backups=[]
config_changed=False
try:
    command(['systemctl','stop','wazi-pilot-agent.service'])
    if config.get('Context'):
        (source/'agent-config.before').write_bytes(original_config)
        config['Context']['WorkerPath']=str(release/'scripts/context_worker.py')
        replace_config(json.dumps(config,sort_keys=True,separators=(',',':')).encode())
        config_changed=True
    for unit,binary in units.items():
        text=command(['systemctl','cat',unit])
        (source/(unit+'.before')).write_text(text)
        starts=[]
        for line in text.splitlines():
            if line.startswith('ExecStart='):
                if line=='ExecStart=':starts=[]
                else:starts.append(line[len('ExecStart='):])
        if len(starts)!=1:raise ValueError('one effective service command required')
        original=starts[0]
        match=re.match(r'(/[^\s]+)(.*)',original)
        if not match or pathlib.Path(match[1]).name not in (binary,binary.removeprefix('wazi-')):raise ValueError('unexpected service executable')
        directory=pathlib.Path('/etc/systemd/system')/(unit+'.d')
        directory.mkdir(mode=0o755,exist_ok=True)
        override=directory/'99-qualified-platform.conf'
        prior=override.read_bytes() if override.exists() else None
        backups.append((override,prior))
        if prior is not None:(source/(unit+'.override.before')).write_bytes(prior)
        override.write_text('[Service]\nExecStart=\nExecStart='+str(release/binary)+match[2]+'\n')
        os.chmod(override,0o644)
    command(['systemctl','daemon-reload'])
    command(['systemctl','restart','wazi-controlplane-dev.service'])
    context=ssl.create_default_context(cafile='/etc/wazi-controlplane-dev/server-ca.crt')
    for attempt in range(30):
        try:
            with urllib.request.urlopen('https://127.0.0.1:9443/healthz',context=context,timeout=2) as response:
                if response.status==200:break
        except Exception:
            if attempt==29:raise
            time.sleep(1)
    command(['systemctl','restart','wazi-pilot-agent.service'])
    for attempt in range(30):
        try:
            with socket.socket(socket.AF_UNIX) as client:client.settimeout(2);client.connect('/run/wazi-phase1/agent.sock')
            break
        except OSError:
            if attempt==29:raise
            time.sleep(1)
    if hashes()!=before:raise ValueError('operation journal changed during service deployment')
    for route in ('/','/assets/favicon.ico','/assets/wazi-wordmark-open.png'):
        with urllib.request.urlopen('https://127.0.0.1:9443'+route,context=context,timeout=3) as response:
            if response.status!=200:raise ValueError('browser entry or asset unavailable')
except Exception:
    if config_changed:replace_config(original_config)
    for path,prior in reversed(backups):
        if prior is None:path.unlink(missing_ok=True)
        else:path.write_bytes(prior)
    command(['systemctl','daemon-reload'])
    for unit in units:subprocess.run(['systemctl','restart',unit],check=False,timeout=30)
    raise
receipt={'state':'deployed','source':json.loads((source/'qualification.json').read_text()),'releaseDirectory':str(release),'services':list(units),'journalFilesPreserved':len(before),'browserEntry':'https://127.0.0.1:9443/','completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
(source/'platform-deployment.json').write_text(json.dumps(receipt,indent=2))
print(json.dumps(receipt,indent=2))
PY
