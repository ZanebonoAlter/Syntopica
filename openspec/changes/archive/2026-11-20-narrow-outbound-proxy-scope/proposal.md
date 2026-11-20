<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: content-enrichment, ai-summary, discovery -->

## Why

全局出站代理（`http_proxy_url`）经 httpclient 工厂作用于**所有**非 loopback 出站请求：AI 调用（打标签/embedding/摘要）、数据增强、订阅目录同步等全部被劫持经 mihomo 转发——`proxyRouteFunc` 只豁免 loopback、不认 `no_proxy` 名单；后果是 mihomo 重启/故障会连坐拖断 AI 调用（熔断回退有失败窗口），且 mihomo 规则变化可能把国内 AI 域名流量引向机场出口。2026-11-20 已临时清空 `http_proxy_url` 止血（依赖环境变量兜底，治标），本 change 把代理作用域收窄为外部内容抓取链路，根治。

## What Changes

- `httpclient.New` 默认出站 transport 改为**强制直连**（不读环境代理变量），默认语义从「跟随全局代理 + 环境变量兜底」收窄为「永远直连」。
- 新增 `WithCrawlScope` Option：仅三条外部内容抓取链路保留全局代理能力（failover 熔断回退、SetProxy 即时生效、环境变量兜底全部不变）——RSS feed 拉取（reader/rss_parser）、正文提取（reader readability 主通道 + firecrawl 降级通道）、文章外链图片（imageproxy）。
- 正文提取主通道改造：`ReadabilityCrawler.ScrapePage` 原先走 go-readability 库内自建裸 client（不受全局代理/熔断管控），改为工厂 client 自行抓取 + 库仅解析（`NewParser().Parse`）两段式，库内裸 client 路径退役。
- datasources 的裸 `&http.Client{}` 同步收编进 httpclient 工厂，但它属数据增强研究源栈（EIA/JODI/GDELT 等）而非 feed 拉取，归直连组（GDELT 对代理出口 IP 限流 429，直连实测正常）。
- 其余调用点（airouter×3、dataenrichment×3、datasources 研究源、discovery catalog×2、reader icon、searxng）随默认切换自动变为强制直连——打标签/embedding/摘要不再触达 mihomo；discovery 目录同步目标为自建 RSSHub（内网），直连。
- discovery 代理设置 API 的注释/文案同步收窄（「feed 抓取等所有外部请求」→「外部内容抓取链路」）。
- spec：`outbound-proxy-failover` 三条 requirement 作用域措辞收窄 + 新增「非抓取出站强制直连」requirement。

## Capabilities

### Modified Capabilities
- `outbound-proxy-failover`：全局出站代理的作用域从「所有非 loopback 出站」收窄为「外部内容抓取链路（feed 拉取、正文提取、外链图片）」；新增非抓取链路强制直连的行为契约；熔断回退、单飞试探、配置即时生效等韧性语义保持不变（仅作用域措辞收窄）。
