#!/usr/bin/env python3
"""Exercise only disposable, uniquely named units on the GitHub runner's systemd."""
import importlib.util
import os
from pathlib import Path
import subprocess
import sys
import uuid

ROOT = Path(__file__).resolve().parents[1]


def main():
    assert sys.platform == 'linux' and os.geteuid() == 0
    assert os.environ.get('GITHUB_ACTIONS') == 'true', 'Disposable CI runner only'
    assert Path('/proc/1/comm').read_text().strip() == 'systemd'
    spec = importlib.util.spec_from_file_location('uninstall_systemd', ROOT/'vps/deploy/uninstall.py')
    u = importlib.util.module_from_spec(spec); spec.loader.exec_module(u)
    prefix = 'xnvgatebox-uninstall-check-' + uuid.uuid4().hex
    timer, service, failed = prefix+'.timer', prefix+'.service', prefix+'-failed.service'
    files = {
        timer: '[Unit]\nDescription=Disposable uninstall timer check\n[Timer]\nOnActiveSec=1h\n[Install]\nWantedBy=timers.target\n',
        service: '[Service]\nType=oneshot\nExecStart=/bin/true\n',
        failed: '[Service]\nType=oneshot\nExecStart=/bin/false\n',
    }
    paths = []
    class Recording(u.System):
        def __init__(self): self.events = []
        def run(self, args): self.events.append(args); return super().run(args)
    runner = Recording(); tool = u.Uninstaller(runner=runner)
    try:
        for name, contents in files.items():
            path = Path('/run/systemd/system')/name
            with path.open('x') as f: f.write(contents)
            paths.append(path)
        runner.run(['systemctl','daemon-reload'])
        runner.run(['systemctl','enable','--now',timer])
        assert runner.run(['systemctl','show','--property=ActiveState','--value',timer]).strip() == 'active'
        tool.stop_unit(timer)
        assert not any(a[:2] == ['systemctl','reset-failed'] for a in runner.events)
        assert not (Path('/etc/systemd/system/timers.target.wants')/timer).exists()
        assert not (Path('/run/systemd/system/timers.target.wants')/timer).exists()
        # After the same show query, capture whether a redundant reset fails.
        redundant = subprocess.run(['systemctl','reset-failed',timer],capture_output=True,text=True)
        print('Inactive timer: stopped, disabled, reset skipped; redundant reset exit:', redundant.returncode)
        start = subprocess.run(['systemctl','start',failed],capture_output=True,text=True)
        assert start.returncode != 0
        assert runner.run(['systemctl','show','--property=ActiveState','--value',failed]).strip() == 'failed'
        tool.stop_unit(failed)
        assert runner.run(['systemctl','show','--property=ActiveState','--value',failed]).strip() == 'inactive'
        assert any(a == ['systemctl','reset-failed',failed] for a in runner.events)
        assert all(p.read_text() == files[p.name] for p in paths)
        print('Failed oneshot: reset and final inactive state verified; fixture files unchanged')
    finally:
        for name in files:
            subprocess.run(['systemctl','disable','--now',name],capture_output=True)
            subprocess.run(['systemctl','reset-failed',name],capture_output=True)
        for path in paths: path.unlink()
        runner.run(['systemctl','daemon-reload'])
    assert all(not runner.loaded(name) for name in files)
    assert all(not p.exists() for p in paths)
    print('PASS: real systemd stop/reset and fixture cleanup; no project services used')


if __name__ == '__main__': main()
