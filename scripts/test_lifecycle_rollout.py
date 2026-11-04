import copy
import datetime as dt
import unittest

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


if __name__=='__main__': unittest.main()
