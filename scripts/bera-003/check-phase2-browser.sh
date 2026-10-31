#!/usr/bin/env bash
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
python3 - "$1" <<'PY'
import datetime,hashlib,http.cookiejar,json,pathlib,ssl,subprocess,sys,time,urllib.error,urllib.request
source=pathlib.Path(sys.argv[1])
assert str(source).startswith('/home/wazi-dev/wazi-phase2/')
record=json.loads((source/'live/live-context-evidence.json').read_text())
config=json.loads(pathlib.Path('/etc/wazi-phase1/agent.json').read_text())
state=pathlib.Path(config['Runtime']['StateDirectory'])
def hashes():
    return {p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in (state/'journal').glob('*.json')}
before=hashes()
op=None
for line in pathlib.Path('/etc/wazi-controlplane-dev/operator.env').read_text().splitlines():
    if line.startswith('WAZI_OPERATOR_TOKEN='):
        op=line.split('=',1)[1].strip().strip('"').strip("'")
assert op
tls=ssl.create_default_context(cafile=config['Identity']['CAFile'])
jar=http.cookiejar.CookieJar()
class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self,*args,**kwargs): return None
opener=urllib.request.build_opener(NoRedirect,urllib.request.HTTPSHandler(context=tls),urllib.request.HTTPCookieProcessor(jar))
def request(method,path,body=None,headers=None):
    req=urllib.request.Request('https://localhost:9443'+path,
         None if body is None else json.dumps(body).encode(),method=method,
         headers={'Content-Type':'application/json',**(headers or {})})
    try:
        with opener.open(req,timeout=30) as response:
            return response.status,response.read(),response.headers
    except urllib.error.HTTPError as error:
        return error.code,error.read(),error.headers
for path in ('/','/healthz','/context','/context.js'):
    status,raw,headers=request('GET',path)
    assert status==200,(path,status)
    assert op.encode() not in raw
status,_,_=request('GET','/v1/contexts/'+record['contextId'])
assert status==401
status,raw,_=request('POST','/v1/context-sessions',{}, {'X-Wazi-Operator-Token':op})
assert status==201
code=json.loads(raw)['bootstrapCode']
status,raw,headers=request('POST','/v1/context-sessions/exchange',{'code':code},{'Origin':'https://localhost:9443'})
assert status==200
csrf=json.loads(raw)['csrf']
cookie=headers['Set-Cookie']
assert 'Secure' in cookie and 'HttpOnly' in cookie and 'SameSite=Strict' in cookie
status,_,_=request('POST','/v1/context-sessions/exchange',{'code':code},{'Origin':'https://localhost:9443'})
assert status==401
status,_,_=request('POST','/v1/contexts/retrieve',{}, {'Origin':'https://attacker.invalid','X-Wazi-CSRF':csrf})
assert status==403
status,_,_=request('POST','/v1/contexts/retrieve',{}, {'Origin':'https://localhost:9443'})
assert status==403
status,raw,_=request('GET','/v1/contexts/'+record['contextId'])
assert status==200
snapshot=json.loads(raw)
subprocess.run(['systemctl','restart','wazi-pilot-agent.service'],check=True,timeout=60,capture_output=True)
subprocess.run(['systemctl','is-active','--quiet','wazi-pilot-agent.service'],check=True,timeout=30,capture_output=True)
status,raw,_=request('GET','/v1/contexts/'+record['contextId'])
deadline=time.monotonic()+15
while status==503 and time.monotonic()<deadline:
    time.sleep(0.2)
    status,raw,_=request('GET','/v1/contexts/'+record['contextId'])
assert status==200 and json.loads(raw)==snapshot, ('stored snapshot after restart',status)
assert before==hashes(), ('journal changes', [name for name,value in hashes().items() if before.get(name)!=value])
with urllib.request.urlopen('http://127.0.0.1:13000/api/healthz',timeout=5) as response:
    assert json.load(response)['status']=='pass'
result={'httpsRootAndInspector':True,'unauthenticatedReadRejected':True,'oneUseBootstrap':True,
        'secureHttpOnlySameSiteCookie':True,'crossOriginAndMissingCsrfRejected':True,
        'storedContextSurvivesAgentRestart':True,'journalFilesUnchanged':len(before),
        'giteaHealth':'pass','completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
(source/'live/browser-evidence.json').write_text(json.dumps(result,indent=2,sort_keys=True)+'\n')
print(json.dumps(result,sort_keys=True))
PY
