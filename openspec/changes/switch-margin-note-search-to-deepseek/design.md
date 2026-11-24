# Design — switch-margin-note-search-to-deepseek

## Context

动机见 proposal.md「Why」。本节只列与本设计直接相关的现状约束：

- **调用方极窄**：`internal/platform/searxng` 只被 4 处引用——`topicgraph/service/margin_notes_websearch.go`、`margin_notes_qa.go`（+ 其测试）、`discovery/handler/discovery_handler.go`。`Result{Title,URL,Content}` 三字段形状可直接被新入口继承。
- **凭据已在库里**：`ai_vendors` 中已存在 name=`DeepSeek`、base_url=`https://api.deepseek.com/v1`、provider_type=`openai_compatible`、已填 key 的记录；`deepseek-flash` / `deepseek-v4-pro` 两个模型已注册并绑在 `digest_polish` / `data_enrichment_analysis` 的降级位。**不需要新增密钥存储**。
- **出站网络**：airouter 与 searxng 客户端都走 `httpclient.New` 默认的 `sharedDirect`（强制直连、忽略环境代理）。实测树莓派直连 `api.deepseek.com` 连接 40ms、搜索 0.46s，`no_proxy` 已含 `deepseek.com`，无代理问题。
- **red line 25 / 27 原文**（`docs/reference/flow/daily-report.md:350,353`）多处写死「SearXNG」与落点 `platform/searxng/`，本 change 需同步修订措辞与落点（语义不变）。
- **现有合并逻辑**（`margin_notes_websearch.go:285-305`）：按 query 顺序 × 各自结果顺序拼接、URL trim 精确去重、截前 5；`searxng.Search` 丢弃 `score`。
- **Bocha 另一条独立链路**（`dataenrichment/service/web_search.go`）不属于本 change 范围，见 Non-Goals。

## Goals / Non-Goals

**Goals**
- 页边注联网后端整体切换为 DeepSeek Search API，SearXNG 相关代码/配置/路由/前端面板全部移除。
- 新入口的错误按类别可区分（not_configured / auth / quota / rate_limited / timeout / unavailable），探测面板能把「密钥错」和「余额没」告诉用户。
- 多 query 合并改为跨 query 相关度重排。

**Non-Goals**
- 不改 QA 模型绑定、不改 prompt 措辞（「仅供参考 / 允许空引用」）、不引入 airouter `tools` 字段。
- 不把 `dataenrichment` 的 Bocha `WebSearcher` 迁到新入口（Bocha key 已欠费是另一个问题，另开 change）。
- 不做搜索结果缓存、不做重试。
- 不新增独立配置键：凭据只认 `ai_vendors`。

## Decisions

### D1. 新入口放 `internal/platform/websearch`，接口只暴露「搜」与「探测」两个函数
- **结论**: 新包 `websearch`，导出 `Search(ctx, query) ([]Result, error)` 与 `ProbeHealth(ctx) (Health, error)`；凭据解析内置（每次现读 `ai_vendors`），调用方不再传 endpoint。
- **落点**: `backend-go/internal/platform/websearch/search.go:Search`
- **取舍**: 拒绝「保留 `searxng` 包名只在内部换实现」——包名与实际后端不符会误导后续维护者，且 `searxng` 的 `Search(ctx, endpoint, query)` 签名要求调用方传 endpoint，与新凭据模型不匹配；拒绝「做成可插拔多 provider」——用户明确要求不留 SearXNG，多 provider 抽象是为不存在的第二实现付费。

### D2. 凭据解析：按 base_url 主机匹配 DeepSeek vendor，不新增配置键
- **结论**: 查 `ai_vendors` 中 `enabled=true AND provider_type='openai_compatible' AND api_key<>''` 且 base_url 主机为 `api.deepseek.com` 的记录（多条取 id 最小）；找不到返回 `ErrNotConfigured`。探测响应透出 `vendor_configured` 供面板渲染空态。
- **落点**: `backend-go/internal/platform/websearch/credentials.go:ResolveCredentials`
- **取舍**: 拒绝「新增 `ai_settings.web_search_config` 独立存 key」——与 vendor 两层注册表红线（ai-summary 约束 4）方向相反，且会出现两份 DeepSeek key 要维护；拒绝「按 vendor name='DeepSeek' 匹配」——name 是用户可改的自由文本，主机匹配才稳定。

### D3. 错误分类用哨兵错误 + `Kind` 方法，而非字符串前缀
- **结论**: 定义 `type Error struct { Kind ErrorKind; Status int; Msg string }`，`ErrorKind` 枚举 `NotConfigured/Auth/Quota/RateLimited/Timeout/Unavailable`；402 → quota；403 且响应体含 `quota`/`余额`/`money`/`arrears` → quota，否则 → auth；`errors.Is(err, context.DeadlineExceeded)` 或 net timeout → timeout；其余 → unavailable。调用方 `errors.As` 取 Kind。
- **落点**: `backend-go/internal/platform/websearch/errors.go:ErrorKind`
- **取舍**: 拒绝「沿用 `fmt.Errorf` 字符串前缀」——searxng 现状就是字符串，探测面板无法区分「实例没开 json」和「实例挂了」，这次换后端正是为了把这个坑填掉。

### D4. 合并池按 `rerank_score` 跨 query 稳定排序
- **结论**: `Result` 增加 `Score float64`（DeepSeek `rerank_score`，0~3）；合并时 URL trim 去重（首次出现为准），`sort.SliceStable` 按 Score 降序，截前 5。
- **落点**: `backend-go/internal/topicgraph/service/margin_notes_websearch.go:mergeMarginNoteSearchResults`（实现期修订：沿用仓库既有函数名，behavior 按 D4）
- **取舍**: 拒绝「按 authority 优先」——authority 是站点级权威度（wikipedia 永远 4），不反映与本问题的相关性；拒绝「Score × Authority 加权」——两个量纲未标定，拍脑袋权重没有依据，先只按相关度，上线后看效果再调。

### D5. 探测复用 `Search` 本体，`auth`/`quota`/`rate_limited` 归 `unreachable` 并透出 `error_kind`
- **结论**: `ProbeHealth` 用固定探针词 `connectivity probe` 调一次 `Search`；status 映射：`ErrNotConfigured`→`disabled`；`Timeout`/`Unavailable`/`Auth`/`Quota`/`RateLimited`→`unreachable`（`error_kind` 给具体类别）；200 空结果→`degraded`；有结果→`ok`。
- **落点**: Scenario「密钥失效时探测指向鉴权」
- **取舍**: 拒绝「把 auth/quota 单列为第五、六态」——四态枚举已被前端 `healthStatusMeta` 与 spec 固定，扩枚举要改前端样式映射；用 `error_kind` 子字段是加法变更，旧消费方零影响。

### D6. 前端：tab 原位替换 + 旧深链重定向，面板不提供可编辑字段
- **结论**: `SettingsSectionDatasourcesNetwork.vue` 的 tabs 中 `searxng` 项替换为 `web-search`（label「网页搜索」），旧深链在 `SettingsWorkspace.vue` 做两级重定向（旧 section 键 `searxng` → 本分区 `web-search` tab；`tab=searxng` → `tab=web-search`），分区容器 `SettingsSectionDatasourcesNetwork.vue` 另留同值兜底映射（实现期修订：仓库无 `legacyTabMap`，tab 参数由分区容器管）；`WebSearchConfigPanel` 只展示关联状态 + 测试连接，无表单提交。
- **落点**: `front/app/components/dialog/WebSearchConfigPanel.vue`
- **取舍**: 拒绝「在面板里放 API Key 输入框」——那会出现两个地方能改同一把 key，与 D2 的「凭据单一来源」矛盾；用户真要换 key 去「AI 模型」分区改。

### D7. 页边注卡三态标记复用现有弱化标记样式族
- **结论**: `MarginNoteCard.vue` 在现有 `web_search_status==='empty'` 分支旁增加 `ok` 分支，按 `cited_web_sources.length` 区分「已联网」/「已联网 · 未采用」；沿用同一 class 族与字号。
- **落点**: `front/app/features/tags/components/daily-report/MarginNoteCard.vue`
- **取舍**: 拒绝「把 `ok+未采用` 渲染成警示色」——模型不引用是被 prompt 显式许可的合法行为（约束 25），不是错误；用弱化色保持与「联网搜索无结果」同一视觉层级。

### D8. 红线文档同步：daily-report 约束 25 / 27 的「SearXNG」措辞改为「统一网页搜索入口」，落点更新
- **结论**: 归档前把 `docs/reference/flow/daily-report.md:350` 的「提问当次的 SearXNG 合并搜索结果池」改为「提问当次的统一网页搜索合并结果池」，「SearXNG 只用原始网页结果（无 AI 总结模式…）」改为「搜索入口只用原始网页结果（DeepSeek Search 返回 rerank/summary，不含 AI 总结段）」；`:353` 的 `platform/searxng.ResolveConfiguredEndpoint` 改为 `platform/websearch.ResolveCredentials`、`searxng.Search` 改为 `websearch.Search`、`SearxngConfigPanel.vue` 改为 `WebSearchConfigPanel.vue`、路由改 `/api/settings/web-search/health`。合并规则补「跨 query 按 rerank_score 重排」。语义零变更。
- **落点**: `docs/reference/flow/daily-report.md`
- **取舍**: 无备选：AGENTS.md 要求归档前 `doc-impact.sh verify`，红线文档落点失效会阻断归档；且「SearXNG」字面留在红线里会误导后续 change 的约束注入。

## Risks / Trade-offs

- [DeepSeek Search 余额耗尽时页边注联网静默失效，与 2026-10 SearXNG 事故形态相同] → 探测面板 `error_kind=quota` 明示「余额或额度不足」；`web_search_status=empty` 标记照常出现；同时 Warn 日志带 Kind 可 grep。本 change 不做运行时余额告警（Non-Goal）。
- [`rerank_score` 跨 query 可比性未验证（不同 query 的分数分布可能不同）] → 用 `SliceStable` 保证同分保序，先上线观察；若出现某条 query 系统性低分，再在 D4 基础上加 per-query 归一化。
- [删除 `searxng_config` 后，旧部署若只配了 `SEARXNG_URL` 没配 DeepSeek vendor] → 后端返回 `disabled`，问答照常（静默降级语义不变），设置面板显示「未关联供应商」并指引去 AI 模型；部署后影响会在完工汇报里明示。
- [`config.AppConfig.Searxng` 字段删除会影响读取 `searxng.endpoint` 的旧 config.yaml] → 加载器对未知键容忍（现有行为），只删 Go 字段与 env 绑定；不改用户的 yaml 文件。
- [树莓派直连 `api.deepseek.com` 若某天需要代理] → airouter 整体属直连组（narrow-outbound-proxy-scope D2 明确拒绝 per-destination 代理），这是已知能力缺口不是本 change 引入；届时与 AI 调用一起失败，排查方向明确。

## Migration Plan

1. 合并部署后：`searxng` Docker 容器可停可删（`docker rm -f searxng`），无其他消费者。
2. `ai_settings.searxng_config` 行留在库里不删（无代码读它，无害；避免碰真库 DELETE）。可在后续清理 change 中一并处理。
3. 回滚：git revert 本 change 即可恢复 SearXNG 链路；容器若已删需重新 `docker run`。

## Open Questions

- DeepSeek Search `count` 参数取 5 还是 10？取 10 能让跨 query 重排有更大候选池，但单次响应体更大（含 summary）。**可上线后调**，不影响 spec/tasks 结构。初始取 10。
