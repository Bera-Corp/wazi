#!/usr/bin/env bash
# Real, approved pilot actions only; no synthetic execution receipts.
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
python3 - "$@" <<'PY'
import datetime, hashlib, http.client, json, os, pathlib, socket, ssl, subprocess, sys, time, urllib.error, urllib.request
mode,directory=sys.argv[1:3]
source=pathlib.Path(directory)
assert source.parent==pathlib.Path('/home/wazi-dev/wazi-phase2')
config=json.loads(pathlib.Path('/etc/wazi-phase1/agent.json').read_bytes())
runtime=config['Runtime']
evidence=source/'package-evidence'; evidence.mkdir(mode=0o700,exist_ok=True)
operator=None
for line in pathlib.Path('/etc/wazi-controlplane-dev/operator.env').read_text().splitlines():
    if line.startswith('WAZI_OPERATOR_TOKEN='): operator=line.split('=',1)[1].strip().strip('"').strip("'")
assert operator
tls=ssl.create_default_context(cafile=config['Identity']['CAFile'])
tls.load_cert_chain(config['Identity']['CertFile'],config['Identity']['KeyFile'])
class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self,*args,**kwargs): return None
opener=urllib.request.build_opener(NoRedirect,urllib.request.HTTPSHandler(context=tls))
def api(method,path,body=None):
    request=urllib.request.Request('https://localhost:9443'+path,None if body is None else json.dumps(body,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode(),method=method,headers={'Content-Type':'application/json','X-Wazi-Operator-Token':operator})
    try:
        with opener.open(request,timeout=45) as response: return response.status,json.load(response)
    except urllib.error.HTTPError as response:
        return response.code,json.load(response)
def write(name,value):
    (evidence/name).write_text(json.dumps(value,sort_keys=True,indent=2)+'\n')
def assert_ok(status,value):
    assert status in (200,201,202),(status,value)
if mode in ('export-test-revocation','export-clear-revocation'):
    status,current=api('GET','/v1/revocations/current'); assert_ok(status,current)
    manifest=json.loads((source/'releases/phase2/nginx/manifest.json').read_bytes())
    target='sha256:'+hashlib.sha256(json.dumps(manifest,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode()).hexdigest()
    current.pop('signature',None)
    now=datetime.datetime.now(datetime.timezone.utc)
    current.update(sequence=current['sequence']+1,generatedAt=(now-datetime.timedelta(seconds=1)).isoformat(timespec='seconds').replace('+00:00','Z'),expiresAt=(now+datetime.timedelta(minutes=25)).isoformat(timespec='seconds').replace('+00:00','Z'))
    reason='phase2-qualification-temporary-revocation'
    if mode=='export-test-revocation':
        assert not any(entry.get('manifestDigest')==target for entry in current['entries'])
        current['entries'].append({'applicationId':manifest['appId'],'releaseVersion':manifest['version'],'manifestDigest':target,'revokedAt':now.isoformat(timespec='seconds').replace('+00:00','Z'),'reason':reason})
    else:
        removed=[entry for entry in current['entries'] if entry.get('manifestDigest')==target and entry.get('reason')==reason]
        assert len(removed)==1
        current['entries']=[entry for entry in current['entries'] if entry not in removed]
    write('revocation-list.unsigned.json',current)
    print(json.dumps({'preparedSequence':current['sequence'],'mode':mode,'entries':len(current['entries'])}))
elif mode in ('check-revoked-denial','check-revocation-cleared'):
    state=pathlib.Path(runtime['StateDirectory'])
    def hashes(): return {path.name:hashlib.sha256(path.read_bytes()).hexdigest() for path in (state/'journal').glob('*.json')}
    before=hashes()
    status,result=api('POST','/v1/revocations',json.loads((source/'revocation-list.json').read_bytes())); assert_ok(status,result)
    status,proposal=api('POST','/v1/context-proposals',{'project':'wazi-nginx-02','operation':'backup','backupId':'bak_revocation_probe'})
    if mode=='check-revoked-denial': assert status==422 and 'revok' in proposal.get('detail','').lower(),(status,proposal)
    else: assert status==200 and proposal['context']['decision']=='accept',(status,proposal)
    assert before==hashes()
    record={'check':mode,'journalFilesUnchanged':len(before),'denied':mode=='check-revoked-denial','checkedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
    write(mode+'.json',record); print(json.dumps(record))
elif mode=='negative-retrieval':
    state=pathlib.Path(runtime['StateDirectory'])
    before={path.name:hashlib.sha256(path.read_bytes()).hexdigest() for path in (state/'journal').glob('*.json')}
    status,value=api('POST','/v1/context-proposals',{'project':'wazi-mysql-02','operation':'backup','backupId':'bak_denial_probe'}); assert_ok(status,value)
    import copy
    job=value['job']; changed=copy.deepcopy(job); changed['budget']['ownerBudget']['ramMb']=1
    status,denied=api('POST','/v1/contexts/retrieve',changed); assert status==200 and denied['decision']=='reject'
    changed=copy.deepcopy(job); changed['envelope']['expiresAt']='2020-01-01T00:00:00Z'
    status,stale=api('POST','/v1/contexts/retrieve',changed); assert status==200 and stale['decision']=='reject'
    signed=json.loads((source/'releases/phase2/mysql/manifest.json').read_bytes()); signed['signature']['value']='invalid-signature'
    status,tampered=api('POST','/v1/manifests',signed); assert status==400 and tampered['error']=='manifest_rejected'
    assert before=={path.name:hashlib.sha256(path.read_bytes()).hexdigest() for path in (state/'journal').glob('*.json')}
    record={'tamperedBudgetDenied':True,'expiredEnvelopeDenied':True,'invalidSignatureDenied':True,'journalFilesUnchanged':len(before),'checkedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
    write('negative-retrieval.json',record); print(json.dumps(record))
elif mode=='cleanup-never-started-postgresql':
    # Only this qualification's failed fresh install, before any DB ran.
    project='wazi-postgresql-01'
    result=json.loads((evidence/(project+'-install-result.json')).read_bytes())
    receipt=result['operation']
    assert receipt['RecoveryRequired'] and receipt['PendingIntent']['capability']=='compose.up'
    identity=receipt['Operation']['identity']
    assert identity['applicationId']=='org.postgresql.postgresql' and identity['operation']=='install'
    prefix=['sudo','-n','-u','wazi-runtime','env','HOME='+runtime['Home'],'XDG_RUNTIME_DIR=/run/user/1002','PATH=/usr/bin:/bin','docker','--host',runtime['Endpoint']]
    def docker(args): return subprocess.run(prefix+args,check=True,capture_output=True,timeout=30).stdout
    ids=docker(['ps','-a','--filter','label=com.docker.compose.project='+project,'--format','{{.ID}}']).decode().split()
    containers=[]
    for identity_id in ids:
        item=json.loads(docker(['inspect',identity_id]))[0]
        assert not item['State']['Running'] and item['State']['StartedAt'].startswith('0001-')
        containers.append({'id':identity_id,'state':item['State']})
        docker(['rm',identity_id])
    name=project+'_data'
    volume=json.loads(docker(['volume','inspect',name]))[0]
    path=pathlib.Path(runtime['DataRoot'])/'volumes'/name/'_data'
    assert volume['Mountpoint']==str(path) and path.resolve()==path
    assert volume['Labels']['dev.wazi.project']==project and volume['Labels']['dev.wazi.tenant']==runtime['TenantScope']
    assert not list(path.iterdir()),'never remove populated qualification data'
    docker(['volume','rm',name])
    networks=docker(['network','ls','--filter','label=com.docker.compose.project='+project,'--format','{{.ID}}']).decode().split()
    for network in networks:
        inspected=json.loads(docker(['network','inspect',network]))[0]
        assert not inspected['Containers'] and inspected['Labels']['com.docker.compose.project']==project
        docker(['network','rm',network])
    observation={'project':project,'neverStartedContainers':containers,'volumeWasEmpty':True,'volumeRemoved':name,'networksRemoved':networks}
    raw=json.dumps(observation,sort_keys=True,separators=(',',':')).encode()
    observation_digest='sha256:'+hashlib.sha256(raw).hexdigest()
    class UnixConnection(http.client.HTTPConnection):
        def connect(self):
            self.sock=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM); self.sock.settimeout(15); self.sock.connect('/run/wazi-phase1/agent.sock')
    connection=UnixConnection('pilot')
    connection.request('POST','/reconcile',json.dumps({'operationId':identity['operationId'],'stepId':receipt['PendingIntent']['stepId'],'observedState':'absent','snapshotDigest':observation_digest}),{'Content-Type':'application/json'})
    response=connection.getresponse(); reconciled=json.loads(response.read())
    assert response.status==200 and reconciled['state']=='blocked'
    observation.update(snapshotDigest=observation_digest,reconciliation=reconciled,verifiedAt=datetime.datetime.now(datetime.timezone.utc).isoformat())
    write('failed-fresh-install-reconciliation.json',observation)
    print(json.dumps({'state':'blocked','neverStarted':True,'emptySyntheticVolumeRemoved':True,'snapshotDigest':observation_digest}))
elif mode=='cleanup-synthetic-first-instance':
    profile=sys.argv[3]
    assert profile in ('postgresql','mysql','nginx')
    project='wazi-'+profile+'-01'
    proof=json.loads((evidence/(project+('-nginx-verify.json' if profile=='nginx' else '-verify.json'))).read_bytes())
    if profile=='nginx': assert proof['responseSha256']==hashlib.sha256(b'Wazi Nginx ready\n').hexdigest()
    else: assert proof['proofSha256']==hashlib.sha256(b'wazi-phase2-synthetic-proof\n0').hexdigest()
    stopped=json.loads((evidence/(project+'-uninstall-result.json')).read_bytes())
    assert stopped['state']=='completed' and stopped['operation']['Operation']['identity']['operation']=='uninstall'
    job=json.loads((evidence/(project+'-backup-approved-job.json')).read_bytes())
    assert job['artifact']['projectName']==project and job['envelope']['operation']=='backup'
    backup=next(step['parameters']['backupId'] for step in job['plan']['steps'] if step['capability']=='volume.backup')
    package=next(p for p in config['Packages'] if p['Runtime']['Project']==project)
    extension='.age' if package.get('BackupEncryption',{}).get('Recipients') else '.dump'
    file=pathlib.Path('/srv/wazi-runtime/phase2')/project/'backups'/(backup+extension)
    metadata=json.loads(file.with_name(file.name+'.json').read_bytes())
    with file.open('rb') as handle: payload='sha256:'+hashlib.file_digest(handle,'sha256').hexdigest()
    assert metadata['PayloadDigest']==payload and metadata['Project']==project and metadata['Tenant']==runtime['TenantScope']
    prefix=['sudo','-n','-u','wazi-runtime','env','HOME='+runtime['Home'],'XDG_RUNTIME_DIR=/run/user/1002','PATH=/usr/bin:/bin','docker','--host',runtime['Endpoint']]
    def docker(args): return subprocess.run(prefix+args,check=True,capture_output=True,timeout=30).stdout
    assert not docker(['ps','-a','--filter','label=com.docker.compose.project='+project,'--format','{{.ID}}']).strip()
    name=project+('_config' if profile=='nginx' else '_data')
    volume=json.loads(docker(['volume','inspect',name]))[0]
    assert volume['Labels']['dev.wazi.project']==project and volume['Labels']['dev.wazi.tenant']==runtime['TenantScope']
    assert volume['Mountpoint']==runtime['DataRoot']+'/volumes/'+name+'/_data'
    docker(['volume','rm',name])
    record={'profile':profile,'project':project,'syntheticVolumeRemoved':name,'verifiedCheckpointRetained':payload,'preRestoreCheckpointsRetained':True,'completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
    write(project+'-synthetic-cleanup.json',record); print(json.dumps(record))
elif mode=='bootstrap':
    status,value=api('POST','/v1/context-sessions',{})
    assert_ok(status,value)
    write('browser-session.private.json',value)
    print('One-use browser session code issued; expires in 60 seconds')
elif mode=='publish':
    revocation=json.loads((source/'revocation-list.json').read_bytes())
    publications=[]
    for profile in ('postgresql','mysql','nginx','uptime-kuma'):
        folder=source/'releases/phase2'/profile
        for name in ['manifest']+[path.stem for path in sorted(folder.glob('procedure-*.json'))]:
            record=json.loads((folder/(name+'.json')).read_bytes())
            publications.append(('/v1/manifests' if name=='manifest' else '/v1/procedures',record))
    for endpoint,record in publications:
        status,value=api('POST',endpoint,record)
        assert_ok(status,value)
    status,value=api('POST','/v1/revocations',revocation)
    assert_ok(status,value)
    print('Published verified smaller-package contracts and fresh signed revocation list')
elif mode=='pause-mysql-for-readiness-test':
    prefix=['sudo','-n','-u','wazi-runtime','env','HOME='+runtime['Home'],'XDG_RUNTIME_DIR=/run/user/1002','PATH=/usr/bin:/bin','docker','--host',runtime['Endpoint']]
    ids=subprocess.run(prefix+['ps','--filter','label=com.docker.compose.project=wazi-mysql-02','--format','{{.ID}}'],check=True,capture_output=True,timeout=30).stdout.decode().split()
    assert len(ids)==1
    subprocess.run(prefix+['stop','--time','10',ids[0]],check=True,capture_output=True,timeout=30)
    write('mysql-readiness-fault.json',{'project':'wazi-mysql-02','container':ids[0],'deliberatelyStopped':True,'observedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()})
    print('Stopped only the synthetic MySQL second instance for a bounded readiness denial test')
elif mode in ('propose','execute','expect-readiness-failure'):
    project,operation=sys.argv[3:5]
    backup=sys.argv[5] if len(sys.argv)>5 else ''
    status,value=api('POST','/v1/context-proposals',{'project':project,'operation':operation,'backupId':backup})
    write(project+'-'+operation+'-proposal.json',value)
    assert_ok(status,value)
    assert value['context']['decision']=='accept' and value['context']['executionApproved'] is False,value['context']
    job=value['job']
    sys.path.insert(0,str(source/'scripts'))
    import context_engine as engine, validate
    for schema,record in [('execution-context-envelope-v2.schema.json',job['envelope']),('deployment-plan.schema.json',job['plan']),('compose-artifact.schema.json',job['artifact'])]:
        assert not engine._record_schema_errors(schema,record), 'canonical proposal schema mismatch'
    snapshot=value['context']['snapshot']
    procedures=[snapshot['procedure']]+([snapshot['rollbackProcedure']] if snapshot.get('rollbackProcedure') else [])
    errors=validate.plan_binding_errors(job['plan'],job['envelope'],snapshot['manifest'],snapshot['host'],snapshot['policy'],job['artifact'],procedures,eval_time=value['context']['evaluatedAt'])
    assert not errors,errors
    if mode=='propose':
        print(json.dumps({'project':project,'operation':operation,'context':value['context']['contextId'],'decision':'accept','executionApproved':False}))
        raise SystemExit(0)
    for action in job['envelope']['requiredApprovals']:
        status,approval=api('POST','/v1/context-approvals',{'caseId':job['caseId'],'plan':job['plan'],'envelope':job['envelope'],'hostId':runtime['HostID'],'action':action,'decision':'approved','validForSeconds':600})
        assert_ok(status,approval)
        job['approvals'].append({'approvalId':approval['approvalId'],'action':action})
    write(project+'-'+operation+'-approved-job.json',job)
    started=time.monotonic()
    status,result=api('POST','/v1/context-deployments',job); assert_ok(status,result)
    while time.monotonic()-started<240:
        status,result=api('GET','/v1/context-deployments/'+job['jobId']); assert_ok(status,result)
        if result.get('state')!='pending-or-running': break
        time.sleep(1)
    write(project+'-'+operation+'-result.json',result)
    if mode=='expect-readiness-failure':
        assert project=='wazi-mysql-02' and operation=='backup'
        observed=result['operation']
        assert observed['Operation']['state']=='failed' and not observed['RecoveryRequired']
        assert not any(event['kind']=='mutation-intent' for event in observed['Operation']['events'])
        assert not any((pathlib.Path('/srv/wazi-runtime/phase2')/project/'backups'/(backup+extension)).exists() for extension in ('.age','.dump'))
    else:
        assert result.get('state')=='completed',(result.get('state'),result.get('detail'),result.get('operation'))
    print(json.dumps({'project':project,'operation':operation,'jobId':job['jobId'],'state':result['state'],'seconds':round(time.monotonic()-started,2),'completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}))
elif mode in ('nginx-mutate','nginx-verify'):
    project='wazi-nginx-01'
    prefix=['sudo','-n','-u','wazi-runtime','env','HOME='+runtime['Home'],'XDG_RUNTIME_DIR=/run/user/1002','PATH=/usr/bin:/bin','docker','--host',runtime['Endpoint']]
    def docker(args): return subprocess.run(prefix+args,check=True,capture_output=True,timeout=30).stdout
    container=docker(['ps','--filter','label=com.docker.compose.project='+project,'--format','{{.ID}}']).decode().split()
    assert len(container)==1
    path=pathlib.Path(runtime['DataRoot'])/'volumes'/(project+'_config')/'_data/conf.d/default.conf'
    assert path.resolve()==path and path.is_file() and not path.is_symlink()
    if mode=='nginx-mutate':
        path.write_text("server { listen 80; server_name _; location / { default_type text/plain; return 200 'mutated-phase2'; } }\n")
        docker(['exec',container[0],'nginx','-t']); docker(['exec',container[0],'nginx','-s','reload'])
    expected=b'mutated-phase2' if mode=='nginx-mutate' else b'Wazi Nginx ready\n'
    deadline=time.monotonic()+10
    while True:
        with urllib.request.urlopen('http://127.0.0.1:18080/',timeout=3) as response: body=response.read()
        if body==expected: break
        assert time.monotonic()<deadline,'nginx response mismatch'
        time.sleep(0.2)
    record={'project':project,'action':mode,'responseSha256':hashlib.sha256(body).hexdigest(),'configSha256':hashlib.sha256(path.read_bytes()).hexdigest(),'verifiedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
    write(project+'-'+mode+'.json',record); print(json.dumps(record))
elif mode in ('seed','mutate','verify'):
    profile,project=sys.argv[3:5]
    assert profile in ('postgresql','mysql') and project in ('wazi-'+profile+'-01','wazi-'+profile+'-02')
    prefix=['sudo','-n','-u','wazi-runtime','env','HOME='+runtime['Home'],'XDG_RUNTIME_DIR=/run/user/1002','PATH=/usr/bin:/bin','docker','--host',runtime['Endpoint']]
    def docker(args,input=None): return subprocess.run(prefix+args,input=input,check=True,capture_output=True,timeout=30).stdout
    container=docker(['ps','--filter','label=com.docker.compose.project='+project,'--format','{{.ID}}']).decode().split()
    assert len(container)==1
    sql={'seed':"CREATE TABLE phase2_proof (id INTEGER PRIMARY KEY, value VARCHAR(100)); INSERT INTO phase2_proof VALUES (1,'wazi-phase2-synthetic-proof');",
         'mutate':"UPDATE phase2_proof SET value='mutated'; CREATE TABLE phase2_after_backup (id INTEGER);",
         'verify':"SELECT value FROM phase2_proof WHERE id=1; SELECT COUNT(*) FROM information_schema.tables WHERE table_name='phase2_after_backup';"}[mode]
    args=['exec','-i']
    if profile=='postgresql': args+=['--env','PGSERVICEFILE=/run/secrets/client.conf',container[0],'psql','--dbname=service=wazi','--no-password','--set=ON_ERROR_STOP=1','-At']
    else: args+=[container[0],'mysql','--defaults-extra-file=/run/secrets/client.conf','--batch','--skip-column-names','wazi']
    output=docker(args,sql.encode()).decode().strip()
    if mode=='verify': assert output=='wazi-phase2-synthetic-proof\n0', 'restored database proof mismatch'
    record={'profile':profile,'project':project,'action':mode,'proofSha256':hashlib.sha256(output.encode()).hexdigest() if mode=='verify' else None,'verifiedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
    write(project+'-'+mode+'.json',record); print(json.dumps(record))
else:
    raise SystemExit('unsupported test action')
PY
