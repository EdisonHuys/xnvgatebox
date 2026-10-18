"""Shared IP intelligence normalization. Missing booleans remain null."""
from __future__ import annotations

import re

from .common import ToolError, public_ipv4, utc_now


RULE_VERSION = "residential-1"


def flag(value):
    return value if type(value) is bool else None


def classify(raw: dict, settings: dict, provider: str, expected_ip: str) -> dict:
    if public_ipv4(raw.get("ip")) != expected_ip:
        raise ToolError("intelligence_ip_mismatch")
    privacy = raw.get("privacy") if isinstance(raw.get("privacy"), dict) else {}
    network = raw.get("asn") if isinstance(raw.get("asn"), dict) else {}
    company = raw.get("company") if isinstance(raw.get("company"), dict) else {}
    hosting = flag(privacy.get("is_hosting"))
    mobile = flag(privacy.get("is_mobile", raw.get("is_mobile")))
    residential = flag(privacy.get("is_residential", raw.get("is_residential")))
    kind = str(network.get("type") or company.get("type") or raw.get("network_type") or "").lower()
    kinds = {str(value).lower() for value in (network.get("type"), company.get("type"), raw.get("network_type")) if value}
    asn = str(network.get("asn") or "").upper()
    if asn.isdigit():
        asn = "AS" + asn
    if not re.fullmatch(r"AS[0-9]+", asn):
        asn = None
    isp = network.get("name") or network.get("org") or company.get("name") or raw.get("isp")
    if not isinstance(isp, str):
        isp = None
    country = raw.get("country_code")
    if isinstance(raw.get("country"), dict):
        country = country or raw["country"].get("iso_code")
    if not isinstance(country, str) or not re.fullmatch(r"[A-Z]{2}", country.upper()):
        country = None
    else:
        country = country.upper()
    consumer = asn in settings["consumer_asns"] or bool(isp and any(pattern.casefold() in isp.casefold() for pattern in settings["consumer_isp_patterns"]))
    dc = hosting is True or bool(kinds.intersection(("hosting", "datacenter", "data center")))
    mob = mobile is True or bool(kinds.intersection(("mobile", "cellular")))
    business = bool(kinds.intersection(("business", "enterprise", "education", "government")))
    conflict = sum((dc, mob, business)) > 1 or (residential is True and (dc or mob or business))
    ip_type, reason, confidence = "unknown", "insufficient_fields", "unknown"
    if conflict:
        reason = "conflicting_network_evidence"
    elif dc:
        ip_type, reason, confidence = "datacenter", "explicit_hosting", "high"
    elif mob:
        ip_type, reason, confidence = "mobile", "explicit_mobile", "high"
    elif business:
        ip_type, reason, confidence = "business_isp", "explicit_business", "high"
    elif residential is True and hosting is False and country and asn and isp:
        ip_type, reason, confidence = "strict_residential", "explicit_residential", "high"
    elif residential is not False and hosting is False and kind == "isp" and consumer and country and asn and isp:
        ip_type, reason, confidence = "likely_residential", "consumer_isp_no_hosting", "medium"
    elif kind == "isp" and hosting is False and country and asn and isp:
        ip_type, reason, confidence = "business_isp", "isp_residential_unproven", "low"
    city = raw.get("city")
    if isinstance(city, dict):
        names = city.get("names")
        city = city.get("name") or (names.get("en") if isinstance(names, dict) else None)
    if not isinstance(city, str) or not city.strip() or len(city) > 100 or any(ord(c) < 32 for c in city):
        city = None
    return {"ip": expected_ip, "country": country, "city": city, "asn": asn, "isp": isp, "ip_type": ip_type,
            "hosting": hosting, "mobile": mobile, "residential": residential,
            "proxy": flag(privacy.get("is_proxy")), "vpn": flag(privacy.get("is_vpn")),
            "abuse": flag(privacy.get("is_abuser")), "reason": reason, "confidence": confidence,
            "provider": provider, "queried_at": utc_now(), "rule_version": RULE_VERSION,
            "raw": raw}
