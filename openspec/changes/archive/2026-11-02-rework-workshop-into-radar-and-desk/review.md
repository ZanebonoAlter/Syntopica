# 叙事工坊改版：用户投诉后的独立 UI / 产品回归审查

审查日期：2026-10-25。对象：`rework-workshop-into-radar-and-desk`。本轮只审查，**没有修业务代码、改数据库、生成数据、部署或归档**。

## 结论

**不能把这次改版判为“旧功能已完整迁移”。确认 3 项 High、7 项 Medium。**

用户感受到的功能丢失有真实依据，但不是“所有能力都被删了”：

1. **实质丢失/不可达**：删除版块入口、工坊内原有正文阅读承载、文章来源/构成标签/质量筛选、跨期追踪命中总览。
2. **还在但错位**：话题生命周期管理藏到侦探墙；治理全库动作放进单版块菜单；日报阅读中切版块入口被拿掉。
3. **数据仍在，被视图挡住**：只有候选话题的版块有真实日报，却因没有 active 泳道而失去日报入口、雷达显示 0 分区。
4. **文档记载的主动取舍**：五个 tab 退役、卡片墙改矩阵、DAG 降到编排、归档改日期平铺、阅读台取消左右常驻栏。这些不能直接算实现误删；但批准简化布局不等于批准取消原有使用故事。

**建议：暂停把当前版本作为完整迁移成果收口，先恢复能力，再决定哪些旧展示确实不要。** 不建议为了消除投诉再次无边界重画一套 UI。

## 修复进展（2026-10-25 晚，主线程回写）

> 原报告结论保留为审查时快照，不改写；以下为后续修复状态，证据见 `ui-verification/report.md` A-41…A-54。

| 项 | 状态 | 落点 |
| --- | --- | --- |
| R-1 无活跃泳道被挡日报 | 已修复 | 控制台独立拉最近日报，回退条 + 顶栏「最新日报」；归档默认该日报日；`console-no-lanes-fallback` |
| R-2 删除版块 | 已修复 | 治理 → 设置危险区，二次确认后 `deleteBoard`，成功回雷达 |
| R-3 站内正文阅读 | 已修复（归档 + 日报抽屉） | 归档抽屉/日报文章抽屉「站内阅读」复用 `ArticlePreviewModal` + `ArticleContentView` |
| R-4 归档筛选/诊断 | 已修复 | 来源、构成标签、方向显示开关、逐标签「计算明细」（MatchDetailPanel）；同日按完整发布时间排序 |
| R-5 话题管理入口深埋 | 已修复 | 治理右轨「话题管理」直达 `TopicManageDialog`；侦探墙保留为附加入口 |
| R-6 阅读台跨版块/跨期追踪 | 部分修复 | 索引顶部版块下拉；日期下拉标注 watch 命中期；命中期点击定位的完整故事仍待补测 |
| R-7 历史格子看归档丢日期 | 已修复 | SectionDrawer 带日期 → 归档范围锁定该日 |
| R-8 同日时间乱序 | 已修复 | `sortArchiveArticles` 按完整 `pub_date` 倒序，id 仅平局兜底 |
| R-9 全库回填范围不清 | 已修复 | 菜单标「匹配回填（全库）」+ 执行前 confirm |
| R-10 验收只证页面存在 | 部分修复 | 新增 50 条实页/产品任务账本；`8.4` 旧面板清理、`9.3` reader 完整回归、`9.5` 旧宿主用例仍待办，未归档 |

## 方法与证据边界

- 按《开发执行规范》§0.6、§5.3，结合 `layout.md`、`test-design.md`、项目 UI-review 职责执行：旧能力去向、完整用户故事、运行时可操作性、展示与数据语义分开检查。
- 对照 proposal / discussion / ui-design / specs / design / tasks / test-cases / 旧验收报告；检查旧宿主、新路由和实际调用点。旧动作还与 `git show HEAD:<path>` 交叉核对，不 checkout、不回滚。
- 基线 HEAD：`e9c76760`。工作树有 178 条混合状态记录，存在其他 active change；**不把全树 diff 都归给本 change**。本报告主要归因于已确认的 `/tags` 路由切换及新宿主接线。
- 实页用一个 agent-browser 会话访问真实静态托管 `http://localhost:5100`，不是原型或 mock；已关闭自己的会话。没有清理 47 个归属未知的历史进程。
- GET 抽查：overview；1974、1980、50413、246030 的日报列表/详情/泳道；1974、50413 的文章窗口。没有直连数据库或执行 SQL。
- 有真实 1440×900、1920×1080、375×667 证据，但不是所有页面×视口×主题的完整笛卡尔覆盖。写动作只检查接线，不执行。
- 独立静态审查、实页审查、`zai-coding-cn/glm-5.3-flash` 视觉目检均已汇总；父线程重新核验后才采信。**不照抄子线程结论**：已否决“话题管理全站不可达”“方向徽标等价于方向筛选恢复”等判断。
- 核心源文件修改时间早于当前静态产物（`backend-go/frontend/index.html` 2026-10-25 12:56 UTC），未为审查重建/替换运行版本。

## 八个维度的判定

| 维度 | 判定 |
| --- | --- |
| 布局 | 有 active 泳道的矩阵、阅读台、手机线性布局未见关键横向破版；无 active 泳道时主舞台空白、治理占据注意力，缺少内容阅读兜底 |
| 使用 | 导航壳能用，但“找到文章→站内读正文”“无正式泳道→读已有日报”“历史格子→同日材料”断链 |
| 数据来源 | 基本来自真实 API，不是演示数据；但 active 泳道数量被拿来代表版块内容活跃度，组织状态与内容存在性混淆 |
| 产品定位 | 把“每天读日报、空闲深读、追事件演进”的阅读工作台，部分缩窄成了“看已整理泳道+查匹配证据”的控制台 |
| 用户故事完整性 | 新场景有覆盖，旧能力迁移不是一一对账；旧组件文件和单测还在，不代表人能进入它 |
| 人类可操作性 | 有些入口只是隐藏，有些已经无新家；日常管理需经可视化侦探墙绕行，不合理 |
| 实际展示 | 抽查能显示真实内容；确认归档同日时间乱序、候选版块零值误导及空态缺少出路；不把进场白屏截图算稳定白屏 bug |
| 误删/错位 | 下列 R-1…R-10 给出可追溯事实和恢复方向；主动批准的视觉取舍另列，不混算 |

## 数据口径核对

| 页面 | 实际来源 | 需要分清的语义 |
| --- | --- | --- |
| 雷达 | `GET /semantic-boards/overview` → active 版块 + 最新日报摘要 + `lane-dynamics` | 数量来自 **active 泳道**的 section，不是全部日报/候选/未分类内容，也不是未来事件预测 |
| 矩阵 / 演进带 | `lane-dynamics` + 泳道 snapshot | 矩阵格是 section 数，演进带热度/行尾线索数另算；都不是文章数 |
| 归档 | `GET /semantic-boards/:id/articles` + 当前匹配标签；日报成员关系由 `related_article_ids` 派生 | “未入日报”只针对所选窗口，不等于从未进入任何历史日报；当前匹配分数也不等于当年生成时冻结的证据 |
| 阅读台 | 日报列表/详情、topic lifeline、文章详情、watch/批注 | 日报内容和既有 mini 生命线仍在；删常驻栏不应连带取消内容入口与跨期导航 |
| 治理 | 体检、来源、构成各自 API | 体检候选是构成治理，不是“候选话题”；两种候选不应混为一谈 |

2026-10-25 只读取数结果：

| 版块 | 最新日报 | 实际 section / thread | 当前话题状态 | 雷达 anchor / 当日分区 / attention |
| --- | --- | --- | --- | --- |
| 1974 中东 | 1239，10-25 | 4 / 7 | 4 active | 10-25 / 4 / 22 |
| 1980 生成式 AI | 1240，10-25 | 4 / 12 | 4 active | 10-25 / 4 / 24 |
| 50413 影视作品 | 1237，10-24 | 1 / 2 | 1 candidate | 空 / 0 / 0 |
| 246030 卫生健康 | 1238，10-24 | 1 / 1 | 1 candidate | 空 / 0 / 0 |

另：3122 最新数据日是 10-24，全局日期是 10-25；不应把各版块不同数据日都笼统称作“今天”。源码对主行已有部分日期提示，因此**不宣称所有行都未标日期**；本轮未完成 3122 实页验证。

计量单位也需要讲清：1974 同一期 `article_count=12`、`event_tag_count=10`、section=4、thread=7。雷达“事件”字段来自 event tag 数，不能与线索/分区混称。数字不同本身不代表数据错，但标签必须准确。

## Findings

### R-1 2026-10-25【High】已有日报被 active 泳道门槛挡住，雷达零值也在掩盖内容
- **状态**：已修复（控制台独立拉最近日报 + 无泳道回退条；50413 实页验证）
- **故事**：选择一个还没有正式泳道的版块，直接读已经生成的日报。
- **实页复现**：`/tags/50413` 在 1920 与 375 下无最新/今日日报入口，仅提示“暂无活跃泳道——在日报里孵化话题后出现”。直达 `/tags/50413/2026-10-24` 却能读到真实日报。常温展开后还显示“近 14 天 0 分区”。
- **根因**：`backend-go/internal/topicgraph/repository/board_radar_repository.go:180-209` 仅遍历 active lanes 计数；`front/app/features/tags/composables/useBoardConsole.ts:250-257` 只从 lanes 推最新日期；`BoardConsoleShell.vue:457-466` 按该日期决定是否给日报按钮。`BoardArchiveView.vue:26-32` 又会在无 anchor 时默认今天，50413 的历史主内容因而不是默认所见。
- **布局后果**：无泳道时仍邀请“选一条泳道”，主舞台大片空白，治理信息反而成为唯一有内容的区域。不能让用户先整理/转正话题才能阅读。
- **证据**：`04-50413-console-1920.png`、`09-50413-console-375.png`、`04-50413-report-1920.png`、`08-mobile-radar-expanded.snapshot.txt`；对应 GET 证据见本轮 `data/`。
- **恢复**：最新日报身份独立于泳道取数；无泳道也给最近日报/归档/候选内容入口；区分正式泳道分区数和全版块内容数。**不需要用户重生成历史日报。**

### R-2 2026-10-25【High】删除版块没有迁到新宿主
- **状态**：已修复（治理 → 设置危险区，二次确认；未在真库执行删除）
- **旧能力**：旧 `TagsPage.vue:205-218` 给 `BoardListSidebar` 绑定 `@delete="handleDeleteBoard"`；`useBoardCRUD.ts:127-137` 调用既有删除 API。
- **现在**：旧 TagsPage 无路由挂载；新 `BoardRail.vue` 只有导航/新建，`BoardSettingsForm.vue` 没有删除区；生产代码中的 `deleteBoard()` 调用仍只在旧 CRUD 通路。不是换个名字藏起来，而是新页面没有承接。
- **契约**：ui-design 的危险操作行仍明确包含“删除版块，二次确认”。
- **证据层级**：旧/新接线与全局调用点核验；**没有实点删除，不能声称删除 API 成败已测试**。
- **恢复**：治理设置补危险区，复用原删除规则和确认，保留不可撤销/影响范围说明；不必恢复旧侧栏。

### R-3 2026-10-25【High】原文章阅读被缩成证据/摘要，深读故事漏迁移
- **状态**：已修复（归档抽屉「站内阅读」复用 ArticlePreviewModal；日报抽屉加同入口）
- **实页复现**：50413 → 归档 → 近 7 天 → 点“本地视频 → 序列帧动画精灵图”。抽屉只有匹配规则、日报状态和外站“打开原文”，没有已抓取正文或站内阅读入口。
- **旧能力**：`TagsPage.vue` → `ArticlePreviewModal.vue` → 原 `ArticleContentView`，并有收藏/更新/前后篇交互。新 `BoardArchiveView.vue:250-329` 只显示证据；日报 `DeskArticleDrawer.vue:30-32,71-79` 优先 AI 摘要，并将 HTML 拍平成纯文字，也不是原正文阅读承载。
- **设计错位**：“归档需要看匹配理由”被实现成“归档只剩匹配理由”。“只读”限制不应被理解为不能阅读站内内容；收藏属另一个需恢复的便捷动作，不和只读归档强行混为一谈。
- **归因**：新规格/任务没有完整继承旧深读故事，Component Reuse 却仍写“基于 ArticlePreviewModal 内容”；不是仅仅一个漏掉的按钮。
- **证据**：`10-50413-archive-7days-375.png`、`10-50413-article-drawer-375.png` 与同名前缀 snapshot。
- **恢复**：保留证据区，同时复用现有文章正文渲染/预览或提供明确站内阅读入口；顺手收藏、上下篇另列迁移项。外链不能替代缓存正文阅读。

### R-4 2026-10-25【Medium】文章筛选与匹配诊断被缩水，不是等价替换
- **状态**：已修复（来源/构成标签/方向开关 + MatchDetailPanel「计算明细」）
- **丢失**：旧 `BoardTimelinePanel.vue:66-140` 的来源筛选、构成辅助标签筛选、质量/时间排序、显示方向不符开关，未迁到新归档；新 `BoardArchiveView.vue:51-62` 仅有日期和全部/未入/弱证据。
- **诊断差异**：旧 `MatchDetailPanel.vue:22,83-113,127-178` 可核查具体辅助标签、相似度配对、命中率/阈值/方向相似度；新证据区只有规则名、分数和通用说明。“规则解释”不等于“本次匹配计算明细”。
- **重要纠正**：`useBoardArchive.ts:148-155` 没传 `show_direction_mismatch=true`，后端默认隐藏相应标签。只有方向徽标模板，**不等价于恢复“查看被隐藏匹配”能力**。
- **证据层级**：静态接线确认；本轮没有对所有筛选恢复路径逐一实页操作。
- **恢复**：归档加入高级筛选，复用现有 `feed_id`、`auxiliary_label_id`、`sort`、方向参数；增加“计算明细”入口。不要在前端把 event tag id 错当辅助标签 id 来硬过滤。

### R-5 2026-10-25【Medium】话题管理还在，但日常管理被埋进侦探墙
- **状态**：已修复（治理右轨「话题管理」直达 TopicManageDialog）
- **事实**：新矩阵/治理/编排没承接原泳道旁的重命名、归档/恢复、删除。**但不能称这些功能全站已删除**：`DailyReportReader.vue:709,827` → `TopicDetectiveWall.client.vue:710-729,849` → `components/dialog/TopicManageDialog.vue:98,160,182,210` 仍有完整动作。
- **错位**：管理普通话题，需要先找到日报话题、展开生命线、进入可视化侦探墙并聚焦节点，再找“话题管理”。它依赖与治理无关的重型可视化和有内容的节点，不是合理的新家。
- **证据边界**：路径为源码确认；本轮未实际打开最终管理窗，也没执行写动作。
- **恢复**：在版块治理或泳道菜单给直接入口，复用现有管理对话框与确认语义。侦探墙入口保留为附加入口。

### R-6 2026-10-25【Medium】阅读台删栏时连带丢了跨版块与跨期追踪导航
- **状态**：处理中（已补版块下拉与追踪日期标记；定位命中期完整故事未验）
- **实页**：1974 阅读台 1920/375 能选择日期、回矩阵、回主页，但没有阅读中直接切其他版块的入口；手机仍可“回矩阵→雷达→其他版块”，不是完全无法切换。
- **接线**：`DailyReportReader.vue:636-681` 只在非 desk Sidebar 中放 boards/todayBoards/select-board；desk Index 没接版块切换。`DailyReportDeskShell.vue:106-118` 仍传对应参数和事件，UI 发不出来。
- **跨期追踪**：旧日报列表有 `activeWatchSummaries` 命中 chip，可直达命中期并定位 watch；新日期下拉没有同等命中总览，壳也没透传 initialWatchId。当期 `DailyReportWatchIndex` 定位仍在，**不能说追踪功能全部丢了**。
- **证据**：`06-1974-reader-stable-1920.png`、`07-1974-reader-375.png`；追踪跨期结论为静态检查，定位写/读流程未完整实测。
- **恢复**：日期控件附近加紧凑版块选择及命中标识/命中期导航；不要求加回已批准取消的常驻四栏。

### R-7 2026-10-25【Medium】从历史格子“看归档”没有带过去所看的日期
- **状态**：已修复（SectionDrawer 带日期，归档锁定该日）
- **源码复现链**：`SectionDrawer.vue:32,181-183` 的 `viewArchive` 无日期参数；`BoardConsoleShell.vue:272-275` 仅关闭抽屉并切视图；`BoardArchiveView.vue:30-32` 重新默认最新数据日。
- **用户后果**：明明在看 10-12 的事件，落地却是最新一天的文章；用户需要自己发现并改日期，容易误判该事件缺材料。
- **证据层级**：静态接线确认，计划的前后页浏览器复现未完成；不以普通矩阵截图冒充跳转证据。
- **恢复**：事件携带 section 日期，归档显式接收该日；普通视图切换则保存用户已有起止日期与筛选。

### R-8 2026-10-25【Medium】归档前端重排把同一天的真实发布时间打乱
- **状态**：已修复（完整 pub_date 倒序，id 仅平局兜底）
- **实页及 GET 复核**：50413 近 7 天，10-21 显示 `09:02 → 20:19 → 23:20 → 21:36…`。原 API `sort=time` 返回 `23:20 → 21:36 → 20:32 → 20:19…`，被前端二次排序覆盖。
- **根因**：`useBoardArchive.ts:114-119` 只比较日期切片，同日直接 `b.id-a.id`；入库 ID 不等于发布时间。
- **契约**：规格前文承诺“按发布时间倒序”，排序节又写“同日发布时间/id”，有歧义；不能用后一处含混文字替代真人可理解的时间顺序。
- **证据**：`10-50413-archive-7days.snapshot.txt`，`data/articles-50413-7days.json`。乱序位置的专门截图未完成，不能把列表入口图当作该位置截图。
- **恢复**：完整发布时间倒序，ID 只作为时间相同的平局兜底；不改历史文章时间。

### R-9 2026-10-25【Medium】单版块“匹配回填”入口实际作用于全库，缺少范围提示
- **状态**：已修复（菜单标「全库」+ 执行前 confirm；未实际执行）
- **接线**：`BoardConsoleShell.vue:157-160,494-498` 版块“更多”菜单的匹配回填直接调用旧函数；`useTagsPage.ts:151-155` 实际传 `{ mode: 'all' }`，没有当前 board id 或确认步骤。
- **错位**：旧全局动作搬进单版块上下文后，用户容易以为只重算眼前版块。全库函数不是本次新造的，**问题在迁入口没有一并迁移作用范围语义**。
- **证据边界**：源码静态核验；本轮没有点击回填，避免真实数据变化和资源消耗。
- **恢复**：明确标“全库”，加作用范围/成本确认，或真的提供当前版块范围；不要只在设计文档里解释而让界面继续含混。

### R-10 2026-10-25【Medium】验收在证明新页面存在，没有证明旧故事完整迁移
- **状态**：处理中（账本/去向表已补；8.4/9.3/9.5 未完成）
- **账本证据**：旧 A-5 把“采纳后成员可为 0”计 pass，最多证明预填名称；A-6 的启用条件主要来自单测；A-36 打开治理弹窗不等于走完确认/保存。`tasks.md` 中 9.3 reader 回归、9.5 旧宿主用例调整仍未勾，本轮初始状态为 52/59，不是全部完成。
- **不能过度推论**：现有编排代码 `BoardComposeOverlay.vue:92-100` 对零成员已有提示，因此本轮不把“零成员”单独判成已复现新 bug；仍需补有真实成员的完整采纳故事及过期候选补救。
- **合同漂移**：ui-design 头注记载 10-25 批准两栏，但 User Journey 还要求右侧演进同步、Component Reuse 还写四区；tasks 4.4 已勾“遗漏优先排序”，新 spec 却明确禁跨日期遗漏优先。test-cases S5 还引用不存在的 `console/DailyReportDeskShell.test.ts`，实际文件在 `components/daily-report/`。
- **恢复**：先做旧能力/故事继承表，逐项写迁往哪里、主动撤除还是保留、真人如何完成；修好 High 后补完整故事证据，再统一合同。**不要通过删除失败断言或把合同改窄来证明迁移成功。**

## 旧功能去向表（避免把“换入口”一概算删除）

| 旧能力 | 本轮结论 |
| --- | --- |
| 五 tab、泳道卡片墙、DAG 同级视图 | 文档记载主动退役/重组，不自动回滚 |
| 阅读台最左历史栏/右演进栏，归档话题分组 | ui-design 10-25 头注记载用户批准；若要恢复需重新确认，而不是擅自推翻 |
| 正式泳道矩阵、逐日演进、mini 生命线 | 仍在，实页看到；完整链不等于所有分支已测试 |
| 关键事件标记/确认/不采纳 | 仍有入口；本轮未执行写操作 |
| 侦探墙、话题管理 | 仍有源码路径；后者入口过深，见 R-5 |
| 合并泳道、回填历史话题、候选启用 | 编排态保留；写入成败本轮未测 |
| 体检、构成、来源、版块编辑、新建 | 新壳有承接；部分只作静态核验，不宣称全部实页通过 |
| 匹配参数、升级建议、标签合并、标签池 | 更多菜单/弹窗保留，不能称都被删除；全局作用范围须讲清 |
| 研究候选、研究历史、QA/辩论 | research 路由仍承接旧面板；受其他 change 影响，不将其 diff 全归本 change |
| 页边注、划选问答、批注深链、当期 watch 定位 | 实现仍在；本轮未把入口存在当作整条故事已通过 |
| 旧 `/tags?board=&report=&annotation=` | 有路由反查/中间件承接；本轮未重走全部边界 |
| 删除版块 | 无新入口，R-2 |
| 文章正文、收藏、上下篇 | 工坊新入口承载缩水；全站搜索/阅读功能不等于已经迁回工坊，R-3 |
| 来源/辅助标签/质量/方向筛选及详细匹配计算 | 未等价迁移，R-4 |
| 跨期 watch 命中总览、阅读中直接切版块 | 丢失/降级，R-6 |
| 删除日报 | 没找到旧 UI 曾提供该能力的证据，不列成误删 |

## 产品取舍与恢复顺序

**P0，先恢复用户主权和内容可达**：R-1 无泳道也能读日报；R-2 删除版块安全入口；R-3 站内正文阅读。

**P1，恢复原工作流**：归档高级筛选/计算明细、直接话题管理、阅读台紧凑版块/追踪导航、历史日期交接、时间排序、全库动作范围确认。

**P2，再谈布局取舍**：保留雷达/矩阵可以，但必须把候选/未分类作为真实内容的一级分支，而非“还没整理好所以当不存在”；正文阅读与匹配审计应是同一文章的两种视图，不应互相替代。阅读台两栏可以继续，不必为了恢复功能重加四个常驻栏。是否恢复常驻演进/旧分组，需要用户重新决定。

后续验证至少要有这些完整故事，而不是只验控件：
- 无 active 泳道但有历史日报 → 从雷达进入 → 打开日报 → 读材料。
- 未入日报文章 → 日期/来源/标签定位 → 站内读完整正文 → 回原日期/筛选；收藏另作允许的写入验证。
- 历史矩阵格 → 同日 section → 同日归档 → 回原泳道。
- 读一个版块 → 找追踪命中期 → 定位 → 切另一个版块，保留可理解的阅读状态。
- 话题和版块管理的重命名/归档/恢复/删除，在隔离测试数据中验证，不能在真库试危险动作。

## 本轮运行证据与未覆盖

见 [本轮 UI 复审证据账本](ui-verification/complaint-review-2026-10-25/report.md)。原 `ui-verification/report.md` 未被覆盖；它的 pass 不替代本轮回归结论。

- 已有：雷达→1974 控制台；阅读台/分区滚动；手机经雷达切 50413；无 active 泳道空态与历史日报直达；归档日期预设→证据抽屉；API/同日排序核对；三视口截图目检。
- 未完整覆盖：dark 全套、3122 数据日实页、编排采纳/启用、研究和治理完整流程、侦探墙管理窗、watch 定位、页边注深链/划选问答、所有旧深链分支、日期下拉实际翻期。没有触发任何业务写入。
- **截图纠错**：`03-50413-1440.png` 实际 1280×633，不计 1440 验收；1440 以 `02-console-1440.png` 为准。50413 同场景的 1440 未覆盖。`05-...reader...` 为进场态，`11-...evidence-only...` 为白屏过渡，不作稳定缺陷证据。
- 没有改代码，所以未跑应用测试/build，也未因纯审查去重生成或重部署；仅检查报告格式、链接、截图尺寸和证据一致性。

## 部署后影响与需要的操作

- **本轮**：无部署，用户看到的行为不变，问题尚未修复；只新增审查/反馈/证据，并给 reference 活文档加复审风险入口。
- **后续恢复方向**：恢复入口和文章承载会让原有数据重新可用，不应要求用户先转正所有候选、重建话题、重生成日报或清库。
- **手动操作**：本轮不需要用户执行命令、改配置或处理数据。只需下一步决定是否按 P0/P1 恢复；已批准的两栏/日期平铺暂不擅动。
- **旧数据降级**：无 topic / 无 active 泳道 / 无体检快照的历史数据都应保留阅读和归档出口；证据/聚合缺失可显示未知或有限能力，不能用空舞台或 0 冒充没有内容。

## 2026-10-26 第二轮投诉复核（当前未解决项）

上文是 10-25 审查及后续修复的历史，不代表下列问题已解决。本轮只读实页与规范调查，完整证据、范围边界和返工建议见 [第二轮复盘](ui-verification/norm-audit-2026-10-26/report.md)。本轮没有产品修复或部署。

### R-11 2026-10-26【High】计算明细复用内联组件却丢失宿主布局，点击后在视口外
- **状态**：待处理
- 1440×900：归档 → 首篇文章 → 计算明细，`.match-detail-panel` y=937.625、position=static，父为 `.archive`，不在文章抽屉内且不可见。旧宿主的 sticky/宽度/滚动约束没有承接。
- `BoardArchiveView.vue` 将面板接在 AppDrawer 后。组件存在/API 成功不代表人能看到；原 A-43 的 pass 应撤回。修复需明确抽屉内详情或受控次级浮层及返回上下文。
- 证据：`ui-verification/norm-audit-2026-10-26-match-detail-after-click-1440.png` 与同目录 report.md A-1。

### R-12 2026-10-26【Medium】全局治理与重复新建入口的信息架构定位错误
- **状态**：待处理
- ui-design 承认系统/全库动作放板块内会误导，却把其新家设为板块“更多”；右轨又复制一组全局入口，新建板块在雷达/左栏/更多/右轨出现。
- R-9 添加“全库”与 confirm 只补风险提示，不等于职责定位正确。按本次用户要求，重审全局治理回总览；板块仅保留本板块动作，并为必要快捷入口说明角色。设计改动需重新审批。

### R-13 2026-10-26【Medium】复合事件的辅助标签与 description 解释链未完整承接
- **状态**：待处理
- 当前事件标签/配对/组合成员前端类型没有 description，组合成员在计算面板中主要被拼成公式字符串；只恢复规则与分数未完成用户对构成的理解故事。
- 需先明确事件、组合成员、辅助标签描述与本次匹配的展示/数据契约，不能只给规则名加 tooltip。静态确认，未声称本轮完整走完复合命中实页。

### R-14 2026-10-26【Medium】日报日期与板块切换仅恢复功能，目标宽屏操作区未完成产品验收
- **状态**：待处理
- 新增两个 select 和局部样式；已有根字号宽屏缩放，不能说完全没适配。但原 A-46 只有功能切换，不能证明控件层级、排布、长名称及展开态在目标视口达标。
- 本轮补 2560×1440 CSS 等效视口测量与截图，不自行将其升级为产品验收 pass；需按用户本次反馈修订操作区设计。

### R-15 2026-10-26【Medium】编排小圆点与混合焦点推荐不等价于旧关系浏览及待确认引导
- **状态**：待处理
- 旧版区分非泳道匈牙利 similarity DAG 与 identity 延续；新版初始不画边，焦点将两类关系混合排序。71 节点/0 初始线、点击后 1 identity 实线，不能证明旧浏览故事保留。
- 文档记载过“关系图降为编排辅助”，因此本轮不武断定性为算法误删或未经批准删 UI；但必须重新确认跨泳道相似、同话题延续、待确认关联各自的新承接，不能用矩阵连续性替代所有关系语义。新手引导应覆盖从识别事件到选料/命名/保存的整条路径，不只补一句提示。

### R-16 2026-10-26【Medium】证据存在性门禁被当作产品交付判定，修复复验仍停在入口可达
- **状态**：待处理
- 核验前 trace 脚本报告 54 pass、52 张截图齐全，但 A-43 已被实际点击反证。A-35/36/46 只能证明入口/功能存在，不能替代定位、展示和宽屏产品判断。
- 现有规范已经要求展示与故事复核，应先执行。后续规范补强应聚焦动作作用域/主入口、复用宿主契约、旧信息语义去向；门禁若改造，须区分留痕齐备与必验故事最新结果，不能让脚本承担主观美感判断。详见复盘第三至第五节。

## FB6 总览阶段：独立增量审查（2026-10-26）

审查者为独立 reviewer（openai-codex/gpt-6.1-sol:high，run 9c1a7f06）；逐段对照父基线副本与新增源文件，只读未重跑门禁。此阶段只总览获准，8A.4仍未完成；以下不关闭R-11…R-16等旧问题，也不改变旧UI验收账本。

### R-17 2026-10-26【Medium】首次请求失败与业务空态同时出现

- **状态**：已修复（18组件目标测试及真实静态故障/重试/空集复验）
- 复核结论：通过。初稿BoardRadarView.vue:331–339丢失baseline的`!error`，混淆首次失败与成功空集；该事实保留。
- 修复：主业务空态排除error、首次失败统计隐藏/中性报头；新用例保留旧列表刷新失败可读，且补齐右栏三态。
- 中间实页仍在阅读侧栏误报“今天还没有日报/暂未触发升温条件”，失败图overview-stage-2026-10-27/overview-failure-injected-1440.png保留。最终closure实测head=今日雷达、error=true、empty=false、rail=false、falseBusiness=false；原生重试恢复4关注/1升温。克隆真实HTTP外壳仅令boards=[]时empty=true/error=false/rail=true。最终依据同目录overview-failure-injected-closure.png、overview-empty-injected-envelope-closure.png及report.md，不以单测绿替实页。

### R-18 2026-10-26【Medium】新增视图字段边界与组件规模未满足规范

- **状态**：已修复（Radar边界、真实raw接线、规模及本阶段类型0已复核）
- 复核结论：通过。初稿位置：BoardRadarStory.vue:14–115、boardRadarHelpers.ts:28–47、BoardRadarView.vue（1122行/约31.8KB）。新增Story/helper继续消费snake_case和数值ID；主组件仍超500行/~15KB规则。基线违规不是新增代码豁免。
- 最小修复：仅Radar API分支建立camelCase/字符串ID客户端边界，后端JSON协议不变；测试真实raw响应→客户端接口接线，而非仅更换mock。按加载导航/治理宿主/报头阅读展示等内聚职责拆分，不搬成另一巨型文件，不改其它API或产品区域。

### R-19 2026-10-26【Low】已批准文案的规格同步与命令记录欠账

- **状态**：已修复（delta文案、实际命令和测试分拆已更正，历史结果保留）
- 复核结论：通过。初审workshop-radar delta曾要求旧“今天要看/约2.7倍/今天没有明显升温的版块”，已批准原型与实现采用“热点/2.7×/暂未触发升温条件”。允许仅同步该delta显示文案，不改阈值/统计/来源或退回旧UI。
- 初次实际为15组件+7helper=22测试，工具结果确认22 passed及eslint exit=0；原报告13+9分拆有误，ESLint记录省略后两文件目录。须按完整可执行命令更正记录，保留初次22通过的历史事实。现有配置忽略Vue已诚实记录，不称Vue lint通过，不新增lint工具。
- 父裁决：先完成R-17/R-18及文档纠正，再统一8A.4；不得据初次目标单测绿色宣布总览交付或整个change通过。

### R-20 2026-10-27【Medium】窄屏父级scoped列规则覆盖子行，关注强度溢出视口

- **状态**：已修复（真实静态实页几何复验）
- 复核结论：通过。原始390视口document.scrollWidth=514，rd-story实际宽361.22但计算grid仍六列，关注强度列right=513.95；375暗主题同现象。不是原型/fixture验证。父限定表头选择器后，实际390/375均为三列，scrollWidth分别390/375；1440/1920/2560仍无整页横溢，旧失败图保留，修后图见同目录*-final.png。
- 证据：ui-verification/overview-stage-2026-10-27/overview-editorial-390.png、overview-dark-375.png（保留失败图）。父View的`.rd-grid` scoped样式可命中子Story根，覆盖其移动端列规则。
- 最小修复：仅总览CSS限定表头规则或共享本页列变量，避免父scoped污染子根媒体查询；不能用overflow-x:hidden盖住字段或删关注强度/入口。无TS/API行为变更，不重跑全套门禁；实际构建部署后复测375/390及1440/2560。
- 旁注：匹配规则dialog实际中心point命中，取消可返回；Esc关闭有离场动画，应wait真实消失后断言，不能用即时query误报失败。AppDialog既有role/focus能力不在本轮扩改。

### R-21 2026-10-27【Medium】自设120rem阅读列上限偏离已批准4K控制台宽屏原型

- **状态**：已修复（真实宽窄实页复验）
- 复核结论：2560下radar.width=2560，无整页横溢，1440/1920/375/390亦通过；见ui-verification/overview-stage-2026-10-27/overview-editorial-2560-closure.png及同目录report.md。初稿BoardRadarView.vue `.radar { max-width:120rem; margin:0 auto }` 把2560 CSS视口的总览压到约1920宽，两侧约320px空边；批准原型overview-final-2560.png使用宽屏主舞台，当前overview-editorial-2560-final.png明显缩为居中阅读列。1440正常不能代替2560验收。
- 最小修复：仅本页移除该上限，沿workspace满可用宽度和原有gutter；不改全局AppPageShell或其它页面，不强拉行高填满空白。不改统计与数据，正常态分区/字段保持。全局菜单名应对齐已批原型“全局治理”，不搬入原型的离线导航/调试壳。

### R-22 2026-10-27【Medium】观察窗静默预览后的同值提交遗漏URL写入

- **状态**：已修复（父复核committedRange分支；40项定向回归通过，原生拖选留G2/G5复验）
- 父复核源码：useObservationWindow.applyObservationRange 在 writeQuery:false 预览时已更新 observation；松手用默认选项再次提交同一范围时 unchanged=true，于是跳过 writeRangeQuery。G1交接明确建议G2使用此流程，因此实际拖选深链无法落盘，影响返回恢复。
- 修复判据：预览不写URL，最终同值提交须恰写一次正确start/end，保留days/lane/report/annotation；提交后再次重复不再写；非法输入不改已提交范围。可修条件或提供显式提交API，但须更新G2交接并加实际状态单测，不实现G2 UI。

- **修复记录（2026-10-27 G1 实现批）**：`useObservationWindow` 引入 `committedRange`（URL 已反映的已提交范围；深链恢复对齐、切档/切板块重置、导出供 G5 返回状态）；`applyObservationRange` 写 query 条件从「与 observation 同值」改为「与 committedRange 不同」——预览（writeQuery:false）只更新显示，松手同值提交恰写一次（保留 days/lane/report/annotation），重复提交幂等，非法输入 observation/committedRange 均不动。G2 交接与 §9 示例已同步（preview→commit 两段式）。新用例：`R-22：预览不写 URL；同值松手提交恰一次（保留 lane）；重复幂等；非法不改已提交范围`、`R-22：深链恢复后预览他值再提交，恰一次写新值`。命令：`cd front && pnpm test:unit app/features/tags/composables/useBoardConsole.observation.test.ts app/features/tags/composables/useBoardConsole.test.ts --maxWorkers=2` → 40/40 绿；`pnpm exec eslint app/features/tags/composables/useObservationWindow.ts app/features/tags/composables/useBoardConsole.observation.test.ts` → exit 0。

### R-23 2026-10-27【Low】非法days提示固定声称回退14日却保留了其他当前档

- **状态**：已修复（父复核动态当前档提示及初始14日分支；定向状态回归通过）
- 父复核源码：外部URL days非法时watch保留当前档、不发请求，但提示固定“已回退14日档”；当前合法档为7/30/365时文案与实际状态不符。
- 修复判据：区分首次非法参数回落默认14与已有合法档时保留当前档；告知实际采用档位，且不发送非法请求。定向测试覆盖外部非法days与当前非14档。

- **修复记录（2026-10-27 G1 实现批）**：外部 days watcher 非法文案改为「days 参数不合法（<raw>），保持 <当前档> 日档」（保留当前合法档、不发非法请求）；初始非法（mount 时 URL days 非法）仍回默认 14，useBoardConsole 初始 parse 文案保留「已回退 14 日档」。新用例：`R-23：外部非法 days 保留当前合法档并告知实际档；不发非法请求`（30 档下改 days=8 → 保持 30 + 文案含「保持 30 日档」+ 零新请求；修正回合法档 notice 清空），OX-9 初始非法分支断言强化（含「已回退 14 日档」）。命令同 R-22 批次：40/40 绿 + eslint exit 0。

### R-24 2026-10-27【Medium】编年切到无分区或未取到日仍可能沿用旧报告引用

- **状态**：已修复（父复核display上下文gate、动作与失效分支，45项定向复验通过；完整实页留8B.4）
- 父复核LaneChronicle.vue：非content上下文watch直接return，current仍取reading.current；canOpenReport只检查旧reportId，onOpenReport把旧reportId与新props.selectedDate组合。正文虽显示gap/unavailable，页头仍可能启用旧报告动作，状态横幅也可能属于旧日。
- 修复判据：ready→gap/unavailable/未选日期/同日换无分区泳道后，旧current不进入新上下文的页头/横幅/正文，旧报告不能配新日期发出；在途旧响应也不能重新启用。board/lane/date/分区上下文须一致，重新进入content可正常读取。补实际组件负向用例。
- **修复记录（2026-10-28 G4 实现批）**：①`useLaneDayReading` 读取上下文增加 `sectionKey`（从当日 sections 派生，调用方无法错配），跨上下文判定 = laneKey/date/sectionKey 任一变化（先清显示再加载）；新增 `suspendDisplay()`（离开可读上下文时失效在途显示，不动成功缓存/在途请求）；finish 组装 current 携带 sectionKey。②`LaneChronicle` 新增 display gate：`reading.current` 仅在与当前 props 的 lane/date/sections 完全一致时进入页头/横幅/正文/动作；watch 离开可读上下文即 suspendDisplay，watch source 纳入 sectionKey（分区集变化触发重读）；`canOpenReport`/`onOpenReport` 均基于 display，旧 reportId 不能配新日期发出；四类横幅拆出 `LaneChronicleBanners` 子组件同受 gate。③负向用例：旧成功→gap/unavailable/未选日期/同日换无分区泳道（横幅/正文隐藏+旧报告禁用+零新请求+无 openReport emit）、在途旧详情晚到不重新启用、重入 content 缓存命中恢复并发出正确 `{date,reportId}` 对、同日 sections 变化不冒旧分区；composable 层 suspendDisplay 后成功缓存保留/重入不重发、sectionKey 变化后 force 失败不 stale 保留旧分区内容。命令：`cd front && pnpm test:unit app/features/tags/composables/useLaneDayReading.test.ts app/features/tags/components/console/LaneChronicle.test.ts --maxWorkers=2` → 45/45 绿；`pnpm exec eslint` 同批 3 个 TS 文件 exit 0。

### R-25 2026-10-27【Low】聚合已有分区时索引未匹配被表述成“这一天没有日报”

- **状态**：已修复（父复核中性匹配提示及定向断言，不冒称无日报）
- 父复核LaneChronicle no-report横幅及openReportTitle：聚合已返回该日section，报告列表没找到时仍直称无日报。读取覆盖/更新/暂不可匹配与真实不存在不能混淆。
- 修复判据：中性说明未匹配当日日报详情并可重试，不声称业务无日报、不自动生成或借别日数据；匹配后恢复正确入口。
- **修复记录（2026-10-28 G4 实现批）**：no-report 横幅改为「未匹配到当日日报详情，暂只能显示聚合返回的标题。」+ 刷新按钮（force 重读索引+详情）；openReportTitle 改「未匹配到当日日报」；不自动生成、不借别日（原行为不变，仅文案与入口语义修正）。用例断言中性文案且不含「这一天没有日报」。命令同 R-24 批次 45/45 绿。

### R-26 2026-10-27【Low】编年默认摊开三项计数且未知总量可能显示为已知数字

- **状态**：已修复（简短合计、折叠计数说明及未知值定向断言通过，真实键盘留8B.4）
- LaneChronicleHeader默认直接显示桶峰值/当日合计/桶数，偏离获批标题优先稿的简短合计+按需计数说明；metricsText直接用m.total，还需检查hasUnknown，不能把未知显示成0或完整合计。
- 修复判据：默认只简短真实合计，其余进可键盘展开的计数说明；未知标待核/问号或明确已知部分，不冒完整总量。组件测试确认默认折叠、展开后口径仍可查。
- **修复记录（2026-10-28 G4 实现批）**：默认只显 `当日合计 N 条`（简短真实合计）；桶峰值/桶数/口径移入 `<details>`「计数口径」（原生 summary 键盘可聚焦/展开，默认折叠，口径不删）；`dayCellMetrics.hasUnknown` 处理：全未知标 `?`、部分未知标 `≥已知部分`，details 附「部分桶线索数未知，不补 0」。用例：默认折叠（无 open 属性）/点击展开后口径可查（桶峰值 3 条·主题桶数 2 个）、全未知 `当日合计 ? 条`、部分未知 `当日合计 ≥3 条`。命令同 R-24 批次 45/45 绿。

### R-27 2026-10-28【High】壳层全局根字号缩放越过获批角色字号基线

- **状态**：已修复（独立代码复核通过，父准入构建/只读S9；实页与真实生成时序不在本条代码结论内）
- Shell:367–377写html.fontSize=clamp(15px,.86vw,30px)，2560下推算根22px、左右轨约374/462px。不是实页测量；证据与父裁决见[独立报告](review-matrix-g5-code.md)。删除全局字号写入/恢复，局部按角色token；补不改html字号回归，纠正§13伪依据。
- **修复记录（2026-10-28 R27–R33 定向批）**：删除 `BoardConsoleShell` 的 `onMounted/onBeforeUnmount` 全局 `document.documentElement.style.fontSize` 写入/恢复及生命周期导入；角色字号保留现有局部 rem/token，不做整页倍乘或 <=16px 守卫。§13 改为「壳级弹层/刷新编排（不写全局 html 根字号）」，不再声称该公式来自 ui-design。负向用例：挂载/卸载后 html `font-size` 保持预设 `17px`。未改 design/prototype。

### R-28 2026-10-28【High】新日报生成后最新元信息不刷新导致旧锚排除新日

- **状态**：已修复（独立代码复核通过，父准入构建/只读S9；实页与真实生成时序不在本条代码结论内）
- 元信息仅mount/换board读取，生成关闭只load A，旧reportAnchor压过新A日。补元信息刷新/重试，保0active候选日报，不仅max active日期掩盖问题。mock验证新日头部与源axis末列，不真生成。
- **修复记录（2026-10-28 R27–R33 定向批）**：`useConsoleShellData.loadReportMeta` 改为保留同板块最后成功值、失败置 `reportMetaStale`（从未成功才 `reportMetaUnavailable`），暴露 `reportMetaLoading`；`useConsoleRefresh` 串行「元信息→A」。`DailyReportGenerateDialog` 在真实 `done` 上升沿 emit `completed`；壳层在 completed、对话框关闭（cancel）与顶栏显式刷新均调用同一刷新链；顶部刷新按钮在 stale/unavailable 显示重试语义。用例：生成完成→新日期头部+`matrix-cell-10-新日`；0 active→新日期头部+空态；关闭→刷新；失败保留旧值标陈旧→重试恢复新日；`DailyReportGenerateDialog` 单测 done 上升沿 completed。不真生成。

### R-29 2026-10-28【Medium】历史池失败未接单独错误与重试入口

- **状态**：已修复（独立代码复核通过，父准入构建/只读S9；实页与真实生成时序不在本条代码结论内）
- G1已交retryPool/poolError/poolStaleNotice但Stage未透传，A重试不恢复B。补独立B重试/错误陈旧提示；仅B重发、名单范围泳道不变，成功恢复，不清错冒成功。
- **修复记录（2026-10-28 R27–R33 定向批）**：`needsPool/poolError/poolStaleNotice/retryPool` 从 G1 透传到 Stage，历史池错误/陈旧提示独立于 A 错误，按钮仅触发 `retryPool`。`ensurePool` 在途防重（不叠发）、成功才清 error/stale，失败保留旧池并标陈旧；重试期间提示保留、按钮禁用（不清错冒恢复），无自动重试风暴。用例：B 失败→提示+仅重发 365→A 调用数/选中泳道/14 日档不变→成功恢复历史格；G1 另测 stale 保留旧池与成功清提示。

### R-30 2026-10-28【Medium】返回上下文同tick双replace可能丢观察范围URL

- **状态**：已修复（独立代码复核通过，父准入构建/只读S9；实页与真实生成时序不在本条代码结论内）
- ReturnContext连续applyRange/selectLane基于陈旧query，真router异步提交时后写覆盖start/end，同步mock掩盖。合并一次有效query或确实等导航，补异步router测试及实页返回刷新，保显式/非法URL规则。
- **修复记录（2026-10-28 R27–R33 定向批）**：返回恢复改为「先无写回填 observation/lane 状态，再合成单次完整 query（days/start/end/lane，删 focus）并 await router.replace」，不再逐项写 URL；G1 `selectLane` 增加可选 `writeQuery:false`，不改默认行为。URL 显式 `days/start/end/lane` 及非法项仍优先，不覆盖为缓存值。用例：异步 fake router 同一次返回只提交 1 次且四键齐全；另测非法 days + 显式范围/lane 不被缓存覆盖。

### R-31 2026-10-28【Medium】小视口保存滚动取到非滚动stage的0

- **状态**：已修复（独立代码复核通过，父准入构建/只读S9；实页与真实生成时序不在本条代码结论内）
- stage.scrollTop=0不会触发??window.scrollY；<1440与手机返阅读位丢失、focus可能拉走。按真实宿主存还并preventScroll；补windowY=120与stage对照、1280/390实页。
- **修复记录（2026-10-28 R27–R33 定向批）**：用 `scrollHeight > clientHeight` 判定真实滚动宿主，二选一（stage 或 window）保存/恢复，不再 `stage.scrollTop ?? window.scrollY`；stage 非滚动时存/还 windowY。焦点恢复统一 `focus({ preventScroll: true })`（返回上下文与对话框关闭）。用例：非滚动 stage + windowY=120 存 120；桌面可滚 stage 存 stageY=80；window 宿主恢复调用 `scrollTo(0,120)`。1280/390 实页留 S9。

### R-32 2026-10-28【Low】设置与归档指引仍指向已撤全局入口

- **状态**：已修复（独立代码复核通过，父准入构建/只读S9；实页与真实生成时序不在本条代码结论内）
- 设置/归档旧文案仍说顶部更多或治理右轨。精准改实际总览全局入口/本板块管理，不重绘其它页，补文案断言。
- **修复记录（2026-10-28 R27–R33 定向批）**：仅改两处文案——设置提示指向「今日雷达（总览）的版块治理 / 本页随板块管理进入」；归档方向校验指向「当前版块板块管理 → 构成与治理」，日报状态指向「日报生成或当前版块板块管理」。补 BoardSettingsForm/BoardArchiveView 文案/反旧词断言，不重绘页面。

### R-33 2026-10-28【Low】候选人工激活入口误开新建泳道编排

- **状态**：已修复（独立代码复核通过，父准入构建/只读S9；实页与真实生成时序不在本条代码结论内）
- 右轨候选按钮emit openCompose而非TopicManageDialog。保人工激活语义，接本板块话题管理，不以改名解释错误动作；补落点/board scope，不真激活。
- **修复记录（2026-10-28 R27–R33 定向批）**：`BoardContextRail` 候选 action 改 `openTopics`，经 Body/Workface 透传到壳层打开既有本板块 `TopicManageDialog`（boardId），不再开 `BoardComposeOverlay`；新建泳道原顶栏入口保留。用例：rail 点击后 topic dialog open + boardId=1974、compose 未挂载；workface context 透传 openTopics。未执行真实激活。

### R-34 2026-10-28【Medium】返回深滚动在编年异步增高前被钳制

- **状态**：已修复（源码复核与run2真实慢链/用户取消/1440、1280、390滚动通过；run3无相关运行码变动）
- 首轮S9 F1：1440 stage 382→302且最终max仍382；1280 window 693→414。nextTick后单次恢复早于详情增高。须等正确上下文内容可恢复后应用，用户后续操作/切上下文/卸载应取消旧恢复，不造占位空白/魔法sleep。原证据与回退见ui-verification/matrix-stage-2026-10-28/parent-triage.md。
- **修复记录（2026-10-28 R34–R39 定向批）**：新增 `console/restoreScroll.ts` 有限帧恢复（默认 180 帧、无固定 sleep/空白撑高）：立即应用后仅在未达目标且未被取消时重试；按 `scrollHeight>clientHeight` 选 stage 或 window 唯一宿主。取消来源：wheel/touchstart/pointerdown/keydown、目标 board/lane/date/days/range 变化、切板块、scope dispose；焦点 `preventScroll`。负向用例用 clamp 假宿主构造 302→382（重试恢复 382）、用户取消、上下文变化、内容终短有界耗尽；shell R31 window 宿主 `scrollTo(0,120)` 保留。真实 1440/1280/390 往返留 K3。

### R-35 2026-10-28【Medium】切到已缓存档时读回旧日期范围

- **状态**：已修复（deferred负向及run2真实365/14切档通过，run3烟测回归）
- 首轮S9 F2：30档自定义10-15..16→已缓存365，URL移除start/end但可见仍2列。源加载快于router提交时重灌旧query；须切档默认范围正确，保自由范围不自动改档与R22/R30语义，补异步router+缓存命中测试。
- **修复记录（2026-10-28 R34–R39 定向批）**：`onTierSwitch` 增加 `writeQuery/restoreQuery` 语义；用户档按钮/返回恢复置 `suppressQueryRestore`，源加载后直接采用目标档默认区间，不再回读 router 尚未提交的旧 start/end；外部 URL `days` 变更走 `fromUrl`（不重写 days、保留显式 start/end 恢复）。用例：异步 router 延迟提交 + 365/14 缓存立即命中，验证 sourceDays、默认 observation、visibleAxis 长度、UI range、URL 无旧 start/end；shell 同时验证矩阵轴 365。自由范围恰 7/30/365 不切档（OX-1）与 R22/R30 用例照跑。
- **第二轮返修（2026-10-28，待父复核/实页）**：shell 负向改为手动 deferred 导航（不靠一微任务）：保持旧 `days=30/start/end` 未提交、缓存续体已跑后先断言 UI range/轴已是 365 默认且 URL 仍旧；再释放导航，断言 URL 变 `days=365`、无 start/end，UI/轴仍一致。外部显式 range 与 R22/R30 用例保留。

### R-36 2026-10-28【Low】短窗30列仍横滚，违反MX-6明确合同

- **状态**：已修复（run2实测30/31单屏、32+内滚；run3再测30列845/845无横溢）
- 首轮F3：1440下sw1315/cw845。父核第22步与MX-6明确仅>31列容器横滚、≤31单屏。修短窗响应式列，保日期/数值清晰与完整可查；不改合同迎合实现，不整页缩放。
- **修复记录（2026-10-28 R34–R39 定向批）**：`ObservationMatrixGrid` 以 `axis.length > 31` 分 `is-scrollable`：≤31 为单屏 `minmax(0,1fr)` 网格、日期两行 `MM`/`DD`（完整日期在 title/aria），不制造容器横滚；>31 保留内部横滚、sticky 标签列与短日期。移除原始终 `min-width:max-content`。用例：30/31 无 scrollable、32/365 有；未改 spec/test-cases，真实 1440 几何留 K3。

### R-37 2026-10-28【Low】手机刷选手柄命中宽度仅3.15px

- **状态**：已修复（晚SVG真实接线及run2冷首屏24px命中通过；极短窗物理精调限制明示，不冒称真机）
- 首轮F4真实命中区3.15×81.4px；增加至少24CSSpx命中面，短范围/端点边界左右均可抓，不能透明区重叠盖掉一端或破坐标映射。原生pointer/取消与键盘须保留。
- **修复记录（2026-10-28 R34–R39 定向批）**：新增透明 `.orc-hit` overlay（`brush-hit-start/end`），24px 宽、起上/止下各 50% 高，短窗/单日不互相覆盖；`handleHitStyle` 用 CSS min/max 将端点夹回绘图区；坐标仍走实际 plot rect，pointerdown 手柄不改起止（无跳变），SVG 手柄键盘/aria 保留。用例：24px 样式、上下分区、单日、端点夹取、拖 overlay 按真实映射 commit。真实 elementFromPoint/手指留 K3。
- **第二轮返修（2026-10-28，与 R40 合并；待父复核/实页）**：上下分热区改为按真实像素生成（`RangeHandleHits.vue`）：间距 ≥24 各自 24px 区；<24 合并为一个 24+间距的区，指针按最近可见端解析，单日同 x 时按上下半区；`grabOffset` 保留抓取偏移，首动不把端点对齐指针；SVG 键盘/aria/取消保留。用例：375 等效 7/14 日左右可见线中点各自命中断点、外缘 6px 按下后移动 1px 不跳、单日上/下、两池边界、非重叠区拖动映射。真实 elementFromPoint/手指留 K3。
- **生产接线补齐（2026-10-28 第三轮，待父复核/实页）**：`usePlotWidth` 增加 `watch(plotRef,{flush:'post'})`，异步 SVG 出现/消失/替换时重新测宽、断开旧 RO 并观察新 SVG；卸载清理。Controls 负向从初始 `bounds=null/range=null` 无 SVG 开始，数据晚到后确认真实 `brush-hit-overlap` 且 width 非 0；`usePlotWidth.test` 覆盖 SVG 出现/消失回 0。未改最近端/offset 逻辑。
- **N1 收口（2026-10-28，待父定向实页）**：run2 发现 24≤gap 时端点被夹到绘图区边缘会使两个 24px 区实际重叠，DOM 后者 end 抢事件。`buildHitRegions` 改为先算夹取后的实际区间：只要两 24px 区相交（不论原始 gap）就返回一个合并区，`resolveHitEndpoint` 按最近可见端解释；接口/视觉/24px/坐标/grabOffset 不变。负向覆盖左夹偏向 end、右夹偏向 start，及旧短 7/14/单日/池边界/非重叠区回归。

### R-38 2026-10-28【Low】本期站内原文弹层缺对话语义与初始焦点

- **状态**：已修复（run2/3真实dialog语义、初焦、关闭回触发点/滚动不变；旧90vw/搜索/iframe未重绘）
- 首轮F5。90vw为AppDialog明确存量兼容项，本次不重绘reader/改全站宽度；只补当前matrix调用的语义与初始焦点，可选小API默认保旧，保搜索高亮/iframe高度/阅读行为与关闭焦点恢复。
- **修复记录（2026-10-28 R34–R39 定向批）**：`AppDialog` 新增默认关闭的 `semanticDialog/dialogLabel`：true 时 role=dialog/aria-modal/label/tabindex 在 dialog 本体，nextTick 聚焦容器（preventScroll）；`ArticlePreviewModal` 新增 `dialogSemantics` 并透传，只有 matrix Shell 启用。90vw/搜索高亮/MutationObserver/iframe 85vh/阅读行为未改，关闭回触发点仍由 Shell preventScroll。用例：AppDialog 默认无 role/焦点、opt-in 角色与初始焦点；ArticlePreviewModal 默认 false、matrix true 才透传；原高亮/高度测试照跑。

### R-39 2026-10-28【Low】编年业务标签被强制大写

- **状态**：已修复（run2真实computed transform为none，Agent原样；系统kicker仍uppercase）
- 首轮F6中Agent显示AGENT；保API业务标题/标签原大小写，系统kicker排版可保。下一轮用computed text-transform与真实画面核对，不以DOM原text就宣视觉逐字。
- **修复记录（2026-10-28 R34–R39 定向批）**：`LaneChronicleHeader` kicker 拆为系统 `.lch-kicker-system`（保留 uppercase/letter-spacing）与业务 `.lch-lane-label`（原大小写、ellipsis/title）；父级 `.lch-kicker` 不再 uppercase。用例断言 `Agent 架构重构…` 保持原样且系统 kicker 仍「当日线索」。真实 computed text-transform 留 K3，未以 DOM text 自证视觉。

### R-40 2026-10-28【Medium】扩大手柄热区后短窗误抓端且首拖可能跳日期

- **状态**：已修复（run2短窗/首动通过；N1夹边实际交叠再收口，run3三视口原生pointer左右带归属与提交取消通过）
- 独立代码复核：两视觉线同高，短窗间距<24px时上下透明区均覆盖两线，按任一线中点模式由y决定，必有一端被抢；手机7/14日与桌面7日可触发。无grabOffset还会在热区边缘首动对齐指针，跳若干天。按真实可见端点正确判端、保24px可操作区与单日/边界、保持抓取偏移；补375等效短窗两端中点/外缘1px/单日/端点负向。不得裁小热区或弱化合同蒙绿，实页留K3。
- **修复记录（2026-10-28 第二轮，待父复核/实页）**：`buildHitRegions` 以真实像素宽度决定“两独立 24px 区”或“一个 24+间距合并区”；`resolveHitEndpoint` 在合并区按最近可见端点解析，单日同 x 以 y 上下半区区分；`BrushGesture.grabOffset` 让 start/end 首动保持相对偏移。`ObservationRangeControls` 用 `usePlotWidth` 读取真实 SVG 宽并只把 region kind 交给父层，父层用真实 plot rect 解析端点。用例覆盖 375 等效 7/14 中点、外缘 6px+1px 首动、单日上/下、两池边界、非重叠区；未缩小 24px 合同。

### N2/N3 2026-10-28 run2 留账

- **N2（Info，触控物理限制）**：375 上 1px≈1.15 日，极短 1–2 日窗两线相距近，24px 合并区无法用手指精确区分单线；精调路径为日期输入/键盘，**不是数据错误、不改映射为每 px 1 天、不新增 UI**。已在 ui-navigation 链路 5 记明。
- **N3（Low，空摘要文案）**：产品保留「暂无摘要记录」作为 loaded 的 `thread.summary=''` 等价说明，不改工程味 copy；新增组件断言 loaded 空摘要展开显示该文案，未取到报告不冒该空态；test-cases-matrix-stage 变体 7 措辞同步为「暂无摘要记录」并注明不等同未取到。

### R-42 2026-10-30【Medium】报头「主页」入口归位的档位口径无自动化锚点 + 8F 缺本轮验证账面 + 实页证据少两档

- **状态**：已修复（实现层可放行；整 change 归档门禁另计，不因本条洗绿）
- **来源**：8F / FB-13 独立只读 review（子线程，绑定 `minimax-cn/MiniMax-M3.1-Flash:max`）。审查结论为「需修」，实现层本身判通过（需求符合、三档 flex 未破、`.rd-more summary` 既有规则未改坏、a11y 与既有回归面无问题）。
- **M1（Medium）档位口径零自动化锚点**：新用例只断言两者「共用同一个类」，把 `.rd-action-btn` 的 min-height/字号/边框改掉照样绿；而仓库 eslint 配置未含 `**/*.vue`（实测该 .vue 判 "File ignored because no matching configuration was supplied"、退出 0），happy-dom 也不应用 scoped CSS → 模板与样式对 lint/单测双重失明（与 §5.3 记录的 2026-09-07「误删 scoped 规则全链路失明」同款形状）。
  - **修复**：新增源码样式锚用例（`readFileSync` 读 `.vue`，仓库已有 14 处同款先例），锚住用户确认的四项档位值（min-height 2rem / font-size 0.7rem / border 1px solid `--color-border-medium` / hover 边框与文字变 `--color-accent`）、治理菜单三条标记规则（`cursor:pointer` / `list-style:none` / `::-webkit-details-marker`）与 `.rd-more[open] summary` accent 态；并加两条反向锚（`.rd-issue-actions` 不得 `row-reverse` / `order:-`，防 DOM 序不变而视觉左右互换）。
- **M2（Medium）8F 段缺本轮验证结果账面**：8F.3 勾了 lint 却无 lint 结果，验证节只有「期望命令」没有实际结果节，与 8E 的记账口径（`### 8E 本轮验证结果`）不一致。
  - **修复**：tasks 验证节补 `### 8F 本轮验证结果（2026-10-30，不等于整 change 验收）`，逐条记命令 → 实际退出码/用例数，并写明 lint 的真实覆盖口径（只覆盖 `.ts`，`.vue` 靠样式锚 + 实页兜底）。
- **M3（Medium）实页证据缺 1920×1080 与 390×844**：本 change 声明 `ui-impact: major`，layout.md 对 major 硬性要求 1440×900 与 1920×1080 双桌面视口、触窄屏另加 375/390；8F 又确实动了窄屏行为。
  - **修复**：补齐四档机械断言（几何 + computed 档位 + `scrollWidth === clientWidth`），证据 `ui-verification/radar-home-8f/report.md`（A-8F-1…8）与 5 张截图。
- **L1（Low）`.rd-home` 退化为零引用标记类**：保留但在 `.rd-action-btn` 注释块注明「`.rd-home` 目前仅作语义标记（无独立样式），改档位请改 `.rd-action-btn`」，防后人误判漏写样式而另起一档。
- **L2（Low）`.rd-heading` 变单子元素后 `display:flex; gap` 空规则**：删 `display:flex`/`gap`，**保留** `align-items:flex-end`（标题与操作区基线对齐，属视觉红线）与 `min-width:0`（防溢出）。
- **L3（Low）用例未覆盖「治理菜单与新建版块仍在操作区」**：补一条结构断言（主页 → 治理 → 新建版块 的兄弟元素序），不依赖 happy-dom 不实现的原生 disclosure 行为。
- **L4（Low）「紧贴」只锁 DOM 序**：并入 M1 样式锚（反向锚 row-reverse/order），因为「主页在治理左边」本身是用户诉求。
- **L5（Low）FB-13 缺逐字原话**：补 `**用户原话**` 并与标题行去重（原话：「tags叙事工坊总览界面我觉得主页这个标签和别的分的太开了，放全局治理左边是不是更合理」）。
- **L6（Low）8F.1「（先红后绿）」措辞与 §2 现行口径不合**：改为「用例先行」，红态事实改记入验证节（实现前实测 4 例中 3 红 1 绿，红在「不在操作区」「不在标题区」「未共用档」三处）。
- **L7（Low）delta spec 未把用户确认的档位值写进合同**：Requirement 正文补同档口径四值（与 M1 样式锚配套承保）。
- **L8（Low／存量，不属 8F）`.rd-more-menu { z-index: 40 }` 是未登记的手写层级**：该组件相对 HEAD 为 untracked 新文件、无法 diff 佐证为 8F 引入；按 layout.md「仅在主动重构该组件时收敛」的口径，8F minor 改动不构成新增欠账。留账给整 change 归档时顺手登记进遗留手写层级表，不在本轮擅改共享文档。

### R-43 2026-11-01【Medium】后端 `window.days` 回显口径与契约字面冲突

- **状态**：已修复
- **来源**：新建泳道编排后端 review（子线程，`zai-coding-cn/glm-5.3:max`，用户 2026-11-01 当场指定模型）；brief 见 `review-lane-compose-backend-brief.md`，完整报告见 `review-lane-compose-implementation.md` §二。
- **M1**：契约 §1.1 写「`window.days` 为请求的 days」，实现回显**生效值**（`days>90`→90、`days<=0`→0、`from/to` 档→`to-from+1`）。二者字面冲突。
  - **修复**：契约口径定稿为「回显**生效**窗口天数（与同对象 `from/to` 自洽）」，实现与 D10 断言不动；契约 §1.1 已改。

### R-44 2026-11-01【Low】白盒表 D6 字面例与「跨度 ≤90 天」规则自相矛盾

- **状态**：已修复
- **来源**：同上（审核 M2）。
- **问题**：`test-cases-lane-compose-implementation.md` §3.2 的 D6 字面输入 `from=2026-10-01&to=2026-12-31`（跨度 92 天）先撞「超 90 天 → 400」规则，与该行想考查的「`to` 晚于最新日报应允许」互斥；**实现正确，是用例表自相矛盾**。
  - **修复**：D6 字面输入改为 `from=2026-10-20&to=2026-12-31`（72 天）；补 D3b 记录规则优先级（跨度 → 窗外 → 其它）。

### R-45 2026-11-01【Low】`window.from/to` 可为 null 未在契约定义

- **状态**：已修复
- **来源**：同上（审核 L1）。
- **问题**：无已完成日报或 `days<=0` 全历史档时 `from`/`to` 为显式 JSON null，契约只规定了 `latest_completed_date=null`，未定义前两者形态。
  - **修复**：契约 §1.1 补「无锚点/全历史时 from、to 为 null（不是空串）」「无已完成日报时三数组均为 `[]`；`days<=0` 全历史档为全历史数据」；前端 payload 类型同步改 `string | null`（`front/app/api/persistentTopics.ts`）。

### R-46 2026-11-01【Low】候选链输入隐含「有可用 embedding」前提未写进契约

- **状态**：已修复
- **来源**：同上（审核 L2）。
- **修复**：契约 §2.1 补前提说明（端点既有过滤：无向量 section 既不进链、也不进画布可选池）。

### R-47 2026-11-01【Low】后端 `days` 缺省语义与契约「默认 30」不一致

- **状态**：已修复
- **来源**：同上（审核 L3）。
- **问题**：后端缺省 `days=0`（≈全历史，HEAD 既有行为），契约写「默认 30」。
  - **修复**：契约改口径「**后端保持向后兼容（缺省 0）**，「默认近 30 天」由前端快捷档显式传 30 保证」；不改动后端既有行为。

### R-48 2026-11-01【Medium】审核指出日期窗两条用例缺口（from/to 与 days 同给、days<=0 全历史回显）

- **状态**：已修复
- **来源**：同上（审核 D7 条）。
- **修复**：后端子线程补 D12（`from`/`to` 与 `days` 同给 → `days` 被忽略、`window.days=to-from+1`、范围外分区不出现）与 D13（`days=0` → `window.days=0`、`from`/`to` 为 null、数据为全历史）两条用例，handler 用例增至 13 条、repository 19 条。

### R-49 2026-11-01【High】前端重写丢失「历史待启用话题（带入草稿 / 直接启用原话题）」既有能力

- **状态**：已修复
- **来源**：主线程能力去向核对（AGENTS §0.6「操作允许性变更必盘点 UI 绑定点」+ review「能力去向表」纪律）；`explore-findings.md` 早已写明「页面当前还存在合并/历史回填入口，重新布局需对账不得默认删除」。
- **问题**：三栏工作面重写把旧编排面的历史待启用话题入口（管理资格名单 + 带入草稿 + 直接启用）整块丢了，delta spec「历史管理名单 SHALL 通过明确入口保留」与「人工名称不被覆盖」场景里的「从历史名单带入草稿」落空。
  - **修复**：派返工子线程补回——工作面工具条「历史待启用话题」入口 → 名单弹窗（累计命中/最近出现/搜索/置灰原因）→ 「带入草稿」只带窗口内 owned 成员（不自动补相似、不截 20）、自动名绑定 `history:<topicId>` 且人工名永不覆盖、来源清空时自动名一并清空；「直接启用原话题」仅 `can_activate` 可点，走既有 `updateTopic(status=active)`、不重算质心。能力去向表见 `review-lane-compose-implementation.md` §三。

### R-50 2026-11-01【Medium】真机 E2E 误触发真实创建（自动预填名未被清空）并污染本地数据

- **状态**：已修复
- **来源**：主线程真机 agent-browser E2E（`ui-verification/lane-compose-e2e-2026-10-31/verify.mjs`）。
- **问题**：E13「缺名称拒绝创建」故事里，命名弹窗的**名称被自动预填**（候选话题名），脚本直接点「确认创建」→ 后端真创建了 `active/manual` 话题 2041，并把 sections 7345/7409/7514 从 candidate 话题 1984 改挂到 2041（`topic_match_confidence=manual`、快照清空），**违反「验证含副作用的 SQL 一律不碰真库」纪律**。
  - **修复**：① 脚本改为先清空名称再提交，并加断言「未真创建（候选链数量不变）」；② 数据回滚：从当日 04:00 备份 `backups/pg-key-20261101-040001.dump` 精确取回 3 行的原始 5 个字段（`persistent_topic_id=1984`、`topic_match_distance`、`topic_match_confidence=auto_new/anchor_hit`、`topic_status_at_report=candidate`、`lane_tier=l3_new/l2_llm`），事务内回写并删除 2041；回滚后接口复测：155 sections / 267 relations / **3 条候选链（1984 链恢复 3 成员、跨度 3 天）**，与测试前完全一致；备份留档 `/tmp/syntopica-e2e-backup/`。

### R-51 2026-11-01【Low】E2E 用例把「半选时点标题」误判为取消整组

- **状态**：已修复
- **来源**：同上（用例侧，非产品缺陷）。
- **问题**：契约 §2.4 的口径是「点标题一次补齐该片段全部成员；**全选时**再点标题 = 取消整组」，E2E 用例却从半选态直接断言取消。
  - **修复**：用例拆成两条——半选时点标题 → 补齐到 3/3；全选时再点标题 → 取消整组；产品行为与契约一致，未改代码。
