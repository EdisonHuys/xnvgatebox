"""Behavior tests use synthetic observations; they never claim a real VPN succeeded."""
import base64
import copy
import csv
import io
import json
import os
import socket
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))
from vpngate_toolbox.checker import check_node
from vpngate_toolbox.classifier import classify
from vpngate_toolbox.cli import main, output_lock
from vpngate_toolbox.common import ServiceError, ToolError, digest, expires_at, public_ipv4, utc_now
from vpngate_toolbox.config import load_env, load_settings, mode_a_environment
from vpngate_toolbox.full_chain import XraySession, parse_echo, verify, verify_no_fallback
from vpngate_toolbox.intelligence import enrich
from vpngate_toolbox.pipeline import publish, run_mode_a
from vpngate_toolbox.pool import build_pool
from vpngate_toolbox.subscription import connection_from_link, make_connection
from vpngate_toolbox.vpngate import parse_csv, profile_remotes

SETTINGS = load_settings(ROOT / "config/settings.json")
ENV = {"checker_url": "https://checker.example.com/check", "edge_host": "edge.example.com",
       "uuid": "12345678-1234-4123-8123-123456789abc", "username": "vpn", "password": "test-password",
       "xray_bin": str(ROOT / "tools/bin/xray")}
EXIT = "73.1.1.1"


def csv_data(ip="8.8.8.8", proto="tcp", score=100):
    stream = io.StringIO()
    stream.write("*vpn_servers\n")
    writer = csv.writer(stream)
    writer.writerow(["#HostName", "IP", "Score", "Ping", "CountryShort", "OpenVPN_ConfigData_Base64"])
    profile = f"client\ndev tun\nproto {proto}\nremote {ip} 1194\n<ca>\nremote invalid.example 9\n</ca>\n"
    writer.writerow(["vpn-test", ip, score, "-", "US", base64.b64encode(profile.encode()).decode()])
    stream.write("*\n")
    return stream.getvalue().encode()


def image():
    return {"ip": EXIT, "country_code": "US", "asn": {"asn": "AS7922", "name": "Comcast", "type": "isp"},
            "privacy": {"is_hosting": False}}


def node():
    return parse_csv(csv_data(), SETTINGS)[0][0]


def checked_node():
    value = node()
    candidate = value["protocols"]["sstp"]["candidates"][0]
    response = {"type": "sstp", "success": True, "hostname": candidate["host"], "port": candidate["port"],
                "responseTime": 350, "exit": image(), "password": "upstream-secret"}
    return check_node(value, ENV, SETTINGS, transport=lambda *args, **kwargs: response)


class FakeSession:
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


def passing_factory(*args):
    return FakeSession([EXIT.encode()] * 4 + [b"Example Domain"])


def blocking_factory(*args):
    return FakeSession([ToolError("curl_failed_97"), ToolError("curl_failed_97")])


class WorkerOriginTests(unittest.TestCase):
    def test_one_origin_overrides_legacy_endpoints(self):
        for origin in ["https://custom-a.example.com", "https://custom-b.example.net/"]:
            with self.subTest(origin=origin), patch.dict(os.environ, {
                "WORKER_URL": origin, "VLESS_UUID": ENV["uuid"],
                "CHECKER_URL": "https://old.example.com/check", "EDGETUNNEL_HOST": "old.example.com",
            }, clear=True):
                result = mode_a_environment()
                expected = origin.rstrip("/")
                self.assertEqual(result["checker_url"], expected + "/check")
                self.assertEqual(result["edge_host"], expected.split("://")[1])

    def test_worker_origin_rejects_non_origin_values(self):
        for origin in ["http://example.com", "https://example.com/admin", "https://example.com/?token=x", "https://example.com/#x", "https://example.com:8443"]:
            with self.subTest(origin=origin), patch.dict(os.environ, {"WORKER_URL": origin, "VLESS_UUID": ENV["uuid"]}, clear=True):
                with self.assertRaises(ToolError):
                    mode_a_environment()

    def test_legacy_endpoints_remain_supported(self):
        with patch.dict(os.environ, {"CHECKER_URL": ENV["checker_url"], "EDGETUNNEL_HOST": ENV["edge_host"], "VLESS_UUID": ENV["uuid"]}, clear=True):
            self.assertEqual(mode_a_environment()["edge_host"], ENV["edge_host"])


class SourceTests(unittest.TestCase):
    def test_id_stays_stable_when_ip_and_profile_change(self):
        first = parse_csv(csv_data(), SETTINGS)[0][0]
        second = parse_csv(csv_data(ip="1.1.1.1"), SETTINGS)[0][0]
        self.assertEqual(first["node_id"], second["node_id"])
        self.assertNotEqual(first["protocols"]["openvpn"]["profile_sha256"], second["protocols"]["openvpn"]["profile_sha256"])
        self.assertEqual(second["protocols"]["openvpn"]["status"], "not_tested")
        self.assertIsNone(second["protocols"]["sstp"]["expected_exit_ip"])

    def test_discovery_is_not_truncated_by_probe_budget(self):
        data = csv_data().decode().replace("*\n", "")
        row = data.splitlines()[-1].replace("vpn-test", "vpn-other")
        data += row + "\n*\n"
        nodes, profiles, _ = parse_csv(data.encode(), {**SETTINGS, "candidate_limit": 1})
        self.assertEqual(len(nodes), 2)
        self.assertTrue(all(n["protocols"]["openvpn"]["status"] == "not_tested" for n in nodes))
        self.assertEqual(len(profiles), 1)

    def test_udp_port_is_not_an_sstp_capability(self):
        value = parse_csv(csv_data(proto="udp"), SETTINGS)[0][0]
        self.assertEqual([c["port"] for c in value["protocols"]["sstp"]["candidates"]], [443])
        self.assertEqual(value["protocols"]["sstp"]["status"], "not_tested")

    def test_inline_material_is_not_parsed_as_commands(self):
        self.assertEqual(profile_remotes("proto tcp\nremote 8.8.8.8 443\n<ca>\nremote evil 99\n</ca>"),
                         [{"host": "8.8.8.8", "port": 443, "transport": "tcp"}])

    def test_private_and_malformed_source_rejected(self):
        nodes, _, rejected = parse_csv(csv_data(ip="127.0.0.1"), SETTINGS)
        self.assertEqual(nodes, [])
        self.assertEqual(rejected, 1)
        with self.assertRaises(ToolError):
            parse_csv(b"<html>502</html>", SETTINGS)

    def test_pool_without_cloudflare_keeps_profiles_and_digest(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp)
            fixture = path / "source.csv"
            fixture.write_bytes(csv_data())
            with patch.dict(os.environ, {}, clear=True):
                pool = build_pool(SETTINGS, path / "data", fixture)
            self.assertEqual(len(pool["nodes"]), 1)
            protocol = pool["nodes"][0]["protocols"]["openvpn"]
            self.assertEqual(digest((path / "data" / protocol["profile_ref"]).read_bytes()), protocol["profile_sha256"])
            self.assertEqual((path / "data/node_pool.json.sha256").read_text().strip(), digest((path / "data/node_pool.json").read_bytes()))


class ClassifierTests(unittest.TestCase):
    def test_strict_and_likely_are_separate(self):
        raw = image()
        self.assertEqual(classify(raw, SETTINGS, "test", EXIT)["ip_type"], "likely_residential")
        raw["privacy"]["is_residential"] = True
        self.assertEqual(classify(raw, SETTINGS, "test", EXIT)["ip_type"], "strict_residential")

    def test_missing_hosting_is_not_false(self):
        raw = image()
        raw["privacy"] = {}
        raw["is_datacenter"] = False  # Old Checker synthesized flag is intentionally ignored.
        result = classify(raw, SETTINGS, "test", EXIT)
        self.assertEqual(result["ip_type"], "unknown")
        self.assertIsNone(result["hosting"])
        self.assertIsNone(result["proxy"])

    def test_company_hosting_blocks_consumer_label(self):
        raw = image()
        raw["company"] = {"type": "hosting"}
        self.assertEqual(classify(raw, SETTINGS, "test", EXIT)["ip_type"], "datacenter")
        raw["privacy"]["is_residential"] = True
        self.assertEqual(classify(raw, SETTINGS, "test", EXIT)["ip_type"], "unknown")

    def test_explicit_negative_residential_is_not_likely(self):
        raw = image()
        raw["privacy"]["is_residential"] = False
        self.assertNotEqual(classify(raw, SETTINGS, "test", EXIT)["ip_type"], "likely_residential")

    def test_intelligence_must_describe_queried_ip(self):
        with self.assertRaisesRegex(ToolError, "intelligence_ip_mismatch"):
            classify(image(), SETTINGS, "test", "1.1.1.1")


class CheckerTests(unittest.TestCase):
    def test_checked_node_retains_debt_and_no_credentials(self):
        value = checked_node()
        self.assertEqual(value["protocols"]["sstp"]["status"], "passed")
        self.assertEqual(value["protocols"]["openvpn"]["status"], "not_tested")
        self.assertIn("T1", value["protocols"]["sstp"]["baseline_assurance"])
        self.assertNotIn("upstream-secret", json.dumps(value))

    def test_429_is_a_service_error_not_a_dead_node(self):
        def unavailable(*args, **kwargs):
            raise ServiceError("http_429")
        value = check_node(node(), ENV, SETTINGS, unavailable)
        self.assertEqual(value["protocols"]["sstp"]["status"], "not_tested")
        self.assertEqual(value["protocols"]["sstp"]["service_error"], "http_429")

    def test_embedded_target_429_not_misclassified(self):
        for endpoint, code in [("/api/lookup", "checker_intelligence_service_unavailable"),
                               ("/json", "checker_echo_service_unavailable")]:
            with self.subTest(endpoint=endpoint):
                response = {"type": "sstp", "success": False, "error": "Target " + endpoint + " request failed: HTTP/1.1 429"}
                value = check_node(node(), ENV, SETTINGS, lambda *args, **kwargs: response)
                self.assertEqual(value["protocols"]["sstp"]["service_error"], code)
                self.assertEqual(value["protocols"]["sstp"]["status"], "not_tested")

    def test_wrong_candidate_and_private_exit_rejected(self):
        response = {"type": "sstp", "success": True, "hostname": "other.example", "port": 443, "exit": image()}
        value = check_node(node(), ENV, SETTINGS, lambda *args, **kwargs: response)
        self.assertNotEqual(value["protocols"]["sstp"]["status"], "passed")
        for ip in ("10.0.0.1", "198.51.100.1", "::1", "junk", ""):
            with self.assertRaises(ToolError):
                public_ipv4(ip)


class VerificationTests(unittest.TestCase):
    def setUp(self):
        self.value = checked_node()
        self.connection = make_connection(self.value, ENV, SETTINGS)

    def test_published_and_tested_config_are_identical(self):
        self.assertEqual(connection_from_link(self.connection.link()).sha256, self.connection.sha256)
        self.assertNotIn("global=", self.connection.path)
        self.assertIn("globalproxy=1", self.connection.path)
        self.assertFalse(self.connection.xray_config(12345)["inbounds"][0]["settings"]["udp"])

    def test_credentials_are_encoded_once_for_edge_path(self):
        env = {**ENV, "password": "p@ss&=?"}
        connection = make_connection(self.value, env, SETTINGS)
        self.assertEqual(connection_from_link(connection.link()).path, connection.path)
        self.assertIn("%2540", connection.path)

    def test_two_rounds_and_content_must_all_pass(self):
        result = verify(self.connection, EXIT, SETTINGS, "unused", passing_factory)
        self.assertEqual(result["status"], "passed")
        self.assertEqual(result["actual_exit_ips"], [EXIT] * 4)
        bad = FakeSession([EXIT.encode(), EXIT.encode(), b"1.1.1.1"])
        result = verify(self.connection, EXIT, SETTINGS, "unused", lambda *args: bad)
        self.assertEqual(result["failure_code"], "exit_ip_mismatch")
        self.assertTrue(bad.closed)

    def test_empty_content_does_not_pass(self):
        session = FakeSession([EXIT.encode()] * 4 + [b""])
        result = verify(self.connection, EXIT, SETTINGS, "unused", lambda *args: session)
        self.assertEqual(result["failure_code"], "content_check_failed")

    def test_no_fallback_control_rejects_any_success(self):
        broken = make_connection(self.value, ENV, SETTINGS, invalid=True)
        self.assertEqual(verify_no_fallback(broken, SETTINGS, "unused", blocking_factory)["status"], "passed")
        result = verify_no_fallback(broken, SETTINGS, "unused", passing_factory)
        self.assertEqual(result["failure_code"], "invalid_sstp_direct_fallback_detected")
        result = verify_no_fallback(broken, SETTINGS, "unused", lambda *args: FakeSession([ToolError("curl_unavailable")]))
        self.assertEqual(result["status"], "failed")

    def test_tls_abort_control_does_not_accept_certificate_errors(self):
        broken = make_connection(self.value, ENV, SETTINGS, invalid=True)
        aborted = lambda *args: FakeSession([ToolError("curl_failed_35"), ToolError("curl_failed_35")])
        self.assertEqual(verify_no_fallback(broken, SETTINGS, "unused", aborted)["blocked_requests"], 2)
        self.assertEqual(verify(self.connection, EXIT, SETTINGS, "unused", aborted)["status"], "failed")
        for code in (60, 77):
            factory = lambda *args: FakeSession([ToolError("curl_failed_" + str(code))])
            self.assertEqual(verify_no_fallback(broken, SETTINGS, "unused", factory)["status"], "failed")

    def test_echo_json_and_garbage(self):
        self.assertEqual(parse_echo(b'{"ip":"73.1.1.1"}'), EXIT)
        for response in (b"<html>73.1.1.1</html>", b'{"ip":"127.0.0.1"}', b"::1"):
            with self.assertRaises(ToolError):
                parse_echo(response)

    def test_stale_and_changed_config_never_publish(self):
        self.value["mode_a"] = verify(self.connection, EXIT, SETTINGS, "unused", passing_factory)
        guard = verify_no_fallback(make_connection(self.value, ENV, SETTINGS, invalid=True), SETTINGS, "unused", blocking_factory)
        with tempfile.TemporaryDirectory() as tmp:
            output, runtime = Path(tmp) / "data", Path(tmp) / "runtime"
            self.value["mode_a"]["expires_at"] = "2000-01-01T00:00:00Z"
            self.assertEqual(publish({"nodes": [self.value]}, output, runtime, ENV, SETTINGS, guard), 0)
            self.assertEqual((runtime / "subscription.txt").read_text(), "")
            self.assertEqual(json.loads((output / "cf_manifest.json").read_text())["nodes"], [])
            self.value["mode_a"]["expires_at"] = expires_at(3600)
            with self.assertRaisesRegex(ToolError, "publication_config_changed"):
                publish({"nodes": [self.value]}, output, runtime, {**ENV, "uuid": "different"}, SETTINGS, guard)


class PipelineTests(unittest.TestCase):
    def test_end_to_end_orchestration_and_private_subscription(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            fixture = root / "source.csv"
            fixture.write_bytes(csv_data())
            output, runtime = root / "data", root / "runtime"
            report = run_mode_a(SETTINGS, ENV, output, runtime, fixture,
                                checker=lambda value, *args: checked_node(),
                                verifier=lambda *args: verify(*args, session_factory=passing_factory),
                                guard_verifier=lambda *args: verify_no_fallback(*args, session_factory=blocking_factory),
                                enricher=lambda *args: {"enabled": False})
            self.assertEqual(report["status"], "passed")
            self.assertEqual(report["published"], 1)
            subscription = runtime / "subscription.txt"
            self.assertIn(ENV["uuid"], subscription.read_text())
            self.assertEqual(subscription.stat().st_mode & 0o777, 0o600)
            self.assertNotIn(ENV["uuid"], "".join(path.read_text() for path in output.glob("*.json")))
            manifest = json.loads((output / "cf_manifest.json").read_text())
            self.assertEqual(manifest["kind"], "vpngate_cf_validated_nodes")
            self.assertEqual(manifest["nodes"][0]["expected_exit_ip"], EXIT)
            self.assertNotIn(ENV["password"], (output / "cf_manifest.json").read_text())
            pool = json.loads((output / "node_pool.json").read_text())
            self.assertEqual(pool["nodes"][0]["protocols"]["openvpn"]["status"], "not_tested")
            # An upstream failure clears old working subscriptions rather than republishing them.
            report = run_mode_a(SETTINGS, ENV, output, runtime, fixture,
                                checker=lambda value, *args: {**value, "protocols": {**value["protocols"], "sstp": {"status": "not_tested", "service_error": "http_429"}}},
                                enricher=lambda *args: {"enabled": False})
            self.assertEqual(report["status"], "failed")
            self.assertEqual(subscription.read_text(), "")
            self.assertEqual(json.loads((output / "mode_a_validated.json").read_text())["nodes"], [])

    def test_missing_parameters_clear_prior_publication(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            runtime = root / "runtime"
            runtime.mkdir()
            (runtime / "subscription.txt").write_text("old subscription")
            with patch.dict(os.environ, {}, clear=True):
                status = main(["--config", str(ROOT / "config/settings.json"), "--env-file", str(root / "none"),
                               "mode-a", "--output", str(root / "data"), "--runtime", str(runtime)])
            self.assertEqual(status, 2)
            self.assertEqual((runtime / "subscription.txt").read_text(), "")

    def test_file_lock_refuses_concurrent_build(self):
        with tempfile.TemporaryDirectory() as tmp:
            with output_lock(Path(tmp)):
                with self.assertRaisesRegex(ToolError, "another_build_is_running"):
                    with output_lock(Path(tmp)):
                        self.fail("lock bypass")

    def test_invalid_settings_clear_existing_subscription(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "runtime").mkdir()
            (root / "runtime/subscription.txt").write_text("old")
            (root / "bad.json").write_text("not json")
            status = main(["--config", str(root / "bad.json"), "mode-a", "--output", str(root / "data"), "--runtime", str(root / "runtime")])
            self.assertEqual(status, 2)
            self.assertEqual((root / "runtime/subscription.txt").read_text(), "")
            self.assertEqual(json.loads((root / "data/report.json").read_text())["status"], "configuration_error")

    def test_export_from_actions_evidence_checks_config_and_expiry(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            output, runtime = root / "data", root / "runtime"
            output.mkdir()
            value = checked_node()
            connection = make_connection(value, ENV, SETTINGS)
            value["mode_a"] = verify(connection, EXIT, SETTINGS, "unused", passing_factory)
            guard = verify_no_fallback(make_connection(value, ENV, SETTINGS, invalid=True), SETTINGS, "unused", blocking_factory)
            view_path = output / "mode_a_validated.json"
            view_path.write_text(json.dumps({"nodes": [value], "no_fallback_control": guard}))
            arguments = ["--config", str(ROOT / "config/settings.json"), "--env-file", str(root / "none"),
                         "export-subscription", "--view", str(view_path), "--output", str(output), "--runtime", str(runtime)]
            with patch.dict(os.environ, {"EDGETUNNEL_HOST": ENV["edge_host"], "VLESS_UUID": ENV["uuid"],
                                         "VPN_USERNAME": ENV["username"], "VPN_PASSWORD": ENV["password"]}, clear=True):
                self.assertEqual(main(arguments), 0)
                self.assertEqual((runtime / "subscription.txt").read_text().strip(), connection.link())
                guard["expires_at"] = "2000-01-01T00:00:00Z"
                view_path.write_text(json.dumps({"nodes": [value], "no_fallback_control": guard}))
                self.assertEqual(main(arguments), 3)
                self.assertEqual((runtime / "subscription.txt").read_text(), "")

    def test_intelligence_cache_reuses_ip_and_keeps_headers_private(self):
        calls = []

        def lookup(*args, **kwargs):
            calls.append(kwargs["headers"])
            return image()

        with tempfile.TemporaryDirectory() as tmp, patch.dict(os.environ, {"INTELLIGENCE_URL_TEMPLATE": "https://api.example.com/{ip}", "INTELLIGENCE_TOKEN": "private-key"}):
            cache = Path(tmp) / "cache.json"
            stats = enrich([checked_node(), checked_node()], SETTINGS, cache, lookup)
            self.assertEqual(stats["queries"], 1)
            self.assertEqual(calls, [{"X-API-Key": "private-key"}])
            self.assertNotIn("private-key", cache.read_text())

    def test_env_does_not_execute_shell_and_respects_existing_values(self):
        with tempfile.TemporaryDirectory() as tmp, patch.dict(os.environ, {"EXISTING": "keep"}):
            env_file = Path(tmp) / ".env"
            env_file.write_text("EXISTING=replace\nTEST_VALUE='$(echo not-executed)'\n")
            load_env(env_file)
            self.assertEqual(os.environ["EXISTING"], "keep")
            self.assertEqual(os.environ["TEST_VALUE"], "$(echo not-executed)")


@unittest.skipUnless((ROOT / "tools/bin/xray").is_file(), "Install pinned Xray for real process/config smoke test")
class RealXrayTests(unittest.TestCase):
    def test_configuration_listener_and_process_are_cleaned(self):
        connection = make_connection(checked_node(), ENV, SETTINGS)
        session = XraySession(connection, ENV["xray_bin"], 10)
        with session:
            process = session.process
            directory = Path(session.directory.name)
            port = session.port
            self.assertIsNone(process.poll())
            self.assertEqual((directory / "config.json").stat().st_mode & 0o777, 0o600)
        self.assertIsNotNone(process.poll())
        self.assertFalse(directory.exists())
        with self.assertRaises(OSError):
            socket.create_connection(("127.0.0.1", port), timeout=0.2)

    def test_exception_inside_verification_also_stops_real_process(self):
        session = XraySession(make_connection(checked_node(), ENV, SETTINGS), ENV["xray_bin"], 10)
        with self.assertRaisesRegex(ToolError, "injected_exit_mismatch"):
            with session:
                process = session.process
                directory = Path(session.directory.name)
                raise ToolError("injected_exit_mismatch")
        self.assertIsNotNone(process.poll())
        self.assertFalse(directory.exists())


if __name__ == "__main__":
    unittest.main()
