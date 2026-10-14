# xnvgatebox VPS Standalone

当前支持 Debian / Ubuntu、Linux amd64、systemd。管理进程、内置 Xray、共享候选池都在 VPS 本地运行。已有 3x-ui 不会被卸载或修改数据库。

## 安装与升级

```sh
curl -fsSL https://raw.githubusercontent.com/hanhaoops/xnvgatebox/main/vps/install.sh | bash
```

默认从本项目 GitHub Release 下载完整包和 SHA256 清单，不下载上游项目的安装脚本或管理程序。包内 Xray 固定为 `config/xray-release.json` 的版本，并在构建时验证官方归档 SHA256。下载失败或初次构建候选池失败时不会停止现有管理进程。

本地包安装：`bash vps/install.sh --mode vps --package /path/xnvgatebox-linux-amd64.tar.gz`，相邻目录必须有 `checksums.txt`。更新前在 `/var/backups/xnvgatebox/` 备份旧二进制、服务环境和项目状态。独立管理采用 native 后端；保留旧管理路径、密码和订阅 token。原 3x-ui 入站不会自动转成内置 Xray 入站，需在管理页新建并绑定。

## 管理和订阅

`xnvgatebox access` 打印管理地址格式和密码。管理 UI 默认直接监听 VPS `8899`，使用随机访问路径和登录密码；SOCKS 仅在 `127.0.0.1` 监听。

1. 进入“新建出口”，选择国家和数量，总上限三个。地区没有可用节点会失败，不使用其他国家补齐。
2. 在“新建节点”选择 VLESS、TCP、REALITY 和未被占用的公开端口；目标站点须支持 TLS 1.3。
3. 绑定节点到对应出口。每个公开入站端口对应一个本地 SOCKS Slot。
4. 点击“订阅”，复制一个链接到客户端；添加节点和出口绑定后刷新订阅即可。
5. “导出 Xray 配置”下载出站和 routing 片段，模板 inbound 标签为 `inbound-slot-01` 等，接入外部 Xray 时替换为实际标签。导出含本地 SOCKS 认证，仅供管理员使用。

节点替换保持 `slot-01`、`127.0.0.1:17928` 和该入站端口不变。第二、第三 Slot 分别为 `17929`、`17930`。默认出站为 blackhole，未绑定、失效或无匹配规则的请求不能直连母机公网。

## 本地刷新

`xnvgatebox-pool.timer` 每十分钟运行同一个 Python Pool Builder，输出 `/var/lib/xnvgatebox/pool/node_pool.json`；管理进程每三十秒检查新快照。`xnvgatebox refresh` 手动立即执行，界面的“刷新节点池”只重新读取本地快照。采集失败保留上一份完整快照。国家等来源配置在 `/etc/xnvgatebox/pool.json`。

没有 GitHub Actions、Cloudflare、远程 node_pool 也可独立运行。抓取阶段全量发现，昂贵协议探测另行限额；配置存在不表示节点可用。

## 验收与限制

先通过 Slot 的两个 HTTPS expected 探测，再通过 SOCKS 检查 actual 出口并排除母机 IP。客户端全链路仍需实际连接对应公开 Xray 入站确认。类型未知显示 unknown，技术连通不能等同于住宅类型证明。ISP/ASN 画像和策略持久化仍待后续开发。

源码中保留原 MIT 许可证和来源记录于 `fanout/UPSTREAM.md`。内部旧目录、兼容环境变量和服务名暂保留，用户入口统一叫 xnvgatebox。
