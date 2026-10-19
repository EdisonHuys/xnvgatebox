#!/usr/bin/env python3
"""Real kernel cleanup in a new PID, network and private mount namespace only."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]


def command(*args):
    return subprocess.check_output(args, text=True).strip()


def child(host_net, host_mount):
    assert os.geteuid() == 0
    assert os.readlink('/proc/self/ns/net') != host_net
    assert os.readlink('/proc/self/ns/mnt') != host_mount
    assert [v['ifname'] for v in json.loads(command('ip','-j','link'))] == ['lo']
    command('mount','--make-rprivate','/')
    for path in ('/run/netns','/etc/netns'):
        Path(path).mkdir(exist_ok=True)
        command('mount','-t','tmpfs','tmpfs',path)
    spec=importlib.util.spec_from_file_location('uninstall_live',ROOT/'vps/deploy/uninstall.py')
    u=importlib.util.module_from_spec(spec);spec.loader.exec_module(u)
    with tempfile.TemporaryDirectory(prefix='xnvgatebox-uninstall-') as tmp:
        root=Path(tmp)
        def put(name, text='disposable fixture'):
            p=root/name;p.parent.mkdir(parents=True,exist_ok=True);p.write_text(text)
        for unit in u.UNITS: put('etc/systemd/system/'+unit,(ROOT/'vps/deploy'/unit).read_text())
        put(u.WORK+'/netbase','187');put(u.WORK+'/state.json','{"tunnels":[{"slot":1}]}')
        put(u.WORK+'/.connectivity-test/diagnostic.json','{"slot":-1,"network_index":16383}')
        put('usr/local/bin/xnvgatebox');put('usr/local/bin/xnvgatebox-fanout');put('usr/local/lib/xnvgatebox/marker')
        put('etc/x-ui/x-ui.db','protected-panel');put('usr/bin/openvpn','protected-dependency')
        put('var/backups/xnvgatebox/install-old/project.tar.gz')
        put('etc/systemd/system/fanout.service','[Service]\nExecStart=/usr/local/bin/fanout -dir /var/lib/fanout\n')
        put('usr/local/bin/f','WORK_DIR=/var/lib/fanout\nSERVICE=fanout\nBIN=/usr/local/bin/fanout\n')
        put('usr/local/bin/fanout');put(u.LEGACY_WORK+'/state.json','{"tunnels":[{"slot":2}]}')
        class Kernel(u.System):
            def loaded(self, unit): return (root/'etc/systemd/system'/unit).exists()
            def run(self, args):
                if args[0]=='systemctl':
                    if args[1]=='show':
                        if 'FragmentPath' in args[2]: return '/etc/systemd/system/'+args[-1]
                        if 'DropInPaths' in args[2]: return ''
                        return 'inactive'
                    return ''
                return super().run(args)
        runner=Kernel()
        def network(relative,slot,index,base):
            tag='' if relative==u.LEGACY_WORK else u.instance_tag('/'+relative)
            ns='fo'+tag+str(slot);iface='fov'+tag+str(slot);peer='fop'+tag+str(slot)
            address=lambda offset:'10.%d.%d.%d' % (base,(index*4+offset)//256,(index*4+offset)%256)
            command('ip','netns','add',ns)
            command('ip','link','add',iface,'type','veth','peer','name',peer)
            command('ip','link','set',peer,'netns',ns)
            command('ip','addr','add',address(1)+'/30','dev',iface)
            command('ip','-n',ns,'addr','add',address(2)+'/30','dev',peer)
            command('iptables','-A','FORWARD','-i',iface,'-s',address(0)+'/30','-j','REJECT','--reject-with','icmp-admin-prohibited')
            command('iptables','-t','nat','-A','POSTROUTING','-s',address(0)+'/30','-j','MASQUERADE')
            put('etc/netns/'+ns+'/resolv.conf')
            return ns
        own=network(u.WORK,1,64,187)
        network(u.WORK,-1,16383,187);network(u.LEGACY_WORK,2,128,99)
        command('iptables','-A','FORWARD','-s','10.99.0.0/16','-j','ACCEPT')
        command('ip','netns','add','unrelated-namespace')
        command('ip','link','add','keep-test','type','dummy')
        command('ip','addr','add','172.17.0.1/16','dev','keep-test')
        command('iptables','-A','FORWARD','-i','keep-test','-j','ACCEPT')
        command('iptables','-t','nat','-A','POSTROUTING','-s','172.17.0.0/16','-j','MASQUERADE')
        tool=u.Uninstaller(root,runner,purge=True,legacy=True)
        before=(command('ip','-j','link'),command('iptables-save'))
        tool.plan();assert before==(command('ip','-j','link'),command('iptables-save'))
        process=subprocess.Popen(['ip','netns','exec',own,'sleep','60'])
        try:
            # Wait for the child to enter its namespace, without assuming timing.
            import time
            deadline=time.monotonic()+5
            while not command('ip','netns','pids',own):
                assert time.monotonic()<deadline;time.sleep(.01)
            try: tool.execute()
            except u.Refuse: pass
            else: raise AssertionError('live namespace process was not protected')
            assert (root/u.WORK/'state.json').exists()
            assert process.poll() is None
        finally:
            process.terminate();process.wait(timeout=5)
        tool.execute();tool.execute()
        assert [v['name'] for v in json.loads(command('ip','-j','netns','list'))]==['unrelated-namespace']
        links=[v['ifname'] for v in json.loads(command('ip','-j','link'))]
        assert set(links)=={'lo','keep-test'},links
        filter_rules=command('iptables','-S','FORWARD')
        nat_rules=command('iptables','-t','nat','-S','POSTROUTING')
        assert '-i keep-test -j ACCEPT' in filter_rules and '10.99.' not in filter_rules
        assert '172.17.0.0/16' in nat_rules and '10.187.' not in nat_rules and '10.99.' not in nat_rules
        assert (root/'etc/x-ui/x-ui.db').read_text()=='protected-panel'
        assert (root/'usr/bin/openvpn').read_text()=='protected-dependency'
        assert not (root/'var/backups/xnvgatebox').exists()
        assert all(not (root/p).exists() for p in (*u.OWNED,*u.LEGACY_PATHS,'usr/local/bin/f'))
        print(json.dumps({'isolated_linux_uninstall':'passed','owned_namespaces_removed':3,'unrelated_namespace_interface_rules_preserved':True,'live_process_refused':True,'repeat_safe':True}))


if __name__=='__main__':
    if len(sys.argv)==4 and sys.argv[1]=='--isolated-child': child(sys.argv[2],sys.argv[3])
    else:
        if sys.platform!='linux' or os.geteuid()!=0: raise SystemExit('Requires Linux root; always launches isolated namespaces')
        subprocess.run(['unshare','--mount','--net','--pid','--fork','--mount-proc',sys.executable,str(Path(__file__).resolve()),'--isolated-child',os.readlink('/proc/self/ns/net'),os.readlink('/proc/self/ns/mnt')],check=True)
