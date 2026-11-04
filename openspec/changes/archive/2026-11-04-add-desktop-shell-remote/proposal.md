<!-- complexity: complex -->
<!-- ui-impact: minor -->

## Why

现有产品只有服务器装法（浏览器访问 `http://<host>:5100/`）。用户主力机是 Windows，需要「双击即用」的桌面应用入口；这也是桌面装法（Tauri 壳 + Go sidecar + embedded PG）的第一步——壳、WebView2/WebKitGTK 渲染、Windows 打包是后续所有客户端工作共用的底座，先用最小可用版本把这些最容易翻车的部分一次验证掉。

## What Changes

- 新增 `desktop/shell/`：Tauri 2 最小可运行壳，窗口加载现有 SPA 静态产物（**远程模式**，默认 `http://127.0.0.1:5100/`，可由配置/环境切换到局域网或其他可达地址），同源 `/api` 零改动、零 CORS。
- 窗口级体验（minor UI）：
  - 窗口背景色对齐双主题（editorial / dark），消除 WebView 首帧白闪；
  - 远端不可达时的**连接失败页**（说明 + 重试 + 改地址入口），复用 `error.vue` 全屏兜底页的视觉约定，不引入新的信息架构；
  - 单实例（重复启动聚焦已有窗口）。
- 平台渲染适配：Linux/arm64 强制 `WEBKIT_DISABLE_DMABUF_RENDERER=1`（2026-11-02 实测：默认 GL 路径整窗花屏，关 DMABUF 后 62fps/p95 17ms 正常）；Windows 用 WebView2 默认配置，不做额外降级。
- Windows 分发：NSIS per-user 安装包（免管理员），WebView2 走 Evergreen + 首启检测提示（不内嵌 Fixed Version）。
- 构建与文档：`desktop/README.md`（开发/构建/打包命令、地址配置、平台注意事项）。
- **不含**：Go sidecar 进程管理、embedded PG、素材包同步协议（迭代 0 的 0-5/0-6/0-7 与迭代 2，另行 change）。壳骨架按「后续可加 sidecar」预留 spawn 边界，但本 change 不实现。

## Capabilities

### New Capabilities

- `desktop-shell`: Tauri 壳的窗口装载契约（远程地址 + 同源 API）、连接失败兜底、平台渲染适配（Linux DMABUF 关闭 / Windows WebView2）、单实例，以及 Windows NSIS 分发要求。

### Modified Capabilities

（无）

## Impact

- **新增代码**：`desktop/shell/`（`Cargo.toml`、`src/main.rs`、`tauri.conf.json`、兜底页静态资源）、`desktop/README.md`；构建/打包脚本按需置于 `desktop/shell/` 内。
- **依赖**：Rust ≥1.99（实测 1.99.1）、Tauri 2.12.5、Node/pnpm（前端产物已存在，本 change 不构建前端）、Windows 侧 WebView2 Runtime、Linux 侧 webkit2gtk-4.1 + GTK3 dev。
- **不改动**：`backend-go/`、`front/`（零改动——加载既有静态产物，`NUXT_PUBLIC_API_BASE=/api` 同源契约已存在）。
- **部署后用户可见变化**：Windows 多一个桌面应用入口，功能与浏览器访问同一服务器完全一致；服务器部署形态与数据不变。
- **旧数据降级**：不涉及数据库与数据迁移。
- **已知限制（本 change 刻意保留）**：远程模式依赖服务端可达，断网/服务端停机时只能显示连接失败页；本地离线能力由后续 change（sidecar + embedded PG）提供。
