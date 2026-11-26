
## 投诉复审：雷达与控制台日期只来自active泳道，候选日报会被漏掉

2026-10-25 只读GET实测：/api/semantic-boards/overview 中 board50413「影视作品」与246030「卫生健康」均有 report_date=2026-10-24，但anchor_date为空、today_sections/attention=0；对应GET lane-dynamics?days=14 has_reports=true、lanes=[]。最新日报GET /daily-reports/1237含1个candidate分区/2条线索，/1238含1个candidate分区/1条线索，非无数据。代码：board_radar_repository.go buildRadarRow只遍历dynamics.Lanes求数量（180-209），不含candidate/unassigned；useBoardConsole.ts:250-257最新日期也只遍历lanes；BoardConsoleShell.vue:457依赖latestDataDate才给日报入口；BoardArchiveView.vue:26-32 asOf为空回今天，可能把这类板块已有历史日报/档案隐藏在错误默认日期后。风险是「组织状态尚未active」被误表成「没有新闻/日报」，违背雷达先注意力路由再读日报的主故事。GET原始证据暂存在.pi/reviews/radar-desk-2026-10-25/data/，最终报告需链接可留存证据。另board3122 report_date/anchor_date=10-24而global date=10-25、today_sections=1，需UI标明按各版块数据日比较，不是同一今天。未执行任何写操作。

**引用**：backend-go/internal/topicgraph/repository/board_radar_repository.go:buildRadarRow、front/app/features/tags/composables/useBoardConsole.ts:latestDataDate、front/app/features/tags/components/console/BoardConsoleShell.vue、front/app/features/tags/components/console/BoardArchiveView.vue

<!-- pinned 2026-10-25T14:09:18Z -->

## UI迁移复审：正文与删除版块漏迁，话题管理不是全站删除

2026-10-25最终独立UI-review结论落review.md（3 High、7 Medium均待处理）；实页及GET证据在ui-verification/complaint-review-2026-10-25/report.md、data/。关键代码事实：旧TagsPage已无路由挂载，其BoardListSidebar @delete→useBoardCRUD.handleDeleteBoard→sbApi.deleteBoard未迁入新BoardRail/BoardSettingsForm。旧文章链TagsPage→ArticlePreviewModal→ArticleContentView有站内正文；新BoardArchiveView:250-329只有规则/分数/日报状态/原文外链，DeskArticleDrawer:30-32优先AI摘要且去HTML，不能代替旧正文阅读。不要误判话题管理全站删除：DailyReportReader @open-detective→TopicDetectiveWall.client.vue:710-729/849→components/dialog/TopicManageDialog.vue仍有rename/archive/restore/delete写动作，只是日常入口深埋，需在治理/泳道菜单直达。旧归档来源/辅助标签/质量/方向开关和完整MatchDetailPanel未等价迁移；新仅方向徽标不能查看默认被后端隐藏的direction_mismatch，useBoardArchive请求没传开关。SectionDrawer viewArchive不带date，控制台只切view导致归档回最新日；useBoardArchive.sortArchiveArticles同日按id二次排序覆盖API sort=time，真实50413的10-21顺序从23:20…变成09:02→20:19→23:20。修复优先恢复能力，不要求用户重生成数据。10-25 ui-design头注明确记载两栏/删右栏/日期平铺获批，不能把这些主动取舍直接判误删；不改合同为现实现背书。自己的单浏览器已close，03-50413-1440.png实际1280×633、05/11过渡截图不计稳定缺陷。

**引用**：front/app/features/tags/components/console/BoardArchiveView.vue、front/app/features/tags/components/daily-report/DeskArticleDrawer.vue、front/app/features/tags/components/TopicDetectiveWall.client.vue、front/app/components/dialog/TopicManageDialog.vue、front/app/features/tags/composables/useBoardArchive.ts、openspec/changes/rework-workshop-into-radar-and-desk/review.md

<!-- pinned 2026-10-25T14:39:48Z -->

## 计算明细丢失宿主布局：实页落到归档列表底部

2026-10-26 只读实页核验 localhost:5100/tags/1974：1440×900，切归档→第一篇文章→计算明细后，.match-detail-panel rect={x:271.5,y:937.625,w:882,h:448.9375}，position=static，parent=.archive，不在文章 AppDrawer 中，视口内不可见；elementFromPoint 采样命中抽屉 FOOTER 而非面板。BoardArchiveView.vue:388 附近将 MatchDetailPanel 放在 AppDrawer 之后，归档根 .archive 是纵向 flex+overflow:auto。MatchDetailPanel.vue:347 起只有内联 aside 样式，无 Teleport/fixed/宿主定位。旧 HEAD 的 BoardTimelinePanel.vue:200/267 用 tags-match-detail-panel 类补 width:320px、sticky 和滚动约束，新宿主未承接。原 ui-verification/report.md A-43 宣称 pass，product-match-detail-1440.png 实际只展示文章证据抽屉，不能证明计算面板可见。新证据位于 ui-verification/norm-audit-2026-10-26-match-detail-after-click-1440.png。修复必须明确承载（抽屉内详情切换或受控次级浮层），不能只断言组件存在。

**引用**：front/app/features/tags/components/console/BoardArchiveView.vue、front/app/features/tags/components/MatchDetailPanel.vue、front/app/features/tags/components/BoardTimelinePanel.vue、openspec/changes/rework-workshop-into-radar-and-desk/ui-verification/report.md

<!-- pinned 2026-10-26T01:45:39Z -->

## 旧 DAG 与泳道关系语义不可用矩阵和混合 top6 等价替换

旧 HEAD BoardThreadBrowser.vue:38-40/195-197 将 timeline=匈牙利相似度 DAG（过滤 identity）与 lanes=话题泳道（仅 identity）分开；旧 SVG 1534-1541 区分同话题延续与相似度并展示距离。后端 daily_report_matching.go 仍有 hungarianAssignment→phase1Hungarian，RebuildBoardRelations→writeIdentityEdges 写同话题 identity 边；GetBoardSectionTimeline 返回 relation_type，没有因为换壳移除边数据。新 BoardComposeOverlay.vue:canvasRows/sectionRowKey 把 active 放各行，其余压到 pending 一行；所有 section 都是小圆点。focusId=null、showAllRelations=false，drawnLines 默认空；relatedItems 将 identity/similarity 混合按距离对同一对端点去重再 slice(0,6)，全量最多240线。2026-10-26实页1974：71节点，初始0线；点第一个已归属节点只出现1条identity实线。当前能画线≠保留旧非泳道关系浏览。discussion.md:92 写了关系图降级为编排选料辅助；ui-design退役节也写DAG降级，但没有证明矩阵连续性可表达跨泳道相似/二分配对，也没独立对账待确认关联故事。后续须分别盘点 similarity/identity/待确认可发现性和用途，不能直接声称算法删除，也不能只把全部关联默认打开当完成。

**引用**：front/app/features/tags/components/BoardThreadBrowser.vue、front/app/features/tags/components/console/BoardComposeOverlay.vue、backend-go/internal/topicgraph/repository/daily_report_matching.go、backend-go/internal/topicgraph/repository/daily_report_repository.go:GetBoardSectionTimeline、openspec/changes/rework-workshop-into-radar-and-desk/discussion.md

<!-- pinned 2026-10-26T01:46:00Z -->

## 全局治理定位冲突与标签描述数据契约缺失

ui-design.md Information Architecture 明确匹配参数/合并/回填/标签池为系统/全库动作、放版块栏会误导，却把新家设成当前版块「更多」。BoardGovernanceRail.vue:147-177 又在版块治理复制新建/扩充/回填/规则/合并/标签池；BoardConsoleShell.vue:537-545 的更多和 BoardRail 均再有新建版块；root验收 A-35 将重复入口列作pass、A-36只验弹窗打开。BoardArchiveView 映射 compositionLabels 时只保留 id/label（BoardConsoleShell.vue:640）；前端 semanticBoards.ts 的 BoardArticleTag、DirectHitAuxiliary、MatchDetailPair、CompositeHitComponent 均无description，MatchDetailPanel 的 compositeChains 仅将成员label拼成公式字符串，缺事件完整辅助/组合成员描述的可读展示。日报切换控件 DailyReportDeskIndex.vue:48-76 是原生select+局部样式，版块font-size .7rem/日期.72rem。实页2560×1440（4K150% CSS等效，不是实物4K缩放）：根22.016px，两个控件约323.64×37.06/34.31，字号15.85/15.41px；已有根字号宽屏缩放，不能称完全没响应式，但原验收A-46只有1440功能切换，不能代表4K产品适配通过。

**引用**：openspec/changes/rework-workshop-into-radar-and-desk/ui-design.md、front/app/features/tags/components/console/BoardGovernanceRail.vue、front/app/features/tags/components/console/BoardConsoleShell.vue、front/app/api/semanticBoards.ts、front/app/features/tags/components/MatchDetailPanel.vue、front/app/features/tags/components/daily-report/DailyReportDeskIndex.vue

<!-- pinned 2026-10-26T01:46:18Z -->

## FB-4：矩阵现有分区计数与线索计数分离，用户选同日主题桶max

代码事实：useBoardConsole.ts:buildLaneRow 的 counts[date]=day.sections.length；clueCounts[date]=各section的 thread_count 之和，空缺时回退 events.length+folded_count。BoardMatrixView.vue:cellCount 仍读 counts，所以矩阵显示的是分区/桶数量，不是线索。buildDayAxis 每日一列，当前没有多日期合列。counts 还被点击可达性/lastActive/休眠分组使用，clueTotal用于排序；改变显示数字时不要把线索0等同于已有分区不存在，也不要未经要求重写排序或生命周期。用户在FB-4后的结构化确认：归档文章主点击站内读原文；同泳道同日主题桶3/7条线索应显示7（max），不是sum10；移除右侧重复线索总数。只需展示聚合，不回填/重生成日报。新原型需要一次看全匹配解释、标签默认全展示及hover/focus说明，关系交互和关键事件留待单独讨论，审批仍pending。

**引用**：front/app/features/tags/composables/useBoardConsole.ts:buildLaneRow、front/app/features/tags/composables/useBoardConsole.ts:buildLaneRows、front/app/features/tags/components/console/BoardMatrixView.vue:cellCount、openspec/changes/rework-workshop-into-radar-and-desk/human-feedback.md:FB-4

<!-- pinned 2026-10-26T12:28:17Z -->

## FB-5回查：报刊指挥台基准是原P1/P2，演进与watch不能因关系另议被删

沿harness pin.write#69576（2026-10-23，session01a16942）读原会话用户可见消息，找到discussion.md。§2明确采用console（固定视口/分区栅格/状态常驻/点击钻取），否决data wall/发光仪表盘；用户原话为个人4K观看便利，日常看日报、闲时钻文章，希望知道最近哪个板块火热/今天值得看。原用户a50ad689选P1首屏+P2板块详情；演进以topic贯穿雷达→矩阵→演进带→日报→原文。现BoardRadarView已有attention分档、3日环比、迷你泳道活跃矩阵、今日该读/升温/延迟标记；BoardLaneEvolution已有日期节点、密集日标签、今日事件、密集三日、态势折叠与snapshot待结算；FB4把前者压薄、后者变成一句summary+两个按钮，是本次被退回的具体缺口。WatchManagePanel与泳道演进是不同能力，二者都不在‘关系图另议所以可删’范围。当前stage仅总览+矩阵/追踪，报刊风×战略控制大屏，归档方向冻结，日报后做；新原型须保留上述能力并改善层级/密度，而非回卡片堆或造新KPI。父线程已只读捕获当前/tags和/tags/1974的2560CSS截图以及12板块四窗口(7/14/30/365)lane-dynamics原样响应，均在新prototype/reference。对应窗成员可能不同，不能从365响应自行裁14成员；原样JSON数组日期倒序，锚点须求max。日格3/7→7仍确认，排序/休眠/后端统计不因此改写，其他显示口径需明确不混淆。

**引用**：openspec/changes/rework-workshop-into-radar-and-desk/discussion.md:2-4、docs/research/narrative-workshop-screen/explore-findings.md、front/app/features/tags/components/console/BoardRadarView.vue、front/app/features/tags/components/console/BoardLaneEvolution.vue、front/app/features/tags/components/console/BoardConsoleShell.vue、openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-26-overview-matrix/reference/README.md

<!-- pinned 2026-10-26T14:38:47Z -->

## 矩阵自由视窗与产品取数能力边界

本轮矩阵原型已交单HTML，审批仍pending，未改产品。当前产品lane-dynamics仅接受days=7/14/30/365固定档，无任意start/end；原型自由日期以365日真实样本池做显示切片，并明确标“所选成员/统计未取数”，不能据此把365响应当作自选范围的成员、排序、休眠或统计结果。正式实现前须先补任意起止API及这些口径的设计/spec/用例，不可直接复制原型前端截取冒充后端支持。来源source-breakdown使用独立滚动窗口，样本仅有14份，缺少逐日来源明细，因此原型不画来源趋势；缺档标未取数而非0。历史视窗的lane摘要是最新结算态势，不是历史总结；桌面和手机现共用laneNarrative，历史/样本窗默认折叠并标最新结算日与“非本视窗总结”，日条目才按自选日期呈现。旧批准总览原型hash0fd1e55b95c4dd63707a108e4ab9904bfcf84ce3b2dbefc74b8ff76adc05863b保持不变。

**引用**：openspec/changes/rework-workshop-into-radar-and-desk/ui-design-matrix-draft.md、openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-27-matrix-free-range/prototype.js:laneNarrative、front/app/features/tags/composables/useBoardConsole.ts

<!-- pinned 2026-10-27T03:14:54Z -->

## 右轨健康评分与两类候选必须分开

BoardHealthPanel/BoardHealthSignal的健康数据由useBoardHealth读取GET /semantic-boards/:id/health-checkup，score=latest.llm_score，绝不是radar attention；展示分档>=70 good、>=60 mid、<60 low，缺score保持null。趋势按真实trend.checked_at/llm_score，少于2点不画线，不把多次体检点称多天。父2026-10-27只读GET实测1974 latest.score82、checked_at2026-10-26T21:39:33+08、ghost_matches9、vector_missing0、trend7点；1980 score92、ghost2、trend7点。latest字段为id/semantic_board_id/checked_at/metrics/uncaptured_candidates/llm_score/llm_comment/created_at。uncaptured_candidates是尚未纳入板块构成的辅助标签推荐，例如1974美国31tags/28篇、特朗普22tags/23篇；1980GitHub23tags/35篇、GPT-6 19tags/26篇。capture走addComposition，不是激活PersistentTopic。必须和话题candidate（连续命中资格+人工激活）、topic-watches（active/paused关注）区分，不能因设计右轨自动变status。健康/候选取最新快照需标体检日期，不随历史日期视窗冒充历史评分；可用llm_comment及真实metrics表达建议，不调用生成或回填来填原型。

**引用**：front/app/features/tags/composables/useBoardHealth.ts:useBoardHealth、front/app/features/tags/components/console/BoardHealthSignal.vue、front/app/features/tags/components/console/BoardGovernanceRail.vue、front/app/api/topicWatches.ts:useTopicWatchesApi

<!-- pinned 2026-10-27T03:58:43Z -->

## FB9 原型追踪管理错带泳道与无真实集合的文章入口

当前待审title-first原型prototype.js:evolution在选中泳道标题下额外放管理追踪(data-lane)和intent articles。openWatch(laneId)的管理参数为{boardId:state.board,topicId:laneId||state.lane||null}，所以即使删除泳道按钮，右轨管理仍会默认带当前泳道；修订必须同时移除事件/参数层的隐式lane过滤，板块追踪管理只用boardId，具体关注项的persistentTopicId只供定位泳道。intentMap.articles把“泳道的全部文章”描述成“既有文章归档（泳道筛选）”，但实际仅openWorkspace承接说明，没有真实泳道文章集合接线；已核对BoardArchiveView实际props为boardId/asOf/initialDate/labels，无lane筛选props。不能把这个原型臆加入口说成核实过的旧能力，撤原型入口不等于删除产品功能；板块文章归档与真实thread.related_article_ids必须保留。现演进节点大小和最密集三日排序均用day.total（日线索合计），MatrixGraphics.laneTrend也用日合计；日格仍按同日各桶max，不混称。brush趋势是poolLanes逐日total求和，不是全库板块热度。

**引用**：openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-27-matrix-title-first/prototype.js:evolution/intent/openWatch、openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-27-matrix-title-first/matrix-graphics.js:brush/laneTrend、front/app/features/tags/components/console/BoardArchiveView.vue:props

<!-- pinned 2026-10-27T06:44:13Z -->

## 矩阵实施前的真实读取边界与五条截断

front/app/api/laneDynamics.ts和repository/lane_snapshot_repository.go现读接口按days回溯至板块MAX(period_date)，不是任意endDate；滚动summary只有topic_lane_snapshots单行最新值，不能充当历史日摘要。GetBoardLaneDynamics主集合为当前active与active watch关联非归档话题，按窗内不同归属日期数DESC、最近命中DESC、ID排序，零命中行不返回；candidate提示当前人工激活可见集合，recent_hint可不在所选窗口。LaneTimelineSection只返回section_id/label/thread_count/events(前5标题)/folded_count，无thread_id或summary。产品标题→摘要必须复用真实日报详情里的thread稳定ID，不把原型同section唯一exact标题映射搬为产品恒久关联方案。选中某日后可按section/report读完整线索列表解决前5截断，而不是把未返回标题说成不存在。自由日期有两条合法路线需用户定：①真实365数据池+独立7/14/30/365查询档名单，观察窗仅裁切，保持排序与成员稳定；②新增真实from/to读取，成员/排序/休眠随选窗按原规则重算（只展示分组，不改status）。不能把“缺start/end接口”说成所有自由观察窗均无法用真实数据实现，也不能在未裁决时擅自换算法/历史成员规则。

**引用**：front/app/api/laneDynamics.ts:LaneTimelineSection/LaneDynamicsLane、backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:GetBoardLaneDynamics、backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:GetBoardReportAnchor/listLaneSectionThreads、front/app/api/dailyReports.ts:DailyReportThread

<!-- pinned 2026-10-27T07:33:32Z -->

## 报告列表 lookback 与文章来源字段边界（G4）

报告列表端点与 lane 池锚点不同源（G4 已核实并实现）：ListReports(boardID, days) 以 NormalizeReportDate(time.Now())（服务器今日）回溯 days-1 天含端点、days<=0 默认 7、无上限（backend-go/internal/topicgraph/repository/daily_report_repository.go:427-440）；lane-dynamics 365 池按板块 MAX(period_date)（anchor）回溯。停更板块 anchor 早于今天时死写 days=365 会漏池首报告。G4 解法：api/laneDayReading.ts reportIndexDaysFor(requiredStart, today) = max(365, daysBetweenInclusive(requiredStart, today))，requiredStart=anchor-364 与目标日取更早者；索引仅当前板块 1 份、days 需求扩大后整体替换、已有更大覆盖不重复取。G5 若需绝对最新日报元信息（含 0 泳道版块），不得拿本组索引或 active 行最近日期反推「板块无报告」。另：GET /articles/:id 响应无来源名字段（仅 feed_id，ToDict 已核），相关文章元信息只展示标题/时间——若产品要来源名是共享 API 缺口，需报父另议。

<!-- pinned 2026-10-27T14:58:59Z -->

## 所选日泳道外内容：真实日报快照与候选-only日期选择盲区

2026-10-28仅原型只读冻结、未改产品：新原型目录revision-2026-10-28-day-context-rail的data-audit.json/README可追溯31份1974板块真实报告。10/23报告#1211有7分区/11线索，排除正式泳道（含休眠已可读行）3分区后余4个报告时candidate；只作当日阅读，UI中文“当时待观察”，不表示当前可激活。10/26报告#1249的3分区全部已在正式泳道，真实空态，不应硬补候选。10/02真实有4未归属分区但没有正式泳道日格，仅靠点泳道格/节点无法选该日；实施前必须讨论板块级日期列头或完整日报索引入口，不能把disabled格伪造为有内容。6/14存在topicId非空且状态快照null（历史身份未记录），6/20有真实无topicId的未归属，两者不能混称candidate。7/15存在历史active但本档未列入，需独立标注。日期须和主阅读同一所选日，不可右区锁旧日主阅读跳新日；旧完整阅读快照未含10/23，原型通过已冻结#1211的真实thread.summary只读adapter补齐，未取正文明确未导出样本。当前仅待审离线原型；审批/实施不默认通过。

**引用**：openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-28-day-context-rail/data-audit.json、openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-28-day-context-rail/day-context-model.js、openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-28-day-context-rail/validation.md

<!-- pinned 2026-10-28T04:20:44Z -->

## 泳道外报日单入口原型v2：补上候选-only日期与无分区日导航

前一条日级读取发现的10/02选择盲区，现已在待审离线原型revision-2026-10-28-day-context-picker验证方案，尚未产品实现/获批。右侧日期标题唯一新增入口（手机复用），用真实128条报告索引选日；窗外确认后保持14日长度，以目标10/02截止移到09/19—10/02，A成员/排序/休眠不变，365只补同ID历史。10/02报告#980的4个真实未归属分区合9线索，主当前泳道无分区但右区可读，切泳道保所选日。无分区日不能用有内容数组indexOf(selectedDate)=-1判前后皆无：须从当前窗、当前泳道真实有分区日期按日期两侧找最近日。lane1844在10/02上一为10/01，点击后三处日期同步且窗/档不变；09/20为窗内最早内容日，下一09/21。原型31份详情/128索引，未导出显示未取到；selectedDate未单独写URL不是正式产品持久化约定。当前HTML1804449B SHA d118718e136433f9acea3eb15260d776c6da3aa34ac8aafb35160ad301a7d41a，旧两版不变。

**引用**：openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-28-day-context-picker/README.md、openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-28-day-context-picker/validation.md、openspec/changes/rework-workshop-into-radar-and-desk/ui-design.md

<!-- pinned 2026-10-28T06:43:47Z -->

## 卫生健康冷启动断链：有12份日报但selectedDate依赖正式泳道

2026-10-28独立只读GET与代码定位（细证据explore-cold-start-observation.md）：246030卫生健康有12份completed日报(10/07–24)、section-timeline16分区/12日/当前可见9个candidate ID；lane-dynamics 7/14/30/365均has_reports=true且lanes=[]，topics管理列表只返回达门槛候选1781（不等DB仅1候选）。1781关联8日内容杂，不能推荐直接激活来填泳道。报告1238/1228/1208等详情含完整thread摘要，API不缺。真正gate：useConsoleSelection.selectedLaneRow=null→laneConsoleView.reconcileSelectedDate的if(!lane)return null→BoardConsoleStage LaneChronicle v-if selectedLaneRow不挂→useLaneDayReading索引/详情都不加载；ObservationMatrix no-lanes不画日期轴。旧reportAnchor只通header/meta/pool，未通板块日阅读。应以板块日报索引驱动独立selectedDate，无正式泳道时主区读全日分区；时间入口收敛，不能再加右日历/靠激活候选绕过。section-timeline persistent_topic为当前属性、section.status不是话题快照，历史状态取报告topic_status_at_report；仅6/12份详情被本轮诊断抽样，余未推断。现有lifeline/relations可读历史记录，但无候选成长率/历史hit曲线；隐藏候选管理资格不因只读报告绕过。仅设计/只读，不是产品实现或审批。

**引用**：front/app/features/tags/components/console/useConsoleSelection.ts、front/app/features/tags/components/console/laneConsoleView.ts、front/app/features/tags/components/console/BoardConsoleStage.vue、front/app/features/tags/composables/useLaneDayReading.ts、openspec/changes/rework-workshop-into-radar-and-desk/explore-cold-start-observation.md

<!-- pinned 2026-10-28T07:37:53Z -->

## 冷启动原型：报日独立主轴、素材编排缺口与四种时间口径

新待审原型revision-2026-10-28-cold-start-observation已以246030真实12期日报/0正式泳道演示报日→原始分区→跨日对照→内存所选素材→人工建题承接，中东1974成熟对照；不是产品实现或批准。重要实施点：当前BoardComposeOverlay/useInlineCompose按compose-candidates(boardId,days=14)拉池，锚最新完成日报、需section向量、days上限90，Overlay无initialSectionIds/外部素材入参；原型“池内/窗外”只按日期示意，未冻结向量资格，保存禁用，不能当已支持跨日报带素材保存。前台还需区分selectedDate、sourceDays(A名单四档7/14/30/365)、start/end(图裁切)，来源统计7/30/90独立第四口径；原型仅A14/来源30取样，不代表删除产品档位。overview.date只是全站资料日(10/27)，不能命名serverToday或卫生健康板块截止(实际最新日报10/24)，当前原型已改globalDataDate并删重复误标签。原型无存储/深链/加载失败实现，正式实施须补合同。旧三版保留。

**引用**：openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-28-cold-start-observation/README.md、openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-28-cold-start-observation/data-audit.md、openspec/changes/rework-workshop-into-radar-and-desk/ui-design.md

<!-- pinned 2026-10-28T09:02:17Z -->

## 用户明确批准：保留矩阵＋当日其他话题折叠层（压缩交接）

2026-10-28用户原话：「可以，这版我通过了，先把讨论记录pin一下，然后我打算压缩会话」。这是UI明确批准，但本轮只要求记录/压缩，不启动产品实现。获批文件ui-prototype/revision-2026-10-28-matrix-day-others/standalone.html，696943B，SHA256 cdb0b70e31fbc00740f04e908d5df2f372bc48fd8871a106416efb8d2e404448；最终12图在screens/visual-2026-10-28-v3-final/。
讨论收敛：用户最初嫌右轨空，偏向所选日尚未进入正式泳道的内容；后指出日期入口太多、卫生健康0正式泳道冷启动根本不能选日。文字日报主舞台/素材托盘稿被用户明确否定为丑且偏离主体，独立日期picker也不作为实施基线。用户要求保留当前矩阵、演进、滑动筛选、下方内联阅读；最终只增加与休眠同级的「当日其他话题」折叠组，纵向全宽、summary在内容前，紧凑行与正式行密度一致。成熟默认收起，0正式时默认展开；不再增加日历/日期条/另一套前后日期控制。
冷启动依据：卫生246030真实12期日报/16分区/0正式泳道，API有完整数据。现产品selectedLaneRow=null→reconcileSelectedDate null→LaneChronicle不挂，必须解除报日依赖正式泳道。日期轴由真实日报索引保留，列头/既有格/演进共用所选日；滑动只裁观察范围，范围排出选日时回窗内最近报日，不跳active日或today。sourceDays仍A名单/排序/休眠基准，B365只补同ID历史，来源7/30/90另口径。
其他话题取所选日真实report.sections，排除正式/休眠已可读分区；每个分区一行，只在所选日标该分区线索数，不伪造持久泳道/跨日合并。正式格仍同日主题桶max、演进日合计另算。点击复用原下方完整标题/原始thread.summary及真实关系/相关文章；历史candidate、无归属、未知不混当前管理资格，不提供绕门槛激活，不自动建/转正/归档或回填旧快照。API边界统一camelCase/字符串ID，保legacy消费者与既有阅读行为/原文/状态动作。
实施前仍需：按获批范围补delta specs/字段来源/测试资产M-R继承与冷启动完整故事（卫生0正式→选日报→其他展开→原下方阅读，成熟名单/休眠/滑筛回归）；落实板块日报索引/详情独立读取与selectedDate深链、加载/错误/缓存边界，不能直接复制离线JS/假数据。原型局限（中东轴止10/26但已冻10/27详情、移动日期条定位、hash非产品深链、无真实空日报样本）不能当产品规则。旧文字页、独立日历、素材托盘/跨窗建题入参扩展、其它日报/归档/关系页面未扩批；旧根53/1/2与他域债不因本次UI批准洗绿。
执行纪律：每轮独立任务用全新短上下文线程，不再薅旧01a1806长线程。用户最新指定opencode-go/deepseek-v4.1-flash:max，已经完整目录核实；故障停报，不静默换provider/model/CLI，同cwd单writer。只跑受影响测试，pnpm test:unit参数不加--，maxWorkers=2；上线静态部署另在实施验收后按规则执行。
用户随后通知六项增量Go门禁（lint/vet/build、dataenrichment/tagmanagement/topicgraph short tests）均已转绿。本轮未改Go、未自行重跑这些命令，按门禁通知记录并发阻塞解除，不归功本原型；也不等整change已完成/可归档。当前没有产品部署/提交/归档，等待压缩后用户启动下一步。

**引用**：openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-28-matrix-day-others/standalone.html、openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-28-matrix-day-others/validation.md、openspec/changes/rework-workshop-into-radar-and-desk/ui-design.md、openspec/changes/rework-workshop-into-radar-and-desk/explore-cold-start-observation.md

<!-- pinned 2026-10-29T02:40:39Z -->

## 8C准入父裁决：保留跨日带、排出回窗内最新报日、单writer串行

合同子线程01a185ac已完成8C.1，父已核验并校正，不可直接按子报告P1“暂不实现”落代码。以当前implementation-matrix-day-others.md/test-cases-matrix-day-others.md/specs为准：①已获批的同topicId跨日观察带必须纳入G3，只读复用section-timeline，按本窗真实记录日投影，>1日节点、1日说明、缺身份/失败/未取到不补0；节点峰值任一计数缺失为未知，同归属≠同事件/正式泳道，不用当前topic状态改历史。节点原子切同一selectedDate与稳定report/section；G3专用只读依赖不复制G1日报读取器。②范围排出所选日回“新窗内最新报日”，不是最近/等距；此前pin/brief里“最近报日”为错误提议，明确被本裁决覆盖。窗内无报日null，不跳today/active日；保留原连续日历轴，仅可选集合=报日交轴，不把轴压缩成仅报日。③冻结卫生图休眠0仍有同级summary(0/本窗无)，不得隐藏组标题；成熟默认收起其他、0正式默认展开。④无新增date= query，但打开日报/归档返回必须以稳定board/report/section/date恢复其他阅读槽、兼容旧lane返回。⑤手机与桌面互斥渲染同一连续日期轴，每视口一条入口，不是增第二日期条。⑥主仓库G1→G2→G3→G4全串行、每包全新短上下文，不可同cwd把G2/G3并行写。⑦静态验收必须新产物：源码review/目标测试后核foreign归属、备份/源码指纹，仅一次deploy-frontend.sh --no-restart(generate已含构建)铺新候选，按index/chunk/hash在5100跑S11-A/B/C，失败回滚、通过留盘，不重复build或重启无改后端。
父已补root test-cases.md的S11入口、tasks验证节本增量19 Scenario计划映射、5个新增Scenario(观察带/失败/节点/零休眠标题/返回)，旧103条映射与53/1/2等旧债不洗绿。strict+tasks-tail再次exit0，获批原型仍696943B/cdb0b70e…，UI approved。新增跨日字段已补spec锚/白盒OBV与S11-C，不可作不适用划除。API/后台/旧UI不扩域。
另一工作线又出现auxrepair字段缺失回归，随后用户提供lint/vet/build三项转绿通知；本会话未修Go/未重跑，不把旧Go失败当新阻塞，不推断所有short tests本次已重跑。父一次CodeGraph探索超时，不unlock/restart pi；后续已知路径可精确read/必要literal搜索，避免重复大范围探索。下一包G1只做日报读取store/normalizer/其他组纯模型/选日与目标测试，G2/G3/G4稍后新线程接，没实现或部署产品。

**引用**：openspec/changes/rework-workshop-into-radar-and-desk/implementation-matrix-day-others.md、openspec/changes/rework-workshop-into-radar-and-desk/test-cases-matrix-day-others.md、openspec/changes/rework-workshop-into-radar-and-desk/tasks.md、openspec/changes/rework-workshop-into-radar-and-desk/specs/workshop-board-console/spec.md

<!-- pinned 2026-10-29T04:25:07Z -->

## 8C G3收口接口与结构规范前置（G4必读）

G3收口线程01a18712完成，报告matrix-day-others-g3-closure.md；源组件OtherTopicReading366/14163B、BoardNarrowSummary385/12352B，按观察带/状态块/窄屏条/列表/组拆小。子目标4文件49例(8/13/16/12)、相邻Shell38/TS eslint绿；父实际复验OtherTopicReading+useOtherTopicObservation两文件29例绿，不称父跑全49或真实浏览器。
G4必须用最新接口：OtherTopicReading新增retryReport({date,sectionId})，组件不直接读旧reportId；父按ensureReportIndex({force:true})→ensureReportDay(date,{force:true})顺序再核稳定section。readingSettled由useOtherReadingSettledSignal发，需onMounted+nextTick、报告终态+观察ready/error/missing-id且refreshing=false；同context刷新可再次发，旧排队/卸载作废，不等相关文章、不主动滚。observer新增invalidate()：零GET、清成功数据/作废在途；生成完成若无其他行选择只invalidate，有选择则invalidate后ensureObservation(topicId)。不要照早期G3报告无retry事件或只看view.ready接线。
重要真实API核验：section-timeline正数days静默钳90天，当前导出loadBoardSectionTimeline(boardId)固定days0全历史；不再接窗口参数，覆盖all基于请求口径不靠records min/max推断。历史persistent_topic_id真实存在，nested当前topic属性不映历史。父GET：卫生246030 16sections/14307B/163ms，中东1974 702/693593B/1364ms；懒读板块单缓存，当前量可用，不承诺所有未来板块大小，不改后端。
G4前必须完成8C.4S结构包：code-style§8>500行/~15KB要按内聚拆，剩G1 useLaneDayReading523/25292B、G2Grid647/21457B，以及useLaneDayReading.test918/50337B、ObservationMatrix.test588/29954B、OtherTopicReading.test590/26300B。只结构不改公共API/DOM/行为，用Vitest JSON原case fullName/status multiset前后核对不丢用例/增skip，更新M/R/Scenario实际测试路径。G4 Shell约494/15045B、Shell.test1095/44663B不归结构包，G4接线时也须拆职责，不能堆成巨壳。仍主仓库单writer/新短线程串行，未部署/未提交归档，整体故事/真实UI和旧债不洗绿。

**引用**：openspec/changes/rework-workshop-into-radar-and-desk/ui-verification/matrix-day-others-g3-closure.md、openspec/changes/rework-workshop-into-radar-and-desk/implementation-matrix-day-others.md、front/app/features/tags/components/console/useOtherReadingSettled.ts、front/app/features/tags/composables/useOtherTopicObservation.ts

<!-- pinned 2026-10-29T10:22:06Z -->

## 其他话题跨日关联只按 topic ID，手动编排可切断原 ID 集合

当前观察带不消费 section relations：front/app/api/otherTopicObservation.ts normalizeSectionTimeline 仅映射 sections；useOtherTopicObservation.ts projectObservationDays 按 persistentTopicId 严格相等过滤，再按可见 axis 投影。后端 GetBoardSectionTimeline 不按 topic status 排除 active。dayOtherTopics.ts collectExclusions/isExcluded 排除 A 名单（正式+休眠）topicId 的全部当日分区，即使历史快照是 candidate 也不在其他组。手动编排 CreateManualTopic 会创建全新 active topic，并仅将所选 usable sections 通过 UpdateSectionTopicAssignment 迁到新 ID，因此未选旧 sections 与新泳道不再属于同一 ID 观察集合；不能凭标题/相似关系自动接续。otherTopicSelection.ts selectDayHeader 只改日期，日期 watcher 清旧其他选择；selectObservedDay 原子写日期+report/section/topic，可保跨日阅读。黄点设计需复用同 ID nodes，不承诺自动跨泳道接续。

**引用**：front/app/features/tags/composables/useOtherTopicObservation.ts:projectObservationDays、front/app/api/otherTopicObservation.ts:normalizeSectionTimeline、front/app/features/tags/components/console/dayOtherTopics.ts:collectExclusions、front/app/features/tags/components/console/otherTopicSelection.ts:selectObservedDay、backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:CreateManualTopic、backend-go/internal/topicgraph/repository/daily_report_repository.go:GetBoardSectionTimeline

<!-- pinned 2026-10-30T02:55:18Z -->

## 新建泳道当前宿主与底层编排能力并不等同

BoardComposeOverlay 是当前全屏新建泳道宿主，embedDays=14。画布点击 onNodeClick 设置 focusId，并仅对未归属 active 的节点 toggle；按天事件列表只 toggle、不设置 focus，因此列表选中不会更新焦点关联。宿主 relatedItems 消费 getBoardSectionTimeline(boardId,14).relations，按焦点节点查两端、按 distance 升序去重取6条，已归属 active 只读；addRelated 会把焦点切到新加入节点。useInlineCompose 另有按 selected embedding 均值（无选中时 query embedding）算 recommendations 的能力，但宿主传给 ComposeSidebar 的 recommendations 固定为空，也未接 recommend 事件，当前页面没有启用整组推荐。搜索触发 embedQuery，但宿主画布/按天列表不消费 rankedPool/nodeInfo，只按标题关键词过滤候选侧栏。adopt 会覆盖名称、追加候选owned与无归属相似节点（本次最多20），并非只预选候选自身、不替换已有选择。activate 直接updateTopic(status=active)，成功关闭宿主；save 则 createManualLane 新建，二者不是同一动作。宿主把候选启用门槛覆盖为 consecutive_hits>=2，与 composable 原 can_activate 及业务阈值有差异，设计不得将其默认固化为业务规则。页面当前还存在合并/历史回填入口，重新布局需对账不得默认删除。

**引用**：front/app/features/tags/components/console/BoardComposeOverlay.vue:onNodeClick, relatedItems, addRelated, sidebarItems、front/app/features/tags/composables/useInlineCompose.ts:adopt,activate,save,recommendations、front/app/features/tags/components/ComposeSidebar.vue

<!-- pinned 2026-10-30T05:53:56Z -->

## 候选启用/采纳/人工建题质心不同，人工成线不受相似边门禁

1) 自动candidate创建：daily_report_assignment.go assignAndUpdateTopics将首个section向量写Embedding；SaveReport事务提交后对touched话题调用UpdateCentroidOnSectionChange。ComputeTopicCentroid按report.period_date DESC,s.id DESC取最近CentroidWindow条有向量section（默认30条，不是30天），有效>=2时等权均值，<2回退topic.Embedding；topicAnchorVec匹配优先Centroid否则Embedding。2) 直接启用候选：UpdateTopic只改status/label/updated_at，检查累计HitCount门槛，不重算Embedding/Centroid；所以沿用已有滚动质心，缺失才首义回退。3) 采纳预览：useInlineCompose.adopt用当前14天pool内该candidate owned section均值扩选，仅前端草稿；不是读后台Centroid。4) 人工保存CreateManualTopic聚合全部有效已选section向量均值，写新topic.Embedding、active/manual；当前创建未显式写Centroid，匹配经Embedding回退取得该均值，后续日报命中会滚动刷新Centroid。保存不校验selected之间距离阈值、也不要求预存关系边。5) RebuildBoardRelations每次先删除版块旧边再算similarity；writeIdentityEdges将同topic节点按后续报告日期串接，不受相似距离门禁。故“归到同一泳道”与“保留不同归属、手动画永久关系边”不是同一能力；当前路由未提供手工关系边写API，不能仅插边宣称永久纠错（重建会清掉）。6) guided原型add/openPool可加入原14天未归属active池任意项，无关系/距离门槛，但把全量池放次级弹窗、图变只读，削弱了人工对照纠错；active节点仍被宿主/原型禁选。后端manual会重新归属有效section，但前端跨正式泳道纠错是另一个需显式确认的边界，不应静默开启。

**引用**：backend-go/internal/topicgraph/repository/daily_report_topic_repository.go:ComputeTopicCentroid,UpdateTopic、backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:CreateManualTopic、backend-go/internal/topicgraph/repository/daily_report_assignment.go:topicAnchorVec,assignAndUpdateTopics、backend-go/internal/topicgraph/repository/daily_report_repository.go:SaveReport、backend-go/internal/topicgraph/repository/daily_report_matching.go:RebuildBoardRelations,writeIdentityEdges、front/app/features/tags/composables/useInlineCompose.ts:adopt、openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-30-lane-compose-guided/prototype.js:add,openPool,graphClick

<!-- pinned 2026-10-30T08:39:46Z -->

## 新建泳道原型最新方向：主面材料与关系不得由推荐筛掉

用户否定只露算法推荐的引导版：算法漏连时人工需能直接对照/选入，不可把全量材料和图藏进弹窗。最新待审原型为 ui-prototype/revision-2026-10-30-lane-compose-correction/standalone.html，保留起点引导，编排主面默认全部13非active节点与4条原边；可显示52active只读参照，得到65节点112条可见两端原边。标题只聚焦，checkbox/独立移除改成员，无关系/距离阈值不阻止人工创建，人工选择不伪造关系边。主图不是relatedItems top6；右轨才沿焦点top6提示，默认高亮不筛掉节点/边。质心说明须明确：candidate/active都会随日报touched刷新所属最近CentroidWindow条section均值（默认30条），不足2回Embedding；UpdateTopic启用只改状态，不补算。旧缺centroid风险未修，snapshot topics未导出向量，不能假称已健康。原型37条交互检查含无边7347人工加入与7409组合后仍可创建模拟；不证明两新闻事实相关。跨active移入及永久手工语义边仍未批准，不静默开启。此前三栏/引导版合同与原型仅为历史，不作最新布局依据。

**引用**：openspec/changes/rework-workshop-into-radar-and-desk/ui-design-lane-compose-draft.md、openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-30-lane-compose-correction/prototype.js、backend-go/internal/topicgraph/repository/daily_report_topic_repository.go:ComputeTopicCentroid,UpdateTopic

<!-- pinned 2026-10-30T09:18:56Z -->

## 默认候选不是精选推荐：管理资格名单与编排方向混用

2026-10-30只读重看实际已部署 /tags/1974 新建泳道（等networkidle且65节点出现，排除加载中空态），确认为32个已中断候选被默认展开。实际接口采样：中东topics候选32/当前14天有可编排owned=0；生成式AI1980候选14/owned=0；开发者工具3030候选35/owned=6（34 stale）。原因：listBoardTopics消费ListTopicsByBoardAll（全量，不是TOP50截断），FilterVisibleTopics只对candidate检查累计HitCount>=UpgradeThreshold（默认3），无近期材料/内聚度/是否适合建线筛选；排序为status archived后置、hit_count DESC,id ASC；前端再can_activate优先/consecutive_hits排序，无主组则ComposeSidebar自动展开全部brokenGroup。更重要：新建编排复用了“话题管理资格名单”：当前pool里有中东10/AI5/工具45个candidate未在topics名单，逐个brief命中数全1或2，是observing被管理可见门槛合法隐藏，不是接口50条上限或漏数据。因此当前方向推荐不应简单复用管理可激活名单；管理可见门槛和用户手工编排指导应分开讨论，不能为纠偏破坏管理门槛。准确性例：工具candidate1769标题“Meta新模型智能指数超Gemini 4，OpenAI洽谈收购安全团队”，本窗3成员实际为Google访问限制、Anthropic/OpenAI同日更新、ChatGPT多智能体升级；至少标题与成员不一、是产品新闻混装，不足以当清晰叙事线。对照1906 OpenCompare发布及后续开源平替进展，同项目连续性更清楚。只用数据标签举例，不验证新闻世界事实。截图与原始API样本暂存/tmp/syntopica-default-candidates-1030/，current-stable-1974.png是稳定态；current-default-1974.png是过早截取的加载中，不可当最终无数据证据。本轮未改原型/产品，不写数据、不调用AI、不自动归档候选。

**引用**：backend-go/internal/topicgraph/repository/daily_report_topic_repository.go:ListTopicsByBoardAll,FilterVisibleTopics、backend-go/internal/topicgraph/handler/daily_report_handler.go:listBoardTopics、front/app/features/tags/components/ComposeSidebar.vue:watch(mainGroup)、front/app/features/tags/composables/useInlineCompose.ts:sidebarItems、front/app/features/tags/components/console/BoardComposeOverlay.vue:loadTopics,sidebarItems

<!-- pinned 2026-10-30T09:45:24Z -->

## 方向核对版最新起点：当前材料分组与历史启用名单分离

最新待审入口为 ui-prototype/revision-2026-10-30-lane-compose-directions/standalone.html。起点contained1120朴素列表；从compose-candidates当前非active candidate节点按原topic ID分组（含管理列表未展示的observing），展示提案为>=2条且跨>=2报告日，按最新材料排序，默认3组、其余可展开；不是后台新聚类或质量评分，也不是启用门槛。原话题名降级标身份，真实事件/日期作为主内容。previewPicks与selected分离，默认零预选，beginComposition只带明确勾选项，不调用adopt整组/相似补选；可跨组挑选，跨组不擅自命名。历史topics名单只有显式入口，原型启用资格沿can_activate（不是连续>=2），不改产品后端/PATCH规则。编排workspace保留纠错版全部13可选节点/原边、无边可人工加、active只读、Esc草稿保护。数据归组当前3个ID1984/1914/1845各2事件，不声称都准确；本次减少默认库存曝光并展示证据，未修后台语义质量或旧质心。各旧版合同/原型保留，本版仍pending，不作实现批准。

**引用**：openspec/changes/rework-workshop-into-radar-and-desk/ui-design-lane-compose-draft.md、openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-30-lane-compose-directions/prototype.js:buildDirections,renderDirections,beginComposition、openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-30-lane-compose-directions/verify.mjs

<!-- pinned 2026-10-30T10:50:56Z -->

## 最新点线关系版取代方向列表：进入即65点112边

用户再次否定方向列表，明确偏好最初点连接模型；当前待审入口已改 ui-prototype/revision-2026-10-30-lane-compose-graph/standalone.html，阶段合同 ui-design-lane-compose-draft.md。此前按candidate ID>=2跨日/默认3组展示提案已撤回，不进入当前主面，也未改过后台。现在直接workspace：左13个真实可选事件/编号/checkbox，中默认65点及两端在pool内全部112原关系（52active只读默认显示），右当前查看/已选成员/最后命名。原timeline是67/123，另外2节点不在compose pool，不造节点或边。图日期packing仅几何避让、不新聚类；原relation_type完整保留，无top6截断。点/标题只查看；add/remove只改成员，不能顺手移动focus或压暗其他边；焦点只强调邻接边，其余关系保持正常可见。Tab可见自绘圆环、不用遮编号的SVG原生方框；看全图恢复只读参照且保留成员；关只读显示13点4边。历史管理仍server can_activate；实际32项全true，disabled未覆盖；历史adopt本窗owned均空，实测新增0只验预填和保成员，不能宣称相似扩选成功分支通过。主面无边7347可与7409人工共选，不画假边、不受候选管理门槛限制。仍pending，只改原型，未动产品/后台/旧质心/快照、未部署。

**引用**：openspec/changes/rework-workshop-into-radar-and-desk/ui-design-lane-compose-draft.md、openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-30-lane-compose-graph/prototype.js:graphHTML,add,renderInspector、openspec/changes/rework-workshop-into-radar-and-desk/test-cases-lane-compose-graph-prototype.md

<!-- pinned 2026-10-30T14:04:57Z -->

## 用户纠正：左栏要候选泳道整组采纳；日期与连续三天口径待定

2026-10-30用户明确纠正点线版：左侧不是13条散装事件，应为候选话题泳道；点击一次候选应整组选中其现有成员，例如3个事件节点一次勾上；再按计算质心推荐补充事件，优先不在已有正式泳道中的。日期筛选必须纳入。连续>=3天的时间语义（窗口内曾连续/截至结束仍连续）和日期控件待用户确认，不能擅自拿累计3次替代。当前产品真实代码useInlineCompose.adopt先取pool内同persistentTopicId owned均值，预选owned后又自动补persistentTopicId==null相似项，总上限20；这与用户这次‘整组基础成员＋随后推荐’需要分清，不能把相似补选悄悄自动勾上。recommendations本已有unassigned优先/active次级的向量距离分组，可复用而不是只展示原relations邻接。后端GetComposeCandidates以最新completed日报为窗口锚，正days最多90，days<=0走几乎全历史（100000天），只有下界cutoff无显式起止参数；前端默认embedDays30。管理topics可见/CanActivate仍按累计HitCount>=UpgradeThreshold，不是实际连续日期，连续三天若作新建界面筛选必须单独声明，不悄悄更改全局生命周期/管理门槛。当前仍只探索设计，不改后台、产品或旧数据。

**引用**：front/app/features/tags/composables/useInlineCompose.ts:adopt,recommendations、backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:GetComposeCandidates、backend-go/internal/topicgraph/handler/daily_report_handler.go:listBoardTopics,composeCandidates

<!-- pinned 2026-10-30T14:33:34Z -->

## 最终修正：原连接跨越3天；候选整组带入与同窗质心推荐

用户最终说的是“连接跨越3天”，不是“连续3天”。先前连续3天口径/近30天空候选统计已撤回。当前待审入口 ui-prototype/revision-2026-10-30-lane-compose-centroid/standalone.html。candidateChains按原candidate ID、当前窗原identity/similarity边取连通片段，UTC首末日期差>=3天、>=2节点；允许断档，不按HitCount/ConsecutiveHits或固定3节点筛。点击adoptLane一次追加整片实际成员，去重、不截20、不自动相似补选、不吞人工项；name空才预填，focus不动。日期7/14/30/90及支持90窗内自选共控候选/图/推荐，有草稿改窗先确认清空，取消恢复控件与草稿。推荐用当前有效成员均值/原接口余弦阈值，其他candidate也在非active优先组，active次级只读，推荐不自动勾选、不造边。历史mean/distance已委托同一纯函数；原后台滚动Centroid不是本次草稿均值。真实新样本最新10/30：30天155点267边3候选；1984含7345@10/27、7409@10/28、7514@10/30，点击精确3选，23非active/114active达阈；1914两点跨度4也合格。7/14/30/90候选1/2/3/19。未入pool的timeline节点不补造。左侧素材候选可含观察中，历史管理按FilterVisibleTopics累计门槛合法隐藏，不能为强行同一可见集破坏管理门槛；采纳成新草稿≠启用原candidate。归档来源保留原非active资格并明确标注，不自动重启旧话题。12计算测试+45真实浏览器检查已过；date原生控件需native setter+input/change后真实Apply，不能用CDP insertText空值误测。产品取日期锚须最新completed元数据，不能在空首尾日报时照搬本固定样本的section min/max。仍只原型/制品，pending，未写产品/后台/旧数据或部署。

**引用**：openspec/changes/rework-workshop-into-radar-and-desk/ui-design-lane-compose-draft.md、openspec/changes/rework-workshop-into-radar-and-desk/specs/workshop-lane-compose/spec.md、openspec/changes/rework-workshop-into-radar-and-desk/ui-prototype/revision-2026-10-30-lane-compose-centroid/compose-logic.js、openspec/changes/rework-workshop-into-radar-and-desk/test-cases-lane-compose-centroid-prototype.md

<!-- pinned 2026-10-30T17:27:52Z -->
