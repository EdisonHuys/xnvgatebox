"""Native VPS acceptance: real subscriptions, exact exit checks and bounded faults.

Private plans hold access data. Public evidence never contains profiles, tokens,
UUIDs, passwords or provider responses. No Cloudflare or 3x-ui operations.
"""
from __future__ import annotations

import argparse
import base64
import concurrent.futures
import http.cookiejar
import json
import os
import re
import signal
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from dataclasses import dataclass, field
from pathlib import Path

from .common import ToolError, https_url, public_ipv4, utc_now, write_json
from .full_chain import XraySession, child_environment, parse_echo

ECHO_URLS = ("https://api.ipify.org", "https://ipv4.icanhazip.com")
CONTENT_URL = "https://www.example.com/"
TRANSPORT_FAILURES = {"curl_failed_7", "curl_failed_28", "curl_failed_35", "curl_failed_52",
                      "curl_failed_55", "curl_failed_56", "curl_failed_97"}


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise ToolError("redirect_rejected")


def request_bytes(opener, url, data=None):
    try:
        with opener.open(urllib.request.Request(url, data=data), timeout=15) as response:
            blob = response.read(1_000_001)
            if response.status != 200 or len(blob) > 1_000_000:
                raise ToolError("http_response_rejected")
            return blob
    except (urllib.error.URLError, TimeoutError, OSError, ValueError):
        raise ToolError("http_request_failed") from None


@dataclass(repr=False)
class NativeConnection:
    host: str
    port: int
    client_id: str = field(repr=False)
    sni: str = field(repr=False)
    public_key: str = field(repr=False)
    short_id: str = field(repr=False)
    fingerprint: str
    flow: str

    def xray_config(self, socks_port):
        return {"log": {"loglevel": "none"},
                "inbounds": [{"tag": "acceptance", "listen": "127.0.0.1", "port": socks_port,
                              "protocol": "socks", "settings": {"auth": "noauth", "udp": False}}],
                "outbounds": [{"tag": "slot", "protocol": "vless",
                               "settings": {"vnext": [{"address": self.host, "port": self.port,
                                                        "users": [{"id": self.client_id, "encryption": "none", "flow": self.flow}]}]},
                               "streamSettings": {"network": "tcp", "security": "reality",
                                                  "realitySettings": {"serverName": self.sni, "password": self.public_key,
                                                                      "shortId": self.short_id, "fingerprint": self.fingerprint}}},
                              {"tag": "block", "protocol": "blackhole"}],
                "routing": {"rules": [{"type": "field", "network": "udp", "outboundTag": "block"},
                                      {"type": "field", "ip": ["::/0"], "outboundTag": "block"},
                                      {"type": "field", "inboundTag": ["acceptance"], "outboundTag": "slot"}]}}


def connection_from_native_link(link):
    try:
        p = urllib.parse.urlsplit(link)
        q = urllib.parse.parse_qs(p.query, keep_blank_values=True)
        allowed = {"type", "security", "encryption", "sni", "pbk", "sid", "fp", "flow"}
        if p.scheme != "vless" or not p.hostname or not p.port or p.password is not None or set(q) - allowed:
            raise ValueError()
        if any(len(v) != 1 for v in q.values()):
            raise ValueError()
        q = {k: v[0] for k, v in q.items()}
        if any(q.get(k) != v for k, v in {"type": "tcp", "security": "reality", "encryption": "none"}.items()):
            raise ValueError()
        client = str(uuid.UUID(urllib.parse.unquote(p.username or "")))
        key = q.get("pbk", "")
        if not re.fullmatch(r"[A-Za-z0-9_-]{43}", key) or len(base64.urlsafe_b64decode(key + "=")) != 32:
            raise ValueError()
        sid, sni = q.get("sid", ""), q.get("sni", "")
        if len(sid) > 16 or len(sid) % 2 or not re.fullmatch(r"[0-9a-fA-F]*", sid):
            raise ValueError()
        if not sni or not re.fullmatch(r"[A-Za-z0-9.-]+", sni) or q.get("flow", "") not in ("", "xtls-rprx-vision"):
            raise ValueError()
        fp = q.get("fp", "")
        if fp not in {"chrome", "firefox", "safari", "ios", "android", "edge", "360", "qq", "random", "randomized"}:
            raise ValueError()
        return NativeConnection(p.hostname, p.port, client, sni, key, sid, fp, q.get("flow", ""))
    except (ValueError, TypeError, AttributeError):
        raise ToolError("unsupported_native_subscription_profile") from None


def parse_subscription(blob):
    try:
        # The service defaults to base64; do not substitute a hand-built profile.
        text = base64.b64decode(blob.strip(), validate=True).decode("utf-8")
        profiles = [connection_from_native_link(line.strip()) for line in text.splitlines() if line.strip()]
        if not profiles or len(profiles) > 32 or len({(p.host, p.port) for p in profiles}) != len(profiles):
            raise ValueError()
        return profiles
    except (ValueError, UnicodeError):
        raise ToolError("invalid_subscription_body") from None


class LocalManager:
    def __init__(self, state_dir):
        self.state_dir = Path(state_dir).resolve()
        settings = json.loads((self.state_dir / "settings.json").read_text())
        base = (self.state_dir / "basepath").read_text().strip().strip("/")
        addr = settings.get("listen_addr") or "127.0.0.1"
        if addr in {"0.0.0.0", "all"}:
            addr = "127.0.0.1"
        # Credentials may only be sent over this explicit loopback connection.
        if addr not in {"127.0.0.1", "::1"}:
            raise ToolError("loopback_manager_required")
        host = "[::1]" if addr == "::1" else addr
        self.base = f"http://{host}:{int(settings['port'])}/" + (base + "/" if base else "")
        self.token = settings["sub_token"]
        if not isinstance(self.token, str) or not self.token.strip():
            raise ToolError("existing_subscription_token_required")
        jar = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}),
                                                  urllib.request.HTTPCookieProcessor(jar), NoRedirect())
        password = (self.state_dir / "password").read_text().strip()
        if not password:
            raise ToolError("existing_management_password_required")
        data = urllib.parse.urlencode({"password": password}).encode()
        request_bytes(self.opener, self.base + "login", data)

    def plan(self):
        view = json.loads(request_bytes(self.opener, self.base + "api/exits"))
        if view.get("backend") != "native" or view.get("panel") or not 1 <= len(view.get("exits", [])) <= 32:
            raise ToolError("native_slots_required")
        host = public_ipv4(view["public_ip"])
        rows = []
        for e in sorted(view["exits"], key=lambda e: e["slot"]):
            inbounds = [i for i in (e.get("inbounds") or []) if i.get("enable")]
            if e.get("status") != "up" or len(inbounds) != 1 or inbounds[0].get("protocol") != "vless":
                raise ToolError("live_bound_vless_slots_required")
            expected = public_ipv4(e["exit_ip"])
            if expected == host:
                raise ToolError("host_exit_detected")
            rows.append({"slot": e["slot"], "socks_port": e["port"], "public_port": inbounds[0]["port"],
                         "expected_exit_ip": expected, "ip_type": (e.get("egress") or {}).get("ip_type", "unknown"),
                         "socks_user": e["socks_user"], "socks_pass": e["socks_pass"]})
        if len({r["slot"] for r in rows}) != len(rows) or len({r["expected_exit_ip"] for r in rows}) != len(rows):
            raise ToolError("independent_slot_exits_required")
        path = urllib.parse.urlsplit(self.base).path.rstrip("/")
        url = f"http://{host}:{urllib.parse.urlsplit(self.base).port}{path}/sub?" + urllib.parse.urlencode({"token": self.token})
        captured = request_bytes(self.opener, self.base + "sub?" + urllib.parse.urlencode({"token": self.token}))
        plan = {"version": 1, "captured_at": utc_now(), "host_ip": host, "subscription_url": url,
                "subscription_base64": captured.decode("ascii"), "slots": rows}
        match_profiles(plan, parse_subscription(captured))
        return plan


def match_profiles(plan, profiles):
    if plan.get("version") != 1 or not 1 <= len(plan.get("slots", [])) <= 32:
        raise ToolError("slot_plan_required")
    if len({r["slot"] for r in plan["slots"]}) != len(plan["slots"]) or len({r["public_port"] for r in plan["slots"]}) != len(plan["slots"]):
        raise ToolError("unique_slot_plan_required")
    if any(r["socks_port"] != 17927 + r["slot"] or not 1 <= r["public_port"] <= 65535 or not 1 <= r["slot"] <= 32 for r in plan["slots"]):
        raise ToolError("fixed_slot_port_mismatch")
    host = public_ipv4(plan["host_ip"])
    ips = [public_ipv4(r["expected_exit_ip"]) for r in plan["slots"]]
    if host in ips or len(set(ips)) != len(ips):
        raise ToolError("independent_non_host_exits_required")
    by_port = {p.port: p for p in profiles}
    if len(profiles) != len(plan["slots"]) or set(by_port) != {r["public_port"] for r in plan["slots"]}:
        raise ToolError("subscription_slot_mapping_mismatch")
    if any(p.host != plan["host_ip"] for p in profiles):
        raise ToolError("subscription_server_mismatch")
    return by_port


class LocalSocksSession:
    def __init__(self, row, timeout=20):
        self.row, self.timeout = row, timeout

    def __enter__(self):
        return self

    def __exit__(self, *_):
        pass

    def get(self, url):
        https_url(url)
        user, password = self.row["socks_user"], self.row["socks_pass"]
        if any(c in user + password for c in "\r\n\x00"):
            raise ToolError("invalid_socks_credentials")
        auth = (user + ":" + password).replace("\\", "\\\\").replace('"', '\\"')
        config = f'proxy = "socks5h://127.0.0.1:{int(self.row["socks_port"])}"\nproxy-user = "{auth}"\n'
        cmd = ["curl", "--disable", "--config", "-", "--silent", "--show-error", "--fail", "--ipv4",
               "--noproxy", "", "--proto", "=https", "--proto-redir", "=https", "--connect-timeout", "5",
               "--max-time", str(self.timeout), "--max-filesize", "65536", "--write-out", "\n%{http_code}", url]
        try:
            result = subprocess.run(cmd, input=config.encode(), stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                                    timeout=self.timeout + 1, env=child_environment())
        except subprocess.TimeoutExpired:
            raise ToolError("curl_failed_28") from None
        except OSError:
            raise ToolError("curl_unavailable") from None
        if result.returncode:
            raise ToolError("curl_failed_" + str(result.returncode))
        body, _, status = result.stdout.rpartition(b"\n")
        if status != b"200" or len(body) > 65536:
            raise ToolError("https_response_rejected")
        return body


def verify_session(session, expected, host_ip):
    expected = public_ipv4(expected)
    actual = []
    for _ in range(2):
        for url in ECHO_URLS:
            ip = parse_echo(session.get(url))
            if ip == host_ip or ip != expected:
                raise ToolError("exit_ip_mismatch")
            actual.append(ip)
    if b"Example Domain" not in session.get(CONTENT_URL):
        raise ToolError("content_check_failed")
    return actual


def verify_plan(plan, xray, vantage, *, check_socks=False, session_factory=XraySession, fetch=True):
    if fetch:
        url = plan["subscription_url"]
        p = urllib.parse.urlsplit(url)
        if p.scheme not in {"http", "https"} or p.hostname != plan["host_ip"] or p.username or p.fragment:
            raise ToolError("subscription_origin_rejected")
        opener = urllib.request.build_opener(NoRedirect())  # preserves environment proxy policy
        profiles = parse_subscription(request_bytes(opener, url))
        original = parse_subscription(plan["subscription_base64"].encode())
        if sorted(profiles, key=lambda p: p.port) != sorted(original, key=lambda p: p.port):
            raise ToolError("published_client_configuration_changed")
    else:
        profiles = parse_subscription(plan["subscription_base64"].encode())
    by_port = match_profiles(plan, profiles)
    report = {"status": "passed", "source": vantage, "verified_at": utc_now(), "nodes": []}
    for row in plan["slots"]:
        result = {k: row[k] for k in ("slot", "socks_port", "public_port", "expected_exit_ip", "ip_type")}
        try:
            with session_factory(by_port[row["public_port"]], xray, 60) as session:
                result["actual_exit_ips"] = verify_session(session, row["expected_exit_ip"], plan["host_ip"])
            if check_socks:
                with LocalSocksSession(row) as session:
                    result["socks_exit_ips"] = verify_session(session, row["expected_exit_ip"], plan["host_ip"])
            result["status"] = "passed"
        except ToolError as exc:
            result.update(status="failed", failure_code=str(exc))
            report["status"] = "failed"
        report["nodes"].append(result)
    return report


def probe_blocked(session):
    codes = []
    for url in ECHO_URLS:
        try:
            session.get(url)
        except ToolError as exc:
            if str(exc) not in TRANSPORT_FAILURES:
                raise
            codes.append(str(exc))
        else:
            # Any successful body is failure evidence, including a malformed IP.
            raise ToolError("request_succeeded_during_fault")
    return codes


def namespace_name(state_dir, slot):
    path = str(Path(state_dir).resolve())
    if path == "/var/lib/fanout":
        return "fo" + str(slot)
    h = 2166136261
    for b in path.encode():
        h = ((h ^ b) * 16777619) & 0xffffffff
    return f"fo{h & 0xffff:04x}{slot}"


def owned_openvpn(executable, args, config_path, process_ns_inode, slot_ns_inode):
    try:
        return (Path(executable).name == "openvpn" and process_ns_inode == slot_ns_inode
                and args[args.index(b"--config") + 1].decode() == config_path)
    except (ValueError, IndexError, UnicodeError):
        return False


def slot_process(state_dir, slot):
    ns = namespace_name(state_dir, slot)
    cfg = str(Path(state_dir).resolve() / (ns + ".ovpn"))
    try:
        result = subprocess.run(["ip", "netns", "pids", ns], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                                timeout=10, check=True)
    except (OSError, subprocess.SubprocessError):
        raise ToolError("slot_namespace_unavailable") from None
    matches = []
    for word in result.stdout.split():
        pid = int(word)
        try:
            proc = Path("/proc") / str(pid)
            args = (proc / "cmdline").read_bytes().split(b"\0")
            if not owned_openvpn(os.readlink(proc / "exe"), args, cfg, (proc / "ns/net").stat().st_ino,
                                 (Path("/run/netns") / ns).stat().st_ino):
                continue
            matches.append(pid)
        except (OSError, IndexError, UnicodeError):
            continue
    if len(matches) != 1:
        raise ToolError("unique_owned_openvpn_process_required")
    return matches[0]


def kill_slot_process(state_dir, slot):
    if os.geteuid() != 0 or not hasattr(os, "pidfd_open") or not hasattr(signal, "pidfd_send_signal"):
        raise ToolError("root_and_pidfd_required")
    pid = slot_process(state_dir, slot)
    fd = os.pidfd_open(pid)
    try:
        if slot_process(state_dir, slot) != pid:
            raise ToolError("slot_process_changed_before_fault")
        signal.pidfd_send_signal(fd, signal.SIGKILL)
    finally:
        os.close(fd)
    return pid


def fault_test(manager, xray, slot, recovery_timeout):
    plan = manager.plan()
    baseline = verify_plan(plan, xray, "server-local-public-client", check_socks=True)
    report = {"status": "failed", "source": "server-local-public-client", "slot": slot,
              "verified_at": utc_now(), "baseline": baseline, "fault_injected": False}
    if baseline["status"] != "passed":
        report["failure_code"] = "fault_requires_passing_baseline"
        return report
    row = next(r for r in plan["slots"] if r["slot"] == slot)
    profiles = match_profiles(plan, parse_subscription(plan["subscription_base64"].encode()))
    # Start and prove the actual public client before killing OpenVPN. A client
    # startup/configuration failure must never count as fail-closed evidence.
    try:
        with XraySession(profiles[row["public_port"]], xray, 75) as public, LocalSocksSession(row, 10) as socks:
            verify_session(public, row["expected_exit_ip"], plan["host_ip"])
            old_pid = kill_slot_process(manager.state_dir, slot)
            report["fault_injected"] = True
            with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
                futures = {"public": pool.submit(probe_blocked, public), "socks": pool.submit(probe_blocked, socks)}
                for name, future in futures.items():
                    try:
                        report["blocked_" + name] = {"status": "passed", "failure_codes": future.result()}
                    except ToolError as exc:
                        report["blocked_" + name] = {"status": "failed", "failure_code": str(exc)}
    except ToolError as exc:
        report["failure_code"] = str(exc)
        if not report["fault_injected"]:
            return report
    blocked = all(report.get("blocked_" + kind, {}).get("status") == "passed" for kind in ("public", "socks"))
    deadline = time.monotonic() + recovery_timeout
    while time.monotonic() < deadline:
        try:
            recovered = manager.plan()
            old_clients = parse_subscription(plan["subscription_base64"].encode())
            new_clients = parse_subscription(recovered["subscription_base64"].encode())
            if sorted(old_clients, key=lambda p: p.port) != sorted(new_clients, key=lambda p: p.port):
                raise ToolError("recovery_changed_client_configuration")
            if slot_process(manager.state_dir, slot) == old_pid:
                raise ToolError("replacement_process_not_observed")
            report["recovery"] = verify_plan(recovered, xray, "server-local-public-client", check_socks=True)
            report["status"] = "passed" if blocked and report["recovery"]["status"] == "passed" else "failed"
            return report
        except ToolError as exc:
            if str(exc) == "recovery_changed_client_configuration":
                report["failure_code"] = str(exc)
                return report
            time.sleep(3)
    report["failure_code"] = "recovery_deadline_exceeded"
    return report


def compare_state(before, after):
    def facts(directory):
        directory = Path(directory)
        settings = json.loads((directory / "settings.json").read_text())
        state = json.loads((directory / "state.json").read_text())
        native = json.loads((directory / "native.json").read_text())
        return {
            "management_password_preserved": (directory / "password").read_bytes(),
            "management_path_preserved": (directory / "basepath").read_bytes(),
            "subscription_token_preserved": settings["sub_token"],
            "default_type_policy_preserved": settings.get("residential_only"),
            "management_listener_preserved": (settings.get("port"), settings.get("listen_addr") or "0.0.0.0"),
            "slot_identity_and_credentials_preserved": sorted(
                (r["slot"], r["port"], r.get("socks_user"), r.get("socks_pass"), r.get("stopped", False)) for r in state["tunnels"]),
            "inbound_clients_and_keys_preserved": sorted(
                ({k: v for k, v in r.items() if k not in {"bound_to", "bound_slot", "name_mode"}} for r in native["inbounds"]), key=lambda r: r["id"])}
    a, b = facts(before), facts(after)
    checks = {k: a[k] == b[k] for k in a}
    before_policies = {r["slot"]: r.get("policy") for r in json.loads((Path(before)/"state.json").read_text())["tunnels"]}
    after_policies = {r["slot"]: r.get("policy") for r in json.loads((Path(after)/"state.json").read_text())["tunnels"]}
    checks["explicit_slot_policies_preserved"] = all(p is None or after_policies.get(slot) == p for slot, p in before_policies.items())
    return {"status": "passed" if all(checks.values()) else "failed", "verified_at": utc_now(), "checks": checks}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    capture = sub.add_parser("capture-plan", help="Read local management API into a private plan")
    capture.add_argument("--state-dir", type=Path, required=True)
    capture.add_argument("--output", type=Path, required=True)
    verify = sub.add_parser("verify", help="Fetch real subscription and probe every entry")
    verify.add_argument("--plan", type=Path, required=True)
    verify.add_argument("--xray", required=True)
    verify.add_argument("--vantage", choices=["server-local-public-client", "external-client"], required=True)
    verify.add_argument("--socks", action="store_true", help="Also probe local fixed SOCKS; server use only")
    verify.add_argument("--output", type=Path, required=True)
    fault = sub.add_parser("fault", help="Kill only an owned test Slot OpenVPN and verify recovery")
    fault.add_argument("--state-dir", type=Path, required=True)
    fault.add_argument("--xray", required=True)
    fault.add_argument("--slot", type=int, choices=range(1, 33), required=True)
    fault.add_argument("--recovery-timeout", type=int, default=300)
    fault.add_argument("--output", type=Path, required=True)
    compare = sub.add_parser("compare-state", help="Compare consistent private snapshots without exposing values")
    compare.add_argument("--before", type=Path, required=True)
    compare.add_argument("--after", type=Path, required=True)
    compare.add_argument("--output", type=Path, required=True)
    args = parser.parse_args(argv)
    try:
        output = args.output.resolve()
        protected = [getattr(args, name, None) for name in ("state_dir", "before", "after")]
        if any(p is not None and output.is_relative_to(p.resolve()) for p in protected):
            # Do not write even an error report over live state or a backup.
            print(json.dumps({"status": "failed", "failure_code": "output_must_be_outside_state_directories"}))
            return 1
        if args.command == "verify" and output == args.plan.resolve():
            print(json.dumps({"status": "failed", "failure_code": "output_must_not_replace_private_plan"}))
            return 1
        if args.command == "capture-plan":
            report = LocalManager(args.state_dir).plan()
            write_json(args.output, report, private=True)
            print(json.dumps({"status": "captured", "private": True, "slot_count": len(report["slots"])}))
            return 0
        if args.command == "compare-state":
            report = compare_state(args.before, args.after)
        elif args.command == "fault":
            if not 30 <= args.recovery_timeout <= 900:
                raise ToolError("invalid_recovery_timeout")
            report = fault_test(LocalManager(args.state_dir), args.xray, args.slot, args.recovery_timeout)
        else:
            if args.socks and args.vantage != "server-local-public-client":
                raise ToolError("local_socks_requires_server_vantage")
            if args.plan.stat().st_mode & 0o077:
                raise ToolError("private_plan_permissions_required")
            plan = json.loads(args.plan.read_text())
            report = verify_plan(plan, args.xray, args.vantage, check_socks=args.socks)
        write_json(args.output, report, private=True)
        print(json.dumps({"status": report["status"], "command": args.command}))
        return int(report["status"] != "passed")
    except ToolError as exc:
        report = {"status": "failed", "failure_code": str(exc), "verified_at": utc_now()}
    except (OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError):
        report = {"status": "failed", "failure_code": "invalid_input_or_runtime_unavailable", "verified_at": utc_now()}
    # Never print exception repr, URLs, command stderr or source configuration.
    try:
        write_json(args.output, report, private=True)
    except OSError:
        print(json.dumps({"status": "failed", "failure_code": "report_write_failed"}))
        return 1
    print(json.dumps({"status": "failed", "failure_code": report["failure_code"]}))
    return 1
