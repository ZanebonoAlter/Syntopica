# UI 验收建账 — retire-story-adopt-event-metrics

> 档位：minor（见 `ui-design.md`）
> 建账时间：2026-11-20（归档前补账，非实现期原始留痕）
> 证据性质：**本次归档前实跑组件测试取得**，非回溯声明。

## 判定条目

### A-1 S1 矩阵格值 = 当天线索合计 + 热度行序 + 行内归一色深【pass】

- **依据**：`cd front && pnpm test:unit app/features/tags/components/console/observationLanes.test.ts app/features/tags/components/console/BoardLaneEvolution.test.ts --maxWorkers=2` → 2 文件 **38 tests passed**（ObservationLanes 27 + BoardLaneEvolution 11），2026-11-20 实跑。
- 覆盖：格值取线索合计、色深按行内归一且 0/null 不着色、aria 含「N 条线索」、线索全未知显示 `?`。

### A-2 S2 日报同事件聚簇上今日重点（前端定位链路）【pass】

- **依据**：`pnpm test:unit app/features/tags/components/daily-report/DailyReportMasthead.cluster.test.ts --maxWorkers=2` → **8 tests passed**，含「点击定位 DOM 顺序第一条 tag_ids 含 cluster_id 的线程」主链路。

### A-4 S4 调热度权重（候选泳道权重）【pass】

- **依据**：`pnpm test:unit app/features/tags/components/MatchingConfigDialog.laneHeat.test.ts --maxWorkers=2` → **8 tests passed**，覆盖已保存值回显、缺项回退 0.5、越界拒绝不覆盖。

### A-5 布局三视口（1440×900 / 1920×1080 / 390×844 无横溢）【未覆盖】

- **原因**：本 change 的矩阵/雷达样式在归档前已被后续 change（`console-rails-live-signals`、`fix-radar-overview-false-zero`、`matrix-clue-fallback-and-rail-merge`）重做并各自验收；此处重跑三视口无法对应本 change 当时的 DOM，属**场景已被取代**而非遗漏。若后续发现矩阵横溢回归，按现行版块态势页单独立 change。

### A-6 S3 story 停写后读端照旧（UI 读路径）【未覆盖】

- **原因**：S3 的判据是后端写路径硬停写 + 读端可查，属后端验证范围（`daily_report_story_highlights_test.go`、`cmd/backfill-stories/main_test.go`），不在 UI 验收口径内；本报告不重复建账。

## 结论

UI 相关的 3 条可机械断言场景（A-1/A-2/A-4）全部实跑通过；2 条不适用/已取代场景如实标注未覆盖及原因。
