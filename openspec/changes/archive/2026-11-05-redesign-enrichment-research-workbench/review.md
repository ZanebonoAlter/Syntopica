# Review 意见账（6.1 change-review-glm，2026-10-15）

> 审查范围：制品（proposal/design D1-D8/三 delta spec/ui-design/tasks/test-cases）+ git 工作区 diff（后端 handler/repository/service/migration + 前端全量）+ 只读验证（service/handler go test、前端 53 例）。

### R-1 2026-10-15【High】话题新闻背景脏草稿离开静默丢失（弹层卸载销毁 drafts Map + 切键清空未 stash）
- **状态**：已修复（宿主关闭弹层经 exposed dirty 走 useConfirm 确认；composable 切 topic/gran/period 键前 stashDraft 进 drafts Map）
- 修法：宿主关闭弹层时经 exposed dirty 走 useConfirm 确认；composable 切 topic/gran/period 键前把脏稿 stash 进 drafts Map（保留语义）。

### R-2 2026-10-15【High】「可阅读」view 语义违反 spec（重研中已有报告候选从可阅读 tab 消失；view 未在分页前生效）
- **状态**：已修复（服务端 view 参数：repo SQL EXISTS 分页前过滤 pending/readable + handler 传 live 候选排除；前端 tab 改 emit select-view，删客户端 status 过滤；省略 all 兼容旧客户端）
- 修法：服务端 view 参数（pending=无成功报告且非 live 候选、readable=存在成功报告，SQL EXISTS 分页前过滤；省略=all 兼容旧客户端）；前端 tab 改传 view，删除客户端 status 过滤。

### R-3 2026-10-15【Medium】小量级折线 Y 轴刻度全变 0（fmtAxis 未走阅读短格式）
- **状态**：已修复（fmtAxis 走 formatReadingValue）
- 修法：yLabels 改用 formatReadingValue。

### R-4 2026-10-15【Medium】期间不可解析时折线仍按数组位置等距连线
- **状态**：已修复（期间不可解析分支强制 mode=discrete）
- 修法：buildChartLayout 的不可解析分支强制 mode=discrete。

### R-5 2026-10-15【Medium】新不变量未落「业务约束与不变量」编号红线 38-41（constraint-injection 注入源缺位）
- **状态**：已修复（红线 38-41 已落「业务约束与不变量」节；根因：当时 edit 调用因同批溯源行匹配失败整批未落地）
- 修法：把链路节新增的五条提炼为编号红线 38-41（首行加粗自含红线句）。注：当时该 edit 调用因同批溯源行匹配失败整批未落地，非写入后被覆盖。

### R-6 2026-10-15【Medium】ignoreCandidate/restoreCandidate 续体无 epoch 守卫（切板块后到达会同步写旧板块 activeCandidateId 且 loading 卡住）
- **状态**：已修复（ignore/restore 记录发起时 epoch，响应后 stillCurrent 才续作）
- 修法：发起时记录 epoch，响应后 stillCurrent 才续作（刷新详情/重拉列表/提示）。

### R-7 2026-10-15【Low】409 candidate_ignored 前端文案直出英文后端串（不改轮询语义）
- **状态**：已采纳（随 R-6 一并改：按 data.reason 出中文「该线索已忽略，请先恢复」）

### R-8 2026-10-15【Low】data_freshness.sources JSON 顺序非确定（前端已排序兜底）
- **状态**：误报丢弃（view.sourceLatest 在 signalSourceLatestPeriods 已按工具名排序，序列化天然确定序；前端排序为防御性双保险）

### R-9 2026-10-15【Low】复制值检测仅逐字匹配 trimSignalFloat 串（变体漏检，启发式记录）
- **状态**：已驳回（spec 最低线满足；更强检测需数值归一化管线，过度设计，记录备查）

### R-10 2026-10-15【Low】reportPresentation 测试标题「>3× 中位」与实现阈值（max>10×min）不一致
- **状态**：已修复（随修复批次改标题）

### R-11 2026-10-15【Low】新增 handler 测试 -count>1 种子累积失败（沿用具名共享 SQLite 仓内既有模式）
- **状态**：已驳回（与 TestSignalResearch* 同模式，仓内一致；测试默认 -count=1）

### R-12 2026-10-15【Low】忽略在任意 board job 在跑时 409（比 spec 场景更严）
- **状态**：已驳回（与 D1「同一临界区」决策一致且已在 api/board-signals.md 如实文档化，有意收严）

### R-13 2026-10-15【Low】详情版本区依赖当前周期 reports（切周期后短暂为空，回退 latest_result_id，无数据风险）
- **状态**：已驳回（浏览只读，回退路径正确；记录备查）

### R-14 2026-10-15【Low】reader 对未知块类型防御性静默跳过无局部标记
- **状态**：已驳回（后端已拒该形态；兼容 text 分支仍可读，记录备查）

### R-15 2026-10-15【Low】工作树混入并发 change 产物（semantic_board_matching*/fix-board-ghost 制品/server 二进制）
- **状态**：已采纳（9.1 已盘点：commit 拆分时按 concurrency-status 归属人工剥离；server 二进制不入库）

### R-16 2026-10-16【High】刷新/重进恢复用的 getBoardAnalysisStatus 拼接了从未注册的板块级子路径（恒 404，D6 恢复承诺失效）
- **状态**：已修复（boardSignals.ts 改打根级 `/enrichment/analysis-status?scope=board&id=`；恢复时任务周期快照改用 job 自带 granularity/period；浏览器断言端点 200 + 单测锁参数）

### R-17 2026-10-16【High】handleSelectView 因缺右花括号嵌套进 handleVisibilityChange，模板 @select-view 解析为 undefined 被 prod 静默丢弃——已忽略 tab 从不重拉
- **状态**：已修复（顶层化 + BoardEnrichmentPanel.test.ts 新增接线锚：stub emit select-view 断言 selectCandidateView 调用参数；走查发现于 A9 场景，lint/typecheck/既有单测均未拦截，锚测试为同类回归的机械防线）

### R-18 2026-10-16【Medium】loadCandidates 无请求级参数守卫，同板块连点筛选时慢的旧筛选响应可覆盖新筛选结果
- **状态**：已修复（请求时快照 visibility/view，响应落地前与当前 refs 比对不符即丢弃；epoch 只防跨板块，此守卫补齐同板块乱序）

### R-19 2026-10-16【High】话题总览 SVG 画布 pointerdown 立即 setPointerCapture，click 被重定向到容器——泳道/节点真实点击全被拖拽平移吞掉
- **状态**：已修复（BoardThreadBrowser focus/svg 两处拖拽：捕获延迟到 pointermove 越过阈值；纯点击永不捕获。既有「阈值内不吞/超阈值吞」单测语义保持全绿；浏览器实证：真实指针序列点泳道进聚焦 ✓、拖拽后 click 仍被抑制 ✓）

### R-20 2026-10-16【Low】候选详情相关话题 chip 不可点（只有「背景预览」小按钮可点），用户点击 chip 无反应
- **状态**：已采纳（chip 整体可点开只读背景预览，role=button+tabindex+Enter 可达；内嵌小按钮保留并 stop 防双触发；接线锚测试锁行为）
