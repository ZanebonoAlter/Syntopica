# Tasks · 编排态节点「已确认关键线索」★ + 工具条入口隐藏

> 范围以 `proposal.md` 为准：★ 只读投影 + 隐藏两个工具条入口；不做事件标签、不做关联环、不改右栏。
> 影响包（按 `bash scripts/harness/change-scope.sh` 口径）：后端 `internal/topicgraph/{repository,handler}`；前端 `app/api/persistentTopics.ts` + `components/compose/{LaneComposeCanvas,LaneComposeCandidates,LaneComposeInspector}.vue` + `components/console/BoardComposeOverlay.vue`。

## 1. 后端：确认注记投影

- [x] 1.1 `key_event_markers.go` 新增 `ListConfirmedKeyEventsBySections(sectionIDs []uint)`：一条 `WHERE source_section_id IN ? AND state='confirmed'` 批量查、按 section 分组，空输入直接返回空 map；只读、不写任何行。 → `cd backend-go && go test ./internal/topicgraph/repository -run 'TestKeyEvent_ListConfirmedKeyEventsBySections' -count=1`
- [x] 1.2 `daily_report_manual_topic.go` 的 `ComposeCandidateSection` 追加 `ConfirmedKeyEvents`（json `confirmed_key_events`，恒数组不 null），`GetComposeCandidates` 在 `clue_count` 装配后调 1.1 回填；handler 不改（整体透传）。 → `cd backend-go && go test ./internal/topicgraph/handler -run 'TestComposeCandidates_ConfirmedKeyEvents' -count=1`
- [x] 1.3 handler 测试三条硬断言：① 归属正确（多 section 多 marker 不串）；② 无标记 section 是 `[]` 不是 null；③ 混入 `suggested`/`dismissed` 不进 ★。 → 同上命令全绿

## 2. 前端：类型与展示

- [x] 2.1 `persistentTopics.ts` 新增 `ComposeConfirmedKeyEvent` 与 `ComposeCandidate.confirmedKeyEvents`，在 `normalizeCandidate` 归一：缺字段→`[]`、`thread_id`/`confirmed_at` 为 null 时省略；字符串 id 口径与既有 `id` 一致。 → `cd front && pnpm test:unit app/api/persistentTopics.test.ts --maxWorkers=2`
- [x] 2.2 `LaneComposeCanvas.vue` 星形节点：`confirmedKeyEvents` 非空时把 `<circle class="lcc-canvas__point">` 换成五角星 `<path>`（外接半径同 `pointRadius`、空心/实心两态、`data-role="key-star"`），无标记仍画圆点；已选时 `checkPath` 的 ✓ 缩到星形内切圆（≈0.32R）且不溢出；节点 aria-label 追加「已确认关键线索 N 条」。 → `cd front && pnpm test:unit app/features/tags/components/compose/LaneComposeCanvas.test.ts --maxWorkers=2`
- [x] 2.3 `LaneComposeCanvas.vue` 节点卡：日期行之后追加「★ 已确认关键线索」区块，单条显示快照标题、多条逐条列出；空则整块不渲染。 → 同上，断言卡文本含全部标题
- [x] 2.4 `LaneComposeCandidates.vue` 左栏成员条目：**日期行右端**显示 ★（同一字段，无开关）。 → `cd front && pnpm test:unit app/features/tags/components/console/BoardComposeOverlay.test.ts --maxWorkers=2`
- [x] 2.5 `LaneComposeInspector.vue` 右栏②「可补充的事件」条目：**日期行首**显示 ★，不改「＋ 加入」与接近度。 → `cd front && pnpm test:unit app/features/tags/components/compose/LaneComposeInspector.test.ts --maxWorkers=2`

- [x] 2.6 拖拽手感（2026-11-23 用户反馈）：拖拽分支 `pointerdown` 调 `preventDefault()`（不选文本）；`.is-dragging` 加 `user-select:none`；`dragDelta` 让被拖节点/整组**本体跟随光标**（`draggedTransform`），`resetGesture` 清空。 → `pnpm test:unit LaneComposeCanvas --maxWorkers=2` 含「本体跟随 + defaultPrevented + 松手清空」断言（89 例通过）

## 3. 工具条入口隐藏

- [x] 3.1 `BoardComposeOverlay.vue` 工具条移除「显示已有泳道（只读）」复选框与「历史待启用话题」按钮；删除 `onToggleAssigned` / `openHistory` / `onHistoryAdopt` / `onHistoryActivate` / `historyOpen` 与 `LaneComposeHistoryDialog` 的 import + 渲染；`showAssigned` 保持恒 `false`（只读参照层不显示）。 → `pnpm test:unit BoardComposeOverlay --maxWorkers=2` 且工具条 DOM 中 `lane-compose-show-assigned` / `lane-compose-history-open` 均不存在
- [x] 3.2 把原「入口存在」的用例改成隐藏契约断言（两个入口不存在、历史弹窗不打开、只读参照层不渲染），不删用例覆盖。 → 同上

## 4. 测试

- [x] 4.1 后端仓库层：`TestKeyEvent_ListConfirmedKeyEventsBySections`（多 section 批量、按 state 过滤、NULL section 指针不误挂）。 → `cd backend-go && go test ./internal/topicgraph/repository -run 'ComposeCandidates|KeyEvent' -count=1` 期望 ok
- [x] 4.2 后端 handler：三条 `TestComposeCandidates_ConfirmedKeyEvents*`。 → `cd backend-go && go test ./internal/topicgraph/handler -run 'TestComposeCandidates' -count=1` 期望 ok
- [x] 4.3 前端组件：`LaneComposeCanvas.test.ts`（星形替换圆点、外接尺寸与同半径圆点一致、已选实心星 + ✓ 不溢出、卡内逐条）+ `BoardComposeOverlay.test.ts`（左栏日期行 ★、工具条两入口不存在）+ `LaneComposeInspector.test.ts`（右栏②日期行 ★）+ `persistentTopics.test.ts`（归一）。 → `cd front && pnpm test:unit app/api/persistentTopics.test.ts app/features/tags/components/compose/LaneComposeCanvas.test.ts app/features/tags/components/compose/LaneComposeInspector.test.ts app/features/tags/components/console/BoardComposeOverlay.test.ts --maxWorkers=2` 期望全绿
- [x] 4.4 测试欠账巡检：`bash scripts/harness/test-patrol.sh --report`，本 change 影响文件不得有红项；域外红用 `--register` 记账。 → 实测 exit 0；台账历史欠账在 be-dataenrichment / fe-* 分片，本 change 影响包（be-topicgraph / fe-tags）无红项

## 5. 文档

<!-- doc-impact: flow, api -->

- [x] 5.1 `docs/reference/flow/daily-report.md`：业务约束与不变量追加一条——关键注记在编排态是只读投影，只认 `source_section_id` 精确归属，指针失配不显示、不做模糊补挂（引用既有约束 30/31）。 → `grep -n '只读投影' docs/reference/flow/daily-report.md` 命中
- [x] 5.2 `docs/reference/flow/daily-report.md` 前端入口清单：移除「显示已有泳道（只读）」与 `LaneComposeHistoryDialog.vue` 入口描述，注明入口已隐藏、能力保留。 → `grep -n 'LaneComposeHistoryDialog' docs/reference/flow/daily-report.md` 仅剩历史存档说明或无命中
- [x] 5.3 `docs/reference/flow/semantic-board.md`：泳道工作面一节补「编排态节点 ★ 口径」与代码入口（`LaneComposeCanvas.vue` / `LaneComposeCandidates.vue` / `LaneComposeInspector.vue`）。 → `grep -n '已确认关键线索' docs/reference/flow/semantic-board.md` 命中
- [x] 5.4 `docs/reference/api/daily-reports.md`：compose-candidates 响应示例与字段说明补 `confirmed_key_events`（恒数组、thread_id/confirmed_at 可空）。 → `grep -n 'confirmed_key_events' docs/reference/api/daily-reports.md` 命中
- [x] 5.5 归档前对账：`bash scripts/harness/doc-impact.sh verify openspec/changes/lane-compose-topic-event-tags && bash scripts/harness/check-standards.sh --change lane-compose-topic-event-tags`。 → 实测两者 exit 0（`archive-readiness.sh` ① ② 绿）

## 6. 验证

- [x] 6.1 `cd backend-go && golangci-lint run ./internal/topicgraph/...` → 期望 exit 0
- [x] 6.2 `cd backend-go && go test ./internal/topicgraph/repository ./internal/topicgraph/handler -run 'ComposeCandidates|KeyEvent' -count=1` → 期望 ok
- [x] 6.3 `cd front && pnpm lint` → 期望 exit 0（本 change 触碰文件 0 warning）
- [x] 6.4 `cd front && pnpm exec nuxi typecheck` → 期望 exit 0、`grep -c "error TS"` = 0
- [x] 6.5 `cd front && pnpm test:unit app/api/persistentTopics.test.ts app/features/tags/components/compose/LaneComposeCanvas.test.ts app/features/tags/components/compose/LaneComposeInspector.test.ts app/features/tags/components/console/BoardComposeOverlay.test.ts --maxWorkers=2` → 期望全绿
- [x] 6.6 `cd front && NODE_OPTIONS=--max-old-space-size=3072 pnpm build` → 期望 exit 0
- [x] 6.7 `openspec validate lane-compose-topic-event-tags` → 期望 `is valid`
- [x] 6.8 视觉：实拍带 ★ 的编辑中态 + 工具条（两个入口已消失），写入 `ui-verification/report.md`（minor 至少 1 条判定条目）。 → 实测 A-1/A-2/A-3/A-5 pass、A-4/A-6 未覆盖并写明原因（report.md 六条判定）；headless 视口固定 1280×633，星形细节以 DOM 断言 + 节点卡文本 + 原型对账为据
