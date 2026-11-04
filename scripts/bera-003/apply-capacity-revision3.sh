#!/usr/bin/env bash
set -euo pipefail
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
export WAZI_CAPACITY_RELEASE="$1"
python3 - <<'PY'
import copy,datetime,hashlib,json,os,pathlib,socket,stat,subprocess,sys,time
release=pathlib.Path(os.environ['WAZI_CAPACITY_RELEASE'])
assert release.parent==pathlib.Path('/opt/wazi-dev/releases')
sys.path.insert(0,str(release/'scripts'))
import validate
config_path=pathlib.Path('/etc/wazi-phase1/agent.json')
budget_path=pathlib.Path('/etc/wazi-phase2/budget.json')
config=json.loads(config_path.read_bytes());prior_budget=json.loads(budget_path.read_bytes())
state=pathlib.Path(config['Runtime']['StateDirectory'])
assert {p.name for p in (state/'queue/requests').glob('*.json')}=={p.name for p in (state/'queue/results').glob('*.json')},'queue must be idle'
capacity=json.loads(subprocess.check_output(['/usr/local/libexec/wazi-phase2-quota','capacity']))
assert capacity['usedKiB']<11*1024*1024,'shared data exceeds reduced quota'
assert prior_budget['ownerBudget']=={'cpuCores':4.5,'ramMb':4096,'storageMb':49152}
now=datetime.datetime.now(datetime.timezone.utc)
budget=copy.deepcopy(prior_budget)
budget.update(budgetId='budget_phase2_uptime_revision3',approvedAt=now.isoformat(),approvedBy='pilot-owner (explicit 4 November 2026 storage reallocation and sequential testing)',validFrom=(now-datetime.timedelta(seconds=1)).isoformat(),validUntil=(now+datetime.timedelta(days=7)).isoformat())
for allocation in budget['allocations']:
    if allocation['role']=='application':allocation['resources'].update(storageMb=27648,ramMb=2432)
    elif allocation['role'] in ('backup-helper','restore-helper'):allocation['resources']['storageMb']=4096
assert not validate.contribution_budget_errors(budget)
schemas={p.name:validate.load(p) for p in (release/'schemas').glob('*.schema.json')}
assert not validate.schema_errors(validate.make_validator(schemas['contribution-budget.schema.json'],validate.build_registry(schemas)),budget)
canonical=lambda v:json.dumps(v,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode()
digest='sha256:'+hashlib.sha256(canonical(budget)).hexdigest()
for package in config['Packages']:
    package.update(ApplicationStorageBudgetMb=27648,BudgetDigest=digest,BudgetFile=str(budget_path))
backup=pathlib.Path('/etc/wazi-phase2')/('capacity-revision3-'+now.strftime('%Y%m%dT%H%M%SZ'))
backup.mkdir(mode=0o700)
originals={p:p.read_bytes() for p in (config_path,budget_path)}
for p,raw in originals.items():(backup/p.name).write_bytes(raw)
def replace(path,raw):
    before=path.stat();temporary=path.with_name(path.name+'.revision3-next')
    with temporary.open('xb') as output:output.write(raw);output.flush();os.fsync(output.fileno())
    os.chown(temporary,before.st_uid,before.st_gid);os.chmod(temporary,stat.S_IMODE(before.st_mode));os.replace(temporary,path)
subprocess.run(['systemctl','stop','wazi-pilot-agent.service'],check=True)
try:
    replace(budget_path,canonical(budget));replace(config_path,canonical(config))
    subprocess.run(['setquota','-P','30000','0',str(11*1024*1024),'0','0','/srv/wazi-runtime'],check=True)
    subprocess.run(['systemctl','start','wazi-pilot-agent.service'],check=True)
    subprocess.run(['systemctl','is-active','--quiet','wazi-pilot-agent.service'],check=True)
    for attempt in range(30):
        try:
            with socket.socket(socket.AF_UNIX) as client:client.settimeout(2);client.connect('/run/wazi-phase1/agent.sock')
            break
        except OSError:
            if attempt==29:raise
            time.sleep(1)
except Exception:
    for p,raw in originals.items():replace(p,raw)
    subprocess.run(['setquota','-P','30000','0',str(capacity['hardKiB']),'0','0','/srv/wazi-runtime'],check=False)
    subprocess.run(['systemctl','restart','wazi-pilot-agent.service'],check=False)
    raise
record={'applicationStorageMb':27648,'sharedStorageMb':11264,'protectedReserveMb':10240,'ownerStorageMb':49152,'runtimeLimitUnchanged':True,'budgetDigest':digest,'sequentialApplications':['postgresql','uptime-kuma'],'approvedBy':'pilot-owner','completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
(backup/'receipt.json').write_text(json.dumps(record,indent=2));print(json.dumps(record,indent=2))
PY
