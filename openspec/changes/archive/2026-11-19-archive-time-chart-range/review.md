# Review 建账：archive-time-chart-range

审查对象：proposal / ui-design / specs / design / tasks + 实现 diff。
审查线程：`change-review`（只读），2026-11-19。

### R-1 2026-11-19【Medium】按日聚合对 `pub_date IS NULL` 无防护 → 整个端点 500

- **状态**：已修复
- 证据：`board_article_daily_counts_handler.go` 的 `CAST(DATE(a.pub_date) AS VARCHAR(10))` 对 NULL 产出 NULL 分组，扫进 `Day string` 触发 `converting NULL to string is unsupported` → 500。存量 `articles.pub_date` 可空（回填是 2026-06-11 才加的）。
- 处置：SQL 加 `WHERE a.pub_date IS NOT NULL`（无日期文章本就无法定位到某天）；新增 `TestBoardArticleDailyCounts_NilPubDate`（造一条 NULL pub_date 且命中版块标签的文章 → 断言 200、无空 date、既有日期计数不受影响）。

### R-2 2026-11-19【Medium】「自然日」边界隐式依赖 DB 会话时区，制品未钉死

- **状态**：已修复
- 证据：`DATE(pub_date)` 取会话时区；默认 DSN 钉 `TimeZone=Asia/Shanghai`，但 `DATABASE_DSN` / 容器 `TZ` 可覆盖 → 曲线日与列表分组/「今天」静默偏 8 小时。
- 处置：`docs/reference/flow/semantic-board.md` 红线 36 与 `docs/reference/api/semantic-boards.md` 新端点条目补「归日按数据库会话时区（默认 Asia/Shanghai）；改 DSN/TZ 必须同步修正口径并重测」；同时补 `pub_date IS NULL` 排除说明。不改 SQL（跨 SQLite 测试库的 `AT TIME ZONE` 需要方言分支，收益不抵复杂度）。

### R-3 2026-11-19【Medium】计数迟到导致视窗收窄时，`committedRange ⊆ viewport` 不变量失守

- **状态**：已修复
- 证据：`viewport` 计算里有一段「earliest 晚于 anchorEnd 就抹成 null」的守卫，恰好让「计数到的历史晚于已提交区间」时视窗丢掉左贴；且没有任何 watch 在 counts 到达后重建不变量。后果：`ObservationRangeControls` 拿到负下标把选区画到轴外，后续手势能从轴上取到空日期。
- 处置：① 删掉该守卫，earliest 直接交给 `viewportFor`（其自身已处理两种边界）；② 新增 `watch([countsEarliest, upperBound])`，区间越界时先把 `anchorEnd` 跟到区间末，仍越界再把区间钳进视窗（`clampRangeToBounds`）；③ 新增用例「计数迟到把视窗收窄时，已提交区间被钳回视窗内」，断言 `bounds.start`、`range ⊆ bounds`。

### R-4 2026-11-19【Medium】`useArchiveDailyCounts` 的缓存/竞态纪律零测试

- **状态**：已修复
- 证据：模块级 60s 缓存、`requestSeq` 竞态守卫、stale-while-revalidate、失败不覆盖旧值四条纪律没有断言；`BoardArchiveView.test.ts` 还为绕开缓存「每个用例换 boardId」。
- 处置：新增 `front/app/features/tags/composables/useArchiveDailyCounts.test.ts` 4 条用例（TTL 内二次挂载不再请求 / `reload(true)` 绕过 TTL / 切版块旧响应不覆盖新版块 / 刷新失败保留旧值且暴露 error）。

### R-5 2026-11-19【Medium】父侧「Esc/pointercancel 回滚」与「同值提交不重拉」无测试

- **状态**：已修复
- 处置：`BoardArchiveView.test.ts` 补两条：preview → cancel 后区间复原且 `getBoardArticles` 不增；同值 commit 不重复拉列表。

### R-6 2026-11-19【Low】空版块仍画 0 基线折线与「暂无时间曲线」文案矛盾

- **状态**：已修复
- 处置：`earliest === null` 时 `series` 传 `null`（不画 0 线，走「时间范围趋势未取到」分支），反馈行保留「本版块还没有命中文章，暂无时间曲线」。

### R-7 2026-11-19【Low】`ui-design.md` 与实现的曲线态文案三处不一致

- **状态**：已修复
- 处置：按实现回写 `ui-design.md`——loading 文案为「时间范围加载中…」（组件按 `title` 生成）；empty 为主图不画线 + 反馈行一句 + 导航条只留两端标签；窄屏折叠摘要取**已提交区间**（与 deck 起止一致）。

### R-8 2026-11-19【Low】重试按钮位置与 `design.md` 不符

- **状态**：已修复
- 处置：`design.md` §3.4 改为「图表上方右对齐的 `.ar-time-error` 行」——组件只管曲线与日期表单，错误重试不进组件。

### R-9 2026-11-19【Low】`toCountsMap` 与 composable 内 `toState` 重复实现同一转换

- **状态**：已修复
- 处置：`useArchiveDailyCounts` 改为调用 `archiveTimeSeries.toCountsMap`（单一实现），删除本地 `toState`，`ArchiveCountsState` 保留为 `ArchiveCounts` 的类型别名。

### R-10 2026-11-19【Low】导航条 slider 无障碍属性不完整、disabled 仍可聚焦

- **状态**：已修复
- 处置：补 `aria-valuemin` / `aria-valuemax` / `aria-valuenow`（视窗起点下标）/ `aria-disabled`，disabled 时 `tabindex=-1`。

### R-11 2026-11-19【Low】`archiveInitialDate` 粘滞：第二次进归档不再落今天

- **状态**：已修复
- 证据：`useConsoleShellActions.handleViewArchive` 无日期入参时保留旧带入日期。
- 处置：改为 `date ?? null`——点顶栏「文章归档」普通切换回到日历今天，从历史格子「看归档」仍以带入日期优先；`BoardConsoleShell*` 五个测试文件全绿。

### R-12 2026-11-19【Low】`proposal.md` Impact 段与实现落点不一致

- **状态**：已修复
- 处置：Impact 的后端两行改为实际文件（新 handler 文件 + `board_crud_handler.go` 路由注册）；前端补 `useArchiveDailyCounts.ts` 与 `useBoardArchive.ts`（同源 today / 删 `archiveWindowRange`）。

- 审毕无发现（审查范围：见下）

- 后端成员口径与列表主查询逐字一致（共用 `loadBoardTagIDs`、同 `feeds` 内连接、同 `DATE` 归日、`COUNT(DISTINCT)`）；真库只读实测逐日对账 mismatched=0（2026-11-13 = 298；2026-11-19 = 120 = `/articles` total）。
- 空版块契约（200 + null + 空数组）、稀疏升序、`earliest/latest` 由首末行派生（无二次全扫）。
- 手势主链路（拖动 delta 相对基线、Esc 回滚、表单越界先移视窗、同值不重拉、预览不请求）、请求纪律（计数只随 boardId）、口径隔离（缺日已知 0 vs `laneTrendSeries` 的 null）、复用缺省安全（`ObservationRangeControls` 三 prop + footer 插槽缺省即现状）、文档一致性（API 示例 ↔ handler；红线 36 ↔ 实现）均已核对。
- `simplify-workshop-matrix-copy` 在 `ObservationRangeControls.vue` / 其测试里的矩阵文案精简属另一 change，未计入本 change 判定。
