"""Opt-in raw CSV mirror; all retrieval and validation execute locally."""
from datetime import datetime, timezone
import hashlib
import json
import re

from .common import ServiceError

PROVIDER = 'GeorgeXie2333/vpngate-list-mirror'
INDEX_URL = 'https://raw.githubusercontent.com/' + PROVIDER + '/main/latest.json'


def timestamp(value):
    if not isinstance(value, str):
        raise ValueError('timestamp')
    parsed = datetime.fromisoformat(value.replace('Z', '+00:00'))
    if parsed.tzinfo is None:
        raise ValueError('timezone')
    return parsed


def fetch_mirror(fetcher, timeout, max_age_hours, previous=None, now=None):
    try:
        raw = fetcher.fetch(INDEX_URL, timeout, limit=65_536)
    except ServiceError as exc:
        code = 'mirror_index_invalid' if str(exc) == 'response_too_large' else 'mirror_index_unavailable'
        raise ServiceError(code) from None
    try:
        index = json.loads(raw)
        if type(index['schema_version']) is not int or index['schema_version'] != 1:
            raise ValueError('schema')
        commit = index['data_commit']
        if not isinstance(commit, str) or not re.fullmatch('[a-f0-9]{40}', commit):
            raise ValueError('commit')
        if index.get('source_url') != 'https://www.vpngate.net/api/iphone/':
            raise ValueError('upstream')
        fetched = timestamp(index['fetched_at'])
        description = index['files']['data/vpngate.csv']
        size, sha = description['bytes'], description['sha256']
        if type(size) is not int or not 1 <= size <= 12_000_000:
            raise ValueError('size')
        if not isinstance(sha, str) or not re.fullmatch('[a-f0-9]{64}', sha):
            raise ValueError('hash')
        count = index['server_count']
        if type(count) is not int or not 1 <= count <= 100_000:
            raise ValueError('count')
    except (ValueError, TypeError, KeyError):
        raise ServiceError('mirror_index_invalid') from None
    now = now or datetime.now(timezone.utc)
    age = (now - fetched).total_seconds()
    if age < -300:
        raise ServiceError('mirror_time_invalid')
    if age > max_age_hours * 3600:
        raise ServiceError('mirror_snapshot_expired')
    previous = previous or {}
    if previous.get('mirror_provider') == PROVIDER:
        try:
            older = timestamp(previous.get('snapshot_fetched_at'))
        except ValueError:
            older = None
        if older is not None and fetched < older:
            raise ServiceError('mirror_snapshot_rollback')
        if older == fetched and previous.get('sha256') not in (None, sha):
            raise ServiceError('mirror_snapshot_conflict')
    urls = ['https://raw.githubusercontent.com/' + PROVIDER + '/' + commit + '/data/vpngate.csv',
            'https://cdn.jsdelivr.net/gh/' + PROVIDER + '@' + commit + '/data/vpngate.csv']
    for url in urls:
        try:
            data = fetcher.fetch(url, timeout, limit=size)
        except ServiceError as exc:
            if str(exc) == 'response_too_large':
                raise ServiceError('mirror_integrity_failed') from None
            continue
        if len(data) != size or hashlib.sha256(data).hexdigest() != sha:
            raise ServiceError('mirror_integrity_failed')
        return data, {'name': url, 'kind': 'mirror', 'mirror_provider': PROVIDER,
                      'snapshot_fetched_at': fetched.astimezone(timezone.utc).isoformat(),
                      'data_commit': commit, 'integrity_verified': True,
                      'publisher_count': count}
    raise ServiceError('mirror_download_failed')
