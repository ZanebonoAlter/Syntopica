# 审查报告 — add-prototype-slop-filter

> 审查类型：类型 A（openspec change 制品 + 规范文本一致性审查）。只读审查，除本文件外未改动任何仓库文件；未执行 build / 测试。
> 审查对象：`proposal.md` / `design.md` / `ui-design.md` / `tasks.md` / delta spec，以及被审产出 `docs/reference/standard/frontend/interaction.md` 新增三节。
> 对照：`docs/reference/standard/frontend/theming.md`（56 行全读）、`interaction-conventions.md`（69 行全读）、`layout.md`「交互模式选择判据」节、`loading-experience.md`。

## 总评

方向成立（吸收 antislop 的判据本体、不吸收 Delivery Gate 报告形态），但**当前版本不宜归档**：新增三节里有 3 条 High 会让「评审者逐条核对」这件事在旗舰场景（画布类原型）无法执行，且 delta spec 与文档正文存在 1 处可验证的落点缺口。问题集中在**可核性**与**载体缺失**，不是方向错误——按下面建议补齐后可归档。

- 3 条 High（V3/V4/V5 与画布类原型形态互斥；诚实占位/伪造分界不可判定；第 8 个 Scenario 无文档落点）
- 5 条 Medium（破折号条款前提与 spec 漂移；`—` 语义双关；与 Data Provenance 的审批缺口；对比度无人可测 + 裁决句不完整；加载态与 loading-experience.md 可能互斥）
- 4 条 Low（elevation/radius 无定义口径；delta 与正文措辞漂移；行数预算与实测不符；task 6.4 验证口径不可判定）

---

### R-1 2026-11-15【High】V3/V4/V5 在画布类原型（状态图）形态下不可核，且与同文件「画布类 SHALL NOT 用可点 HTML」互斥
- **状态**：已修复
- **位置**：`docs/reference/standard/frontend/interaction.md:193-195`（V3/V4/V5）、`:172`、`:177`（画布类原型形态）；delta spec `specs/frontend-interaction-contract/spec.md:64-70`（两条 Scenario）；design.md D3
- **问题**：V3（`outline: none` 即违规）、V4（Tab/Enter/Esc 走通）、V5（响应 `prefers-reduced-motion`、`>5s` 动效须有暂停控制）都是 HTML/CSS 层属性。但同一文件的「Requirement: 画布类交互的原型形态」规定画布类原型 MUST 采用「状态图 + 操作表」、SHALL NOT 用可点 HTML 模拟，且验收基准是「三张状态图逐态对账 + `interaction-table.md` 六列逐行对账，**不需要可交互页面**」。静态状态图上没有焦点样式、没有 Tab 序列、没有媒体查询，两条 Scenario 的 WHEN（「focus 样式写成 `outline: none`」「画布类原型有持续脉冲动效」）预设了可点 HTML —— 恰是画布类被禁止的形态。后果：旗舰 major UI 场景下 V3–V5 无判定对象（要么空转，要么把原型形态推回可点 HTML 而违反既有 MUST）。
- **依据**：`interaction.md:172`、`:177` 与新增 `:193-195` 直接冲突；`docs/reference/standard/frontend/` 全目录 grep 无任何「状态图」= HTML 的说明；`design.md` D3 只论证「为什么要在原型阶段定」，未论证「在状态图形态下怎么核」。
- **建议**：给 V3/V4/V5 指定非 HTML 载体，二选一——(a) 在 `interaction-table.md` 增一列（如「可访问性/动效」）承载焦点顺序、焦点样式、Esc 行为、减动降级描述，V3/V4/V5 改为「必须声明且声明内容可核」；(b) 明确本节仅适用于可点 HTML 原型，画布类原型用 (a) 的声明式替代。同步把两条 Scenario 的 WHEN 收窄到「可点 HTML 原型」并补一条画布类的声明式 Scenario。

### R-2 2026-11-15【High】「诚实占位 vs 伪造内容」的分界不可判定，且「显式标注示例」出口把已测出的 slop 模式合法化且无剂量上限
- **状态**：已修复
- **位置**：`docs/reference/standard/frontend/interaction.md:118-119`（三档表）；design.md D1（「诚实占位」白名单与「明确标注 `示例`」出口）
- **问题**：(a) 白名单 `email@example.com` 与禁例 `johndoe@example.com` 只差 local part，规则未给判定依据——`alice@example.com`、`张三@example.com` 归哪档？`Your Name` 之外的通用占位名（`Full Name`、`N/A`、`xxx`）是否合法也未界定，评审者只能靠猜，正是本 change 要消灭的猜。(b) 「显式标注「示例」」没有书写形式定义（单独写「示例」/ 前缀/ 后缀/ 图例？）。(c) 该出口没有剂量上限：`proposal.md` 记录旧原型 `示例` 命中 249 处，说明「示例」正是被判定为 slop 的高频模式；现把它列为合法出口，而「≤12 短语/屏」的计数口径只数按钮名/标签/徽标/标题，**不数表格单元值**，一张 20 行全「示例」的表在该预算下计 0 短语 → 占位标记实际上无上限。
- **依据**：`interaction.md:118` 白名单与 `:119` 禁例不可由同一判据推出；`interaction.md:159` 计数口径不含数据单元值；`proposal.md:10` 记录 `示例` 249 处 / `fake` 163 处。
- **建议**：把诚实占位白名单收敛成封闭可枚举形式（只保留 `Your Name` / `email@example.com` / `[REAL DATA]` / `—` / 固定字面如「示例数据」），写明「显式标注」必须同时出现的载体（列头/表注/图例三选一），并补一句与真实值的分界规则（如「local part 取自白名单才算占位，否则按伪造判」）；同时给占位标记加剂量上限（如每屏 ≤2 处占位标记，且表格占位列 MUST 在 provenance 表有行）。

### R-3 2026-11-15【High】delta 的第 3 个 Scenario「诚实占位被接受」在 interaction.md 正文无落点，tasks 2.1 的「逐条一致」验证不成立
- **状态**：已修复
- **位置**：`openspec/changes/add-prototype-slop-filter/specs/frontend-interaction-contract/spec.md:24-26`；`docs/reference/standard/frontend/interaction.md:111-129`；`tasks.md` 2.1 与「Scenario → 落地对账」表第 3 行
- **问题**：delta 声明 3 个 Requirement / 8 个 Scenario，但 `interaction.md` 的「占位数据与伪造内容必须区分」节只有 2 条 Scenario（模板假名被驳回、无对照期的涨幅被驳回），**「诚实占位被接受」只存在于 delta spec，正文无对应判据**。正文只承载「禁」的一半，缺「放行」的一半——照正文评审会把诚实占位误判为待改（正是 design D1 风险表想避免的误伤）。tasks 2.1 却把「delta 与文档逐条一致（Scenario 名）」标为已通过；对账表把该 Scenario 的落地方式写成「人工：对读……表第 2 行」，用「人工」掩盖了「正文里根本没有这条 Scenario」的事实。
- **依据**：`grep -n '^#### Scenario'` delta 得 8 条，`interaction.md` 全文 14 条，其中本次新增仅 7 条，缺 `诚实占位被接受`；tasks 2.1 与对账表表述见上。
- **建议**：把「诚实占位被接受」补进 `interaction.md` 正文（WHEN 原型空表单用 `Your Name` / `email@example.com`，或某处写 `[REAL DATA]` → THEN 判定合格，SHALL NOT 被要求填真数），并把对账表该行改为指向正文；若认为正文不需要，则从 delta spec 删掉该 Scenario，两处只能留一处。

### R-4 2026-11-15【Medium】破折号条款的立论前提不成立、未进 delta spec，且落库即产生存量违规
- **状态**：已修复
- **位置**：`docs/reference/standard/frontend/interaction.md:200`；design.md D4 与「Non-Goals」（proposal.md:28）
- **问题**：design D4 的理由是「本仓库全部文档与界面均为中文」，但仓库确有英文界面文案：`front/app/features/settings/components/SettingsSectionDatasources.vue:110/151`（`API Key`）、`front/app/features/ai/components/VendorCard.vue:190`（`stale`）、`front/app/features/tags/components/DailyReportReader.vue:682`（`Board Dynamics`）、`front/app/features/ai/components/VendorDialog.vue:249`（`OpenAI Compatible`）。更直接的反例：`front/app/features/tags/components/MatchingConfigDialog.vue:115/134/158/187` 的可见文案 `direct_hit — 精确重叠` / `hit_rate — 命中率达标` / `max_sim — 最高相似度达标` / `weighted — 加权综合` 已是「英文标识 + 单 `—`」，新规一旦生效即判定存量产品代码违规，而本 change 声明「不改产品代码」、proposal 只说「新规则只对 rollout 后创建的 change 生效」（管 change，不管既有代码）。此外这条 MUST 只写在文档里，delta spec 的「视觉与动效」Requirement 不含它——归档后 spec 与文档将出现一条只在文档里的 MUST（单向权威）。
- **依据**：`grep -rnE '>[A-Za-z][A-Za-z ,'"'"'\-\.]{3,}<' front/app --include=*.vue` 命中上述文件；`interaction.md:200`；delta spec `:48-62` 无破折号条款。
- **建议**：三选一并保持文档与 spec 同步——(a) 删除该条（本仓库 UI 以中文为主，收益接近零、误伤面确定存在）；(b) 保留但限定「仅英文整句文案（不含中文、不含代码标识）」并写明存量豁免（如「存量组件不受限」+ 列表登记到 `theming.md` 或 `interaction-conventions.md`），同时补进 delta spec；(c) 降级为软提醒而非 MUST。

### R-5 2026-11-15【Medium】`—` 同时是「诚实占位」白名单与破折号规则对象，边界未定义
- **状态**：已修复
- **位置**：`docs/reference/standard/frontend/interaction.md:118` 与 `:200`
- **问题**：`:118` 把 `—` 列为诚实占位的合法写法，`:200` 又规定「英文界面文案 SHALL NOT 用 `—`；中文文案的 `——` 不受此限」——中文语境下的**单** `—`（既非英文文案、也非 `——`）既未被豁免也未被禁止，两个审阅者可以得出相反结论。而 `—` 恰恰是本仓库前端的占位符主流写法（`front/app/features/settings/components/SettingsSectionAiHealth.vue:146` 的 `<span>—</span>`、`front/app/features/settings/components/DegradationPanel.vue:87` 的 `— 调用 · — 降级 · — 失败`、`FeedSourceQualityPill.vue` 注释亦以「—」占位）。按「与占位白名单同源」的读法，占位写法本身可能违反同节另一条 MUST。
- **依据**：`:118` 与 `:200` 并列；`grep -rn '—' front/app --include=*.vue` 命中上述占位用法。
- **建议**：一句话划界并前后对齐——例如「`—` 作为数据占位符属诚实占位档，不受破折号禁令约束；破折号禁令只约束句内标点用法，且仅适用于英文整句文案」，并把 `:200` 的豁免补上「数据占位符」。

### R-6 2026-11-15【Medium】新增「诚实占位 → 允许」与既有「Data Provenance → `none` 须 `needs-approval` + 用户确认」之间缺一句衔接
- **状态**：已修复
- **位置**：`docs/reference/standard/frontend/interaction.md:91-109`（数据来源必须显式声明）、`:111-129`（占位节）；`openspec/specs/ui-design-workflow/spec.md:32`
- **问题**：既有条款要求「每个用户可见数据项一行」，`source` 取 `none` 者 MUST 标 `needs-approval` 并在对话中获用户确认后才能进实现；枚举口径含「界面上独立可指认的数字、列表、徽标或图表」。新节把诚实占位判为「允许」，却没说它是否仍需 provenance 行、是否仍要 `needs-approval` 与用户确认。于是同一份占位原型有两种读法：(a) 新节「允许」= 放行，可跳过审批 → 违反 provenance；(b) provenance 仍要求逐项 `none` + 审批 → 一个诚实占位原型被卡在审批上，而新节没说这不构成违规。这两条正是评审重点点名的「会不会互相打架」，答案是会——缺一句判据。
- **依据**：`interaction.md:104`「取 `none` 者 MUST 标 `needs-approval` 并在对话中获用户确认后才可进实现」与 `:118`「诚实占位 …… 允许」并存；`ui-design-workflow/spec.md:32` 同款要求。
- **建议**：在占位节补一行判据，例如「诚实占位仍 MUST 在 provenance 表有行，`source` 填 `none` + `needs-approval`；'诚实占位' 只免除『必须换成真数』，不免除审批」——把「内容档」与「审批态」两个维度分开，冲突即消。

### R-7 2026-11-15【Medium】V1/V2 对比度无人可测，且「冲突时以 V1/V2 为准」只裁了取值、不裁改哪个文件
- **状态**：已修复
- **位置**：`docs/reference/standard/frontend/interaction.md:191-192`、`:198`；design.md D3 与风险表末行
- **问题**：(a) 「谁测」缺位：原型是静态 HTML/状态图，仓库无对比度工具链；条款既没说测量对象（HTML 内联色值 vs 状态图像素）、也没说测量人、也没说证据落在哪（design D5 说落在 `ui-verification/report.md` 的判定条目，但正文未引用该载体），评审者实际只能目测——那 4.5:1 就是装饰性数字。(b) 裁决句「冲突时以 V1 / V2 的对比度下限为准」不完整：它裁了「取值 vs 下限」，但没裁「发现 token 组合不达标时改谁的文件」；而 `theming.md` 自身**没有任何对比度要求**（`docs/reference/standard/frontend/*.md` 中 `4.5:1` 仅出现在 `interaction.md:191`），所以一个不达标的 token 组合对两份文档同时「合规」——双权威真空。裁决句不足以防住这点。
- **依据**：`:198` 分工句；`grep -rn '4\.5:1' docs/reference/standard/frontend/` 只命中 `interaction.md:191`；`theming.md` 全文 56 行无对比度/焦点环条款。
- **建议**：把裁决句补成三段式：「颜色取值与间距 → `theming.md`；可读性/可操作性下限 → 本节 V1–V6；发现 token 组合不达 V1/V2 时 → 原型 MUST 换用达标 token，token 本身不达标则报 `theming.md` 修订，SHALL NOT 在原型里私自调色绕过」。另加一句测量口径（测 HTML 内联色值、结果写进 `ui-verification/report.md` 判定条目）。

### R-8 2026-11-15【Medium】「空态/加载/错误态」节未声明适用范围，与 `loading-experience.md` 的加载态规定可能互斥
- **状态**：已修复
- **位置**：`docs/reference/standard/frontend/interaction.md:131-151`（尤其 `:138` 加载行）；`docs/reference/standard/frontend/loading-experience.md:3`（唯一权威声明）与 `:12-14`（四层加载体验）
- **问题**：新节标题与表格都未写「只管原型」还是「原型 + 产品界面」。若含产品界面：`:138` 要求加载态 MUST 说清「在加载什么」、SHALL NOT 出现「裸 spinner」，而 `loading-experience.md`（自称 SPA 首屏/路由加载的唯一权威）规定路由切换用「顶部进度条 + 居中轻量 spinner（`role=status`）」，首屏用「纯静态内联 CSS/spinner + 拟真进度条」——即仓库既有的加载规范就是以 spinner + 进度条为形态，且未要求文字说明，新节与它构成同一加载态的两套完成度要求；两文档互不引用（`interaction.md` 未链接 `loading-experience.md`，后者也未回指）。若只管原型：节标题应显式限定，否则产品实现阶段会误用。
- **依据**：`loading-experience.md:3`「本文件是 SPA 首屏加载/路由切换/加载失败兜底的唯一权威」、`:12-14` 四层加载契约；`interaction.md` 全文无 `loading-experience` 引用（grep 无命中）。
- **建议**：在节首写明适用范围（建议「原型与产品界面同口径，例外以 `loading-experience.md` 为准」），或在「加载」行加一句「SPA 首屏/路由加载以 `loading-experience.md` 为准」，并在两份文档各加一行互指，避免同一 spinner 两处判定。

### R-9 2026-11-15【Low】V6 的「shadow 仅标示 elevation」「同页 radius ≤3 档」缺定义，无判定依据
- **状态**：已修复
- **位置**：`docs/reference/standard/frontend/interaction.md:196`
- **问题**：`elevation` 在 `docs/reference/standard/frontend/` 全目录只出现这一次（无档位表、无定义），评审者无法判断某处 shadow 是否「标示 elevation」；`layout.md` 的「浮层层级契约」是 z-index 数字层级，与视觉 elevation 是两回事，易被混为一谈。「≤3 档」也未给计数口径（按不同数值计？还是按组件类别计？）。
- **依据**：`grep -rn 'elevation|阴影|shadow' docs/reference/standard/frontend/*.md` 仅命中 `interaction.md:196`。
- **建议**：要么由 `theming.md` 给一档最小 elevation 表（如 卡片/浮层/全屏遮罩 三档）并在 V6 引用，要么把 V6 这半句改为可判定的形式（如「SHALL NOT 用阴影表达装饰性层次」）；「档」的计数口径补一句「按本页出现的不取值数计」。

### R-10 2026-11-15【Low】delta 与文档正文措辞漂移，tasks 2.1「字面相同」的验证口径不成立
- **状态**：已修复
- **位置**：delta spec `specs/frontend-interaction-contract/spec.md:6-14` 对 `interaction.md:113-119`；tasks 2.1
- **问题**：delta 写「真实值：来自 `api <path>` / `表.字段` / `派生 <公式>` 的实际数据」，文档正文压成「从 `api` / 表取到的真数」——`派生 <公式>` 这一类在正文消失（派生值算不算真实值？正文读者会自己判断）；delta 写「SHALL 属于**且仅属于**以下两档之一」，正文写「SHALL 属于下列两档之一」；三个 Requirement 在 delta 的排列顺序（占位 → 空态 → 视觉）与正文实际位置（占位 → 空态 → …… → 视觉在文末）不一致。tasks 2.1 声称「与 `interaction.md` 新增的三个标题字面相同」并标已通过，但正文三个标题确实字面相同、节内细则并不逐字一致，验证口径写得过宽。
- **依据**：delta `:8`（`api <path>` / `表.字段` / `派生 <公式>`）对 `interaction.md:117`；delta `:6` 对 `interaction.md:113`；`grep -n '^### Requirement' interaction.md` 显示视觉节在 `:185`（文末）。
- **建议**：正文补回 `派生 <公式>`；口径改为「Requirement 标题与判据数值一致」（不强求逐字相同），并把 tasks 2.1 的验证命令写成可机械执行的形式（`grep -c` 两个文件的 Requirement 标题集合后比对）。

### R-11 2026-11-15【Low】文档行数预算的增量估算与实测不符，220 行上限余量仅 4 行
- **状态**：已修复
- **位置**：design.md §3「文档预算调整」；`interaction.md:10`（预算行）；tasks 4.2
- **问题**：design 称「新增三节（三张判定表）约 45 行 → 调整到 ≤220 行」，但 `wc -l interaction.md` 实测 216 行，且预算自 ≤180 抬到 ≤220（+40）后只剩 4 行余量；按 design 自己的余量算法（45 行增量 → 预算 190）本不该抬到 220。差额说明增量估算没实测过，下一个 change 只要再加一节就必然触顶「必须拆文件」。
- **依据**：`wc -l docs/reference/standard/frontend/interaction.md` → 216；`interaction.md:10` 预算行 ≤220；design.md §3 称约 45 行。
- **建议**：把 §3 的估算改为实测值（如「实测 +N 行 → 预算 ≤220」），或直接把三节合并/外移到独立文件（`prototype-content.md`）以保住 `interaction.md` 的 ≤180 预算，减少与「原型预算」主题在同一文件内的拥挤。

### R-12 2026-11-15【Low】task 6.4「反向检查（防误伤）」的验证口径不可判定
- **状态**：已修复
- **位置**：tasks 6.4
- **问题**：判据写「`git status --porcelain -- front/ backend-go/ .pi/extensions/` 中无本 change 引入的改动 → 期望结果：本 change 只碰 `docs/reference/standard/frontend/`」。该命令在当前工作区实际返回 87 行（`.pi/extensions/ui-design-gate.ts` 等 3 个、若干 `backend-go/`、`front/` 下 50 项），命令本身无法区分「本 change 引入」与「工作区既有脏改」；另外 `docs/reference/standard/frontend/interaction.md` 目前是未跟踪新文件（`??`），意味着本 change 的增量在 git 侧无法与此前未归档的 `add-ui-contract-ladder` 增量分离，归档 diff 会把整个 216 行记作一次提交。
- **依据**：`git status --porcelain -- front/ backend-go/ .pi/extensions/ | wc -l` → 87；`git status --porcelain -- docs/reference/standard/frontend/interaction.md` → `?? …`；`git ls-files docs/reference/standard/frontend/` 未含 `interaction.md`。
- **建议**：把 6.4 改成可判定的口径（比对 change 起始基线 commit：`git diff --stat <base>..HEAD -- front/ backend-go/ .pi/extensions/` 应为空），并在归档前确认 `interaction.md` 的跟踪状态（`git add`），否则本 change 的「只碰 docs」声明无法被 diff 复核。

---

## 核实记录（只读验证）

| 验证动作 | 结果 |
| --- | --- |
| 读 `AGENTS.md` 优先级宪法 | 本次为只读审查，不涉及实现，不触发优先级冲突 |
| `wc -l docs/reference/standard/frontend/interaction.md` | 216（预算 ≤220，R-11 依据） |
| `grep -n '^#### Scenario' interaction.md` / delta spec | 14 条 / 8 条；本次新增部分 7 条，缺「诚实占位被接受」（R-3 依据） |
| `grep -rn '4\.5:1\|prefers-reduced-motion\|outline: *none' docs/reference/ openspec/specs/` | 4.5:1 仅 `interaction.md:191`；`prefers-reduced-motion` 另见 `loading-experience.md:29`、`openspec/specs/spa-loading-ux/spec.md`、`user-onboarding/spec.md`（R-7/R-8 依据） |
| `grep -rn 'elevation\|阴影\|shadow' docs/reference/standard/frontend/*.md` | 仅 `interaction.md:196`（R-9 依据） |
| `grep -rn '无权限' front/app openspec/specs/ docs/reference/` | 产品侧仅 404/403 合并语义（`boardHealthWorkspace.ts:369-391`），无权限态仅出现在本次新增三行（R-8 语境参考，不单独成条） |
| `grep -rnE '>[A-Za-z][A-Za-z ,'"'"'\-\.]{3,}<' front/app --include=*.vue` | 命中 `API Key` / `stale` / `Board Dynamics` / `OpenAI Compatible` 等英文 UI 文案（R-4 依据） |
| `grep -rn '—' front/app --include=*.vue` | `direct_hit — 精确重叠` 等英文标识 + 单 `—`；以及 `—` 占位主流写法（R-4/R-5 依据） |
| `grep -rn 'needs-approval' docs/reference/ openspec/specs/` | `interaction.md:100/103/104`、`openspec/specs/ui-design-workflow/spec.md:32`（R-6 依据） |
| `git status --porcelain -- front/ backend-go/ .pi/extensions/` | 87 行（R-12 依据） |
| `git status --porcelain -- docs/reference/standard/frontend/interaction.md` | `??`（未跟踪，R-12 依据） |
| `bash scripts/harness/review-trace.sh openspec/changes/add-prototype-slop-filter` | 见交付报告（写入本文件后执行，退出码 0） |

> 未执行 build / 测试 / `openspec validate`（本次审查约束为只读，且 brief 明示不跑 build 与测试）。
