# Explore Findings — switch-margin-note-search-to-deepseek

> 2026-11-23 探索阶段实测与代码事实。实现阶段直接据此动手，无需重探。

## 实测数据（树莓派本机，2026-11-23）

| 指标 | SearXNG（现状，`http://localhost:8889`） | DeepSeek Search API |
| --- | --- | --- |
| 单次延迟 | **4.03s**（brave `too many requests` / duckduckgo `CAPTCHA` / wikidata `access denied`，只剩 `google cse`） | **0.46s**（直连，connect 40ms） |
| 排序信号 | `score` 有但 `searxng.go:43-47` 丢弃 | `rerank_score` 0~3，实测 1.49/1.43/1.30/1.21/1.19 |
| 权威/时效 | 无 | `authority` 0~4、`date` RFC3339、`site_name` |
| 摘要长度 | ~100 字，截 200 runes | `snippet` 58~126 字 / `summary` 239~1596 字 |
| 结果质量（同 query「2019年朝鲜战争为何未结束」） | web.dev「什么是测试」 | 联合早报、ABC 中文、公共电视、俄卫星 |

- `annotation_qas.web_search_status` 分布：ok 3 / empty 3 / NULL 26（五五开）
- 真实审计（11-22 两轮）：query 生成成功（4B 模型 522ms），回答 `web_sources: []` —— **搜索每次都跑了，是模型主动不引用**（prompt「仅供参考」+ 显式许可空数组）。本 change 不动 prompt（Non-Goal）。
- **Bocha key 已欠费**：`403 You do not have enough money or package quota`。`dataenrichment` 域 `web_search` 工具一直在失败，不在本 change 范围。

## DeepSeek Search API 契约

- `POST https://api.deepseek.com/v1/tools/search`，`Authorization: Bearer <key>`（与 chat 同一把 key）
- 请求：`{query(1~128字符, 支持 site:/lang:/after: 语法), count(1~50, 默认10), need_content(bool, 默认false), ori_question?, localization?}`
- 响应：`{results: [{rerank_score, title, url, snippet, summary, content, site_name, site_icon, date, authority}]}`
- 计费：5 元/千次成功请求（`results` 非空才计费）；QPS 100
- 响应体顶层只有 `results` 一个键

## 凭据现状（`ai_vendors`）

```
name=DeepSeek | base_url=https://api.deepseek.com/v1 | provider_type=openai_compatible | enabled=t | has_key=t
  └ deepseek-flash (thinking=off, ok)
  └ deepseek-v4-pro (thinking=low, ok)
```
已绑 `digest_polish` / `data_enrichment_analysis` 降级位；**未绑 `open_notebook`**（本 change 不动）。

## 现有代码落点（要删/要改）

| 文件 | 处置 |
| --- | --- |
| `backend-go/internal/platform/searxng/{searxng,health}.go` + tests | **删除** |
| `backend-go/internal/platform/searxng/searxng.go:58-96` | `Search(ctx, endpoint, query)`：GET `/search?q=&format=json&language=zh-CN`，8s 超时，只解析 title/url/content，截 200 runes，4MB body 上限 |
| `backend-go/internal/platform/searxng/health.go:58-74` | `ResolveConfiguredEndpoint`：DB `searxng_config` > env `SEARXNG_URL` > yaml `searxng.endpoint`；`enabled=false` 不是绝对否决（会落到 env） |
| `backend-go/internal/platform/searxng/health.go:87-125` | `ProbeHealth` 四态 `disabled/unreachable/degraded/ok`，探针词 `connectivity probe` |
| `backend-go/internal/topicgraph/service/margin_notes_websearch.go:46-52` | `marginNoteWebSearchFn` 包变量（测试可替换）→ 调 `searxng.Search` |
| 同文件 `:68-70` | 每次（含并行 goroutine 内）解析 endpoint |
| 同文件 `:233-252` | `searchWebForMarginNoteWithQueryGen` 降级树入口，返回 `(results, status)` |
| 同文件 `:258-280` | `searchWebParallelForMarginNote` 多 query 并行扇出（≤3） |
| 同文件 `:285-305` | 合并：URL trim 去重保序，按 query 顺序拼接，截前 5 —— **改为按 Score 重排** |
| 同文件 `:41` | `marginNoteSearxngResultLimit = 5` |
| `backend-go/internal/topicgraph/service/margin_notes_qa.go:358-372` | `searchWebForMarginNote` 单 query 回退路径 |
| 同文件 `:72-107` | `AskMarginNote` 主干；`:108` capability `open_notebook` |
| 同文件 `:293-334` | `buildMarginNoteQAPrompt`；`:328` 联网块「仅供参考」；`:329-334` 仅 `len(webResults)>0` 拼块 |
| 同文件 `:378` | `filterWebSourcesAgainstCandidates` 白名单 |
| `backend-go/internal/topicgraph/handler/margin_notes_handler.go:38,157-175` | `POST /annotations/:id/questions` |
| `backend-go/internal/discovery/routes.go:60-63` | `GET/POST /settings/searxng`、`GET /settings/searxng/health` —— **替换为 `/settings/web-search`、`/settings/web-search/health`** |
| `backend-go/internal/discovery/handler/discovery_handler.go:457-536` | `GetSearxngHealth`（457-469，外层 8s ctx）/ `GetSearxngSettings`（473-491）/ `SaveSearxngSettings`（502-536） |
| `backend-go/internal/platform/aisettings/config_store.go:22,130-138` | `searxngConfigKey` / `LoadSearxngConfig` / `SaveSearxngConfig` —— **删除** |
| `backend-go/internal/platform/config/config.go:143-144,189,288-289` | `Searxng.Endpoint` 字段、env 绑定、yaml 读取 —— **删除** |
| `front/app/api/searxng.ts` + `api/index.ts:13-14` | **删除/替换** |
| `front/app/components/dialog/SearxngConfigPanel.vue` + `.test.ts` | **删除**，新建 `WebSearchConfigPanel.vue` |
| `front/app/features/settings/components/SettingsSectionDatasourcesNetwork.vue:8,28` | tabs 中 `searxng` 项 → `web-search` |
| `front/app/features/settings/components/SettingsWorkspace.vue:43,51` | 分区描述文案；`legacyTabMap` 加 `searxng → web-search` |
| `front/app/features/tags/components/daily-report/MarginNoteCard.vue:154-161` | 只判 `empty` 渲染「联网搜索无结果」；`:165-166` `mn-qa__pure` 样式族可复用 |
| `front/app/features/tags/components/daily-report/MarginNoteCard.test.ts` | 「联网正常时不显示标记」用例需翻转断言 |

## 红线文档落点（归档前必须同步）

- `docs/reference/flow/daily-report.md:350`（约束 25）：「提问当次的 SearXNG 合并搜索结果池」「SearXNG 只用原始网页结果（无 AI 总结模式…）」「落点 `platform/searxng/`」
- `docs/reference/flow/daily-report.md:353`（约束 27）：「`platform/searxng.ResolveConfiguredEndpoint`」「`searxng.Search`」「`GET /api/settings/searxng/health`」「`SearxngConfigPanel.vue`」
- 其他含 searxng 段：`configuration.md`、`api/discovery.md`、`api/daily-reports.md`、`architecture/backend.md`、`flow/ai-summary.md`、`flow/data-enrichment.md`、`database/tables/margin-notes.md`

## 不碰的东西

- prompt 措辞、模型绑定（`open_notebook` 仍只有本地 4B）、airouter 无 `tools` 字段、Bocha 链路、`annotation_qas` 列定义与三态枚举。
- 约束 25「单次回答调用 + 前置 query 生成」与约束 27「探测不走 airouter 不落 ai_call_logs」语义零变更。
