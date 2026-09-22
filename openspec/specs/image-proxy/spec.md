# image-proxy Specification

## Purpose

外链图片统一经后端代理加载：注入图床防盗链所需的 Referer/User-Agent、以磁盘缓存复用下载结果并约束占用上限，前端各图片加载点收敛到同一改写入口，根治外链图床 403 图裂。

## Requirements

### Requirement: 代理转发与 Referer 注入

系统 SHALL 提供 `GET /api/image-proxy?url=<urlencoded>` 接口，将 `http(s)` 外链图片经服务端转发返回。转发时系统 MUST 注入 `Referer` 头——默认取图片 URL 自身的 `scheme://host/`——并携带可被图床接受的 `User-Agent`，使「禁空 Referer」与「白名单 Referer（命中图片自身域）」两类防盗链规则均能通过。系统 SHOULD 支持按 host 覆盖 Referer 的配置，以便个别要求主域 Referer 的图床单独适配。缺失 `url`、非 `http(s)` 协议、或指向代理服务自身地址的请求 MUST 被拒绝（防止重定向循环），上游非 200 响应 MUST 原样透传状态码交由前端降级。

#### Scenario: 禁空 Referer 图床经代理成功

- **WHEN** 上游图床（如 `cdnfile.sspai.com`）执行「拒绝空 Referer」规则，前端通过代理请求该图片
- **THEN** 代理向上游发出带 `Referer: https://cdnfile.sspai.com/` 的请求，上游返回 200，代理将图片字节与 `Content-Type` 原样返回给前端

#### Scenario: 非法 url 被拒绝

- **WHEN** 请求携带缺失的 `url`、`ftp://` 等非 http(s) 协议、或指向代理服务自身 host:port 的地址
- **THEN** 接口返回 4xx，且不向上游发出任何请求

#### Scenario: 上游拒绝时透传状态码

- **WHEN** 注入 Referer 后上游仍返回 403（如图床改为 IP 维度封禁或签名失效）
- **THEN** 代理向上游之外零缓存写入，并向前端透传 403 状态码

### Requirement: 磁盘缓存与大小上限

代理 SHALL 将上游 200 且 `Content-Type` 为图片类型的响应落盘缓存（按 URL 哈希命名于固定缓存目录）；缓存命中时 MUST 直接返回缓存字节而不再请求上游。缓存目录 MUST 设总大小上限，超限时按最近访问时间从旧到新淘汰，直至低于上限。非图片类型或非 200 响应 MUST NOT 写入缓存。上限值 SHOULD 可经环境变量调整（默认值须在文档中记录）。

#### Scenario: 二次请求命中缓存

- **WHEN** 同一图片 URL 在缓存有效期内被再次请求
- **THEN** 响应直接来自磁盘缓存，未向上游发出请求

#### Scenario: 缓存超限滚动淘汰

- **WHEN** 缓存目录总大小超过配置上限
- **THEN** 最久未访问的缓存文件被删除，直至总量低于上限，最新访问的文件保留

#### Scenario: 非图片响应不缓存

- **WHEN** 上游返回 200 但 `Content-Type` 为 `text/html`（如防盗链错误页、跳转页）
- **THEN** 响应照常透传，但不写入缓存目录

### Requirement: 前端外链图片统一改写

前端 MUST 提供统一的图片地址改写入口：凡 `http(s)` 外链图片一律改写为同源 `/api/image-proxy?url=...` 形式加载；相对路径、`data:`、`blob:`、已经是代理地址的 URL 及空值 MUST 保持原样。改写 MUST 覆盖全部外链图片加载点：文章列表封面、预览/阅读页头图、正文 markdown/HTML 中的 `<img>`、侦探墙 CanvasTexture 贴图。数据库中的图片字段 MUST 继续存储原始外链地址（改写发生在渲染时），以保证降级与回滚不依赖数据迁移。

#### Scenario: 外链封面改写为代理地址

- **WHEN** 文章列表渲染 `imageUrl` 为 `https://cdnfile.sspai.com/...png` 的文章
- **THEN** 渲染出的 `<img src>` 为同源 `/api/image-proxy?url=<urlencoded>`，图片经代理加载成功

#### Scenario: 非外链地址不改写

- **WHEN** 图片地址为相对路径、`data:` URI、`blob:` URL 或已带代理前缀的地址
- **THEN** 改写入口原样返回该地址，不产生二次代理嵌套

#### Scenario: 侦探墙贴图不再 403

- **WHEN** 侦探墙加载此前触发 403 的 sspai 图片作为贴图
- **THEN** 图片经代理返回 200 完成纹理绘制，浏览器网络面板中对该图床域名不再出现直连请求与 `deny by referer access rule` 的 403

### Requirement: 破图降级衔接

代理失败（透传的 4xx/5xx、网络错误、超时）时，前端 MUST 按各加载点既有降级行为渲染——列表封面降级为 FeedIcon，其余位置维持既有破图/占位表现——MUST NOT 因引入代理而新增错误文案、弹窗或改变既有降级视觉。

#### Scenario: 代理透传 403 时封面降级

- **WHEN** 代理对某封面返回透传的 403
- **THEN** 该行封面按既有 `@error` 逻辑降级渲染 FeedIcon，不出现浏览器破图图标
