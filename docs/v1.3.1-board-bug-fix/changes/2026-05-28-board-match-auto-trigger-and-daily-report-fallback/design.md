## Context

当前 board 匹配管道存在触发断裂：

1. 文章入库 → `tagArticle` → `findOrCreateTag` + `AttachAuxiliaryLabels` + `Enqueue(embedding)`
2. `EmbeddingQueueService.processNext` 生成 identity/semantic/event_keyword embedding 后标记完成
3. **断裂**：没有后续步骤调用 `MatchTopicTag`
4. `MatchTopicTag` 只在手动 backfill 时被 `SemanticBoardBackfillService.processJob` 调用

结果：backfill 之间创建的 event tag 的 `topic_tag_board_labels` 为空。日报的 `collectBoardTags` 通过 `topic_tag_board_labels` JOIN 收集 tag，因此遗漏大量 tag。

实际案例：伊朗局势 board (2853)，5.27 有 33 个相关事件标签（26 篇文章），但只有 tag 98636（5.26 创建、5.26 backfill 匹配的）被日报收录，日报仅 1 篇文章。

相关代码：
- Embedding 队列：`backend-go/internal/domain/tagging/embedding_queue.go` (`processNext`)
- Board 匹配核心：`backend-go/internal/domain/tagging/semantic_board_matching.go` (`MatchTopicTag`)
- 日报收集：`backend-go/internal/domain/daily_report/generator.go` (`collectBoardTags`)
- Backfill 服务：`backend-go/internal/domain/tagging/semantic_board_backfill.go`
- 前端匹配详情：`front/app/features/tags/components/MatchDetailPanel.vue`
- 前端 tag chip：`front/app/features/tags/components/TagsPage.vue`

## Goals / Non-Goals

**Goals:**
- embedding 完成后自动触发 board 匹配，确保新 event tag 在 embedding 就绪后立即获得板块归属
- 日报收集增加兜底补算，即使自动触发因任何原因未执行，日报也不遗漏有辅助标签的 tag
- max_sim 降级匹配（minHits 因辅助标签不足而降低）在匹配结果和前端展示中明确标记和区分

**Non-Goals:**
- 不修改匹配算法本身（direct_hit / hit_rate / max_sim / weighted 的计算逻辑不变）
- 不修改 backfill 机制（手动 backfill 仍可用）
- 不修改辅助标签提取流程
- 不修改日报生成的 LLM 调用逻辑
- 不做历史数据迁移（`downgraded` 字段默认 false，用户可通过 backfill 重算）

## Decisions

### D1: Embedding 完成后自动触发 board 匹配

**决策**: 在 `processNext` 的 embedding 生成完成、标记 completed 之前，对 event tag 调用 `SemanticBoardMatchingService.MatchTopicTag`。

**实现方式**: 新增包级函数 `getSemanticBoardMatchingService()`（`sync.Once` 单例），在 `EmbeddingQueueService.processNext` 中通过该函数获取实例调用 `MatchTopicTag`。不修改 `EmbeddingQueueService` 构造函数签名（避免破坏 `getEmbeddingQueueService` 等 4 个调用点），与现有 `getEmbeddingQueueService()` 的单例模式保持一致。`MatchTopicTag` 本身是幂等的（会 replaceTopicTagBoardLabels），重复调用不会产生脏数据。

**时机选择**: 在所有 embedding（identity + semantic + event_keyword）都生成完毕之后、标记 queue task completed 之前触发。此时 tag 的辅助标签已就绪（`AttachAuxiliaryLabels` 在 `tagArticle` 中先于 embedding 执行），embedding 也已生成，匹配所需数据完整。

**理由**: 这是最自然的触发点——embedding 是匹配的前置条件，embedding 完成即意味着匹配所需的向量数据已就绪。

**备选方案**:
- 在 `tagArticle` 末尾触发 → 被否决，因为此时 embedding 可能还在队列中，匹配无法计算相似度
- 在 `AttachAuxiliaryLabels` 后触发 → 被否决，因为辅助标签有但 embedding 没有，只能做 direct_hit 匹配，无法计算相似度匹配
- 修改 `EmbeddingQueueService` 构造函数接受 matcher 参数 → 被否决，需同步修改 `NewEmbeddingQueueService` 的 4 个调用点，且与现有单例模式不一致

### D2: 日报收集兜底补算

**决策**: `collectBoardTags` 在现有 SQL 查询（通过 `topic_tag_board_labels` JOIN）之外，增加一个补算路径：对于有辅助标签但无 `topic_tag_board_labels` 记录的 event tag，现场调 `MatchTopicTag` 补算，然后将补算结果合并到日报输入中。

**实现方式**: 在现有查询后，额外查询指定日期范围内有文章关联的 event tag，这些 tag 有辅助标签（`topic_tag_semantic_labels` JOIN）但无 `topic_tag_board_labels` 记录，对这些 tag 调用 `MatchTopicTag`，然后从匹配结果中过滤出匹配到当前 `boardID` 的 tag，合并到日报输入。**补算上限为 50 个 tag**，防止历史数据首次触发时批量补算导致日报生成过慢。

**理由**: 日报是面向用户的最终产物，不应依赖上游管道是否完美运行。这是兜底层，正常情况下自动触发（D1）已覆盖，此路径极少触发。

**备选方案**:
- 日报生成前强制全量 backfill → 被否决，太重，日报生成变慢
- 不做兜底，只依赖 D1 → 被否决，如果自动触发因服务重启等原因未执行，日报仍会遗漏
- 不设补算上限 → 被否决，D1 上线前可能有大量未匹配历史 tag，首次日报生成可能超时

### D3: 降级匹配标记

**决策**: 在 `evaluateSemanticBoardMatches` 中，对 max_sim 规则判断 `minHits` 是否被降级（即 `min(config.DirectMaxSimMinHits, len(tagAuxiliaries)) < config.DirectMaxSimMinHits`），如果降级则在匹配结果中设置 `Downgraded: true`。其他规则（direct_hit、hit_rate、weighted）的 `Downgraded` 始终为 `false`（Go 零值即为 false，无需显式设置）。`replaceTopicTagBoardLabels` 写入时将该标记持久化到 `topic_tag_board_labels.downgraded` 列。

**hits 与 minHits 的关系**: `hits` 的值来源于 `scoreSemanticBoardSimilarity` 返回的 `hitRate × max(len(tagAuxiliaries), config.MinEffectiveSample)` 四舍五入，代表实际超过 sim_threshold 的辅助标签数；`minHits` 是 `min(config.DirectMaxSimMinHits, len(tagAuxiliaries))`。两者分母不同——`hits` 用 `MinEffectiveSample` 调整后的分母，`minHits` 用原始 `len(tagAuxiliaries)`。降级仅看 `minHits` 是否小于 `config.DirectMaxSimMinHits`。

**数据模型变更**: `topic_tag_board_labels` 新增 `downgraded boolean NOT NULL DEFAULT false`。

**API 变更**: 两条 API 路径返回 `downgraded` 字段：
- `GET /api/semantic-boards/:id/match-detail/:tagId` → `getTagMatchDetail` 返回 `downgraded` + `effective_min_hits`（用于 MatchDetailPanel）
- `GET /api/semantic-boards/:id/articles` → board articles 查询返回 `downgraded`（用于 TagsPage tag chip 显示）

**前端变更**:
- `MatchDetailPanel.vue`: 匹配流程步骤 ④ 中，当降级时显示 "⚠ 降级匹配（原阈值 2，因仅有 1 个辅助标签降为 1）"
- `TagsPage.vue` 的 tag chip: 降级匹配的 chip 使用更淡的边框色和 "↓" 后缀标记

**理由**: 降级匹配在单辅助标签场景下是合理的设计（否则这些 tag 永远无法通过 max_sim），但需要让用户知道这是"放宽了条件的匹配"，以便判断匹配质量。

**备选方案**:
- 将降级匹配单独存为不同 match_reason（如 "max_sim_downgraded"）→ 被否决，match_reason 是匹配算法标识，降级不是不同算法
- 不持久化，前端实时计算 → 被否决，tag chip 列表需要知道降级状态，不想每条都实时算

### D4: 自动触发的错误处理

**决策**: 自动触发 `MatchTopicTag` 失败时只 log warning，不影响 embedding task 标记 completed。原因：匹配失败不应阻塞 embedding 流程；日报兜底（D2）会补上。

**理由**: 匹配不是 embedding 的核心职责，不应让匹配失败导致 embedding task 反复重试。

## Risks / Trade-offs

- **自动触发增加 embedding 处理延迟**: 每个事件标签多一次匹配调用（约 10-50ms），可接受 → 如有需要可异步化
- **日报兜底增加生成延迟**: 仅当有未匹配标签时触发，正常情况下为空；设有 50 个 tag 补算上限 → 日志监控兜底触发频率，频率高说明 D1 有问题
- **downgraded 字段历史数据**: 已有记录为 false，不代表未降级 → 用户可手动 backfill 重算以获得准确标记
- **并发安全**: `MatchTopicTag` 是幂等的（replace 语义），多路径同时触发不会冲突。`replaceTopicTagBoardLabels` 使用事务内 `DELETE + INSERT`，PostgreSQL 事务隔离保证最终一致性，自动触发（D1）与日报兜底（D2）同时操作同一 tag 时不会产生脏数据
