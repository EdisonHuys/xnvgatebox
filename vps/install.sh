#!/usr/bin/env bash
set -euo pipefail
umask 077
[[ $(id -u) == 0 ]] || { echo '请以 root 运行'; exit 1; }
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || { echo '当前发布包支持 Linux amd64'; exit 1; }
package=''; release_tag=''; job_id=''
while (($#)); do
  case "$1" in
    --package|--version|--job-id|--mode)
      (($# >= 2)) || { echo "参数缺少值: $1"; exit 1; }
      case "$1" in
        --package) package=$2 ;;
        --version) release_tag=$2 ;;
        --job-id) job_id=$2 ;;
        --mode) [[ $2 == vps ]] || { echo '仅支持 --mode vps'; exit 1; } ;;
      esac
      shift 2 ;;
    *) echo "未知参数: $1"; exit 1 ;;
  esac
done
[[ -z $release_tag || $release_tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo '版本须为 vX.Y.Z'; exit 1; }
[[ -z $job_id || $job_id =~ ^[A-Za-z0-9-]{8,64}$ ]] || { echo '更新任务编号无效'; exit 1; }
command -v systemctl >/dev/null || { echo '需要 systemd'; exit 1; }
command -v apt-get >/dev/null || { echo '当前安装入口支持 Debian / Ubuntu'; exit 1; }
command -v flock >/dev/null || { echo '请先安装 util-linux'; exit 1; }
mkdir -p /run/lock
exec 9>/run/lock/xnvgatebox-deploy.lock
flock -n 9 || { echo '已有安装或更新任务运行，请稍后查看状态'; exit 1; }
missing=0
for dependency in curl python3 openvpn ip iptables sstpc pppd unshare mount; do
  command -v "$dependency" >/dev/null || missing=1
done
if ((missing)); then
  DEBIAN_FRONTEND=noninteractive apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install --no-install-recommends -y -qq ca-certificates curl python3 openvpn iproute2 iptables sstp-client ppp util-linux
fi
[[ -n $job_id ]] || job_id=$(python3 -c 'import secrets;print(secrets.token_hex(12))')
status_helper=/usr/local/lib/xnvgatebox/deploy/installer.py
[[ -f $status_helper ]] || status_helper=/var/lib/xnvgatebox-deploy/recover.py
if [[ -f $status_helper ]]; then
  python3 "$status_helper" init --job-id "$job_id"
fi
mark_phase() {
  if [[ -f $status_helper ]]; then
    python3 "$status_helper" mark --job-id "$job_id" --phase "$1" --target "$release_tag"
  fi
}
task_tmp=$(mktemp -d)
on_exit() {
  task_code=$?
  if ((task_code)); then mark_phase failed || true; fi
  rm -rf "$task_tmp"
  exit "$task_code"
}
trap on_exit EXIT
if [[ -f /var/lib/xnvgatebox-deploy/transaction.json ]]; then
  [[ -f /var/lib/xnvgatebox-deploy/recover.py ]] || { echo '存在未完成部署且缺少恢复工具，保留现场'; exit 1; }
  python3 /var/lib/xnvgatebox-deploy/recover.py recover --job-id "$job_id"
fi
mark_phase preparing
if [[ -z $package ]]; then
  if [[ -z $release_tag ]]; then
    curl -fLsS --retry 3 --connect-timeout 15 --max-time 60 -H 'Accept: application/vnd.github+json' \
      https://api.github.com/repos/hanhaoops/xnvgatebox/releases/latest -o "$task_tmp/release.json"
    release_tag=$(python3 - "$task_tmp/release.json" <<'PY'
import json,re,sys
r=json.load(open(sys.argv[1]));tag=r.get('tag_name','')
if not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+',tag) or r.get('draft') or r.get('prerelease'):raise SystemExit('没有可用正式发布')
print(tag)
PY
)
  fi
  release_url="https://github.com/hanhaoops/xnvgatebox/releases/download/$release_tag"
  mark_phase downloading
  curl -fLsS --retry 3 --connect-timeout 15 --max-time 180 "$release_url/xnvgatebox-linux-amd64.tar.gz" -o "$task_tmp/xnvgatebox-linux-amd64.tar.gz"
  curl -fLsS --retry 3 --connect-timeout 15 --max-time 60 "$release_url/checksums.txt" -o "$task_tmp/checksums.txt"
else
  package=$(realpath "$package")
  cp "$(dirname "$package")/checksums.txt" "$task_tmp/checksums.txt"
  cp "$package" "$task_tmp/xnvgatebox-linux-amd64.tar.gz"
fi
mark_phase verifying
# Check only the expected member; never let checksum text choose local paths.
python3 - "$task_tmp" "$release_tag" <<'PY'
import hashlib,json,pathlib,re,sys,tarfile
base=pathlib.Path(sys.argv[1]);expected=sys.argv[2];name='xnvgatebox-linux-amd64.tar.gz'
lines=[line.split() for line in (base/'checksums.txt').read_text().splitlines()]
matches=[v[0].lower() for v in lines if len(v)==2 and v[1].lstrip('*')==name]
if len(matches)!=1 or not re.fullmatch('[a-f0-9]{64}',matches[0]) or hashlib.sha256((base/name).read_bytes()).hexdigest()!=matches[0]:raise SystemExit('安装包校验失败')
with tarfile.open(base/name) as t:
    entries=t.getmembers()
    if len(entries)>10000 or sum(m.size for m in entries)>512*1024*1024:raise SystemExit('安装包过大')
    names=set()
    for m in entries:
        p=pathlib.PurePosixPath(m.name)
        if p.is_absolute() or '..' in p.parts or not p.parts or str(p)!=m.name.rstrip('/') or m.name in names or not (m.isfile() or m.isdir()):raise SystemExit('安装包路径或类型无效')
        names.add(m.name)
    files={m.name:t.extractfile(m).read() for m in entries if m.isfile()}
    sums=json.loads(files['package-sha256.json'])
    if set(sums)!=set(files)-{'package-sha256.json'} or any(hashlib.sha256(files[n]).hexdigest()!=h for n,h in sums.items()):raise SystemExit('安装包文件清单校验失败')
    build=json.loads(files['BUILD.json']);version=files['VERSION'].decode().strip()
    if version!=build.get('version') or (expected and (version!=expected or build.get('source_dirty') is not False or build.get('manager_origin')!='built_from_source')):raise SystemExit('安装包版本或源码信息不匹配')
    if 'deploy/installer.py' not in files:raise SystemExit('所选旧版本不支持完整包更新，请使用对应版本的安装说明')
    out=base/'unpack';out.mkdir(mode=0o700)
    for m in entries:
        p=out/m.name
        if m.isdir():p.mkdir(parents=True,exist_ok=True,mode=0o700)
        else:p.parent.mkdir(parents=True,exist_ok=True,mode=0o700);p.write_bytes(files[m.name]);p.chmod(m.mode & 0o777)
PY
python3 "$task_tmp/unpack/deploy/installer.py" install --stage "$task_tmp/unpack" --job-id "$job_id"
echo '安装完成。查看管理地址和密码：xnvgatebox access'
echo '首次使用：登录后按“出口总览 → 首次使用”操作。'
echo '1. 建议先启用 VPS 本机直出，验证客户端接入；此项使用服务器公网 IP。'
echo '2. 在“我的订阅”复制订阅链接，添加到 v2rayN 等客户端，更新订阅并选择节点使用。'
echo '3. 需要 VPN Gate 时，在“出口管理”设置目标数量；程序自动建立接入入口，等“已就绪”后更新客户端订阅。'
echo '4. “候选节点”用于按需诊断，无需先全部检测；第三方 SOCKS5 等可选连接在“代理资源”管理。'
