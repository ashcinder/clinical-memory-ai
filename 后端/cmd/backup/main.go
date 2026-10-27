package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"linji/internal/server"
)

func ini(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`).Replace(s) + `"`
}
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	output := flag.String("output", "", "输出 .tar.gz 路径，默认 数据库数据/backups/ 下")
	flag.Parse()
	cfg, err := server.LoadConfig()
	if err != nil {
		return err
	}
	if *output == "" {
		*output = filepath.Join(cfg.DataDir, "backups", "linji-"+time.Now().Format("20060102-150405")+".tar.gz")
	}
	temporary, err := os.MkdirTemp("", "linji-backup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	clientConfig := "[client]\nuser=" + ini(cfg.DBUser) + "\npassword=" + ini(cfg.DBPassword) + "\nhost=" + ini(cfg.DBHost) + "\nport=" + cfg.DBPort + "\n"
	if cfg.DBSocket != "" {
		clientConfig += "socket=" + ini(cfg.DBSocket) + "\nprotocol=SOCKET\n"
	} else {
		clientConfig += "protocol=TCP\n"
	}
	defaults := filepath.Join(temporary, "client.cnf")
	if err = os.WriteFile(defaults, []byte(clientConfig), 0600); err != nil {
		return err
	}
	executable := os.Getenv("MYSQLDUMP_BIN")
	if executable == "" {
		executable = "mysqldump"
	}
	sqlPath := filepath.Join(temporary, "linji.sql")
	sqlFile, err := os.OpenFile(sqlPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	dump := exec.Command(executable, "--defaults-extra-file="+defaults, "--single-transaction", "--skip-lock-tables", "--no-tablespaces", "--set-gtid-purged=OFF", "--column-statistics=0", cfg.DBName)
	dump.Stdout = sqlFile
	var diagnostic strings.Builder
	dump.Stderr = &diagnostic
	err = dump.Run()
	closeErr := sqlFile.Close()
	if err != nil {
		return fmt.Errorf("mysqldump 失败：请检查 MySQL 连接信息、应用账户权限和 mysqldump 路径。未生成备份。")
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.MkdirAll(filepath.Dir(*output), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("无法创建备份（不会覆盖已有文件）：%w", err)
	}
	success := false
	defer func() {
		file.Close()
		if !success {
			_ = os.Remove(*output)
		}
	}()
	gzipWriter := gzip.NewWriter(file)
	archive := tar.NewWriter(gzipWriter)
	add := func(path, name string) error {
		return filepath.Walk(path, func(p string, info os.FileInfo, e error) error {
			if e != nil {
				return e
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("备份目录不接受符号链接")
			}
			relative, e := filepath.Rel(path, p)
			if e != nil {
				return e
			}
			entry := name
			if relative != "." {
				entry = filepath.ToSlash(filepath.Join(name, relative))
			}
			header, e := tar.FileInfoHeader(info, "")
			if e != nil {
				return e
			}
			header.Name = entry
			if e = archive.WriteHeader(header); e != nil {
				return e
			}
			if info.Mode().IsRegular() {
				f, e := os.Open(p)
				if e != nil {
					return e
				}
				_, e = io.Copy(archive, f)
				closeErr := f.Close()
				if e != nil {
					return e
				}
				return closeErr
			}
			return nil
		})
	}
	if err = add(sqlPath, "linji.sql"); err != nil {
		return err
	}
	if err = add(filepath.Join(cfg.DataDir, "uploads"), "uploads"); err != nil {
		return err
	}
	if err = archive.Close(); err != nil {
		return err
	}
	if err = gzipWriter.Close(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	success = true
	absolute, _ := filepath.Abs(*output)
	fmt.Println("完整备份已保存：" + absolute)
	return nil
}
