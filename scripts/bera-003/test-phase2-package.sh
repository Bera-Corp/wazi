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
    request=urllib.request.Request('https://localhost:9443'+path,None if body is None else json.dumps(body).encode(),method=method,headers={'Content-Type':'application/json','X-Wazi-Operator-Token':operator})
    try:
        with opener.open(request,timeout=45) as response: return response.status,json.load(response)
    except urllib.error.HTTPError as response:
        return response.code,json.load(response)
def write(name,value):
    (evidence/name).write_text(json.dumps(value,sort_keys=True,indent=2)+'\n')
def assert_ok(status,value):
    assert status in (200,201,202),(status,value)
if mode=='publish':
    status,value=api('POST','/v1/revocations',json.loads((source/'revocation-list.json').read_bytes()))
    assert_ok(status,value)
    for profile in ('postgresql','mysql','nginx'):
        folder=source/'releases/phase2'/profile
        for name in ('manifest','procedure-install','procedure-upgrade','procedure-backup','procedure-restore','procedure-uninstall'):
            status,value=api('POST','/v1/manifests' if name=='manifest' else '/v1/procedures',json.loads((folder/(name+'.json')).read_bytes()))
            assert_ok(status,value)
    print('Published verified smaller-package contracts and fresh signed revocation list')
elif mode in ('propose','execute'):
    project,operation=sys.argv[3:5]
    backup=sys.argv[5] if len(sys.argv)>5 else ''
    status,value=api('POST','/v1/context-proposals',{'project':project,'operation':operation,'backupId':backup})
    write(project+'-'+operation+'-proposal.json',value)
    assert_ok(status,value)
    assert value['context']['decision']=='accept' and value['context']['executionApproved'] is False,value['context']
    job=value['job']
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
    assert result.get('state')=='completed',(result.get('state'),result.get('detail'),result.get('operation'))
    print(json.dumps({'project':project,'operation':operation,'jobId':job['jobId'],'state':result['state'],'seconds':round(time.monotonic()-started,2),'completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}))
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
