<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board, scheduler -->
<!-- 域声明理由：编辑路径 backend-go/internal/tagmanagement/service/core/（→ semantic-board 打标契约）与 backend-go/internal/tagmanagement/service/core/tag_queue.go、handler/、repository/（→ scheduler 健康门与队列）命中 doc-impact-applies 标签 -->

# 删除 heuristic 规则词兜底打标（抽取失败即失败，交队列重试）

## Why

用户报障「版块『生成式 AI 与大模型厂商』（board 1980）大量显示 Coding / AI Agent 标签，11-11 日报只有 4 篇文章」。2026-11-12 诊断钉死：**症状有两副面孔，同一个病根**——`heuristic` 规则词兜底打标。

**病根本体**（`internal/tagmanagement/service/core/extractor_heuristic.go`，124 行）：7 条硬编码规则 `AI Agent` / `OpenAI` / `Anthropic` / `Multimodal` / **`Coding`（含匹配 "code"、"编程"、"开发"）** / `Infra` / `NVIDIA`，外加产品型号 token（GPT-4/Claude 3/Gemini/Llama）、feed 名首个大写词、分类名。

**两条触发路径**（都必须删）：
1. 提取层 `extractor_enhanced.go:50` —— `extractMergedCandidates` 失败 → `extractWithHeuristic` → 规则打标；
2. 编排层 `article_tagger.go:194` —— `ExtractTags` 返 err → `legacyExtractTopics` → 规则打标。

**为什么这是病根而不是表征**：
- **兜底吞掉了错误**：`extractWithHeuristic` 返回 `nil` error → 打标任务被标 `completed` → 该文章**永远不会被重打**，失败被永久掩盖；
- **真库实测污染量**：`article_topic_tags` 中 `source='heuristic'` 共 **1724 行**；`topic_tags` 中 `source='heuristic'` 共 **28 个**（其中 9 个挂在 board 1980，**`Coding` 与 `AI Agent` 正在其中**）；
- **后果一（版块泛词刷屏）**：`Coding` 规则含子串 "code"，命中即打标，score 0.881 挂上 board 1980；用户看到的就是满屏 Coding / AI Agent；
- **后果二（日报塌方）**：日报只消费 `category='event'` 的标签，这 1724 篇文章只有泛词、没有 event 标签 → 一篇都进不了日报。

**为什么现在删**：`3ecdf3b7`（摘除事件指纹融合抽取）已修掉触发源——带【事件指纹】段的 4B 模型调用失败率 11-06 达 41%，成功调用零失败，正是那次故障把大批文章推进了 heuristic 兜底。但**兜底路径本身还在**：任何未来的抽取失败（网络抖动、模型超时、provider 挂掉）都会重演同一出——泛词刷版块 + event 断粮 + 任务被误标完成。删掉兜底，失败就如实是失败。

**存量必须一起清**：只改代码不动存量，`Coding` / `AI Agent` 仍挂在版块上、日报仍缺文章——用户报障不会消失。

## What Changes

### 代码删除

| 文件 | 动作 |
| --- | --- |
| `internal/tagmanagement/service/core/extractor_heuristic.go` | **整文件删除**（`ExtractTopics` / `topicRules` / `extractFeedEntity` / `slugify` / `productTokenPattern` / `heuristicPunctPattern`——已核实全部无他处调用方） |
| `internal/tagmanagement/service/core/extractor_enhanced.go` | 删 `extractWithHeuristic`（:256-281）；`ExtractTags` 的 `if err != nil { return te.extractWithHeuristic(input, err) }` → **返回错误给调用方**；`ExtractionResult.Source` 字段删除（删除后恒为 `llm`，无存在价值） |
| `internal/tagmanagement/service/core/article_tagger.go` | 编排层 `err != nil` 分支：不再 `legacyExtractTopics`，**返回错误**（任务失败 → 队列重试）；`source = "heuristic"` 删 |
| `internal/tagmanagement/service/core/tagger.go` | 删 `legacyExtractTopics`（:40-56） |

### 失败语义变更（用户 2026-11-12 拍板）

抽取彻底失败（重试耗尽）时：**不写任何标签**（文章保持无标签）→ 打标任务标 `failed` → `tag_jobs` 既有退避重试机制生效（1 分钟 → 30 分钟，最多 `max_attempts=5`）→ 仍失败停在 `failed` 态，在标签队列面板可见（`last_error` 已展示），可经批量 retag 重打。

**不引入新机制**：`MarkFailed` / `failureBackoff` / `max_attempts` 全部是既有链路，本 change 只让错误能走到那里（此前被兜底吞掉）。

### 存量清理与重打

1. 删 `article_topic_tags` 中 `source='heuristic'` 的 **1724 行**；
2. 删「**无任何非-heuristic 引用**」的 heuristic 标签（实测 **14 个**，如 `ai新闻` / `游戏相关` / `技术` / `Infra` / `Multimodal` / `HackerNews`）及其挂载行、向量行（FK 全 CASCADE）；
3. 对「**已被 LLM 认领**」的 14 个 heuristic 标签（`Coding` / `AI Agent` / `OpenAI` / `Anthropic` / `Steam` / `NVIDIA` / `GPT-6` 等）只把 `source` 改为 `llm`，**保留标签本身**——它们已由 LLM 独立产出过，按字面删 28 个会误删真标签；
4. 只重算受影响文章的 `articles.tag_count`（全库存量另有约 1157 行历史不一致，不属本次范围，不动）；
5. 这批文章回到「无标签」态 → 批量 retag 重新 AI 打标 → 拿到真 event 标签 → 能进日报。

**清理必须在事务内执行**（`BEGIN; …; COMMIT;`），执行前 `count(*)` 留底并确认 `backups/pg-key-*.dump` 备份路径（红线：`standard/backend/testing.md` §🛑）。

### 规范与文档

- `openspec/specs/tagging-domain/spec.md`：Requirement「heuristic 兜底仅限提取调用失败」→ 改写为「提取失败不降级，直接判任务失败」；Requirement「泛词标签黑名单拦截」的写入路径清单去 `heuristic 兜底`；
- `openspec/specs/auxiliary-label/spec.md`：Requirement「Tag 提取单次调用输出 event/person 与 keyword 双数组」中两条 heuristic 条款（整体失败回退 heuristic / keyword 空时用 heuristic 展示兜底——后者**早已与代码脱节**，`mergeExtractedTags` 从未实现该兜底）一并改写；
- `docs/reference/flow/semantic-board.md`：红线 19 与顶部红线段（「仅提取调用 err != nil 时降级」）改为「不降级，失败即失败」；
- `docs/reference/flow/scheduler.md`：健康门理由（「打标不可用时文章只会带 heuristic 降级标签入库，污染下游」）改为「文章无标签入库，不污染下游」；
- `docs/reference/database/tables/topic-tags.md`：`topic_tags.source` 与 `analyses.source` 枚举去 `heuristic`；
- `docs/reference/flow/reading.md`：提取容错节的「回落 mono（含 heuristic 兜底）」去兜底表述；
- `docs/reference/database/DATA_LIFECYCLE.md`：Tag 术语口径的 source 取值去 `heuristic`；
- `openspec/specs/tagging-domain/spec.md` 的 `Tagging domain package structure`：该 requirement 此前仍停留在 `internal/domain/tagging/` 旧布局且引用已删除的 `extraction.ExtractTopics`，本 change 一并拉回实际目录结构（`internal/tagmanagement/` + `service/<子包>`）。

## Capabilities

### Modified Capabilities

- `tagging-domain`（`openspec/specs/tagging-domain/spec.md`）：打标失败语义——删除 heuristic 兜底 requirement，改为「提取失败不降级、任务失败重试」；泛词黑名单的写入路径清单同步。
- `auxiliary-label`（`openspec/specs/auxiliary-label/spec.md`）：双数组提取 requirement 的失败与空 keyword 兜底条款改为「不回退 heuristic」。

## Impact

- **代码**：后端 4 个文件（`extractor_heuristic.go` 删除 + 3 个改动）+ 6 个测试文件（`extractor_test.go` / `extractor_enhanced_test.go` / `article_tagger_test.go` / `sampling_path_test.go` / `hard_merge_test.go` / `extractor_generic_subject_test.go`）；前端**零改动**（`TagQueuePanel.vue` 已展示 `last_error`，失败原因本来就可见）。
- **数据**：`article_topic_tags` -1724 行、`topic_tags` -28 个（及其挂载/向量行）、`articles.tag_count` 重算；随后 1724 篇文章重新 AI 打标（本地 4B 模型，按当前吞吐需一段时间）。
- **行为变化**：
  - ✅ 今后抽取失败**不再**产生 Coding / AI Agent 等泛词标签，也不再把失败伪装成成功；
  - ✅ 失败文章会被队列自动重试 5 次（此前一次都不重试）；
  - ⚠️ 打标不可用时，文章以**无标签**状态入库（此前带泛词标签）——这正是 `scheduler.md` 健康门想要的结果；
  - ⚠️ 存量 1724 篇文章在重打完成前不进日报（此前也不进——它们没有 event 标签，只是多了泛词噪音）。
- **回滚**：`git revert` 本 change 提交即可恢复兜底路径；**存量删除的数据不可自动恢复**（需重新 retag，或从 `backups/pg-key-*.dump` 备份还原——执行前已留底）。
