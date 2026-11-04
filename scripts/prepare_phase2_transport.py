"""Prepare short-lived artifact download URLs; output is private, never committed."""
import json
import argparse
from pathlib import Path
import subprocess
import sys
import urllib.error
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('output', type=Path)
    parser.add_argument('--run-id', type=int, required=True)
    parser.add_argument('--attempt', type=int, default=1)
    parser.add_argument('--profiles', nargs='+', choices=('postgresql','mysql','nginx','uptime-kuma'), default=['postgresql','mysql','nginx'])
    args = parser.parse_args()
    if args.run_id <= 0 or args.attempt <= 0:
        raise ValueError('positive-build-run-required')
    credential = subprocess.check_output(['gh', 'auth', 'token'], text=True).strip()
    artifacts = json.loads(subprocess.check_output(['gh', 'api', 'repos/Bera-Corp/wazi/actions/runs/'+str(args.run_id)+'/artifacts?per_page=100']))['artifacts']
    opener = urllib.request.build_opener(NoRedirect)
    urls = {}
    for profile in dict.fromkeys(args.profiles):
        artifact, = [item for item in artifacts if item['name'] == 'phase2-'+profile+'-'+str(args.run_id)+'-'+str(args.attempt) and not item.get('expired')]
        request = urllib.request.Request('https://api.github.com/repos/Bera-Corp/wazi/actions/artifacts/'+str(artifact['id'])+'/zip', headers={'Authorization':'Bearer '+credential, 'Accept':'application/vnd.github+json'})
        try:
            opener.open(request, timeout=30)
            raise RuntimeError('expected short-lived download redirect')
        except urllib.error.HTTPError as response:
            if response.code != 302:
                raise RuntimeError('artifact URL unavailable') from None
            url = response.headers['Location']
            if not url.startswith('https://'):
                raise RuntimeError('non-TLS artifact URL rejected')
            urls[profile] = url
    with args.output.open('x',encoding='utf-8') as output:
        json.dump(urls,output)
    print('Prepared '+str(len(urls))+' short-lived archive transports; credentials remain local')


if __name__ == '__main__':
    main()
