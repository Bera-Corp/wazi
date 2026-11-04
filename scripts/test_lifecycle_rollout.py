import copy
import datetime as dt
import unittest
from unittest import mock

import lifecycle_rollout as rollout


def target(name):
    return {'targetId':name,'forward':[{'kind':'checkpoint','job':{'jobId':'job_'+name+'_backup'}},{'kind':'upgrade','job':{'jobId':'job_'+name+'_upgrade'}},{'kind':'health','job':{'jobId':'job_'+name+'_health'}}], 'recovery':[{'kind':'downgrade','job':{'jobId':'job_'+name+'_restore'}},{'kind':'health','job':{'jobId':'job_'+name+'_recovered'}}]}


def response(job, state, **snapshot):
    return {'jobId':job['jobId'],'state':'completed' if state=='completed' else 'rejected-or-reconciliation-required','operation':{'Operation':{'state':state},**snapshot}}


class RolloutTests(unittest.TestCase):
    def setUp(self):
        self.plan={'automaticRecovery':True,'waves':[{'holdSeconds':60,'targets':[target('canary')]},{'holdSeconds':0,'targets':[target('fleet')]}]}
        self.state={'phase':'forward','wave':0,'target':0,'action':0,'completedTargets':[]}

    def test_wave_hold_prevents_fleet_submission(self):
        calls=[]
        def request(name,method,job):
            calls.append((name,method,job['jobId']))
            return response(job,'completed')
        for _ in range(3): rollout.tick(self.plan,self.state,request)
        self.assertEqual(self.state['phase'],'holding')
        count=len(calls)
        rollout.tick(self.plan,self.state,request)
        self.assertEqual(len(calls),count)
        self.state['until']=(dt.datetime.now(dt.timezone.utc)-dt.timedelta(seconds=1)).isoformat()
        rollout.tick(self.plan,self.state,request)
        self.assertEqual(calls[-1][0],'fleet')

    def test_ambiguous_mutation_never_launches_downgrade(self):
        self.state['action']=1
        rollout.tick(self.plan,self.state,lambda target,method,job:response(job,'running',RecoveryRequired=True))
        self.assertEqual(self.state['phase'],'blocked')
        self.assertNotIn('recoveryTargets',self.state)

    def test_failed_health_recovers_current_then_prior_targets(self):
        self.state.update(wave=1,action=2,completedTargets=[[0,0]])
        rollout.tick(self.plan,self.state,lambda target,method,job:response(job,'failed'))
        self.assertEqual(self.state['recoveryTargets'],[[0,0],[1,0]])
        calls=[]
        def request(name,method,job):
            calls.append(name)
            return response(job,'completed')
        for _ in range(5): rollout.tick(self.plan,self.state,request)
        self.assertEqual(calls,['fleet','fleet','canary','canary'])
        self.assertEqual(self.state['phase'],'recovered')

    def test_crash_after_submit_queries_same_job_before_advancing(self):
        remote={}; posts=[]
        def request(name,method,job):
            if method=='GET': return remote.get(job['jobId'])
            posts.append(job['jobId']);remote[job['jobId']]=response(job,'completed')
        original=copy.deepcopy(self.state)
        rollout.tick(self.plan,self.state,request)
        self.state=original
        rollout.tick(self.plan,self.state,request)
        self.assertEqual(posts,['job_canary_backup'])
        self.assertEqual(self.state['action'],1)

    def test_wrong_job_receipt_blocks_progress(self):
        rollout.tick(self.plan,self.state,lambda target,method,job:response({'jobId':'job_other'},'completed'))
        self.assertEqual(self.state['phase'],'blocked')
        self.assertEqual(self.state['action'],0)

    def test_transport_uses_controlplane_operator_header(self):
        policy={'caFile':'ca','certificateFile':'cert','keyFile':'key','targets':{'canary':'https://localhost:9443'},'credentialEnvironment':'WAZI_ROLLOUT_CREDENTIAL'}
        reply=mock.MagicMock()
        reply.__enter__.return_value.read.return_value=b'{"jobId":"job_check"}'
        opener=mock.Mock()
        opener.open.return_value=reply
        with mock.patch.object(rollout.ssl,'create_default_context'), mock.patch.object(rollout.urllib.request,'build_opener',return_value=opener), mock.patch.dict(rollout.os.environ,{'WAZI_ROLLOUT_CREDENTIAL':'synthetic'}):
            rollout.transport(policy)('canary','GET',{'jobId':'job_check'})
        headers=dict(opener.open.call_args.args[0].header_items())
        self.assertEqual(headers.get('X-wazi-operator-token'),'synthetic')
        self.assertNotIn('Authorization',headers)

    def test_timed_health_observation_waits_without_api_calls(self):
        self.state.update(action=2,targetStartedAt=dt.datetime.now(dt.timezone.utc).isoformat())
        self.plan['waves'][0]['targets'][0]['forward'][2]['observeAfterSeconds']=60
        calls=[]
        rollout.tick(self.plan,self.state,lambda *args:calls.append(args))
        self.assertEqual(calls,[])
        self.assertEqual(self.state['action'],2)

    def test_cached_health_result_before_observation_window_blocks(self):
        self.state.update(action=2,targetStartedAt=(dt.datetime.now(dt.timezone.utc)-dt.timedelta(seconds=120)).isoformat())
        self.plan['waves'][0]['targets'][0]['forward'][2]['observeAfterSeconds']=60
        def request(target,method,job):
            value=response(job,'completed')
            value['operation']['Operation']['updatedAt']=(dt.datetime.now(dt.timezone.utc)-dt.timedelta(seconds=300)).isoformat()
            return value
        rollout.tick(self.plan,self.state,request)
        self.assertEqual(self.state['phase'],'blocked')


if __name__=='__main__': unittest.main()
