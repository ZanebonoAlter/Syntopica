
## httpclient 代理机制现状与改造点

**现状机制**（backend-go/internal/platform/httpclient/httpclient.go）：
- `SetProxy(rawURL)`：校验 scheme（http/https/socks5）→ Clone DefaultTransport + `Proxy=proxyWithLoopbackBypass(u)` → 存包级 `proxyTransport`（RWMutex）。空串清空（置 nil）。
- `New(opts...)`：构造时**快照** transport——`currentProxyTransport()` 非空用之，否则 `http.DefaultTransport`。已构造 client 永远用旧 transport（注释 "clients already built keep their transport"）→ 运行时改代理对启动单例（RSSParser/FirecrawlService/airouter 等）不生效。
- 回环绕过：`isLoopbackHost`（localhost/127/8/::1/空 host → Proxy 返回 nil 直连），在 Proxy 函数内。
- 空代理时回落 DefaultTransport（保留 HTTP_PROXY 环境变量兜底语义）——改造时新 Proxy 函数需在 URL 空时返回 `http.ProxyFromEnvironment(req)` 保持等价。

**调用方**（零改动）：`cmd/server/main.go:73-80` 启动注入（LoadProxyConfig → SetProxy）；`internal/admin/handler/discovery_handler.go:482` SaveProxySettings（POST /api/settings/proxy，契约不变）；`internal/platform/aisettings/config_store.go` proxyConfigKey="http_proxy_config"。

**httpclient.New 的使用方**（全部受益，无需改）：reader/service/{rss_parser(20s),readability_crawler(30s),firecrawl_service,icon_store}、admin/service/{catalog_extras,catalog_sync_service}、dataenrichment/service/{fingenius_client,web_search,tool_registry}、airouter/{fallback,openai_compatible,test_connection}。

**改造落点**：全部在 httpclient 包内——`failover.go`（新：failoverTransport 单例 + breaker{mutex,downUntil,probing} + isProxyDialFailure）+ `httpclient.go`（SetProxy 改 atomic 写 URL；New 未显式 WithTransport 时一律取包级单例）。对外 SetProxy 签名不变。
- 错误分类：errors 链找 `*net.OpError` 且 Addr host:port == 代理地址（仅拨代理失败触发熔断+直连重试；502/CONNECT 非 200/socks5 拒目标不触发）；兜底方案 = 自定义 DialContext 拨号层比对地址。
- 直接绕过代理的例外：`internal/platform/safefetch/safefetch.go` **故意不用 httpclient**（不信任内容独立路径），不受影响。

**既有测试**（改造后应继续绿，语义兼容）：httpclient_test.go（TestSetProxy_AppliesToNewClients/EmptyClearsProxy/RejectsUnsupportedScheme、TestNew_WithTransportOverridesProxy 显式覆盖语义保留）、proxy_loopback_test.go（TestProxyBypass_* 三条 + TestIsLoopbackHost）。

**文档待修**：docs/reference/configuration.md:291「保存即时生效（SetProxy 运行时替换全局 transport）」为错误表述（实际快照语义）——本 change 修正；flow/scheduler.md「代理污染防线」节补熔断语义。

**引用**：backend-go/internal/platform/httpclient/httpclient.go:SetProxy、backend-go/internal/platform/httpclient/httpclient.go:New、backend-go/cmd/server/main.go、backend-go/internal/admin/handler/discovery_handler.go:SaveProxySettings

