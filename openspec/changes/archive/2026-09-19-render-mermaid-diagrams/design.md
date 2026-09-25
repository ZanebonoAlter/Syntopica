# Design — render-mermaid-diagrams

## Context

现状渲染管线全同步：`marked.parse(md)` 产 HTML 字符串 → `v-html` 挂载，调用点 6+ 处（`useArticleContentView.ts` 两处 + `utils/markdown.ts` 四面板），全部同步调用。` ```mermaid ` 围栏被 marked 当普通代码块输出。主题机制为 `<html data-theme="editorial|dark">`（inline script 先于 hydration 设置）。`.markdown-body` 基础排版是 tags 三面板共享宿主（CSS 红线：基础段不动、纯增量可加）。容器方案已经用户原型审批定为方案 B（细分隔线轻容器），见 ui-design.md §7 审批记录。

## Goals / Non-Goals

- **Goals**：客户端按需渲染、失败无损降级、主题双适配 + 切换重绘、共享渲染器全宿主覆盖、零数据迁移。
- **Non-Goals**：服务端预渲染、图点击放大/交互、mermaid 语法自动修复、LLM 提示词改造（让它少产非法 mermaid）、打印样式适配。

## Decisions

### D1 渲染时机：v-html 后 DOM 后处理，不改 marked 管线

marked 支持 async renderer，但那会把全部 6+ 个同步 `parse` 调用点改为 async 链，改动面与风险远超收益。选定：宿主内容就绪（watch + `nextTick`）后扫描宿主容器内 `pre > code.language-mermaid`，逐块替换为 SVG。管线保持同步，宿主侵入仅「挂一个 composable + 容器 ref」。

**备选否决**：marked async renderer（6+ 调用点连锁改签名）；服务端预渲染（Go 侧需 headless Chromium，树莓派不可行）。

### D2 挂接形态：`useMermaidRender(hostRef, source)` composable

新建 `front/app/composables/useMermaidRender.ts`（共享位，阅读页与 tags 面板均自动导入）。宿主传入容器 ref + 内容 computed，composable 内部：内容变化 → `nextTick` → 扫描 → 渲染。重入幂等：已替换为 SVG 的块跳过；v-html 重渲染冲掉 SVG 后重扫描自然恢复。

**备选否决**：包装组件 `<MarkdownBody>`（四面板 + 阅读页共 5 处要改组件树，侵入更大）。

### D3 mermaid 加载：动态 `import('mermaid')` + 模块级单例

首次发现 mermaid 块才拉取 chunk（vite 代码分割）；模块级 Promise 单例防并发重复加载。`mermaid.initialize({ startOnLoad: false, securityLevel: 'strict' })` 一次性执行。主题映射：editorial → `theme: 'default'`，dark → `theme: 'dark'`。主题切换监听用 `MutationObserver` 观察 `documentElement` 的 `data-theme` 属性（主题切换入口分散，观察者比逐处 hook 可靠），触发后对已渲染块按新主题重绘；带版本号 token 防快速连点竞态。

### D4 失败兜底：逐块渲染 + 降级标记

不用 `mermaid.run` 整批执行（单块语法错误会中断整批）。逐块 `mermaid.render` + try/catch：失败块加 `data-mermaid-failed` 保留源码 `<pre>`，紧随其後插入一行 warning 色提示（`--color-warning`，文案含失败原因缩略）；单块失败不影响同宿主其他块。

### D5 容器样式（方案 B）：`.mermaid-block` 纯增量规则

`ArticleContent.css` 基础段**纯增量**新增：`.mermaid-block`（上下 `--color-border-subtle` hairline、`padding`、`overflow-x: auto`）+ `.mermaid-block .fig-cap`（小号 `--color-text-muted` 图题）。图题文案「图 N · <宿主内顺序号>」由渲染器按宿主文档顺序生成（同宿主多图递增）。不触碰任何既有 `.markdown-body` 元素规则——共享宿主红线合规。

### D6 超宽兜底：`useMaxWidth` + 横向滚动

mermaid 配置 `useMaxWidth: true`（SVG 默认缩至容器宽）；容器 `overflow-x: auto` 兜底超宽图，列版式不破。

## Risks / Trade-offs

- [mermaid chunk 体积大（gzip 约 700KB+）] → 动态 import 仅含 mermaid 块的页面才加载；单用户本地场景首屏不受影响
- [LLM 产出非法 mermaid 比例高] → 降级路径一等公民（源码 + 提示），不做语法修复
- [快速切换主题的重绘竞态] → 重绘任务带版本 token，过期任务丢弃
- [流式更新的面板宿主频繁触发扫描] → 扫描幂等且 `nextTick` 合并；已渲染块跳过，开销为 DOM 查询级别
- [SSR 误执行] → 渲染全部位于客户端 watch 回调 + 动态 import，服务端零路径

## Migration Plan

纯前端，无后端/DB 改动。部署走 `bash scripts/dev/deploy-frontend.sh`（构建 → 铺静态产物 → 重启后端），存量 29 篇含 mermaid 的文章打开即见图。回滚 = 还原前端静态产物，无数据残留。

## Open Questions

无（容器方案已经原型审批定为 B；主题重绘行为已随方案 B 确定为必选）。
