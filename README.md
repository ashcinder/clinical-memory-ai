# 临记 · Go + MySQL + Android Java

医生个人诊疗笔记与材料库：保存治疗过程，用文字、图片和音频检索历史经验，查看原始步骤与资料来源，接入可配置的 AI 模型进行检索辅助、经验回顾或日常对话。

## 当前技术栈与四个目录

| 目录 | 内容 |
| --- | --- |
| `后端/` | Go 1.24+，标准库 HTTP 服务，MySQL 驱动，鉴权、检索、多模型对话 / 视觉 / ASR 适配与测试 |
| `前端APP/` | Android Studio 原生 **Java** 工程；Gradle 使用 Groovy，不含 `.kt` / `.kts` 源文件 |
| `前端Web/` | 浏览器原生 HTML、CSS、JavaScript；不需要 Node/npm 或打包工具 |
| `数据库数据/` | MySQL 建库与建表 SQL、SQL 备份、上传原件、旧版 SQLite 的只读归档 |

MySQL 的 InnoDB 引擎文件由你已经安装的 MySQL 服务管理，本项目不修改其 datadir，也不把运行中的 MySQL 文件移入工程。方便查看和测试的数据采用 SQL 导出与原始附件形式保存在“数据库数据”目录。

## 启动顺序

依次启动 **MySQL → Go 后端 → Web（可选独立进程）→ Android APP**。Web 和 Android 使用同一个后端账户和同一个 MySQL 数据库。

### 1. MySQL：确认服务、初始化项目数据库

你本机安装的 MySQL 命令位于 `/usr/local/mysql/bin/`。如果服务已经运行，不需要重新启动。先验证实际可用的 MySQL 管理员账户：

```bash
/usr/local/mysql/bin/mysql -u root -p
```

在密码提示处输入密码，终端不回显。若出现 `Access denied ... (1045)`，需要核对用户名、密码和端口；应用不会重置 root 密码。登录 MySQL 的账户密码与临记软件的登录账户密码是两套不同的凭据。

如果服务尚未运行，Oracle 安装包版 macOS 通常可通过系统设置的 MySQL 面板启动，或：

```bash
sudo /usr/local/mysql/support-files/mysql.server start
```

以下命令中的相对路径均从项目根目录开始。

首次准备项目：

```bash
cd 后端
# 若 .env 尚不存在，再复制；不要覆盖你已经填写的配置
[ -f .env ] || cp .env.example .env
bash setup-db.sh
```

`setup-db.sh` 会提示输入 MySQL 管理员密码，创建 `linji` 数据库与一个独立的应用账户，将**应用账户**写入 `.env`，文件权限为 0600；不保存管理员密码。使用其他管理员账户可执行 `bash setup-db.sh -admin-user=你的管理员用户名`。需要该管理员拥有建库、建用户和授权权限。此脚本只需首次运行，日常启动无需重复运行。

如果你已自行准备好数据库和应用账户，直接编辑 `后端/.env`，不必执行 setup-db.sh：

```dotenv
DB_HOST=127.0.0.1
DB_PORT=3306
DB_NAME=linji
DB_USER=你的MySQL应用用户名
DB_PASSWORD='你的MySQL应用密码'
HOST=127.0.0.1
PORT=8000
```

建库 SQL 在 `数据库数据/初始化.sql`，建表 SQL 在 `数据库数据/schema.sql`。后端首次启动会自动建表，应用账户需拥有该库的 SELECT、INSERT、UPDATE、DELETE、CREATE、INDEX、ALTER、REFERENCES 权限。密码包含 `#`、空格或 `$` 等符号时建议使用单引号包裹，避免变量展开；`!` 是英文和中文时属于不同字符。

### 2. Go 后端

终端一：

```bash
cd 后端
bash run.sh
```

等价命令为 `go run ./cmd/server`，首次会下载 go.mod / go.sum 固定的依赖。无需 Python、虚拟环境或 uvicorn。

- API：**http://127.0.0.1:8000/api/health**
- API 说明：**http://127.0.0.1:8000/docs**
- 后端自带 Web 页面：**http://127.0.0.1:8000**

默认监听本机。真机通过局域网访问时：

```bash
HOST=0.0.0.0 bash run.sh
```

仅改后端监听地址即可，不需要把 MySQL 3306 端口开放给手机。

编译为可执行文件：

```bash
go build -o bin/linji-server ./cmd/server
./bin/linji-server
```

启动时会检查数据库连接。若连接信息有误会直接给出说明并退出，不会偷偷退回 SQLite。

### 3. Web 前端

方式 A：后端已自带静态页面，直接访问 **http://127.0.0.1:8000**，不需要再启动前端进程。

方式 B：需要分别启动时，保留终端一，另开终端二：

```bash
cd 前端Web
bash run.sh
```

访问 **http://127.0.0.1:5173**。该进程使用 Go 提供静态页面，并把 `/api` 代理到后端，保持 Cookie 同源登录。后端未启动时会明确提示无法连接。不要用 `file://` 直接打开 index.html，也不要只用不带代理的静态服务器。

独立 Web 默认仅监听 `127.0.0.1:5173`，需要改端口可在 `.env` 设置 `WEB_PORT`。浏览器首次打开后创建**临记软件账户**，可选择导入明确标注的虚构示例。

### 4. Android Java APP

1. Android Studio 选择 **Open**，打开 `前端APP` 文件夹。
2. 等待 Gradle 同步。使用 Android Studio 自带 JDK 21（最低 JDK 17），并安装 Android SDK 33。
3. 选择模拟器或通过 USB 连接手机，点击绿色 **Run ▶**。
4. 在 APP 连接页填写后端地址，点击“连接 / 检查服务”，再用与 Web 相同的临记软件账户登录。

地址：

- Android Studio 模拟器：`http://10.0.2.2:8000`
- 真机：`http://电脑的局域网IP:8000`；电脑、手机同一 Wi-Fi，后端用 `HOST=0.0.0.0 bash run.sh`，并允许防火墙 8000 端口。
- 正式部署：使用 HTTPS 域名。Debug 允许本地 HTTP，Release 禁用明文 HTTP。

命令行构建：

```bash
cd 前端APP
./gradlew assembleDebug
# 已连接设备时安装（也可直接用 Android Studio Run）
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

APK：`前端APP/app/build/outputs/apk/debug/app-debug.apk`。业务源码全部为 Java；界面、文件选择、录音、鉴权和 HTTP 调用均为原生 Android 实现。targetSdk 33 用于本地测试，发布到应用商店前应升级 targetSdk 并配置正式签名。

## 现成测试资料

项目提供三组明确标注的虚构示例；创建软件账户后，在后端目录运行 `go run ./cmd/import-samples` 即可导入，每组包括文字、合成录音和公开授权的解剖示意图。Web 经验检索中的“试用示例”和 Android 首页的“试用图片 / 录音示例”可以直接选择测试素材，无需先把文件传到手机。也可在“数据库数据/测试资料”中自行选择原文件。详细来源和每组测试关键词见 `数据库数据/测试资料/使用说明.md`。

当前未配置视觉模型和 ASR 时，只有这六个字节完全相同的固定演示文件会匹配预置描述或转写；页面明确标注“示例预置文本”。真实新图、新录音需要配置识别服务或手动补充文本，不会被当作已自动识别。

## 配置 AI 模型、图片和音频

在 `后端/.env` 中配置服务，保存后重启 Go 后端；密钥只保存在后端，**不要发送到聊天中**。对话及检索关键词使用 `AI_PROVIDER`，图片识别使用 `VISION_PROVIDER`，可以分别选择不同服务。示例使用 DeepSeek：

```dotenv
AI_PROVIDER=deepseek
DEEPSEEK_API_KEY=在此填写你的密钥
DEEPSEEK_MODEL=deepseek-flash
VISION_PROVIDER=deepseek
VISION_MODEL=deepseek-flash
```

也可以选择下列服务。先在对应平台确认账号拥有该模型的 API 权限与额度；模型名可根据平台当前提供的型号更改。各服务通过兼容 `/chat/completions` 的接口调用，实际可用性以服务商为准。 可参考 [DeepSeek API](https://api-docs.deepseek.com/api/create-chat-completion/)、[智谱 GLM-4.6V](https://docs.bigmodel.cn/cn/guide/models/vlm/glm-4.6v)、[MiniMax OpenAI SDK](https://platform.minimax.cn/docs/api-reference/text-openai-api) 和 [Kimi API 配置](https://www.kimi.com/help/kimi-api/api-troubleshooting)。

| AI_PROVIDER / VISION_PROVIDER | 对话密钥 | 对话模型设置 | 图片模型示例 |
| --- | --- | --- | --- |
| `deepseek` | `DEEPSEEK_API_KEY` | `DEEPSEEK_MODEL=deepseek-flash` | `VISION_MODEL=deepseek-flash` |
| `glm` | `GLM_API_KEY` | `GLM_MODEL=glm-4.5` | `VISION_MODEL=glm-4.6v` |
| `kimi` | `KIMI_API_KEY` | `KIMI_MODEL=kimi-k2.5` | `VISION_MODEL=kimi-k2.5` |
| `minimax` | `MINIMAX_API_KEY` | `MINIMAX_MODEL=MiniMax-M3.1-Flash` | `VISION_MODEL=MiniMax-M3.1-Flash` |
| `none` | 不需要 | 关闭该能力 | 关闭云端视觉 |

例如同时使用 MiniMax 对话与 GLM 视觉，在现有 `.env` 中**修改已有的同名行**并填写密钥：

```dotenv
AI_PROVIDER=minimax
MINIMAX_API_KEY=填写MiniMax密钥
MINIMAX_MODEL=MiniMax-M3.1-Flash
VISION_PROVIDER=glm
GLM_API_KEY=填写智谱密钥
VISION_MODEL=glm-4.6v
```

`VISION_MODEL` 必须与选定的视觉服务对应；原项目的 `.env` 可能已有 `VISION_MODEL=deepseek-flash`，切换视觉服务时要一起修改。未设置 `VISION_PROVIDER` 时默认跟随对话服务，并使用该服务默认的视觉模型。仅当需要使用其他兼容网关时，才覆盖 `VISION_BASE_URL` / `VISION_API_KEY`。不要把 `VISION_API_KEY=` 留为空行，空值会关闭视觉调用。详见 [配置示例](后端/.env.example)。

语音转写需单独设置 `ASR_BASE_URL`、`ASR_API_KEY`、`ASR_MODEL`，使用兼容 `/audio/transcriptions` 的服务。对话模型密钥本身不会自动开通语音转写。PDF 的文字层本地提取；扫描件可作为图片上传。

对话或关键词整理将问题和已识别的查询内容发送给对话服务；图片上传/重新提取会将图片发送给视觉服务；经验回顾还会发送命中笔记的片段。调用可能产生费用，真实患者资料应先脱敏并确认所选平台的数据处理方式。

可选本地 PDF 提取工具来自 Poppler，如已使用 Homebrew 可安装 `brew install poppler`；不是 Go 服务的运行前提。所有提取结果都应人工核对。云端失败保留原件并提示重试；不把失败伪装成已识别。

Web 录音需 localhost / HTTPS 与麦克风权限。局域网 HTTP 下可上传已有音频；Android 原生录音不受这一浏览器限制。单个文件限 20MB。

## 检索和数据保存

Go 将笔记、标签、步骤和附件文本分成中文单字/双字及英文词，持久写入 MySQL 的 `note_terms` 倒排索引，按匹配权重排序。无需修改 MySQL 的 ngram 配置或添加向量数据库。修改正文/材料文本、关联/解除关联及删除时通过事务同步更新索引。

配置对话服务后，模型先从查询中整理关键词和同义词，MySQL 再按文字倒排索引检索；历史经验对话最多取 5 篇所属账户的笔记供模型总结，页面显示可打开的来源。图片在上传时先由视觉模型生成可核对的描述，既有图片可在材料页面选择“重新提取”，关联笔记的索引会更新。结果是基于识别文本和关键词的检索，并非图像向量相似度匹配；模型可能误识别或遗漏穴位，结果必须核对原图与笔记。AI 关键词整理失败时回退到原文检索并提示。日常对话不自动读取资料库，主动附加的材料除外。

删除笔记会保留原始材料；删除材料会移除文件与识别文本。旧对话仍保存当时的来源摘要，删除笔记不会改写历史回复。

### 查看 MySQL 数据

```bash
cd 后端
go run ./cmd/db-info
```

也可用 MySQL Workbench / DBeaver，填写 `.env` 中同样的主机、端口、用户名和数据库名。查看 `notes`、`attachments`、`conversations`、`messages`、`note_terms` 等表。

### 备份

先停后端，避免备份过程中上传/删除原件；MySQL 可以继续运行：

```bash
cd 后端
bash backup.sh
# 或指定路径
bash backup.sh -output '/你选择的目录/临记备份.tar.gz'
```

默认输出到 `数据库数据/backups/`，包含 `linji.sql` 和 `uploads/`，不包含 `.env` 或密钥。使用本机 mysqldump，连接密码通过权限为 0600 的临时配置传入，不放在命令行参数中。找不到命令时设置 `MYSQLDUMP_BIN=/usr/local/mysql/bin/mysqldump`。

恢复时先备份现有资料并停后端，将 `linji.sql` 导入**专用空数据库**，再恢复 `uploads/`，调整 `.env` 指向恢复库。SQL 中含账户/会话及隐私资料，请妥善保管。页面“导出笔记 JSON”是便于查看的导出，不是含附件原件的完整备份。

## 验证

```bash
cd 后端
go test ./...
go vet ./...
```

没有测试 MySQL 连接时，数据库集成测试会明确跳过。使用**专用测试实例**运行完整测试：

```bash
MYSQL_TEST_DSN='root@tcp(127.0.0.1:3307)/?charset=utf8mb4&parseTime=true' go test -race ./... -count=1
```

示例中的 3307 是本次独立验证实例，不是你的日常 MySQL 端口；不要照抄为日常 `.env`。测试只创建/删除随机 `linji_test_` 前缀的库，测试账户需要相应建库和删库权限，不会清空已有业务数据库。详细结果见 `后端/验证记录.md`。

## 旧版本与使用边界

当前版本使用 Go 和 MySQL，不包含历史数据库或旧版归档。

这是个人本地测试原型，已实现密码哈希、过期会话、登录限流、资源归属校验、上传大小限制、Web HttpOnly Cookie 和 Android Keystore 令牌加密。未提供医院级审计、角色权限、静态数据加密与灾备。启用云端服务会发送相关问题、命中笔记片段或材料到对应服务，应先脱敏并取得适当授权。模型回复与历史经验需要医生核对。

## 许可证

项目代码采用 [MIT License](LICENSE)。测试图片的来源及授权说明见 [测试资料使用说明](数据库数据/测试资料/使用说明.md)。
