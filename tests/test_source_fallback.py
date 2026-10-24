"""Candidate copies never become external VPN/egress evidence."""
import base64
import copy
import csv
from datetime import datetime, timedelta, timezone
import hashlib
import io
import json
from pathlib import Path
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT/'src'))
from vpngate_toolbox.common import ServiceError, ToolError
from vpngate_toolbox.config import load_settings, OFFICIAL_PRIMARY_URL, OFFICIAL_BACKUP_URL
from vpngate_toolbox.mirror_source import INDEX_URL, PROVIDER
from vpngate_toolbox.pool import build_pool
from vpngate_toolbox.source_fetch import SourceFetcher, _connect_source, _SourceHTTPSConnection
from vpngate_toolbox.vpngate import validate_mirror_profile


def candidate_csv():
    stream = io.StringIO(); stream.write('*vpn_servers\n')
    writer = csv.writer(stream)
    writer.writerow(['#HostName','IP','CountryShort','OpenVPN_ConfigData_Base64','verified','ip_type'])
    writer.writerow(['test.example.invalid','8.8.8.8','US',
                     base64.b64encode(b'client\ndev tun\nproto tcp\nremote 8.8.8.8 1194\n').decode(),
                     'passed','strict_residential'])
    stream.write('*\n')
    return stream.getvalue().encode()


class MirrorFallbackTests(unittest.TestCase):
    def test_official_backup_precedes_mirror_and_keeps_verdicts_untested(self):
        def transport(url, timeout):
            self.calls.append(url)
            if url == OFFICIAL_PRIMARY_URL: raise ServiceError('https_unavailable')
            if url == OFFICIAL_BACKUP_URL: return self.data
            self.fail('mirror should not be requested when official backup works')
        pool = build_pool(self.settings, self.output, transport=transport)
        self.assertEqual(self.calls, [OFFICIAL_PRIMARY_URL, OFFICIAL_BACKUP_URL])
        self.assertEqual(pool['source']['kind'], 'official_backup')
        self.assertEqual(pool['source']['name'], OFFICIAL_BACKUP_URL)
        self.assertTrue(all(n['protocols']['openvpn']['status']=='not_tested' for n in pool['nodes']))

    def test_legacy_official_source_expands_without_rewriting_private_file(self):
        path=self.output/'settings.json'; settings=copy.deepcopy(self.settings)
        settings['source_urls']=[OFFICIAL_PRIMARY_URL]; path.write_text(json.dumps(settings))
        before=path.read_bytes()
        self.assertEqual(load_settings(path)['source_urls'],[OFFICIAL_PRIMARY_URL,OFFICIAL_BACKUP_URL])
        self.assertEqual(path.read_bytes(),before)
        for sources in ([],['https://private.example/']):
            settings['source_urls']=sources;path.write_text(json.dumps(settings))
            self.assertEqual(load_settings(path)['source_urls'],sources)

    def test_mirror_rejects_executable_external_and_obfuscated_options(self):
        base = 'client\ndev tun\nproto tcp\nremote 8.8.8.8 1194\n'
        for extra in ('plugin /tmp/a.so', 'up /tmp/run', 'config /tmp/other',
                      '--plugin a.so', 'script-security 2', 'ca /etc/shadow',
                      'log /etc/cron.d/evil', 'management 127.0.0.1 1',
                      'remote 127.0.0.1 443', 'remote localhost 443',
                      '<ca>\n</key>\nplugin a.so\n</ca>',
                      '<ca>\nplugin a.so\n</ca>', 'cipher AES-128-CBC\\\nplugin x',
                      '<ca>\nAAAA', '"plugin" a.so', 'cipher AES\x00'):
            with self.subTest(option=extra):
                with self.assertRaisesRegex(ToolError, 'unsafe_mirror_profile'):
                    validate_mirror_profile(base+extra+'\n', '8.8.8.8')
        validate_mirror_profile(base+'<ca>\n-----BEGIN CERTIFICATE-----\nAAAA==\n-----END CERTIFICATE-----\n</ca>\n', '8.8.8.8')

    def test_unsafe_mirror_csv_cannot_publish_or_write_profiles(self):
        original = base64.b64encode(b'client\ndev tun\nproto tcp\nremote 8.8.8.8 1194\n')
        dangerous = base64.b64encode(b'client\nremote 8.8.8.8 1194\nplugin /tmp/evil.so\n')
        self.data = self.data.replace(original, dangerous)
        self.index['files']['data/vpngate.csv'] = {'bytes':len(self.data),'sha256':hashlib.sha256(self.data).hexdigest()}
        before = self.previous()
        with self.assertRaisesRegex(ToolError, 'no_candidates_matching_filters'):
            build_pool(self.settings, self.output, transport=self.transport)
        self.assertEqual((self.output/'node_pool.json').read_bytes(), before)
        self.assertFalse((self.output/'configs').exists())

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(); self.addCleanup(self.tmp.cleanup)
        self.output = Path(self.tmp.name)/'pool'
        self.output.mkdir()
        self.settings = load_settings(ROOT/'config/settings.json')
        self.settings['mirror_fallback']['enabled'] = True
        self.data = candidate_csv()
        self.index = {'schema_version':1, 'data_commit':'a'*40, 'fetched_at':datetime.now(timezone.utc).isoformat(),
                      'source_url':'https://www.vpngate.net/api/iphone/', 'server_count':1,
                      'files':{'data/vpngate.csv':{'bytes':len(self.data),'sha256':hashlib.sha256(self.data).hexdigest()}},
                      'intelligence_status':'passed', 'ip_type':'strict_residential'}
        self.calls = []

    def transport(self, url, timeout):
        self.calls.append(url)
        if url in self.settings['source_urls']: raise ServiceError('https_timeout')
        if url == INDEX_URL: return json.dumps(self.index).encode()
        return self.data

    def previous(self, source=None):
        blob=json.dumps({'source':source or {},'nodes':[{'node_id':'previous-private-marker'}]}).encode()
        (self.output/'node_pool.json').write_bytes(blob)
        return blob

    def test_opt_in_required_and_failure_keeps_previous_file(self):
        self.settings['mirror_fallback']['enabled']=False
        before=self.previous()
        with self.assertRaisesRegex(ServiceError,'pool_source_unavailable'):
            build_pool(self.settings,self.output,transport=self.transport)
        self.assertEqual(self.calls,self.settings['source_urls'])
        self.assertEqual((self.output/'node_pool.json').read_bytes(),before)

    def test_original_source_success_never_contacts_mirror(self):
        def transport(url,timeout):
            self.calls.append(url); return self.data
        pool=build_pool(self.settings,self.output,transport=transport)
        self.assertEqual(self.calls,[OFFICIAL_PRIMARY_URL])
        self.assertEqual(pool['source']['kind'],'official')

    def test_local_fetch_checks_fixed_commit_and_does_not_import_evidence(self):
        pool=build_pool(self.settings,self.output,transport=self.transport)
        self.assertEqual(pool['source']['kind'],'mirror')
        self.assertEqual(pool['source']['retrieval'],'local_builder')
        self.assertEqual(pool['source']['mirror_provider'],PROVIDER)
        self.assertEqual(pool['source']['snapshot_fetched_at'],self.index['fetched_at'])
        self.assertTrue(pool['source']['integrity_verified'])
        self.assertIn('/'+'a'*40+'/data/vpngate.csv',self.calls[-1])
        node=pool['nodes'][0]
        self.assertEqual(node['protocols']['openvpn']['status'],'not_tested')
        self.assertIsNone(node['protocols']['openvpn']['egress'])
        self.assertIsNone(node['protocols']['sstp']['expected_exit_ip'])
        self.assertNotIn('intelligence_status',pool['source'])

    def test_expired_future_and_invalid_index_do_not_replace_previous(self):
        cases = [
            ({'fetched_at':(datetime.now(timezone.utc)-timedelta(hours=25)).isoformat()},'mirror_snapshot_expired'),
            ({'fetched_at':(datetime.now(timezone.utc)+timedelta(minutes=10)).isoformat()},'mirror_time_invalid'),
            ({'schema_version':True},'mirror_index_invalid'),
            ({'data_commit':'../../private'},'mirror_index_invalid'),
            ({'source_url':'https://other.example/secret'},'mirror_index_invalid'),
        ]
        original=copy.deepcopy(self.index)
        for mutation, code in cases:
            with self.subTest(code=code):
                self.index={**original,**mutation}; self.calls=[]
                before=self.previous()
                with self.assertRaisesRegex(ServiceError,code): build_pool(self.settings,self.output,transport=self.transport)
                self.assertEqual((self.output/'node_pool.json').read_bytes(),before)
                self.assertEqual(self.calls[-1],INDEX_URL)

    def test_older_snapshot_and_conflicting_same_time_are_rejected(self):
        old={'mirror_provider':PROVIDER,'snapshot_fetched_at':(datetime.now(timezone.utc)+timedelta(seconds=30)).isoformat()}
        before=self.previous(old)
        with self.assertRaisesRegex(ServiceError,'mirror_snapshot_rollback'): build_pool(self.settings,self.output,transport=self.transport)
        self.assertEqual((self.output/'node_pool.json').read_bytes(),before)
        old['snapshot_fetched_at']=self.index['fetched_at']; old['sha256']='b'*64
        self.previous(old)
        with self.assertRaisesRegex(ServiceError,'mirror_snapshot_conflict'): build_pool(self.settings,self.output,transport=self.transport)

    def test_corrupted_file_is_not_published(self):
        before=self.previous(); self.data=self.data[:-5]
        with self.assertRaisesRegex(ServiceError,'mirror_integrity_failed'): build_pool(self.settings,self.output,transport=self.transport)
        self.assertEqual((self.output/'node_pool.json').read_bytes(),before)

    def test_same_commit_cdn_fallback_only_on_download_failure(self):
        def transport(url,timeout):
            if '/'+('a'*40)+'/' in url:
                self.calls.append(url); raise ServiceError('https_unavailable')
            return self.transport(url,timeout)
        pool=build_pool(self.settings,self.output,transport=transport)
        self.assertIn('@'+'a'*40+'/data/vpngate.csv',self.calls[-1])
        self.assertEqual(pool['source']['data_commit'],'a'*40)

    def test_country_filter_is_not_bypassed_to_fill_slots(self):
        before=self.previous(); self.settings['countries']=['JP']
        with self.assertRaisesRegex(ToolError,'no_candidates_matching_filters'): build_pool(self.settings,self.output,transport=self.transport)
        self.assertEqual((self.output/'node_pool.json').read_bytes(),before)

    def test_late_primary_result_cannot_publish_after_mirror(self):
        self.settings['pool_source_timeout_seconds']=0.02
        def transport(url,timeout):
            if url in self.settings['source_urls']:
                time.sleep(0.2); return self.data.replace(b'test.example',b'late.example')
            return self.transport(url,timeout)
        pool=build_pool(self.settings,self.output,transport=transport)
        before=(self.output/'node_pool.json').read_bytes()
        self.assertEqual(pool['source']['kind'],'mirror')
        time.sleep(0.25)
        self.assertEqual((self.output/'node_pool.json').read_bytes(),before)

    def test_wait_bound_does_not_join_slow_transport(self):
        def slow(url,timeout):
            time.sleep(0.6); return b'late'
        start=time.monotonic()
        with self.assertRaisesRegex(ServiceError,'source_request_timeout'): SourceFetcher(slow).fetch('https://example.com',0.03)
        self.assertLess(time.monotonic()-start,0.4)

    def test_dead_first_address_does_not_consume_whole_connect_budget(self):
        attempts, timeouts = [], []
        class Connection:
            def settimeout(self,value): timeouts.append(value)
            def connect(self,address):
                attempts.append(address)
                if len(attempts)==1: raise TimeoutError('first unavailable')
            def close(self): pass
        addresses=[(2,1,6,'',('127.0.0.'+str(n),443)) for n in (1,2,3)]
        with patch('vpngate_toolbox.source_fetch.socket.getaddrinfo',return_value=addresses), \
                patch('vpngate_toolbox.source_fetch.socket.socket',side_effect=lambda *args:Connection()):
            _connect_source(('example.com',443),10)
        self.assertEqual(len(attempts),2)
        self.assertLess(timeouts[0],4)

    def test_source_tls_still_checks_hostname_and_certificate(self):
        import ssl
        connection=_SourceHTTPSConnection('example.com',timeout=10)
        self.assertTrue(connection._context.check_hostname)
        self.assertEqual(connection._context.verify_mode,ssl.CERT_REQUIRED)

    def test_legacy_settings_default_enabled_and_explicit_opt_out_preserved(self):
        settings=copy.deepcopy(self.settings); settings.pop('mirror_fallback');settings.pop('pool_source_timeout_seconds')
        path=Path(self.tmp.name)/'settings.json';path.write_text(json.dumps(settings))
        value=load_settings(path)
        self.assertEqual(value['mirror_fallback'],{'enabled':True,'max_age_hours':24})
        settings['mirror_fallback']={'enabled':False,'max_age_hours':24};path.write_text(json.dumps(settings))
        self.assertFalse(load_settings(path)['mirror_fallback']['enabled'])
        settings['mirror_fallback']={'enabled':'true','max_age_hours':24};path.write_text(json.dumps(settings))
        with self.assertRaisesRegex(ToolError,'invalid_setting_mirror_fallback'):load_settings(path)


if __name__ == '__main__': unittest.main()
