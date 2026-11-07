package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func benchmarkQuotaPreflight(t *testing.T, c Config, dir string) {
	t.Helper()
	endpoint, err := url.Parse(c.VisionURL)
	if err != nil || c.VisionProvider != "deepseek" || endpoint.Hostname() != "api.deepseek.com" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.deepseek.com/user/balance", nil)
	req.Header.Set("Authorization", "Bearer "+c.VisionKey)
	r, err := newAIClient().Do(req)
	if err != nil {
		t.Fatal("模型余额预检失败，未启动付费测试")
	}
	defer r.Body.Close()
	var data struct {
		Available bool `json:"is_available"`
	}
	if r.StatusCode != 200 || json.NewDecoder(r.Body).Decode(&data) != nil {
		t.Fatal("模型余额预检返回异常，未启动付费测试")
	}
	raw, _ := json.MarshalIndent(map[string]any{"http_status": r.StatusCode, "is_available": data.Available, "timestamp": now()}, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "quota-preflight.json"), raw, 0600)
	if !data.Available {
		t.Fatal("DeepSeek 余额不足：未启动付费评测；请充值后重试")
	}
}

func benchmarkClientArgs(t *testing.T, a *App) ([]string, string) {
	t.Helper()
	var db string
	if err := a.DB.QueryRow("SELECT DATABASE()").Scan(&db); err != nil || !strings.HasPrefix(db, "linji_test_") {
		t.Fatal("只允许在 linji_test_ 独立测试库保存/恢复状态")
	}
	c, err := mysql.ParseDSN(os.Getenv("MYSQL_TEST_DSN"))
	if err != nil {
		t.Fatal("无效的专用测试库 DSN")
	}
	quote := func(s string) string {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`).Replace(s) + `"`
	}
	config := "[client]\nuser=" + quote(c.User) + "\npassword=" + quote(c.Passwd) + "\n"
	if c.Net == "unix" {
		config += "socket=" + quote(c.Addr) + "\nprotocol=SOCKET\n"
	} else {
		host, port, e := net.SplitHostPort(c.Addr)
		if e != nil {
			t.Fatal("测试库地址无效")
		}
		config += "host=" + quote(host) + "\nport=" + port + "\nprotocol=TCP\n"
	}
	path := filepath.Join(t.TempDir(), "client.cnf")
	if os.WriteFile(path, []byte(config), 0600) != nil {
		t.Fatal("不能写入私有测试客户端配置")
	}
	return []string{"--defaults-extra-file=" + path}, db
}

func benchmarkCopyUploads(source, dest string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dest, 0700); err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		raw, e := os.ReadFile(filepath.Join(source, entry.Name()))
		if e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(dest, entry.Name()), raw, 0600); e != nil {
			return e
		}
	}
	return nil
}

func benchmarkSaveState(t *testing.T, a *App, dir string) {
	t.Helper()
	state := filepath.Join(dir, "state")
	if os.MkdirAll(state, 0700) != nil {
		t.Error("不能创建评测状态目录")
		return
	}
	args, db := benchmarkClientArgs(t, a)
	out, err := os.OpenFile(filepath.Join(state, "database.sql"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		t.Error("不能保存测试状态")
		return
	}
	cmd := exec.Command(env("MYSQLDUMP_BIN", "mysqldump"), append(args, "--single-transaction", "--skip-lock-tables", "--no-tablespaces", "--set-gtid-purged=OFF", "--column-statistics=0", db)...)
	cmd.Stdout = out
	var diagnostic bytes.Buffer
	cmd.Stderr = &diagnostic
	runErr := cmd.Run()
	out.Close()
	if runErr != nil {
		t.Error("测试库快照保存失败")
		return
	}
	if err = benchmarkCopyUploads(filepath.Join(a.Config.DataDir, "uploads"), filepath.Join(state, "uploads")); err != nil {
		t.Error("测试附件快照保存失败")
		return
	}
	raw, err := os.ReadFile(filepath.Join(state, "database.sql"))
	if err != nil {
		t.Error(err)
		return
	}
	sum := sha256.Sum256(raw)
	metadata, _ := json.MarshalIndent(map[string]any{"database_sha256": hex.EncodeToString(sum[:]), "model_signature": a.imageSignature(context.Background()), "timestamp": now(), "test_database": true}, "", "  ")
	_ = os.WriteFile(filepath.Join(state, "metadata.json"), metadata, 0600)
}

func benchmarkRestoreState(t *testing.T, a *App, dir string) {
	t.Helper()
	state := filepath.Join(dir, "state")
	raw, err := os.ReadFile(filepath.Join(state, "database.sql"))
	if err != nil {
		t.Fatal("缺少评测状态快照")
	}
	upper := strings.ToUpper(string(raw))
	if strings.Contains(upper, "CREATE DATABASE") || strings.Contains(upper, "DROP DATABASE") || strings.Contains(upper, "\nUSE ") {
		t.Fatal("评测快照禁止切换数据库")
	}
	var metadata struct {
		SHA       string `json:"database_sha256"`
		Signature string `json:"model_signature"`
	}
	m, e := os.ReadFile(filepath.Join(state, "metadata.json"))
	if e != nil || json.Unmarshal(m, &metadata) != nil {
		t.Fatal("缺少快照校验信息")
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != metadata.SHA {
		t.Fatal("快照校验失败")
	}
	if a.imageSignature(context.Background()) != metadata.Signature {
		t.Fatal("当前图片模型与缓存不同，请使用新的评测目录")
	}
	args, db := benchmarkClientArgs(t, a)
	cmd := exec.Command(env("MYSQL_BIN", "mysql"), append(args, db)...)
	cmd.Stdin = bytes.NewReader(raw)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err = cmd.Run(); err != nil {
		t.Fatal("评测快照恢复失败")
	}
	if err = benchmarkCopyUploads(filepath.Join(state, "uploads"), filepath.Join(a.Config.DataDir, "uploads")); err != nil {
		t.Fatal("评测附件恢复失败")
	}
	t.Log("已恢复独立评测库与图片识别缓存，未重复识别已建立的图库")
}

func benchmarkArchiveRun(dir string) error {
	names := []string{"results.json", "results.jsonl", "progress.json", "summary.json", "failures.json", "candidate-coverage.json", "api-usage.json", "error-audit.json", "测试结果.md", "manifest.json", "gallery-map.json", "quota-preflight.json", "gallery-observations.json"}
	if _, err := os.Stat(filepath.Join(dir, "results.json")); err != nil {
		return nil
	}
	archive := filepath.Join(dir, "attempts", time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err := os.MkdirAll(archive, 0700); err != nil {
		return err
	}
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(archive, name), raw, 0600); err != nil {
			return err
		}
	}
	// Preserve the old profiles and attachment bytes before the new run saves state.
	state := filepath.Join(dir, "state")
	if _, err := os.Stat(state); err == nil {
		if err = benchmarkCopyUploads(state, filepath.Join(archive, "state")); err != nil {
			return err
		}
		if err = benchmarkCopyUploads(filepath.Join(state, "uploads"), filepath.Join(archive, "state", "uploads")); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}
