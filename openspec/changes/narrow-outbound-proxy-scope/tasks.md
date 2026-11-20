# Tasks: narrow-outbound-proxy-scope

<!-- doc-impact: flow, configuration -->
<!-- doc-impact-excuse: api=discovery_handler 仅改注释文案，API 契约（路由/请求/响应）零变化，api 文档无需更新 -->

## 1. httpclient 工厂收窄

- [x] 1.1 新增 `directTransport()`（`DefaultTransport.Clone()` + `Proxy = nil`）与 `WithCrawlScope() Option`（base 选 `packageTransport()`）；`New` 默认 base 改为 `directTransport()`
- [x] 1.2 更新 `SetProxy`/`New`/`WithProxyScope` 相关注释：全局代理仅作用于 `WithCrawlScope` 标记的外部内容抓取链路

## 2. 作用域标记（三条抓取链路，review 后修正）

- [x] 2.1 `reader/service/rss_parser.go`：feed 拉取主链路，`NewRSSParser` 追加 `WithCrawlScope()`（review H1：datasources 并非 feed 拉取）
- [x] 2.2 `reader/service/readability_crawler.go`：正文提取主通道，`NewReadabilityCrawler` 追加 `WithCrawlScope()`，`ScrapePage` 改为工厂 client 自行抓取 + `readability.NewParser().Parse` 两段式（review H2：库内自建裸 client 不受代理/熔断管控）
- [x] 2.3 `reader/service/firecrawl_service.go`：正文提取降级通道，`httpclient.New(...)` 追加 `WithCrawlScope()`
- [x] 2.4 `platform/imageproxy/handler.go`：构造处追加 `WithCrawlScope()`
- [x] 2.5 `datasources/fetch.go`：裸 `&http.Client{}` 收编为 `httpclient.New(WithTimeout)`（无 scope，归直连组）+ 外部赋 `CheckRedirect`（手工重定向校验保留）

## 3. 直连链路审计（review M3 补记：reader 包 5 处全部核对）

- [x] 3.1 reader 包 `httpclient.New` 逐点核对：`rss_parser.go:27`（feed 拉取→scope）、`rss_parser.go:287`（首页探测，归 icon 直连组）、`icon_store.go:71`（icon 直连）、`readability_crawler.go:67`（正文提取→scope + 两段式改造）、`firecrawl_service.go:60`（正文提取降级→scope）；其余调用点（airouter×4、dataenrichment×3、datasources、discovery×2、searxng）默认直连后无行为破坏（目标均为国内直连/内网可达），必要处留注释说明归属

## 4. 文案与 spec 同步

- [x] 4.1 discovery 代理设置 API（`GetProxySettings`/`SaveProxySettings`）注释与描述文案收窄为「外部内容抓取链路」

## 5. 测试与验证

- [x] 5.1 单测：强制直连不读环境代理（`t.Setenv HTTPS_PROXY` + httptest 断言直连）；`WithCrawlScope` client 仍按 `SetProxy` 路由；既有 failover/熔断测试保持绿
- [x] 5.2 `golangci-lint run ./internal/platform/httpclient/...` + 影响包 `go test`（httpclient、datasources、reader/service、imageproxy）
- [x] 5.3 重启后端（`bash scripts/dev/start-dev.sh --restart`），mihomo 日志验证：`main` 进程的 AI 域名行（deepseek/minimax/bigmodel）绝迹；恢复 `http_proxy_url` 后抓取链路仍走代理
