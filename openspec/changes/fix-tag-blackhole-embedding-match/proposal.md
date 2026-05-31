## Why

`findOrCreateTag` 在 embedding 相似度匹配时错误地覆盖了已有标签的 label/slug，导致标签变成"黑洞"：一个 tag 被反复覆盖为不同标签名，每次覆盖触发 embedding 重生成（text_hash 变化），新 embedding 又把更多不相关的标签匹配到同一个 tag，形成恶性循环。

**实际影响**：tag 94712（"共产党员"）被关联到 71 篇完全不相关的文章（义诊、股市快讯、Codex 讨论、电影票房等），累积 144 条 embedding 记录（正常 tag 仅 2-4 条）。LLM 日志中只有 1 篇文章真正提取了"共产党员"，其余 70 篇都是通过 embedding 匹配错误关联的。

## What Changes

- **降级 embedding 高相似度匹配**：`TagMatch` 中 embedding 相似度达到 `HighSimilarity` 阈值时，不再返回 `MatchType: "exact"`，改为返回 `MatchType: "candidates"`。embedding 只负责"找相似的"，标签合并只发生在 slug/alias 精确匹配时。
- **删除 keyword 类别阈值覆盖**：embedding 高相似度已不再自动合并，`CategoryThresholdOverrides` 中 keyword 条目无意义，删除。统一使用默认阈值。
- **清理膨胀的 embedding 记录**：`SaveEmbedding` 在保存新 embedding 时清理同一 tag 同一 type 的旧记录（text_hash 不匹配的），防止 embedding 无限膨胀。
- **增量记录合并建议**：`findOrCreateTag` 创建新 tag 时，如果 `TagMatch` 产生了 candidates，将候选对写入 `tag_merge_suggestions` 表。用户可通过合并 UI 直接查看，无需全量扫描。
- **异步全量扫描（SSE 推送进度）**：提供手动触发的全量扫描，后台按 tag 遍历复用 `FindSimilarTags`，通过 SSE 实时推送扫描进度。扫描结果增量写入同一张 `tag_merge_suggestions` 表。
- **接入标签合并 UI**：在 TagsPage 左侧栏添加"标签合并"按钮，接入已有的 `TagMergePreview` 组件。该组件改为从 `tag_merge_suggestions` 表读取候选对，并支持触发全量扫描（SSE 进度展示）。
- **合并后标记**：`MergeTagsWithCustomNameHandler` 完成合并后，将相关 suggestion 状态标记为 `merged`。
- **数据修复**：清理被污染的 `article_topic_tags` 关联，移除 tag 94712 与无关文章的错误关联。

## Capabilities

### New Capabilities
- `tag-merge-suggestions`: 合并建议增量记录表，双通道写入（实时增量 + 异步全量扫描），统一查询。SSE 推送全量扫描进度。
- `tag-merge-ui`: 在标签管理页提供标签合并界面，从 suggestion 表读取候选对，支持触发全量扫描并实时展示进度。

### Modified Capabilities
- `tag-embedding-management`: embedding 高相似度匹配降级为 candidates；增加旧 embedding 记录清理，防止同一 tag 同一 type 的记录无限膨胀；删除 keyword 类别阈值覆盖。

## Impact

- **核心代码**：`backend-go/internal/domain/tagging/embedding.go`（`TagMatch`、`SaveEmbedding`）、`backend-go/internal/domain/tagging/tagger.go`（`findOrCreateTag` 调用 `RecordMergeSuggestions`）
- **新增代码**：`backend-go/internal/domain/tagging/tag_merge_suggest.go`（`RecordMergeSuggestions`、异步扫描逻辑、SSE handler）
- **新增模型**：`TagMergeSuggestion`（`backend-go/internal/domain/models/topic_graph.go`）
- **前端代码**：`front/app/features/tags/components/TagsPage.vue`（接入 TagMergePreview）、`TagMergePreview.vue`（改为读 suggestion 表 + 触发全量扫描）
- **API 变更**：`GET /merge-preview` 改为查 `tag_merge_suggestions` 表；新增 `POST /merge-preview/scan`（触发全量扫描）、`GET /merge-preview/scan/stream`（SSE 进度推送）
- **数据库**：新增 `tag_merge_suggestions` 表；需要清理 `topic_tag_embeddings` 表中 tag 94712 的冗余记录；需要审查并清理 `article_topic_tags` 中被污染的关联
- **风险评估**：同义词标签（如"AI"/"人工智能"）会被创建为独立记录而非自动合并，增量记录会自动捕获这些候选对，用户通过合并 UI 手动处理
