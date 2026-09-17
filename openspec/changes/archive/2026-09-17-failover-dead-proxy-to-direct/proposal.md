<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: content-enrichment -->

## Why

全局出站代理（`http_proxy_url`，如 Clash）配置后是**一刀切**开关：代理进程挂掉/端口不通时，所有经 httpclient 发出的出站请求（RSS 抓取、Firecrawl、图标下载、AI 路由、dataenrichment）在「拨代理」这一步就失败——本来直连就能成功的国内 RSS 源也跟着失败，且无任何降级机制。

排查还发现同根问题：文档与接口注释声称「保存即时生效」（`configuration.md` 出站代理节、`SaveProxySettings` 注释），但 `httpclient.New()` 在构造时快照 proxyTransport，`RSSParser` 等服务是启动时建好的单例——运行时改代理对已构造 client 实际**不生效**，抓取要重启才换代理。

## What Changes

- **代理不可达熔断降级直连**：httpclient 新增故障感知 transport——「拨代理本身失败」（连接拒绝/拨号超时，dial 地址=代理地址）即打开熔断（默认 60s 窗口），本请求立即直连重试（dial 阶段失败请求未发出，重试无副作用）；窗口内后续请求全部直连零等待；到期放行单个请求试探代理（单飞防惊群），通则恢复、败则续期。
- **错误分类边界**：仅「拨代理失败」触发熔断；代理活着但目标失败（HTTP 4xx/5xx、经代理 CONNECT 后的目标错误、socks5 拒绝）不触发——那类站点可能正是需要代理的，直连也不会更好。
- **代理配置变更真正即时生效**：代理 URL 改为 atomic 存储、Proxy 函数每请求动态读取，`SetProxy` 只原子换 URL——所有已构造 client（含启动单例）立即按新配置路由，无需重启；换址时重置熔断状态。
- 回环直连绕过（ai-health-reprobe 既有要求）不变，与新熔断共存；未配置代理时仍回落 `ProxyFromEnvironment`（环境变量兜底语义不变）。

## Capabilities

### New Capabilities

- `outbound-proxy-failover`: 全局出站代理的故障熔断直连回退（正常→熔断→试探恢复状态机）、错误分类边界（拨代理失败 vs 目标失败）、代理配置变更即时生效语义

### Modified Capabilities

（无——ai-health-reprobe 的「本地回环请求绕过全局出站代理」要求保持不变）

## Impact

- 后端：`backend-go/internal/platform/httpclient/`（核心改动：新增 failover transport + 代理 URL atomic 化；`SetProxy` 对外签名不变）、`cmd/server/main.go` 调用方零改动
- 行为变化：配置了代理且代理挂掉时，出站请求自动降级直连（此前全部失败）；代理恢复后最长 60s 自动接回；设置页改代理立即对抓取/AI 生效（此前要重启）
- 文档：`docs/reference/configuration.md`（出站代理节：修正「即时生效」表述 + 补熔断语义）、`docs/reference/flow/scheduler.md`（代理污染防线节补充熔断降级）
- 无前端改动、无数据库变更、无 API 变更（`GET/POST /api/settings/proxy` 契约不变）
