# review — clarify-lane-correction-effect-timing

审查范围：`BoardComposeOverlay.vue` 顶部徽标组新增 + 修正模式页脚删除 + scoped 样式改写 + `BoardComposeOverlay.correction.test.ts` 断言迁移（并发 change `rework-lane-compose-swimlane-canvas` 的阈值三档 / 检索态 / 工具条改写不在本次范围，另见报告「并发观察」节）。审查日期：2026-11-22。

### R-1 2026-11-22【High】ui-design Interaction Decisions 的「模式」列用了自造词，不在 interaction.md 模式词表内
- **状态**：已修复
- **处置（补记 2026-11-22）**：已按建议 (a) 改写 `ui-design.md`「## Interaction Decisions」：D1/D3 模式列改用词表取值 `canvas`（工作面主模式）、D2 用 `inline-edit`；forbidden 列同步改成词表内的模式名（`dialog`, `drawer`）；「结论型徽标 / 只给结论 / 非教学语」等表述移入决策正文，并补判据编号（P1 / P2）。未走 (b) 扩充词表路径——本 change 无新交互面需求。
- **处置**：`openspec/specs/ui-design-workflow/spec.md:31` 规定「`模式` MUST 取自 `docs/reference/standard/frontend/interaction.md` 的模式词表」，同 spec 的 Scenario「决策台账缺模式词表取值」明确写「制品 SHALL 判定为不完整，review MUST 要求改用词表取值或先提案扩展词表」；`interaction.md:19-30` 同款 SHALL NOT「在 change 内临时发明模式名（「轻量面板」「侧边浮窗」「快捷卡片」一律不合法）」。本 change `ui-design.md:39-41` 三行的模式列分别写 `status-badge（状态结论）`、`conclusion-only`、`conclusion-only`，三者均不在词表（page / panel / drawer / dialog / inline-edit / canvas / command）内；且全仓（含 archive）所有 ui-design.md 中仅本 change 出现这两个词。处置二选一：(a) 改用词表取值——D1–D3 实为纯文案/展示决策而非交互位置，模式列可写该工作面主模式 `canvas`，把「结论型徽标 / 只给结论 / 非教学语」写进「决策」列自由文本；(b) 若确需新模式，先在 requirements 档按 interaction.md 判据提案扩充词表再回填台账（不得就地自创）。附带：同一行的 forbidden 列写的是「解释型长句 / 底部重复徽标 / 全局保存按钮」等非模式词，建议按台账核对口径一并对齐。

### R-2 2026-11-22【Medium】Data Provenance 的三项 none 源未标 needs-approval，且自述「无需用户审批」
- **状态**：已修复
- **处置（补记 2026-11-22）**：三行 source 保持 `none` 并**补标 `needs-approval`**（说明列写明「静态结论文案，非数据值」+ 用户 2026-11-22 对话确认记录位置）；**删除了**末行「按 interaction.md 规则标 none 即可，无需用户审批」这句与规则相反的引述。口径取 (a)（徽标仍按「界面上独立可指认的徽标」计入枚举），未走 (b) 移出表。
- **处置**：`interaction.md:100-104` 与 `ui-design-workflow/spec.md:31` 均要求「取 `none` 者 MUST 标 `needs-approval` 并在对话中获得用户确认后方可进入实现」。`ui-design.md:47-51` 三枚徽标 source 均写 `none`，既未标 `needs-approval`，也未记用户确认，末行更写「按 interaction.md 规则标 none 即可，无需用户审批」——该表述与规则原文相反。缓解证据：stage.md 记录了 2026-11-22 用户拍板「结论型徽标」形态与「顶部常驻」方向，实质已有用户确认。处置建议二选一：(a) 三行加 `needs-approval` 并在说明列指向 stage.md 的用户拍板记录（对话确认已在案，补齐机器可读标记即可）；(b) 按 interaction.md「装饰性静态文案不计」的枚举口径，将三枚静态徽标从 Data Provenance 表移出（表内只留真实数据项），同时删掉末行那句错误引述——注意口径二选一后不可两处并存（若移出，须确认 `lane-correction-surface`「无来源的装饰性徽标 SHALL NOT 出现」仍由主 spec 自带的「只保留结论型徽标」豁免覆盖）。

### R-3 2026-11-22【Medium】顶部徽标组丢失作用域锚点，且与左栏既有徽标同屏重复
- **状态**：已采纳
- **处置（补记 2026-11-22）**：取建议 (c)——不删左栏徽标、不给顶部加作用域词，而是在 `design.md` D2「作用域口径」与 `ui-design.md`「复用组件与布局模式」/「明确不做」记清分工：**顶部 = 全局生效时机摘要**（覆盖加入 / 移出 / 首义 / 改名四类动作，故「归属立即生效」对改名不准确，不加单一作用域词）；**左栏面板头 = 分块作用域**（成员归属 / 首义范围），属既有组件、本次不扩范围去重。
- **处置**：被删页脚原文是「归属 · 立即生效」+「次日日报生效」，新徽标组写「立即生效」+「次日日报生效」+「无需保存」（`BoardComposeOverlay.vue:414-418`），作用域锚点「归属」随页脚删除而丢失：单看标题行，三枚徽标无法自解释哪一项动作对应哪种生效时机。而「归属 · 立即生效」（`LaneMemberList.vue:110`）与「次日日报生效」（`LaneAnchorScope.vue:104`）仍在左栏面板头常驻——ui-verification 截图 `lane-correction-effect-badges-1440x900.png` 可见「次日日报生效」在同一屏出现两次（标题行右上 + 首义范围面板头），「立即生效」与「归属 · 立即生效」亦重复。这与 proposal「内容上移，不两处重复」的自述论证不自洽。处置建议（可选，按影响面排序）：(a) 顶部徽标保留作用域（`归属立即生效` 仍满足 ≤6 字）；(b) 或删除左栏 `LaneAnchorScope.vue:104` 的重复徽标以真正去重（属扩范围，需先确认左栏该徽标是否承担其它语义）；(c) 若维持现状，建议在 design.md 记录「顶部=全局结论摘要 / 左栏=分块作用域」的分工口径，避免后续被当重复缺陷反复翻案。

### R-4 2026-11-22【Low】delta spec Scenario「生效时机徽标不是解说句」缺机械护栏（断言过弱）
- **状态**：已修复
- **处置（补记 2026-11-22）**：测试改为精确断言 `expect(badges.text()).toBe('立即生效次日日报生效无需保存')` + 逐枚 `text().length <= 6` 断言（实测 `text()` 三段直接相连、无空白无换行）；子线程做过护栏非空转自检（阈值改 `<=5` 即红）。
- **处置**：delta spec 的 Scenario「生效时机徽标不是解说句」要求「每项 ≤6 字 / SHALL NOT 出现解释生效机制、派生值口径或操作教学的句子」，但 `BoardComposeOverlay.correction.test.ts:145-151` 用的是 `expect(badges.text()).toContain('立即生效')` 等三条包含式断言——未来若追加一句科普（如「（首义与重心次日重算）」）测试仍全绿，该 Scenario 无任何护栏。建议改为精确断言：`expect(badges.text()).toBe('立即生效次日日报生效无需保存')`，并补逐条长度断言（三枚均 ≤6 字），把「不是解说句」落成可执行判据。

### R-5 2026-11-22【Low】新建模式用例只做负向断言，未锚定「工作面确实渲染」
- **状态**：已修复
- **处置（补记 2026-11-22）**：新建模式用例补正锚 `expect(w.find('[data-testid="lane-compose-canvas"]').exists()).toBe(true)`。
- **处置**：`BoardComposeOverlay.correction.test.ts:315-324` 仅断言 `lane-correction-effect-badges` 不存在，未同时证明组件已渲染出新建工作面；若将来 mount 抛错或窄屏分支（`narrowState` 被置 true）生效，该用例会假绿。当前 `beforeEach` 已把 `narrowState.value = false` 且同 describe 块另有用例断言 `lane-compose-canvas` 存在，实际风险低，但建议在本用例补一行 `expect(w.find('[data-testid="lane-compose-canvas"]').exists()).toBe(true)` 作为正锚。

### R-6 2026-11-22【Low】「无需保存」徽标用 --color-text-muted，11px 下对比度低于 WCAG AA
- **状态**：已修复
- **处置（补记 2026-11-22）**：`.bco__timing--plain` 改用 `var(--color-text-secondary)`（浅色约 6.5:1，达 AA；仍是中性色阶，不与 accent 实线徽标抢眼）。
- **处置**：`.bco__timing--plain { color: var(--color-text-muted) }`（`BoardComposeOverlay.vue:997-999`）。浅色主题 `--color-text-muted: #8a8a8a` on `--color-bg-base: #faf7f2` 对比度约 3.2:1；深色主题 `rgba(255,255,255,0.35)` on 暗底约 3.3:1——均低于 11px 常规文字的 AA 阈值 4.5:1。被删页脚原本用 `--color-text-secondary`（浅色 #5a5a5a ≈ 6.5:1），本改动把「回答用户报障的那一枚徽标」降到了更弱的色阶。建议 `.bco__timing--plain` 改用 `var(--color-text-secondary)`（仍比 accent 实线徽标弱、保留中性区分，但可读），或提至 12px 并加粗。

### R-7 2026-11-22【Low】状态徽标置于标题行最右，与 interaction-conventions §1 的取向相反
- **状态**：已采纳
- **处置（补记 2026-11-22）**：维持现状并记判断——`design.md` D1「右置判断」：该条字面作用域是列表项级状态图标，本处是页级汇总徽标，标题行右区无其它 meta 元素可挤压，且实测两档视口内完整可见（`ui-verification/report.md` A-2）。
- **处置**：`docs/reference/standard/frontend/interaction-conventions.md` §1「状态标记必须在左侧醒目位，不得埋在右侧 meta」，理由是「右侧 meta 聚集计数/箭头，状态会被淹没到根本看不见」。本实现用 `margin-left: auto` 把徽标组推到标题行最右（`BoardComposeOverlay.vue:971-979`）。该条字面作用域是「条目标题的状态图标（列表项级）」，此处是页级汇总徽标、不严格适用；且标题行右区无其它 meta 元素，ui-verification 实测 1440×900 / 1920×1080 两档徽标组完整在视口内（1920 实测 x:1688 / w:212 / right ≤ innerWidth），可见性成立。仅作取向提示：可考虑让徽标紧邻标题与窗口标签之后（不 push 到最右），或在 design.md 显式记录「标题行右区为空区、右置不构成 meta 挤压」这一判断，避免后续被同一原则反复质疑。
