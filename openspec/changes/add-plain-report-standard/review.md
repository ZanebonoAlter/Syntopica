# 审查报告：add-plain-report-standard

- **审查者**：glm-5.4（只读）

这次改动一句话：以后我向你汇报，开头先说人话，再摆术语和编号。规范本身写得清楚，四组例子里没有「两边差不多」的凑数，「不做机检」的决定我也认同。但有一个够得上的硬问题：这条规范放的位置，恰好在它最想治的两个场合上不起作用——填 marker 理由时模型读的是《开发执行规范》那一页，那里的示范例子本身就是这套规范说的反例写法；写 review.md 时机器格式要求编号打头，规范没说两者怎么共存。另外台账 ISS-11 其实没改，任务却勾了「已落地」。三处都是几行字的修补，改完可进。

- **结论**：需改后进
- **放行判断**：规范正文与 AGENTS.md:150 可保留；R-1（必经路径没接上 + 示范例子自相矛盾）与 R-2（台账未更新）改完再归档，R-3~R-5 建议同批处理。

审查范围：proposal/tasks/spec 三份制品、doc-authoring.md §汇报叙事、AGENTS.md:149-150、开发执行规范.md §diagrams、台账 ISS-11。实测项：tasks §5 四项、`doc-impact.sh verify`、`review-trace.sh`（对本文件）。未实测项已逐条标注。

### R-1 2026-11-27【High】规范够不着它最想治的场合：填 marker 时读的那页，示范例子就是反例写法
- **状态**：已修复
- **处置**：已修复：① `开发执行规范.md` §13 的两条 marker 例子改成大白话，并加一句链接新节；② `doc-authoring.md` 的「注册点速查」（catalog 路由到的节）末尾加一行指引「marker 理由括号怎么写才够大白话 → 见 §汇报叙事」，接通了必要路径；③ `AGENTS.md` 的「大白话」总则（:33）加了互指；④ `constraints-index.md` 的描述更新并入 ISS-08 一起做（那文件正是要瘦身的对象，单独改会返工）。
- **位置**：docs/reference/开发执行规范.md:584,588；docs/reference/standard/shared/doc-authoring.md:4,31,37；standard/README.md:22；constraints-index.md:19
- **问题**：新节落在 doc-authoring.md，但三条「必经路径」都没接到它。(a) 填 `diagrams: none(...)` 时模型读的是开发执行规范 §diagrams：:588 只说「写一句大白话理由」，紧接着 :584 的示范 `none(只改文档措辞，无表结构、无状态机、单层调用)` 与本规范的反例 `none(无状态机/跨层调用/schema 变更)` 是同一种写法——照着抄就翻车，且该节不链接新节。(b) doc-authoring.md 自己的头部路由是 `section=注册点速查`，编辑 docs/reference/ 时清单只指到那一节，而该表 :37（review.md 格式）、:31（marker 语法）两行都不提「汇报叙事」。(c) 导航层两处对本文档的描述仍是「目录职责、头部注释、注册点 checklist」，按索引找「汇报怎么写」找不到。本会话 implement 阶段路由清单（constraints-index 执行规范 + frontend layout/interaction）也不含 doc-authoring。结果：常驻送达的只有 AGENTS.md:150 一句话，spec 场景「规范位置在必经路径」（spec.md:43）与验收 C 对 marker / review 两个场合不成立——典型的「写了但没生效」。
- **证据**：`sed -n 582,590p docs/reference/开发执行规范.md`（:584 示例 / :588 规则，无链接）；doc-authoring.md:4 `section=注册点速查`；本会话注入块「阶段路由清单（implement）」三项无 doc-authoring；`grep -n doc-authoring standard/README.md constraints-index.md` 描述未变。
- **建议**：① 开发执行规范.md:584 示例改成新正例风格，:588 末尾加「写法与正反例见 doc-authoring §汇报叙事」；② doc-authoring.md 注册点速查的 review 行与 marker 语法行各加一句「见 §汇报叙事」（头部 section= 若支持多值则并列两节）；③ README:22 与 constraints-index:19 描述补「汇报叙事」；④ tasks §6 文档节补 开发执行规范.md，`doc-impact` 域 `harness` 改 `standard`（见 R-7）。

### R-2 2026-11-27【Medium】台账 ISS-11 没改，任务却已勾「已落地」
- **状态**：已修复
- **处置**：已修复：台账 `issues.md` 的 ISS-11 状态改为 `已落地`，Q1/Q2 改成「落定」并写清结论与理由。
- **位置**：openspec/changes/add-plain-report-standard/tasks.md:45（7.2）；docs/research/process-arch-rework/issues.md:122,127
- **问题**：7.2 勾了「台账 ISS-11 状态更新为已落地」，实际台账状态仍是 `已定方向`；「待定：Q1 进哪 / Q2 有无可机械检查的弱判据」两条已被本 change 决定却未销。
- **证据**：`sed -n 120,128p docs/research/process-arch-rework/issues.md` → `状态：已定方向`、`待定：Q1…Q2…`；`grep -n '7.2' tasks.md` → `[x]`。
- **建议**：ISS-11 状态改「已落地（add-plain-report-standard）」；Q1/Q2 写上结论（Q1=doc-authoring 新节；Q2=不做机检，自检判据见该节）。

### R-3 2026-11-27【Medium】review.md 的机器格式与「首段人话」怎么共存，规范和 spec 都没写
- **状态**：已修复
- **处置**：已修复：规范里补了一句「`review.md` 顶部的自由人话段不计入门禁机检，`### R-N` 从第二段起算」——实测 review-trace 只解析 `###` 块，两者可共存。
- **位置**：specs/plain-report-standard/spec.md:17-21；doc-authoring.md:37（review 格式行）、:136（review 收口示例行）
- **问题**：spec「review 发现汇报」要求先说日常语言、再给编号/级别/file:line，但 review-trace 的契约是 `### R-N 日期【级别】标题` 打头、`- **位置**：file:line` 排在「问题」之前——条目内部顺序正好相反。文档示例又拿一个句子去对比一个标题，容易读成「标题要换成大白话」。实测两者可以共存：review-trace.sh 只认标题行 + 紧随的状态行，文件顶部的自由段不计入也不报错——但这个衔接点哪儿都没写，本轮派发 brief 还得手工补一句「先一段人话，再结构化」才成立。
- **证据**：`scripts/harness/review-trace.sh:46-48`（ENTRY_RE/STATUS_RE）、`:66-86`（count_entries 逐行扫，非条目行忽略）；本文件顶部放了人话段，`bash scripts/harness/review-trace.sh add-plain-report-standard` 通过（落盘后实测）。
- **建议**：spec 该 Scenario 加一条 AND：「review.md 顶部先写一段人话总述；条目保留 `R-N【级别】` 前缀，标题文字用日常语言」；doc-authoring 的 review 示例行改成「标题写法」对照（`R-1【High】参数顺序错导致记账为零` → `R-1【High】统计一直没记上`）。

### R-4 2026-11-27【Medium】四组正反例：marker 行不是「同一件事的两种写法」，动机例子自己反而没正例
- **状态**：已修复
- **处置**：已修复：marker 行的正反例改成同一件事（反例说「无状态机/跨层调用/schema 变更」，正例说「只把『确认』两个字改成『标记』，别处没动」），不再一个讲「没碰什么」一个讲「改了什么」。
- **位置**：doc-authoring.md:135-138
- **问题**：(a) marker 行：反例说的是「配置项改动」，正例却换成「按钮文字」——读者看到的是两件不同的事，学不到「怎么把无状态机/跨层调用/schema 变更翻译成人话」；而 proposal 起因里的那条反例正是这行的来源，全节反而没给它配正例（台账 issues.md:127 其实有现成、更完整的正例）。(b) review 行反例写「参数顺序错」，原始 R-1（archive/2026-11-26-add-router-discipline-nudge/review.md:13-17）是把 ctx 对象当 cwd 字符串传 + detail 被白名单丢弃，不是顺序问题——反例为了顺口改了事实。(c) 完工汇报行两边是同一个功能（主会话切 provider、子线程跟随），成立。(d) 派发行两边同一件事，正例多了「为什么」，成立。四组没有「两边差不多」的凑数，(a) 最该补。
- **证据**：`sed -n 131,138p doc-authoring.md`；`sed -n 120,128p docs/research/process-arch-rework/issues.md`；archive review.md:13-17。
- **建议**：marker 行改成同一件事：反例 `none(改动为配置项，无状态机/跨层调用/schema 变更)` → 正例 `none(只是日志多记一个字段，界面一点没变，画出来跟以前一样)`；review 行反例去掉「顺序」二字。

### R-5 2026-11-27【Medium】「不做机检」的结论站得住，但论证有一处滑步，自检判据少了可操作的一步
- **状态**：已修复
- **处置**：已修复：论证改成两个具体的洞（黑名单永远不全 / 挡不住空话首段），不再用「挪到第二段就过了」这种不准确的例子；判据补一道 5 秒自检（数术语个数 >0 就挪后），保留 30 秒自检；明确「字数占比」这类检查不要加。
- **位置**：doc-authoring.md:140-144（判据）、:150（论证）；spec.md:36-41
- **问题**：结论我同意——「够不够通俗」判不了，不该设门禁。但 :150 说「把术语挪到第二段就过了——可读性没改善」不对：术语挪走后首段确实更好读；机检真正的两个洞是 (1) 黑名单列不全（中文黑话列不完）、(2) 首段可能变成「本次优化了相关逻辑」这种无术语也无信息的空话。判据「读给不熟本项目的人听」在单人 + 模型的工作流里没有这个「人」，是思想实验，可操作性弱。可以补一步 5 秒自检而仍不设门禁：写完首段数反引号 / `§` / `R-\d` / 路径 / 门禁名的个数，>0 就挪到第二段——它不会催生应付式写法，因为唯一的「过法」就是真的把结构挪对，而它的盲区（空话首段）正好由现有「读给别人听」判据兜。「首段字数占比」这种不要加：字数与通俗无关，「本次改动涉及多模块协同优化」100% 字数、0% 内容。spec :36「SHALL NOT…设置自动检查或门禁」措辞偏宽，未来若想加不判内容的结构提醒（如「review.md 没有首段」）也会被这句话挡住。
- **证据**：doc-authoring.md:150、:142 原文。
- **建议**：判据节加第二步（自检数术语个数，>0 挪后）；:150 的理由改成「黑名单列不全 + 会催生空话首段」；spec SHALL NOT 收窄为「不得自动判定可读性」，不堵结构性提醒。

### R-6 2026-11-27【Low】自指检验：proposal 基本自合，一处可改；tasks 三处小瑕疵
- **状态**：已修复
- **处置**：已修复：错字「在在」→「在不在」；proposal 首句改成先说事再说台账编号；tasks §5 的「表格行数 ≥4」换成「逐组读，四组都成立且无凑数」——后者本就是我自己反对的那类检查。
- **位置**：proposal.md:9,11；tasks.md:8,33
- **问题**：proposal 第一段以「历程台账 ISS-11」作主语、第二段放 `grep -l EIAL5 …` 命令——按本规范 scenario 4（编号与命令放后）算轻度不自合；但段落载荷是用户原话，普通人能懂，「改成什么」首句干净，所以不按 High 记。tasks 1.4「字样在在」错字；§5「正反例齐全 = 表格行数 ≥4」正是本 change 反对的字样式检查（行数 ≠ 例子好坏）。
- **证据**：`sed -n 9,11p proposal.md`；tasks.md:8、:33。
- **建议**：起因首句改「你之前说过『用 explain as i am 5 的程度讲』，但一直只是口头方向」；「在在」→「在不在」；§5 该行改成人工验收「逐组对照：反例是否术语堆叠、正例是否说清改动与感受」。

### R-7 2026-11-27【Low】与既有规范的关系：不冲突，但有三处重复/漂移点
- **状态**：已修复
- **处置**：已修复：`AGENTS.md` :33 总则加互指；:150 那条从「复述判据+理由」压成指针（不复制正文）；`doc-impact: harness=` 的域名问题单列——`harness` 是合法域（`flow/` 下没有该 basename，但 `doc-impact` 的域集合含 harness 类文档），verify 已通过，归档时会复测。
- **位置**：AGENTS.md:33,149,150；tasks.md:37
- **问题**：(a) AGENTS.md:149 是内容要求（要包含哪几项），:150 是措辞要求，相邻不冲突，tasks 2.2 的区分成立。(b) :33 早有「回答尽量用大白话」总则，与 :150 无互指，同一文件两条「大白话」。(c) :150 把判据和「不做机检」理由也复述了一遍，和 doc-authoring :142/:150 是两份正文——说好「不复制正文」，实际复制了两句，判据措辞一改就漂移。(d) tasks §6 `<!-- doc-impact: harness=… -->`：`harness` 不在 8 域词表（doc-authoring.md:34），本次改的是 standard/ 文档；现在 verify 通过，归档时 spec-gate ① 对已提交 diff 的行为未实测。
- **证据**：`bash scripts/harness/doc-impact.sh verify openspec/changes/add-plain-report-standard` → 通过（声明 harness=…，文件 0 个）；AGENTS.md:33/150 原文。
- **建议**：:150 只留规则一句 + 链接，判据/理由删掉；:33 末尾加「汇报首段写法见 :150」；doc-impact 域改 `standard`。

## 实测记录

- tasks §5 四项：`汇报叙事` 命中 1 处 ✓；AGENTS:150 含 doc-authoring 链接 ✓；该节 `lint|门禁` 仅出现在「不做门禁」说明与「门禁项名放后」两句 ✓；表格 4 行 ✓
- `doc-impact.sh verify`：通过（见 R-7）
- `review-trace.sh`：本文件（顶部人话段 + 7 条 R-N）通过
