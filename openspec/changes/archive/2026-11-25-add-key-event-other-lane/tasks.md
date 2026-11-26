# Tasks: 关键事件时间视野「其他」兜底泳道

## 1. 纯函数：未归属事件分组

- [x] 1.1 `front/app/features/tags/composables/useBoardKeyEvents.ts` 新增并导出 `groupUnassignedEvents(events, assignedTopicIds)`：返回 `Map<date, BoardKeyEventItem[]>`，收录 `topicId === null` 或 `topicId` 不在 `assignedTopicIds` 的事件，按 `sourcePeriodDate` 分组、保持接口序；不修改既有 `buildKeyEventIndex` 语义（`null` 仍不进话题索引）。

## 2. 视图：伪行与统一投影

- [x] 2.1 `front/app/features/tags/components/console/KeyEventTimelineView.vue` 定义常量 `OTHER_ROW_TOPIC_ID = '__other__'`（仅本组件使用，MUST NOT emit `selectLane`）。
- [x] 2.2 新增 `otherEventsByDate`（`groupUnassignedEvents` 结果按 `axisSet` 过滤）与 `otherProjection`（伪 `ObservationLaneRow`：`label='其他'`、`watchLinked=false`、`sectionTotal=0`、`dormant=false`、`snapshot=null`，`byDate` 取轴内日期）；轴内无事件时不产生行。
- [x] 2.3 新增 `visibleProjections = [...activeProjections, ...renderedDormant, ...otherProjections]`，并替换原本写死 `[...activeProjections.value, ...renderedDormant.value]` 的四处：`pendingGroups`、`rowCardEventMap`、`runLabelLayout` 输入与模板行渲染。
- [x] 2.4 `dayEventsByDate` 改为「`eventsByDate` 全量 × 轴内过滤」（不再按 `rowTopicIds` 过滤）；`windowEventCount`、`projectionByTopicId` 计入全部休眠行与「其他」行；`dormantDayCount` 仍只算休眠行。
- [x] 2.5 模板：在休眠组之后新增「其他」行头（`.kev-head-col` 内，含行名 / `N 事件 · 最近 MM/DD` / 当日条数）与行体（复用正式行结构：生命线、节点、`KeyEventLabel`、日聚焦卡片列），行体加 `is-other` 标记与 `data-testid="key-event-row-other"`；行头不绑定点击。
- [x] 2.6 `emptyState === 'no-lanes'` 时若「其他」行有事件仍照常渲染行与说明行；`eventsLoading` / 首错 / stale 三态沿用既有逻辑，MUST NOT 用空行冒充。

## 3. 测试

- [x] 3.1 `front/app/features/tags/composables/useBoardKeyEvents.test.ts`：新增 `groupUnassignedEvents` 用例——① `topicId=null` 的事件被收录；② 不在 `assignedTopicIds` 的话题事件被收录；③ 已归属事件被排除；④ 同日多事件保持接口序。
- [x] 3.2 `front/app/features/tags/components/console/KeyEventTimelineView.test.ts`：改写「候选话题不占行」相关断言为「不新增候选行 + 候选事件出现在『其他』行」；新增用例——① 有兜底事件时渲染「其他」行且位于休眠组之后；② 休眠组收起时「其他」行仍可见；③ 无兜底事件时不渲染该行；④ 选中日只有「其他」行有事件时视图头读数正确、行头当日条数正确；⑤ 点击「其他」行头不 emit `selectLane`、不切换视图；⑥ `topicId=null` 的历史注记出现在「其他」行且显示话题名快照。
- [x] 3.3 `front/app/features/tags/composables/useBoardKeyEvents.test.ts` 中「未知话题丢弃不进索引」的既有用例保留（`buildKeyEventIndex` 语义未变）。

## 4. 文档

<!-- doc-impact: flow -->

- [x] 4.1 `docs/reference/flow/semantic-board.md` 关键事件视图相关节补一条行为说明：行集合 = 持久化话题行 + 末尾「其他」兜底行（候选/归档/无分区/无归属事件），独立常显不参与休眠折叠，读数含兜底行。
- [x] 4.2 检查 `docs/reference/flow/semantic-board.md` 的「业务约束与不变量」是否需要新增一条红线（未归属 confirmed 事件 MUST NOT 被视图丢弃）；若新增，遵循 `standard/shared/doc-authoring.md` 红线句格式。

## 5. 验证

- [x] 5.1 `cd front && pnpm lint` → 期望：0 error（既有 warning 不计）。**实测**：exit 0，7 个既有 warning（本 change 文件 0 条）。
- [x] 5.2 `cd front && pnpm exec nuxi typecheck` → 期望：`Found 0 errors`。**实测**：exit 0，无 error。
- [x] 5.3 `cd front && pnpm test:unit app/features/tags/composables/useBoardKeyEvents.test.ts app/features/tags/components/console/KeyEventTimelineView.test.ts --maxWorkers=2` → 期望：全绿，新用例全部通过。**实测**：2 files / 134 tests passed。
- [x] 5.4 `cd front && pnpm build` → 期望：构建成功、无新增警告。**实测**：`deploy-frontend.sh --no-restart` 内置构建 exit 0（首次裸 build 曾因机器内存瞬时不足 OOM，加 `--max-old-space-size=5120` 通过）。
- [x] 5.5 实机（`bash scripts/dev/start-dev.sh` + agent-browser 或人工）：版块「开发者工具与开源生态」时间视野切 14 天 → 期望：休眠组之后出现「其他」行，11-18 两条、11-24 一条 candidate 事件可见；视图头读数与实际节点数一致；与右栏「当日其他话题」交叉核对不重不漏。证据记 `ui-verification/report.md`。**实测**：`其他` 行 = `3 事件 · 最近 11/24 / 当日 1 条`，选中日 11/24 视图头 `11/24 · 1 条`；截图 `other-lane-3030-14d.png`。
- [x] 5.6 `bash scripts/harness/doc-impact.sh verify openspec/changes/add-key-event-other-lane` → 期望：声明的 flow 域已更新、对账通过。**实测**：通过（声明: flow，文件 1 个）。
