#!/usr/bin/env bash
set -euo pipefail
[[ -f /.dockerenv && $(cat /proc/1/comm) == systemd && -d /package ]] || exit 1
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
run_install() {
  if ! bash /src/vps/install.sh --package /package/xnvgatebox-linux-amd64.tar.gz >/tmp/install.log 2>&1; then
    tail -120 /tmp/install.log
    journalctl -u xnvgatebox-fanout --no-pager -n 40
    exit 1
  fi
}
run_install
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
  ppp_version=$(rpm -q --qf '%{VERSION}' ppp)
  # dryrun loads the real plugin without a VPN peer or kernel PPP negotiation.
  pppd plugin "$(rpm --eval '%{_libdir}')/pppd/$ppp_version/sstp-pppd-plugin.so" dryrun noauth >/tmp/ppp.log 2>&1 || { cat /tmp/ppp.log; exit 1; }
fi
run_install
python3 /src/tests/distro_verify.py compare
test -n "$(find /var/backups/xnvgatebox -name project.tar.gz -print -quit)"
xnvgatebox uninstall --yes --purge --legacy
test ! -e /usr/local/lib/xnvgatebox
test ! -e /var/lib/xnvgatebox
test ! -e /var/backups/xnvgatebox
echo 'PASS: real dependencies, systemd install/update, private settings, network isolation, uninstall'
