# Review：add-key-event-other-lane

审查方式：change-review 只读子线程（制品 ↔ 实现 diff 一致性）+ 主线程逐条处置。
审查时点：2026-11-25。

### R-1 2026-11-25【Medium】首错/stale 判据仍用「按话题索引」，仅含无归属事件的版块会在刷新失败时丢掉「其他」行内容

- **状态**：已修复
- 处置：`eventsFirstError` 改为 `events.value.length === 0`（不再依赖 `eventIndex.size`），补用例「只有无归属事件时刷新失败：旧内容与「其他」行保留」。
- 证据：`front/app/features/tags/components/console/KeyEventTimelineView.vue`（`eventsFirstError`）、`KeyEventTimelineView.test.ts`（该用例绿）。

### R-2 2026-11-25【Low】悬停轴头日期强调对「无归属」事件失效（只反查按话题索引）

- **状态**：已修复
- 处置：`hoveredDate` 改用全量 `eventsByDate` 反查，`null` 归属事件同样点亮轴头。

### R-3 2026-11-25【Low】`is-other` 是死钩子：兜底行与正式话题行视觉上无差别

- **状态**：已修复
- 处置：补 `.kev-head-cell.is-other .kev-row-name { color: var(--color-text-muted); }`，与 `is-dormant` 同一弱化口径（只弱化行头文字，不动节点/标签/卡片对比度）。

### R-4 2026-11-25【High】两个 in-progress change 对同一 Requirement 的 MODIFIED 文本互斥且互不包含

- **状态**：已修复
- 处置：① 本 change delta 的 MODIFIED 已纳回 `key-event-day-focus-cards` 的两段正文（日期选择器 / 窗外取消聚焦）与两个 Scenario，形成合并文本；② 同步把 `key-event-day-focus-cards` delta 的旧口径句（「候选事件 MUST NOT 在本视图呈现」）与对应 Scenario 改为「不单独占行、事件收进『其他』兜底行」，消除后归档覆盖风险；③ `design.md` 风险条目订正为「已合并」并写明建议归档顺序（先 day-focus-cards，再本 change）。
- 证据：两个 change 的 `specs/workshop-board-console/spec.md`；`openspec validate add-key-event-other-lane` → valid。

### R-5 2026-11-25【Medium】ui-design 的 `no-lanes` 场景未落成 spec Scenario

- **状态**：已修复
- 处置：ADDED「关键事件「其他」兜底行」补 Scenario「无持久化话题但有兜底事件」；对应组件用例「无持久化话题但有兜底事件：说明行与「其他」行并存」。

### R-6 2026-11-25【Medium】「其他」行有 4 个行为点未被测试钉住

- **状态**：已修复
- 处置：补 4 条用例——① `expandedDormant: true` 时断言「其他」行排在已渲染休眠行之后；② `no-lanes` + 兜底事件并存；③ 仅 `topicId=null` 事件 + error + stale 保留旧内容；④ 同日 2 条兜底事件 → 2 节点 + 2 标签。
- 证据：`KeyEventTimelineView.test.ts`（2 files / 134 tests 全绿）。

### R-7 2026-11-25【Low】「轴外日期的事件不进『其他』行 / 不计入读数」无用例

- **状态**：已修复
- 处置：补用例「轴外日期的事件不进兜底行、不计入读数」。

### R-8 2026-11-25【High】flow 文档正文仍写旧契约，与红线 #37 及实现自相矛盾

- **状态**：已修复
- 处置：改写 `docs/reference/flow/semantic-board.md` 关键事件视图节的「行集合与行序」为「持久化话题行 + 末尾『其他』兜底行」新口径，并修正「右轨联动」段的「候选话题不占行」表述；红线 #37 与变更溯源行已在实现阶段同步（`doc-impact.sh verify` 通过）。

### R-9 2026-11-25【Low】与 `key-event-day-focus-cards` 共用同一源文件，提交/回滚边界需显式拆分

- **状态**：已采纳
- 处置：两 change 共用 `KeyEventTimelineView.vue` / 其测试文件；本 change 的净增量集中在「其他」行相关片段与 `useBoardKeyEvents.ts` 的纯函数。提交时按 change 拆分（或提交信息显式记录共用文件的回滚边界）；归档顺序建议见 R-4。

### R-10 2026-11-25【Low】实现旁注释仍描述旧口径（「无归属事件丢弃」）

- **状态**：已修复
- 处置：`useBoardKeyEvents.ts` 文件头与 `normalizers/boardKeyEvents.ts` 的 `topicId` 字段注释改为「不进话题索引，由视图『其他』兜底行承接」。

## 新增（主线程自检，非子线程发现）

### R-11 2026-11-25【High】跨版块 stale 事件会被「其他」行全量收进来

- **状态**：已修复
- 说明：切版块后事件层 stale 保留旧版块数据，正式行靠 `rowTopicIds` 天然隔离，但「其他」行按「不在行集合」收容会把旧版块的未归属事件也收进来。
- 处置：新增 `eventsBoardMatches`（`contentSource.boardId === props.boardId`）门槛，`otherEventsByDate` 与 `dayEventsByDate` 在来源版块不匹配时返回空；补用例「跨版块 stale 数据不进兜底行」。

## 结论

- 子线程提出的 2 个 High（R-4 / R-8）与 Medium（R-1 / R-5 / R-6）均已处置；Low 全部处置或采纳。
- 归档前置：① 先归档 `key-event-day-focus-cards`（stage: implement），再归档本 change；② 归档门禁需全绿（当前 `golangci-lint` / `go vet` 的红来自工作树外部未归属文件 `daily_report_backfill_migration.go`、`auxlabel/*` 与 `cmd/aux-label-dedup-parity`，非本 change 改动面，零后端改动）。
