2026-10-19 安装与首次使用修复：已合入 PR #7 并发布 [v0.6.2](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.6.2)，正式来源 `3a4d004`。首次缺少项目服务时先读取 systemd LoadState，不再发出 is-enabled 缺失误报；真正的查询失败仍中止安装。首页、控制台总览及安装结束提示补齐登录、可选直出验证、客户端导入/更新订阅、VPN Gate 等待就绪和第三方资源步骤，普通使用无需全量检测或自建入站。82 项本机 Python、既有 Node、GitHub Python 3.9/3.12、Linux 254 个顶层用例 / 313 个通过事件（7 个实测/辅助入口跳过）、race 及正式完整包校验通过，匿名下载与最新正式版接口已验证。本轮只发布 GitHub，未部署新装 VPS；使用者已报告重装 Debian 12 安装成功，浏览器只读观察到候选通过和运行出口就绪，v0.6.2 线上更新及新装客户端全链路仍待另行验证。CF 暂停，3x-ui 不修改，unknown 不称家宽。见 [本轮证据](evidence/github-install-guide-20261019.json) 和 [首次使用](../README.md#首次使用从登录到客户端上网)。

2026-10-19 重装前交接：用户已在后台成功完成 v0.6.0 → v0.6.1 在线更新；随后只读确认运行版本、干净正式来源 `cdc5d7e`、更新状态 completed/complete、无未完成部署以及新版页面，5 项检查通过。停止本项目取得一致私有备份后恢复服务，2,215 个归档文件内容、当前配置/凭据/端口/策略/启停保留、3x-ui 原进程/配置和可用订阅的来源顺序等 7 项复核通过；当时订阅可观察本机直出和自定义代理，未强求 VPN Gate 节点供给。当前完整部署备份已下载 Mac 并核对 SHA256，源码/Git 历史和本机私有输入分别备份并验证。安装文档的一键命令已接上 xnvgatebox access，首次安装自动生成随机路径/密码并立即显示，已有更新保留原凭据。见 [重装与备份说明](BACKUP_REINSTALL.md) 和 [脱敏记录](evidence/pre-reset-20261019.json)。用户自行重装 Debian 12，首次验收先不导入旧状态；全新 OS 和本轮新的代理客户端全链路未计为通过。CF 暂停，3x-ui 不修改，unknown 不称家宽。

2026-10-19 本轮已发布 [v0.6.1](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.6.1)，PR #6 已合入 main，来源 `cdc5d7e`。出口管理统一展示本机直出、自定义 SOCKS5 和 VPN Gate；自定义卡片与代理资源共享检测/启停/编辑，订阅选择页及实际默认/分类订阅按本机直出 → 自定义代理 → VPN Gate 排列，组内稳定编号。Node、浏览器合成演示、GitHub Python 3.9/3.12、Linux 254 个顶层用例 / 313 个通过事件（7 个显式实测/辅助入口跳过）、race 与正式完整包检查通过，公开附件及全部成员摘要/源码快照已验证，最新正式版接口返回 v0.6.1。本轮严格只发布 GitHub，没有访问、部署或重启测试 VPS；v0.6.0 → v0.6.1 的实际在线更新等待用户在后台点击后验收，不提前称为通过。CF 暂停、3x-ui 不动、unknown 不称家宽。见 [本轮记录](evidence/github-exit-sources-20261019.json)。

2026-10-19 当前接续：[v0.6.0](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.6.0) 已发布并通过现有网页在线更新部署，来源 `0bd7113`，PR #5 已合入 main。手动第三方 SOCKS5、国家/城市与备注、稳定 VLESS/REALITY 接入及分类订阅已完成。用户私有代理的 VPS/Mac 准确出口和 HTTPS 内容、17 项实际链路与故障检查、22 项重启/范围/私密性检查、12 项在线更新及 9 项原配置保留检查通过。79 项 Python、256 个 Linux 顶层用例 / 319 个通过事件（4 个显式/辅助入口跳过）、Node 和 GitHub 含 race 检查通过。首轮 VPS/Mac 传输错误和验收脚本在重启后遇到空 VPN Gate 范围的记录保留，按原证书和出口标准复核通过。升级前及最终完整私有备份均保存在 VPS/Mac。见 [使用说明](THIRD_PARTY_SOCKS.md) 和 [脱敏证据](evidence/vps-third-party-20261018.json)。供应商私有文件、凭据和备份不得进入 GitHub；CF 暂停，3x-ui 原进程/配置不动，unknown 不称家宽。全新 Debian 12 操作系统验收仍待用户重装后执行。

2026-10-18 补丁 [v0.5.1](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.5.1) 已发布并通过现有在线升级部署，来源 `64efc17`。修复原始 SSTP 暂缓/取消退回“未检测”和旧轮询覆盖新任务；仅移除设置右侧重复项目链接，在线更新保留。Linux 248 个顶层用例 / 311 个通过事件、Node 竞态回归和 GitHub 检查通过；线上两个真实占用节点、后续任务保留提示、原观测不被覆盖及 9 项配置保留检查通过。一致私有备份保存在 VPS/Mac。原始 SSTP 的直接连接和参考 Actions 实际产物见 [使用说明](SSTP_USAGE.md)，本轮见 [脱敏证据](evidence/vps-sstp-status-20261018.json)。CF 暂停，3x-ui 原进程/配置不动，unknown 不称家宽。

2026-10-18 发布收尾：[v0.5.0](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.5.0) 已发布，来源 `7a65429`，PR #3 已合入 main。Debian 12 测试 VPS 真实完整包在线更新、12 项更新检查、9 项设置保留、实际安装故障恢复、独立新目录直出及最终 10 项复核通过；78 项 Python、246 个 Linux 顶层用例 / 309 个通过事件及 GitHub 含更新 race 通过。VPS/Mac 的直出、原 Slot 2 VLESS/SOCKS 复验通过，Mac 首轮 TLS 传输失败保留并按原标准针对性复核通过。正式版完整私有备份已保存在 VPS 与 Mac。安装、登录、在线更新及全新系统验收见 [安装更新说明](INSTALL_UPDATE.md) 和 [脱敏证据](evidence/vps-release-20261018.json)；用户重装 Debian 12 后的全新 OS 验收仍待执行。CF 暂停，3x-ui 原进程/配置保留，unknown 不称家宽。

# Mac 开发交接（2026-10-15）

2026-10-18 代理资源已开发并部署：干净开发包 `5daca26` 增加“代理资源 → VPS SOCKS5 / 原始 SSTP”，复用官方候选池，在 VPS 本地隔离验证 SSTP/PPP、正常证书、两处准确 HTTPS 出口及 HTTPS 内容，私有保存与导出，支持用户并发、优先/去重/取消及可关闭的周期检测。首轮 98 个候选中 57 个通过，后续复查及重启快照为 54 个有效结果；数字是当时观测，不保证后续供给。239 个 Linux 顶层用例 / 295 个通过事件、64 项 Python、最终 GitHub 全部检查含新增周期与并发 race 通过。16 项升级保留检查、真实 16 路取消清理、私有导出、重启保留及最终 13 项复核通过；完整私有备份保留在 VPS 和 Mac。原默认订阅的 VPS 直出与原 Slot 2 的 VLESS/SOCKS 已通过 VPS/Mac 复验；Slot 1/3 正向连通受上游条件限制，不强求国家或数量。保留原 32 目标、16 并发、端口、凭据与策略；CF 暂停、3x-ui 原进程/配置不动，unknown 不称家宽。见 [代理资源约定与记录](PROXY_RESOURCES.md) 和 [脱敏证据](evidence/vps-sstp-resources-20261018.json)。

2026-10-18 追加：干净开发包 `ff10d2a` 已部署，VPS 本机直出进入默认订阅；外部认证 SOCKS5 可显式开启并私有导出，内部监听保持本机。原 Slot 1/2 的 VLESS、外部 SOCKS 及直出通过 VPS/Mac 复验；停止 Slot 1 时两端无回落，恢复和正常重启后端口、凭据及意图保留。Linux 231 个顶层用例 / 287 个通过事件、GitHub 含 race、23 项升级保留与最终 10 项复核通过。首轮传输失败、一次最终 TLS 重试均保留。原 32 目标和用户已保存的 16 并发保留，原 Slot 1/2 已开启外部 SOCKS，其他出口由用户选择。见 [最新证据](evidence/vps-direct-socks-20261018.json) 和 [操作说明](CONSOLE.md)。CF 暂停，Worker 未实测，不公开上传凭据；证书自动申请暂缓，3x-ui 不修改，unknown 不称家宽。

2026-10-18 最新接续：最终干净开发包 `19e2840` 已部署原授权 VPS，统一检测与用户自定并发/出口数，完整候选清单、手动优先和动态网络身份已实现。Linux 227 个顶层用例 / 283 个通过事件、GitHub 含 race、真实四路取消/强杀恢复、正常重启及 22 项配置保留检查通过。安装服务旧 32 上限和停服时后台保存停止标志的竞态均已修复并保留失败记录，原启停意图已恢复。客户端验收范围见 [最新脱敏证据](evidence/vps-unified-checks-20261018.json)，操作与真实地址容量见 [CONSOLE.md](CONSOLE.md)。CF 暂停，3x-ui 不修改，unknown 不称家宽；正式发布仍为 v0.4.0。

2026-10-18 更新：开发分支 `codex/console-capacity-20261017` 的干净包 `ee0e2d1` 已部署原授权 VPS，包含默认两路/可选逐个候选检测、阶段显示、保守 TCP 预检和“接入管理 / 新增接入入口”命名。Linux 216 个顶层用例 / 272 个通过事件、并发路径 race、真实双诊断取消/强杀恢复、原三条 VPS/Mac 订阅复验通过。升级前完整私有备份已下载 Mac，当前 32 个目标、凭据与线上已有检测任务保留。见 [脱敏证据](evidence/vps-parallel-checks-20261018.json)；接续以此及 [CONSOLE.md](CONSOLE.md) 的最新记录为准。正式发布仍为 v0.4.0，CF 保持暂停，3x-ui 不修改，unknown 不称家宽。

2026-10-17 追加：登录与连接配置页已重做，旧默认 Slot 名称兼容已修复；测试 VPS 已安装干净开发包 `95cd217`，保留升级前当前的 32 个目标出口及现有配置。新验证记录见 [控制台说明](CONSOLE.md#验证记录)。CF 继续暂停，3x-ui 不修改。

2026-10-17 控制台开发接续：本机分支 `codex/console-capacity-20261017` 保留所有私有运行文件；新控制台、出口数量和独立检测说明见 [CONSOLE.md](CONSOLE.md)。正式版仍是下述 v0.4.0，开发候选已完成 VPS/Mac 四出口订阅和恢复三个原出口后的强杀复验，见 [控制台验收](evidence/vps-console-20261017.json)。

2026-10-16 更新：本轮成果已合入 main 并发布 [v0.4.0](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.4.0)，干净候选包经测试 VPS 升级和 Mac 外部复验，公开附件与已验收候选一致。见 [发布证据](evidence/vps-release-20261016.json)。

本机已接续 `389288d` 并进行了真实 VPS 升级、订阅、故障及完整备份恢复测试；修复和保留的失败记录见 [Mac 实测](VPS_UPGRADE_VALIDATION.md#2026-10-15-mac-实测)。云端限制仅描述最初交接环境；当前以最新实测章节为准。

后续用户明确授权第三 Slot 换其他国家，当前为 JP/KR/KR；换国家后的三个独立出口已完成本机及 Mac 真实订阅、第三 Slot 强杀阻断与恢复复验；原端口、订阅和客户端凭据保留。最新追加验收见 [授权换国家记录](VPS_UPGRADE_VALIDATION.md#2026-10-15-用户授权第三-slot-换国家)。

后续验收不强求指定国家当时必须有合适 IP。上游候选不足时，确认程序正确提示、保留策略和端口、等待重试及阻断回落即可；该国连通项记为上游条件不足，稍后有节点再测或由用户选择其他国家。三个独立出口可以来自同一国家，不因某国暂不可用而判定程序验收失败。

接续分支：`codex/standalone-migration-20261015`。该分支包含云端开发和后续 Mac 实测修复，测试 VPS 已升级并完成验收；已合入 main 并发布 v0.4.0，以下保留接续及复验说明。请先读取 [DEVELOPMENT.md 顶部](DEVELOPMENT.md) 和 [历史 VPS 实测记录](VPS_STANDALONE_VALIDATION.md)。

## 在 Mac 建立独立开发目录

以下依赖安装命令适用于已经安装 Homebrew 的 Mac。新目录避免覆盖之前本机未提交的工作；目录已存在时更换名字，不要直接清理旧工作区。

```sh
brew install python@3.12 go node
git clone --branch codex/standalone-migration-20261015 --single-branch \
  https://github.com/hanhaoops/xnvgatebox.git xnvgatebox-mac
cd xnvgatebox-mac
python3.12 -m venv .venv
source .venv/bin/activate
python -m pip install -e .
python scripts/install_xray.py
python -m unittest discover -s tests -v
node tests/test_worker_guards.mjs
node tests/test_cf_control.mjs
```

安装脚本按 Mac 的 Intel/Apple Silicon 架构下载对应官方 Xray，校验仓库固定 SHA256；不要复制云端的 Linux 二进制。Python 依赖为标准库，pip 安装用于设置本地入口。若下载失败，保留错误并处理本机网络或 CA 配置，不关闭 TLS 校验。

Go 管理器依赖 Linux namespace，不在 macOS 上运行。Mac 可编辑源码、运行 Python/Node 测试，并交叉编译 Linux 二进制：

```sh
mkdir -p build
(cd vps/fanout && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags '-X main.version=dev-mac' \
  -o ../../build/xnvgatebox-manager .)
```

完整 Go 单元测试及真实 namespace/监听测试应在有权限的 Linux 环境执行。不要在 Mac 上用替代 namespace 行为伪装这些测试通过。

## 已完成与验证范围

- OpenVPN 实际出口画像：共享 Python adapter/classifier，独立 SOCKS actual 核验，私有运行证据；缺服务、失败或过期如实 unknown，不能称家宽。
- Slot 策略及生命周期：国家、类型、ISP/ASN、HTTPS 延迟限制持久化，缺证据等待；固定身份与端口，停止/恢复/删除，取消旧任务并清理连接。
- 配置回滚：内置 Xray 候选先静态校验，文件事务与失败补偿、未完成事务恢复；手动恢复上次内置配置/策略，无法恢复则阻断。Xray 整体重启可能中断其他连接，回滚可能恢复旧客户端凭据。
- 迁移检查：候选管理器 `-check-state -dir 私有状态副本目录` 只读检查，不启动服务或生成凭据；旧版三 Slot 合成样例及恢复前 blackhole 验证。
- 验收工具：`scripts/vps_acceptance.py` 采集私有计划、读取真实订阅并验证三 Slot、检查本机 SOCKS、指定 OpenVPN 故障/恢复、比较迁移保留项。VPS 本机公开客户端与外部客户端报告分开标记。
- 打包：本地固定 Xray 归档也必须校验，`BUILD.json` 记录提交、dirty 状态及源码摘要。默认版本为 `dev`，正式版本必须显式指定。

云端最后验证：Python 普通 59 项通过，完整 61 项仅两项既有真实 Xray 进程测试受 socket 禁令影响；Go 普通 174 个顶层用例通过（含子用例 224 个 pass 事件），迁移/配置/策略路径 race 通过。固定 Xray 服务端/客户端静态解析、开发构建、嵌入管理 JS 语法、测试包成员校验和解包后的离线检查通过。云端没有完成真实 API/OpenVPN/SOCKS/公开订阅的新版本验收，不能把这些本地结果当作真实链路成功。

之前构建的 `dev-20261015-migration` 包在云端私有工作目录，未上传 GitHub release；Mac 可以从本分支重新构建，无需依赖那个包。

## 可重复的 Mac 验收流程

用户已经授权原 VPS 作为测试服务器，允许备份、升级和真实故障验收；测试机 SSH 信息通过私有渠道保存，密码不得写入仓库、脚本、命令行参数或公开报告。云端阶段未建立 SSH；后续 Mac 已实际完成本项目的备份、升级、故障和完整还原验收。以下步骤保留用于复验。

1. 核对 SSH 主机身份、本项目服务/状态目录及 3x-ui 状态，只操作本项目。Cloudflare 已撤下暂停，不恢复其部署或自动任务。
2. 停止本项目并取得完整私有备份（旧二进制、Xray、服务/环境、状态与证书），对一致副本执行候选 `-check-state`。检查画像配置及有效策略：没有可信国家/ISP证据时等待是预期行为，不自动放宽。
3. 依据 [VPS_UPGRADE_VALIDATION.md](VPS_UPGRADE_VALIDATION.md) 构建 Linux 测试包、核验后升级，比较 Slot/端口/token/UUID/密钥保留情况。若从 Mac 打包，构建器需通过 `--xray-archive` 提供 **Linux** 官方归档（同样校验固定 pin）；不能把 Mac Xray 放入 Linux 包。
4. 上游候选充足时验证三个实际独立出口和真实聚合订阅，国家可以重复；候选不足时验证正确等待和阻断，并记录尚未覆盖的连通项。指定 Slot 掉线后不得回落母机，有合适节点恢复后验证原订阅可用；Mac 外部结果单独保存。真实配置提交/运行回滚故障也需单独验收。
5. 按已覆盖功能、程序缺陷和上游可用性分别更新验收记录，再评估合入 main 和准备 release。程序故障保留证据，必要时按完整备份恢复旧安装；单纯某国缺少节点不要求回退安装。

所有合成样例中的密码和 RFC 7748 密钥均为公开测试数据，不能用于部署。私有 `state.json`、`native.json`、`settings.json`、事务恢复记录、原始订阅和验收计划都不能提交。仓库之外的云端日志和 SSH 网络诊断不属于本分支交付内容。
