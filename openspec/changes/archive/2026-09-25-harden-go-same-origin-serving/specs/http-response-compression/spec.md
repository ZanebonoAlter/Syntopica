## Purpose

Syntopica 的 HTTP 响应压缩契约：静态资源与 API 文本响应对浏览器一律走内容协商压缩，且压缩能力属于应用自身、不依赖任何前置反代，因为公网访问路径可能直达 Go 进程。

## ADDED Requirements

### Requirement: 文本响应对浏览器协商压缩

服务端 SHALL 依据请求的 `Accept-Encoding` 对文本类响应压缩（至少 gzip）：内容包括 `text/*`、`application/json`、`application/javascript`、`image/svg+xml`。压缩 SHALL 同时覆盖 API 路由（`/api/*`、`/icons/*`、`/health`）与静态资源路由（`/_nuxt/*`、SPA 兜底 HTML、其他静态文件），即 MUST 在应用进程内生效，SHALL NOT 依赖前置反代提供压缩。压缩响应 SHALL 携带 `Content-Encoding` 与 `Vary: Accept-Encoding`。

#### Scenario: 静态脚本被压缩

- **WHEN** 浏览器请求 `/_nuxt/<hash>.js` 且请求头含 `Accept-Encoding: gzip`
- **THEN** 响应 `Content-Encoding` SHALL 为 `gzip`，`Vary` SHALL 含 `Accept-Encoding`，且请求体（wire bytes）显著小于未压缩体积

#### Scenario: API JSON 被压缩

- **WHEN** 客户端请求 `GET /api/articles?per_page=100` 且请求头含 `Accept-Encoding: gzip`
- **THEN** 响应 SHALL 为 gzip 压缩，解压后 JSON 内容与未压缩时逐字节等价

#### Scenario: 未声明压缩能力时返回原文

- **WHEN** 客户端请求同一路径但不带 `Accept-Encoding`（或不含 gzip）
- **THEN** 响应 SHALL 为未压缩原文，`Content-Encoding` SHALL NOT 出现

### Requirement: 压缩边界与幂等

服务端 SHALL NOT 对已压缩格式重复压缩（`woff2`、`png`、`jpg`/`jpeg`、`webp`、`gif`、`zip`、`gz` 等），SHALL NOT 压缩小于阈值（默认 1 KB）的响应，并 MUST 保持已有流式响应（WebSocket 升级、AI 流式输出）不受影响。

#### Scenario: 字体与图片不被重复压缩

- **WHEN** 浏览器请求 `/_nuxt/*.woff2` 或 `/favicon.png` 且带 `Accept-Encoding: gzip`
- **THEN** 响应 SHALL NOT 带 `Content-Encoding`，且响应体与磁盘文件字节数一致

#### Scenario: 小响应不压缩

- **WHEN** 某 JSON 响应未压缩体积 < 1 KB（如 `/api/notifications/unread-count`）
- **THEN** 响应 SHALL NOT 被压缩（避免压缩开销大于收益）

#### Scenario: WebSocket 升级不受影响

- **WHEN** 前端请求 `/ws` 并携带 `Upgrade: websocket`
- **THEN** 升级 SHALL 成功（压缩层 MUST NOT 干预非 HTTP 正文响应）

#### Scenario: HEAD 请求安全

- **WHEN** 客户端对任意文本资源发 `HEAD` 请求
- **THEN** 服务端 SHALL 返回与 `GET` 一致的状态与头部语义，MUST NOT 因压缩层报错
