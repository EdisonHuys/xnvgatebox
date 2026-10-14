# VPS Standalone 验收（2026-10-15）

## 已完成

- 当前 Git 历史已用 `git bundle --all` 备份并验证，全部跟踪源码已归档并校验。私有备份在本机 `runtime/backups/before-vps-20261014-235335/`，不提交公开仓库。
- 线上 Cloudflare 源码、普通变量、域名绑定清单和 manifest 已保存到该私有备份目录。`xnvgatebox` Worker 删除成功，API 列表确认不存在，项目域名绑定数量为零。KV 数据保留。
- 旧 CF 自动任务已停用；仓库 `update.yml` 改为仅手动构建共享候选池，不再调用或发布 CF。
- Linux CI 以 root 运行全部 Go 测试，包含 namespace 恢复、默认阻断、固定 Slot 标签和认证导出；Python 35 项回归通过。
- GitHub Release `v0.3.0` 安装包由通过测试的 Actions 产物发布，外部归档 SHA256 与包内成员校验均通过。包内固定版本 Xray 的官方归档 SHA256 在构建时验证。
- 在 Debian 12、1 vCPU、约 1 GB 内存测试 VPS 上实测本项目安装入口，当前程序显示 `xnvgatebox v0.3.0`。管理端直接监听 `8899`，原访问路径、密码、订阅 token 保留。
- 内置 Xray 模式无需 3x-ui；原 `x-ui` 服务仍为 active，未自动修改其数据库。每十分钟的本地 pool timer active，管理器读取新快照。
- 首次全量发现本地 99 个候选；不要求远程池、CF 或 Actions 账号凭据。仅来源配置存在不表示候选已连通。

## 实际 Slot 状态

| Slot | 国家 | SOCKS 地址 | 实测出口 | 类型 |
| --- | --- | --- | --- | --- |
| slot-01 | JP | 127.0.0.1:17928 | 219.107.54.157 | unknown |
| slot-02 | KR | 127.0.0.1:17929 | 221.158.213.241 | unknown |
| slot-03 | RU | 127.0.0.1:17930 | 46.159.102.159 | unknown |

以上是采集时的运行状态，会随志愿者节点变化。三个 namespace 独立，SOCKS actual IP 与对应 expected IP 一致，均不是 VPS 自身公网 IP。俄罗斯 Slot 在多个候选超时后成功，不跨国家自动补位。

## 真实客户端链路

本輪创建内置 Xray 的 VLESS/TCP/REALITY 入站以测试统一订阅。预先选用的 `www.microsoft.com` 目标通过普通 TLS 预检，但真实 REALITY 握手失败。该结果不能算订阅可用。

仅本机监听的临时 Xray 服务对照测试：Microsoft 失败；Tesla 和 Apple 两个目标通过同一条日本 Slot，实际出口为 `219.107.54.157`。临时进程和文件自动清理，公开调试配置已恢复由管理器生成。

目标站点要求参照 [XTLS REALITY 官方说明](https://github.com/XTLS/REALITY)。普通 TLS 1.3 检查不足以证明目标在当前环境下可用于完整 REALITY 链路；每条公开入站仍必须实测。

公开服务目标调整、三个公开入站全链路验证和订阅聚合数量仍等待具体目标调整确认。不得将当前 unknown 节点称为已证实家宽。

## 强杀与自动恢复

仅定位并强杀本项目 Slot 1 的 OpenVPN 进程，没有停止其他出口或 3x-ui。立即进行两次经 `127.0.0.1:17928` 的 HTTPS SOCKS 请求，均返回 curl 97，响应为空，未回落 VPS 公网 IP。随后健康监测自动恢复；Slot 1 仍为 `17928`，出口 `219.107.54.157`，三个 Slot 均重新确认 up。完整故障证据保存在 VPS 项目私有状态目录的 `fault-verification.json`，不含客户端凭据。
