# 开发与测试

## 本机候选池

Python 3.9+，核心使用标准库。候选构建不执行 OpenVPN 配置，也不证明节点连通：

```sh
python3 scripts/toolbox.py pool
```

生成 `data/node_pool.json`、摘要和 `data/configs/*.ovpn`；可通过 `--csv-file /absolute/path/vpngate.csv` 解析官方格式快照。配置见 `config/settings.json`。真实验活另需 curl、Xray 和对应协议依赖。

## 回归测试

```sh
python3 scripts/install_xray.py
python3 -m unittest discover -s tests -v
node tests/test_worker_guards.mjs
node tests/test_cf_control.mjs
node tests/test_console_sstp.mjs
node tests/test_console_third_party.mjs
node tests/test_console_flow.mjs
node tests/test_console_automation.mjs
```

Python 检查解析、证据一致性、异常响应、证书拒绝、订阅清理、私密性、锁和进程清理。Node 检查控制台交互和保留的 Worker 边界。Linux Go 测试包含网络隔离、出口生命周期、阻断及并发检查。合成测试与真实 VPN 节点验收分别记录，不因前者通过而认定后者可用。

## GitHub Actions

- `test.yml`：Python 3.9/3.12 与 Node 回归。
- `vps.yml`：Linux Go/race、卸载验证、完整包构建及多发行版安装矩阵。
- 手动运行 `vps.yml` 时可选 `live_mirror`，在一次性 Linux 环境运行生产 Builder，自行请求公开镜像索引与固定提交 CSV；不发布候选、不连接 VPN、不替代用户 VPS 验收。
- `release-vps.yml`：手动发布通过测试的完整包，核对来源、版本与文件摘要。
- `update.yml`：仅手动构建共享候选池，不定时运行、不部署 Cloudflare。

多发行版测试在一次性特权容器中运行真正的 systemd、发行版包管理器和安装器，执行安装、实际订阅 HTTPS 访问、更新保留与卸载。测试入口仅允许在 GitHub 一次性 runner 上运行；它不覆盖独立 VPS 的内核、SELinux 或服务商防火墙。范围见 [系统兼容性](LINUX_SUPPORT.md)。

周期任务用受控时间和临时状态验证默认 60 分钟、最低 30 分钟、开关、失败退避、重启保留与手动任务互斥；Python 和 Go 共享刷新状态，避免系统定时器重复请求来源。测试不向 VPN Gate 做高频检测。

Clash 输出用固定官方 Mihomo 测试内核验证配置；Debian 12 隔离安装测试还使用真实订阅连接本机 VLESS/REALITY，校验 HTTPS 出口与内容，并确认选中代理失效时不回落 DIRECT。测试内核仅下载到 CI，不进入产品安装包。GUI 版本未提供，不将内核测试视为所有 Clash Verge 版本已验收。

## 记录与参考

[开发记录](DEVELOPMENT.md)、[Mac 交接](MAC_HANDOFF.md)、[升级与故障验收](VPS_UPGRADE_VALIDATION.md)、[早期独立部署验收](VPS_STANDALONE_VALIDATION.md) 和 [初轮记录](ROUND1.md) 保留各版本的证据与环境限制。历史 Cloudflare 设计不代表当前部署方式，上游节点实测也不保证持续在线。

接口资料：[VPN Gate](https://www.vpngate.net/en/)、[Xray](https://xtls.github.io/en/config/transport.html)、[ipify](https://www.ipify.org/)、[icanhaz](https://github.com/major/icanhaz)、[IPLocate](https://www.iplocate.io/docs/getting-started/authentication)。
