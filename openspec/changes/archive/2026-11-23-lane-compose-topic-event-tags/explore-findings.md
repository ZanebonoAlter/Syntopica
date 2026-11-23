
## ★ 注记投影：调用链与注入点

## 调用链

`GET /api/semantic-boards/:id/persistent-topics/compose-candidates`
→ `composeCandidates`（backend-go/internal/topicgraph/handler/daily_report_handler.go:1231，路由 116）
→ `TopicGraphRepository.GetComposeCandidates`（repository/daily_report_manual_topic.go:264）
→ 前端 `getComposeCandidates`（front/app/api/persistentTopics.ts:322-380）→ `useInlineCompose.ts` 原样透传 → `BoardComposeOverlay.vue` → `LaneComposeCanvas` / `LaneComposeCandidates`。

## 后端注入点（3 处）

1. `repository/key_event_markers.go`（`CountConfirmedThreadMarkers` 657 行之后）新增 `ListConfirmedKeyEventsBySections(sectionIDs []uint)`：一条 `WHERE source_section_id IN ? AND state='confirmed'` + 按 section 分组；空输入返回空 map。同包无循环依赖。
2. `repository/daily_report_manual_topic.go:84-98` DTO `ComposeCandidateSection` 加 `ConfirmedKeyEvents []ComposeConfirmedKeyEvent`（json `confirmed_key_events`，恒数组）；装配点在 `clue_count` 赋值后（约 326-336）。
3. `docs/reference/api/daily-reports.md:900-935` 补字段契约。

handler 不改（1315 行整体透传）。

## 字段形状

```json
"confirmed_key_events": [
  { "marker_id": 11, "thread_id": 1001, "title": "冻结快照标题", "confirmed_at": "2026-11-20T10:00:00Z" }
]
```
- `thread_id` / `confirmed_at` 可空 → JSON 省略
- 无标记 → `[]`，不给 null

## 前端注入点（3 处）

1. `front/app/api/persistentTopics.ts`：`ComposeCandidate` 30-51 加 `confirmedKeyEvents`；新增 `ComposeConfirmedKeyEvent`；`normalizeCandidate` 241-256 手写映射（compose 路径**不走 camelizeKeys**），缺字段→`[]`。
2. `components/compose/LaneComposeCanvas.vue`：节点 `g.lcc-canvas__node` 1405-1469，★ 插在最后一个子元素（保证压过 focus-ring/括号），`pointer-events:none`，缩放渲染下限 6px；卡模板 1551-1575 在日期行后加区块；节点 aria-label 1427 补条数。
3. `components/compose/LaneComposeCandidates.vue` 成员条目（110-126）尾部加 ★。

## 数据口径事实

- marker 建行时 `source_section_id` / `source_thread_id` 同批写入（key_event_markers.go:293-294 / 352-353），重建由 `ReAttachKeyEventMarkers`（718-720）按指纹同批重指；FK `ON DELETE SET NULL`（postgres_migrations.go:4115-4116）。
- 因此**只按 section 查**即完备；thread 已删但 section 指针在的残留标记也能覆盖。
- 两指针都 NULL = 脱离快照（重建指纹失配），**不挂任何节点、不模糊补挂**（daily-report 红线 31：失配=独立失效快照，不伪造 section/关系）。
- 只有 `state='confirmed'` 进 ★；AI 只写 `suggested`（红线 30）。
- 既有撞车：`ThreadRelatedEvents.vue:219` 的 ★ 是「候选线索命中已确认注记」的推荐星，与本界面「该事件下有线索被确认」不是一回事，文案必须写「已确认关键线索」。

## 测试落点

- handler：backend-go/internal/topicgraph/handler/compose_candidates_handler_test.go（既有 setup:30 / payload:167 / call:208 / decode:220）→ 三条 `TestComposeCandidates_ConfirmedKeyEvents*`
- repository：key_event_markers_pg_test.go（风格参照 674 行）+ daily_report_manual_topic_test.go
- 前端：LaneComposeCanvas.test.ts（fixture 工厂 section():21-40、mountRows:393、pointOf:418）／BoardComposeOverlay.test.ts（左栏）／persistentTopics.test.ts

**引用**：backend-go/internal/topicgraph/handler/daily_report_handler.go:1231、backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:264、backend-go/internal/topicgraph/repository/key_event_markers.go:657、front/app/api/persistentTopics.ts:241、front/app/features/tags/components/compose/LaneComposeCanvas.vue:1405、front/app/features/tags/components/compose/LaneComposeCandidates.vue:110

<!-- pinned 2026-11-22T11:16:29Z -->
