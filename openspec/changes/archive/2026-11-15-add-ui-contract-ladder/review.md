# Review — add-ui-contract-ladder

### R-1 2026-11-15【High】delta 的「八节+三节」与画布原型形态未同步 schema/模板/§3.1 → 新契约在制品生成路径上不可执行
- **状态**：已修复
- **位置**：`openspec/changes/add-ui-contract-ladder/specs/ui-design-workflow/spec.md`（「ui-design 作为必经规划制品」三节段 + 「major UI 原型必须获得用户确认」原型形态段）；`openspec/schemas/syntopica-ui/schema.yaml:103-104,118-121`；`openspec/schemas/syntopica-ui/templates/ui-design.md:3,11-12`；`docs/reference/开发执行规范.md:206`
- **问题**：delta 把 `ui-design.md` 的必备结构改成「八节 + 三节」并规定画布类原型形态，但机器生成路径（`openspec/config.yaml` 默认 `schema: syntopica-ui`）的指令与模板仍只写「八节 + ui-prototype/ 静态 HTML 原型」，新契约在 agent 真正产出 `ui-design.md` 的那一步没有任何落点。
- **依据**：schema.yaml:104「必填八节（与模板同序）：User Journey / Information Architecture / …」；:118-121「Prototype：在 change 目录 `ui-prototype/` 产出静态可丢弃原型（独立 HTML/静态资源…）」且 marker「指向 change 根内相对路径（如 `ui-prototype/index.html`）」；templates/ui-design.md:3 默认 marker `<!-- ui-prototype: ui-prototype/index.html -->`、:11「major → 完整八节 + ui-prototype/ 静态原型」；开发执行规范.md:206「`ui-design.md` 八节合同 + `ui-prototype/` 静态原型」。而 delta 要求 `ui-design.md` MUST 另含 `## Interaction Decisions` / `## Data Provenance` / `## 明确不做`，且 canvas 主模式 MUST 用 `states/` 三态图 + `interaction-table.md`。四处对同一制品给出互斥的结构清单 → 照 schema 生成的 `ui-design.md` 必然缺三节，且对画布类必产 HTML 原型；`## 明确不做` 除 delta 外全仓无任何格式/判据来源（grep 仅命中 delta 与 开发执行规范 §3.1.1 的节名）。
- **建议**：把 schema + 模板 + 开发执行规范 §3.1 表纳入本 change 的 tasks：① schema.yaml major 段改为「八节 + 三节结构化声明（`## Interaction Decisions` / `## Data Provenance` / `## 明确不做`，三节为 `ui-design.md` 内小节）」，Prototype 段改为「canvas 主模式 → `states/` 三态图 + `interaction-table.md`；其余模式 → 可点 HTML（marker 指向 `ui-prototype/index.html`）」；② templates/ui-design.md 补三节骨架 + 两个 Prototype 变体 marker；③ 开发执行规范 §3.1 表「八节合同」改为「八节 + 三节结构化声明」；④ 补 `## 明确不做` 的格式与判据来源（每条须能读成「不出现在本界面里的具体东西」，禁「暂不支持」措辞）。

### R-2 2026-11-15【Medium】画布形态下「原型展示 / 差异说明」的措辞未同步（归档门禁本身不受影响）
- **状态**：已修复
- **位置**：`docs/reference/开发执行规范.md:303-333`（§5.3 major 双层验收）；`openspec/schemas/syntopica-ui/schema.yaml:124`（审批流「用浏览器向用户展示原型」）；`openspec/specs/ui-design-workflow/spec.md`（主 spec 基线「UI 完成验收必须对照批准基线」，本 change 未 MODIFIED）
- **问题**：画布类批准基线从「可点 HTML 原型」换成「状态图 + 操作表」后，「用浏览器展示原型」「实现与已批准原型的差异说明」两步在规范层没有对应动作，而 §5.3 与 schema 措辞一字未改。
- **依据**：schema.yaml:124「agent 用浏览器向用户展示原型」；§5.3「③ 实现与已批准原型的差异说明」；主 spec「UI 完成验收必须对照批准基线」同样以原型为比对对象之一（`ui-design.md` 仍在，故比对基准并未消失，但「原型」这一半的形态变了）。**已核实**：`scripts/harness/ui-verification-trace.sh` ③ major 档只校验 `ui-verification/` 目录 + `report.md` 合法判定条目 + 条目点名截图真实存在 + 未覆盖条目带原因，与原型是否可点无关 → 归档门禁**不会**因此变得无法满足（brief 的该假设不成立）；真正的缺口是展示/比对动作无措辞，执行者会在「没有 HTML 拿什么展示、拿什么比对」处卡住。
- **建议**：在 delta 或 §5.3 补降级口径：「canvas 主模式以 `states/` 三态图 + `interaction-table.md` 作为批准基线；审批展示方式为逐张状态图 + 操作表走查（无浏览器可交互形态）；差异说明的比对基准为三态图逐态对账 + `interaction-table.md` 六列逐行对账」，并把 §5.3 ③ 与 schema 审批流的「已批准原型」措辞放宽为「已批准原型或（canvas 类）状态图 + 操作表」。

### R-3 2026-11-15【Medium】四条机械判据中三条缺计数口径与核对对象 → 拿规范无法不靠猜地判定 pass/fail
- **状态**：已修复
- **位置**：`docs/reference/standard/frontend/interaction.md:107-121`（原型预算表）、`:71-86`（决策台账）、`:88-105`（数据来源）；`openspec/changes/add-ui-contract-ladder/specs/frontend-interaction-contract/spec.md`（同名 Requirement）
- **问题**：文字预算、`forbidden` 出现、provenance 完整性这三条判据都缺可机械执行的计数口径与核对对象，review 只能主观判断——正是 D6「本次只落规范文本、由 change-review 人工核」的前提条件未被满足。
- **依据**：(1) 文字预算 interaction.md:114「≤12 个短语/屏；禁止 >40 字完整句（空态、错误、二次确认除外）」——「短语」未定义（几个字、按钮/标签/标题各计几个），「屏」在 canvas 形态下未定义（`states/` 三张状态图算 1 屏还是 3 屏），例外「空态、错误、二次确认」未给边界；且 :52「界面文字只承担三类职责」管产品界面、:114 管原型可见文字，两套纪律的适用对象未在文中点明。(2) forbidden：:81 只说「原型或实现里出现即视为偏离，须在 review 中说明或回退」，但原型是 `states/` + `interaction-table.md` 时「出现 dialog 模式」指什么没有核对对象；forbidden 列的取值域（是否必须取 7 模式词表）未定义。(3) provenance：:90「每个用户可见数据项一行」没有枚举方法（界面每个标签/数字是否各算一项）；:99 `source` 取 `api <path>` 未要求端点真实存在 → 该列可填不可核，形式化风险最高的一条恰是 D4 想解决的血缘问题。真正可核的只剩两条：原型字段 ⊆ provenance 表（:104）、原型目录 md 份数/行数。
- **建议**：给三条判据各补一行可执行口径，例如：「短语」= 独立可见文本节点（按钮/标签/徽标/标题各计 1），长度按字符数计（标点计入）；「屏」= 一张状态图或一个 HTML 视图；forbidden 核对对象固定为「`ui-design.md` 台账 + `interaction-table.md` 的 触发/结果 列 + 原型可见文本」，其中出现弹窗式交互即命中，forbidden 取值 MUST 取自 7 模式词表；provenance 的 `api <path>` MUST 能在 `front/app/api/**` 或后端路由表 grep 到，否则只能填 `none` + `needs-approval`。

### R-4 2026-11-15【Medium】原型目录「md ≤1 份」与画布形态必需的 `interaction-table.md` 冲突，决策表还有两个落脚点
- **状态**：已修复
- **位置**：`docs/reference/standard/frontend/interaction.md:113`（预算表）、`:123-131`（画布形态节）；`openspec/changes/add-ui-contract-ladder/specs/frontend-interaction-contract/spec.md`（「原型目录受预算约束」/「画布类交互的原型形态」）；`openspec/changes/add-ui-contract-ladder/tasks.md:31`（存量合并目标 `node-decisions.md`）
- **问题**：画布主模式的原型 MUST 产出 `interaction-table.md`（一份 md），而预算表规定 `ui-prototype/` 设计说明类 md ≤1 份且括注为「决策表」；若决策表同时落在 `ui-prototype/`（tasks 5.2 的合并目标 `node-decisions.md` 就在该目录内），画布类原型必然 2 份 md → 违反自己的预算；若决策表只在 `ui-design.md`（delta 要求三节），预算表的括注「（决策表）」又无处安放。两种读法都让「原型体积」判据在画布场景无法判定 pass/fail。
- **依据**：interaction.md:113「设计说明类 md | ≤1 份（决策表），≤300 行；禁止按模块散装的 `node-spec.md` / `parts-table.md` / `*-verification.md` 堆积」；:128「`interaction-table.md`：`操作` / `触发` / `前置` / `结果` / `反馈` / `失败`，每操作一行」；delta spec 同口径两处；tasks.md:31 合并目标路径 `openspec/changes/add-home-pipeline-status-page/ui-prototype/node-decisions.md`。
- **建议**：明确「决策表唯一落脚点 = `ui-design.md` 的 `## Interaction Decisions` 小节」；`ui-prototype/` 内 md 上限重写为「canvas 主模式 ≤2 份（`interaction-table.md` + 可选 `states/README.md`），其余模式 ≤1 份；每份 ≤300 行」，并把 delta spec 预算条目同步为同一口径。

### R-5 2026-11-15【Medium】`pinned` 只禁「替换/简化/合并」，仍留出被解释成「顺手优化」的漏洞
- **状态**：已修复
- **位置**：`docs/reference/standard/frontend/interaction.md:80-86`；`openspec/changes/add-ui-contract-ladder/specs/ui-design-workflow/spec.md`（「pinned 决策被实现阶段改写」）
- **问题**：现措辞与唯一反例只堵住「换模式并同步改台账」这一种形态，其余偏离路径未封堵；「停下并回问用户」既没有最小形式也没有阻塞性。
- **依据**：interaction.md:80「实现与原型 **MUST NOT 替换、简化、合并**，也 MUST NOT 因为『弹窗更好实现』而改写台账；认为必须偏离时 MUST 停下并回问用户」；反例 :84-86 仅覆盖「把 pinned 的 canvas 改成 dialog 并同步改了台账」。未封堵：(a) 把一条 pinned 拆成「pinned(部分) + open(其余)」；(b) 保留模式但改写实现方式（拖拽改成点开侧栏、调触发时序/位置）——「替换/简化/合并」不覆盖「同一决策的交互形态改写」；(c) 放宽该行 `forbidden` 列（`dialog` 改成 `drawer`）既不算「改写该决策」也未被点名；(d) 「回问」未写最小形式与阻塞性（未写「须附偏离理由 + 用户裁决写回台账前不得继续实现」），执行者可以「回问过」之后自行推进；(e) 未写谁可合法改 pinned（用户改主意时的合法更新路径缺失）。design.md 风险表已把「`pinned` 被 agent 解释成『可优化』」列为已知风险，但规范文本只给了一句反例，不足以闭合。
- **建议**：pinned 条目补三条禁令 —— 「SHALL NOT 拆分或降级为 open」「SHALL NOT 在不改模式的前提下改写该决策的交互形态（触发方式/时序/位置）」「SHALL NOT 放宽该行 forbidden 列」；补唯一合法变更路径 —— 「仅用户在对话中明确改口后，由 agent 写回台账并追加『变更来源：用户 <日期>』，否则视为违反契约」；补回问最小形式 —— 「回问 MUST 附偏离理由与两种候选，且在用户答复并写回台账前 MUST NOT 继续实现」。

### R-6 2026-11-15【Medium】tasks.md 已勾选项的自带验证命令与实际产出/仓库现状不符
- **状态**：已修复
- **位置**：`openspec/changes/add-ui-contract-ladder/tasks.md:14`（2.1）、`:50`（8.1）、`:58`（9.1）、`:62`（9.5）
- **问题**：四条已勾选项的验证命令按现状实测不成立，违反《开发执行规范》§11.1 条件 3「tasks.md『验证』节每条命令实测零失败」。
- **依据**：① 2.1 验证为 `grep -n '交互模式选择判据' docs/reference/standard/frontend/layout.md`，实测无命中——layout.md:41 实际节标题是「交互模式必须在布局定稿前显式选择」，命令必红；② 8.1 描述「在 `layout.md` 顶部『相关契约』处登记」，实测 layout.md 无该节，实际登记是散在 :45/:52/:63/:83 的交叉引用；③ 9.1 `ls` 4 个文件 + 两份 delta spec = 6 个文件，期望结果却写「7 个文件齐全」，计数错；④ 9.5 `git diff --stat front/ backend-go/` 期望「空」，实测当前工作树 front/ 有 26 个文件改动（其他 change 并行脏树；本 change 自身 front/ 零 diff）。
- **建议**：2.1 的 grep 关键词改为实际标题（或把节标题改回「交互模式选择判据」以对齐任务书，二选一）；8.1 描述改为「在 layout.md 的 Requirements 三处交叉引用 + 开发执行规范 §3.1 登记」；9.1 期望改为「6 个文件齐全」；9.5 改为限定本 change 路径（如 `git diff --stat -- docs/reference/standard/frontend/ docs/reference/开发执行规范.md`），期望结果写成「本 change 未改产品代码」。

### R-7 2026-11-15【Medium】新文档未按 doc-authoring checklist 在 `standard/README.md` 登记，导航层不可发现
- **状态**：已修复
- **位置**：`docs/reference/standard/README.md`（「这层装什么」表 + 「目录结构」树）；`docs/reference/constraints-index.md`（执行规范表）；`docs/reference/standard/shared/doc-authoring.md`（§checklist「新增 standard 文档」第 3/4 步）
- **问题**：`interaction.md` 自称「唯一权威」，但标准层索引既未加行也未进目录树；`check-standards` D 段「防孤立引用」目前只是靠《开发执行规范》:208 的一次引用侥幸通过，不是制度化登记。
- **依据**：doc-authoring.md §checklist「新增 standard 文档」第 3 步「`standard/README.md`「这层装什么」表加行，且文件名被 开发执行规范/AGENTS.md/子目录 AGENTS 之一引用」、第 4 步「`constraints-index.md` 执行规范表登记（导航可见性）」；实测 standard/README.md 的交互相关行只有 `frontend/interaction-conventions.md`（:13 与目录树 :32），无 `interaction.md`；constraints-index.md 执行规范表只有目录级 `standard/frontend/` 与单列的 `standard/frontend/layout.md`，无 interaction.md——tasks 8.4 明确判定「不进常驻索引」，理由「经 `layout.md` 与 JIT 标签可达」，但该索引表是按主题单列登记的，目录级一行不等于新主题在注入层可达。
- **建议**：standard/README.md「这层装什么」加一行「交互模式选择 / 界面文字纪律 / 原型预算 → `frontend/interaction.md`」，目录树加 `interaction.md` 一行；constraints-index.md 执行规范表在 layout.md 行旁加 `standard/frontend/interaction.md` 一行；若坚持不进索引，需在 doc-authoring.md 记录该例外并写清理由（否则下次按 checklist 复查会重复判为漏项）。

### R-8 2026-11-15【Low】存量瘦身任务留在本 change 的 tasks 内，与 proposal 自述矛盾
- **状态**：已修复
- **位置**：`openspec/changes/add-ui-contract-ladder/tasks.md:25-33`（§5，5.2/5.3/5.4 未勾）；`openspec/changes/add-ui-contract-ladder/proposal.md`（「存量处理（与本 change 并行，不在本 change 的 tasks 内）」）
- **问题**：proposal 明写存量瘦身「不在本 change 的 tasks 内」，tasks.md 却把合并 61 份 md、删除冗余、记 FB-3 列为本 change 的待办且仍未勾选——归档时 §11.1 要求任务全绿，不勾则本 change 无法归档，勾掉则是另一 change 的成果在本 change 留痕。
- **依据**：tasks.md:31-33 三条未勾任务的验证分别指向 `add-home-pipeline-status-page/ui-prototype/` 与其 `human-feedback.md`；节标题虽写了「非本 change 规范产物，仅登记执行留痕」，但仍以任务项形态留在同一份 tasks 里。
- **建议**：把 5.2–5.4 移出本 change（挂到 `add-home-pipeline-status-page` 的 tasks，或另立 change），本 change 只保留一条指向该 change 的留痕指针；`~/backups/ui-prototype-add-home-20261115.tar.gz` 这一不可逆备份路径也一并随任务迁移。

### R-9 2026-11-15【Low】`interaction.md` 的 MUST 未按 none/minor/major 分档，与 delta spec 冲突
- **状态**：已修复
- **位置**：`docs/reference/standard/frontend/interaction.md:73`（台账 MUST）、`:90`（provenance MUST）；`openspec/changes/add-ui-contract-ladder/specs/ui-design-workflow/spec.md`（「none 产生最小制品」「minor 复用既有交互」）
- **问题**：delta 规定 none 档「三节结构化声明 SHALL NOT 被要求」、minor 档只需两节；interaction.md 却写「`ui-design.md` MUST 含 `## Interaction Decisions` 表」「MUST 含 `## Data Provenance` 表」且无分档豁免 → none 档 change 按 interaction.md 判定即不合规，与 delta 的 none 档条款直接冲突。
- **依据**：interaction.md:73「`ui-design.md` MUST 含 `## Interaction Decisions` 表」；:90「`ui-design.md` MUST 含 `## Data Provenance` 表，每个用户可见数据项一行」；delta spec「none 产生最小制品…三节结构化声明 SHALL NOT 被要求」「minor…MUST 补齐 `## Interaction Decisions` 与 `## Data Provenance` 两节」。
- **建议**：两处 MUST 后补「（none 档豁免；minor 档只需 `## Interaction Decisions` 与 `## Data Provenance` 两节）」，使规范文本与 delta 的分档口径逐档对齐。

### R-10 2026-11-15【Low】与同目录既有 `interaction-conventions.md` 的权威域未划界
- **状态**：已修复
- **位置**：`docs/reference/standard/frontend/interaction.md:7-8`（权威源声明）；`docs/reference/standard/frontend/interaction-conventions.md`（首部权威声明 + §1/§2/§3/§5）
- **问题**：同目录两个文件名前缀相同、都自称「唯一权威」且互不引用，而两者都规定界面文字/状态展示类规则，读者与 agent 无法判断冲突时听谁的。
- **依据**：interaction-conventions.md 首部「本文件是前端『交互友好性 / 可观测性展示分层』约定的唯一权威」，其 §1（状态标记位置）、§2（状态说明行不得伪装成动作）、§3（正文层不出现任何数字）、§5（数值/日期呈现分层）同样管界面文字；interaction.md:7「本文件是『交互模式怎么选、界面文字放什么、原型该长什么样』的唯一权威」，其 §界面文字三职责允许「状态结论（徽标、空态、错误、计数）」含数字——两处的层级/职责边界（正文层 vs 探究层）没有任一文件点明（已核实不构成硬矛盾，但读者需靠推断）。
- **建议**：interaction.md 头部权威源行补一句「状态标记位置、可观测性展示分层与数值呈现格式见 `interaction-conventions.md`；本文件管模式选择、界面文字职责边界与原型形态」，并在 interaction-conventions.md 首部加反向深链。

### R-11 2026-11-15【Low】`doc-impact-applies` 辖区未覆盖真实前端目录中的 UI 决策点
- **状态**：已修复
- **位置**：`docs/reference/standard/frontend/interaction.md:3-5`
- **问题**：辖区前缀只覆盖 `front/app/features/`、`front/app/pages/`、`front/app/components/ui/`、`front/app/components/dialog/`，漏掉真实存在、且正落在本规范辖区的其他目录 → 编辑这些文件时本规范不会被 JIT 注入。
- **依据**：抽查 `front/app/components/ui/`（AppDialog/AppDrawer/AppPageShell ✓ 覆盖）、`front/app/components/dialog/`（TopicManageDialog.vue 等 + `*ConfigPanel.vue` ✓ 覆盖）、`front/app/features/`（tags/discovery/ai 等 ✓ 覆盖）；未覆盖：`front/app/components/common/`（AppTooltip.vue、NotifyContainer.vue——tooltip/浮层文字与层级正属本规范辖区）、`front/app/composables/useConfirm.ts`（「二次确认」弹窗 = dialog 判据与「必要确认与反馈」职责）、`front/app/components/{feed,layout,article}/`。layout.md 有同样缺口（既有惯例），但新文件自称「唯一权威」，辖区宜更全。
- **建议**：标签补 `front/app/composables/, front/app/components/common/`（至少这两处），并与 layout.md 同步补齐以免两份规范的注入覆盖面长期不一致。
