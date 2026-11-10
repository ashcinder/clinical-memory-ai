package server

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

type Config struct {
	BasePath                                             string
	Root, DataDir, WebDir, Addr, WebAddr                 string
	DBHost, DBPort, DBName, DBUser, DBPassword, DBSocket string
	DeepSeekKey, DeepSeekURL, DeepSeekModel              string
	VisionKey, VisionURL, VisionModel                    string
	AIProvider, VisionProvider                           string
	ProviderURLs                                         map[string]string
	ASRKey, ASRURL, ASRModel                             string
	CloudSearchDisabled                                  bool
	ImageDINOModel, ImageDINOPython                      string
	ImageVisualCandidates                                int
	LocalOrganizerModel, LocalOrganizerCLI               string
	LocalASRModel, OCRDataDir                            string
	CookieSecure                                         bool
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
func LoadConfig() (Config, error) {
	root := os.Getenv("PROJECT_ROOT")
	if root == "" {
		dir, err := os.Getwd()
		if err != nil {
			return Config{}, err
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "前端Web", "index.html")); err == nil {
				root = dir
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return Config{}, fmt.Errorf("找不到项目根目录，请在项目中启动或设置 PROJECT_ROOT")
			}
			dir = parent
		}
	}
	root, _ = filepath.Abs(root)
	// Existing environment variables take priority; API keys are never printed.
	if err := godotenv.Load(filepath.Join(root, "后端", ".env")); err != nil && !os.IsNotExist(err) {
		return Config{}, fmt.Errorf(".env 格式错误，请检查引号和换行")
	}
	c := Config{Root: root, DataDir: env("DATA_DIR", filepath.Join(root, "数据库数据")), WebDir: filepath.Join(root, "前端Web"),
		Addr: net.JoinHostPort(env("HOST", "127.0.0.1"), env("PORT", "8000")), WebAddr: net.JoinHostPort("127.0.0.1", env("WEB_PORT", "5173")),
		DBHost: env("DB_HOST", "127.0.0.1"), DBPort: env("DB_PORT", "3306"), DBName: env("DB_NAME", "linji"), DBUser: env("DB_USER", "linji"), DBPassword: os.Getenv("DB_PASSWORD"), DBSocket: os.Getenv("DB_SOCKET"),
		DeepSeekKey: os.Getenv("DEEPSEEK_API_KEY"), DeepSeekURL: strings.TrimRight(env("DEEPSEEK_BASE_URL", "https://api.deepseek.com"), "/"), DeepSeekModel: env("DEEPSEEK_MODEL", "deepseek-flash"),
		ASRKey: os.Getenv("ASR_API_KEY"), ASRURL: strings.TrimRight(os.Getenv("ASR_BASE_URL"), "/"), ASRModel: env("ASR_MODEL", "whisper-1"), CookieSecure: env("COOKIE_SECURE", "false") == "true"}
	// The legacy DeepSeek fields hold the selected chat provider so older installations
	// continue to work. Keys never leave the backend settings response.
	c.AIProvider = strings.ToLower(env("AI_PROVIDER", "deepseek"))
	c.ProviderURLs = map[string]string{}
	for name, defaultURL := range aiProviderURLs {
		c.ProviderURLs[name] = strings.TrimRight(env(strings.ToUpper(name)+"_BASE_URL", defaultURL), "/")
	}
	visionDefault := c.AIProvider
	c.VisionProvider = strings.ToLower(env("VISION_PROVIDER", visionDefault))
	provider := func(name string) (string, string, string, error) {
		switch name {
		case "deepseek":
			return os.Getenv("DEEPSEEK_API_KEY"), strings.TrimRight(env("DEEPSEEK_BASE_URL", "https://api.deepseek.com"), "/"), env("DEEPSEEK_MODEL", "deepseek-flash"), nil
		case "glm":
			return os.Getenv("GLM_API_KEY"), strings.TrimRight(env("GLM_BASE_URL", "https://open.bigmodel.cn/api/paas/v4"), "/"), env("GLM_MODEL", "glm-4.5"), nil
		case "kimi":
			return os.Getenv("KIMI_API_KEY"), strings.TrimRight(env("KIMI_BASE_URL", "https://api.moonshot.cn/v1"), "/"), env("KIMI_MODEL", "kimi-k2.5"), nil
		case "minimax":
			return os.Getenv("MINIMAX_API_KEY"), strings.TrimRight(env("MINIMAX_BASE_URL", "https://api.minimax.cn/v1"), "/"), env("MINIMAX_MODEL", "MiniMax-M3.1-Flash"), nil
		case "none":
			return "", "", "", nil
		default:
			return "", "", "", fmt.Errorf("未知模型服务 %q；请选择 deepseek、glm、kimi、minimax 或 none", name)
		}
	}
	var err error
	c.DeepSeekKey, c.DeepSeekURL, c.DeepSeekModel, err = provider(c.AIProvider)
	if err != nil {
		return Config{}, err
	}
	c.VisionKey, c.VisionURL, c.VisionModel, err = provider(c.VisionProvider)
	if err != nil {
		return Config{}, err
	}
	if c.VisionProvider == "glm" {
		c.VisionModel = env("GLM_VISION_MODEL", "glm-4.6v")
	}
	if c.VisionProvider == "deepseek" {
		c.VisionModel = env("DEEPSEEK_VISION_MODEL", "deepseek-flash")
	}
	if c.VisionProvider == "minimax" {
		c.VisionModel = env("MINIMAX_VISION_MODEL", "MiniMax-M3.1-Flash")
	}
	c.VisionKey = env("VISION_API_KEY", c.VisionKey)
	c.VisionURL = strings.TrimRight(env("VISION_BASE_URL", c.VisionURL), "/")
	c.VisionModel = env("VISION_MODEL", c.VisionModel)
	c.ImageDINOModel = strings.TrimSpace(os.Getenv("IMAGE_DINO_MODEL"))
	c.ImageDINOPython = strings.TrimSpace(os.Getenv("IMAGE_DINO_PYTHON"))
	if c.ImageDINOModel != "" && !filepath.IsAbs(c.ImageDINOModel) {
		c.ImageDINOModel = filepath.Join(root, c.ImageDINOModel)
	}
	if c.ImageDINOPython != "" && strings.ContainsRune(c.ImageDINOPython, filepath.Separator) && !filepath.IsAbs(c.ImageDINOPython) {
		c.ImageDINOPython = filepath.Join(root, c.ImageDINOPython)
	}
	candidateLimit, err := strconv.Atoi(env("IMAGE_VISUAL_CANDIDATES", strconv.Itoa(maxVisualCandidates)))
	if err != nil || candidateLimit < 1 || candidateLimit > maxVisualCandidates {
		return Config{}, fmt.Errorf("IMAGE_VISUAL_CANDIDATES 需介于 1 和 %d", maxVisualCandidates)
	}
	c.ImageVisualCandidates = candidateLimit

	c.BasePath = strings.TrimRight(env("PUBLIC_BASE_PATH", ""), "/")
	if !regexp.MustCompile(`^(/[A-Za-z0-9_-]+)*$`).MatchString(c.BasePath) {
		return Config{}, fmt.Errorf("PUBLIC_BASE_PATH 需为 /clinical-memory 形式的路径")
	}
	c.DataDir, _ = filepath.Abs(c.DataDir)
	c.LocalOrganizerModel = env("LOCAL_ORGANIZER_MODEL", "")
	c.LocalOrganizerCLI = env("LOCAL_ORGANIZER_CLI", "llama-cli")
	c.LocalASRModel = env("LOCAL_ASR_MODEL", filepath.Join(c.DataDir, "models", "ggml-base.bin"))
	c.OCRDataDir = env("OCR_DATA_DIR", filepath.Join(c.DataDir, "models", "tessdata"))
	return c, nil
}
func (c Config) MySQLConfig() *mysql.Config {
	m := mysql.NewConfig()
	m.User = c.DBUser
	m.Passwd = c.DBPassword
	m.DBName = c.DBName
	m.Net = "tcp"
	m.Addr = net.JoinHostPort(c.DBHost, c.DBPort)
	if c.DBSocket != "" {
		m.Net = "unix"
		m.Addr = c.DBSocket
	}
	m.ParseTime = true
	m.Loc = time.UTC
	m.Timeout = 5 * time.Second
	m.ReadTimeout = 20 * time.Second
	m.WriteTimeout = 20 * time.Second
	m.Params = map[string]string{"charset": "utf8mb4"}
	return m
}
