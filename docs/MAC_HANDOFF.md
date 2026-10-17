# Mac 开发交接（2026-10-15）

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
