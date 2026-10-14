# VPN Gate 家宽出口工具箱

共用一个 Pool Builder，先实现无 VPS 的 Cloudflare/VLESS 验活，再实现固定 VPS Exit Slot。VPN Gate Box 管理界面同时提供 Validated Nodes、Exit Slots、节点切换与订阅导出。开发基准见 [DEVELOPMENT.md](docs/DEVELOPMENT.md)。

第一轮已经提供可执行的候选池构建、SSTP HTTP 检测适配、strict/likely 家宽分级、VLESS/WS 订阅生成、Xray 最终链路检查和 Actions 工作流。**2026-10-08 已部署两个独立 Cloudflare Worker，并通过 v0.1 真实链路验收：10 个候选中 8 个 SSTP 通过、2 个最终 VLESS 通过，失效 SSTP 后端的两次请求均失败。** 2026-10-09 又在公开 GitHub Actions 中完成托管验收：50 个候选中 39 个 SSTP 通过、27 个最终 VLESS 通过；同日完成一台小规格 Debian VPS 的 v0.2 单 Slot 验收，固定 `127.0.0.1:17928` 经 namespace 隔离、expected/actual 一致，强杀 OpenVPN 后 SOCKS 失败并自动恢复。VPN Gate Box WebUI 已部署到 VPS，能显示共享 Validated Nodes、Exit Slots、健康状态、手动换节点和订阅入口；3x-ui 自动写入和多 Slot 仍待后续验收。当前节点画像仍为 unknown，住宅列表为空。实测记录见 [CF_DEPLOYMENT.md](docs/CF_DEPLOYMENT.md) 与 [VPS_DEPLOYMENT.md](docs/VPS_DEPLOYMENT.md)。

## 本机使用

Python 3.9+，核心仅使用标准库；最终验活还需要 curl 和 Xray。Linux/macOS 支持本机文件锁。

```sh
python3 scripts/toolbox.py pool
```

无需 Cloudflare、UUID 或远程节点池地址即可生成 `data/node_pool.json`、对应 SHA256 和 `data/configs/*.ovpn`。OpenVPN 配置不执行，协议状态保持 `not_tested`；本命令是后续 VPS Standalone 使用的同一个 Builder。可用 `--csv-file /absolute/path/vpngate.csv` 解析官方格式快照。国家过滤、候选预算等在 `config/settings.json` 修改。

开始 Mode A 前，部署一次 xnvgatebox Worker；它提供管理、检查、数据面和订阅入口。按 [Worker 适配说明](worker/README.md) 核对统一入口，复制 `.env.example` 为 `.env`，设置 `VLESS_UUID` 和必要的凭据。可选访问控制与画像参数也在此文件；工具读取 `.env`，不执行其中任何 shell 表达式。

```sh
python3 scripts/install_xray.py
python3 scripts/toolbox.py mode-a
```

安装器使用 [固定的官方版本与 SHA256](config/xray-release.json)，支持 Linux x86_64、macOS arm64/x86_64。也可用 `--archive /absolute/path/archive.zip` 校验离线归档，不使用 latest 动态下载。

执行顺序：CSV → 真 SSTP/PPP 检测 → 出口画像 → 无效 SSTP 反向测试 → 实际 VLESS/WS 配置 → 两个独立 HTTPS Echo 各测两次 → 小体积 HTTPS 内容检查。任意出口不相等、测试服务异常、配置变化或无效 SSTP 能成功访问，都不发布该链路。Xray 临时配置权限 0600，进程、端口和文件退出时清理。

| 产物 | 含义 |
| --- | --- |
| `data/node_pool.json` | 候选与协议证据；不等于最终可消费列表 |
| `data/mode_a_validated.json` | 当前环境最终 VLESS 检查通过且带有效期的节点 |
| `data/residential.json` | 最终通过且分类为 strict 或 likely 的节点；保留两者差别 |
| `data/cf_manifest.json` | 可发布到 Cloudflare KV 的无凭证统一节点池 |
| `data/nodes.txt` | 最终通过节点的 SSTP 入口清单，供查看，不含 VLESS UUID |
| `data/report.json` | 反向测试、最终出口、配置摘要和失败原因，不含 UUID |
| `runtime/subscription.txt` | 通过验证的 VLESS 明文订阅，权限 0600，不提交、不上传公开 artifact |

`mode-a` 每次开始会清空旧的发布视图和订阅；异常、缺参数或零节点成功不会沿用旧订阅。返回码 0 表示成功，2 表示输入/配置/工具问题，3 表示没有通过的最终节点。证据默认 60 分钟有效；导出器拒绝过期证据。已经复制到其他客户端的链接不能由本地文件撤回，客户端应刷新订阅。

住宅类型不是可达性的前提；未知或机房出口仍可出现在最终可用列表，但不会进入住宅列表。画像 API 可用 `INTELLIGENCE_URL_TEMPLATE=https://iplocate.io/api/lookup/{ip}` 和 `INTELLIGENCE_TOKEN`，密钥使用 `X-API-Key` 头；相同 IP 默认缓存 24 小时，每次查询有预算。原 Checker 画像接口实测返回 429，新部署版本改为官方 IP-only 端点取得出口基线；省略独立画像配置时 ASN/ISP/住宅类型保持 unknown，并保留 T1 来源标签，不补 false。消费者 ASN/ISP 清单只是 likely 的辅助条件，可配置，不能单独证明住宅。

## GitHub Actions

工作流 [update.yml](.github/workflows/update.yml) 支持手动触发，并默认每 20 分钟尝试自动刷新：

- `pool-only`：无需任何 Cloudflare Secrets，构建共享候选池。
- `serverless`：设置与 `.env.example` 同名的仓库 Secrets 后执行实际链路检查；可选打开 `publish_cf` 将 `data/cf_manifest.json` 发布到自有 control Worker 的 KV。

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
