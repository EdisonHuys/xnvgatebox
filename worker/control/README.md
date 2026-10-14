# xnvgatebox Worker route module

这里保存唯一 `xnvgatebox` Worker 的可测试路由实现。公开发布入口在 [`../xnvgatebox/`](../xnvgatebox/)；管理、订阅、检查入口和数据面 WebSocket 入口都由同一个脚本处理。它只读取 GitHub Actions 发布到 KV 的 `manifest:current`，因此不会接收任意 `proxyip`、SOCKS5 地址或未经验证的节点。

接口：

- `GET /health`：返回配置、模块状态和当前 manifest 是否可用，不返回凭证。
- `GET /admin`：手工管理页面。管理员 token 只通过 `Authorization: Bearer ...` 请求 `/api/status`。
- `GET /api/status`：管理员查看节点池和有效期。
- `GET /api/manifest`：管理员导出当前无凭证 manifest。
- `GET /sub?token=...`：返回 Base64 编码的 VLESS 订阅。token 不写入页面和 manifest。
- `GET /check?proxy=sstp://...`：在本 Worker 内完成 SSTP/PPP/TCP 出口检查，并要求 `CHECKER_TOKEN`。
- `GET /?sstp=...&globalproxy=1` + WebSocket：在本 Worker 内完成 TCP-only VLESS 到 SSTP 的转发，严格限制节点主机名。

Worker 普通运行变量：

```text
VLESS_UUID       # UUID v4
ADMIN_TOKEN      # 管理接口 token
SUB_TOKEN        # 订阅 token
VPN_USERNAME     # 可选，默认 vpn
VPN_PASSWORD     # 可选，默认 vpn
DATA_PLANE_BASE_PATH # 可选；覆盖 manifest.data_plane.base_path
CHECKER_TOKEN    # /check 管理鉴权；未设置时使用 VLESS_UUID
```

订阅入口自动跟随访问 `/sub` 的 HTTPS 域名与端口，服务器地址、TLS SNI、WebSocket Host 始终一致。旧 `PUBLIC_HOST`、`DATA_PLANE_HOST`、`DATA_PLANE_PORT` 和 manifest 中的入口地址不再覆盖订阅请求地址。GitHub 仓库 Variables 使用唯一 `WORKER_URL` 配置验活入口，不在工作流中写死部署域名。

`MANIFEST` 是 KV namespace binding，固定读取键 `manifest:current`。没有 KV 时可以在本地开发用 `MANIFEST_JSON`，生产环境不要把 manifest JSON 塞进源码或公开变量。

```sh
cp wrangler.toml.example wrangler.toml
# 将示例中的占位值替换为普通变量后部署
npx wrangler deploy
```

GitHub Actions 使用 `scripts/publish_cf_manifest.py` 通过 Cloudflare API 原子替换同一 KV 键。需要仓库 Secrets：`CF_API_TOKEN`、`CF_ACCOUNT_ID`、`CF_KV_NAMESPACE_ID`。发布步骤默认关闭，手工运行时选择 `publish_cf=true` 才会写入 KV。

统一 Worker 的 `/check` 接口兼容 `CHECKER_TOKEN`，但 GitHub Actions 默认直接使用已有的 `VLESS_UUID` 作为检查凭据，避免单独的 `CHECKER_TOKEN` 在 Worker 重部署后失配。

当前版本已移除 `LEGACY_CHECKER_URL` 和 `LEGACY_EDGE_URL`。节点检查和 TCP-only 数据面在本 Worker 内执行，不依赖其他已部署的 Worker。运行时参数使用普通变量；示例文件只放占位值，真实值由部署入口生成。
