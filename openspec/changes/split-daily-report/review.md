# 审查报告：split-daily-report

> 本文件为 `split-daily-report` 的只读审查记录（条目格式契约见 `openspec/specs/review-trace-gate/spec.md`）。

- **审查者**：MiniMax M3.1 Flash（只读）
- **结论**：需改后进
- **放行判断**：条数与正文守恒做得干净（27 条逐字零差异、159+99=258、REMOVED 20 条无误删），但**归档后的三族形态尚不可执行**：新建 capability `daily-report` 的 domain marker 与 3 个新 capability 的 Purpose 全部漏排（违 `spec-domain-index` 的 MUST，`check-standards.sh --change` 已 WARN），且「改生成算法只开 `daily-report`」这条核心判据被自家制品证伪——族名 `daily-report` 不是 capability，族跨 2 个文件，红线 35/20 被拆到两族。

## 发现

### R-1 2026-11-26【High】新建 capability `daily-report` 的 domain marker 与 3 个新 capability 的 Purpose 全部漏排
- **状态**：已修复
- **处置**：`split-spec-family.sh` 支持 `--purpose`（新建族必需），`family-purpose.tsv` 已写 interface/ops 两族文案；就地扩容的 `daily-report-system` 由 D3 修正后不需要新 Purpose（归档后手改为族级文案，tasks 8.3a）
- **位置**：`openspec/changes/split-daily-report/design.md` D6 第 3 条；`tasks.md` 8.3a / 8.3b / 8.3c；`proposal.md`「What Changes」表与「Impact」
- **问题**：本 change 会**新建 3 个 capability**：`daily-report`（match-fallback 的 2 条）、`daily-report-interface`（16 条）、`daily-report-ops`（9 条）。但 D6 的 marker 计划只点名了 `daily-report-system` / `daily-report-interface` / `daily-report-ops` 三个，**`daily-report` 这个新 capability 全程没出现过**；tasks 8.3 同样只安排 3 条 marker 任务、且没有一条是给 `daily-report` 的。归档时 openspec 会对 `daily-report` 写出**无 marker** 的 main spec，违反 `openspec/specs/spec-domain-index/spec.md`「每个 `openspec/specs/*/spec.md` MUST 在文件头部声明业务域」。另外 3 个新 capability 的 Purpose 在归档后会是占位串 `TBD - created by archiving change split-daily-report. Update Purpose after archive.`（校验器报 WARNING「Purpose section is still a placeholder」），tasks 只给 `daily-report-system` 安排了「手改 Purpose」，另外 3 个没有；上一族 change（`merge-board-diagnostic-specs`）的 review 已把「两族 Purpose 归档后为 TBD 占位」记为 finding。
- **证据**：
  - `bash scripts/harness/check-standards.sh --change split-daily-report` → J 段 3 条 WARN：`capability 'daily-report' 尚无 main spec（归档合并后请补 <!-- domain: ... -->）`，另两条为 interface / ops。
  - 用 openspec 2.1.1 `dist/core/specs-apply.js` 的 `buildUpdatedSpec` 对本 change 的 6 个 delta 逐个预演：`daily-report` / `daily-report-interface` / `daily-report-ops` 三个 `exists=false` 的重建稿首部均为 `# <cap> Specification` + `## Purpose` + `TBD - created by archiving change split-daily-report. Update Purpose after archive.`；`new Validator().validateSpecContent()` 对三者各报 1 条 WARNING「Purpose section is still a placeholder rather than a Purpose anyone wrote」。
  - `openspec/specs/spec-domain-index/spec.md`：「每个 `openspec/specs/*/spec.md` MUST 在文件头部（第一个 `#` 标题之前）声明业务域」。
- **建议**：D6 与 tasks 8.3 补三条：(a) `daily-report` → `<!-- domain: daily-report -->`（其第 2 条 `日报排除 direction_mismatch 标签` 与 `semantic-board` 红线 4 交叉，可按「改这族必须读哪些 flow 红线」取 `daily-report, semantic-board`）；(b) `daily-report-interface` → `<!-- domain: daily-report, ui -->`；(c) `daily-report-ops` → `<!-- domain: daily-report -->`。同时为 3 个新 capability 各补一句真 Purpose（≥50 字符、不含 TBD/TODO），并把「归档后写 Purpose」写成显式任务行，不只挂在 `daily-report-system` 上。

### R-2 2026-11-26【High】族名 `daily-report` 不是 capability：「改生成算法只开 `daily-report`」的判据不成立，且偏离 repo 既定「就地扩容」做法
- **状态**：已修复
- **处置**：**设计错误已纠**：族名从 `daily-report` 改为 `daily-report-system`，其 delta 内同写 ADDED（match-fallback 2 条）+ REMOVED（20 条），归档后 23 条 / 711 行、marker 保留。design D3 重写并补记「族名必须等于既有 capability 名，否则就是新建」
- **位置**：`proposal.md`「What Changes」判据行；`design.md` D3；`specs/daily-report/spec.md`
- **问题**：所谓 `daily-report` 族，实际由**两个 capability** 组成——21 条 A/E/F 组 Requirement 留在 `openspec/specs/daily-report-system/spec.md`，2 条 match-fallback 落到新建的 `openspec/specs/daily-report/spec.md`。因此 proposal 的判据「改生成算法/选题规则 → 只开 `daily-report`」按字面不成立：改生成算法要开的是 `daily-report-system`，而 `daily-report` 这个名字指向的只有 2 条兜底补算/方向排除的条目，读者按族名找会扑空。族跨 2 个文件这一点在 D6/tasks 里被记成「就地扩容」，但 `docs/reference/standard/shared/doc-authoring.md`「族名与旧 capability 重名时就地扩容」写的是另一种做法：**旧 capability 保留，只把兄弟成员的 Requirement 以 `## ADDED Requirements` 追加进去**（不搞改名新建、不另起同名新 capability）。本 change 走的正是「另起同名新 capability」这条没被标准覆盖的路。
- **证据**：
  - `openspec/changes/split-daily-report/specs/daily-report/spec.md` 只有 2 条 Requirement；`daily-report-system` delta 的 REMOVED 有 20 条、留原位 21 条（实测重建稿 21 req / 99 sc）。
  - 已实测的更简方案：把 2 条 match-fallback 以 `## ADDED Requirements` 与 20 条 `## REMOVED Requirements` **合并进同一个 `specs/daily-report-system/spec.md` delta**，用 openspec 2.1.1 `buildUpdatedSpec` 预演 → counts `{added:2, removed:20}`、重建稿 **23 req / 105 sc / 711 行**、首行 marker 原样保留、validator `valid=true` 零 ERROR。即：**不需要新建 `daily-report` capability**，族就是 1 个文件，spec 总数 150 − 2 + 2 = 150（而不是 151），R-1 的 marker/Purpose 缺口也随之消失。
- **建议**：二选一并同步改 proposal/design/tasks：(a) 改用上面的「单 delta 合并」形态（族名就叫 `daily-report-system`，符合 doc-authoring 的就地扩容定义，判据变成「改生成算法只开 `daily-report-system`」——成立）；或 (b) 保留新建 `daily-report` 的形态，但必须改掉判据措辞（说明族跨 2 个文件）、补 R-1 的 marker/Purpose、并把 proposal「新建 interface/ops 2 个」改成 3 个（见 R-10）。

### R-3 2026-11-26【High】interface/ops 的实际切分线不是「接口 vs 运维」：ops 族含 2 个 HTTP 端点，红线 35 / 20 被拆到两族
- **状态**：已修复
- **处置**：「日报重建窗口守卫」「终态通知计数口径与 WS 一致」从 ops 移入 interface——它们对应红线 20（generate API 与 TriggerNowWithDate 超窗同口径）与红线 35（WS 终态消息与通知三态同口径），都在端点/消息侧。ops 剩 7 条，仍含 backfill 端点但那是运维动作入口；切分线定为「动作性质」而非「有无端点」
- **位置**：`design.md` D1（「B + C 都属「日报怎么对外暴露」… D 属「日报出不来时怎么办」」）；`proposal.md` 判据行；`specs/daily-report-ops/spec.md` 与 `specs/daily-report-interface/spec.md`
- **问题**：制品与设计给出的切分线对不上，且两个被 brief 点名的跨族契约确实被拆开：
  - **ops 族里有端点**：`日报缺口可见性` 规定 `GET /api/daily-reports/gaps`，`日报缺口异步补填` 规定 `POST /api/daily-reports/backfill-gaps`，`日报调度窗口的生成时刻与兜底时刻可读` 引用 `GET /api/poll`。而 proposal 判据写的是「改端点/界面/状态块 → 只开 `daily-report-interface`」——按这条判据，这三条应当在 interface，实际在 ops。
  - **红线 20 被拆两族**：`日报重建窗口守卫`（ops）规定的是 `POST /api/daily-reports/generate`（该端点本体在 interface）的 4xx 拒绝行为，与 `TriggerNowWithDate` 同口径。改这个端点的错误语义必须同时改两族。
  - **红线 35 被拆两族**：WS 侧的三态计数在 interface 的 `日报生成 WebSocket 进度广播`（正文明写「`failed_count` / `empty_count` SHALL 与终态通知口径一致（`notification.DailyReportTerminal`）」），通知侧在 ops 的 `日报终态通知的计数口径与 WS 终态消息一致`（正文又明写「其 WS 终态消息（`daily_report_complete`）协议 SHALL 保持不变」）。加第 4 态计数要同时改两族。
  - 实测词频印证耦合面：变更后的三族里「终态通知」interface 8 次 / ops 12 次，「缺口」interface 26 次 / ops 28 次，`api/daily-reports` interface 13 次 / ops 7 次。
  - 补充：radar（归 interface）与 ops 的耦合其实**强于**与 interface 的耦合——radar 的「管线状态块的缺口区与补齐入口」正文写「补齐 SHALL 走异步补填接口」「补填入口 SHALL 沿用生成入口的双队列门槛」，直接依赖 ops 的缺口/补填契约与 interface 的生成端点门槛。
- **证据**：`specs/daily-report-ops/spec.md` 第 148 / 187 行（两个 `api/daily-reports` 端点）、第 58 行（`POST /api/daily-reports/generate` 返回错误）、第 98 行（`GET /api/poll`）；`specs/daily-report-interface/spec.md` 第 89–92 行（三态计数 + 指向 `notification.DailyReportTerminal`）；`docs/reference/flow/daily-report.md` 红线 20 与 35 原文。
- **建议**：把 D1/proposal 的判据改成与制品一致的表述——真实切分线是「**生成期（daily-report / interface）vs 缺档期（ops）**」，不是「接口 vs 运维」；并对红线 20、35 在两族各留一句显式互指（"该口径的另一半见 `daily-report-ops` 的 `日报终态通知的计数口径与 WS 终态消息一致`"），避免下次改 WS 终态消息只改一边。若坚持按「接口面归 interface」切，则把 ops 里两个缺口端点的**端点契约**移入 interface、缺口判定口径留 ops（跨族互指仍需补）。

### R-4 2026-11-26【Medium】tasks 8.3a「归档后文件从 delta 重建会丢 marker」与 openspec 2.1.1 实测相反，照做会插出重复 marker
- **状态**：已修复
- **处置**：tasks 8.3a 更正为「marker 不用补」——`buildUpdatedSpec` 实测预演显示 daily-report-system 是纯删除+追加、marker 保留、新增 0 行
- **位置**：`tasks.md` 8.3a 括注「（另需补 marker，归档后文件从 delta 重建会丢）」
- **问题**：`daily-report-system` 的 delta 只有 REMOVED，归档走的是「把 delta 合进现有 main spec」而非「从 delta 重建」：openspec 的 `buildUpdatedSpec` 取现有 main spec 的 `parts.before`（marker + `## Purpose`）作为基线，重建稿首行仍是原 marker。实测重建稿 675 行 / 21 req / 99 sc，**相对原文件 0 行新增**（纯删除）、diff 只有被搬走的 20 个块、21 条保留 Requirement 顺序原样、marker 一字未动。实施者按括注字面「补 marker」，会在已有 marker 的文件里再插一行，产出**重复 domain marker**。
- **证据**：用 openspec 2.1.1 `dist/core/specs-apply.js` 的 `buildUpdatedSpec` 预演 `daily-report-system` delta：`counts={removed:20}`、`noRequirementBlocks=false`、`unaccountedContent=["<!-- domain: daily-report, semantic-board -->", "归属预算 SHALL 为每份报告 60s…"]`（第二项是原 spec 里 `日报生成编排流水线` 块内一段散落正文，属原有问题，不影响本次判定）；重建稿首行 = `<!-- domain: daily-report, semantic-board -->`；`diff 原文件 重建稿` 的新增行数 = **0**。
- **建议**：删掉 8.3a 的括注，改为「marker 自动保留，归档后核对首行仍是 `<!-- domain: daily-report, semantic-board -->` 即可」；8.3a 只保留「手改 Purpose」。顺带把 `unaccountedContent` 里那段散落正文（`日报生成编排流水线` 块内的「归属预算 SHALL 为每份报告 60s…」段落，位置在两个 `#### Scenario:` 之间）登记为内容问题（可并入 task 3.4）。

### R-5 2026-11-26【Medium】`split-plan.tsv` 标称「可复现」，但消费它的生成器未入库
- **状态**：已修复
- **处置**：生成器入库为 `scripts/harness/split-spec-family.sh`（含 --kind/--purpose），`split-plan.tsv` 与 `spec-classification.txt` 均在 change 目录，可复现
- **位置**：`design.md` D4「新增 `split-plan.tsv`（存 change 目录，**可复现**）」；`tasks.md` 2.1 / 2.2 / 2.3 / 2.4
- **问题**：仓库里没有任何脚本消费 `split-plan.tsv`——`scripts/harness/merge-spec-family.sh` 是 cap 级粒度（设计里也承认「粒度不够」），本次的按标题子集切分是**一次性 ad-hoc 生成**的，生成器没入库。后果：(a) 「按 plan 重跑能否得到同样的 delta」目前答案是**没有生成器可跑**；(b) `spec-classification.txt` 的「26 business / 1 impl」是纯序号表、没有 Requirement 标题对照，序号只有在复原生成器的取块顺序（族 → cap → Requirement）后才可解读，同样不可复现。
- **证据**：`grep -rn 'split-plan' --include='*.sh' --include='*.py' .` 只命中 `tasks.md` / `design.md` 自身，无任何脚本引用；`scripts/harness/` 下只有 `merge-spec-family.sh`（cap 级）。`spec-classification.txt` 27 行形如 `19 impl` / `20 business`，无标题列（#19 = `定时任务复用` 只有在按 `split-plan.tsv` 的族序 daily-report(2) → interface(16) → ops(9) 展开才成立）。
- **建议**：把生成器入库（建议 `scripts/harness/split-spec-family.sh <plan.tsv>`，复用 `merge-spec-family.sh` 的 `requirement_blocks` / `first_change` / kind-file 逻辑），让 D4 的「可复现」成立；或在 D4 如实改写为「本次为一次性生成，`split-plan.tsv` 只留作切分意图记录，不保证可重跑」，并把 `spec-classification.txt` 改成带 Requirement 标题的对照表。

### R-6 2026-11-26【Medium】文档面漏 semantic-board 域：`radar-pipeline-status 保留独立` 的旧结论与 ISS-04 反例已陈旧
- **状态**：已采纳
- **处置**：「radar 保留独立」的旧说法出现在上族 design 与此前台账，属历史记录；本 change 已在 design D2 记明推翻理由（判据不变、对照组变了）。台账 ISS-04 在收尾时一并更新
- **位置**：`tasks.md` 7.1 / 7.2 / 7.3；`proposal.md` 头部 `<!-- constraint-domains: daily-report, semantic-board -->`
- **问题**：本 change 声明了 semantic-board 域，且把 `radar-pipeline-status` 并进了 `daily-report-interface`，但文档任务只覆盖了 `flow/daily-report.md`：
  - `docs/reference/flow/semantic-board.md` 的变更溯源表仍写「`radar-pipeline-status` 保留独立（雷达页状态块 UI 契约）」——本 change 把它拆散并入 interface，这行变陈旧。
  - `docs/research/process-arch-rework/issues.md` ISS-04 第 106/107 行把 radar 当作**归族判据的反例**（「5 个 board-* 里 radar-pipeline-status 与其余 4 个零改动连带，硬并会把 UI 契约混进分析族」）——本 change 恰好硬并了它，该反例要么改写、要么补注「反例只在『诊断/研究』这一对对照下成立」。task 7.2 只说「回填 ISS-04（第五族）」，没说处理这两行。
- **证据**：`grep -rn 'radar-pipeline-status' docs/` 命中 `docs/reference/flow/semantic-board.md:27`（正文描述状态块本身，可保留）、`:568`（溯源表「保留独立」，陈旧）、`docs/research/process-arch-rework/issues.md:106`、`:107`（反例，陈旧）。
- **建议**：tasks 7.1 扩为「`flow/daily-report.md` + `flow/semantic-board.md` 各加一行变更溯源」；task 7.2 明确要求改写 ISS-04 第 106/107 行对 radar 的「保留独立」与「反例」表述（或补注其适用边界）。另：主 spec 侧扫描干净——`grep -rn 'daily-report-match-fallback\|radar-pipeline-status' openspec/specs/` 除两个待退役文件自身外零引用，退役后不留悬挂 capability 引用。

### R-7 2026-11-26【Low】impl 条目未排在族末尾，违反 doc-authoring 约定
- **状态**：已修复
- **处置**：`split-spec-family.sh` 内置排序：business 在前、impl 在后，各自保持 plan 顺序
- **位置**：`openspec/changes/split-daily-report/specs/daily-report-ops/spec.md`（`定时任务复用` 是该 delta 的第 1 条 Requirement，第 3 行）
- **问题**：`docs/reference/standard/shared/doc-authoring.md` 规定「合并时发现施工记录（包结构/命名/一次性迁移/纯测试要求）→ **排到族 spec 末尾**并加 `<!-- kind: impl -->`」。现存先例一致：`openspec/specs/tagging/spec.md` 的 impl 条目在 914–1126 行（文件末）、`tag-matching/spec.md` 在 896–1126 行（文件末）。本 change 把唯一的 impl 条目放在 ops 族第一位。
- **证据**：`grep -n 'kind: impl' openspec/specs/*/spec.md` → `tagging/spec.md:914…1126`、`tag-matching/spec.md:896…1126`、`board-api/spec.md:476`、`board-diagnostics/spec.md:636`（均在族末尾）；本 change 的 `specs/daily-report-ops/spec.md:6` 却是首条。
- **建议**：ops delta 里把 `定时任务复用` 移到最后一条（合并脚本生成顺序天然如此，本次手工调整即可）。

### R-8 2026-11-26【Low】`定时任务复用` 判 impl 偏宽松（含一条面向行为的 SHALL）；classification 文件无标题映射
- **状态**：已修复
- **处置**：`spec-classification.txt` 改为标题键格式（plan 重排不影响映射），并注明 `定时任务复用` 判 impl 偏宽松的边界
- **位置**：`spec-classification.txt`（唯一 impl = #19）；`design.md` D5；`specs/daily-report-ops/spec.md` 第 3–12 行
- **问题**：D5 的预判条件是「若正文**只是**『复用既有 cron 注册』，偏 impl」。实际正文三句里两句是施工记录（复用 `scheduler_tasks` 的 `narrative_summary` 任务、`check_interval` 保持 86400s），第三句是**面向行为的 SHALL**：「定时任务 SHALL 异步执行并通过 WS 广播进度」。整条判 impl 会把这条行为契约降级为「施工记录」。另外 `spec-classification.txt` 只有序号 → business/impl，没有 Requirement 标题，#19 = `定时任务复用` 依赖外部复原索引顺序，不可自证。
- **证据**：`specs/daily-report-ops/spec.md` 第 8 行：「定时任务 SHALL 异步执行并通过 WS 广播进度。」；`spec-classification.txt` 27 行全为 `N business|impl`。
- **建议**：把该条拆成两条（impl：复用 `scheduler_tasks`/`narrative_summary`/`check_interval`；business：调度路径异步执行且经 WS 广播进度），或整条保持 business（它同时规定了调度路径与 WS 的关系）；`spec-classification.txt` 补 Requirement 标题列或注明序号口径。

### R-9 2026-11-26【Low】direction_mismatch 在 daily-report 族内出现两处口径
- **状态**：已采纳
- **处置**：direction_mismatch 在 daily-report 族内两处口径重叠（match-fallback 的「排除」与 A 组的「线索排序后置」），登记到跨族引用清扫批次
- **位置**：新建 `specs/daily-report/spec.md` 的 `日报排除 direction_mismatch 标签`（第 25–38 行）vs 保留原位的 `openspec/specs/daily-report-system/spec.md` 的 `日报质量筛选` 规则 2（第 122 行「过滤 `direction_mismatch = true`」）
- **问题**：两处都规定日报收集侧排除 direction_mismatch，且都点到「主查询 + fallback 路径」。这是**搬运前就存在**的跨 capability 重叠（不是本 change 造的），但合并后同族两个 capability 各说一遍，读者容易误判为两套规则。tasks 3.3 的「跨 spec 重复检查」只列了 direction_mismatch × `tag-matching`/`board-diagnostics` 那一对跨族对，没列族内这一对。顺带回答 brief 的 A.3：**「日报侧排除」本身确实是日报域的独立契约**（方向判定归 `board-diagnostics`，日报侧只消费结果），跨族交叉是合理分工，不构成分族错误——需要修的只是同族重复的可见度。
- **证据**：`grep -n 'direction_mismatch' openspec/specs/daily-report-system/spec.md` → 第 122 行在 `日报质量筛选` 的筛选规则列表第 2 条；同文件第 1241/1258 行（`日报缺口可见性`）亦出现；`specs/daily-report/spec.md` 第 30 行整条重述该排除规则。
- **建议**：在 task 3.3 的重复检查清单里补上族内这一对，至少在两条 Requirement 之间加一句互指（「主查询级排除见 `日报排除 direction_mismatch 标签`；管线级筛选规则见 `日报质量筛选` 第 2 条」），并登记进既有「跨族引用清扫」batch。

### R-10 2026-11-26【Low】proposal 内部数字不自洽（删 16→20 条 / 新建 2→3 个 / 三族行数 726·697·260）
- **状态**：已修复
- **处置**：proposal 三族数字改为 711 / 787 / 227，spec 数改为 150 → 150
- **位置**：`proposal.md`「What Changes」表与「Impact」段
- **问题**：三处对不上：
  - 「`daily-report-system` 保留原名，只留 A+E+F（**删 16 条**搬走）」——实际删 **20** 条（B4+C7+D9）；design D3 写的是 20，proposal 写 16。
  - 「spec 数量：150 → 151（退役 … 2 个，**新建 interface/ops 2 个**，`daily-report-system` 保留）」——实际新建 **3** 个（还有 `daily-report`）；151 这个总数对（150 − 2 + 3），但枚举漏了一个，与 D6/tasks 的 marker 计划漏 `daily-report` 是同一个疏漏。
  - 三族行数「726 / 697 / 260」与归档后的实际 main spec 行数不符：实测 **719（= 675 `daily-report-system` + 44 `daily-report`）/ 728 `daily-report-interface` / 283 `daily-report-ops`**；其中 726 恰好等于 interface 的 **delta 文件**行数，697/260 与任何口径都对不上。D5 的 1200 预警线该看的是**归档后 main spec 行数**，按该口径三族均低于 1200（结论不变，数字要改）。
- **证据**：openspec 2.1.1 `buildUpdatedSpec` 预演 6 个 delta 得到的重建稿行数：`daily-report-system` 675、`daily-report` 44、`daily-report-interface` 728、`daily-report-ops` 283；`wc -l` 于各 delta 文件：726 / 38 / 280 / 101。
- **建议**：proposal 统一改用「归档后 main spec 行数」口径，并修 16→20、2→3；若采纳 R-2 的单 delta 形态，三个数字整体重写为 711 / 728 / 283。

### R-11 2026-11-26【Low】`日报排除 direction_mismatch 标签` 不含 SHALL/MUST（继承的校验器 WARNING）
- **状态**：已采纳
- **处置**：`日报排除 direction_mismatch 标签` 缺 SHALL/MUST 是继承自原 spec 的 WARNING，本 change 不改语义
- **位置**：`openspec/changes/split-daily-report/specs/daily-report/spec.md` 第 25–38 行
- **问题**：`openspec validate split-daily-report` 报 WARNING「ADDED "日报排除 direction_mismatch 标签" should contain SHALL or MUST (RFC 2119 best practice)」——正文是祈使句「collectBoardTags 的主查询和 fallback 补算均排除 direction_mismatch=true 的标签」。该问题继承自 `daily-report-match-fallback` 原文，非本次搬运引入；task 3.4 声明「内容问题登记（本次不修）」，需确保它被登记而非漏记。
- **证据**：`openspec validate split-daily-report` 输出首条 WARNING（`daily-report/spec.md: ADDED "日报排除 direction_mismatch 标签" should contain SHALL or MUST`）。
- **建议**：在 task 3.4 的登记清单里点名这一条（可与「跨 spec 重复检查」同批处理：改成 SHALL 句式 + 与 `日报质量筛选` 规则 2 互指，一次改两处）。

### R-12 2026-11-26【Low】域级指标未收敛：daily-report 域拆分后 16 cap/4413 行 → 17 cap/5234 行；给出 4 族备选
- **状态**：已采纳
- **处置**：design 新增 D8「域级未收敛」：拆完 daily-report 域仍有 17 cap / 5234 行，拆分是减法不是收敛；interface 787 行是下一个候选
- **位置**：`proposal.md`「不做的事」；`design.md` D1 / D5
- **问题**：拆的是「最大单文件」，不是「域」：`daily-report` 域当前 16 个 capability / 4413 行；本次拆分后变成 **17 个 capability / 5234 行**（16 − 2 退役 + 3 新建；行数 4413 − 38 − 194 + 44 + 728 + 283 = 5234）。`docs/reference/standard/shared/doc-authoring.md` 的族化触发条件是「族内碎片已多（>1500 行 或 **改一次要读 >3 个 spec**）」——拆分后 daily-report 域仍远在这条线之上，且该域内 4 个日报族 capability（675 / 44 / 728 / 283）里任取一项改动都可能牵动另外三个（见 R-3）。另 `daily-report-interface` 728 行 / 16 req / **114 scenarios**，其中 12 条是 UI 契约（4 API + 7 C 组 + 5 radar），是下一个可能长成巨型的文件。
- **证据**：按 `<!-- domain: -->` 统计全仓 `openspec/specs/*/spec.md`：`daily-report` 域现状 16 cap / 4413 行（最大 `daily-report-system` 1417）；`semantic-board` 域 24 cap / 12501 行；`topic-graph` 域 20 cap / 6195 行。
- **建议**：proposal「不做的事」补一句本次的收益边界（只降单文件上限、不收敛域碎片数），把 domain 级收敛明确留在 ISS-04 的剩余项里。若要继续推进 daily-report 域，可评估 4 族备选：`daily-report`（A/E/F，23 req）/ `daily-report-api`（4 req）/ `daily-report-ui`（7 + 5 = 12 req，对齐 repo 的 `ui` 域惯例）/ `daily-report-ops`（9 req）——接口面与界面面分开后，R-3 里「端点该去 interface」的争议自然消解，且 `daily-report-ui` 可整体挂 `<!-- domain: ui, daily-report -->`。

## 核实记录

**已机器复核（只读，未改动任何仓库文件）**

| 项 | 命令 / 方法 | 结果 |
| --- | --- | --- |
| 条数守恒 | 自写解析：按 `### Requirement:` 切块比对 change 的 6 个 delta | ADDED **27**（interface 16 + ops 9 + daily-report 2）；REMOVED **27**（daily-report-system **20** + match-fallback 2 + radar 5） |
| 逐字搬运 | 同上，去掉 `<!-- from -->` / `<!-- kind -->` 行后逐字比对源 main spec | 差异 **0** 处；27 条标题在源 main spec 中全部存在 |
| Scenario 守恒 | 同上 | 搬走 **159** + 留原位 **99** = **258** = 原 3 spec 总和（221 + 6 + 31） |
| REMOVED 未误删 | `daily-report-system` REMOVED 20 条标题全部在 main spec 内；留原位 21 条 = A(15) + E(5) + F(1) | 一致 |
| 归档后形态预演 | openspec 2.1.1 `dist/core/specs-apply.js` 的 `buildUpdatedSpec` + `findSpecUpdates` 逐个 delta 预演 | `daily-report-system` **675 行 / 21 req / 99 sc**（相对原文件**新增 0 行**，纯删除、顺序原样、marker 保留）；`daily-report` **44 / 2 / 6**；`daily-report-interface` **728 / 16 / 114**；`daily-report-ops` **283 / 9 / 39**；两个待退役 spec 重建后 0 requirement → 摘 marker 后可退役 |
| 退役前置 | `decideSpecOutcome` / `contentTheMergeCannotName` 读码 + 预演 | `daily-report-match-fallback`、`radar-pipeline-status` 的 `unaccountedContent` 仅剩 marker 行 → **tasks 4.3 摘 marker 必要且充分**；`daily-report-system` 因 marker + 一段散落正文 → 不会被误判退役 |
| marker 三态 | 预演首部 | `daily-report-system` marker **保留**（R-4）；`daily-report-interface` / `daily-report-ops` 需归档后补（R-1）；`daily-report` **漏排**（R-1） |
| change 校验 | `openspec validate split-daily-report` | valid（13 条 WARNING，全部继承自原文：长 Requirement / 缺 SHALL） |
| 全仓 spec 校验 | `openspec validate --specs` | **150 passed, 0 failed**（`--strict` 下 85 failed，是全仓既有状况，非本次引入） |
| 归档门禁自查 | `bash scripts/harness/archive-readiness.sh split-daily-report` | 6/7 绿，唯一红项是 review-trace 零痕迹（即本文件） |
| 域声明检查 | `bash scripts/harness/check-standards.sh --change split-daily-report` | J 段 3 条 WARN：3 个新 capability 尚无 main spec（R-1） |
| 域历史 | 读 `scripts/harness/domain-history.sh` 第 2/6 段逻辑 + 现网基线 | 现网 daily-report 域 16 cap；退役的 2 个 capability 在归档后 `CAP_DOMAIN` 为空 → `split-daily-report` 会进第 6 段「孤儿 delta」（D7 判断成立）；被删 20 条的历史 delta 仍指向 `daily-report-system`（marker 保留）→ 仍能命中、不成孤儿（D7 判断成立）；但**若 `daily-report` 不补 marker（R-1），它自己也会被算作孤儿** |

**未复核 / 超出本次范围**

- 归档不可逆性：`openspec archive` 的产物（change 目录移入 `archive/`、3 个新 main spec、2 个 main spec 删除、1 个 main spec 重建）全在一次提交内，`git revert` 可整体恢复；但**归档前摘 marker 的改动若单独提交，revert 需连带该提交**。注意当前工作树已有 7 个来自上一族 change（`merge-topic-specs`）的未提交 main spec 删除（`board-article-api` / `board-editing-ui` / `board-management-api` / `board-upgrade-expand` / `topic-key-event-markers` / `upgrade-candidate-time-window` / `watch-materialized-topic`），这是**本次 change 之前**就存在的脏树；本 change 归档提交若与之混提，一次 revert 会把上一族已退役的 spec 一起恢复。属既有问题，本报告只作提示，不计入本次发现。
- 零代码改动，故未跑后端/前端测试；本次验收即结构与守恒，验证手段即上表。
- `daily-report` 域内其余 14 个 capability（`daily-report-margin-notes` / `section-relations` / `board-lane-dynamics` 等）是否应并入本族，属 ISS-04 的剩余项，本 change 的「不做的事」已明确不并，未展开评估。
