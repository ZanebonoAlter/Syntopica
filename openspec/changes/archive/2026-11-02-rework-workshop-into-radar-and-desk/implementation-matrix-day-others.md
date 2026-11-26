# 矩阵「当日其他话题」增量实施合同（8C.1 产出，本文件只做映射，不改代码）

> 批准基准：`ui-prototype/revision-2026-10-28-matrix-day-others/standalone.html`（696,943 字节，SHA256 `cdb0b70e31fbc00740f04e908d5df2f372bc48fd8871a106416efb8d2e404448`）。用户 2026-10-28 原话「可以，这版我通过了，先把讨论记录 pin 一下，然后我打算压缩会话」，压缩交接后「可以继续了」；批准记录见 [ui-approval-matrix-day-others.md](ui-approval-matrix-day-others.md)。
> 本文件把**已冻结的增量 UI**映射到产品组件/composable/API/路由，列状态模型、字段来源、独占辖区与验收落点。**不含产品代码改动**；离线样本 JSON、原型常数、调试壳一律不得进产品。旧四稿（矩阵基线 `ad22103a…`、picker `d118718e…`、右轨 `20842557…`、冷启动 `e4e42a4e…`）与旧失败/未覆盖账目不因本增量改写或洗绿。
> 范围边界：只覆盖「矩阵/演进/滑动观察窗/下方内联阅读」上的 (a) 日报日驱动的日期轴（含 0 正式泳道冷启动）、(b) 「当日其他话题」折叠组与就地下读、(c) 窄屏同源日期条与组/阅读。**不扩批**文字日报主舞台、独立日期 picker、素材托盘/跨日报建题、关系/关键事件、编排 UI 或任何生命周期动作。

## 1. 用户影响与迁移预期

**上线后用户可见变化**

- 0 正式泳道但有日报的版块（现场样本：卫生健康 246030）不再只看到「暂无活跃泳道」：日期轴保留，日期列头按**真实报日**可选；可展开「当日其他话题」读所选日报里不属于正式/休眠泳道的分区、完整线索标题与原始摘要。
- 成熟板块（现场样本：中东 1974）矩阵/演进/滑筛/内联阅读不变，只多一个**默认收起**的「当日其他话题」组（与休眠同级的独立折叠组，summary 在各自内容之前）；**休眠组 summary 从原网格底部前移到其自身行之前**（与批准原型的组序一致，开关与阈值语义不变）。
- 日期列头成为与日格/演进节点同一 `selectedDate` 的第三处入口（不新增日历/第二条日期导航）；点击无日报的列头不可选。
- 窄屏（<768）以同一份日期轴渲染横向可滑的报日条 + 其他话题组 + 同一阅读面板；不再因 0 正式泳道只剩一句空态。

**明确不变**

- `sourceDays`（7/14/30/365）仍是名单/前端行序/休眠/候选的唯一基准；B 365 池只补 A 同 ID 历史；拖选/输入/缩放/键盘永不换名单（design D11，OX-1/OX-2/OX-11）。
- 正式日格仍是桶峰值 max；演进节点/趋势仍是日合计 sum；行尾无总数列；休眠分组与既有阈值不变（新增组不参与名单/排序/休眠）。
- 右轨四区、全局治理唯一入口（总览）、研究工作台、归档、日报阅读台、编排态、生成入口与既有缓存/失败分层/返回上下文契约不动。
- 站内阅读与状态动作走既有 `ArticlePreviewModal`（收藏乐观更新+失败回滚、全屏层级由宿主传入），不为其他话题另造第二套预览。

**迁移与操作**

- 无数据库迁移、无需重新生成日报、无需回填历史、无需清理或配置。后端零改动（无新端点/参数/字段）。
- 旧日报缺 `topic_status_at_report`/`persistent_topic_id` 时诚实降级为「当时状态未记录」/「当次未归属」，**不回填、不补写**。
- 部署为前端静态发布（`bash scripts/dev/deploy-frontend.sh`），后端零改动不重启。

## 2. 真实读取边界（已核实的代码事实，产品数据流只能建立在这些之上）

| # | 事实 | 源码锚点 |
| --- | --- | --- |
| F1 | `lane-dynamics` 的 lanes = 本板块非归档话题 ∩（active ∪ watch 关联）**且窗内有 ≥1 分区**；窗内零分区的泳道不返回；窗口锚定板块 `MAX(period_date)` 往回 N 天 | `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:GetBoardLaneDynamics`（356 起） |
| F2 | lane 的每个 section 只带 `section_id/label/thread_count/events(前 5 条标题)/folded_count`，**无 thread id、无 summary**；截断常量 `LaneTimelineEventsPerSection=5` | 同上 `LaneDynamicsTimelineSection`（230–238）、常量 27 行 |
| F3 | 报日索引 = `GET /semantic-boards/:id/daily-reports?days=N`（`api/dailyReports.getBoardDailyReports`）：**锚服务器今天**、days 默认 7、无上限钳制；列表条目含 `id/period_date/title/created_at`，**不含 sections** | `backend-go/internal/topicgraph/handler/daily_report_handler.go:listBoardDailyReports`（295）；`front/app/api/dailyReports.ts:272` |
| F4 | 完整日报 = `GET /daily-reports/:id`：全部 sections（`cluster_index` ASC）+ 全量 threads（`id/section_id/title/summary/confidence/related_article_ids/fit_distance/created_at`）；section 另带真实历史字段 `topic_status_at_report`（active\|candidate\|null）、`persistent_topic_id`、`topic_match_confidence`、`lane_tier`、`bucket_title`、`article_count` | `front/app/api/dailyReports.ts:152-182 / 217`；`backend-go/internal/topicgraph/repository/daily_report_models.go:145` |
| F5 | 稳定 ID 同源：lane-dynamics 的 `section_id` 就是 `daily_report_sections.id`（即报告详情 section id），可按 ID 精确 join，禁止标题匹配 | `lane_snapshot_repository.go:168,498`（`SectionID: s.ID`）；`api/normalizers/laneDayReading.ts` 头部注释 |
| F6 | 既有读取 store `useLaneDayReading` 已持：报告列表索引仅当前板块 1 份、报告详情 ≤14、文章 ≤20、Map FIFO、同键在途合乘、失败不入成功缓存、epoch 晚到丢弃、切板块/卸载释放；但 `ensureDayReading` **入参要求 lane 当日 `WindowLaneDay`**，无法直接服务「报告优先」的泳道外分区读取 | `front/app/features/tags/composables/useLaneDayReading.ts`（`ensureDayReading` 155–221、缓存上限常量 85–87、`resetForBoard` 262） |
| F7 | 现状断链：`emptyState='no-lanes'` 时 `ObservationMatrix` 不挂网格（日期轴消失）；`reconcileSelectedDate` 在 `!lane` 直接返回 null；`BoardConsoleStage` 以 `v-if="selectedLaneRow"` 挂 `LaneChronicle` | `console/ObservationMatrix.vue:126-141`；`console/laneConsoleView.ts:reconcileSelectedDate`（约 100–110）；`console/BoardConsoleStage.vue:160-176` |
| F8 | `selectedDate` 由壳层唯一持有，**不写 URL**；返回上下文（sessionStorage 一次性）保存 lane/date/days/start/end/滚动；URL 既有键仅 `days/start/end/lane`，日报阅读走 `/tags/:boardId/:date` | `console/BoardConsoleShell.vue:11,171,312,366`；`console/useConsoleReturnContext.ts`（R30 合并单次 replace） |
| F9 | 观察轴 = `visibleAxis`（`buildDayAxis(range.end, 含两端天数)`）；轴锚点 = `reportAnchor`（overview/latest）或 A 数据最新日；`today` 用 Asia/Shanghai | `composables/useBoardConsole.ts:137-163`；`composables/useObservationWindow.ts` |
| F10 | 文章详情唯一正源 `GET /articles/:id`；站内阅读/收藏/全屏已由壳层 `ArticlePreviewModal`（`dialogSemantics`）承接，收藏乐观更新+失败回滚、全屏层级 9200 | `BoardConsoleShell.vue:350-410,486`；`ArticlePreviewModal` / `ArticleContentView` 契约（tasks 6.6/6.7） |
| F11 | A 名单含 active + dormant（休眠是前端展示分组，`sectionTotal≤2`，不写状态）；候选是另一实体（`sourceWindow.candidates`），**不得**进入其他话题组 | `console/observationLanes.ts:buildObservationRows/splitObservationRows`（12 行契约注释起） |
| F12 | 报告生成完成后的刷新链 = `DailyReportGenerateDialog @completed/@cancel → refreshConsole`（`loadReportMeta` + A `load`），当前**不会**刷新报日索引 | `BoardConsoleShell.vue:196-199,465-470`；`console/useConsoleRefresh.ts`（32 行） |

**API 改动结论：后端零改动、零新端点、零新参数、零写接口。** 日期轴/组/阅读全部复用 F3/F4 既有读取；新增的仅是**前端读取面**：报日索引暴露 + 报告优先的当日分区读取（共享同一 store 与缓存），以及在 normalizer 边界补 F4 已存在的历史字段。

## 3. 展示字段盘点（问句⑤：每个展示字段必须有真实来源与 spec 锚）

| 展示字段 | 来源（真实链路） | 边界/降级 | spec 锚 |
| --- | --- | --- | --- |
| 日期列头可点/不可点 | `reportDates` = 报日索引 `period_date` 集合 ∩ 可见观察轴（F3+F9） | 索引未取到 → 列头不可选 + 明确提示；无日报日不可点但轴保留 | workshop-board-console「日报日驱动的日期轴」 |
| 组标题与计数「当日其他话题 · N」 | 所选日报详情 sections 经 F5 排除后的行数（F4） | 加载中/失败/无报日/无详情时**不给数字**，给状态文案；0 行给「当日分区都已可在泳道读到」 | 同上「当日其他话题折叠组」 |
| 组默认展开/收起 | 首份 A 响应的正式行数（active+dormant）是否为 0，仅初始化一次；之后用户主权 | 切板块复位；两组合并/串联折叠禁止 | 同上 |
| 其他行标题 | 所选日报 section `cluster_label`（当日原题，逐字） | 长标题两行 clamp + 完整 `title`/`aria-label`；下方阅读区不截 | 同上 |
| 其他行计数（仅所选日格） | 该 section 全量 threads 长度（F4；不是 lane `thread_count` 聚合） | 0 条 = 合法 `0`；未取到详情 = `?` 且行不可点（不补 0） | 同上 |
| 其他行状态文本（tooltip/aria） | section `topic_status_at_report`（active→当时已转正／candidate→当时待观察／archived→当时已归档；null+topicId→当时状态未记录；无 topicId→当次未归属） | **禁用当前 `topic.status`**；不渲染资格徽标、不给激活入口 | 同上 + F4 字段注释 |
| 其他行徽标 | 不新增徽标；正式行的「追踪」徽标与休眠标签保持原样 | 其他行不冒充泳道/watch；不显示候选管理字段 | 同上 |
| 分组与顺序 | 正式行 → 当日其他话题（summary+行）→ 休眠泳道（summary+行）；两 group 同级、全宽、独立开关 | 0 正式行也保留该顺序（正式区为 0 行） | 同上 |
| 阅读面板分区标题 | 选中 section 的 `cluster_label`（逐字） | 详情已更新找不到该 section → 中性「当日报告已更新，未找到该分区」+ 刷新；不借别日/别分区 | workshop-board-console「当日其他话题折叠组与就地下读」 |
| 阅读面板线索标题/摘要 | `thread.title` / `thread.summary` 逐字（F4；同 ID 链） | 空 summary → 「暂无摘要记录」；同一时刻仅一条展开；不截断 | 同上（复用既有工作面义务） |
| 相关文章 | `thread.related_article_ids → GET /articles/:id`（按需、≤20 缓存、单项失败局部重试） | ids 空 → 「相关文章列表为空」；失败不改总数、不声称无文章 | board-lane-dynamics「矩阵观察窗读取边界」（缓存/稳定 ID 义务继承） |
| 打开日报/看归档/读原文动作 | 既有：`/tags/:boardId/:date` 路由、归档视图带日期、`ArticlePreviewModal` | 历史日动作带完整日期；latest 唯一主入口仍在头部，不重复/不误删 | workshop-board-console「板块入口作用域与最新日报主入口」 |
| 数据截止/今天 | `anchorDate`（A/报日锚）与 `today`（Asia/Shanghai）分别表达（F9） | 不把服务器今天当板块截止；不把 10/27 已生成日报说成「未来数据」 | workshop-board-console「矩阵视图」 |
| 空/加载/失败/未取到 | 见 §4.6 状态矩阵 | 「0 条真实」「无日报」「详情未取到」「索引未取到」「泳道当日未取到」五种不同语义 | 同上 + 「观察窗与自由日期显示」 |

字段归属说明：`topic_status_at_report`/`persistent_topic_id`/`cluster_label` 均为 daily-report-system 既有日报 section 字段（F4），本增量只在控制台读取面消费，**不修改 reader/生成/数据集**，因此不追加 `daily-report-system` delta；若父认为「其他话题状态文案」应归入该能力，可后续一行补 Scenario（不改实现）。

### 3.1 获批观察带字段补充（父准入校正）

| 展示字段 | 唯一来源/语义 | spec锚 |
| --- | --- | --- |
| 本窗记录日/可点节点 | section-timeline中可靠同persistentTopicId的真实分区日期，再按连续观察轴裁切；>1日才画节点，未取到不冒0 | 同归属跨日观察带只列本窗真实记录日 |
| 观察节点计数 | 同ID同日分区threadCount峰值；任一缺失则未知，不是日合计/正式泳道状态 | 同上 |
| 观察加载/失败/缺身份说明 | 独立只读观察依赖状态；不阻塞已取得的日报，不读当前topic属性当历史 | 跨日观察缺身份或读取失败不借内容 |
| 节点目标与返回阅读身份 | 真实report/section/date与板块稳定ID，经同一报告读取store核对；不按相似标题猜配 | 点观察节点切换同一选中日并读取真实分区；其他话题打开日报后返回恢复原阅读 |

## 4. 状态模型与读取隔离（实现决策书）

### 4.1 日期轴与选中日（`selectedDate` 唯一，三处入口）

- 日期轴仍 = `visibleAxis`（观察范围裁切）；**可选择性**由报日索引决定（列头只对 `reportDates` 内的日期渲染为按钮）。轴不因 0 正式泳道消失，也不新增第二套日期状态。
- `selectedDate` 校正（唯一实现点 `laneConsoleView.reconcileSelectedDate`，由 `useConsoleSelection` 调用，新增 `reportDates` 入参）按以下优先级，**顺序固定**：
  1. `current` 在可见轴内 且（`current` 是报日 或 该泳道当日有分区）→ 保留（用户点列头/切泳道后不把人搬走；泳道当日无内容时由既有编年给空态）。
  2. `current == null`（进入/切板块）→ 选中泳道可见区间内最后有分区日（既有 CH-1）；无泳道或无内容日 → 可见轴内**最新报日**。
  3. `current` 已不在可见轴（范围排出）→ 新窗内**最新报日**（与冻结原型一致，不按离旧选日距离另算）；窗内无报日 → `null`（诚实空态，**不**跳今天、**不**跳有 active 泳道的日子）。
- 报日索引未就绪/失败：不清空既有 lane 内容日选择；列头不可选 + 提示 + 重试（见 §4.6）。索引到达后按上述规则再校正一次。
- **原型取样限制不继承**：原型轴止于 10/26 只因快照只取到那一刻；产品按**实时**报日索引工作，10/27 及之后只要后端确有日报，对应列头就可选、可读（「已冻结 10/27 日报未纳入快照」不代表产品禁止读取）。同理，原型 hash/本地 storage 不是深链/刷新契约，产品不照搬。
- 不新增 URL `date=`（见 §8 P4）：选日为显示态，刷新恢复默认；返回上下文沿用既有一次性 sessionStorage（lane/date/days/start/end/滚动）。从阅读台返回的 pending date 若是报日 → 规则 1 保留（0 正式泳道板块也能回到原报日）。其他话题返回还须以稳定board/report/section/date身份恢复原阅读槽；G4沿用既有返回载体扩展，不能仅恢复日期却丢所选分区。旧载体兼容，不新增date= query。
- 范围/档/板块变更沿用既有原子写 query 语义（R30/R35），本增量不改 query 键。

### 4.2 当日其他话题组（`dayOtherTopics.ts` 纯模型 + 组组件）

- **成员定义（按身份，不按可读性凑）**：所选日报详情全部 sections 中，排除满足任一条件的 section：
  1. `sectionId` ∈ 任一正式/休眠泳道在**所选日**解析为 `content` 的 sections（`resolveLaneDay(row.topicId, selectedDate)`，A/B 均算）；
  2. `persistentTopicId` ∈ A 名单 topicId 集合（防后端窗口缺省导致重复归类）。
  两条件取并集；候选池（`sourceWindow.candidates`）、watch 列表、辅助标签、当前 topic 名**一律不得**进入。
- **行顺序** = 报告详情 `cluster_index` ASC（后端当日原序）；**不得**按线索数/名称重排（重排会伪造"隐藏榜"语义）。
- **只标所选日**：组内行只在 `date === selectedDate` 的格显示真实线索数；其余日期为空白非交互格（`aria-hidden`、不可聚焦），不构成跨日泳道。
- **派生位置**：其他组视图模型（state/rows/statusText/note/selectedSectionId）由壳层用 `dayOtherTopics.buildOtherTopics(reportDay, allLanes, resolveLaneDay)` 派生后经 props 下发，`ObservationMatrix`/`ObservationMatrixGrid`/`DayOtherTopicsGroup`/`OtherTopicReading` 均为**纯受控**（不自建网络/日期/排除逻辑）；这是避免组件层再算一套排除口径的唯一保证。
- **组状态机**（`no-date | index-loading | index-error | loading | no-report | error | ready`）：`ready` 时行数可为 0（合法）；计数只在 `ready` 出现；`no-report` 文案必须写明「不是 0 条其他话题」。
- **组顺序与休眠结构（本增量可见变化）**：DOM/视觉顺序固定为「正式活跃行 → 当日其他话题（summary + 自身行）→ 休眠泳道（summary + 自身行）」；休眠 summary SHALL 从现网格底部**前移到其自身行之前**（与批准原型一致），休眠行随组下移；休眠 0 行仍保留同级 summary（0 +「本窗无」，无内容可禁用展开），与冻结卫生截图一致；不得伪造休眠行。两 toggle 均 `aria-expanded`，键盘可聚焦，重渲染后焦点留在同一 toggle。`data-testid="dormant-toggle"` 保留在休眠 summary 上（位置变、选择器不破）；其他组 summary 用新 testid `other-topics-toggle`。
- **泳道可读性备注**：当某 A 行在所选日解析为 `unavailable`（365 池未取到）时，组说明追加一句「部分泳道当日内容未取到，其他话题按当日报告分区列出」；不因此把泳道分区补进组或把组判失败。
- **默认态**：首份 A 响应后按「正式行数（active+dormant）== 0」初始化一次 `othersOpen`（0 行 → 展开，成熟 → 收起）；之后只由用户 toggle 改变，切板块复位。**状态归属**：`othersOpen` 与 `selectedOther` 均由壳层（G4）持有并复位，`DayOtherTopicsGroup` 纯受控（无自有持久态）；与 `expandedDormant` **完全独立**（互不串联），两 toggle 均为原生 `button` + `aria-expanded`，重渲染后焦点留在同一 toggle。
- **折叠不影响读取边界**：summary 计数需要所选日报告详情，因此 `selectedDate` 确定后**无论组是否展开**都走一次 `ensureReportDay`（与泳道编年同一请求、同一缓存）；不得因收起而隐藏计数或延迟到展开才取（否则与原型已见行为不一致）。

### 4.3 阅读选择（单一阅读槽）

- 壳层新增 `selectedOther: { sectionId: string, date: string } | null`；`selectedDate` 变化 / 范围排出 / 切板块 / 切泳道 → 清空 `selectedOther`（section id 是当日局部身份，不能跨日复用）。
- 阅读槽优先级：`selectedOther !== null` → 其他话题阅读面板；否则 `selectedLaneRow !== null` → 既有 `LaneChronicle`；否则空态提示（0 泳道时指向「点列头选日报日 / 展开当日其他话题」，latest 打开日报入口仍在头部）。
- 点其他行/其他格：只更新 `selectedOther` + 阅读槽，**不弹层、不强滚、不抢焦点**；「查看所选日 ↓」仍可显式滚动到阅读槽。
- 其他话题阅读面板：分区标题 + 全量线索（`thread` 逐字标题/摘要）+ 相关文章按需 + 当日动作（打开日报/看归档）+ 顶部「当日其他话题 · 日期」；复用 `LaneChronicleThread` / `LaneRelatedArticles`（它们已接受 `reading` store），**不复制摘要/相关文章实现**。
- 详情已更新/找不到该 section/0 线索/读取失败：分别给中性文案与重试；**不**借用别日或相似标题（继承「稳定 ID 关联」红线）。
- **获批跨日观察带必须保留（G3）**：只读复用现有section-timeline API，以同一可靠persistentTopicId关联真实分区，按当前观察轴过滤记录日；本窗>1记录日才画可点节点带，1日仅一句状态、无ID/无记录/未取到均诚实说明，不补0、不造连续泳道、不称同一事件。节点日数只数记录日，节点计数取同ID同日真实分区threadCount峰值（任一分区计数缺失则该日峰值为未知，不冒0），不重算正式格或演进统计。只使用section历史身份/内容，端点当前topic属性不冒充历史状态。
- 跨日带点真实节点，原子更新同一selectedDate与稳定目标report/section阅读身份；从该日详情核对section ID后复用同一阅读槽，不借相似标题。失败/无记录/同日多分区/跨板块晚到/生成刷新均有状态与测试。G3新增只负责section-timeline的受控只读依赖（API归一化+composable，板块单份缓存/epoch/失败重试）；禁止复制日报index/detail读取器，后者仍全部共享本节单store。
- 只读：组与阅读面板零写请求（不含收藏等文章状态动作——那由既有预览弹窗承担）。

### 4.4 单一读取 store（禁止第二个读取器）

- 把 `useLaneDayReading` 实例**上提到壳层**（每个版块 1 个），由壳层同时服务：矩阵日期轴（报日索引）、`LaneChronicle`（lane 路径）、其他话题组（报告优先路径）与阅读面板。
- `LaneChronicle` 新增可选 `reading?: LaneDayReadingStore` prop：注入时使用注入实例；未注入时保持现存自建行为（仅测试/旧调用兜底）。产品路径必须注入，防止双份索引/详情缓存与重复 GET。
- store 新增：
  - `ensureReportIndex({ force? })`：按既有 `reportIndexDaysFor(poolStart, today)` 计算 lookback 取列表；已覆盖则复用（列表 1 份/板块），失败记 `indexError` 可重试。
  - `reportDates`（`ComputedRef<ReadonlySet<string>>`，来自索引）。
  - `ensureReportDay(date, { force? })`：索引 → 精确 `period_date` 命中 → 详情（同一 `reportCache`）→ 发布 `reportDay`（`{date,status,error,reportId,sections}`）。
  - **独立 epoch**：lane 读取与报告优先读取各一个序号（现 `seq` 拆 `laneSeq`/`reportSeq`），互不取消；切板块/卸载统一复位与清缓存。
  - **报告日显示 gate**：`reportDay` 必须带 `date`，面板/组渲染前与当前 `selectedDate` 比对；`selectedDate` 变化先清显示再加载（不把旧日 sections 挂在新日期标题下），同日刷新失败才允许 stale 保留旧内容；离开阅读槽时失效在途显示。
  - 缓存契约不变：详情 ≤14、文章 ≤20、索引 1 份、FIFO、在途合乘、失败不入成功缓存、当前读对象独立持有（CA-3..8 逐条继承）。
- **新增读取量**：进入版块首帧多一次报日列表 GET（每板块 1 次、随后的日期轴/编年/其他组全部复用）；所选日报详情与既有编年共用一次（同缓存键）；不预取全年详情、不为其他产出额外大字段请求。
- 生成完成刷新：`refreshConsole` 之后追加 `ensureReportIndex({force:true})`（新日报日立即出现在列头），再触发 A/元信息既有链；失败只标索引陈旧，不影响矩阵可读。

### 4.5 窄屏（同源，不是第二套日期导航）

- `BoardNarrowSummary` 增加报日条：保留同一连续`visibleAxis`，仅`reportDates`内日期可选（非报日不压轴）；桌面网格与窄屏为互斥渲染、每视口只有一个报日轴入口，点击写同一 `selectedDate`；选中日仅通过日期条自身scrollLeft定位到条内可见位置（挂载/选日变化后测量，无固定sleep）；不得用会滚动祖先/整页的scrollIntoView。跨日节点位于下方阅读区时，切日也不得把整页拉回顶部日期条。
- 0 正式泳道时不再只给空态：报日条 + 「当日其他话题」列表（当日分区、标题两行 clamp + 完整 title/aria、所选日线索数）+ 下方同一阅读面板；泳道卡与迷你条保持原样。
- 窄屏不渲染矩阵网格（D9 不变），不新增日历/独立 picker。

### 4.6 边界/降级矩阵（实现必须逐项给出可见答案）

| 场景 | 行为 |
| --- | --- |
| 报日索引加载中 | 轴照画（列头不可点）；泳道内容日选择照旧；组 summary 显示「读取报日…」 |
| 报日索引失败 | 列头不可点 + 内联提示「报日索引未取到，日期列头暂不可选」+ 重试；既有矩阵/编年/右轨不受阻；0 泳道板块不误报「无日报」 |
| 所选日无日报（范围落在无报日窗） | 轴保留；组给「所选日没有日报（不是 0 条其他话题）」；阅读槽不借别日；不自动跳今天 |
| 轴内含无报日（早于首报/晚于最新报/报日之间的空档） | 列头渲染为不可点文本（title「该日没有日报」）；日格/演进仍按泳道三态；不改轴、不裁剪到报日 |
| 所选日报告详情加载中 | 组 summary「正在读取当日报告…」；行不出现；阅读面板骨架 |
| 所选日报告详情失败 | 组错误文案 + 重试（与矩阵错误分层，不阻塞泳道）；已加载过的同一报告命中缓存 |
| 详情成功但 0 个可列分区 | 「当日分区都已可在正式/休眠泳道读到」——**不是**加载失败 |
| 某 A 泳道当日 `unavailable` | 组照常（按当日报告分区），追加「部分泳道当日内容未取到」说明；不补 0、不重复列 |
| 其他分区 0 线索 | 行显示 `0`、可点读（0 条是合法结果；面板给合法空列表） |
| 0 正式泳道 | 网格渲染列头 + 当日其他话题组（默认展开）+ 0 行说明；休眠组 0 行仍有0行summary/本窗无；不再用整块空态替换网格 |
| 长标题 | 行两行 clamp + 完整 title/aria；阅读面板全文不截 |
| 当天 = 今天 / 锚点早于今天 | 今天列高亮 + 数据截止文案沿用既有语义，不新造 |
| 切板块/卸载 | 索引/详情/文章缓存与组/阅读/折叠态全复位；晚到响应按 epoch 丢弃 |
| 跨月/跨年范围 | 轴连续（`buildDayAxis` 既有）；报日集合按日历日字符串比较 |

## 5. 批准原型 → 产品落点映射（含单 seam / 多 seam 分类）

| 原型区域（matrix-day-others） | 产品落点 | 动作 | seam 分类 |
| --- | --- | --- | --- |
| 日期列头选日（桌面） | `console/ObservationMatrixGrid.vue`：报日列头改 button，emit `selectDayHeader(date)`；`BoardConsoleStage.vue` 复接为既有 `select-date`（壳层已有 `selectedDate` handler），不新增第二个入口 | 改 | 单 seam（Grid→Stage→Shell 一条事件链，不改状态源） |
| 0 正式泳道仍保留轴/组 | `console/ObservationMatrix.vue`：`no-lanes` 分支改为「紧凑说明 + 网格（0 行）」；`BoardConsoleStage.vue` 的 `console-no-lanes-fallback` 降为不挡网格的内联说明 | 改 | 单 seam（只换渲染分支与文案，`emptyState` 语义不变） |
| 「当日其他话题」组（summary 前、行、只标所选日、默认态、独立折叠） | 新 `console/DayOtherTopicsGroup.vue`（`display: contents` 参与父网格）+ 新 `console/dayOtherTopics.ts`（纯模型）；events `toggleOthers` / `selectOther({sectionId,date})`（经 Grid→Stage→Shell） | 新增 | 多 seam（模型/组件/壳层状态三处，需按 §6 独占写域） |
| 「休眠泳道」组顺序/独立性 | `ObservationMatrixGrid.vue`：休眠 summary 从底部**前移到自己行之前**，其他组排在正式行后、休眠组前；两 toggle 独立；休眠行随组下移 | 改（现有 DOM 顺序改变） | 单 seam（同一网格内的顺序与状态） |
| 其他行就地下读（完整标题/逐字摘要/相关文章） | 新 `console/OtherTopicReading.vue`（复用 `LaneChronicleThread`/`LaneRelatedArticles`）+ 壳层 `selectedOther` + `BoardConsoleStage.vue` 阅读槽 | 新增 | 多 seam（读取 store 上提 + 选择状态 + 组件） |
| 报日索引/报告优先读取 | `composables/useLaneDayReading.ts`（索引暴露/`ensureReportDay`/双 epoch）+ `api/normalizers/laneDayReading.ts`（补 `topic_status_at_report`/`persistent_topic_id`） | 改 | 多 seam（store 单点，消费者经 props） |
| 选中日校正（报日回退/范围排出） | `console/laneConsoleView.ts` + `console/useConsoleSelection.ts`（新增 `reportDates` 依赖） | 改 | 单 seam（唯一落点，壳层只传参） |
| 窄屏同源日期条 + 组 | `console/BoardNarrowSummary.vue` + 壳层/Stage 透传 | 改 | 单 seam（写同一 `selectedDate`，不新增状态） |
| 单一读取 store 上提 | `BoardConsoleShell.vue`（创建 + 注入）+ `LaneChronicle.vue`（可选 `reading` prop，未注入自建） | 改 | 多 seam（壳/编年两侧签名必须同批落地，单独改一侧即坏） |
| 生成完成刷新索引 | `BoardConsoleShell.vue`（`refreshConsole` 后 `ensureReportIndex({force:true})`） | 改 | 单 seam |
| 右轨/头/全局治理/归档/阅读台/编排/研究 | 不动 | — | — |

**不动清单（foreign/他域，本增量禁改）**：`BoardContextRail.vue`/`RailHealthBrief.vue`/`RailSourcesBrief.vue`/`BoardWideWorkface.vue`、`BoardRadar*` 总览簇、`BoardArchiveView.vue`、`daily-report/**` 阅读台与 `BoardDailyReportTimeline.vue`、`BoardComposeOverlay.vue`/`useInlineCompose.ts`、`BoardThreadBrowser.vue`、`TopicManageDialog`/`WatchManagePanel`、后端全部（`lane_snapshot_repository.go`/`daily_report_*`/handler）、`docs/reference/**`。若实现期发现必须触碰，停工列准确 hunk + 理由交父协调。

## 6. 独占写域与小包分工（按 §0.6 互斥辖区；同一文件只能 1 个 writer）

| 包 | 独占文件 | 依赖 | 交付 |
| --- | --- | --- | --- |
| **G1 数据与模型** | `composables/useLaneDayReading.ts`、`api/normalizers/laneDayReading.ts`（及其 `.test.ts`）、`console/dayOtherTopics.ts`（新）、`console/laneConsoleView.ts`、`console/useConsoleSelection.ts`（及两个 `.test.ts`） | 无 | 报日索引/`ensureReportDay`/双 epoch/共享缓存；其他组纯模型（排除/状态机/文案）；选中日三级校正 |
| **G2 网格与组** | `console/ObservationMatrix.vue`、`console/ObservationMatrixGrid.vue`、`console/DayOtherTopicsGroup.vue`（新）+ 同名测试 | G1（读模型/类型） | 报日列头可选、0 行网格、组顺序/默认态/独立性/只标所选日、长标题两行 |
| **G3 阅读、跨日带与窄屏** | `console/OtherTopicReading.vue`（新）、`console/BoardNarrowSummary.vue`、新`api/otherTopicObservation.ts`（仅在既有API未归一时做边界适配）、`composables/useOtherTopicObservation.ts`及对应测试（不改G1的日报读取器） | G1、G2 | 其他分区阅读面板（复用thread/related）、获批同topicId本窗跨日观察带与节点切日、失败/未知状态、窄屏同源报日轴与组 |
| **G4a 状态/返回契约** | 新console内聚控制器；既有`useConsoleReturnContext.ts`/`consoleReturnContext.ts`/`useConsoleRefresh.ts`及其目标测试（按需要最小兼容扩展） | G1–G3、8C.4S | 唯一reading/observer组合、其他选择/默认态/刷新与生成失效、返回载体稳定board/report/section/date+阅读key、旧lane/四参/用户意图取消兼容；不写Vue |
| **G4b 壳层接线（最后串行收口）** | `console/BoardConsoleShell.vue`、`BoardConsoleBody.vue`（必要透传）、`BoardConsoleStage.vue`、`consoleStageTypes.ts`、`LaneChronicle.vue`及Shell测试/内聚helper；必要G2/G3重试意图透传、无泳道轴锚接线须列hunk | G4a | store上提注入/新旧事件接线、真实单阅读槽与返回恢复、元信息/报日独立首读；不改其他页面/统计。Shell与大测试不得超标准 |
| **G5 验收（父主导）** | `ui-verification/**`（新证据目录） | G1–G4 | 定向测试/typecheck/build 集中一次、真实静态 agent-browser 主故事、独立 review、部署影响说明 |

- G4前加入8C.4S纯结构规范包：G1索引职责/G2网格及三份大测试按code-style§8内聚拆分，每source/test≤500行/~15KB，公共接口/DOM不变、测试清单前后一致、更新实际资产路径。G4的Shell/其测试本就贴近或超过阈值，接线时也须合理分职责，不能再堆进巨型壳。
- 主仓库单writer，G1→G2→G3→8C.4S→G4全串行，各包全新短上下文线程，不复用长线程；G4最后收口。测试文件随源同组，不跨组共享写入。必要的既有返回上下文载体改动只归G4，在实际接线前定位并报告，不顺带重构导航。
- 明确禁止：在组件层散落 `String(id)`/raw snake_case 映射（转换只在 normalizer/store 边界）；在 G2/G3 组件内自行发请求或另建日期状态；把其他组写进 A 名单/排序/休眠/计数。

## 7. 验收命令与分段门禁（本批只跑轻量文档验证，不跑产品命令）

- **G1**：`cd front && pnpm test:unit app/features/tags/components/console/dayOtherTopics.test.ts app/features/tags/components/console/laneConsoleView.test.ts app/features/tags/composables/useLaneDayReading.test.ts app/api/normalizers/laneDayReading.test.ts --maxWorkers=2`（参数不带 `--`，路径从 `front/` 起算；新文件由 G1 同批创建）+ `pnpm exec eslint <本组 .ts>`。
- **G2**：`cd front && pnpm test:unit app/features/tags/components/console/ObservationMatrix.test.ts app/features/tags/components/console/DayOtherTopicsGroup.test.ts --maxWorkers=2` + eslint 同组 `.ts`。
- **G3**：`pnpm test:unit .../OtherTopicReading.test.ts .../BoardNarrowSummary.test.ts --maxWorkers=2` + eslint。
- **G4**：`pnpm test:unit .../BoardConsoleShell.test.ts .../LaneChronicle.test.ts .../ObservationMatrix.test.ts --maxWorkers=2`；`pnpm exec nuxi typecheck`（父集中一次；他域 `SignalReportView.test.ts` 7 条旧债如实保留）；`pnpm lint`由父收口批集中跑；不单独重复pnpm build，以下安全候选发布的单次generate同时承担构建验证（不与测试/浏览器并行）。
- **集成/独立 review**：本增量独立 review 建账（High/Medium 修复回写证据目录，未完成项进 `human-feedback.md`），不得用 UI 批准代替 review。
- **真实 UI**：静态托管 `:5100`（后端零改动不重启）跑 `test-cases-matrix-day-others.md` §真实浏览器计划（0 正式泳道冷启动 + 成熟板块不回归），1440×900/1920×1080/2560×1440/375×667/390×844、双主题；证据落 `ui-verification/matrix-day-others-<date>/report.md`。折线/触屏/元素命中类断言按既有 `elementFromPoint` 方法，不截图冒充。
- **候选发布→验收→留盘**：源码独立review/目标测试先通过，核当前静态基线/备份/源码指纹/foreign前端归属；若含未获部署许可的foreign UI或构建漂移，先停协调。仅一次`bash scripts/dev/deploy-frontend.sh --no-restart`完成generate并铺盘（不重复pnpm build、不重启后端），以新index/chunk/hash在:5100跑G5真实故事；失败回滚备份，通过后正式留盘并健康检查，不能先验旧版再称新稿通过。不commit/归档。
- 本批（8C.1）只跑：`openspec validate rework-workshop-into-radar-and-desk --strict`、`bash scripts/harness/check-tasks-tail.sh openspec/changes/rework-workshop-into-radar-and-desk`、`bash scripts/harness/test-assets.sh <capability>` 反查。禁止 pnpm/go/浏览器/构建。

## 8. 父裁决登记（准入结论，不得静默削减）

| # | 事项 | 本合同默认 | 需父裁决 |
| --- | --- | --- | --- |
| P1 | 获批同topicId跨日观察带 | **父裁决：纳入G3，不得删减冻结稿能力**；只读复用section-timeline，按本窗真实记录日展示/切日，当前topic属性不冒历史快照 | 已裁决，见§4.3；补spec/主故事/失败态与节点点击测试，不按“不适用”划除 |
| P2 | 范围排出所选日报日口径 | **父裁决：与冻结稿统一为窗内最新报日**；无报日为null，不跳今天或active日 | 已裁决，旧“最近/等距”提议不采用 |
| P3 | 主账 `test-cases.md` S11入口 | 父登记当前8C故事入口，保留旧账历史状态；本阶段Scenario映射补入tasks验证节 | 已纳入本次准入收口 |
| P4 | selectedDate URL与返回 | **父裁决：不新增date= query**，保持既有四参及刷新默认；其他话题经既有返回上下文稳定恢复board/report/section/date与阅读槽 | 已裁决；本次不新增可分享其他分区深链，不得以此省略返回恢复 |
| P5 | 窄屏同源报日轴 | **父裁决：保留**；同一连续visibleAxis/可选reportDates/selectedDate，桌面与窄屏互斥，每视口只有一条报日轴，不新增日历/独立选日器/前后日条 | 已裁决，不可变成手机不能选日 |

## 9. 部署影响预告（交付时向用户汇报的底稿）

前端单次安全静态候选发布并验收留盘（`bash scripts/dev/deploy-frontend.sh --no-restart`，包含generate），后端零改动不重启；无数据迁移/重生成/回填/配置。用户可见：0 正式泳道板块可按真实报日读当日全部分区与完整线索；成熟板块矩阵/滑筛/演进/内联阅读不变，多一个默认收起的「当日其他话题」；**休眠 summary 从底部前移到自身行前**；日期列头成为第三个同源选日入口；窄屏有同源报日条。旧日报缺历史状态字段时显示「当时状态未记录/当次未归属」，不补写。整 change 未完成、旧失败/未覆盖账目不因本增量清零。
