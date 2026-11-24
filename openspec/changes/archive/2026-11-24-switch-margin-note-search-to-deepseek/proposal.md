<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: daily-report, discovery -->

## Why

页边注问答的联网参考一直靠本地 SearXNG，实测（2026-11-23）只剩 google cse 一个引擎在响应（brave 限流 / duckduckgo CAPTCHA / wikidata 拒绝），单次搜索 4.0 秒且结果偏题；同一问题 DeepSeek Search API 0.46 秒返回权威来源。SearXNG 部署运维成本高、效果差，直接整体替换。

## What Changes

- 页边注联网后端由本地 SearXNG **整体替换**为 DeepSeek Search API（`POST https://api.deepseek.com/v1/tools/search`）；**移除** `internal/platform/searxng` 包及其配置键 `searxng_config`、路由 `/api/settings/searxng(+/health)`、前端 `SearxngConfigPanel`，不再保留 SearXNG 作为可选 provider。
- 新增统一网页搜索入口 `internal/platform/websearch`，密钥复用 `ai_vendors` 中 provider_type=`openai_compatible` 且 base_url 命中 DeepSeek 的 vendor 记录（不新增密钥存储）。
- 多 query 合并池改为**跨 query 按相关度分重排**后取前 5，替代现在的「按 query 顺序拼接、保留引擎顺序」。
- 搜索摘要进 prompt 的上限由每条 200 runes 提到 400 runes（DeepSeek 摘要比 SearXNG snippet 更长更完整）。
- 设置面板：数据源区移除 SearXNG 配置入口，改为「网页搜索」配置，展示 DeepSeek 连通性与余额状态。
- 页边注卡片：QA 落库的 `web_search_status` 新增可见标记（已联网 / 已联网但未采用 / 未联网），**不改**静默降级语义与回答形态。
- **BREAKING**：`searxng_config` 配置键与 `/api/settings/searxng` 路由下线；依赖该键的旧部署需改为配置 DeepSeek vendor key。

## Capabilities

### New Capabilities
- `web-search-provider`: 统一网页搜索入口的契约——配置来源与优先级、请求/结果形状、错误分类、健康探测、失败语义。

### Modified Capabilities
- `daily-report-margin-notes`: 联网参考块的数据源由 SearXNG 改为统一网页搜索入口；多 query 合并改为跨 query 相关度重排；摘要上限 200→400 runes；`web_search_status` 语义不变但新增「已联网未采用」的可见标记。
- `settings-workspace`: 数据源区的 SearXNG 配置面板替换为网页搜索配置面板，探测四态沿用。

## Impact

- **后端**：删除 `internal/platform/searxng/`；新增 `internal/platform/websearch/`；改 `topicgraph/service/margin_notes_websearch.go`、`margin_notes_qa.go`（仅合并与截断逻辑，不动 prompt 与降级树）；改 `platform/aisettings/config_store.go`、`discovery/handler/discovery_handler.go`、`discovery/routes.go`。
- **前端**：删 `front/app/api/searxng.ts`、`components/dialog/SearxngConfigPanel.vue`（及测试）；新增对应 web-search 文件；改 `SettingsSectionDatasourcesNetwork.vue`、`SettingsWorkspace.vue`、`api/index.ts`、`MarginNoteCard.vue`。
- **配置**：`SEARXNG_URL` 环境变量与 `config.yaml` 的 `searxng.endpoint` 失效；DeepSeek 密钥取自已有 `ai_vendors` 记录，无需新增配置项。
- **数据**：`annotation_qas.web_search_status` 取值集合不变（`disabled|ok|empty`），旧行 NULL 不回填。
- **成本**：DeepSeek Search 按成功请求计费 5 元/千次，一次提问最多 3 次搜索；`results` 为空不计费。
- **明确不做**：不改 QA 模型绑定、不改 prompt 的「仅供参考 / 允许空引用」措辞、不引入模型侧工具调用（airouter 仍无 `tools` 字段）。
