# Go 后端

Linux 部署包、Nginx/systemd 模板、环境配置和迁移见 [内网部署说明](deploy/intranet/README.md)。项目根目录运行 `bash 后端/deploy/intranet/build-bundle.sh` 构建；服务器运行预编译二进制，无需 Go。支持 HTTPS_PROXY/NO_PROXY 出站代理，账户保存模型设置后保留管理员配置的 AI 网关地址。

运行要求：Go 1.24+、MySQL 8.0+（已使用本机 MySQL 9.7.1 二进制验证），不依赖 Python。

- 首次准备数据库：`bash setup-db.sh`，在终端输入管理员密码；脚本创建独立应用账户并写入 `.env`。
- 启动：`bash run.sh` 或 `go run ./cmd/server`。
- 编译：`go build -o bin/linji-server ./cmd/server`。
- 数据查看：`go run ./cmd/db-info`。
- 完整备份：停后端后运行 `bash backup.sh`。
- API：http://127.0.0.1:8000/api/health，说明：http://127.0.0.1:8000/docs。
- 测试：`go test ./...`；完整 MySQL 集成测试需设置 `MYSQL_TEST_DSN`，参阅根目录 README。

主要源码：`internal/server/`，包括鉴权、MySQL 事务、中文倒排索引、附件处理、多模型 / 视觉 / ASR HTTP 适配。运行时自动执行内嵌的 `schema.sql`，与“数据库数据/schema.sql”保持一致。

.env 已加入 Git 忽略；不要将数据库或 AI 服务密码写进源码或文档。setup-db.sh 不保存管理员密码。后端检查连接失败会退出，不会切换成其他数据库。

AI 设置可在 Web 或 APP 界面中修改；每个账户的密钥以 0600 权限保存在数据目录 `ai-settings/` 中，不会返回前端或随普通备份导出。

GET /api/sync 提供账户同步版本；PUT /api/auth/me 修改显示名称。笔记和模型配置支持 expected_version，过期编辑返回 409。

## 本地搜索

运行 `bash setup-local-search.sh` 准备本地 OCR 与语音模型，在 Web / APP 关闭“云端 AI 搜索”后生效。macOS 使用本机 Vision OCR，其他系统使用 Tesseract；语音使用 whisper.cpp + FFmpeg，最长 120 秒。新建 image_fingerprints 表由启动迁移自动完成，旧图片首次被搜索时补建缓存。开关通过 `/api/search-settings` 更新并参与账户同步；不清除 API 密钥。日常对话独立于搜索开关。

`LINJI_RUN_OFFLINE=1 go test -v ./internal/server -run TestLiveOfflineExtraction` 验证实际本地识别引擎。`MYSQL_TEST_DSN=... go test -race ./...` 包含断言云端请求次数为零的关闭开关测试、精确与压缩图匹配、权限和缓存验证。

分类与标签由 MySQL 的 `user_taxonomy` 按账户保存，启动时自动建表。`GET /api/taxonomy` 返回可编辑目录、使用数量及 `version`；`POST /api/taxonomy` 接受 `kind`（type/tag）、`action`（create/rename/delete）、`id`、`name`、`expected_version`。修改在事务中同步更新笔记分类/标签、全文索引和同步版本，版本冲突返回 409；删除类型将笔记移到保留类型，删除标签只移除关联。预设类型保留稳定 ID 与旧分类别名，自定义类型使用独立 ID。
