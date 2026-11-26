# Review 建账：narrow-outbound-proxy-scope

审查人：zai-coding-cn/glm-5.3:max（change-review 只读档）；审查范围：四份制品一致性 + 全部 backend-go diff + 七项重点（spec/实现一致性、otel 组合、Option 顺序、调用点行为保持、全仓漏改、env 兜底测试取舍、影响面归属）。结论：不通过（2 High）→ 修复后全绿。

### R-1 2026-11-20【High】作用域成员映射错位：datasources 非 feed 拉取，rss_parser 未标 scope
- **状态**：已修复
- `reader/service/rss_parser.go:27` 补 `WithCrawlScope()`（feed 拉取主链路归位）；`datasources/fetch.go` 去掉 scope 归数据增强直连组（GDELT 直连实测正常）；proposal/design/tasks/spec 四份制品的作用域成员表述同步修正。

### R-2 2026-11-20【High】正文提取主通道 readability 不经工厂（库内裸 client），firecrawl 仅降级通道
- **状态**：已修复
- `readability_crawler.go` 的 `ScrapePage` 改为工厂 client（WithCrawlScope + ctx 感知）自行抓取 → Content-Type 校验 → `readability.NewParser().Parse` 两段式；库内自建裸 client 路径退役；死字段 `c.client` 激活。

### R-3 2026-11-20【Medium】TestNew_DefaultAlwaysDirect 行为断言有恒真风险
- **状态**：已修复
- 补 `sharedDirect().Proxy != nil` 最强直接断言（关闭 ProxyFromEnvironment once 缓存导致的漏检口子）。

### R-4 2026-11-20【Medium】EnvFallbackRoute 断言在无环境代理环境退化为 nil==nil
- **状态**：已采纳
- 改为 `reflect` 函数指针断言（direct.Proxy 即 stdlib ProxyFromEnvironment 本体，恒有区分度）；行为级验证归部署清单。

### R-5 2026-11-20【Medium】tasks 3.1 直连链路审计清单不完整（reader 包漏核 3 处）
- **状态**：已修复
- tasks 3.1 补记 reader 包 5 处逐点核对（rss_parser:27 scope、rss_parser:287 首页探测归直连、icon_store 直连、readability_crawler scope、firecrawl scope）。

### R-6 2026-11-20【Medium】ADDED 两个 Scenario 无针对性测试
- **状态**：已采纳
- 采纳注释推导链方案（TestNew_DefaultAlwaysDirect 注释写明「调用点 unscoped ⇒ 由工厂契约直连」）；子进程行为级测试过重，不做。

### R-7 2026-11-20【Low】failover.go:44 注释过时
- **状态**：已修复
- 改为 crawl-scoped 措辞。

### R-8 2026-11-20【Low】工作区混入本 change 无关改动
- **状态**：已采纳
- 确认系并行会话（fix-reader-lede-blank-space / add-ai-generated-article-detection 等在途 change）文件；提交时按文件清单严格分开，未触碰。

### R-9 2026-11-20【Low】design D1 措辞滞后（directTransport → 实现为 sharedDirect）
- **状态**：已修复
- design D1 对齐实现措辞（sync.OnceValue 单例）。

- 审毕无发现（审查范围：otelhttp 组合与 SetInstrumentation 切换、WithCrawlScope/WithTransport 顺序让位保护、三个调用点行为保持、全仓 17 处 httpclient.New 盘点、启动注入链路、三条域红线核对——审查方均确认无问题）
