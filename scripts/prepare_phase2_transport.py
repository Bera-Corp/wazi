"""Prepare short-lived artifact download URLs; output is private, never committed."""
import json
from pathlib import Path
import subprocess
import sys
import urllib.error
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def main():
    credential = subprocess.check_output(['gh', 'auth', 'token'], text=True).strip()
    artifacts = json.loads(subprocess.check_output(['gh', 'api', 'repos/Bera-Corp/wazi/actions/runs/41191007408/artifacts']))['artifacts']
    opener = urllib.request.build_opener(NoRedirect)
    urls = {}
    for profile in ('postgresql', 'mysql', 'nginx'):
        artifact, = [item for item in artifacts if item['name'] == 'phase2-'+profile+'-41191007408-1']
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
    Path(sys.argv[1]).write_text(json.dumps(urls),encoding='utf-8')
    print('Prepared three short-lived archive transports; credentials remain local')


if __name__ == '__main__':
    main()
