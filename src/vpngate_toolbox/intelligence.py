"""Optional independent HTTPS intelligence lookup with per-IP cache and budget."""
from __future__ import annotations

import json
import os
from pathlib import Path
from urllib.parse import quote

from .classifier import classify
from .common import ServiceError, ToolError, digest, expires_at, get_json, https_url, is_fresh, public_ipv4, write_json


def enrich(nodes: list, settings: dict, cache_path: Path, transport=get_json, protocol_name="sstp"):
    template = os.getenv("INTELLIGENCE_URL_TEMPLATE", "")
    if not template:
        return {"enabled": False, "queries": 0, "service_errors": []}
    if "{ip}" not in template:
        raise ToolError("intelligence_ip_placeholder_required")
    https_url(template.replace("{ip}", "1.1.1.1"))
    try:
        cache = json.loads(cache_path.read_text())
    except (FileNotFoundError, ValueError):
        cache = {}
    if not isinstance(cache, dict):
        cache = {}
    stats = {"enabled": True, "queries": 0, "service_errors": []}
    headers = {"X-API-Key": os.environ["INTELLIGENCE_TOKEN"]} if os.getenv("INTELLIGENCE_TOKEN") else {}
    provider = "independent_https_" + digest(template.encode())[:12]
    unavailable = False
    for node in nodes:
        protocol = node["protocols"][protocol_name]
        if protocol["status"] != "passed":
            continue
        ip = protocol["expected_exit_ip"]
        key = provider + ":" + ip
        entry = cache.get(key, {})
        if not isinstance(entry, dict):
            entry = {}
        try:
            cached = is_fresh(entry.get("expires_at")) and isinstance(entry.get("raw"), dict)
            if cached:
                raw = entry["raw"]
            elif not unavailable and stats["queries"] < settings["intelligence_query_limit"]:
                stats["queries"] += 1
                raw = transport(template.replace("{ip}", quote(ip)), timeout=settings["node_timeout_seconds"], headers=headers)
            else:
                protocol["intelligence_error"] = "intelligence_budget_or_service_unavailable"
                continue
            image = classify(raw, settings, provider, ip)
            image["identity_assurance"] = "normal_https_certificate_validation"
            image["expires_at"] = entry["expires_at"] if cached else expires_at(settings["intelligence_ttl_seconds"])
            if cached:
                image["queried_at"] = entry.get("queried_at")
                image["cache_used"] = True
            protocol["egress"] = image
            if not cached:
                cache[key] = {"raw": raw, "queried_at": image["queried_at"],
                              "expires_at": image["expires_at"]}
        except (ServiceError, ToolError) as exc:
            protocol["intelligence_error"] = str(exc)
            stats["service_errors"].append(str(exc))
            # Do not hammer the provider after 429 or an outage; keep prior evidence labelled.
            unavailable = True
    write_json(cache_path, cache, private=True)
    return stats


def profile_openvpn_exit(ip: str, settings: dict, cache_path: Path, transport=get_json) -> dict:
    """Profile a caller's measured OpenVPN baseline, never an SSTP/server IP.

    This command does not verify the tunnel. The manager must independently
    establish the baseline and check its fixed SOCKS port before publishing it.
    """
    ip = public_ipv4(ip)
    protocol = {"status": "passed", "expected_exit_ip": ip}
    try:
        stats = enrich([{"protocols": {"openvpn": protocol}}], settings, cache_path,
                       transport, protocol_name="openvpn")
    except ToolError as exc:
        stats = {"enabled": True}
        protocol["intelligence_error"] = str(exc)
    if "egress" in protocol:
        return {**protocol["egress"], "protocol": "openvpn", "intelligence_status": "passed"}
    error = protocol.get("intelligence_error", "intelligence_not_configured" if not stats["enabled"]
                         else "intelligence_unavailable")
    return {"ip": ip, "protocol": "openvpn", "country": None, "asn": None, "isp": None,
            "ip_type": "unknown", "hosting": None, "mobile": None, "residential": None,
            "proxy": None, "vpn": None, "abuse": None, "confidence": "unknown",
            "reason": error, "intelligence_error": error,
            "provider": "independent_https_" + digest(os.environ["INTELLIGENCE_URL_TEMPLATE"].encode())[:12]
                        if os.getenv("INTELLIGENCE_URL_TEMPLATE") else None,
            "intelligence_status": "not_configured" if not stats["enabled"] else "failed"}
