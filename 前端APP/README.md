# Android Java 原生客户端

用 Android Studio 打开本目录，等待 Gradle 同步，选择设备，点击 Run。源码全部为 `.java`，构建脚本使用 Groovy `.gradle`；没有 Kotlin 业务代码或 Kotlin DSL。

- JDK 17+，已用 Android Studio 自带 JDK 21 编译。
- Android SDK 33，最低设备 API 26 / Android 8.0，AGP 9.2.1，Gradle Wrapper 9.4.1。
- `./gradlew assembleDebug` 生成 `app/build/outputs/apk/debug/app-debug.apk`。
- 模拟器连接后端 `http://10.0.2.2:8000`；真机使用电脑局域网 IP，Go 后端需监听 0.0.0.0。
- Debug 支持本地 HTTP，Release 仅允许 HTTPS。发布前升级 targetSdk、配置正式签名。

MainActivity.java：原生页面、笔记管理、材料、检索、对话、录音与文件选择。
Api.java：JSON / multipart HTTP 请求。
TokenStore.java：Android Keystore 加密保存会话，不保存密码。
CacheProvider.java：给本机查看器临时授权读取已鉴权下载的原件，不传递后端令牌。
Json.java：JSON 字段辅助。

软件账户与 Web 相同。APP 只连 Go HTTP 接口，不直连 MySQL、不保存 DeepSeek 密钥。MySQL 只需对本机后端开放。

首页的“试用图片 / 录音示例”可直接选择随 APP 附带的六个固定测试文件，使用预置文本检索；真实新文件仍需视觉 / ASR 服务或人工补充。
