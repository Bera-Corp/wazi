"""Private alpha reliability ledger, support queue and explicit feedback export.

Records operator-supplied evidence references, not raw application logs, secrets,
beneficiary identities or model prompts. No network or execution API is exposed.
Metrics are measured from these observations; they are not independent assurance.
"""
import argparse
import datetime as dt
import hashlib
import html
import json
import os
from pathlib import Path
import re
import sqlite3
import sys

sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
sys.path.insert(0,str(Path(__file__).resolve().parent))
from community.contracts import canonical, closed, environment, identifier, sha, strict_json
from secret_scan import scan_text
import validate

KINDS={"deployment":{"success","failure"},"rollback":{"success","failure"},"restore":{"success","failure"},
       "planner":{"valid","invalid"},"compatibility":{"compatible","incompatible","false-compatible"},
       "safety":{"clear","secret-exposure","uncited-action","arbitrary-shell"}}
CATEGORIES={"installation","backup","restore","rollback","compatibility","usability","metadata"}
TRANSITIONS={"open":{"triaged"},"triaged":{"resolved","open"},"resolved":{"closed","open"},"closed":{"open"}}


def utc():return dt.datetime.now(dt.timezone.utc).isoformat()


def private_identifier(value):
    if not isinstance(value,str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,127}",value):
        raise ValueError("invalid-private-identifier")
    return value


def checked(value):
    raw=canonical(value)
    if len(raw)>65536 or "wazi-secret-scan: allow" in raw.decode("ascii") or scan_text(raw.decode("ascii")):
        raise ValueError("bounded-secret-free-observation-required")
    return raw


def proofs(value):
    if not isinstance(value,list) or not 1<=len(value)<=16 or len(set(value))!=len(value):raise ValueError("unique-evidence-digests-required")
    for digest in value:sha(digest)


class Ledger:
    def __init__(self,path):
        path=Path(path)
        self.path=path
        if path.is_symlink():raise ValueError("ledger-symlink-forbidden")
        if os.name=="posix":os.umask(0o077)
        self.db=sqlite3.connect(path,timeout=15)
        self.db.row_factory=sqlite3.Row
        self.db.executescript("""
        PRAGMA foreign_keys=ON;
        CREATE TABLE IF NOT EXISTS curated(tenant TEXT, app TEXT, PRIMARY KEY(tenant,app));
        CREATE TABLE IF NOT EXISTS observations(tenant TEXT, id TEXT, trial TEXT, kind TEXT, attempt INTEGER, digest TEXT, body BLOB, recorded TEXT,
          PRIMARY KEY(tenant,id), UNIQUE(tenant,trial,kind,attempt));
        CREATE TABLE IF NOT EXISTS tickets(tenant TEXT,id TEXT,body BLOB,state TEXT,PRIMARY KEY(tenant,id));
        CREATE TABLE IF NOT EXISTS ticket_metadata(tenant TEXT,id TEXT,created TEXT,PRIMARY KEY(tenant,id));
        CREATE TABLE IF NOT EXISTS support_events(tenant TEXT,id TEXT,ticket TEXT,digest TEXT,body BLOB,recorded TEXT,PRIMARY KEY(tenant,id));
        """)
        self.db.commit()
    def close(self):self.db.close()
    def curate(self,tenant,apps):
        private_identifier(tenant)
        if not isinstance(apps,list) or not 5<=len(apps)<=10 or len(set(apps))!=len(apps):raise ValueError("five-to-ten-curated-apps-required")
        for app in apps:identifier(app)
        with self.db:
            current={row[0] for row in self.db.execute("SELECT app FROM curated WHERE tenant=?",(tenant,))}
            if current and current!=set(apps):raise ValueError("curation-is-immutable-create-new-alpha-cohort")
            self.db.executemany("INSERT OR IGNORE INTO curated VALUES(?,?)",[(tenant,app) for app in apps])
    def scope(self,tenant,app):
        private_identifier(tenant);identifier(app)
        if not self.db.execute("SELECT 1 FROM curated WHERE tenant=? AND app=?",(tenant,app)).fetchone():raise ValueError("app-not-curated-for-tenant")
    def record(self,event):
        closed(event,"schemaVersion eventId trialId tenantScope applicationId releaseDigest environment target kind outcome attempt deterministic interventions elapsedMs evidenceDigests")
        if event["schemaVersion"]!="1.0.0":raise ValueError("observation-version")
        self.scope(event["tenantScope"],event["applicationId"]);private_identifier(event["eventId"]);private_identifier(event["trialId"])
        sha(event["releaseDigest"]);environment(event["environment"]);proofs(event["evidenceDigests"])
        if event["target"] not in ("local","vps") or event["kind"] not in KINDS or event["outcome"] not in KINDS[event["kind"]]:raise ValueError("observation-kind-or-outcome")
        if event["kind"] in ("compatibility","safety") and event["attempt"]!=1:raise ValueError("audit-decisions-are-not-retriable")
        if type(event["deterministic"]) is not bool:raise ValueError("deterministic-boolean-required")
        for key,low,high in (("attempt",1,10),("interventions",0,10000),("elapsedMs",0,86400000)):
            if type(event[key]) is not int or not low<=event[key]<=high:raise ValueError("observation-number-bound")
        raw=checked(event);digest=hashlib.sha256(raw).hexdigest();tenant=event["tenantScope"]
        with self.db:
            self.db.execute("BEGIN IMMEDIATE")
            old=self.db.execute("SELECT digest FROM observations WHERE tenant=? AND id=?",(tenant,event["eventId"])).fetchone()
            if old:
                if old[0]!=digest:raise ValueError("conflicting-observation-replay")
                return False
            if self.db.execute("SELECT count(*) FROM observations WHERE tenant=?",(tenant,)).fetchone()[0]>=100000:raise ValueError("alpha-observation-capacity")
            prior=self.db.execute("SELECT body FROM observations WHERE tenant=? AND trial=? AND kind=? ORDER BY attempt DESC LIMIT 1",(tenant,event["trialId"],event["kind"])).fetchone()
            previous=strict_json(prior[0]) if prior else None
            if event["attempt"]!=(previous["attempt"]+1 if previous else 1):raise ValueError("consecutive-attempts-required")
            if previous:
                if any(previous[key]!=event[key] for key in ("applicationId","releaseDigest","environment","target")):raise ValueError("retry-binding-changed")
                if previous["outcome"] in ("success","valid"):raise ValueError("successful-trial-is-terminal")
            self.db.execute("INSERT INTO observations VALUES(?,?,?,?,?,?,?,?)",(tenant,event["eventId"],event["trialId"],event["kind"],event["attempt"],digest,raw,utc()))
        return True
    def ticket(self,value):
        closed(value,"schemaVersion ticketId tenantScope applicationId releaseDigest environment category severity evidenceDigests")
        if value["schemaVersion"]!="1.0.0":raise ValueError("support-version")
        self.scope(value["tenantScope"],value["applicationId"]);private_identifier(value["ticketId"]);sha(value["releaseDigest"])
        environment(value["environment"]);proofs(value["evidenceDigests"]);raw=checked(value)
        if value["category"] not in CATEGORIES or value["severity"] not in ("P0","P1","P2","P3"):raise ValueError("support-classification")
        with self.db:
            old=self.db.execute("SELECT body FROM tickets WHERE tenant=? AND id=?",(value["tenantScope"],value["ticketId"])).fetchone()
            if old:
                if bytes(old[0])!=raw:raise ValueError("conflicting-support-ticket")
                return False
            if self.db.execute("SELECT count(*) FROM tickets WHERE tenant=?",(value["tenantScope"],)).fetchone()[0]>=10000:raise ValueError("support-capacity")
            self.db.execute("INSERT INTO tickets(tenant,id,body,state) VALUES(?,?,?,?)",(value["tenantScope"],value["ticketId"],raw,"open"))
            self.db.execute("INSERT INTO ticket_metadata VALUES(?,?,?)",(value["tenantScope"],value["ticketId"],utc()))
        return True
    def transition(self,value):
        closed(value,"eventId tenantScope ticketId from to evidenceDigests")
        private_identifier(value["tenantScope"]);private_identifier(value["ticketId"]);private_identifier(value["eventId"]);proofs(value["evidenceDigests"])
        raw=checked(value);digest=hashlib.sha256(raw).hexdigest()
        with self.db:
            self.db.execute("BEGIN IMMEDIATE")
            old=self.db.execute("SELECT digest FROM support_events WHERE tenant=? AND id=?",(value["tenantScope"],value["eventId"])).fetchone()
            if old:
                if old[0]!=digest:raise ValueError("conflicting-support-transition")
                return False
            row=self.db.execute("SELECT state FROM tickets WHERE tenant=? AND id=?",(value["tenantScope"],value["ticketId"])).fetchone()
            if not row or row[0]!=value["from"] or value["to"] not in TRANSITIONS.get(row[0],set()):raise ValueError("support-transition-invalid")
            self.db.execute("UPDATE tickets SET state=? WHERE tenant=? AND id=?",(value["to"],value["tenantScope"],value["ticketId"]))
            self.db.execute("INSERT INTO support_events VALUES(?,?,?,?,?,?)",(value["tenantScope"],value["eventId"],value["ticketId"],digest,raw,utc()))
        return True
    def report(self,tenant,min_samples=20):
        private_identifier(tenant)
        if type(min_samples) is not int or not 1<=min_samples<=10000:raise ValueError("sample-bound")
        stored=list(self.db.execute("SELECT body,recorded FROM observations WHERE tenant=? ORDER BY rowid",(tenant,)))
        rows=[strict_json(row[0]) for row in stored]
        rows.sort(key=lambda event:(event['trialId'],event['kind'],event['attempt']))
        final={};first={}
        for event in rows:
            key=(event["trialId"],event["kind"]);final[key]=event
            if event["attempt"]==1:first[key]=event
        metrics={}
        def metric(name,n,d,threshold,maximum=False,strict=False):
            rate=n/d if d else None
            passed=(rate<threshold if strict else rate<=threshold) if maximum and rate is not None else rate>=threshold if rate is not None else False
            metrics[name]={"numerator":n,"denominator":d,"rate":rate,"threshold":threshold,
                           "state":"not-measured" if not d else "fail" if not passed else "insufficient-samples" if d<min_samples else "pass"}
        for kind,name,threshold in (("deployment","firstAttemptSuccess",.9),("rollback","rollbackSuccess",.95),("restore","restoreVerification",1),("planner","plannerSchemaAfterRetry",.99)):
            selected=[e for e in (first if kind=="deployment" else final).values() if e["kind"]==kind]
            metric(name,sum(e["outcome"] in ("success","valid") for e in selected),len(selected),threshold)
        deployments=[e for e in final.values() if e["kind"]=="deployment"]
        metric("deterministicDeployments",sum(e["deterministic"] for e in deployments),len(deployments),.8)
        compatible=[e for e in final.values() if e["kind"]=="compatibility" and e["outcome"] in ("compatible","false-compatible")]
        metric("falseCompatible",sum(e["outcome"]=="false-compatible" for e in compatible),len(compatible),.02,True,True)
        # All historical findings count: a later clear observation cannot erase one.
        safety=[e for e in rows if e["kind"]=="safety"]
        for outcome in ("secret-exposure","uncited-action","arbitrary-shell"):
            metric(outcome,sum(e["outcome"]==outcome for e in safety),len(safety),0,True)
        coverage={}
        for e in final.values():
            if e["kind"] in ("deployment","restore") and e["outcome"]=="success":
                coverage.setdefault((e["applicationId"],e["releaseDigest"],canonical(e["environment"])),set()).add((e["target"],e["kind"]))
        releases={}
        for (app,release,observed),seen in coverage.items():
            for target in ("local","vps"):
                if {(target,"deployment"),(target,"restore")}<=seen:releases.setdefault((app,release),set()).add(target)
        complete={app for (app,release),targets in releases.items() if targets=={"local","vps"}}
        tickets=[dict(row) for row in self.db.execute("SELECT state,count(*) AS count FROM tickets WHERE tenant=? GROUP BY state",(tenant,))]
        names={item['applicationId']:item['name'] for item in strict_json((Path(__file__).resolve().parents[1]/'community/seeds.json').read_bytes())}
        cohort=[]
        recent=[dict(strict_json(row['body']),recordedAt=row['recorded']) for row in stored[-20:]][::-1]
        for app, in self.db.execute('SELECT app FROM curated WHERE tenant=? ORDER BY app',(tenant,)):
            observations=[strict_json(row['body']) for row in stored if strict_json(row['body'])['applicationId']==app]
            latest=observations[-1]['releaseDigest'] if observations else None
            targets={}
            for target in ('local','vps'):
                qualified=any(application==app and release==latest and {(target,'deployment'),(target,'restore')}<=seen for (application,release,observed),seen in coverage.items())
                targets[target]='observed' if qualified else 'not-recorded'
            cohort.append({'applicationId':app,'name':names.get(app,app),'releaseDigest':latest,'targets':targets,'observations':len(observations),'sameReleaseReady':app in complete})
        support=[]
        for row in self.db.execute('SELECT t.id,t.body,t.state,m.created FROM tickets t LEFT JOIN ticket_metadata m ON m.tenant=t.tenant AND m.id=t.id WHERE t.tenant=? ORDER BY t.rowid DESC LIMIT 100',(tenant,)):
            support.append(dict(strict_json(row['body']),state=row['state'],createdAt=row['created']))
        complete={item['applicationId'] for item in cohort if all(state=='observed' for state in item['targets'].values())}
        for item in cohort:item['sameReleaseReady']=item['applicationId'] in complete
        return {"schemaVersion":"1.0.0","basis":"operator-recorded-evidence-references","generatedAt":utc(),"minimumSamples":min_samples,
                "observations":len(rows),"metrics":metrics,"appsWithSameReleaseLocalAndVpsRecovery":sorted(complete),"support":tickets,
                "cohort":cohort,"recentObservations":recent,"supportTickets":support,
                "feedbackPolicy":{"scope":"alpha-support","automaticSharing":False,"requiresExplicitConsent":True},
                "alphaThresholdsMet":len(complete)>=5 and all(m["state"]=="pass" for m in metrics.values()),
                "independentAssurance":False}
    def feedback(self,tenant,ticket_id,consent=False):
        if consent is not True:raise ValueError("explicit-feedback-export-consent-required")
        row=self.db.execute("SELECT body,state FROM tickets WHERE tenant=? AND id=?",(tenant,ticket_id)).fetchone()
        if not row:raise ValueError("support-ticket-not-found")
        value=strict_json(row[0])
        return {key:value[key] for key in ("applicationId","releaseDigest","environment","category","severity","evidenceDigests")}|{"state":row[1],"scope":"owner-approved-file-export-only","sent":False}


def queue_evidence(queue,job_id,tenant):
    private_identifier(tenant)
    if not re.fullmatch(r'job_[A-Za-z0-9_-]{1,96}',job_id):raise ValueError('invalid-job-id')
    queue=Path(queue);documents=[]
    for part in ('requests','results'):
        path=queue/part/(job_id+'.json')
        if path.is_symlink() or path.resolve().parent!=queue.resolve()/part or not path.is_file() or path.stat().st_size>4<<20:raise ValueError('terminal-job-receipt-required')
        documents.append(strict_json(path.read_bytes()))
    job,result=documents;snapshot=result.get('operation') or {};operation=snapshot.get('Operation') or {};identity=operation.get('identity') or {};envelope=job['envelope']
    expected={'jobId':job_id,'caseId':job['caseId'],'tenantScope':tenant,'applicationId':envelope['applicationId'],'manifestDigest':envelope['manifestDigest'],
              'hostId':envelope['hostProfileRef']['recordId'],'envelopeId':envelope['envelopeId'],'envelopeDigest':validate.canonical_digest(envelope),
              'operation':job['plan']['operation'],'planDigest':validate.canonical_digest(job['plan'])}
    if envelope['tenantScope']!=tenant or job['jobId']!=job_id or any(identity.get(key)!=value for key,value in expected.items()):raise ValueError('job-receipt-scope-mismatch')
    return job,result,operation,'sha256:'+hashlib.sha256(canonical(result)).hexdigest()


def configure(ledger,value):
    """Configure one private target; never infer runtime qualification."""
    closed(value,'tenantScope applications environment target')
    checked(value);private_identifier(value['tenantScope']);environment(value['environment'])
    if value['target'] not in ('local','vps'):raise ValueError('invalid-alpha-target')
    apps=value['applications']
    if not isinstance(apps,list) or not 5<=len(apps)<=10 or len(set(apps))!=len(apps):raise ValueError('five-to-ten-curated-apps-required')
    for app in apps:identifier(app)
    payloads={'alpha-cohort.json':{'tenantScope':value['tenantScope'],'applications':apps},
              'alpha-environment.json':value['environment'],'alpha-target.json':{'target':value['target']}}
    directory=ledger.path.parent
    # Validate all existing files before creating anything. Configuration never
    # silently changes the environment attached to observations in this ledger.
    for name,payload in payloads.items():
        path=directory/name
        if path.is_symlink():raise ValueError('configuration-symlink-forbidden')
        if path.exists() and (path.stat().st_size>65536 or strict_json(path.read_bytes())!=payload):raise ValueError('alpha-configuration-conflict')
    ledger.curate(value['tenantScope'],apps)
    for name,payload in payloads.items():
        path=directory/name
        if not path.exists():
            descriptor=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
            with os.fdopen(descriptor,'wb') as output:output.write(canonical(payload))
    return {'state':'configured','applications':len(apps),'target':value['target'],'runtimeQualificationGranted':False}


def private_action(ledger,tenant,request,queue,configuration):
    """Owner-authenticated support and receipt intake; cannot execute a job."""
    checked(request)
    action=request.get('action');configuration=Path(configuration)
    if action=='feedback-export':
        closed(request,'action requestId ticketId consent');private_identifier(request['requestId'])
        return {'state':'export-prepared','feedback':ledger.feedback(tenant,request['ticketId'],request['consent']),'sent':False}
    if action not in ('open-ticket','transition-ticket','import-job'):raise ValueError('unknown-alpha-action')
    fields={'open-ticket':'action requestId jobId category severity','transition-ticket':'action requestId ticketId from to jobId','import-job':'action requestId jobId trialId attempt deterministic interventions'}
    closed(request,fields[action]);private_identifier(request['requestId'])
    job,result,operation,proof=queue_evidence(queue,request['jobId'],tenant)
    observed=environment(strict_json((configuration/'alpha-environment.json').read_bytes()))
    ledger.scope(tenant,job['envelope']['applicationId'])
    if action=='open-ticket':
        identity='ticket_'+hashlib.sha256((tenant+':'+request['requestId']).encode()).hexdigest()[:32]
        value={'schemaVersion':'1.0.0','ticketId':identity,'tenantScope':tenant,'applicationId':job['envelope']['applicationId'],'releaseDigest':job['envelope']['manifestDigest'],
               'environment':observed,'category':request['category'],'severity':request['severity'],'evidenceDigests':[proof]}
        created=ledger.ticket(value)
        return {'state':'ticket-created' if created else 'already-recorded','ticketId':identity}
    if action=='transition-ticket':
        row=ledger.db.execute('SELECT body FROM tickets WHERE tenant=? AND id=?',(tenant,request['ticketId'])).fetchone()
        if not row:raise ValueError('support-ticket-not-found')
        ticket=strict_json(row[0])
        if ticket['applicationId']!=job['envelope']['applicationId'] or ticket['environment']!=observed:raise ValueError('support-evidence-application-or-environment-mismatch')
        snapshot=result.get('operation') or {}
        if request['to'] in ('resolved','closed') and (operation.get('state')!='completed' or result.get('state')!='completed' or snapshot.get('RecoveryRequired') or snapshot.get('PendingIntent')):raise ValueError('successful-bound-evidence-required-to-resolve')
        value={'eventId':request['requestId'],'tenantScope':tenant,'ticketId':request['ticketId'],'from':request['from'],'to':request['to'],'evidenceDigests':[proof]}
        changed=ledger.transition(value)
        return {'state':'ticket-updated' if changed else 'already-recorded','ticketId':request['ticketId']}
    target=strict_json((configuration/'alpha-target.json').read_bytes())
    closed(target,'target')
    mapping={'schemaVersion':'1.0.0','jobs':{request['jobId']:{key:request[key] for key in ('trialId','attempt','deterministic','interventions')}}}
    result=import_queue(ledger,queue,tenant,observed,target['target'],mapping)
    return dict(result,state='observation-recorded' if result['imported'] else 'no-new-observation')


def dashboard(report):
    rows="".join("<tr><td>"+html.escape(name)+"</td><td>"+html.escape(metric["state"])+"</td><td>"+str(metric["numerator"])+"/"+str(metric["denominator"])+"</td></tr>" for name,metric in report["metrics"].items())
    return '<!doctype html><html lang="en"><meta charset="utf-8"><title>Wazi private alpha</title><meta name="viewport" content="width=device-width"><body><h1>Wazi private alpha</h1><p>Operator-recorded observations. Missing evidence is not a pass. This file contains private cohort results.</p><table><caption>Release and environment bound reliability</caption><tr><th>Measure</th><th>State</th><th>Evidence</th></tr>'+rows+'</table><p>Applications with same-release local/VPS deployment and restore evidence: '+str(len(report["appsWithSameReleaseLocalAndVpsRecovery"]))+'</p></body></html>'


def import_queue(ledger,queue,tenant,observed,target,mapping):
    """Import protected local receipts with explicit trial/retry attribution."""
    environment(observed);private_identifier(tenant);closed(mapping,"schemaVersion jobs")
    if mapping["schemaVersion"]!="1.0.0" or target not in ("local","vps") or not isinstance(mapping["jobs"],dict) or len(mapping["jobs"])>1000:raise ValueError("queue-import-policy-invalid")
    queue=Path(queue)
    if queue.is_symlink() or not queue.is_dir():raise ValueError("private-queue-directory-required")
    imported=0
    for job_id,trial in mapping["jobs"].items():
        if not re.fullmatch(r"job_[A-Za-z0-9_-]{1,96}",job_id):raise ValueError("queue-job-id-invalid")
        closed(trial,"trialId attempt deterministic interventions")
        documents=[]
        for part in ("requests","results"):
            path=queue/part/(job_id+".json")
            if path.is_symlink() or path.resolve().parent!=queue.resolve()/part or not path.is_file() or path.stat().st_size>4<<20:raise ValueError("queue-receipt-file-invalid")
            documents.append(strict_json(path.read_bytes()))
        job,result=documents;snapshot=result.get("operation") or {};operation=snapshot.get("Operation") or {}
        identity=operation.get("identity") or {};envelope=job["envelope"]
        if snapshot.get("PendingIntent") or snapshot.get("RecoveryRequired"):raise ValueError("ambiguous-receipt-not-alpha-success")
        bindings={"jobId":job_id,"tenantScope":tenant,"applicationId":envelope["applicationId"],"manifestDigest":envelope["manifestDigest"],"operation":job["plan"]["operation"],
                  "planDigest":validate.canonical_digest(job["plan"]),"caseId":job["caseId"],"hostId":envelope["hostProfileRef"]["recordId"],
                  "envelopeId":envelope["envelopeId"],"envelopeDigest":validate.canonical_digest(envelope)}
        if job["jobId"]!=job_id or envelope["tenantScope"]!=tenant or any(identity.get(key)!=value for key,value in bindings.items()):raise ValueError("queue-receipt-identity-mismatch")
        state=operation.get("state")
        if state not in ("completed","failed") or (state=="completed" and result.get("state")!="completed"):raise ValueError("terminal-receipt-required")
        kind={"install":"deployment","upgrade":"deployment","restore":"restore","rollback":"rollback"}.get(identity["operation"])
        if kind is None:continue
        start=dt.datetime.fromisoformat(operation["createdAt"].replace("Z","+00:00"));finish=dt.datetime.fromisoformat(operation["updatedAt"].replace("Z","+00:00"))
        if start.tzinfo is None or finish.tzinfo is None or finish<start:raise ValueError("receipt-time-order")
        event={"schemaVersion":"1.0.0","eventId":job_id,"trialId":trial["trialId"],"tenantScope":tenant,"applicationId":identity["applicationId"],
               "releaseDigest":identity["manifestDigest"],"environment":observed,"target":target,"kind":kind,"outcome":"success" if state=="completed" else "failure",
               "attempt":trial["attempt"],"deterministic":trial["deterministic"],"interventions":trial["interventions"],
               "elapsedMs":int((finish-start).total_seconds()*1000),"evidenceDigests":["sha256:"+hashlib.sha256(canonical(result)).hexdigest()]}
        if ledger.record(event):imported+=1
    return {"imported":imported,"scope":"local-private-terminal-receipts","safetyOrPlannerEvidenceInferred":False}


def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument("--database",required=True);sub=parser.add_subparsers(dest="action",required=True)
    for name in ("configure","curate","record","ticket","transition"):
        sub.add_parser(name).add_argument("--input",required=True)
    report=sub.add_parser("report");report.add_argument("--tenant",required=True);report.add_argument("--minimum-samples",type=int,default=20);report.add_argument("--html-output")
    export=sub.add_parser("feedback");export.add_argument("--tenant",required=True);export.add_argument("--ticket",required=True);export.add_argument("--consent",action="store_true");export.add_argument("--output",required=True)
    ingest=sub.add_parser("import-queue")
    for name in ("queue","tenant","environment","target","mapping"):ingest.add_argument("--"+name,required=True)
    args=parser.parse_args();ledger=Ledger(args.database)
    try:
        if args.action in ("configure","curate","record","ticket","transition"):
            path=Path(args.input)
            if path.is_symlink() or path.stat().st_size>65536:raise ValueError("bounded-regular-input-required")
            value=strict_json(path.read_bytes())
            if args.action=="configure":result=configure(ledger,value)
            elif args.action=="curate":
                closed(value,"tenantScope applications");ledger.curate(value["tenantScope"],value["applications"]);result={"curated":True}
            else:result={"recorded":getattr(ledger,args.action)(value)}
        elif args.action=="report":
            result=ledger.report(args.tenant,args.minimum_samples)
            if args.html_output:
                with Path(args.html_output).open("x",encoding="utf-8") as output:output.write(dashboard(result))
        elif args.action=="import-queue":
            result=import_queue(ledger,args.queue,args.tenant,strict_json(Path(args.environment).read_bytes()),args.target,strict_json(Path(args.mapping).read_bytes()))
        else:
            result=ledger.feedback(args.tenant,args.ticket,args.consent)
            with Path(args.output).open("xb") as output:output.write(canonical(result))
        print(json.dumps(result,sort_keys=True,indent=2))
    finally:ledger.close()


if __name__=="__main__":main()
