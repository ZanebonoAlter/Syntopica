### R-1 2026-11-07【High】「今日新起」条目不显示来源数与热度，违反本 change 改写的红线 29
- **状态**：已修复
- **位置**：front/app/features/tags/components/console/BoardRadarReadingRail.vue:158-171（新起档模板）；docs/reference/flow/semantic-board.md:411（红线 29）
- **事实**：
  - 红线 29（本 change 改写）原文：「…的事件线归入独立的「今日新起」档，**SHALL 显示其来源数与热度**、SHALL NOT 显示倍率、SHALL NOT 以 0 倍或无限倍硬凑」。
  - 实现的新起条目模板只渲染 `<span>{{ storyTitle(item) }}</span>`（同文件 :170），既无来源数、也无热度、也无版块名与锚点日。对照：同组件「今日该读」条目（:101-106）渲染版块名 + 标题 + 来源数 + 热度 + 锚点日；「今天可能升温」条目（:146-152）渲染倍率 + 理由句（版块名 / 当日热度 / 近 3 日基线）。三档字段口径不一致。
  - 该 SHALL 未被任何测试或验收覆盖：spec delta `specs/workshop-radar/spec.md`「今日新起单列且不显示倍率」只要求 SHALL NOT 显示倍率；`ui-verification/report.md` A-4 的通过依据也只看「无倍率数字」。
  - 数据侧字段其实已经到位：`storyAria()`（:58-62）把 boardLabel / 来源数 / 热度都拼进了 title/aria，只是可见文本没渲染。
- **影响**：
  - 直接违反红线 29 的 SHALL（该红线正是本 change 亲手改写的），归档门禁（§11「文档与实现一致」）无法通过。
  - 实测口径下「今日新起」是主力档（`test-cases.md` §4.1：`new_count=120`、升温档为空），读者面对 3 条只有标题的条目，看不到版块归属、来源数与热度，无法判断重要性，也无法与「今日该读」里同一事件线的呈现对照。
- **处置建议**：
  - 在新起条目下补一行 meta（版块名 + 来源数 + 热度 + 该版块锚点日），复用既有 `.rd-read-meta` / `.rd-warm-why` 骨架与 `--color-text-muted` token，不新增色值与断点（落在 ui-design 的复用契约内）；字段取自 `RadarStoryItem` 已有值，改动只在模板。
  - 另一条路：主线程拍板改写红线 29 去掉该 SHALL —— 但红线句是 constraint-injection 注入源，改文档须走 openspec 并同步 spec delta。

### R-2 2026-11-07【Medium】「未回填」判定复用 lane-dynamics 窗口代理：空板/候选话题板被误标「未回填」且整板剔除
- **状态**：已修复
- **位置**：backend-go/internal/topicgraph/repository/board_radar_repository.go:319-320（`StoriesBackfilled` 取值）、:271（清单门控）、:485-488（未回填判定）；backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:690-733（`statsBySection` 样本范围）
- **事实**：
  - `row.StoriesBackfilled = dynamics.StoriesBackfilled`（board_radar_repository.go:320），而该值由 `loadLaneStoryAggregates` 统计，其样本集 `statsBySection` 只覆盖 `listLaneWindowSections`（`persistent_topic_id IS NOT NULL`）且话题为 active ∪ watch-linked 的 section 的 thread（lane_snapshot_repository.go:690-733）。即：**有日报但没有任何 section/thread 的板**、**section 全部锚定在 candidate 话题的板**，`StoriesBackfilled` 都为 false。
  - `finalizeRadarStories` 的判定是 `!row.StoriesBackfilled && row.ReportDate != ""`（:485-488）→ 上述两类板被计入 `unbackfilled_board_ids`；`radarBoardStoryPoints` 的门控 `!row.StoriesBackfilled`（:271）→ 这两类板在雷达清单里 0 条目。
  - 既有 handler 测试即复现该语义：`backend-go/internal/topicgraph/handler/board_radar_overview_handler_test.go:164-177` 造了一个「有 2 份日报、零 section」的板并断言 `row.StoriesBackfilled == false`（「无泳道线程：窗口内无 story_id → 未回填」）；按 `finalizeRadarStories` 的判定，该板同时会进 `unbackfilled_board_ids`。
- **影响**：
  - 降级原因标注错误且具误导性：这类板并不存在「需要回填的事件身份」（要么根本没有事件 thread，要么事件 thread 已回填、只是话题未转正），界面却提示「N 个版块事件身份未回填，未纳入事件线排序」，把用户注意力引向 `cmd/backfill-stories`。
  - 同时这些板即便存在 `board_stories` + `story_signals`（真有事件线）也被整板剔除，红线 30 追求的诚实降级反而被自身的标签破坏（真实原因不是回填，却被写成回填）。
- **处置建议**：
  - 雷达侧改用板级判定「该版块窗口内是否存在带 `story_id` 的 thread」（不依赖话题锚定），并把「未回填 / 无可回填对象（无 section）/ 不可判定（dynamics 取数失败）」拆成三态；清单门控与 `unbackfilled_board_ids` 都用板级判定，话题是否锚定不该决定事件线是否上榜。
  - 最小改动版本：`buildRadarRow` 里把 `StoriesBackfilled` 换成雷达自算的板级布尔（或 *uint 三态），`finalizeRadarStories` 的判定随之调整。

### R-3 2026-11-07【Medium】事件线测量时刻取 lane-dynamics 轴末日，而非该版块自己的最新日报日
- **状态**：已修复
- **位置**：backend-go/internal/topicgraph/repository/board_radar_repository.go:321-323（`anchor := dynamicsAnchor(dynamics)` → `row.AnchorDate`）、:271-276（以 `row.AnchorDate` 定位测量时刻）、:437-450（顶层 `out.Date`）、:490-492（lagging 判定）；backend-go/internal/topicgraph/repository/board_radar_stories.go:86-104
- **事实**：
  - `row.AnchorDate` 来自 `dynamicsAnchor(dynamics)`（:392-402）= 各泳道 timeline 的最大日期，而泳道 timeline 只含 active ∪ watch-linked 话题锚定的 section；该版块自己的最新日报日是 `row.ReportDate`（`GetLatestReportBrief`，board_radar_repository.go:566-592，不按状态过滤）。二者在「最新一期日报的 section 未锚定到活跃话题」时分叉：`row.AnchorDate < row.ReportDate`；无任何泳道 section 时 `row.AnchorDate == ""` → `radarBoardStoryPoints` 直接返回 nil（:271）。
  - 顶层 `out.Date` 仍取各板 `ReportDate` 的最大值（:437-450），所以这类板会被 `finalizeRadarStories` 判进 `lagging_board_ids`（:490-492），条目上的 `anchor_date` 回退到更早的日子，测量时刻也随之提前（48h 窗少累积）。
- **影响**：
  - 与红线 30「各版块的『今日』= 该版块自己的锚点日（其最新日报日，即 14 天轴末日）」的定义分叉：红线把「最新日报日」与「14 天轴末日」当作同一个东西，实现只满足后者。
  - 无活跃泳道的板整板缺席于事件线清单，却仍出现在版块行里（读者看到同一版块「行里有、清单里没有」），design D2 的取舍理由（滞后板不整板缺席）在此被自身的锚点来源抵消。
- **处置建议**：
  - 测量时刻改用该版块自己的最新日报日（`row.ReportDate`；无日报即不产条目），与 lane-dynamics 轴解耦；若坚持复用轴末日，则必须在 `stories` 段显式区分「轴末日 ≠ 最新日报日」的版块并给出说明（当前响应无此标记，读者无从判断）。

### R-4 2026-11-07【Medium】单版块事件线计算失败只落日志，不进 partial / 降级标记 → 不完整排序被当作完整排序
- **状态**：已修复
- **位置**：backend-go/internal/topicgraph/repository/board_radar_repository.go:278-291（`radarBoardStoryPoints` 的两条错误分支）
- **事实**：
  - `RadarBoardStoryPoints` 出错 → `logging.Warnf` + `return nil`（:279-282）；`RadarStoryReportDates` 出错 → `logging.Warnf` + `return points`（:288-291，落点日期全空 → 条目退化为「该事件线暂无正文落点」的静态行）。
  - 两条路径都不置 `row.DataDelayed`、不改 `feed.Partial`、不进 `feed.DegradedBoardIDs` —— 这三个标记目前只在端点超时分支产生（:250-257）。
  - 与 `test-cases.md` §3.4「部分失败重试 → 该版块条目缺席并标降级」的处置约定不一致。
- **影响**：该版块条目静默缺席（或整板退化为不可点的静态行），界面照常呈现一份「完整」的 Top 5 排序，没有任何「排序不完整」提示 —— 与红线 30「端点超时降级…SHALL 标 partial + degraded_board_ids，界面 SHALL 明示『本次事件线排序不完整』」同源的诚实降级纪律在此失效。
- **处置建议**：
  - 让 `radarBoardStoryPoints` 回传降级标记（如 `(points []RadarStoryPoint, degraded bool)`），调用方写入该板的降级集合，`finalizeRadarStories` 据此置 `partial = true` 并把该板 id 放进 `degraded_board_ids`。
  - 不要复用 `row.DataDelayed`：那是「整板数据延迟」语义，复用会把「事件线计算失败」误标成超时降级（版块行会显示「数据延迟」）。

### R-5 2026-11-07【Low】多跳 merge 链把「最终存活者」整条剔除出清单
- **状态**：已修复
- **位置**：backend-go/internal/topicgraph/repository/board_radar_stories.go:100-113
- **事实**：`tainted` 的判定是「`evidence.survivors` 中的某 id 不在 `evidence.strict` 里 → 把它的最终存活者标脏」。多跳链 X→Y→Z 时，`strictStorySurvivors`（story_heat.go:54-77）因 `next.MergedInto != nil` 丢弃的是 **X**，而 `tainted` 落在 **Z** 上；Z 本身是干净存活者（有标题、有热度、有 signals），却因 `tainted[id]` 被 `continue` 掉（:110-113）。
- **影响**：罕见但真实的静默丢条目（无任何降级标注）；design D4 的措辞是「多跳保守丢弃」多跳那条，实现扩大到存活者。存量数据里 `story_heat.go:135` 仍在打「resolve unexpected merged chain」告警，说明该异常确实存在。
- **处置建议**：只剔除多跳链上的前驱（逐条按 `strict` 缺失的 id 去掉其自身 heat），不要连坐最终存活者；无法判定时按「保留 + 标注」处理而非静默丢弃。

### R-6 2026-11-07【Low】`lagging_board_ids` / `degraded_board_ids` 归一后无消费点，design 承诺的「N 个版块滞后」提示未实现
- **状态**：已修复
- **位置**：front/app/api/normalizers/boardRadar.ts:186-188、:276-278；front/app/features/tags/composables/useBoardRadarOverview.ts（只导出 `unbackfilledBoardCount` / `partialStories`）；front/app/features/tags/components/console/BoardRadarReadingRail.vue（只渲染未回填与 partial 两条提示）
- **事实**：`grep laggingBoard|degradedBoard` 在 `front/app` 下只命中 normalizer 与测试夹具，无任何组件消费。design D2 明写顶层 `date`「只用于页头与『有 N 个版块滞后』的提示」；`docs/reference/api/daily-reports.md:315,317` 也把两个字段写进了响应契约。
- **影响**：跨版块混排的已知系统性偏差（滞后板少一天累积）只能靠条目上的「截至 MM/DD」逐条推断，读者无法知道有几个版块落后；`degraded_board_ids` 也只有 generic 提示，没有可追的板 id。
- **处置建议**：补一条「N 个版块数据滞后（各条目按自己的锚点日计）」的右栏提示（消费 `laggingBoardIds.length`）；或在 design / API 文档里删掉「提示」承诺，避免契约与实现分叉。

### R-7 2026-11-07【Low】验收证据缺 390×844 窄屏；A-7「排序不完整明示」的实测依据不成立
- **状态**：已修复
- **位置**：openspec/changes/surface-story-events-in-radar/ui-verification/report.md（A-7、A-10）；openspec/changes/surface-story-events-in-radar/ui-design.md「目标检查」；openspec/changes/surface-story-events-in-radar/tasks.md 4.4
- **事实**：
  - ui-design「目标检查」与 tasks 4.4 均要求「1440×900、1920×1080，另检 390×844 换行不溢出」；验收报告 A-10 只记了 1440 与 1920 两档的 `scrollWidth == innerWidth`，没有任何 390×844 观测数据。
  - A-7「排序不完整明示【pass】」称「…在降级数据下出现，见 A-5 之前的观测」，但同一批观测样本（`test-cases.md` §4.1 与 A-2/A-6）里 `partial=false`、`degraded_board_ids=[]`，没有降级样本——该条实际只有单测背书（报告自己在 A-9/A-11 用「pass（单测覆盖）」标注，A-7 未标）。
- **影响**：本 change 的右栏新增三档条目与更长标题行（窄屏溢出风险面变大），ui-design 目标检查少一档；归档证据强度不足，且把单测覆盖写成了实测通过。
- **处置建议**：补 390×844 实测（标题换行、来源数/热度行不溢出、按钮可访问名称不被裁断）；把 A-7 表述改为「单测覆盖，未取得降级实测样本（`partial=false`）」，或造出降级样本后补一条实测。

### R-8 2026-11-07【Low】`test-cases.md` §2「继承与调整」表首行的旧测试指向与实际代码不符
- **状态**：已修复
- **位置**：openspec/changes/surface-story-events-in-radar/test-cases.md §2 表格第 1 行
- **事实**：该行把「升温判定（版块级：当日事件线数 ≥3 且合计热度 ≥2× 近三日日均）」的旧测试指向 `boardRadarHelpers.test.ts` 与 `BoardRadarView.test.ts`；但 `boardRadarHelpers.test.ts` 内只有 `tierLabel` / `trendOf` / 轴与延迟判定断言（其 diff 中没有任何升温断言）。真正承载版块级升温断言的是 `useBoardRadarOverview.test.ts`（「升温（热度口径，workshop-radar 契约）：todayStories ≥3 且 todayHeat/last3Heat ≥2 倍才列入」），且它按 design D10 被**保留**为旧响应回退路径（并非「改写为事件线级」）。`BoardRadarView.test.ts` 的事件线级断言是本 change **新增**的 describe 块，旧断言亦未被删改。
- **影响**：按 §2 表追「旧断言是否被无声改掉」会指向错误文件；实际结论是本 change 未删改旧断言（新增事件线级断言 + 保留回退分支），但表里的描述与代码不符，削弱归档对账的可信度。
- **处置建议**：把该行「旧测试」列改为 `useBoardRadarOverview.test.ts`、`BoardRadarView.test.ts`，处置措辞改为「保留为旧响应回退路径（版块级）；新增事件线级断言」。

### R-9 2026-11-07【Low】tasks.md 勾选状态与实际改动不一致（文档已改但任务未勾）
- **状态**：已修复
- **位置**：openspec/changes/surface-story-events-in-radar/tasks.md 5.1 / 5.2 / 6.7
- **事实**：5.1/5.2 仍为 `[ ]`，但实际已改：`docs/reference/flow/semantic-board.md` 改写红线 29/30 并补「后端雷达事件线清单 / 前端雷达右栏」代码入口；`docs/reference/flow/daily-report.md:241` 已写 `?story=` 深链定位契约；`docs/reference/api/daily-reports.md:293-390` 已写 `stories` 段字段表与降级说明。6.7（`doc-impact.sh verify` + `check-standards.sh`）同样未勾。
- **影响**：归档门禁按 tasks 勾选核对时会认为「reference 文档未更新」，与实做不符（§11 归档纪律要求勾选与实做一致）；也让 R-1 这类「文档已写、实现没跟上」的问题被勾选状态掩盖。
- **处置建议**：勾上 5.1/5.2 并把「当前制品任务不执行这些 reference 改动」的措辞改掉；跑完 6.7 两条脚本后再勾。

### R-10 2026-11-07【Low】工作区改动无法与兄弟 change 隔离，提交/归档范围需人工确认
- **状态**：已采纳
- **位置**：backend-go/internal/topicgraph/repository/board_radar_repository.go（Attention=Σlane_heat、story_counts、heat_days、泳道按事件线密度排序等属 add-event-story-heat）；front/app/features/tags/components/daily-report/DailyReportTopicSection.vue（`locateRequest` / `findThreadByStory` / `drm-thread--lit` 本体属 surface-story-heat-in-report-top）；front/app/features/tags/components/DailyReportReader.vue（`handleLocateHighlight` 本体同属该兄弟 change）
- **事实**：`watchdog_diff` 显示工作区相对 HEAD 有 103 个文件改动、6849 增 / 645 删，含 add-event-story-heat（已归档但未提交）、surface-story-heat-in-report-top、boardhealth 体检、迁移与两个回填 CLI 等；本 change 名下文件里也混有兄弟 change 的实现段。本 change 的真正新增是未跟踪文件 `backend-go/internal/topicgraph/repository/board_radar_stories.go`、`board_radar_stories_test.go` 等。因 `story_heat.go` 整体未跟踪，无法用差异核验「本 change 未改 `ComputeBoardStoryHeatByDay` 返回值语义」这一条任务约束（代码读下来语义确实未变，但缺差异证据）。
- **影响**：若按「本 change 相关文件」批量暂存，会把兄弟 change 的未归档实现一并带入提交/归档；review 侧对 `story_heat.go` 的「未改语义」只能靠人工读码确认，无差异证据。
- **处置建议**：提交前用 `git add -p` 逐段确认本 change 的实现边界，或先让兄弟 change 落库/提交；归档说明中写明本 change 依赖 add-event-story-heat 的未提交产物（`board_stories` / `story_signals` 表与 `story_heat.go`），避免单独回滚本 change 时把依赖拆散。
