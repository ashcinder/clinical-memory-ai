# 原生 Web 客户端

HTML、CSS、JavaScript，无 Node/npm 构建依赖。

启动方法：

1. 启动 MySQL 和 Go 后端。
2. 直接访问 http://127.0.0.1:8000；后端已托管 Web 文件。
3. 如需独立启动，运行本目录 `bash run.sh`，访问 http://127.0.0.1:5173。脚本调用 Go 的静态服务器，将 /api 同源代理到后端。

不要通过 file:// 打开 index.html，也不要用缺少 API 代理的静态服务器。通过 HttpOnly Cookie 登录，不在 localStorage 中存储令牌。动态正文按文本转义展示，AI 回复用纯文本和来源按钮呈现。

localhost / HTTPS 支持 MediaRecorder 录音，需授权麦克风，最长两分钟。单个材料限 20MB。

经验检索的“试用示例”提供固定图片和录音：仅当原始文件 SHA-256 一致时使用预置文本。新文件需要识别服务或手动补充文本。
