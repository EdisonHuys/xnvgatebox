# 第三方依赖与参考边界

本轮 Python 解析器、分类器、HTTP 适配、订阅生成、Xray 测试编排、测试和工作流为本项目新编写的实现，按 GPL-3.0-only 发布，完整许可见 [LICENSE](LICENSE)。没有复制 gate、CF-vpngate、AimiliVPN 的实现代码。

2026-10-08 增加独立 Worker 部署准备工具。`scripts/prepare_workers.py` 校验固定提交与 SHA256，将两份上游程序分别放到忽略的 `runtime/deploy/checker`、`runtime/deploy/edge`，保留原文件、完整 LICENSE、来源、修改说明和生成文件摘要。两个程序不合并。Checker 的新增入口按 GPL-3.0-only；独立 EdgeTunnel 的新增入口按 GPL-2.0-only，完整许可见 [worker/edge/LICENSE](worker/edge/LICENSE)。根目录 GPL v3 不覆盖该明确标注 GPL v2 的独立组件。部署准备不代表 Cloudflare 部署或真实链路验收完成。

| 组件 | 本轮使用方式 | 许可 / 来源 |
| --- | --- | --- |
| VPN Gate 官方 CSV | 运行时读取公开节点与配置数据；配置作为未执行的数据保存 | [官方入口](https://www.vpngate.net/en/)，使用范围依官方项目说明 |
| sstp-client | CentOS 安装时从固定版本 1.0.20 源码构建本机 RPM，匹配系统 PPP；独立进程运行，不修改上游源码 | [上游](https://gitlab.com/sstp-project/sstp-client/-/tree/1.0.20)、GPL-2.0-or-later；安装保留 COPYING，归档 SHA256 固定于安装脚本 |
| CheckSocks5 | 独立部署准备；新增认证与 SSTP API 边界；调用其 HTTP 接口 | GPL v3；固定提交 `2f31cf9a242444eb22905d888d4d8871b0625b3a`，保留完整 LICENSE 和 README |
| EdgeTunnel | 独立部署准备；强制 SSTP/globalproxy、关闭管理页面和 UDP；生成对应 VLESS/WS 参数 | GPL v2；固定提交 `af4f9837e1843e34159018713bc8749ccec3004d`，保留完整 LICENSE 和上游源文件；不与 GPL v3 Checker 合并 |
| Xray-core | 下载官方 v26.3.27 二进制，按归档 SHA256 验证后本地执行；二进制不纳入 Git | [官方发布](https://github.com/XTLS/Xray-core/releases/tag/v26.3.27)、[MPL-2.0](https://github.com/XTLS/Xray-core/blob/v26.3.27/LICENSE)；安装器保留归档内 LICENSE / README |
| GitHub 官方 Actions | 固定提交引用 checkout、setup-python、upload-artifact | 官方仓库许可；固定提交写在工作流中 |
| fanout | `vps/fanout/` 作为 v0.2 Multi-Exit 底座；读取本项目共享 `node_pool.json`，并保留完整上游版权与许可 | MIT；上游提交 `d7ce5224caf3469b19876982abdf8d1be16c4a8b`；修改说明见 [`vps/fanout/UPSTREAM.md`](vps/fanout/UPSTREAM.md) |

完整的五项目许可证证据和源码行为见 [REFERENCE_AUDIT.md](docs/REFERENCE_AUDIT.md)。GPL 许可正文是标准许可文本，不是 Checker 实现代码的复用。
