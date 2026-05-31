## 1. Embedding 高相似度匹配降级

- [x] 1.1 修改 `TagMatch`（`embedding.go`）：embedding 相似度 ≥ `HighSimilarity` 时返回 `MatchType: "candidates"` 而非 `"exact"`，将匹配结果放入 `Candidates` 列表
- [x] 1.2 编写单元测试：验证 embedding 高相似度返回 candidates，slug/alias 匹配仍返回 exact

## 2. 删除 keyword 阈值覆盖

- [x] 2.1 删除 `CategoryThresholdOverrides` 中的 `"keyword"` 条目（`embedding.go`）
- [x] 2.2 验证 `ThresholdsForCategory("keyword")` 返回 `DefaultThresholds`

## 3. SaveEmbedding 清理旧记录

- [x] 3.1 修改 `SaveEmbedding`（`embedding.go`）：保存前删除同一 `topic_tag_id + embedding_type` 下 `text_hash` 不匹配的旧记录
- [x] 3.2 编写单元测试验证清理行为

## 4. TagsPage 接入标签合并 UI

- [x] 4.1 在 `TagsPage.vue` 中 import `TagMergePreview` 组件
- [x] 4.2 添加 `showMergePreview` 状态，左侧栏操作按钮区添加"标签合并"按钮
- [x] 4.3 在 template 中添加 `<TagMergePreview :visible="showMergePreview" @close="..." />`
- [x] 4.4 合并完成后刷新标签数据

## 5. 合并建议增量记录

- [x] 5.1 新增 `TagMergeSuggestion` 模型（`models/topic_graph.go`），字段：id, new_tag_id, existing_tag_id, new_label, existing_label, category, similarity, status(pending/merged/dismissed), source(incremental/full_scan), created_at, updated_at。UNIQUE(new_tag_id, existing_tag_id), INDEX(status, similarity DESC)
- [x] 5.2 实现 `RecordMergeSuggestions(newTagID uint, candidates []TagCandidate)` 函数（`tagging/tag_merge_suggest.go`）：逐个写入 suggestion，已存在则 skip，source='incremental'
- [x] 5.3 修改 `findOrCreateTag`（`tagger.go`）：candidates 分支在创建新 tag 后调用 `RecordMergeSuggestions`
- [x] 5.4 编写测试：验证 suggestion 写入、去重 skip、source 字段

## 6. 异步全量扫描 + SSE

- [x] 6.1 实现异步全量扫描逻辑（`tagging/tag_merge_suggest.go`）：遍历所有 active tag，每个 tag 调用 `FindSimilarTags`，结果写入 suggestion 表（已存在则 skip，source='full_scan'），通过 channel 推送进度
- [x] 6.2 新增 `POST /merge-preview/scan` handler：触发异步扫描（全局单例，同时只允许一个扫描任务）
- [x] 6.3 新增 `GET /merge-preview/scan/stream` SSE handler：Gin `c.Stream()` + `c.SSEvent()` 推送进度（status, total, scanned, current_category, new_suggestions）
- [x] 6.4 修改 `ScanMergePreviewHandler`（`GET /merge-preview`）：改为查 `tag_merge_suggestions` 表（`WHERE status='pending' ORDER BY similarity DESC`）
- [x] 6.5 编写测试：验证异步扫描写入 suggestion、SSE handler 返回正确 Content-Type、并发触发保护

## 7. 合并后标记 suggestion

- [x] 7.1 修改 `MergeTagsWithCustomNameHandler`：合并完成后，将 `tag_merge_suggestions` 中涉及 source_tag_id 或 target_tag_id 的 pending 记录标记为 `merged`
- [x] 7.2 新增 `DismissSuggestionHandler`（`POST /merge-preview/dismiss`）：将指定 suggestion 标记为 `dismissed`

## 8. 前端适配

- [x] 8.1 修改 `TagMergePreview.vue`：scan 按钮改为调用 `POST /merge-preview/scan`，用 `EventSource` 连接 `GET /merge-preview/scan/stream` 接收进度
- [x] 8.2 添加 SSE 进度展示 UI：进度条 + 当前类别 + 已发现建议数
- [x] 8.3 添加"忽略"按钮：调用 `POST /merge-preview/dismiss`

## 9. 数据修复

- [ ] 9.1 清理 tag 94712 的冗余 embedding 记录（保留最新一对 identity+semantic）
- [ ] 9.2 清理 tag 94712 的 article_topic_tags 关联：移除与"共产党员"无关的文章关联
- [ ] 9.3 排查其他 embedding 数量异常的 tag（>10 条），确认是否存在类似污染
