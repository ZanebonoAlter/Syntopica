# Design: narrow-outbound-proxy-scope

## Context

- httpclient 工厂（`internal/platform/httpclient`）持有进程级共享 `packageTransport`（failoverTransport：proxy 路由显式 ProxyURL + direct 路由保 ProxyFromEnvironment + 三态熔断），`SetProxy` 写全局 `proxyURLValue`、每请求读取、即时生效。
- 现状：`New` 无显式 Transport 时一律挂 `packageTransport` → 全部 `httpclient.New` 用户（airouter、dataenrichment、discovery、imageproxy、searxng、reader×2）都被全局代理劫持；`proxyRouteFunc` 仅豁免 loopback，不认 `no_proxy`。
- 2026-11-20 止血：settings 里 `http_proxy_url` 已清空（`POST /api/settings/proxy`），当前出站靠进程环境变量兜底（治标，依赖启动方式）。

## Goals / Non-Goals

- Goals：全局代理作用域收窄为三条外部内容抓取链路（feed 拉取、正文提取、外链图片）；其余出站强制直连；抓取链路韧性语义（熔断/单飞/即时生效/环境兜底）零退化。
- Non-Goals：不引入 per-destination 代理路由表；不改 `no_proxy` 相关语义（该方案已被作用域收窄取代）；不改 settings API 形状。

## Decisions

### D1：transport 层零改动，收窄「谁用它」
`packageTransport` 原样保留（failover + 熔断 + SetProxy 即时生效），新增进程级 `sharedDirect`（`sync.OnceValue` 单例）= `http.DefaultTransport.Clone()` 后 `Proxy = nil`（保留 HTTP2/TLS/dialer 默认，纯直连——`Proxy: nil` 即不读环境变量）。`New` 的默认 base 从 `packageTransport` 换成 `sharedDirect`；新增 `WithCrawlScope() Option` 显式选择 `packageTransport`。
备选被否：「每个调用点继续用 packageTransport + 加 no_proxy 判定」——依赖名单维护，语义分散；「按域名判定 scope」——配置复杂化，不需要。

### D2：作用域成员（review 后修正）
- 标记 `WithCrawlScope()`：`reader/service/rss_parser.go`（feed 拉取主链路 RefreshFeed→ParseFeedURL）、`reader/service/firecrawl_service.go`（正文提取降级通道）、`reader/service/readability_crawler.go`（正文提取主通道，改造为工厂抓取+库解析两段式）、`platform/imageproxy/handler.go`（外链图片）。
- 默认直连：airouter `openai_compatible/fallback/store_vendor`、dataenrichment `fingenius_client/web_search/tool_registry`、datasources `fetch.go`（数据增强研究源栈，非 feed 拉取；GDELT 直连实测正常）、discovery `catalog_sync/catalog_extras`（自建 RSSHub）、reader `icon_store`、searxng。
- 备注：首版实现曾把 datasources 误标为 feed 拉取、且漏掉 readability 主通道（review H1/H2），已按上方归属修正。

### D3：datasources 裸 client 收编（归直连组）+ readability 主通道两段式改造
`fetch.go` 的 `&http.Client{Timeout, CheckRedirect}` 收编为 `httpclient.New(WithTimeout)`（无 scope，强制直连）后外部赋 `CheckRedirect`（手工重定向校验保留）；`SetClient` 测试注入口不受影响。`readability_crawler.go` 的 `ScrapePage` 改为：工厂 client（WithCrawlScope + ctx 感知请求）自行抓取 → Content-Type 校验 → `readability.NewParser().Parse(body, parsedURL)` 解析；库内自建裸 client 路径（FromURL）退役，代理/熔断/即时生效对正文提取主通道生效。

### D4：SetProxy 语义不动
仍是全局 URL 值 + breaker reset；作用面由「使用 packageTransport 的 client 集合」决定（收窄后即三条抓取链路）。`main.go` 启动注入链路不变。settings 文案收窄为「外部内容抓取链路的出站代理」。

## Risks / Trade-offs

- [曾依赖环境代理的直连链路将变为纯直连]：逐点核对过业务归属——discovery=自建 RSSHub（内网）、searxng=内网搜索、AI 供应商=国内直连可达、icon=一般国内 CDN；接受。若未来出现国外依赖目标，走 `WithCrawlScope` 纳入作用域即可。
- [imageproxy 图片流量过 mihomo]：用户明确要求保留（外链图片含被墙图床）。
- [止血态与落地态的差异]：落地前（现状）抓取链路靠环境变量兜底走 mihomo；落地后恢复 `http_proxy_url` 配置即可回到显式契约，AI 链路不再受影响。

## Migration Plan

单次部署生效；无数据迁移。部署后可选：`POST /api/settings/proxy` 恢复 `http_proxy_url`（feed 抓取需要翻墙时），AI 链路不受影响即本 change 的验收点之一。

## Open Questions

- 无（作用域成员已与用户逐条确认：firecrawl=本地 readability 保留；imageproxy 保留；discovery=自建 RSSHub 直连）。
