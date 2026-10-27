# Go 后端

运行要求：Go 1.24+、MySQL 8.0+（已使用本机 MySQL 9.7.1 二进制验证），不依赖 Python。

- 首次准备数据库：`bash setup-db.sh`，在终端输入管理员密码；脚本创建独立应用账户并写入 `.env`。
- 启动：`bash run.sh` 或 `go run ./cmd/server`。
- 编译：`go build -o bin/linji-server ./cmd/server`。
- 数据查看：`go run ./cmd/db-info`。
- 完整备份：停后端后运行 `bash backup.sh`。
- API：http://127.0.0.1:8000/api/health，说明：http://127.0.0.1:8000/docs。
- 测试：`go test ./...`；完整 MySQL 集成测试需设置 `MYSQL_TEST_DSN`，参阅根目录 README。

主要源码：`internal/server/`，包括鉴权、MySQL 事务、中文倒排索引、附件处理、DeepSeek / 视觉 / ASR HTTP 适配。运行时自动执行内嵌的 `schema.sql`，与“数据库数据/schema.sql”保持一致。

.env 已加入 Git 忽略；不要将数据库或 AI 服务密码写进源码或文档。setup-db.sh 不保存管理员密码。后端检查连接失败会退出，不会切换成其他数据库。
