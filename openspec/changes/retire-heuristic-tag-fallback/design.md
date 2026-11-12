# Design — retire-heuristic-tag-fallback

## D1. 删除范围（精确锚点）

### 后端

| 文件 | 位置 | 动作 |
| --- | --- | --- |
| `internal/tagmanagement/service/core/extractor_heuristic.go` | 全文件（124 行） | **整文件删除**。含 `topicRule` 类型、`topicRules` 7 条规则、`productTokenPattern`、`heuristicPunctPattern`、`ExtractTopics`、`extractFeedEntity`、`slugify`。已核实：`slugify`（小写）与 `Slugify`（导出）是两个不同函数，本文件的 `slugify` **无他处调用方**；`extractFeedEntity` / `productTokenPattern` / `heuristicPunctPattern` / `topicRules` 均为本文件私有。**实施时发现设计漏记**：同包测试有 3 个用例直接调用 `ExtractTopics`（`extractor_test.go` 的 `TestExtractTopicsFindsCanonicalAITopicsAndEntities` / `...DeduplicatesAliases` / `...FallsBackToFeedAndCategoryWhenTextIsSparse`）——函数随文件退役，这 3 条**只能删除**（不是改断言），连带删掉只剩它们使用的 helper `topicSlugs` / `countMatches` / `findTopic`（`unused` linter 会红）；`topicLabels` 被 `extractor_enhanced_test.go` 使用，保留 |
| `internal/tagmanagement/service/core/extractor_enhanced.go` | :256-281 `extractWithHeuristic` | 函数删除 |
| 同上 | :50-52 `ExtractTags` 的失败分支 | `return te.extractWithHeuristic(input, err)` → `return nil, err`（错误上抛给编排层） |
| 同上 | :38 `ExtractionResult.Source` 字段 | 删除（删除后恒为 `"llm"`，字段无存在价值；:82/:107 两处构造同步去字段） |
| `internal/tagmanagement/service/core/article_tagger.go` | :191-199 编排层失败分支 | `tags = legacyExtractTopics(input); source = "heuristic"` → **返回错误**（`fmt.Errorf` 带上原始 err），不再产出任何标签 |
| 同上 | :198 `source = result.Source` | → `source = "llm"`（常量；`ExtractionResult.Source` 删除后的唯一取值） |
| `internal/tagmanagement/service/core/tagger.go` | :40-56 `legacyExtractTopics` | 函数删除（唯一调用方是 `article_tagger.go:194`，随之删除） |

**保留**（与本次删除无关、不得连带清理）：`filterGenericLabels` 泛词黑名单（LLM 侧仍可能产出「新闻」等泛词，黑名单仍有用）、`limitArticleTags`、`findOrCreateTag` 的三级去重、跨 feed reuse 路径。

### 测试

| 文件 | 动作 |
| --- | --- |
| `internal/tagmanagement/service/core/extractor_test.go` | :145-158 parse err 分支断言 `Source=="heuristic"` → 改为断言 `err != nil` 且 `result == nil`；:404-422 「重试耗尽仍失败 → 整体回退 heuristic」用例 → 改为「重试耗尽仍失败 → 返回错误，不产出任何标签」 |
| `internal/tagmanagement/service/core/extractor_enhanced_test.go` | :38-56 零候选用例断言 `result.Source == "llm"` → 字段删除后改为断言零标签 + 不触发降级；:13 注释更新 |
| `internal/tagmanagement/service/core/article_tagger_test.go` | :119-162 「零 → 文章不写入任何行、不产生 heuristic 来源标签」用例保留并反向强化；:158 `link.Source == "heuristic"` 断言 → 改为「任务返回错误且文章无任何标签行」 |
| `internal/tagmanagement/service/core/sampling_path_test.go` | :19 注释「让 tagArticle 走 heuristic fallback 并把标签 persist 进测试 DB」→ 该用例整体意图失效，改为「抽样路径下提取失败 → 任务失败、文章无标签」 |
| `internal/tagmanagement/service/core/hard_merge_test.go` | :79 fixture 的 `ArticleTopicTag{Source: "heuristic"}` **是种子数据不是注释** → 改为 `Source: "reuse"`（保「多来源 link 迁移」覆盖，值取合法来源） |
| `internal/tagmanagement/service/core/extractor_generic_subject_test.go` | 仅注释提及 heuristic → 注释同步 |

**改注释（review L2 更正）**：`internal/platform/analysispause/health_gate_compose_test.go:310` 原文「producing heuristic-degraded tags」**就是本链路**（原文设计判其「命中无关英文词 heuristics」有误）；注释改为「landing untagged（规则词兜底已退役）」，测试行为不变。

### 顺带清理的注释残留（非本 change 的行为面，但注释里的取值集合已失真）

| 文件 | 位置 | 动作 |
| --- | --- | --- |
| `internal/models/topic_graph.go` | :28 / :97 `// llm, heuristic, manual` | → `// llm, manual（heuristic 已于 2026-11-12 退役）` |
| `internal/tagmanagement/models/tag_models.go` | :59 `// ai, heuristic, cached` | → `// ai, cached（heuristic 已于 2026-11-12 退役）` |
| `internal/platform/aihealth/aihealth.go` | :65 健康门理由「文章只会带 heuristic 降级标签入库，污染下游」 | → 「文章只能以无标签入库（规则词兜底已退役）」——门禁本身不变 |

## D2. 失败语义变更（队列重试链路）

现状与改后对照：

```
现状（heuristic 兜底）：
  LLM 失败 → extractWithHeuristic 吞掉错误 → 返回 Tags(泛词), Source=heuristic, err=nil
           → article_tagger 走 else 分支 → persistArticleTags 写入泛词
           → tag_jobs 标 completed（★ 失败被永久掩盖，文章永不重打）

改后：
  LLM 失败（重试耗尽）→ ExtractTags 返回 err
           → article_tagger 返回 err（不写任何标签）
           → tag_queue.go:304 MarkFailed(job, msg, failureBackoff(attempt))
           → status=pending, available_at=now+backoff（1→2→4→8→16 分钟；max_attempts=5 时 30 分钟档不可达）
           → attempt_count 累加；>= max_attempts(5) → status=failed（终态，可人工/批量 retag）
```

**不引入新机制**：`MarkFailed` / `failureBackoff` / `max_attempts` / 「failed 态在标签队列面板可见 + `last_error` 展示」全部是既有链路（`front/app/features/settings/components/TagQueuePanel.vue:282`）。本 change 只让错误能走到那里。

**聚合（aggregate）路径不变**：`tagAggregateArticle` 失败仍回落 mono 路径（`article_tagger.go:172-188`），mono 也失败才算整体失败——这是既有编排，不在本 change 范围。

## D3. 存量清理与重打

### 清理范围（真库实测，2026-11-12）

| 对象 | 数量 | 说明 |
| --- | --- | --- |
| `article_topic_tags` where `source='heuristic'` | 1724 行 | 这些文章的唯一标签来源；删后文章回到「无标签」 |
| `topic_tags` where `source='heuristic'` | 28 个 | 含 `Coding`、`AI Agent`（board 1980 上挂 9 个） |
| `topic_tag_board_labels` 引用上述 28 个标签的行 | 随删 | 删标签前先删挂载行（或依赖 FK 级联，**执行前必须先确认 FK 行为**） |
| `topic_tag_embeddings` 引用上述 28 个标签的行 | 随删 | 同上 |
| `articles.tag_count` | 重算 | 受影响文章的标签计数归零 |

### 执行纪律（红线：`standard/backend/testing.md` §🛑）

1. 执行前 `count(*)` 留底并确认 `backups/pg-key-*.dump` 备份路径；
2. **先在 `BEGIN; …; ROLLBACK;` 事务里演练**，核对影响行数与 FK 行为，确认无误再正式提交；
3. 正式执行**仍包在事务里**（`BEGIN; …; COMMIT;`），不裸跑 `psql -c`；
4. `topic_tags` 的删除顺序：先 `topic_tag_board_labels` → `topic_tag_embeddings` → `article_topic_tags` → `topic_tags`，避免依赖级联行为的不确定性。

### 重打

清理后这批文章处于「无标签」态。批量 retag 走既有 `POST /api/tag-jobs/retag-today`（`handler/tag_queue_handler.go:274`）或逐篇 `RetagArticle`（`article_tagger.go:107`，Force=true 不复用 sibling 标签）。

**注意**：重打要调 LLM（本地 4B 模型）。**根因（事件指纹）已在 `3ecdf3b7` 摘除，重打前应先验证单篇调用成功率**——这是本 change 的效果核对前置条件。

### D3.1 执行期发现的两处问题（已修，留痕）

**① 无 FK 表的历史孤儿导致清理脚本超时**——`auxiliary_description_repair_items` / `tag_merge_suggestions` / `embedding_queues` 对 `topic_tags` **无外键**，初版脚本用 `NOT EXISTS (SELECT 1 FROM topic_tags ...)` 全表扫，命中**存量历史孤儿**（repair_items 53488 行、merge_suggestions 10802 行）→ 740M 次比较 → 演练卡死 >180s。
修正：先用临时表 `_deleted_heur_tags` 锁定本次待删的 14 个标签 id，清理语句只针对该小集合（`IN (SELECT id FROM _deleted_heur_tags)`）→ 演练秒级完成，**存量孤儿不越界清理**（那是另一笔账）。

**② `articles.tag_count` 的批量 UPDATE 极慢**——`articles` 行含 `content` / `ai_content_summary` 等宽列，UPDATE 整行重写 → WAL 刷盘；1142 行相关子查询形态在本机实测 200 行就要 7s（且与后台 quality_score 作业抢 WAL/IO，load 飙到 13）。
修正：① 改两段式（先置 0、再按剩余关联用一次聚合回填）；② **清理期间停后端**（`scripts/dev/start-dev.sh stop back`），消除并发写。
另注：`article_tagger.go:404` 的打标路径**本来就会重算 tag_count**（`UPDATE articles SET tag_count = (SELECT COUNT(*) ...)`），所以重打会顺带修正受影响文章的计数——清理阶段的显式重算只是让状态立即一致。

### D3.2 重打阻塞：模型输出「schema 外壳」形态（独立根因，另案）

20 篇试验批（11-11 无标签文章）实测：**模型 82% 的响应是 schema 外壳**——

```json
{"type":"object","properties":{"event_person_tags":[...],"keyword_tags":[...]},"required":["event_person_tags","keyword_tags"]}
```

数组被塞在 `properties` 里，而解析器只认顶层键 → `merged extraction object has neither event_person_tags nor keyword_tags keys` → 任务失败。

`ai_call_logs` 的形态分布（按天）：

| 日期 | 调用数 | schema 外壳 | 顶层双数组（正常） |
| --- | --- | --- | --- |
| 11-05 | 2168 | 0 | 1011 |
| 11-06 | 17519 | 0 | 1164 |
| 11-09 | 5388 | 0 | 1918 |
| 11-10 | 5248 | 1774 | 547 |
| 11-11 | 3659 | 2902 | 229 |
| 11-12 | 1423 | 1168 | 112 |

**结论**：模型输出形态问题**早于**本 change 且**独立于** heuristic 兜底——即便兜底还在，这些文章也只是得到泛词而非 event 标签（日报一样缺）。本 change 的价值是让失败**可见 + 可重试**，而非掩盖。
**处置**：解析器容错（识别 schema 外壳并取 `properties` 内的双数组）属**另一个 change**（`tolerate-schema-envelope-extraction-output`）；修好后统一重打这 2488 篇（1142 篇本次受影响 + 1345 篇本来无标签），否则重打无产出、白烧本地 4B 算力。
本次试验批（20 篇）已置 `failed` 暂停，不继续消耗。

## D4. 明确不做

- **不动 `downgraded` 标记退役**（`2026-11-12-retire-downgraded-mount` 已归档上线，用户 2026-11-12 明确「保留，不回滚」）；
- **不动泛词黑名单**（LLM 侧仍可能产出「新闻」「技术」等泛词，黑名单仍要拦）；
- **不动 `min(2, N)` 放宽准入**、不动方向校验、不动 AI 路由降级观测（`DegradationPanel`，同名不同物）；
- **不动 aggregate → mono 的回落编排**（既有设计，aggregate 失败回落 mono 不是「泛词兜底」）；
- **不新增重试机制**（复用既有 `MarkFailed` + `failureBackoff`）。

## D5. 规范与文档同步点

| 位置 | 改什么 |
| --- | --- |
| `openspec/specs/tagging-domain/spec.md` | Requirement「heuristic 兜底仅限提取调用失败」→ **改写为**「提取失败不降级，直接判任务失败」；Requirement「泛词标签黑名单拦截」的写入路径清单去掉 `heuristic 兜底` |
| `openspec/specs/auxiliary-label/spec.md` | Requirement「Tag 提取单次调用输出 event/person 与 keyword 双数组」：「整体失败回退 heuristic」→ 「整体失败即失败，任务失败重试」；「keyword 空时用 heuristic keyword 展示兜底」→ 「不生成 heuristic keyword 兜底」（该条款**早已与代码脱节**：`mergeExtractedTags` 从未实现） |
| `docs/reference/flow/semantic-board.md` | 顶部红线段与红线 19：「仅提取调用 err != nil 时降级 heuristic」→ 「不降级，失败即失败交队列重试」 |
| `docs/reference/flow/scheduler.md` | 健康门理由：「文章只会带 heuristic 降级标签入库，污染下游」→ 「文章无标签入库，不污染下游」 |
| `docs/reference/database/tables/topic-tags.md` | `topic_tags.source` 与 `analyses.source` 枚举去 `heuristic`（历史值由 D3 清理清零） |
| `docs/reference/flow/reading.md` | 提取容错节：`aggregate → mono` 回落链路去「含 heuristic 兜底」表述 |
| `docs/reference/database/DATA_LIFECYCLE.md` | Tag 术语口径的 `source` 取值去 `heuristic` |
| `openspec/specs/tagging-domain/spec.md`（包结构 requirement） | 该 requirement 停在 `internal/domain/tagging/` 旧布局且引用已删除的 `extraction.ExtractTopics`；本 change 拉回实际结构并修正「依赖方向全部单向」场景断言 |
