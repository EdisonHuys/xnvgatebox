from __future__ import annotations

import argparse
import fcntl
import json
import shutil
import sys
from contextlib import contextmanager
from pathlib import Path

from .common import ToolError, atomic_write, utc_now, write_json
from .config import load_env, load_settings, mode_a_environment
from .pipeline import clear_publication, publish, run_mode_a
from .pool import build_pool
from .intelligence import profile_measured_exit
from .country import attach_country


@contextmanager
def output_lock(output: Path, wait=False):
    output.mkdir(parents=True, exist_ok=True)
    with (output / ".build.lock").open("w") as handle:
        try:
            fcntl.flock(handle, fcntl.LOCK_EX | (0 if wait else fcntl.LOCK_NB))
        except BlockingIOError:
            raise ToolError("another_build_is_running") from None
        try:
            yield
        finally:
            fcntl.flock(handle, fcntl.LOCK_UN)


def main(argv=None):
    parser = argparse.ArgumentParser(description="VPN Gate shared pool and Mode A verifier")
    parser.add_argument("--config", type=Path, default=Path("config/settings.json"))
    parser.add_argument("--env-file", type=Path, default=Path(".env"))
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ("pool", "mode-a"):
        command = sub.add_parser(name)
        command.add_argument("--csv-file", type=Path, help="Use a local official-format CSV instead of fetching")
        command.add_argument("--output", type=Path, default=Path("data"))
        command.add_argument("--runtime", type=Path, default=Path("runtime"))
    export = sub.add_parser("export-subscription")
    export.add_argument("--view", type=Path, default=Path("data/mode_a_validated.json"))
    export.add_argument("--output", type=Path, default=Path("data"))
    export.add_argument("--runtime", type=Path, default=Path("runtime"))
    profile = sub.add_parser("profile-openvpn", help="Profile a measured OpenVPN exit IP; does not verify a tunnel")
    profile.add_argument("--ip", required=True)
    profile.add_argument("--cache", type=Path, default=Path("runtime/intelligence/cache.json"))
    sstp_profile = sub.add_parser("profile-sstp", help="Profile a measured SSTP exit; not a tunnel check")
    sstp_profile.add_argument("--ip", required=True)
    sstp_profile.add_argument("--cache", type=Path, default=Path("runtime/intelligence/sstp-cache.json"))
    socks_profile = sub.add_parser("profile-socks5", help="Profile a measured third-party SOCKS5 exit")
    socks_profile.add_argument("--ip", required=True)
    socks_profile.add_argument("--cache", type=Path, default=Path("runtime/intelligence/socks5-cache.json"))
    for command in (profile, sstp_profile, socks_profile):
        command.add_argument("--country", action="store_true", help="Independently locate the measured IP without inferring residential type")
    args = parser.parse_args(argv)
    try:
        if args.command in ("profile-openvpn", "profile-sstp", "profile-socks5"):
            settings = load_settings(args.config)
            load_env(args.env_file)
            # Multiple Slots can finish their baselines concurrently. Serialize
            # cache read/query/write without locking or rewriting the node pool.
            with output_lock(args.cache.parent, wait=True):
                image = profile_measured_exit(args.ip, settings, args.cache, args.command.removeprefix("profile-"))
            if args.country:
                with output_lock(args.cache.parent / "country", wait=True):
                    image = attach_country(image, args.cache.parent / "country/cache.json")
            print(json.dumps(image, ensure_ascii=False))
            return 0
        with output_lock(args.output):
            if args.command == "mode-a":
                clear_publication(args.output, args.runtime)
            elif args.command == "export-subscription":
                atomic_write(args.runtime / "subscription.txt", b"", private=True)
            try:
                settings = load_settings(args.config)
                load_env(args.env_file)
            except (ToolError, OSError, ValueError, KeyError, TypeError) as exc:
                if args.command == "mode-a":
                    write_json(args.output / "report.json", {"status": "configuration_error",
                               "failure_code": str(exc) if isinstance(exc, ToolError) else "invalid_input_or_filesystem_error",
                               "generated_at": utc_now()})
                raise
            if args.command == "pool":
                pool = build_pool(settings, args.output, args.csv_file)
                print(json.dumps({"status": "candidate_pool_built", "nodes": len(pool["nodes"]), "full_chain_verified": False}))
                return 0
            if args.command == "mode-a":
                # Empty any prior subscription even if configuration/tool preflight fails.
                clear_publication(args.output, args.runtime)
                try:
                    environment = mode_a_environment()
                    binary = Path(environment["xray_bin"])
                    if not binary.is_file() or not shutil.which("curl"):
                        raise ToolError("install_xray_and_curl_first")
                    report = run_mode_a(settings, environment, args.output, args.runtime, args.csv_file)
                except ToolError as exc:
                    clear_publication(args.output, args.runtime, "configuration_error")
                    write_json(args.output / "report.json", {"status": "configuration_error", "failure_code": str(exc), "generated_at": utc_now()})
                    raise
                # Keep the concise status line used by Actions, but include the
                # actionable failure code so a failed run can be diagnosed from
                # the job log without downloading the private evidence artifact.
                print(json.dumps({key: report.get(key) for key in
                                  ("status", "published", "failure_code", "candidates",
                                   "sstp_passed", "checker_service_errors",
                                   "checker_service_error_codes")}))
                return 0 if report["status"] == "passed" else 3
            view = json.loads(args.view.read_text())
            # Export from downloaded public evidence without rerunning a different config.
            args.runtime.mkdir(parents=True, exist_ok=True)
            atomic_write(args.runtime / "subscription.txt", b"", private=True)
            environment = mode_a_environment(require_checker=False)
            count = publish(view, args.output, args.runtime, environment, settings, view.get("no_fallback_control", {}))
            print(json.dumps({"status": "exported" if count else "no_fresh_verified_nodes", "published": count}))
            return 0 if count else 3
    except ToolError as exc:
        print("Error: " + str(exc), file=sys.stderr)
        return 2
    except (OSError, ValueError, KeyError, TypeError):
        print("Error: invalid_input_or_filesystem_error", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
