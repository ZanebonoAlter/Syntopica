# Tasks: lane-matrix-coverage-and-others-row

> 依赖说明：三个子项互相独立，可并行实现；「其他话题整行」依赖后端 `other_topics` 字段先落地（前端在字段缺失时自动隐藏整行，故可先写前端壳）。
>
> **执行顺序**：第 1 组（用例先行）SHALL 先于第 2~6 组完成（契约 MODIFIED/ADDED，用例先定）。

## 1. 用例与契约（先行）

- [x] 1.1 后端契约测试骨架：在 `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` 增补 `other_topics` 用例（真实 0 值日 / active 排除 / candidate·archived·null 计入 / 综合稿不计 clue / 非 completed 日报排除 / 无锚点空数组）；先跑出红
- [x] 1.2 前端纯函数与组件测试骨架：`coverageLanes.test.ts`（active 过滤/去重/排序/last_seen 缺失）、`ObservationMatrix.test.ts`（覆盖组默认展开/失败重试/其他话题行点击）、`ObservationMatrixGrid.test.ts`（其他话题格 0/—/旧响应隐藏、休眠 summary 名字）、`ObservationMatrixLaneRow.test.ts`（重命名菜单与输入框）、`laneDynamicsWindow.test.ts`（`other_topics` 三态）；先跑出红
- [x] 1.3 锁定口径不变量（写进测试断言）：其他话题格值 = 当日其他话题组线索数；综合稿不计；非 completed 日报不产条目；覆盖组与 A 档行零重复

## 2. 后端 · `other_topics` 逐日聚合

- [x] 2.1 在 `lane_snapshot_repository.go` 新增 `LaneDynamicsOtherDay`（`date` / `sections` / `clues`）并挂到 `LaneDynamicsResponse.OtherTopics`（json `other_topics`，空数组非 null）；验证：`cd backend-go && go build ./...` 通过
- [x] 2.2 实现 `listOtherTopicsWindow`：一次窗口聚合 SQL（`daily_report_sections` ⨝ `board_daily_reports` ⟕ `daily_report_threads`，completed、日期闭区间、`persistent_topic_id IS NULL OR NOT IN (active∪watch topic ids)`，`COUNT(DISTINCT s.id)` + `SUM(CASE WHEN scope<>'composite' THEN 1 ELSE 0 END)`），排除集为空时不加 `NOT IN`；验证：组 1.1 用例除「真实 0 补位」外全绿
- [x] 2.3 补齐「有 completed 日报但 0 个其他分区」的日期骨架（`{sections:0, clues:0}`），保证每个 completed 日报日恰好一条；验证：组 1.1 的 0 值日用例绿
- [x] 2.4 失败策略：聚合失败向上返回错误、端点 500（不静默 0）；无锚点返回 `[]`；验证：新增错误注入用例断言返回 error 而非空切片，无锚点用例断言 `[]`
- [x] 2.5 口径复核：`other_topics` 不影响 `lanes` / `candidates` / `lane_heat`；验证：既有 lane-dynamics 用例全部保持绿（`go test ./internal/topicgraph/repository -run TestLaneDynamics`）

## 3. 前端 · 覆盖组（窗口内未进日报）

- [x] 3.1 新增纯模块 `front/app/features/tags/components/console/coverageLanes.ts`：`buildCoverageLanes(rows, topics)` 过滤 active、去重、按最近日期/名字排序、`last_seen_date` 空串→null；验证：1.2 的 `coverageLanes.test.ts` 绿
- [x] 3.2 `useConsoleShellData` 新增 `boardTopics / topicsLoading / topicsError / loadBoardTopics`（seq 防晚到、切板块清空重置、失败清空 + error）；验证：类型检查通过 + 逻辑由 3.3 组件测试覆盖
- [x] 3.3 `ObservationMatrixGrid` 新增覆盖组（休眠组之后）：默认展开、summary 显示数量+名字+「最近一次」、行复用弱化行样式 + 「未进日报」标、格全「—」；成功且为空整组不渲染；失败显示一行「窗口外泳道清单未取到 · 重试」；验证：1.2 的矩阵测试绿
- [x] 3.4 覆盖行行头 ⋯ 可用（重命名/编辑/归档三个动作上抛），点菜单不改变选中泳道/选中日；验证：`ObservationMatrixLaneRow.test.ts` 断言 `@click.stop` 与事件载荷
- [x] 3.5 覆盖清单在新建泳道保存后、改名后、归档后刷新；验证：壳层接线人工核对 + 组件测试断言 `loadTopics` 回调被调用（可注入 spy）

## 4. 前端 · 行头就地重命名

- [x] 4.1 `ObservationMatrixLaneRow` 菜单新增「重命名」（第一项，仅当 `renameLane` prop 存在）；验证：1.2 的菜单用例绿
- [x] 4.2 行内编辑：名字按钮 → 输入框（自动 focus/select），Enter 调 `renameLane(topicId, draft)`，`true` 退出、`false` 保留并报错；Esc / blur 取消（保存中不取消）；空名提示「泳道名不能为空」；验证：1.2 的行组件用例绿
- [x] 4.3 壳层 `renameLane`：`Number(topicId)` 校验 → `updateTopic(id,{label})` → 成功 toast「泳道名已更新」+ `Promise.all([load(), loadBoardTopics()])`；失败 toast 并返回 false；验证：类型检查 + 组件测试注入 spy 断言调用参数
- [x] 4.4 函数 prop 透传链：`BoardConsoleStage` / `BoardWideWorkface` / `ObservationMatrix` / `ObservationMatrixGrid` / `ObservationMatrixLaneRow`，并加入 `consoleStageTypes` / `consoleStageBinding`；验证：`pnpm exec nuxi typecheck` 通过

## 5. 前端 · 其他话题整行

- [x] 5.1 `front/app/api/laneDynamics.ts` 增加 `LaneDynamicsOtherDay` / `other_topics?`；`normalizers/laneDynamicsWindow.ts` 归一为 `otherTopics: Record<string, {date,sections,clues}>` + `otherTopicsAvailable`（区分旧后端与空数组）；验证：1.2 的 normalizer 用例绿
- [x] 5.2 新增 `ObservationMatrixOtherRow.vue`：行头「其他话题」+「未入泳道」标；逐日格（有数据→clues 可点、title 带分区数；无数据→「—」禁用）；行内 `clueDepth` 归一着色；选中日淡底；点击 emit `selectOthersDay(date)`；无泳道生命周期操作；验证：1.2 的 Grid/Matrix 用例绿
- [x] 5.3 `ObservationMatrixGrid` 在活跃行之后、当日其他话题组之前渲染该行（仅 `otherTopicsAvailable`）；验证：组件测试断言 DOM 顺序
- [x] 5.4 壳层接线 `selectOthersDay`：清其他选择 + 写同一 `selectedDate` + `othersOpen=true`；验证：`ObservationMatrix.test.ts` 断言事件上抛 + `useConsoleReadingWiring` 单测（如可注入）或组件集成用例
- [x] 5.5 旧后端兼容：`other_topics` 缺省 → 整行不渲染；矩阵其余行为不变；验证：1.2 用例绿

## 6. 组件装配与文案

- [x] 6.1 `ObservationMatrix` / `BoardConsoleStage` / `BoardWideWorkface` / `consoleStageTypes` / `consoleStageBinding` 增加 `coverageLanes / coverageLoading / coverageError / retryCoverage / renameLane / otherTopics` 等字段并按层透传；验证：`pnpm exec nuxi typecheck` 通过
- [x] 6.2 休眠折叠条补泳道名（满足既有 spec「折叠条显示数量与泳道名」），名字超长截断但 title/aria 完整；验证：`ObservationMatrixGrid.test.ts` 断言名字文本存在
- [x] 6.3 文案核对：只放动作标签/状态结论，不写机制解释句；「其他话题（未入泳道）」「窗口内未进日报」「最近一次 MM-DD」为固定文案；验证：review 逐条核对 + 组件测试断言文本

## 7. Review

- [x] 7.1 派子线程聚焦 review（制品 + diff），意见按 High/Medium/Low 处置后 `review.md` 建账；验证：`bash scripts/harness/review-trace.sh openspec/changes/lane-matrix-coverage-and-others-row` 不报零痕迹，High 条目均非「待处理」

## 8. 测试

- [x] 8.1 后端影响包：`cd backend-go && go test ./internal/topicgraph/repository ./internal/topicgraph/handler`（按 `change-scope.sh` 判定收敛，不跑全量）
- [x] 8.2 前端组件：`cd front && pnpm test:unit app/features/tags/components/console/ObservationMatrix.test.ts app/features/tags/components/console/ObservationMatrixGrid.test.ts app/features/tags/components/console/ObservationMatrixLaneRow.test.ts app/features/tags/components/console/coverageLanes.test.ts app/api/normalizers/laneDynamicsWindow.test.ts --maxWorkers=2`（参数不带 `--`）
- [x] 8.3 门禁：`cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...`；`cd front && pnpm lint && pnpm exec nuxi typecheck`

## 9. 文档

<!-- doc-impact: flow, api -->

- [x] `docs/reference/flow/semantic-board.md`：矩阵覆盖组/其他话题行的业务约束与入口
- [x] `docs/reference/api/semantic-boards.md`：`other_topics` 字段与场景
- [x] `docs/reference/flow/daily-report.md`：如该文档登记 lane-dynamics 读模型，同步「未入泳道」口径
- [x] 变更溯源：上述 flow 文档按 §12 追加本 change 链接

## 10. 验证

| Scenario | 测试文件 |
| --- | --- |
| 新建泳道窗口外仍可见 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 覆盖行可维护 | `front/app/features/tags/components/console/ObservationMatrixLaneRow.test.ts` |
| 同窗口内泳道不重复 | `front/app/features/tags/components/console/coverageLanes.test.ts` |
| 清单失败诚实降级 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 一眼看出哪天的其他话题多 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| 无日报日不可点 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| 点击跳到当日其他话题 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 旧后端缺字段时整行隐藏 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| 与当日其他话题组同口径 | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` + `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| 行头就地改名 | `front/app/features/tags/components/console/ObservationMatrixLaneRow.test.ts` |
| 取消与空名 | `front/app/features/tags/components/console/ObservationMatrixLaneRow.test.ts` |
| 失败不伪装成功 | `front/app/features/tags/components/console/ObservationMatrixLaneRow.test.ts` |
| 未入泳道逐日读数含真实零 | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` |
| 未入泳道排除口径与矩阵其他组一致 | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` |
| 未完成日报不参与 | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` |
| 折叠条列出名字 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |

- `cd backend-go && go build ./...` → PASS
- `cd backend-go && go test ./internal/topicgraph/repository -run 'TestLaneDynamics|TestOtherTopics'` → PASS
- `cd backend-go && golangci-lint run ./... && go vet ./...` → PASS
- `cd front && pnpm lint` → PASS
- `cd front && pnpm exec nuxi typecheck` → PASS
- `cd front && pnpm test:unit app/features/tags/components/console/ObservationMatrix.test.ts app/features/tags/components/console/ObservationMatrixGrid.test.ts app/features/tags/components/console/ObservationMatrixLaneRow.test.ts app/features/tags/components/console/coverageLanes.test.ts app/api/normalizers/laneDynamicsWindow.test.ts --maxWorkers=2` → PASS
- `bash scripts/harness/doc-impact.sh verify openspec/changes/lane-matrix-coverage-and-others-row` → PASS
- `bash scripts/harness/check-standards.sh --change lane-matrix-coverage-and-others-row` → PASS
- `bash scripts/harness/check-tasks-tail.sh openspec/changes/lane-matrix-coverage-and-others-row` → PASS
