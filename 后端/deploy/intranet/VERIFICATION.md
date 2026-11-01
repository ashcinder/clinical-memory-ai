# 内网版本验证记录

验证日期：2026-10-29（北京时间）。分支：`codex/intranet-deployment`。

## 已完成

- `go test ./...`、`go vet ./...` 通过。
- 在单独初始化的 MySQL 实例上执行 `go test -race ./... -count=1` 通过；使用随机测试库，不连接现有业务库。
- 新增回归测试：账户保存搜索配置后保留管理员指定的对话/视觉网关，切换服务使用对应网关；HTTPS 反向代理下 Cookie/Bearer 两类会话的跨端读写、同步版本和过期编辑冲突。
- Android 无默认地址的 Debug/Release 构建通过；指定 HTTPS 默认地址和内部 CA 公钥证书的 Debug/Release 构建通过。
- 验证生成的 Android 配置：内部 CA 仅限精确主机、Release 禁止明文、Debug 可用于本地 HTTP 调试；APK 确实包含 CA。重新以默认参数构建后，旧 CA 从生成目录和 APK 清除，默认服务地址为空。
- Android 非法 HTTP 默认地址、带 `/api` 的默认地址、误传私钥及非 CA 服务器证书均被拒绝。
- Linux amd64 部署包交叉编译成功。核对归档不包含 `.env`、业务 SQL 备份、uploads、账户配置、私钥或 macOS AppleDouble 元数据。
- 在本机用同一构建脚本生成原生运行包，配合**实际 Nginx、临时 CA 签发的 HTTPS 证书、独立 MySQL 和临时附件目录**完成：入口重定向、网段限制（含根路径与 API 的 403）、只读检查脚本、Cookie/Bearer 跨端读写和同步、鉴权附件上传下载、服务重启后数据及登录保留。
- Shell 语法和 `git diff --check` 通过。

## 验证范围

运行环境为 macOS，实际 Nginx/Go/MySQL 链路在本机完成；Linux 包完成交叉编译，未在目标 Linux 服务器执行 systemd 安装或重启验收。Android 验证包括构建、打包及资源内容，未使用真实手机接入目标内网。模型服务测试使用模拟接口，未上传真实资料或调用付费 AI。

部署方仍需填写实际域名、允许网段、数据库配置和证书，验证目标 Wi-Fi/VLAN/DNS、真实手机证书信任、外部 AI 出站、开机自启、备份恢复及容量。Release APK 尚未配置正式签名。多人管理和共享权限不在本次改动范围。

详细本机记录位于已忽略的 `后端/output/intranet-mysql-tests.log`、`intranet-runtime-verification.json`、`intranet-android-verification.json` 及 Android 构建日志；临时测试服务完成后关闭。
