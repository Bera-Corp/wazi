#!/usr/bin/env bash
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
python3 - "$@" <<'PY'
import copy, datetime, hashlib, http.client, json, os, pathlib, re, socket, ssl, subprocess, sys, urllib.error, urllib.request
mode, directory = sys.argv[1:3]
source = pathlib.Path(directory)
assert re.fullmatch(r'/home/wazi-dev/wazi-phase2/\d{8}T\d{6}Z',str(source))
config = json.loads(pathlib.Path('/etc/wazi-phase1/agent.json').read_text())
state = pathlib.Path(config['Runtime']['StateDirectory'])
out = source/'live'
out.mkdir(mode=0o755,exist_ok=True)
os.chmod(out,0o755)
identity = config['Identity']
tls = ssl.create_default_context(cafile=identity['CAFile'])
tls.load_cert_chain(identity['CertFile'],identity['KeyFile'])
op = None
for line in pathlib.Path('/etc/wazi-controlplane-dev/operator.env').read_text().splitlines():
    if line.startswith('WAZI_OPERATOR_TOKEN='):
        op = line.split('=',1)[1].strip().strip('"').strip("'")
assert op
def canonical(value):
    return json.dumps(value,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode()
def digest(value):
    return 'sha256:'+hashlib.sha256(canonical(value)).hexdigest()
def write(name,value):
    path=out/name
    path.write_bytes(canonical(value)+b'\n')
    os.chmod(path,0o644)
def api(method,path,value=None,operator=False,headers=None):
    h={'Content-Type':'application/json'}
    if operator:
        h['X-Wazi-Operator-Token']=op
    h.update(headers or {})
    request=urllib.request.Request('https://localhost:9443'+path,None if value is None else canonical(value),method=method,headers=h)
    try:
        with urllib.request.urlopen(request,context=tls,timeout=30) as response:
            return response.status,json.loads(response.read())
    except urllib.error.HTTPError as error:
        return error.code,json.loads(error.read())
status,current=api('GET','/v1/revocations/current')
assert status==200
if mode=='export-revocation':
    value=copy.deepcopy(current)
    value.pop('signature',None)
    now=datetime.datetime.now(datetime.timezone.utc)
    value.update(sequence=current['sequence']+1,generatedAt=(now-datetime.timedelta(seconds=2)).isoformat(timespec='seconds').replace('+00:00','Z'),expiresAt=(now+datetime.timedelta(minutes=25)).isoformat(timespec='seconds').replace('+00:00','Z'))
    write('revocation-list.unsigned.json',value)
    print(json.dumps({'exportedSequence':value['sequence'],'preservedEntries':len(value['entries'])}))
    raise SystemExit(0)
assert mode=='verify'
signed=json.loads((source/'revocation-list.json').read_text())
assert signed['entries']==current['entries'] and signed['listId']==current['listId'] and signed['sequence']==current['sequence']+1
status,reply=api('POST','/v1/revocations',signed,operator=True)
assert status in (200,201),(status,reply)

class UnixConnection(http.client.HTTPConnection):
    def connect(self):
        self.sock=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM)
        self.sock.settimeout(30)
        self.sock.connect('/run/wazi-phase1/agent.sock')
connection=UnixConnection('pilot')
connection.request('POST','/profile')
response=connection.getresponse()
profile_reply=json.loads(response.read())
assert response.status==200
status,profiles=api('GET','/v1/host-profiles')
assert status==200
profile=profiles[config['Runtime']['HostID']]
job=json.loads((state/'queue/requests/job_gitea_rollback_4.json').read_text())
assert digest(job['budget'])==config['BudgetDigest']

# Re-prove kernel hard-quota enforcement on a disposable managed volume only.
runtime=config['Runtime']
prefix=['sudo','-n','-u','wazi-runtime','env','HOME='+runtime['Home'],'XDG_RUNTIME_DIR=/run/user/'+str(runtime['RuntimeUID']),'LC_ALL=C','PATH=/usr/bin:/bin']
docker=prefix+['docker','--host',runtime['Endpoint']]
def run(args,check=True,**kwargs):
    return subprocess.run(args,check=check,capture_output=True,timeout=60,**kwargs)
volume='wz-phase2-qual-'+source.name.lower()
project='32001'
mount='/srv/wazi-runtime'
report=run(['repquota','-P',mount]).stdout.decode()
assert not any(line.split() and line.split()[0]=='#'+project for line in report.splitlines()),'qualification project already in use'
created=False
quota=False
try:
    assert not run(docker+['volume','ls','--filter','name=^'+volume+'$','--format','{{.Name}}']).stdout.strip()
    run(docker+['volume','create',volume]); created=True
    raw=run(docker+['volume','inspect',volume]).stdout
    info=json.loads(raw)[0]
    path=pathlib.Path(info['Mountpoint'])
    expected=pathlib.Path(runtime['DataRoot'])/'volumes'/volume/'_data'
    assert path==expected and path.resolve()==expected and info['Driver']=='local'
    assert run(['findmnt','-T',str(path),'-n','-o','FSTYPE']).stdout.strip()==b'ext4'
    assert 'prjquota' in run(['findmnt','-T',str(path),'-n','-o','OPTIONS']).stdout.decode()
    run(['chattr','-p',project,'+P',str(path)])
    run(['setquota','-P',project,'0','8192','0','0',mount]); quota=True
    probe=path/'quota-probe.bin'
    run(prefix+['dd','if=/dev/zero','of='+str(probe),'bs=1M','count=7','conv=fsync','status=none'])
    overflow=run(prefix+['dd','if=/dev/zero','of='+str(probe),'bs=1M','count=2','oflag=append','conv=notrunc,fsync','status=none'],check=False)
    assert overflow.returncode!=0 and b'Disk quota exceeded' in overflow.stderr
    assert probe.stat().st_size<=8*1024*1024
finally:
    if created:
        run(docker+['volume','rm',volume])
    if quota:
        run(['setquota','-P',project,'0','0','0','0',mount])
daemon=json.loads(run(docker+['info','--format','{{json .}}']).stdout)
now=datetime.datetime.now(datetime.timezone.utc)
start=(now-datetime.timedelta(seconds=1)).isoformat(timespec='seconds').replace('+00:00','Z')
end=(now+datetime.timedelta(minutes=10)).isoformat(timespec='seconds').replace('+00:00','Z')
qualification=copy.deepcopy(job['qualification'])
qualification['observedAt']=start
qualification['expiresAt']=end
qualification['daemon']['serverVersion']=daemon['ServerVersion']
qualification['daemon']['rootDir']=daemon['DockerRootDir']
qualification['storage']['storageFreeMb']=__import__('shutil').disk_usage(daemon['DockerRootDir']).free//(1024*1024)
job['qualification']=qualification
job['jobId']='job_phase2_context_'+source.name
job['caseId']='case_phase2_context_'+source.name
job['approvals']=[]
for record in (job['envelope'],job['plan']):
    record['createdAt']=start
    record['expiresAt']=end
job['envelope']['envelopeId']='env_phase2_context_'+source.name
job['plan']['envelopeId']=job['envelope']['envelopeId']
job['plan']['planId']='plan_phase2_context_'+source.name
job['envelope']['hostProfileRef']['digest']=digest(profile)
job['envelope']['managedVolumeQualificationRef']['digest']=digest(qualification)
write('proposal.json',job)
status,result=api('POST','/v1/contexts/retrieve',job,operator=True)
write('retrieval-result.json',result)
assert status==200 and result.get('decision')=='accept',(status,result.get('reasons'),result.get('error'))
assert result['executionApproved'] is False
context_id=result['contextId']
status,stored=api('GET','/v1/contexts/'+context_id,operator=True)
assert status==200 and stored==result
tampered=copy.deepcopy(job)
tampered['budget']['ownerBudget']['ramMb']=1
status,denied=api('POST','/v1/contexts/retrieve',tampered,operator=True)
assert status==200 and denied['decision']=='reject'
stale=copy.deepcopy(job)
stale['envelope']['expiresAt']='2020-01-01T00:00:00Z'
status,denied_stale=api('POST','/v1/contexts/retrieve',stale,operator=True)
assert status==200 and denied_stale['decision']=='reject'
with urllib.request.urlopen('http://127.0.0.1:13000/api/healthz',timeout=5) as response:
    assert json.load(response)['status']=='pass'
record={'contextId':context_id,'decision':result['decision'],'executionApproved':False,
        'caseCitations':len(result['successCaseRefs']),'tamperedBudgetRejected':True,
        'expiredEnvelopeRejected':True,'quotaOverflowDenied':True,'giteaHealth':'pass',
        'completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
write('live-context-evidence.json',record)
print(json.dumps(record,sort_keys=True))
PY
