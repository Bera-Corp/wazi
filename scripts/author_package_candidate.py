"""Explicit opt-in entry point for the deferred provider provision. Never deploys."""
import argparse
import datetime
import json
from pathlib import Path

from candidate_author import JSONChatProvider, SpendLedger, author_candidate


def checked_setup(value, authorized):
    if value.get('enabled') is not True or not authorized:
        raise ValueError('model-calls-deferred-or-spending-not-authorized')
    pricing=value.get('pricing') or {}
    if pricing.get('verified') is not True or not str(pricing.get('sourceUrl','')).startswith('https://'):
        raise ValueError('verified-provider-pricing-required')
    observed=datetime.datetime.fromisoformat(pricing['verifiedAt'].replace('Z','+00:00'))
    now=datetime.datetime.now(datetime.timezone.utc)
    if observed.tzinfo is None or not datetime.timedelta(0)<=now-observed<=datetime.timedelta(days=7):
        raise ValueError('provider-pricing-observation-stale')
    return value


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for name in ('setup','request','ledger','output','run-id'):
        parser.add_argument('--'+name,required=True)
    parser.add_argument('--authorize-spend',action='store_true',help='Explicitly authorize only the configured run cap')
    args=parser.parse_args()
    setup=checked_setup(json.loads(Path(args.setup).read_text()),args.authorize_spend)
    request=json.loads(Path(args.request).read_text())
    provider=JSONChatProvider(setup['endpoint'],setup['model'],setup['credentialEnvironment'],setup['allowedHost'])
    # Reserve the output before any call. An occupied file is evidence,
    # never silently replace it or spend on a second attempt.
    with Path(args.output).open('x',encoding='utf-8') as output:
        ledger=SpendLedger(args.ledger,args.run_id,setup['capUsd'])
        try:
            result=author_candidate(provider,application_id=request['applicationId'],release_version=request['releaseVersion'],sources=request['sources'],approved_images=request['approvedImages'],reserve_budget=ledger.reserve,pricing=setup['pricing'],max_output_tokens=setup['maxOutputTokens'])
        except Exception:
            json.dump({'state':'candidate-authoring-failed','runId':args.run_id,'reservationMayRemain':True},output)
            raise
        finally:
            ledger.close()
        json.dump(result,output,sort_keys=True,indent=2)
    print('Unsigned candidate written; qualification, review and signing remain required')


if __name__=='__main__':
    main()
