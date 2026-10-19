# xnvgatebox

在 Linux VPS 上管理多个隔离出口，使用固定本地 SOCKS 端口和内置 Xray，通过一个订阅链接聚合多个出站。无需安装 3x-ui；已有面板可以使用导出的标准 Xray JSON 手工接入。

**当前开发重心是 VPS Standalone。Cloudflare 部署已备份撤下，旧 CF 自动发布任务已停用。** Mode A 的源码与测试暂保留为历史参考，不是当前部署入口。

最新版本：[v0.6.2](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.6.2)。修复首次安装查询尚不存在服务时的误导报错，新增首次使用引导。出口管理统一展示本机直出、自定义代理和 VPN Gate；订阅按此顺序输出，自定义备注旁保留来源标识。新增手动第三方 SOCKS5 管理，支持国家/城市标注与检测、用户备注命名、独立启停和分类订阅。客户端仍通过 VLESS/REALITY 接入，供应商凭据不进入订阅；故障不切换出口。使用和验证范围见 [第三方代理说明](docs/THIRD_PARTY_SOCKS.md)。保留独立控制台、VPN Gate 多出口、VPS 本机直出、外部 SOCKS5、原始 SSTP 与完整包在线更新；安装和升级见 [说明](docs/INSTALL_UPDATE.md)。使用者已重装 Debian 12 并报告安装成功；管理页已观察到候选通过和运行出口就绪，完整客户端验收仍需另行完成。

## Debian 12 安装

已测环境为 Debian 12 amd64、systemd。Debian / Ubuntu amd64 可使用同一入口，其他系统尚未验收。以 root 执行：

```sh
apt-get update && apt-get install -y ca-certificates curl
curl -fsSL https://raw.githubusercontent.com/hanhaoops/xnvgatebox/v0.6.2/vps/install.sh -o /tmp/xnvgatebox-install.sh && bash /tmp/xnvgatebox-install.sh --version v0.6.2 && xnvgatebox access
```

安装包含管理程序、Pool Builder、固定版本 Xray 和系统协议依赖。VPS 自行解析 VPN Gate 官方当次公开清单，每十分钟刷新，不需要 GitHub 或 Cloudflare 账号，也不需要 3x-ui。

首次安装自动生成独立随机管理路径、初始密码和订阅凭据。上面的一键命令最后会直接显示管理地址和密码；以后也可重新查看：

```sh
xnvgatebox access
```

在浏览器打开它打印的私有管理地址并输入密码。默认管理端口为 8899，服务器防火墙需允许访问此端口。首次安装会生成新的路径、密码和订阅凭据；重装系统后旧地址不会自动恢复。

重复安装或在线更新保留已有路径和密码，`xnvgatebox access` 只查看，不重置凭据。管理地址和密码请保存在私有位置。

## 首次使用：从登录到客户端上网

**不需要先把全部候选测完，也不需要手动新增入站。** 建议先用本机直出验证客户端，再按需要添加 VPN Gate 或自有代理；已有就绪出口时可直接从第 3 步导入订阅。

1. **登录管理页。** 使用安装完成时显示的地址和密码；忘记地址时在 VPS 执行 `xnvgatebox access`。控制台「出口总览」也有可展开的首次使用说明。
2. **建议先启用一个基础连接。** 在「出口管理 → VPS 本机直出」点击「启用」。此入口使用服务器公网 IP，可在没有 VPN Gate 节点时验证客户端接入；它是可选项，不会成为其他出口的故障备用。
3. **把订阅添加到客户端。** 在「我的订阅」点击「复制订阅链接」。在 v2rayN 等支持 VLESS / REALITY 的客户端新建订阅分组、粘贴订阅地址并更新订阅；选择 `VPS 本机直出`（或已就绪出口），启用客户端的系统代理或相应代理模式，再访问网页验证。服务器防火墙须允许管理页显示的实际**接入端口**，只开放 8899 管理端口不足以连接代理。重装系统后须使用新订阅地址。
4. **添加 VPN Gate 出口。** 在「出口管理 → 调整 VPN Gate 数量」填写需要的目标数量，国家可留空。程序自动寻找候选、连接，并创建和绑定对应的 VLESS / REALITY 接入入口；等待出口显示「已就绪」，再在客户端更新原订阅、手动选择该出口。数量由使用者决定，实际受主机资源、地址/端口及上游供给限制；目标数量不保证当时全部就绪。没有可信国家画像时，指定国家的策略可能等待；可先保持国家不限且允许 unknown，仍不能据此称为家宽。
5. **有自己的固定代理时再添加。** 在「代理资源 → 第三方 SOCKS5」填写地址、端口、账号密码，可选国家、城市与备注；保存后检测，确认「启用」与「加入订阅」已开启，再更新客户端订阅。可在「我的订阅」单独选自有代理，也可与其他来源一起使用；客户端显示用户备注，供应商原始凭据留在 VPS。见 [第三方代理说明](docs/THIRD_PARTY_SOCKS.md)。
6. **其他资源按需使用。** 「候选节点」用于刷新来源、单条或批量检测及排障，不是订阅的前置步骤。「代理资源 → VPS SOCKS5」供其他电脑或程序通过本 VPS 接入，需显式开启外部端口；「原始 SSTP」导出给支持 SSTP 的客户端，自身不是可直接导入 v2rayN 的 VLESS 订阅。普通使用无需进入「接入管理」新增入口；自行新增的入口须绑定已有出口，未绑定时阻断流量。

### 页面状态怎样看

| 页面或提示 | 含义与下一步 |
| --- | --- |
| 候选「检测通过」 | 表示那次临时连接通过，不会自动把每个通过的候选变成常驻出口；继续在出口管理设置数量。记录过期或候选配置变化后需重测。 |
| 出口「连接中 / 等待候选」 | 当前还不能提供该出口的订阅连接，等待就绪或查看检测原因；不需要反复新建入站。 |
| 出口「已就绪」 | 当前连接已经通过运行验活，可更新客户端订阅获取入口；不保证上游永久在线。 |
| 「待验证」或 unknown | 国家或类型缺少可信证据，与当前连通状态分开判断；「待验证」且「已就绪」可以同时出现，不能称为已验证家宽。 |
| 订阅选项变灰 | 勾选「全选」时单项由全选控制；取消相应全选后可逐个选择。VPN Gate 实际只输出已就绪入口。 |
| 新建入站「未绑定」 | 只有客户端接入配置，没有出口，连接会阻断；绑定到已有就绪出口，或使用程序自动生成的入口。 |

订阅默认按本机直出 → 自定义代理 → VPN Gate 排列；客户端需主动更新订阅获取变化，再手动选择使用的节点。VPN Gate 和自定义代理故障时阻断，不自动切换本机直出。指定国家暂时没有合适 IP 时保留策略等待，不强求国家或出口数量。

操作细节见 [控制台说明](docs/CONSOLE.md)、[代理资源说明](docs/PROXY_RESOURCES.md) 和 [VPS 文档](vps/README.md)。

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

2026-10-19 使用者已亲自在后台完成 v0.6.0 → v0.6.1 更新，随后核对实际版本与更新记录通过。重装前的当前部署、源码及 Git 历史已在本机私有保存并校验；范围和恢复说明见 [备份与重装](docs/BACKUP_REINSTALL.md)。

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
