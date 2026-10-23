"""Inspect a disposable installed instance without printing its credentials."""
import hashlib
import http.client
import json
from pathlib import Path
import subprocess
import sys
import urllib.parse

sys.path.insert(0, '/usr/local/lib/xnvgatebox/src')
from vpngate_toolbox.vps_acceptance import parse_subscription
from vpngate_toolbox.full_chain import XraySession
from vpngate_toolbox.common import get_bytes

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
if sys.argv[1] == 'save':
    c.request('POST', base+'/api/console/direct', '{"enabled":true}',
              {'Cookie': cookie, 'Content-Type': 'application/json'})
    r = c.getresponse(); r.read()
    assert r.status == 200, 'Host direct creation failed'
c.request('GET', base+'/api/sub', headers={'Cookie': cookie})
r = c.getresponse(); subscription = json.loads(r.read())
assert r.status == 200
c.request('GET', base+'/sub?'+urllib.parse.urlencode({
    'token': subscription['token'], 'host': '127.0.0.1', 'vpn': '0', 'third': '0'}))
r = c.getresponse(); blob = r.read()
assert r.status == 200
profiles = parse_subscription(blob)
assert len(profiles) == 1
c.close()
expected = get_bytes('https://api.ipify.org').strip()
with XraySession(profiles[0], str(p/'bin/xray'), 60) as client:
    assert client.get('https://api.ipify.org').strip() == expected
    assert b'Example Domain' in client.get('https://www.example.com/')
subprocess.run([str(p/'bin/xray'), 'version'], check=True, stdout=subprocess.DEVNULL)
names = ['basepath', 'password', 'settings.json', 'native.json']
state = {n: hashlib.sha256((p/n).read_bytes()).hexdigest() for n in names}
record = Path('/tmp/xnvgatebox-ci-state.json')
if sys.argv[1] == 'save':
    record.write_text(json.dumps(state)); record.chmod(0o600)
else:
    assert state == json.loads(record.read_text()), 'Private settings changed during update'
print('PASS: authenticated management, real subscription HTTPS chain, private identity')
