package main

import (
	"database/sql"
	"fmt"
	"linji/internal/server"
	"log"
)

func main() {
	cfg, err := server.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("mysql", cfg.MySQLConfig().FormatDSN())
	if err != nil {
		log.Fatal("数据库配置无效")
	}
	defer db.Close()
	if err = db.Ping(); err != nil {
		log.Fatal("无法连接 MySQL，请检查 .env 和数据库服务")
	}
	fmt.Println("数据库：", cfg.DBName)
	for _, table := range []string{"users", "notes", "attachments", "conversations", "messages"} {
		var count int
		if err = db.QueryRow("SELECT COUNT(*) FROM `" + table + "`").Scan(&count); err != nil {
			log.Fatal("查询失败，可能尚未启动后端自动建表")
		}
		fmt.Printf("%s: %d\n", table, count)
	}
	rows, err := db.Query("SELECT id,title,category,DATE_FORMAT(occurred_on,'%Y-%m-%d') FROM notes ORDER BY updated_at DESC LIMIT 20")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	fmt.Println("最近笔记：")
	for rows.Next() {
		var id, title, category, date string
		if err = rows.Scan(&id, &title, &category, &date); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s | %s | %s | %s\n", id, title, category, date)
	}
	if err = rows.Err(); err != nil {
		log.Fatal(err)
	}
}
