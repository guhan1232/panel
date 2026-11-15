# 寒语馨面板 · 脚本仓库（jiaoben 分支）

> 面板「软件商店 → 脚本应用」的脚本托管分支。面板从本分支的 `catalog.json` 读取脚本清单，
> 用户在面板里浏览、填参数、一键安装执行。

## 目录结构

```
jiaoben/
├── catalog.json     # 脚本清单（面板读取的入口，remoteApp 数组）
├── README.md        # 本文档（规范）
└── scripts/
    ├── hello-test.sh        # 连通性测试
    ├── system-tune.sh       # 系统基础优化
    ├── nginx-log-clean.sh   # Nginx 日志轮转清理
    └── ddns-cloudflare.sh   # Cloudflare DDNS
```

## 清单格式（catalog.json）

顶层为 remoteApp 数组，每条：

| 字段 | 必填 | 说明 |
|---|---|---|
| `key` | ✓ | 全局唯一标识（小写字母数字连字符） |
| `name` | ✓ | 显示名 |
| `desc` | ✓ | 一句话说明 |
| `kind` | ✓ | 固定 `script` |
| `version` | 建议 | 语义化版本 |
| `script` | ✓ | 脚本 URL（推荐指向本分支 raw 地址），也可内嵌完整 bash 内容 |
| `params` | 可选 | 参数声明数组：`{key, label, def}` |

```json
[
  {
    "key": "my-app",
    "name": "我的应用",
    "desc": "一句话说明",
    "kind": "script",
    "version": "1.0.0",
    "script": "https://cnb.cool/hanyuxin/panel/-/git/raw/jiaoben/scripts/my-app.sh",
    "params": [
      { "key": "domain", "label": "域名", "def": "example.com" }
    ]
  }
]
```

## 脚本编写规范

1. **bash + `set -e`**：出错立即退出（非 0 退出码 = 安装失败）。
2. **参数占位符**：`{{参数key}}`，安装时被面板替换为用户填写的值；脚本内同时可用环境变量 `PANEL_APP_NAME`（实例名）。
3. **幂等性**：重复执行不应破坏系统（先检查再操作）。
4. **输出留痕**：关键步骤 echo——面板把执行输出写入 `data/appstore/scripts/<实例>.sh.log`，可在面板查看。
5. **敏感信息**：密码/Token 类参数在面板命令审计中自动掩码；脚本内不要把凭据写入公共文件。
6. **卸载**：脚本类应用卸载 = 删除脚本与登记记录；如需反向清理，请在脚本里用标记文件判断（如 `/var/lib/<app>.installed`）自行支持。

## 如何添加新脚本

1. 在 `scripts/` 新建 `脚本名.sh`（小写字母数字连字符）。
2. 在 `catalog.json` 追加一条清单（`script` 字段指向 raw URL）。
3. 提交到本分支（jiaoben）→ 合并回 main（git checkout main && git merge jiaoben && git push）→ 面板「应用源刷新」后即可看到并安装。
   > 说明：CNB 平台 raw 文件端点仅对默认分支（main）生效，因此面板读取的是 main 上的 jiaoben/ 目录；jiaoben 分支作为脚本开发工作区。

## 面板侧行为

- 应用源默认地址：`https://cnb.cool/hanyuxin/panel/-/git/raw/main/jiaoben/catalog.json`（可在面板「应用源」处覆盖/清除）。
- 安装 = 拉取脚本 → 渲染参数占位符 → 落盘 `data/appstore/scripts/<实例>.sh` → bash 执行（限时 30 分钟）→ 输出留痕。
- 自定义脚本走「上传脚本」，同样受 30 分钟限制；两者都可卸载。
