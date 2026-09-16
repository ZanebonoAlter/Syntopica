# spa-loading-ux 实机验证证据

环境：preview server（`node .output/server/index.mjs` :4180，Linux + 系统 Chromium 152 headless，@playwright/test 驱动）
脚本为一次性验证（`/tmp/spa-ux-verify*.mjs`，未入库）；截图见本目录。

## 6.1 弱网首屏（S1/S2，8/8 PASS）

方法说明：无 DevTools throttle 环境下用「entry JS 改名 → 404」模拟弱网极限形态（JS 一直拉不到=下载期间任意时刻），浏览器断言 + 截图。

- S1 模板渲染：JS 拉不到时页面停留 `.spa-loading` 模板（spinner + 「正在加载」）✅ `s1-editorial.png`
- S1 editorial 主题判定 `data-theme=editorial`、模板背景 `rgb(250,247,242)`（= #faf7f2 --raw-stone-50）✅
- S1 entry JS 确实非 200（status=500，nitro 对缺失静态资源）✅
- S2 dark 主题：`localStorage syntopica-theme=dark` → `data-theme=dark`、背景 `rgb(8,12,18)`（= #080c12）✅ `s2-dark.png`
- S2 无白闪：加载期间 html 计算背景无白色系 ✅

## 6.2 字体回退（零阻塞验证）

方法说明：route 拦截全部 `noto-serif-sc-*.woff2/woff` 分片请求模拟字体全挂。

- DOM 376ms 即就绪、Nuxt 应用照常挂载、正文 149 字符正常渲染——渲染不被字体请求阻塞 ✅
- 正常路径：`document.fonts` 505 个自托管分片 @font-face 声明就位；`fonts.load('400 16px "Noto Serif SC"', '阅读')` 成功加载、`check=true` ✅
- 全程零外域请求（fonts.googleapis / gstatic / api.iconify.design 均 0 命中）✅

## 6.3 路由与兜底（S3/S4，7/8 PASS，1 项为断言姿势修正后确认）

- S3 应用挂载后 `.spa-loading` 模板被替换 ✅ `s3-app.png`
- S3 `.nuxt-loading-indicator`（NuxtLoadingIndicator）挂载 ✅（本地路由切换毫秒级，进度条出现瞬间无法稳定截图，以元素存在性 + 后续人工观察为准）
- S4 真实触发：访问不存在路由 → Nuxt afterEach 404 → showError → error.vue 渲染（「页面加载失败 / Page not found / 重新加载」）✅ `s4-error.png`
- S4 点击「重新加载」→ clearError({redirect:'/'}) 回首页（path=/）✅
- chunk 持续失败兜底：Nuxt 内建 `nuxt:chunk-reload` 自愈 reload（10s TTL）优先；自愈无效（30s 窗口二次失败）由 `plugins/chunk-error-fallback.ts` 接管 showError——机制经源码审查（nuxt/dist/app/plugins/chunk-reload.client.js + composables/chunk.js）+ 决策函数单测锁定；持续断网端到端复现需真实网络故障注入，标注为剩余人工观察项

## 单元测试

- `app/error.test.ts` 4/4 ✅、`app/plugins/chunk-error-fallback.test.ts` 3/3 ✅
- 全量 `pnpm test:unit`：`app/composables/useOnboarding.test.ts` 17 失败为存量环境问题（stash 对照验证：移除本 change 全部改动后仍失败；node v26 localStorage 实验特性 + happy-dom mock 冲突），与本 change 无关
