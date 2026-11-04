"""Bounded scheduler for one operator-configured durable lifecycle rollout.

Only the existing tick function submits jobs. No new approvals, commands,
credential renewal or remote execution interface is introduced.
"""
import argparse
import datetime as dt
import json
from pathlib import Path
import signal
import time

import lifecycle_rollout as rollout
import validate


def advance(database,identifier,policy):
    db=rollout.connection(database)
    try:
        with db:
            db.execute('BEGIN IMMEDIATE')
            row=db.execute('SELECT digest,plan,state FROM rollouts WHERE id=?',(identifier,)).fetchone()
            if row is None:raise ValueError('rollout-not-found')
            plan=json.loads(row[1]);state=json.loads(row[2])
            if validate.canonical_digest(plan)!=row[0]:raise ValueError('persisted-plan-digest-mismatch')
            rollout.checked_plan(plan)
            try:state=rollout.tick(plan,state,rollout.transport(policy))
            except ValueError:state.update(phase='blocked',reason='transport-or-job-policy-rejected')
            db.execute('UPDATE rollouts SET state=? WHERE id=?',(json.dumps(state),identifier))
            db.execute('INSERT INTO events(rollout,recorded,state) VALUES(?,?,?)',(identifier,rollout.timestamp(),json.dumps(state)))
            return state
    finally:db.close()


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for name in ('database','rollout','policy'):parser.add_argument('--'+name,required=True)
    parser.add_argument('--interval-seconds',type=int,default=15)
    parser.add_argument('--max-ticks',type=int,default=10000)
    args=parser.parse_args()
    if not 1<=args.interval_seconds<=300 or not 1<=args.max_ticks<=100000:raise ValueError('scheduler-bound')
    policy_path=Path(args.policy)
    if policy_path.is_symlink() or policy_path.stat().st_size>65536:raise ValueError('bounded-private-policy-required')
    policy=json.loads(policy_path.read_text(encoding='utf-8'))
    stopped=False
    def stop(*unused):
        nonlocal stopped
        stopped=True
    for name in ('SIGINT','SIGTERM'):
        if hasattr(signal,name):signal.signal(getattr(signal,name),stop)
    previous=None
    for _ in range(args.max_ticks):
        if stopped:return
        state=advance(args.database,args.rollout,policy)
        summary={key:state[key] for key in ('phase','wave','target','action','reason') if key in state}
        if summary!=previous:print(json.dumps({'recordedAt':rollout.timestamp(),**summary}),flush=True);previous=summary
        if state['phase'] in ('completed','recovered','blocked'):return
        deadline=time.monotonic()+args.interval_seconds
        while not stopped and time.monotonic()<deadline:time.sleep(min(.25,max(0,deadline-time.monotonic())))
    print(json.dumps({'state':'tick-limit-reached','rolloutContinuesInDatabase':True}),flush=True)


if __name__=='__main__':main()
