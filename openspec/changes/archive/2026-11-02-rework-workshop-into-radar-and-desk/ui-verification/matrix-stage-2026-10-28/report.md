# 矩阵静态新版 S9 独立真实读取验收（K3/UI）

- 验收人：独立 K3/UI 验证线程（非实现作者；commandcode/deepseek/deepseek-v4.1-flash:max，implementer 档，未派子线程）
- 时间：2026-10-28 03:13–04:05（Asia/Shanghai）
- 唯一 origin：`http://localhost:5100`（用户侧 `http://192.168.5.24:5100`）；**未**访问 host:80 旧 nginx
- 浏览器会话：`matrix-s9-6d5795ec8cb0`（agent-browser 0.37.1；结束 finally `close` 本会话，未 `close --all`）
- 结论：**核心读取链路可用、视觉与批准基准同构；但发现 2 个 Medium 缺陷（档切换范围不重置、深滚动返回不恢复），S9 不能判全绿，建议修复后复验；数据安全/白屏层面无需回滚。**

## 1. 现场与部署核对（真实命令/事实）

| 项 | 实测 |
| --- | --- |
| 入口 SHA256 | `a8b20eccf1e5921a7159a6987fd49520ade37df58666016a9ec9c70fb1bb088b`（与父给定一致，7457B） |
| chunk `/_nuxt/tzVlOmOt.js` | HTTP 200，348987B，SHA `b5ff145f2106d4354cd9028384b29a41f2e131c03e84b7525fe356fd5031b710`（仅记录存在/可取） |
| 后端 | PID 1061075 存活（验收时 ELAPSED 03:19:25），**未重启**；未 build/typecheck/test/dev |
| 数据锚 | overview `data.date=2026-10-27`；今日（10-28）尚无日报（`/daily-reports/today` 空）→ 矩阵轴终点=锚点，`今日 — 条` |
| board 选择 | 真实 board 1974（15 天档泳道/休眠 1）与 1980（6 活跃）；0-active 有报告 50413、跨日报告 246030 只读核对 |
| 只读 API | overview / lane-dynamics(7/14/30/365) / daily-reports / reports detail / articles / health-checkup / topic-watches / source-breakdown(7/30/90) 全部设置 `--noproxy '*'`、落盘 `api/*.json` |

未修改：产品源码、后端、原型、旧报告、其它 change、tasks/review；未勾 8B.3/8B.4；未 commit/archive。

## 2. 安全边界（先建闸后点击）

- **浏览器级按 method 阻断**（非页面晚包装 fetch）：自写 CDP 客户端 `tools/cdp-method-guard.mjs`，Node 内置 WebSocket 连 `agent-browser get cdp-url` 的 browser ws，`Target.setAutoAttach(flatten)` + 对每个 page/iframe session `Fetch.enable(urlPattern:'*', requestStage:'Request')`；**仅 GET/HEAD/OPTIONS 走 continueRequest**，其余 `Fetch.failRequest(BlockedByClient)` 并落 JSONL（`write-guard.jsonl`）。
- 生效证明（在打开文章之前建立）：
  - `POST /api/__guard_selftest` → 页面 `TypeError: Failed to fetch`，guard 记 `blocked POST`（19:15:36Z）。
  - `GET /api/semantic-boards/overview` → 200（正文读取不受影响）。
  - 打开相关文章时真实链路**发出** `POST /api/reading-behavior/track-batch` → 被 CDP 阻断并记录（19:18:50Z）。**报告不声称该写入成功**，只说“既有采集链路有发出意图、未落库”。
- 逐条核对 `write-guard.jsonl`：`ready 1 / attach 5 / blocked 2`；除自检与 reading-behavior 外无任何非 GET 请求被放行。
- 未点生成/收编/激活/删除/保存；仅开管理入口/对话框后取消（匹配规则、标签池、追踪管理），guard 计数未增加。

## 3. 方法与证据

- 真实点击/指针/键盘：`agent-browser click / find role / mouse move|down|up / press / fill / scrollintoview / screenshot`；滚动与焦点用 `eval` **只读测量**（未用 eval 触发业务动作、未调 Vue `$emit`/私有方法）。
- 故障模拟只作用于当前独立浏览器指定 GET：`network route '**/lane-dynamics?days=365*' --abort`、`'**/api/articles/148022*' --abort`，验后 `unroute` 恢复真实读取。**mock/模拟均单独标注**，未当真数据。
- 大输出未进上下文：API 原文落 `api/*.json`，截图落 `png/*.png`；机械量落 `metrics-*.json` / `findings-*.json` / `requests-summary.json`。

## 4. S9 主链 25 步逐项

> 判定：PASS=真实数据+实页断言通过；FAIL=与用例期望冲突且有证据；PARTIAL=主路径过但存在缺陷或未覆盖子项；未验=真库无该数据或本轮禁触发。

| 步 | 结果 | 实页证据（真实 ID/数值） |
| --- | --- | --- |
| 1 矩阵默认视图/格值=桶峰值/无行尾/断档可读 | PASS | 1974 A14：5 活跃+休眠 1；1980 lane90 `2026-10-27` 格 aria「桶峰值 **3** · **3** 个主题桶 · 当日合计 **7**」（API `[2,2,3]`）；1974 lane1844 `10-16` =「峰值 1 · 2 桶 · 合计 2」（API `[1,1]`）；`matrix-grid` 0 个行合计节点；gap 格 disabled、文本 `—` |
| 2 三指标 title/aria + 无分区 | PASS | 同上 aria 与 `title` 逐字；gap `title="这一天这条泳道没有分区"`、aria「…：无分区」 |
| 3 点有分区格：不开抽屉/selectedDate/编年更新/不强抢 | PASS | 点 1844-10-16 → `?lane=1844`，编年头 `2026-10-16`，`.app-dialog-overlay=0`，stage.scrollTop 0→0，焦点仍在被点 cell |
| 4 点无分区格 | PASS | 点 lane5-10-21（is-gap）→ 编年仍 `2026-10-27`、无浮层/toast；提示经 `title/aria` 提供 |
| 5 编年全量标题（>5 可读）/默认收束 | PASS | report 1033（10-07）section 6295 真实 **7** 线程：编年列出 **7** 条标题（与 API 逐字一致）+「已返回 1 桶 · 7 条标题」；全部默认收束 |
| 6 单条真实摘要/互斥展开 | PASS | thread 14173 summary（142 字）逐字；点 14174 后 14173 `aria-expanded=false`，仅一条展开 |
| 7 相关文章按需/缓存/站内正文 | PASS | 14173 展开→GET 148014/148022/148030；点位 148359 → `ArticlePreviewModal` 打开真实 `content`（130 字，DOM 与 API 逐字相同）；关闭恢复焦点到 `lane-chronicle-article-148359`、stage.scrollTop 549 不变 |
| 8 前后有内容日/边界/滚动保持 | PASS | 1844：10-16→next→10-17→10-18；真实指针点击前后 `scrollTop 200→200`；10-26 时 next disabled 且 title「已到最新有内容日」，10-14 时 prev disabled「已到最早有内容日」 |
| 9 演进带点径/合计/无最密三日/态势折叠 | PASS | 节点数=当日合计（1844 10-16=2，矩阵格=1）；dot `14.6603px`=6+√3×5、`19.2288px`=6+√7×5；无 densest 区块；态势 `<details>` 默认收起「最近结算态势 · 2026-10-27（非本视窗总结）」，展开文本与 API snapshot.summary **完全相等**（200 字，末尾同为“利雅”） |
| 10 >31 日折叠 | PASS | 365 日窗：逐日节点 0 个，`展开逐日索引（365 日）` 默认收起（展开 29 个有内容日）；趋势图在，Y 轴 28/14/0、X 2025-10-28 / 2026-04-28 / 2026-10-27 |
| 11 刷选新区间：sourceDays/名单/行序/休眠不变；B 仅补同 ID | PASS | 拖选 2026-04-13..2026-10-12：URL 恰一次 `start/end`，tier 仍 14，行序 `[1204,1844,2,9,5]` 不变，脚注「历史由 365 日池补充（不改名单）」，只发 1 次 `days=365`，无新增 `days=14` |
| 12 恰 7/14/30/365 日不切档 | PASS | 日期输入恰 7 日→35 格、30 日→150 格、365 日→1825 格、回自身 14 日→70 格；sourceDays 始终 14，行序恒为 `[1204,1844,2,9,5]`，无档请求 |
| 13 输入 10-06~10-26 联动 + URL | PASS | 21 日轴/105 格；URL `start=2026-10-06&end=2026-10-26`；矩阵/演进/编年同轴（编年落 10-26） |
| 14 手柄键盘 | PASS | start 手柄 `←` 10-06→10-05（aria-valuenow 343→342）、`Shift+→` +7 日→10-12、`End`→10-26；end 手柄 `→`→10-27、`Home`→10-26；焦点始终在手柄 |
| 15 明确切独立档 | **FAIL** | 基本切档可用：7 天→1 次 `days=7`、行序按 A7 重排 `[1204,2,1844,9,5]`、选中泳道保留；365 与 B 共用 **0** 新增请求。**但目标档已缓存时范围不重置**：30 档 + 自定义 10-15..10-16 → 点「近 1 年」→ URL `days=365`（start/end 被删）、日期输入仍 10-15..10-16、矩阵仅 2 列 12 格，而脚注/名单已按 365（见 F2、截图 `s9-1440-tier365-range-not-reset.png`） |
| 16 右轨顺序/结论/来源独立窗 | PASS | DOM y 顺序 体检71→候选236→追踪417→来源557；体检分数 **82**=`llm_score`（非 attention39），评语首句两行 38.9px 截断+「完整评语与依据」；候选 top3「美国 34 tags·30 篇 / 特朗普 25·24 / 胡塞武装 17·19」，计数 50；watch「美伊形式对市场影响 关注中」（API status=active, persistent_topic_id 1205）；来源 7日=150篇/6源、30日=158/7、90日=158/7，「独立统计，不随时间视窗」 |
| 17 追踪管理仅板块作用域 | PASS | 右轨「管理 ↗」→「我在追踪 / 本版块 · 1 个关注（active + paused）」，列表仅该板块 1 条关注；无 lane/topic 选择器；Esc 关闭后焦点回 `rail-watch-manage` |
| 18 全局入口只在总览 | PASS | 版块页 header/rail 反向断言 0 个「回填/标签池/合并/新建板块/更多」按钮；`/tags` 报头「新建版块」+ `全局治理` 菜单 5 项（扩充版块/匹配回填（全库）/匹配规则/标签合并/标签池）；开「匹配规则」740px 对话框→取消、开「标签池」（辅助 188534）→Esc，均无写请求 |
| 19 最新日报唯一主入口 + 历史动作 | PASS | header「打开日报 · 10/27」→ `/tags/1974/2026-10-27?lane=1844`（既有阅读台）；selectedDate=10-26 时编年另有「打开 10-26 日报」 |
| 20 切板块清场 | PASS | 1974→1980：URL `/tags/1980`，tier 回 14、范围 10-14..10-27、选中 lane90、编年 10-27、无 1844 残留；health 82→78 |
| 21 深链四参刷新 | PASS | `?lane=1844&days=30&start=2026-09-28&end=2026-10-27` 四参全恢复（仅 days=30 请求）；`days=abc`/`days=8`→回退 14+提示（只发 days=14）；`start>end`→「起始日期不能晚于截止日期」；池外→「日期超出可取数范围（2025-10-28 ~ 2026-10-27）」 |
| 22 双视口/窄屏 | PARTIAL | 1440/1920/2560 均 `scrollHeight==clientHeight`（无页面纵向滚动），1440/1920 矩阵容器不横滚（sw=cw），375/390 `scrollWidth==clientWidth` 且矩阵/右轨不渲染；**MX-6 字面偏离**：1440 下 30 列（≤31）已出现容器内横滚（`matrix-scroll sw=1315 > cw=845`，格宽缩到 33.6px）；页面仍无横滚（见 F3） |
| 23 A/B 分层失败与重试 | PASS | 阻断 365：1974 A14 仍可读（行序不变），横幅「365 日历史池未取到，历史日显示 ?（不改名单/范围）（Failed to fetch）」+「重试历史池」，80 个 `?` 格；解除后重点→恰 1 次 `days=365` 200、`?` 归 0；375 同一横幅/按钮可达（elementFromPoint 命中，页面无横溢），重试恰 1 次 200 |
| 24 读取边界/缓存 | PARTIAL | 真实计数：报告列表每板块 1 份；跨日连点 10-15→10-16→10-15→10-16 仅新增 1 次详情（`daily-reports/1126`）；同文章跨 thread 复用；**14/20 容量与 FIFO 逐出未在真实浏览器验**（仅单测） |
| 25 研究工作台真实承接与返回 | **FAIL（滚动）** | `/tags/1974/research` 真实研究页（发现粒度/月度周期/发现信号/候选/深入分析）；返回后 URL 四参/泳道/日期/范围均恢复，但**深滚动位置不恢复**：1440 下 stage 382→302、1280 下 window 693→414（150 中段可精确恢复），详见 F1 |

## 5. 五组变体走查

| # | 变体 | 结果 | 证据 |
| --- | --- | --- | --- |
| 1 | 日期空串/空白/缺零/斜杠/start>end/池外 | PASS | 空串与纯空白→「请填写起止日期」；`2026-10-6`/`2026/10/06`→格式提示；原视窗/URL 不变，输入不被静默补零 |
| 2 | 单日 / 2 日 / 全池 365 日 | PASS | 单日→5 格、2 日→10 格、365 日→1825 格；单日也渲染 |
| 3 | days=abc/8 | PASS（0/-1 未单测，同归一分支） | 均回退 14+提示，只发 `days=14` |
| 4 | A 行在观察轴内无分区 / B 独有泳道 | PASS | 窗口横扫 7/14/30/365 日，行序恒为 A14 五条（+休眠 1205）；B365 独有 lane **471** 从未进入名单/候选；A 行保留、区间外全 `—` |
| 5 | thread_count 缺省 | 未验 | 真库 12 板块 A14 全带 `thread_count`，无缺省样本 |
| 6 | 当日报告被删 / section 不在详情 | 未验 | 真实库无该数据；未造假 |
| 7 | summary 空 / related 空 / 文章失败 | PARTIAL | 空样本无（近期报告 0 条空 summary/空 related）；**单篇失败 PASS**：阻断 148022 → 该项「文章 #148022 加载失败（Failed to fetch）+ 重试」，另 2 篇正常，重试恰 1 次 200 |
| 8 | 无日报版块空态 | 未验（另测 0-active 有报告） | 真库 12 板块都有日报；50413（0 活跃、有报告）显示「这个版块还没有活跃泳道，但已有日报内容…最新日报唯一入口在标题区「打开日报 · 10/27」」，矩阵 0 格、编年不渲染、演进空提示 |
| 9 | 今天无日报（锚点早于今天） | PASS | 轴终点=2026-10-27；矩阵头「数据截止 2026-10-27（之后没有新日报）」，演进「今日 — 条」；无 10-28 列/highlight |
| 10 | 跨月/跨年观察范围 | PASS | 365 日轴 1825 格无重复；日期序列 2025-10-28…2026-10-27 连续 |
| 11 | 幂等（重复点同格/同标题/同范围） | PASS | 连点同格 4 次仅 1 次新详情请求；展开态幂等；同范围重复应用无额外请求 |
| 12 | 切板块旧响应竞态 | 未验 | 需构造晚到响应；仅单测覆盖（OX-10/RD-4） |
| 13 | A 成功 B 失败 / A 首错 / A 刷新失败 | PARTIAL | B 失败两种视口均 PASS；A 首错/A 刷新失败仅单测（未模拟） |
| 14 | 编年读取失败 | 未验 | 未模拟编年列表/详情失败（矩阵与右轨失败未在本轮构造） |
| 15 | 超长泳道名/标题/评语 | PARTIAL | 泳道名 ellipsis 且 `title` 全文（"Agent 架构重构与标准化协作生态"）；评语两行封顶 + 「完整评语与依据」；超长 thread 标题/正文未专门构造样本 |
| 16 | 范围应用连点 | PASS | 连续应用同范围/不同范围均只产生一次状态更新，无重复写/重复请求 |

## 6. 机械断言（JSON 已落盘）

| 断言 | 实测 |
| --- | --- |
| root 字号（勿人为设 16 骗断言） | 1440/1920/2560/375/390 全部 `16px`（`document.documentElement` computed） |
| 视口/溢出 | 1440: sw=cw=1440, sh=ch=900；1920: 1920×1080；2560: 2560×1440；375: 375×1914，sw=cw=375；390: 390×1914，sw=cw=390；1280: 1280×1522，sw=cw=1280 → 均无横向溢出 |
| 左右轨 | 1440: 272 / 255；1920: 272 / 303；2560: 272 / 303；1280: 240 / 255（layout 登记 180–300 / 230–400 内） |
| 填可用宽/无 120rem 上限 | 2560 matrix `x=289.6 w=1916.8`（右轨起 2241），未居中压缩 |
| header 新刷新钮换行/裁内容 | 1440/1920/2560 单行：header h=56.2，子元素 bottom=45.6；375/390 header 171.9 多行但无裁切/溢出 |
| 主滚动宿主 | 1440/1920/2560：`MAIN.console-stage`（1440 sh=1322 ch=844；1920 1397/1024；2560 1397/1384）；1280/375/390：默认文档滚动（无内部 scroll host，`hosts=[]`） |
| 手柄 hit 区 + elementFromPoint | 1440：handle rect 8.01×81.4，`elementFromPoint` 命中 `brush-handle-start/end`；375：3.15×81.4，仍命中（真触摸未验，见 F4）；selection 1440 28.6px/375 11.26px |
| 拖选 preview 不写 commit/Esc | 拖动中 URL 不变、feedback「松开应用」；Esc 恢复原选区且松手不提交；提交恰一次且 URL 含 start/end |
| 365 双端/反向/平移/缩放/键盘 | 反向 02-27→07-13 规范化；平移 +46 日（宽度保持）；放大 137→69 日、缩小→138 日；`←/Shift+→/Home/End` 均生效（详见步 14） |
| 宽工作面可达 | 右轨「完整评语与依据 / 查看候选与依据 / 来源明细与口径」入口在；匹配规则对话框 740px、标签池 188534、追踪面板 520px；elementFromPoint 命中按钮本体；Esc/取消关后焦点回触发点（追踪管理实测） |
| 主题切换真实 | `data-theme=dark` → body bg `rgb(8,12,18)`；浅色 `rgb(250,247,242)`；非改 dataset 伪主题（按钮点击触发） |
| 返回 URL/滚动 | 报告/研究往返 URL 四参恢复；滚动见 F1（150→150 OK；382→302；693→414） |

## 7. 截图清单与本轮看图结论

全部为标准 PNG（无拼图/无旧图复用），位于 `png/`：

| 文件 | 真实状态 | 我看图后的判断 |
| --- | --- | --- |
| `s9-1440-editorial-default.png` | 1974 A14 默认（浅色） | 与批准基准同构：左板块栏/中视窗+刷选+矩阵/右体检→候选→追踪→来源；头部含新增「刷新」不换行；矩阵 5 行 14 列、gap 为 `—`、末列选中描边；无行尾合计列 |
| `s9-1440-dark-default.png` | 1980 默认（深色） | 深色 token 生效（bg #080c12），热力格对比可读；右轨评语两行封顶带省略号；无溢出/无裁切 |
| `s9-1920-editorial-default.png` | 1980 1920 浅色 | 三栏比例随视口展开，矩阵填满可用宽，头部仍单行；演进节点与首线索标签可见 |
| `s9-1920-dark-default.png` | 同上深色 | 深色下层级（背景/卡片/热力）分明，选中日描边可见 |
| `s9-2560-editorial-default.png` | 1980 2560 浅色 | 填满 2560（未 120rem 居中），左右轨固定宽、主舞台拉伸；信息密度可接受 |
| `s9-2560-dark-default.png` | 同上深色 | 同构；无明显暗色对比失效 |
| `s9-375-editorial-default.png` | 375 浅色 | 头部 4 行折行但完整（矩阵/归档/研究工作台、日报、刷新、板块管理、新建泳道、主题）；时间视窗堆叠；无横溢 |
| `s9-375-dark-default.png` | 375 深色 | 同构；刷选选区在右侧窄带，可见小竖线 |
| `s9-390-editorial-default.png` / `s9-390-dark-default.png` | 390 双主题 | 同 375；窄屏摘要「泳道摘要 · 窄屏」+ 迷你分区条 + 「下方编年 / 当天线索 ↓」 |
| `s9-1440-article-modal.png` | 相关文章点开 | 真实正文（来源/日期/作者/标题/正文），不是导语中间层；阅读列居中留白，弹层为既有 ArticlePreviewModal（90vw） |
| `s9-1440-b-pool-failure.png` | 365 被阻断 | 横幅+「重试历史池」清晰；A 窗口数据仍可读，历史为 `?` 未补 0；矩阵出现容器内横滚 |
| `s9-375-b-pool-failure.png` | 375 365 被阻断 | 横幅与重试按钮在 375 可达、不裁切；窄屏摘要用虚线段表达未知 |
| `s9-1440-tier365-range-not-reset.png` | 365 档 + 旧 2 日范围（F2 现场） | 「近 1 年」高亮但矩阵仅 10-15/10-16 两列、日期输入仍 2 日、脚注已写「按 365 日档」——档/轴/名单自相矛盾，直观可见 |
| `s9-1440-rail-7d.png` | 右轨 7 日来源窗 | 150 篇/6 源、前三源 63/40/25 篇与 42%/27%/17%，“另有 3 个来源”；与 source-breakdown 一致 |

批准基准差异（与 `revision-2026-10-27-matrix-scope-cleanup/screens/scope-axes-default-1440.png`、`lane-axes-mobile-375.png` 对照）：区块顺序、双轴图、档按钮、矩阵语义、右轨四段、窄屏摘要+编年结构一致；差异为用户已批准的导航改字（关系/全景→研究工作台）、评审增量「刷新/池失败重试」、以及真实数据日期/数值不同（允许）。

## 8. Findings

### F1（Medium）返回上下文深滚动不恢复
- 触发：1440 滚动到编年中段/底部 → 打开日报（或研究工作台）→ 返回版块矩阵。
- 实测：1440 stage `382→302`（max 仍 382，等 4s 不纠正）；1280 window `693→414`（文档高 1522）；中段 `150→150` 精确。
- 定位（只读推断）：`front/app/features/tags/components/console/useConsoleReturnContext.ts` 的 `await nextTick(); applyScrollTop(context.scrollTop)` 只应用一次；编年详情异步加载前 `scrollHeight` 较小，深值被钳制，内容增高后无二次应用。
- 影响：步 25“返回恢复滚动”不成立；阅读位置丢失（不涉数据）。

### F2（Medium）切到已缓存档时观察范围不重置
- 触发：30 档 + 自定义 10-15..10-16 → 点「近 1 年」（该档已在缓存）。
- 实测：URL `?lane=1844&days=365`（start/end 被删），日期输入仍 10-15..10-16、矩阵 12 格/2 列，脚注/名单已按 365 重算（`可见 2 日 · 365 日档全窗 73 条`）。
- 定位（只读推断）：`useObservationWindow.onTierSwitch` 置空范围后，`handleSourceLoaded()` 立即 `restoreRangeFromQuery()`，读取的是 `router.replace(writeTierQuery)` 尚未提交的旧 `start/end`；命中缓存时该竞态必现（网络慢时旧代码路径反而“看起来正常”）。
- 影响：档/轴/名单不一致，违反步 15「范围重置为该档默认区间」，深链与可见状态分叉。

### F3（Low，需裁决）MX-6“≤31 列单屏”未达
- 1440 下 30 列窗口：`matrix-scroll sw=1315 / cw=845`（容器内横滚），格宽 33.6px；页面无横滚。用例字面要求 ≤31 为单屏网格。属产品取舍/契约措辞问题，非数据错误。

### F4（Low，静态风险）窄屏手柄 hit 区仅 3.15px
- 375：`brush-handle-*` rect `3.15×81.4`，`elementFromPoint` 命中（hitbox 真实存在），但无 24/44px 级触摸目标；键盘与日期输入为替代路径。真机触摸未验。

### F5（Low，既有组件）文章弹层宽度/可访问性
- 复用既有 `ArticlePreviewModal`：1296px（90vw，非 AppDialog 四档）、无 `role=dialog`、打开时焦点未移入弹层（关闭仍正确回触发点）。属存量组件，不判本期造假。

### F6（Low，视觉）编年标题被 CSS uppercase
- 窄屏/编年标题「AGENT 架构重构与标准化协作生态」与泳道行「Agent …」大小写不一致（`text-transform: uppercase` 作用于中英混排标题）。不影响数据。

## 9. 未验 / 模拟声明

- **未验**：thread_count 缺省；日报/section 删除或缺失；summary 空/related 空；无日报板块空态；切板块晚到响应；编年读取失败；A 首错/A 刷新失败；报告详情 14 份/文章 20 份 FIFO 逐出与“当前阅读项稳定”；snapshot=null 待结算占位（真库 12 板块全有快照）；0 线索有分区格（真库无此类样本）；真实生成 DB 提交与 WS 先后（本轮禁触发，按父要求不计入 S9 绿）；真机触摸手势（仅 headless 指针）。
- **模拟**：`days=365` 与 `article 148022` 的 GET 阻断（仅当前浏览器、验后解除）；自检 POST 与 reading-behavior POST 的阻断为安全闸证明，不是业务写成功。
- 上轮债务未动：根 53 pass / 1 fail / 2 未覆盖、旧投诉、7 条 SignalReport 类型债；未改 tasks/review，未勾 8B.3/8B.4。

## 10. 结论与最少修复建议

- 真实 S9：**23 步 PASS / 2 步 FAIL（15、25）/ 2 步 PARTIAL（22、24）**；核心读取、矩阵/演进/编年/右轨/入口/深链/错误恢复均真实可用，视觉与批准基准同构，不需要因数据/白屏回滚。
- 但 **F1、F2 属 Medium 实质缺陷**（阅读位置丢失、档轴不一致），建议父裁决修复后复验：
  1. F1：把滚动恢复改成“内容稳定后再应用”（监听编年就绪/scrollHeight 变化，或在 `useLaneDayReading` ready 后重试一次，`preventScroll` 保焦点）。
  2. F2：切档时同步写入目标档默认范围（不依赖 router 异步回读），或在 `handleSourceLoaded` 内跳过“自身切档刚删除的旧 start/end”（加一次性抑制标志），并补“缓存命中切档”回归用例。
  3. F3：裁决 MX-6 字面（≤31 单屏）与最小可读格宽；若保留容器横滚，更新用例/合同措辞。
  4. F4：给 SVG 手柄加不可见加宽 hit rect（≥20px）以覆盖触摸。
  5. F5/F6 记入存量/视觉小项，可后议，不阻本阶段。
