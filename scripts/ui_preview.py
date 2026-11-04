"""Read-only loopback preview of Wazi's actual UI files. No deployment API."""
import argparse
import json
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
from pathlib import Path
import mimetypes
import sys
import urllib.parse

ROOT=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT))
from community.store import Store
from community.contracts import strict_json


def handler(snapshot=None):
    database=ROOT/'artifacts/ui-preview/catalog.sqlite'
    database.parent.mkdir(parents=True,exist_ok=True)
    store=Store(database);store.seed(strict_json((ROOT/'community/seeds.json').read_bytes()))
    assets=ROOT/'services/internal/controlplane/assets'
    class Preview(BaseHTTPRequestHandler):
        def log_message(self,*unused):pass
        def respond(self,status,body,mime='application/json'):
            if not isinstance(body,bytes):body=json.dumps(body).encode()
            self.send_response(status);self.send_header('Content-Type',mime);self.send_header('Content-Length',str(len(body)));self.send_header('Cache-Control','no-store');self.end_headers()
            if self.command!='HEAD':self.wfile.write(body)
        def do_POST(self):
            self.close_connection=True
            self.respond(405,{'error':'read-only-preview','detail':'Use a configured private workbench for real actions.'})
        do_PUT=do_DELETE=do_PATCH=do_POST
        def do_HEAD(self):self.do_GET()
        def do_GET(self):
            path=urllib.parse.urlsplit(self.path).path
            if path=='/healthz':return self.respond(200,{'status':'ok','scope':'read-only-ui-preview'})
            if path=='/api/starter':
                selected=urllib.parse.parse_qs(urllib.parse.urlsplit(self.path).query).get('application',[''])[0]
                project=next((item for item in store.applications() if item['applicationId']==selected),None)
                if project is None:return self.respond(404,{'error':'application-not-found'})
                return self.respond(200,{'schemaVersion':'1.0.0','applicationId':selected,'repository':project['repository'],'state':'discovery-starter','executionAllowed':False,'nextSteps':['Pin upstream commit and image digests','Preserve source licenses','Create typed configuration and procedures','Qualify in an isolated environment','Request owner review and signing']})
            if path=='/api/applications':return self.respond(200,{'applications':store.applications()})
            if path=='/api/me':return self.respond(200,{'user':None,'consent':None,'signInConfigured':False})
            if path=='/preview-data':
                empty={'cohort':[],'metrics':{},'observations':0,'support':[],'supportTickets':[],'recentObservations':[],
                       'appsWithSameReleaseLocalAndVpsRecovery':[],'minimumSamples':20,'alphaThresholdsMet':False,'generatedAt':None}
                return self.respond(200,dict(snapshot if snapshot is not None else empty,preview=True,snapshotSupplied=snapshot is not None))
            if path.startswith(('/v1/','/auth/')):return self.respond(403,{'error':'read-only-preview'})
            files={'/context':assets/'context.html','/context.js':assets/'context.js','/alpha':assets/'alpha.html','/alpha.js':assets/'alpha.js','/apps':ROOT/'community/web/index.html','/dashboard':ROOT/'community/web/index.html'}
            if path=='/':
                if (assets/'status.html').is_file():files['/']=assets/'status.html'
                else:
                    source=(ROOT/'services/internal/controlplane/status.go').read_text(encoding='utf-8')
                    mark=chr(96);body=source.split('const statusPage = '+mark)[1].split(mark)[0]
                    return self.respond(200,body.encode(),'text/html; charset=utf-8')
            if path.startswith('/assets/'):
                name=path.removeprefix('/assets/')
                if '/' not in name and '\\' not in name:
                    if name in ('catalog.css','catalog.js'):files[path]=ROOT/'community/web'/name
                    elif name=='wordmark.png':files[path]=assets/'wazi-wordmark-open.png'
                    elif Path(name).suffix in ('.css','.png','.ico','.svg','.zip'):files[path]=assets/name
            file=files.get(path)
            if file is None or file.is_symlink() or not file.is_file():return self.respond(404,{'error':'preview-page-not-found'})
            raw=file.read_bytes()
            if file.suffix=='.html':raw=raw.replace(b'data-mode="live"',b'data-mode="preview"')
            return self.respond(200,raw,mimetypes.guess_type(str(file))[0] or 'application/octet-stream')
    return Preview


if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--port',type=int,default=3688);parser.add_argument('--snapshot')
    args=parser.parse_args()
    if not 1024<=args.port<=65535:raise ValueError('unprivileged-loopback-port-required')
    snapshot=None
    if args.snapshot:
        path=Path(args.snapshot)
        if path.is_symlink() or path.stat().st_size>1<<20:raise ValueError('bounded-preview-snapshot-required')
        snapshot=strict_json(path.read_bytes())
    print('Read-only Wazi UI preview: http://127.0.0.1:'+str(args.port)+'/alpha',flush=True)
    ThreadingHTTPServer(('127.0.0.1',args.port),handler(snapshot)).serve_forever()
