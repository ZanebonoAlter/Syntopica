
## 日报生成入口/闸门/窗口/handler 契约

入口：POST /api/daily-reports/generate（daily_report_handler.go:triggerGenerateDailyReport），body {date, board_id | board_ids}，date 缺省今天、按 Local 解析；board_id 与 board_ids 同时给 → 400 conflicting_board_scope。三条前置按序：JSON 类型错 → 400；重建窗口守卫 IsDateOutsideRebuildWindow(date, now, tagging.LoadTagEdgeRetentionDays(db)) → 400 reason=out_of_retention_window；生成闸门 tryAcquireGenerationGate() 失败 → 409 already_running。之后 jobID=uuid，go generateSingleBoard / generateSelectedBoards / generateAllBoards，各自 defer releaseGenerationGate()，单板 ctx 10 分钟。
计数三态：dailyReportCounts{saved, failed, empty}；broadcastDone 发 daily_report_done（含 failed_count/empty_count），notifyDailyReportTerminal(date,total,saved,failed,empty) 走 notification.DailyReportTerminal；成功后有 launchBoardHealthCheckup(scope) 异步体检（30 分钟）。
WS 载荷：buildProgressMessage(jobID,status,boardID,boardName,saved,progress) 目前无 date；buildDoneMessage 无 date。

**引用**：backend-go/internal/topicgraph/handler/daily_report_handler.go:triggerGenerateDailyReport、backend-go/internal/topicgraph/handler/daily_report_handler.go:buildProgressMessage、backend-go/internal/topicgraph/handler/daily_report_handler.go:buildDoneMessage、backend-go/internal/topicgraph/service/generation_gate.go:TryAcquireDailyReportGeneration

<!-- pinned 2026-11-24T15:01:10Z -->

## 调度器补档范式（只补空/旧到新/预算/队列门槛）

job_daily_report.go:backfillMissingReports(ctx, today) 是现成范式：① countUnfinishedTagJobs() 有 pending/leased 就跳过整轮（顺延不丢），failed 只报数不阻断；② retentionDays=tagging.LoadTagEdgeRetentionDays(db)；③ 扫描 [today-retention, today-1]，每天 CollectBoardIDsForDate(day) 取有内容的板 → ReportExistsForBoardDate(board,day) 跳过已有 → generateAndSaveReport(ctx, board, day)；④ 单板失败 warn 继续；⑤ ctx 超时即 break 并把剩余天数写日志；⑥ 返回 (backfilled, data)。整轮 gate 由 job 层 tryAcquireDailyReportGate() 持有（job_daily_report.go:343-349），defer release。
generateAndSaveReport 是包级 seam（var），测试可替换；daily_report.GenerateAndSaveReport 真实实现。调度器补档是 scheduled-only（len(targetDate)==0 才跑）。

**引用**：backend-go/internal/admin/scheduler/job_daily_report.go:backfillMissingReports、backend-go/internal/admin/scheduler/job_daily_report.go:countUnfinishedTagJobs

<!-- pinned 2026-11-24T15:01:10Z -->

## 缺口口径：严格 Step1 谓词 vs 宽松 CollectBoardIDsForDate

严格口径（生成 Step1，daily_report_orchestrator.go:collectBoardTags）：topic_tags.status='active' AND category='event'；JOIN topic_tag_board_labels(semantic_board_id=board)；JOIN article_topic_tags；JOIN articles，pub_date ∈ [当日 00:00, +24h)；NOT COALESCE(direction_mismatch,false)；ExclusiveGap>0 时追加 NOT EXISTS(SELECT 1 FROM topic_tag_board_labels stronger WHERE same tag AND stronger.board<>this.board AND stronger.score >= this.score + gap)。GROUP BY tag 后 Order article_count DESC, id ASC。
宽松口径（repository CollectBoardIDsForDate）：同样的表连接，但**不看 direction_mismatch、不看排他 gap**，只 DISTINCT semantic_board_id；且先检查四张表存在否则返回空（SQLite 测试退化）。
结论：缺口判定必须复制严格口径，否则 generation 会因 Step1 空候选 return nil（report==nil = 当日无内容），补填反复空跑烧 LLM。
filterTagsByQuality 不会把非空集合清空（kept<10 时回捞 weighted），所以严格口径下「有 tag」⇒ 必定出报告（除非 LLM 失败）。残留盲区：无 board label 的 fallback 现场匹配路径（collectBoardTags 中 unmatchedTagIDs 分支）不参与缺口判定。

**引用**：backend-go/internal/topicgraph/service/daily_report_orchestrator.go:collectBoardTags、backend-go/internal/topicgraph/repository/daily_report_repository.go:CollectBoardIDsForDate、backend-go/internal/topicgraph/service/daily_report_orchestrator.go:filterTagsByQuality

<!-- pinned 2026-11-24T15:01:22Z -->

## 补档三处穿越污染 + prev_report_id 顺序约束

穿越点 1（必须修）：daily_report_repository.go:ListTopicRecentBriefs(boardID, sinceDays, perTopicLimit) 用 cutoff := time.Now().AddDate(0,0,-sinceDays) 且 today := NormalizeReportDate(time.Now()) 做同日排除；orchestrator 传 (7,5)。补 5 天前的日报会注入之后发生的 section（违反「补档产物与当天生成等价」约束）。修法：加 reportDate 参数，cutoff/today 都相对 reportDate。
穿越点 2（接受并文档标注）：ListAnchorableTopicsByBoard(boardID, reportDate, cfg) 只对 topic 做窗口/上限筛选，distance 用的是 board_persistent_topics.centroid 当前值 → 历史 tag 用漂移后质心分 L1/L2/L3。
穿越点 3（接受并文档标注）：assignAndUpdateTopics 写 topic_status_at_report 快照取的是话题当前 status，不是历史状态。
顺序约束：findPreviousReportBrief(boardID, date) 用 period_date < date AND status='completed' 取一次 → 必须旧到新补，链才成立。

**引用**：backend-go/internal/topicgraph/repository/daily_report_repository.go:ListTopicRecentBriefs、backend-go/internal/topicgraph/repository/daily_report_topic_repository.go:ListAnchorableTopicsByBoard、backend-go/internal/topicgraph/service/daily_report_orchestrator.go:findPreviousReportBrief

<!-- pinned 2026-11-24T15:01:22Z -->

## 重建破坏性：SaveReport 删 section/thread，人工注记按 id 挂

SaveReport（daily_report_repository.go:SaveReport）同日重跑 = 整份覆盖：找到 existing 后 `Delete(&DailyReportThread{}).Where(report_id)` 与 `Delete(&DailyReportSection{}).Where(report_id)`，再插新 section/thread（section 分 withEmb / withoutEmb 两批，withoutEmb 用 Omit("embedding") 防 pgvector 拒空串）。所以「重新生成」会换掉全部 section/thread 主键。
依赖 section/thread id 的人工事实：report_annotations（ReportID + SectionID + ThreadID? + quoted_text + 字符偏移；代码注释明说「日报 period 生成后不可变（backfill 整份覆盖重建除外）…重建后偏移失配由前端降级为 quoted_text 模糊匹配、再失配显示原文已变更」）、topic_key_event_markers.source_section_id、lane 人工归属/锚定 trail（按 section id 记录）。⇒ 补填必须只补空，绝不在补填路径重建已有报告；重建只由用户显式发起且需二次确认。
唯一约束：board_daily_reports 有 (semantic_board_id, period_date) 唯一键，并发写靠事务级 advisory lock（rebucket 与 SaveReport 共用同一 key）。

**引用**：backend-go/internal/topicgraph/repository/daily_report_repository.go:SaveReport、backend-go/internal/topicgraph/repository/margin_notes_repository.go:ReportAnnotation、backend-go/internal/topicgraph/repository/key_event_markers.go:TopicKeyEventMarker

<!-- pinned 2026-11-24T15:01:36Z -->

## 前端管线状态块结构与数据流

管线状态块：front/app/features/tags/components/console/BoardRadarStatusBlock.vue（839 行，L0 /tags 右栏底部常驻卡）。数据全来自 useRadarPipelineStatus(options)，options.rows = RadarStatusBoard[]（boardId(string|number)/label/reportDate/todayClues），由雷达总览载荷传入；loading/error 也传入。卡片状态机 phase: idle|blocked|picker-open|submitting|done|failed；展开态就地 picker（pickerOpen，板块多选 + 全选/全不选 + selectedCount），生成走 generateDailyReport({date, board_ids})；进度/终态读 WS daily_report_progress / daily_report_done 并按 job_id 归属；终态计数读消息自带 failed_count/empty_count（三态），不反推。
队列门槛：pending+processing>0 或 embedding 未知时 disable（与定时路径同款）。生成成功后 emit 由父层刷新雷达载荷（BoardConsoleShell 侧）。
composable：front/app/features/tags/composables/useRadarPipelineStatus.ts（387 行），只 ensureStarted 既有对账轮询，不新建 setInterval；今日判定用 todayKey()（本地日）。
既有「重新生成」警示：清单内替换警示 + 确认按钮文案「确认替换」（当前无二次弹窗）。confirm 管道：front/app/composables/useConfirm.ts + components/ui/AppConfirmDialog.vue（app.vue 挂载）。

**引用**：front/app/features/tags/components/console/BoardRadarStatusBlock.vue、front/app/features/tags/composables/useRadarPipelineStatus.ts、front/app/composables/useConfirm.ts、front/app/components/ui/AppConfirmDialog.vue

<!-- pinned 2026-11-24T15:01:36Z -->

## 前端补填入口现状与不可达原因

1) 总览卡片（BoardRadarStatusBlock.vue 约 424-443 行）按钮文案 todayReady ? '重新生成今日日报' : '立即生成今日日报'，onclick=openPicker（只开板块勾选，无日期）；data-testid=radar-status-generate。生成请求日期写死今天（todayKey）。
2) 日报阅读台 /tags/:boardId/:date（pages/tags/[boardId]/[date].vue + DailyReportDeskShell.vue）：日期下拉 DailyReportDeskIndex.vue 的选项只来自 reader.reports（已有报告），缺报日期根本进不了列表 → 「补填」不可达。空状态 DailyReportReader.vue:~640 有 drm-desk-empty + desk-empty-generate → generateForDesk() 调 generateDailyReport({date: initialDate, board_id})，但只有手打 URL 才能到达。
3) 今日快读右栏 BoardRadarReadingRail.vue 只查 getTodayReports()（今天），empty 文案「今天还没有日报。」。
4) DailyReportGenerateDialog.vue 有 date input + 板块下拉，但 selectedDate 初值 = new Date().toISOString().slice(0,10)（UTC 截断，本地 0-8 点成昨天）；正确写法见 front/app/utils/todayKey.ts（该文件注释已点名这个 bug）。
5) useDailyReportProgress.ts 的 progress 是 Map<board_id, BoardProgress>，WS 载荷无 date → 多天批量补填会互相覆盖，必须扩 payload 或换键。

**引用**：front/app/features/tags/components/console/BoardRadarStatusBlock.vue、front/app/features/tags/components/daily-report/DailyReportDeskIndex.vue、front/app/features/tags/components/DailyReportReader.vue、front/app/features/tags/components/DailyReportGenerateDialog.vue、front/app/utils/todayKey.ts、front/app/composables/useDailyReportProgress.ts

<!-- pinned 2026-11-24T15:01:47Z -->

## 迁移注册/通知入口/路由顺序/测试地貌

迁移：版本化迁移用 Migration 结构注册在 postgresMigrations()（backend-go/internal/platform/database/postgres_migrations.go），新表模式参考 lane_manual_correction_migration.go / topic_lane_constraint_migration.go；daily_report_* 表由 internal/topicgraph 注册模型，迁移内对可选表要 no-op（tableExists 守卫）。
通知：入口 notification.DailyReportTerminal(date time.Time, counts DailyReportCounts)（backend-go/internal/platform/notification/service.go:57），落 notifications 表 + WS notification 广播；白名单要求「每次运行至多一条终态」。
路由：RegisterDailyReportRoutes(api)（daily_report_handler.go:27）已挂 rebucket/generate/today/backfill-embeddings 等；新端点在该函数内注册，注意 /daily-reports/:id 是通配，静态路径（gaps、backfill-gaps）必须注册在 :id 之前或用不冲突路径。
测试地貌：后端仓库测试放在 repository/*_test.go（sqlite 退化路径常见）；handler 测试用包级 seam 替换 broadcastProgress / generateReportEntry；前端组件测试同目录 *.test.ts，固定命令 pnpm test:unit <file> --maxWorkers=2（参数不带 --）。

**引用**：backend-go/internal/platform/database/postgres_migrations.go:postgresMigrations、backend-go/internal/platform/notification/service.go:DailyReportTerminal、backend-go/internal/topicgraph/handler/daily_report_handler.go:RegisterDailyReportRoutes

<!-- pinned 2026-11-24T15:01:47Z -->
