#!/usr/bin/env python3
"""One complete-package deployment transaction; private state, no panel writes."""
import argparse
import hashlib
import http.client
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import sys
import tarfile
import time
import urllib.parse

OWNED = ('var/lib/xnvgatebox', 'usr/local/lib/xnvgatebox', 'etc/xnvgatebox',
         'usr/local/bin/xnvgatebox', 'usr/local/bin/xnvgatebox-fanout',
         'etc/default/xnvgatebox-fanout', 'etc/systemd/system/xnvgatebox-fanout.service',
         'etc/systemd/system/xnvgatebox-pool.service', 'etc/systemd/system/xnvgatebox-pool.timer')
UNITS = ('xnvgatebox-fanout.service', 'xnvgatebox-pool.timer', 'xnvgatebox-pool.service')
MESSAGES = {'preparing':'准备更新', 'dependencies':'准备系统依赖', 'downloading':'下载完整安装包',
            'verifying':'校验安装包', 'pool':'准备候选清单', 'stopping':'停止本项目服务',
            'backup':'保存完整项目备份', 'checking':'检查已有配置', 'installing':'安装完整程序',
            'health':'验证新版本服务', 'rollback':'恢复原有部署', 'complete':'更新完成',
            'failed':'更新失败，原部署保持或已恢复', 'recovery_required':'恢复未完成，请重新运行安装入口'}

def atomic_json(path, data):
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    tmp = path.with_name('.' + path.name + '-' + str(os.getpid()))
    try:
        with open(tmp, 'w', encoding='utf-8') as f:
            os.chmod(tmp, 0o600)
            json.dump(data, f, ensure_ascii=False)
            f.flush(); os.fsync(f.fileno())
        os.replace(tmp, path)
        fd = os.open(str(path.parent), os.O_RDONLY)
        try: os.fsync(fd)
        finally: os.close(fd)
    finally:
        if tmp.exists(): tmp.unlink()


def process_identity(pid):
    try:
        stat = Path('/proc/' + str(pid) + '/stat').read_text()
        return stat[stat.rfind(')') + 2:].split()[19], Path('/proc/sys/kernel/random/boot_id').read_text().strip()
    except (OSError, IndexError): return '', ''


class Layout:
    def __init__(self, root=Path('/')):
        self.root = Path(root)
        self.state = self.path('var/lib/xnvgatebox-deploy')
        self.job = self.state / 'status.json'
        self.journal = self.state / 'transaction.json'
        self.work = self.path('var/lib/xnvgatebox/fanout')
        self.lib = self.path('usr/local/lib/xnvgatebox')
    def path(self, name): return self.root / name


def mark(layout, job_id, phase, status='running', **extra):
    try: job = json.loads(layout.job.read_text())
    except FileNotFoundError: job = {}
    if job.get('id') not in (None, job_id): return
    if phase == 'failed' and job.get('status') in ('failed', 'completed'): return
    now = time.time()
    start, boot = process_identity(os.getpid())
    job.update(id=job_id, status=status, phase=phase, message=MESSAGES[phase],
               started_at=job.get('started_at', now), updated_at=now,
               pid=os.getpid(), process_start=start, boot_id=boot)
    job.update(extra)
    atomic_json(layout.job, job)


def init_job(layout, job_id):
    """Called only after the shell owns the host deployment lock."""
    try: previous = json.loads(layout.job.read_text())
    except FileNotFoundError: previous = {}
    if previous.get('id') == job_id: return
    atomic_json(layout.job, {'id': job_id, 'started_at': time.time()})
    mark(layout, job_id, 'preparing')


def checked_archive(archive, destination, expected_version=''):
    """No links, duplicate paths, unlisted files or unchecked archive members."""
    with tarfile.open(archive) as t:
        members = t.getmembers()
        if len(members) > 10000 or sum(m.size for m in members) > 512 * 1024 * 1024:
            raise ValueError('package_size_rejected')
        names = set()
        for m in members:
            p = PurePosixPath(m.name)
            if (p.is_absolute() or '..' in p.parts or not p.parts or str(p) != m.name.rstrip('/') or m.name in names
                    or not (m.isfile() or m.isdir())):
                raise ValueError('package_entry_rejected')
            names.add(m.name)
        files = {m.name: t.extractfile(m).read() for m in members if m.isfile()}
        required = {'package-sha256.json','BUILD.json','VERSION','bin/xnvgatebox-manager','bin/xray',
                    'deploy/installer.py','deploy/xnvgatebox','scripts/toolbox.py','config/settings.json'}
        if not required.issubset(files): raise ValueError('package_incomplete')
        sums = json.loads(files['package-sha256.json'])
        if set(sums) != set(files) - {'package-sha256.json'}:
            raise ValueError('package_manifest_incomplete')
        if any(hashlib.sha256(files[n]).hexdigest() != h for n,h in sums.items()):
            raise ValueError('package_digest_rejected')
        version = files['VERSION'].decode().strip()
        build = json.loads(files['BUILD.json'])
        if build.get('version') != version or (expected_version and version != expected_version):
            raise ValueError('package_version_rejected')
        if expected_version and (build.get('source_dirty') is not False or build.get('manager_origin') != 'built_from_source'):
            raise ValueError('release_source_rejected')
        destination.mkdir(mode=0o700)
        for m in members:
            path = destination / m.name
            if m.isdir(): path.mkdir(parents=True, exist_ok=True, mode=0o700)
            else:
                path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
                path.write_bytes(files[m.name]); path.chmod(m.mode & 0o777)
    return version


class Runner:
    def run(self, args, check=True, **kwargs):
        return subprocess.run([str(x) for x in args], check=check, **kwargs)
    def loaded(self, unit):
        # is-enabled --quiet still writes an error for absent units on Debian.
        # show reports not-found without treating a fresh install as a failure;
        # a real systemd/bus error must still abort the deployment snapshot.
        state = self.run(['systemctl','show','--property=LoadState','--value',unit],
                         stdout=subprocess.PIPE, text=True).stdout.strip()
        if not state: raise RuntimeError('systemd_load_state_missing')
        return state != 'not-found'
    def active(self, unit):
        if not self.loaded(unit): return False
        return self.run(['systemctl','is-active','--quiet',unit],check=False).returncode == 0
    def enabled(self, unit):
        if not self.loaded(unit): return False
        return self.run(['systemctl','is-enabled','--quiet',unit],check=False).returncode == 0


class Installer:
    def __init__(self, stage, job_id, layout=None, runner=None):
        if not re.fullmatch(r'[A-Za-z0-9-]{8,64}',job_id): raise ValueError('invalid_job_id')
        self.stage = Path(stage) if stage else None
        self.id = job_id
        self.l = layout or Layout()
        self.r = runner or Runner()
    def progress(self, phase, **extra): mark(self.l,self.id,phase,**extra)
    def journal(self, data): atomic_json(self.l.journal,data)
    def service_snapshot(self):
        return {u:{'active':self.r.active(u),'enabled':self.r.enabled(u)} for u in UNITS}
    def stop(self, before=None):
        for u in (UNITS[1],UNITS[2],UNITS[0]):
            if before is None or before[u]['active']: self.r.run(['systemctl','stop',u],check=before is not None)
    def restore_services(self, before):
        self.r.run(['systemctl','daemon-reload'])
        for u in UNITS:
            if (self.l.path('etc/systemd/system') / u).exists():
                self.r.run(['systemctl','enable' if before[u]['enabled'] else 'disable',u])
                if before[u]['active']: self.r.run(['systemctl','start',u])
    def backup(self, record):
        path = Path(record['backup'])
        temp = path.with_suffix('.partial')
        with tarfile.open(temp,'w:gz',dereference=False) as t:
            for name in record['existing_paths']: t.add(self.l.path(name),arcname=name)
        temp.chmod(0o600);os.replace(temp,path)
    def restore_files(self, record):
        # Our private archive is still checked: symlinks may be restored as
        # objects, but no archive entry may traverse through a link ancestor.
        archive = Path(record['backup'])
        with tarfile.open(archive) as t:
            members=t.getmembers(); links={m.name for m in members if m.issym() or m.islnk()}
            for m in members:
                p=PurePosixPath(m.name)
                if p.is_absolute() or '..' in p.parts or not any(m.name==v or m.name.startswith(v+'/') for v in OWNED):
                    raise ValueError('backup_path_rejected')
                if any(str(a) in links for a in p.parents): raise ValueError('backup_link_parent_rejected')
                if not (m.isfile() or m.isdir() or m.issym() or m.islnk()): raise ValueError('backup_type_rejected')
                if m.islnk() and (m.linkname not in {v.name for v in members} or PurePosixPath(m.linkname).is_absolute() or '..' in PurePosixPath(m.linkname).parts):
                    raise ValueError('backup_hardlink_rejected')
            for name in OWNED:
                p=self.l.path(name)
                if p.is_symlink() or p.is_file():p.unlink()
                elif p.exists():shutil.rmtree(p)
            for m in members:
                p=self.l.path(m.name);p.parent.mkdir(parents=True,exist_ok=True)
                if m.isdir():p.mkdir(exist_ok=True);p.chmod(m.mode & 0o777)
                elif m.isfile():p.write_bytes(t.extractfile(m).read());p.chmod(m.mode & 0o777)
                elif m.issym():os.symlink(m.linkname,p)
            for m in members:
                if m.islnk():os.link(self.l.path(m.linkname),self.l.path(m.name))
    def recover(self):
        if not self.l.journal.exists(): return False
        record=json.loads(self.l.journal.read_text())
        if record.get('phase')=='committed':self.l.journal.unlink();return True
        if not record.get('before') or not re.fullmatch(r'[A-Za-z0-9-]{8,64}',record.get('id','')):
            raise ValueError('invalid_deployment_journal')
        self.progress('rollback')
        self.stop()
        if record['phase'] in ('installing','health'):
            self.restore_files(record)
        self.restore_services(record['before'])
        self.l.journal.unlink()
        return True
    def prepare_pool(self):
        self.progress('pool')
        target=self.stage/'prepared-pool';target.mkdir(mode=0o700)
        config=self.l.path('etc/xnvgatebox/pool.json')
        if not config.exists():config=self.stage/'config/settings.json'
        try:
            self.r.run(['python3',self.stage/'scripts/toolbox.py','--config',config,'pool','--output',target],stdout=subprocess.DEVNULL)
            return target
        except subprocess.CalledProcessError:
            previous=self.l.path('var/lib/xnvgatebox/pool/node_pool.json')
            if not previous.is_file():raise
            # Keep a previous usable snapshot when the source is temporarily
            # unavailable. No replacement happens before the consistent backup.
            value=json.loads(previous.read_text())
            if not isinstance(value.get('nodes'),list) or not value['nodes']:raise ValueError('no_previous_pool')
            return None
    def apply_files(self, pool):
        self.l.lib.parent.mkdir(parents=True,exist_ok=True)
        incoming=self.l.lib.with_name('.xnvgatebox-'+self.id)
        if incoming.exists():shutil.rmtree(incoming)
        shutil.copytree(self.stage,incoming,ignore=shutil.ignore_patterns('prepared-pool','__pycache__','*.pyc'))
        if self.l.lib.exists():shutil.rmtree(self.l.lib)
        os.replace(incoming,self.l.lib)
        self.l.work.mkdir(parents=True,exist_ok=True,mode=0o700)
        self.l.work.chmod(0o700)
        for src,dest,mode in [('deploy/xnvgatebox','usr/local/bin/xnvgatebox',0o755),('bin/xnvgatebox-manager','usr/local/bin/xnvgatebox-fanout',0o755),('bin/xray','var/lib/xnvgatebox/fanout/bin/xray',0o700)]:
            p=self.l.path(dest);p.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(self.stage/src,p);p.chmod(mode)
        config=self.l.path('etc/xnvgatebox/pool.json');config.parent.mkdir(parents=True,exist_ok=True,mode=0o700)
        if not config.exists():shutil.copyfile(self.stage/'config/settings.json',config);config.chmod(0o600)
        env=self.l.path('etc/default/xnvgatebox-fanout');env.parent.mkdir(parents=True,exist_ok=True)
        if not env.exists():env.write_text('FANOUT_VALIDATED_POOL_FILE=/var/lib/xnvgatebox/pool/node_pool.json\nFANOUT_SLOT_PORT_BASE=17928\n');env.chmod(0o600)
        settings=self.l.work/'settings.json'
        if not settings.exists():atomic_json(settings,{'port':8899,'listen_addr':'0.0.0.0','residential_only':False})
        if pool is not None:
            output=self.l.path('var/lib/xnvgatebox/pool');output.mkdir(parents=True,exist_ok=True,mode=0o700)
            shutil.copytree(pool,output,dirs_exist_ok=True)
        units=self.l.path('etc/systemd/system');units.mkdir(parents=True,exist_ok=True)
        for p in (self.stage/'deploy').iterdir():
            if p.suffix in ('.service','.timer') and p.name in UNITS:
                shutil.copyfile(p,units/p.name);(units/p.name).chmod(0o644)
        self.r.run(['systemctl','daemon-reload'])
    def health(self, expected):
        # Only localhost/private configured bind address, no proxy, redirects,
        # credentials or management path in output. Verify the actual process.
        deadline=time.monotonic()+45
        while time.monotonic()<deadline:
            conn=None
            try:
                if not self.r.active(UNITS[0]):raise ValueError('service_not_active')
                settings=json.loads((self.l.work/'settings.json').read_text())
                host=settings.get('listen_addr') or '127.0.0.1'
                if host in ('0.0.0.0','all','::'):host='127.0.0.1'
                base='/'+(self.l.work/'basepath').read_text().strip().strip('/')
                conn=http.client.HTTPConnection(host,int(settings['port']),timeout=3)
                body=urllib.parse.urlencode({'password':(self.l.work/'password').read_text().strip()})
                conn.request('POST',base+'/login',body,{'Content-Type':'application/x-www-form-urlencoded'})
                response=conn.getresponse();response.read();cookie=response.getheader('Set-Cookie','').split(';')[0]
                if not cookie:raise ValueError('login_not_ready')
                conn.request('GET',base+'/api/console',headers={'Cookie':cookie})
                response=conn.getresponse();data=response.read(1000000)
                if response.status==200 and json.loads(data).get('version')==expected:
                    conn.request('GET',base+'/api/exits',headers={'Cookie':cookie})
                    response=conn.getresponse();view=json.loads(response.read(1000000))
                    if response.status==200 and view.get('backend')=='native' and not view.get('panel'):return
            except (OSError,ValueError,KeyError,http.client.HTTPException):pass
            finally:
                if conn:conn.close()
            time.sleep(.5)
        raise RuntimeError('new_version_health_failed')
    def install(self):
        init_job(self.l, self.id)
        if self.l.journal.exists():self.recover()
        self.progress('preparing',target=(self.stage/'VERSION').read_text().strip())
        pool=self.prepare_pool()  # Source failure happens before service stop.
        before=self.service_snapshot()
        existing=[n for n in OWNED if self.l.path(n).exists() or self.l.path(n).is_symlink()]
        fresh=not self.l.lib.exists()
        backup_dir=self.l.path('var/backups/xnvgatebox')/('install-'+self.id)
        backup_dir.mkdir(parents=True,exist_ok=False,mode=0o700)
        record={'id':self.id,'phase':'stopping','before':before,'existing_paths':existing,'backup':str(backup_dir/'project.tar.gz')}
        self.l.state.mkdir(parents=True,exist_ok=True,mode=0o700)
        # Recovery remains available even if the library replacement is interrupted.
        shutil.copyfile(Path(__file__),self.l.state/'recover.py');(self.l.state/'recover.py').chmod(0o700)
        self.journal(record)
        try:
            self.progress('stopping');self.stop(before)
            record['phase']='backup';self.journal(record);self.progress('backup');self.backup(record)
            self.progress('checking',backup_available=True)
            if not fresh:
                self.r.run([self.stage/'bin/xnvgatebox-manager','-check-state','-dir',self.l.work],stdout=subprocess.DEVNULL)
            record['phase']='installing';self.journal(record);self.progress('installing')
            self.apply_files(pool)
            record['phase']='health';self.journal(record);self.progress('health')
            if fresh or before[UNITS[0]]['active']:
                self.r.run(['systemctl','start',UNITS[0]]);self.health((self.stage/'VERSION').read_text().strip())
            if fresh:
                self.r.run(['systemctl','enable',UNITS[0],UNITS[1]])
            if fresh or before[UNITS[1]]['active']:self.r.run(['systemctl','start',UNITS[1]])
            record['phase']='committed';self.journal(record)
            mark(self.l,self.id,'complete','completed',backup_available=True,rolled_back=False)
            self.l.journal.unlink()
        except BaseException:
            try:
                self.recover()
                mark(self.l,self.id,'failed','failed',backup_available=Path(record['backup']).is_file(),rolled_back=True)
            except BaseException:
                mark(self.l,self.id,'recovery_required','failed',backup_available=Path(record['backup']).is_file(),rolled_back=False)
            raise


def main():
    os.umask(0o077)
    p=argparse.ArgumentParser();p.add_argument('command',choices=['extract','install','recover','init','mark','status'])
    p.add_argument('--archive');p.add_argument('--stage');p.add_argument('--expected-version',default='')
    p.add_argument('--job-id',default='');p.add_argument('--phase',default='preparing');p.add_argument('--target',default='')
    args=p.parse_args();l=Layout()
    try:
        if args.command=='extract':checked_archive(args.archive,Path(args.stage),args.expected_version)
        elif args.command=='install':Installer(args.stage,args.job_id).install()
        elif args.command=='recover':Installer(None,args.job_id).recover()
        elif args.command=='init':
            if not re.fullmatch(r'[A-Za-z0-9-]{8,64}',args.job_id):raise ValueError('invalid_job')
            init_job(l,args.job_id)
        elif args.command=='mark':
            if not re.fullmatch(r'[A-Za-z0-9-]{8,64}',args.job_id) or args.phase not in MESSAGES:raise ValueError('invalid_job')
            start,boot=process_identity(os.getppid())
            mark(l,args.job_id,args.phase,'failed' if args.phase=='failed' else 'running',target=args.target,pid=os.getppid(),process_start=start,boot_id=boot)
        else:
            try:job=json.loads(l.job.read_text())
            except FileNotFoundError:job={'status':'idle','message':'尚无更新任务'}
            print(json.dumps({k:v for k,v in job.items() if k not in ('pid','process_start','boot_id')},ensure_ascii=False))
    except BaseException as exc:
        if args.command in ('install','recover'):
            try:
                job=json.loads(l.job.read_text())
                if job.get('status')=='running':mark(l,args.job_id,'recovery_required' if l.journal.exists() else 'failed','failed')
            except OSError:pass
        # Never print exception text: package/config/HTTP paths can carry secrets.
        print('部署未完成；请查看私有安装日志或更新状态（'+type(exc).__name__+'）',file=sys.stderr)
        return 1
    return 0

if __name__=='__main__':sys.exit(main())
