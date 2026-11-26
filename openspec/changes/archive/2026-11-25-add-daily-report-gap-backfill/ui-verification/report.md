# UI 验收报告 — add-daily-report-gap-backfill

> **本轮 = 上一轮三处残留的最终复验（2026-11-25 10:05–10:13）**。上一轮（09:35–09:44）留下 3 条残留：缺料提示 10.24px + muted（≈3.0:1）、禁用的行级 ghost「补这天」opacity .4、缺料文案不分原因。本轮**只验这 3 条的修复 + 视觉收口**，A-1…A-11 原样保留（判定与依据一字未改），A-12…A-15 为本轮新增条目。新增截图 radar-gap-final-editorial-1440x900.png / radar-gap-final-dark-1440x900.png（积压态展开态）、radar-gap-final-editorial-unknown-1440x900.png（Embedding 计数不可得态）、radar-gap-final-editorial-idle-1440x900.png / radar-gap-final-dark-idle-1440x900.png（队列已清空可用态）。

验收环境：本地静态托管（后端同端口 :5100，同源 /api），agent-browser 无头浏览器（session `uiverify`，视口 1440×900），页面 http://127.0.0.1:5100/tags（系统代理下 localhost 首次导航被代理吞掉，改用同源同构的 127.0.0.1 直连）。构建产物 backend-go/frontend 已确认含本轮三处修复：`index.BPH6rXPU.css` 内 `.rs-gap-btn[data-v-361faf1f]:disabled{opacity:.55}`、`.rs-gap-blocked[data-v-361faf1f]{color:var(--color-text-secondary);font-size:.7rem}`；产物 JS 内同时含「队列还没清空」与「队列状态未知」两套文案。
被验对象：L0 雷达总览页（/tags）右栏底部管线状态块「近 7 天缺口」折叠区。
数据基线：`GET /api/daily-reports/gaps` → days=7、missing_days=1、total_missing=12、缺口日仅 2026-11-23（12 个板块），本轮全程未变。
队列实况：复验窗口内服务端队列一直在跑（10:05 时 `tag_queue` 待打标 30，10:12 时 pending 1 / processing 0，`embedding_queue` 全程 0/0），真实态时点不稳；故 A-12/A-13/A-14 三条一律用浏览器网络层拦截 `**/api/poll` 的**确定性报文**复验——积压态报文 `tag_queue{pending 3, processing 1}` + `embedding_queue{pending 0, processing 0}`；未知态报文在同一基础上**删除 `embedding_queue` 字段**。
对比度口径：对目标元素读 `getComputedStyle` 的 `color` / `background-color`，沿祖先链把半透明底色（`color(srgb … / a)`、`rgba()`）逐层合成到不透明底色上得到「有效底色」，再把字色（含自身 alpha）合成上去得到「有效字色」，按 WCAG 2.x 相对亮度公式算比值；组不透明度（按钮 opacity .55）按「整组以该不透明度压到背景上」处理。
操作红线：全程未点击「补这天」「一次补齐」「立即生成今日日报」等会触发真实 LLM 管线的按钮；只做折叠展开、读文本/几何/计算样式、切主题、截图。唯一的写操作是浏览器网络层对 `**/api/poll` 的响应拦截（不落服务端、不触发管线）；收尾已 `network unroute`，并复查服务端 `/api/poll` 计数继续自然推进（completed 791→793）、页面回落到实时值「待打标 1」，服务端数据未被改动。复验结束已关闭全部 agent-browser / Chromium 进程、主题还原为 editorial 并清掉 `localStorage['syntopica-theme']`（与开工前一致）。
备注：10:07 一次 reload 后页面总览短暂报「failed to get board overview / 总览读取失败」（后端瞬时抖动），重载后恢复；本轮交付的两张 final 截图均取自渲染健康的页面。

### A-1 缺口区按真实数据渲染【pass】
复验：打开 http://localhost:5100/tags（1440×900），管线状态块展开态，点行头 `radar-gap-head` 展开后实测行头 `radar-gap-summary` 文本「近 7 天缺口 1 天 / 12 篇」；`radar-gap-list` 下**仅 1 行** `radar-gap-day-2026-11-23`，行文本「11-23 | 12 个板块缺 | 补这天」；底部 `radar-gap-backfill-all` 文本「一次补齐 12 篇」含 12，与接口 missing_days=1 / total_missing=12 一致。截图 radar-gap-editorial-1440x900.png、radar-gap-1440x900.png（已按样式修复后重拍）。

### A-2 补填按钮可用态【pass】
复验（真实态，双队列已排空）：`radar-gap-backfill-2026-11-23`（补这天）disabled=false、aria-disabled=null；`radar-gap-backfill-all`（一次补齐 12 篇）disabled=false、aria-disabled=null；两档视口（1440×900 与 1920×1080）复测均为 disabled=false。**本条口径补充**：补填入口的可用性自本轮起另受队列门槛约束（双队列未排空即禁用），故「可用」只在双队列排空时成立；积压分支见 A-9。截图 radar-gap-editorial-1440x900.png、radar-gap-1920x1080.png（已按样式修复后重拍）。

### A-3 1440×900 不裁切【pass】
复验：展开态实测 `radar-gap-body` getBoundingClientRect = left 1100.2 / top 788.1 / right 1386.2 / bottom 865.4，right 1386.2 ≤ innerWidth 1440（余量 53.8px）、bottom 865.4 ≤ innerHeight 900（余量 34.6px）；状态块 `radar-status-block` bottom 880.8 ≤ 900。按钮不换行：「补这天」「一次补齐 12 篇」文本 Range 客户端矩形数 = 1，scrollWidth == clientWidth（59/59、110/110，overflowX=0）；缺口行三个子元素（11-23 right 1132.8 / 12 个板块缺 right 1200 / 补这天 right 1386.2）均未溢出行容器（row right 1386.2，溢出量 ≤ 0）。截图 radar-gap-1440x900.png、radar-gap-editorial-1440x900.png（已按样式修复后重拍）。

### A-4 1920×1080 不裁切【pass】
复验：切到 1920×1080 展开态实测 `radar-gap-body` rect = left 1474.6 / top 968.1 / right 1866.2 / bottom 1045.4（余量：右 53.8px / 下 34.6px），right 1866.2 ≤ 1920、bottom 1045.4 ≤ 1080；状态块 bottom 1060.8 ≤ 1080。按钮同样单行（Range 矩形数 = 1，scrollWidth == clientWidth 59/59、110/110），缺口行子元素无溢出（row right 1866.2）。截图 radar-gap-1920x1080.png（已按样式修复后重拍）。

### A-5 既有五项事实未被破坏（回归）【pass】
复验（真实态）：`radar-status-block` 全文实测含「标签队列：队列已清空」「Embedding 队列：队列已清空」「生成窗口：生成 21:00 · 兜底 23:30」「最新日报：2026-11-24」「今日状态：今日日报未生成」五项事实；`data-testid="radar-status-generate"`=「立即生成今日日报」、`radar-status-today`=「今日日报未生成」、`radar-status-latest`=「2026-11-24」、`radar-status-queue`=「队列已清空」、`radar-status-schedule-time`=「生成 21:00」五个 testid 在展开态全部存在，缺口区新增未挤掉既有行。截图 radar-gap-1440x900.png、radar-gap-editorial-1440x900.png（已按样式修复后重拍）。

### A-6 折叠/展开可逆【pass】
复验：同一页面连续操作取三次状态快照——状态 1 展开（aria-expanded=true，body 存在，行「11-23 | 12 个板块缺 | 补这天」，两按钮 disabled=false）→ 点行头收起（aria-expanded=false，body 不存在）→ 再点行头展开（aria-expanded=true，body 存在，行文本与两按钮文本/disabled 与状态 1 逐字一致）。截图 radar-gap-1440x900-collapsed.png（收起态）、radar-gap-editorial-1440x900.png（展开态）。

### A-7 二次确认真实链路【未覆盖】
未覆盖原因：今日（2026-11-25）无日报，`radar-status-generate` 走「立即生成今日日报」首次生成路径，页面本就不出现确认弹窗；要看到二次确认必须先触发今日生成，而点击将真的跑一条完整 LLM 流水线（触及每分钟烧额度的红线），故不在真实环境验证。判定依据为组件测试 front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts 的 describe「BoardRadarStatusBlock — 重新生成二次确认」四条用例（今日已出：确认替换后弹二次确认（危险样式 + 写明破坏性后果）/ 今日已出：取消确认不发起生成请求 / 今日已出：确认后才发起重新生成 / 首次生成：不弹二次确认，直接发起），该文件本轮实跑 68 条用例全绿。

### A-8 缺口取数失败降级【pass】
判定通道为自动化测试（浏览器端不人为打断 /api/daily-reports/gaps，避免扰动运行环境）：front/app/features/tags/composables/useRadarPipelineStatus.test.ts 的 describe「useRadarPipelineStatus — 缺口取数（一次性请求 + 独立降级）」用例「取数失败：只降级缺口区，不产出「无缺口」结论，其余事实不受影响」断言 gapError 置位、gapLoaded 保持 false、gapsByDate 为空且 pollReady/queueIdle/overviewError 不受影响；front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts 用例「缺口取数失败：只降级缺口区，其余五项事实照常（读取失败 + 重试）」覆盖呈现层降级；本轮实跑两文件共 110 条用例全绿（useRadarPipelineStatus 42 + BoardRadarStatusBlock 68；上一轮为 39+66=105，新增 5 条为队列门槛相关用例）。

### A-9 队列门槛：积压 ⇒ 两入口禁用 + 缺料说明【pass】
复验（真实态队列已排空）：`GET /api/poll` 复查 tag_queue={pending 0, processing 0, failed 1}、embedding_queue={pending 0, processing 0}，按 D8 口径 failed 不计入门槛 → 真实态下两按钮 disabled=false、无缺料提示（**与主线程 brief 里「标签队列有积压 → 应为禁用」的预期不符：复验时刻积压已排空**，这是数据时点差异，不是实现缺陷）。为复验「积压 ⇒ 禁用」分支，用 agent-browser 在浏览器网络层拦截 `**/api/poll` 注入 `tag_queue.pending=3 / processing=1`（纯客户端模拟：不触发任何 LLM 管线、不写服务端数据；事后复查服务端 /api/poll 仍为 pending 0/processing 0/failed 1，未被改动）。模拟态实测：状态块显示「标签队列：待打标 4」；`radar-gap-backfill-2026-11-23`（补这天）与 `radar-gap-backfill-all`（一次补齐 12 篇）DOM 属性 disabled=true（outerHTML 含 `disabled=""`）、computed opacity 0.4；`[data-testid="radar-gap-queue-blocked"]` 存在且可见（offsetParent 非空，rect left 1100.2 / top 849.0 / right 1386.2 / bottom 865.4），文本「队列还没清空：补出来的候选会不全，清空后才能补。」。组件层佐证：front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts 用例「队列未排空：补填入口禁用 + 缺料说明（与生成同款双队列判据）」「队列已清空：补填入口可用且不出缺料说明」，front/app/features/tags/composables/useRadarPipelineStatus.test.ts 用例「队列积压时补填入口禁用（与生成同款双队列判据）」「双队列清空时补填入口可用」，两文件本轮实跑 110 例全绿。截图 radar-gap-queue-blocked-1440x900.png。

### A-10 双主题无嵌套异质色块【pass】
复验：editorial 下 `.rs-gap-body` 计算样式 background-color=`rgba(0, 0, 0, 0)`、border-width=`0px`（border-top-width 0px / border-bottom-width 0px）、padding=0px、box-shadow=none；外层卡片 `radar-status-block` background=`color(srgb 0.85098 0.290196 0.290196 / 0.06)`（accent 6% 半透明底）——缺口区不再自带任何面。dark 下 `.rs-gap-body` 计算样式 background-color=`rgba(0, 0, 0, 0)`、border-width=`0px`、padding=0px、box-shadow=none；外层卡片 background=`color(srgb 0.941176 0.541176 0.294118 / 0.0510588)`，与卡片同底、无硬接缝。行间分隔如实记录：本轮窗口仅 1 个缺口日（2026-11-23），该行命中 `.rs-gap-list > .rs-gap-row:first-child { border-top: 0 }`，实测 border-top-style=none、border-top-width=0px，故「行间虚线」在本轮真实数据下无第二行可验（CSS 规则 `.rs-gap-row { border-top: 1px dashed var(--color-border-subtle) }` 对第二行起生效）；实际可见的分隔线是「一次补齐」行上方的 `.rs-gap-actions` border-top=1px solid rgba(26,26,26,0.08)（editorial）/ rgba(255,255,255,0.06)（dark）。按钮底色/字色：editorial「补这天」ghost background-color=`rgba(0, 0, 0, 0)`（透明无边框）、color=`rgb(90, 90, 90)`（对卡片底 rgb(248,237,232) 对比度 ≈ 5.99:1，过 WCAG AA）；「一次补齐 12 篇」secondary background-color=`rgba(255, 255, 255, 0.85)`、color=`rgb(26, 26, 26)`（≈ 17.1:1）。dark「补这天」ghost background-color=`rgba(0, 0, 0, 0)`、color=`rgba(255, 255, 255, 0.6)`（≈ 7.16:1）；「一次补齐 12 篇」secondary background-color=`rgba(255, 255, 255, 0.06)`、color=`rgba(255, 255, 255, 0.9)`（≈ 13.2:1）。截图 radar-gap-editorial-1440x900.png、radar-gap-dark-1440x900.png。

### A-11 深浅主题切换不重置缺口状态【pass】
复验：/tags 页本身没有主题开关（该页 header 只有「主页 / 全局治理 / 新建版块」，无主题按钮；主题开关在 /settings 与信息流顶栏），故同页复验采用与 `useTheme.setTheme` 完全同款的 DOM 操作（`document.documentElement.setAttribute('data-theme', …)` + `localStorage['syntopica-theme']`）切主题。同页 editorial → dark → editorial 往返三态实测：行头文本恒为「近 7 天缺口 1 天 / 12 篇」、`aria-expanded` 恒为 true、`radar-gap-body` 恒存在、缺口行恒为「11-23 | 12 个板块缺 | 补这天」、两按钮文本恒为「补这天」/「一次补齐 12 篇」、两按钮 disabled 恒为 false（真实态队列已排空，缺料提示在三态中恒不出现）、行几何恒为 left 1100.2 / right 1386.2；只有计算样式随主题变（editorial 卡底 accent 6% 淡红 → dark 卡底 accent 5.1% 淡橙，`.rs-gap-body` 两主题恒为透明）。应用真实开关佐证：/settings 页真实点击 `button[title="切换为深色模式"]` → `document.documentElement[data-theme]` 由 editorial 变 dark、`localStorage['syntopica-theme']` 同步为 dark、按钮 title 变「切换为浅色模式」；再点一次恢复 editorial（复验结束后环境已还原为 editorial）。截图 radar-gap-editorial-1440x900.png、radar-gap-dark-1440x900.png。

### A-12 缺料提示可读性（11.2px + secondary）【pass】
复验（积压态，浏览器网络层拦截 `**/api/poll` 注入 `tag_queue{pending 3, processing 1}` + `embedding_queue{pending 0, processing 0}`，1440×900）：`[data-testid="radar-gap-queue-blocked"]` 计算样式 **font-size = 11.2px**（= 0.7rem，达 ≥11.2px 门槛；上一轮的 10.24px 已消除）、color = `rgb(90, 90, 90)`（editorial）/ `rgba(255, 255, 255, 0.6)`（dark）；父卡片 `.rs-block` 底色 = `color(srgb 0.85098 0.290196 0.290196 / 0.06)`（editorial，叠在 body 底 `rgb(250, 247, 242)` 上 → **有效底色 `rgb(248, 237, 232)`**）/ `color(srgb 0.941176 0.541176 0.294118 / 0.0510588)`（dark，叠在 body 底 `rgb(8, 12, 18)` 上 → **有效底色 `rgb(20, 18, 21)`**），按上述口径估算有效字色与卡片底的对比度：**editorial 5.99:1**（有效字色 `rgb(90, 90, 90)`）、**dark 7.16:1**（`rgba(255,255,255,0.6)` 合成到 `rgb(20,18,21)` → 有效字色 `rgb(161, 160, 161)`），两主题均 ≥ 4.5:1（上一轮 ≈3.0:1 / ≈3.2:1 已修复）。文案实测「队列还没清空：补出来的候选会不全，清空后才能补。」。截图 radar-gap-final-editorial-1440x900.png、radar-gap-final-dark-1440x900.png。

### A-13 禁用 ghost 按钮可读性（opacity .55）【pass】
复验（同一积压态）：`[data-testid="radar-gap-backfill-2026-11-23"]`（行级 ghost「补这天」）computed `opacity` = **0.55**（`getComputedStyle` 实测，editorial 与 dark 各一次均 0.55；构建产物内 `.rs-gap-btn[data-v-361faf1f]:disabled{opacity:.55}` 覆盖全局 `.app-button:disabled{opacity:.4}`），DOM `disabled` 属性为 true、`aria-disabled` 为 null。文字与卡片底的近似对比度：**editorial 2.36:1**（字色 `rgb(90,90,90)` 以 0.55 组不透明度压到有效底色 `rgb(248, 237, 232)` 上 → 有效字色 `rgb(161, 156, 154)`）、**dark 2.99:1**（`rgba(255,255,255,0.6)` × 0.55 → 有效字色 `rgb(97, 97, 98)`，底 `rgb(20, 18, 21)`），较上一轮 opacity .4 的 ≈1.8:1 明显改善。**口径说明：禁用控件不受 WCAG 1.4.3（文本对比度）约束**，此处数值只作可用性记录，不作合规判据；判定 pass 的依据是「有效 opacity 已由 .4 调整为 .55 且目标 testid 命中」。截图 radar-gap-final-editorial-1440x900.png、radar-gap-final-dark-1440x900.png。

### A-14 未知态文案分叉（Embedding 计数不可得）【pass】
复验（浏览器网络层把 `**/api/poll` 响应换成**不含 `embedding_queue` 字段**的报文，其余同积压态）：`[data-testid="radar-gap-queue-blocked"]` 文本实测为「**队列状态未知：无法确认是否已清空，清空后才能补。**」，脚本化断言 `text.includes('队列状态未知') === true`、`text.includes('还没清空') === false`（分叉成立，未误落入「队列还没清空」）；同页 `radar-status-embedding-queue` 显示「状态未知」、卡片内提示为「Embedding 队列状态未知：无法确认是否已清空，暂不可生成。」；两入口仍为 disabled（`补这天` computed opacity 0.55、`一次补齐` disabled=true）。对照积压态文案「队列还没清空：补出来的候选会不全，清空后才能补。」，两种原因互斥、不会混用。截图 radar-gap-final-editorial-unknown-1440x900.png。

### A-15 视觉收口（双主题 1440×900 展开态）【pass】
复验：1440×900 展开态两主题截图 radar-gap-final-editorial-1440x900.png、radar-gap-final-dark-1440x900.png（PNG IHDR 实测均 1440×900）。**无嵌套异质面**：缺口区 `.rs-gap-body` 计算样式 background-color=`rgba(0, 0, 0, 0)`、border-top-width=0px / border-bottom-width=0px、padding=0px、box-shadow=none，两主题一致——缺口区直接坐在卡片 `.rs-block` 底上（editorial `color(srgb 0.85098 0.290196 0.290196 / 0.06)`、dark `color(srgb 0.941176 0.541176 0.294118 / 0.0510588)`），无自带异色面/边框；行 `.rs-gap-row` 背景 `rgba(0, 0, 0, 0)`、border-top-width=0px（本轮仅 1 个缺口日，命中 `:first-child` 去上边框规则）。**按钮单行不溢出**：两个按钮的文本 Range 客户端矩形数 = 1（`补这天` top 775.4 / bottom 790.4；`一次补齐 12 篇` top 820.4 / bottom 835.4），scrollWidth == clientWidth（59/59、110/110，overflowX=visible），行容器 right 1386.2 = 按钮 right 1386.2（溢出量 0），缺口行三个子元素（11-23 right 1132.8 / 12 个板块缺 right 1200 / 补这天 right 1386.2）均未越界；卡片 bottom 880.8 ≤ 视口 900、缺口区 bottom 865.4 ≤ 900（不裁切）。附可用态（队列已清空、无缺料提示、按钮 disabled=false）两主题截图 radar-gap-final-editorial-idle-1440x900.png、radar-gap-final-dark-idle-1440x900.png。


### A-16 禁用态一致性（两个入口共用 .55）【pass】
复验（积压态，浏览器网络层拦截 `**/api/poll` 注入 `tag_queue{pending 3, processing 1}` + `embedding_queue{pending 0, processing 0}`，1440×900，2026-11-25 10:26–10:29；构建产物已含本轮改动 `index.Cwiu3-lW.css` 内 `.rs-gap-act[data-v-31e2ab34]:disabled{opacity:.55}`，两个按钮 DOM 均带 `rs-gap-act` 类）：行级 ghost `[data-testid="radar-gap-backfill-2026-11-23"]` computed `opacity` = **0.55**、`disabled` = true；底部 secondary `[data-testid="radar-gap-backfill-all"]` computed `opacity` = **0.55**、`disabled` = true —— 两主题各测一遍，四个读数全部等于 0.55，**禁用态强度已一致**（上一轮为 .55 vs .4）。有效字色 ↔ 卡片有效底的近似对比度：editorial 行级 ghost 有效字色 `rgb(161, 156, 154)` / 有效底 `rgb(248, 237, 232)` = **2.36:1**，editorial 底部 secondary 有效字色 `rgb(129, 128, 127)` / 有效底 `rgb(254, 252, 252)`（其自带底 `rgba(255,255,255,0.85)` 叠在卡底上）= **3.87:1**；dark 行级 ghost 有效字色 `rgb(97, 97, 98)` / 有效底 `rgb(20, 18, 21)` = **2.99:1**，dark 底部 secondary 有效字色 `rgb(143, 143, 144)` / 有效底 `rgb(34, 33, 35)`（自带底 `rgba(255,255,255,0.06)`）= **4.97:1**。**口径说明：禁用控件不受 WCAG 1.4.3（文本对比度）约束**，上述数值只作可用性记录，不作合规判据；本条判定 pass 的依据是「两个入口 computed opacity 相等且都等于 0.55、disabled 均为 true」。附带回归：撤掉拦截回到真实态（双队列已清空）后，两按钮 opacity 回到 1、`disabled` = false、缺料提示消失，说明 `.rs-gap-act:disabled` 只作用于禁用态、不影响可用态。截图 radar-gap-final-disabled-parity-1440x900.png（editorial，1440×900）。

## 复验发现的问题（供主线程判断，均非阻断项）

1. ~~**缺料提示对比度不足**~~ —— **本轮已修**（见 A-12）：改为 `.rs-gap-blocked`（0.7rem = 11.2px + `--color-text-secondary`），实测 editorial 5.99:1 / dark 7.16:1。
2. ~~**禁用态 ghost 按钮几乎看不见**~~ —— **本轮已修**（见 A-13）：行级 ghost `.rs-gap-btn:disabled` 由 opacity .4 上调到 .55，有效字色对比度 editorial ≈1.8:1 → 2.36:1、dark → 2.99:1。
3. **「有失败任务」仍不在提示语覆盖范围**：门槛判据里 failed 不计入（D8），真实态常见「1 个失败 + 队列已排空」；缺料文案本轮虽已按「Embedding 计数不可得」与否分叉，但「有失败任务」这一原因仍无独立话术，用户看到的仍是「队列还没清空…」。未触发真实补填请求（红线），无法确证服务端 409 判据是否同样忽略 failed。
4. **/tags 雷达页无主题开关**：主题切换只在 /settings 与信息流顶栏可用，缺口区所在卡片无法就地切主题；本轮 A-11/A-12 的同页切换只能用与 `useTheme.setTheme` 同款的属性操作模拟。属可用性缺口，非本轮修复范围。
5. **dark 下「一次补齐 12 篇」的按钮边界偏弱**（沿用上一轮）：secondary 底色 `rgba(255,255,255,0.06)` 只比卡片亮 6%，与浅色主题（白底 85%）相比不够显眼（未禁用时文字对比度 13.2:1 仍过关，属观感问题）。
6. ~~**「一次补齐 12 篇」的禁用态仍是全局 opacity .4**~~ —— **已修**（见 A-16）：两个入口改为共用 `.rs-gap-act:disabled { opacity: .55 }`，积压态实测行级 ghost 与底部 secondary 的 computed opacity 均为 0.55（editorial / dark 各一遍），禁用态强度一致。

## 与 ui-design 契约的一致性核对

无阻断性不一致。逐条对照 openspec/changes/add-daily-report-gap-backfill/ui-design.md：

- 行头带 `N 天 / M 篇`：实测「近 7 天缺口 1 天 / 12 篇」，符合。
- 折叠区就地展开、复用既有卡片与 `.rs-*` 样式族、不弹窗不跳页：实测点击行头就地展开 `radar-gap-body`，页面无弹窗、无路由变化，符合。
- 「补这天」「一次补齐」均为 `AppButton` size=sm；本轮 `补这天` 已由 secondary 降为 ghost（background-color 实测 `rgba(0,0,0,0)`、border-width 0px），「一次补齐 12 篇」保持 secondary —— 与本轮样式修复一致，仍未新增基础组件。
- 队列门槛（补填入口与生成入口同款双队列判据）：实测积压态两入口 disabled=true + 缺料说明可见，见 A-9。
- 三条降级/空态文案：读取中→「近 7 天缺口 读取中」、空态→「近 7 天无缺口」、失败→缺口区呈「读取失败」并给「重试」按钮（契约表述为显示「读取失败 · 重试」，实现为同一区域内「读取失败」文本 + 「重试」按钮两个元素的同款信息呈现，信息等价、仅排版粒度不同，不构成契约违背）。
- 补填进行中禁用两个入口、行头转「补齐中 n/m」的分支未在浏览器实测（需真实触发补填，触及红线），由组件测试用例「补填进行中：两个入口禁用 + 进度行带报告日 + 行头转进度」覆盖并全绿。
