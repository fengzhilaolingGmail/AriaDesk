# AriaDesk

基于 [aria2](https://github.com/aria2/aria2) 的 Windows 桌面下载管理器，支持浏览器插件联动，一键接管浏览器下载。

## 功能特性

**下载管理**
- 支持 HTTP/HTTPS 直链、磁力链接、BT 种子任务
- 任务暂停 / 恢复 / 删除，删除时可选同时清理已下载文件（含 `.aria2` 续传数据）
- 实时速率统计、任务列表分页、下载历史本地持久化
- 一键定位已下载文件（多文件种子自动定位到任务目录）

**aria2 一体化管理**
- 自动检测 / 一键安装 aria2，无需手动配置环境
- aria2 版本检查、在线升级、回退与卸载
- 卡片式 aria2.conf 配置页，常用参数可视化修改
- 每次启动自动更新公共 Tracker 列表，提升 BT 下载速度
- 下载目录、同时下载数等设置即时生效，无需重启服务

**浏览器插件联动（Chrome / Edge）**
- 自动接管浏览器下载，推送到 AriaDesk 由 aria2 高速下载
- 支持磁力链接与种子推送（种子由插件携带 Cookie 抓取，需登录站点也能下）
- 可选携带 Cookie / Referer / User-Agent，绕过防盗链限制
- 推送结果页面弹窗反馈，下载完成后浏览器端通知

**其他**
- 系统托盘常驻、开机自启、关闭最小化到托盘
- 下载完成 Windows 系统通知
- 明暗主题自适应

## 下载与安装

从 [Releases](https://github.com/fengzhilaolingGmail/AriaDesk/releases) 页面下载：

| 文件 | 说明 |
|---|---|
| `AriaDesk-amd64-installer.exe` | Windows 安装包（推荐） |
| `AriaDesk.exe` | 免安装单文件版 |
| `ariadesk-extension-x.y.z.zip` | 浏览器插件包 |

## 使用说明

### 桌面端

1. 安装并启动 AriaDesk，首次使用若未检测到 aria2，按首页引导一键安装
2. 顶部状态栏显示「已连接」即表示 aria2 服务正常
3. 点击「新建任务」粘贴下载链接，或通过「管理」页添加种子文件
4. 在「设置」页可修改下载目录、同时下载数、RPC 端口等

### 浏览器插件

1. 解压插件 zip 包
2. 浏览器打开 `chrome://extensions`，开启「开发者模式」
3. 点击「加载已解压的扩展程序」，选择解压目录
4. 打开 AriaDesk「设置」页，在浏览器插件区域显示配对 token
5. 将 token 填入插件的设置页，完成配对
6. 之后浏览器下载会自动推送到 AriaDesk；右键链接 / 种子也可手动推送

> 插件与桌面端通过本机 `127.0.0.1:20000~20005` 端口通信，token 配对后可正常推送。

## 从源码构建

### 环境要求

- Go 1.23+
- Node.js 18+
- Wails CLI v2（`go install github.com/wailsapp/wails/v2/cmd/wails@latest`）
- （可选）NSIS 3.x，打包 Windows 安装包时需要

### 开发调试

```bash
wails dev
```

### 编译

```bash
# 免安装单文件版，输出 build/bin/AriaDesk.exe
wails build
# 或使用脚本
powershell -ExecutionPolicy Bypass -File scripts\build-app.ps1

# Windows 安装包（需要 NSIS），输出 build/bin/AriaDesk-amd64-installer.exe
wails build -nsis

# 浏览器插件 zip，输出 build/bin/ariadesk-extension-<version>.zip
powershell -ExecutionPolicy Bypass -File scripts/pack-extension.ps1
```

## 项目结构

```
├── main.go / app.go      # Wails 入口与后端 API
├── internal/
│   ├── aria2/            # aria2 检测、安装、版本管理与配置文件同步
│   ├── rpc/              # aria2 JSON-RPC 客户端
│   ├── extsrv/           # 浏览器插件推送服务（127.0.0.1 本地 HTTP）
│   ├── history/          # 下载历史本地持久化
│   ├── config/ secret/   # 设置与 token 存储
│   └── tray/ autostart/  # 系统托盘与开机自启
├── frontend/             # Vue 3 + Vite + Element Plus + Pinia
├── extension/            # 浏览器插件（Manifest V3）
└── scripts/              # 编译与打包脚本
```

## 数据目录

应用数据存放于 `%APPDATA%\AriaDesk\`：

- `settings.json` — 应用设置
- `history.json` — 下载历史
- `token.pass` / `ext.pass` — RPC 密钥与插件配对 token（加密存储）
- `aria2/aria2.conf`、`aria2/aria2.log`、`aria2/session.txt` — aria2 配置、日志与任务会话

## 支持项目 / 赞助

如果 AriaDesk 对你有帮助，欢迎请作者喝杯咖啡 ☕

| 微信 | 支付宝 |
|:---:|:---:|
| <img src="images/wx.png" width="200" alt="微信收款码"> | <img src="images/zfb.jpg" width="200" alt="支付宝收款码"> |

## 开源协议

[Apache License 2.0](LICENSE) · 作者：风之老凌
