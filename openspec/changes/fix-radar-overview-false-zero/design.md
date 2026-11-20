# design — 雷达总览「未取到 ≠ 0」+ 滞后沉底 + 端点性能治本

## 事实基线（2026-11-19 实测，非推测）

- 端点 `GET /api/semantic-boards/overview`：13 个活跃版块，连续 13 次调用中 **3 次撞 5 秒预算**（5.00s / 5.23s / 10.47s），其余 0.4–2.5s。撞线那次的响应与用户截图逐字段一致（`report_date=""`、`attention=0`、`today_clues=0`、全行 `data_delayed=true`、顶层 `date` 回退到今天）。
- 用户截图标题是「总览 · 11月19日」= 今天，而真实最新日报日是 11-18 —— 这个「日期跳到今天」正是**全部版块被降级**的指纹（全局 `date` 取各行 `report_date` 最大值，全空时回退今天）。
- 正常路径下页面**并不全 0**：13 板中 11 板有非零关注度（最高 16.5、当日线索合计 66）。即「当天日报没生成」本身没问题（各版块锚点日取自己的最新日报日，spec:68），出问题的是超时降级把「没取到」写成「就是 0」。
- **慢的不是 SQL**：被 GORM 记成 `[SLOW SQL] 7182ms` 的 `board_persistent_topics` 查询，单独 `EXPLAIN (ANALYZE, BUFFERS)` 跑只要 **1.5ms**（Bitmap Index Scan，33 blocks）。GORM 的 Trace 计时**包含连接池等待** → 真实瓶颈是**等连接/等 IO**（当时机器 load average 8.5，后台 AI/embedding 任务在狂写库）。
- 现有索引已够用（`idx_persistent_topics_board_status`、`idx_daily_report_threads_section_id`、`idx_daily_report_sections_report_id`、`idx_daily_report_sections_persistent_topic_id`、`idx_board_daily_reports_board_period`）→ **不加索引**。
- 每版块约 9 条 SQL × 13 版块 ≈ **117 次往返**，全部抢 `max_open_conns=25` / `max_idle_conns=5` 的池子。
- 滞后实例：「刚果疫情新闻」锚点日 `2026-11-07`、全局 `2026-11-18`，关注度 3.0 却占主列表第 10 位（排在关注度 2.5 的版块前）。

## Decisions

### D1：未取到用「可空字段 + 行级 `data_unavailable`」表达，不新增顶层字段
- 落点：`backend-go/internal/topicgraph/repository/board_radar_repository.go:RadarBoardRow`
- 取舍：`attention` / `today_clues` / `last3_clues` / `prev3_clues` / `report_article_count` / `report_event_count` 改成 `*float64` / `*int`（JSON `null` = 未知，正常行输出数字，含真实的 0）+ 新增行级 `data_unavailable`。**不**加顶层 `partial` 字段——前端从行派生「本次数据未取全」即可，少一处真相源。代价：Go 侧字段变指针，既有测试断言要改写法（断言值一个没动）。

### D2：`data_unavailable` 覆盖两条降级路径；真实空态不标
- 落点：`backend-go/internal/topicgraph/repository/board_radar_repository.go:markRadarRowsUnavailable` 与 `board_radar_repository.go:buildRadarRow`
- 取舍：① 端点超时未完成该版块、② 该版块泳道动态计算失败（此前**完全没有任何标记**，是本次补的缺口）→ 都标 `data_unavailable=true` 且计数为 `null`。真实空态（近 14 天无日报无泳道、取数成功）仍是完整零值行（`data_unavailable=false`、计数为 0），保住 spec「无数据版块返回零值」。代价：前端要同时认 `data_delayed`（超时）与 `data_unavailable`（两类），但前端只用后者判呈现，前者保留原义供既有右栏逻辑用。

### D3：全局 `date` 在「全部未取到」时不冒充今天
- 落点：`backend-go/internal/topicgraph/repository/board_radar_repository.go:finalizeRadar`
- 取舍：`date` = 各行 `report_date` 最大值；**全部行未取到且没有任何行有 `report_date` 时保持空串**（不再回退今天）；真没日报（全是完整行）仍回退今天（既有行为与既有测试不变）。代价：全部降级时报头不再显示日期（eyebrow 退化为「总览」），但这是诚实的——此时确实不知道今天是几号。

### D4：spec delta 只 ADDED，不 MODIFIED 既有 requirement
- 落点：`openspec/changes/fix-radar-overview-false-zero/specs/workshop-radar/spec.md`
- 取舍：并发 active change `retire-story-adopt-event-metrics` 正在 MODIFIED 同一个「版块总览聚合端点」requirement。两个 delta 若都 MODIFIED 同一条，归档顺序会让后归档的覆盖先归档的 → 本 change 只**新增**两条独立 requirement（「降级不得呈现为业务结论」「滞后版块不占前排」），零冲突。代价：新增 requirement 与既有 requirement 有少量语义重叠（读者可能觉得啰嗦），换来归档零冲突。

### D5：前端「未取到」三处分别表达，不造 0
- 落点：`front/app/features/tags/composables/useBoardRadarOverview.ts`、`front/app/features/tags/components/console/BoardRadarMasthead.vue`、`front/app/features/tags/components/console/BoardRadarStory.vue`、`front/app/features/tags/components/console/BoardRadarView.vue`
- 取舍：报头标题在全部未取到时改「本次数据未取全，请重试」、部分未取到时改「N 个板块值得关注（M 个版块未取到）」；热点数与当日线索合计在存在未取到版块时显示「—」（未知）；行内改「数据未取到」+「—」；提示条**复用既有 `.rd-error`**（`role="alert"` + 既有「重试」按钮），不新造组件/弹窗/toast。分档只对**已知行**做，未取到的行不占任何名次。代价：`attention` 等字段在前端要一路 null-safe（`numOrNull` + `?? '—'`），改面比「加个标记」大，但换来「忽略标记也渲染不出 0」的强约束。

### D6：滞后版块逐行沉底（用户 2026-11-19 当场决定）
- 落点：`front/app/features/tags/composables/useBoardRadarOverview.ts:laggingRows`、`front/app/features/tags/components/console/BoardRadarView.vue`
- 取舍：滞后 = 该版块自己的锚点日 ≠ 全局锚点日（`anchorDate` 为空**不算**滞后）。滞后版块不进 hot/warm/cold 任何分档、不占前排名次，**逐行**显示在列表末尾并保留「数据至 MM/DD」中性徽标；未取到的版块排在滞后之下（最底）。排序只在前端做（后端仍按 attention 降序返回）→ 零后端改动。代价：主列表行序变成四段（关注 → 常温折叠 → 滞后 → 未取到），需要接号机制保证序号连续（沿用 `BoardRadarCold` 的 `rank-offset`）。备选（折叠成一行摘要 / 并入常温）已被用户否掉。

### D7：端点性能治本——批量板级查询 + ctx 传播 + 连接池，不加索引
- 落点：`backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:batchBoardLevelContext`、`backend-go/internal/topicgraph/repository/board_radar_repository.go:GetBoardRadarOverview`、`backend-go/configs/config.yaml`
- 取舍：把 4 条板级查询（anchor / watch-linked / board topics / 最新日报摘要）从「每版块 1 条」改成「所有版块各 1 条」，往返数 117 → 约 70；雷达路径的查询全部挂 ctx（预算到期即取消在飞的查询，连接立刻归还）；`max_idle_conns` 5→25、`max_open_conns` 25→40。**不加索引**（已实测现有索引够用）、**不改任何统计口径**（批量路径复用同一套聚合代码，只换取数方式）、**默认不改 5 秒预算**（不让页面变慢；仅在实测证明仍频繁撞线时才调大并说明）。代价：板级取数要从 `GetBoardLaneDynamics` 里抽成可复用加载器（公开单版块函数签名与行为不变，其它调用方零改动），是一次中等规模重构；口径回归靠「批量路径与单版块路径产出同一行」的测试钉住。

### D8：并发脏文件只做定点增量
- 落点：`front/app/features/tags/components/console/BoardRadarStory.vue`
- 取舍：该文件（及 `BoardRadarReadingRail.vue` 等）已有其它 active change 的未提交改动（`RowActionMenu` 行尾菜单等）→ 本 change 只做定点增量编辑，**不回退/重排**他人改动；每轮派发都把这条写进 brief。代价：diff 混杂，review 时要按 hunk 粒度看。
