## Context

- 现有前端是纯客户端 SPA（`ssr:false`），静态产物已由后端同端口托管（`backend-go/internal/app/static.go` 的 `spaFallback`），生产构建用 `NUXT_PUBLIC_API_BASE=/api`（见 `scripts/dev/deploy-frontend.sh`），即**同源相对**。
- 2026-11-02 在 Pi 上完成 Tauri 2 探针：Tauri 2.12.5 + WebKitGTK 2.54 可成功编译并渲染现有 SPA（阅读页/日报页/矩阵页），但**默认 GL 路径整窗花屏**，`WEBKIT_DISABLE_DMABUF_RENDERER=1` 下 62fps / p95 17ms 正常；`LIBGL_ALWAYS_SOFTWARE=1` 也正常但更慢（60.2fps / p95 21ms）。
- 目标主力平台是 Windows（WebView2/Chromium），Linux 与 macOS ARM 只需可用。
- 仓库尚无任何 Tauri 工程文件（无 `tauri.conf.json` / `Cargo.toml`），`desktop/` 目前只有 `lab/shell_sim.py` 与 README。

## Goals / Non-Goals

**Goals**

- 用最少的代码量拿到一个 Windows 可安装、可双击启动、行为与浏览器一致的桌面入口。
- 把这些高风险点一次性验证掉：壳编译与打包链路、WebView2 渲染、首帧体验、远端不可达的用户可恢复路径。
- 让壳的启动边界（进程 spawn 位置）为后续 sidecar 形态留出扩展点，但本 change 不实现 sidecar。

**Non-Goals**

- 不做本地离线能力（无 sidecar、无 embedded PG、无素材包同步）。
- 不改前端与后端一行代码（同源 `/api` 已满足）；不做导航裁剪与客户端专属向导。
- 不做自动更新、托盘常驻、协议深链、代码签名。

## Decisions

### D1. 采用「远程 URL 装载」而非把前端打进安装包

- **结论**：壳窗口直接导航到配置的服务端地址（默认 `http://127.0.0.1:5100/`），前端产物继续由后端静态托管；安装包内不打包前端资源。
- **落点**：`desktop/shell/tauri.conf.json`
- **取舍**：打包进 `resources` + 自定义协议被否——需引入 `window.__SYNTOPICA_API__` 注入链、改 `front/app/utils/api.ts`、处理 SPA 路径 fallback，多 2~3 天工作量，而它解决的是「离线可用」，本 change 明确不做离线。

### D2. 前端零改动，不引入 apiBase 注入链

- **结论**：因为壳加载的是**同源页面**，`getApiBaseUrl()` 返回的 `/api` 直接命中同一 origin，无需任何注入。
- **落点**：`front/app/utils/api.ts`
- **取舍**：`window.__SYNTOPICA_API__` 注入方案被推迟到引入本地 sidecar 的 change（那时后端端口动态、页面可能来自本地资源，才需要注入）。

### D3. 连接失败页实现为壳内静态 HTML

- **结论**：失败页是 `desktop/shell/ui/error.html`（内联样式与 inline SVG 图标），由壳在导航失败时加载；重试与改地址通过 Tauri 命令回到 Rust 侧。
- **落点**：`desktop/shell/ui/error.html`
- **取舍**：复用 `front/app/error.vue` 被否——它需要 Nuxt/Vue 运行时，而失败页存在的唯一场景正是「远端不可达、拿不到前端产物」。

### D4. Linux 启动前强制 `WEBKIT_DISABLE_DMABUF_RENDERER=1`

- **结论**：在创建窗口之前（`main()` 起始处）按 `cfg(target_os)` 设置该环境变量；Windows/macOS 不设置任何渲染开关。
- **落点**：`desktop/shell/src/main.rs`
- **取舍**：`LIBGL_ALWAYS_SOFTWARE=1` 被否——虽然也能消除花屏，但把整个渲染退到 CPU（实测 p95 21ms vs 17ms），而关 DMABUF 只影响零拷贝路径；交给用户手动设置被否——要求「适配对用户不可见」。

### D5. WebView2 走 Evergreen，不内嵌固定版本

- **结论**：安装包不携带 WebView2 固定版；启动时若缺失，壳提示并给出 Evergreen 安装引导。
- **落点**：`desktop/shell/tauri.conf.json`
- **取舍**：Fixed Version 内嵌（+约 120MB，可离线）被否——与「安装包 ≤150MB」目标冲突，且 Win11 自带、Win10 绝大多数已装。

### D6. Windows 安装包用 NSIS per-user

- **结论**：`nsis` 目标、`installMode: currentUser`，免管理员。
- **落点**：`desktop/shell/tauri.conf.json`
- **取舍**：MSI/WiX 被否——per-machine 安装需管理员，且 WiX 工具链更重；本 change 无企业分发需求。

### D7. 窗口首帧背景色内联复制主题令牌

- **结论**：`tauri.conf.json` 的窗口 `backgroundColor` 取 `front/app/assets/css/main.css` 语义层的背景色值，失败页同样内联该值，改主题色时同步（与 `spa-loading-template.html` 同一维护方式）。
- **落点**：`desktop/shell/tauri.conf.json`
- **取舍**：运行时从远端读取主题再设置被否——首帧早于任何远端请求，来不及。

### D8. 开发/验证在 Linux 本机，出包在 Windows 本机

- **结论**：壳的开发与渲染验证在现有 Linux（arm64）机器上完成（可编译、可截图取证），Windows 安装包在目标 Windows 机器上执行 `tauri build` 产出。
- **落点**：`desktop/README.md`
- **取舍**：在 Linux 上交叉编译 Windows 产物（`cargo-xwin`）被否——签名、NSIS、WebView2 检测都要回 Windows 验证，交叉链路收益低、维护成本高。

## Risks / Trade-offs

- [远端为明文 HTTP，局域网可接受、公网不可接受] → README 明确「仅限可信局域网/本机」，公网访问需前置 TLS 反代（后续 change）。
- [远程模式下浏览器安全上下文限制：`navigator.clipboard`、`crypto.subtle` 等 API 在 `http://<lan-ip>` 属非安全上下文不可用] → 本 change 先记录：实现时逐一核对 SPA 用到的受限 API，若有依赖则在失败页之外增加提示；根治（HTTPS/本地 sidecar 起源）放到后续 change。（当前唯一已知用途 `crypto.getRandomValues` 不受限。）
- [WebView2 Runtime 缺失导致白窗] → 启动前检测并引导（Scenario「缺少 WebView2 Runtime」）。
- [主题色与 SPA 令牌漂移] → 两处内联值在文件头互指注释，改令牌时同步检查。
- [单实例在多用户切换/远程桌面下行为差异] → 以「同一用户会话内单实例」为验收口径，跨用户会话不作为本 change 要求。
- [壳与 SPA 双份加载态风格不一致] → 壳只负责窗口底色，应用内加载态完全交给既有首屏模板，避免两套进度动画。

## Migration Plan

1. 在 Windows 目标机安装 Rust（MSVC 目标）+ VS Build Tools + WebView2 Runtime（多数已具备）。
2. `desktop/shell` 下 `tauri build` 产出 `*.exe`（NSIS per-user）。
3. 用户双击安装 → 开始菜单入口；首次启动填/确认服务端地址。
4. 回滚：卸载应用即可；服务器与数据完全不涉及，无迁移、无回滚脚本。

## Open Questions

- 自动更新通道（Tauri updater vs 手工下发安装包）——等壳真正对外分发时再定，不影响本 change 的 specs 与任务拆分。
- 托盘常驻与后台同步——与本地 sidecar 形态一起设计，本 change 保持「关窗即退出」。
