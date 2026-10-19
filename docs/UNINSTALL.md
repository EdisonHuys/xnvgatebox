# 卸载与旧版清理

v0.8.0 起支持正式卸载命令。在 VPS 以 root 执行；普通命令默认保留 `/var/backups/xnvgatebox` 中已有备份，程序、配置、管理密码、订阅凭据及检测记录会删除。执行前列出路径，并要求输入 `yes` 确认。

先预览彻底清理范围：

```sh
xnvgatebox uninstall --dry-run --purge --legacy
```

确认后卸载当前项目、已有备份及可确认归属的默认旧 fanout：

```sh
xnvgatebox uninstall --purge --legacy
```

只卸载当前项目并保留备份，用 `xnvgatebox uninstall`。`--purge` 同时永久删除本项目备份；`--legacy` 额外清理标准旧版的 `/var/lib/fanout`、`fanout.service`、`/usr/local/bin/fanout`，以及具有旧菜单签名的 `/usr/local/bin/f`。无关的 `f` 工具保留。自动化需明确添加 `--yes`，不接受隐式管道确认。

## 旧版本或卸载中断后

v0.8.0 可能在正常停止服务后执行多余的 `systemctl reset-failed`，导致卸载中止。该错误发生在文件和备份删除之前，但可能已停用部分服务；无需重新安装或重装系统，重新下载 v0.8.1 或更新的独立工具重试即可。新版仅对失败状态进行重置，并复核结果，真正的系统操作失败仍中止。

v0.7.0 及更早版本没有该命令。可以直接下载固定版本、独立运行的卸载工具，无需先升级或启动代理：

```sh
curl -fsSL https://raw.githubusercontent.com/hanhaoops/xnvgatebox/v0.8.1/vps/deploy/uninstall.py -o /tmp/xnvgatebox-uninstall.py
python3 /tmp/xnvgatebox-uninstall.py --dry-run --purge --legacy
python3 /tmp/xnvgatebox-uninstall.py --purge --legacy
```

临时下载文件可以在完成后自行删除。重新安装使用 [安装命令](../README.md#debian-12-安装)，会生成新的管理身份和订阅；旧客户端订阅需替换。使用旧备份恢复属于迁移，不能算全新安装验收。

## 清理和保留的边界

清理标准 systemd 安装的三个单元、程序与两个命令、环境配置、项目数据和部署状态。网络清理同时核对实例名称、持久网段、出口/检测资源记录和实际地址；按具体规则删除 NAT 和转发项、namespace、veth 及其专用 DNS 目录，不刷新全局防火墙。

3x-ui、其他实例、其他 namespace/接口/规则、系统共享依赖、全局 IP 转发设置及系统日志保留。部署协调锁由系统管理，保留到系统清理，避免卸载与安装间的竞争。旧版安装没有记录修改前的全局 IP 转发值，因此不猜测原值。许可证和上游来源说明继续保留在源码中；过时的独立安装脚本、f 菜单及服务示例已从当前仓库移除。

安装/更新共用部署锁。未完成部署、活动更新、实例仍运行、服务停止失败、符号链接/挂载点、网段冲突或缺少归属证据时中止，不能报告卸载成功。中止可能已停止部分本项目服务；核对原因后可用独立工具重试。文件删除阶段失败时也如实报未完成，不承诺自动回滚删除的数据。

自动清理只支持标准目录和标准 systemd 单元。自定义工作目录、服务参数/覆盖配置及旧 OpenRC/SysV 服务须先核对归属；工具会保留现场，不能按名称批量强删。任意手工下载目录、用户另存的归档和共享系统日志不属于本项目受管路径。

本轮只发布工具及 GitHub 完整包；隔离 Linux 测试不等于已在用户 VPS 执行卸载。CF 保持暂停，unknown 不称家宽。
