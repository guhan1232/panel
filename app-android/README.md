# 寒语馨面板 · Android APP

原生 Android 客户端（Kotlin + Jetpack Compose，无 GMS 依赖）。设计文档见 [docs/APP.md](../docs/APP.md)。

## 功能

- **扫码登录**：扫 Web 登录页「扫码登录」二维码 —— 一次扫码完成「添加服务器 + APP 登录 + Web 免密登录」
- **多服务器管理**：面板地址 + 安全入口 + 账密（含 2FA），会话令牌 30 天滑动续期
- **仪表盘**：CPU/内存/磁盘/负载/网络实时监控（3s 轮询）
- **网站管理**：列表、启停开关、详情
- **软件商店**：模板一键安装（参数表单）、已装应用升级/卸载（面板依赖保护两段式强卸确认）
- **文件管理**：浏览、上传、下载、新建目录、重命名、删除（回收站）、文本编辑
- **消息中心**：告警事件列表
- **安全**：令牌 EncryptedSharedPreferences 加密存储、自签证书 TOFU 指纹锁定、生物识别锁（指纹/锁屏凭据）

## 构建

要求：JDK 17+、Android SDK（Platform 35 / Build-Tools 35）、Gradle 8.9+。

```bash
# 1. local.properties 指向 SDK（sdk.dir=C\:\\Users\\<user>\\AppData\\Local\\Android\\Sdk）
# 2. release 签名（可选）：keystore.properties + keystore/*.jks（不入库）
storeFile=keystore/<name>.jks
storePassword=...
keyAlias=...
keyPassword=...
# 3. 构建
./gradlew assembleRelease   # 产物 app/build/outputs/apk/release/
```

## 扫码登录协议（与面板 v0.41.0+ 配合）

二维码内容契约：`hanyuxin://login?v=1&host=<面板地址>&entrance=<入口码>&key=<key>&v2=<2fa 0/1>`

APP 端流程：扫码 → 已有该服务器会话则直接 confirm+approve（浏览器免密登录）；否则落库服务器 → 账密登录（含 2FA）→ 自动 confirm+approve。协议细节见 docs/APP.md §5。
