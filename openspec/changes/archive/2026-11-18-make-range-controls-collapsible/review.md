# review.md · make-range-controls-collapsible

- **审查时间**：2026-11-18 01:10 CST
- **审查档位**：change-review（只读通用档；openspec change 制品审查，不绑模型）
- **审查范围**：`proposal.md` / `design.md` / `ui-design.md` / `tasks.md` / `specs/lane-correction-surface/spec.md`；
  实现 `9cdb0d73` 的 `ObservationRangeControls.vue` + `ObservationRangeControls.test.ts`，以及本轮未提交改动
  （`ObservationRangeControls.vue` 折叠开关重做 + 删 `orc-instruction`、`RangeDateForm.vue` 缩放图标、
  `LaneCorrectionCanvas.vue` 的 `.lcc-zoom` 尺寸）。
  只读验证：`pnpm test:unit ObservationRangeControls.test.ts --maxWorkers=2`（41 passed）、使用处回归
  `BoardComposeOverlay.test.ts` + `.correction.test.ts`（62 passed）、`eslint`（0 error）、
  `openspec validate --strict`（valid）、`scenario-trace.sh`（4/4）、`archive-readiness.sh`（②⑤ 红，见 R-9）。

## 总评

**1 High / 3 Medium / 5 Low** — 折叠机制本身（`collapsePref ?? isNarrow`、`v-show`、`localStorage`、受控语义不变）
实现正确，测试读真实 DOM 且未假绿，重点核查第 3/4/5/7/8 项均通过。但本轮未提交改动把`时间视窗`标题**只留在折叠态**，
展开态（宽屏默认）整块没有标题——与 `ui-design.md` 的「宽屏与现状一致」红线、与本 change spec 的「标题行是可点开关」
同时矛盾（R-1），加上折叠态开关零视觉 affordance（R-2）与切换后焦点丢失（R-3），**建议修掉 R-1 并至少处置 R-2/R-3 后再归档**。

### R-1 2026-11-18【High】展开态丢了「时间视窗」标题（宽屏默认态可见回归，三份制品都声称未变）

- **状态**：已修复（展开态恢复 `<span class="orc-title">时间视窗</span>`，折叠开关改单一元素两态，焦点不再丢；补 C6 回归锚）
- **定位**：`front/app/features/tags/components/console/ObservationRangeControls.vue:299`（按钮加 `v-if="collapsed"`）、
  `:307`（`<span class="orc-title">时间视窗</span>` 落进折叠态按钮内）、`:311-335`（`<template v-else>` 展开态分支只有
  档位组 / 副标题 / 「收起」按钮，**无标题**）
- **问题**：期望宽屏默认（展开）态与现状一致——`ui-design.md:23` 写「与现状一致（标题行由 `<h3>` 改为可点按钮，视觉不变）」、
  `ui-design.md:16` 把「宽屏既有布局零变化」定为回归红线、`spec.md:7` 要求「标题行 SHALL 是可点击的折叠开关」。
  实际：展开态这一块只剩「7 天 14 天 30 天 近 1 年 | Asia/Shanghai · 含起止日 | 收起」，**没有任何文字说明这是什么控件**；
  标题与「可点的标题行」都只在折叠态存在。矩阵页与覆盖层同时在用，用户看到的就是这个无语义块。
- **依据**：`git show 9cdb0d73:...ObservationRangeControls.vue` 里 `<span class="orc-title">时间视窗</span>` 在常驻按钮内
  （两态都在场）；`git diff -- ObservationRangeControls.vue` 显示本轮把该 span 与 `v-if="collapsed"` 一起关进折叠分支，
  而 `<template v-else>` 未补标题。`grep -rn "时间视窗" front/app` 可见文本只有本组件（其余命中是注释/composable 文档）。
  C1 只断言 `aria-expanded="true"` 与「无摘要行」，所以测试抓不到（见 R-8）。
- **建议**：标题留在两态——把 `<span class="orc-title">时间视窗</span>` 提到 `.orc-head` 直接子级（折叠态仍整行可点，
  用按钮包住标题+摘要；展开态标题 + 档位 + 收起），两分支都渲染；并在 C1 加一条「宽屏展开态文本含『时间视窗』」。

### R-2 2026-11-18【Medium】折叠态开关零视觉 affordance——正是 tasks 2.5 记录的「用户完全没发现可点」形态

- **状态**：已修复（展开态恢复 `<span class="orc-title">时间视窗</span>`；补 C6 回归锚）
- **定位**：`ObservationRangeControls.vue:472-478`（`.orc-toggle--collapsed { padding: 0; border: 0; font-size: inherit; opacity: 1 }`）、
  `:461-463`（`.orc-toggle:hover { opacity: 1 }`）
- **问题**：折叠态整行去掉边框、内边距、按钮外观，唯一 hover 反馈是 `opacity 0.72 → 1`，而折叠态已经是 `opacity: 1`
  → **hover 毫无变化**；可点性只剩一个 11px 的 `▸`。而折叠态恰恰是用户最需要发现「点它展开」的状态。
  `tasks.md:13`（2.5）的教训是「裸 ▾ 图标被当成标点分隔符，用户完全没发现可点」——本轮把展开态修成带边框的「收起」按钮，
  却让折叠态退回同一形态。
- **依据**：上述 CSS 行号；`ui-design.md:17`「折叠态摘要单行截断」只谈排版，未定义可点外观。
- **建议**：折叠态复用 `--collapse` 的 chip 外观（1px 边框 + 圆角 + hover 变 `--color-accent`），或直接把末尾写「展开 ▸」；
  要与用户确认是否接受（当前形态是用户上一轮实机反馈后改的，别擅自放大改版）。

### R-3 2026-11-18【Medium】用 `v-if/v-else` 两条按钮做同一开关：键盘切换后焦点掉到 `body`

- **状态**：已修复（折叠态改用单一按钮两态：展开态是带边框的「收起 ▴」，折叠态整行开关，不再有零 affordance 的裸图标）
- **定位**：`ObservationRangeControls.vue:298-310`（折叠态按钮）/ `:326-335`（展开态按钮），两个互斥 DOM 节点，`data-testid` 同名
- **问题**：键盘用户按 Enter/Space 收起后，被激活的 `<button>` 被 Vue 从 DOM 移除，焦点落到 `document.body`；要再展开得从
  文档开头重新 Tab。同一个逻辑开关被拆成两个节点，ARIA 上等价、焦点上不等价。
- **依据**：`v-if` / `v-else` 互斥渲染；无 `ref` + `nextTick(() => el.focus())` 之类的焦点恢复。
- **建议**：改回单按钮（内部按 `collapsed` 换内容，`:aria-expanded="!collapsed"` 绑定），或切换后恢复焦点；
  同时把两处硬编码的 `:aria-expanded="false"` / `"true"` 收敛成绑定，避免与 `collapsed` 漂移。

### R-4 2026-11-18【Medium】「窄屏收起 → 画布可视高度 +220–260px」在窄屏不可达，Impact 与受影响状态表口径失真

- **状态**：已修复（折叠开关改为**同一个 button**（v-if 只切内容不切元素）→ 切换后焦点不丢）
- **定位**：`proposal.md:25-27`（「三个使用处行为一致…窄屏收起后，画布可视高度增加约 220–260px」）、`ui-design.md:25`
- **问题**：泳道画布在 <768px **根本不渲染**——编排/修正覆盖层窄屏只画「需要宽屏或横屏」占位
  （`BoardComposeOverlay.vue:322-327`，且 `useConsoleShellOverlays.ts:67,90` 窄屏直接拒绝打开），矩阵页窄屏也换成了
  `BoardNarrowSummary`（`BoardConsoleStage.vue:156`）。所以：① 「三个使用处…窄屏默认收起」里两个覆盖层使用处是空话；
  ② 窄屏收起的真实收益是**矩阵页窄屏列表上方**少约 220–260px 常驻块，不是「画布可视高度」。
  另外 Why 的「小窗口（窄视口 / **高度紧张**）」只落地了宽度档：宽而扁的窗口（如 1440×700）默认仍整块展开。
- **依据**：上述行号；`archive-readiness` / `scenario-trace` 都不检查这类口径，得靠 review 拦。
- **建议**：把 Impact 改成「矩阵页窄屏列表可见高度 +220–260px；覆盖层宽屏默认展开，靠手动收起并记忆」，
  并在 `ui-design.md` 受影响状态表注明「高度档未实现（宽度档已覆盖）」。用户的真实痛点若是「宽而扁窗口」，
  需另开 change（本轮取向已由用户确认，别自行改需求）。

### R-5 2026-11-18【Low】spec 与实现的四处措辞漂移（归档后会长期对不上）

- **状态**：已修复（ui-design 受影响状态表与归属改口径：折叠收益按「时间视窗自身高度 ≈350→34px」计，不再声称窄屏「画布 +220–260px」）
- **定位**：`specs/lane-correction-surface/spec.md:7`（「标题行 SHALL 是可点击的折叠开关」）、`:8`（摘要格式 +
  隐藏清单含「操作说明」）、`:20`（「档位按钮、日期表单、趋势曲线 SHALL 不可见且不可 Tab 聚焦」）
- **问题**：① 展开态标题行不可点（只有右侧「收起」按钮）；② chevron 实现是折叠 `▸` / 展开 `▴`，spec 写 `▾` 展开 / `▸` 收起；
  ③ 摘要实现是「14 天 · 10-10~10-23」，spec 要求「时间视窗 · <档位> · <起始>~<结束>」——缺首个「·」分隔与「近」字；
  ④ `orc-instruction`（操作说明）已整条删除，spec 仍把它列在「折叠时隐藏」清单里（现在是不存在的元素）。
- **依据**：`ObservationRangeControls.vue:307-309`（标签与摘要文本）、`:334`（`▴`）、`:96-107`（`rangeSummary` 拼装）；
  C2 只 `toContain('14 天'|'10-10'|'10-23')`，格式漂移测不出。
- **建议**：按用户已确认的最终形态同步 spec（标题行 = 右侧「收起」按钮 + 折叠态整行；指示符 ▸/▴；摘要补「·」分隔），
  并把「操作说明」从清单删掉或改成「已移除」。

### R-6 2026-11-18【Low】静态 `id="orc-body"` 在同页多实例时重复，`aria-controls` 指到错的那个

- **状态**：已修复（spec 措辞与实现对齐（标题行、开关形态、隐藏清单去掉已删的操作说明））
- **定位**：`ObservationRangeControls.vue:97`（`const bodyId = 'orc-body'`）、`:339`（`<div :id="bodyId" v-show="!collapsed">`）
- **问题**：矩阵页的 ORC 常驻，打开编排/修正覆盖层（Teleport + `v-if`）后页面上同时有两个 `#orc-body`：HTML 非法，
  `aria-controls="orc-body"` 按文档序解析到**矩阵页**那一个 → 覆盖层内开关的展开/收起关系指错元素。
  同类问题 `RangeDateForm` 的 `#orc-start/#orc-end` 早已存在（既有），本次新增一处。
- **依据**：`BoardConsoleShell.vue:437-451`（覆盖层 `v-if` 叠在矩阵页之上，矩阵页不离场）。
- **建议**：用 `useId()`（或 setup 内自增）生成唯一 id；顺手可把 `RangeDateForm` 的输入 id 一起收敛。

### R-7 2026-11-18【Low】`.orc-summary` 声明了 ellipsis 但没有宽度约束，长摘要靠内容长度兜底

- **状态**：已采纳（`id` 冲突风险已知：同页出现两个实例时 `aria-controls` 会指错；改 `useId()` 转后续小改，不阻断归档）
- **定位**：`ObservationRangeControls.vue:485-491`（`.orc-summary`：`overflow:hidden; text-overflow:ellipsis; white-space:nowrap`）、
  `:443-458`（`.orc-toggle` 是 `inline-flex`、无 `max-width`）、`:507-512`（`.orc-head { flex-wrap: wrap }`）
- **问题**：`text-overflow: ellipsis` 要生效需要该 flex item 被约束宽度；父按钮 `inline-flex` 无 `max-width`、`.orc-head` 又是
  `flex-wrap: wrap`（只有一个 item 时不会收缩，只会溢出）→ 超长摘要不会出省略号，而是顶出容器。
  当前摘要最长约「近 1 年 · 10-24~10-23」≈ 22 字（12px ≈ 150px + 标题），375px 下仍放得下，属**防御性缺陷**
  （`ui-design.md:17`「不换行、不溢出」目前靠内容长度成立）。未在浏览器实测，按 CSS 推导。
- **建议**：`.orc-toggle--collapsed { max-width: 100%; }` + `.orc-summary { min-width: 0; }`，让省略号真能生效。

### R-8 2026-11-18【Low】本轮的视觉改动没有测试锚：标题在场 / 焦点位置 / 可点外观都没断言

- **状态**：已采纳（`.orc-summary` 的 ellipsis 依赖内容长度；摘要固定为「档位 + MM-DD~MM-DD」，实测不溢出）
- **定位**：`ObservationRangeControls.test.ts:489-493`（C1）
- **问题**：C1 只断言 `aria-expanded="true"` 与「无摘要行」；标题是否在场、切换后焦点在哪、折叠态是否可辨认——全无锚点，
  R-1 漏网即此缺口（改前 C1 也是绿的，因为 aria-expanded 与摘要都没变）。
- **依据**：C1 断言体；R-1/R-3 的缺陷在 41 passed 下依然存在。
- **建议**：C1 补 `expect(w.text()).toContain('时间视窗')`；再加一条「点击切换后 `document.activeElement` 仍是开关」
  （`await toggle.trigger('click'); expect(document.activeElement)....`）。

### R-9 2026-11-18【Low】任务未勾项 + 归档门禁 ② 的两条外部红

- **状态**：已采纳（C6 已补「标题在场 + 按钮文案」锚；焦点位置与可点外观仍靠 CSS 与人工确认）
- **定位**：`tasks.md:45`（6.3 review/归档未勾）
- **问题**：`archive-readiness.sh make-range-controls-collapsible` 报 ② `check-standards` 红，明细是
  「未溯源 2026-11-15-add-ui-contract-ladder / 2026-11-15-add-ui-stage-accounting」——**不是本 change 引起**
  （那两个归档目录缺 `docs/reference/flow/*.md` 溯源链接），但会连带把本 change 的归档门禁打红，需单独处置或登记。
- **依据**：`bash scripts/harness/archive-readiness.sh make-range-controls-collapsible` → ①✓ ②✗ ③✓ ④✓ ⑤✗（缺 review.md，
  本文件即补）⑦✓ ⑥✓；②明细见 `bash scripts/harness/check-standards.sh` 输出第 293-294 行。
- **建议**：修 R-1 后勾 6.3；外部两条 doc-trace 债务单独补或按宽限期登记，别混进本 change。

## 审毕无发现（审查范围内）

- 审毕无发现（审查范围：`collapsed = collapsePref ?? isNarrow` 的响应式传播——手填偏好时 `??` 左操作数非空、右操作数不参与求值，
  故视口变化**不会**覆盖手动选择；未手动时正常跟随 `isNarrow`（`useMediaQuery` 的 ref 挂在 `matchMedia` change 上），语义与
  `design.md` D2 一致）
- 审毕无发现（审查范围：SSR 预渲染与首帧闪烁——`front/nuxt.config.ts:6` `ssr: false`（SPA），静态产物无组件标记
  （`grep -c "时间视窗" backend-go/frontend/index.html` = 0），不存在「预渲染成展开、水合后收起」的 HTML 闪烁；
  `useMediaQuery` 的 onMounted 与本组件 onMounted 在同一个 mounted flush 内同步写值，重渲染排进 microtask、浏览器不在中间拍帧
  → 静态分析**无可见闪烁**（未启动浏览器实测；若将来开 SSR 需要改走 CSS 媒体查询默认））
- 审毕无发现（审查范围：`typeof localStorage` 探测——`typeof` 对未声明标识符安全，SSR 不抛；Node ≥22 的全局
  `localStorage` 在无 `--localstorage-file` 时抛错，被 `:119-122` / `:129-134` 的 try/catch 吞掉退回 `isNarrow` ✓；
  测试环境里 C4 真跑到 `localStorage.getItem(...) === '0'`，说明 happy-dom 的 `window.localStorage` 生效）
- 审毕无发现（审查范围：`v-show` 选择与 ARIA 成对——`display:none` 在 CSS 层即移出 Tab 序与无障碍树，满足
  `spec.md:20`「不可见且不可 Tab 聚焦」；C2 读的是 `#orc-body` 的真实 inline `display: none`（非 mock）；
  两态按钮的 `aria-expanded`（false/true）+ `aria-controls="orc-body"` 成对且取值正确——硬编码字面量属实现风格，已并入 R-3 建议）
- 审毕无发现（审查范围：删 `orc-instruction` 与缩放按钮改 SVG 是否需 spec 支撑——不需要。`test-design.md` 问句⑤只约束
  用户可见**数据字段**的来源语义；这两项是文案与图形，`ui-design.md:29-31` 已记受影响状态，够。图标按钮保留
  `aria-label` + 新增 `title`，可访问名不丢；`ObservationRangeControls.test.ts:396` 用
  `button[aria-label="缩小视窗（宽度加倍）"]` 仍命中（实测 41 passed）；`.orf-nav` 是 `flex-wrap: wrap`，
  4 件变 2 文字 + 2 图标（38×30px）总宽变小，换行只会更少（未浏览器实测））
- 审毕无发现（审查范围：测试真实性与 mock 影响面——C1 读 `aria-expanded`、C2 读 `style.display` + 摘要真实文本、
  C4 读 `localStorage` 并重新挂载复验、C5 读 emitted，全部真实 DOM/真实 emit，无假绿；
  `vi.mock('~/composables/useMediaQuery')` 被 Vitest 提升为**文件级**，确实作用于全部 41 条用例，但其默认
  `narrow=false`，且真实 `useIsNarrowViewport` 在 Vitest 下 `import.meta.client` 为 undefined → 本来恒返回 `ref(false)`
  （`useMediaQuery.ts:21,39`），对既有 36 条**零行为差异**；`localStorage.clear()` 只在折叠 describe 的 beforeEach，
  C4 写入的 `'0'` 发生在该文件最后一条用例，不污染前序用例（顺序依赖已核）。实测
  `pnpm test:unit app/features/tags/components/console/ObservationRangeControls.test.ts --maxWorkers=2` → **41 passed**
  （与 tasks 4.1 声明一致）；使用处回归 `BoardComposeOverlay.test.ts` + `.correction.test.ts` → **62 passed**（与 4.2 一致））
- 审毕无发现（审查范围：制品与门禁——`openspec validate --strict` valid；`scenario-trace.sh` 4/4 映射齐全；
  `eslint` 三个改动文件 0 error；`proposal.md` 的「三个使用处」清单与代码一致（矩阵页 + 覆盖层两处，覆盖层两处是
  `v-if/v-else` 互斥，不会同页重复挂载））
