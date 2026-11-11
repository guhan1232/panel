# 脚本应用开发文档（脚本仓库 · jiaoben 分支）

> 面板「软件商店 → 脚本应用」支持两类脚本：**仓库托管**（本仓库 `jiaoben` 分支托管、main 生效）与**自定义上传**。本文档规定脚本与清单的编写规范、开发流程与面板侧行为。

## 1. 存放与生效流程

```
jiaoben 分支（脚本开发工作区）
   │  在 scripts/ 新建脚本 + catalog.json 登记清单
   │  git push origin jiaoben
   ▼
合并回 main（git checkout main && git merge jiaoben && git push）
   ▼
面板读取 main 的 jiaoben/catalog.json（应用源默认地址）
   ▼
「软件商店」出现脚本 → 用户填参数 → 一键安装
```

> **平台限制**：CNB 的 raw 文件端点（`/-/git/raw/<ref>/<path>`）仅对默认分支（main）生效，非默认分支一律 404。因此**面板读取 main**，jiaoben 分支作为开发工作区。

默认应用源地址（可在面板「应用源」处覆盖或清除）：

```
https://cnb.cool/hanyuxin/panel/-/git/raw/main/jiaoben/catalog.json
```

## 2. 清单格式（catalog.json）

顶层兼容两种：`{"apps": [...]}`（推荐，jiaoben 现行）或裸数组。每条应用：

| 字段 | 必填 | 说明 |
|---|---|---|
| `key` | ✓ | 唯一标识，`^[a-z0-9-]+$` |
| `name` | ✓ | 显示名 |
| `desc` | ✓ | 一句话说明 |
| `kind` | ✓ | `script`（脚本类）；`compose`/`native` 为容器/二进制类，另见应用商店文档 |
| `version` | 建议 | 语义化版本 |
| `script` | script 必填 | 脚本 URL（`https://cnb.cool/hanyuxin/panel/-/git/raw/main/jiaoben/scripts/<名>.sh`）或完整 bash 内容 |
| `params` | 可选 | 参数声明：`{key, label, def}`，安装时面板渲染表单 |

## 3. 脚本编写规范

1. **bash + `set -e`**：非 0 退出 = 安装失败。
2. **参数占位符** `{{key}}`：安装时替换为用户填写值；同时注入环境变量 `PANEL_APP_NAME`（实例名）。
3. **幂等**：重复执行不破坏系统（先检查再操作）。
4. **输出留痕**：关键步骤 echo——输出写入 `data/appstore/scripts/<实例>.sh.log`，面板可查看。
5. **敏感信息**：密码/Token 类参数在面板命令审计中自动掩码；不要把凭据写入公共文件。
6. **时长**：执行限时 30 分钟（超时即失败）。
7. **卸载**：脚本类应用卸载 = 删除脚本与登记记录；如需反向清理，用标记文件（如 `/var/lib/<app>.installed`）自行判断。

## 4. 如何新增一个脚本（示例流程）

```bash
# 1. 切到脚本开发分支
git checkout jiaoben

# 2. 写脚本 + 登记清单（catalog.json 追加一条）
vim jiaoben/scripts/my-app.sh
vim jiaoben/catalog.json

# 3. 推送分支
git push origin jiaoben

# 4. 合并回 main 生效
git checkout main && git merge jiaoben && git push origin main
```

面板侧：软件商店 → 应用源刷新（或重启面板）→ 「可安装软件」出现新脚本 → 填参数安装 → 查看日志。

## 5. 面板侧行为要点

- 安装 = 拉取脚本 → 渲染 `{{参数}}` → 落盘 `data/appstore/scripts/<实例>.sh` → bash 执行 → 输出留痕。
- **凭据安全**：密码/Token 类参数在审计记录中自动掩码（`-p/--password/TOKEN=` 等模式）。
- **自定义脚本**（上传即用）与仓库脚本同一执行引擎与日志机制；两者均可卸载。
- **重复实例名**会被拒绝；卸载后可重装。

## 6. 当前内置脚本（示例）

| key | 功能 | 参数 |
|---|---|---|
| hello-test | 连通性测试（无副作用） | note |
| system-tune | 时区/Swap/句柄/内核参数优化 | timezone, swap_size |
| nginx-log-clean | Nginx 日志按大小轮转与过期清理 | max_mb, keep_days |
| ddns-cloudflare | Cloudflare A 记录 DDNS | cf_token, zone, record |
