from __future__ import annotations

import json
import os
import re
import uuid
from pathlib import Path
from urllib.parse import urlsplit

from .common import ToolError, https_url


def load_env(path: Path):
    """Read simple KEY=VALUE files without executing shell syntax."""
    if not path.exists():
        return
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        key, separator, value = line.partition("=")
        if not separator or not re.fullmatch(r"[A-Z][A-Z0-9_]*", key.strip()):
            raise ToolError("invalid_env_file")
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
            value = value[1:-1]
        os.environ.setdefault(key.strip(), value)


def load_settings(path: Path) -> dict:
    settings = json.loads(path.read_text())
    settings.setdefault('pool_source_timeout_seconds', 10)
    settings.setdefault('mirror_fallback', {'enabled': False, 'max_age_hours': 24})
    source_timeout = settings['pool_source_timeout_seconds']
    mirror = settings['mirror_fallback']
    if type(source_timeout) is not int or not 2 <= source_timeout <= 20:
        raise ToolError('invalid_setting_pool_source_timeout_seconds')
    if (not isinstance(mirror, dict) or type(mirror.get('enabled')) is not bool
            or type(mirror.get('max_age_hours')) is not int or not 1 <= mirror['max_age_hours'] <= 168):
        raise ToolError('invalid_setting_mirror_fallback')
    if not isinstance(settings.get('source_urls'), list):
        raise ToolError('invalid_setting_source_urls')
    limits = {"candidate_limit": 500, "sstp_concurrency": 16, "xray_concurrency": 8,
              "node_timeout_seconds": 120, "chain_timeout_seconds": 180,
              "chain_ttl_seconds": 86400, "intelligence_ttl_seconds": 604800,
              "intelligence_query_limit": 1000}
    for key, maximum in limits.items():
        value = settings.get(key)
        if type(value) is not int or not 1 <= value <= maximum:
            raise ToolError("invalid_setting_" + key)
    for url in settings["source_urls"] + settings["echo_urls"] + [settings["content_url"]]:
        https_url(url)
    echo_hosts = {urlsplit(url).hostname for url in settings["echo_urls"]}
    if len(echo_hosts) != 2 or len(settings["echo_urls"]) != 2:
        raise ToolError("two_distinct_echo_hosts_required")
    if len({".".join(host.split(".")[-2:]) for host in echo_hosts}) != 2:
        raise ToolError("two_distinct_echo_providers_required")
    if settings.get("edge_adapter") != "edgetunnel-globalproxy-v1":
        raise ToolError("unsupported_edge_adapter")
    if not re.fullmatch(r"/[A-Za-z0-9/_-]*", settings["edge_base_path"]):
        raise ToolError("invalid_edge_base_path")
    if type(settings["edge_port"]) is not int or not 1 <= settings["edge_port"] <= 65535:
        raise ToolError("invalid_edge_port")
    if not settings.get("content_marker"):
        raise ToolError("content_marker_required")
    if not all(re.fullmatch(r"[A-Z]{2}", country) for country in settings["countries"]):
        raise ToolError("invalid_country_filter")
    return settings


def mode_a_environment(require_checker=True) -> dict:
    origin = os.getenv("WORKER_URL", "").strip()
    if origin:
        parsed = urlsplit(https_url(origin))
        if (parsed.path not in ("", "/") or parsed.query or parsed.fragment
                or parsed.username or parsed.password or parsed.port not in (None, 443)):
            raise ToolError("invalid_worker_url")
        edge_host = parsed.hostname.lower()
        checker_url = "https://" + edge_host + "/check"
    else:
        edge_host = os.getenv("EDGETUNNEL_HOST", "").lower()
        checker_url = os.getenv("CHECKER_URL", "")
    required = (["CHECKER_URL"] if require_checker else []) + ["EDGETUNNEL_HOST", "VLESS_UUID"]
    values = {"CHECKER_URL": checker_url, "EDGETUNNEL_HOST": edge_host, "VLESS_UUID": os.getenv("VLESS_UUID")}
    missing = [key for key in required if not values[key]]
    if missing:
        raise ToolError("missing_environment:" + ",".join(missing))
    endpoint = https_url(checker_url) if require_checker else None
    if endpoint and urlsplit(endpoint).query:
        raise ToolError("checker_url_must_not_have_query")
    host = edge_host
    if not re.fullmatch(r"[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?", host) or "." not in host:
        raise ToolError("invalid_edgetunnel_host")
    try:
        user_id = str(uuid.UUID(os.environ["VLESS_UUID"]))
    except ValueError:
        raise ToolError("invalid_vless_uuid") from None
    return {"checker_url": endpoint, "edge_host": host, "uuid": user_id,
            "username": os.getenv("VPN_USERNAME", "vpn"), "password": os.getenv("VPN_PASSWORD", "vpn"),
            "xray_bin": os.getenv("XRAY_BIN", "tools/bin/xray")}
