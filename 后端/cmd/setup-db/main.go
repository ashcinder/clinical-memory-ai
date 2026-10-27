// Creates only this project's database and an isolated application account.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"golang.org/x/term"
	"linji/internal/server"
)

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return hex.EncodeToString(b)
}
func main() {
	admin := flag.String("admin-user", "root", "MySQL 管理员用户名")
	flag.Parse()
	cfg, err := server.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`).MatchString(cfg.DBName) {
		log.Fatal("DB_NAME 只允许字母、数字和下划线")
	}
	fmt.Print("MySQL 管理员密码（输入不显示）: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		log.Fatal("无法读取密码，请在终端中运行本命令")
	}
	mysqlCfg := cfg.MySQLConfig()
	mysqlCfg.User = *admin
	mysqlCfg.Passwd = string(password)
	mysqlCfg.DBName = ""
	db, err := sql.Open("mysql", mysqlCfg.FormatDSN())
	if err != nil {
		log.Fatal("MySQL 配置无效")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		if databaseError, ok := err.(*mysql.MySQLError); ok {
			log.Printf("MySQL 错误编号：%d", databaseError.Number)
		} else {
			log.Printf("连接失败：%v", err)
		}
		log.Fatal("管理员连接失败。请确认密码字符（含感叹号）、MySQL 运行状态和 DB_HOST / DB_PORT；未修改任何账户密码。")
	}
	if _, err = db.Exec("CREATE DATABASE IF NOT EXISTS `" + cfg.DBName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		log.Fatal("创建项目数据库失败：", err)
	}
	user := "linji_" + randomHex(4)
	appPassword := randomHex(24)
	// Generated values contain only ASCII letters, digits and underscore.
	if _, err = db.Exec("CREATE USER '" + user + "'@'localhost' IDENTIFIED BY '" + appPassword + "'"); err != nil {
		log.Fatal("创建应用账户失败：", err)
	}
	if _, err = db.Exec("GRANT SELECT,INSERT,UPDATE,DELETE,CREATE,INDEX,ALTER,REFERENCES ON `" + cfg.DBName + "`.* TO '" + user + "'@'localhost'"); err != nil {
		log.Fatal("授权应用账户失败：", err)
	}
	path := filepath.Join(cfg.Root, "后端", ".env")
	values, err := godotenv.Read(path)
	if os.IsNotExist(err) {
		values = map[string]string{}
	} else if err != nil {
		log.Fatal("读取 .env 失败")
	}
	values["DB_USER"] = user
	values["DB_PASSWORD"] = appPassword
	// Use a temporary 0600 file + rename so root credentials are never saved.
	content, err := godotenv.Marshal(values)
	if err != nil {
		log.Fatal(err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".env-setup-*")
	if err != nil {
		log.Fatal(err)
	}
	tempName := temporary.Name()
	defer os.Remove(tempName)
	if _, err = temporary.WriteString(strings.TrimSpace(content) + "\n"); err != nil {
		temporary.Close()
		log.Fatal(err)
	}
	if err = temporary.Close(); err != nil {
		log.Fatal(err)
	}
	if err = os.Rename(tempName, path); err != nil {
		log.Fatal("保存应用连接配置失败：", err)
	}
	fmt.Printf("已创建数据库 %s 与独立应用账户，连接信息已写入后端/.env（权限 0600）。管理员密码未保存。\n", cfg.DBName)
}
