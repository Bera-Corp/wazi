"""Durable canary/migration orchestration over already approved Wazi jobs.

No shell, SQL, signing, approval issuance, or direct Docker access is exposed.
Each action is revalidated by the existing host executor when it is submitted.
"""
import argparse
import datetime as dt
import json
import os
from pathlib import Path
import re
import sqlite3
import ssl
import urllib.error
import urllib.parse
import urllib.request

import validate


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise ValueError('lifecycle-redirect-forbidden')


def timestamp():
    return dt.datetime.now(dt.timezone.utc).isoformat()


def checked_plan(plan):
    if set(plan) != {'rolloutId','waves','automaticRecovery'} or not re.fullmatch(r'rollout_[A-Za-z0-9_-]{1,64}',plan['rolloutId']) or type(plan['automaticRecovery']) is not bool:
        raise ValueError('rollout-shape-invalid')
    if not 1 <= len(plan['waves']) <= 32:
        raise ValueError('wave-count-invalid')
    jobs, targets = set(), set()
    for wave in plan['waves']:
        if set(wave) != {'holdSeconds','targets'} or type(wave['holdSeconds']) is not int or not 0 <= wave['holdSeconds'] <= 86400 or not 1 <= len(wave['targets']) <= 100:
            raise ValueError('wave-shape-invalid')
        for target in wave['targets']:
            if set(target) != {'targetId','tenantScope','hostId','applicationId','fromManifestDigest','toManifestDigest','forward','recovery'} or target['targetId'] in targets:
                raise ValueError('target-shape-invalid')
            targets.add(target['targetId'])
            for name in ('fromManifestDigest','toManifestDigest'):
                if not re.fullmatch(r'sha256:[0-9a-f]{64}',target[name]):
                    raise ValueError('transition-digest-invalid')
            if target['fromManifestDigest'] == target['toManifestDigest']:
                raise ValueError('distinct-releases-required')
            for direction in ('forward','recovery'):
                actions=target[direction]
                if not 1 <= len(actions) <= 32:
                    raise ValueError('action-count-invalid')
                for action in actions:
                    if set(action) != {'kind','job'} or action['kind'] not in ('checkpoint','migration','upgrade','health','quiesce','restore','downgrade'):
                        raise ValueError('action-shape-invalid')
                    job=action['job']; envelope=job['envelope']
                    if job['jobId'] in jobs or not re.fullmatch(r'job_[A-Za-z0-9_-]{1,96}',job['jobId']) or not job.get('approvals'):
                        raise ValueError('unique-preapproved-jobs-required')
                    jobs.add(job['jobId'])
                    if any(envelope.get(key) != target[key] for key in ('tenantScope','hostId') if key in envelope):
                        raise ValueError('job-target-mismatch')
                    if envelope.get('tenantScope') != target['tenantScope'] or envelope['hostProfileRef']['recordId'] != target['hostId'] or envelope['applicationId'] != target['applicationId']:
                        raise ValueError('job-scope-mismatch')
                    operation=job['plan']['operation']
                    allowed={'checkpoint':('backup',),'migration':('upgrade',),'upgrade':('upgrade',),'health':('health-check','upgrade','restore','rollback'),'quiesce':('uninstall','rollback'),'restore':('restore',),'downgrade':('restore','rollback')}
                    if operation not in allowed[action['kind']]:
                        raise ValueError('action-operation-mismatch')
                    expected=target['fromManifestDigest'] if (direction == 'recovery' and action['kind'] != 'quiesce') or action['kind'] == 'checkpoint' else target['toManifestDigest']
                    if envelope['manifestDigest'] != expected:
                        raise ValueError('job-release-mismatch')
                    if action['kind']=='health' and any(step['capability'] not in ('manifest.verify','health.check','compose.ps') for step in job['plan']['steps']):
                        raise ValueError('observation-job-must-be-read-only')
            forward=[a['kind'] for a in target['forward']]
            recovery=[a['kind'] for a in target['recovery']]
            if forward[0]!='checkpoint' or forward[-1]!='health' or 'upgrade' not in forward or recovery[-1]!='health' or not any(kind in recovery for kind in ('restore','downgrade')):
                raise ValueError('checkpoint-upgrade-and-recovery-path-required')
    errors=[]
    validate.check_secrets(plan,'rollout',errors)
    if errors:
        raise ValueError('rollout-secret-policy-rejected')
    return plan


def connection(path):
    if os.name=='posix':
        os.umask(0o077)
    db=sqlite3.connect(path,timeout=30)
    db.execute('PRAGMA busy_timeout=30000')
    db.execute('CREATE TABLE IF NOT EXISTS rollouts (id TEXT PRIMARY KEY, digest TEXT NOT NULL, plan TEXT NOT NULL, state TEXT NOT NULL)')
    db.execute('CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY, rollout TEXT NOT NULL, recorded TEXT NOT NULL, state TEXT NOT NULL)')
    db.commit()
    return db


def transport(policy):
    context=ssl.create_default_context(cafile=policy['caFile'])
    context.load_cert_chain(policy['certificateFile'],policy['keyFile'])
    opener=urllib.request.build_opener(urllib.request.HTTPSHandler(context=context),NoRedirect)
    def request(target, method, job):
        origin=policy['targets'][target]
        parsed=urllib.parse.urlsplit(origin)
        if parsed.scheme!='https' or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment or parsed.path not in ('','/'):
            raise ValueError('private-https-origin-required')
        route='/v1/deployments'+('/'+job['jobId'] if method=='GET' else '')
        data=validate.canonical_bytes(job) if method=='POST' else None
        req=urllib.request.Request(origin.rstrip('/')+route,data=data,method=method)
        req.add_header('Content-Type','application/json')
        req.add_header('X-Wazi-Operator-Token',os.environ[policy['credentialEnvironment']])
        try:
            with opener.open(req,timeout=30) as response:
                raw=response.read((1<<20)+1)
                if len(raw)>1<<20:
                    raise ValueError('job-response-bound')
                return json.loads(raw)
        except urllib.error.HTTPError as error:
            if error.code==404 and method=='GET':
                return None
            raise ValueError('job-api-rejected:'+str(error.code)) from None
    return request


def observe(response):
    snapshot=response.get('operation') or {}
    if snapshot.get('RecoveryRequired') or snapshot.get('PendingIntent'):
        return 'ambiguous'
    operation=snapshot.get('Operation') or {}
    state=operation.get('state')
    if state in ('blocked','reconciliation-required','cancelled'):
        return 'ambiguous'
    if state=='failed':
        return 'failed'
    if response.get('state')=='completed' and state=='completed':
        return 'completed'
    if response.get('state')=='rejected-or-reconciliation-required':
        return 'ambiguous'
    return 'waiting'


def tick(plan,state,request):
    if state['phase'] in ('completed','recovered','blocked'):
        return state
    if state['phase']=='holding':
        if dt.datetime.now(dt.timezone.utc)<dt.datetime.fromisoformat(state['until']):
            return state
        state.update(phase='forward',wave=state['wave']+1,target=0,action=0)
        if state['wave']==len(plan['waves']):
            state['phase']='completed'
            return state
    if state['phase']=='recovery':
        if not state['recoveryTargets']:
            state['phase']='recovered'
            return state
        wi,ti=state['recoveryTargets'][-1]
        target=plan['waves'][wi]['targets'][ti]
        actions=target['recovery']
    else:
        target=plan['waves'][state['wave']]['targets'][state['target']]
        actions=target['forward']
    action=actions[state['action']]; job=action['job']
    response=request(target['targetId'],'GET',job)
    if response is None:
        request(target['targetId'],'POST',job)
        return state
    if response.get('jobId') != job['jobId']:
        state.update(phase='blocked',blockedJob=job['jobId'],reason='job-response-identity-mismatch')
        return state
    outcome=observe(response)
    if outcome=='waiting':
        return state
    if outcome=='ambiguous' or (outcome=='failed' and state['phase']=='recovery'):
        state.update(phase='blocked',blockedJob=job['jobId'])
        return state
    if outcome=='failed':
        if plan['automaticRecovery'] and action['kind']!='checkpoint':
            target_ref=[state['wave'],state['target']]
            state['recoveryTargets']=list(state['completedTargets'])
            if target_ref not in state['recoveryTargets']:
                state['recoveryTargets'].append(target_ref)
            state.update(phase='recovery',action=0)
        else:
            state.update(phase='blocked',blockedJob=job['jobId'])
        return state
    state['action']+=1
    if state['action']<len(actions):
        return state
    state['action']=0
    if state['phase']=='recovery':
        state['recoveryTargets'].pop()
        return state
    state['completedTargets'].append([state['wave'],state['target']])
    state['target']+=1
    if state['target']==len(plan['waves'][state['wave']]['targets']):
        hold=plan['waves'][state['wave']]['holdSeconds']
        state.update(phase='holding',until=(dt.datetime.now(dt.timezone.utc)+dt.timedelta(seconds=hold)).isoformat())
    return state


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--database',required=True)
    parser.add_argument('--rollout',required=True)
    commands=parser.add_subparsers(dest='command',required=True)
    init=commands.add_parser('init');init.add_argument('--plan',required=True)
    step=commands.add_parser('tick');step.add_argument('--policy',required=True)
    commands.add_parser('status')
    args=parser.parse_args()
    db=connection(args.database)
    try:
        with db:
            db.execute('BEGIN IMMEDIATE')
            if args.command=='init':
                raw=Path(args.plan).read_bytes()
                if len(raw)>32<<20:
                    raise ValueError('rollout-plan-bound')
                plan=checked_plan(json.loads(raw))
                if plan['rolloutId']!=args.rollout:
                    raise ValueError('rollout-identity-mismatch')
                state={'phase':'forward','wave':0,'target':0,'action':0,'completedTargets':[]}
                db.execute('INSERT INTO rollouts VALUES(?,?,?,?)',(args.rollout,validate.canonical_digest(plan),json.dumps(plan),json.dumps(state)))
            else:
                row=db.execute('SELECT digest,plan,state FROM rollouts WHERE id=?',(args.rollout,)).fetchone()
                if row is None:
                    raise ValueError('rollout-not-found')
                plan=json.loads(row[1]);state=json.loads(row[2])
                if validate.canonical_digest(plan)!=row[0]:
                    raise ValueError('persisted-plan-digest-mismatch')
                if args.command=='tick':
                    policy=json.loads(Path(args.policy).read_text(encoding='utf-8'))
                    try:
                        state=tick(plan,state,transport(policy))
                    except ValueError:
                        state.update(phase='blocked',reason='transport-or-job-policy-rejected')
                    db.execute('UPDATE rollouts SET state=? WHERE id=?',(json.dumps(state),args.rollout))
            if args.command!='status':
                db.execute('INSERT INTO events(rollout,recorded,state) VALUES(?,?,?)',(args.rollout,timestamp(),json.dumps(state)))
            print(json.dumps(state,indent=2))
    finally:
        db.close()


if __name__=='__main__':
    main()
