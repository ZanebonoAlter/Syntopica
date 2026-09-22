<!-- complexity: complex -->
<!-- ui-impact: minor -->

## Why

外部图床普遍启用 Referer 防盗链，外链图片直连会 403 图裂：2026-09-22 实测 `cdnfile.sspai.com` 返回 `403 + x-exception-info: deny by referer access rule`（火山 CDN「禁空 Referer」规则），当前代码中侦探墙贴图（`CardGroup.ts` 的 `image.referrerPolicy = 'no-referrer'`）必触发；其他图床还存在「白名单 Referer」型规则，逐个图床打前端补丁不可持续。需要一个统一出口，让所有外链图片经后端带上合适的 Referer 请求，一次根治。

## What Changes

- 后端新增图片代理接口 `GET /api/image-proxy?url=...`：对目标 URL 注入 Referer（默认取图片自身 origin，可配 per-host 覆盖）与 User-Agent 后转发，流式返回图片响应。
- 代理内置磁盘缓存：缓存落 `data/image-cache/`（URL 哈希命名），仅缓存 200 且 `content-type` 为图片的响应，带总大小上限与滚动清理（按最近访问时间淘汰）。
- 前端外链图片统一改写走代理：文章列表封面、预览/阅读页头图、正文 markdown/HTML 里的 `<img>`、侦探墙 CanvasTexture 贴图等所有外链图片加载点收敛到一个 `proxiedImageUrl()` 工具；相对路径 / `data:` / `blob:` / 已代理地址不改写。
- 删除侦探墙 `CardGroup.ts` 中的 `image.referrerPolicy = 'no-referrer'`（在代理后面不再需要，且它正是当前 403 的直接触发点）。
- 破图降级行为保持：列表封面 `@error` 降级 FeedIcon 的既有逻辑不变，代理失败同样走该降级。

## Capabilities

### New Capabilities

- `image-proxy`: 外链图片代理能力——Referer/UA 注入策略、转发与状态透传、磁盘缓存与大小上限、前端统一改写入口与破图降级衔接。

### Modified Capabilities

（无——侦探墙、阅读页等既有 spec 均未对图片来源加载方式提出 spec 级要求，本次不改动既有需求。）

## Impact

- 后端：`backend-go/internal/app/router.go`（新路由）、新增 image-proxy handler/service 包（转发、Referer 策略、磁盘缓存与清理）、`data/image-cache/` 新增运行时数据目录（加入 .gitignore）。
- 前端：新增 `proxiedImageUrl()` 工具及调用点改写——`ArticleCardView.vue`（封面）、`ArticleContentPreviewPanel.vue`（头图）、正文渲染管线（`useArticleContentView.ts` / `markdown.ts` 的产物 img 改写）、侦探墙 `CardGroup.ts`（贴图 + 删 no-referrer）；排查 `FeedIcon.vue` 等其余外链 src。
- API：新增 `GET /api/image-proxy`；无既有 API 变更，无破坏性改动。
- 依赖/系统：不引第三方依赖；缓存目录占用磁盘需上限约束（树莓派 SD 卡）；上游图床按其防盗链规则可能仍拒绝（透传状态码，由前端降级兜底）。
