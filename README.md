# xnvgatebox

在 Linux VPS 上管理多个隔离出口，使用固定本地 SOCKS 端口和内置 Xray，通过一个订阅链接聚合多个出站。无需安装 3x-ui；已有面板可以使用导出的标准 Xray JSON 手工接入。

**当前开发重心是 VPS Standalone。Cloudflare 部署已备份撤下，旧 CF 自动发布任务已停用。** Mode A 的源码与测试暂保留为历史参考，不是当前部署入口。

最新发布：[v0.5.0](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.5.0)。本版包含独立控制台、自定出口数与检测并发、VPS 本机直出、外部 SOCKS5、原始 SSTP 资源管理及完整包在线更新。安装更新规范与验证范围见 [安装和更新说明](docs/INSTALL_UPDATE.md)。

## Debian 12 安装

已测环境为 Debian 12 amd64、systemd。Debian / Ubuntu amd64 可使用同一入口，其他系统尚未验收。以 root 执行：

```sh
curl -fsSL https://raw.githubusercontent.com/hanhaoops/xnvgatebox/v0.5.0/vps/install.sh -o /tmp/xnvgatebox-install.sh && bash /tmp/xnvgatebox-install.sh --version v0.5.0
```

安装包含管理程序、Pool Builder、固定版本 Xray 和系统协议依赖。VPS 自行解析 VPN Gate 官方当次公开清单，每十分钟刷新，不需要 GitHub 或 Cloudflare 账号，也不需要 3x-ui。安装完成后执行：

```sh
xnvgatebox access
```

在浏览器打开它打印的私有管理地址并输入密码。默认管理端口为 8899，服务器防火墙需允许访问此端口。首次安装会生成新的路径、密码和订阅凭据；重装系统后旧地址不会自动恢复。

1. 在“总览”启用 **VPS 本机直出**，复制默认订阅到 v2rayN，刷新订阅并验证。该入口明确使用 VPS 公网 IP，可独立启停。
2. 设置目标出口数，程序自动建立接入入口并绑定出口；数量和检测并发由用户决定，实际受主机资源、地址/端口及上游节点限制。也可在“接入管理”自建入口并选择已有出口。
3. 在“候选节点”刷新来源和检测连通性。来源数量不等于可用节点数量；实际国家未知时显示待验证，不用来源国家冒充实际出口。
4. 在“代理资源”管理可选外部 SOCKS5，以及实测后的原始 SSTP 参数。外部 SOCKS5 经本 VPS 转发，需要 VPS 在线；原始 SSTP 由使用方自行建连，最近实测通过不保证持续可用。

VPN Gate 出口故障时阻断对应连接，不自动切换到本机直出。指定国家当时没有合适 IP 时保留策略并等待；不强求国家或出口数量。没有可信画像时类型保持 unknown，不能称为已验证家宽。操作细节见 [控制台说明](docs/CONSOLE.md)、[代理资源说明](docs/PROXY_RESOURCES.md) 和 [VPS 文档](vps/README.md)。

## 在线更新

管理页“设置 → 版本与更新”检查正式版、查看说明并执行完整包更新。更新会短暂中断连接；管理服务恢复后重新登录可查看结果。也可使用命令行：

```sh
xnvgatebox version
xnvgatebox check-update
xnvgatebox update
xnvgatebox update-status
```

安装、命令行更新和网页更新共用部署锁。更新前停止本项目服务并在 `/var/backups/xnvgatebox/` 保存一致的完整项目备份，保留端口、凭据、订阅和用户设置；校验、状态检查或运行验证失败会恢复原部署。恢复未完成会保留记录，下次安装先恢复。备份含私密信息，不应上传公开仓库；它不是整机系统备份。已有 v0.4.0 安装先执行上面新版安装命令完成一次迁移，之后即可使用网页更新。

重装系统前请将备份另存到本机。重装后的第一轮全新测试先不导入旧状态，按 [Debian 12 全新验收清单](docs/INSTALL_UPDATE.md#全新系统验收清单) 验证；操作系统重装由使用者自行进行。

## 本机使用

Python 3.9+，核心仅使用标准库；最终验活还需要 curl 和 Xray。Linux/macOS 支持本机文件锁。

```sh
python3 scripts/toolbox.py pool
```

无需 Cloudflare、UUID 或远程节点池地址即可生成 `data/node_pool.json`、对应 SHA256 和 `data/configs/*.ovpn`。OpenVPN 配置不执行，协议状态保持 `not_tested`；本命令是后续 VPS Standalone 使用的同一个 Builder。可用 `--csv-file /absolute/path/vpngate.csv` 解析官方格式快照。国家过滤、候选预算等在 `config/settings.json` 修改。

旧 Cloudflare 验活资料保留在历史文档，当前不部署或执行 Mode A。

## GitHub Actions

`update.yml` 目前仅支持手动构建共享候选池，不定时运行，也不部署或发布 Cloudflare。`test.yml` 执行 Python/Node 回归，`vps.yml` 执行 Linux Go 测试并构建测试包；正式 release 由单独的手动工作流处理。

本轮 Mac 开发接续与环境限制见 [MAC_HANDOFF.md](docs/MAC_HANDOFF.md)，真实 VPS 升级、订阅、故障和备份恢复结果见 [Mac 实测](docs/VPS_UPGRADE_VALIDATION.md#2026-10-15-mac-实测)。历史 Mode A 文档仅供参考，当前不恢复 CF。

## 验证

```sh
python3 -m unittest discover -s tests -v
node tests/test_worker_guards.mjs
node tests/test_cf_control.mjs
```

Python 回归覆盖入口变化、字段缺失、画像冲突、429、IP 不匹配、直出反向测试、证书错误拒绝、过期和配置变化拒绝发布、旧订阅清空、密钥不进入公共产物、锁和清理；29 项 Worker 边界检查加 10 项 control Worker 测试覆盖认证、协议、目标限制、过期 manifest 和参数覆盖拒绝。安装 Xray 后额外检查真实二进制接受配置、loopback 监听与进程清理。构造测试与真实 VPN 验收分别记录；当前开发分支的备份、升级迁移、真实订阅及故障恢复实测见 [VPS_UPGRADE_VALIDATION.md](docs/VPS_UPGRADE_VALIDATION.md)；此前版本记录见 [VPS_STANDALONE_VALIDATION.md](docs/VPS_STANDALONE_VALIDATION.md)。实测记录不承诺上游节点持续在线。

第一轮结果与后续实测步骤见 [ROUND1.md](docs/ROUND1.md)。核心工具许可为 GPL-3.0-only；独立 EdgeTunnel 修改为 GPL-2.0-only，依赖与复用边界见 [THIRD_PARTY.md](THIRD_PARTY.md)。本版不支持未经验证的 UDP、IPv6、XHTTP。

接口来源：[VPN Gate 官方 CSV](https://www.vpngate.net/en/)、[Xray 配置文档](https://xtls.github.io/en/config/transport.html)、[ipify 官方接口](https://www.ipify.org/)、[icanhaz 官方实现](https://github.com/major/icanhaz)、[IPLocate 身份认证](https://www.iplocate.io/docs/getting-started/authentication)。
