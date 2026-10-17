"""Synthetic regression; none of these tests contact or signal a VPS."""
import base64
import copy
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from contextlib import ExitStack, redirect_stdout
from unittest.mock import patch, Mock
from urllib.parse import urlencode

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))
from vpngate_toolbox import vps_acceptance as a
from vpngate_toolbox.common import ToolError

KEY = "hSDwCYkwp1R0i33ctD73Wg2_Og0mOBr066SpjqqbTmo"


def link(i=1, **overrides):
    q = {"type": "tcp", "security": "reality", "encryption": "none", "sni": "fixture.example.invalid",
         "pbk": KEY, "sid": "0123456789abcdef", "fp": "chrome", "flow": "xtls-rprx-vision", **overrides}
    return f"vless://12345678-1234-4123-8123-123456789ab{i}@8.8.8.8:{30000+i}?" + urlencode(q) + "#fixture-secret-label"


def plan():
    return {"version": 1, "host_ip": "8.8.8.8", "subscription_url": "http://8.8.8.8:8899/fixture-secret/sub?token=fixture-token",
            "subscription_base64": base64.b64encode("\n".join(link(i) for i in range(1, 4)).encode()).decode(),
            "slots": [{"slot": i, "socks_port": 17927+i, "public_port": 30000+i, "expected_exit_ip": f"73.1.1.{i}",
                       "ip_type": "unknown", "socks_user": f"fixture-user-{i}", "socks_pass": f"fixture-pass-{i}"}
                      for i in range(1, 4)]}


class Session:
    def __init__(self, responses):
        self.responses = iter(responses)
        self.closed = False

    def __enter__(self):
        return self

    def get(self, url):
        value = next(self.responses)
        if isinstance(value, Exception):
            raise value
        return value

    def __exit__(self, *_):
        self.closed = True


def passing_factory(profile, binary, timeout):
    ip = f"73.1.1.{profile.port - 30000}".encode()
    return Session([ip]*4 + [b"Example Domain"])


class VpsAcceptanceTests(unittest.TestCase):
    def test_subscription_accepts_configurable_independent_slot_count(self):
        for count in (1, 5, 32):
            with self.subTest(count=count):
                p = plan()
                p["slots"] = [dict(p["slots"][0], slot=i, public_port=30000+i, socks_port=17927+i,
                                   expected_exit_ip=f"73.1.1.{i}") for i in range(1, count+1)]
                profiles = a.parse_subscription(base64.b64encode("\n".join(link().replace(":30001?", f":{30000+i}?") for i in range(1, count+1)).encode()))
                self.assertEqual(len(a.match_profiles(p, profiles)), count)

    def test_unbound_slot_null_inbounds_reports_not_ready(self):
        manager = a.LocalManager.__new__(a.LocalManager)
        manager.opener = object()
        manager.base = "http://127.0.0.1:8899/fixture/"
        view = {"backend": "native", "panel": "", "public_ip": "8.8.8.8",
                "exits": [{"slot": i, "status": "starting", "inbounds": None} for i in range(1, 4)]}
        with patch.object(a, "request_bytes", return_value=json.dumps(view).encode()) as fetch:
            with self.assertRaisesRegex(ToolError, "live_bound_vless_slots_required"):
                manager.plan()
            self.assertEqual(fetch.call_count, 1)

    def test_exact_real_link_fields_and_no_direct_outbound(self):
        profile = a.connection_from_native_link(link())
        config = profile.xray_config(19001)
        out = config["outbounds"][0]
        self.assertEqual(out["settings"]["vnext"][0]["users"][0]["id"], "12345678-1234-4123-8123-123456789ab1")
        self.assertEqual(out["streamSettings"]["realitySettings"]["password"], KEY)
        self.assertEqual(out["streamSettings"]["realitySettings"]["shortId"], "0123456789abcdef")
        self.assertEqual([o["protocol"] for o in config["outbounds"]], ["vless", "blackhole"])
        self.assertNotIn("allowInsecure", json.dumps(config))

    def test_unsupported_or_ambiguous_profiles_are_not_silently_rewritten(self):
        for overrides in [{"security": "none"}, {"type": "ws"}, {"encryption": "auto"}, {"pbk": "invalid"},
                          {"sid": "123"}, {"sid": "gh"}, {"sni": ""}, {"fp": ""}, {"flow": "unsupported"},
                          {"allowInsecure": "1"}]:
            with self.subTest(overrides=overrides), self.assertRaises(ToolError):
                a.connection_from_native_link(link(**overrides))
        with self.assertRaises(ToolError):
            a.connection_from_native_link(link().replace("#", "&type=tcp#", 1))

    def test_subscription_mapping_rejects_wrong_host_port_or_duplicate(self):
        p = plan()
        profiles = a.parse_subscription(p["subscription_base64"].encode())
        self.assertEqual(len(a.match_profiles(p, profiles)), 3)
        for bad in [b"invalid", base64.b64encode((link()+"\n"+link()).encode())]:
            with self.assertRaises(ToolError):
                a.parse_subscription(bad)
        for mutate in [lambda p: p.update(host_ip="1.1.1.1"), lambda p: p["slots"][0].update(public_port=30999),
                       lambda p: p["slots"][0].update(expected_exit_ip="8.8.8.8"),
                       lambda p: p["slots"][0].update(expected_exit_ip="73.1.1.2")]:
            bad = copy.deepcopy(p)
            mutate(bad)
            with self.assertRaises(ToolError):
                a.match_profiles(bad, profiles)

    def test_all_three_live_results_are_required_and_unknown_stays_unknown(self):
        p = plan()
        report = a.verify_plan(p, "unused", "synthetic-test", session_factory=passing_factory, fetch=False)
        self.assertEqual(report["status"], "passed")
        self.assertEqual(len(report["nodes"]), 3)
        self.assertTrue(all(r["ip_type"] == "unknown" and len(r["actual_exit_ips"]) == 4 for r in report["nodes"]))
        for value in ["fixture-token", "fixture-pass", "fixture-user", "12345678", KEY, "fixture-secret"]:
            self.assertNotIn(value, json.dumps(report))
        def mismatch(profile, *_):
            return Session([b"8.8.8.8"])
        report = a.verify_plan(p, "unused", "synthetic-test", session_factory=mismatch, fetch=False)
        self.assertEqual(report["status"], "failed")
        self.assertTrue(all(r["failure_code"] == "exit_ip_mismatch" for r in report["nodes"]))

    def test_content_and_two_independent_https_checks(self):
        session = Session([b"73.1.1.1"]*4 + [b"unexpected page"])
        with self.assertRaisesRegex(ToolError, "content_check_failed"):
            a.verify_session(session, "73.1.1.1", "8.8.8.8")
        with self.assertRaisesRegex(ToolError, "exit_ip_mismatch"):
            a.verify_session(Session([b"73.1.1.1", b"73.1.1.9"]), "73.1.1.1", "8.8.8.8")

    def test_blocking_requires_transport_failure_not_bad_response_or_cert_error(self):
        self.assertEqual(a.probe_blocked(Session([ToolError("curl_failed_97")]*2)), ["curl_failed_97"]*2)
        for response in [b"", b"not an IP", b"8.8.8.8", ToolError("curl_failed_60"), ToolError("xray_start_failed")]:
            with self.subTest(response=type(response).__name__), self.assertRaises(ToolError):
                a.probe_blocked(Session([response]))

    def test_refetched_subscription_must_be_the_same_actual_client_config(self):
        p = plan()
        changed = base64.b64encode("\n".join(link(i, sid="abcd") for i in range(1, 4)).encode())
        with patch.object(a, "request_bytes", return_value=changed), self.assertRaisesRegex(ToolError, "configuration_changed"):
            a.verify_plan(p, "unused", "synthetic-test", session_factory=passing_factory)
        reordered = base64.b64encode("\n".join(link(i) for i in [3, 2, 1]).encode())
        with patch.object(a, "request_bytes", return_value=reordered):
            self.assertEqual(a.verify_plan(p, "unused", "synthetic-test", session_factory=passing_factory)["status"], "passed")

    def test_curl_credentials_use_stdin_and_do_not_inherit_proxy_environment(self):
        row = plan()["slots"][0]
        with patch.dict(os.environ, {"HTTPS_PROXY": "fixture-secret-proxy"}), patch.object(a.subprocess, "run") as run:
            run.return_value = subprocess.CompletedProcess([], 0, stdout=b"73.1.1.1\n200")
            self.assertEqual(a.LocalSocksSession(row).get(a.ECHO_URLS[0]), b"73.1.1.1")
            args, kw = run.call_args
            self.assertNotIn(row["socks_pass"], " ".join(args[0]))
            self.assertIn(row["socks_pass"].encode(), kw["input"])
            self.assertNotIn("HTTPS_PROXY", kw["env"])
            self.assertEqual(args[0][0:2], ["curl", "--disable"])

    def test_process_ownership_checks_and_pidfd_recheck(self):
        cfg = "/fixture/state/fo1.ovpn"
        self.assertTrue(a.owned_openvpn("/usr/sbin/openvpn", [b"openvpn", b"--config", cfg.encode()], cfg, 42, 42))
        for exe, args, inode in [("/usr/sbin/xray", [b"--config", cfg.encode()], 42),
                                  ("/usr/sbin/openvpn", [b"--config", b"/other.ovpn"], 42),
                                  ("/usr/sbin/openvpn", [b"--config"], 42),
                                  ("/usr/sbin/openvpn", [b"--config", cfg.encode()], 43)]:
            self.assertFalse(a.owned_openvpn(exe, args, cfg, inode, 42))
        with patch.object(a.os, "geteuid", return_value=0), patch.object(a.os, "pidfd_open", return_value=123, create=True), \
             patch.object(a.os, "close") as close, patch.object(a.signal, "pidfd_send_signal", create=True) as kill, \
             patch.object(a, "slot_process", side_effect=[111, 222]):
            with self.assertRaisesRegex(ToolError, "process_changed"):
                a.kill_slot_process("/fixture/state", 1)
            kill.assert_not_called()
            close.assert_called_once_with(123)

    def test_fault_requires_baseline_and_checks_recovery_even_when_blocking_fails(self):
        manager = Mock(state_dir=Path("/fixture/state"))
        manager.plan.return_value = plan()
        with patch.object(a, "verify_plan", return_value={"status": "failed"}), patch.object(a, "kill_slot_process") as kill:
            result = a.fault_test(manager, "unused", 1, 30)
            self.assertFalse(result["fault_injected"])
            kill.assert_not_called()
        with ExitStack() as stack:
            verify = stack.enter_context(patch.object(a, "verify_plan", return_value={"status": "passed"}))
            stack.enter_context(patch.object(a, "XraySession", return_value=Session([b"73.1.1.1"]*4+[b"Example Domain"])))
            stack.enter_context(patch.object(a, "LocalSocksSession", return_value=Session([])))
            stack.enter_context(patch.object(a, "kill_slot_process", return_value=111))
            stack.enter_context(patch.object(a, "slot_process", return_value=222))
            stack.enter_context(patch.object(a, "probe_blocked", side_effect=ToolError("request_succeeded_during_fault")))
            report = a.fault_test(manager, "unused", 1, 30)
            self.assertEqual(report["status"], "failed")
            self.assertEqual(report["recovery"]["status"], "passed")
            self.assertTrue(report["fault_injected"])
            self.assertEqual(verify.call_count, 2)

    def test_private_capture_and_error_reports_do_not_print_credentials(self):
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / "plan.json"
            state = Path(tmp) / "state"
            stdout = io.StringIO()
            with patch.object(a, "LocalManager") as manager, redirect_stdout(stdout):
                manager.return_value.plan.return_value = plan()
                self.assertEqual(a.main(["capture-plan", "--state-dir", str(state), "--output", str(output)]), 0)
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)
            self.assertNotIn("fixture-token", stdout.getvalue())
            self.assertIn("fixture-token", output.read_text())
            with patch.object(a, "LocalManager", side_effect=ValueError("fixture-secret-password")), redirect_stdout(stdout):
                self.assertEqual(a.main(["capture-plan", "--state-dir", str(state), "--output", str(output)]), 1)
            self.assertNotIn("fixture-secret", stdout.getvalue() + output.read_text())

    def test_output_cannot_overwrite_active_state_backup_or_private_plan(self):
        with tempfile.TemporaryDirectory() as tmp:
            state = Path(tmp)/"state"
            state.mkdir()
            password = state/"password"
            password.write_text("fixture-preserved-password")
            with redirect_stdout(io.StringIO()), patch.object(a, "LocalManager") as manager:
                self.assertEqual(a.main(["capture-plan", "--state-dir", str(state), "--output", str(password)]), 1)
                manager.assert_not_called()
            self.assertEqual(password.read_text(), "fixture-preserved-password")

    def test_migration_compare_reports_only_preservation_bools(self):
        import shutil
        with tempfile.TemporaryDirectory() as tmp:
            before, after = Path(tmp)/"before", Path(tmp)/"after"
            shutil.copytree(ROOT/"vps/fanout/testdata/migration-v031", before)
            shutil.copytree(before, after)
            state = json.loads((after/"state.json").read_text())
            state["tunnels"][0]["hostname"] = "fixture-new-node"
            (after/"state.json").write_text(json.dumps(state))
            native = json.loads((after/"native.json").read_text())
            native["inbounds"][0]["bound_to"] = "fixture-new-node"
            (after/"native.json").write_text(json.dumps(native))
            self.assertEqual(a.compare_state(before, after)["status"], "passed")
            native["inbounds"][0]["clients"][0]["id"] = "fixture-rotated-id"
            (after/"native.json").write_text(json.dumps(native))
            result = a.compare_state(before, after)
            self.assertFalse(result["checks"]["inbound_clients_and_keys_preserved"])
            self.assertNotIn("fixture-", json.dumps(result))

    def test_pinned_xray_parses_native_client_configuration_without_listening(self):
        binary = ROOT/"tools/bin/xray"
        if not binary.exists():
            self.skipTest("pinned Xray absent")
        with tempfile.TemporaryDirectory() as tmp:
            cfg = Path(tmp)/"client.json"
            cfg.write_text(json.dumps(a.connection_from_native_link(link()).xray_config(19001)))
            result = subprocess.run([str(binary), "run", "-test", "-c", str(cfg)], stdout=subprocess.DEVNULL,
                                    stderr=subprocess.DEVNULL, timeout=10)
            self.assertEqual(result.returncode, 0)


class PackageTests(unittest.TestCase):
    def test_local_archive_still_requires_pinned_checksum(self):
        spec = importlib.util.spec_from_file_location("build_vps_package", ROOT/"scripts/build_vps_package.py")
        build = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(build)
        pin = json.loads((ROOT/"config/xray-release.json").read_text())
        with tempfile.TemporaryDirectory() as tmp:
            archive = Path(tmp)/"bad.zip"
            archive.write_bytes(b"fixture-bad-archive")
            with patch.object(build.urllib.request, "urlopen") as network, self.assertRaisesRegex(RuntimeError, "SHA256 mismatch"):
                build.pinned_xray_archive(pin, archive)
            network.assert_not_called()


if __name__ == "__main__":
    unittest.main()
