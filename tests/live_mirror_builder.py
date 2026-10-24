"""Explicit, disposable Linux check of this machine fetching raw mirror data."""
import json
import os
from pathlib import Path
import platform
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT/'src'))
from vpngate_toolbox.config import load_settings
from vpngate_toolbox.pool import build_pool
from vpngate_toolbox.mirror_source import PROVIDER

assert os.getenv('GITHUB_ACTIONS') == 'true' and platform.system() == 'Linux'
settings = load_settings(ROOT/'config/settings.json')
settings['source_urls'] = ['https://127.0.0.1:9/']
settings['mirror_fallback'] = {'enabled':True, 'max_age_hours':24}
with tempfile.TemporaryDirectory() as tmp:
    pool = build_pool(settings, Path(tmp))
    source = pool['source']
    assert source['kind'] == 'mirror' and source['retrieval'] == 'local_builder'
    assert source['mirror_provider'] == PROVIDER and source['integrity_verified']
    assert pool['nodes']
    for node in pool['nodes']:
        assert node['protocols']['openvpn']['status'] == 'not_tested'
        assert node['protocols']['openvpn']['egress'] is None
        assert node['protocols']['sstp']['expected_exit_ip'] is None
    print(json.dumps({'status':'local_linux_mirror_acquisition_passed',
                      'nodes':len(pool['nodes']), 'rejected_rows':source['rejected_rows'],
                      'snapshot_fetched_at':source['snapshot_fetched_at'],
                      'retrieved_at':source['retrieved_at'], 'data_commit':source['data_commit'],
                      'csv_sha256':source['sha256'], 'live_vpn_validated':False}))
