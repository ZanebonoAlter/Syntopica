<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与变更

- **入口**：Windows 安装后从开始菜单/桌面快捷方式启动壳窗口；壳加载既有 SPA（远程模式），应用内导航、页面结构、交互模式**零改动**。
- **本 change 唯一新增的窗口级界面**：
  1. 窗口首帧背景（消除 WebView 加载前的白闪，背景色对齐当前主题）；
  2. 连接失败页（仅在远端不可达/超时时出现）。
- 不新增页面路由、不改变信息架构、不改变任何页面的布局模式。

## 受影响状态

| 状态 | 出现条件 | 表现 | 承接方 |
| --- | --- | --- | --- |
| loading（窗口首帧） | 窗口创建 → 远端 HTML 到达 | 窗口底色 = 主题背景色（editorial 米白 / dark 近黑），无白闪 | 壳窗口 `backgroundColor` |
| loading（应用内） | HTML 到达后 JS 挂载前 | 复用现有首屏模板（spinner + 拟真进度 + 短句），**不新增** | `front/app/spa-loading-template.html`（既有） |
| error（连接失败） | 远端不可达 / 超时 / 非 2xx | 壳内静态失败页：图标 + 一句原因 + 「重试」+「修改地址」 | 本 change 新增（壳内静态页） |
| success | 远端可达 | SPA 完整功能，与浏览器访问一致 | 既有前端 |
| empty | 不适用 | — | — |

## 复用组件与布局模式

- **布局模式**：失败页属全屏兜底，**不走** `AppPageShell` 四模式（与 `standard/frontend/loading-experience.md` 对「全屏 fallback 不属于 page shell」的口径一致）；单列居中，内容宽度沿用 `AppDialog` sm 档的 420px 视觉口径，不做自由宽度。
- **视觉约定复用**：失败页沿用 `front/app/error.vue` 的既有全屏兜底形态（居中单列 + `mdi:alert-circle` + 重试按钮），但实现为壳内独立静态 HTML（不引入 Vue/Nuxt 运行时依赖）。
- **主题令牌**：窗口背景色与失败页底色从 `front/app/assets/css/main.css` 的语义层令牌取值后**内联复制**（与 `spa-loading-template.html` 同一维护方式；改主题色时两处需同步）。
- 不新增 Vue 组件、不新增图标资源（`mdi:alert-circle` 以 inline SVG 内联，避免壳内联网取图标）。

## 验收映射

- **人工：Windows 平台**（证据记入 `ui-verification/report.md`）：
  - A-1 双击安装包安装（per-user，无管理员提示）→ 启动 → 窗口先见主题底色、随后 SPA 首屏模板、最终进入阅读页；
  - A-2 服务端停机（或改错地址）→ 见连接失败页（文案/图标/两个按钮可见）→ 点「重试」恢复；
  - A-3 关闭窗口后重开 → 无残留进程、地址配置保持上一次值。
- **人工：Linux/arm64 平台**（证据记入同一文件）：启动窗口渲染正常（无整窗花条纹）——2026-11-02 探针已确认 `WEBKIT_DISABLE_DMABUF_RENDERER=1` 下阅读页 / 日报页 / 叙事工坊矩阵页均正常渲染（62fps / p95 17ms）。
- **双视口检查**：1440×900 与 1920×1080 下失败页居中、不出现横向滚动（人工 + 截图）。
- 无组件测试：壳内不含 Vue 组件，失败页为静态 HTML；断言以人工验证 + 截图为准，按 ui-verification 报告格式建账。
