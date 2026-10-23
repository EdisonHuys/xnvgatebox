# Linux 安装兼容性

## 目标系统

本轮统一安装器面向 Linux x86_64、root、systemd：

| 系统 | 软件来源 | 当前状态 |
| --- | --- | --- |
| Debian 12 / 13 | 发行版 APT 仓库 | 自动新装、更新、卸载通过；Debian 12 有此前 VPS 记录 |
| Ubuntu 22.04 / 24.04 | 发行版 APT 仓库（含 Universe） | 自动新装、更新、卸载通过 |
| CentOS Stream 9 / 10 | 系统仓库、EPEL、固定上游 SSTP 源码 | 自动新装、更新、卸载通过；独立 VPS 待验收 |

从 v0.9.0 起使用同一个安装入口，命令见 [README](../README.md#安装)。v0.8.1 及更早安装器仍只支持 Debian / Ubuntu。ARM64、CentOS 7、CentOS Linux 8、OpenVZ 等受限容器和非 systemd 系统不在本轮范围；Rocky/Alma/RHEL 尚未单独验收。

## CentOS 依赖

使用 DNF 安装系统依赖及 EPEL OpenVPN。EPEL 9/10 未提供所需 SSTP 包，安装器下载上游 `sstp-client 1.0.20`，验证 SHA256 后构建本机 RPM，匹配本机 PPP 版本。只安装客户端、库和 PPP 插件，不覆盖系统 PPP 或第三方服务。源码地址和固定摘要在 `vps/install.sh` 中，保留 GPL-2.0-or-later 许可。

本机 RPM 名为 `xnvgatebox-sstp-client`，通过 DNF 管理；与其他共享依赖一样，项目卸载不自动删除它。系统 PPP 大版本升级时应同步重建对应插件。构建依赖包括 gcc、make、autoconf、automake、libtool、OpenSSL/libevent/PPP 开发包和 rpm-build，部分来自 CRB。安装失败保留错误，不能跳过依赖后称为安装成功。

RPM 创建上游所需的 `sstpc` 系统用户与组供连接后降权，已有同名账户不修改；卸载项目不删除共享账户。系统证书使用发行版 CA bundle，不关闭证书或主机名校验。

## 防火墙与隔离

安装器不关闭 SELinux、firewalld 或服务商防火墙。请按实际端口放行管理和接入；管理服务为 root 的独立 systemd 服务。VPN Gate 还需要 TUN、network/mount namespace、veth、iptables NAT/filter 和 IPv4 转发能力。受限 VPS 需要服务商提供这些能力，不能通过安装软件补足。

## 自动验收边界

矩阵使用各发行版原始容器，安装真实依赖、启动 systemd、执行完整包新装与同版本更新，检查随机管理身份、真实订阅 HTTPS 链路、隔离网络规则和完整卸载；CentOS 额外验证本机 RPM 及 PPP 插件加载。

特权容器共享 GitHub runner 内核，不替代真实 VPS 的 SELinux enforcing、firewalld、TUN/PPP 模块、服务商网络与 VPN Gate 正向连接验收。当前没有 CentOS VPS，未将这些项目计为通过。
