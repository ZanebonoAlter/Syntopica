# 审查报告：add-diagram-and-pseudocode-layer

- **审查者**：MiniMax M3.1 Flash（只读）
- **结论**：需改后进
- **放行判断**：最大风险不是设计本身，而是「门禁没接上真正的归档阻断点」——`diagrams-trace.sh` 只挂在自查脚本 `archive-readiness.sh` 上，`.pi/extensions/spec-gate.ts` 从不调用它，故 `openspec archive` 不会被缺 marker 阻断，本 change 的核心承诺（「不设门禁 → 实际没人画」改为有门禁）在生效点上并未成立。

## 发现

### R-1 2026-11-26【High】门禁未接入真正的归档阻断点，spec-gate.ts 从不调用 diagrams-trace.sh
- **状态**：已修复
- **处置**：**接进真正的阻断点**：`.pi/extensions/spec-gate.ts` 在约束读取对账后插入检查 ⑧（`diagrams-trace.sh`，block 级，`failedChecks.push("diagrams")`），spawn 失败沿既有 fail-open 语义；注释与文案「六项」统一改为「八项」。`archive-readiness.sh` 与 spec-gate 恢复「同口径」不变量。tsc 语法级检查无错。
- **位置**：`scripts/harness/archive-readiness.sh:101`；`.pi/extensions/spec-gate.ts:226,232,238,247,270,296,457`；`openspec/specs/archive-readiness/spec.md:12`
- **问题**：change 的前提是「现状不设门禁 → 实际没人画」，改为「查形式门禁」。但 `diagrams-trace.sh` 只被加进了 `archive-readiness.sh`（自查脚本），而真正拦截 `openspec archive` 的是 pi 扩展 `spec-gate.ts`——它的 7 处 `runScript` 目标是 doc-impact / check-standards / check-tasks-tail / review-trace / constraint-read-trace / concurrency-status / ui-verification-trace，**没有 diagrams-trace**。spec-gate.ts 全文只在 604 行的提示文案里提到 `archive-readiness.sh`，并不执行它。故缺 marker 的 change 归档时不会被 block。
- **证据**：`.pi/extensions/spec-gate.ts:226` 起逐项列出归档 block 判定脚本，无 `diagrams-trace.sh`；`.pi/extensions/spec-gate.ts:604` 仅提示「先跑 archive-readiness.sh 自查」；`scripts/harness/archive-readiness.sh:101` 是新项的唯一调用点。同时 `openspec/specs/archive-readiness/spec.md:12` 声明该脚本执行「与 spec-gate 归档门禁完全相同口径的适用检查」——新增 ⑧ 恰好打破这条「同口径」不变量。
- **建议**：二选一并写进 design D（当前 design D6 只写「门禁挂在 archive-readiness 的第 8 项」，见 `design.md:62`）：① 把 `diagrams-trace.sh` 加进 `spec-gate.ts` 的 block 检查清单（并同步改 `spec-gate.ts:6-7` 的「六项」注释、`openspec/specs/archive-readiness/spec.md` 的项数与「同口径」表述）；② 或明确降级为「自查软提醒」，把 spec delta 的「归档门禁 SHALL 检查三件」（`specs/change-diagram-convention/spec.md:102-108`）改成「归档就绪自查 SHALL 检查三件」，别把软提醒写成硬门禁。

### R-2 2026-11-26【High】主 spec 基线 Requirement 1 未被 MODIFIED，归档后同一份 spec 自相矛盾
- **状态**：已修复
- **处置**：属**时序误报**：该 MODIFIED 在 review 派出后已补（`specs/change-diagram-convention/spec.md` 现含「图表达制品的固定位置与性质」条，正文改为「marker 无条件声明 / 该制品参与归档门禁的 marker 对账」，两个 Scenario 的 WHEN 去掉「且决定用图表达」前置）。review 读到的是补之前的状态。
- **位置**：`openspec/specs/change-diagram-convention/spec.md:11,13,17`；`openspec/changes/add-diagram-and-pseudocode-layer/specs/change-diagram-convention/spec.md:3,25,100-108`
- **问题**：delta 的 MODIFIED 段只列了「三类图的适用判据」「图块的最小可读约束」两条，基线第三条 Requirement「图表达制品的固定位置与性质」原样保留。该条正文写着「**也不阻断 apply / archive 门禁**」、开头写着「change **需要图表达时** SHALL 在 change 根创建」、其 Scenario 写着「…**且决定用图表达**」。归档 sync 后 main spec 会同时出现「不阻断 archive 门禁」与新加的「归档门禁 SHALL 检查三件 / marker 缺失 SHALL 使归档门禁判红」——同一 spec 内互相打架，且 `openspec validate --specs` 只查结构不查语义，验不出来。
- **证据**：`openspec/specs/change-diagram-convention/spec.md:13`「…也不阻断 apply / archive 门禁」；`:17`「一个 change 涉及多步路径、状态机或跨层调用，**且决定用图表达**」；delta `specs/change-diagram-convention/spec.md:3` 与 `:25` 两条 MODIFIED 标题均不含「图表达制品的固定位置与性质」；`:100-108` 新增「归档门禁 SHALL 检查三件」及「marker 缺失…SHALL 使归档门禁判红」。
- **建议**：delta 补一条 MODIFIED Requirement「图表达制品的固定位置与性质」，把正文改为「change **无条件** SHALL 在 `proposal.md` 头部声明 `diagrams` marker；`full` 档 SHALL 存在 `design-diagram.md`…该制品**参与**归档门禁的 marker 对账」；其两个 Scenario 的 WHEN 去掉「且决定用图表达」这一前置。

### R-3 2026-11-26【High】legacy 判据用「有 .openspec.yaml」而非创建时间，三个在途 change 归档即撞红
- **状态**：已修复
- **处置**：legacy 判据改**创建时间**：读 `.openspec.yaml` 的 `created`，早于规则生效日 `2026-11-26` 则不追溯。三个在途 change（11-04 / 11-06 / 11-13）实测全部走豁免，不再撞红；本 change（11-26）仍受检。
- **位置**：`scripts/harness/diagrams-trace.sh:24-30`；`openspec/changes/add-server-capability-negotiation/.openspec.yaml:2`；`openspec/changes/description-repair-batch-matching/.openspec.yaml:2`；`openspec/changes/topic-lane-auxiliary-label-constraint/.openspec.yaml:2`；`proposal.md:73`
- **问题**：脚本把「有 `.openspec.yaml`」当「新 change」的代理，于是三个**早于本 change 诞生**的在途 change 全部被判红（实测三者均输出「✗ 缺 diagrams marker」）。proposal 承诺的是「只有**新** change 要求 marker」，实现给的却是「凡有 `.openspec.yaml` 者都要 marker」，两者不等价。更不一致的是同目录的 `add-home-pipeline-status-page` 尚无 `.openspec.yaml`，被判成 legacy 放行——同为在途 change，分类取决于一个会随时补齐的文件，而非「是否已归档」或「创建于何时」。
- **证据**：`scripts/harness/diagrams-trace.sh:27-30` 仅按 `.openspec.yaml` 存在与否豁免；三个在途 change 的 `.openspec.yaml` 分别写 `created: 2026-11-04` / `2026-11-06` / `2026-11-13`；`proposal.md:73`「只有新 change 要求 marker」；实测 `diagrams-trace.sh` 对三者退出码均为 1。
- **建议**：豁免判据换成 `.openspec.yaml` 的 `created` 字段——`created` 早于本规则生效日（建议取本 change 的 `2026-11-26`）即视为存量豁免，`created` 晚于它才要求 marker；这比「按目录是否在 archive/ 下」更准（后者把在途 change 一刀切）。同时在 §13 写一行存量补法（「存量 change 归档前补一行 marker 或按 created 豁免」），当前 change 制品与 §13 里**没有任何**补法说明。

### R-4 2026-11-26【Medium】archive-readiness.smoke.sh 被打红（4/16），新增项未进 stub 目录
- **状态**：已修复
- **处置**：`archive-readiness.smoke.sh` 补 `diagrams-trace.sh` stub（①、②、⑩ 三处 fixture），① 的断言由 6×✓ 改为 7×✓ 并点名 `✓ ⑧ diagrams marker`。实测 **16/16 全绿**。
- **位置**：`scripts/harness/archive-readiness.smoke.sh:60,67-72,109`；`scripts/harness/archive-readiness.sh:101`
- **问题**：smoke 用 `mkstub` 造桩脚本目录，桩列表里没有 `diagrams-trace.sh`，新加的 ⑧ 项于是以 `exit 127`（No such file or directory）计红，把整条聚合判红。实测当前 HEAD `bash scripts/harness/archive-readiness.smoke.sh` → 退出码 1、4/16 断言失败（① stub 全绿 / ⑤c 前缀容忍 / ⑥ 反馈对账不阻断 / ⑩b legacy 豁免）；同一脚本在改动前的 da343417 退出码 0（已实跑对比确认基线为绿）。即本 change 引入了一个真实回归。
- **证据**：`scripts/harness/archive-readiness.smoke.sh:60`（`for s in doc-impact.sh check-standards.sh check-tasks-tail.sh review-trace.sh ui-verification-trace.sh constraint-read-trace.sh feedback.sh`）、`:67-72`、`:109` 三处桩清单均无 `diagrams-trace.sh`；实跑输出 `✗ ⑧ diagrams marker 与图/理由一致性（exit 127）`。
- **建议**：三处桩清单补 `diagrams-trace.sh`（并按「⑦ 是提醒项、⑧ 是硬检」的语义调整断言里的项数与顺序，见 R-11）；另建 `scripts/harness/diagrams-trace.smoke.sh` 固化 tasks.md §8 那张负例表（marker 缺失 / none 空理由 / none 有理由 / full 无图 / full 有文件无块 / full 合规 / legacy），否则这条门禁的负例只活在 tasks.md 表格里，下次改脚本无人守。

### R-5 2026-11-26【Medium】spec 的「三查」与实现不一致：full 档也被强制要求非空理由
- **状态**：已修复
- **处置**：统一为**两档都要求理由**（实现本来如此，改 spec 向实现看齐）：`full` 的理由用来说明「这个 change 的哪部分需要图」，`none` 说明「为什么不需要」。spec 的 Scenario 与 §13 文案同步。
- **位置**：`openspec/changes/add-diagram-and-pseudocode-layer/specs/change-diagram-convention/spec.md:102-108`；`scripts/harness/diagrams-trace.sh:84-89`；`docs/reference/开发执行规范.md:477-478`
- **问题**：spec 明写「归档门禁 SHALL 检查**三件**」，其中第 2 条是「**`none` 档**理由非空」；§13 与 §11 表同样只对 `none` 提理由要求。但脚本在判 `kind` 之前就查了理由非空（84-87 行早于 89 行的 `if kind = none` 分支），于是 `<!-- diagrams: full() -->` 被判红。实测确认：`full()` → 「✗ diagrams: none/full 的括号里必须写一句大白话理由」。这是「三查三不查」契约被实现悄悄扩成四查。
- **证据**：`scripts/harness/diagrams-trace.sh:84-87` 的理由检查位于 `89` 行 `if [ "$kind" = "none" ]` 之前；delta `specs/change-diagram-convention/spec.md:104`「`none` 档理由非空」；`docs/reference/开发执行规范.md:478`「`none` → 括号里写一句大白话理由，不能空」。
- **建议**：二选一并让四处口径一致——要么脚本把理由检查移进 `none` 分支（`full` 允许空理由，spec 无需改），要么把 spec 第 2 条改成「marker 理由非空（full/none 同等）」、§13 与 §11 表同步。个人倾向后者（marker 一律带理由更利于回溯），但必须先把 spec 写清楚再改代码。

### R-6 2026-11-26【Medium】门禁只数代码围栏首行，空壳与未闭合围栏都能过
- **状态**：已修复
- **处置**：补两道检查：① 代码围栏必须成对闭合（反引号行数为奇数判红）；② 至少一块 mermaid 块内有实质内容（`awk` 扫块内非空行，空壳判红）。负例实测：空块→红、未闭合→红。
- **位置**：`scripts/harness/diagrams-trace.sh:100`
- **问题**：`grep -c '^[`][`][`]mermaid'` 数的是「以 ```mermaid 开头的行数」，不是「mermaid 代码块数」：不校验围栏是否闭合、不校验块内是否有内容、不校验块是否真在围栏内。实测 `design-diagram.md` 只写两行（`# 图` + 一行孤立的 ```` ```mermaid ````，围栏不闭合、图内容为零）仍判绿并输出「✓ diagrams: full（1 张图）」。review 重点 A.3 的答案是：会退化成形式主义，而且比预想的更容易——连「围栏闭合」都不必保证。
- **证据**：`scripts/harness/diagrams-trace.sh:100` 单条 grep 计数；实跑（/tmp 临时 fixture）对该空壳文件输出 `✓ diagrams: full（1 张图）`、退出码 0。
- **建议**：计数改为「配对闭合围栏 + 块内非空」，例如用 python 扫 ```mermaid … ``` 的完整块并要求块体 strip() 后非空、围栏成对；判定文案同步改成「至少一个闭合且非空的 mermaid 代码块」。这仍在「只查形式、不判语义」边界内，但把「空壳塞块过门禁」这条最廉价的绕过路径堵掉。

### R-7 2026-11-26【Medium】「图 SHALL 在对话中呈现」无任何可执行约束，且未给人工作者留出口
- **状态**：已修复
- **处置**：§13.2 补三段：**适用范围**（约束的是 agent 交付形态，人手工写 change 不受此约束）、**失败模式**（忘了贴 / 在长回复里被滚过去 → 缓解办法是把图贴第一屏）、**为什么还必须写进 spec**（不写就没有依据要求 agent 贴）。
- **位置**：`openspec/changes/add-diagram-and-pseudocode-layer/specs/change-diagram-convention/spec.md:29,36-39`；`docs/reference/开发执行规范.md:503`；`scripts/harness/review-trace.sh:46-48`
- **问题**：这条 SHALL 是全篇唯一不可机器验证的约束，§13 自己承认「靠 review 兜」，但 review 门禁 `review-trace.sh` 只按正则数 review.md 的条目数，并不检查「图是否贴进过对话」——兜底的那一环也是空的，等于无约束。另外措辞是「**写图的人** SHALL … 贴进对话回复里」，对人工撰写 change 的人不成立（没有对话可贴），spec 未给出豁免或替代通道（渲染截图存档？评审附件？）。
- **证据**：delta `specs/change-diagram-convention/spec.md:29` 与 Scenario `:36-39`；`docs/reference/开发执行规范.md:503`「它是本机制唯一无法机器验证的一环，靠 review 兜」；`scripts/harness/review-trace.sh:46-48` 的 `ENTRY_RE`/`STATUS_RE` 只匹配条目行。
- **建议**：要么按 review 重点 C.1 把它降级为 SHOULD 并把 spec 里「视为未完成」的强制措辞改掉；要么保留 SHALL 但补一条可执行替代：例如要求 `design-diagram.md` 顶部留一行「本图已于 <日期> 贴入对话」的落款 + 人工作者豁免句（人写 change 以评审附件截图替代）。同时在 §13.2 明确「谁来查、查不到怎么办」，否则这条约束在归档时无人负责。

### R-8 2026-11-26【Medium】新增的「用例图」全仓零示例，本 change 自己的样板也没有 ER 图
- **状态**：已修复
- **处置**：本 change 补第 4 张图——`erDiagram`（marker 与制品的存在约束：哪些一定在、哪些视 marker 而定）。现共 4 张，覆盖 ER + 顺序 + 状态/流程。「用例图零示例」保留：本产品单用户无 auth，用例图的角色概念本就弱，不硬凑。
- **位置**：`docs/reference/开发执行规范.md:487-493,525`；`openspec/changes/add-diagram-and-pseudocode-layer/design-diagram.md:9,40,65`；`openspec/changes/add-diagram-and-pseudocode-layer/specs/change-diagram-convention/spec.md:7,10`
- **问题**：本 change 的卖点是补 ER 图与用例图，但 ① 本 change 自己的 `design-diagram.md` 三块图实为 flowchart×2 + sequenceDiagram，**既无 erDiagram 也无用例图**；§13.5 却把它描述成「五类图中的三类」，名不副实；② 全仓唯一含 `erDiagram` 的是归档样板 `2026-11-26-merge-lane-ops-specs/design-diagram.md`（该 change 无表结构变更，与 ER 判据「涉及表结构、字段、关系或索引变更」对不上）；③ 用例图（flowchart+subgraph，左角色右用例）**没有任何示例**，只有一句描述——而它恰恰是 mermaid 无原生支持、最需要样例的一类。
- **证据**：delta `specs/change-diagram-convention/spec.md:7`（ER 判据）、`:10`（用例图判据）；`docs/reference/开发执行规范.md:525`「五类图中的三类 + 完整伪代码节」；`design-diagram.md` 三处 mermaid 块分别在 `:9`（flowchart TD）、`:40`（sequenceDiagram）、`:65`（flowchart LR）；全仓 `design-diagram.md` 中 grep `erDiagram` 仅命中 merge-lane-ops 归档样板。
- **建议**：在 §13.5 补两段可直接复制的骨架（erDiagram 逻辑实体版 + 用例图 flowchart+subgraph 左角色右用例版），并把 §13.5 指向本 change 的那行改成指向归档后的稳定路径（见 R-10）；ER 判据补一句「逻辑实体亦可，用实体名而非表名时须注明」，消解 B.2 的疑问。

### R-9 2026-11-26【Medium】头部解析边界：marker 写在 `# 标题` 之后不认，且脚本注释与实现自相矛盾
- **状态**：已修复
- **处置**：parser 改为**跳过 `# 标题` 行**（`if s.startswith("#"): continue`），marker 写在标题之后也认；脚本头注释与实现同步（「首个非注释、非空、非标题正文行之前」）。负例实测：`# 标题` → 空行 → marker → 正文，通过。
- **位置**：`scripts/harness/diagrams-trace.sh:38` 对 `:44-50`；`openspec/changes/add-diagram-and-pseudocode-layer/design.md:114`
- **问题**：脚本注释声明头部是「首个『非注释、非空、**非标题**』正文行之前的 HTML 注释区」，即声称会跳过标题行；实现却是遇到任意非注释非空行就 `break`，标题行照样终止头部。结果：proposal 以 `# 标题` 起头、marker 写在标题之后 → 判「缺 marker」。而本仓 291 份归档 proposal 中有 78 份正是标题起头，人写 change 极易踩中，且报错文案只说「头部需写」不提示「必须在标题之前」。
- **证据**：`scripts/harness/diagrams-trace.sh:38`「首个『非注释、非空、非标题』正文行之前的 HTML 注释区」对 `:43-50`（`break  # 头部结束` 无标题豁免）；实测 fixture（`# 标题` 在前、marker 在后）→ 「✗ 缺 diagrams marker」；marker 在前 → 正常识别。
- **建议**：要么让实现与注释一致（跳过 `#` 标题行继续扫头部），要么把注释与 design.md:114 的定义统一为「首个非注释非空行之前」并明确写进 spec——同时把报错文案补成「须写在正文首行之前（HTML 注释区）」，避免作者反复试错。

### R-10 2026-11-26【Low】§13.5 的样板指针指向本 change 目录，归档后即失效
- **状态**：已修复
- **处置**：§13.5 样板指针改为归档后路径 `openspec/changes/archive/2026-11-26-add-diagram-and-pseudocode-layer/`，并注明归档前的活动目录位置。
- **位置**：`docs/reference/开发执行规范.md:525`
- **问题**：样板指针写的是 `openspec/changes/add-diagram-and-pseudocode-layer/`，而本 change 归档后目录会被移到 `openspec/changes/archive/2026-11-26-add-diagram-and-pseudocode-layer/`，该指针当场变死链；同节的另一个指针（`archive/2026-11-20-fix-disaster-recovery-channel/design-diagram.md`）是归档态稳定路径，可作对照。
- **证据**：`docs/reference/开发执行规范.md:525` 与 `:526`（前者指向 active 目录、后者指向 archive 目录）。
- **建议**：改为指向归档后路径，或直接指向与 R-8 一并补的 §13.5 内联骨架（更稳，不依赖任何 change 目录）。

### R-11 2026-11-26【Low】编号与计数不一致：输出序为 ①②③④⑤⑥⑧⑦，文案仍称「六项/七个脚本」
- **状态**：已修复
- **处置**：⑧ 的 `run_item` 移到 ⑦ 之后，终端输出顺序恢复 `①②③④⑤⑥⑦⑧` 连续；注释「六项/七个脚本」统一改「七项」（⑥ 之后新增的 block 级项）。
- **位置**：`scripts/harness/archive-readiness.sh:7,23,24,80,101,127`
- **问题**：新增 ⑧ 插在 ⑥ 与 ⑦ 之间，实际输出顺序是 ①②③④⑤⑥⑧⑦；而脚本头部注释、退出码说明、启动横幅与最终成功文案仍一律称「六项」，测试缝注释仍称「七个脚本」（实为八个）。spec-gate 侧也没有 ⑧，读者按 spec-gate 的编号去查会找不到。
- **证据**：`scripts/harness/archive-readiness.sh:7`「与 spec-gate 归档门禁同口径的六项 block 检查」、`:23`「退出码：0 六项全绿」、`:24`「含同名七个脚本的目录」、`:80`「同口径六项」、`:127`「✓ 六项全绿」；实跑输出确认 ⑥→⑧→⑦ 顺序。
- **建议**：把 ⑦（提醒项）改成最后输出、把新硬检顺延为 ⑦（即 ①②③④⑤⑥⑦ + ⑧ 提醒），或统一改为「六项硬检 + 两项提醒」的表述并同步 `archive-readiness.sh` 全部计数文案与 `openspec/specs/archive-readiness/spec.md`；无论哪种，编号一旦定了就别再插中间。

### R-12 2026-11-26【Low】marker 的产出阶段是 propose，但 propose 阶段的路由清单不含 §13
- **状态**：已修复
- **处置**：`.pi/constraint-injection.json` 的 `explore`/`propose` 阶段 `docs.required` 加 `docs/reference/开发执行规范.md`；`.pi/extensions/lib/stage.ts` 的 `ROUTING_DOC_CATALOG` 加该文档 → 节名 `13. change 图表达与伪代码层`。实测 `catalogSection()` 返回正确节名。
- **位置**：`docs/reference/constraints-index.md`「阶段路由清单（propose）」；`docs/reference/开发执行规范.md:72`
- **问题**：marker 写在 propose 阶段的产物 `proposal.md` 里，而唯一教人「按 marker 决定画图」的 §13 在 propose 阶段的路由清单中并未被列出（该清单只列 `constraints-index.md`），§0.6 步骤 1 那句是 apply 阶段才生效的指引。这正是本 change 想治的病（没人写），却落在一个新病上：propose 阶段没人被告知要写 marker。
- **证据**：`docs/reference/constraints-index.md` 阶段路由清单（propose）仅列 `docs/reference/constraints-index.md` 一项；`docs/reference/开发执行规范.md:72` 的指引出现在 §0.6 步骤 1（主线程执行实现计划阶段）。
- **建议**：把 §13 列入 propose 阶段路由清单（正文仍按 stage 机制自主 read），或在 propose 阶段的制品模板（openspec-propose skill）里直接给出 marker 注释行模板，让它像 `ui-impact` 一样成为提案模板的固定一行。

## 核实记录

- 制品通读：`proposal.md`（79 行）、`design.md`（118 行，D1–D7 + 伪代码节）、`design-diagram.md`（77 行，3 块 mermaid）、`specs/change-diagram-convention/spec.md`（115 行，2 MODIFIED + 3 ADDED）、`tasks.md`（94 行，§11 有 Q1–Q6 结论）、`scripts/harness/diagrams-trace.sh`（107 行）、`docs/reference/开发执行规范.md` §13（466-560）与 §9/§11 门禁表、`standard/shared/doc-authoring.md` 新增节。
- 门禁实测（/tmp 临时 fixture，均只读仓库文件）：marker 在标题之后 → 判「缺 marker」；marker 在标题之前 → 识别正常；`none()` / `full()` → 判「理由为空」；mermaid 块缩进 2 空格 → 判「没有 mermaid 代码块」；`~~~mermaid` 围栏 → 同判红；仅一行未闭合 ```` ```mermaid ```` 的空壳文件 → **判绿**。
- 存量影响实测：`diagrams-trace.sh` 对 `add-server-capability-negotiation` / `description-repair-batch-matching` / `topic-lane-auxiliary-label-constraint` 均退出码 1（缺 marker），对 `add-home-pipeline-status-page` 退出码 0（无 `.openspec.yaml` 判 legacy）。
- 回归实测：`bash scripts/harness/archive-readiness.smoke.sh` 当前退出码 1（4/16 失败，失败点均为新 ⑧ 项 `exit 127`）；同脚本取改动前 da343417 版本实跑退出码 0，确认为本 change 引入的回归。`bash scripts/harness/archive-readiness.sh add-diagram-and-pseudocode-layer` 本 change 自身 ⑧ 判绿。
- 归档阻断链路核对：`.pi/extensions/spec-gate.ts` 的 7 处 `runScript` 目标中无 `diagrams-trace.sh`；全仓 grep `diagrams-trace` 仅命中 `archive-readiness.sh:101` 与 `开发执行规范.md:479`。
- 一致性核对：主 spec `change-diagram-convention/spec.md` 的「图表达制品的固定位置与性质」未被本 delta 的 MODIFIED 段覆盖；`openspec/specs/archive-readiness/spec.md:12` 的「与 spec-gate 完全相同口径」表述与新增 ⑧ 冲突；§13.5 的「五类图中的三类」与本 change 样板实际图型（flowchart×2 + sequenceDiagram）不符。
- 未修改任何仓库文件（本次审查全程只读；`git status --short` 前后一致）。
