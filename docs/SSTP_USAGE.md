# 原始 SSTP 的使用与参考产物格式

2026-10-18 核对。这里的协议名称是 **SSTP**。本项目“原始 SSTP”导出的是 VPN Gate 连接参数及 VPS 观测，不是 FTP 文件服务，也不是自动生成的 VLESS 订阅。

## 本项目导出的内容

文本一行一个连接参数，示意：

```text
sstp://vpn:vpn@vpn-example.opengw.net:443
```

这是参数表示法，并不保证每个客户端都支持导入这个 URI。JSON 的 `kind` 是 `vpngate_vps_sstp_observations`，包含服务器域名、检测所用 IP、通过的端口、用户名/口令、观测时间、有效期和结果。只导出通过且仍有效的记录；VPS 测得可用不等于用户本地或 CF 当前也可用。

## 直接连接的方法

在支持 SSTP 的客户端中使用复制的参数。Windows 可新建系统 VPN 连接，使用内置提供程序，VPN 类型选择 SSTP，服务器填写导出的 `域名:端口`，用户名和密码均为 `vpn`。如果端口是 443，可省略后缀；其他端口保留。服务器须使用 `*.opengw.net` 等官方域名正常验证证书，不能把导出中的检测用 IP 直接替换为服务器名。

连接关系是 `使用端 → VPN Gate SSTP`。保存参数后，数据不经过本 VPS，VPS 离线也不会因此中断这个独立连接；但 VPN Gate 节点仍可能下线或地址变化。使用端的 DNS、网络限制及协议支持也会影响连接。

依据：[VPN Gate 官方 SSTP 指南](https://www.vpngate.net/cn/howto_sstp.aspx)。本文未把官方操作指南当作本轮 Windows GUI 拨号实测。

v2rayN 的本项目订阅使用 VLESS；不能把上述 `sstp://...` 文本当成现有 VLESS 订阅。要在目前的 v2rayN 链路中使用 VPN Gate，可继续使用本项目 VPS 已转换的入口。若将原始 SSTP 交给外部 Worker，则需要该 Worker 实现 SSTP/PPP 转发，并按其配置格式填写；普通 Worker 或仅做检测的 Worker 不会自动变成 VLESS 转发服务。

## 先前参考项目的 Actions 输出

只读核对当前源码及公开产物，没有运行它们的工作流，也没有复制其实现。

| 项目 | Actions 与生成产物 | 如何消费 |
| --- | --- | --- |
| `hezhanleiok/gate`，提交 `4ff9b273cba0c02fb413444f1a515f2dd0e75cb5` | `check.yml` 调用 `vpngate.py`，输出 `public/data.json`、`public/index.html`、`public/nodes.txt`，再将整个 public 发布到 Pages | nodes.txt 每行类似 `CF入口:443#节点名称$sstp://vpn:vpn@服务器域名:端口`，用于懂这种格式的 Worker/订阅生成器；不是单独的标准 VLESS 链接 |
| `xibabro/CF-vpngate`，提交 `a53b8c1d11aff978d623d493df8479b5e9f3f923` | 维护 CSV，检测后输出 `vpngate-v2ray.txt`（每行 VLESS）、`vpngate-clash.yaml`，另有 XHTTP 文件，并提交回仓库 | VLESS 使用 TLS/WebSocket，路径内包含 URL 编码的 `/fdip=sstp://vpn:vpn@域名:端口?ed=2560`，仍需其配套 Worker 的域名、入口及认证配置 |

固定来源：[gate 产物生成](https://github.com/hezhanleiok/gate/blob/4ff9b273cba0c02fb413444f1a515f2dd0e75cb5/vpngate.py)、[gate 工作流](https://github.com/hezhanleiok/gate/blob/4ff9b273cba0c02fb413444f1a515f2dd0e75cb5/.github/workflows/check.yml)、[CF-vpngate 工作流](https://github.com/xibabro/CF-vpngate/blob/a53b8c1d11aff978d623d493df8479b5e9f3f923/.github/workflows/sync.yml)、[CF-vpngate 链接生成](https://github.com/xibabro/CF-vpngate/blob/a53b8c1d11aff978d623d493df8479b5e9f3f923/vpngate-daily.py)。

核对时公开 `vpngate-v2ray.txt` 确为逐行 VLESS，首条的传输为 WebSocket、TLS，路径嵌入 SSTP。文件数量、上次成功运行和生成链接都不证明全部节点当前仍可用。Gate 的类型命名规则也不作为本项目的住宅证据；unknown 仍是 unknown。

GitHub 在这些链路中负责调度、保存/发布文件；持续转发由客户端、Worker 和 VPN Gate 承担，Actions runner 不常驻代理。我们的 VPS 已可承担本地抓取、检测和私有清单导出，但没有自动发布兼容上述 Worker 的清单或创建 CF VLESS 订阅。本轮 CF 继续暂停，不部署、不接入其公开服务。
