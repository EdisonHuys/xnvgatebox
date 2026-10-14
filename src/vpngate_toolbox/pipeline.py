"""Mode A orchestration; candidates and final validated views stay separate."""
from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from threading import Event

from .checker import check_node
from .common import ToolError, atomic_write, is_fresh, utc_now, write_json
from .full_chain import verify, verify_no_fallback
from .intelligence import enrich
from .manifest import build_cf_manifest, empty_cf_manifest
from .pool import build_pool, save_pool
from .subscription import make_connection


def clear_publication(output: Path, runtime: Path, reason="run_in_progress"):
    generated_at = utc_now()
    view = {"schema_version": 1, "generated_at": generated_at, "status": reason, "nodes": []}
    write_json(output / "mode_a_validated.json", view)
    write_json(output / "residential.json", view)
    write_json(output / "cf_manifest.json", empty_cf_manifest(reason, generated_at))
    atomic_write(runtime / "subscription.txt", b"", private=True)
    atomic_write(output / "nodes.txt", b"")


def publish(pool: dict, output: Path, runtime: Path, environment: dict, settings: dict, guard: dict):
    valid = []
    links, candidates = [], []
    for node in pool["nodes"]:
        evidence = node.get("mode_a", {})
        if (guard.get("status") != "passed" or not is_fresh(guard.get("expires_at"))
                or evidence.get("status") != "passed" or not is_fresh(evidence.get("expires_at"))):
            continue
        connection = make_connection(node, environment, settings)
        if connection.sha256 != evidence.get("config_sha256"):
            raise ToolError("publication_config_changed")
        expected = node["protocols"]["sstp"].get("expected_exit_ip")
        if evidence.get("actual_exit_ip") != expected or not evidence.get("actual_exit_ips") or any(ip != expected for ip in evidence["actual_exit_ips"]):
            raise ToolError("publication_exit_mismatch")
        valid.append(node)
        links.append(connection.link())
        protocol = node["protocols"]["sstp"]
        candidates.append("sstp://" + protocol["host"] + ":" + str(protocol["port"]) + " # " + node["node_id"])
    generated_at = utc_now()
    view = {"schema_version": 1, "generated_at": generated_at, "scope": "tcp_ipv4_ws", "no_fallback_control": guard, "nodes": valid}
    write_json(output / "mode_a_validated.json", view)
    residential = {**view, "nodes": [node for node in valid if node["protocols"]["sstp"]["egress"]["ip_type"] in ("strict_residential", "likely_residential")]}
    write_json(output / "residential.json", residential)
    write_json(output / "cf_manifest.json", build_cf_manifest(valid, generated_at, settings, environment))
    atomic_write(output / "nodes.txt", ("\n".join(candidates) + ("\n" if candidates else "")).encode())
    atomic_write(runtime / "subscription.txt", ("\n".join(links) + ("\n" if links else "")).encode(), private=True)
    return len(valid)


def run_mode_a(settings: dict, environment: dict, output: Path, runtime: Path, csv_file=None,
               builder=build_pool, checker=check_node, verifier=verify, guard_verifier=verify_no_fallback, enricher=enrich):
    clear_publication(output, runtime)
    report = {"started_at": utc_now(), "milestone": "v0.1", "status": "failed",
              "baseline_technical_debt": "T1_checker_tls_identity", "published": 0}
    try:
        pool = builder(settings, output, csv_file)
        report["candidates"] = len(pool["nodes"])
        circuit = Event()

        def check(node):
            if circuit.is_set():
                node["protocols"]["sstp"].update({"status": "not_tested", "service_error": "batch_checker_unavailable"})
                return node
            result = checker(node, environment, settings)
            if "service_error" in result["protocols"]["sstp"]:
                circuit.set()
            return result

        with ThreadPoolExecutor(max_workers=settings["sstp_concurrency"]) as executor:
            budget = settings["candidate_limit"]
            pool["nodes"] = list(executor.map(check, pool["nodes"][:budget])) + pool["nodes"][budget:]
        report["intelligence"] = enricher(pool["nodes"], settings, runtime / "intelligence-cache.json")
        passed = [node for node in pool["nodes"] if node["protocols"]["sstp"]["status"] == "passed"]
        report["sstp_passed"] = len(passed)
        report["checker_service_errors"] = sum("service_error" in node["protocols"]["sstp"] for node in pool["nodes"])
        service_error_codes = {}
        for node in pool["nodes"]:
            code = node["protocols"]["sstp"].get("service_error")
            if code:
                service_error_codes[code] = service_error_codes.get(code, 0) + 1
        report["checker_service_error_codes"] = service_error_codes
        if not passed:
            save_pool(pool, output)
            raise ToolError("no_sstp_passed_nodes")
        guard = guard_verifier(make_connection(passed[0], environment, settings, invalid=True), settings, environment["xray_bin"])
        report["no_fallback_control"] = guard
        if guard["status"] != "passed":
            save_pool(pool, output)
            raise ToolError("no_fallback_control_failed")

        def test(node):
            connection = make_connection(node, environment, settings)
            node["mode_a"] = verifier(connection, node["protocols"]["sstp"]["expected_exit_ip"], settings, environment["xray_bin"])
            node["mode_a"]["no_fallback_control_at"] = guard["verified_at"]
            return node

        with ThreadPoolExecutor(max_workers=settings["xray_concurrency"]) as executor:
            list(executor.map(test, passed))
        pool["generated_at"] = utc_now()
        save_pool(pool, output)
        report["published"] = publish(pool, output, runtime, environment, settings, guard)
        report["status"] = "passed" if report["published"] else "failed"
        report["node_results"] = [{"node_id": node["node_id"], "mode_a": node.get("mode_a", {"status": "not_tested"})} for node in pool["nodes"]]
    except ToolError as exc:
        clear_publication(output, runtime, "failed")
        report["failure_code"] = str(exc)
    except BaseException:
        clear_publication(output, runtime, "interrupted")
        report["failure_code"] = "unexpected_error_or_interruption"
        raise
    finally:
        report["finished_at"] = utc_now()
        write_json(output / "report.json", report)
    return report
