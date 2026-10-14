#!/usr/bin/env bash
set -euo pipefail
umask 077
[[ $(id -u) == 0 ]] || { echo '请以 root 运行'; exit 1; }
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || { echo '当前发布包支持 Linux amd64'; exit 1; }
package=''
while (($#)); do
  case "$1" in
    --package) package=$2; shift 2 ;;
    --mode) [[ $2 == vps ]] || { echo '仅支持 --mode vps'; exit 1; }; shift 2 ;;
    *) echo "未知参数: $1"; exit 1 ;;
  esac
done
command -v systemctl >/dev/null || { echo '需要 systemd'; exit 1; }
command -v apt-get >/dev/null || { echo '当前安装入口支持 Debian / Ubuntu'; exit 1; }
DEBIAN_FRONTEND=noninteractive apt-get update -qq
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ca-certificates curl python3 openvpn iproute2 iptables
task_tmp=$(mktemp -d)
trap 'rm -rf "$task_tmp"' EXIT
if [[ -z $package ]]; then
  release='https://github.com/hanhaoops/xnvgatebox/releases/latest/download'
  curl -fLsS --retry 3 "$release/xnvgatebox-linux-amd64.tar.gz" -o "$task_tmp/xnvgatebox-linux-amd64.tar.gz"
  curl -fLsS --retry 3 "$release/checksums.txt" -o "$task_tmp/checksums.txt"
  package="$task_tmp/xnvgatebox-linux-amd64.tar.gz"
else
  package=$(realpath "$package")
  cp "$(dirname "$package")/checksums.txt" "$task_tmp/checksums.txt"
  cp "$package" "$task_tmp/xnvgatebox-linux-amd64.tar.gz"
  package="$task_tmp/xnvgatebox-linux-amd64.tar.gz"
fi
(cd "$task_tmp" && sha256sum --check checksums.txt)
mkdir "$task_tmp/unpack"
# Reject paths/links before extracting the root-owned deployment package.
python3 - "$package" "$task_tmp/unpack" <<'PY'
import sys,tarfile,pathlib,json,hashlib
base=pathlib.Path(sys.argv[2])
with tarfile.open(sys.argv[1]) as t:
    for member in t.getmembers():
        p=pathlib.PurePosixPath(member.name)
        if p.is_absolute() or '..' in p.parts or not (member.isfile() or member.isdir()):
            raise SystemExit('invalid package entry')
    t.extractall(base)
for name,want in json.loads((base/'package-sha256.json').read_text()).items():
    if hashlib.sha256((base/name).read_bytes()).hexdigest()!=want:
        raise SystemExit('package member digest mismatch')
PY
stamp=$(date -u +%Y%m%d-%H%M%S)
mkdir -p /var/backups/xnvgatebox /var/lib/xnvgatebox/fanout /var/lib/xnvgatebox/pool /etc/xnvgatebox
if [[ -e /usr/local/lib/xnvgatebox || -e /etc/systemd/system/xnvgatebox-fanout.service ]]; then
  files=(var/lib/xnvgatebox)
  for path in usr/local/lib/xnvgatebox etc/default/xnvgatebox-fanout etc/systemd/system/xnvgatebox-fanout.service usr/local/bin/xnvgatebox-fanout; do
    [[ ! -e /$path ]] || files+=("$path")
  done
  tar -C / -czf "/var/backups/xnvgatebox/$stamp.tar.gz" "${files[@]}"
fi
[[ -f /etc/xnvgatebox/pool.json ]] || cp "$task_tmp/unpack/config/settings.json" /etc/xnvgatebox/pool.json
# Build the first full candidate pool before stopping the existing manager.
python3 "$task_tmp/unpack/scripts/toolbox.py" --config /etc/xnvgatebox/pool.json pool --output /var/lib/xnvgatebox/pool
install -d -m 755 /usr/local/lib/xnvgatebox
cp -a "$task_tmp/unpack/." /usr/local/lib/xnvgatebox/
install -m 755 /usr/local/lib/xnvgatebox/deploy/xnvgatebox /usr/local/bin/xnvgatebox
[[ -f /etc/default/xnvgatebox-fanout ]] || cat > /etc/default/xnvgatebox-fanout <<'ENV'
FANOUT_VALIDATED_POOL_FILE=/var/lib/xnvgatebox/pool/node_pool.json
FANOUT_SLOT_PORT_BASE=17928
ENV
# Disable the old upstream manager service before replacing its binary.
systemctl stop xnvgatebox-fanout.service 2>/dev/null || true
install -m 755 /usr/local/lib/xnvgatebox/bin/xnvgatebox-manager /usr/local/bin/xnvgatebox-fanout
install -d -m 700 /var/lib/xnvgatebox/fanout/bin
install -m 700 /usr/local/lib/xnvgatebox/bin/xray /var/lib/xnvgatebox/fanout/bin/xray
# Preserve existing access/password/subscription settings. New installs allow
# direct VPS access and show unknown classification without calling it home IP.
[[ -f /var/lib/xnvgatebox/fanout/settings.json ]] || echo '{"port":8899,"listen_addr":"0.0.0.0","residential_only":false}' > /var/lib/xnvgatebox/fanout/settings.json
install -m 644 /usr/local/lib/xnvgatebox/deploy/*.service /etc/systemd/system/
install -m 644 /usr/local/lib/xnvgatebox/deploy/*.timer /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now xnvgatebox-pool.timer
systemctl enable xnvgatebox-fanout.service
systemctl restart xnvgatebox-fanout.service
systemctl is-active --quiet xnvgatebox-fanout.service
echo '安装完成。查看管理地址和密码：xnvgatebox access'
echo '管理界面：新建出口 → 新建节点（VLESS/REALITY）→ 绑定出口 → 订阅。'
