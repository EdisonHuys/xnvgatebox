# xnvgatebox single Worker

这是项目对外的唯一 Cloudflare Worker 入口，提供管理页、订阅、节点检查入口和数据面 WebSocket 入口。`worker/control/worker.mjs` 保存可单元测试的路由实现，本目录只提供唯一发布入口。

Worker 内置节点出口检查和 TCP-only VLESS 数据面，直接通过 Cloudflare Sockets 连接 VPN Gate SSTP 节点，不转发到其他 Worker。部署只需要这个 Worker 和一个 KV Namespace。

生产部署只绑定一个 KV Namespace（键 `manifest:current`）。订阅节点的服务器地址、TLS SNI、WebSocket Host 和入口端口跟随 `/sub` 请求的 HTTPS 域名与端口。用户通过自定义域名订阅，就使用该域名连接；不受旧 `PUBLIC_HOST` 或 manifest 主机名影响。

GitHub 仓库 Variables 中设置 `WORKER_URL` 为准备使用的 HTTPS 订阅入口（只填 origin，不含 `/sub` 或 token）。Actions 从这一处配置派生检查与数据面地址。改用新域名时更新这一变量，再以 `serverless`、`publish_cf=true` 运行验证。
