# 项目协作规则

用户要求：每次功能、界面或交互修改，都同时更新以下目标，并覆盖 Web 与 Android Java APP：

1. 腾讯云公网服务：https://124.221.183.247/clinical-memory/。
2. 局域网/内网版本源码、Web 部署包和 APP。
3. 本机测试服务：http://localhost:8000/#search，以及对应的本机测试 APP。

当前公网工作目录：`/Users/tangyucinder/.codex/worktrees/public-multiselect/多模态AI数据库检索`。
当前内网与本机工作目录：`/Users/tangyucinder/开发/多模态AI数据库检索`，本机 8000 服务从此目录运行。
若目录变化，先检查 git worktree list 和实际进程，不猜测。

- 修改共用的 Go、Web、Java 业务代码后，运行 `后端/tools/sync_variants.py --target <另一工作目录> --write`，再以只读模式核对。两种 APP 使用同一份 Java 代码，通过 BuildConfig.DEFAULT_SERVER_URL 区分连接地址。
- 各环境的 .env、真实数据库/附件、模型密钥、TLS/CA、Gradle 构建方式和部署网段规则独立保留；不在代码同步时覆盖这些配置。
- 同一功能版本同时生成公网 APP、内网 APP、本机模拟器测试 APP，以及更新的内网部署包。公网版预填公网 HTTPS；内网版保留内部地址/CA 能力；本机测试版仅用于 debug，默认 10.0.2.2:8000。
- 功能改动完成前，验证公网资源与接口、本机 8000 新功能，并检查内网包/APP 的版本与构建；不要把源码已复制表述成内网实际服务器已上线。
- 对本机已有数据升级前保留备份；测试新建/删除用独立测试账户或数据库，不删除真实笔记。
- 用户已授权日常将项目修改同步部署到上述目标。除非任务明确改变范围，不再只更新单一分支或只更新 Web。
- 最终说明三个目标的实际更新情况及未完成项，提供新版 APP 的位置。
