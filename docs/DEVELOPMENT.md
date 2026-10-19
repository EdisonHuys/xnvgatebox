2026-10-19 卸载补丁开发意图：用户实际运行 v0.8.0 独立工具，在 systemctl reset-failed 处中止；按执行顺序尚未进入文件/备份删除阶段，可能已停用部分项目服务。修复正常 inactive 单元仍被无条件重置的问题，仅对 failed 单元清除失败状态并复核；竞态中已转为 inactive 或已卸载须有查询证据，真正系统操作失败继续保留文件。增加严格的服务状态回归和 GitHub runner 真实 systemd 验证，保留隔离网络清理检查；发布 v0.8.1 独立工具与完整包，旧版本无需重装即可重新下载后重试。只更新 GitHub，不连接或卸载用户 VPS；CF 暂停，3x-ui 不修改，unknown 不称家宽。下方 v0.8.0 记录保留历史测试边界。

2026-10-19 正式卸载与旧版清理已发布 [v0.8.0](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.8.0)，PR #9 已合入，正式来源 `2cc003c`。新增 xnvgatebox uninstall 与旧版本可直接下载的独立工具，支持只读预览、执行确认、默认保留备份、--purge 和 --legacy。部署锁、未完成恢复、服务/实例占用、残留进程、符号链接、挂载点与归属冲突均受保护；按实例、网段、记录和实际接口精确清理，不重置全局防火墙/IP 转发，不移除共享依赖或修改 3x-ui。删除旧 install.sh、f.sh 和 fanout.service，将组件 README 改为本项目说明，许可证、上游来源和运行兼容保留。99 项本机 Python（含 14 项卸载回归）、GitHub Python 3.9/3.12、Linux 258 个顶层用例 / 317 个通过事件与 race 通过，7 个实测/辅助入口跳过；新增隔离 Linux 验证实际清理 3 个 namespace、接口和规则，保留对照资源，验证残留进程拒绝和重复执行，服务控制使用模型。38 个包成员、源码快照、匿名下载、固定标签安装器及独立卸载脚本核验通过，latest 返回 v0.8.0。只发布 GitHub，未部署或卸载用户 VPS；实际 systemd 卸载和新装客户端链路仍待单独验收。CF 暂停，3x-ui 不修改，unknown 不称家宽。见 [卸载说明](UNINSTALL.md) 和 [脱敏记录](evidence/github-uninstall-20261019.json)。

2026-10-19 首次使用流程修复已发布 [v0.7.0](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.7.0)，PR #8 已合入，正式来源 `0e3aebe`。候选观察按配置摘要和 15 分钟有效期参与选点，正式连接仍完整验活；诊断与正式 OpenVPN 建连共享用户并发，恢复优先、等待名额单独显示。国家查询按实测 IP 进行正常 HTTPS 验证、精确匹配和缓存，与住宅分类分离，unknown 不抬升。接入管理统一列表，新增可绑定等待中的稳定出口，自动/手动入口有明确启停与删除语义；修正详情实际安全方式显示。首次未保存数量预填 4，保留既有 0/数量。首页只留「设置出口 → 等待就绪 → 复制订阅 → 导入使用」，每步 4 字；长说明移至 [首次使用](FIRST_USE.md)。85 项本机 Python、Node、浏览器合成演示、GitHub Python 3.9/3.12、Linux 258 个顶层用例 / 317 个通过事件及 race 通过，7 个实测/辅助入口跳过；37 个包成员、源码快照、匿名下载、固定标签脚本和 latest 接口校验通过。本轮严格只发布 GitHub，未部署新装 VPS，新版后台实际升级与客户端全链路待用户操作后验收。旧 SSH 身份不可沿用，后续访问须核实重装后的主机指纹。CF 暂停，3x-ui 不修改，unknown 不称家宽。见 [脱敏记录](evidence/github-first-use-flow-20261019.json)。

2026-10-19 安装与首次使用修复：已合入 PR #7 并发布 [v0.6.2](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.6.2)，正式来源 `3a4d004`。首次缺少项目服务时先读取 systemd LoadState，不再发出 is-enabled 缺失误报；真正的查询失败仍中止安装。首页、控制台总览及安装结束提示补齐登录、可选直出验证、客户端导入/更新订阅、VPN Gate 等待就绪和第三方资源步骤，普通使用无需全量检测或自建入站。82 项本机 Python、既有 Node、GitHub Python 3.9/3.12、Linux 254 个顶层用例 / 313 个通过事件（7 个实测/辅助入口跳过）、race 及正式完整包校验通过，匿名下载与最新正式版接口已验证。本轮只发布 GitHub，未部署新装 VPS；使用者已报告重装 Debian 12 安装成功，浏览器只读观察到候选通过和运行出口就绪，v0.6.2 线上更新及新装客户端全链路仍待另行验证。CF 暂停，3x-ui 不修改，unknown 不称家宽。见 [本轮证据](evidence/github-install-guide-20261019.json) 和 [首次使用](../README.md#首次使用从登录到客户端上网)。

2026-10-19 重装前交接：用户已在后台成功完成 v0.6.0 → v0.6.1 在线更新；随后只读确认运行版本、干净正式来源 `cdc5d7e`、更新状态 completed/complete、无未完成部署以及新版页面，5 项检查通过。停止本项目取得一致私有备份后恢复服务，2,215 个归档文件内容、当前配置/凭据/端口/策略/启停保留、3x-ui 原进程/配置和可用订阅的来源顺序等 7 项复核通过；当时订阅可观察本机直出和自定义代理，未强求 VPN Gate 节点供给。当前完整部署备份已下载 Mac 并核对 SHA256，源码/Git 历史和本机私有输入分别备份并验证。安装文档的一键命令已接上 xnvgatebox access，首次安装自动生成随机路径/密码并立即显示，已有更新保留原凭据。见 [重装与备份说明](BACKUP_REINSTALL.md) 和 [脱敏记录](evidence/pre-reset-20261019.json)。用户自行重装 Debian 12，首次验收先不导入旧状态；全新 OS 和本轮新的代理客户端全链路未计为通过。CF 暂停，3x-ui 不修改，unknown 不称家宽。

2026-10-19 本轮已发布 [v0.6.1](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.6.1)，PR #6 已合入 main，来源 `cdc5d7e`。出口管理统一展示本机直出、自定义 SOCKS5 和 VPN Gate；自定义卡片与代理资源共享检测/启停/编辑，订阅选择页及实际默认/分类订阅按本机直出 → 自定义代理 → VPN Gate 排列，组内稳定编号。Node、浏览器合成演示、GitHub Python 3.9/3.12、Linux 254 个顶层用例 / 313 个通过事件（7 个显式实测/辅助入口跳过）、race 与正式完整包检查通过，公开附件及全部成员摘要/源码快照已验证，最新正式版接口返回 v0.6.1。本轮严格只发布 GitHub，没有访问、部署或重启测试 VPS；v0.6.0 → v0.6.1 的实际在线更新等待用户在后台点击后验收，不提前称为通过。CF 暂停、3x-ui 不动、unknown 不称家宽。见 [本轮记录](evidence/github-exit-sources-20261019.json)。

2026-10-19 当前完成：[v0.6.0](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.6.0) 已发布并通过网页完整包在线更新部署，来源 `0bd7113`，PR #5 已合入 main。手动第三方 SOCKS5、国家/城市与备注、稳定 VLESS/REALITY 入口和分类订阅已实现；不接供应商 API，不开发 GitHub 自动节点发布。79 项 Python、256 个 Linux 顶层用例 / 319 个通过事件（4 个显式/辅助入口跳过）、Node 和 GitHub 含 race 通过。真实供应商链路经 VPS/Mac 准确出口与 HTTPS 内容验证；17 项实际链路/故障、22 项重启/范围/私密性、12 项在线更新及 9 项原配置保留检查通过。初始传输失败和重启后空 VPN Gate 范围均保留记录；最终备份与升级前备份已保存在 VPS/Mac。需求先于实现、操作与限制见 [第三方 SOCKS5](THIRD_PARTY_SOCKS.md)，实测见 [脱敏证据](evidence/vps-third-party-20261018.json)。CF 暂停、3x-ui 原进程/配置不动、unknown 不称家宽，全新 Debian 12 验收待用户重装后执行。

2026-10-18 补丁 [v0.5.1](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.5.1) 已发布并通过现有在线升级部署，来源 `64efc17`。修复原始 SSTP 暂缓/取消退回“未检测”和旧轮询覆盖新任务；仅移除设置右侧重复项目链接，在线更新保留。Linux 248 个顶层用例 / 311 个通过事件、Node 竞态回归和 GitHub 检查通过；线上两个真实占用节点、后续任务保留提示、原观测不被覆盖及 9 项配置保留检查通过。一致私有备份保存在 VPS/Mac。原始 SSTP 的直接连接和参考 Actions 实际产物见 [使用说明](SSTP_USAGE.md)，本轮见 [脱敏证据](evidence/vps-sstp-status-20261018.json)。CF 暂停，3x-ui 原进程/配置不动，unknown 不称家宽。

2026-10-18 发布收尾：[v0.5.0](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.5.0) 已发布，来源 `7a65429`，PR #3 已合入 main。Debian 12 测试 VPS 真实完整包在线更新、12 项更新检查、9 项设置保留、实际安装故障恢复、独立新目录直出及最终 10 项复核通过；78 项 Python、246 个 Linux 顶层用例 / 309 个通过事件及 GitHub 含更新 race 通过。VPS/Mac 的直出、原 Slot 2 VLESS/SOCKS 复验通过，Mac 首轮 TLS 传输失败保留并按原标准针对性复核通过。正式版完整私有备份已保存在 VPS 与 Mac。安装、登录、在线更新及全新系统验收见 [安装更新说明](INSTALL_UPDATE.md) 和 [脱敏证据](evidence/vps-release-20261018.json)；用户重装 Debian 12 后的全新 OS 验收仍待执行。CF 暂停，3x-ui 原进程/配置保留，unknown 不称家宽。

# 当前开发：控制台与可配置出口数量（2026-10-17）

2026-10-18 代理资源已开发并部署：干净开发包 `5daca26` 增加“代理资源 → VPS SOCKS5 / 原始 SSTP”，复用官方候选池，在 VPS 本地隔离验证 SSTP/PPP、正常证书、两处准确 HTTPS 出口及 HTTPS 内容，私有保存与导出，支持用户并发、优先/去重/取消及可关闭的周期检测。首轮 98 个候选中 57 个通过，后续复查及重启快照为 54 个有效结果；数字是当时观测，不保证后续供给。239 个 Linux 顶层用例 / 295 个通过事件、64 项 Python、最终 GitHub 全部检查含新增周期与并发 race 通过。16 项升级保留检查、真实 16 路取消清理、私有导出、重启保留及最终 13 项复核通过；完整私有备份保留在 VPS 和 Mac。原默认订阅的 VPS 直出与原 Slot 2 的 VLESS/SOCKS 已通过 VPS/Mac 复验；Slot 1/3 正向连通受上游条件限制，不强求国家或数量。保留原 32 目标、16 并发、端口、凭据与策略；CF 暂停、3x-ui 原进程/配置不动，unknown 不称家宽。见 [代理资源约定与记录](PROXY_RESOURCES.md) 和 [脱敏证据](evidence/vps-sstp-resources-20261018.json)。

2026-10-18 本机直出与外部 SOCKS5 已完成并部署：干净开发包 `ff10d2a` 增加独立启停的“VPS 本机直出”，原默认订阅包含已启用直出，支持范围排除或只选直出；VPN Gate 故障、停止、删除及未绑定仍阻断，绝不自动切换。SOCKS5 支持显式开启独立外部认证端口，复用已有 Slot 隧道与账号口令，内部端口仍只监听本机；提供登录后的私有导出。Linux 231 个顶层用例 / 287 个通过事件、GitHub 含 race、23 项升级保留检查及最终 10 项复核通过。直出及原 Slot 1/2 的 VLESS、外部 SOCKS 已由 VPS 和 Mac 实测，停止 Slot 1 时两端阻断且直出对照继续可用；恢复及正常重启后原端口/凭据/启停意图保留。首轮传输失败及最终一次 TLS 错误重试保留，未放宽出口或证书校验。见 [本轮脱敏证据](evidence/vps-direct-socks-20261018.json)。CF 暂停、Worker 端到端未测，不自动上传凭据；证书自动申请暂缓，3x-ui 不修改，unknown 不称家宽。

2026-10-18 统一检测与自定数量已实现并部署：最终干净包 `19e2840` 统一“检测”入口、结果与阶段；运行出口复用连接，其他候选临时验证。并发为可保存的数字输入，支持 64/128，目标出口移除固定 32 限制，官方候选列表移除 200 项截断；手动检测可加入批量任务并优先、重复提交去重。旧网段兼容保留，诊断使用独立身份与空闲地址。227 个 Linux 顶层用例 / 283 个通过事件、Go vet、GitHub 含调度及停服路径 race 通过；真实四路、取消/强杀恢复、正常停服重启和 22 项保留检查通过。部署中发现的安装服务旧上限及停服保存竞态已修复，错误停止标志按原意图恢复，失败现场保留。真实订阅复验范围见 [本轮脱敏证据](evidence/vps-unified-checks-20261018.json)。测试机配置不作为所有用户的上限；实际地址、端口和资源约束明确报告，不静默调低。CF 暂停，3x-ui 不修改，unknown 不称家宽。

2026-10-18 更新：干净开发包 `ee0e2d1` 已部署原授权 VPS，候选默认两路检测并可选择逐个，显示阶段进度，增加保守 TCP 预检；正式出口恢复优先，等待临时诊断完整清理后接管同一节点。包含“接入管理 / 新增接入入口”命名调整。真实双诊断、取消、管理进程强杀恢复及原三条 VPS/Mac 订阅复验通过，保留当前 32 个目标和现有凭据；线上已有的 93 项检测任务保留。见 [本轮脱敏证据](evidence/vps-parallel-checks-20261018.json) 与 [控制台说明](CONSOLE.md#验证记录)。本机直出及其他隧道协议尚未实现。CF 继续暂停，3x-ui 不修改，unknown 不称家宽。

2026-10-17 追加：登录与连接配置页已重做，旧默认 Slot 名称兼容已修复；测试 VPS 已安装干净开发包 `95cd217`，保留升级前当前的 32 个目标出口及现有配置。新验证记录见 [控制台说明](CONSOLE.md#验证记录)。CF 继续暂停，3x-ui 不修改。

开发分支 `codex/console-capacity-20261017`：重做独立管理控制台，新增 0–32 个目标出口、自动建立并绑定内置入站、订阅按实际国家命名和范围选择，以及真实来源刷新和隔离的连通性检测。干净开发包 `42b3537` 已完成 VPS/Mac 四出口订阅、恢复为三个原出口后的强杀恢复和凭据保留复验，见 [脱敏证据](evidence/vps-console-20261017.json)。正式发布仍为 v0.4.0；本分支的测试和操作说明见 [控制台说明](CONSOLE.md)。

“全量来源”指完整解析官方当次公开清单，不等于获得官方网站全球所有服务器。候选数量、检测结果和运行出口数量分开报告。指定国家无合适候选时等待，不强求凑齐；unknown 不称为家宽。CF 继续暂停，3x-ui 不修改。

# 当前开发方向（2026-10-16）

2026-10-16 发布收尾：PR #1 已合入 main，正式 [v0.4.0](https://github.com/hanhaoops/xnvgatebox/releases/tag/v0.4.0) 已发布。原始候选来自干净提交 `371174d`，GitHub CI、测试 VPS 原安装入口升级、15 项保留检查及 VPS/Mac 三节点订阅冒烟均通过；公开下载摘要与该候选一致。详见 [发布证据](evidence/vps-release-20261016.json)。下文保留开发及验收过程。

2026-10-15 Mac 接续：已拉取指定 `389288d`，保留本机私有运行文件并实测备份、开发包升级、三节点真实订阅、Slot 1 强杀恢复和完整备份还原。发现并修复 Echo 换行解析、候选重试饥饿及未绑定入站验收崩溃；当前为开发测试版。RU 多轮出现超时，末轮保持 RU 策略自动换点后，三个节点的本机和 Mac 外部复验均通过；详细范围和保留的失败记录见 [Mac 实测](VPS_UPGRADE_VALIDATION.md#2026-10-15-mac-实测)。CF 仍暂停，3x-ui 未修改，unknown 仍不称家宽。


后续收到用户明确授权第三 Slot 换国家，已将第三 Slot 从 RU 改为 KR（先试的两个 US 候选失败并保留记录）；当前策略为 JP/KR/KR，三个不同实际出口的 VPS 本机与 Mac 外部真实订阅均通过，第三 Slot 强杀期间两端阻断及自动恢复后整轮复验也全部通过。第三 Slot 仍为 `17930`，公开入站仍为 `30003`，原订阅及客户端凭据保留。追加验收见 [授权换国家记录](VPS_UPGRADE_VALIDATION.md#2026-10-15-用户授权第三-slot-换国家)。

验收口径：指定国家有没有合适 IP，取决于上游节点供给及其可达性，不作为本程序必须保证的条件。候选不足时，正确提示、保留策略与端口、等待重试并阻断回落即可通过对应功能检查；该国正向连通项记为“上游条件不足，暂未验证”，不要求反复试到成功。可使用当时可用的国家验证多出口，三个出口无需来自三个不同国家。程序本身的选点、绑定、凭据保留或防回落错误仍按缺陷处理。

此前 `v0.3.1` VPS 实测：三个固定 SOCKS Slot 及 `30001/30002/30003` 三个 VLESS/REALITY 入站通过同一个真实订阅完成外部验活；重启自动恢复绑定、强杀后阻断及端到端自动恢复通过。出口类型仍 unknown，ISP/家宽认定不在已完成项内。历史结果见 [VPS_STANDALONE_VALIDATION.md](VPS_STANDALONE_VALIDATION.md)。

用户决定暂停 Cloudflare 模式，备份并撤下线上 Worker，全力开发 VPS Standalone。以下旧 Mode A / Hybrid 章节作为历史设计保留，不再是当前交付目标；不得新增 Cloudflare 部署或恢复其定时发布。

当前交付链路：官方全量来源 → 统一 Python Pool Builder → VPS 本地候选池 → 独立 OpenVPN namespace → 固定 SOCKS Slot → 内置 Xray → 一个聚合订阅。管理与订阅共用 VPS 上的一个进程，不要求 3x-ui、Cloudflare 或 GitHub Actions 凭据。

本轮已完成的验收范围（证据见顶部链接）：

1. 备份源码、Git 历史、线上 CF 配置/代码/manifest，删除 `xnvgatebox` Worker，停用旧 CF 工作流。
2. 一个完整 Linux amd64 安装包、一条安装命令；本机每十分钟刷新共享候选池，刷新失败保留旧池。
3. 上限三个可配置出口，固定 `slot-01..03` 和 `17928..17930`。不固定地区，不把未知节点标记为家宽。
4. 内置 Xray 无可用绑定时默认 blackhole；换节点不改变 Slot 标签和 SOCKS 端口。管理页导出标准 Xray JSON，第一版本不写 3x-ui 数据库。
5. 真实订阅经内置 Xray 出网，出口与对应 Slot 一致；掉线请求失败，不回落 VPS IP。

当前版本已完成代码复核、开发分支提交、PR 合并及正式发布。OpenVPN 出口国家证据、Slot 策略、配置回滚及真实迁移/故障链路已完成本轮实测，详细范围见顶部记录；住宅类型、ISP/ASN 仍未验收。下文云端进度保留为开发历史，不作为当前待办。

2026-10-15 云端开发进度（未发布、未升级 VPS）：已接入实际 OpenVPN 基线 IP → 统一 Python intelligence adapter/classifier → 固定 SOCKS actual 独立核验，新增私有 `slot_health.json` 画像证据及 Slot 展示。候选展示不再沿用 SSTP 的住宅/ISP/ASN 画像；未配置服务、查询失败或过期保持 unknown。实现与配置见 [vps/README.md](../vps/README.md#openvpn-出口画像)。画像模拟回归和普通单元测试已验证，真实 API、namespace/SOCKS 与公开订阅链路仍需有权限环境实测，不能视为已完成住宅类型验收。该轮仅完成画像；后续策略与回滚进度见下文。

后续云端开发进度（同日，仍未发布）：已实现每 Slot 的国家、允许类型、preferred/allowed/blocked ISP、allowed/blocked ASN 和本机 SOCKS HTTPS 延迟上限，保存到私有恢复状态。自动选点、手动指定、换节点和重启恢复均使用同一硬策略；缺证据或无合格节点保留等待/阻断，不放宽条件。停止保留编号、端口和策略，启动重新验活；删除才释放编号，并保留导出的明确阻断。增加操作串行/取消、旧连接清理、候选冷却及策略管理界面。模拟回归和策略路径 race 检查不替代真实 API/隧道验收；该轮仅完成策略与生命周期，回滚进度见下文。配置语义见 [Slot 策略与生命周期](../vps/README.md#slot-策略与生命周期)。


配置回滚开发进度（同日，未发布、未升级 VPS）：内置 Xray 变更先生成私有候选并静态校验，再通过有界 undo 记录提交配置；校验失败保留旧进程，启动或持久化失败尝试恢复上次提交配置，无法恢复则停止服务并保留恢复记录。启动先恢复未完成的受管文件事务，再加载配置。Slot 状态持久化与管理设置（密码、路径、端口、订阅 token）也接入私有事务；新管理监听先预留，提交后才切换。提供“回滚内置配置”和“恢复上次策略”：前者可恢复旧客户端凭据，后者必须重新按策略验活，不恢复历史 healthy。详情见 [配置事务与回滚](../vps/README.md#配置事务与回滚)。

本轮验证：Python 普通 44 项、Go 普通 165 个顶层用例（含子用例 191 个通过事件）、配置与策略路径 race、固定版 Xray 26.3.27 静态配置解析、开发构建和管理脚本语法检查通过。完整 Python 两项真实进程测试及完整 Go 的 Setns/监听用例仍受平台权限限制；没有绕过隔离。固定 Xray 解析检查只验证生成配置，不启动监听或真实隧道。真实画像 API、OpenVPN/SOCKS/Xray 全链路、迁移与掉线恢复仍待有权限环境验收。内置 Xray 仍整体重启，不能宣称连接无中断；没有实现跨全部文件/外部系统的一次性事务、任意版本历史或安装包回滚，不涉及 CF 和 3x-ui 写入。

迁移准备进度（同日，仍未发布）：新增 `-check-state -dir 私有状态副本目录` 只读检查，不要求 root、不启动服务、不生成凭据或恢复写入；报告不含密码、配置和密钥。恢复流程整份校验、整批准备后再发布，拒绝半套凭据、重复删除记录及损坏配置，避免部分恢复或静默轮换凭据。三 Slot 合成旧版样例验证身份、原凭据与绑定保留，以及重验前全部阻断。新增 9 个迁移顶层回归，普通 Go 合计 174 项（含子用例 224 个通过事件）；迁移/配置/策略路径 race 和非 root 的实际 CLI 检查通过。后续真实环境验收清单及检查范围见 [VPS_UPGRADE_VALIDATION.md](VPS_UPGRADE_VALIDATION.md)，本地检查不改变 v0.3.1 实测记录。

后续验收准备（同日）：用户已授权原 VPS 作为测试服务器，可备份、升级并执行真实故障验收；3x-ui 和暂停的 CF 仍保持。已增加 `scripts/vps_acceptance.py` 的私有计划采集、真实订阅正向核验、指定 Slot OpenVPN 强杀与恢复核验、迁移前后保留项比较。工具如实区分 VPS 本机公开入站测试与外部客户端测试，不把配置/证书错误算作有效故障阻断。打包脚本支持校验本地固定 Xray 归档，并写入版本、基线提交、dirty 标记与源码快照摘要。新增合成回归不能视为已完成实际部署或远端验收；实际结果必须另行记录。

本机接续：云端开发成果提交至 `codex/standalone-migration-20261015` 开发分支；Mac 启动、已验证范围与交接说明见 [MAC_HANDOFF.md](MAC_HANDOFF.md)。云端阶段未建立 SSH；后续 Mac 已实际完成备份、升级、故障注入与完整还原，见顶部实测。

---

# xnvgatebox 出口节点与订阅系统分阶段开发文档

修订日期：2026 年 10 月 10 日。范围：v0.1～v1.0；实现进度见 [ROUND1.md](ROUND1.md)。

开发基准：用户确认的 `DEVELOPMENT_revised.md`。后续实现按本文的运行模式、统一部署入口、阶段顺序与验收条件执行。

## 0 产品命名与一次部署原则

- 对外产品名、管理界面标题、订阅页面和安装文档统一使用 **xnvgatebox**。仓库为 [`hanhaoops/xnvgatebox`](https://github.com/hanhaoops/xnvgatebox)。
- “出口节点”“出口槽位”“订阅”“管理面”是用户界面和操作文档的默认术语。来源站点、协议名、上游仓库名和代码兼容字段中的 `VPN`/`SSTP`/`OpenVPN` 可以在必要的技术上下文中保留，但不作为产品品牌或用户必须理解的概念。
- 用户只执行一次统一安装/部署流程，选择 `serverless`、`vps` 或 `hybrid` 运行模式；不得要求用户分别创建 Checker、Edge、Control 等多个项目，也不得要求重复上传多个 Worker。
- Cloudflare 最终形态严格限制为 **一个 Worker + 一个 KV Namespace**。管理、订阅、节点验证和数据面入口由同一个 Worker 按路径或请求类型处理；不得通过安装器自动创建隐藏的第二个 Worker 来满足“部署一次”。
- **v0.x 不得新增独立 Pages、D1、Durable Object、第二 KV、第二 Worker 作为必需依赖；如确有必要，必须先修改开发文档。** 不得新增第二个控制面或管理服务。内部模块拆分不能突破这个资源边界。
- Worker 内部可以按模块组织检测、数据面、管理和订阅代码，但这是单脚本内部结构。涉及第三方代码时必须遵守各自许可证；不以“拼接多个上游 Worker”的方式规避许可证边界，必要功能应由本项目自有、可兼容许可的代码实现。
- 当前已存在的 `vpngate-checker`、`vpngate-edge` 和 `vpngate-control` 是第一轮验证用的过渡部署。下一轮必须把它们合并为一个 xnvgatebox Worker（单一部署命令、单一配置文件、单一状态页）。
- VPS 模式同样只提供一个安装入口；Multi-Exit、namespace、固定 SOCKS 端口和 Xray 导出由同一安装流程配置。3x-ui 是可选消费端，不是额外的部署项目。
- VPS 安装必须提供一条可审计、可重复执行的命令，例如 `xnvgatebox install --mode vps`；命令完成依赖检查、程序安装、配置初始化、namespace/SOCKS 服务注册和健康检查，不要求用户分步执行多条部署命令。
- **`xnvgatebox install --mode vps` 在完全没有 GitHub Actions、Cloudflare Worker、远程 `node_pool.json` 的情况下也必须可用。** 首次安装可以下载公开发布包和系统依赖，但不得要求用户部署外部控制面、配置 GitHub/Cloudflare 凭据或预先提供远程节点池；安装后由 VPS 本机生成池、验证出口，并提供管理入口和订阅链接。

项目先完成三个可运行、可演示的闭环：**无 VPS 的 VLESS 节点、VPS 上多个固定出口、可直接粘贴的 3x-ui/Xray 配置**。核心产品是 Exit Slot：后台 VPN Gate 节点可以更换，`slot_id + SOCKS port + outbound tag` 保持不变。

项目提供三种**运行模式**：**Serverless / No VPS、VPS Standalone、Hybrid**。它们是同一部署包的配置档，不是三套需要分别安装的项目。所谓“统一节点池”首先是统一的数据模型、抓取器、分类器和验证语义，**不等于必须依赖一个中心化 GitHub 远程池**。Mode A 默认由 GitHub Actions 构建池并供 Cloudflare 使用；Mode B 既可以消费远程池，也可以在 VPS 本机运行同一套 Pool Builder 生成本地池，从而做到只用一台 VPS 也能独立工作。Hybrid 则使用远程池做初筛、VPS 本机完成最终 OpenVPN/Slot 验活。

统一池保存节点事实、协议能力、出口画像和验证证据；不同运行环境自行完成最终链路验证。Mode B 不要求先在 GitHub Actions 完成一次 B Full Chain，再到目标 VPS 重测。Cloudflare 的核心管理和订阅能力由本项目自己的单 Worker 提供；EdgeTunnel 只保留为可选的外部兼容接口，不成为核心运行依赖。参考项目的源码与许可证结论保留在 [REFERENCE_AUDIT.md](REFERENCE_AUDIT.md)。以下为拟实现方案，不代表链路已实测成功。

## 1 开发顺序与三个里程碑

| 版本 | 实现范围 | 完成条件 |
| --- | --- | --- |
| v0.1 / 里程碑一 | VPN Gate → SSTP → Cloudflare → VLESS | 至少一条最终订阅链路实测通过，actual_exit_ip 等于 expected_exit_ip，无需 VPS |
| v0.2 | Linux VPS → OpenVPN → netns → 固定 SOCKS5 | 一个 Slot 实测通过；强杀 VPN 后不从母机出网 |
| v0.3 / 里程碑二 | 最多三个可配置地区的 Slot | `slot-01`、`slot-02`、`slot-03` 分别使用 :17928、:17929、:17930；上游候选充足时验证三个独立出口（国家可重复），不足时验证等待与阻断，国家与分类如实展示 |
| v0.4 / 里程碑三 | Xray / 3x-ui 一键配置生成 | 输入真实 inbound tag，直接复制 outbound、routing 或配置片段；每入站实测绑定一个 Slot |
| v0.5 | 自动换 IP 与故障迁移 | 同策略选替代节点，端口/tag 不变，新出口重新核验 |
| v0.6 | WebUI 与节点画像 | Validated Nodes、Exit Slots 两区，生命周期操作、ISP/ASN/分类展示及复制按钮 |
| v1.0 | 完整安全加固与可选自动接管 | 强化 kill switch、权限隔离、签名分发、恢复能力；按需要增加 3x-ui API |

每个版本按实际覆盖的功能和真实链路记录验收结果，不预设开发天数。Slot 的地区由用户配置，不设置固定国家模板；某地区没有符合策略的节点时，可以保留等待，也可由用户改用其他地区继续验收，不必为某个国家持续补测或阻塞其他已完成项。节点供给不足不算程序缺陷，尚未测到的连通能力也不标为通过；不以错误国家或机房节点补齐“家宽演示”。

### 1.1 下一阶段范围锁定

下一阶段只解决三件事，完成前不扩功能：

1. **Cloudflare 三服务合一**：一个 Worker 内提供检测、管理、订阅与数据面，绑定唯一 KV。用户只发布一次、设置必要参数；生成的订阅使用该 Worker 的入口，并重新完成正向链路和失效后端对照验证。
2. **VPN Gate 全量发现 + 分层筛选**：统一 Pool Builder 获取并解析全量来源清单；粗筛、真握手、最终验活分别限额，不能在抓取阶段只截取 50 个节点。报告各阶段的输入、测试和通过数量，预算外节点保持未测。
3. **VPS 三 Slot**：一条命令独立安装，在没有 Actions、Cloudflare 或远程池时使用本地 Pool Builder；上游候选充足时验证三个固定编号 Slot 同时验活，不足时验证等待与阻断。端口与 tag 稳定，掉线不能回落母机出口。

前三项验收前，不新增数据库、Worker 或管理服务，不做 3x-ui API 自动写入，也不开发新的外部兼容适配器。已有管理与订阅能力只做这三项所需的接入和修正；定时刷新、完整画像和后续里程碑另行推进。

## 2 统一节点池与验证职责

```text
VPN Gate → 唯一抓取器 → 配置解析 / 去重 / 基础检查
                           ↓
                    node_pool.json
          节点事实 / SSTP与OpenVPN能力 / 按协议的出口画像
          检测来源、时间、状态与失败原因
                   ↙                    ↘
Mode A：SSTP检测 → Cloudflare/VLESS最终验活 → 发布订阅
Mode B：VPS选择候选 → 本机OpenVPN/SOCKS最终验活 → Slot healthy
                                         ↓
                                  v0.4：Xray绑定验活
```

- 全项目只有一套抓取、规范化、画像、分类与出口比较逻辑。`Pool Builder` 是可复用模块：可在 GitHub Actions 运行，也可在 VPS 本机运行；**禁止为了 VPS Standalone 再写第二套 VPN Gate 抓取器或第二套分类器**。
- Serverless 默认由 Actions 发布共享池；VPS Standalone 默认在本机生成同结构 `node_pool.json`；Hybrid 可下载远程池做初筛。三者 schema 与核心逻辑一致。VPS 的运行结果写 `slot_health.json`，它是运行证据，不是另一套节点事实模型。
- 池里的协议状态为 `not_tested / passed / failed / stale`。配置存在只表示发现该入口，不能把 OpenVPN available 自动置为 true；未在 runner 检测 OpenVPN 的候选仍可由 VPS 选择并实测。
- `node_pool.json` 是候选与证据池，不因为文件名暗示“所有模式都已验证”。Mode A 发布列表与 Mode B healthy 列表各自检查本环境证据，不靠一个全局 `full_chain_verified` 布尔值准入；需要展示“已验证节点”时由当前 profile 派生视图生成。
- 文件职责固定：`node_pool.json` 是全量发现后的统一候选与证据池；`mode_a_validated.json` 只保存完成 Mode A 全链路验活的发布清单；`cf_manifest.json` 只从该发布清单派生，裁剪为不含凭据的 Cloudflare manifest。KV 的 `manifest:current` 只接收这个派生产物，禁止直接发布候选池或只通过 SSTP 握手的列表，也不再新增含义模糊的 `validated_nodes.json`。
- SSTP 与 OpenVPN 可能有不同公网出口。国家、ASN、ISP、IP 类型、延迟和 expected IP 按协议保存；SSTP 画像不能直接充当 OpenVPN 画像。
- Actions 默认只做抓取、SSTP检测及 Mode A 验活，**不安装 OpenVPN、不要求 B Full Chain**。可选基础 OpenVPN 检测以后按需要加入，不作为 Mode B 前置条件。

远程池模式在 v0.x 使用用户自己配置的 HTTPS 地址和 SHA256 核对下载内容，OpenVPN 配置按摘要绑定；VPS Standalone 不要求配置远程池地址。SHA256 用于内容完整性，不代替发布者签名；公开多人消费池的签名机制放到 v1.0。先采用 JSON 临时文件写完后替换，不做不可变版本目录、签名 manifest 或复杂 latest 协议。


### 2.1 一个部署包、三种运行模式

统一部署入口读取一个配置文件，再选择运行模式。Cloudflare 模式只发布一个 Worker；VPS 模式只执行一条安装命令。用户不需要分别部署表格中的组件。

| Profile | 部署入口 | 节点池来源 | 最终验活位置 | 适用目标 |
| --- | --- | --- | --- | --- |
| Serverless / No VPS | 一次发布一个 xnvgatebox Worker + KV | Actions 运行 Pool Builder | Actions + Cloudflare/VLESS | 完全不需要 VPS 的订阅方案 |
| VPS Standalone | 一条命令安装 xnvgatebox VPS 单元 | **同一 Pool Builder 在 VPS 本机运行** | VPS 本机 OpenVPN/SOCKS/Xray | 一台 VPS 多地区、多 Slot 出口 |
| Hybrid | 一条命令安装 VPS，并在同一 Worker 配置远程池 | 远程池初筛 + 本机补测 | VPS 本机 | 减少 VPS 抓取/分类成本，同时保留本机真实性 |

三个 Profile 共用 `models / source parser / classifier / selection policy / exit comparison`。统一的含义是“逻辑、配置和数据契约统一”，不是强制所有运行模式连接同一个中心服务。任何 Profile 都不得因为远程池不可用而偷偷切换为另一套未经验证的来源；VPS Standalone 应显式配置为本地 Pool Builder。模式切换只改配置，不重新部署一套程序。

VPS Standalone 的独立性需实际验收：不提供 GitHub/Cloudflare 凭据和远程池地址，执行统一安装命令，由本机获取来源清单、建立候选池、连接并验证 Slot；本机管理和订阅无需外部控制面。独立运行不代表离线运行，来源清单、依赖下载和出口验证仍需要正常互联网访问。

## 2.2 统一 Cloudflare 管理层、数据面与订阅接口

Mode A 的正式链路由一个 xnvgatebox Worker 承载：GitHub Actions 生成并验证节点池；Cloudflare KV 存储当前版本的短期 manifest；同一个 Worker 内提供管理页、状态页、订阅接口、节点验证和数据面入口。检测、数据面、管理和订阅可以在脚本内部保持模块隔离，但用户只上传/发布一次。GitHub Actions 不承载用户代理流量，Worker 也不能把未验证的 GitHub IP/端口直接当成出口。

```text
GitHub Actions：同一 Pool Builder 全量发现
  → node_pool.json（候选池，不直接发布订阅）
  → SSTP 检测 + 实际 VLESS 全链路验活
  → mode_a_validated.json（Mode A 已验活发布清单）
  → cf_manifest.json（派生的无凭据 manifest，带版本和有效期）
  → 唯一 Cloudflare KV 的 manifest:current
  → xnvgatebox Worker（唯一 Cloudflare Worker）
       /admin   管理与状态
       /sub     带 token 的 VLESS 订阅
       /        只接受受控的节点参数
```

Worker 的 VLESS 链接必须由当前 manifest 派生，不能接受任意 `proxyip`、任意 SOCKS5 地址或任意目标主机。只允许 manifest 中仍然有效的 VPN Gate 节点，并在发布前后保留 `expected_exit_ip == actual_exit_ip` 证据。管理接口使用单独的管理员口令或 Cloudflare Access；订阅 token 只能读取当前有效版本，manifest 过期或为空时返回非 200，避免客户端被空订阅清空。

EdgeTunnel 适配器只输出兼容的节点/订阅格式或调用导入接口，不把它的 Worker 源码复制到本项目，也不增加用户的部署步骤。若未来启用该适配器，许可证、来源和运行边界仍单独记录；它不是 xnvgatebox 的主链路依赖。最终发布仍只有一个 xnvgatebox Worker。

Cloudflare 里程碑重新定义为：

| 阶段 | 工作 | 验收 |
| --- | --- | --- |
| CF-1 | manifest schema、版本/过期和原子发布 | **已实现**：`data/cf_manifest.json` 从 `mode_a_validated.json` 派生，脚本单次 PUT 到唯一 KV 的 `manifest:current` |
| CF-2 | 自有 Worker 管理页、`/api/status`、受保护 `/sub` | **已实现源码**：`worker/control` 提供 `/admin`、`/api/status`、`/sub`；订阅不含凭证 |
| CF-3 | Worker VLESS/SSTP 入口接入 manifest | 每条发布链接重新验证 expected/actual，失效节点不再发布 |
| CF-4 | GitHub Actions 定时刷新与回滚 | 30/60 分钟刷新，发布失败保留上一份未过期版本并报警 |
| CF-5 | 可选兼容导入适配 | 外部适配器只作为格式消费者，不成为主链路依赖，也不增加部署步骤 |

## 3 最小 Node 与 Exit Slot 模型

Node 记录实际节点，ID 不根据 Exit IP 或排名生成。hostname 的入口 IP、端口、配置变动时更新记录、清除相关旧证据。下面为结构示例，文档地址不是实际可用节点：

```json
{
  "node_id": "vpngate-us-001", "hostname": "vpn-example.opengw.net",
  "server_ip": "192.0.2.10", "advertised_country": "US",
  "protocols": {
    "sstp": {
      "port": 443, "status": "passed", "expected_exit_ip": "198.51.100.20",
      "egress": {"country": "US", "asn": "AS7922", "isp": "Comcast", "ip_type": "likely_residential"},
      "observed_by": "checker-worker", "verified_at": "2026-10-07T13:00:00Z"
    },
    "openvpn": {
      "profile_ref": "configs/example.ovpn", "profile_sha256": "example-digest",
      "status": "not_tested", "expected_exit_ip": null, "egress": null
    }
  }
}
```

Exit Slot 保存稳定入口、选择策略及当前本机结果：

```json
{
  "slot_id": "slot-01", "target_country": "US",
  "allowed_ip_types": ["strict_residential", "likely_residential"],
  "preferred_isp": ["Comcast", "Spectrum", "AT&T"], "blocked_isp": [],
  "allowed_isp": [], "maximum_latency_ms": 1500,
  "current_node_id": "vpngate-us-001", "namespace": "xng-slot-01", "tun_device": "tun120",
  "socks_host": "127.0.0.1", "socks_port": 17928, "xray_outbound_tag": "exit-slot-01",
  "expected_exit_ip": "198.51.100.20", "actual_exit_ip": "198.51.100.20",
  "status": "healthy", "last_switch_at": "2026-10-07T13:03:00Z"
}
```

Slot ID、端口、tag 创建后写入本地 JSON，换节点、重启或修改国家均不重新计算。新建 Slot 使用 `slot-01` 等固定编号，默认 outbound tag 使用 `exit-slot-01`，namespace 使用 `xng-slot-01`；代码和默认配置不得按国家生成 Slot 身份、namespace 或 tag。`target_country` 只是可修改的策略字段；从 US 改为 DE，`slot-01 / 17928 / exit-slot-01` 仍然不变，显示名称可以修改。已有 Slot 的 ID 和 tag 不自动重命名，以免破坏既有绑定；编号规则适用于新建配置。

端口冲突报错，不随机换端口；停止保留 Slot 与端口预留，删除不重排其他 Slot。v0.x 用文件锁、单 Slot 串行操作和简单状态 `stopped / connecting / healthy / blocked / waiting_for_nodes`，暂不引入 SQLite、完整状态机框架或独立 agent。

## 4 家宽分类分级

分类以实际公网出口为对象，保留原始 API 数据、provider、查询时间、理由和置信度。用户界面明确区分“数据库住宅”与“疑似家宽”，不会为了数量把 unknown 标成住宅。

| ip_type | 用户展示 | 判断条件 |
| --- | --- | --- |
| strict_residential | Strict Residential / 数据库明确识别住宅 | API 针对该 IP 明确住宅，必要字段完整，且没有 hosting/mobile/business 冲突 |
| likely_residential | Likely Residential / Consumer ISP / 疑似家宽 | hosting 明确为 false，网络明确为 ISP，ASN/ISP 命中消费者宽带辅助清单，没有机房、移动或企业网络证据冲突 |
| mobile | Mobile | 明确移动网络证据 |
| datacenter | Datacenter | 明确 hosting/datacenter 证据 |
| business_isp | Business / ISP | 明确企业网络，或只有 ISP 属性而不足以判断家宽 |
| unknown | Unknown | 关键判断字段缺失、查询失败或冲突无法解决 |

例如 Comcast / AS7922 + hosting=false + network=ISP + 无冲突，可以标 likely_residential；仅 ISP 名称、仅 hosting=false 或仅主机名均不够。住宅正面字段没有提供时允许走 likely 判断；hosting 等 likely 所需字段缺失则不能补 false。proxy/vpn/abuse 缺失保留 null，分别展示。

Slot 默认可显式允许 strict 与 likely，两者分别计数和展示；只接受数据库住宅时配置 `allowed_ip_types=["strict_residential"]`。旧 `residential` 兼容值只映射到 strict。原始画像与规则版本保存，方便以后调整，不增加新的自建 IP 情报服务。

## 5 v0.1 无 VPS 最小闭环

沿用三个参考项目的职责分工：gate 的抓取组织思路、CheckSocks5 的实际 SSTP检测、CF-vpngate 的 Xray 最终测试思路。第一轮曾通过分离的 Checker/Edge 适配层完成验证；下一阶段必须收敛为单个 xnvgatebox Worker 内的检测与数据面模块。EdgeTunnel 仅保留可选接口，不作为必须部署的 Gateway，也不先复制其 Worker 源码。

1. 唯一抓取器解析 VPN Gate 全量 CSV、OpenVPN配置，写入候选池后按第 10 节分层预算选择 SSTP 候选；TCP入口只是候选，需要真握手。
2. Checker 完成 SSTP → PPP → IPCP → 隧道内 TCP/HTTP，记录预期公网出口；PPP内部地址、server_ip 不能当出口。
3. 按该 IP 获取 ASN/ISP/国家与分类，加入共享池的协议证据。
4. 生成实际要发布的 VLESS/WS 配置，强制经过同一个 xnvgatebox Worker 的 SSTP 数据面；可选 EdgeTunnel 只消费兼容格式，不把 `global=1` 当通用标准。
5. Actions 启动临时 Xray SOCKS 入站，通过 `socks5h` 请求两个独立 HTTPS IP Echo，校验证书与合法 IP；两者都必须等于 expected_exit_ip。再校验一个小体积 HTTPS 内容请求。
6. 检测用的就是发布用的配置，不只在测试时添加全局代理参数；出口不同、握手失败或响应异常就不发布。
7. 输出基础 `node_pool.json`、Mode A 通过最终链路的 `mode_a_validated.json`、`residential.json`（保留 strict/likely 等级）、`nodes.txt`、受保护的 `subscription.txt` 和简短检测报告；无需 WebUI 即可演示。`node_pool.json` 不能与“已通过最终 VLESS 验活”的列表混为一谈。

v0.1 必测：使用无效 SSTP入口访问 Worker 可直接访问的网站，应失败；最终订阅重复验证成功；清理 Xray 临时进程；没有把 Cloudflare直出当成VPN成功。MVP先支持 TCP/IPv4/WS，拒绝未验证的 UDP/IPv6/XHTTP，不另走原生DNS/UDP出口。

**技术债 T1：Checker 用户态 TLS 身份认证不完整。** v0.1 可使用现有检测结果作为标明来源的初步 expected，但最终 curl 必须保持正常 HTTPS 证书验证，并经两个独立端点交叉核验。双端点和 IP相等不能修复原 TLS实现的身份认证缺陷；报告区分“链路实测通过”和“基线来源待加固”。修复 TLS身份校验放入 v1.0，自建签名 Echo 仅作以后可选方案，不阻挡 MVP，也不把该基线称为已完成独立安全认证。原始画像不盲信 Checker 默认补 false 的字段。

## 6 v0.2～v0.3 VPS 固定多出口

Multi-Exit 使用 host SOCKS → 对应 netns → OpenVPN → VPN Gate 路线。实现借鉴过 MIT 许可的外部参考项目，但产品代码以 VPN Gate 的统一节点池、Slot 模型和本项目的安全验收为准，不建立第二套抓取器。第一阶段只改节点来源接口，使其既能读取远程池，也能读取 VPS Standalone 本机 Pool Builder 的结果；监听改为 loopback，端口与 tag 由 Slot 固定配置。保留并验证 namespace 线程恢复、健康检查和母机保护；只有现有实现无法满足“断线绝不回落母机”时再做最小补丁，不提前引入 Unix IPC、namespace agent、cgroup 或复杂 root-helper。

```text
slot-01 → 127.0.0.1:17928 → host SOCKS → xng-slot-01 / tun120 → 出口节点
slot-02 → 127.0.0.1:17929 → host SOCKS → xng-slot-02 / tun121 → 出口节点
slot-03 → 127.0.0.1:17930 → host SOCKS → xng-slot-03 / tun122 → 出口节点
```

地区配置示例（不是固定国家模板）：

```ini
[slot-01]
target_country = US
[slot-02]
target_country = JP
[slot-03]
target_country = KR
```

### 本机验证与 expected 的来源

1. VPS 从 `node_pool.json` 选候选；该池可以来自远程共享池，也可以由 VPS Standalone 在本机使用同一 Pool Builder 生成。随后检查配置摘要、清理不允许的脚本/plugin/include，建立独立 netns，再运行 OpenVPN；母机默认路由不变。
2. 在该 namespace 内，绑定 tun 对两个 HTTPS IP Echo 发出基线请求。合法公网 IP 一致且不同于 VPS 公网 IP 后，得到该协议本机 expected；随后调用**同一套 classifier / intelligence adapter** 对这个 OpenVPN 出口做国家、ASN、ISP、IP 类型画像，再判断是否符合 Slot 策略。池里没有 OpenVPN 出口证据也可用此过程补测；如果 VPS 未配置画像 API，则类型必须保持 unknown，不能沿用 SSTP 的住宅标签。
3. 再通过固定 SOCKS端口发独立 HTTPS请求取得 actual，要求两个端点都与 expected 完全相等，才标 healthy。不能把同一次 SOCKS查询结果同时赋给 expected 和 actual。
4. 若池有新鲜 OpenVPN基线，先比较；变化时标明旧证据失效，重新完成上述隧道基线、画像和端口验证，保存本机证据。不能在健康检查中遇到不匹配就静默改 expected。
5. v0.4 加入实际 Xray outbound/入站验活。v0.2 的 SOCKS成功不提前宣称已完成 3x-ui链路测试；不要求 runner 预验 B。

### 从 v0.2 开始保留的最小阻断

“VPN断线不能回落到VPS公网IP”必须从第一个 SOCKS Slot 起成立，不能把所有 kill switch 工作推迟到 v1.0：

- namespace业务默认路由仅经 tun，外层VPN server使用经veth的明确路由；veth/NAT仅允许该VPN endpoint，不提供任意Internet默认回退。
- 所有 SOCKS出站socket在对应netns创建并绑定指定tun；绑定失败立即报错，禁止重试为未绑定socket。域名解析的socket同样绑定tun，不回退母机resolver。IPv6与UDP ASSOCIATE暂不支持。
- 最小防火墙先装后连：限制veth出口、阻断IPv6；不使用会绕过业务禁止规则的通用 established 放行。无需先实现UID/cgroup权限体系，tun绑定与规则一起防止普通代理利用VPN endpoint例外直出。
- 检测未完成、VPN退出、tun消失或出口不匹配时拒绝新业务并关闭旧连接；探测走本地受控测试路径。不要等下一轮健康检查才阻止已知断线。
- 真机强杀OpenVPN、删tun、撤路由并保留veth/NAT，检查固定SOCKS请求失败和母机抓包无业务直出。仅健康IP比较通过不能替代此验收。

v0.3 同时运行 `slot-01`、`slot-02`、`slot-03`，分别核验国家、IP、画像和端口，检查出口互不串流且 IP 不同。地区由用户配置；如果某地区暂时没有符合策略的节点，可由用户调整目标国家完成三个独立 Slot 的功能验收，但必须如实展示实际国家/类型。没有符合策略的节点就显示 waiting_for_nodes；不得自动放宽住宅、国家或 ISP 规则。验收还须确认修改一个 Slot 的国家或替换其后台节点后，该 Slot 的 ID、端口、namespace 和 tag 不变，其他两个 Slot 不受影响。完整权限分离、广泛故障恢复和强化防火墙仍留到 v1.0。

## 7 v0.4 一键生成可粘贴的 Xray 配置

先提供 CLI 导出，v0.6 增加按钮。用户只需选择 Slot 并填写 3x-ui 真实 inbound tag；工具生成格式完整的 outbound 对象、routing 规则及批量片段，不要求用户手写 JSON 字段。**这里“3x-ui 可直接使用”的 v0.4 定义，就是生成与当前 Slot/真实 inbound tag 完全匹配、可以直接复制粘贴的配置，并提供明确粘贴位置；不是要求用户自行理解后重写 JSON。** 自动调用 3x-ui API 修改面板属于后续能力。

例如 `slot-01` 的标准批量片段；复制单项时分别输出对应 outbound 对象与 routing 规则：

```json
{
  "outbounds": [
    {"tag": "exit-slot-01", "protocol": "socks", "settings": {"servers": [{"address": "127.0.0.1", "port": 17928}]}},
    {"tag": "xnvgatebox-block", "protocol": "blackhole", "settings": {}}
  ],
  "routing": {"rules": [
    {"type": "field", "inboundTag": ["inbound-01"], "network": "udp", "outboundTag": "xnvgatebox-block"},
    {"type": "field", "inboundTag": ["inbound-01"], "network": "tcp", "outboundTag": "exit-slot-01"}
  ]}
}
```

工具根据输入的真实 tag 替换 inbound-01，同时生成受管入站的 UDP blackhole 规则和对应 outbound，排在 TCP 映射之前。批量导出使用标准 `outbounds` 与 `routing.rules`，没有自定义包装字段。

CLI输出“复制 outbound / 复制 routing / 导出全部”对应内容；WebUI沿用相同exporter。校验重复tag、端口、悬空引用和规则顺序，用固定Xray版本检查配置，再经测试入站观测出口。Slot不健康或停止仍保留映射到原端口，不能删规则后落到默认direct。删除Slot须导出原绑定的显式block处理。

不写3x-ui数据库，不在v0.x自动接管面板；“应用到3x-ui”按钮到API功能实际实现后才启用。

## 8 v0.5 自动换 IP 与 Slot 策略

故障顺序：暂停业务 → 清理旧连接/隧道 → 从统一池选择符合策略的新Node → 本机OpenVPN基线与画像 → 固定SOCKS actual核验 → 恢复healthy。Comcast A换成Spectrum B后，`slot-01 / 17928 / exit-slot-01`不变；切换会中断旧TCP连接，暂不做无缝热切换。

策略最少包含country、allowed_ip_types、preferred/allowed/blocked ISP、ASN允许/阻止列表和maximum latency。国家/类型/黑名单/allowed是硬限制，preferred只是排序；想“只用三家ISP”需配置allowed。手动指定Node也需遵守硬策略，缺字段则不能宣称满足该项。

国家与类型最终使用本机OpenVPN出口画像，来源国家和SSTP画像只用于初筛。maximum latency用本机SOCKS/Xray请求测量，不能把来源ping当本机延迟。禁止健康检查自动执行expected=actual；新IP只能通过明确重新建连、基线和分类过程接受。

每Slot操作加锁、取消已停止的任务，先重连旧节点或按策略替换，失败候选短暂冷却并退避。没有候选保持blocked/waiting；不改变国家/类型、不扩大ISP范围、不随机改端口。重启只恢复固定配置，必须重新验证后才healthy。

## 9 v0.6 WebUI

Validated Nodes：Country、按协议的Exit IP、ASN、ISP、strict/likely等类型、Latency、SSTP/OpenVPN状态、对应环境Full Chain、Last Verified。支持查看理由、添加到Slot；未测/过期证据不显示绿灯。

Exit Slots：Name、目标国家、Current/Expected Exit IP、ASN、ISP、SOCKS Port、Tunnel、Health、Last Switch。支持启动、停止、换IP、指定节点、修改国家/ISP策略、删除及配置复制/导出。

界面体验：创建 `slot-01` 并设置目标国家 → 显示127.0.0.1:17928和实际画像 → 复制Xray outbound → 复制routing。HTTP管理接口随WebUI提供必要登录与访问限制；远程自动3x-ui API集成以后再做。Mode A 管理页由唯一 Worker 提供；VPS 页面使用同一安装包内的管理入口，不新增独立管理服务，也不通过静态页面直接执行root命令。

## 10 最小实现与运行预算

```text
src/        vpngate.py / models.py / pool.py / classifier.py / checker.py / subscription.py
scripts/    full_chain_check.py
worker/     一个发布入口 / check、data、admin、subscription 内部模块
vps/        multi-exit manager / netns / socks / health / exporter / install
data/       node_pool.json / mode_a_validated.json / cf_manifest.json / residential.json
runtime/    slots.json / slot_health.json / private-subscription.txt（忽略提交）
public/     index.html / nodes.txt
.github/workflows/update.yml
docs/       DEVELOPMENT.md / REFERENCE_AUDIT.md
```

v0.x 核心流水线用 Python，`Pool Builder` 必须既能在 Actions 调用，也能在 VPS Standalone 调用；Cloudflare 最终只发布一个 Worker，VPS 尽量复用现有 Multi-Exit 底座。不要为了三种运行模式复制三套抓取/分类实现，也不先搭 TypeScript 迁移、多 JSON Schema、SQLite、签名服务或 IPC 框架。共享 JSON 字段和小量必要模型校验即可。统一部署入口、配置文件和状态页优先于增加新的可部署服务。

### 10.1 全量发现与分层检测预算

以下是下一阶段的目标配置；不代表当前代码已经完成这些配置字段：

```ini
discovery_limit = unlimited
prescreen_limit = 300
expensive_probe_limit = 50
full_chain_limit = 20
```

```text
VPN Gate 来源全量发现、解析和去重
  → node_pool.json：保留全部合法候选
  → 最多 300 个候选进入粗筛
  → 从粗筛结果选最多 50 个进行真实隧道握手
  → 从握手通过者选 10～20 个进行最终链路验活
  → mode_a_validated.json：仅保留最终验活通过且未过期者
  → cf_manifest.json → 唯一 KV → 订阅
```

- `discovery_limit=unlimited` 表示不按节点数量截断来源清单；完整解析并去重后才应用探测预算。网络超时、响应大小和配置合法性检查仍然保留；来源不完整时显式报告，不能把截断清单称为全量发现。
- `prescreen_limit=300` 是进入低成本检测的最多候选数。筛选和排序基于完整池及用户策略，不只取来源顺序中的前 300 行；预算允许时兼顾目标地区，并在后续批次轮换未测候选，避免长期只探测同一小批节点。来源国家只供初筛，最终国家仍由实际出口画像确认。
- `expensive_probe_limit=50` 是真实 SSTP 握手的最多候选数，不是抓取上限；VPS 使用同一分层逻辑时，这一阶段执行本机 OpenVPN 握手。所有阶段均不得以入口 TCP 连通替代隧道成功。
- `full_chain_limit` 是最终链路探测的最多候选数，起始值可选 10～20（默认 20）。通过上一步的节点更少时只测试现有合格者；这个数是预算，不是发布数量或成功率承诺。Mode A 经实际 VLESS 配置验活，VPS 经固定 SOCKS/Xray 入口验活。
- 预算外节点保留在 `node_pool.json` 中，状态保持 `not_tested` 或已有证据的真实状态；过期证据标为 `stale`。不得把预算耗尽标为节点失败，也不得跳过最终验活后把节点加入发布清单。
- 报告记录发现、去重、粗筛、握手、最终验活和发布的数量，以及预算耗尽、来源异常和服务错误，便于判断是候选不足还是检测预算不足。全量发现后也不能为满足数量而放宽 Slot 的国家、ISP 或类型策略。

Actions 保留手动触发，同时启用默认每 20 分钟一次的 schedule；手动运行用于立即刷新或诊断，定时运行用于持续更新。起始并发与超时：SSTP并发4、Xray并发2、单节点限时30秒、完整链路45秒、瞬态错误最多重试1次；根据实际用量调整分层预算，不回到抓取阶段硬截断来源。画像按出口IP缓存24小时，链路证据默认60分钟过期。GitHub schedule 为尽力调度，实际可能延迟或漏跑；20分钟刷新频率提供重叠窗口，但不保证持续可用，过期清单仍须停止订阅发布。429/Worker异常单独报告，不把服务限额当所有节点失效；旧报告可保留但过期节点不发订阅。具体平台配额见审计文档。

## 11 v0.x 底线与 v1.0 加固

| v0.x立即保留 | v1.0再做或按需要增加 |
| --- | --- |
| 真实握手、两个HTTPS Echo、actual==expected、最终配置一致 | Checker TLS身份认证修复；signed Echo仅可选 |
| 最小tun绑定、防回退路由/防火墙、断线失败关闭 | 强化kill switch、广泛故障注入、权限分离与恢复 |
| 自有HTTPS源、SHA256、JSON写入替换、简单锁 | 公共池Ed25519签名、防回滚、复杂发布/事务机制 |
| 固定Slot身份、端口与tag，不健康不落默认direct | 多机调度、复杂状态机、无缝切换 |
| 配置脚本清理、loopback监听、Secrets、无公开UUID/私有订阅 | root-helper/cgroup/独立agent与完整审计 |
| 命令/配置导出；WebUI实现时必要鉴权 | 3x-ui API自动应用、备份与回滚 |

保留既有许可边界：fanout MIT可复用并保留声明；CheckSocks5按GPL v3条件处理；gate与CF-vpngate无明确许可时仅参考；AimiliVPN暂仅功能参考。EdgeTunnel仅保留外部兼容接口，不将许可兼容性未确认的Worker源码混成一份，不把第二个部署作为必需依赖。许可记录不能用“组合项目”代替。

v0.1 已在本机和 GitHub Actions 完成；v0.2 现用保留许可声明的 Multi-Exit 底座读取同一 `node_pool.json`，并已在一台小规格 Debian VPS 上完成一个固定 Slot 的真实验收（包括掉线失败、kill-switch 和自动恢复），证据见 [`VPS_DEPLOYMENT.md`](VPS_DEPLOYMENT.md)。Cloudflare 已完成单 Worker 合并的第一版实现：普通运行变量、唯一 KV、内置 SSTP 检查和 TCP-only 数据面，不依赖旧 Worker。当前仍需补充 Cloudflare Sockets 实际正向/失效后端验收、全量发现与分层筛选、VPS 三 Slot；未完成这些验收前，不能把本地单元测试当成完整链路交付。外部兼容适配器和 3x-ui API 自动写入继续后置。
