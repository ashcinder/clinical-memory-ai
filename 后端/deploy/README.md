# 腾讯云部署与运维

> 本文为已有公网实例的历史部署记录。当前分支的新部署请使用 [内网部署说明](intranet/README.md) 和内网 Nginx/systemd 模板；不要将本文的公网 IP、证书路径或 ACME 续期方式直接复制到内网。

当前实例入口：`https://124.221.183.247/clinical-memory/`。Android 服务地址填写 `https://124.221.183.247/clinical-memory`，无需追加 `/api`。

使用独立系统用户 `clinicalmemory`、MySQL 业务库 `clinical_memory`、数据库账户 `clinicalmemory@localhost`。数据库与 18090 应用端口仅供本机访问。用户选择全新数据库，初始凭据仅在本机 `后端/output/云端访问信息.txt`，不进 Git。

## 目录

- `/srv/clinical-memory/current`：当前代码版本链接。
- `/srv/clinical-memory/releases/20261028-01`：Go 二进制、Web 和源码。
- `/srv/clinical-memory/tools/bin/whisper-cli`：whisper.cpp v1.9.4 CPU 引擎。
- `/var/lib/clinical-memory`：上传材料、账户模型配置、下载模型。
- `/etc/clinical-memory/clinical-memory.env`：部署环境配置（root 0600）。
- `/etc/systemd/system/clinical-memory.service`：开机自启与故障重启。
- `/etc/nginx/project-locations/clinical-memory.conf`：本项目独立路径。
- `/etc/nginx/sites-available/project-gateway`：80/443 共享项目入口。

## 运维命令

```bash
sudo systemctl status clinical-memory
sudo systemctl restart clinical-memory
sudo journalctl -u clinical-memory -n 100 --no-pager
curl https://124.221.183.247/clinical-memory/api/health
sudo nginx -t
sudo systemctl reload nginx
```

更新代码时创建新的 `releases/版本`，检查后切换 `current` 链接并重启服务；数据目录与环境配置独立于代码。回退时恢复旧链接。

后续项目可增加 `/另一个项目/` 及独立本机端口，并在 `/etc/nginx/project-locations/` 新增 location 配置，避免占用本项目接口路径和 Cookie。未来使用独立域名也可新增 Nginx server 块。

## HTTPS

使用 IP 地址短期证书，证书文件在 `/etc/letsencrypt/live/124.221.183.247/`，`snap.certbot.renew.timer` 自动检查续期。云侧入站 80/443 必须允许公网访问，ACME 验证不能只允许个人 IP。

2026-10-28 实测：webroot 的正式校验仍间歇性返回连接拒绝；改用 Certbot standalone 后，正式证书成功签发，公网已提供新证书。自动续期配置在 `/etc/letsencrypt/renewal/124.221.183.247.conf`，使用 `authenticator = standalone`、`pre_hook = systemctl stop nginx`、`post_hook = systemctl start nginx`。每次实际续期会短暂停止共享 Nginx 入口，完成或校验失败后均执行启动钩子；Go 与 MySQL 保持运行。后续部署到此 Nginx 的其他项目也会受到这次短暂中断。

本次证书到期时间为北京时间 2026-11-04 01:36:50，后续由定时任务提前续期。保留 `/var/www/acme` 及现有 HTTP challenge 路由，当前续期由 standalone 接管 80 端口。

验证命令：

```bash
sudo certbot renew --cert-name 124.221.183.247 --dry-run
sudo systemctl list-timers snap.certbot.renew.timer
```

## 本地识别

OCR 使用 Tesseract 简体中文 + 英文；录音使用 FFmpeg 解码与 whisper.cpp base 多语言模型（≤120秒）。API 密钥默认未配置，云端搜索默认关闭。系统从材料页重新提取即可使用本地识别。识别文字可能错误，尤其穴位编号，需核对。

## 验证记录

本机 `后端/output/云端部署测试.json`、`cloud-restart-test.json` 与 `云端部署测试报告.md` 记录公开 HTTPS、会话、笔记、图片指纹、OCR、录音转写、原件访问和重启测试。测试记录与材料完成后清理，线上账户保留为空库。
