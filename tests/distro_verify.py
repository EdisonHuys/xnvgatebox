"""Inspect a disposable installed instance without printing its credentials."""
import hashlib
import http.client
import json
from pathlib import Path
import subprocess
import sys
import urllib.parse

assert Path('/.dockerenv').exists()
p = Path('/var/lib/xnvgatebox/fanout')
settings = json.loads((p/'settings.json').read_text())
base = '/' + (p/'basepath').read_text().strip().strip('/')
password = (p/'password').read_text().strip()
assert len(base) > 8 and len(password) >= 12
c = http.client.HTTPConnection('127.0.0.1', settings['port'], timeout=10)
c.request('POST', base+'/login', urllib.parse.urlencode({'password': password}),
          {'Content-Type': 'application/x-www-form-urlencoded'})
r = c.getresponse(); r.read()
cookie = r.getheader('Set-Cookie', '').split(';')[0]
assert cookie
c.request('GET', base+'/api/exits', headers={'Cookie': cookie})
r = c.getresponse(); data = json.loads(r.read())
assert r.status == 200 and isinstance(data, dict)
c.close()
subprocess.run([str(p/'bin/xray'), 'version'], check=True, stdout=subprocess.DEVNULL)
names = ['basepath', 'password', 'settings.json', 'token']
state = {n: hashlib.sha256((p/n).read_bytes()).hexdigest() for n in names}
record = Path('/tmp/xnvgatebox-ci-state.json')
if sys.argv[1] == 'save':
    record.write_text(json.dumps(state)); record.chmod(0o600)
else:
    assert state == json.loads(record.read_text()), 'Private settings changed during update'
print('PASS: authenticated management, Xray execution, private identity')
