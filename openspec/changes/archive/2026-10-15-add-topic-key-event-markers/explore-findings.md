
## 关键事件标记的最小入口与重建、信号快照边界

已核实：1) backend-go/internal/topicgraph/repository/daily_report_models.go 的 DailyReportThread 是独立日叙事条目，字段 ID/ReportID/SectionID/Title/Summary/TagIDs/RelatedArticleIDs/CreatedAt；PersistentTopic 归属要经所在 section 取得，thread 本身无 topic_id。第一版可在既有 thread 上呈现推荐及标记动作，不需要把整个 section 当成一件事件。2) front/app/features/tags/components/daily-report/DailyReportTopicSection.vue 以 section.threads 渲染当日与历史 thread，thread header 已有按钮触发 toggleThread，展开后可查 related_article_ids 并打开文章。新增标记动作不得嵌成 button 内的 button，也不能误触发已有展开动作。3) TopicGraphRepository.SaveReport 同 board/date 保留 report ID 但先删除旧 DailyReportThread 和 DailyReportSection，再插入新 sections/threads，旧 thread/section ID 不稳定。人工已确认标记不能只依赖将被删除的 thread 字段/FK；设计需保留轻量快照及来源退化语义，不能自动把旧确认转移给语义相近的新条目。4) SignalMaterialBuilder.AssembleSignalMaterial 读取所有状态的本 board 泳道，排除 lane.CreatedAt>cutoff；filterSignalSections 按目标周期与 cutoff 过滤，冻结 SignalMaterial 到 discovery.input_snapshot；SignalLaneMaterial 当前含 LaneID/Label/Status/Summary/Background/Articles（section_id/period_date/cluster_label/thread_titles）。已确认关键事件如作为发现线索，应随本次材料冻结并保持同 board/lane、周期与 cutoff 的事实边界，后续取消标记不可回写旧批次/候选/报告。现有研究仍只由用户点击候选 research 手动触发；不得因为确认标记自动研究或绕开信号依据白名单。

**引用**：backend-go/internal/topicgraph/repository/daily_report_models.go:DailyReportThread、backend-go/internal/topicgraph/repository/daily_report_repository.go:SaveReport、front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:toggleThread、backend-go/internal/dataenrichment/service/signal_material.go:AssembleSignalMaterial、openspec/specs/board-signal-reports/spec.md

<!-- pinned 2026-10-11T06:47:00Z -->

## 标记动作的现有 UI 绑定与泳道回看载体

UI 设计核实：DailyReportTopicSection.vue 的当日与历史 thread header 均是 button，disabled 条件为 !thread.related_article_ids?.length，@click 调 toggleThread 展开文章；标记控件需作同级独立动作，不能嵌套 button，不能继承没有文章时 header 的 disabled。thread-fit 的跑题状态标识在标题左位，关键事件状态不得覆盖/替代该警告。BoardThreadBrowser.vue 已有 viewMode=timeline|lanes|focus；onLaneClick -> enterFocus(topicId)，focus 视图有话题名、日期窗口（7/14/30/全部）与横向 section 时间轴，点击 section 在 btb-focus-inline 就地展开 thread（明确不打开 popup overlay）。非 focus 现有 thread popup 不新增新的浮层。focusMeta.empty 只根据 focusNodes（真实 section）判断；关键标记快照在原 section 被重建删除后仍可能存在，不能沿用 focusMeta.empty 提前返回而隐藏确认节点，也不能为快照伪造 section 图节点/identity 边。BoardThreadBrowser.focus.test.ts 已测 focus 就地展开、只显示当前泳道、拖拽不误触与空态；新增关键筛选应扩展这些资产而非删现有断言。可复用 AppButton（ghost/secondary，sm，loading/disabled）作为单步操作与 aria-pressed 筛选；AppToggle 现实现 click 在 div，没有原生键盘 button 语义，本 change 不为筛选顺手改基础组件。

**引用**：front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:toggleThread、front/app/features/tags/components/BoardThreadBrowser.vue:enterFocus、front/app/features/tags/components/BoardThreadBrowser.vue:focusMeta、front/app/features/tags/components/BoardThreadBrowser.focus.test.ts、front/app/components/ui/AppButton.vue、front/app/components/ui/AppToggle.vue

<!-- pinned 2026-10-11T06:52:39Z -->

## 泳道可读性反馈：节点密度与标题截断

用户明确选择“现有话题泳道长时间轴上高亮关键节点”，拒绝列表式关键回看，要求先优化原型。当前 BoardThreadBrowser.vue 的 focus 视图 FOCUS_COL_W=96，focus节点label max-width=92px、font-size=.72rem，文本经过truncateLabel按LABEL_MAX.slice截断；总泳道列宽=148*zoomScale，节点半径=7*fontScale。节点label小且密度高是确切的可读性限制，不应只加星标而不解决标题、日期和同日多个节点布局。本轮原型调整需保持时间位置稳定、普通动态可达、人工confirmed与AI建议视觉分层、来源快照不伪造section/关系；只改change原型和制品。主信息结构从列表筛选变为长轴交互，应将UI契约改major并重新等待用户对原型明确确认，不凭用户选择方向写approved。

**引用**：front/app/features/tags/components/BoardThreadBrowser.vue:FOCUS_COL_W、front/app/features/tags/components/BoardThreadBrowser.vue:truncateLabel、front/app/features/tags/components/BoardThreadBrowser.vue:focusMeta

<!-- pinned 2026-10-11T07:35:40Z -->

## FB-2校正：版块内总览与现有主题源，不替换状态色

现有TagsPage.vue（303-312）在contentTab='topic-overview'才挂载BoardThreadBrowser，board-id=selectedBoardId；左侧BoardListSidebar选择版块，selectedBoardId=null时是全部文章池，不是跨版块泳道总览。BoardThreadBrowser.vue watch [props.boardId,days]加载版块数据，现有viewMode为timeline/lanes/focus，lanes内点泳道enterFocus。当前页面壳为'叙事工坊'紧凑顶栏、左版块列表、右板块内容/话题总览/日报/文章/数据增强tabs，不应另造杂志封面式独立关键事件页。前端双主题唯一源front/app/assets/css/main.css：editorial bg-base=#faf7f2 bg-elevated=#f5f0e6 accent=#d94a4a，dark bg-base=#080c12 bg-elevated=rgba(17,27,38,.98) accent=rgba(240,138,75,.85)。根元素data-theme=editorial|dark，useTheme管理。BoardThreadBrowser statusFill只取--color-thread-status-emerging/continuing/split/merge/ending（新兴/持续/分化/合并/结束），重要性不允许覆盖这些既有状态色：用独立外环/星标/title样式。FB-2要求先选择版块、总览只显示该版块多条泳道、可进原focus，原型示例AI与住房必须分属不同版块。已对localhost:5100/tags只读看现有壳与泳道（临时截图/tmp，不入change），参考浏览器已关闭；不算产品交互验收，不修改真实数据。旧原型把不同域lane放一个示例版块及绿色主题，已被用户否定，勿作为新基线。

**引用**：front/app/features/tags/components/TagsPage.vue、front/app/features/tags/components/BoardListSidebar.vue、front/app/features/tags/components/BoardThreadBrowser.vue:statusFill、front/app/assets/css/main.css、docs/reference/standard/frontend/theming.md

<!-- pinned 2026-10-11T09:05:24Z -->

## 真实日报载体：一章多条线索，标记宿主是 DailyReportThread

2026-10-11只读实查静态localhost:5100/tags，选择AI版块、进入10/9日报、打开现有全屏阅读层。真实UI为报纸式封面/头条/highlights＋目录/正文/页边注（右注栏<1100沿原抽屉），不是TagsPage tab内扁平事件列表。正文按TopicGroup分组，group.sections可多项，section.threads逐条显示；现场第一展开group含1个section-card、5个thread，文章计数分别2/1/2/1/1；点第一条header只展开它的2篇依据。未点生成/编辑/归档/研究/批注写操作，浏览器keyevents-real-daily-lineage已close确认。
代码：DailyReportTopicSection.vue:270起article v-for thread in section.threads；.drm-thread__header本身是BUTTON，包thread.title、summary与related_article_ids篇数，并@toggleThread；无文章时header.disabled，但不得把这个disabled继承给有效标记。新标记/AI建议/确认/不采纳须宿主article里的同级控件，不能嵌套button或挂整个TopicGroup/section头。summary的data-mn-anchor=t:<thread.id>是页边注锚，保留原文/锚与fit_distance跑题提示。
后端DailyReportThread（daily_report_models.go:180-203）有ID、ReportID、SectionID、Title、Summary、TagIDs、RelatedArticleIDs、Confidence、FitDistance；它不直接持有topic_id，归属通过section.PersistentTopicID验证。标记粒度是thread：同section两条不同线索独立，单thread多篇文章仍只一条标记；不得sectionHasKey→全章/整泳道重要，不更改section锚定或生命周期。
现设计D1/D2本来选thread快照/来源动作，错误在现原型fixture一thread=一section＋日报扁平列表，不能作为真实日报接入基线。下一版原型须真实阅读器层级＋同section多thread fixture，验证标A不影响B/父标题状态计数关系/页边注。总览确认线索注记也按marker/thread区分，不能伪造多个真实section或借section标题当线索标题。当前原型148断言是旧简化形状检查，不是此多线索契约或产品验收。

**引用**：front/app/features/tags/components/BoardDailyReportTimeline.vue:548-598、front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:97-109,270-334、front/app/api/dailyReports.ts:DailyReportThread、backend-go/internal/topicgraph/repository/daily_report_models.go:DailyReportThread、docs/reference/flow/daily-report.md:需求说明

<!-- pinned 2026-10-11T15:10:22Z -->
