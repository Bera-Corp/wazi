#!/usr/bin/env bash
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
release="$1"
case "$release" in /opt/wazi-dev/releases/20*T*Z-platform) ;; *) exit 1 ;; esac
test -f "$release/community/server.py"
export WAZI_COMMUNITY_RELEASE="$release"
python3 - <<'PY'
import datetime,json,os,pathlib,pwd,shutil,ssl,subprocess,time,urllib.request
release=pathlib.Path(os.environ['WAZI_COMMUNITY_RELEASE'])
user='wazi-community-dev'
try:account=pwd.getpwnam(user)
except KeyError:
    subprocess.run(['useradd','--system','--home-dir','/var/lib/'+user,'--no-create-home','--shell','/usr/sbin/nologin',user],check=True)
    account=pwd.getpwnam(user)
state=pathlib.Path('/var/lib/'+user);config=pathlib.Path('/etc/'+user)
for directory,mode in ((state,0o700),(state/'objects',0o700),(config,0o750)):
    directory.mkdir(mode=mode,exist_ok=True);os.chmod(directory,mode);os.chown(directory,account.pw_uid if directory.is_relative_to(state) else 0,account.pw_gid)
for name in ('server.crt','server.key'):
    target=config/name
    shutil.copyfile(pathlib.Path('/etc/wazi-controlplane-dev')/name,target)
    os.chown(target,0,account.pw_gid);os.chmod(target,0o640)
environment=config/'oauth.env'
if not environment.exists():environment.write_text('')
os.chmod(environment,0o600)
unit=pathlib.Path('/etc/systemd/system/wazi-community-dev.service')
prior=unit.read_bytes() if unit.exists() else None
if prior is not None:(config/'service.before').write_bytes(prior)
text='''[Unit]
Description=Wazi private community catalog
After=network.target
[Service]
Type=simple
User=wazi-community-dev
Group=wazi-community-dev
EnvironmentFile=/etc/wazi-community-dev/oauth.env
Environment=PYTHONDONTWRITEBYTECODE=1
WorkingDirectory=RELEASE
ExecStart=/usr/bin/python3 -m community.server --database /var/lib/wazi-community-dev/catalog.sqlite --objects /var/lib/wazi-community-dev/objects --origin https://localhost:9444 --bind 127.0.0.1 --port 9444 --certificate /etc/wazi-community-dev/server.crt --key /etc/wazi-community-dev/server.key
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/wazi-community-dev
CapabilityBoundingSet=
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
MemoryMax=256M
CPUQuota=25%
TasksMax=64
Restart=on-failure
[Install]
WantedBy=multi-user.target
'''.replace('RELEASE',str(release))
unit.write_text(text);os.chmod(unit,0o644)
try:
    subprocess.run(['systemd-analyze','verify',str(unit)],check=True)
    subprocess.run(['systemctl','daemon-reload'],check=True)
    subprocess.run(['systemctl','enable','--now',unit.name],check=True)
    subprocess.run(['systemctl','restart',unit.name],check=True)
    context=ssl.create_default_context(cafile='/etc/wazi-controlplane-dev/server-ca.crt')
    for attempt in range(30):
        try:
            with urllib.request.urlopen('https://localhost:9444/',context=context,timeout=3) as response:
                if response.status!=200:raise ValueError('catalog root unavailable')
            with urllib.request.urlopen('https://localhost:9444/api/applications',context=context,timeout=3) as response:
                applications=json.load(response)
            break
        except Exception:
            if attempt==29:raise
            time.sleep(1)
except Exception:
    subprocess.run(['systemctl','stop',unit.name],check=False)
    if prior is None:unit.unlink(missing_ok=True)
    else:unit.write_bytes(prior)
    subprocess.run(['systemctl','daemon-reload'],check=False)
    if prior is not None:subprocess.run(['systemctl','start',unit.name],check=False)
    raise
receipt={'state':'private-catalog-deployed','release':str(release),'root':'https://localhost:9444/','catalogResponded':True,'oauthConfigured':bool(environment.read_text().strip()),'applicationDataMigrated':False,'completedAt':datetime.datetime.now(datetime.timezone.utc).isoformat()}
(config/'deployment.json').write_text(json.dumps(receipt,indent=2))
print(json.dumps(receipt,indent=2))
PY
