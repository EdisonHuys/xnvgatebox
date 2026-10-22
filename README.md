# xnvgatebox

在 Linux VPS 上管理多个隔离出口，并通过一个订阅链接统一提供给客户端。项目内置 Xray，支持 VPN Gate 出口、VPS 本机直出、手动第三方 SOCKS5 和原始 SSTP；不要求安装 3x-ui。

最新版本：[v0.8.1](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.8.1)。安装、在线更新、备份恢复和卸载均使用完整包校验与部署锁，管理密码、订阅凭据和出口设置保存在本机私有目录。

## 支持的平台与安装

已验收平台为 Debian 12 amd64 + systemd。Ubuntu amd64 使用相同入口；CentOS、RHEL、Rocky Linux 和 AlmaLinux 当前不在安装器支持范围内，不能只把 `apt-get` 替换成 `dnf` 或 `yum`。这些系统还需要单独验证 RPM 依赖、SSTP/PPP 软件包、iptables 兼容性和完整 systemd 链路后，才会提供正式命令。

Debian 12 或兼容的 Ubuntu amd64 以 root 执行：

```sh
apt-get update && apt-get install -y ca-certificates curl
curl -fsSL https://raw.githubusercontent.com/hanhaoops/xnvgatebox/v0.8.1/vps/install.sh -o /tmp/xnvgatebox-install.sh && bash /tmp/xnvgatebox-install.sh --version v0.8.1 && xnvgatebox access
```

安装包含管理程序、候选池构建器、固定版本 Xray 和系统协议依赖。VPS 自行解析 VPN Gate 官方当次公开清单，每十分钟刷新，不需要 GitHub 或 Cloudflare 账号，也不需要 3x-ui。

首次安装自动生成独立随机管理路径、初始密码和订阅凭据。上面的一键命令最后会直接显示管理地址和密码；以后也可重新查看：

```sh
xnvgatebox access
```

在浏览器打开它打印的私有管理地址并输入密码。默认管理端口为 8899，服务器防火墙需允许访问此端口。首次安装会生成新的路径、密码和订阅凭据；重装系统后旧地址不会自动恢复。

重复安装或在线更新会保留已有路径和密码，`xnvgatebox access` 只查看，不重置凭据。管理地址和密码请保存在私有位置。

## 首次使用：从登录到客户端上网

1. 设置出口
2. 等待就绪
3. 复制订阅
4. 导入使用

[详细操作与状态说明](docs/FIRST_USE.md)

## 在线更新

管理页“设置 → 版本与更新”检查正式版、查看说明并执行完整包更新。更新会短暂中断连接；管理服务恢复后重新登录可查看结果。也可使用命令行：

```sh
xnvgatebox version
xnvgatebox check-update
xnvgatebox update
xnvgatebox update-status
```

安装、命令行更新和网页更新共用部署锁。更新前停止本项目服务并在 `/var/backups/xnvgatebox/` 保存一致的完整项目备份，保留端口、凭据、订阅和用户设置；校验、状态检查或运行验证失败会恢复原部署。恢复未完成会保留记录，下次安装先恢复。备份含私密信息，不应上传公开仓库；它不是整机系统备份。已有 v0.4.0 安装先执行上面新版安装命令完成一次迁移，之后即可使用网页更新。

重装系统前请将备份另存到本机。全新系统验收步骤见 [安装与更新说明](docs/INSTALL_UPDATE.md)；备份和恢复边界见 [备份与重装](docs/BACKUP_REINSTALL.md)。

## 本机使用

Python 3.9+，核心仅使用标准库；最终验活还需要 curl 和 Xray。Linux/macOS 支持本机文件锁。

```sh
python3 scripts/toolbox.py pool
```

无需 Cloudflare、UUID 或远程节点池地址即可生成 `data/node_pool.json`、对应 SHA256 和 `data/configs/*.ovpn`。OpenVPN 配置不执行，协议状态保持 `not_tested`；本命令是后续 VPS Standalone 使用的同一个 Builder。可用 `--csv-file /absolute/path/vpngate.csv` 解析官方格式快照。国家过滤、候选预算等在 `config/settings.json` 修改。

当前安装与运行不依赖 Cloudflare。

## GitHub Actions

`update.yml` 目前仅支持手动构建共享候选池，不定时运行，也不部署或发布 Cloudflare。`test.yml` 执行 Python/Node 回归，`vps.yml` 执行 Linux Go 测试并构建测试包；正式 release 由单独的手动工作流处理。

macOS 开发限制见 [MAC_HANDOFF.md](docs/MAC_HANDOFF.md)；VPS 升级、订阅、故障和备份恢复记录见 [VPS_UPGRADE_VALIDATION.md](docs/VPS_UPGRADE_VALIDATION.md)。历史 Mode A 文档仅供参考。

## 验证

```sh
python3 -m unittest discover -s tests -v
node tests/test_worker_guards.mjs
node tests/test_cf_control.mjs
```

Python 回归覆盖入口变化、字段缺失、画像冲突、429、IP 不匹配、直出反向测试、证书错误拒绝、过期和配置变化拒绝发布、旧订阅清空、密钥不进入公共产物、锁和清理；29 项 Worker 边界检查加 10 项 control Worker 测试覆盖认证、协议、目标限制、过期 manifest 和参数覆盖拒绝。安装 Xray 后额外检查真实二进制接受配置、loopback 监听与进程清理。构造测试与真实 VPN 验收分别记录；当前开发分支的备份、升级迁移、真实订阅及故障恢复实测见 [VPS_UPGRADE_VALIDATION.md](docs/VPS_UPGRADE_VALIDATION.md)；此前版本记录见 [VPS_STANDALONE_VALIDATION.md](docs/VPS_STANDALONE_VALIDATION.md)。实测记录不承诺上游节点持续在线。

第一轮结果与后续实测步骤见 [ROUND1.md](docs/ROUND1.md)。核心工具许可为 GPL-3.0-only；独立 EdgeTunnel 修改为 GPL-2.0-only，依赖与复用边界见 [THIRD_PARTY.md](THIRD_PARTY.md)。本版不支持未经验证的 UDP、IPv6、XHTTP。

接口来源：[VPN Gate 官方 CSV](https://www.vpngate.net/en/)、[Xray 配置文档](https://xtls.github.io/en/config/transport.html)、[ipify 官方接口](https://www.ipify.org/)、[icanhaz 官方实现](https://github.com/major/icanhaz)、[IPLocate 身份认证](https://www.iplocate.io/docs/getting-started/authentication)。

## 卸载

以 root 执行 `xnvgatebox uninstall`，默认保留已有备份。彻底清理并包括可确认归属的旧版安装：

```sh
xnvgatebox uninstall --dry-run --purge --legacy
xnvgatebox uninstall --purge --legacy
```

执行时确认删除，详见 [卸载与旧版清理](docs/UNINSTALL.md)。v0.7.0 及更早版本可按文档下载独立卸载工具。
