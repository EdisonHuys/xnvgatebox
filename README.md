# xnvgatebox

在 Linux VPS 上管理多个隔离出口，使用固定本地 SOCKS 端口和内置 Xray，通过一个订阅链接聚合多个出站。无需安装 3x-ui；已有面板可以使用导出的标准 Xray JSON 手工接入。

**当前开发重心是 VPS Standalone。Cloudflare 部署已备份撤下，旧 CF 自动发布任务已停用。** Mode A 的源码与测试暂保留为历史参考，不是当前部署入口。

## VPS 一条命令安装

目前支持 Debian / Ubuntu、Linux amd64、systemd，以 root 执行：

```sh
curl -fsSL https://raw.githubusercontent.com/hanhaoops/xnvgatebox/main/vps/install.sh | bash
```

安装包包含本项目管理程序、统一 Pool Builder 和校验固定版本的 Xray。安装后由 VPS 自己从官方全量清单构建候选池，每十分钟刷新，无需 GitHub/Cloudflare 账号或远程节点池。再次安装前自动备份已有项目状态。

运行 `xnvgatebox access` 查看直接访问 VPS 的管理地址和密码。界面操作：新建出口 → 新建 VLESS/REALITY 节点 → 绑定出口 → 复制订阅。一个订阅包含所有已绑定入站，各入站使用独立公开端口和对应固定 SOCKS Slot。不要把本地 SOCKS 端口当作客户端的 VLESS 端口。

`xnvgatebox refresh` 立即刷新候选池，`xnvgatebox status` 查看服务；`xnvgatebox install --mode vps` 更新完整部署包。默认最多三个 Slot，国家可在界面选择。未绑定或失效出口的内置 Xray 流量被阻断。

候选池只说明发现了节点；OpenVPN 与 SOCKS 必须在目标 VPS 实测后才显示健康。没有出口画像证据时类型显示 unknown，不能宣传为已验证家宽。详细安装、验收和限制见 [VPS 文档](vps/README.md)。许可证与来源见 [参考审计](docs/REFERENCE_AUDIT.md)。

## 本机使用

Python 3.9+，核心仅使用标准库；最终验活还需要 curl 和 Xray。Linux/macOS 支持本机文件锁。

```sh
python3 scripts/toolbox.py pool
```

无需 Cloudflare、UUID 或远程节点池地址即可生成 `data/node_pool.json`、对应 SHA256 和 `data/configs/*.ovpn`。OpenVPN 配置不执行，协议状态保持 `not_tested`；本命令是后续 VPS Standalone 使用的同一个 Builder。可用 `--csv-file /absolute/path/vpngate.csv` 解析官方格式快照。国家过滤、候选预算等在 `config/settings.json` 修改。

旧 Cloudflare 验活资料保留在历史文档，当前不部署或执行 Mode A。

## GitHub Actions

工作流 [update.yml](.github/workflows/update.yml) 支持手动触发，并默认每 20 分钟尝试自动刷新：

- `pool-only`：无需任何 Cloudflare Secrets，构建共享候选池。
- `serverless`：在仓库 Variables 配置 `WORKER_URL`（HTTPS 订阅入口，不含路径或 token），再设置 UUID 等仓库 Secrets，执行实际链路检查；可选打开 `publish_cf` 将 `data/cf_manifest.json` 发布到唯一 Worker 的 KV。检查与数据面地址都由 `WORKER_URL` 派生，无部署域名硬编码。

定时运行使用 `serverless` 链路并自动发布 Cloudflare manifest；手动运行仍可选择 `pool-only` 或 `serverless`，并可显式打开 `publish_cf`。公开 artifact 只包括池、配置数据、验活证据、`cf_manifest.json` 和报告，不包含 `.env` 或订阅。成功后下载 `mode_a_validated.json` 等证据到 `data/`，在本机使用相同数据面/UUID 和配置生成私有订阅：

```sh
python3 scripts/toolbox.py export-subscription
```

导出会重新比较订阅配置摘要、最终 IP 与有效期，拒绝用不同 UUID/域名/路径拼接旧验证结果。只信任你自己的证据来源；SHA256 是内容校验，不是发布者签名。Cloudflare KV 发布和 control Worker 部署需要按 [`worker/control/README.md`](worker/control/README.md) 配置；定时任务可能延迟数分钟，manifest 通过有效期拒绝过期订阅，必要时仍可从 Actions 页面手动运行。

## 验证

```sh
python3 -m unittest discover -s tests -v
node tests/test_worker_guards.mjs
node tests/test_cf_control.mjs
```

31 项 Python 测试覆盖入口变化、字段缺失、画像冲突、429、IP 不匹配、直出反向测试、证书错误拒绝、过期和配置变化拒绝发布、旧订阅清空、密钥不进入公共产物、锁和清理；29 项 Worker 边界检查加 10 项 control Worker 测试覆盖认证、协议、目标限制、过期 manifest 和参数覆盖拒绝。安装 Xray 后额外检查真实二进制接受配置、loopback 监听与进程清理。构造测试与真实 VPN 验收分别记录；当前真实证据在 `data/report.json`、`data/mode_a_validated.json`，有效期为一小时，不能据此承诺节点持续在线。

第一轮结果与后续实测步骤见 [ROUND1.md](docs/ROUND1.md)。核心工具许可为 GPL-3.0-only；独立 EdgeTunnel 修改为 GPL-2.0-only，依赖与复用边界见 [THIRD_PARTY.md](THIRD_PARTY.md)。本版不支持未经验证的 UDP、IPv6、XHTTP。

接口来源：[VPN Gate 官方 CSV](https://www.vpngate.net/en/)、[Xray 配置文档](https://xtls.github.io/en/config/transport.html)、[ipify 官方接口](https://www.ipify.org/)、[icanhaz 官方实现](https://github.com/major/icanhaz)、[IPLocate 身份认证](https://www.iplocate.io/docs/getting-started/authentication)。
