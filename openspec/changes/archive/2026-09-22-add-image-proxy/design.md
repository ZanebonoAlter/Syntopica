## Context

动机见 proposal.md（Why）。与设计相关的现状与约束：

- 无任何图片代理/缓存设施，前端 `<img>` 与侦探墙 `Image` 均直连外链；侦探墙还显式设了 `referrerPolicy = 'no-referrer'`（`CardGroup.ts:279`）。
- 实测证据（2026-09-22）：`cdnfile.sspai.com` 空 Referer → `403 + x-exception-info: deny by referer access rule`；任意非空 Referer（含 `http://localhost:5100/`、任意第三方域）→ 200；该 CDN 同时返回 `access-control-allow-origin: *`（CORS 无障碍）。
- 单用户本地应用、无鉴权；后端 Gin/GORM，前端 Nuxt 4；开发/运行环境为树莓派 SD 卡，磁盘与内存都紧。
- 防盗链规则分两类：**禁空 Referer**（sspai 这类）与**白名单 Referer**（只放行自家域名/通配）。两类都可被「带图片自身 origin 的 Referer」满足或大概率满足。

## Goals / Non-Goals

**Goals:**

- 所有外链图片经后端代理加载，注入能过防盗链的 Referer/UA，403 图裂一次根治。
- 磁盘缓存复用下载结果，带总大小上限，适配 SD 卡。
- 前端改写收敛为单一工具函数，后续新增图片加载点照抄即可。
- 不引第三方依赖；不改数据库；回滚零数据迁移。

**Non-Goals:**

- 不做图片重压缩/缩放/格式转换（尺寸处理交给上游图床的 `imageView` 类参数）。
- 不做代理访问控制与多用户鉴权（单用户本地应用，`/api/*` 与现有接口同等待遇）。
- 不镜像/抓取第三方站点的其他资源（仅图片 GET 转发）。
- 不迁移或改写库中已存的图片原始 URL。

## Decisions

### D1. Referer 策略：默认图片自身 origin + per-host 覆盖表

转发时默认注入 `Referer: <图片URL的 scheme>://<host>/`。

- 对**禁空 Referer** CDN：非空即放行（sspai 实测任意域都 200）。
- 对**白名单自家域** CDN：图片自身 host 落在其 `*.host` 通配内，大概率命中。
- 备选 1「写死 sspai 主域」：只治一个图床，不通用，弃。
- 备选 2「注入本站 origin」：白名单型 CDN 必拒，弃。
- 备选 3「空 Referer」：sspai 类必拒，即现状问题本身，弃。
- 预留 per-host 覆盖配置（如遇要求主域 `https://sspai.com/` 的图床可配置修正），初始表为空，实测遇到再加。

UA 采用浏览器 UA 字符串（常量），避免上游拦 `Go-http-client` 之类默认 UA。

### D2. 缓存：磁盘内容寻址 + mtime 即 LRU + 目录总量上限

- key = `sha256(原始URL)`，文件落 `data/image-cache/<hash>`（旁路 `.meta` 记 content-type？——不需要：`http.ServeFile`/读文件时以探测 content-type 为准，图片嗅探可靠，省 meta 文件）。
- 命中判定 = 文件存在；命中时 touch 更新 mtime，把 mtime 当最近访问时间用（ext4 `relatime` 下 atime 不可靠）。
- 淘汰时机：每次写入后若目录总量超上限即触发，按 mtime 从旧删到低于上限的 90%（留水位避免每次写都扫目录）；另在后端启动时跑一次。
- 上限：默认 256MB，`IMAGE_CACHE_MAX_MB` 环境变量可调。只缓存 200 + `image/*`（含 `image/webp` 等）响应。
- 备选「内存 LRU」：树莓派内存紧且重启失效，弃；备选「不缓存」：用户已否决（重复下载浪费带宽）。
- 并发击穿（同图并发首次下载）：单用户并发极低，不做 singleflight，重复下载一次代价可接受——记录为有意取舍。

### D3. 改写位置：渲染时前端工具函数，库里存原始 URL

- 统一导出 `proxiedImageUrl(url)`（放 `front/app/utils/`）：空值/相对路径/`data:`/`blob:`/已含 `/api/image-proxy` 前缀的原样返回；其余 `http(s)` 改写为 `/api/image-proxy?url=${encodeURIComponent(url)}`（同源相对路径，天然绕开 baseURL 与 CORS 差异）。
- 调用点：`ArticleCardView`（`coverSrc` computed）、`ArticleContentPreviewPanel`（`articleImageUrl` 传入处）、正文渲染产物（`useArticleContentView` / `markdown.ts` 渲染后对 `<img src>` 做字符串改写，沿用该文件既有的 img 正则处理风格）、侦探墙 `CardGroup.nodeImageUrl`。
- DB 存原始外链不动：改写只发生在渲染层 → 回滚只需还原前端，无数据迁移；未来若要换自建图床也在同一点位收敛。
- 备选「入库时就存代理地址」：回滚要洗数据，且代理路径变化会污染全库，弃。

### D4. 代理实现形态：独立 handler + 直接流式转发

- 新增 `backend-go/internal/platform/imageproxy/`（handler + cache 子组件），挂 `GET /api/image-proxy` 于现有 router；不进 GORM、不开新表。
- 转发用 `net/http` 客户端流式 copy（`io.Copy` 到 `c.Writer` 时先写缓存到临时文件，200 且 image 类型再 rename 落缓存——单写者天然无半文件问题）。
- 缓存命中路径直接 `c.File(...)`，手工补 content-type（依 `http.DetectContentType` 或首次探测结果）与 `X-Image-Proxy-Cache: HIT` 响应头（调试/验收用）。
- 超时：上游请求 15s context 超时；失败透传 502/504。
- 备选「反向代理库（如 httputil.ReverseProxy）」：我们只需要 GET + 缓存落盘，`ReverseProxy` 的Flush/缓存钩子反而绕，直接手写 copy 更直白，弃库。

### D5. SSRF 取舍

仅接受 `http/https`、拒绝指向代理自身 host:port（防循环）即可；**不**默认封禁内网段——单用户本地应用，用户可能有意代理内网图床/局域网设备图，封死反而碍事。该风险记录在案，若未来暴露到公网再收紧。

## Risks / Trade-offs

- [代理成单点，后端挂则全部图裂] → 后端本就是应用单点（API 同源）；前端 `@error` 降级保留，最坏回到 FeedIcon/占位，不出破图。
- [SD 卡写入放大] → 只缓存图片、总量上限 256MB（可调）、淘汰按水位触发；缓存目录在 `data/` 下随现有数据卷管理。
- [图床防盗链升级为 IP 封禁/URL 签名] → Referer 注入救不了签名类，透传状态码 + 前端降级兜底；签名类需按图床单独做签名生成，超出本 change 范围（Open Questions 外的已知边界）。
- [正文 HTML 字符串改写 img src 的正则脆弱] → 沿用 `useArticleContentView` 既有 img 正则风格并配单测；畸形 HTML 由既有 guard 兜底。
- [缓存旧图不再更新（图床换了图但 URL 不变）] → 图床 URL 通常含唯一路径/哈希，内容变更极少；接受。需要强一致时用户可删 `data/image-cache/` 热清，文档记录该操作。
- [SSRF 面] → 见 D5，仅 http(s) + 拒自指；单用户本地可接受。

## Migration Plan

1. 后端先合入（新路由对存量行为零影响），前端后合入开始改写——两步可分 commit，任一时刻系统都可用。
2. 部署即生效，无 DB 迁移、无配置必填项。
3. 回滚：revert 前端改写调用（或把 `proxiedImageUrl` 改为恒等函数）即回到直连；库中始终是原始 URL，无需洗数据。缓存目录可直接整目录删除，无副作用。

## Open Questions

（无——覆盖范围与缓存策略已由用户拍板：全部外链图片走代理 + 磁盘缓存带上限。）
