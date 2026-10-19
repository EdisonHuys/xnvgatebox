"""Independent country evidence for a measured IP; never a residential claim."""
from __future__ import annotations

import json
import os
from pathlib import Path
import re
import time
from urllib.parse import quote

from .common import ToolError, digest, expires_at, get_json, https_url, is_fresh, public_ipv4, utc_now, write_json


def country_code(value):
    return isinstance(value, str) and re.fullmatch(r'[A-Z]{2}', value) is not None and value not in ('ZZ', 'XX')


def attach_country(image: dict, cache_path: Path, transport=get_json) -> dict:
    """Caller serializes this small cache separately from paid intelligence."""
    ip = public_ipv4(image['ip'])
    if image.get('intelligence_status') == 'passed' and country_code(image.get('country')) and is_fresh(image.get('expires_at')):
        return image
    template = os.getenv('COUNTRY_URL_TEMPLATE', 'https://api.country.is/{ip}')
    result = {'ip': ip, 'status': 'not_configured' if not template else 'failed', 'error': 'country_not_configured'}
    image = {**image, 'country': None, 'country_evidence': result}
    if not template:
        return image
    try:
        if '{ip}' not in template:
            raise ToolError('country_ip_placeholder_required')
        https_url(template.replace('{ip}', ip))
        provider = 'country_https_' + digest(template.encode())[:12]
        result['provider'] = provider
        try:
            cache = json.loads(cache_path.read_text())
        except (FileNotFoundError, ValueError):
            cache = {}
        if not isinstance(cache, dict):
            cache = {}
        key = provider + ':' + ip
        entry = cache.get(key, {})
        if isinstance(entry, dict) and entry.get('ip') == ip and is_fresh(entry.get('expires_at')):
            if entry.get('status') == 'passed' and country_code(entry.get('country')):
                image['country'] = entry['country']
                image['country_evidence'] = {**entry, 'cache_used': True}
                return image
            if entry.get('status') == 'failed':
                image['country_evidence'] = entry
                return image
        retry = cache.get('retry:' + provider, 0)
        if isinstance(retry, (int, float)) and retry > time.time():
            result['error'] = 'country_service_cooldown'
            return image
        # A shared cache lock serializes queries and keeps the default service
        # below its documented request rate. Failures open a short circuit.
        last = cache.get('last:' + provider, 0)
        if isinstance(last, (int, float)):
            time.sleep(max(0, min(0.12, 0.12 - (time.time() - last))))
        cache['last:' + provider] = time.time()
        raw = transport(template.replace('{ip}', quote(ip)), timeout=4, headers={})
        if not isinstance(raw, dict) or public_ipv4(raw.get('ip')) != ip:
            raise ToolError('country_ip_mismatch')
        code = raw.get('country')
        if not country_code(code):
            raise ToolError('country_response_invalid')
        result.update(status='passed', country=code, error='', queried_at=utc_now(), expires_at=expires_at(86400), identity_assurance='normal_https_certificate_validation')
        image['country'] = code
        cache[key] = dict(result)
        write_json(cache_path, cache, private=True)
    except ToolError as exc:
        # Only static error codes escape, never service URLs or response data.
        result.update(error=str(exc), queried_at=utc_now(), expires_at=expires_at(60))
        if 'cache' in locals() and 'key' in locals():
            cache[key] = dict(result)
            cache['retry:' + provider] = time.time() + 30
            write_json(cache_path, cache, private=True)
    return image
