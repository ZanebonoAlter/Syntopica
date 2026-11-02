## 0. 契约与脚手架

- [x] 0.1 在 `docs/reference/standard/frontend/layout.md` 登记固定视口栅格例外（理由 / 目标视口 / 溢出策略），并在 ui-design.md 与 policy 交叉引用
- [x] 0.2 建立新旧组件共存目录约定（`components/console/` 新壳组件，旧面板保留至阶段④），在 `front/app/features/tags/README` 或组件头注释写清归属
- [x] 0.3 把 `BoardHealthPanel` 拆为「信号区（评分/趋势/候选 topN）」与「明细区（质量表/组合/候选全量）」两个可独立渲染的区块（对外 props 与事件不变）
- [x] 0.4 独立 review（2026-10-25 用户投诉后第二轮）：按 `review.md` 逐项复核旧能力迁移、真实数据语义与产品任务；High/Medium 修复回写 `ui-verification/report.md`，未完成项登记 `human-feedback.md` 处理中

## 1. 路由与骨架

- [x] 1.1 `front/app/pages/tags.vue` 拆为两层：`/tags` 与 `/tags/:boardId`；旧 `TagsPage.vue` 保留为路由未切换时的兜底，深链参数 `?board=&report=&annotation=` 映射到新路由
- [x] 1.2 新增 `BoardConsoleShell`（面包屑 + 版块栏 + 舞台 + 治理右轨三区，workspace + 固定视口栅格），承载视图切换与选中泳道状态
- [x] 1.3 选中泳道状态落到 `?lane=<topicId>`，支持刷新恢复与非法 id 回退（分区数最多的泳道）
- [x] 1.4 面包屑与版块切换：切版块时清空旧版块的展开状态与抽屉

## 2. 矩阵与演进带

- [x] 2.1 新增 `BoardMatrixView`：泳道 × 日期网格（14/7/30/全部）、行尾合计、今天列高亮、格内 section 数、空格断档样式
- [x] 2.2 休眠泳道折叠：窗口内 ≤2 个 section 的活跃泳道折叠成一行可展开条（前端派生，不改话题状态）
- [x] 2.3 命中格打开 `SectionDrawer`（32rem overlay）：分区标题、当日事件、当日日报线索、两个动作（打开日报 / 看归档）；不加「加入新泳道」以避免移动已归属 section；空格给出提示
- [x] 2.4 演进带：选中泳道按日期渲染节点（大小 = section 数，≥3 日显示标签）、今日事件、最密集三天、态势句折叠入口
- [x] 2.5 矩阵与演进带的 loading / empty / error 态与「保留上次数据」策略

## 3. 治理右轨与抽屉

- [x] 3.1 `BoardGovernanceRail`：体检分 + 分档色 + 趋势迷你条 + 趋势摘要 + 挂载/幽灵/向量缺失 + 候选 top3（含收编）+ 来源 top3（占比条）+ 研究/追踪入口
- [x] 3.2 `BoardGovernanceDrawer`（32rem，五页签）：构成质量表、组合标签、候选全量、来源全量（窗口/排序）、版块设置
- [x] 3.3 右轨失败不阻塞矩阵（局部错误 + 重试）；无体检快照显示「等待首轮体检」且构成管理可用
- [x] 3.4 全局参数收回版块页「更多」菜单（匹配参数、标签合并、扩充版块、匹配回填、标签池）；设置页完整迁移另立变更

## 4. 归档

- [x] 4.1 后端：`GET /api/semantic-boards/:id/articles` 增加 `in_report` 与 `report_status` 过滤（口径派生自窗口内日报分区线索，service 层唯一实现）
- [x] 4.2 `BoardArchiveView`：日期统计条（文章/已入/未入/弱证据）、日期快捷+自由起止、平铺文章行（标题/来源/全部命中标签/入日报状态）、来源与方向筛选
- [x] 4.3 展开组：每篇一行的日期/标题/来源/规则分数 + 「未入日报」标记 + 规则语义解释（含降级与方向校验说明）
- [x] 4.4 排序：按日期分组倒序 → 同日完整发布时间倒序（id 仅作同时间平局兜底）；不跨日期按遗漏打乱
- [x] 4.5 文章抽屉（60vw）：是否入日报、逐标签完整证据、站内阅读、打开原文；保留归档上下文

## 5. 编排态

- [x] 5.1 `BoardComposeOverlay`：全屏画布（日期列 × active 泳道行 + 待确认行）、顶栏（名称 / 成员 / 平均距离 / 离群 / 取消 / 保存）
- [x] 5.2 接入 `useInlineCompose`：采纳（预填名 + 预勾上限 20）、确认启用（连续 ≥2 天才可点，置灰带提示）、已中断折叠、搜索过滤
- [x] 5.3 关联推荐：点节点按 `section-timeline.relations` 距离取 top-N，画连线 + 侧栏列出并可一键加入；全量关系开关默认关
- [x] 5.4 保存校验（成员/名称）→ 创建 → 回矩阵高亮新泳道；取消/Esc 丢弃并恢复
- [x] 5.5 合并泳道入口（复用现有 source/target 对话框）与回填/回刷移入编排态或 ⋯ 菜单

## 6. 日报阅读台

- [x] 6.1 路由 `/tags/:boardId/:date` + `DailyReportDeskShell`
- [x] 6.2 把 `BoardDailyReportTimeline` 的 reader 从 fixed 浮层抽到路由容器，内部逻辑零改动（页边注 / watch 定位 / 深链 / lifeline 缓存）
- [x] 6.3 **2026-10-27 用户复审恢复旧版阅读页**：大刊头 + 今日重点 top3、左目录（本期目录/持续追踪/历史日报/版块切换）、宽屏正文自适应、右侧常驻页边注栏，页面整体滚动；撤下 40rem 两栏、日期下拉、scroll-spy 与页边注抽屉（<1100px 仍保留抽屉兜底）
- [x] 6.4 撤下 `DailyReportDeskEvolution` 与「在矩阵中查看这条线」入口（随新版索引一并撤下；矩阵聚焦入口留在版块态势页）
- [x] 6.5 返回/Esc 回矩阵并恢复进入前的选中泳道；无日报空态给生成入口
- [x] 6.6 2026-10-28 用户反馈修复：日报预览的状态同步空转（站内阅读预览的 `@favorite`/`@article-update` 原为空实现 → 点收藏完全无响应）——改乐观更新 + 失败回滚 + 错误提示，矩阵页日报时间线预览同步同契约
- [x] 6.7 2026-10-28 用户反馈修复：弹窗内「全屏」被遮罩盖住（`ArticleContentView` 全屏层硬编码 `z-[100]` < `AppDialog` 遮罩 9100 → 点了像没反应）——全屏层级改由宿主经 `fullscreenZIndex` prop 传入（弹窗宿主 9200，仍低于 toast 9999）；阅读页原地全屏保持 100

## 7. 窄屏降级

- [x] 7.1 `<768px` 雷达单列列表（不渲染矩阵 / 右轨）
- [x] 7.2 版块摘要：泳道列表 + 每行 14 天迷你条 + 「看今天日报」入口
- [x] 7.3 阅读台单栏（<1100px 隐藏页边注栏、<768px 抽屉全屏化）
- [x] 7.4 编排入口在窄屏禁用并提示「新建泳道需要宽屏或横屏」；`useIsNarrowViewport()` 统一读取断点

## 8. 雷达与聚合端点

- [x] 8.1 `BoardRadarView`：热度行（名称 / 今日 / 环比 / 前 3 泳道密度 / 日报标题）、分档标记、常温折叠、今日该读、升温提示
- [x] 8.2 后端 `GET /api/semantic-boards/overview`：单次返回各活跃版块的当日/近 3 日/14 天分区数与最新日报摘要；复用 lane-dynamics 与日报列表 service，5s 超时降级为零值
- [x] 8.3 雷达交互：点行进 L1；「今日该读」进 L1 并高亮；空态与错误态
- [x] 8.4 阶段④清理：删除旧 `contentTab` 分支与退役面板（`LaneDynamicsPanel` 卡片区、`BoardThreadBrowser` 浏览态、`BoardTimelinePanel`、标签池入口）（2026-11-02 执行：物理删除 TagsPage/BoardListSidebar/LaneDynamicsPanel/BoardTimelinePanel/BoardDailyReportTimeline/BoardThreadBrowser/topicFocus/BoardHealthPanel 本体/useBoardTimeline/watchPanelInject 孤儿键及全部配套测试；`useTagsPage` 保留治理+预览职责并删五 tab/timeline 死代码；`BoardSourcePanel`/`AuxiliaryLabelPool`/`LaneDynamicsCard`/`useBoardHealth`/侦探墙系新壳在用不动；装配断言由 BoardGovernanceDrawer/BoardWideWorkface 承接；lint 0 err + typecheck 过 + 受影响 4+2 测试文件 49 例全绿）
- [x] 8.5 2026-10-25 产品复审修订：暗色根底色与控制台适配、主页入口、雷达/控制台新建版块、匹配规则/扩充版块/匹配回填/标签合并/标签池回到「更多」菜单、归档改日期筛选 + 平铺文章 + 逐标签证据、编排态「按天选事件」、阅读台「索引 + 正文」两栏 + 页边注抽屉

## 8A. FB6 已批准的总览实施（其他区域不随本阶段批准）

- [x] 8A.1 按 implementation-overview-stage.md 与已批准单HTML总览重排 BoardRadarView 总览簇；真实接口、数据语义、既有路由/全局治理动作保留，不移入原型数据或调试壳（R-17/18 修复轮：首次失败不混空态、Radar API 客户端归一 camelCase/字符串 ID、主 Vue 拆分为 ≤500 行/…15KB 组件簇）
- [x] 8A.2 在 test-cases.md 记录本阶段旧用例继承/调整与必要白盒；新增数据边界转换时验证真实raw响应已接入，不只改组件mock（api/normalizers/boardRadar.test.ts + semanticBoards.test.ts Radar 接线）
- [x] 8A.3 `cd front && pnpm test:unit app/features/tags/components/console/BoardRadarView.test.ts --maxWorkers=2` 及新增受影响用例、受影响文件eslint通过；不跑全量单测（修复轮 5 files / 47 tests 全绿 + TS eslint exit 0，命令全路径见 test-cases.md S8）
- [x] 8A.4 父线程独立Review与统一收尾：必要typecheck/构建，静态实页总览故事/宽窄双主题核验，安全发布与范围说明；证据 `ui-verification/overview-stage-2026-10-27/report.md`。49目标测试通过后最终UI增量18组件测试通过；本阶段类型0，全局7处他域类型欠账已登记。已静态部署且未重启后端；其他旧fail/未覆盖不据此洗绿，整change未归档

## 8B. 矩阵/追踪获批后的实现准备与交付（其他页面不扩批）

- [x] 8B.1 记录用户明确“可以，我批准了”：冻结scope-cleanup单HTML/hash，正式ui-design与矩阵阶段合同批准范围同步；证据 `ui-approval-matrix-stage.md`（仅UI批准，旧验收欠账不洗绿）
- [x] 8B.2 将用户已确认的观察窗路线（design D11）及真实读取边界落实到spec/字段契约/test-cases：sourceDays仅明确独立档按钮/URL days改变；A唯一决定名单/source全轴前端clueTotal→sectionTotal→ID排序/休眠/候选，B365只补A同ID历史，拖至恰7/30/365日也不切base；补齐标题→摘要→相关文章及板块级管理的旧用例继承与调整，不复制样本、不自行切成from/to统计模式
  - 产出：`specs/workshop-board-console` 与 `specs/board-lane-dynamics` delta 修订（日格桶峰值/演进日合计/观察窗/当日工作面/入口作用域）+ 新 `implementation-matrix-stage.md`（映射/字段锚/互斥分工）+ 新 `test-cases-matrix-stage.md`（主链路/五组变体/继承与调整表/OX/MX/EV/CH/RD/CA/RS 分支表/真实浏览器计划）+ 总账 `test-cases.md` 增 S9 入口；旧账 53 pass/1 fail/2 未覆盖不重写不洗绿
  - 验收：`openspec validate rework-workshop-into-radar-and-desk --strict` 通过；`bash scripts/harness/check-tasks-tail.sh openspec/changes/rework-workshop-into-radar-and-desk` 通过；本任务勾选前需父线程语义验收OX-1/2/7/8/11…14与CA-1…8（A/B失败分层、365去重、四档/列表1/报告14/文章20、FIFO/跨板块卸载释放/失败不入成功缓存/当前读项稳定），以及P1–4与真实研究入口；strict/tail绿不替代语义验收
- [x] 8B.3 按§0.6明确互斥实现辖区与定向验证，再按批准合同落实矩阵/演进/右轨及必要同域接线；不复制离线数据，不新增无依据入口，不改其他待审页面或生命周期
  - 分组（implementation-matrix-stage.md §5，同文件互斥、BoardConsoleShell/useBoardConsole 单组独占）：G1 数据层（useBoardConsole 双通道/epoch/缓存 + observationRange 纯函数）→ G2 视窗控件（ObservationRangeControls/LaneTrendChart，可与之并行）∥ G3 矩阵+演进 → G4 当日编年（useLaneDayReading/LaneChronicle/SectionDrawer 路径退役）→ G5 壳层与右轨收敛（最新日报主入口/撤更多菜单/右轨重排/板块管理/窄屏）串行收口；后端零改动，foreign 文件（TagsPage/BoardThreadBrowser/daily-report/**/lane_snapshot_repository.go 等）禁改
  - 验收（每组）：`cd front && pnpm test:unit <受影响文件...> --maxWorkers=2`（不带 `--`）全绿 + `pnpm exec eslint <受影响 TS 文件>` exit 0；不跑全量单测/build/浏览器（留 8B.4）
- [x] 8B.4 定向测试与真实静态交互核验、独立review、部署影响说明及安全静态发布；通过前不把UI批准当产品交付，旧fail/未覆盖继续留账
  - 阶段证据：`ui-verification/matrix-stage-2026-10-28/delivery-summary.md`（首轮失败留存、run2真实修复复验、run3 N1/关键烟测）；第24步按原表指定纯逻辑+组件层由当前70目标补证，未冒称浏览器容量大样本通过。生产静态index=`a35cd4cb…`、后端未重启；旧根53/1/2、他域7类型债、真机/真实生成时序未验保留，不代表整个change完成或可归档。
  - 核验清单：按 test-cases-matrix-stage.md §真实浏览器计划跑静态 ：5100 主故事（1440/1920/2560 + 375/390，双主题，机械断言），证据落 `ui-verification/matrix-stage-<date>/report.md`；独立 review 建 review.md 账；typecheck/build 由本批集中跑一次
  - 部署影响：`bash scripts/dev/deploy-frontend.sh` 静态发布（后端零改动不重启）；向用户汇报可见变化（日格桶峰值/行尾总数移除/点格改就地下读/时间窗控件换刷选/右轨重排与全局入口归总览）与「无需迁移/重生成/配置、旧数据不回填」

## 8C. 矩阵「当日其他话题」已批准增量（2026-10-28；其他页面不扩批）

- [x] 8C.1 合同/字段/故事/M-R 继承与必要 delta specs（本增量实施前准备，不改产品代码）：新增 `implementation-matrix-day-others.md`（用户影响与零迁移、字段来源表、日期轴/选中日模型、组与阅读状态机、单-多 seam、独占辖区、G1/G2/G3 门禁、未裁决项）与 `test-cases-matrix-day-others.md`（S11 冷启动+成熟不回归主链路、五组变体、效果核对、展示字段盘点、继承与调整逐行表、RDY/OTH/ORD/GRP/RDC/NAR 白盒、真实浏览器计划）；`design.md` 追加 D12 落点；delta 只扩 `workshop-board-console`（日报日驱动的日期轴/当日其他话题折叠组）与 `board-lane-dynamics`（报日与报告优先读取边界），`daily-report-system` reader 零改动不追加 delta
  - 验收（本批实跑）：`openspec validate rework-workshop-into-radar-and-desk --strict` → valid（exit 0）；`bash scripts/harness/check-tasks-tail.sh openspec/changes/rework-workshop-into-radar-and-desk` → ✓ 尾三节/doc-impact 齐全（exit 0）；`test-assets.sh board-lane-dynamics`（8 Requirements/25 Scenarios；archive 4 change）、`workshop-board-console`（无主 spec/无 archive）、`daily-report-system`（27 Requirements/115 Scenarios；archive 15 change）反查结果写回用例账本「实际命令」；本项只代表合同完成，**不代表**产品实现或 UI 验收，8C.2–8C.8 不得据此勾选
- [x] 8C.2 G1 数据与模型（父独立复验R1/R2新增10例已绿；R3覆盖天数收缩时force结果被忽略已由G2线程前置小门禁补测修复并关闭，见下条；父复核 R1 索引新旧响应/R2 清缓存后晚到已复现并修复：新增 10 例先红后绿，修复报告 `ui-verification/matrix-day-others-g1-race-fixes.md`；原 106/66 自测记录保留且不声称覆盖该等竞态）（独占辖区：`composables/useLaneDayReading.ts`、`api/normalizers/laneDayReading.ts`、`components/console/dayOtherTopics.ts`（新）、`components/console/laneConsoleView.ts`、`components/console/useConsoleSelection.ts` 与同组测试）：报日索引暴露/1 份缓存/`ensureReportDay`/lane 与 report 双 epoch；normalizer 补 `topic_status_at_report`/`persistent_topic_id`（字符串 ID/camelCase 只在边界）；其他组纯模型按 sectionId+persistentTopicId 身份排除、cluster_index 序、状态快照文案、只标所选日；选中日三级校正（钢结构见合同 §4.1）。后端零改动
  - 验收（本批实跑，证据与接口交接见 `ui-verification/matrix-day-others-g1.md`）：`cd front && pnpm test:unit app/features/tags/composables/useLaneDayReading.test.ts app/features/tags/composables/laneDayReadingCache.test.ts app/api/normalizers/laneDayReading.test.ts app/features/tags/components/console/dayOtherTopics.test.ts app/features/tags/components/console/laneConsoleView.test.ts app/features/tags/components/console/useConsoleSelection.test.ts --maxWorkers=2` → 6 files / 106 tests passed；`pnpm exec eslint`（本组 10 个 `.ts`）→ 0 error 0 warning；相邻回归 `LaneChronicle.test.ts + BoardConsoleShell.test.ts` 66 tests passed（未破 G4 未改的壳/编年）；未跑全量/typecheck/build/浏览器/后端，未触碰任何 `.vue`/后端/部署
  - 竞态修复验收（R1/R2，报告 `ui-verification/matrix-day-others-g1-race-fixes.md`）：先复现红 `useLaneDayReading.test.ts + laneDayReadingCache.test.ts` → 2 files / **10 failed | 46 passed (56)**（旧列表缩回报日、旧失败写 indexError、旧列表致 lane 误报 no-report；clear 后旧响应回填/删新在途槽；切板块旧成功/失败晚到复活条目）；修复后同命令 → **2 files / 56 tests passed**（连跑 3 次 56 稳定）；本包 6 文件完整集合 → **6 files / 116 tests passed**（旧 106 + 新增 10，细分 normalizer 9 / useLaneDayReading 45 / laneDayReadingCache 11 / dayOtherTopics 20 / laneConsoleView 23 / useConsoleSelection 8）；本组 12 个 `.ts` eslint exit 0；相邻 `LaneChronicle + BoardConsoleShell` 66 tests passed。接口无签名变更（缓存结果仅加可选 `stale?`），未新增第二 store/缓存/helper 文件
  - R3 关闭验收（生成后 anchor 前移使索引需求覆盖 369→365，force 新鲜列表被「已有更大覆盖不缩回」比较丢弃；报告 `ui-verification/matrix-day-others-g2.md` §A）：先复现红 `useLaneDayReading.test.ts -t 'R3 —'` → **2 failed | 45 skipped**（`expected [ '2026-10-22' ] to deeply equal [ '2026-10-22', '2026-10-28' ]`——新报日不可见；第二例实发 365 而非在途 369）；最小修复只动 `requestIndex` 的**有效 days = max(请求, 当前成功索引, 同板块在途)** 后同命令 **2 passed**；G1 原 6 文件集合 → **6 files / 118 tests passed**（116 + R3 新增 2，不把新例冒旧已覆盖）；本组 12 个 `.ts` eslint exit 0；相邻 `LaneChronicle + BoardConsoleShell` 66 tests passed。未改接口/缓存/选日/normalizer/`.vue`
- [x] 8C.3 G2 网格与组（独占：`components/console/ObservationMatrix.vue`、`ObservationMatrixGrid.vue`、`DayOtherTopicsGroup.vue`（新）+ 同组测试）：0 正式泳道仍渲染日期轴（`no-lanes` 不再替换网格）；报日列头可选且与非报日可区分；组顺序（正式行→其他→休眠）/summary 在前（休眠 summary 从底部前移）/全宽/默认态只初始化一次/与休眠互不串联；其他行两行 clamp+完整 title/aria、只标所选日；长标题不撑破
  - 验收（报告 `ui-verification/matrix-day-others-g2.md` §B，实际跑，Linux 本机串行、参数不带 `--`）：`cd front && pnpm test:unit app/features/tags/components/console/ObservationMatrix.test.ts app/features/tags/components/console/ObservationMatrixGrid.test.ts app/features/tags/components/console/DayOtherTopicsGroup.test.ts --maxWorkers=2` → **3 files / 55 tests passed**（matrix 34 / grid 9 / group 12）；直接消费者回归加 `BoardConsoleShell.test.ts` → **4 files / 93 tests passed**（shell 38 未破）；本组 3 个 `.ts` eslint exit 0（**Vue 文件未被 eslint 覆盖，不声称 Vue lint 绿**）。接口：`reportDates`/`indexLoading`/`indexError`/`otherTopics`/`othersOpen`/`selectedOtherSectionId` 全部可选，新事件 `selectDayHeader(date)`/`toggleOthers`/`selectOther({sectionId,date,topicId?})`，旧事件 `selectLane/selectDate/toggleDormant/retry/gotoSelected/generate` 保留；testid `matrix-dayhead-<date>`/`matrix-report-index-hint`/`other-topics-toggle`/`other-topics-count`/`other-topics-note`/`other-topics-extra`/`other-topic-row-<id>`/`other-topic-cell-<id>-<date>` 新增，`dormant-toggle` 保留。可见变化：0 正式泳道/无日报降为网格前内联说明（生成入口仍在）、休眠 summary 前移至自身行前且 0 行仍保留（本窗无、禁用）、报日列头成第三处同源选日。**未跑** typecheck/build/全量/浏览器/Go/部署，默认展开态初始化与真实视觉留 G4/G5
- [x] 8C.4 G3 阅读、跨日观察与窄屏（父复核 G3-M1/G3-M2/G3-S1 已关闭，证据 `ui-verification/matrix-day-others-g3-closure.md`；旧 38 例与相邻 38 历史保留，新角只在本包目标/标准尺寸/TS eslint 层面已验，不代表 UI 验收或整 change 完成）（独占：`components/console/OtherTopicReading.vue`（新）、`BoardNarrowSummary.vue`、必要的新section-timeline API归一化/只读composable + 同组测试）：保留获批同topicId本窗真实记录日观察带（>1日可点节点、峰值/未知不冒0、同归属不称同一事件、节点原子切日与稳定分区选择、失败重试/晚到隔离）；其他分区阅读面板（分区原题 + 全量线索 + 逐字摘要单条互斥 + 相关文章按需复用 cache + 打开日报/归档），复用 `LaneChronicleThread`/`LaneRelatedArticles` 不复制实现；窄屏同源报日条（同一 `selectedDate`、选中日滚入视野）+ 其他组 + 同一阅读槽，0 正式泳道不再只剩空态
  - 验收（本批实跑，证据与 G4 接口交接见 `ui-verification/matrix-day-others-g3.md`）：`cd front && pnpm test:unit app/api/otherTopicObservation.test.ts app/features/tags/composables/useOtherTopicObservation.test.ts app/features/tags/components/console/OtherTopicReading.test.ts app/features/tags/components/console/BoardNarrowSummary.test.ts --maxWorkers=2` → **4 files / 38 tests passed**（API 8 / hook 10 / 阅读组件 8 / 窄屏 12）；相邻 `BoardConsoleShell.test.ts` → **1 file / 38 tests passed**；本组 6 个 `.ts` eslint exit 0（Vue 文件不被 eslint 配置覆盖，不称 Vue lint 绿）。观察带固定 `days=0`（端点全部历史；正数会被后端静默钳到 90 天）与未验边界见报告。未跑全量/typecheck/build/浏览器/Go/部署；不代表 UI 验收或整 change 完成
  - 收口批（2026-10-29 全新短线程实跑，证据与 G4 交接见 `ui-verification/matrix-day-others-g3-closure.md`）：同一命令 → **4 files / 49 tests passed**（API 8 / hook 13 / 阅读组件 16 / 窄屏 12；旧 38 例中 API 原「显式 days 透传」1 例按新契约（不开放窗口 days）替换为窗口守卫，其余保留并新增 11，不与旧 38 相加当总数）；相邻 `BoardConsoleShell.test.ts` → **1 file / 38 tests passed**；本组 7 个 `.ts` eslint exit 0；S1 拆分后 source 全过 500 行/~15KB（`OtherTopicReading` 530→366 行/14163B、`BoardNarrowSummary` 795→385 行/12352B）。M1 新接口 `retryReport({date,sectionId})`（G4 必须 force 索引→force 当日）；M2 终态信号等挂载+DOM+观察真实终态（`refreshing` 门）；观察 `invalidate()` 懒失效不 GET。未跑全量/typecheck/build/浏览器/Go/部署；G4 顺序、真实几何/触屏、大测试与 G1/G2 文件体量另包，仍不冒称已验
- [x] 8C.4S 纯结构规范收口（G4前，单writer）：按`standard/frontend/code-style.md`§8将G1读取store/索引职责与G2网格内聚拆小，并拆useLaneDayReading/ObservationMatrix/OtherTopicReading三份大测试和小共享fixture；每文件≤500行/~15KB，不压行/删用例/造大万能工具，不改公共API/DOM/行为。拆分前后目标Vitest JSON用例multiset与通过/跳过数一致；必要同步S11/M-R/Scenario→测试路径，结果记录`ui-verification/matrix-day-others-structure.md`，不把结构包冒新功能或真实视觉验收
  - 验收：精确受影响测试文件集合`pnpm test:unit <实际文件清单> --maxWorkers=2`前后通过且无新增skip/少例；本组TS eslint exit0；逐文件行/字节核规范；strict/tasks-tail通过，真实几何留G5，不build/全量/Go/部署
  - 实跑验收（2026-10-29 结构规范包，报告 `ui-verification/matrix-day-others-structure.md`）：重构前 10 文件/**189 passed**、重构后同行为集合 18 文件/**189 passed**，`fullName + status` 多重集 SHA 前后一致（缺例 0 / 新增 0 / skip 0 / todo 0）；本组 19 个 `.ts` `pnpm exec eslint` exit 0（0 error 0 warning；Vue 文件不被 eslint 配置覆盖，不称 Vue lint 绿）；全部 source/test/helper ≤500 行且 ≤15360B；`openspec validate --strict` 与 `check-tasks-tail.sh` 通过。未跑全量/typecheck/build/浏览器/Go/部署。
  - 拆分落点（before→after 行/字节）：G1 `useLaneDayReading.ts` 522/25292 → 169/7913（组合根，仍唯一状态所有者）+ `laneDayReportIndex.ts` 181/9486（索引请求/覆盖/请求序/陈旧）+ `laneDayReportDay.ts` 141/6961（报告优先读取）+ `laneDayCurrentReading.ts` 177/8256（编年当前日）+ `laneDayRelatedArticles.ts` 109/5199（相关文章条目）；G2 `ObservationMatrixGrid.vue` 646/21457 → 447/15224 + `ObservationMatrixDayHeader.vue` 151/5077 + `ObservationMatrixLaneRow.vue` 212/6012（多根片段，不加 wrapper，列头/行样式随子组件走、其他组 `:deep` 留在父）。三份大测试按内聚子套件拆为 5（useLaneDayReading）+3（ObservationMatrix）+3（OtherTopicReading）个 test 文件 + 3 个非 `*.test.ts` harness（fixture 共享、断言与用例名不变）。G4 接口保持：`useLaneDayReading` 导出/返回键/签名、Grid 公共 props/emits/testid/类名与 DOM 顺序均未改。
- [x] 8C.5A G4a 状态/读取/返回契约准备（父已独立24例绿；G4b入场两个前置角已补真红→绿，56/94历史保留）：新增壳层单份reading+observation的最小组合控制器、其他分区选择与首份A默认展开、受控重试/生成后失效编排；扩既有返回载体/恢复helper支持稳定其他分区/阅读key与旧lane兼容、URL四参优先与用户滚动意图取消，先逻辑/作用域测试。不写Shell/Body/Stage/Chronicle/G2/G3视图，不新date= query。报告`ui-verification/matrix-day-others-g4a.md`；子接口稳定后才G4b
  - G4b 入场前置（2026-10-29，证据 `ui-verification/matrix-day-others-g4b.md`）：`useConsoleReading.retryReport` 在 force 索引 await 之后、任何旧 date 读取之前复核 board+选中日（含被清空），过时直接 `applied=false` 停止（不再先写 reportDay 再补救）；`otherTopicSelection.selectOther` 改为只接受 G1 `otherTopics.rows` 成员（正式/休眠泳道分区虽然也在当日详情里，但不得进入其他选择；组 ready 0 行同样拒绝）。新增 4 例先真红（2×旧日抢写 + 2×成员越权）→ 最小修后同 6 文件 **60 tests passed**（56 + 4 新例），旧 38 例 Shell 与 G4a 历史 56/94 不复算不冒称
  - 验收（本批实跑，证据与 G4b 接线接口见 `ui-verification/matrix-day-others-g4a.md`）：`cd front && pnpm test:unit app/features/tags/components/console/useConsoleReading.test.ts app/features/tags/components/console/otherTopicSelection.test.ts app/features/tags/components/console/otherTopicSelection.select.test.ts app/features/tags/components/console/useConsoleReturnContext.test.ts app/features/tags/components/console/consoleReturnContext.test.ts app/features/tags/components/console/useConsoleRefresh.test.ts --maxWorkers=2` → **6 files / 56 tests passed**（11/7/8/13/13/4）；相邻旧 `BoardConsoleShell.test.ts` → **1 file / 38 tests passed**；合并 **7 files / 94 tests passed**；本组 12 个 `.ts` `pnpm exec eslint` exit 0；全部 source/test/harness ≤500 行且 ≤15360B（`useConsoleReturnContext.ts` 15301B 最接近）。接口：`useConsoleReading`（单份 reading+observation、`bindSelectedDate`、`retryReport` force 索引→force 当日→稳定 section 核对、`refreshAfterRebuild` invalidate/按当前 topic 重读）；`otherTopicSelection`（`selectedOther`/`othersOpen` 首份成功 A 仅一次、`otherTopics`=G1 buildOtherTopics、`selectDayHeader`/`selectOther`/`selectObservedDay` 合法性 guard 与清场）；`consoleReturnContext` 可选 `other`/`othersOpen` + `otherReadingKey`（旧六字段形态不变）；`useConsoleReturnContext` 阅读 key 分离 + settled/滚动按 key+date 匹配 + 显式 lane 优先/同 lane 往返恢复；`useConsoleRefresh.after` 防重窗口内附属刷新。未跑全量/typecheck/lint/build/浏览器/Go/部署，未改任何 `.vue`/G1–G3/后端；不代表 G4b 接线或真实 UI 验收
- [x] 8C.5B G4b 真实壳层/舞台接线：在G4a接口上接Shell→Body→Stage→Grid/Narrow/OtherReading、LaneChronicle注入同一store，补必要重试意图透传与零泳道轴/日期锚；Shell/大测试按规范内聚拆；完整目标回归通过后勾下方8C.5，不以仅控制器绿称界面完成
  - 实跑验收（2026-10-29，报告 `ui-verification/matrix-day-others-g4b.md`）：构造顺序 consoleState → useConsoleReading（1 reading + 1 observation）→ useConsoleSelection（注入 reportDates 三态）→ bindSelectedDate → useOtherTopicSelection → useConsoleReturnContext（selectedOther/othersOpen）→ useConsoleRefresh.after；Shell→Body→Stage→Grid/Narrow 完整透传 reportDates/索引态/otherTopics/othersOpen/selectedOther 与 selectDayHeader/toggleOthers/selectOther/selectObservedDay/retryReport/retryReportChain；LaneChronicle 可选 `reading` 注入共享实例（未注入保旧自建，28 例仍绿）；单一阅读槽（selectedOther → OtherTopicReading 含观察带，否则原编年，其他阅读隐藏旧演进并抑制泳道高亮，0 正式给方向提示）；点已选泳道/普通格显式 `clearOther`；重试链 force 索引 → force 当日报告（Stage 新增 `matrix-report-chain-notice` 最小入口，不只刷 A）；生成完成 meta→A→after 单窗口内强刷索引/当日报告 + observer 懒失效 + lane 编年按刷新后缓存非 force 重建；内部归档→矩阵同实例返回（滚动/其他阅读槽恢复）。内聚拆分：`useConsoleReadingWiring`/`useConsoleShellOverlays`/`useConsoleShellRail`/`useConsoleShellActions`/`consoleStageBinding`/`laneChronicleMetrics`/`consoleReturnScroll`，Shell 493/15045B → 444/14486B、LaneChronicle 399/14947B → 389/14597B、返回 hook 412/15301B → 264/10244B + scroll helper 232/8847B，全部 ≤500 行/15360B。旧 Shell 38 例拆为 4 个子套件 + harness/fixtures：拆前/拆后 Vitest JSON `fullName+status` 多重集一致（38 passed / 38 passed，缺例 0、新增 0、skip 0）；新增集成用例 9 例先在断开阅读接线状态真红 9/9，恢复后全绿。目标批次 **24 files / 265 tests passed**（含 G2/G3/LaneChronicle/useReturn 相邻回归），本组 21 个 `.ts` eslint exit 0（0 error 0 warning；Vue 不被 eslint 覆盖不称 Vue lint 绿）。未跑全量/typecheck/build/浏览器/Go/SQL/部署/commit；真实像素/触摸/服务端生成时序仍归 8C.7
- [x] 8C.5 G4 壳层接线串行收口（独占：`BoardConsoleShell.vue`、`BoardConsoleStage.vue`、`consoleStageTypes.ts`、`LaneChronicle.vue`（可选 reading 注入）+ `BoardConsoleShell.test.ts`）：读取 store 上提单实例并注入编年/其他面板；`selectedOther`/`othersOpen`（首份 A 响应初始化一次、切板块复位）与单一阅读槽及清场规则；列头/组事件接线；生成完成后强制刷新报日索引；索引失败提示+重试；兼容既有四参深链与返回载体，补稳定其他分区board/report/section/date返回恢复，不新增date= query
  - 验收（2026-10-29 实跑，证据 `ui-verification/matrix-day-others-g4b.md`）：`cd front && pnpm test:unit <5 个 Shell 分组+集成用例 + 19 个相邻目标文件> --maxWorkers=2` → **24 files / 265 tests passed**（Shell 旧 38 例拆 4 文件 10/7/16/5 全绿，集成 9 例全绿）；本组 21 个 `.ts` `pnpm exec eslint` exit 0；全部本包 source/test/helper ≤500 行且 ≤15360B（最大 `BoardConsoleShellNavigation.test.ts` 15294B / `LaneChronicle.vue` 14597B / `BoardConsoleShell.vue` 14486B）。**`pnpm exec nuxi typecheck` 与 `pnpm lint` 未跑**（按 brief 归父 8C.6 集中批）；真实浏览器 1440/1920/375 双主题留 8C.7，不以组件测试冒充；他域 `SignalReportView.test.ts` 旧债不洗绿
- [x] 8C.6 集成与静态门禁（父集中）：`cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit <本增量全部受影响文件...> --maxWorkers=2 `（不与其它重负载并行；不另跑pnpm build；8C.8的单次静态generate同时承担构建验证）；不改后端不跑 Go
  - 实跑验收（2026-10-29/30 父集中批，类型修复报告 `ui-verification/matrix-day-others-8c6-typecheck-fix.md`）：`pnpm lint` → exit 0（0 error，8 warning 均他域既有）；`pnpm exec nuxi typecheck` → 本 change 15 条类型错误全消（子修+父两次复跑确认），仅剩他域 `SignalReportView.test.ts` 7 条旧债原样（不洗绿，另行处理）；`pnpm test:unit <35 个受影响文件并集：G4b 24 + 8C.4S 拆分落点> --maxWorkers=2` → **35 files / 361 tests passed**（exit 0，0 failed/0 skipped）
  - 父裁决（独立评审 §五.2）：`otherTopicSelection` 的 `reportDates` 维持接线侧直传（与声明侧收宽等价）；L-1 恒真条件（useConsoleReading.ts:150-155）留后续小修，见 human-feedback.md
- [x] 8C.7 真实 UI 验收 + 独立 review + 部署影响说明：静态 `:5100`（后端零改动不重启）跑 `test-cases-matrix-day-others.md` §真实浏览器（S11-A冷启动246030 / S11-B成熟不回归1974 / S11-C跨日观察与返回；1440×900、1920×1080、2560×1440、375×667、390×844；双主题；机械断言+只读保护，既有文章状态动作真写能力用组件测试覆盖、浏览器业务POST拦截不碰真库），证据落 `ui-verification/matrix-day-others-<date>/report.md`；独立 review 建账（High/Medium 修复回写，未完成项进 `human-feedback.md`），不以 UI 批准代替 review，不洗绿根账 53/1/2
  - 实跑验收（2026-10-29/30）：①旧产物 `1cca5ffc…` 上 S11-A/B/C 机械断言全绿（`ui-verification/matrix-day-others-2026-10-29/report.md`，32 图，5 视口×双主题，零业务写：3616 GET+1 既有埋点）；②Medium/Low 文案项修复回写（`matrix-day-others-8c7-m1-fix.md`：删 0 正式旧横幅+窄屏改「0 正式泳道」，7 文件 61 例两遍一致，父抽验 22 例绿）；③独立评审 `review-matrix-day-others.md`（change-review-glm，无 High，1 Medium=产物未含修复→已由④收口，Low/L 留账 human-feedback）；④新产物 `2ba136527001200c…`/入口 `BBjtIkfj.js` 复验 11/11 全绿（`reverify-after-m1-fix.md`，12 图：指纹一致、M1 横幅 DOM 不存在且内联说明唯一、L1 文案逐字、S11-A 主链、1974 冒烟、零业务写）
- [x] 8C.8 安全候选发布与最终留盘（父执行）：源码独立review/目标测试先通过，核当前静态基线、备份、构建源指纹与foreign前端改动归属；存在未获部署许可foreign UI或构建漂移先停协调。`bash scripts/dev/deploy-frontend.sh --no-restart`单次generate构建并铺盘，后端零改动不重启，不重复pnpm build；以新index/chunk/hash在:5100跑8C.7，不得验旧产物，失败回滚备份，通过才留盘与健康检查；向用户汇报可见变化（0 正式泳道可按报日读分区、成熟板块多一个默认收起的其他组、日期列头同源选日、窄屏报日条）与「无迁移/无重生成/无回填/无配置、后端零改动」，并说明旧日报缺历史状态字段的降级文案
  - 实跑验收（2026-10-29/30）：并发线归属核查（两线静止+其前端目标测试 9 文件/159 例绿+board-health 数据面通+auxrepair 端点 404 属他线）→ 用户裁决「先核查再决定」→「现在部署候选」；a35 基线备份 `/home/zanebono/software/deploy-backups/frontend-a35-2026-10-29-15-35-.tar.gz`（38MB）；两次单次 generate 铺盘（候选 `1cca5ffc…` → M1 修复后最终 `2ba136527001200c…`/入口 `BBjtIkfj.js`），后端零改动未重启（PID 16270），均在新 hash 上验收、未验旧产物；最终产物复验 11/11 全绿 → 留盘（不回滚）；旧日报缺 `persistent_topic_id`/快照状态字段的分区：观察带不显示节点、状态词降级不猜，缺身份不冒 0（真库当前 0 样本，组件层覆盖，见 human-feedback C4）

## 8D. 方向 A 改版：「当日其他话题」→「当日分区面板」（2026-10-30；UI 批准 pending）

> 触发：用户 2026-10-30 实页反馈「当日其他话题的 UI 我不是很满意」，选项裁定**方向 A**（退出跨日矩阵，改成选中日的当日分区面板），落地方式「先只出原型，不碰产品代码」。
> 合同：[implementation-day-partition-panel.md](implementation-day-partition-panel.md)；设计段：`ui-design.md` §8D。
> 与 8C 的关系：8C 数据口径/排除口径/状态快照/返回契约**一律不变**，只改呈现结构；8C 已实现但**尚未归档**，本阶段在归档前改写其呈现契约，故并入本 change（主 spec 尚未生成，单独开 change 无基线可对）。

- [x] 8D.1 改写 delta spec：`specs/workshop-board-console/spec.md` 的「当日其他话题折叠组与就地下读」→ 「当日分区面板与就地下读（方向 A）」；同步场景（面板位置与默认态 / 分区不占跨日矩阵行 / 成员来自所选日报 / 当时状态不篡改历史 / 就地下读完整标题与摘要 / 0 行与无详情区分 / 只读零写 / 同归属跨日观察带 / 点观察节点切日 / 跨日观察缺身份 / 零休眠仍保留组标题 / 打开日报后返回恢复 / **视觉降噪不压过内容**（新增））
- [x] 8D.2 写实施合同 `implementation-day-partition-panel.md`：问题清单 P1–P9、目标结构、字段与口径（不变）、状态矩阵、交互契约、视觉降噪 9 条、验收断言 A1–A10、边界
- [x] 8D.3 写 `ui-design.md` §8D 八节（User Journey / IA / Interaction / State Matrix / Layout / Component Reuse / Prototype / Acceptance）+ 头部加「2026-10-30 改版声明：8C 该增量呈现结构被 8D 取代、UI 批准重置 pending」
- [x] 8D.4 human-feedback 建账：`human-feedback.md` FB-11（用户原话 + 裁定 + 边界）
- [x] 8D.5 出离线原型（子线程）：`ui-prototype/revision-2026-10-30-day-partition-panel/standalone.html`（689,584B，SHA256 `342fcc56d3ec4c4094cb644ca573c420a239f9b4e7be04d6796b4c547ac189e1`），复用冻结矩阵稿的 CSS/模块与**真实冻结样本**（246030 / 1974），未编造数据；构建脚本自带的冻结 hash / 模块体积 / 离线校验全过；**未碰产品代码、未碰冻结稿**
  - 实跑（子线程 + 父复核）：`node build-standalone.mjs` → `build-result.json` 全过；`node verify-prototype.mjs` → `verification.json` A1–A10 全 pass（43 个真实日报逐日对成员/顺序/原题）；15 张真浏览器截图在 `screens/`；父线程另实测点按（选日/展开分区/点跨日节点切日），见 `ui-verification/day-partition-panel-2026-10-30/report.md`
- [x] 8D.6 父审查原型（主线程）：agent-browser 真实复验 1440×900 / 1920×1080 / 375×812 × 明暗双主题 × 0 泳道板（246030）/ 成熟板（1974），逐条核对 A1–A10；证据落 `ui-verification/day-partition-panel-2026-10-30/report.md`（含 5 张入库截图与冻结基线 hash 复验）
  - 实跑（父线程独立复核，非子线程自证）：A1 全文无「当日其他话题」渲染；A5 实点展开后面板文本含「同归属其它日 · 6 个记录日 · 点径为线索峰值」且逐日 `—`/数字区分；A6 `details.open === false`；点跨日节点 10/13 原子切日且面板换成该日真实分区；冻结五稿 hash 未变；`front/` 无本阶段改动
- 8D.7–8D.11 已被用户否决的退出矩阵方案取代，不再实施或等待其批准；旧稿与旧原型仅留历史，不记已完成。

## 8E. 矩阵内复用视觉与关联日黄点（minor，用户已批准）

- [x] 8E.1 先更新 ui-design / implementation-day-partition-panel / delta spec / human-feedback 及用例映射；旧 8D 非当前合同
- [x] 8E.2 实现同款矩阵行、同 ID 可见日黄点标题与原子导航，普通日期语义不变
- [x] 8E.3 去阅读区重复日期带、保留状态/重试/缺 ID 提示，复用泳道正文视觉，移除冷启动无效选泳道卡
- [x] 8E.4 定向单测、change-scope、lint、typecheck（CPU 串行），更新 reference
- [x] 8E.5 主线程独立 review + 浏览器亮暗/宽窄验收后协调 build/部署：只读review的 R-41 High 已复现并修复；父核阅4个选择/接线文件，真实卫生健康桌面连续黄点跨日、窄屏跨日、普通日期清场、hover标题与成熟板矩阵对照已验，1440浅色/1920深色/375深色无页面横向溢出；`bash scripts/dev/deploy-frontend.sh` 修复后再次成功（generate + 静态铺盘 + 后端健康200），最终新构建复验10/16→10/22→10/23保留目标通过。证据 `ui-verification/matrix-date-markers-2026-10-30/report.md`；不表示整change归档门禁或历史欠账通过。
- [x] 8E.6 独立 review 的 High（review-matrix-date-markers.md R-41）：报日索引缺该日时黄点点击被选中日普通回落静默撤销 → 先补复现测试（红）再修（绿）：新增「已验证记录日」（当前观察目标在观察带里的真实同 ID 记录日）作为比报日索引更强的第二证据源，只保护当前这一个目标；普通日期/缩窗排出/切板块/切泳道/手动清选择的回落语义不变

## 8F. 总览报头「主页」入口归位到操作区（minor，用户已确认 2026-10-30）

- [x] 8F.1 用例先行：新增 `BoardRadarMasthead.test.ts`（位置紧邻治理 / 标题区不再挂入口 / 入口仍指向 `/` 且有文案与 aria / 与治理共用 `rd-action-btn` 档 / 同组成员序 / 源码样式锚共 6 例）
- [x] 8F.2 实现：`BoardRadarMasthead.vue` 把主页入口从标题簇移到右上操作区（治理菜单左侧），抽出 `rd-action-btn` 共享档（min-height 2rem / 0.7rem 字 / border-medium / hover 变 accent），治理菜单 `summary` 改用同档；去掉窄屏隐藏「主页」文字的旧规则（入口不再挤在标题旁）
- [x] 8F.3 定向单测 + lint + typecheck（`BoardRadarMasthead.test.ts` 6 例、`BoardRadarView.test.ts` 20 例、typecheck 退出 0）
- [x] 8F.4 独立 review 建账（review.md R-42，Medium×3 + Low×6 已修，L8 存量留账）+ 浏览器实页四档验收，证据 `ui-verification/radar-home-8f/report.md`（A-8F-1…8 + 5 张截图）
- [x] 8F.5 部署前端静态产物并汇报部署后影响/用户操作/旧数据降级

## 8G. 归档视图重做（major，UI 批准 pending 2026-10-31；修复 2026-10-25 版四类问题）

> 背景：用户复盘归档现状——三代风格混杂（手写骨架+旧 MatchDetailPanel 掉出视口+原生控件）、行内标签四色与最强匹配丢失、组内质量排序被 delta 一刀切、主点击偏离旧版习惯。合同 `ui-design-archive-rework.md`，原型 `ui-prototype/revision-2026-10-31-archive-rework/standalone.html`（163 篇真实冻结样本 + 149 标签真实 description 回填）。**未获用户批准原型前不写产品代码（ui-design-gate 硬拦截）**；已批总览/矩阵不重绘。
>
> v2 修订（2026-10-31 用户反馈）：右侧证据工作面内容意味不明 → 移除；行主点击 = 文章预览弹窗（复用 ArticlePreviewModal）；标签 chip → 锚定 popover（标签描述为核心展开内容 + 本次匹配 + 计算明细就地展开）；composite_hit 紫色已拍板。
>
> v10 修订（2026-10-31 用户第十轮·高保真）：类别如实标注（事件/关键词/人物，描述区自适应）；组合命中展示真实推导组合链（挂齐组件即挂载）；keyword 与事件结构同构同展示。
>
> v9 修订（2026-10-31 用户第九轮）：入板过程空阶段展示未命中原因（真实差距数值或真实阈值+未达结论）；命中步之后阶段标「未走到（命中即停）」弱化区分。
>
> v8 修订（2026-10-31 用户第八轮）：判定面板退役；入板 chip 点击=锚定 popover（与事件卡同形态，纯数据五阶段得分+入板依据+本文标签，零说明文字）。
>
> v7 修订（2026-10-31 用户第七轮澄清）：入板 chip 点击打开「本文入板过程·各阶段得分」判定面板（filtered_tags 按命中阶段分组 + 入板依据标记 + 空阶段如实）；行尾判定钮退役（行回四列）。
>
> v6 修订（2026-10-31 用户第六轮澄清）：「匹配流程」做成行内 chip——每文章标签带末尾「入板④相似度」（步号=最强判定、虚线边、点开通用流程）；158/163 行有 chip（无标签不显示）。
>
> v5 修订（2026-10-31 用户第五轮）：匹配流程独立为工具行通用入口（dialog：⓪-⑤每步做什么/真实阈值/命中即停/方向校验）；事件卡 popover 只留私有判定行（第 N 步即命中 + 指向通用流程）。
>
> v4 修订（2026-10-31 用户第四轮）：判定钮独立列修越界；popover/面板去卡片堆改栏线；判定叙事改匹配流程瀑布（旧版 flowSteps：⓪-⑤命中即停，阈值全局真实、数值按需实时）；判定面板叙事化（第 N 步即命中）。
>
> v3 修订（2026-10-31 用户第三轮）：层级语义修正（match-detail 实为事件×版块维度，hits=事件辅助逐个对版块池最相似 sim≥阈值）；事件卡 popover 三层（本文标签 / 事件辅助×版块收编 ✓✗ + M/N / 规则代入式藏展开）；新增文章判定面板（行尾「判定」→ 右侧：文章级公式 + 构成标签汇总 + 本文全部标签）；冻结层 tag-hierarchy.json（事件辅助全集/文章全标签/版块辅助池）。

- [x] 8G.0 原型与合同：八节合同 + standalone 原型 v2（四色标签/最强匹配/组内质量↔时间切换/文章预览弹窗占位/锚定标签卡 popover（描述+计算明细就地展开）/筛选覆盖面/空态/双主题/375 线性化）+ 冻结样本（articles-1974-7d.json 含 description 回填、match-detail-238319.json）+ agent-browser 静态走查 v2（分组 6×163、五色、排序/筛选/空态、popover 锚定视口内、238319 真实明细、文章弹窗、暗色、窄屏底部档无溢出）
- [x] 8G.1 delta 修订：「归档排序」改组间日期倒序+组内默认质量可切时间（新增 2 Scenario）；「按日期平铺」补四色编码/最强匹配/主点击文章弹窗（新增 2 Scenario）；新增「标签卡 popover」requirement（描述+就地明细+历史缺失如实标注，新增 3 Scenario）；board-article-api delta 补 filtered_tags.description 字段（新增 1 Scenario）；validate --strict 通过
- [x] 8G.2 用例先行（v10 批准 2026-10-31）：BoardArchiveView.test 16 例 + ArchiveMatchPopover.test 18 例（两模式/辅助✓✗/组合链/五阶段 whyMiss/未走到/Esc 点外/竞态）+ useBoardArchive.test 扩展 + matchQuality.test
- [x] 8G.3 实现（8G.3a/b/c 三段）：后端 description 字段+测试（反向验证过）；composite 紫 token/matchQuality 扩展（stepNoOf/stepShortOf/catLabel）/getTagAuxiliaryLabels；useBoardArchive 组内排序+Map 聚合分组；BoardArchiveView 重绘（四列行/四色 chip/入板 chip/最强匹配/主点击/筛选覆盖面/匹配流程 dialog）；ArchiveMatchPopover（标签卡+入板两模式，按需实时拉取 getArticle/getTagAuxiliaryLabels/getMatchDetail）；ArticleTag.description 类型透传；旧证据抽屉与 MatchDetailPanel 挂载移除（组件本体保留给旧页面）
- [x] 8G.4 验证：前端 60+53 测试过（BoardArchiveView/ArchiveMatchPopover/useBoardArchive/matchQuality + Shell 集成）；后端 tagmanagement 全绿（handler 49 用例）+ golangci-lint 0 issues；nuxi typecheck 全量绿；eslint 0。遗留红 iconify-subset（非本 change 引起，已 test-patrol 登记，本段贡献 -2）
- [x] 8G.5 浏览器实页验收（2026-10-31，agent-browser localhost:5100 实页）：归档视图 17 行×入板 chip×最强匹配；入板过程 popover（⓪✗①✗交集1<2 ③✗R=0.400≤0.50 ④✓入板依据1.00 ⑤未走到——真实差距实时拉取）；标签卡 popover（事件类别/真实描述/本文标签/辅助×收编2/5逐对最相似对/私有判定行+真实代入式）；主点击=ArticlePreviewModal；排序切换；匹配流程 dialog 六步真实阈值；暗色主题；375 窄屏两列无溢出。独立只读 review 归档前补（见 11 节门禁）
- [x] 8G.6 部署（2026-10-31 deploy-frontend.sh：构建+铺盘+后端重启+健康 200）；部署后影响见 change 会话汇报——归档视图重绘上线、filtered_tags 新增 description 字段（纯增量）、旧数据无迁移无降级、用户无需操作

## 8G.7 review 修复段（review-archive-rework-8g.md，R-01~R-05/R-10 全采纳）

- [x] 8G.7a 后端 R-02：`boardArticleQualitySortJoin`（LEFT JOIN 子查询 + ROW_NUMBER 每文最优 tier/score，复刻 MatchTier 口径含降级/方向排除，无证据落最差档）先全局 ORDER BY 再分页；time 序 SQL 既有全局；delta 补「质量序跨分页稳定」requirement+Scenario；R-10 断言 `TestBoardArticles_QualitySortGlobalOrder`（per_page=3 三页拼接全局序 + weighted 0.95 不压 direct 0.8 + time 全局序）。tagmanagement 全绿、lint 0、validate 157。（子线程 429 中断于测试前，主线程补齐收口；不静默换模型，主线程直接完成）
- [x] 8G.7b 前端（8G.7b 子线程完成）：R-01 两处 strongestTag→filtered_tags[0]（测试用 [0]=direct_hit 0.62≠最高分 weighted 0.91 锁口径）；R-03 头区「仅弱证据」+徽标「弱证据命中（不计入强证据）」；R-04 三处口径句；R-05 LIVE_CALC_NOTE+scoreDriftNote（hit_rate/weighted 差>0.001 显式⚠）；R-10 乱序不重排断言。55 测试过、typecheck 全量绿、eslint 0
- [x] 8G.7c 复验部署（2026-10-31）：deploy-frontend.sh 重新部署；R-02 API 实证 7 天窗 163 篇 page1(100, tier 0→3 递增)/page2(63, 全 tier3) 跨页边界稳定全局序成立；R-01 实页 id=155448（max_sim 1.0 高分在场）正确显示「直接命中 0.70/入板①精确」；默认窗口 17 行入板分布 tier 序合理。review 两个 High 均实证修复

## 9. 测试

- [x] 9.1 组件单测：`BoardMatrixView`（格数/断档/休眠折叠/今天列）、`BoardArchiveView`（日期筛选/平铺文章/筛选/证据文案）、`BoardRadarView`（排序/常温折叠/升温阈值）、`BoardComposeOverlay`（采纳预勾/确认启用禁用/质量指标）、`SectionDrawer`（空格提示）
- [x] 9.2 路由与状态：`?lane=` 恢复与非法回退、深链 `?report=&annotation=` 映射、返回矩阵恢复选中泳道
- [x] 9.3 阅读台回归：页边注、watch 定位、lifeline 缓存、Esc 返回（以既有用例为基线迁移到新宿主）（2026-11-02 盘点补齐：页边注=`useMarginNotes.test`（9 例拉取/落锚/提问/删除）+`useMarginNoteSelection.test`（划选 guard 5 例）+深链=`tags-deep-link.test`；watch 定位=`DailyReportWatchIndex.test`（click→locate）；lifeline 缓存=`useDailyReportReader.test`「deduplicates lifeline and article requests」；Esc 返回=**新补**`DailyReportReader.test`「Esc 关闭阅读层；有浮层开着时 Esc 让位」——窄屏页边注抽屉与浮层走同一支 guard，窄屏分支由真机 ui-verification 兑底；组件级用例不依赖旧宿主，8.4 删除后无断链）
- [x] 9.4 后端单测：overview 端点（零值/超时降级）、articles `in_report` 与 `report_status` 过滤（2026-11-02 执行：新建 `board_radar_overview_handler_test.go` 三用例——空版块空数组/无数据零值行不 500、最新日报 brief 挂行；`board_radar_repository_test.go` 补 date 回退今天；articles 过滤经核对 `board_article_handler_test.go` 已覆盖 in_report/not_in_report/weak/别名/非法参数 400 无需重写；**测试抓出真 bug**：空版块 early return 绕过 finalizeRadar 致顶层 date 空串，已修；5s 迟到行降级无法单测稳定复现，由真机 ui-verification 兑底，测试文件头已留痕）
- [x] 9.5 更新因 tab 结构变化而失效的既有用例（BoardHealthPanel / TagsPage / BoardThreadBrowser 宿主断言）（随 8.4 一并处置：退役组件连测试删除；`BoardSourcePanel.test.ts` 旧「TagsPage 装配层」段删除，新宿主断言已由 `console/BoardGovernanceDrawer.test.ts`（board-source-panel 渲染+行数）与 `console/BoardWideWorkface.test.ts` 承接）
- [x] 9.6 阅读台文章预览状态同步回归：收藏点击→乐观翻转→接口失败回滚 + 错误提示；连点不丢更新（`DailyReportDeskShell.test.ts` + `useArticleFavoriteToggle.test.ts`）
- [x] 9.7 弹窗内全屏层级回归：全屏层 z-index > 弹窗遮罩且 < toast（`ArticlePreviewModal.test.ts`）+ 正文组件不得回迁硬编码 `z-[100]` 的源码锚（`ArticleContentView.test.ts`）

## 10. 文档

<!-- doc-impact: flow, api, database, standard -->

- [x] 10.1 `docs/reference/flow/semantic-board.md`：视图入口（雷达/矩阵/归档/编排）、治理入口分工、休眠泳道、全局参数收回「更多」菜单
- [x] 10.2 `docs/reference/flow/daily-report.md`：阅读台路由化 + 旧版三栏恢复（大刊头/今日重点/左目录/宽屏正文/右页边注）、深链
- [x] 10.3 `docs/reference/flow/topic-graph.md`：编排态（画布/候选/关联推荐/质量指标）与矩阵关系
- [x] 10.4 `docs/reference/standard/frontend/layout.md`：固定视口栅格例外 + 窄屏线性化规则
- [x] 10.5 `docs/reference/api/`：overview 端点与 articles `in_report`/`report_status` 契约
- [x] 10.6 归档前跑 `bash scripts/harness/doc-impact.sh verify` 与 `bash scripts/harness/check-standards.sh` 对账

## 11. 验证

### 8C新增Scenario落点（计划映射，不等于测试已实现/通过）

| Scenario | 测试文件 |
| --- | --- |
| 零正式泳道保留日期轴 | front/app/features/tags/components/console/ObservationMatrix.test.ts, front/app/features/tags/components/console/ObservationMatrix.dayHeaders.test.ts |
| 列头与日格共用同一选中日 | front/app/features/tags/components/console/BoardConsoleShell.test.ts, front/app/features/tags/components/console/BoardConsoleShell.integration.test.ts |
| 范围排出回窗内最新报日 | front/app/features/tags/components/console/laneConsoleView.test.ts |
| 无报日诚实空态 | front/app/features/tags/components/console/ObservationMatrix.test.ts |
| 报日索引失败不误报 | front/app/features/tags/composables/useLaneDayReading.reportIndex.test.ts, front/app/features/tags/components/console/ObservationMatrix.dayHeaders.test.ts |
| 组位置与默认态 | front/app/features/tags/components/console/ObservationMatrix.groups.test.ts |
| 成员来自所选日报 | front/app/features/tags/components/console/dayOtherTopics.test.ts |
| 只标所选日 | front/app/features/tags/components/console/DayOtherTopicsGroup.test.ts |
| 当时状态不篡改历史 | front/app/features/tags/components/console/dayOtherTopics.test.ts |
| 就地下读完整标题与摘要 | front/app/features/tags/components/console/OtherTopicReading.test.ts |
| 0 行与无详情区分 | front/app/features/tags/components/console/dayOtherTopics.test.ts |
| 只读零写 | front/app/features/tags/components/console/BoardConsoleShell.test.ts + BoardConsoleShellNavigation.test.ts（8C.5B 拆分后原例所在文件） |
| 同归属跨日观察带只列本窗真实记录日 | front/app/features/tags/composables/useOtherTopicObservation.test.ts |
| 点观察节点切换同一选中日并读取真实分区 | front/app/features/tags/components/console/BoardConsoleShell.integration.test.ts（+ 控制器 otherTopicSelection.select.test.ts；8C.5B 实跑） |
| 跨日观察缺身份或读取失败不借内容 | front/app/features/tags/composables/useOtherTopicObservation.test.ts |
| 零休眠仍保留同级组标题 | front/app/features/tags/components/console/ObservationMatrix.groups.test.ts |
| 其他话题打开日报后返回恢复原阅读 | front/app/features/tags/components/console/BoardConsoleShellNavigation.test.ts（路由往返）+ BoardConsoleShell.integration.test.ts（内部归档同实例返回）+ useConsoleReturnContext.test.ts |
| 零泳道仍按报日读分区 | front/app/features/tags/composables/useLaneDayReading.reportDay.test.ts |
| 报日索引不改聚合口径 | front/app/features/tags/components/console/laneConsoleView.test.ts |

以上是本增量19个Scenario的实现计划；故事入口为test-cases.md/S11-A/B/C。其他103条旧Scenario仍需按既有资产补全，不因本表视为通过；新增文件在对应代码包落地后再验存在/执行，真实浏览器结果另行登记。

- [ ] 11.1 后端：`cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...`，期望全部通过无输出错误
- [ ] 11.2 后端影响包：`cd backend-go && go test ./internal/tagmanagement/... ./internal/topicgraph/...`，期望 PASS
- [x] 11.3 前端静态检查：`cd front && pnpm lint`，期望无 error
- [x] 11.4 前端类型：`cd front && pnpm exec nuxi typecheck`，期望无类型错误
- [x] 11.5 受影响单测：`cd front && pnpm test:unit <受影响文件...> --maxWorkers=2`，期望全部通过（参数不带 `--`）
- [x] 11.6 构建：`cd front && pnpm build`，期望构建成功
- [x] 11.7 双视口验收：agent-browser 在 1440×900 与 1920×1080 跑主链路 A-1…A-8，证据写 `ui-verification/report.md`，期望 ≥1440 无页面级纵向滚动
- [x] 11.8 窄屏基线：375×667 打开雷达 / 版块摘要 / 阅读台，期望无横向溢出且编排入口给出宽屏提示
- [x] 11.9 归档门禁：`bash scripts/harness/ui-verification-trace.sh` 与 `openspec validate rework-workshop-into-radar-and-desk --strict`，期望通过

### 8E Scenario → 测试映射

见 [test-cases-matrix-date-markers.md](test-cases-matrix-date-markers.md)。本轮精确映射（旧 8C 表保留历史欠账，不据本轮通过洗绿）：

| Scenario | 测试文件 |
| --- | --- |
| 矩阵其他组复用泳道视觉且只绘当日格 | front/app/features/tags/components/console/DayOtherTopicsGroup.test.ts |
| 同 ID 关联日黄点与真实标题 | front/app/features/tags/components/console/ObservationMatrix.dayHeaders.test.ts |
| 黄点点击原子保留阅读目标且普通日期不变 | front/app/features/tags/components/console/BoardConsoleShell.integration.test.ts |
| 换选清点与观察诚实降级 | front/app/features/tags/components/console/OtherTopicReading.observation.test.ts |
| 阅读区去重并复用泳道正文视觉 | front/app/features/tags/components/console/OtherTopicReading.test.ts |
| 零正式泳道仅保留有用阅读方向 | front/app/features/tags/components/console/BoardConsoleShell.integration.test.ts |
| 历史成员与返回上下文不回归 | front/app/features/tags/components/console/BoardConsoleShellNavigation.test.ts |
| 缺索引观察日黄点点击不被回落撤销（R41） | front/app/features/tags/components/console/BoardConsoleShell.integration.test.ts + laneConsoleView.test.ts + useConsoleSelection.test.ts + otherTopicSelection.select.test.ts |

定向命令及期望：
- `cd front && pnpm test:unit ObservationMatrix.dayHeaders.test.ts ObservationMatrix.groups.test.ts ObservationMatrixGrid.test.ts DayOtherTopicsGroup.test.ts useOtherTopicObservation.test.ts OtherTopicReading.observation.test.ts OtherTopicReading.test.ts OtherTopicReading.signals.test.ts BoardConsoleShell.integration.test.ts BoardConsoleShellSelection.test.ts BoardConsoleShellStates.test.ts BoardConsoleShellNavigation.test.ts otherTopicSelection.select.test.ts --maxWorkers=2` → 指定套件通过（不跑全量）。
- `bash scripts/harness/change-scope.sh` → 显示本次前端影响（全树并发噪音单独说明）。
- `cd front && pnpm lint && pnpm exec nuxi typecheck` → 无本增量错误。

### 8F Scenario → 测试映射

| Scenario | 测试文件 |
| --- | --- |
| 返回主页入口与全局治理同组 | front/app/features/tags/components/console/BoardRadarMasthead.test.ts |

定向命令及期望：
- `cd front && pnpm test:unit app/features/tags/components/console/BoardRadarMasthead.test.ts app/features/tags/components/console/BoardRadarView.test.ts --maxWorkers=2` → 指定套件通过（不跑全量）。
- `cd front && pnpm exec nuxi typecheck` → 无本增量类型错误（退出 0）。

### 8F 本轮验证结果（2026-10-30，不等于整 change 验收）

- 用例先行红态（实现前实测）：`BoardRadarMasthead.test.ts` 4 例中 **3 红 1 绿**，红在「不在操作区」「标题区仍挂入口」「未共用操作按钮档」三处；绿的是「入口仍指向 `/` 且有文案与 aria」（改动前后都成立）。
- `cd front && pnpm test:unit app/features/tags/components/console/BoardRadarMasthead.test.ts app/features/tags/components/console/BoardRadarView.test.ts --maxWorkers=2` → **2 文件 / 26 用例通过**（含 review 补的同组成员序与源码样式锚 2 例）。
- `cd front && pnpm exec eslint app/features/tags/components/console/BoardRadarMasthead.test.ts` → 退出 0、0 error。
  **覆盖口径**：仓库 eslint 配置未含 `**/*.vue`，`.vue` 的模板与 scoped CSS 对 lint **天然失明**（实测该 .vue 判 "File ignored because no matching configuration was supplied"、退出 0）→ 档位口径由源码样式锚用例 + 实页几何双重承保，不以「lint 过了」冒充模板/CSS 过了。
- `cd front && pnpm exec nuxi typecheck` → 退出 0、无类型错误（工具链 DEP0205 deprecation warning 非 TS 错误）。
- `bash scripts/harness/check-tasks-tail.sh openspec/changes/rework-workshop-into-radar-and-desk` → 通过。
- `bash scripts/harness/scenario-trace.sh …` → 118 个待对账 Scenario / 109 项失败（较本轮前 117/109 **未新增缺口**；本轮 Scenario 映射缺口为 0，失败项均为该 change 历史欠账，不因本轮洗绿）。
- `bash scripts/harness/ui-verification-trace.sh …` → 通过（report.md 建账 56 条，点名截图全部在盘）。
- `bash scripts/harness/check-standards.sh --change rework-workshop-into-radar-and-desk` → 通过 253 / 失败 0。
- `bash scripts/dev/deploy-frontend.sh` → 构建（pnpm generate）→ 铺 `backend-go/frontend/` → 重启后端 → `/health` 200；实页四档（1440/1920/375/390）+ 亮暗双主题几何与档位实测全对，见 `ui-verification/radar-home-8f/report.md`。
- 独立只读 review（子线程）：`review.md` R-42（Medium×3 已修：样式锚 / 本轮验证账面 / 四档实页；Low×6 已修；L8 `.rd-more-menu` z-index 属存量留账给整 change 归档时登记）。
- 未触真库、未改后端/接口/数据口径、未 commit、未归档。


### 8E 本轮验证结果（2026-10-30，不等于整 change 验收）

- 上列定向 Vitest 最终 **13 文件 / 116 用例通过**（12.92s）；包含 hover/focus 浮到 body、同日多题单点、首分区导航、缺 ID/清选择清点、切回命中缓存、窄屏同源导航、阅读区无重复日期条及共享行样式机械锚。
- `cd front && pnpm lint` → 退出 0、0 error、8 条其他文件既有 warning；`cd front && pnpm exec nuxi typecheck` → 退出 0、无类型错误，工具链 DEP0205 deprecation warning。
- `bash scripts/harness/change-scope.sh` → 退出 0、识别前端 lint；全树另列并发其他 change 后端/未知路径，本轮不执行那些域的测试。
- `bash scripts/harness/doc-impact.sh context rework-workshop-into-radar-and-desk` → 退出 1，官方提示 context 已退役、由 constraint-injection 自动注入取代；不是本 change 行为错误，未使用逃生口。
- `bash scripts/harness/check-tasks-tail.sh openspec/changes/rework-workshop-into-radar-and-desk` → 通过。
- `bash scripts/harness/scenario-trace.sh openspec/changes/rework-workshop-into-radar-and-desk` → **仍失败**：117 个待对账 Scenario / 109 项旧映射问题。本轮 7 个精确映射缺口为 0；主线程明确批准仅补本轮、不扩修其他旧账，不将全 change 门禁写绿、不归档。
- `git diff --check -- docs/reference/flow/semantic-board.md` → 通过；`git diff --cached --name-only` → 空（无暂存文件）。未 reset/checkout/commit，不改无关脏文件。
- 8E.5 仍待：独立 review、浏览器亮暗宽窄视觉/任务验收、build 与部署由主线程协调，本 writer 未执行、未起 dev server、未触 DB。
- 浏览器复核入口：`http://localhost:5100/tags/246030`（卫生健康，既有 2026-10-07–24 冷启动报日）与 `http://localhost:5100/tags/1974`（成熟对照，选 2026-10-21/23/24 报日及实际同 ID 关联日；必要时用现有观察窗显示历史，不新增 date query）。本轮未重新 GET 或浏览器验证这些历史样本，实际可读日期以当前真实索引为准。
- 部署影响：矩阵其他组同款视觉、现有日期轴关联点、阅读区去重；无需迁移/回填/重生成/配置。旧无 ID 分区保留阅读且无黄点；缺分区/读取失败诚实降级及重试，不回写历史。

### 8E.6 review R-41 修复批验证结果（2026-10-30）

- 复现先行：新增 4 例集成用例（桌面列头黄点 / 窄屏日期条黄点 / 普通报日仍清旧选择 / 连续第二次黄点点击）**先红**——
  桌面 `expected '2026-10-23' to be '2026-10-21'`、窄屏阅读槽整块消失；复现口径按复核结论从**非默认回落日**
  出发（先切到另一个真实报日，再点索引缺该日的观察日；回落值恰等于回落前选中日时看不出搬走）。
- 修复：`laneConsoleView.reconcileSelectedDate` 新增 `verifiedDay` 参数（已验证记录日与报日同等对待，轴外仍回落）；
  `useConsoleSelection` 新增 `verifiedDay` 依赖；`useOtherTopicSelection` 持有 `observedTargetDay`（只认
  `selectObservedDay` 原子写入的那一个日子，普通选行/清场/切泳道/切板块/换日立即失效）；
  `useConsoleReadingWiring` 用同步 watcher 把它喂给选中日校正（构造顺序 3 在 5 之前，由壳层持有该值）。
- `cd front && pnpm test:unit app/features/tags/components/console --maxWorkers=2` → **49 文件 / 582 用例通过**（含新增 4+4+1+1 例）。
- `cd front && pnpm lint` → 退出 0、0 error、8 条其他文件既有 warning；`cd front && pnpm exec nuxi typecheck` → 退出 0、无类型错误。
- 未跑浏览器/未 build/未部署（本 writer 不执行，主线程协调）；未触真库、未提交、未归档、未改其他 change 文件。
- 修复批文档：[review-matrix-date-markers.md](review-matrix-date-markers.md)（R-41 High：主张→复核→复现证据→修复口径→非回归论证）、[test-cases-matrix-date-markers.md](test-cases-matrix-date-markers.md)（新增用例映射与白盒边界）。

<!-- scenario-trace: 归档全量对账（2026-11-02 补：其余 140 个 ADDED/MODIFIED Scenario 的逐字映射；8C 落点表已覆盖其余 15 个） -->
| Scenario | 测试文件 |
| --- | --- |
| 打开工坊首屏 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 从雷达进入版块 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 热点行带理由与密度 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 常温版块折叠 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 升温判定 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 无升温 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 一次请求渲染雷达 | front/app/features/tags/composables/useBoardRadarOverview.test.ts |
| 无数据版块 | backend-go/internal/topicgraph/handler/board_radar_overview_handler_test.go |
| 质量序跨分页稳定 | backend-go/internal/tagmanagement/handler/board_article_handler_test.go |
| 标记未入日报 | backend-go/internal/tagmanagement/handler/board_article_handler_test.go |
| 标签描述随标签返回 | backend-go/internal/tagmanagement/handler/board_article_handler_test.go |
| 按遗漏过滤 | backend-go/internal/tagmanagement/handler/board_article_handler_test.go |
| 弱证据过滤 | backend-go/internal/tagmanagement/handler/board_article_handler_test.go |
| 默认展示泳道动态区 | front/app/features/tags/components/console/ObservationLaneEvolution.test.ts |
| 只读语义 | front/app/features/tags/components/console/ObservationLaneEvolution.test.ts |
| 发展时间线结构 | front/app/features/tags/components/console/ObservationLaneEvolution.test.ts |
| 一句话态势展示 | front/app/features/tags/components/console/ObservationLaneEvolution.test.ts |
| 单日多事件 | front/app/features/tags/components/console/ObservationLaneEvolution.test.ts |
| 自由日期不触发新聚合 | front/app/features/tags/composables/useLaneTrendData.test.ts |
| 365档池请求去重 | front/app/features/tags/composables/useLaneTrendData.test.ts |
| 有界缓存与当前阅读稳定 | front/app/features/tags/composables/useLaneDayReading.cache.test.ts |
| 稳定 ID 关联 | front/app/features/tags/components/console/ObservationLaneEvolution.test.ts |
| 零写边界 | front/app/features/tags/composables/useBoardConsole.observation.test.ts |
| 弹窗内进入全屏 | front/app/features/tags/components/ArticlePreviewModal.test.ts |
| 退出全屏 | front/app/features/tags/components/ArticlePreviewModal.test.ts |
| 阅读页原地全屏不回归 | front/app/features/tags/components/ArticlePreviewModal.test.ts |
| 站内阅读预览收藏即时生效 | front/app/features/articles/composables/useArticleFavoriteToggle.test.ts |
| 收藏失败回滚并提示 | front/app/features/articles/composables/useArticleFavoriteToggle.test.ts |
| 连点不丢更新 | front/app/features/articles/composables/useArticleFavoriteToggle.test.ts |
| 预览内文章状态更新回流 | front/app/features/tags/components/daily-report/DailyReportDeskShell.test.ts |
| 展示日报卡片列表 | front/app/features/tags/components/daily-report/DailyReportDeskShell.test.ts |
| 打开全屏日报详情 | front/app/features/tags/components/DailyReportReader.test.ts |
| 宽屏双栏与窄屏降级 | front/app/features/tags/components/DailyReportReader.test.ts |
| 双主题渲染 | front/app/features/tags/components/daily-report/dailyReportMagazine.test.ts |
| 按持久话题状态分区 | front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts |
| 展开并缓存话题生命线 | front/app/features/tags/composables/useDailyReportReader.test.ts |
| identity 连线跨越空白日期 | front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts |
| lifeline 加载失败 | front/app/features/tags/composables/useDailyReportReader.test.ts |
| 展开 thread 文章并复用缓存 | front/app/features/tags/composables/useDailyReportReader.test.ts |
| 进入完整话题生命线 | front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts |
| 键盘关闭详情 | front/app/features/tags/components/DailyReportReader.test.ts |
| 空状态 | front/app/features/tags/components/DailyReportReader.test.ts |
| 加载超过 30 天 | front/app/features/tags/components/DailyReportReader.test.ts |
| 切换 board 重置状态 | front/app/features/tags/composables/useDailyReportReader.test.ts |
| 显示来源构成 | front/app/features/tags/components/BoardSourcePanel.test.ts |
| 打开完整来源表 | front/app/features/tags/components/BoardSourcePanel.test.ts |
| 切换板块重新拉取 | front/app/features/tags/components/BoardSourcePanel.test.ts |
| 空态引导切窗口 | front/app/features/tags/components/BoardSourcePanel.test.ts |
| 面板无写动作 | front/app/features/tags/components/BoardSourcePanel.test.ts |
| 默认只看数据日 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 自由日期范围 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 无文章 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 同一天多篇文章 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 一篇多标签 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 主点击读原文 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 标签色彩编码可扫描 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 筛选遗漏 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 弱证据筛选 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 解释弱证据 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 方向校验标记 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 标签描述展开 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 事件辅助×版块收编层级 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 强证据文章判定 | front/app/utils/matchQuality.test.ts |
| 各阶段得分分组 | front/app/utils/matchQuality.test.ts |
| 命中即停的阶段语义 | front/app/utils/matchQuality.test.ts |
| 通用匹配流程说明入口 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 事件私有判定行 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 仅弱证据文章判定 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 无标签 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 计算明细就地展开 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 历史计算缺失如实标注 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 日期优先 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 组内质量默认 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 切换时间序 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 无写动作 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 面包屑返回雷达 | front/app/features/tags/components/console/BoardConsoleShellNavigation.test.ts |
| 恢复上次选中泳道 | front/app/features/tags/components/console/BoardConsoleShellSelection.test.ts |
| 同日分桶不冒充分区热度 | front/app/features/tags/components/console/ObservationMatrixGrid.test.ts |
| 矩阵格值是桶峰值 | front/app/features/tags/components/console/ObservationMatrixGrid.test.ts |
| 渲染矩阵 | front/app/features/tags/components/console/ObservationMatrix.test.ts |
| 断档可读 | front/app/features/tags/components/console/ObservationMatrix.test.ts |
| 零线索可开 | front/app/features/tags/components/console/ObservationMatrix.test.ts |
| 查看某日事件 | front/app/features/tags/components/console/SectionDrawer.test.ts |
| 空格提示 | front/app/features/tags/components/console/SectionDrawer.test.ts |
| 单条真实摘要 | front/app/features/tags/components/console/LaneChronicle.test.ts |
| 相关文章按需 | front/app/features/tags/components/console/LaneChronicle.test.ts |
| 完整读取失败降级 | front/app/features/tags/components/console/LaneChronicle.test.ts |
| 切换泳道 | front/app/features/tags/components/console/BoardConsoleShellSelection.test.ts |
| 断档日期不造假节点 | front/app/features/tags/components/console/ObservationLaneEvolution.test.ts |
| 长窗趋势图 | front/app/features/tags/components/console/ObservationLaneEvolution.test.ts, front/app/features/tags/components/console/LaneTrendChart.test.ts |
| 拖选不重排 | front/app/features/tags/components/console/ObservationRangeControls.test.ts |
| 精确档回切 | front/app/features/tags/components/console/ObservationRangeControls.test.ts |
| 明确切独立档 | front/app/features/tags/components/console/ObservationRangeControls.test.ts |
| 档池分层失败 | front/app/features/tags/components/console/laneConsoleView.test.ts |
| 非法日期反馈 | front/app/features/tags/components/console/laneConsoleView.test.ts, front/app/features/tags/components/console/ObservationRangeControls.test.ts |
| 深链恢复 | front/app/middleware/tags-deep-link.test.ts, front/app/features/tags/components/console/BoardConsoleShellNavigation.test.ts |
| 折叠 | front/app/features/tags/components/console/ObservationMatrix.test.ts |
| 展开休眠 | front/app/features/tags/components/console/ObservationMatrix.test.ts |
| 右轨展示体检信号 | front/app/features/tags/components/console/BoardContextRail.test.ts |
| 无体检快照 | front/app/features/tags/components/console/BoardContextRail.test.ts |
| 追踪管理仅板块作用域 | front/app/features/tags/components/console/BoardContextRail.test.ts |
| 回填入口 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 研究工作台真实承接 | front/app/features/tags/components/console/BoardConsoleShellNavigation.test.ts |
| 无泳道仍有日报入口 | front/app/features/tags/components/console/BoardConsoleShellStates.test.ts |
| 进入编排态 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 查看与选入分离 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 取消 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 快捷日期与边界 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts, backend-go/internal/topicgraph/handler/daily_report_handler_test.go |
| 有草稿切换日期 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 倒置或越界日期 | backend-go/internal/topicgraph/handler/daily_report_handler_test.go |
| 跨期连接而非连续出现 | backend-go/internal/topicgraph/repository/compose_chain_test.go |
| 没连接或跨度不足 | backend-go/internal/topicgraph/repository/compose_chain_test.go |
| 采纳预填 | front/app/features/tags/composables/useLaneCompose.test.ts |
| 取消整组与半选 | front/app/features/tags/composables/useLaneCompose.test.ts |
| 人工名称不被覆盖 | front/app/features/tags/composables/useLaneCompose.test.ts |
| 清空与取消本次 | front/app/features/tags/composables/useLaneCompose.test.ts |
| 已有泳道显式参考 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 历史启用资格 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 从手动列表选事件 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 画布节点可识别 | front/app/features/tags/components/compose/LaneComposeCanvas.test.ts |
| 搜索过滤 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 归档来源可识别 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 采纳后质量刷新 | front/app/features/tags/composables/laneComposeLogic.test.ts |
| 点击节点推荐关联 | front/app/features/tags/composables/laneComposeLogic.test.ts |
| 从推荐项加入 | front/app/features/tags/composables/laneComposeLogic.test.ts |
| 无关联 | front/app/features/tags/composables/laneComposeLogic.test.ts |
| 无距离数据 | front/app/features/tags/composables/laneComposeLogic.test.ts |
| 相近程度不等于已选 | front/app/features/tags/composables/laneComposeLogic.test.ts |
| 右栏只做三件事 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 已选里取消勾选 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 画布标题可读且不打架 | front/app/features/tags/components/compose/LaneComposeCanvas.test.ts |
| 选中后相关节点显示标题缩略 | front/app/features/tags/composables/laneComposeLogic.test.ts |
| 缩放/平移后节点卡不塌扁 | front/app/features/tags/components/compose/LaneComposeCanvas.test.ts |
| 拖拽平移不误伤点选 | front/app/features/tags/components/compose/LaneComposeCanvas.test.ts |
| 相关节点在已有泳道里 | front/app/features/tags/composables/laneComposeLogic.test.ts |
| 节点卡跟随选中节点并可快捷选入 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 取消节点卡与取消选中 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 缺名称 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 保存成功 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 窄屏提示 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |

## 新建泳道设计修订（2026-10-30，仅原型，未批准产品实现）

- [x] 按用户最终“连接跨越3天、不是连续3天”修订候选定义、整组基础成员带入、日期共窗与均值推荐，更新proposal/design/ADDED delta及阶段合同。
- [x] 只读采样真实90天pool/topics/timeline，默认30天3候选；1984含10/27、28、30三个节点，一次精确带入，推荐不自动混选。
- [x] 构建独立候选整组原型，目标纯逻辑12条＋agent-browser完整故事45条通过，13图包含两桌面/双主题/自选/空态/归档来源/两窄屏。
- [x] pi-web独立复核无阻断；非阻断建议取舍及更新后重验记录在当前review.md，不把管理观察中门槛改为与左候选同可见集。
- [x] 用户反馈「整组带入看不出选中/缺明确取消、已有泳道与待划分话题交叉致拓扑图杂乱、质心相近点缺直观区分、已选与推荐视觉混淆」→ 新建勾选与相近度版原型（`ui-prototype/revision-2026-10-30-lane-compose-selection/`）：原生复选框＋半选态、取消整组/清空已选/取消本次、已有泳道默认不画（显式开启才作只读参照）、推荐按真实余弦距离映射点大小与填色深浅、已选用实心＋✓独立标识。
- [x] 先复现旧版缺陷（内部 `selected` 确有7345/7409/7514，但候选区0个原生复选框、无取消整组）再改，不以内部状态正确否定用户观察。
- [x] 勾选版纯逻辑19条＋agent-browser 52条通过、14图（两桌面/双主题/自选范围/空态/半选/显式参照/两窄屏）。
- [x] 用户要求右栏瘦身＋画布做主角：右栏改为「当前事件线索 → 可补充的事件 → tab（已选事件/按标题找）」，删掉关联事件清单、计算依据折叠、话题泳道参照、其余推荐折叠；手动找事件搬进右栏 tab；画布加标题三档（话题带/全部/编号）与悬停完整标题浮层；两侧栏收窄把宽度让给画布。逻辑22条＋浏览器63条通过。
- [x] 画布标题与右栏 tab 当面拍板：默认「话题带 + 散点悬停」，全部/编号两档仅原型对比不进产品；右栏两页 tab（已选事件 / 按标题找），可补充的事件固定在上方段。
- [x] 用户指出画布点太小且溢出右侧：修因——画布 viewBox 宽度原是开页那刻算死的旧值（改视口不重算），导致整体被放大又裁边；改为按容器实际像素 1:1 画 + 窗口变化重排重绘，点放大到 1440 下普通点约12px/已选约20px，标题避让加边界检查、贴右编号自动翻边、话题标题无空间时收进横条。逻辑22条＋浏览器65条全绿。
- [x] 用户要求 tooltip 跟随选中节点并带快捷选入/移除：悬停出卡、点选后钉在该节点旁并跟随，卡内「＋选入/移出」真实改成员并当场翻字，右栏“当前事件的线索”改为只叙述不重复放按钮，已有泳道点只读不给选入。浏览器70条全绿。
- [x] 用户指出「点了没法取消展示」：节点卡右上角加 ×＝取消选中该节点（卡片收起、右栏线索清空），再点一次同一节点同样取消；取消后悬停别的点仍能临时读全文，不永久钉死。浏览器74条全绿。
- [x] 独立复核（只读子线程）0 阻断、4 建议＋6 观察；S1–S4 与 O1/O2/O5/O6 已在原型真修并补断言（手写名不被历史带入覆盖、半选卡片可视、推荐半径恒小于已选半径、死CSS清理、图面留白收紧），O3 写入素材边界、O4 判为 pending UX 不改；处置明细见 review.md §十。
- [x] 用户审阅并明确批准当前新建泳道原型（2026-10-31 用户原话「原型通过了，按规范开始实现吧」）；`ui-design.md` 的 `ui-approval` 已由 pending 改 approved，批准范围仅限新建泳道编排工作面。
- [x] 编写产品实现契约 [`lane-compose-implementation-contract.md`](lane-compose-implementation-contract.md)：后端端点扩展（`from`/`to` + `relations` + `candidate_chains` + `window`）、候选链服务端唯一口径、质心/推荐前端口径、视觉与交互口径、前端落点表、Scenario→测试文件映射、实现红线。
- [x] 实现编排六步步骤 3–5：派发后端实现子线程、前端实现子线程、后端 review 子线程（`zai-coding-cn/glm-5.3:max`），跑影响包门禁、更新 flow 溯源。（已执行完毕：后端/前端实现与 review 建账见 §12 与 `review-lane-compose-implementation.md`，R-43~R-51 全部修复，验证 12.5.1–12.5.3 全绿——本行此前漏勾，2026-11-02 补勾）

制品：[阶段合同](ui-design-lane-compose-draft.md)、[故事账本·勾选版](test-cases-lane-compose-selection-prototype.md)、[故事账本·质心版](test-cases-lane-compose-centroid-prototype.md)、[原型与执行记录](ui-prototype/revision-2026-10-30-lane-compose-selection/review.md)、[产品实现契约](lane-compose-implementation-contract.md)。本段完成项仅指原型，不代表本change整体完成、产品测试清账或真实保存/PATCH验收。

## 12. 新建泳道编排产品实现（2026-10-31，原型已批准进入实现）

> 契约来源：[`lane-compose-implementation-contract.md`](lane-compose-implementation-contract.md)（唯一口径）。delta spec：[`specs/workshop-lane-compose/spec.md`](specs/workshop-lane-compose/spec.md)。批准：`ui-design.md` `ui-approval: approved`（仅新建泳道编排工作面）。
> **不得把原型离线样本写进产品**；候选链口径服务端唯一实现，质心/推荐按既有端点设计在前端实时算。

### 12.0 前置

- [x] 12.0.1 用户批准原型并落 `ui-approval: approved`
- [x] 12.0.2 实现契约成文（端点字段 / 候选链 / 质心 / 视觉 / 交互 / 落点表 / 映射表 / 红线）
- [x] 12.0.3 `doc-impact.sh suggest` 预勾选（flow / api / database / standard）并写入下方文档节
- [x] 12.0.4 复杂逻辑白盒用例枚举（候选链分支表 / 日期窗校验表 / 质心边界）→ 落 `test-cases-lane-compose-implementation.md`

### 12.1 后端（`backend-go/internal/topicgraph/`）

- [x] 12.1.1 新增候选链唯一实现（建议 `repository/compose_chain.go`）：按 candidate topic_id 分组 → 原 relations 无向连通分量 → 节点≥2 且首末 UTC 日差≥3 → 稳定 key 与排序；**不同 topic_id 不合并、不连通分量不混选**
- [x] 12.1.2 `compose-candidates` 响应扩展：`relations`（窗口内两端可见的 identity+similarity 原边）、`candidate_chains`、`window{from,to,days,latest_completed_date}`；`sections` / `match_threshold` **向后兼容不变**
- [x] 12.1.3 日期窗参数 `from`/`to`（`YYYY-MM-DD`，闭区间，`days` 被忽略）与校验：单边 400 / 倒置 400 / 超 90 天支持窗 400（**不静默截断**）/ 窗外 400；`to` 晚于最新已完成日报允许
- [x] 12.1.4 既有 `POST .../persistent-topics/manual`（`active/manual`、均值初始向量、单事务）**不改语义**
- [x] 12.1.5 后端测试：候选链分支表白盒用例 + handler 日期窗校验 + relations/chains 返回形状

### 12.2 前端（`front/app/features/tags/`）

- [x] 12.2.1 `api/persistentTopics.ts` 扩展 `getComposeCandidates`（`days`/`from`/`to`）与 `relations`/`candidate_chains`/`window` 类型
- [x] 12.2.2 新增 `composables/laneComposeLogic.ts`：从原型 `compose-logic.js` 逐字移植 `centroidOf`/`cosineFor`/`centroidRecommendations`/`laneSelectionState`/`candidateSelection`/`estimateTextWidth`/`truncateText`/`boxesOverlap`/`placeLabels`/`pointAppearance`（**不含 `candidateChains`**——服务端口径）
- [x] 12.2.3 新增 `composables/useLaneCompose.ts`：窗口 / 候选链 / 选中集合 / 焦点 / 质心推荐 / 命名 / 草稿确认 / 日期非法保留旧范围
- [x] 12.2.4 重写 `components/console/BoardComposeOverlay.vue` 为三栏工作面；新增 `components/compose/LaneComposeCandidates.vue`、`LaneComposeCanvas.vue`、`LaneComposeInspector.vue`
- [x] 12.2.5 原生 checkbox 三态（unchecked/indeterminate/checked）与成员计数、图上✓、保存可用性**同源派生**；整组补齐去重、**不截 20 条、不自动补相似、不吞组外补选**
- [x] 12.2.6 自动预填名跟随来源组；**人工名不得被预填或历史带入覆盖**；取消组同时清该组自动名
- [x] 12.2.7 画布：容器真实像素 1:1、窗口变化重排重绘、话题带＋散点悬停、标题避让不越界、推荐半径恒小于已选半径、已有 active 泳道默认隐藏且只读
- [x] 12.2.8 节点卡：悬停出卡 → 选中后钉住跟随 → 卡内「＋选入/移出」真实生效翻字 → 卡上 × 或再点同一节点 = 取消选中
- [x] 12.2.9 右栏只做三段（当前事件线索只叙述 / 可补充的事件 / tab：已选事件 + 按标题找）；已选逐条真实复选框，取消即移出
- [x] 12.2.10 `useInlineCompose.ts` **不改**（`BoardThreadBrowser.vue` 在用）；旧编排面被三栏工作面替换的能力去向记入 review
- [x] 12.2.11 前端测试：纯函数 Vitest（质心/推荐/选择/避让）、组件用例（右栏三段两 tab、节点卡、日期共窗与草稿确认、已有泳道默认隐藏、窄屏提示）
- [x] 12.2.12 **相关节点标题**（用户 2026-11-01 补充建议 + 同日澄清口径，实现期落产品）：**相关** = 1 跳原关系 ∪ **质心较近的可补充推荐**（右栏「可补充的事件」同一批，不含 active 只读参照）——**可画的**在**节点下方**显示**标题缩略**（≤10 字带省略号，冲突先向下错行）；**只读的**（已有泳道）改为底部提示行点名（只读梯子 10px 密排放不下逐点标题）；只认原关系、不做传递闭包、已选成员自身不算；落点 `laneComposeLogic.ts:relatedNodeIds` + `placeRelatedTitles`（纯函数）+ `LaneComposeCanvas.vue`（落位渲染与提示行）；用例 `laneComposeLogic.test.ts` R1–R5/T1–T2 + `BoardComposeOverlay.test.ts` 组件级 R6–R8（FB-15）
- [x] 12.2.13 **画布滚轮缩放**（用户 2026-11-01 补充建议，子线程实现）：画布上滚轮以光标为锚点缩放（0.5×–3×、步进 1.1）、右下角读数 + 重置、**100% 仍满足 1:1 契约**（viewBox 不变、只改元素尺寸 → 日期列不重排）、缩小补偿保证渲染半径 ≥3px、节点卡按缩放换算仍贴节点跟随、滚轮 `preventDefault` 不滚走页面；落点 `laneComposeLogic.ts`（clampZoom/zoomStep/zoomAnchorScroll/scaledPointRadius）+ `LaneComposeCanvas.vue` + 新增 `LaneComposeCanvas.test.ts`；用例 Z1–Z8（单测 11 例 + 真机 E2E E16 七条）
- [x] 12.2.14 **画布拖拽平移**（用户 2026-11-01 补充建议）：按住左键拖画布即平移（内容跟手、滚动位置与位移反向）；拖 ≠ 点（<4px 算点，节点点击照常生效）、拖完那一下 click 不误选节点、**指针捕获只在判定为拖的那一刻启用**（`pointerdown` 即捕获会改派 click 导致节点点不中——这是实现期实测踩到的坑）、节点卡与缩放控件不参与平移；落点 `laneComposeLogic.ts:panScroll/isDragBeyond` + `LaneComposeCanvas.vue`；用例 P1–P6（单测 5 例 + 真机 E2E E17 三条）
- [x] 12.2.15 **节点卡宽度固定**（2026-11-01 用户报障「超长显示 bug」修复）：绝对定位卡片的 shrink-to-fit 宽度被「容器宽 − left」夹住、而 left 又用「量出来的宽度」算 → **反馈环**把卡片塌成 92px 竖条（标题一字一行）；修法：宽度改用常量 `CARD_WIDTH = 300` 参与定位、量出来的只取高度、样式固定宽度 + `max-height` 封顶 + 卡内滚动；回归锚 `LaneComposeCanvas.test.ts` Z8（源码锚）+ 真机 E16.8（真实浏览器量宽 ≥240px）

### 12.3 review（步骤 4）

- [x] 12.3.1 后端 review 子线程（`zai-coding-cn/glm-5.3:max`）：按 topic-graph / daily-report 红线审 diff，意见 High/Medium/Low 处置后建账 `review-lane-compose-implementation.md`
- [x] 12.3.2 能力去向表（旧 `BoardComposeOverlay` 编排面 / 旧 `useInlineCompose.adopt` 语义 → 新承接或主动退役）

### 12.4 文档

<!-- doc-impact: flow api database standard -->

- [x] 12.4.1 `docs/reference/flow/daily-report.md` / `topic-graph.md`：补新建泳道编排的口径与不变量（候选链跨度口径、质心来源、只读边界）
- [x] 12.4.2 `docs/reference/api/`：compose-candidates 新增参数与响应字段
- [x] 12.4.3 `docs/reference/standard/frontend/layout.md`：编排工作面三栏契约（若形成通用契约再登记）

### 12.5 验证（步骤 5）

- [x] 12.5.1 后端：`cd backend-go && golangci-lint run ./... && go vet ./... && go test ./internal/topicgraph/... && go build ./...`
- [x] 12.5.2 前端：`cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit <影响文件> --maxWorkers=2 && pnpm build`（build 与其它重活不并行）
- [x] 12.5.3 真机端到端：`node ui-verification/lane-compose-e2e-2026-10-31/verify.mjs` → **65 checks / 0 failed**（含 1440/1920 双视口、375/390 窄屏、画布几何量测、节点卡钉住与取消、日期共窗草稿确认）；报告 `ui-verification/lane-compose-e2e-2026-10-31/report.md`
- [ ] 12.5.4 归档门禁（实现段不归档、整 change 归档时统一跑）：`bash scripts/harness/check-standards.sh` + `bash scripts/harness/doc-impact.sh verify` + `bash scripts/harness/check-tasks-tail.sh openspec/changes/rework-workshop-into-radar-and-desk`

### 12.6 Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 编排态入口与全屏画布（进入编排态 / 查看与选入分离 / 取消） | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| 编排日期范围（快捷日期与边界 / 有草稿切换日期 / 倒置或越界日期） | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts`、`backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 候选话题侧栏（跨期连接而非连续出现 / 没连接或跨度不足 / 采纳预填 / 取消整组与半选 / 人工名称不被覆盖 / 清空与取消本次 / 已有泳道显式参考 / 历史启用资格） | `backend-go/internal/topicgraph/repository/compose_chain_test.go`、`front/app/features/tags/composables/useLaneCompose.test.ts` |
| 可读事件与人工补选（从手动列表选事件 / 画布节点可识别 / 搜索过滤 / 归档来源可识别） | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| 成员质心与关联推荐（采纳后质量刷新 / 点击节点推荐关联 / 从推荐项加入 / 无关联 / 无距离数据 / 相近程度不等于已选） | `front/app/features/tags/composables/laneComposeLogic.test.ts` |
| 右栏职责与画布标题（右栏只做三件事 / 已选里取消勾选 / 画布标题可读且不打架 / 节点卡跟随选中节点并可快捷选入 / 取消节点卡与取消选中） | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| 保存与只读边界（缺名称 / 保存成功） | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| 窄屏禁用编排（窄屏提示） | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |

