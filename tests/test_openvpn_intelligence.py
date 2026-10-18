"""Synthetic IP intelligence tests; no VPN or external network is contacted."""
import contextlib
import io
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'src'))
from vpngate_toolbox.cli import main
from vpngate_toolbox.common import ServiceError, ToolError
from vpngate_toolbox.config import load_settings
from vpngate_toolbox.intelligence import enrich, profile_openvpn_exit, profile_measured_exit

SETTINGS = load_settings(ROOT / 'config/settings.json')
IP = '73.1.1.1'
TEMPLATE = 'https://intelligence.example.com/{ip}'


def observation(ip=IP):
    return {'ip': ip, 'country_code': 'US', 'asn': {'asn': 'AS7922', 'name': 'Comcast', 'type': 'isp'},
            'privacy': {'is_hosting': False}}


class OpenVPNIntelligenceTests(unittest.TestCase):
    def test_socks5_city_uses_shared_evidence_and_rejects_bad_city(self):
        for raw_city, expected in [('Los Angeles', 'Los Angeles'), ({'names': {'en': 'Tokyo'}}, 'Tokyo'), (123, None), ('bad\ncity', None)]:
            with self.subTest(city=raw_city):
                raw = {**observation(), 'city': raw_city}
                image = profile_measured_exit(IP, SETTINGS, self.cache.with_name(str(len(str(raw_city))) + '.json'), 'socks5', lambda *a, **k: raw)
                self.assertEqual(image['protocol'], 'socks5')
                self.assertEqual(image['city'], expected)
                self.assertEqual(image['ip'], IP)

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.cache = Path(self.tmp.name) / 'cache.json'
        self.env = patch.dict(os.environ, {'INTELLIGENCE_URL_TEMPLATE': TEMPLATE,
                                         'INTELLIGENCE_TOKEN': 'test-secret'}, clear=True)
        self.env.start()
        self.addCleanup(self.env.stop)

    def test_not_configured_is_unknown_without_query(self):
        with patch.dict(os.environ, {'INTELLIGENCE_URL_TEMPLATE': ''}):
            image = profile_openvpn_exit(IP, SETTINGS, self.cache,
                                        lambda *a, **k: self.fail('must not query'))
        self.assertEqual(image['intelligence_status'], 'not_configured')
        self.assertEqual(image['ip_type'], 'unknown')
        self.assertEqual(image['ip'], IP)
        for field in ('country', 'asn', 'isp', 'hosting', 'residential', 'proxy', 'vpn', 'abuse'):
            self.assertIsNone(image[field])
        self.assertFalse(self.cache.exists())

    def test_real_exit_binding_evidence_and_private_cache(self):
        calls = []
        def lookup(url, **kwargs):
            calls.append((url, kwargs['headers']))
            return observation()
        image = profile_openvpn_exit(IP, SETTINGS, self.cache, lookup)
        self.assertEqual(calls, [(TEMPLATE.replace('{ip}', IP), {'X-API-Key': 'test-secret'})])
        self.assertEqual(image['protocol'], 'openvpn')
        self.assertEqual(image['ip_type'], 'likely_residential')
        self.assertEqual(image['confidence'], 'medium')
        self.assertEqual(image['reason'], 'consumer_isp_no_hosting')
        self.assertEqual(image['raw'], observation())
        for field in ('provider', 'queried_at', 'expires_at', 'rule_version', 'identity_assurance'):
            self.assertTrue(image[field])
        self.assertNotIn('test-secret', json.dumps(image) + self.cache.read_text())
        self.assertNotIn('intelligence.example.com', json.dumps(image))
        self.assertEqual(self.cache.stat().st_mode & 0o777, 0o600)

    def test_same_ip_reuses_shared_adapter_cache_and_query_time(self):
        sstp = {'protocols': {'sstp': {'status': 'passed', 'expected_exit_ip': IP}}}
        enrich([sstp], SETTINGS, self.cache, lambda *a, **k: observation())
        image = profile_openvpn_exit(IP, SETTINGS, self.cache,
                                    lambda *a, **k: self.fail('cache should be reused'))
        self.assertTrue(image['cache_used'])
        self.assertEqual(image['queried_at'], sstp['protocols']['sstp']['egress']['queried_at'])
        self.assertEqual(image['expires_at'], sstp['protocols']['sstp']['egress']['expires_at'])
        self.assertEqual(image['protocol'], 'openvpn')

    def test_new_ip_or_provider_never_inherits_old_image(self):
        profile_openvpn_exit(IP, SETTINGS, self.cache, lambda *a, **k: observation())
        calls = []
        def lookup(url, **kwargs):
            calls.append(url)
            return {'ip': '8.8.4.4'}
        image = profile_openvpn_exit('8.8.4.4', SETTINGS, self.cache, lookup)
        self.assertEqual(image['ip_type'], 'unknown')
        self.assertIsNone(image['isp'])
        self.assertEqual(len(calls), 1)
        with patch.dict(os.environ, {'INTELLIGENCE_URL_TEMPLATE': 'https://other.example.net/{ip}'}):
            image = profile_openvpn_exit(IP, SETTINGS, self.cache, lambda *a, **k: {'ip': IP})
        self.assertEqual(image['ip_type'], 'unknown')
        self.assertIsNone(image['asn'])

    def test_wrong_response_ip_is_rejected_not_cached(self):
        image = profile_openvpn_exit(IP, SETTINGS, self.cache, lambda *a, **k: observation('8.8.4.4'))
        self.assertEqual(image['intelligence_error'], 'intelligence_ip_mismatch')
        self.assertEqual(image['ip_type'], 'unknown')
        self.assertEqual(json.loads(self.cache.read_text()), {})

    def test_expired_cache_is_requeried_and_outage_keeps_unknown(self):
        profile_openvpn_exit(IP, SETTINGS, self.cache, lambda *a, **k: observation())
        cache = json.loads(self.cache.read_text())
        for entry in cache.values():
            entry['expires_at'] = '2000-01-01T00:00:00Z'
        self.cache.write_text(json.dumps(cache))
        def outage(*args, **kwargs):
            raise ServiceError('http_429')
        image = profile_openvpn_exit(IP, SETTINGS, self.cache, outage)
        self.assertEqual(image['ip_type'], 'unknown')
        self.assertEqual(image['intelligence_status'], 'failed')
        self.assertEqual(image['reason'], 'http_429')
        self.assertIsNone(image['isp'])
        refreshed = profile_openvpn_exit(IP, SETTINGS, self.cache, lambda *a, **k: {'ip': IP})
        self.assertEqual(refreshed['ip_type'], 'unknown')
        self.assertNotEqual(refreshed['expires_at'], '2000-01-01T00:00:00Z')

    def test_insufficient_or_conflicting_fields_cannot_claim_residential(self):
        for raw in ({'ip': IP}, {**observation(), 'company': {'type': 'hosting'}},
                    {**observation(), 'privacy': {'is_residential': True, 'is_hosting': True}}):
            self.cache.unlink(missing_ok=True)
            image = profile_openvpn_exit(IP, SETTINGS, self.cache, lambda *a, **k: raw)
            self.assertNotIn(image['ip_type'], ('strict_residential', 'likely_residential'))

    def test_private_baseline_rejected_before_query(self):
        for ip in ('127.0.0.1', '10.0.0.1', '::1', 'invalid'):
            with self.subTest(ip=ip), self.assertRaises(ToolError):
                profile_openvpn_exit(ip, SETTINGS, self.cache, lambda *a, **k: self.fail('must not query'))

    def test_malformed_template_is_unknown_safe_error(self):
        for template in ('https://example.com/no-placeholder', 'http://example.com/{ip}'):
            with patch.dict(os.environ, {'INTELLIGENCE_URL_TEMPLATE': template}):
                image = profile_openvpn_exit(IP, SETTINGS, self.cache, lambda *a, **k: self.fail('must not query'))
            self.assertEqual(image['ip_type'], 'unknown')
            self.assertEqual(image['intelligence_status'], 'failed')
            self.assertNotIn('example.com', json.dumps(image))

    def test_malformed_cache_is_rebuilt(self):
        for value in ([], {'bad': 'entry'}):
            self.cache.write_text(json.dumps(value))
            image = profile_openvpn_exit(IP, SETTINGS, self.cache, lambda *a, **k: observation())
            self.assertEqual(image['ip_type'], 'likely_residential')

    def test_sstp_profiles_its_measured_ip_and_keeps_protocol(self):
        image = profile_measured_exit(IP, SETTINGS, self.cache, 'sstp', lambda *a, **k: observation())
        self.assertEqual(image['protocol'], 'sstp')
        self.assertEqual(image['country'], 'US')
        with self.assertRaises(ToolError):
            profile_measured_exit(IP, SETTINGS, self.cache, 'tcp', lambda *a, **k: self.fail('must not query'))
        env_file = Path(self.tmp.name) / 'sstp.env'
        env_file.write_text('INTELLIGENCE_URL_TEMPLATE=\n')
        with patch.dict(os.environ, {}, clear=True), contextlib.redirect_stdout(io.StringIO()) as stream:
            code = main(['--config', str(ROOT / 'config/settings.json'), '--env-file', str(env_file),
                         'profile-sstp', '--ip', IP, '--cache', str(self.cache)])
        self.assertEqual(code, 0)
        image = json.loads(stream.getvalue())
        self.assertEqual(image['protocol'], 'sstp')
        self.assertEqual(image['ip_type'], 'unknown')
        self.assertIsNone(image['country'])

    def test_cli_uses_no_cf_credentials_and_does_not_write_pool(self):
        env_file = Path(self.tmp.name) / 'intelligence.env'
        env_file.write_text('INTELLIGENCE_URL_TEMPLATE=\n')
        with patch.dict(os.environ, {}, clear=True), contextlib.redirect_stdout(io.StringIO()) as stream:
            code = main(['--config', str(ROOT / 'config/settings.json'), '--env-file', str(env_file),
                         'profile-openvpn', '--ip', IP, '--cache', str(self.cache)])
        self.assertEqual(code, 0)
        self.assertEqual(json.loads(stream.getvalue())['intelligence_status'], 'not_configured')
        self.assertFalse((self.cache.parent / 'node_pool.json').exists())


if __name__ == '__main__':
    unittest.main()
