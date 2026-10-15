# 开发版本迁移验证

状态：2026-10-15，本地回归已完成，尚未发布或升级 VPS。本文件描述画像、Slot 策略、配置回滚开发版本的迁移检查。它不改变 [v0.3.1 外部验收记录](VPS_STANDALONE_VALIDATION.md)，也不代表新代码已通过真实链路验收。

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

## 后续有权限测试环境的验收

当前云端禁止 Setns 和 TCP 监听，不能在这里完成以下步骤。测试应使用独立 Linux 测试机及私有状态副本，避免同机固定端口冲突；本轮没有连接或修改线上 VPS、3x-ui 或 Cloudflare。

用户后续已授权原 VPS 作为测试机，允许备份、升级和真实故障验收；3x-ui 和 CF 不在修改范围。授权不等于已经连通：目前 SSH 连接检查尚未进入认证，实际迁移和真实验收仍未执行。

| 场景 | 必须确认的结果 |
| --- | --- |
| 旧版升级启动 | 三 Slot 编号、端口、订阅 token、管理入口、UUID/REALITY 密钥保持；重新创建 namespace 和验证证据，不继承旧 healthy |
| 实际画像与策略 | 从同一 Slot 的实际 OpenVPN 出口查询；未配置服务、失败或过期如实 unknown；缺国家/ISP/ASN 证据就等待，不用 SSTP 或来源字段冒充 |
| 真实聚合订阅 | 从测试机外部读取实际订阅，逐节点生成客户端；各自访问两个 HTTPS IP 查询和内容页，actual 等于对应 Slot expected，且不是母机 IP |
| 无合格候选 | 保留原策略、Slot 编号和端口，显示等待/阻断；不跨国家或放宽类型补位 |
| 掉线与恢复 | 仅对测试实例注入故障；SOCKS 和公开入站均失败，不能回落母机；自动恢复后同一订阅重新可用，无需手工绑定 |
| 静态配置错误 | 候选被拒绝，旧进程和已提交文件保留；不把失败变更加入回滚历史 |
| 启动/提交失败 | 旧文件和运行配置恢复；恢复也失败则停止 Xray、阻断链接并保留恢复记录，不伪报成功 |
| 中断后重启 | 启动加载前恢复未完成事务；受管文件集合不匹配或记录损坏则报错，不能默认初始化覆盖 |
| 手动回滚 | Native 恢复上次入站/客户端/绑定，按当前 Slot 状态生成路由；策略回滚重新验活，停止 Slot 保持停止 |
| 管理设置失败 | 端口预留或文件提交失败时保留旧监听、路径、密码和 token；新监听只在完整提交后启用 |

已有国家策略即使显式允许 unknown，也仍需要可信的国家证据。没有画像服务时保持等待是预期行为，不能为延续旧版的技术连通而自动弱化策略。Native 回滚可能重新启用旧客户端凭据；整体重启 Xray 会中断其他连接。升级前保留完整私有备份，回退二进制与恢复整份备份属于单独的测试步骤，不等同于管理页的配置回滚。

只有这些实际链路和故障场景通过后，才能更新新版本验收结论及准备发布。不得以本地合成样例、静态解析或历史 v0.3.1 的成功代替本轮验收；unknown 不能称家宽。

## 可重复运行的验收工具

测试包包含 `scripts/vps_acceptance.py`，只使用标准库、同包 Python 模块、curl 和固定版 Xray。它当前支持本次已验收形态：三个固定 SOCKS Slot，各绑定一个 VLESS/TCP/REALITY 入站。不支持的订阅字段或传输组合会报错，不改写为另一份手工配置。

以下命令须在已授权且具备对应网络权限的测试主机执行。本云端仅完成合成回归和静态解析，没有执行实际故障注入。

1. 停止本项目管理器后，将状态目录和旧部署文件备份到权限为 `0700` 的私有备份目录；备份文件用 `0600`，保留旧二进制、Xray、服务单元、环境文件及证书。不要停止或备份改写 3x-ui。以停止后的一致副本运行候选二进制的 `-check-state`，再决定是否升级。升级前检查画像服务配置：缺可信国家证据时旧版地区策略会等待，不能自动放宽来使测试通过。
2. 使用本项目安装入口从已核对校验和的本地测试包升级。确认二进制版本和 `BUILD.json` 对应本次源代码；保留完整备份以便恢复旧安装。不要从测试包执行更新到公开 latest 的流程。
3. 在测试 VPS 上采集私有计划：

```sh
python3 /usr/local/lib/xnvgatebox/scripts/vps_acceptance.py capture-plan \
  --state-dir /var/lib/xnvgatebox/fanout \
  --output /var/backups/xnvgatebox/acceptance-plan.json
```

工具从状态文件读取管理口令，仅向显式 loopback 管理地址登录，读取实时 Slot API 和实际 base64 订阅。需要三个独立、非母机的 up 出口，否则报错。私有计划含 token、实际订阅和 SOCKS 凭据，自动写为 `0600`；不要发布。管理器只监听具体外部地址而未监听 loopback 时，此入口拒绝发送口令，不自动更改监听。

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

它比较管理入口/密码、订阅 token、默认类型策略、Slot 编号/端口/凭据/停止意图、已有显式策略及入站客户端和密钥；允许后台节点及绑定随恢复改变，只输出是否保留的布尔值。输出不得放入被测状态/备份目录，不能覆盖私有计划。所有命令非零退出均表示未通过；报告文件是 `0600`，stdout 只输出状态/固定错误码。私有计划和原始状态不可附到公开验收记录。

管理设置/Native 配置的事务故障目前由 Go 故障注入回归覆盖，工具的 `fault` 模式只执行 OpenVPN 掉线注入。真实文件写入失败、Xray 重启失败及回滚中断仍须按上表在可恢复的测试副本上单独验证，不能以 `fault` 成功代替。
