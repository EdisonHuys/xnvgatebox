# xnvgatebox

在 Linux VPS 上管理多个隔离出口，使用固定本地 SOCKS 端口和内置 Xray，通过一个订阅链接聚合多个出站。无需安装 3x-ui；已有面板可以使用导出的标准 Xray JSON 手工接入。

**当前开发重心是 VPS Standalone。Cloudflare 部署已备份撤下，旧 CF 自动发布任务已停用。** Mode A 的源码与测试暂保留为历史参考，不是当前部署入口。

最新发布：[v0.4.0](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.4.0)，已完成干净源码构建及 VPS/Mac 候选升级实测，见 [发布验收记录](docs/evidence/vps-release-20261016.json)。

开发中的新版控制台与可配置出口数量见 [控制台说明](docs/CONSOLE.md)，发布版操作流程见下文。

## VPS 一条命令安装

目前支持 Debian / Ubuntu、Linux amd64、systemd，以 root 执行：

```sh
curl -fsSL https://raw.githubusercontent.com/hanhaoops/xnvgatebox/main/vps/install.sh | bash
```

安装包包含本项目管理程序、统一 Pool Builder 和校验固定版本的 Xray。安装后由 VPS 自己完整解析官方当次公开清单并构建候选池，每十分钟刷新，无需 GitHub/Cloudflare 账号或远程节点池。再次安装前自动备份已有项目状态。

运行 `xnvgatebox access` 查看直接访问 VPS 的管理地址和密码。v0.4.0 发布版界面操作：新建出口 → 新建 VLESS/REALITY 节点 → 绑定出口 → 复制订阅。一个订阅包含所有已绑定入站，各入站使用独立公开端口和对应固定 SOCKS Slot。不要把本地 SOCKS 端口当作客户端的 VLESS 端口。

`xnvgatebox refresh` 立即刷新候选池，`xnvgatebox status` 查看服务；`xnvgatebox install --mode vps` 更新完整部署包。v0.4.0 发布版默认最多三个 Slot，国家可在界面选择；本开发分支改为 0–32 个目标出口，自动建立并绑定入站。未绑定或失效出口的内置 Xray 流量被阻断。

指定国家能否建立出口，取决于 VPN Gate 上游当时提供的节点及其可达性。暂时没有合适 IP 时，如实提示暂不可用或等待，不要求每个国家随时有出口，也不因此判定本程序故障。功能验收可使用当时可用的国家；三个独立出口可以来自同一国家。

候选池只说明发现了节点；OpenVPN 与 SOCKS 必须在目标 VPS 实测后才显示健康。没有出口画像证据时类型显示 unknown，不能宣传为已验证家宽。详细安装、验收和限制见 [VPS 文档](vps/README.md)。许可证与来源见 [参考审计](docs/REFERENCE_AUDIT.md)。

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
