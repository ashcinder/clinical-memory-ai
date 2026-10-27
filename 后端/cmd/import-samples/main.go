package main

import (
	"flag"
	"fmt"
	"linji/internal/server"
	"log"
)

func main() {
	user := flag.String("user", "", "资料所属的临记账户；只有一个账户时可省略")
	flag.Parse()
	cfg, err := server.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	app, err := server.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer app.DB.Close()
	result, err := app.ImportSamples(*user)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("已为账户 %s 导入 %d 组虚构案例，已存在跳过 %d 组。每组包含文字、图片、录音。\n", result.Username, result.Created, result.Skipped)
}
