# Test Cases — switch-margin-note-search-to-deepseek

测试单元=Requirement 用户故事，本文串成完整故事；spec Scenario 只是断言片段。complexity: simple（无状态机；错误分类为枚举映射；合并重排为单函数纯逻辑）。

## ⓪ 契约核查：继承与调整

apply 启动时先跑 `bash scripts/harness/test-assets.sh daily-report-margin-notes settings-workspace web-search-provider` 反查旧资产并逐行处置：

| 旧 Scenario / 旧测试 | 处置 | 动作 |
| --- | --- | --- |
| 「搜索失败静默降级」「query 生成失败回退划词搜索」「编造 URL 白名单剔除」「纯模型知识标注口径」 | 保留不变 | 语义零变更，旧测试照跑绿 |
| 「搜索词随问题语义变化」「联网补强回答」 | 保留不变 | 语义不变，只换搜索后端实现 |
| 「联网正常时不显示标记」 | **语义翻转** | 原断言「ok 不渲染」改为「ok 且未引用 → 渲染『已联网 · 未采用』」；`MarginNoteCard.test.ts` 对应用例改断言，不改名 |
| 「联网失效时回答带降级标记」「未启用联网时不显示标记」「历史问答不显示标记」 | 保留不变 | — |
| `searxng_test.go` / `health_test.go` / `searxng_health_handler_test.go` / `SearxngConfigPanel.test.ts` | **删除** | 对应实现整体下线，由 `websearch/*_test.go` / `websearch_health_handler_test.go` / `WebSearchConfigPanel.test.ts` 接替 |
| `margin_notes_qa_test.go` 中 mock 搜索函数 | 签名变更 | 跟随 `websearch.Search` 新签名改 mock；断言增量补重排与 400 runes |
| `annotation_qas_web_search_status_migration_test.go` | 保留 | 列与三态枚举不变 |

## 故事一：读者提问后拿到更相关的联网参考

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 三条搜索词各自返回结果，第 3 条首条 rerank 最高 | 跨 query 合并按相关度重排 | 合并池去重后该条排首位；前 5 条按 Score 降序 | service 单测 | `margin_notes_qa_test.go` |
| 2 | 同一 URL 在两条搜索词中都出现 | （约束 25 去重保序） | 只保留首次出现的那条 | service 单测 | 同上 |
| 3 | DeepSeek 返回 1600 字 summary | （400 runes 截断） | 进 prompt 的该条摘要长度 ≤400 runes | service 单测 | 同上 |
| 4 | 搜索失败/超时/未配置 | 搜索失败静默降级 | 回答照常；无网络来源行；`web_search_status=empty`/`disabled` | service 单测 | 同上 |

## 故事二：读者在卡片上看清「搜了没」「用了没」

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | `ok` 且 `cited_web_sources.length>0` | 联网并被引用时显示已联网 | 渲染「已联网」`data-testid=mn-web-ok` | Vitest | `MarginNoteCard.test.ts` |
| 2 | `ok` 且 `cited_web_sources.length===0` | 联网正常时不显示标记（翻转后） | 渲染「已联网 · 未采用」`data-testid=mn-web-unused`，title 提示 | Vitest | 同上 |
| 3 | `empty` | 联网失效时回答带降级标记 | 渲染「联网搜索无结果」（现有） | Vitest | 同上 |
| 4 | `disabled` / NULL | 未启用联网时不显示标记 / 历史问答不显示标记 | 不渲染任何联网标记 | Vitest | 同上 |

## 故事三：运维者在设置面板确认 DeepSeek 搜索可用

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 无 DeepSeek vendor | 未配置时探测显示 disabled | `disabled` + `vendor_configured=false` 空态 + 去 AI 模型指引 | handler / Vitest | `websearch_health_handler_test.go`、`WebSearchConfigPanel.test.ts` |
| 2 | 点击「测试连接」，key 无效 | 密钥失效时探测指向鉴权 | `unreachable` + `error_kind=auth`，面板「鉴权失败」 | platform / handler / Vitest | `health_test.go`、handler 测试、面板测试 |
| 3 | 余额耗尽 | 余额耗尽时探测指向额度 | `unreachable` + `error_kind=quota`，面板「余额或额度不足」 | 同上 | 同上 |
| 4 | 服务可达但探针无结果 | 引擎全挂时探测显示 degraded | `degraded`，面板「服务可达但搜索无结果」 | 同上 | 同上 |
| 5 | 正常 | 点击测试连接获得四态结果 | `ok` + latency_ms；loading 态防重复点击 | 同上 | 同上 |
| 6 | 旧深链 `tab=searxng` | （Migration） | 重定向到 `tab=web-search` | Vitest | `SettingsWorkspace.test.ts` |

## 故事四：搜索入口自身的错误分类（platform 白盒，用完即弃）

> 本节为临时白盒测试，归档前按 `standard/shared/test-design.md` 评估是否保留；错误分类是本 change 新引入的外部契约（`web-search-provider` spec「错误分类」已固化为 Requirement），故**保留**为永久业务测试。

| 步 | 输入 | 期望 Kind | 落点 |
| --- | --- | --- | --- |
| 1 | 401 | `Auth` | `search_test.go` |
| 2 | 402 | `Quota` | 同上 |
| 3 | 403 + body 含 `quota`/`money` | `Quota` | 同上 |
| 4 | 403 + 其他 body | `Auth` | 同上 |
| 5 | 429 | `RateLimited` | 同上 |
| 6 | context deadline / net timeout | `Timeout` | 同上 |
| 7 | 500 / 连接拒绝 / 坏 JSON | `Unavailable` | 同上 |
| 8 | 空查询词 / >128 字符 | 本地拒绝，不发请求 | 同上 |
| 9 | 200 空 results | 无错误，空列表 | 同上 |
