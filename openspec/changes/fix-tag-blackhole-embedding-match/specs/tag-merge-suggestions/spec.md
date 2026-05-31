## ADDED Requirements

### Requirement: tag_merge_suggestions 表

系统 SHALL 维护 `tag_merge_suggestions` 表，记录可能需要合并的相似标签对。

#### Schema
- `id`: PK, auto-increment
- `new_tag_id`: FK → topic_tags，被提取的新标签
- `existing_tag_id`: FK → topic_tags，相似的已有标签
- `new_label`: text，新标签的 label
- `existing_label`: text，已有标签的 label
- `category`: text，标签类别
- `similarity`: float，embedding 相似度 (0-1)
- `status`: text，枚举 `pending` / `merged` / `dismissed`，默认 `pending`
- `source`: text，来源 `incremental` / `full_scan`
- `created_at`: timestamp
- `updated_at`: timestamp

#### Constraints
- UNIQUE (`new_tag_id`, `existing_tag_id`)
- INDEX (`status`, `similarity` DESC)

### Requirement: 增量记录合并建议

`findOrCreateTag` 创建新 tag 时，如果 `TagMatch` 返回 candidates，SHALL 将候选对写入 `tag_merge_suggestions`。

#### Scenario: 新标签有相似候选
- **WHEN** `findOrCreateTag` 的 `TagMatch` 返回 `candidates`
- **AND** 新 tag 创建成功（获得 new_tag_id）
- **THEN** 对每个 candidate 写入一条 `tag_merge_suggestion`（status=pending, source=incremental）
- **AND** 以 `(new_tag_id, candidate_tag_id)` 为唯一键，已存在则 skip

#### Scenario: 新标签无相似候选
- **WHEN** `TagMatch` 返回 `no_match` 或 `exact`
- **THEN** 不写入 suggestion

### Requirement: 异步全量扫描

用户可手动触发全量扫描，遍历所有 active tag，每个 tag 调用 `FindSimilarTags` 查找相似对，结果写入 `tag_merge_suggestions`。

#### Scenario: 触发全量扫描
- **WHEN** 收到 `POST /merge-preview/scan` 请求
- **THEN** 后端在 goroutine 中启动扫描，立即返回 202
- **AND** 遍历所有 active tag，每个 tag 调用 `FindSimilarTags(tag, category, 10, 'semantic')`
- **AND** 相似度 ≥ LowSimilarity 的结果写入 suggestion（source=full_scan）
- **AND** 已存在的对 skip，不重复写入

#### Scenario: 并发保护
- **WHEN** 已有一个扫描任务在运行
- **AND** 收到新的 `POST /merge-preview/scan` 请求
- **THEN** 返回 409 "scan already in progress"

### Requirement: SSE 进度推送

全量扫描通过 `GET /merge-preview/scan/stream`（SSE）实时推送进度。

#### Message Format
```json
{
  "status": "scanning",
  "total": 590,
  "scanned": 342,
  "current_category": "keyword",
  "new_suggestions": 23
}
```

扫描完成后发送最终消息：
```json
{
  "status": "done",
  "total": 590,
  "new_suggestions": 47
}
```

#### Scenario: 客户端连接 SSE
- **WHEN** 前端创建 `EventSource('/api/topic-tags/merge-preview/scan/stream')`
- **THEN** 收到 Content-Type: text/event-stream 响应
- **AND** 扫描进行中持续收到 progress 事件
- **AND** 扫描完成后收到 done 事件并关闭

### Requirement: 查询合并建议

`GET /merge-preview` 改为查询 `tag_merge_suggestions` 表。

#### Scenario: 查询 pending 建议
- **WHEN** 前端调用 `GET /merge-preview`
- **THEN** 返回 `tag_merge_suggestions` 中 status=pending 的记录
- **AND** 按 similarity DESC 排序
- **AND** 附带 source/new_tag 的文章数量统计

### Requirement: 合并后标记 suggestion

合并操作完成后，SHALL 将相关的 suggestion 标记为 merged。

#### Scenario: 合并后标记
- **WHEN** `POST /merge-with-name` 完成合并（source→target）
- **THEN** 更新 `tag_merge_suggestions` 中 `new_tag_id=source` 或 `existing_tag_id=source` 的 pending 记录为 merged
- **AND** 同时更新涉及 target 的 pending 记录为 merged（因为 target 可能被重命名）
