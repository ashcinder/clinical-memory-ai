-- 用 MySQL 管理员导入本文件；只创建本项目数据库，不修改其他数据库或用户。
CREATE DATABASE IF NOT EXISTS linji CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
-- 推荐自行创建独立应用账户：将下面的密码换成自己的，再取消注释执行。
-- CREATE USER 'linji'@'localhost' IDENTIFIED BY '替换成你自己的强密码';
-- GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, INDEX, ALTER, REFERENCES ON linji.* TO 'linji'@'localhost';
-- 后端首次启动会自动创建所有表。也可以在 linji 库中手动导入 schema.sql。
