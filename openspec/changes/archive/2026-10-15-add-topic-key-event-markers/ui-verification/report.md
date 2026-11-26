# UI 人工验收报告：add-topic-key-event-markers（tasks.md 4.4）

- 执行日期：2026-10-13 08:46 – 09:15
- 环境：静态站 + 后端同端口 `http://localhost:5100/`（2026-10-13 部署产物，真实数据）；agent-browser 无头浏览器，session `ke-acceptance`
- 板块：「生成式 AI 与大模型厂商」（board id **1980**，tag_count 623）；最新日报 **1088**（2026-10-12，26 篇 · 6 话题）；泳道话题 **89**「全球 AI 算力成本上升引发的计费与管控变革」；线索 thread **13709**（用户申请Claude Max 20x替代即将到期的ChatGPT Pro）/ **13710**（用户对比Claude与OpenAI：封号与降智限速的取舍）
- 预算合规：全程仅触碰 thread 13709/13710 两条线索；无任何「生成日报/重新生成/页边注问答」类操作；未跑测试/构建、未改源代码。终态两条 marker 均 `state=dismissed`、confirmed=0，自洽收尾。

## ① 执行时间线（动作 + 观察 + 网络结果）

| 时间 | 动作 | 观察 / 网络结果 |
| --- | --- | --- |
| 08:46 | `open /tags`（1440×900）→ 选板块 1980 → 切「日报」页签 | 左栏 12 板块真实数据；「生成日报」按钮可见未触碰；期刊卡列表 10/12–10/7 |
| 08:48 | 点 10/12 期刊卡进全屏 reader；话题 01 泳道展开 2 条线索 | `GET /api/daily-reports/1088` 200；`GET /api/daily-reports/1088/key-events` 200 → `{markers:[], status:"idle"}` → **本期无 AI 建议**（A1 按 brief 转 A2 手标路径）；截 `ke-reader-topics-1440.png` |
| 08:49 | A2①：鼠标点 thread 13710「标为关键」 | `PUT /api/daily-reports/topics/89/key-events/threads/13710` **200**；行内变「已标记 \| 取消标记」；截 `ke-reader-marked-1440.png` |
| 08:50 | A8+A2②：点线索 header 后按 Tab → 焦点落「取消标记」（原生 `<button.ke-actions__btn--unmark>`）→ Enter | `PATCH /api/daily-reports/topics/89/key-events/1` **200**；行内变「已不采纳 \| 仍标为关键」（取消=dismissed、可再手标，见⑤-P2） |
| 08:51 | A2③：鼠标点 thread 13709「标为关键」 | `PUT …/key-events/threads/13709` **200**；「已标记 \| 取消标记」；截 `ke-reader-confirmed-1440.png` |
| 08:52 | A3：F5 刷新 → 重选板块 → 重进日报 → 重开 1088 | `GET /api/daily-reports/1088/key-events` 200；13709 仍「已标记」、13710 仍「已不采纳」→ **持久化 pass** |
| 08:54 | A4：进话题总览 → 「话题泳道」lanes 视图 | 每话题 `GET /api/daily-reports/topics/{89,93,94,88,718,1654…}/key-events?days=14` 全 200；泳道 89 节点（2026-10-12, section 6535）右上出现 `text.btb-node-ke-badge`「**★1**」，`fill=rgb(217,74,74)`（红色 accent）；dismissed 的 13710 **不产生**徽标；`GET /topics/89/key-events` 仅返回 1 条 confirmed → **只显 confirmed pass**；截 `ke-overview-lanes-star-1440/-star-zoom-1440.png` |
| 08:55–08:57 | A5：点泳道 89 进 focus（8 列节点，末列 ★1）→ 开「突出关键节点」→ 关 | aria-pressed false→true→false，label「突出关键节点」↔「取消突出」；开：7 条无 marker 列 `opacity 0.32`、★ 列 `opacity 1`，节点数 8→8 不变、坐标逐一比对不变、`elementFromPoint` 命中可点（点击变暗节点成功打开详情）；关：全部恢复 opacity 1；截 `ke-focus-highlight-on-1440.png`、`ke-focus-normal-1440.png` |
| 08:58 | A6①：focus 点 ★ 节点列 → 原位详情（★ 快照标题 + 「来源日报 2026-10-12」+「取消」+ 原新闻保留）→ 点「来源日报」 | **无任何反应**（无导航、无新增网络请求）→ 定位为 P1 缺陷（见⑤）：此实例是 `BoardDailyReportTimeline` 内嵌的 BoardThreadBrowser，**未绑定 `@open-daily-report`** |
| 09:00–09:05 | A6②：改走顶部「话题总览」页签独立实例：lanes → 泳道 89 → focus → ★ 节点详情 → 点「来源日报」 | 页签切回「日报」（`tags-content-tab--active`）+ reader 深链打开并定位 **2026-10-12**（`GET /api/daily-reports/1088` 200，masthead「10 月 12 日 · 周一」）；线索行 13709「已标记」→ **跳转定位 pass**；截 `ke-focus-detail-panel-1440.png`、`ke-source-report-jump-1440.png` |
| 09:06 | A11：`network route "**/key-events/**" --abort` → 点详情内「取消」 | `PATCH /api/daily-reports/topics/89/key-events/2` 被拦截（无 status）；行内出现 `role=alert`「**保存结果未知，已重新读取最新状态，请重试（可重试取消）**」；按钮恢复可点（disabled=false）；★ 未丢；随后自动 `GET /api/daily-reports/1088/key-events` 200（结果不明→重读语义）；截 `ke-focus-cancel-error-1440.png` |
| 09:07 | A7：解除拦截 → 重试「取消」 | `PATCH …/key-events/2` **200**；★ 全部消失、focus 列数 8→8（**背景节点一个不少**）、错误提示清除；回 lanes：徽标 0、泳道仍在；回日报 reader：两线索均「已不采纳 \| 仍标为关键」（非确认态）；截 `ke-focus-after-cancel-1440.png`、`ke-overview-lanes-after-cancel-1440.png`、`ke-reader-after-cancel-1440.png` |
| 09:09 | A9：顶栏「切换为深色模式」→ 拍深色 reader + 深色总览；后恢复 editorial | `data-theme` editorial↔dark 切换正常；深色下 lanes 七条泳道/日期标尺/五态节点色渲染正常；截 `ke-reader-dark-1440.png`、`ke-overview-dark-1440.png` |
| 09:11–09:12 | A10：`set viewport 375 812`；日报 reader 与总览 lanes 双检查 | reader：`document.scrollWidth=375`（≤390 ✓）、标记按钮滚动可达、`elementFromPoint` 命中、**真实点击「仍标为关键」→「已标记」**（`PUT …/threads/13709` 200）→ 点「取消标记」还原（`PATCH …/2` 200）；总览：`scrollWidth=375` 无整页横溢，轴容器 `.btb-svg-scroll` 内部滚动（scrollWidth 2316 > clientWidth 285，90 个 DAG 节点全在）；截 `ke-reader-marked-375.png`、`ke-overview-lanes-375.png` |
| 09:14 | 终态对账 | `GET /1088/key-events`：marker 1/2 均 `state=dismissed`；`GET /topics/89/key-events`：confirmed=0；主题已恢复 editorial；`agent-browser close` |
| 11:41–11:50 | **P6 复验（session `ke-dark-fix`）**：先在旧构建上复现——dark 下 lanes 点节点开 popup，computed style 实测 `.btb-popup` `color=rgb(0,0,0)`（继承 body 的 UA 默认黑）配深底 `rgba(17,27,38,0.98)`、`.btb-popup-status` `color:white` 配提亮填充 `rgb(156,163,175)` | 确认 P6a 黑底黑字根因（详见⑤-P6a）；修复 `BoardThreadBrowser.vue` 后单测/typecheck/lint 过，`deploy-frontend.sh` 重部署 |
| 11:53–11:59 | P6 修复后双主题实测：API 手标 thread 13245（topic 2，PUT 200，marker 5）→ dark lanes popup / dark focus inline / editorial popup / dark 375 popup 四轮 computed style + 截图 | dark popup `color=rgba(255,255,255,0.9)`、标题 serif 加粗可换行、摘要 `rgba(255,255,255,0.35)` 换行展示、状态胶囊深字 `rgb(26,26,26)` 配亮填充；editorial 无回归（`rgb(26,26,26)` 配暖白底）；375 无横溢；截 `ke-detail-dark-1440.png`、`ke-focus-dark-1440.png`、`ke-detail-editorial-1440.png`、`ke-detail-dark-375.png` |
| 12:00 | P6 终态对账 | `PATCH …/topics/2/key-events/5` unmark 200 → `GET /topics/{1,2}/key-events` confirmed=0；viewport 已还原 1440 |

## ② A1–A11 逐条判定

| # | 判定 | 依据 |
| --- | --- | --- |
| A1 日报确认建议 | **未覆盖（本期无建议，按 brief 转 A2）** | 7 个板块最新日报 `GET /key-events` 全部 `status:"idle"`、`markers:[]`；reader 内 0 个「AI 建议」徽标（DOM 全文不含）；遵守红线未触发任何生成类任务 |
| A2 无建议手标 | **pass** | 鼠标「标为关键」→「已标记 \| 取消标记」（PUT 200）；「取消标记」→ 非确认态、出现「仍标为关键」可再手标入口（PATCH 200）。注：取消落在「已不采纳」标签态，系 spec 设计语义（见⑤-P2），满足 brief「回到未标态（可用）」意图 |
| A3 刷新持久化 | **pass** | F5 + 重选板块 + 重开同日报后 13709「已标记」/13710「已不采纳」保持；`GET /1088/key-events` 复读一致 |
| A4 总览只显 confirmed | **pass** | confirmed 的 13709 → 泳道节点右上红色 `★1`（#d94a4a，`title="该来源 1 条已确认关键线索"`）；dismissed 的 13710 无徽标；话题读端只返回 confirmed（`/topics/89/key-events` 无 dismissed 行） |
| A5 突出模式 | **pass** | 开关 `aria-pressed` 正确翻转；开启后无 marker 的 7 列 opacity 0.32、★ 列 1；节点集/坐标不变（逐列 rect 比对）、变暗节点可点击（点击成功开详情）；关闭恢复 1 |
| A6 原位详情与来源日报导航 | **pass** | 详情面板：★ 快照标题、来源日期、「来源日报 2026-10-12」「取消」、原新闻列表俱在；顶部实例点「来源日报」→ 跳回日报页签并深链打开 2026-10-12 reader。首轮内嵌实例按钮无响应（P1），**2026-10-13 修复后复验 pass**：内嵌实例同按钮 → reader 深链打开并定位 2026-10-12、控制台零报错（证据 `ke-recheck-a6-1440.png`） |
| A7 取消与背景 | **pass** | focus 详情「取消」→ PATCH 200；★ 消失、focus 8 列与 lanes 泳道/节点全部保留（只少星）；错误提示清除；日报侧同线索回非确认态（跨入口缓存同步） |
| A8 键盘 | **pass** | 线索 header 上 Tab 一次即聚焦 ke 按钮（原生 `<button type=button>`，class `ke-actions__btn--unmark`），Enter 触发真实写请求（PATCH 200） |
| A9 双主题 | **pass** | 顶栏「切换为深色模式/浅色模式」入口存在可用；editorial/dark 各有总览+日报截图（4 张，见③）；深色下泳道/状态色/文字对比正常 |
| A10 375px | **pass** | reader 与总览 `document.scrollWidth` 均 =375（无整页横溢）；时间轴局部滚动（2316>285）；标记按钮可见、命中测试通过、**真实点击生效**（标记→取消全链路 200）；可见按钮尺寸 100×25（命中区规范针对 canvas 节点，见⑤-P5） |
| A11 失败回滚 | **pass** | `network route --abort` 拦截 `**/key-events/**` 后点取消：行内 `role=alert` 错误「保存结果未知，已重新读取最新状态，请重试（可重试取消）」、按钮恢复可点、★ 不丢、自动重读报告注记（GET 200）；解除拦截后幂等重试成功 |

**总判定：10 pass（含 A6 修复后复验轮）/ 1 未覆盖（A1，环境无建议数据）→ 4.4 主故事除「在日报确认 AI 建议」外全链路走通；P1 已修复并复验 pass。**

## ③ 截图清单（同目录 ui-verification/，命名 ke-<场景>-<视口>.png）

| 文件 | 状态 | 视口 |
| --- | --- | --- |
| `ke-reader-topics-1440.png` | reader 话题正文、线索行未标（两个「标为关键」） | 1440×900 |
| `ke-reader-marked-1440.png` | A2 标记后（「已标记 \| 取消标记」） | 1440×900 |
| `ke-reader-confirmed-1440.png` | thread 13709 已确认态（A3 前基线） | 1440×900 |
| `ke-overview-lanes-star-1440.png` / `-star-zoom-1440.png` | A4 lanes 视图 ★1 红徽标（滚动态/居中态） | 1440×900 |
| `ke-focus-highlight-on-1440.png` | A5 突出模式开（普通节点变暗、★ 列保持） | 1440×900 |
| `ke-focus-normal-1440.png` | A5 突出模式关（全部恢复） | 1440×900 |
| `ke-focus-detail-panel-1440.png` | A6 ★ 节点原位详情（快照标题/来源日报/取消） | 1440×900 |
| `ke-source-report-jump-1440.png` | A6 「来源日报」跳转后 reader 定位 2026-10-12 | 1440×900 |
| `ke-focus-cancel-error-1440.png` | A11 网络拦截下取消失败的行内错误提示 | 1440×900 |
| `ke-focus-after-cancel-1440.png` | A7 取消后 focus（★ 消失、背景 8 列全在） | 1440×900 |
| `ke-overview-lanes-after-cancel-1440.png` | A7 取消后 lanes（徽标 0、泳道在） | 1440×900 |
| `ke-reader-after-cancel-1440.png` | A7 取消后日报侧（两线索非确认态） | 1440×900 |
| `ke-reader-dark-1440.png` | A9 深色 reader（含线索行动作区） | 1440×900 |
| `ke-overview-dark-1440.png` | A9 深色总览泳道 | 1440×900 |
| `ke-reader-marked-375.png` | A10 375px reader 标记态（移动端可点证据） | 375×812 |
| `ke-overview-lanes-375.png` | A10 375px lanes（无整页横溢、轴局部滚动） | 375×812 |
| `ke-recheck-a6-1440.png` | P1 复验：内嵌实例「来源日报」点击后 reader 深链定位 2026-10-12 | 1440×900 |
| `ke-detail-dark-1440.png` | P6a/P6b 修复后：dark lanes popup 详情（serif 标题+摘要+右侧动作、状态胶囊可读） | 1440×900 |
| `ke-focus-dark-1440.png` | P6b 修复后：dark focus ★ 列 inline 详情（原型形态） | 1440×900 |
| `ke-detail-editorial-1440.png` | P6 修复后：editorial popup 详情（无回归对照） | 1440×900 |
| `ke-detail-dark-375.png` | P6 修复后：dark 375px popup（无横溢、动作不挤标题） | 375×812 |

## ④ 机械断言（DOM 文本检查）

- **总览页不含「AI 建议」**：lanes/focus 两种状态下 `document.body.innerText.includes('AI 建议')` 均 **false**；`确认/不采纳` 类挑选按钮在总览可见按钮中 **0 命中** → 未确认建议不泄漏总览 → **pass**
- **总览页「待确认」字面**：命中 1 处「**待确认 / 未分类**」——为 BoardThreadBrowser **既有**泳道行标签（无 active 话题归属 section 的归集桶，`laneRows` 的 `unassigned` 行，BoardThreadBrowser.vue L248），属话题归属概念、与关键事件建议无关；建议/注记相关元素（★ 徽标、ke-actions、理由、虚线候选）均不含该字样 → 断言**意图**（建议状态不泄漏）pass，字面命中已如实记录（⑤-P3）
- **日报页建议行含理由文本**：**无法执行**——本期 `status=idle` 无任何建议行；替代观测：0 个「AI 建议」徽标、全部有效线索渲染「标为关键」手动入口、无归属线索禁标说明逻辑存在（KeyEventMarkerActions `actionable` 分支）
- **★ 计数语义**：`★n` = 该来源 section 下独立 confirmed 数（13709 确认后 ★1；两 marker 均 dismissed 后 ★0）；dismissed/suggested 不产生 ★ → DOM 与 API 双向一致
- **写请求留痕**：`PUT /daily-reports/topics/89/key-events/threads/{13710,13709}`、`PATCH /daily-reports/topics/89/key-events/{1,2}` 全部 200（A11 的 1 次 PATCH 被拦截为无 status）；无其他非预期写请求（无 report 重建、无信号发现/研究调用）

## ⑤ 问题清单

- **P1（中，功能缺陷，建议修复后复验 A6）**：`BoardDailyReportTimeline.vue` L441-446 内嵌的 `<BoardThreadBrowser>` 只绑定 `@open-article`/`@open-detective-wall`，**漏绑 `@open-daily-report`**。用户从日报 reader 内「话题总览」进入的总览（与顶部「话题总览」页签是不同实例）里，★ 详情的「来源日报」按钮 emit 无人监听 → 点击零反馈（无导航、无网络请求、无错误提示）。顶部「话题总览」页签实例绑定完整、行为正确。修复建议：内嵌实例补一行 `@open-daily-report` 转发（时间线自身已有 reader 深链打开能力）。
  - **复验：2026-10-13 修复后复验 pass（证据：截图 `ke-recheck-a6-1440.png` + 导航行为）**。新 session `ke-recheck` 重走 P1 路径：手标 13709（PUT 200）→ reader 内嵌「话题总览」→ 泳道 89 focus → ★ 节点详情 → 点「来源日报 2026-10-12」→ reader 深链打开并正确定位（masthead「10 月 12 日 · 周一」、线索行「已标记」），`agent-browser console` 零 error/warn；随后取消该注记恢复终态 confirmed=0（PATCH 200，两条 marker 均 dismissed）
- **P2（信息，设计语义确认）**：取消/不采纳后日报行内显示「已不采纳」徽标 +「仍标为关键」，而非回到裸「标为关键」。与 spec「不采纳和取消 SHALL 保留原新闻，允许对仍有效的来源重新手动标记」一致（取消≈dismissed、非否定）；brief 的「回到未标态（可用）」以满足，但验收者若预期字面「标为关键」需知此语义。总览侧不受影响（dismissed 不产生 ★）。
- **P3（信息，机械断言字面噪声）**：总览页存在既有字样「待确认 / 未分类」（无归属话题泳道标签）。若后续验收以字符串包含做「总览无待确认」断言会误报，建议断言改为检查 ke 相关元素/aria 或排除该既有标签。
- **P4（信息，数据环境）**：全部 7 个板块最新日报关键事件推荐 `status=idle`，无 AI 建议行，A1 确认路径本次无法在真实数据上执行（未触发推荐生成任务，避免与「生成类按钮」红线冲突）。建议在具备建议数据的环境补跑 A1（含确认→总览 ★1 的建议链路）。
- **P5（低，可用性观察）**：reader 线索行动作按钮可见尺寸 100×25px；Layout Contract 的「窄屏 44×44 命中区」条款针对时间轴 canvas 节点（可见小点外有真实按钮），日报 sm 按钮不在该条款范围，375px 实测无遮挡可点；仅作记录。
- **P6a（中，暗色可读性缺陷，2026-10-13 已修复+复验 pass）**：dark 主题下话题总览（TagsPage → 话题总览 → BoardThreadBrowser）点开节点详情「黑底黑字」。**根因（computed style 实测）**：popup 经 `<Teleport to="body">` 挂载，脱离页面容器继承链，而应用未在 `body` 上设主题文字色 → `.btb-popup` 计算色 `rgb(0,0,0)`（UA 默认）配深底 `rgba(17,27,38,0.98)`，popup 内所有未显式设色元素（`.btb-thread` 行、`.btb-ke-detail` 容器/标题、star 图标）全部继承黑字；另有 `.btb-popup-status` 硬编码 `color:white`，dark 下状态填充色已专门提亮（main.css dark 块 `--color-thread-status-*`），白字配提亮填充（如 ending `#9ca3af`、split `#fb923c`）不可读；`.btb-ke-detail` 系列引用了不存在的 `--color-border` token，一直落在亮米色回退值。**修复（BoardThreadBrowser.vue 组件内 token 化，未动 main.css）**：`.btb-popup`/`.btb-ke-detail`/`.btb-focus-inline` 容器显式接 `color: var(--color-text-primary)`（修 Teleport 继承链）；`.btb-popup-status`/`.btb-ke-toggle[pressed]`/`.btb-focus-node__ke` 的文字色改 `var(--color-text-inverted)`（editorial 填充深→近白字、dark 填充亮→深字，双主题自动反转）；`--color-border` 系引用改 `--color-border-medium/strong`、hint 的未定义 `--color-text-faint` 改 `--color-text-muted`。**复验**：dark popup 实测 `color=rgba(255,255,255,0.9)`、状态胶囊深字 `rgb(26,26,26)` 配亮填充；editorial 对照无回归（`rgb(26,26,26)` 配暖白底）。证据：`ke-detail-dark-1440.png`、`ke-detail-editorial-1440.png`。
- **P6b（中，详情形态与原型不一致，2026-10-13 已对齐+复验 pass）**：用户反馈「原型中直接对点开的线索做标记处理挺不错」。原型（ui-prototype `renderThread`/`.event*`/`.thread-actions`）点开节点显示**完整线索条目**：serif 加粗可换行标题 + muted 小字摘要 + 右侧动作区，而实现原为「一行 nowrap 截断标题挤两个按钮」。**修复**：`BoardThreadBrowser.vue` 三处注记详情行（focus 快照详情、focus inline、lanes popup）统一升级为 `__row = ★图标 + __main（__title serif 700 换行 + __summary muted 0.75rem 换行，marker 有 `summary_snapshot` 才显示）+ __actions（来源日报/取消 靠右不挤标题）`；快照详情原先独立的摘要段落并入行内避免重复。**复验**：dark/editorial/375 下标题均完整换行、摘要可读、动作靠右；证据 `ke-detail-dark-1440.png`、`ke-focus-dark-1440.png`、`ke-detail-dark-375.png`。修复涉及单测 24/24 过（断言按 testid/button 文本，无需改）；typecheck 0 错、lint 0 error。

## ⑥ 结论

- ui-design.md 主故事（在日报手标→总览 confirmed ★n→突出模式保背景→原位详情→刷新→取消→背景仍在）在真实产品上**全链路走通**；键盘、失败回滚、双主题、375px 路径全部 pass。
- A1 因本期无 AI 建议数据未覆盖（环境事实，非产品缺陷）；A6 首轮在「日报页签内嵌总览」路径发现 P1 死按钮缺陷，**同日修复后复验 pass**（复验记录见②A6 行与⑤P1 条目），顶部「话题总览」页签路径两轮均正常。
- 未勾 tasks.md、未 commit、未改任何源代码/配置；验收写动作收尾自洽（终态 confirmed=0，主题已还原 editorial，浏览器已 `agent-browser close`）。
- **补充（2026-10-13 P6 修复轮，session `ke-dark-fix`）**：本轮按用户真实产品反馈修复⑤-P6a/P6b（仅改 `front/app/features/tags/components/BoardThreadBrowser.vue`），重新部署后双主题复验 pass；本轮手标 thread 13245 复验后已取消，终态仍 confirmed=0；截图见③新增 4 行。
