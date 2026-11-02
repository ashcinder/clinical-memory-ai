# MySQL SQL、备份与附件

当前数据库为 MySQL，由本机 MySQL 服务管理引擎数据文件。本目录保存建表脚本和虚构测试素材；数据库内容、上传原件与备份不会提交到 Git。

- `初始化.sql`：创建项目数据库，附独立应用账户授权示例。
- `schema.sql`：MySQL 表结构，与 Go 后端内嵌迁移一致。
- `uploads/`：上传原件，UUID 文件名；原名与所属关系保存在 MySQL attachments 表。
- `backups/`：`后端/backup.sh` 生成的 SQL + 原件完整备份。

查看数据：在“后端”目录执行 `go run ./cmd/db-info`，或用 Workbench / DBeaver 连接 .env 中的 MySQL 实例。

主要表：users、sessions、notes、attachments、note_terms（中文倒排索引）、conversations、messages、login_attempts、app_state（并发初始化锁）、user_sync（跨端同步版本）。标签和步骤用 MySQL JSON 字段保存。

首次启动 Go 后端自动建表。MySQL 应用账户需要本项目库的建表与读写权限。备份前停后端，MySQL 继续运行，执行 `bash backup.sh`。完整备份不包含 .env，但包含账户和隐私资料，仍需妥善保管。

`user_taxonomy` 保存各账户的资料类型和独立标签目录（包括尚未用于笔记的条目）；`config` 为 JSON。查看时可运行 `SELECT user_id, JSON_PRETTY(config) FROM user_taxonomy;`。尚未修改目录的账户按默认七个类型读取。类型、标签的修改请通过界面完成，以便同时更新笔记与检索索引。
