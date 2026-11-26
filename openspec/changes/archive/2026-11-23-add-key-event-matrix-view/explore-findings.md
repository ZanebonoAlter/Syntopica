
## 关键事件数据链与矩阵页接入点

## 数据源
`topic_key_event_markers`（人工主权层，`state` = suggested/confirmed/dismissed）关键字段：
- `persistent_topic_id`（话题维度，可空）
- `source_period_date`（date，矩阵列直接对齐）
- `title_snapshot` / `summary_snapshot` / `topic_label_snapshot`（确认时冻结快照，不反查实时名）
- `article_ids_snapshot`（JSON 数组，关联文章 ID，确认时冻结）
- `semantic_board_id`（版块维度）

索引 `idx_topic_key_events_topic_state`（semantic_board_id, state, source_period_date）。

## 实测数据（2026-11-21）
- 全库 confirmed 72 条 / 31 个话题，最早 2026-10-10、最晚 2026-11-22（功能上线才累积出 ~44 天）
- 按周 6–15 条；版块 1980 有 28 条，15220 有 5 条（其中 4 条挂在 candidate 话题上）
- 结论：14 天窗口经常整屏无节点，看 30 天才合理；候选话题事件不并入视图

## 现有读端（缺口）
- 只有按单话题：`GET /api/daily-reports/topics/:id/key-events?days=N&state=confirmed`（handler `key_event_handler.go:RegisterKeyEventRoutes`）
- 矩阵有 N 行，逐行拉不可行 → 需新增 board 级批量：`GET /api/semantic-boards/:id/key-events?days=N`，只返回 confirmed
- `lane-dynamics` 的 `candidates[]` 没有 `timeline[]`，所以不能靠它承载事件行

## 矩阵页接入点
- 主舞台：`BoardConsoleStage.vue`（窄屏 `BoardNarrowSummary` / 宽屏 `ObservationMatrix` 二选一）
- 行序唯一实现：`observationLanes.ts:buildObservationRows`（lane_heat 降序 → clueTotal 降序 → topicId 升序；休眠 = 全窗分区 ≤2）
- 视图切换入口：`BoardConsoleHeader.vue` 的 `.console-seg`（现有 `activeView` = matrix / archive）
- 关键事件写动作唯一归属：日报阅读台 `KeyEventMarkerActions.vue`（topic-graph 约束 10）

**引用**：backend-go/internal/topicgraph/repository/key_event_markers.go:TopicKeyEventMarker、backend-go/internal/topicgraph/handler/key_event_handler.go:RegisterKeyEventRoutes、front/app/features/tags/components/console/observationLanes.ts:buildObservationRows、front/app/features/tags/components/console/BoardConsoleHeader.vue

<!-- pinned 2026-11-23T02:27:58Z -->

## 事件→文章→阅读闭环的复用点

## 事件 → 文章 → 阅读闭环（复用既有链路，不新建阅读器）

1. **事件侧数据**：`topic_key_event_markers.article_ids_snapshot: number[] | null`（确认时冻结的文章 ID，前端类型见 `front/app/api/dailyReports.ts:212-213` 的 `tag_ids_snapshot` / `article_ids_snapshot`）。
2. **文章元信息按需读**：`front/app/features/tags/composables/laneDayRelatedArticles.ts`
   - `readRelated(articleId)` / `ensureRelatedArticles(ids)`
   - 缓存本体由父 store 注入（`laneDayReadingCache`，≤20 FIFO、在途合乘、失败不入成功缓存）
   - 晚到隔离：按 board + boardEpoch 双道判定，不发布过期响应
   - 即：展开文章列表时按需读元信息，**不要新建第二份文章缓存**
3. **阅读预览**：`front/app/features/tags/components/console/BoardConsoleShell.vue:534` 已挂载 `ArticlePreviewModal`（`front/app/features/tags/components/ArticlePreviewModal.vue`），矩阵当日工作面与归档列表都走它。
   - 新视图只需 `emit('open-article', articleId)`，由 Shell 接既有弹窗
   - 关闭语义：弹窗关闭后回到原位置（滚动与展开态由调用方保持）
4. **写动作归属**：注记确认/取消只在日报阅读台 `KeyEventMarkerActions.vue`（topic-graph 约束 10），矩阵级视图只读。

## 结论
关键事件视图的文章闭环 = 展开 `article_ids_snapshot` → `laneDayRelatedArticles` 按需读 → `open-article` → `ArticlePreviewModal`。三处都已有实现，本 change 只接线。

**引用**：front/app/features/tags/composables/laneDayRelatedArticles.ts、front/app/features/tags/components/console/BoardConsoleShell.vue、front/app/features/tags/components/ArticlePreviewModal.vue、front/app/api/dailyReports.ts:TopicKeyEventMarker

<!-- pinned 2026-11-23T02:30:12Z -->

## 横向拖拽/列宽复用点与原型定位

## 横向平移与列宽（照抄新建泳道画布，不另写手感）

- `front/app/features/tags/composables/laneComposeLogic.ts:panScroll`（193 行）/ `isDragBeyond`（208 行）：被 `LaneCorrectionCanvas.vue:34-35` 引用。
- 画布实现（`LaneCorrectionCanvas.vue:230-300`）：
  - 空白处 `pointerdown`（`closest('.lcc-card, .lcc-zoom, button, .lcc-fold-check')` 排除）
  - 位移超阈值才 `setPointerCapture` 并进入平移，避免误触点击
  - `scrollLeft = panStart.left - dx`
- 列宽公式（同文件 `columnWidth`）：
  - `MIN_COLUMN_WIDTH = 64`
  - `columnWidth = max(64, (viewportWidth - axisX0 - RIGHT_PAD) / dates.length)`
  - `vbWidth = max(viewportWidth, axisX0 + columnWidth * dates.length + RIGHT_PAD)`
  - 原则：**宁可撑宽画布横向滚动，也不压缩列宽**（与纵向"宁滚动不重叠"一致）

## 关键事件视图落点
- 拖拽：复用 `panScroll` / `isDragBeyond`，阈值 5px
- 列宽：`max(64, 主舞台可用宽 / 可见日数)`；超宽屏均分填满，窄屏守 64 后横向滚动
- 行头 sticky left:0 + 轴首列 sticky，防止滚动后日期压到行名

## 原型定位（用户 2026-11-21）
`ui-prototype/index.html` 只作**功能布局与骨架参考**，不是 1:1 像素基线；实现以 `ui-design.md` 合同与 specs 为准，间距/字号/装饰按产品主题与既有组件规范落地。

**引用**：front/app/features/tags/composables/laneComposeLogic.ts:panScroll、front/app/features/tags/composables/laneComposeLogic.ts:isDragBeyond、front/app/features/tags/components/console/LaneCorrectionCanvas.vue

<!-- pinned 2026-11-23T02:30:23Z -->

## 关键事件视图实机验收发现的 2 个 P1 缺陷根因

实机验收（agent-browser，1440/1920/2560 三档）发现 2 个 P1 缺陷，根因已定位到具体代码，修复时按此改：

1) **同��多个事件标签全部重叠（同一层）**
   - 现象：14 天窗内 row 93 的 6 个标签 computed `top` 恒为 46px，两两重叠 11 对（单对横向重叠 70~247px）；row 971 重叠 3 对；30 天窗 17 对。行高未按层数增高。
   - 根因：`front/app/features/tags/components/console/KeyEventTimelineView.vue` 里 `<KeyEventLabel :ref="el => setLabelRef(event.id, el)">` 对**组件** ref 的回调收到的是组件公开实例（ComponentPublicInstance），不是 DOM 元素；`setLabelRef` 把它存进 `labelEls`，于是 `labelEls.get(item.key)?.offsetWidth` 恒为 `undefined` → `?? 0` → **宽度 0**。
   - 后果：`layoutEventLabels(items, bodyWidth)`（`keyEventLayout.ts`，纯函数本身正确，有单测）收到的 width 全为 0 → 右缘收拢条件 `left + width > containerWidth` 永不成立 → `left = 节点 x + 9`；层占用判定 `left >= right + 4` 恒真 → 所有标签挤进第 0 层。标签同时溢出画布（内层 1088px 但 scrollWidth 到 1342px）。
   - 修复方向：让测量拿到真实元素（`$el` / `defineExpose` 暴露元素 / 用 `getBoundingClientRect` 的 DOM 查询），或直接用 `el instanceof HTMLElement` 判定后取 `$el.offsetWidth`。
   - 为什么单测没抓到：组件测试只 mock 了 `HTMLElement.prototype.offsetWidth`，但值根本没落到 HTMLElement 上；`KeyEventLabel.test.ts` 测的是组件自身（定位由父注入），不覆盖父的测量接线。

2) **行内虚线轴（生命线）不可见**
   - 现象：6 行 `.kev-axis-line` 实测宽 0px、高 1px，三档视口与空态板都看不到生命线（只剩孤立节点）。
   - 根因：`.kev-row-body` 是 `flex: 0 0 auto` 且只有绝对定位子元素（`position: relative` 但宽 0），`.kev-axis-line` 用 `left:0;right:0` 撑不开 → 宽 0。`.kev-cell` 靠显式 `left/width` 定位所以节点正常，这掩盖了 body 宽度为 0 的问题。
   - 修复方向：给 `.kev-row-body` 显式宽度（如 `width: bodyWidth`）或把虚线轴改为 `width: bodyWidth` 的绝对定位层；注意别因此破坏「列宽下限 64px + 舞台内横滚」的布局契约。

另有 2 个 P2：2560 首屏列宽未均分（`containerWidth` 挂载时取值偏小 + ResizeObserver 未回填，重挂载后即恢复 123.2px 均分）；切 30 天时选中日不自动滚入视口（`scrollSelectedDateIntoView` 只 watch `selectedDate`，不 watch 轴/窗口变化，手动点「所选日居中」可补偿）。
证据：`openspec/changes/add-key-event-matrix-view/ui-verification/report.md`（A-8/A-9/A-12/A-23/A-25 为 fail 条目）。

**引用**：front/app/features/tags/components/console/KeyEventTimelineView.vue:setLabelRef、front/app/features/tags/components/console/KeyEventTimelineView.vue:runLabelLayout、front/app/features/tags/components/console/KeyEventLabel.vue、front/app/features/tags/components/console/keyEventLayout.ts:layoutEventLabels、openspec/changes/add-key-event-matrix-view/ui-verification/report.md

<!-- pinned 2026-11-23T06:45:50Z -->
