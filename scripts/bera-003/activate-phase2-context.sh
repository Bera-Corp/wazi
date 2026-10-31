#!/usr/bin/env bash
# Upgrade context inspection only. Never changes application volumes or budgets.
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
run_dir="$1"
case "$run_dir" in /home/wazi-dev/wazi-phase2/20*T*Z) ;; *) exit 1 ;; esac
python3 - "$run_dir" <<'PY'
import datetime, hashlib, json, os, pathlib, re, shutil, socket, ssl, subprocess, sys, urllib.request

source = pathlib.Path(sys.argv[1])
stamp = source.name
assert re.fullmatch(r'\d{8}T\d{6}Z', stamp)
target = pathlib.Path('/opt/wazi-dev') / ('phase2-' + stamp)
config_path = pathlib.Path('/etc/wazi-phase1/agent.json')
original = config_path.read_bytes()
config = json.loads(original)
state = pathlib.Path(config['Runtime']['StateDirectory'])
assert str(state).startswith('/srv/wazi-runtime/')
requests = {p.name for p in (state/'queue/requests').glob('*.json')}
results = {p.name for p in (state/'queue/results').glob('*.json')}
assert requests == results, 'queue must be idle'
def journal_hashes():
    return {p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in (state/'journal').glob('*.json')}
before = journal_hashes()
for name in before:
    assert json.loads((state/'journal'/name).read_text())['state'] in ('completed','failed','blocked','cancelled')
def run(args, **kwargs):
    return subprocess.run(args, check=True, capture_output=True, timeout=60, **kwargs)
def health():
    with urllib.request.urlopen('http://127.0.0.1:13000/api/healthz',timeout=5) as response:
        assert json.load(response)['status'] == 'pass'
health()
if target.exists():
    assert target.is_dir() and not target.is_symlink() and target.stat().st_uid == 0
    for service in ('wazi-controlplane-dev','wazi-pilot-agent'):
        pid = run(['systemctl','show',service+'.service','-p','MainPID','--value']).stdout.decode().strip()
        assert str(target) not in os.readlink('/proc/'+pid+'/exe'), 'cannot overwrite a running installation'
else:
    target.mkdir(mode=0o755)
os.chmod(target,0o755)
for name in ('controlplane','pilot-agent','agent','recordtool'):
    binary = source/'bin'/('wazi-'+name)
    assert binary.is_file()
    shutil.copy2(binary,target/binary.name)
    os.chmod(target/binary.name,0o755)
context_root = target/'context'
context_root.mkdir(mode=0o755,exist_ok=True)
os.chmod(context_root,0o755)
for name in ('scripts','schemas'):
    shutil.copytree(source/name,context_root/name,ignore=shutil.ignore_patterns('__pycache__'),dirs_exist_ok=True)
    for parent,dirs,files in os.walk(context_root/name):
        os.chmod(parent,0o755)
        for filename in files:
            os.chmod(pathlib.Path(parent)/filename,0o644)
database_dir = state/'context'
database_dir.mkdir(mode=0o700,exist_ok=True)
os.chown(database_dir,config['Runtime']['RuntimeUID'],config['Runtime']['RuntimeUID'])
os.chmod(database_dir,0o700)
worker = str(context_root/'scripts/context_worker.py')
database = str(database_dir/'context.sqlite')
config['Context'] = {'PythonExecutable':'/usr/bin/python3','WorkerPath':worker,'DatabasePath':database}
config_backup = config_path.with_name(config_path.name+'.before-phase2-'+stamp)
shutil.copy2(config_path,config_backup)
dropins = []
backups = {}
for service,binary in (('wazi-controlplane-dev','controlplane'),('wazi-pilot-agent','pilot-agent')):
    unit = pathlib.Path('/etc/systemd/system')/(service+'.service')
    starts = [line for line in unit.read_text().splitlines() if line.startswith('ExecStart=')]
    assert len(starts)==1 and starts[0].startswith('ExecStart=/opt/wazi-dev/')
    command = starts[0].split('=',1)[1].split(' ',1)
    assert pathlib.Path(command[0]).name == 'wazi-'+binary
    directory = unit.with_name(unit.name+'.d')
    directory.mkdir(mode=0o755,exist_ok=True)
    dropin = directory/'80-phase2-context.conf'
    backups[dropin] = dropin.read_bytes() if dropin.exists() else None
    replacement = '[Service]\nExecStart=\nExecStart='+str(target/('wazi-'+binary))
    if len(command)>1:
        replacement += ' '+command[1]
    replacement += '\n'
    dropins.append((dropin,replacement))
def write_config(raw):
    temporary = config_path.with_name('.phase2-agent-'+stamp)
    with temporary.open('xb') as handle:
        handle.write(raw); handle.flush(); os.fsync(handle.fileno())
    info = config_path.stat()
    os.chown(temporary,info.st_uid,info.st_gid)
    os.chmod(temporary,info.st_mode & 0o777)
    os.replace(temporary,config_path)
try:
    # Import only terminal receipts from the trusted local queue before switching.
    imported = run(['sudo','-n','-u','wazi-runtime','/usr/bin/python3','-I',worker,'--database',database],
                   input=json.dumps({'action':'sync','queueDirectory':str(state/'queue')}).encode())
    summary = json.loads(imported.stdout)
    assert 'error' not in summary
    run(['systemctl','stop','wazi-pilot-agent.service'])
    write_config(json.dumps(config,sort_keys=True).encode())
    for path,text in dropins:
        path.write_text(text); os.chmod(path,0o644)
    run(['systemctl','daemon-reload'])
    run(['systemctl','restart','wazi-controlplane-dev.service'])
    run(['systemctl','start','wazi-pilot-agent.service'])
    run(['systemctl','is-active','--quiet','wazi-controlplane-dev.service','wazi-pilot-agent.service'])
    health()
    assert before == journal_hashes(), 'journal changed during inspection-only upgrade'
except Exception:
    write_config(original)
    for path,previous in backups.items():
        if previous is None:
            path.unlink(missing_ok=True)
        else:
            path.write_bytes(previous)
    run(['systemctl','daemon-reload'])
    run(['systemctl','restart','wazi-controlplane-dev.service','wazi-pilot-agent.service'])
    raise
record = {'operation':'phase2-context-upgrade','sourceDirectory':str(source),'installedDirectory':str(target),
          'journalFilesUnchanged':len(before),'terminalReceiptImport':summary,
          'giteaHealth':'pass','applicationBudgetsChanged':False,
          'completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
evidence = source/'context-activation.json'
evidence.write_text(json.dumps(record,sort_keys=True,indent=2)+'\n')
print(json.dumps(record,sort_keys=True))
PY
