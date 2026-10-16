# 开发版本迁移验证

状态：2026-10-15，Mac 已接续指定分支并完成实际备份、开发包升级、三节点订阅基线、Slot 1 强杀恢复及完整备份恢复演练。以下区分本轮实测和此前云端的离线结果；历史 [v0.3.1 外部验收记录](VPS_STANDALONE_VALIDATION.md) 保留。本轮仍是开发测试版；原 RU 波动及后续用户授权改为 KR 的验收分别记录。

## 验收口径：上游节点可用性

指定国家能否取得合适 IP，取决于 VPN Gate 上游当时实际提供的候选及其可达性。本程序负责正确发现、筛选、验活和绑定，不承诺每个国家随时有出口，也不要求反复试到某国成功才算完成验收。

上游未提供合适候选时，正确提示暂不可用、保留策略和端口、等待重试并阻断母机回落，即可通过相应功能检查。该国正向连通项记录为“上游条件不足，暂未验证”，不计作程序缺陷，也不标为连通成功。多出口可用当时有合适节点的国家验证，三个独立出口可以来自同一国家；修改已选国家由用户决定。程序的候选处理、绑定、凭据保留或防回落错误仍需修复。

历史报告中的超时、连接失败等保留为当次探测结果，不能仅据这些错误码认定本程序有缺陷。

## 2026-10-15 Mac 实测

接续分支为 `codex/standalone-migration-20261015`，基线 `389288dbfa683210d1e0e9dfbb4137cb5960ec5f`。切换前，原工作区无未提交的受控文件改动；Git 历史、源码及忽略的私有运行文件已完整归档到本机 `runtime/local-backups/20261015-185510/`。后来提供的 `ssh.md` 保留原内容，设为 `0600` 并加入本机 `.git/info/exclude`，不进入源码或打包快照。

测试 VPS 的旧安装在停止本项目管理器、pool service 和 timer 后取得完整一致备份；包括旧二进制、内置 Xray、全部项目状态与证书目录、环境和 systemd 单元。备份归档已核验 SHA256，并复制到本机私有目录。SSH 使用已知主机密钥严格校验，连接密码不进入新脚本或命令参数。

最终测试包版本为 `dev-20261015-migration-acceptance-fix`，固定 Linux Xray 为 `v26.3.27`。包的 `BUILD.json` 如实标记基线提交和 `source_dirty: true`，源码快照摘要为 `8151f5974fcd4d6d62da685a9641a431de24903ea4222a7399f379fb1b3e7018`；包 SHA256 为 `160b3e3213c4cf0c59345a69143332c29a96d3fc9a2e357db71cb3858b80f95d`。这不是 GitHub 正式 release。

实测发现并修复三处问题：HTTPS Echo 自带末尾换行时，SOCKS 耗时解析误拒绝有效响应；一轮探测超过冷却时间后，同一批高排名失败候选挤占后续候选；验收工具对暂未绑定入站的 `null` 列表崩溃。分别补充真实响应形态、六个候选预算和未绑定等待回归。新增显式启用的 Linux 实测测试，见本文最后一节。

| 项目 | 本轮结果 |
| --- | --- |
| 候选只读迁移检查 | 三 Slot、原 SOCKS 凭据及三个入站有效，不生成新凭据，不写状态 |
| 缺画像时的真实阻断 | 保留 JP/KR/RU 策略；默认订阅 HTTP 404，三个 SOCKS 及实际 `bound=0` 订阅客户端均失败；没有母机回落 |
| 开发包实际升级 | 经统一安装入口使用已校验本地包；八项保留检查全部通过 |
| 真实聚合订阅基线 | VPS 本机公开客户端及三个固定 SOCKS 均通过；Mac 外部重新下载实际三节点订阅，也全部通过。完整恢复再升级后的末轮本机和 Mac 外部也全部通过 |
| Slot 1 OpenVPN 强杀 | VPS 本机 SOCKS 两次 curl 97、公开入站两次 curl 35；Mac 同步两次 curl 35。均在有效客户端基线之后注入，未把配置错误算作阻断 |
| 自动恢复 | 新 OpenVPN 进程被确认；原订阅客户端参数保持，VPS 本机三个入站和 SOCKS 全部复验通过。Mac 的故障 Slot 1 也重新通过 |
| 停止与策略回滚 | 停止期间策略写入、回滚后保持停止、恢复原策略和客户端参数均通过；随后整轮复验出现 RU 超时，原失败报告保留 |
| 真实内置配置恢复 | 实际 Xray 拒绝、监听冲突补偿、手动回滚、真实文件提交/回滚失败后阻断、SIGKILL 后恢复及管理监听保留均通过，使用一次性副本 |
| 刷新失败保留旧池 | 独立临时配置产生真实来源下载失败；正式池和全部 profile 摘要不变，正式来源配置不改，timer 已恢复 |
| 完整备份恢复 | 实际还原 `v0.3.1` 二进制、项目目录、环境和服务；保留项全部通过。随后经同一入口装回最终测试包，再次一致比较及 RU 自动换点后的最终一致比较仍全部通过 |

第一轮三节点同时通过时的实际出口为 JP `219.107.54.157`、KR `121.130.89.23`、RU `77.35.247.38`。完整备份恢复再升级后，第三 Slot 在原 RU 策略内自动换到 `5.143.104.161`，本机和 Mac 末轮三个节点均通过，原订阅客户端参数不变。SOCKS 端口仍为 `17928..17930`，公开端口仍为 `30001..30003`。当前没有可信的住宅类型、ISP 或 ASN 结论，三条均为 **unknown**。

VPS 原来没有画像配置。先完成缺画像阻断检查，再接入无需密钥的 HTTPS `ipwho.is` 接口，只请求 `ip,success,country_code`，通过同一 Python adapter 校验实际 OpenVPN IP，独立 SOCKS actual 匹配后使用国家证据。该响应不提供本轮分类所需的住宅字段，因此仍是 unknown，不能称家宽。接口能力及限制见 [官方说明](https://ipwhois.io/documentation)；配置和缓存仅在 VPS 私有目录。

Mac 故障恢复后的第一次整轮复测中，Slot 1 和 Slot 2 通过，RU 返回 `curl_failed_28`；停止/恢复后的整轮 RU 返回 `curl_failed_35`，两次失败均保留。末轮完整恢复再升级时，原 RU 节点已从最新官方池消失，第三 Slot 先阻断等待，随后在 RU 内自动换点并完成三出口复验，未跨国补位。功能通过不代表志愿者节点持续稳定；不能用最终成功删除之前的失败记录。

本轮 Python 62 项通过，Node 边界 29 项及统一 Worker 回归通过；真实 Linux 完整 Go 回归 188 个顶层通过、244 个通过事件，唯一顶层跳过为仅供父用例启动的 `TestLiveNativeInterruptedChild`，SIGKILL 场景已由父用例实际执行。Mac 没有运行 Linux namespace 替代测试。

Cloudflare 保持暂停，未调用部署或恢复定时任务；共享池工作流仍仅手动触发。3x-ui 未写入或重启，数据库和配置指纹与本轮开始时一致，服务保持 active。原始状态、订阅、计划、事务记录及访问文件保留在私有目录，不提交。手机网络及住宅类型不在本次通过结论中。脱敏证据见 [本轮完整记录](evidence/vps-migration-mac-20261015.json)，包含成功和失败各轮结果。

## 2026-10-15 用户授权第三 Slot 换国家

在前一轮 RU 自动换点和三节点复验通过后，收到用户明确授权“允许第三 Slot 换其他国家继续验收”。先将第三 Slot 改为 US；本轮观察到的两个美国候选分别出现 `tun0` 等待超时和 OpenVPN 提前退出，未作为健康节点发布，原始失败日志保留在私有目录。随后明确改为 KR，当前策略为 **JP/KR/KR**，即两个国家、三个独立出口。

换国家后的实际出口为 Slot 1 JP `219.107.54.157`、Slot 2 KR `121.175.117.96`、Slot 3 KR `211.49.151.5`。VPS 本机三个固定 SOCKS/公开入站和 Mac 外部真实订阅均完成两个 HTTPS Echo 各两轮及内容页验证，三个 actual 分别等于各自 expected，且互不相同、均非母机 IP。三条出口类型仍为 **unknown**，不构成住宅、ISP 或 ASN 验收。

随后精确强杀第三 Slot 的受管 OpenVPN。故障前先证明实际订阅客户端可用；故障窗口 VPS 本机 SOCKS 两次 `curl_failed_97`、公开入站两次 `curl_failed_35`，Mac 同步两次 `curl_failed_35`，均未回落母机。新 OpenVPN 进程自动恢复后，VPS 三节点 SOCKS/公开入站和 Mac 三节点整轮真实订阅复验全部通过，原客户端参数保持。

故障恢复后一致副本及实时检查的 15 项均通过：第三 Slot 仅国家由 RU 改为 KR，其他策略字段不变，Slot 1/2 策略不变；Slot 编号、`17928..17930`、公开 `30001..30003`、SOCKS 凭据、订阅 URL/token、UUID/REALITY 密钥和管理设置均保留。3x-ui 数据库/配置指纹不变且服务 active；本项目 pool timer active，无未完成事务。Cloudflare 继续暂停，没有部署或启用定时任务。本次没有变更实现代码或重新发布安装包，先前完整备份、迁移和回归结果继续作为前一轮记录保留。

新增脱敏证据见 [授权换国家与 Slot 3 故障验收](evidence/vps-country-change-20261015.json)。美国候选失败及前一轮 RU 波动均保留；这些时点的成功不能视为志愿者节点的持续稳定保证。

## 已完成的离线检查

开发二进制增加只读入口：

```sh
xnvgatebox-manager -check-state -dir /path/to/private-state-copy -max 3
```

应对停止服务后取得的完整、一致、私有状态副本运行；不要边写边复制，也不要把私有文件提交到仓库。此命令面向内置 Xray 模式，要求副本包含 `state.json`、`native.json`、`settings.json`、`password`、`basepath`；存在 `native.previous.json` 时一并检查。它不要求 root，不创建目录、生成凭据、恢复事务、修改文件或全局设置，不运行子进程或网络请求。

退出码 `0` 表示所检查的持久化结构有效，`1` 表示检查失败或报告写出失败。JSON 始终标注 `scope: native_persisted_structure_only` 和 `live_validation_required: true`。`config_valid` 只覆盖状态身份、策略、凭据配对、管理设置和内置配置的基本存储结构；不检查端口是否实际空闲、证书文件可读性、REALITY 目标可用性、所有 Xray 字段语义或真实客户端连通。

报告包括 Slot 编号、端口、停止状态、迁移后的国家/允许类型、是否沿用旧版默认策略、是否需要在启动时生成缺失的整套旧版 SOCKS 凭据，以及入站数量。报告不返回后台主机名、OpenVPN 配置、管理路径、密码、UUID、密钥或订阅 token。缺少 token 会报错，避免把下一次自动生成 token 误判为“保留原订阅”。

遇到任一未完成事务记录时返回 `pending_transaction`，不自动恢复。应保留记录，通过正常恢复流程处理并重新取得一致副本；不要删除记录来让检查通过。缺失、非普通文件（包括符号链接）或超过 16 MiB 的受检文件会报错。

旧版没有 Slot 策略时，沿用保存的国家及加载后的 `residential_only` 默认值；`FANOUT_RESIDENTIAL_ONLY=0` 的现有覆盖规则也生效。已有显式策略保持原意。两项 SOCKS 凭据均缺失的旧状态可在正式启动时生成；只有一项缺失或内容无效则拒绝恢复，不静默轮换已有客户端凭据。损坏的 `null` 状态/内置配置/管理设置、空管理口令、无效访问路径或监听设置会报错，原文件保留。

合成旧版样例位于 `vps/fanout/testdata/migration-v031/`，字段结构依据基线提交的旧版 schema 创建，全部凭据是公开测试数据，REALITY 密钥采用 RFC 7748 测试向量；不是从 VPS 导出的状态。三 Slot 使用固定 `17928..17930`、三个 VLESS/REALITY 入站，覆盖干净关闭及未完成换节点绑定。回归验证了身份/凭据保留、当前池配置优先、旧节点消失时使用存盘配置，以及恢复后重新验活前所有出站 blackhole。固定版 Xray 对生成配置做静态解析，未启动监听或验证 REALITY 握手。

## 真实测试环境的验收要求

真实测试使用已授权的 Linux VPS 及私有状态副本；Mac 负责外部客户端复验，不模拟 Linux namespace。以上实测章节已记录本轮结果，以下要求供后续复验使用。3x-ui 和暂停的 CF 始终不在修改范围。

| 场景 | 必须确认的结果 |
| --- | --- |
| 旧版升级启动 | 三 Slot 编号、端口、订阅 token、管理入口、UUID/REALITY 密钥保持；重新创建 namespace 和验证证据，不继承旧 healthy |
| 实际画像与策略 | 从同一 Slot 的实际 OpenVPN 出口查询；未配置服务、失败或过期如实 unknown；缺国家/ISP/ASN 证据就等待，不用 SSTP 或来源字段冒充 |
| 真实聚合订阅 | 从测试机外部读取实际订阅，逐节点生成客户端；各自访问两个 HTTPS IP 查询和内容页，actual 等于对应 Slot expected，且不是母机 IP |
| 无合格候选 | 保留原策略、Slot 编号和端口，正确显示暂不可用并等待/阻断，即可通过此项；该国连通记为上游条件不足，可由用户选择其他国家继续 |
| 掉线与恢复 | 仅对测试实例注入故障；SOCKS 和公开入站均阻断，不能回落母机；有合适候选后验证自动恢复及原订阅可用，无候选时验证正确等待 |
| 静态配置错误 | 候选被拒绝，旧进程和已提交文件保留；不把失败变更加入回滚历史 |
| 启动/提交失败 | 旧文件和运行配置恢复；恢复也失败则停止 Xray、阻断链接并保留恢复记录，不伪报成功 |
| 中断后重启 | 启动加载前恢复未完成事务；受管文件集合不匹配或记录损坏则报错，不能默认初始化覆盖 |
| 手动回滚 | Native 恢复上次入站/客户端/绑定，按当前 Slot 状态生成路由；策略回滚重新验活，停止 Slot 保持停止 |
| 管理设置失败 | 端口预留或文件提交失败时保留旧监听、路径、密码和 token；新监听只在完整提交后启用 |

已有国家策略即使显式允许 unknown，也仍需要可信的国家证据。没有画像服务时保持等待是预期行为，不能为延续旧版的技术连通而自动弱化策略。Native 回滚可能重新启用旧客户端凭据；整体重启 Xray 会中断其他连接。升级前保留完整私有备份，回退二进制与恢复整份备份属于单独的测试步骤，不等同于管理页的配置回滚。

按实际覆盖的链路、故障检查和已知程序缺陷更新验收结论及评估发布；上游节点不足的连通项单独注明，不要求指定国家全部在线。实际未测到的能力保留为未验证，本地合成样例、静态解析及历史 v0.3.1 结果各自注明范围；unknown 不能称家宽。

## 候选包与正式发布

普通 push/PR 构建仍标记为 `dev`。发布准备时，手动运行 `VPS tests and package` 并显式传入 `version=vX.Y.Z`，由同一次运行完成 Linux 回归及打包；`BUILD.json` 记录干净源码提交和构建版本。先对该候选包校验并完成安装/升级冒烟，再使用其成功的 run ID 和相同版本运行 `Publish tested VPS package`。

发布流程校验下载包的归档路径、逐文件摘要、完整 manifest、`VERSION`、`BUILD.json` 与测试运行的提交；拒绝 dirty 源码、外供管理器二进制、版本或提交不符，以及覆盖已公开的版本。Cloudflare 的部署和定时任务不参与此流程。

## 可重复运行的验收工具

测试包包含 `scripts/vps_acceptance.py`，只使用标准库、同包 Python 模块、curl 和固定版 Xray。它当前支持本次已验收形态：三个固定 SOCKS Slot，各绑定一个 VLESS/TCP/REALITY 入站。不支持的订阅字段或传输组合会报错，不改写为另一份手工配置。

以下命令须在已授权且具备对应网络权限的测试主机执行。本轮 Mac/VPS 已实际执行，记录见上文。

1. 停止本项目管理器后，将状态目录和旧部署文件备份到权限为 `0700` 的私有备份目录；备份文件用 `0600`，保留旧二进制、Xray、服务单元、环境文件及证书。不要停止或备份改写 3x-ui。以停止后的一致副本运行候选二进制的 `-check-state`，再决定是否升级。升级前检查画像服务配置：缺可信国家证据时旧版地区策略会等待，不能自动放宽来使测试通过。
2. 使用本项目安装入口从已核对校验和的本地测试包升级。确认二进制版本和 `BUILD.json` 对应本次源代码；保留完整备份以便恢复旧安装。不要从测试包执行更新到公开 latest 的流程。
3. 在测试 VPS 上采集私有计划：

```sh
python3 /usr/local/lib/xnvgatebox/scripts/vps_acceptance.py capture-plan \
  --state-dir /var/lib/xnvgatebox/fanout \
  --output /var/backups/xnvgatebox/acceptance-plan.json
```

工具从状态文件读取管理口令，仅向显式 loopback 管理地址登录，读取实时 Slot API 和实际 base64 订阅。这个三节点验收入口需要三个独立、非母机的 up 出口，否则报错；这是该入口的运行前提，不代表目标国家缺少上游节点就是程序验收失败。条件不足时记录未开展此项，并继续检查等待、阻断和其他可测功能。私有计划含 token、实际订阅和 SOCKS 凭据，自动写为 `0600`；不要发布。管理器只监听具体外部地址而未监听 loopback 时，此入口拒绝发送口令，不自动更改监听。

4. 从 VPS 自身检查固定 SOCKS 和公开入站（报告标记为 `server-local-public-client`）：

```sh
python3 /usr/local/lib/xnvgatebox/scripts/vps_acceptance.py verify \
  --plan /var/backups/xnvgatebox/acceptance-plan.json \
  --xray /usr/local/lib/xnvgatebox/bin/xray \
  --vantage server-local-public-client --socks \
  --output /var/backups/xnvgatebox/acceptance-local.json
```

向外部测试主机安全传送私有计划后，在外部运行相同的 `verify`，使用 `--vantage external-client` 并去掉 `--socks`。外部主机必须具备到 VPS 和两个 Echo/内容站点的网络权限；不得把 VPS 自身对公开端口的测试标记为外部验收。订阅重新从实际地址下载，所有连接参数必须与采集时一致，节点顺序变化允许；重定向被拒绝。现有测试 VPS 的 HTTP 订阅地址会被原样使用，不自动改成 HTTPS 或更换 token。每节点两轮访问两个 HTTPS Echo，并检查 Example Domain 内容；任何 IP 不等于本轮 expected 或等于母机 IP都失败。此工具不能替代手机实际网络体验检查。

5. 在 VPS 上对指定 Slot 注入故障并观察恢复：

```sh
python3 /usr/local/lib/xnvgatebox/scripts/vps_acceptance.py fault \
  --state-dir /var/lib/xnvgatebox/fanout \
  --xray /usr/local/lib/xnvgatebox/bin/xray --slot 1 --recovery-timeout 300 \
  --output /var/backups/xnvgatebox/acceptance-fault.json
```

`fault` 会发送 SIGKILL，仅在三 Slot 的完整正向基线通过后执行。它核对 namespace、OpenVPN 可执行文件及精确配置路径，要求唯一匹配，再用 pidfd 重新确认进程身份后发送信号；不按名称批量杀进程。真实公开 Xray 客户端先启动并验证成功，不能把客户端启动错误当阻断证据。SOCKS 和公开入站并发进行两个请求；成功返回任何内容都会失败，证书校验错误不算有效阻断。随后读取实时状态，用恢复后的新 expected 验证三个 Slot，确认 OpenVPN 进程已替换且原订阅客户端配置不变。阻断断言失败时也继续观察恢复，并保留分项失败结果。此模式报告仍为 VPS 自身测试；外部断线窗口须另行同步测试。

6. 对升级前后一致副本执行不泄密的比较：

```sh
python3 /usr/local/lib/xnvgatebox/scripts/vps_acceptance.py compare-state \
  --before /private/backup-before/fanout --after /private/snapshot-after/fanout \
  --output /private/acceptance-preservation.json
```

它比较管理入口/密码、订阅 token、默认类型策略、Slot 编号/端口/凭据/停止意图、已有显式策略及入站客户端和密钥；允许后台节点及绑定随恢复改变，只输出是否保留的布尔值。输出不得放入被测状态/备份目录，不能覆盖私有计划。命令非零退出表示该次检查未成功完成，需结合原因区分上游条件不足、测试前提不足和程序缺陷，不能一概归为程序验收失败；报告文件是 `0600`，stdout 只输出状态/固定错误码。私有计划和原始状态不可附到公开验收记录。

工具的 `fault` 模式只执行 OpenVPN 掉线注入。管理设置/Native 配置的真实 Xray 拒绝、监听冲突、文件提交失败和回滚中断已由下述显式启用的 Linux 实测覆盖；它们使用可恢复的一次性副本，结果与 OpenVPN 故障验收分别记录。

## 显式启用真实配置恢复测试

`native_live_acceptance_test.go` 在默认回归中跳过。它在一次性目录中复制已备份的内置配置和固定 Xray，使用随机入站端口及 blackhole 路由，实际运行 Xray 校验、启动、文件提交和恢复，不创建 Slot、不启动 OpenVPN、不调用面板或 systemd。

在已授权、可执行 bind mount 的 Linux 测试环境中，从 `vps/fanout` 运行：

```sh
XNV_LIVE_ACCEPTANCE=1 \
XNV_LIVE_NATIVE_COPY=/private/consistent-copy/native.json \
XNV_LIVE_XRAY=/usr/local/lib/xnvgatebox/bin/xray \
go test -v -run '^TestLive' -timeout 2m
```

也可在 Mac 用 `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c` 编译后，将测试程序和所需源码样例安全传送到测试 VPS；完整普通回归仍须从对应源码的 `vps/fanout` 目录运行。不要用 Mac Xray 执行 Linux 检查。

测试覆盖真实静态配置拒绝后旧进程不变、端口冲突后旧配置运行恢复、手动回滚按当前不可用 Slot 阻断，以及 bind mount 造成 `rename` 失败时停止 Xray、保留恢复记录，解除故障后重新打开配置恢复。独立子进程在部分提交后被精确 SIGKILL，重新打开时实际清理其 Xray 孤儿进程并恢复事务文件。管理检查使用合成私有凭据和真实 loopback 监听，验证端口占用、文件提交失败及恢复后旧入口保留。

`TestLiveNativeInterruptedChild` 仅由父测试启动，顶层单独跳过是预期行为。任何真实挂载、监听或 Xray 检查失败都应算失败，不降级为模拟通过。测试来源文件只读；临时副本、进程及挂载由测试清理。该组结果仍不替代实际订阅和 OpenVPN 强杀验收。
