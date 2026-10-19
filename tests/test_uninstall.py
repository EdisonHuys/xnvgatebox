"""Destructive paths run only under a disposable root and modelled systemd/network."""
import contextlib
import fcntl
import importlib.util
import io
import json
from pathlib import Path
import shlex
import subprocess
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('uninstall', ROOT / 'vps/deploy/uninstall.py')
u = importlib.util.module_from_spec(spec); spec.loader.exec_module(u)


def put(p, text='synthetic-private'):
    p.parent.mkdir(parents=True, exist_ok=True); p.write_text(text)


class Model:
    def __init__(self, root):
        self.root, self.events = root, []
        self.links, self.ns = {}, {}
        self.rules = {'filter': [], 'nat': []}
        self.updates = ''; self.busy = ''; self.fail_stop = False
        self.fragment = {}
        self.states = {}
        self.fail_reset = False

    def loaded(self, unit): return (self.root / 'etc/systemd/system' / unit).exists()

    def run(self, a):
        self.events.append(a)
        if a[0] == 'systemctl':
            if a[1] == 'list-units': return self.updates
            if a[1] == 'show':
                if 'FragmentPath' in a[2]: return self.fragment.get(a[-1], '/etc/systemd/system/' + a[-1])
                if 'DropInPaths' in a[2]: return ''
                return self.states.get(a[-1], 'inactive')
            if a[1] == 'disable' and self.fail_stop: raise u.Refuse('synthetic stop failure')
            if a[1] == 'reset-failed':
                if self.fail_reset or self.states.get(a[-1], 'inactive') != 'failed':
                    raise u.Refuse('synthetic reset failure')
                self.states[a[-1]] = 'inactive'
            return ''
        if a[:4] == ['ip', '-j', '-4', 'addr']: return json.dumps(list(self.links.values()))
        if a[:4] == ['ip', '-j', 'netns', 'list']: return json.dumps([{'name':n} for n in self.ns])
        if a[:2] == ['ip', '-n']: return json.dumps(self.ns[a[2]])
        if a[:3] == ['ip', 'netns', 'pids']: return self.busy if a[3] in self.ns else ''
        if a[:3] == ['ip', 'netns', 'delete']: self.ns.pop(a[3]); return ''
        if a[:3] == ['ip', 'link', 'delete']: self.links.pop(a[3]); return ''
        if a[0] == 'iptables':
            rules = self.rules[a[4]]
            if a[5] == '-S': return '\n'.join(shlex.join(v) for v in rules)
            if a[5] == '-D': rules.remove(['-A'] + a[6:]); return ''
        raise AssertionError(a)


class UninstallTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(); self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.r = Model(self.root)
        self.i = u.Uninstaller(self.root, self.r)
        for name in u.OWNED:
            if name in ('etc/xnvgatebox','var/lib/xnvgatebox','usr/local/lib/xnvgatebox','var/lib/xnvgatebox-deploy'):
                put(self.root/name/'marker')
            else: put(self.root/name)
        for unit in u.UNITS: put(self.root/'etc/systemd/system'/unit, (ROOT/'vps/deploy'/unit).read_text())
        put(self.root/u.WORK/'netbase','187')
        put(self.root/u.WORK/'state.json',json.dumps({'tunnels':[{'slot':1}]}))
        put(self.root/'var/backups/xnvgatebox/install-old/project.tar.gz')
        put(self.root/'etc/x-ui/x-ui.db','panel-must-survive')
        put(self.root/'etc/sysctl.conf','net.ipv4.ip_forward=1\n')
        put(self.root/'usr/bin/openvpn','shared-dependency')
        self.r.rules['filter'].append(['-A','FORWARD','-i','docker0','-j','ACCEPT'])
        self.r.rules['nat'].append(['-A','POSTROUTING','-s','172.17.0.0/16','-j','MASQUERADE'])
        self.network(u.WORK,1,64,187)

    def network(self, relative, slot, index, base):
        tag = '' if relative == u.LEGACY_WORK else u.instance_tag('/'+relative)
        ns='fo'+tag+str(slot); iface='fov'+tag+str(slot); peer='fop'+tag+str(slot)
        addr=lambda offset:'10.%d.%d.%d' % (base,(index*4+offset)//256,(index*4+offset)%256)
        self.r.links[iface]={'ifname':iface,'addr_info':[{'family':'inet','local':addr(1)}]}
        self.r.ns[ns]=[{'ifname':peer,'addr_info':[{'family':'inet','local':addr(2)}]}]
        cidr=addr(0)+'/30'
        self.r.rules['filter'].append(['-A','FORWARD','-s',cidr,'-i',iface,'-j','REJECT','--reject-with','icmp-admin-prohibited'])
        self.r.rules['nat'].append(['-A','POSTROUTING','-s',cidr,'-j','MASQUERADE'])
        put(self.root/'etc/netns'/ns/'resolv.conf')
        return ns,iface

    def legacy(self):
        put(self.root/'etc/systemd/system/fanout.service','[Service]\nExecStart=/usr/local/bin/fanout -dir /var/lib/fanout\n')
        put(self.root/'usr/local/bin/f','#!/usr/bin/env bash\nWORK_DIR=/var/lib/fanout\nSERVICE=fanout\nBIN=/usr/local/bin/fanout\n')
        put(self.root/'usr/local/bin/fanout')
        put(self.root/u.LEGACY_WORK/'state.json','{"tunnels":[{"slot":2}]}')
        self.network(u.LEGACY_WORK,2,128,99)
        self.r.rules['filter'].append(['-A','FORWARD','-s','10.99.0.0/16','-j','ACCEPT'])

    def protected(self):
        self.assertEqual((self.root/'etc/x-ui/x-ui.db').read_text(),'panel-must-survive')
        self.assertEqual((self.root/'etc/sysctl.conf').read_text(),'net.ipv4.ip_forward=1\n')
        self.assertEqual((self.root/'usr/bin/openvpn').read_text(),'shared-dependency')
        self.assertIn(['-A','FORWARD','-i','docker0','-j','ACCEPT'],self.r.rules['filter'])
        self.assertIn(['-A','POSTROUTING','-s','172.17.0.0/16','-j','MASQUERADE'],self.r.rules['nat'])

    def test_default_removes_owned_state_keeps_backups_and_legacy(self):
        self.legacy(); self.i.execute()
        for name in u.OWNED: self.assertFalse((self.root/name).exists(), name)
        self.assertTrue((self.root/'var/backups/xnvgatebox/install-old/project.tar.gz').exists())
        self.assertTrue((self.root/u.LEGACY_WORK).exists()); self.assertIn('fo2',self.r.ns)
        self.assertEqual(list(self.r.links),['fov2']); self.protected()
        self.i.execute()  # repeated uninstall is harmless

    def test_purge_and_legacy_remove_verified_old_files_and_diagnostic_networks(self):
        self.legacy(); ns,_=self.network(u.WORK,-3,16000,187)
        put(self.root/u.WORK/'.connectivity-test/diagnostic.json','{"slot":-3,"network_index":16000}')
        self.i.purge=self.i.legacy=True; self.i.execute()
        for name in (*u.LEGACY_PATHS,'usr/local/bin/f','var/backups/xnvgatebox'):
            self.assertFalse((self.root/name).exists(),name)
        self.assertFalse(self.r.ns); self.assertFalse(self.r.links)
        self.assertFalse((self.root/'etc/netns'/ns).exists()); self.protected()

    def test_preview_is_read_only_and_does_not_print_private_state(self):
        output=io.StringIO()
        with mock.patch.object(u,'Uninstaller',return_value=self.i), mock.patch.object(u.sys,'platform','linux'), mock.patch.object(u.os,'geteuid',return_value=0), contextlib.redirect_stdout(output):
            self.assertEqual(u.main(['--dry-run']),0)
        self.assertNotIn('synthetic-private',output.getvalue())
        self.assertFalse((self.root/'run/lock').exists())
        self.assertFalse(any(a[0]=='systemctl' and a[1] in ('disable','stop') for a in self.r.events))
        self.assertTrue((self.root/u.WORK/'state.json').exists())

    def test_stop_failure_or_namespace_process_keeps_configuration(self):
        self.r.fail_stop=True
        with self.assertRaises(u.Refuse): self.i.execute()
        self.r.fail_stop=False; self.r.busy='1234'
        with self.assertRaises(u.Refuse): self.i.execute()
        self.assertTrue((self.root/u.WORK/'state.json').exists()); self.protected()

    def test_inactive_units_skip_reset_even_when_reset_would_fail(self):
        self.r.fail_reset = True
        self.i.execute()
        self.assertFalse(any(a[:2] == ['systemctl','reset-failed'] for a in self.r.events))
        self.assertFalse((self.root/u.WORK).exists()); self.protected()

    def test_failed_unit_is_reset_and_verified_before_deletion(self):
        self.r.states[u.UNITS[0]] = 'failed'
        self.i.execute()
        resets = [a[-1] for a in self.r.events if a[:2] == ['systemctl','reset-failed']]
        self.assertEqual(resets, [u.UNITS[0]])
        self.assertEqual(self.r.states[u.UNITS[0]], 'inactive'); self.protected()

    def test_real_reset_failure_keeps_state_and_purge_backup(self):
        self.r.states[u.UNITS[0]] = 'failed'; self.r.fail_reset = True; self.i.purge = True
        with self.assertRaisesRegex(u.Refuse, u.UNITS[0]): self.i.execute()
        self.assertTrue((self.root/u.WORK/'state.json').exists())
        self.assertTrue((self.root/'var/backups/xnvgatebox/install-old/project.tar.gz').exists())
        self.protected()

    def test_reset_error_requires_proven_inactive_or_unloaded_state(self):
        for loaded, states in (([True, True], ['failed', 'inactive']), ([True, False], ['failed'])):
            r = mock.Mock()
            r.loaded.side_effect = loaded
            values = iter(states)
            def run(a):
                if a[1] == 'show': return next(values)
                if a[1] == 'reset-failed': raise u.Refuse('synthetic reset failure')
                return ''
            r.run.side_effect = run
            u.Uninstaller(self.root, r).stop_unit(u.UNITS[0])
        for state in ('failed', 'active', 'activating', ''):
            r.loaded.side_effect = None; r.loaded.return_value = True
            values = iter(['failed', state])
            with self.assertRaises(u.Refuse): u.Uninstaller(self.root, r).stop_unit(u.UNITS[0])

    def test_reset_success_does_not_hide_failed_or_unknown_final_state(self):
        for state in ('failed','active',''):
            r = mock.Mock(); r.loaded.return_value = True
            values = iter(['failed',state])
            r.run.side_effect = lambda a: next(values) if a[1] == 'show' else ''
            with self.assertRaises(u.Refuse): u.Uninstaller(self.root,r).stop_unit(u.UNITS[0])

    def test_system_subprocess_failure_and_recheck_query_error_are_not_ignored(self):
        r = u.System()
        with mock.patch.object(u.subprocess, 'run', return_value=subprocess.CompletedProcess([],1,'','synthetic-private')):
            with self.assertRaisesRegex(u.Refuse, 'reset-failed') as error:
                r.run(['systemctl','reset-failed',u.UNITS[0]])
            self.assertNotIn('synthetic-private',str(error.exception))
        # Use the actual System adapter, with deterministic systemctl responses.
        sequence = [(0,'loaded'), (0,''), (0,'failed'), (1,''), (1,'')]
        with mock.patch.object(u.subprocess, 'run', side_effect=[subprocess.CompletedProcess([],code,value,'') for code,value in sequence]):
            with self.assertRaisesRegex(u.Refuse,'systemctl show'):
                u.Uninstaller(self.root,r).stop_unit(u.UNITS[0])

    def test_update_transaction_and_deployment_lock_block_before_deletion(self):
        put(self.root/'var/lib/xnvgatebox-deploy/transaction.json','{"phase":"installing"}')
        with self.assertRaises(u.Refuse): self.i.execute()
        (self.root/'var/lib/xnvgatebox-deploy/transaction.json').unlink()
        with self.i.lock('run/lock/xnvgatebox-deploy.lock'):
            with self.assertRaises(u.Refuse): self.i.execute()
        self.r.updates='xnvgatebox-update-12345678.service loaded active running updating'
        with self.assertRaises(u.Refuse): self.i.execute()
        self.assertTrue((self.root/u.WORK/'state.json').exists())

    def test_instance_lock_unrelated_legacy_custom_service_and_fragment_are_protected(self):
        with self.i.lock(u.WORK+'/lock'):
            with self.assertRaises(u.Refuse): self.i.execute()
        put(self.root/'usr/local/bin/f','unrelated tool')
        put(self.root/u.LEGACY_WORK/'unknown','unrelated directory')
        self.i.legacy=True
        with self.assertRaises(u.Refuse): self.i.plan()
        self.i.legacy=False; self.r.fragment[u.UNITS[0]]='/usr/lib/systemd/system/foreign.timer'
        with self.assertRaises(u.Refuse): self.i.plan()
        self.r.fragment.clear()
        put(self.root/'etc/systemd/system/xnvgatebox-fanout.service','ExecStart=/usr/local/bin/something-else')
        with self.assertRaises(u.Refuse): self.i.plan()
        self.protected()

    def test_symlink_mount_missing_metadata_and_network_collision_abort(self):
        p=self.root/'etc/xnvgatebox'; (p/'marker').unlink(); p.rmdir(); p.symlink_to(self.root/'etc/x-ui')
        with self.assertRaises(u.Refuse): self.i.plan()
        p.unlink(); p.mkdir()
        with mock.patch.object(u.os.path,'ismount',side_effect=lambda p:Path(p)==self.root/u.WORK):
            with self.assertRaises(u.Refuse): self.i.plan()
        (self.root/u.WORK/'netbase').unlink()
        with self.assertRaises(u.Refuse): self.i.plan()
        put(self.root/u.WORK/'netbase','187')
        self.r.links['foreign']={'ifname':'foreign','addr_info':[{'family':'inet','local':'10.187.2.1'}]}
        with self.assertRaises(u.Refuse): self.i.plan()
        self.protected()

    def test_orphan_rule_is_removed_without_running_namespace_and_cancellation_safe(self):
        self.r.links.clear(); self.r.ns.clear()
        self.i.execute(); self.protected()
        self.assertEqual(len(self.r.rules['nat']),1)

    def test_absent_install_with_unrelated_f_is_idempotent_and_keeps_that_tool(self):
        self.i.execute(); put(self.root/'usr/local/bin/f','other-owner')
        self.i.execute()
        self.assertEqual((self.root/'usr/local/bin/f').read_text(),'other-owner')
        self.i.legacy=True; self.i.execute()
        self.protected()

    def test_shell_command_forwards_only_explicit_uninstall_flags(self):
        bin_dir=self.root/'stub-bin'; target=self.root/'arguments'
        put(bin_dir/'python3', '#!/bin/sh\nprintf "%s\\n" "$@" > "' + str(target) + '"\n')
        (bin_dir/'python3').chmod(0o700)
        import os
        subprocess.run(['/bin/bash', str(ROOT/'vps/deploy/xnvgatebox'), 'uninstall','--dry-run','--purge','--legacy'],env={**os.environ,'PATH':str(bin_dir)},check=True)
        self.assertEqual(target.read_text().splitlines(),['/usr/local/lib/xnvgatebox/deploy/uninstall.py','--dry-run','--purge','--legacy'])

    def test_legacy_cleanup_keeps_an_unrelated_f_symlink(self):
        self.legacy(); p=self.root/'usr/local/bin/f';p.unlink();p.symlink_to(self.root/'etc/x-ui/x-ui.db')
        self.i.legacy=True;self.i.execute()
        self.assertTrue(p.is_symlink());self.protected()

    def test_legacy_cleanup_keeps_unrelated_binary_f_without_decode_failure(self):
        self.legacy();p=self.root/'usr/local/bin/f';p.write_bytes(b'\x7fELF\xff\x00unrelated-tool')
        self.i.legacy=True;self.i.execute()
        self.assertEqual(p.read_bytes(),b'\x7fELF\xff\x00unrelated-tool');self.protected()

    def test_custom_timer_target_and_lifecycle_hooks_are_rejected_before_stop(self):
        p=self.root/'etc/systemd/system'/u.UNITS[0];original=p.read_text()
        put(p,original.replace('[Timer]','[Timer]\nUnit=x-ui.service'))
        with self.assertRaises(u.Refuse):self.i.plan()
        put(p,original)
        p=self.root/'etc/systemd/system'/u.UNITS[2];put(p,p.read_text()+'\nExecStop=/usr/bin/custom-action\n')
        with self.assertRaises(u.Refuse):self.i.plan()
        self.assertFalse(any(a[0]=='systemctl' and a[1]=='disable' for a in self.r.events))

    def test_cli_refuses_unconfirmed_pipe_honors_cancel_and_allows_explicit_yes(self):
        with mock.patch.object(u,'Uninstaller',return_value=self.i), mock.patch.object(u.sys,'platform','linux'), mock.patch.object(u.os,'geteuid',return_value=0), contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            with mock.patch.object(u.sys.stdin,'isatty',return_value=False):
                self.assertEqual(u.main([]),1)
            with mock.patch.object(u.sys.stdin,'isatty',return_value=True), mock.patch('builtins.input',return_value='no'):
                self.assertEqual(u.main([]),0)
            self.assertTrue((self.root/u.WORK/'state.json').exists())
            self.assertEqual(u.main(['--yes']),0)
            self.assertFalse((self.root/u.WORK).exists())
