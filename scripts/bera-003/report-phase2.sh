#!/usr/bin/env bash
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
python3 - "$1" <<'PY'
import datetime,hashlib,json,os,pathlib,sqlite3,ssl,subprocess,sys,time,urllib.request
source=pathlib.Path(sys.argv[1])
assert source.parent==pathlib.Path('/home/wazi-dev/wazi-phase2')
config=json.loads(pathlib.Path('/etc/wazi-phase1/agent.json').read_bytes())
runtime=config['Runtime']; state=pathlib.Path(runtime['StateDirectory'])
def run(args,**kwargs):return subprocess.run(args,check=True,capture_output=True,timeout=30,**kwargs).stdout
prefix=['sudo','-n','-u','wazi-runtime','env','HOME='+runtime['Home'],'XDG_RUNTIME_DIR=/run/user/1002','PATH=/usr/bin:/bin','docker','--host',runtime['Endpoint']]
def hashes():return {path.name:hashlib.sha256(path.read_bytes()).hexdigest() for path in (state/'journal').glob('*.json')}
before=hashes()
requests={path.name for path in (state/'queue/requests').glob('*.json')}
results={path.name for path in (state/'queue/results').glob('*.json')}
assert requests==results
operations=[json.loads(path.read_bytes()) for path in (state/'journal').glob('*.json')]
assert all(item['state'] in ('completed','failed','blocked','cancelled') for item in operations)
containers=[]
ids=run(prefix+['ps','--format','{{.ID}}']).decode().split()
for identity in ids:
    item=json.loads(run(prefix+['inspect',identity]))[0]
    limits=item['HostConfig']
    containers.append({'project':item['Config']['Labels'].get('com.docker.compose.project'),'imageConfig':item['Image'],'memoryBytes':limits['Memory'],'nanoCpus':limits['NanoCpus'],'capAdd':limits['CapAdd'],'capDrop':limits['CapDrop'],'portBindings':limits['PortBindings']})
assert {item['project'] for item in containers}=={'wazi-gitea-phase1','wazi-postgresql-02','wazi-mysql-02','wazi-nginx-02'}
for profile,program,args in [('postgresql','psql',['--dbname=service=wazi','--no-password','-Atc','SELECT 1']),('mysql','mysql',['--defaults-extra-file=/run/secrets/client.conf','--batch','--skip-column-names','--execute','SELECT 1','wazi'])]:
    ids=run(prefix+['ps','--filter','label=com.docker.compose.project=wazi-'+profile+'-02','--format','{{.ID}}']).decode().split()
    command=['exec']+(['--env','PGSERVICEFILE=/run/secrets/client.conf'] if profile=='postgresql' else [])+[ids[0],program]+args
    assert run(prefix+command).strip()==b'1'
for url in ('http://127.0.0.1:13000/api/healthz','http://127.0.0.1:18080/'):
    with urllib.request.urlopen(url,timeout=3) as response:
        raw=response.read()
        if 'healthz' in url: assert json.loads(raw)['status']=='pass'
        else: assert raw==b'Wazi Nginx ready\n'
proof=None
try:
    with urllib.request.urlopen('http://127.0.0.1:13000/api/v1/repos/wazi-pilot/lifecycle-proof/raw/proof.txt',timeout=5) as response:
        proof=hashlib.sha256(response.read()).hexdigest()
    assert proof=='728836b371192e623885770f40710bbe833d0f76d6cdbadfd9495621c25dc3e6'
except urllib.error.HTTPError:
    gitea_ids=run(prefix+['ps','--filter','label=com.docker.compose.project=wazi-gitea-phase1','--format','{{.ID}}']).decode().split()
    assert len(gitea_ids)==1
    repositories=run(prefix+['exec',gitea_ids[0],'find','/var/lib/gitea','-type','d','-name','lifecycle-proof.git']).decode().splitlines()
    assert len(repositories)==1 and repositories[0].startswith('/var/lib/gitea/')
    payload=run(prefix+['exec',gitea_ids[0],'git','--git-dir='+repositories[0],'show','HEAD:proof.txt'])
    proof=hashlib.sha256(payload).hexdigest()
    assert proof=='728836b371192e623885770f40710bbe833d0f76d6cdbadfd9495621c25dc3e6'
database=pathlib.Path(config['Context']['DatabasePath'])
def counts():
    with sqlite3.connect('file:'+str(database)+'?mode=ro',uri=True) as db:
        return {name:db.execute('SELECT COUNT(*) FROM '+name).fetchone()[0] for name in ('cases','evidence','contexts')}
prior_counts=counts()
with sqlite3.connect('file:'+str(database)+'?mode=ro',uri=True) as db:
    context_id=db.execute('SELECT id FROM contexts ORDER BY rowid DESC LIMIT 1').fetchone()[0]
operator=None
for line in pathlib.Path('/etc/wazi-controlplane-dev/operator.env').read_text().splitlines():
    if line.startswith('WAZI_OPERATOR_TOKEN='):
        operator=line.split('=',1)[1].strip().strip('"').strip("'")
assert operator
tls=ssl.create_default_context(cafile=config['Identity']['CAFile']); tls.load_cert_chain(config['Identity']['CertFile'],config['Identity']['KeyFile'])
class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self,*args,**kwargs):return None
opener=urllib.request.build_opener(NoRedirect,urllib.request.HTTPSHandler(context=tls))
def stored_context():
    request=urllib.request.Request('https://localhost:9443/v1/contexts/'+context_id,headers={'X-Wazi-Operator-Token':operator})
    with opener.open(request,timeout=10) as response:return hashlib.sha256(response.read()).hexdigest()
snapshot_digest=stored_context()
run(['systemctl','restart','wazi-pilot-agent.service'])
deadline=time.monotonic()+15
while True:
    try:
        catalog=json.loads(run(['curl','--fail','--silent','--unix-socket','/run/wazi-phase1/agent.sock','http://localhost/contexts/catalog']))
        assert len(catalog)==6
        break
    except Exception:
        if time.monotonic()>deadline:raise
        time.sleep(0.2)
assert hashes()==before and counts()==prior_counts and stored_context()==snapshot_digest
records=[]
for operation in operations:
    identity=operation['identity']
    if identity['applicationId']=='org.gitea.gitea':continue
    records.append({**identity,'state':operation['state'],'createdAt':operation['createdAt'],'updatedAt':operation['updatedAt'],'journalSha256':before[identity['operationId']+'.json']})
record={'kind':'phase2-live-package-report','sourceIdentity':json.loads((source/'source-identity.json').read_text(encoding='utf-8-sig')) if (source/'source-identity.json').exists() else {'directory':str(source)},'containers':containers,'aggregateMemoryBytes':sum(item['memoryBytes'] for item in containers),'aggregateNanoCpus':sum(item['nanoCpus'] for item in containers),'databaseReadiness':'authenticated SELECT 1 passed','giteaHealth':'pass','giteaProofSha256':proof,'journalFilesUnchangedAcrossAgentRestart':len(before),'pendingReconciliation':0,'caseStoreCounts':prior_counts,'caseStoreCountsUnchangedAcrossAgentRestart':True,'storedContextSurvivedRestart':{'id':context_id,'sha256':snapshot_digest},'packageOperations':sorted(records,key=lambda item:item['createdAt']),'aiCalls':0,'aiProvider':'deferred by owner','immich':'live qualification deferred by owner','verifiedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
(source/'phase2-live-report.json').write_text(json.dumps(record,sort_keys=True,indent=2)+'\n')
print(json.dumps({key:record[key] for key in ('verifiedAt','aggregateMemoryBytes','aggregateNanoCpus','databaseReadiness','giteaHealth','giteaProofSha256','journalFilesUnchangedAcrossAgentRestart','pendingReconciliation','caseStoreCounts','storedContextSurvivedRestart')}))
PY
