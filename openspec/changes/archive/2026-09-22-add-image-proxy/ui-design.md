<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

<!--
  marker 契约：ui-impact 与 proposal 一致（minor）；
  minor → approval=not-required、prototype=none，四节轻量契约。
-->

## 入口与入口变更

无新增页面、面板、对话框或导航入口。图片加载入口不变（仍是文章列表封面、预览/阅读页头图、正文 `<img>`、侦探墙贴图），仅其 `src` 由外链直连改为同源 `/api/image-proxy?url=...`。用户可见入口与信息架构零变化。

## 受影响状态

- **loading**：无变化——图片仍走浏览器原生 `loading="lazy"` 与既有占位，代理增加的一跳延迟不改变加载态结构。
- **success**：图片正常显示，与直连视觉一致（代理透传原始字节与 content-type，不做重压缩）。
- **error**：上游 403/失败时代理透传状态码，触发既有 `@error` 降级——列表封面降级 FeedIcon 的逻辑原样保留；正文/头图破图表现与今天直连失败时一致，不新增错误文案或 UI。
- **empty**：不适用（本 change 不引入空态）。

## 复用组件与布局模式

不新增组件、不改布局。改写点全部复用既有渲染位：`ArticleCardView.vue` 的 `row-cover` 区、`ArticleContentPreviewPanel.vue` 的 `article-image` 区、正文渲染管线产物、侦探墙 `CardGroup.ts` CanvasTexture。布局沿用各页面现状（layout mode 不变，见 `docs/reference/standard/frontend/layout.md`），无自由宽度新增。

## 验收映射

- **组件测试**：`ArticleCardView.test.ts` 既有「封面加载失败降级」用例继续通过；新增 `proxiedImageUrl()` 纯函数单测（外链改写/相对路径与 data:/blob:/已代理地址不改写/空值透传）。
- **后端测试**：image-proxy 单测覆盖 Referer 注入、缓存命中/上限淘汰、上游 403 透传（httptest stub，不打真实图床）。
- **人工验证**：侦探墙打开后 sspai 文章贴图不再 403 图裂（浏览器 Network 确认代理请求返回 200、上游无直接 403）。
