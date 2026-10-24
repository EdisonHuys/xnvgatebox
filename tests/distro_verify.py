"""Inspect a disposable installed instance without printing its credentials."""
import hashlib
import http.client
import json
from pathlib import Path
import subprocess
import sys
import time
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
if sys.argv[1] == 'compare-empty':
    assert not data['exits'], 'Missing pool unexpectedly created exits'
if sys.argv[1] == 'bootstrap':
    deadline = time.monotonic()+50
    while True:
        c.request('GET', base+'/api/console', headers={'Cookie':cookie})
        r=c.getresponse(); summary=json.loads(r.read())
        c.request('GET', base+'/api/exits', headers={'Cookie':cookie})
        r=c.getresponse(); data=json.loads(r.read())
        exits=data.get('exits',[])
        if summary['target']==16 and summary['allocated']==16 and len(exits)==16 and all(e['inbounds'] for e in exits): break
        assert time.monotonic()<deadline, 'Pool refresh did not automatically create and bind 16 exits'
        time.sleep(.5)
    assert all(e['status']!='up' for e in exits), 'Synthetic candidates were published as usable'
    # Preserve and test stopped Slot identity while avoiding synthetic retries.
    for e in exits:
        c.request('POST', base+'/api/stop?slot='+str(e['slot']), headers={'Cookie':cookie})
        r=c.getresponse();r.read();assert r.status==200
    record=Path('/tmp/xnvgatebox-ci-state.json');before=json.loads(record.read_text())
    state={n:hashlib.sha256((p/n).read_bytes()).hexdigest() for n in before}
    assert all(state[n]==before[n] for n in before if n!='native.json'), 'Auto allocation changed management identity'
    record.write_text(json.dumps(state))
    identities=[{k:e[k] for k in ['slot','port','socks_user','socks_pass','policy']} for e in exits]
    marker=Path('/tmp/xnvgatebox-ci-auto-exits.json');marker.write_text(json.dumps(identities));marker.chmod(0o600)
    c.close();print('PASS: pool refresh automatically created 16 bound, unverified exits without target submission');sys.exit(0)
if sys.argv[1] == 'compare':
    identities=[{k:e[k] for k in ['slot','port','socks_user','socks_pass','policy']} for e in data['exits']]
    assert identities==json.loads(Path('/tmp/xnvgatebox-ci-auto-exits.json').read_text()), 'Update changed auto Slot identities'
    assert all(e['status']=='stopped' for e in data['exits'])
if sys.argv[1] in ('disable', 'disabled'):
    record = Path('/tmp/xnvgatebox-ci-disabled-state.json')
    if sys.argv[1] == 'disable':
        c.request('POST', base+'/api/console/direct', '{"enabled":false}',
                  {'Cookie': cookie, 'Content-Type': 'application/json'})
        r = c.getresponse(); r.read()
        assert r.status == 200
        c.request('POST',base+'/api/console/target',json.dumps({'count':0,'policy':data['exits'][0]['policy'],'server_name':'www.tesla.com'}),{'Cookie':cookie,'Content-Type':'application/json'})
        r=c.getresponse();r.read();assert r.status==200
    else:
        assert data.get('host_direct') and all(not row['enable'] for row in data['host_direct'])
        assert not data['exits'], 'Explicit zero target was overwritten during update'
        c.request('GET',base+'/api/console',headers={'Cookie':cookie})
        r=c.getresponse();summary=json.loads(r.read());assert summary['target']==0 and summary['target_configured']
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
    assert summary['suggested_target']==16 and not summary['target_configured']
    c.request('GET',base+'/api/console/automation',headers={'Cookie':cookie})
    r=c.getresponse();autom=json.loads(r.read());assert r.status==200
    assert autom['settings']=={'refresh_enabled':True,'refresh_minutes':60,'check_enabled':True,'check_minutes':60}
    c.request('POST',base+'/api/console/automation',json.dumps({'refresh_enabled':True,'refresh_minutes':60,'check_enabled':False,'check_minutes':90}),{'Cookie':cookie,'Content-Type':'application/json'})
    r=c.getresponse();r.read();assert r.status==200
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
if sys.argv[1]=='save' and Path('/mihomo/mihomo').is_file():
    sys.path.insert(0,'/src/tests')
    from mihomo_client import verify_mihomo
    c2=http.client.HTTPConnection('127.0.0.1',settings['port'],timeout=10)
    c2.request('GET',base+'/sub?'+urllib.parse.urlencode({'token':subscription['token'],'host':'127.0.0.1','vpn':'0','third':'0','direct':'1','target':'clash'}))
    response=c2.getresponse();clash_body=response.read();assert response.status==200
    c2.close();verify_mihomo(Path('/mihomo/mihomo'),clash_body,expected)
subprocess.run([str(p/'bin/xray'), 'version'], check=True, stdout=subprocess.DEVNULL)
names = ['basepath', 'password', 'settings.json', 'native.json', 'automation.json']
state = {n: hashlib.sha256((p/n).read_bytes()).hexdigest() for n in names}
record = Path('/tmp/xnvgatebox-ci-state.json')
if sys.argv[1] == 'save':
    record.write_text(json.dumps(state)); record.chmod(0o600)
else:
    assert state == json.loads(record.read_text()), 'Private settings changed during update'
print('PASS: authenticated management, real subscription HTTPS chain, private identity')
