import assert from 'node:assert/strict';
import { handleRequest } from '../worker/control/worker.mjs';
import { parseVless } from '../worker/control/selfhosted_edge.mjs';

const future = new Date(Date.now() + 3600_000).toISOString();
const manifest = {
  schema_version: 1,
  kind: 'vpngate_cf_validated_nodes',
  generated_at: new Date(Date.now() - 60_000).toISOString(),
  expires_at: future,
  scope: 'tcp_ipv4_ws',
  data_plane: { host: 'vpngate-edge.example.com', port: 443, base_path: '/' },
  nodes: [{
    node_id: 'vpngate-us-001', hostname: 'public-vpn-1.opengw.net', country: 'US',
    sstp_host: 'public-vpn-1.opengw.net', sstp_port: 443,
    expected_exit_ip: '73.1.1.1', actual_exit_ip: '73.1.1.1',
    asn: 'AS7922', isp: 'Comcast', ip_type: 'likely_residential', latency_ms: 120,
    verified_at: new Date(Date.now() - 60_000).toISOString(), expires_at: future,
  }],
};

const env = {
  VLESS_UUID: '12345678-1234-4123-8123-123456789abc', ADMIN_TOKEN: 'admin-secret', SUB_TOKEN: 'sub-secret',
  VPN_USERNAME: 'vpn', VPN_PASSWORD: 'p@ss&=?', MANIFEST: { async get(key) { assert.equal(key, 'manifest:current'); return manifest; } },
};
const request = (path, init = {}) => new Request('https://control.example.com' + path, init);
const auth = { authorization: 'Bearer admin-secret' };

const vlessHead = new Uint8Array(26);
vlessHead[0] = 1;
vlessHead.set('12345678123441238123123456789abc'.match(/../g).map(x => Number.parseInt(x, 16)), 1);
vlessHead[17] = 0;
vlessHead[18] = 1;
vlessHead[19] = 0x01;
vlessHead[20] = 0xbb;
vlessHead[21] = 1;
vlessHead.set([93, 184, 216, 34], 22);
assert.deepEqual(parseVless(vlessHead, env.VLESS_UUID), { version: 1, host: '93.184.216.34', port: 443, initial: new Uint8Array(0) });

let response = await handleRequest(request('/health'), env);
assert.equal(response.status, 200);
assert.equal((await response.json()).manifest_ready, true);

response = await handleRequest(request('/api/status'), env);
assert.equal(response.status, 401);

response = await handleRequest(request('/api/status', { headers: auth }), env);
assert.equal(response.status, 200);
assert.equal((await response.json()).nodes[0].expected_exit_ip, '73.1.1.1');

response = await handleRequest(request('/sub?token=wrong'), env);
assert.equal(response.status, 401);
response = await handleRequest(request('/sub?token=sub-secret'), env);
assert.equal(response.status, 200);
const links = atob((await response.text()).trim()).trim().split('\n');
assert.equal(links.length, 1);
assert.match(links[0], /^vless:\/\/12345678-1234-4123-8123-123456789abc@control\.example\.com:443\?/);
const path = new URL(links[0]).searchParams.get('path');
assert.equal(new URL('https://edge.invalid' + path).searchParams.get('sstp'), 'vpn:p%40ss%26%3D%3F@public-vpn-1.opengw.net:443');
assert.equal(new URL('https://edge.invalid' + path).searchParams.get('globalproxy'), '1');

const stale = structuredClone(manifest);
stale.expires_at = '2000-01-01T00:00:00Z';
response = await handleRequest(request('/sub?token=sub-secret'), { ...env, MANIFEST: { async get() { return stale; } } });
assert.equal(response.status, 503);

const invalid = structuredClone(manifest);
invalid.nodes[0].sstp_host = 'attacker.example.com';
response = await handleRequest(request('/sub?token=sub-secret'), { ...env, MANIFEST: { async get() { return invalid; } } });
assert.equal(response.status, 503);

const unifiedEnv = {
  ...env,
  PUBLIC_HOST: 'xnvgatebox.example.workers.dev',
  CHECKER_TOKEN: 'checker-secret',
  __checkSstp: async proxy => ({ exitIp: '73.1.1.1', assignedIp: '10.0.0.2', proxy }),
  __handleVlessWebSocket: async () => new Response('self-hosted', { status: 200 }),
};
response = await handleRequest(request('/health'), unifiedEnv);
const unifiedHealth = await response.json();
assert.equal(unifiedHealth.service, 'xnvgatebox');
assert.deepEqual(unifiedHealth.modules, { management: true, subscription: true, checker: true, data_plane: true });

const checkerPath = '/check?proxy=' + encodeURIComponent('sstp://vpn:vpn@public-vpn-1.opengw.net:443');
response = await handleRequest(request(checkerPath, { headers: { authorization: 'Bearer checker-secret' } }), unifiedEnv);
assert.equal(response.status, 200);
const checkerResult = await response.json();
assert.equal(checkerResult.type, 'sstp');
assert.equal(checkerResult.hostname, 'public-vpn-1.opengw.net');
assert.equal(checkerResult.port, 443);
assert.equal(checkerResult.exit.ip, '73.1.1.1');
const uuidCheckerEnv = { ...unifiedEnv, CHECKER_TOKEN: '' };
response = await handleRequest(request(checkerPath, { headers: { authorization: 'Bearer ' + uuidCheckerEnv.VLESS_UUID } }), uuidCheckerEnv);
assert.equal(response.status, 200);
const edgePath = '/?sstp=' + encodeURIComponent('vpn:vpn@public-vpn-1.opengw.net:443') + '&globalproxy=1';
response = await handleRequest(request(edgePath, { headers: { upgrade: 'websocket' } }), unifiedEnv);
assert.equal(response.status, 200);
response = await handleRequest(request('/sub?token=sub-secret'), unifiedEnv);
const unifiedLinks = atob((await response.text()).trim()).trim().split('\n');
assert.match(unifiedLinks[0], /@control\.example\.com:443\?/);

// Changing the subscription origin must change all entrance names even with
// stale PUBLIC_HOST/DATA_PLANE_HOST/manifest values left by an old deployment.
for (const origin of ['https://custom-a.example.com', 'https://custom-b.example.net:8443']) {
  const res = await handleRequest(new Request(origin + '/sub?token=sub-secret', {
    headers: { 'x-forwarded-host': 'untrusted.example.com' },
  }), { ...unifiedEnv, DATA_PLANE_HOST: 'old.example.com', DATA_PLANE_PORT: '2053' });
  assert.equal(res.status, 200);
  const link = new URL(atob((await res.text()).trim()).trim());
  const incoming = new URL(origin);
  assert.equal(link.hostname, incoming.hostname);
  assert.equal(link.port, incoming.port || '443');
  assert.equal(link.searchParams.get('sni'), incoming.hostname);
  assert.equal(link.searchParams.get('host'), incoming.hostname);
  assert.equal(link.searchParams.get('path'), path);
}
response = await handleRequest(new Request('http://control.example.com/sub?token=sub-secret'), unifiedEnv);
assert.equal(response.status, 503);
console.log('Cloudflare unified Worker tests passed');
