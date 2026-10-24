#!/usr/bin/env bash
set -euo pipefail
[[ -f /.dockerenv && $(cat /proc/1/comm) == systemd && -d /package ]] || exit 1
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
run_install() {
  if ! bash /src/vps/install.sh --package /package/xnvgatebox-linux-amd64.tar.gz >/tmp/install.log 2>&1; then
    tail -120 /tmp/install.log
    journalctl -u xnvgatebox-fanout --no-pager -n 40
    /usr/local/bin/xnvgatebox-fanout -check-state -dir /var/lib/xnvgatebox/fanout || true
    exit 1
  fi
}
# Deterministic upstream outage: a closed loopback HTTPS port, no TLS bypass or
# public host modification. Keep it configured through install and update.
mkdir -p /etc/xnvgatebox
# Python is an installer dependency and may not exist in the pristine image.
sed -e 's#https://www.vpngate.net/api/iphone/#https://127.0.0.1:9/#' \
    -e 's/"node_timeout_seconds": 30/"node_timeout_seconds": 2/' \
    /src/config/settings.json > /etc/xnvgatebox/pool.json
chmod 600 /etc/xnvgatebox/pool.json
run_install
grep -q '先完成后台安装' /tmp/install.log
test ! -e /var/lib/xnvgatebox/pool/node_pool.json
systemctl is-active --quiet xnvgatebox-fanout.service
systemctl is-enabled --quiet xnvgatebox-pool.timer
python3 /src/tests/distro_verify.py save
ip netns add xnv-ci
ip netns exec xnv-ci ip link set lo up
ip netns exec xnv-ci iptables -A OUTPUT -j REJECT
ip netns exec xnv-ci iptables -C OUTPUT -j REJECT
ip netns delete xnv-ci
if command -v rpm >/dev/null; then
  rpm -q xnvgatebox-sstp-client
  sstpc --version
  getent passwd sstpc >/dev/null
  ppp_version=$(rpm -q --qf '%{VERSION}' ppp)
  # dryrun loads the real plugin without a VPN peer or kernel PPP negotiation.
  timeout 10 pppd plugin "$(rpm --eval '%{_libdir}')/pppd/$ppp_version/sstp-pppd-plugin.so" dryrun noauth notty >/tmp/ppp.log 2>&1 || { cat /tmp/ppp.log; exit 1; }
fi
run_install
python3 /src/tests/distro_verify.py compare
test ! -e /var/lib/xnvgatebox/pool/node_pool.json
python3 - <<'PY'
import json
from pathlib import Path
status = json.loads(Path('/var/lib/xnvgatebox-deploy/status.json').read_text())
assert status['status'] == 'completed' and status['warning'] == 'pool_not_ready'
PY
# Recover through a controlled HTTPS source. Trust only its generated CI CA in
# this disposable service; TLS verification remains enabled. No live VPN claim.
command -v openssl >/dev/null || { if command -v apt-get >/dev/null; then apt-get install -y openssl; else dnf -y install openssl; fi; }
python3 /src/tests/pool_source_fixture.py >/tmp/source-fixture.log 2>&1 &
source_pid=$!
trap 'kill "$source_pid" 2>/dev/null || true' EXIT
for attempt in {1..30}; do
  test ! -f /tmp/xnv-ci-source/port || break
  sleep 1
done
test -f /tmp/xnv-ci-source/port || { cat /tmp/source-fixture.log; exit 1; }
python3 - <<'PY'
import json
from pathlib import Path
config = json.loads(Path('/etc/xnvgatebox/pool.json').read_text())
config['source_urls'] = ['https://127.0.0.1:' + Path('/tmp/xnv-ci-source/port').read_text() + '/']
Path('/etc/xnvgatebox/pool.json').write_text(json.dumps(config))
PY
mkdir -p /run/systemd/system/xnvgatebox-pool.service.d
printf '%s\n' '[Service]' 'Environment=SSL_CERT_FILE=/tmp/xnv-ci-source/cert.pem' > /run/systemd/system/xnvgatebox-pool.service.d/fixture.conf
systemctl daemon-reload
if ! systemctl start xnvgatebox-pool.service; then
  journalctl -u xnvgatebox-pool.service --no-pager -n 35
  systemctl show xnvgatebox-pool.service -p Result -p ExecMainStatus -p ActiveState -p SubState
  exit 1
fi
python3 - <<'PY'
import json
from pathlib import Path
pool = json.loads(Path('/var/lib/xnvgatebox/pool/node_pool.json').read_text())
assert pool['nodes'] and pool['generated_at'] and pool['source']['name'].startswith('https://')
assert pool['nodes'][0]['hostname'] == 'ci-synthetic.example.invalid'
PY
run_install
python3 /src/tests/distro_verify.py compare
python3 /src/tests/distro_verify.py disable
run_install
python3 /src/tests/distro_verify.py disabled
rm -rf /run/systemd/system/xnvgatebox-pool.service.d
systemctl daemon-reload
test -n "$(find /var/backups/xnvgatebox -name project.tar.gz -print -quit)"
xnvgatebox uninstall --yes --purge --legacy
test ! -e /usr/local/lib/xnvgatebox
test ! -e /var/lib/xnvgatebox
test ! -e /var/backups/xnvgatebox
echo 'PASS: source outage install/update, real subscription, controlled HTTPS recovery, normal update, network isolation, uninstall'
