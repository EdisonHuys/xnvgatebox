import json
from datetime import datetime, timezone, timedelta
from pathlib import Path
import sys
import tempfile
import threading
import unittest

ROOT=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'src'))
from vpngate_toolbox.pool_schedule import scheduled_pool, load_automation
from vpngate_toolbox.cli import output_lock
from vpngate_toolbox.common import ToolError


class PoolScheduleTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
        self.dir=Path(self.tmp.name)/'private';self.dir.mkdir()
        self.output=Path(self.tmp.name)/'pool';self.output.mkdir()
        self.now=datetime(2026,10,24,12,tzinfo=timezone.utc);self.calls=0

    def build(self):
        self.calls+=1
        return {'nodes':[{}]}

    def run_pool(self,scheduled=True):
        with output_lock(self.output,wait=True):
            return scheduled_pool(self.dir,self.output,scheduled,self.build,now=lambda:self.now)

    def test_defaults_one_hour_and_completion_gate_survives_restart(self):
        self.assertEqual(load_automation(self.dir),{'refresh_enabled':True,'refresh_minutes':60,'check_enabled':True,'check_minutes':60})
        self.assertIsNotNone(self.run_pool())
        self.now+=timedelta(minutes=59);self.assertIsNone(self.run_pool())
        self.now+=timedelta(minutes=1);self.assertIsNotNone(self.run_pool())
        self.assertEqual(self.calls,2)
        self.assertEqual((self.dir/'refresh-state.json').stat().st_mode&0o777,0o600)

    def test_disabled_or_below_minimum_never_requests_source(self):
        cfg=load_automation(self.dir);cfg['refresh_enabled']=False
        (self.dir/'automation.json').write_text(json.dumps(cfg))
        self.assertIsNone(self.run_pool());self.assertEqual(self.calls,0)
        for bad in (29,0,True,30.5,'60'):
            cfg['refresh_minutes']=bad;(self.dir/'automation.json').write_text(json.dumps(cfg))
            with self.assertRaises(ToolError):self.run_pool()
        self.assertEqual(self.calls,0)

    def test_failure_is_backed_off_and_previous_pool_preserved(self):
        previous=b'{"nodes":[{"old":true}]}'
        (self.output/'node_pool.json').write_bytes(previous)
        def fail():self.calls+=1;raise ToolError('pool_source_unavailable')
        with self.assertRaises(ToolError):scheduled_pool(self.dir,self.output,True,fail,now=lambda:self.now)
        self.assertEqual(json.loads((self.dir/'refresh-state.json').read_text())['status'],'failed')
        self.now+=timedelta(minutes=30);self.assertIsNone(self.run_pool())
        self.assertEqual((self.output/'node_pool.json').read_bytes(),previous)
        self.now+=timedelta(minutes=30);self.assertIsNotNone(self.run_pool())
        self.assertEqual(self.calls,2)

    def test_manual_run_resets_clock_and_can_repair_auxiliary_state(self):
        (self.dir/'refresh-state.json').write_text('{broken')
        with self.assertRaises(ValueError):self.run_pool()
        self.assertIsNotNone(self.run_pool(False))
        self.now+=timedelta(minutes=10);self.assertIsNone(self.run_pool())
        self.assertIsNotNone(self.run_pool(False));self.assertEqual(self.calls,2)

    def test_old_pool_timestamp_is_used_without_an_immediate_upgrade_fetch(self):
        (self.output/'node_pool.json').write_text(json.dumps({'generated_at':(self.now-timedelta(minutes=20)).isoformat()}))
        self.assertIsNone(self.run_pool());self.assertEqual(self.calls,0)

    def test_contending_manager_and_timer_recheck_under_same_pool_lock(self):
        errors=[]
        def task():
            try:self.run_pool()
            except Exception as e:errors.append(type(e).__name__)
        threads=[threading.Thread(target=task) for _ in range(2)]
        for t in threads:t.start()
        for t in threads:t.join()
        self.assertEqual(errors,[]);self.assertEqual(self.calls,1)

if __name__=='__main__':unittest.main()
