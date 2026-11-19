# 设计：归档时间导航块

## 结论先行

1. **主图直接复用矩阵的 `ObservationRangeControls`**（复用它的刷选状态机与几何），只给它加三个可选项（`unitLabel` / `title` / `collapseStorageKey`）和一个 `footer` 插槽，缺省值 = 现状，矩阵与工作面行为零变化。**不重写手势**：`rangeBrush.ts` 的状态机已经在矩阵与工作面两条链路上跑过。
2. 归档把 `bounds` 传成**当前 90 天视窗**，把 `range` 传成**已提交的日期区间**；导航条（新增 `ArchiveHistoryStrip`，挂在 `footer` 插槽里）负责平移视窗。
3. 后端新增一个只读聚合端点，SQL 与归档列表主查询共用同一份成员口径。

## 1. 后端设计

### 1.1 端点

```
GET /api/semantic-boards/:id/article-daily-counts
→ 200 { success: true, data: {
    earliest_date: "2024-03-11" | null,
    latest_date:   "2026-11-18" | null,
    days: [{ date: "2026-11-18", count: 145 }, ...]   // 稀疏、升序
  } }
```

- 无查询参数：口径固定，忽略 `feed_id` / `auxiliary_label_id` / `report_status` / `show_direction_mismatch` / 任何日期窗口（传了也不解析，不返回 400——这是给未来留的余地，不是契约）。
- `:id` 非法或版块不存在：沿用同族 handler 的 `parseUintParam` + 现有 404 口径（与 `/articles` 一致：查标签集合为空即返回空结果，不额外区分）。

### 1.2 SQL（单条聚合，一次扫描）

```sql
SELECT DATE(a.pub_date) AS day, COUNT(DISTINCT a.id) AS count
FROM articles a
JOIN article_topic_tags att ON att.article_id = a.id AND att.topic_tag_id IN (?)
JOIN feeds f ON f.id = a.feed_id
GROUP BY DATE(a.pub_date)
ORDER BY day ASC
```

- **成员口径与 `getBoardArticles` 主查询逐字一致**（`board_match_handler.go:157-161`）：同一 `boardTagIDs`（来自 `topic_tag_board_labels.semantic_board_id`，**不**按标签 `status` 过滤——列表主查询也不过滤）、同 `feeds` 内连接、同 `DATE(pub_date)` 归日。
- `COUNT(DISTINCT a.id)`：一篇文章命中多个版块标签时 `att` 会扇出多行，必须去重（列表用 `SELECT DISTINCT`，等价）。
- `earliest_date` / `latest_date` 由结果首末行直接派生，**不再单独跑 MIN/MAX 查询**（避免二次全扫）。
- `len(boardTagIDs) == 0` → 直接返回 `{earliest_date:null, latest_date:null, days:[]}`，不打库。
- 不复用 `articleDateWindowCond`：该端点没有日期窗口，全历史一次拿；`DATE(pub_date)` 的写法与它保持同一字符串口径（避免两边边界漂移）。
- 不复用 `lane-dynamics` 的 `clue_count`（线索数，非文章数），也不复用 `sourcestats`（按 feed 聚合、窗口白名单）。

### 1.3 性能与索引

- 查询必然扫「本版块全部命中文章」一次；`att(topic_tag_id)` / `articles(pub_date)` / `articles(feed_id)` 既有索引够用。实测口径写在 tasks 验证节（树莓派上一个 200 篇级版块应 < 100ms；万篇级版块允许 < 1s）。
- 前端一次进入归档只拉一次（见 §5 缓存），不随筛选/拖选重拉。
- 若真库上出现慢查询，退路是把结果按 `(board_id, 当天)` 落一张可丢弃汇总表并随打标任务增量刷新——**本 change 不做**（无迁移），只在 Open Questions 记录触发条件。

### 1.4 DTO / 路由

- handler 放在 `board_match_handler.go`（同文件已有列表口径与 `boardTagIDs` 取法，避免跨文件漂移）或同包新文件 `board_article_daily_counts_handler.go`；**优先新文件**，不把 600 行的文件继续撑大。
- 路由注册紧挨 `/articles`：`boards.GET("/:id/article-daily-counts", handler.getBoardArticleDailyCounts)`（`board_crud_handler.go:161` 一带）。
- 响应字段名 snake_case（与同族端点一致），Go 结构体 tag 固定。

## 2. 前端状态模型

| 状态 | 类型 | 语义 |
| --- | --- | --- |
| `counts` | `{ earliest: string\|null, latest: string\|null, byDate: Map<string, number> }` | 端点原样；稀疏 → Map |
| `anchorEnd` | `ref<string>` | 视窗右端（含）；视窗 = `[max(earliest, anchorEnd-89), anchorEnd]` |
| `committedRange` | `ref<{start,end}>` | 已提交区间 = 列表请求范围（`useBoardArchive` 的 start/end） |
| `draftRange` | `ref<... \| null>` | 手势中/导航条拖动中的临时区间（只喂给主图 `:range`，不发请求） |
| `upperBound` | computed | `max(todayInShanghai(), latest)`；视窗右端上限 |
| `viewport` | computed `{start,end}` | 见上；宽度 ≤ 90，贴齐 `earliest` 时变窄 |

不变量：

1. **`committedRange ⊆ viewport`**——导航条平移时区间**同步平移**（保持相对位置），日期表单提交越界区间时先把 `anchorEnd` 挪到 `range.end` 再写区间。这样主图永远不会收到「区间在视窗之外」这种需要 `ObservationRangeControls` 处理的新状态（该组件按 `bounds` 下标高亮，负下标会画歪）。
2. 视窗与区间都**不写 URL**；归档没有日期深链契约。
3. `draftRange` 存在期间 deck/列表仍显示 `committedRange` 派生值（预览不重拉）。

### 2.1 默认值

- `initialDate` 存在 → 基准日 = `initialDate`；否则 = `todayInShanghai()`。
- `committedRange = {start: 基准日, end: 基准日}`，`anchorEnd = 基准日`（视窗自动前推 89 天并贴齐 `earliest`）。
- 视窗右端不超过 `upperBound`；`counts` 未就绪时用 `[today-89, today]` 兜底，曲线为空但日期选择照常可用（列表不能被计数端点阻塞）。

### 2.2 跟随规则（三处入口，全部收敛到同一组函数）

| 用户动作 | `anchorEnd` | `committedRange` | 是否请求列表 |
| --- | --- | --- | --- |
| 主图拖空白 / 拖手柄（手势中） | 不变 | 不变（只写 `draftRange`） | 否 |
| 主图松手 commit | 不变 | = commit 区间 | 是（一次） |
| 拖选区平移（手势中/松手） | 不变 | 同上 | 松手时一次 |
| 导航条拖视窗（拖动中） | 随像素连续更新 | 同步平移同偏移（写 `draftRange`） | 否 |
| 导航条松手 | 最终值 | = `draftRange` | 是（一次） |
| 导航条点击某处 | 该日居中后的合法值 | 保持相对位置同步平移 | 是（一次） |
| 日期表单提交 | `range.end`（clamp 到 `[earliest+?, upperBound]`） | = 表单值 | 是（一次） |

「同步平移」= `shiftRange(range, delta)`（复用 `observationRange.ts` 的 `shiftDate` / `clampRangeToBounds`），确保区间不越界：先 `clampRangeToBounds`，再按 `delta` 平移，再 clamp 一次。
导航条拖动只在 `pointerup`（或键盘步进）时提交一次；`pointercancel` / Esc 恢复拖动前的 `anchorEnd` 与区间，不提交（与主图语义一致）。

## 3. 组件拆分

### 3.1 `ObservationRangeControls.vue`（改，三个可选 prop + 一个插槽）

```ts
/** Y 轴单位（缺省「线索数」，矩阵/工作面不变）。 */
unitLabel?: string        // default '线索数'
/** 头部标题（缺省「时间视窗」）。 */
title?: string            // default '时间视窗'
/** 折叠偏好存储键（缺省 'syntopica.orc.collapsed'）。 */
collapseStorageKey?: string
```

- `unit-label="线索数"` 改为 `:unit-label="unitLabel"`；标题两处（展开态 `<span class="orc-title">`、折叠态摘要）改用 `title`；`COLLAPSE_STORAGE_KEY` 改为 computed 取 prop。
- 无曲线提示由硬编码「365 日池趋势未取到（不影响日期选择，不补 0）。」改为 `{{ title }}趋势未取到（不影响日期选择）。`——归档不出现 365/不补 0 的字样（归档口径缺日已知是 0，语义不同）。
- 新增具名插槽 `footer`（在绘图区与反馈行之后、`</template>` 之前），缺省不渲染任何 DOM；矩阵与工作面不传 → DOM 完全不变。
- `tiers` 的容器加 `v-if="tiers.length"`，避免归档传 `[]` 时留下空的无障碍分组。

**为什么不再多开 props**：`selectableBounds` / `maxSpanDays` / `caption` 已经是既有的向后兼容扩展；归档只差「单位 + 标题 + 存储键 + 底部插槽」这四项，全都有安全缺省。

### 3.2 `ArchiveHistoryStrip.vue`（新，受控纯 UI）

Props：`earliest`、`latest`（= upperBound）、`counts: Map<string, number>`、`viewport: {start,end}`、`disabled?: boolean`。
Emits：`viewport-preview: [{start,end}]`（拖动中，父同步写 draft）、`viewport-commit: [{start,end}]`（松手/键盘步进）、`viewport-cancel`（Esc/pointercancel）。

- 画法：全历史每日值 → 一条面积+折线（`viewBox 1000×28`，`preserveAspectRatio="none"`，与主图同一套 visual token），懒算：`archiveTimeSeries.buildStripSeries(earliest, latest, byDate)`。
- 视窗窗口：一个半透明矩形 + 左右 2px 边，宽度 = 视窗天数 / 全历史天数 × 1000；位置随 `viewport` 变化。
- 手势：`pointerdown` 在窗口内 → 拖动平移；在窗外 → 先跳（让点击日居中）再继续拖；`setPointerCapture`；Esc/pointercancel 取消。
- 键盘：窗口 `tabindex=0`、`role="slider"`、`aria-valuetext="08-21 ~ 11-18"`，←/→ 平移 1 天，Shift+←/→ 平移 7 天，Home/End 到最早/最新。
- 像素↔日期只做线性换算（全历史跨度可能上千天，禁止按天渲染 tick）；两端显示 `MM-DD`（最早 / 最新）标签。
- 规模上限：全历史天数 > 4000 时按桶降采样（每桶取 `max` 与 `sum` 的较大者？**不**——导航条只表达密度，用桶内 `sum`，并在实现注释写明「导航条为密度示意，精确值看主图」）。

### 3.3 `archiveTimeSeries.ts`（新，纯函数 + 测试）

```ts
export interface DailyCountsPayload { earliest_date: string|null, latest_date: string|null, days: { date: string, count: number }[] }
export function toCountsMap(payload): { earliest, latest, byDate: Map<string, number> }
export function viewportFor(anchorEnd: string, earliest: string|null, upperBound: string, windowDays = 90): { start: string, end: string }
export function sliceSeries(viewport, byDate): LaneTrendPoint[]        // 缺日 → {value: 0}（已知 0）
export function shiftViewport(viewport, deltaDays, bounds): { start, end } | null   // 越界返回 null
export function shiftRangeWithViewport(range, deltaDays, bounds): { start, end }
export function buildStripSeries(earliest, latest, byDate, maxBuckets?): { date: string, value: number }[]
```

- 复用 `observationRange.ts` 的 `shiftDate` / `daysBetweenInclusive` / `clampRangeToBounds`，不新写日期算术。
- **与 `laneTrendSeries` 的 `null = 未取到，不补 0` 约定相反**：本模块缺日补 **0**，因为一次请求覆盖整段历史，缺口即已知 0；注释里必须写清这一差异，避免以后被「统一口径」改坏。

### 3.4 `BoardArchiveView.vue`（改）

- 删 `PRESETS` / `applyPreset` / `isPreset` / 两个 `AppInput` 日期项；`asOf` prop 移除。
- 新增 `useArchiveDailyCounts(boardId)` 调用（见 §5）与 §2 的状态；主图放在工具行下方：

```vue
<ObservationRangeControls
  :bounds="viewport"
  :range="draftRange ?? committedRange"
  :source-days="viewportDays"
  :series="viewportSeries"
  :tiers="[]"
  :max-span-days="viewportDays"
  :selectable-bounds="fullBounds"
  :loading="countsLoading"
  :notice="countsError ? '时间曲线加载失败，点重试' : null"
  unit-label="篇"
  title="时间范围"
  collapse-storage-key="syntopica.archive.time.collapsed"
  :caption="['每日进版块文章数（全版块命中）', '篇 · 命中本版块标签的文章数，非线索数；不随右侧筛选变化']"
  @preview="onChartPreview"
  @commit="onChartCommit"
>
  <template #footer>
    <ArchiveHistoryStrip ... />
  </template>
</ObservationRangeControls>
```

> 注意：`ObservationRangeControls` 现有的 `series` prop 已经接受 `LaneTrendPoint[]`（date-keyed），归档直接传 `sliceSeries(viewport, byDate)`，**不需要给组件加数据入口**。

- deck 行、工具行（排序/筛选/⚙/ⓘ）与文章列表结构不变。
- 失败重试按钮放在图表**上方**的 `.ar-time-error` 行（右对齐，复用 `ar-text-btn` 样式）——不塞进组件内部，组件只管曲线与日期表单。

## 4. 请求时序

```
进入归档（v-if 挂载）
  ├─ GET article-daily-counts（一次；缓存命中则不发）
  ├─ 计算默认 committedRange/视窗 → watch(range) 触发 GET /articles
  └─ 曲线就绪后重算视窗（若 earliest 使视窗变窄）
拖选/拖导航条/表单
  └─ commit → committedRange 变更 → watch(range) → GET /articles（一次）
切筛选/排序
  └─ 只影响列表请求；MUST NOT 触发 article-daily-counts
```

`BoardConsoleStage` 用 `v-if="activeView === 'archive'"` 挂载归档（`BoardConsoleStage.vue:103`），切走即卸载，因此**每次进入都会重拉计数**。为免在树莓派上反复全表聚合，`useArchiveDailyCounts` 用模块级 `Map<boardId, {payload, at}>` 缓存 **60 秒** TTL；刷新时先呈现旧值再更新（stale-while-revalidate），错误不覆盖旧值（与雷达「刷新失败保留旧值」纪律一致）。

## 5. 边界与口径

| 边界 | 处理 |
| --- | --- |
| 今天没有任何文章 | 默认区间 = 今天；列表空态；导航条把今天画 0；不漂移 |
| 版块无任何命中文章（`earliest=null`） | `series` 传 null（不画 0 基线）并在反馈行显示「本版块还没有命中文章，暂无时间曲线」；视窗兜底 `[today-89, today]`；列表照常按今天请求 |
| 最新文章日晚于今天（未来日期） | `upperBound = max(today, latest)`，视窗右端可达该日 |
| 历史不足 90 天 | 视窗从 `earliest` 起算，宽度 = 实际天数；导航条窗口贴左 |
| 计数端点失败 | 主图内联错误 + 重试；视窗退化为 `[today-89, today]`；列表不受影响 |
| 区间 > 2000 篇（`ARCHIVE_MAX_PAGES × PER_PAGE`） | deck 的「N 篇」是已加载篇数，曲线是全量真值——沿用既有 `truncated` 提示，不新增文案；本 change 不扩大分页上限 |
| 窄屏 | 整块默认折叠（复用 `useIsNarrowViewport` + 独立存储键），摘要行显示「时间范围 · N 天 · MM-DD~MM-DD」 |
| 方向不符的文章 | 计入曲线（与列表行出现一致），行内标签仍按既有口径隐藏 |

## 6. 兼容与清理

- `BoardArchiveView` 的 `asOf` prop 与 `BoardConsoleStage` 的 `:as-of` 绑定一并移除（`BoardConsoleShell` 的 `latestReportDate` 仍被其 expose/其它子组件使用，不动）。
- `normalizeArchiveRange` 的回退时区由宿主本地 `todayLocal()` 改为 `todayInShanghai()`（页面 `today` 同源），删掉 `todayLocal` 或标注仅测试用。
- `archiveWindowRange`（旧导出）保留但若无调用方则删除——tasks 里用 `grep` 确认。
- 删除 `BoardArchiveView.test.ts` 中 `archive-preset-*` 相关用例，新增默认今天/手势/失败隔离用例。

## 7. 测试策略（per `standard/shared/test-design.md`）

- **后端**：`board_article_handler_test.go` 用既有 fixture 造「同文多标签」「跨两日」「方向不符」三组数据，断言聚合值与 `/articles` 返回的 `pagination.total` 一致；空版块断言 null/空数组。只跑该包（`go test ./internal/tagmanagement/handler`）。
- **前端**：`archiveTimeSeries.test.ts`（纯函数边界：最早=今天、跨年、缺日补 0、越界 null）、`ArchiveHistoryStrip.test.ts`（拖动 emit preview/commit、Esc 取消、键盘步进）、`ObservationRangeControls.test.ts`（缺省 prop 保持矩阵文案与存储键）、`BoardArchiveView.test.ts`（默认今天、`initialDate` 优先、commit 才请求、切筛选不重拉计数、计数失败不阻塞列表）。
- 不新增 e2e；视觉手感在主对话手工点检。

## 8. 备选方案与不选的理由

| 方案 | 为什么不用 |
| --- | --- |
| 把 `bounds` 直接传全历史，不做视窗 | 全历史压进一屏后单日只有 1px，拖选精度崩；且与用户明确要求的「90 天窗口 + 导航条」不符 |
| 新写一套刷选手势 | `rangeBrush` 已覆盖反向拖、平移、双端、键盘、cancel；重写必然长出第二套口径 |
| 给 `ObservationRangeControls` 加视窗/选区分离能力 | 改动矩阵共享组件的核心状态模型，回归面覆盖矩阵 + 工作面；收益只是省一个导航条组件 |
| 前端用 `/articles` 逐页拉取再聚合 | 365 天窗口要 20 页上限打满，且列表有筛选口径，曲线会被筛选污染 |

## 9. Open Questions（不阻塞实现）

1. `counts` 缓存 TTL 60s 是否足够？若用户反馈「刚入库的文章曲线没变」，改为手动刷新按钮或 TTL 15s。
2. 导航条桶数上限（当前按宽度分桶）是否需要固定 240 桶以稳定视觉？先按容器宽度，实测后再定。
3. 后端全历史聚合在万篇级版块的真实耗时——tasks 验证节实测；若 > 1s，再考虑 §1.3 的汇总表退路。
