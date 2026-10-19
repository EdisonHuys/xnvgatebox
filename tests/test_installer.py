"""Deployment transactions use a temporary filesystem and fake systemd only."""
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest
from unittest import mock

SOURCE = Path(__file__).resolve().parents[1] / 'vps/deploy/installer.py'
spec = importlib.util.spec_from_file_location('deployment', SOURCE)
d = importlib.util.module_from_spec(spec)
spec.loader.exec_module(d)


def put(path, value='old'):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(value)


class System:
    def __init__(self, layout):
        self.l = layout
        self.states = {u: {'active': False, 'enabled': False} for u in d.UNITS}
        self.events = []
        self.fail_pool = self.fail_check = False

    def active(self, u): return self.states[u]['active']
    def enabled(self, u): return self.states[u]['enabled']

    def run(self, args, check=True, **kwargs):
        args = list(map(str, args))
        self.events.append(args)
        if args[0] == 'python3':
            if self.fail_pool: raise subprocess.CalledProcessError(1, args)
            put(Path(args[-1]) / 'node_pool.json', '{"nodes":[{"node_id":"new"}]}')
        elif '-check-state' in args:
            if self.fail_check: raise subprocess.CalledProcessError(1, args)
        elif args[0] == 'systemctl' and args[1] != 'daemon-reload':
            action = args[1]
            for u in args[2:]:
                if action in ('start', 'stop'): self.states[u]['active'] = action == 'start'
                if action in ('enable', 'disable'): self.states[u]['enabled'] = action == 'enable'
                if action == 'stop' and u == d.UNITS[0]:
                    put(self.l.work / 'flushed', 'consistent-after-stop')
        return subprocess.CompletedProcess(args, 0)


class Deployment(d.Installer):
    fail_health = False

    def health(self, expected):
        if (self.l.lib / 'VERSION').read_text().strip() != expected:
            raise RuntimeError('wrong_actual_version')
        if self.fail_health: raise RuntimeError('simulated_health_failure')


class ServiceSnapshotTests(unittest.TestCase):
    def snapshot(self, loaded, active=(), enabled=()):
        runner = d.Runner()
        def systemctl(args, check=True, **kwargs):
            unit = args[-1]
            if args[1] == 'show':
                return subprocess.CompletedProcess(args, 0, loaded.get(unit, 'not-found') + '\n')
            if unit not in loaded:
                self.fail('queried state of absent unit: would print No such file or directory')
            status = active if args[1] == 'is-active' else enabled
            return subprocess.CompletedProcess(args, 0 if unit in status else 1)
        with mock.patch.object(runner, 'run', side_effect=systemctl):
            return d.Installer(None, 'snapshot-test-01', runner=runner).service_snapshot()

    def test_fresh_install_skips_state_queries_for_all_missing_units(self):
        self.assertEqual(self.snapshot({}),
                         {u: {'active': False, 'enabled': False} for u in d.UNITS})

    def test_existing_enabled_disabled_and_masked_unit_intent_is_preserved(self):
        loaded = dict(zip(d.UNITS, ('loaded', 'loaded', 'masked')))
        self.assertEqual(self.snapshot(loaded, active=d.UNITS[:1], enabled=d.UNITS[:2]), {
            d.UNITS[0]: {'active': True, 'enabled': True},
            d.UNITS[1]: {'active': False, 'enabled': True},
            d.UNITS[2]: {'active': False, 'enabled': False},
        })

    def test_systemd_failure_aborts_snapshot_instead_of_recording_stopped(self):
        # Exercise Runner.run, including check=True, rather than faking a
        # successful snapshot when systemd cannot be reached.
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'systemctl'
            path.write_text('#!/bin/sh\necho "synthetic systemd failure" >&2\nexit 1\n')
            path.chmod(0o700)
            with mock.patch.dict('os.environ', {'PATH': tmp}):
                with self.assertRaises(subprocess.CalledProcessError):
                    d.Installer(None, 'snapshot-test-01').service_snapshot()


class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.l = d.Layout(self.root / 'root')
        self.stage = self.root / 'stage'
        for name in ('VERSION', 'deploy/xnvgatebox', 'bin/xnvgatebox-manager',
                     'bin/xray', 'scripts/toolbox.py', 'config/settings.json'):
            put(self.stage / name, 'v0.5.0' if name == 'VERSION' else 'new')
        put(self.stage / 'config/settings.json', '{}')
        for u in d.UNITS: put(self.stage / 'deploy' / u, 'new unit')
        self.r = System(self.l)
        self.i = Deployment(self.stage, 'test-install-01', self.l, self.r)

    def existing(self):
        for name in d.OWNED:
            if name in ('var/lib/xnvgatebox', 'usr/local/lib/xnvgatebox', 'etc/xnvgatebox'):
                put(self.l.path(name) / 'marker', 'old-' + name)
            else: put(self.l.path(name), 'old-' + name)
        private = {
            'settings.json': '{"port":9999,"listen_addr":"127.0.0.1"}',
            'password': 'synthetic-password', 'basepath': 'synthetic-base',
            'token': 'synthetic-token', 'state.json': '{"synthetic":"credentials"}',
            'native.json': '{"synthetic":"clients"}', 'sstp_results.json': '{"synthetic":"results"}',
        }
        for n, value in private.items(): put(self.l.work / n, value)
        put(self.l.path('etc/xnvgatebox/pool.json'), '{"synthetic":"custom"}')
        put(self.l.path('etc/xnvgatebox/intelligence.env'), 'SYNTHETIC=private')
        put(self.l.path('var/lib/xnvgatebox/pool/node_pool.json'), '{"nodes":[{"node_id":"old"}]}')
        self.r.states[d.UNITS[0]] = {'active': True, 'enabled': True}
        self.r.states[d.UNITS[1]] = {'active': True, 'enabled': True}
        return private

    def job(self): return json.loads(self.l.job.read_text())

    def test_fresh_install_defaults_and_complete_package(self):
        self.i.install()
        self.assertEqual(json.loads((self.l.work / 'settings.json').read_text())['port'], 8899)
        self.assertEqual((self.l.lib / 'scripts/toolbox.py').read_text(), 'new')
        self.assertEqual((self.l.work / 'bin/xray').read_text(), 'new')
        self.assertTrue(self.r.active(d.UNITS[0]))
        self.assertTrue(self.r.enabled(d.UNITS[1]))
        self.assertEqual(self.job()['status'], 'completed')
        self.assertFalse(self.l.journal.exists())

    def test_upgrade_preserves_all_private_settings_and_consistent_backup(self):
        private = self.existing()
        self.i.install()
        for n, value in private.items(): self.assertEqual((self.l.work / n).read_text(), value)
        self.assertEqual(self.l.path('etc/xnvgatebox/intelligence.env').read_text(), 'SYNTHETIC=private')
        backup = next(self.l.path('var/backups/xnvgatebox').rglob('project.tar.gz'))
        self.assertEqual(backup.stat().st_mode & 0o777, 0o600)
        with tarfile.open(backup) as t:
            self.assertEqual(t.extractfile('var/lib/xnvgatebox/fanout/flushed').read(), b'consistent-after-stop')
            for name in d.OWNED: self.assertIn(name, t.getnames())
        self.assertEqual(self.r.states[d.UNITS[0]], {'active': True, 'enabled': True})

    def test_pool_failure_fresh_does_not_stop_services(self):
        self.r.fail_pool = True
        with self.assertRaises(subprocess.CalledProcessError): self.i.install()
        self.assertFalse(any('stop' in e for e in self.r.events))
        self.assertFalse(self.l.journal.exists())

    def test_pool_failure_reuses_previous_nonempty_pool(self):
        self.existing(); self.r.fail_pool = True
        self.i.install()
        self.assertEqual(json.loads(self.l.path('var/lib/xnvgatebox/pool/node_pool.json').read_text())['nodes'][0]['node_id'], 'old')

    def test_empty_old_pool_is_not_accepted_as_fallback(self):
        self.existing(); self.r.fail_pool = True
        put(self.l.path('var/lib/xnvgatebox/pool/node_pool.json'), '{"nodes":[]}')
        with self.assertRaises(ValueError): self.i.install()
        self.assertFalse(any('stop' in e for e in self.r.events))

    def test_state_rejection_restores_services_without_replacing_program(self):
        self.existing(); self.r.fail_check = True
        before = json.loads(json.dumps(self.r.states))
        with self.assertRaises(subprocess.CalledProcessError): self.i.install()
        self.assertEqual((self.l.lib / 'marker').read_text(), 'old-usr/local/lib/xnvgatebox')
        self.assertEqual(self.r.states, before)
        self.assertTrue(self.job()['rolled_back'])

    def test_health_failure_restores_complete_tree_and_service_intent(self):
        private = self.existing(); self.i.fail_health = True
        before = json.loads(json.dumps(self.r.states))
        with self.assertRaises(RuntimeError): self.i.install()
        self.assertFalse((self.l.lib / 'VERSION').exists())
        self.assertEqual((self.l.lib / 'marker').read_text(), 'old-usr/local/lib/xnvgatebox')
        for n, value in private.items(): self.assertEqual((self.l.work / n).read_text(), value)
        self.assertEqual(self.r.states, before)
        self.assertTrue(self.job()['rolled_back'])
        self.assertFalse(self.l.journal.exists())

    def test_fresh_failure_removes_new_owned_files(self):
        self.i.fail_health = True
        with self.assertRaises(RuntimeError): self.i.install()
        for name in d.OWNED: self.assertFalse(self.l.path(name).exists(), name)
        self.assertFalse(self.r.active(d.UNITS[0]))

    def test_disabled_existing_manager_remains_stopped(self):
        self.existing(); self.r.states[d.UNITS[0]] = {'active': False, 'enabled': False}
        self.i.install()
        self.assertFalse(self.r.active(d.UNITS[0]))
        self.assertFalse(self.r.enabled(d.UNITS[0]))

    def test_failed_recovery_keeps_journal_and_can_retry(self):
        self.existing(); self.i.fail_health = True
        real_restore = self.i.restore_files
        self.i.restore_files = lambda _: (_ for _ in ()).throw(OSError('simulated'))
        with self.assertRaises(RuntimeError): self.i.install()
        self.assertTrue(self.l.journal.exists())
        self.assertEqual(self.job()['phase'], 'recovery_required')
        d.mark(self.l, self.i.id, 'failed', 'failed')
        self.assertEqual(self.job()['phase'], 'recovery_required')
        self.i.restore_files = real_restore
        self.i.recover()
        self.assertFalse(self.l.journal.exists())
        self.assertEqual((self.l.lib / 'marker').read_text(), 'old-usr/local/lib/xnvgatebox')

    def test_interrupted_install_restores_old_snapshot(self):
        self.existing()
        archive = self.root / 'backup.tar.gz'
        record = {'id': self.i.id, 'phase': 'installing', 'before': self.i.service_snapshot(),
                  'existing_paths': list(d.OWNED), 'backup': str(archive)}
        self.i.stop(record['before']); self.i.backup(record); self.i.journal(record)
        put(self.l.work / 'password', 'damaged')
        put(self.l.lib / 'unexpected-new-file', 'new')
        self.i.recover()
        self.assertEqual((self.l.work / 'password').read_text(), 'synthetic-password')
        self.assertFalse((self.l.lib / 'unexpected-new-file').exists())
        self.assertTrue(self.r.active(d.UNITS[0]))

    def test_job_init_allows_new_cli_job_but_old_worker_cannot_overwrite(self):
        d.init_job(self.l, 'first-job-123')
        d.mark(self.l, 'first-job-123', 'complete', 'completed')
        d.init_job(self.l, 'second-job-123')
        d.mark(self.l, 'first-job-123', 'failed', 'failed')
        self.assertEqual(self.job()['id'], 'second-job-123')
        self.assertEqual(self.job()['status'], 'running')
        self.assertEqual(self.l.job.stat().st_mode & 0o777, 0o600)


class ArchiveTests(unittest.TestCase):
    def package(self, root, mutation=None, extra=None):
        files = {n: b'synthetic' for n in ('bin/xnvgatebox-manager', 'bin/xray',
                 'deploy/installer.py', 'deploy/xnvgatebox', 'scripts/toolbox.py', 'config/settings.json')}
        files['VERSION'] = b'v0.5.0\n'
        files['BUILD.json'] = json.dumps({'version': 'v0.5.0', 'source_dirty': False,
                                        'manager_origin': 'built_from_source'}).encode()
        files['package-sha256.json'] = json.dumps({n: hashlib.sha256(v).hexdigest() for n, v in files.items()}).encode()
        if mutation: mutation(files)
        archive = root / 'package.tar.gz'
        with tarfile.open(archive, 'w:gz') as t:
            for n, body in files.items():
                m = tarfile.TarInfo(n); m.size = len(body); m.mode = 0o755
                t.addfile(m, io.BytesIO(body))
            if extra: t.addfile(extra)
        return archive

    def test_complete_package_is_extracted_only_after_validation(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.assertEqual(d.checked_archive(self.package(root), root / 'out', 'v0.5.0'), 'v0.5.0')
            self.assertEqual((root / 'out/bin/xray').read_bytes(), b'synthetic')

    def test_rejects_corruption_unlisted_member_wrong_version_and_unsafe_paths(self):
        for case in ('digest', 'unlisted', 'version', 'traversal', 'absolute', 'symlink', 'duplicate'):
            with self.subTest(case=case), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp); mutate = None; extra = None; expected = 'v0.5.0'
                if case == 'digest': mutate = lambda f: f.update({'bin/xray': b'corrupt'})
                if case == 'unlisted': mutate = lambda f: f.update({'unlisted': b'extra'})
                if case == 'version': expected = 'v0.6.0'
                if case in ('traversal', 'absolute', 'symlink', 'duplicate'):
                    extra = tarfile.TarInfo({'traversal': '../escape', 'absolute': '/escape', 'symlink': 'link', 'duplicate': 'VERSION'}[case])
                    if case == 'symlink': extra.type = tarfile.SYMTYPE; extra.linkname = '/escape'
                with self.assertRaises(ValueError): d.checked_archive(self.package(root, mutate, extra), root / 'out', expected)
                self.assertFalse((root / 'out').exists())


if __name__ == '__main__': unittest.main()
