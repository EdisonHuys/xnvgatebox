#!/usr/bin/env python3
"""Scoped removal of the standard systemd installation; no global resets."""
import argparse
from contextlib import ExitStack, contextmanager
import fcntl
import ipaddress
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import sys

OWNED = ('etc/systemd/system/xnvgatebox-pool.timer',
         'etc/systemd/system/xnvgatebox-pool.service',
         'etc/systemd/system/xnvgatebox-fanout.service',
         'etc/default/xnvgatebox-fanout', 'usr/local/bin/xnvgatebox',
         'usr/local/bin/xnvgatebox-fanout', 'etc/xnvgatebox',
         'var/lib/xnvgatebox', 'usr/local/lib/xnvgatebox',
         'var/lib/xnvgatebox-deploy')
UNITS = ('xnvgatebox-pool.timer', 'xnvgatebox-pool.service', 'xnvgatebox-fanout.service')
WORK = 'var/lib/xnvgatebox/fanout'
LEGACY_WORK = 'var/lib/fanout'
LEGACY_PATHS = ('etc/systemd/system/fanout.service', 'usr/local/bin/fanout', LEGACY_WORK)


class Refuse(RuntimeError):
    pass


class System:
    def run(self, args):
        # Never echo commands/output: old units and process arguments may contain credentials.
        try:
            r = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, timeout=45)
        except subprocess.TimeoutExpired:
            raise Refuse('系统操作超时：' + args[0] + ' ' + args[1]) from None
        if r.returncode:
            raise Refuse('系统操作失败：' + args[0] + ' ' + args[1])
        return r.stdout

    def loaded(self, unit):
        value = self.run(['systemctl', 'show', '--property=LoadState', '--value', unit]).strip()
        if not value:
            raise Refuse('无法核对服务状态')
        return value != 'not-found'


def instance_tag(directory):
    n = 2166136261
    for byte in directory.encode():
        n = ((n ^ byte) * 16777619) & 0xffffffff
    return format(n & 0xffff, '04x')


def slot_index(slot):
    if 1 <= slot <= 255:
        return slot * 64
    if slot == 256:
        return 0
    if 257 <= slot <= 16384:
        rank = slot - 257
        return (rank // 63) * 64 + rank % 63 + 1
    raise Refuse('出口编号记录异常')


class Instance:
    def __init__(self, owner, relative):
        self.u, self.relative = owner, relative
        self.work = owner.path(relative)
        self.tag = '' if relative == LEGACY_WORK else instance_tag('/' + relative)
        self.prefix = 'fo' + self.tag
        self.pattern = re.compile(re.escape(self.prefix) + r'(-?[1-9][0-9]*)$')
        self.base = 99 if relative == LEGACY_WORK else None
        p = self.work / 'netbase'
        if p.exists():
            if p.is_symlink(): raise Refuse('网段记录为符号链接')
            value = p.read_text().strip()
            if not value.isdigit() or not 100 <= int(value) <= 255:
                if relative != LEGACY_WORK: raise Refuse('网段记录异常，保留配置')
            elif relative != LEGACY_WORK:
                self.base = int(value)
        self.indices = {}
        state = self.work / 'state.json'
        if state.exists():
            if state.is_symlink(): raise Refuse('出口记录为符号链接')
            data = json.loads(state.read_text())
            for row in data.get('tunnels', []): self.add(row['slot'], slot_index(row['slot']))
            for slot in data.get('deleted_slots', []): self.add(slot, slot_index(slot))
        for p in self.work.glob('*.ovpn'):
            m = self.pattern.fullmatch(p.stem)
            if m and int(m[1]) > 0: self.add(int(m[1]), slot_index(int(m[1])))
        for d in self.work.glob('.connectivity-*'):
            if d.is_symlink() or not d.is_dir(): raise Refuse('检测目录归属异常')
            p = d / 'diagnostic.json'
            if p.exists():
                if p.is_symlink(): raise Refuse('检测记录为符号链接')
                r = json.loads(p.read_text())
                if not -16384 <= r['slot'] <= -1: raise Refuse('检测编号记录异常')
                self.add(r['slot'], r['network_index'])
            else:
                for slot in (240, 241):
                    if (d / (self.prefix + str(slot) + '.ovpn')).is_file(): self.add(slot, slot_index(slot))
        if self.base is None and self.indices: raise Refuse('缺少已分配实例的网段记录，保留配置')

    def add(self, slot, index):
        if type(slot) is not int or not -16384 <= slot <= 16384 or slot == 0 or type(index) is not int or not 0 <= index < 16384:
            raise Refuse('网络资源记录异常')
        if slot in self.indices and self.indices[slot] != index: raise Refuse('网络资源编号冲突')
        self.indices[slot] = index

    def cidr(self, index):
        return str(ipaddress.ip_network('10.' + str(self.base) + '.' + str(index * 4 // 256) + '.' + str(index * 4 % 256) + '/30'))

    def observe(self):
        r = self.u.r
        host = json.loads(r.run(['ip', '-j', '-4', 'addr', 'show']))
        namespaces = json.loads(r.run(['ip', '-j', 'netns', 'list']))
        own_ns, own_links = [], []
        for link in host:
            name = link.get('ifname', '').split('@')[0]
            m = re.fullmatch('fov' + re.escape(self.tag) + r'(-?[1-9][0-9]*)', name)
            addresses = [v['local'] for v in link.get('addr_info', []) if v.get('family') == 'inet']
            if m:
                slot = int(m[1]); found = False
                for address in addresses:
                    ip = ipaddress.ip_address(address)
                    if self.base is not None and ip in ipaddress.ip_network('10.' + str(self.base) + '.0.0/16'):
                        index = int(ip) % 65536 // 4
                        self.add(slot, index); found = True
                if not found:
                    if self.relative == LEGACY_WORK and addresses and slot not in self.indices: continue
                    if addresses or slot not in self.indices or self.base is None:
                        raise Refuse('同名接口缺少本实例网段证据，保留现场')
                own_links.append(name)
            elif self.base is not None and any(ipaddress.ip_address(a) in ipaddress.ip_network('10.' + str(self.base) + '.0.0/16') for a in addresses):
                raise Refuse('实例网段被其他接口使用，保留现场')
        for item in namespaces:
            name = item['name']; m = self.pattern.fullmatch(name)
            if not m: continue
            slot = int(m[1])
            peer = 'fop' + self.tag + str(slot)
            links = json.loads(r.run(['ip', '-n', name, '-j', '-4', 'addr', 'show']))
            seen, foreign = False, False
            for link in links:
                if link.get('ifname', '').split('@')[0] != peer: continue
                for value in link.get('addr_info', []):
                    if value.get('family') != 'inet': continue
                    ip = ipaddress.ip_address(value['local'])
                    if self.base is None or ip not in ipaddress.ip_network('10.' + str(self.base) + '.0.0/16'):
                        foreign = True; continue
                    self.add(slot, int(ip) % 65536 // 4); seen = True
            if foreign:
                if self.relative == LEGACY_WORK and slot not in self.indices: continue
                raise Refuse('同名隧道属于其他网段，保留现场')
            if not seen and slot not in self.indices: raise Refuse('残留隧道缺少归属证据')
            if self.base is None: raise Refuse('缺少网段记录，保留残留隧道')
            own_ns.append(name)
        return own_ns, own_links

    def rules(self):
        if self.base is None: return []
        cidrs = {self.cidr(i) for i in self.indices.values()}
        result = []
        for table, chain in (('filter', 'FORWARD'), ('nat', 'POSTROUTING')):
            for line in self.u.r.run(['iptables', '-w', '5', '-t', table, '-S', chain]).splitlines():
                a = shlex.split(line)
                if a[:2] != ['-A', chain]: continue
                def field(key): return a[a.index(key) + 1] if key in a else ''
                addresses = [field(k) for k in ('-s', '-d')]
                own = False
                if table == 'nat':
                    own = len(a) == 6 and field('-s') in cidrs and field('-j') == 'MASQUERADE'
                else:
                    iface = field('-i') or field('-o')
                    own = bool(re.fullmatch('fov' + re.escape(self.tag) + r'(-?[1-9][0-9]*)', iface)) and any(c in cidrs for c in addresses) and field('-j') in ('ACCEPT', 'REJECT')
                    # Only the old installer wrote these exact global rules.
                    if self.relative == LEGACY_WORK:
                        subnet = '10.99.0.0/16'
                        own = own or a[2:] in (['-s', subnet, '-j', 'ACCEPT'], ['-d', subnet, '-j', 'ACCEPT'])
                if own: result.append((table, ['-D'] + a[1:]))
                elif any(c in cidrs for c in addresses) and field('-j') == 'MASQUERADE':
                    raise Refuse('存在无法确认归属的出口规则，保留现场')
        return result

    def cleanup(self):
        namespaces, links = self.observe()
        rules = self.rules()
        for name in namespaces:
            if self.u.r.run(['ip', 'netns', 'pids', name]).strip():
                raise Refuse('隧道仍有进程，保留文件；请核对服务停止结果后重试')
        for table, args in rules: self.u.r.run(['iptables', '-w', '5', '-t', table] + args)
        for name in namespaces: self.u.r.run(['ip', 'netns', 'delete', name])
        # Deleting a namespace may already remove its paired interface.
        current = {v['ifname'].split('@')[0] for v in json.loads(self.u.r.run(['ip', '-j', '-4', 'addr', 'show']))}
        for name in links:
            if name in current: self.u.r.run(['ip', 'link', 'delete', name])
        for slot in self.indices:
            self.u.remove('etc/netns/' + self.prefix + str(slot))
        after = self.observe()
        if any(after) or self.rules(): raise Refuse('网络资源清理未完成，保留配置')


class Uninstaller:
    def __init__(self, root=Path('/'), runner=None, purge=False, legacy=False):
        self.root, self.r, self.purge, self.legacy = Path(root), runner or System(), purge, legacy

    def path(self, name):
        p = self.root / name
        for ancestor in (p, *p.parents):
            if ancestor == self.root: break
            if ancestor.is_symlink(): raise Refuse('受管路径含符号链接：/' + name)
        return p

    def remove(self, name):
        p = self.path(name)
        if p.is_dir(): shutil.rmtree(p)
        elif p.exists(): p.unlink()

    def legacy_owned(self):
        unit = self.path('etc/systemd/system/fanout.service')
        menu = self.path('usr/local/bin') / 'f'
        service_ok = unit.is_file() and re.findall(r'^ExecStart=(.*)$', unit.read_text(), re.M) == ['/usr/local/bin/fanout -dir /var/lib/fanout'] and not re.search(r'^Exec(StartPre|StartPost|Stop|StopPost|Reload)=', unit.read_text(), re.M)
        menu_text = menu.read_text(errors='replace') if not menu.is_symlink() and menu.is_file() else ''
        menu_ok = menu_text.startswith(('#!/usr/bin/env bash\n', '#!/bin/bash\n')) and all(re.search('^' + re.escape(v) + r'\s*$', menu_text, re.M) for v in ('WORK_DIR=/var/lib/fanout', 'SERVICE=fanout', 'BIN=/usr/local/bin/fanout'))
        if self.path('etc/init.d/fanout').exists(): raise Refuse('检测到旧 OpenRC/SysV 服务，请先核对其管理方式')
        exists = menu_ok or any(self.path(n).exists() for n in LEGACY_PATHS)
        if exists and not (service_ok or menu_ok): raise Refuse('旧 fanout 缺少标准安装归属证据，保留现场')
        if unit.exists() and not service_ok: raise Refuse('旧 fanout 服务已自定义，保留现场')
        return exists, menu_ok

    def plan(self):
        paths, units = list(OWNED), list(UNITS)
        instances = []
        journal = self.path('var/lib/xnvgatebox-deploy/transaction.json')
        if journal.exists() and json.loads(journal.read_text()).get('phase') != 'committed':
            raise Refuse('有未完成的安装或恢复，请先完成恢复再卸载')
        updates = self.r.run(['systemctl', 'list-units', '--all', '--plain', '--no-legend', 'xnvgatebox-update-*.service'])
        for line in updates.splitlines():
            fields = line.split()
            if len(fields) >= 4 and re.fullmatch(r'xnvgatebox-update-[A-Za-z0-9-]{8,64}\.service', fields[0]):
                if fields[2] not in ('inactive', 'failed'): raise Refuse('在线更新任务尚未结束，请稍后重试')
        for unit, marker in zip(UNITS, ('Description=Refresh xnvgatebox candidate pool every ten minutes', 'ExecStart=/usr/bin/python3 /usr/local/lib/xnvgatebox/scripts/toolbox.py --config /etc/xnvgatebox/pool.json pool --output /var/lib/xnvgatebox/pool', 'ExecStart=/usr/local/bin/xnvgatebox-fanout -dir /var/lib/xnvgatebox/fanout -panel native')):
            p = self.path('etc/systemd/system/' + unit)
            if p.exists() and marker not in p.read_text(): raise Refuse('项目服务已自定义，保留现场：' + unit)
            if p.exists():
                text = p.read_text()
                if re.search(r'^Exec(StartPre|StartPost|Stop|StopPost|Reload)=', text, re.M): raise Refuse('服务含自定义生命周期操作，保留现场')
                if unit != UNITS[0] and re.findall(r'^ExecStart=(.*)$', text, re.M) != [marker.removeprefix('ExecStart=')]: raise Refuse('服务启动参数已自定义，保留现场')
                if unit == UNITS[0] and any(v.strip() != 'xnvgatebox-pool.service' for v in re.findall(r'^Unit=(.*)$', text, re.M)): raise Refuse('定时器指向其他服务，保留现场')
        if self.path(WORK).exists(): instances.append(Instance(self, WORK))
        if self.purge: paths.append('var/backups/xnvgatebox')
        if self.legacy:
            owned, menu_ok = self.legacy_owned()
            if owned:
                paths.extend(LEGACY_PATHS); units.append('fanout.service')
                if menu_ok: paths.append('usr/local/bin/f')
                if self.path(LEGACY_WORK).exists(): instances.append(Instance(self, LEGACY_WORK))
        for unit in units:
            if self.r.loaded(unit):
                fragment = self.r.run(['systemctl', 'show', '--property=FragmentPath', '--value', unit]).strip()
                if fragment != '/etc/systemd/system/' + unit:
                    raise Refuse('同名服务来自其他安装位置，保留现场：' + unit)
                if self.r.run(['systemctl', 'show', '--property=DropInPaths', '--value', unit]).strip():
                    raise Refuse('服务含额外覆盖配置，保留现场：' + unit)
        # Refuse mount traversal before stopping or deleting anything.
        for name in paths:
            p = self.path(name)
            if not p.exists(): continue
            if os.path.ismount(p): raise Refuse('受管目录是挂载点，保留现场')
            if p.is_dir():
                for base, dirs, _ in os.walk(p, followlinks=False):
                    if any(not (Path(base)/d).is_symlink() and os.path.ismount(Path(base)/d) for d in dirs):
                        raise Refuse('受管目录包含挂载点，保留现场')
        for i in instances: i.observe(); i.rules()
        return paths, units, instances

    @contextmanager
    def lock(self, name):
        p = self.path(name); p.parent.mkdir(parents=True, exist_ok=True)
        fd = os.open(p, os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600)
        try:
            try: fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError: raise Refuse('安装、更新或项目实例仍在运行') from None
            yield
        finally: os.close(fd)

    def execute(self):
        with self.lock('run/lock/xnvgatebox-deploy.lock'), ExitStack() as locks:
            paths, units, instances = self.plan()
            for unit in units:
                self.stop_unit(unit)
            for i in instances:
                locks.enter_context(self.lock(i.relative + '/lock'))
                # Retain pre-stop indices: graceful shutdown may remove manifests.
                fresh = Instance(self, i.relative)
                for slot, index in fresh.indices.items(): i.add(slot, index)
                i.cleanup()
            for name in paths: self.remove(name)
            self.r.run(['systemctl', 'daemon-reload'])
            for unit in units:
                if self.r.loaded(unit): raise Refuse('仍存在其他位置的同名服务，请核对卸载结果')
            if any(self.path(p).exists() for p in paths): raise Refuse('文件清理未完成')

    def stop_unit(self, unit):
        if not self.r.loaded(unit): return
        self.r.run(['systemctl', 'disable', '--now', unit])
        state = self.r.run(['systemctl', 'show', '--property=ActiveState', '--value', unit]).strip()
        if state == 'inactive': return
        if state != 'failed': raise Refuse('服务尚未停止，保留文件：' + unit)
        # An inactive timer can be garbage-collected after disable --now.
        # Only failed units need reset; recheck even if reset reports an error.
        try:
            self.r.run(['systemctl', 'reset-failed', unit])
        except Refuse:
            if not self.r.loaded(unit): return
            if self.r.run(['systemctl', 'show', '--property=ActiveState', '--value', unit]).strip() == 'inactive': return
            raise Refuse('无法清除服务失败状态，保留文件：' + unit) from None
        if self.r.loaded(unit):
            state = self.r.run(['systemctl', 'show', '--property=ActiveState', '--value', unit]).strip()
            if state != 'inactive': raise Refuse('服务失败状态尚未清除，保留文件：' + unit)


def main(argv=None):
    p = argparse.ArgumentParser(description='卸载 xnvgatebox；默认保留已有备份')
    p.add_argument('--purge', action='store_true', help='同时删除本项目备份，无法恢复')
    p.add_argument('--legacy', action='store_true', help='同时清理已确认归属的默认旧 fanout 安装')
    p.add_argument('--dry-run', action='store_true', help='只读预览，不停止或删除')
    p.add_argument('--yes', action='store_true', help='明确确认卸载（用于自动化）')
    args = p.parse_args(argv)
    try:
        if sys.platform != 'linux' or os.geteuid() != 0: raise Refuse('请在 Linux VPS 以 root 执行')
        u = Uninstaller(purge=args.purge, legacy=args.legacy)
        paths, _, instances = u.plan()
        print('将删除本项目程序、服务、配置和订阅凭据：')
        for name in paths:
            if u.path(name).exists(): print('  /' + name)
        print('备份：' + ('同时删除' if args.purge else '保留 /var/backups/xnvgatebox'))
        if args.dry_run:
            print('仅预览，未修改系统。'); return 0
        if not args.yes:
            if not sys.stdin.isatty(): raise Refuse('非交互执行请明确添加 --yes，或先使用 --dry-run')
            if input('确认卸载请输入 yes：').strip().lower() != 'yes':
                print('已取消。'); return 0
        u.execute()
        print('卸载完成。3x-ui、共享系统依赖和全局 IP 转发设置保留。')
        print('系统运行日志及部署协调锁保留，由系统管理。')
        return 0
    except (Refuse, OSError, ValueError, KeyError, TypeError) as exc:
        print('卸载未完成：' + (str(exc) if isinstance(exc, Refuse) else '配置或系统状态异常，保留现场'), file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
