# 客户端验证记录

## 2026-10-14：Shadowrocket 延迟测试全部超时

### 已确认

- 唯一 Worker 为 `xnvgatebox.waynee.workers.dev`，管理页能加载清单，订阅能被 Shadowrocket 导入。
- 本次恢复任务 [38583639255](https://github.com/hanhaoops/xnvgatebox/actions/runs/38583639255) 完成真实链路验证并成功发布 KV：50 个候选，43 个 SSTP 通过，29 个全链路通过，检查服务错误为 0。
- 恢复后线上 `/health` 实测 `configured=true`、`manifest_ready=true`、`nodes=29`。
- 清单原有效期为 60 分钟，而 GitHub 实际定时运行间隔出现超过 5 小时，导致此前 `manifest_unavailable`。提交 `b319352` 将定时尝试改为每 20 分钟，不放宽过期检查；GitHub 调度仍不能保证准时执行。
- 用户手机使用中国大陆 Wi-Fi。初始截图为 Shadowrocket 延迟测试全部超时；随后用户实际开启代理测试网页，并报告“打不开”。因此问题不仅是测速。
- 当前实现和已验证范围为 TCP/IPv4/WebSocket。不能据延迟测试超时就断定节点全不可用，也不能据 GitHub 验证成功就断定手机网络可用。
- 本机解析返回 `198.18.0.0/15` 中的地址，表明存在 Fake-IP/代理网络路径；本机测试不得标为中国大陆手机直连结果。本机 Python TLS 探测成功过，curl 也出现过 TLS 连接错误，尚不能定位手机失败阶段。
- 节点 ASN/ISP 尚有空值，不能认定所有节点是家宽。客户端显示的国家图标也不能替代出口 IP 验证。

### 用户切换前的操作与恢复方式

1. 在 Shadowrocket 选择一个节点，开启代理，切到全局模式。
2. 访问 `https://api.ipify.org`，记录显示的 IP 或具体错误。
3. 若断网，关闭 Shadowrocket 后回到聊天；无需修改节点参数。
4. 如果能显示 IP，比较管理页该节点的 `expected_exit_ip`。再用 `https://ipv4.icanhazip.com` 交叉核对。

### 下一次继续的位置

- 首先获取上面实际网页测试的结果，不要重复询问已确定的网络与测速情况。
- 若网页正常：区分延迟测试类型与超时阈值，继续验证实际出口，不因测速失败更换架构。
- 若网页也失败：核对 Shadowrocket 节点的 TLS、SNI、WebSocket Host/Path、端口、early data 与导出的 VLESS 配置；用户截图需遮盖 UUID/订阅 token。
- 比较 Worker 实际 `/sub` 导出的配置和 Actions 验活配置，而不只检查 Python 本地生成的链接。
- 本机 Xray 真实复测已完成：`public-vpn-78.opengw.net:443` 经唯一 Worker，四次 HTTPS 出口 IP 均为 `219.100.37.236`，内容检查通过，整轮耗时约 30230ms。该机已有代理/Fake-IP 环境，因此结果不证明手机能直接连接 Worker。
- 当前等待用户确认：手机关闭 Shadowrocket 后是否能访问 Worker `/health`；导入节点是否开启 TLS、端口是否为 443。先根据这两项定位，不把 30 秒整轮耗时等同于单次连接耗时。
- 不要未经诊断就改 UUID、关闭 TLS 校验、放宽验证有效期或新增必需 Worker。

本记录不包含管理员 token、订阅 token 或 UUID。

### 后续定位：订阅入口仍为旧域名

用户确认手机直连 workers.dev 不可达，自定义域名可打开；TLS 已开启，端口443。最终核对发现节点详情仍为旧域名。用户要求跟随订阅请求域名，不在代码中写死域名。正在将 Worker 导出入口改为当前 HTTPS 请求 origin，Actions 入口改为唯一仓库变量 WORKER_URL。后续截图已有一个节点显示242ms，但实际网页成功尚未得到确认。

### 自动跟随域名修复与线上阻塞

- 修复提交 `88d4164`：Worker `/sub` 使用本次 HTTPS 请求域名与端口，同时更新 VLESS 服务器地址、TLS SNI、WebSocket Host。旧 PUBLIC_HOST 和 manifest 地址不覆盖请求域名。Actions 从仓库变量 WORKER_URL 派生检查与数据面入口，源代码和工作流不写死用户部署域名。
- 本地 Node 路由与边界测试通过；34 项 Python 测试全部通过（真实 Xray 进程测试需要允许回环监听）。
- 修复版已部署到唯一 Worker，版本 `61890fe9-5205-43c9-be99-a0b5a38fb7cb`。原有普通变量、唯一 MANIFEST KV 和自定义域名均保留。
- 线上复测自定义域名 `/health` 和 GitHub `/check` 均返回 522。GitHub 运行 `38589682095`、`38589795544` 未发布新清单；手机也确认 522 或打不开。
- 旧版本 `fea14ac6` 做对照也返回 522；已恢复修复版本到 100% 流量。刷新现有自定义域名绑定成功，但没有消除 522。
- Cloudflare API 显示域名 enabled=true、服务为同一 Worker；zone active、paused=false，权威 NS 与公网一致，zone 中没有 Worker 路由。这些状态不证明外部请求已成功到达应用。
- 与该 Worker 相关的 Cloudflare Trust & Safety 邮件显示：2026-10-14 23:28（北京时间）收到 Network abuse 报告，编号 `9b250b0a1159afa9`。邮件未明确说明停用范围，也未提供具体日志。通知与 522 时间接近，平台限制是待确认原因，不能当作已证实的停用结论。
- 修复版的线上真实订阅与代理验收尚未通过，不能宣称已恢复可用。后续应先确认 Cloudflare 通知与资源运行限制，再验证 `/health`、实际 `/sub` 和 HTTPS 出口。
