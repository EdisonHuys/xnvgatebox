"""Country-only evidence stays independent of residential classification."""
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'src'))
from vpngate_toolbox.country import attach_country
from vpngate_toolbox.common import ServiceError

class CountryTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(); self.addCleanup(self.tmp.cleanup)
        self.cache = Path(self.tmp.name)/'country.json'
        self.env = patch.dict(os.environ, {}, clear=True); self.env.start(); self.addCleanup(self.env.stop)
        self.image = {'ip':'8.8.8.8','country':None,'protocol':'openvpn','ip_type':'unknown','residential':None,'intelligence_status':'not_configured'}

    def test_country_needs_no_intelligence_key_and_never_upgrades_type(self):
        calls=[]
        def query(url, **kwargs):
            calls.append((url,kwargs)); return {'ip':'8.8.8.8','country':'US'}
        result=attach_country(self.image,self.cache,query)
        self.assertEqual(result['country'],'US'); self.assertEqual(result['ip_type'],'unknown')
        self.assertIsNone(result['residential']); self.assertEqual(result['intelligence_status'],'not_configured')
        self.assertEqual(calls[0][0],'https://api.country.is/8.8.8.8')
        self.assertEqual(calls[0][1]['headers'],{})
        self.assertEqual(self.cache.stat().st_mode & 0o777,0o600)
        again=attach_country(self.image,self.cache,lambda *a,**k:self.fail('must reuse cache'))
        self.assertEqual(again['country_evidence']['queried_at'],result['country_evidence']['queried_at'])
        self.assertTrue(again['country_evidence']['cache_used'])

    def test_mismatch_invalid_country_and_tls_error_cannot_publish_country(self):
        for raw in ({'ip':'1.1.1.1','country':'US'},{'ip':'8.8.8.8','country':'ZZ'},{'ip':'8.8.8.8','country':'USA'}):
            self.cache.unlink(missing_ok=True)
            result=attach_country(self.image,self.cache,lambda *a,**k:raw)
            self.assertIsNone(result['country']); self.assertEqual(result['country_evidence']['status'],'failed')
        self.cache.unlink(missing_ok=True)
        calls=[]
        def outage(*a,**k):calls.append(1);raise ServiceError('https_unavailable')
        result=attach_country(self.image,self.cache,outage)
        self.assertIsNone(result['country'])
        attach_country({**self.image,'ip':'1.1.1.1'},self.cache,outage)
        self.assertEqual(len(calls),1,'service outage circuit did not suppress a burst')

    def test_stale_cache_and_new_ip_require_new_evidence_and_can_be_disabled(self):
        attach_country(self.image,self.cache,lambda *a,**k:{'ip':'8.8.8.8','country':'US'})
        cache=json.loads(self.cache.read_text())
        for value in cache.values():
            if isinstance(value,dict):value['expires_at']='2000-01-01T00:00:00Z'
        self.cache.write_text(json.dumps(cache))
        result=attach_country(self.image,self.cache,lambda *a,**k:{'ip':'8.8.8.8','country':'DE'})
        self.assertEqual(result['country'],'DE')
        with patch.dict(os.environ,{'COUNTRY_URL_TEMPLATE':''}):
            result=attach_country(self.image,self.cache,lambda *a,**k:self.fail('disabled query'))
        self.assertIsNone(result['country']);self.assertEqual(result['country_evidence']['status'],'not_configured')
