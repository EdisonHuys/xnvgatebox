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
. /etc/os-release
case "${ID:-}:${VERSION_ID:-}" in
  debian:12|debian:13|ubuntu:22.04|ubuntu:24.04) package_family=deb ;;
  centos:9|centos:10) package_family=rpm ;;
  *) echo '支持 Debian 12/13、Ubuntu 22.04/24.04 和 CentOS Stream 9/10'; exit 1 ;;
esac
command -v flock >/dev/null || { echo '请先安装 util-linux'; exit 1; }
mkdir -p /run/lock
exec 9>/run/lock/xnvgatebox-deploy.lock
flock -n 9 || { echo '已有安装或更新任务运行，请稍后查看状态'; exit 1; }
mkdir -p /var/lib/xnvgatebox-deploy
task_tmp=$(mktemp -d /var/lib/xnvgatebox-deploy/install.XXXXXXXX)
trap 'rm -rf "$task_tmp"' EXIT
missing=0
for dependency in curl python3 openvpn ip iptables sstpc pppd unshare mount sysctl; do
  command -v "$dependency" >/dev/null || missing=1
done
if ((missing)); then
  echo '准备系统依赖…'
  if [[ $package_family == deb ]]; then
    DEBIAN_FRONTEND=noninteractive apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install --no-install-recommends -y -qq ca-certificates curl python3 openvpn iproute2 iptables sstp-client ppp util-linux procps
  else
    dnf -y install ca-certificates python3 iproute iptables ppp util-linux procps-ng
    command -v curl >/dev/null || dnf -y install curl
    dnf -y install epel-release
    dnf -y install openvpn
    if ! command -v sstpc >/dev/null; then
      # Build a native RPM against this host's PPP ABI, never install a Fedora binary.
      dnf -y --enablerepo=crb install gcc make autoconf automake libtool pkgconf-pkg-config openssl-devel libevent-devel ppp-devel rpm-build
      curl -fLsS --retry 3 --connect-timeout 15 --max-time 180 \
        https://gitlab.com/sstp-project/sstp-client/-/archive/1.0.20/sstp-client-1.0.20.tar.gz -o "$task_tmp/sstp.tar.gz"
      printf '%s  %s\n' 9150c96c61c71aa3fd0ac7c2b95f60cecb8bf761febb07e00e34794c47eac9fa "$task_tmp/sstp.tar.gz" | sha256sum -c -
      tar -xzf "$task_tmp/sstp.tar.gz" -C "$task_tmp"
      ppp_version=$(rpm -q --qf '%{VERSION}' ppp)
      ppp_release=$(rpm -q --qf '%{RELEASE}' ppp)
      ppp_libdir=$(rpm --eval '%{_libdir}')/pppd/$ppp_version
      if ! pkg-config --exists pppd; then
        mkdir -p "$task_tmp/pkgconfig"
        printf 'Name: pppd\nDescription: Local PPP development headers\nVersion: %s\nLibs:\nCflags:\n' "$ppp_version" >"$task_tmp/pkgconfig/pppd.pc"
        export PKG_CONFIG_PATH="$task_tmp/pkgconfig${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}"
      fi
      (
        cd "$task_tmp/sstp-client-1.0.20"
        autoreconf -fi
        ./configure --prefix=/usr --libdir="$(rpm --eval '%{_libdir}')" --sysconfdir=/etc \
          --with-runtime-dir=/run/sstpc --with-system-ca-path=/etc/pki/tls/certs \
          --with-pppd-plugin-dir="$ppp_libdir" --with-pppd-auth-notify-support
        make -j2
        make DESTDIR="$task_tmp/rpmroot" install
      )
      mkdir -p "$task_tmp/rpmbuild/SPECS"
      # Package only the client, runtime libraries and ABI-specific plugin.
      cat >"$task_tmp/rpmbuild/SPECS/sstp.spec" <<SPEC
Name: xnvgatebox-sstp-client
Version: 1.0.20
Release: 1%{?dist}
Summary: SSTP client built for the local PPP version
License: GPL-2.0-or-later
URL: https://gitlab.com/sstp-project/sstp-client
Requires: ppp = $ppp_version-$ppp_release
Requires(pre): shadow-utils
%description
Unmodified upstream SSTP client, built and packaged locally by xnvgatebox.
%pre
getent group sstpc >/dev/null || groupadd --system sstpc
getent passwd sstpc >/dev/null || useradd --system --gid sstpc --home-dir /run/sstpc --shell /sbin/nologin sstpc
%install
cp -a $task_tmp/rpmroot/. %{buildroot}/
mkdir -p %{buildroot}%{_licensedir}/%{name}
cp $task_tmp/sstp-client-1.0.20/COPYING %{buildroot}%{_licensedir}/%{name}/COPYING
%files
/usr/sbin/sstpc
%{_libdir}/libsstp_api-*.so*
$ppp_libdir/sstp-pppd-plugin.so
%license %{_licensedir}/%{name}/COPYING
%exclude %{_includedir}/*
%exclude %{_libdir}/*.la
%exclude %{_libdir}/*.a
%exclude %{_libdir}/libsstp_api.so
%exclude %{_libdir}/pkgconfig/*
%exclude $ppp_libdir/*.la
%exclude $ppp_libdir/*.a
%{_datadir}/doc/sstp-client/*
%{_mandir}/man8/sstpc.8*
SPEC
      rpmbuild --define "_topdir $task_tmp/rpmbuild" --define '_build_id_links none' -bb "$task_tmp/rpmbuild/SPECS/sstp.spec"
      dnf -y install "$task_tmp"/rpmbuild/RPMS/x86_64/xnvgatebox-sstp-client-*.rpm
    fi
  fi
fi
echo '检查系统依赖…'
python3 -c 'import sys; assert sys.version_info >= (3,9), "需要 Python 3.9+"'
for dependency in curl python3 openvpn ip iptables sstpc pppd unshare mount sysctl; do
  command -v "$dependency" >/dev/null || { echo "缺少系统依赖：$dependency"; exit 1; }
done
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
  echo '下载完整安装包…'
  curl -fL --progress-bar --retry 3 --connect-timeout 15 --max-time 180 "$release_url/xnvgatebox-linux-amd64.tar.gz" -o "$task_tmp/xnvgatebox-linux-amd64.tar.gz"
  curl -fLsS --retry 3 --connect-timeout 15 --max-time 60 "$release_url/checksums.txt" -o "$task_tmp/checksums.txt"
else
  package=$(realpath "$package")
  cp "$(dirname "$package")/checksums.txt" "$task_tmp/checksums.txt"
  cp "$package" "$task_tmp/xnvgatebox-linux-amd64.tar.gz"
fi
mark_phase verifying
echo '校验并解包…'
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
echo '首次使用：设置出口 → 等待就绪 → 复制订阅 → 导入使用。'
