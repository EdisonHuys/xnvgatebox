"""Disposable core verification using the actual generated Clash subscription."""
from contextlib import contextmanager
import json
from pathlib import Path
import socket
import subprocess
import tempfile
import time


@contextmanager
def client(binary,profile):
    with tempfile.TemporaryDirectory(prefix='xnv-mihomo-') as tmp:
        root=Path(tmp)
        with socket.socket() as listener:
            listener.bind(('127.0.0.1',0));port=listener.getsockname()[1]
        config=json.loads(json.dumps(profile))
        config.update({'mixed-port':port,'bind-address':'127.0.0.1','allow-lan':False,'log-level':'error'})
        path=root/'profile.yaml';path.write_text(json.dumps(config));path.chmod(0o600)
        check=subprocess.run([str(binary),'-t','-d',tmp,'-f',str(path)],capture_output=True,timeout=20)
        assert check.returncode==0,'Mihomo rejected generated subscription'
        with (root/'client.log').open('wb') as log:
            proc=subprocess.Popen([str(binary),'-d',tmp,'-f',str(path)],stdout=log,stderr=log)
            try:
                deadline=time.monotonic()+10
                while True:
                    assert proc.poll() is None,'Mihomo exited before listening'
                    try:
                        with socket.create_connection(('127.0.0.1',port),timeout=.2):break
                    except OSError:
                        assert time.monotonic()<deadline,'Mihomo did not listen'
                        time.sleep(.05)
                yield port
            finally:
                proc.terminate()
                try:proc.wait(timeout=5)
                except subprocess.TimeoutExpired:proc.kill();proc.wait()


def request(port,url):
    return subprocess.run(['curl','-q','--noproxy','','--proxy','socks5h://127.0.0.1:'+str(port),'--connect-timeout','5','--max-time','15','-fsS',url],capture_output=True,timeout=20)


def verify_mihomo(binary,body,expected):
    profile=json.loads(body)
    assert len(profile['proxies'])==1
    assert profile['proxy-groups'][0]['type']=='select'
    assert 'DIRECT' not in profile['proxy-groups'][0]['proxies']
    assert profile['proxies'][0].get('skip-cert-verify') is False
    with client(binary,profile) as port:
        reply=request(port,'https://api.ipify.org')
        assert reply.returncode==0 and reply.stdout.strip()==expected,'Mihomo REALITY egress mismatch'
        reply=request(port,'https://www.example.com/')
        assert reply.returncode==0 and b'Example Domain' in reply.stdout,'Mihomo HTTPS content failed'
    # Leave this local port bound but not listening: the selected server fails,
    # while the host's direct control remains available. No fallback is allowed.
    with socket.socket() as blocked:
        blocked.bind(('127.0.0.1',0))
        profile['proxies'][0]['server']='127.0.0.1';profile['proxies'][0]['port']=blocked.getsockname()[1]
        with client(binary,profile) as port:
            assert request(port,'https://www.example.com/').returncode!=0,'Clash profile fell back to DIRECT'
    print('PASS: actual Clash subscription accepted by pinned Mihomo, REALITY HTTPS egress/content and no DIRECT fallback')
