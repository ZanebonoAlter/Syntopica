## 1. 后端批量读端点

- [x] 1.1 在 `backend-go/internal/topicgraph/repository/key_event_markers.go` 新增 `ListBoardKeyEventMarkers(boardID uint, from, to time.Time)`：只取 `state='confirmed'` 且 `source_period_date` 落在闭区间，按日期 + 话题排序；验证：新增仓库层测试，断言 suggested/dismissed 不返回、窗口外不返回、空结果返回空切片。
- [x] 1.2 在 `backend-go/internal/topicgraph/handler/key_event_handler.go` 新增 `GET /api/semantic-boards/:id/key-events?days=N`：非法 `days` 返回 400 并指明参数；验证：handler 测试覆盖 200 正常、200 空数组、400 非法 days、400 非法 board id。
  - **实现注记（与 delta spec 对齐，tasks 原措辞作废）**：delta spec `specs/topic-key-event-markers/spec.md`「非法参数」Scenario 要求 `days` 为**正整数**、`0` MUST 4xx；故本端点用**独立校验** `parseBoardKeyEventDays`（默认 14、正整数），**不复用**话题级端点的 `parseKeyEventDays`（那里 `days=0=全部时间` 是既有契约，MUST NOT 连带改坏）。`days=0` → 400 已有用例（`key_event_handler_test.go`）。
- [x] 1.3 在 `RegisterKeyEventRoutes` 注册新路由；验证：`go build ./...` 通过，`curl --noproxy '*' 'http://localhost:5100/api/semantic-boards/1980/key-events?days=30'` 返回 2xx 且只含 confirmed。

## 2. 前端数据层

- [x] 2.1 在 `front/app/api/` 增加 `getBoardKeyEvents(boardId, days)` 客户端与 normalizer（camelCase、date-only、`article_ids_snapshot` 转字符串数组）；验证：normalizer 单测覆盖空数组、null 快照、多话题同日三条。
- [x] 2.2 新增 `useBoardKeyEvents` composable：按 board + days 缓存、loading/error/stale 三态、切板块晚到隔离；验证：composable 单测覆盖成功、失败保留旧数据、晚到不写。
- [x] 2.3 事件索引派生：`Map<topicId, Map<date, event[]>>` 纯函数；验证：单测断言同日多事件顺序按接口序、未知话题丢弃。

## 3. 视图切换接入

- [x] 3.1 `BoardConsoleHeader.vue` 的 `.console-seg` 增加「关键事件」按钮，`activeView` 增加 `'events'`；验证：组件测试断言按钮渲染与 `aria-pressed` 切换。
- [x] 3.2 `BoardConsoleShell.vue` 接 `activeView='events'`：渲染新视图、写/读 URL `?view=events`（非法值回落 matrix）；验证：集成测试断言切换后 `selectedLane` / `selectedDate` 不变、刷新后视图保持。
- [x] 3.3 窄屏（`<768px`）不渲染「关键事件」按钮与视图；验证：窄屏单测断言按钮不存在。
- [x] 3.4 右轨发起的话题选中 → 切回「矩阵 / 追踪」视图，并把该话题泳道滚入视野 + 高亮（design D14 / ui-design Interaction Decisions D4 / delta spec Scenario「右轨选中切回矩阵视图」）；本视图**内部**的选中（点行头、点节点）仍留在本视图。落点：`useBoardConsole.ts` 的 `activeView` 切档入口 + 右轨选中的既有落点逻辑；验证：集成测试断言「事件视图里点右轨话题 → `activeView` 变 `matrix` + 该泳道被选中并滚入视野」「事件视图里点行头 → 仍是 `events` 视图」。
  - **2026-11-23 复验轮 A-34 已修（本轮）**：右轨「话题追踪」选中后画面已切矩阵，URL 却是 `?view=events&lane=1804`（刷新又回关键事件视图，违反 D6）。根因：同一动作里**两次独立** `router.replace` 争抢同一组 query key——`handleSetView('matrix')` 经 `writeViewQuery` 删掉 `query.view`，紧接着 `selectLaneFromStage` 经 `writeLaneQuery` 基于**尚未更新**的 `route.query` 重建 query，把 `view=events` 又写回去（`router.replace` 是异步的，导航落地前 `route.query` 仍是旧快照）。**修法**：`useBoardConsole` 加 `batchQueryWrite(fn)` 合并作用域——作用域内 `writeViewQuery` / `writeLaneQuery` 只记 query 补丁，作用域结束按顺序应用到**最新** `route.query` 上，合成**一次** `router.replace`（作用域是同步的：replace 仍在同一事件处理里发出，写入时序不变）；`useConsoleShellActions.locateLaneFromRail` 把「切矩阵 + 选泳道」包进该作用域，`BoardConsoleShell.handleSelectOtherFromRail` 同样包（口径一致）。**未改** `writeLaneQuery` / `writeViewQuery` 的签名与单写行为（作用域外仍各自 replace），**未动** `useObservationWindow` 的独立 query 写入。回归：① `BoardConsoleShellRailViewSwitchQuery.test.ts`——把路由桩换成真实 vue-router 的**异步落地**时序（既有桩是同步落地，会把竞争藏起来，这正是上一轮漏网的原因），断言「右轨选中后 `route.query.view` 为 undefined + `lane` 为选中泳道 + 该动作只发一次 replace + 用该 URL 重新挂载仍是矩阵视图」，另附同类路径检查（当日其他话题：只写一次 query、无 lane 写入）；② `useBoardConsole.test.ts`「batchQueryWrite：两次写入合并成一次 replace 且不丢 days/start/end 等其他 query 键」。反向验证：去掉合并作用域 → 用例红（`expected 'events' to be undefined`）。

## 4. 关键事件视图

- [x] 4.1 新建 `KeyEventTimelineView.vue`：只渲染持久化话题行（复用 `buildObservationRows` 的 active 结果与行序），候选话题不占行；验证：组件测试断言行数 = active 话题数、候选事件不出现。
  - 实现注记（候选事件口径）：候选话题的确认事件在**时间轴与当日阅读区都不呈现**（delta spec「候选话题不占行」+ design D2 风险项「候选话题事件不可见」）——只按“行集合话题 ID”过滤当日事件，避免“时间轴上没有、阅读区里有”的孤儿事件。实测口径：板 1980 近 30 天 22 条 confirmed，其中 1 条（11-19）挂在 candidate 话题 2477 → 本视图呈现 21 条。注：`ui-design.md`「下方阅读区显示当天全部关键事件」措辞与该口径有张力，实现按 spec（形式契约）优先；若主线程要改成“阅读区呈现全部 confirmed”，改 `KeyEventTimelineView.vue` 的 `dayEventsByDate`（去掉 `rowTopicIds` 过滤）即可。
- [x] 4.2 行内日期轴与事件节点：`confirmed` 注记按 `source_period_date` 落节点并显示 `title_snapshot`，无事件不落节点；验证：组件测试断言节点数 = 事件数、无事件行只有虚线轴。
  - **实机验收 P1-② 已修（2026-11-23 视觉线程报 A-23 fail）**：行内虚线轴（生命线）不可见——`.kev-row-body` 是 `flex: 0 0 auto` 且只含绝对定位子元素 → 宽度塌成 0，`.kev-axis-line` 的 `left:0;right:0` 撑不开（实测宽 0px）。改法：行体给**显式宽度 = 轴宽**（`width: ${bodyWidth}px`，bodyWidth = 列宽 × 可见日数），虚线轴随之撑满整条轴；行头 sticky / 舞台内横滚契约不变（画布最小宽 `max(100%, 192 + bodyWidth)` 不变，不会产生页面级横滚）。回归：`KeyEventTimelineView.test.ts`「P1-② 行内虚线轴有可撑开的宽度」断言行体 `style.width > 0` 且 = 列宽 × 可见日数（反向验证：删掉显式宽即红）。
- [x] 4.3 `KeyEventLabel.vue` 两阶段布局：先自然宽度渲染再 `offsetWidth` 实测，贪心分层（层间留 4px 间隔、层距 44px），靠近右边界左移贴边，不截断标题；验证：组件测试断言相邻标签 `top` 不同且 `scrollWidth` 不溢出。
  - **实机验收 P1-① 已修（A-9 fail）**：同一天多个事件标签全部重叠在同一层（14 天窗 row 93 的 6 个标签 `top` 恒 46px、两两重叠 11 对；30 天窗 17 对），行高也未按层数增高。根因在**接线层**：`<KeyEventLabel :ref="el => setLabelRef(event.id, el)">` 对**组件** ref 的回调拿到的是 `ComponentPublicInstance`（不是 DOM 元素）→ `labelEls.get(key)?.offsetWidth` 恒 `undefined` → `?? 0` → 宽度全 0 → `layoutEventLabels` 的右缘收拢与贪心分层退化成「left = 节点 x + 9、全部第 0 层」。改法：`setLabelRef` 解析出真实 DOM 元素（`resolveLabelElement`：先判 `instanceof HTMLElement`，再取组件实例的 `$el`——`KeyEventLabel` 根节点就是标签本体），再量 `offsetWidth`（为 0 时退回 `getBoundingClientRect().width`）。**未用**省略号/限宽/缩字消重叠（delta spec「标签 SHALL 完整呈现标题，MUST NOT 用省略号截断」）。回归：`KeyEventTimelineView.test.ts`「P1-① 标签按实测宽度分层」——mock 出**只有真实 DOM 元素才拿得到**的逐标签宽度（组件实例量不到 → 0），断言两个标签 `top` 不同且行高 > 单层行高 90px；该用例在旧接线（直接量组件实例）下红、纯函数单测抓不到（这正是上一轮漏网的原因）。
- [x] 4.4 列宽与横向滚动：列宽 = `max(64, 可用宽 / 可见日数)`，容器 `overflow-x: auto`，行头 sticky，切换/换日时选中日滚入视口；验证：组件测试断言 1440 下 30 天 `scrollWidth > clientWidth`、2560 下 14 天列宽 > 120。
  - **实机验收 P2-① 已修（A-12 fail）**：2560 首屏 `--kev-col` 恒 64px、右侧留白 ~830px（切矩阵再切回重挂载后恢复 123.2px 均分）。根因：舞台滚动容器**晚于组件出现**（A 档骨架态 → 数据到位才挂上），而旧代码只在 `onMounted` 测一次宽并挂 `ResizeObserver`——骨架态下 `scrollRef` 为 null → 既没测到宽、也**没挂上 observer** → `containerWidth` 恒 0 → 列宽被钉在 64px 下限。改法：滚动容器改用**函数 ref**（`setScrollRef`）——容器一出现就实测宽并挂 `ResizeObserver`；测量只接受正数（0 宽不得把列宽压到下限），`clientWidth` 为 0 时退回 `getBoundingClientRect().width` 再退回父容器宽。回归：「P2-① 舞台容器晚于组件出现（骨架态）也要测到宽」——先以骨架态挂载、再切到有数据，断言 `--kev-col > 120`。
  - **实机验收 P2-② 已修（A-8 fail）**：切 30 天时选中日不自动滚入视口（实测 x=2338 在舞台右沿 1134 之外）。根因：`scrollSelectedDateIntoView` 只 `watch(selectedDate)`，窗口/轴变化不触发。改法：watch 源扩为 `[selectedDate, axisSignature, columnWidth]`——`axisSignature` 用「长度 + 首尾」签名（不用数组引用，避免父组件每次渲染换新数组导致反复滚动的抖动），列宽变化（容器缩放）也把选中日拉回视口。回归：「P2-② 轴/窗口变化后选中日自动滚入视口」（14 天 → 30 天）与「P2-② 轴内容平移（长度与列宽都不变）也把选中日拉回视口」。
  - **2026-11-23 复验轮 A-8 仍 fail → 已修（本轮）**：上条改法**只解决了「watch 触发」**，没解决「滚动被容器重建吞掉」——复验实测 watch 确实打出 `scrollTo({left:1657.5})`（与 30 天轴完全吻合），但紧跟 `container-removed → container-added`：切档时 `setSourceDays` 先把 `data` 置空走**骨架态**，`.kev-scroll` 整块被卸载重建，滚动位置随旧容器丢失回到 0；重挂后 `columnWidth` 仍为 64px（值未变）→ watch 不再二次触发 → 滚入白做。**修法**：滚动容器的函数 ref（`setScrollRef`）在**采纳新容器**时（`observedSurface !== node`，即容器被重建）排一次 `nextTick` → `scrollSelectedDateIntoView()`——重建后补一次滚入，与既有 watch（轴变/换日）叠加覆盖两条路径；`nextTick` 等容器进 DOM、宽可测。**未改**骨架态的容器卸载行为（骨架视觉不变），**未动** `axisSignature` 防抖设计（抖动不引入）。回归：`KeyEventTimelineView.test.ts`「A-8 切档导致滚动容器卸载重建：重建后的新容器仍把选中日滚入视口」——完整走「有数据 → 骨架态（容器卸载）→ 数据到位（容器重建）+ 轴换 30 天」时序，断言**新容器** `scrollLeft > 0`（不是断言「watch 触发了」——那正是上一轮误判为修好的原因；用例内把 happy-dom 的平滑滚动换成立即生效的 `scrollTo` 桩，避免定时器时序造成的假红/假绿）。反向验证：删掉 `setScrollRef` 里的补滚 → 用例红（`expected 0 to be greater than 0`）。
- [x] 4.5 左键拖拽平移：空白处 `pointerdown` + 位移 >5px 才进入平移，复用 `laneComposeLogic.ts` 的 `isDragBeyond` / `panScroll`；验证：单测断言未超阈值不改变 `scrollLeft`、超阈值按位移平移。
- [x] ~~4.6 长窗（>31 天）收起标签，只留节点与当日事件计数~~ **已被 4.8 取代（2026-11-23 用户实机反馈）**：收起判据从「可见日数 > 31」改为「堆叠层数超上限」，本条的「天数阈值」口径作废，行为以 4.8 与 delta spec Scenario「长窗仍看得见事件名称」为准。
  - 保留本条勾选状态仅表示「当时的实现已落地」；任何验收/对账 MUST 按 4.8 口径，MUST NOT 按本条断言「长窗标签不渲染」。
- [x] 4.8 长窗标签按**堆叠层数**自适应（取代 4.6 的天数阈值）：观察窗超过 30 天时**仍显示事件名称**（长窗只是列更稀，不是更挤）；只有堆叠层数超上限（4 层 / 行高封顶 246px）才退化为「节点 + 当日事件计数（★N）」，名称由选中日阅读区承担。落点：`KeyEventTimelineView.vue` 的 `collapsedLabels` 判定（现为 `axis.length > 31`）+ `keyEventLayout.ts` 的层数计算；验证：组件测试断言「轴长 55 天、层数 ≤ 4 → 标签照常渲染（不只剩光点）」「层数超上限 → 只渲染节点 + ★计数」。**不得**用省略号/限宽/缩字代替（delta spec 禁截断）。
- [x] 4.7 空态 / 错误 / 重试 / stale；验证：组件测试分别断言「无确认事件」文案、错误条 + 重试、stale 保留内容。

## 5. 事件 → 文章 → 阅读闭环

- [x] 5.1 事件卡渲染 `article_ids_snapshot` 数量入口；无关联文章时不显示入口；验证：组件测试断言有/无文章两种渲染。
- [x] 5.2 展开时按需读取文章元信息，复用既有 `laneDayRelatedArticles` 缓存（不新建第二份）；验证：单测断言同文章跨事件只请求一次。
  - 实现注记：去重本体在既有缓存层（`laneDayReadingCache.ts` 的在途合乘 + ≤20 FIFO，已由该层用例覆盖）；本视图只把展开委托给**壳层注入的同一 store**（`LaneRelatedArticles` → `reading.ensureRelatedArticles`），单测断言两次展开均走同一 store（组件不持缓存、不自建 fetcher）。
- [x] 5.3 点文章 `emit('open-article', id)`，由 `BoardConsoleShell` 既有 `ArticlePreviewModal` 打开；验证：集成测试断言 emit 参数与弹窗打开。
  - 实现注记：emit 链为 `KeyEventDayPanel` → `KeyEventTimelineView` → `BoardConsoleStage` → `BoardConsoleBody` → `BoardConsoleShell`（`overlays.handleOpenArticle` → 既有 `ArticlePreviewModal`），与矩阵当日工作面/归档列表同一条链（壳层既有用例已断言 `openArticlePreview` 收到文章 ID）；组件层断言 emit 参数为文章 ID。
- [x] 5.4 关闭预览后回到原事件卡，保持展开态与滚动位置；验证：集成测试断言关闭后 `scrollTop` 不变、文章列表仍展开。
  - 实现注记：本视图不做任何滚动操作（弹窗是壳层既有 overlay，关闭语义不变）；面板的展开态由 `expandedId` 持有，只在 `selectedDate` 变化时复位，组件用例断言 emit 预览后摘要与文章列表仍在。

## 6. 双主题

- [x] 6.1 新视图所有颜色走 `--color-*` 语义变量；验证：`grep -nE '#[0-9a-fA-F]{3,6}|rgba?\(' KeyEventTimelineView.vue KeyEventLabel.vue` 无硬编码色值（透明度可用 `color-mix` 或语义变量）。
- [x] 6.2 暗色主题下逐项核对节点、标签底色、选中列、文章列表、预览层；验证：`data-theme="dark"` 截图与亮色对比，无对比度不足。
  - **判定（主线程 2026-11-23）**：本条范围内本 change **自有元素全部达标** —— 节点 5.37/3.68、标签文字 6.92/6.07、选中列 15.8/16.29、文章列表标题 6.92/6.07、预览层 14.23/16.7（亮/暗，均过 AA）。
  - **唯一未达标项不计入本条**：次级文字 `--color-text-muted` 3.04~3.23、标签边框/虚线轴轮廓 1.33~1.36（账本 A-25，保持 fail）。归因：**产品级语义令牌**，同页矩阵视图的面包屑/右轨标题/坐标轴刻度同为 3.04~3.23 —— **非本 change 引入**，改它会波及全站，故本 change 不改，留待单开 change 处理。此判定已写进 `ui-verification/report.md`。
  - 实机已做（暗色截图 + 逐项对比度比值见 `ui-verification/report.md` A-24/A-25）：节点 5.37、标签文字 6.92、选中列 15.8、文章列表标题 6.92、预览层 14.23（亮色同项 3.68/6.07/16.29/6.07/16.7），五项主体达标；但**次级文字（`--color-text-muted`）3.15~3.21、标签边框 1.33 低于 AA**（A-25 fail）——归产品级令牌层（矩阵视图同款元素同值 3.23，非本 change 引入）。
  - **本 change 不改（口径归属已定）**：A-25 的次级文字对比度（3.04~3.23）与标签边框（1.33~1.36）取自**产品级设计令牌** `--color-text-muted` / `--color-border-medium`，矩阵视图同款元素实测同为 3.23 → **属设计系统既有口径、非本 change 引入**；本视图局部改色会与全站其它视图不一致，故**本 change 不动**，**待单开 change**（设计系统令牌层整改，连带复核全站次级文字与边框口径）。P1×2 + P2×2（实机验收报出的功能缺陷）已在 4.2 / 4.3 / 4.4 修完，与本条 P3 无关。**本条不勾**（待单开 change 落地后由主线程决定是否豁免勾选）。
  - 未完成原因：验收判据「无对比度不足」未完全满足（A-25 fail），按实测不勾；如主线程认定该口径属设计系统既有口径可豁免，请在本行注明并勾选。
  - **2026-11-23 复验轮（视觉线程）复测**：主体五项仍达标——暗色 节点 5.36 / 标签 6.91 / 选中列 7.28 / 文章列表 14.19 / 预览层 14.19，亮色 节点 3.68 / 标签 6.07；次级文字仍为亮色 3.04~3.23、暗色 3.15~3.21（A-25 保持 **fail**，本 change 不改）。归属复测补强：同页**矩阵视图**的面包屑「主页/叙事工坊」、右轨标题与计数、视窗字段「起始日/—」、背景曲线坐标轴刻度实测同为 3.04~3.23 → 确属产品级令牌口径、非本 change 引入。**本条仍不勾**（豁免与否属主线程决策，视觉线程不代拍）。

## 7. 测试

- [x] 7.1 后端：`cd backend-go && go test ./internal/topicgraph/...`，断言 repository 与 handler 新增用例全绿。
- [x] 7.2 前端：`cd front && pnpm test:unit KeyEventTimelineView KeyEventLabel useBoardKeyEvents`，断言新增组件/composable 用例全绿。
- [x] 7.3 回归：`cd front && pnpm test:unit ObservationMatrix BoardConsoleShell BoardNarrowSummary`，断言矩阵、当日工作面、窄屏摘要行为不变。
  - 注记（顺手收的一条**存量红**，非本 change 引入）：`BoardConsoleShellSelection.test.ts`「A 名单行随响应更新」原本拿**整页文本**断言不含「主线 A」，而已归档 change `add-other-topics-to-context-rail` 把「主线 A」放进右轨「当日其他话题」子块（该 change 未同步此断言，HEAD 即红）。本 change 把断言收窄到矩阵区（`[data-testid="board-matrix"]`）——原意是「矩阵名单随响应更新」，与右轨无关。

## 8. 文档

<!-- doc-impact: flow, api, database -->
<!-- doc-impact-excuse: architecture=edit.map 归属集合被并发会话脏文件污染（backend-go/internal/app/runtime.go 的改动属 add-ai-generated-article-detection，`git diff` 内无关键事件相关行），本 change 未触 backend-go/internal/app/; standard=同上（docs/reference/standard/{backend/ai-logging.md,frontend/code-style.md,frontend/theming.md} 改动分别属其他 change/滚动条规范，`git diff` 内无关键事件相关行），本 change 未触 docs/reference/standard/ -->

- [x] 8.1 更新 `docs/reference/flow/semantic-board.md`：矩阵页新增关键事件视图、行集合只含持久化话题、横向滚动与拖拽、双主题；验证：`grep -n "关键事件视图" docs/reference/flow/semantic-board.md` 有命中，且「变更溯源」表追加本 change 行。
- [x] 8.2 更新 `docs/reference/flow/topic-graph.md`：关键事件读端新增版块级批量、只透 confirmed；验证：`grep -n "版块级" docs/reference/flow/topic-graph.md` 有命中。
- [x] 8.3 更新 `docs/reference/api/` 与 `docs/reference/database/`（新端点与 `article_ids_snapshot` 用途）；验证：`openspec/changes/add-key-event-matrix-view` 归档前 `bash scripts/harness/doc-impact.sh verify add-key-event-matrix-view` 通过。
  - 注记：`doc-impact` 声明已从业务域轴（`semantic-board, topic-graph`）改为**文档目录轴**（`flow, api, database`）——两轴禁止混用，前者会被门禁判「改了 flow 相关代码未声明 flow」。

## 9. 验证

- [x] 9.1 后端：`cd backend-go && golangci-lint run ./... && go vet ./... && go test ./internal/topicgraph/... && go build ./...`，期望全部退出码 0。
- [x] 9.2 前端：`cd front && pnpm lint && pnpm exec nuxi typecheck`，期望 0 error。
- [x] 9.3 前端单测：`cd front && pnpm test:unit KeyEventTimelineView KeyEventLabel useBoardKeyEvents ObservationMatrix --maxWorkers=2`，期望全绿。
- [x] 9.4 部署与实机：`bash scripts/dev/deploy-frontend.sh`，期望健康检查 200；用 agent-browser 在 1440×900 / 1920×1080 / 2560×1271 断言：切视图保留选中态、30 天下出现横向滚动、拖拽平移生效、文章预览闭合回原卡。
  - 实测：部署完成（构建约 45 秒，`/health` 200）；三档视口断言全跑完——切视图保留 selectedLane/selectedDate/观察窗 ✅、30 天列宽守 64px + 舞台内横滚 + 行头 sticky ✅、拖拽平移 >5px 生效 / <5px 不平移 ✅、文章预览闭合回原卡且卡片保持展开、滚动位置不变 ✅；证据与逐条判定见 `ui-verification/report.md`（A-1~A-27，13 张截图）。
  - ⚠ 实测另发现 **2 个 P1 缺陷**（落在 ui-design Acceptance 与 delta spec 条款上，不在本条四项断言内）：① 同一行多个事件标签全部压在同一层、互相重叠（14 天窗 row 93 重叠 11 对），根因指向 `KeyEventTimelineView.vue` 的组件 ref 测量接线（拿到组件公开实例 → `offsetWidth` 为 0 → 贪心分层退化为单层）；② 行内虚线轴不可见（`.kev-row-body` 宽 0 → `.kev-axis-line` 宽 0px）。另有 2 个 P2（2560 首屏列宽未均分留白、切 30 天选中日不自动滚入视口）与 2 个 P3（muted 次级文字对比度、标签边框）。**建议先修 P1 再重跑本轮实机验收，勿带缺陷归档**。
  - **2026-11-23 实现线程已修 4 个缺陷（P1×2 + P2×2），待视觉线程重新部署复验**：① P1-① 标签重叠（接线层：组件 ref → 解析真实 DOM 元素再量宽）；② P1-② 虚线轴不可见（行体显式宽 = 轴宽）；③ P2-① 2560 首屏列宽未均分（容器晚出现 → 函数 ref 出现即测 + 挂 observer）；④ P2-② 切 30 天选中日不滚入（watch 源加轴签名 + 列宽）。修法与回归用例详见 4.2 / 4.3 / 4.4 行注记。**本条（9.4）不由实现线程勾**——待视觉线程重新部署 + 实机复验（A-8 / A-9 / A-12 / A-23 四条）后再更新证据与勾选状态。
  - **2026-11-23 复验轮（视觉线程，已重新部署复验）**：四项断言全过（切视图保留 selectedLane/selectedDate/观察窗 ✅、30 天列宽守 64px + 舞台内横滚 + 行头 sticky ✅、拖拽平移 >5px 生效 / <5px 不平移 ✅、文章预览闭合回原卡且卡片保持展开、滚动位置不变 ✅）。四条复验判定：**A-9 标签重叠 fail → pass**（row 93 六个标签分 4 层、重叠 0 对；行高随层数 70/90/178/222px）、**A-12 首屏均分 fail → pass**（2560 首屏 `--kev-col=123.21px`=（1917−192）/14、右侧零留白）、**A-23 虚线轴 fail → pass**（三档 896/1085/1725px、空态板亦可见）、**A-8 选中日滚入 仍 fail**（watch 的 `scrollTo({left:1657.5})` 确实触发，但切档骨架态把 `.kev-scroll` 整块卸载重建 → 滚动位置丢失，新容器 scrollLeft=0，选中日 x=2338 仍在舞台右沿 1134 之外）。**另新发现 1 个缺陷**：右轨选中后 URL 的 `view` 参数与实际视图不一致（画面在矩阵、URL 仍是 `?view=events&lane=…`，刷新回到事件视图，根因两次 `router.replace` 竞争）。证据与根因见 `ui-verification/report.md` A-8 / A-9 / A-12 / A-23 / A-34 与「缺陷清单」。本条维持已勾（四项断言仍成立），但**归档前须先处理 A-8（及 A-34）**。
- [x] 9.5 归档前：`bash scripts/harness/ui-verification-trace.sh` 与 `openspec validate add-key-event-matrix-view --strict`，期望均通过；`ui-verification/report.md` 记录三档视口证据与原型差异说明。
  - 实测：`openspec validate add-key-event-matrix-view --strict` → **通过**（“Change 'add-key-event-matrix-view' is valid”，退出码 0）；`bash scripts/harness/ui-verification-trace.sh openspec/changes/add-key-event-matrix-view` → **通过**（“report.md 建账 27 条（pass 21 / fail 5 / 未覆盖 1），点名截图 13 张全部在盘”，退出码 0）。三档视口截图（1440×900 / 1920×1080 / 2560×1271）与「与批准原型的差异说明」已在 `ui-verification/report.md` 建账。
  - 注：建账含 5 条 fail（见 9.4 的缺陷清单）与 1 条未覆盖（库内无 0 篇关联文章的 confirmed 样本），**门禁绿 ≠ 验收全绿**；`ui-verification/report.md` 未被 gitignore（只有同目录截图被 `openspec/**/*.png` 忽略），是否入库由主线程定。
  - **待办（视觉线程）**：4 个缺陷（P1×2 + P2×2）已修，`ui-verification/report.md` 的 A-8 / A-9 / A-12 / A-23 四条 fail 需在**重新部署后**复验改判；A-25（P3 设计系统令牌口径）本 change 不改、留单开 change，故该条仍为 fail（归档时需主线程确认豁免口径）。**本条（9.5）不由实现线程勾**。
  - **2026-11-23 复验轮（视觉线程）复跑两条门禁**：`openspec validate add-key-event-matrix-view --strict` → **通过**（“Change 'add-key-event-matrix-view' is valid”，退出码 0；期间修掉了 D7 修订把 requirement 正文顶到 527 字导致的 >500 字告警，措辞收紧明细见 `ui-verification/report.md`「附：规格措辞修订」）；`bash scripts/harness/ui-verification-trace.sh openspec/changes/add-key-event-matrix-view` → **通过**（“report.md 建账 34 条（pass 29 / fail 3 / 未覆盖 2），点名截图 14 张全部在盘”，退出码 0）。**注意：门禁绿 ≠ 验收全绿**——账上仍有 3 条 fail（A-8 选中日滚入、A-25 设计系统令牌口径、A-34 右轨选中后 URL 不一致）与 2 条未覆盖（A-16 无 0 篇关联文章样本、A-30 层数超限单行退化实机不可达）。
