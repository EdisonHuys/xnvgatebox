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
if sys.argv[1] in ('disable', 'disabled'):
    record = Path('/tmp/xnvgatebox-ci-disabled-state.json')
    if sys.argv[1] == 'disable':
        c.request('POST', base+'/api/console/direct', '{"enabled":false}',
                  {'Cookie': cookie, 'Content-Type': 'application/json'})
        r = c.getresponse(); r.read()
        assert r.status == 200
    else:
        assert data.get('host_direct') and all(not row['enable'] for row in data['host_direct'])
    state = {n: hashlib.sha256((p/n).read_bytes()).hexdigest()
             for n in ['basepath', 'password', 'settings.json', 'native.json']}
    if sys.argv[1] == 'disable':
        record.write_text(json.dumps(state)); record.chmod(0o600)
    else:
        assert state == json.loads(record.read_text()), 'Update overwrote explicit disabled intent'
    c.close()
    print('PASS: explicit disabled direct and private identity retained')
    sys.exit(0)
if sys.argv[1] == 'save':
    assert len(data.get('host_direct') or []) == 1 and data['host_direct'][0]['enable'], 'Fresh install lacks default direct access'
    c.request('GET', base+'/api/console', headers={'Cookie': cookie})
    r = c.getresponse(); summary = json.loads(r.read())
    assert r.status == 200 and summary['candidates'] == 0 and not summary['pool_generated_at']
    c.request('GET', base+'/api/console/source-settings', headers={'Cookie': cookie})
    r = c.getresponse(); source_settings = json.loads(r.read())
    assert r.status == 200 and source_settings['mirror_enabled'] is True
    c.request('POST', base+'/api/console/source-settings',
              '{"mirror_enabled":false,"source_timeout_seconds":2,"mirror_max_age_hours":24}',
              {'Cookie':cookie,'Content-Type':'application/json'})
    r = c.getresponse(); r.read()
    assert r.status == 200
    source_config = json.loads(Path('/etc/xnvgatebox/pool.json').read_text())
    assert source_config['source_urls'] == ['https://127.0.0.1:9/']
    assert source_config['pool_source_timeout_seconds'] == 2
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
