
## 后端结算链现状与改造落点

## 后端结算链现状（lane-trend-overview 改造落点）

**触发链**：`service/daily_report_watch.go:76-79` GenerateAndSaveReport 尾部 `spawnLaneSnapshotSettlement(boardID)`（detached goroutine）→ `lane_snapshot.go:189-191` → `settleLaneSnapshotsSafe`（defer recover 全吞 panic，lane_snapshot.go:177-184）→ `settleBoardLaneSnapshots`（lane_snapshot.go:100-145，每泳道 60s 独立 ctx 超时 [laneSnapshotLaneTimeout line 33]、20 泳道 clamp [ListActiveLanesForSnapshot+1 probe, line 111-124]、失败 Warn+continue line 141-144）→ `settleLaneSnapshot`（lane_snapshot.go:147-169）。

**单泳道结算**：`ListLaneSnapshotMaterial(lane.ID, from, anchor, 3)` 取素材（无素材零调用返回 nil）→ `laneSnapshotChatFn(ctx, laneSnapshotSystemPrompt(), userPrompt)` → `truncateRunes(TrimSpace(content), 100)` → 空则 `error("empty llm output")` 按失败跳过 → `UpsertLaneSnapshot(&TopicLaneSnapshot{PersistentTopicID, RollingSummary, AsOfDate: anchor})`。**当前无 detail 字段**。

**D1 改造落点**：① `lane_snapshot.go:43` maxTokens 512→768；② `laneSnapshotSystemPrompt()`（line 61-68）改两版 JSON 契约 `{"summary":"≤100字","detail":"≤500字"}`；③ line 156-163 之后新增 JSON 解析（code fence 剥壳→JSON→字段校验）+ 降级（非 JSON/缺 detail：整段截 100 作短版、detail 空串、Warn 日志、upsert 照常不算失败；缺 summary 维持空输出守卫 return error）；④ upsert 加 RollingDetail。`truncateRunes` 在 `service/watch_materialize_keyword.go:200` 包共享。

**laneSnapshotChatFn**（lane_snapshot.go:41-58）：包级 var（测试直接替换），`Operation: "daily_report.lane_snapshot"`，`Capability: CapabilityDigestPolish`，temperature 0.3。

**D2 落点**：`daily_report_models.go:527-543` TopicLaneSnapshot 在 RollingSummary 后插 `RollingDetail string gorm:"type:text" json:"rolling_detail"`；`lane_snapshot_repository.go:148-158` UpsertLaneSnapshot 的 `DoUpdates: AssignmentColumns` 加 "rolling_detail"。AutoMigrate 注册于 `daily_report_register_models.go:17`（零改动），FK 由版本化迁移 `postgres_migrations.go:3204-3243` 持有（不动，纯加列无 FK）。

**D3 落点**：`lane_snapshot_repository.go:217-222` LaneDynamicsSnapshot{Summary,AsOf} 加 `Detail *string json:"detail,omitempty"`；装配点 line 422-427 `GetBoardLaneDynamics`（snap.RollingDetail 非空串才取址）。handler `lane_dynamics_handler.go` 透传零改动。timeline 折叠：events=titles[:5]，folded=line 442。

**测试环境**：service 包 `lane_snapshot_test.go`（318 行）用 glebarez/sqlite 内存库 + 换全局 repository.Repo 单例（t.Cleanup 还原）+ `laneChatRecorder`/`swapLaneChat`（line 48-71）fake chat fn 注入，直接调包级 `settleBoardLaneSnapshots(ctx, boardID)`。repository 包 `lane_snapshot_repository_test.go` 用 testcontainer PG（`testutil.SetupTestDB(t)`，**禁 SQLite**），helper 在 `lane_test_helpers_test.go`。既有用例名见原文件。

<!-- pinned 2026-09-21T14:22:05Z -->

## 前端现状与改造落点（listContexts 非 fetchContexts）

## 前端现状与改造落点（lane-trend-overview）

**API 层**：
- `api/laneDynamics.ts:16-22` LaneSnapshot{summary, as_of}——加 `detail?: string | null`。请求 `useLaneDynamicsApi().getLaneDynamics(boardId, days?)`（line 84-88），URL `/semantic-boards/${boardId}/lane-dynamics?days=N`。LaneDynamicsResponse{window_days, has_reports, lanes, candidates}；**泳道关联在每条 LaneDynamicsLane.topic_id 上**（无顶层 topic_id map），日报页按 topic_id 从 lanes 数组查。
- `api/boardEnrichment.ts`：**函数名是 `listContexts(topicId, granularity?)`（line 861-870），没有 fetchContexts**——tasks.md 提的 fetchContexts 即它。ContextRow{granularity, period ('2026-W27'/'2026-06'/'2026'/all), content, as_of_date, ...}（line 30-43）。granularity 枚举 week|month|year|all。

**挂载点**：`DailyReportTopicSection.vue` 泳道展开体 `drm-topic__body`（line 201）——当前顶部是今日 section 明细 `drm-topic__sections`（line 202 起），之后才是 `DailyReportMiniLifeline` 节点图（line 299-300，挂载条件原文 `v-if="zone.key === 'active' && group.topicId != null"`，props `:topic-id="group.topicId" :topic-color="group.color"`）。**LaneTrendOverview 插入点 = line 202 之前（body 最顶）**。注意：design 说"置于节点图/当日明细之前"，现状顺序是明细→节点图，趋势区放 body 开头即同时先于两者。
- **首个泳道自动展开在本组件** watch（line 132-138），展开即 emit `ensureLifeline(topicId)`（手动展开 toggleTopic 同样 emit）——这是现成的「泳道展开」信号源，D4 宿主取数可挂同款 emit 或复用该链。
- props：zone/reportDate/lifelineEntries/articleEntries/reportDetails/focusSectionId（Map 均 RequestCacheEntry 风格）。

**宿主**：`BoardDailyReportTimeline.vue`——`useDailyReportReader(toRef(props,'boardId'))`（line 47）持有 detailCache（切日期不重拉）；翻期 shiftReportPeel/selectReportPeel（line 147/155）。**页面级缓存模式**：`RequestCacheEntry<T>{status,data,error}` + `createRequestCache<K,T>` 工厂（`dailyReportMagazine.ts:206-256`，含去重 pending/force 重试）。

**交互范式参照**（复制不共享）：
- 分段按钮：`BoardThreadBrowser.vue:948-958` `.btb-days-btn`/`.active`（v-for options + :class active），样式 line 1676-1701。
- 「还有 N 条」折叠：`LaneDynamicsCard.vue` DAY_EVENT_LIMIT=5 + expandedDays Set + hiddenCount（前端折叠+后端 folded_count），`button.ldc-more` data-testid `lane-day-more-${date}`；后端截断标注 `span.ldc-folded-note`「另有 N 条未载入」。
- 占位条：`.ldc-pending` `background: var(--color-bg-sunken); color: var(--color-text-muted); text-align:center`，文案「态势待结算 · 时间线照常展示」。
- 态势渲染：`p.ldc-stance` + `p.ldc-asof`「汇总截止 M/D」。

**测试模式**：vitest happy-dom + globals + mount（非 shallow）+ `vi.mock('@iconify/vue')` Icon stub + 局部 stubs/vi.mock（`DailyReportTopicSection.test.ts`：子组件换带 slot 透传的 template stub；`BoardDailyReportTimeline.test.ts`：vi.hoisted api mock + vi.mock api 模块 + 子组件 testid stub + attachTo document.body + flushPromises）。vitest.config.ts alias `~ → front/app`、`#imports → test/stubs/nuxt-imports.ts`。

<!-- pinned 2026-09-21T14:22:05Z -->

## NODE_ENV=production 导致前端 vitest 大面积伪红（VTU emitted/stubs 失效）

本机会话 shell 导出了 NODE_ENV=production（env 可见，非仓库代码问题）。影响链：vitest 默认把 node_modules 依赖外部化 → require('vue') 走 index.js 按 NODE_ENV 分支 → 加载 runtime-core.cjs.prod.js；而 vite 内联/编译的 SFC 代码用 vitest 自身的 test 环境 → 同进程内出现 dev/prod 两份 runtime-core 实例。后果：@vue/test-utils 2.4.6 的 emitted() 记录（依赖 dev 构建的 devtools hook：attachEmitListener → Vue.setDevtoolsHook → component:emit）与 global.stubs（同样只对 dev 实例生效）双双静默失效。

症状：wrapper.emitted('x') 恒 undefined；stubs 不生效（真实子组件渲染）；连已提交的既有用例都红（LaneDynamicsCard 2 例、LaneDynamicsPanel 3 例、BoardAnalysisReport 1 例、BoardDailyReportTimeline 2 例在 NODE_ENV=production 下全伪红）。用 `NODE_ENV=test pnpm test:unit …` 全部恢复绿。

对 lane-trend-overview 前端批次的影响与规避（均已落地）：
1) LaneTrendOverview.test.ts 的事件断言不用 wrapper.emitted，改传 onX 监听器 prop（Vue 3 emit 即调用 props.onX，与 devtools 管线无关）——该文件在两种 NODE_ENV 下都绿。
2) DailyReportTopicSection.test.ts 不 stub LaneTrendOverview（stubs 本机不可靠），用 findComponent + 真实 data-testid 断言 FD-13/FD-16。
3) BoardDailyReportTimeline.test.ts 的 HD-2 用本实例工具栏按钮 click 触发翻期而非 document 级 keydown 广播——后者会唤醒 describe A 未卸载的僵尸实例（其 DOM 已分离但 watcher 仍活），且僵尸在 B 的 beforeEach 重 mock 后会拿到 active fixture 触发 +3 次 getLaneDynamics。B 的 describe 自带 resetAllMocks/unmount/afterEach 清理。

主线程门禁跑前端测试时务必 NODE_ENV=test（或 unset NODE_ENV），否则会误判大量既有用例红。建议顺手把此发现登记 test-debt 台账或测试标准文档（standard/backend/testing.md 或前端 testing 文档的「运行环境」节）。

**引用**：front/vitest.setup.ts、front/app/features/tags/composables/useLaneTrendData.ts、front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts

<!-- pinned 2026-09-21T15:34:44Z -->
