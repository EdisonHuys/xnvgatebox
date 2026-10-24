"""Original parser for the official VPN Gate CSV; profiles are data, never executed."""
from __future__ import annotations

import base64
import csv
import io
import re
import shlex

from .common import ToolError, digest, public_ipv4, utc_now


def number(value):
    try:
        return max(0, int(value))
    except (ValueError, TypeError):
        return None


def profile_remotes(profile: str) -> list:
    lines = []
    inside = False
    for line in profile.splitlines():
        line = line.strip()
        if line.startswith("<"):
            inside = not line.startswith("</")
            continue
        if inside or not line or line.startswith(("#", ";")):
            continue
        try:
            lines.append(shlex.split(line, comments=True))
        except ValueError:
            raise ToolError("invalid_openvpn_syntax") from None
    protocol = next((parts[1].lower() for parts in lines if len(parts) > 1 and parts[0] == "proto"), "udp")
    remotes = []
    for parts in lines:
        if len(parts) >= 3 and parts[0] == "remote":
            host, port = parts[1], number(parts[2])
            proto = parts[3].lower() if len(parts) > 3 else protocol
            if not re.fullmatch(r"[a-zA-Z0-9.-]+", host) or not port or port > 65535:
                raise ToolError("invalid_openvpn_remote")
            remotes.append({"host": host, "port": port, "transport": "tcp" if proto.startswith("tcp") else "udp"})
    if not remotes:
        raise ToolError("openvpn_remote_missing")
    return remotes


def validate_mirror_profile(profile: str, server_ip: str):
    """Accept only the data-only subset emitted by VPN Gate's public profiles.

    OpenVPN configs can load root plugins and scripts. A mirror's digest is not
    an authenticity proof, so reject unknown directives rather than strip them.
    Inline PEM bodies cannot escape into another directive or reference files.
    """
    arity = {'client': 0, 'dev': 1, 'proto': 1, 'remote': (2, 3), 'cipher': 1,
             'data-ciphers': 1, 'auth': 1, 'resolv-retry': 1, 'nobind': 0,
             'persist-key': 0, 'persist-tun': 0, 'verb': 1, 'mute': 1,
             'auth-nocache': 0, 'remote-cert-tls': 1, 'tls-version-min': 1}
    inside = None
    for raw in profile.split('\n'):
        line = raw.rstrip('\r').strip(' \t')
        if not line or line.startswith(('#', ';')):
            continue
        if any(ord(c) < 32 and c != '\t' for c in line):
            raise ToolError('unsafe_mirror_profile')
        if inside:
            if line == '</' + inside + '>':
                inside = None
            elif not (re.fullmatch(r'[A-Za-z0-9+/=]+', line) or
                      re.fullmatch(r'-----(?:BEGIN|END) (?:CERTIFICATE|PRIVATE KEY|RSA PRIVATE KEY)-----', line)):
                raise ToolError('unsafe_mirror_profile')
            continue
        if line in ('<ca>', '<cert>', '<key>'):
            inside = line[1:-1]
            continue
        # Quotes, backslash continuations, leading --, file paths and inline
        # option expansion are intentionally outside this narrow grammar.
        if not re.fullmatch(r'[A-Za-z0-9_.: \t-]+', line):
            raise ToolError('unsafe_mirror_profile')
        parts = line.split()
        count = arity.get(parts[0])
        if count is None or len(parts)-1 not in (count if isinstance(count, tuple) else (count,)):
            raise ToolError('unsafe_mirror_profile')
        if parts[0] == 'remote' and (parts[1] != server_ip or not parts[2].isdigit() or not 1 <= int(parts[2]) <= 65535):
            raise ToolError('unsafe_mirror_profile')
        if parts[0] == 'dev' and parts[1] != 'tun':
            raise ToolError('unsafe_mirror_profile')
    if inside:
        raise ToolError('unsafe_mirror_profile')


def parse_csv(data: bytes, settings: dict, *, safe_profiles=False):
    try:
        lines = data.decode("utf-8-sig").splitlines()
    except UnicodeError:
        raise ToolError("invalid_csv_encoding") from None
    if not lines or lines[0].strip() != "*vpn_servers":
        raise ToolError("invalid_vpngate_csv")
    if len(lines) < 2 or not lines[1].startswith("#HostName,"):
        raise ToolError("missing_csv_header")
    body = [lines[1][1:]] + [line for line in lines[2:] if line and not line.startswith("*")]
    reader = csv.DictReader(io.StringIO("\n".join(body)))
    if not {"HostName", "IP", "CountryShort", "OpenVPN_ConfigData_Base64"}.issubset(reader.fieldnames or []):
        raise ToolError("missing_csv_columns")
    found, profiles, rejected = {}, {}, 0
    for row in reader:
        try:
            hostname = row["HostName"].strip().lower()
            if not re.fullmatch(r"[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?", hostname):
                raise ToolError("invalid_hostname")
            if "." not in hostname:
                hostname += ".opengw.net"
            ip = public_ipv4(row["IP"])
            country = row["CountryShort"].upper()
            if not re.fullmatch(r"[A-Z]{2}", country):
                raise ToolError("invalid_country")
            if settings["countries"] and country not in settings["countries"]:
                continue
            encoded = row["OpenVPN_ConfigData_Base64"]
            if len(encoded) > 180_000:
                raise ToolError("profile_too_large")
            raw_profile = base64.b64decode(encoded, validate=True)
            profile = raw_profile.decode("utf-8")
            if safe_profiles:
                validate_mirror_profile(profile, ip)
            remotes = profile_remotes(profile)
            sha = digest(raw_profile)
            ports = list(dict.fromkeys([item["port"] for item in remotes if item["transport"] == "tcp"] + [443]))
            node = {"node_id": "vpngate-" + digest(hostname.encode())[:20], "hostname": hostname,
                    "server_ip": ip, "advertised_country": country, "source_score": number(row.get("Score")) or 0,
                    "source_ping_ms": number(row.get("Ping")), "discovered_at": utc_now(),
                    "protocols": {"sstp": {"status": "not_tested", "candidates": [{"host": hostname, "port": p} for p in ports[:3]],
                                           "expected_exit_ip": None, "egress": None},
                                  "openvpn": {"status": "not_tested", "profile_ref": "configs/" + sha + ".ovpn",
                                              "profile_sha256": sha, "remotes": remotes,
                                              "expected_exit_ip": None, "egress": None}}}
            if hostname not in found or node["source_score"] > found[hostname]["source_score"]:
                found[hostname] = node
                profiles[sha] = raw_profile
        except (ToolError, ValueError, UnicodeError, KeyError, AttributeError, TypeError):
            rejected += 1
    nodes = sorted(found.values(), key=lambda node: (-node["source_score"], node["node_id"]))
    selected_profiles = {node["protocols"]["openvpn"]["profile_sha256"]: profiles[node["protocols"]["openvpn"]["profile_sha256"]] for node in nodes}
    return nodes, selected_profiles, rejected
