#!/usr/bin/env bash
# Initialize private alpha measurement tooling without deploying applications.
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
python3 - "$1" <<'PY'
import datetime,json,os,pathlib,re,subprocess,sys
source=pathlib.Path(sys.argv[1]);assert re.fullmatch(r'/home/wazi-dev/wazi-phase2/\d{8}T\d{6}Z',str(source))
assert json.loads((source/'qualification.json').read_text())['state']=='passed'
release=pathlib.Path('/opt/wazi-dev/releases')/(source.name+'-platform')
config=json.loads(pathlib.Path('/etc/wazi-phase1/agent.json').read_bytes());runtime=config['Runtime'];uid=runtime['RuntimeUID']
assert config['Context']['WorkerPath']==str(release/'scripts/context_worker.py')
root=pathlib.Path(config['Context']['DatabasePath']).parent;assert root.is_dir() and root.stat().st_uid==uid
prefix=['sudo','-u','wazi-runtime','env','HOME='+runtime['Home'],'PYTHONDONTWRITEBYTECODE=1']
def private(path,value):
    with path.open('xb') as output:output.write(json.dumps(value,sort_keys=True,separators=(',',':')).encode())
    os.chown(path,uid,uid);os.chmod(path,0o600)
cohort=root/'alpha-cohort.json'
apps=['org.gitea.gitea','org.postgresql.postgresql','com.mysql.mysql','org.nginx.nginx','pet.kuma.uptime']
if not cohort.exists():private(cohort,{'tenantScope':runtime['TenantScope'],'applications':apps})
def cli(*args):
    raw=subprocess.check_output(prefix+['python3','-I',str(release/'scripts/alpha_ops.py'),'--database',str(root/'alpha.sqlite'),*args],timeout=30)
    return json.loads(raw)
cli('curate','--input',str(cohort))
qualification=json.loads(pathlib.Path(config['Packages'][0]['QualificationFile']).read_bytes())
assert qualification['result']=='qualified' and qualification['storage']['quotaEnforceable']
assert datetime.datetime.fromisoformat(qualification['expiresAt'].replace('Z','+00:00'))>datetime.datetime.now(datetime.timezone.utc)
docker=prefix+['env','XDG_RUNTIME_DIR=/run/user/'+str(uid),'DOCKER_HOST='+runtime['Endpoint'],'docker']
info=json.loads(subprocess.check_output(docker+['info','--format','{{json .}}'],timeout=30))
compose=subprocess.check_output(docker+['compose','version','--short'],text=True,timeout=15).strip().removeprefix('v')
observed={'family':'linux-engine','architecture':{'x86_64':'amd64','aarch64':'arm64'}[info['Architecture']],'desktopVersion':'0','guestVersion':info['KernelVersion'],'dockerVersion':info['ServerVersion'],'composeVersion':compose,'enforcement':'verified'}
observation=root/'alpha-environment.json'
if not observation.exists():private(observation,observed)
else:assert json.loads(observation.read_bytes())==observed
# Explicit attribution of this session's failed bootstrap and subsequent fix.
mapping={'schemaVersion':'1.0.0','jobs':{
 'job_5fdffe774b8054cce4e00f4a':{'trialId':'trial_uptime_bootstrap','attempt':1,'deterministic':True,'interventions':0},
 'job_6ccf03b130cb225b7be46d59':{'trialId':'trial_uptime_bootstrap','attempt':2,'deterministic':True,'interventions':0},
 'job_1539bd24efc6e4e9fd6204ab':{'trialId':'trial_uptime_restore','attempt':1,'deterministic':True,'interventions':0}}}
trial_path=root/'alpha-trials.json'
if not trial_path.exists():private(trial_path,mapping)
else:assert json.loads(trial_path.read_bytes())==mapping
imported=cli('import-queue','--queue',str(pathlib.Path(runtime['StateDirectory'])/'queue'),'--tenant',runtime['TenantScope'],'--environment',str(observation),'--target','vps','--mapping',str(trial_path))
report=cli('report','--tenant',runtime['TenantScope'])
assert report['observations']==3 and not report['alphaThresholdsMet']
assert report['metrics']['firstAttemptSuccess']['numerator']==0 and report['metrics']['firstAttemptSuccess']['denominator']==1
record={'state':'private-alpha-cohort-started','curatedApps':len(apps),'realObservations':report['observations'],'imported':imported['imported'],'alphaThresholdsMet':False,'publicContributionEnabled':False,'completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
(source/'alpha-cohort-started.json').write_text(json.dumps(record,sort_keys=True,indent=2));print(json.dumps(record))
PY
