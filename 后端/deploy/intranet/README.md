# 内网部署版本

目标：Android App 与电脑浏览器访问同一台内网服务器，后端可按配置访问外部 AI。手机无需直接连接外部 AI。数据库、附件存放在内网，启用外部模型时相关查询和材料仍会发送给模型服务。

本分支的测试结果与待验范围见 [验证记录](VERIFICATION.md)。

本文使用以下**示例**，安装前替换为实际值：域名 `linji.internal.example`、客户端网段 `192.168.10.0/24`、服务器地址 `192.168.10.20`。不要直接沿用示例域名或网段。固定 IP 或 DHCP 地址保留、内网 DNS 与 TLS 证书由网络管理员准备。

## 架构与条件

```text
电脑浏览器 ─┐
           ├─ 内网 HTTPS:443 → Nginx → Go 127.0.0.1:18090 → MySQL 127.0.0.1:3306
Android ───┘                               ├─ /var/lib/clinical-memory
                                           └─ 外部 AI HTTPS（可选）
```

- Linux 服务器或虚拟机，systemd、Nginx、MySQL 8.0+；已有 MySQL 可复用专用业务库。本文默认三者在同机。源码构建需要 Go 1.24+，运行预编译包不需要 Go、Node、Python 或 Android SDK。
- 4 核、8GB、SSD 可作为少量用户试运行起点，不代表已测得的并发容量。磁盘按附件总量、索引和备份保留量规划；外部 AI 不要求 GPU。
- 服务器应持续运行且不休眠，建议有线接入。手机 Wi-Fi 与电脑网段须能访问服务器，访客隔离和 VLAN ACL 需由管理员配置；外网手机需要单位 VPN。
- 入站仅向批准的内网/VPN 网段开放 443；不要把 3306、18090 或路由器端口映射到公网。Nginx 模板另有网段限制，不能替代服务器和网络防火墙。
- 若使用外部 AI，服务器需有 DNS、受信任 CA、正确时间以及到所选平台的 HTTPS 出站能力；浏览器和 App 无需具备该出站能力。
- 当前仍是首个账户初始化、账户私有资料模型。本人手机与网页使用同一账户即可共用资料；本分支不新增多人开户、角色和共享资料库。

## 1. 在开发机生成部署包

在源码项目根目录执行：

```bash
bash 后端/deploy/intranet/build-bundle.sh
# ARM64 Linux 服务器使用：
TARGET_ARCH=arm64 bash 后端/deploy/intranet/build-bundle.sh
```

输出在已忽略的 `后端/output/intranet/clinical-memory-intranet-linux-amd64.tar.gz`（或 arm64）。重建时指定新输出目录，脚本拒绝覆盖已有包。包含 Go server/backup/db-info 二进制、Web、建表 SQL、内网模板和本说明；按白名单打包，**不包含现有数据库、附件、.env、模型密钥、证书私钥或本地模型**。

依赖下载发生在构建机。把与服务器 OS/CPU 架构匹配的包传到服务器，无需在服务器下载 Go 依赖。部署包内不含源码，不能在包内重新运行构建脚本。

## 2. 安装到独立目录

以下为首次安装示例，由管理员在服务器操作。已有同名用户、目录、配置或服务时先核对，不覆盖已有部署。

```bash
sudo useradd --system --user-group --home-dir /var/lib/clinical-memory --shell /usr/sbin/nologin clinicalmemory
sudo install -d -m 0755 /srv/clinical-memory/releases
sudo install -d -m 0750 -o clinicalmemory -g clinicalmemory /var/lib/clinical-memory
sudo install -d -m 0700 /etc/clinical-memory /etc/clinical-memory/tls
sudo tar -xzf clinical-memory-intranet-linux-amd64.tar.gz -C /srv/clinical-memory/releases
sudo ln -s /srv/clinical-memory/releases/clinical-memory-intranet-linux-amd64 /srv/clinical-memory/current
sudo chown -R root:root /srv/clinical-memory/releases/clinical-memory-intranet-linux-amd64
sudo chmod -R go-w /srv/clinical-memory/releases/clinical-memory-intranet-linux-amd64
sudo install -m 0600 /srv/clinical-memory/current/后端/deploy/intranet/clinical-memory.env.example /etc/clinical-memory/clinical-memory.env
sudoedit /etc/clinical-memory/clinical-memory.env
```

填写实际 `DB_*`。配置由 systemd 的 EnvironmentFile 读取，不使用 `export` 或 `$变量` 展开，勿直接 `source`。代码目录保持只读，数据放在 DATA_DIR，配置独立保存。Go 启动会创建 uploads 和必要表，数据目录必须可写。

## 3. 数据库：新建或迁移二选一

**全新使用：**通过服务器上的 MySQL 管理工具建立 `clinical_memory`（utf8mb4）以及 `clinicalmemory@localhost` 专用应用账户，权限限定该库：SELECT、INSERT、UPDATE、DELETE、CREATE、INDEX、ALTER、REFERENCES。设置强密码并写入上面的配置，不使用 MySQL root 运行应用。MySQL 仅绑定本机；由管理员确认 localhost 账户与配置中的 TCP/socket 连接匹配。表由 Go 启动创建，无需手工导入空表 SQL。首次启动后由本人通过内网入口创建首个软件账户。

**迁移已有数据：**在旧机停止后端，运行原项目的 `后端/backup.sh`，将 SQL 与 uploads 恢复到专用目标数据库/数据目录，不覆盖其他业务库。迁移现有库时不要另外初始化账户。

- 恢复 SQL 与原始 uploads，核对笔记、附件数量与原件。
- 按需单独安全迁移 `DATA_DIR/ai-settings/`；保持原账户 ID 不变。普通备份没有这些文件。目录归 clinicalmemory，目录 0700、配置文件 0600；也可恢复后重新填写模型设置。
- 不要直接复制 MySQL 正在使用的引擎目录，也不要把旧 `.env` 原样覆盖服务器部署配置。
- 所有数据恢复后，把 DATA_DIR 内需要应用写入的文件归属设为 clinicalmemory；未经过核对不删除旧机资料。

## 4. 内网 HTTPS 与 Nginx

准备域名匹配的服务器证书及完整中间证书链，将证书放到 `/etc/clinical-memory/tls/fullchain.pem`，服务器私钥放到 `/etc/clinical-memory/tls/privkey.pem`（root 0600）。可以使用单位内部 CA，也可以使用客户端已信任的 CA 签发证书；DNS 和证书中的域名必须一致。内部 CA 的根证书需导入电脑的受信任根证书存储，手机 App 的配置见下文。**CA 私钥与服务器私钥都不进入 App 或部署包。**

```bash
# Debian/Ubuntu 的 sites-enabled 示例；其他系统按 Nginx http {} 的实际 include 路径安装。
sudo install -m 0644 /srv/clinical-memory/current/后端/deploy/intranet/clinical-memory.nginx.conf /etc/nginx/sites-available/clinical-memory-intranet
sudoedit /etc/nginx/sites-available/clinical-memory-intranet
sudo ln -s /etc/nginx/sites-available/clinical-memory-intranet /etc/nginx/sites-enabled/clinical-memory-intranet
sudo nginx -t
sudo systemctl reload nginx
```

修改 `server_name`、证书路径和所有 `allow` 规则；实际网段未加入时返回 403。模板仅监听 IPv4，使用 IPv6 时需明确增加监听、防火墙和允许网段。若前面还有负载均衡代理，需由管理员配置可信代理链和访问控制，不直接照用该模板。

Nginx 保留 `/clinical-memory` 前缀，后端 `PUBLIC_BASE_PATH` 必须与其一致。不要把 proxy_pass 改成带尾随 `/` 的形式。Web 与 API 同源，因此无需增加 CORS。模板不使用公网 ACME 校验；内部证书续期由单位证书系统或管理员安排，不沿用历史公网部署的停 Nginx 续期任务。

## 5. 启动和访问

```bash
sudo install -m 0644 /srv/clinical-memory/current/后端/deploy/intranet/clinical-memory.service /etc/systemd/system/clinical-memory.service
sudo systemctl daemon-reload
sudo systemctl enable --now clinical-memory
sudo systemctl status clinical-memory
sudo journalctl -u clinical-memory -n 80 --no-pager
```

MySQL 应先启动并启用开机自启。本服务在连接失败后自动重试，未绑定发行版特有的 mysql/mysqld/mariadb 服务名。

- 电脑浏览器：`https://linji.internal.example/clinical-memory/`。
- App 连接页：`https://linji.internal.example/clinical-memory`，**不加 `/api`**。升级安装会保留旧服务地址，需手工切换。切换地址会清除旧登录令牌，重新登录即可。
- 同一账户共用笔记、材料、对话和模型设置；前台空闲时约每 5 秒检查更新。正在编辑时暂缓刷新。离开内网后无法继续读写，不提供离线同步。
- 手机的 `127.0.0.1` 指手机自身，不能用于访问内网服务器。`10.0.2.2` 仅供 Android 模拟器连接宿主开发机。

## 6. 构建 Android 内网版本

默认 App 不预填公网地址，用户可手工输入内网入口。也可在源码项目的 `前端APP/` 中构建带默认地址的版本：

```bash
./gradlew assembleDebug -PlinjiServerUrl=https://linji.internal.example/clinical-memory
```

内部 CA 场景额外指定 **CA 公钥证书文件**（PEM 或 DER，可为 CA 证书链；相对路径基于前端APP目录）：

```bash
./gradlew assembleDebug \
  -PlinjiServerUrl=https://linji.internal.example/clinical-memory \
  -PlinjiCaCertificate=/absolute/path/to/organization-root-ca.pem
```

该 CA 只用于默认地址对应的精确主机，不自动信任其他内网域名或用户安装的 CA。仍校验证书有效期、主机名和证书链。更换 App 中的地址为另一内部 CA 主机时需重新构建对应配置。发布时使用相同参数执行 `assembleRelease`，并按单位要求配置正式签名；当前仓库未内置签名密钥，生成的 unsigned release APK 不能直接安装。Debug 仅用于验证：允许 HTTP，且 Android debug-overrides 支持调试 CA；Release 保持 HTTPS。参见 [Android 官方网络安全配置](https://developer.android.com/privacy-and-security/security-config)。

生成文件在 `app/build/outputs/apk/`。普通手机不需要新增硬件；最低 Android 8.0。电脑需可用的现代浏览器与麦克风权限；内网 Web 录音仍要求客户端信任的 HTTPS。

## 7. AI 与可选本地识别

登录后在服务设置中选择模型、填写 API 密钥。配置按账户保存在服务器，Web 与 App 共用，手机无需重复填写。`云端 AI 搜索` 控制检索与材料识别；日常对话是独立主动功能。关闭开关不代表服务器全局禁止外发。

服务器可通过 EnvironmentFile 中的 `HTTPS_PROXY`、`NO_PROXY` 使用单位出站代理。`*_BASE_URL` 可由管理员指向兼容网关；账户保存模型设置或切换搜索开关后仍保留管理员配置的地址。视觉单独覆盖使用 `VISION_BASE_URL`，仅作用于 VISION_PROVIDER 对应服务。使用单位 CA 的出站 HTTPS 网关时可配置 `SSL_CERT_FILE`（证书路径需对服务用户可读），不要关闭 TLS 验证。

本地 OCR/ASR 不随部署包分发。需要时单独安装 `tesseract`、`ffmpeg`、`ffprobe`、`whisper-cli`、可选 `pdftotext`；将 chi_sim/eng.traineddata 放入 OCR_DATA_DIR，将 Whisper ggml-base.bin 放入 LOCAL_ASR_MODEL。工具必须位于 service 的 PATH 中，模型文件需可读。可在联网源码环境执行 `后端/setup-local-search.sh` 准备模型后迁入 Linux；**不要迁移 macOS 专用 linji-ocr 可执行文件到 Linux**。本地音频最长 120 秒。没有本地组件时仍可使用文字、图片指纹、已保存识别文本和手工补充文字；新材料自动识别需要对应可用引擎或云端服务。

## 8. 验收、备份与更新

先从实际电脑及手机所在网段验证 DNS、证书和连通性。源码或部署包内均有只读探测脚本：

```bash
bash 后端/deploy/intranet/check.sh https://linji.internal.example/clinical-memory
# 内部 CA 尚未装入 curl 信任库时：
bash 后端/deploy/intranet/check.sh https://linji.internal.example/clinical-memory /path/to/root-ca.pem
```

脚本检查 HTTPS、健康接口、Web 资源和未登录请求被拒绝，不创建账户、不读取笔记、不访问外部 AI。然后人工验证手机上传→网页可见、网页修改→手机同步、鉴权原件/图片/音频、Web 录音和模型请求。重启服务器后核对服务自启与数据保留；使用测试资料完成一次备份恢复演练。HTTPS、443 访问限制和外部 AI 连通性需要在真实目标网络另验。

预编译备份工具依赖 MySQL 客户端的 mysqldump。为使 SQL 与附件一致，备份期间停止应用（MySQL 保持运行）；用临时 systemd 服务读取相同 EnvironmentFile，避免把密码放在命令参数中：

```bash
sudo systemctl stop clinical-memory
sudo systemd-run --wait --pipe --collect --uid=clinicalmemory \
  --property=EnvironmentFile=/etc/clinical-memory/clinical-memory.env \
  --property=WorkingDirectory=/srv/clinical-memory/current/后端 \
  /srv/clinical-memory/current/后端/bin/linji-backup
sudo systemctl start clinical-memory
```

即使备份失败也要检查并恢复应用服务。普通备份仅含 SQL 和 uploads；配置、ai-settings、TLS 证书及本地模型需独立保护和管理。备份应复制到独立存储，不能只留在应用同一块磁盘。

更新时先备份，使用新的 releases 目录名解包（不能覆盖运行目录），核对配置和模型依赖，再切换 current 链接并重启、验收。首次表迁移会执行在配置指向的业务库上；后续涉及数据库结构变化时应事先验证恢复步骤，回退旧二进制并不等于数据库自动回退。

## 故障定位

| 现象 | 优先检查 |
| --- | --- |
| 手机连不上、电脑正常 | Wi-Fi/VLAN/访客隔离、手机 DNS、VPN、允许网段 |
| HTTPS 失败 | 域名与 SAN、完整证书链、客户端 CA 信任、时间、App 构建主机 |
| 403 | Nginx allow 网段与访问路径；同源请求的 Host 是否被代理改写 |
| 502 | Go 是否启动、DB 配置与权限、18090 端口是否一致 |
| 页面正常但 API 404 | PUBLIC_BASE_PATH 与 Nginx 路径是否一致，proxy_pass 是否错误去掉前缀 |
| 登录后看不到旧数据 | 是否连接了不同后端/空数据库，是否使用原账户 |
| AI 失败但资料正常 | 服务器出站 DNS/代理/CA、密钥与模型权限、云端搜索开关 |
| 本地识别失败 | Linux 引擎、模型文件、服务 PATH/读权限和音频时长 |

代码参考：`后端/internal/server/config.go`、`ai_settings.go`、`前端Web/app.js`、`前端APP/app/build.gradle`。内网版本保留已有数据格式与接口；不提供医院级审计、角色权限或灾备保证。
