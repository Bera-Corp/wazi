#!/usr/bin/env bash
# Private qualification workflow only. Does not approve or promote a candidate.
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
python3 - "$1" <<'PY'
import copy,datetime,hashlib,json,os,pathlib,pwd,re,shlex,ssl,subprocess,sys,time,urllib.request
source=pathlib.Path(sys.argv[1]);assert re.fullmatch(r'/home/wazi-dev/wazi-phase2/\d{8}T\d{6}Z',str(source))
assert json.loads((source/'qualification.json').read_text())['state']=='passed'
unit='wazi-controlplane-dev.service'
text=subprocess.check_output(['systemctl','cat',unit],text=True)
command=None
for line in text.splitlines():
    if line.startswith('ExecStart='):command=line[len('ExecStart='):] or None
assert command
args=shlex.split(command)
assert args[args.index('-approver-role')+1]=='maintainer' and '-approver-credentials-file' not in args
assert '-candidate-store-dir' not in args,'workflow already configured; preserve existing state'
original_policy=pathlib.Path(args[args.index('-signing-policy-file')+1]);policy=json.loads(original_policy.read_bytes())
folder=source/'releases/phase2/uptime-kuma';qdir=folder/'qualification-20261104'
qual=json.loads((qdir/'signing-policy.json').read_bytes());signer='wazi-qualification-20261104'
assert set(qual['trustedSigners'])=={signer} and signer not in policy['trustedSigners']
policy['trustedSigners'].update(qual['trustedSigners'])
service_user=subprocess.check_output(['systemctl','show',unit,'-p','User','--value'],text=True).strip()
account=pwd.getpwnam(service_user)
merged=original_policy.with_name('candidate-signing-policy.json')
with merged.open('xb') as output:output.write(json.dumps(policy,sort_keys=True,separators=(',',':')).encode())
os.chown(merged,0,account.pw_gid);os.chmod(merged,0o640)
store=pathlib.Path('/var/lib/wazi-controlplane-dev/candidates');store.mkdir(mode=0o700,exist_ok=False);os.chown(store,account.pw_uid,account.pw_gid)
args[args.index('-signing-policy-file')+1]=str(merged)
args+=['-candidate-store-dir',str(store),'-qualification-signers',signer]
assert all(re.fullmatch(r'[A-Za-z0-9_./:=,-]+',arg) for arg in args)
override=pathlib.Path('/etc/systemd/system/wazi-controlplane-dev.service.d/99-qualified-platform.conf');prior=override.read_bytes()
(source/'controlplane.before-candidates.conf').write_bytes(prior)
config=json.loads(pathlib.Path('/etc/wazi-phase1/agent.json').read_bytes());identity=config['Identity']
context=ssl.create_default_context(cafile=identity['CAFile']);context.load_cert_chain(identity['CertFile'],identity['KeyFile'])
op=None
for line in pathlib.Path('/etc/wazi-controlplane-dev/operator.env').read_text().splitlines():
    if line.startswith('WAZI_OPERATOR_TOKEN='):
        op=line.split('=',1)[1].strip().strip('"').strip("'")
assert op
class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self,*args,**kwargs):return None
opener=urllib.request.build_opener(NoRedirect,urllib.request.HTTPSHandler(context=context))
def api(method,route,body=None):
    request=urllib.request.Request('https://localhost:9443'+route,None if body is None else json.dumps(body,sort_keys=True,separators=(',',':')).encode(),method=method)
    request.add_header('X-Wazi-Operator-Token',op);request.add_header('Content-Type','application/json')
    with opener.open(request,timeout=30) as response:return json.load(response)
try:
    override.write_text('[Service]\nExecStart=\nExecStart='+' '.join(args)+'\n');os.chmod(override,0o644)
    subprocess.run(['systemctl','daemon-reload'],check=True);subprocess.run(['systemctl','restart',unit],check=True)
    for attempt in range(30):
        try:
            api('GET','/v1/package-candidates');break
        except Exception:
            if attempt==29:raise
            time.sleep(1)
except Exception:
    override.write_bytes(prior);subprocess.run(['systemctl','daemon-reload'],check=False);subprocess.run(['systemctl','restart',unit],check=False);raise
manifest=json.loads((folder/'manifest.json').read_bytes());manifest.pop('signature');manifest['reviewState']='candidate';provenance=manifest['provenance']
request={'manifest':manifest,'sources':[{'sourceId':provenance['sourceId'],'url':provenance['sourceId'],'contentHash':provenance['contentHash']}],'citations':[provenance['sourceId']],'origin':{'assistant':'Codex chat','requestDigest':'sha256:'+hashlib.sha256(b'Wazi owner approved Linux integration of existing pilots and Uptime Kuma on 2026-11-04').hexdigest()}}
candidate=api('POST','/v1/package-candidates',request)
candidate=api('POST','/v1/package-candidates/'+candidate['candidateId']+'/qualification',{'candidateDigest':candidate['candidateDigest'],'receipt':json.loads((qdir/'qualification.json').read_bytes())})
assert candidate['qualification']['result']=='passed' and candidate.get('review') is None
record={'state':'qualification-recorded-review-pending','candidateId':candidate['candidateId'],'candidateDigest':candidate['candidateDigest'],'qualificationId':candidate['qualification']['qualificationId'],'reviewApproved':False,'promoted':False,'completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
(source/'candidate-workflow-enabled.json').write_text(json.dumps(record,sort_keys=True,indent=2));print(json.dumps(record))
PY
