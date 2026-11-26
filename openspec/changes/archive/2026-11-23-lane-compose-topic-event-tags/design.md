# Design · 编排态节点「已确认关键线索」★

## Context

见 `proposal.md` — Why。技术上只有三个既有事实需要对齐：

1. `topic_key_event_markers` 的 `source_section_id` / `source_thread_id` 在建行时同批写入（`backend-go/internal/topicgraph/repository/key_event_markers.go`），日报重建时由 `ReAttachKeyEventMarkers` 同批重指；FK 为 `ON DELETE SET NULL`。
2. 编排态画布节点 = `ComposeCandidate`（section），节点卡是 `LaneComposeCanvas.vue` 内的 HTML 浮层；左栏是 `LaneComposeCandidates.vue`。
3. `compose-candidates` 的 `sections` 前端原样透传（`useInlineCompose.ts` → `BoardComposeOverlay.vue` → canvas），新字段不需要改中间层。

## Goals / Non-Goals

**Goals**：把人工确认过的关键线索以零计算副作用的方式带到选料现场；后端一次批量查询，前端两个极小落点。

**Non-Goals**：不做事件标签、不做关联环、不改右栏、不改任何写入路径（proposal Non-Goals 已列，不重复）。

## Decisions

### D1. 按 section 单查投影，不 join thread

- **结论**：新增 `ListConfirmedKeyEventsBySections(sectionIDs []uint)`，一条 `WHERE source_section_id IN ? AND state='confirmed'` 批量查并按 section 分组；空输入直接返回空 map。
- **落点**：backend-go/internal/topicgraph/repository/key_event_markers.go
- **取舍**：按 `source_thread_id → thread.section_id` 反查要多一跳 join，且会漏掉「thread 已删、section 指针仍在」的残留标记；两指针建行时同批写入，section 单查已完备。

### D2. 响应字段恒为数组

- **结论**：`ComposeCandidateSection` 追加 `confirmed_key_events`（`[]ComposeConfirmedKeyEvent`），空则 `[]`，不省略、不给 null；前端归一化把缺失字段补成 `[]`。
- **落点**：backend-go/internal/topicgraph/repository/daily_report_manual_topic.go
- **取舍**：省略字段能省体积，但前端要区分 undefined/null/[] 三种「无」，且与既有 `clue_count` 的「null=未知」口径冲突；这里没有「未知」态，只有「没有」。

### D3. 星形节点、卡内全列

- **结论**：带确认注记的节点主体由圆点换成星形（布尔，不带条数）；条数与全部快照标题在卡内逐条列出。
- **落点**：Scenario 标题「卡内明细逐条列出」
- **取舍**：最初设计是"右上角 7px 小角标"，用户 2026-11-21 看完真实界面后改判为星形（深色底 + 9px 点上角标辨识度差、且要额外占地）；`★2` 会与话题总览的 ★n 计数混淆，小点旁的数字在 0.5× 缩放下不可读。

### D4. 左栏同步显示

- **结论**：左栏成员条目尾部显示 ★，复用同一字段，不加开关。
- **落点**：front/app/features/tags/components/compose/LaneComposeCandidates.vue
- **取舍**：只做画布角标更小，但左栏是唯一能读全文的列表，逐条挑时看不到人工信号，等于把 ★ 只留给「扫」不留给「挑」。

### D5. 挂不上不猜

- **结论**：只认 `source_section_id` 精确归属；指针为 NULL 的 marker 不显示，不做标题/日期/话题模糊匹配。
- **落点**：Scenario 标题「来源脱离的注记」
- **取舍**：按标题模糊匹配能「救回」一部分，但会造出假星，且违反日报红线「失配=独立失效快照，不伪造 section/关系」（daily-report 约束 31）。

### D6. 零计算副作用

- **结论**：★ 不进选择/推荐/质心/阈值任何路径；右栏三段骨架零改动（只在②条目日期行首加一个 ★ 字符）。
- **落点**：Scenario 标题「不参与计算」
- **取舍**：把 ★ 当推荐加权更「聪明」，但违反既有红线「推荐只是距离提示、不构成准入或置信度」（daily-report 约束 39）。

### D7. 旧原型作废、minor 复用契约

- **结论**：`ui-impact: minor`、`ui-prototype: none`；旧 `ui-prototype/` 标 superseded，不重做原型，几何基准指向 rework change 已批准的三态图。
- **落点**：openspec/changes/lane-compose-topic-event-tags/ui-design.md
- **取舍**：重画一张带 ★ 的 `02-editing.svg` 更直观，但只加一个角标不属于「改变信息架构/主流程/布局模式」，minor 允许 `none`；重画还会与已批准的画布基准产生第二份几何真源。

### D8. 星形只换形状，不换尺寸语义

- **结论**：星形外接半径沿用既有 `pointRadius`，缩小沿用**同一套通用像素下限（≥3px）**；不设星形专属放大下限（任何放大都会让星形看起来更近）。已选 ✓ 按星形内切圆缩小（约 0.32R，现圆点为 0.48R）。
- **落点**：Scenario 标题「星形不改变大小语义」
- **取舍**：把星形放大到视觉等效面积（×1.15）更好看，但会让「大小 = 接近程度」多出一个形状偏差，违反既有红线；宁可星形显得略小。50% 缩放下的可辨性因此**偏低**，这是已知取舍，以实拍为准（不引入星形专属下限）。

### D9. 隐藏两个工具条入口，保留能力

- **结论**：工具条不再渲染「显示已有泳道（只读）」复选框与「历史待启用话题」按钮；`showAssigned` 恒 `false`（只读参照层不显示）；`LaneComposeHistoryDialog` 组件与 composable 历史名单逻辑保留但不可达。
- **落点**：front/app/features/tags/components/console/BoardComposeOverlay.vue
- **取舍**：连组件与 composable 一起删更干净，但用户定的是"隐藏"不是"删除"，且删除会牵连推荐分组与 10+ 个测试；保留能力可逆，恢复只需加回控件。

## Risks / Trade-offs

- **与 `ThreadRelatedEvents.vue` 的 ★ 撞车** → 卡内文案写「已确认关键线索」而不是裸星；ui-design 已写明两者口径差异。
- **已选节点 ✓ 与星形拥挤** → ✓ 缩到星形内切圆（≈0.32R）；组件测试断言 ✓ 不溢出星形、节点不回退成圆点。
- **星形在极小尺寸下难辨** → 沿用 3px 通用像素下限；星形面积约为同半径圆的一半，50% 缩放下辨识度低是已知取舍（不放大外接半径，避免伪造接近度）。
- **隐藏入口被误读成"功能坏了"** → 部署说明写清「两个控件是主动移除」；组件保留，未来可恢复。
- **拖拽 `preventDefault` 取消了节点原生聚焦** → 点击节点不再由浏览器自动聚焦；键盘 Tab/Enter 路径不受影响。本次接受（节点卡由 `focusId` 驱动，不依赖 DOM focus），在 ui-design 备注里写明。
- **0.5× 缩放下 ★ 消失** → 渲染尺寸下限 6px（与点半径 ≥3px 的缩放补偿同款）。
- **响应体增大** → marker 只在 confirmed 时非空、按 section 聚合，量级很小；不预加 `omit` 开关，等真有体积问题再照 `omit_embeddings` 先例补。
- **重建失联导致「确认过却没有星」** → 设计意图（不冒认），写进 flow 文档与 proposal 部署影响，避免被当 bug。

## Migration Plan

无数据迁移。发布顺序：后端先发（旧前端忽略新字段）→ 前端后发（缺字段归一为 `[]`，不会炸）。回滚：回滚前端即恢复原样；后端多返回一个字段对旧前端无副作用。

## Open Questions

无。
