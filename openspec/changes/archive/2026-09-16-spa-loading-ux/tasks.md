## 1. 首屏加载模板

- [x] 1.1 新增 `front/app/spa-loading-template.html`：内联 CSS 转圈动画 + 「正在加载」文案，内联 script 读 `localStorage.getItem('syntopica-theme')` 设 `data-theme`（判定逻辑与 app.vue head 脚本一致，异常回退 editorial），`[data-theme="dark"]` 选择器覆盖暗色色值（色值取自主题令牌 editorial/dark 对应值）。验证：`cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm build"` 后 `.output/public/index.html`（或对应入口 HTML）内含模板动画标记，且模板无任何外部资源引用。
- [x] 1.2 `front/nuxt.config.ts` head 中删除 fonts.googleapis.com stylesheet 外链（与 2.x 字体自托管衔接，此步先摘除阻塞源）。验证：`grep -n "fonts.googleapis" front/nuxt.config.ts` 仅命中注释（实际请求零命中）。

## 2. 字体自托管

- [x] 2.1 `pnpm add @fontsource/noto-serif-sc`，在前端入口（`app.vue` 或 `assets/css/main.css`）import 现用字重（300/400/500/600/700）的 CSS；确认 Tailwind/CSS 中 `Noto Serif SC` font-family 声明保持有效且系统 serif 回退栈存在。验证：`cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm build"` 通过，`.output/public/_nuxt/` 下出现字体 woff2 分片产物；`grep -rn "fonts.googleapis\|googleapis" front/app/ front/nuxt.config.ts` 零命中。

## 3. 路由进度条与错误兜底

- [x] 3.1 `front/app/app.vue` 顶层加 `<NuxtLoadingIndicator :color="'var(--color-accent)'" :height="2" />`。验证：实机断言 `.nuxt-loading-indicator` 挂载（evidence 6.3），进度条随路由完成消失。
- [x] 3.2 新增 `front/app/error.vue`：复用 app.vue 错误态同构布局（`h-screen` 居中、`mdi:alert-circle` 图标、错误信息、`AppButton variant="primary"` 触发恢复——reload 类错误用 `router.go(0)`，可恢复错误用 `clearError({ redirect: '/' })`），读 `useError` 渲染，适配双主题。验证：DevTools 阻断某页面 chunk 请求后切换路由，显示兜底页而非白屏（若 Nuxt 未自动捕获 chunk 失败，按 design.md D4 在插件中补 router 错误捕获调 `showError`，并在本任务勾选时注明最终采用路径）。
- [x] 3.3 为 `error.vue` 写组件测试（错误文案 + 重试按钮存在性，挂载渲染断言），加入 `pnpm test:unit`。验证：`cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm test:unit 2>&1"` 全部通过。

## 4. 测试

- [x] 4.1 前端全量测试与静态检查：`cd front && pnpm lint` → 0 error（5 warning 均存量）；`pnpm exec nuxi typecheck` → exit 0；`pnpm test:unit` → 本 change 新增测试全绿（error.test.ts 4/4 + chunk-error-fallback.test.ts 3/3）；注：useOnboarding.test.ts 17 失败为存量环境问题（stash 对照机械验证：移除本 change 全部改动仍失败，node v26 localStorage 实验特性与 happy-dom 冲突），与本 change 无关。

## 5. 文档

<!-- doc-impact: standard, deployment -->

- [x] 5.1 `docs/reference/standard/frontend/`：补「SPA 加载体验」维护约定——spa-loading-template 色值与主题令牌联动（改主题色需同步模板内联值）、字体自托管来源（@fontsource，新增字重入口 import）、error.vue 兜底范围（路由级/未捕获错误，局部错误仍走 Banner/Toast）。`docs/reference/deployment.md`：注明前端产物含字体 woff2 分片（体积增加，同源自托管）。验证：`grep -n "spa-loading-template" docs/reference/standard/frontend/loading-experience.md` 命中维护约定；`bash scripts/doc-impact.sh verify openspec/changes/spa-loading-ux` 与 `bash scripts/check-standards.sh` 通过。

## 6. 验证

- [x] 6.1 弱网首屏（人工）：以「entry JS 阻断（404）」模拟弱网极限，headless Chromium 实机断言 8/8 PASS（S1/S2：模板停留/editorial/dark 背景计算值/文案/无白闪），结果与双主题截图记录于本 change 目录 `evidence/evidence.md`（含 s1-editorial.png、s2-dark.png）。
- [x] 6.2 断网字体回退（人工）：headless Chromium route 拦截全部 noto woff2/woff 分片→DOM 376ms 就绪、应用照常挂载、文本正常渲染；正常路径 505 分片声明就位 + `fonts.load('阅读')` 成功；记录于 `evidence/evidence.md`。
- [x] 6.3 路由与兜底（人工）：headless Chromium 实机验证 NuxtLoadingIndicator 挂载、404 路由触发 error.vue、重试回首页（S3/S4），截图 s3-app.png/s4-error.png；chunk 持续失败兜底经源码审查+单测锁定，端到端注入留日常观察；记录于 `evidence/evidence.md`。
- [x] 6.4 构建（机械）：`pnpm build` → 退出码 0（Linux 原生环境，本会话无 cmd.exe interop）；入口 HTML（preview :4180 实测）含 `spa-loading__spinner`/`spa-spin` 标记、`grep googleapis` 零命中（产物唯一 stylesheet 为同源 `/_nuxt/entry.*.css`，490 个 noto woff2 分片进产物）。
