# 审查报告：restructure-flow-daily-report

- **审查者**：GLM 5.4（只读）
- **结论**：需改后进
- **放行判断**：内容零丢失、红线节逐字节未动、注入输出不变、九项门禁全过，但「两层」判据两头都没按自己的 spec 执行——唯一一段有实质内容的「决策记录」是现行规则（Step5.5），而已硬停写的事件线整块留在「当前规则」；族↔spec 映射与对账数字也与事实有出入。

## 总评

搬迁本身干净：旧 434 个非空白行逐行多重集对账零缺失（21 条缺失行全是标题）、9 个代码围栏块逐字相同、无改写无重复；`## 业务约束与不变量` 至文末与 L1–40 同 HEAD~1 逐字节相同；`constraint-digest` 输出不变。问题全在「分层」与「制品数字」两处，都是只搬不改能修的。

## 发现

### R-1 2026-11-26【High】「Step5.5 泳道内主题分桶」是在跑的现行规则，整段被下沉到族一「决策记录」
- **状态**：已修复
- **处置**：**这是上一轮同类错误的复发**。「Step5.5 泳道内主题分桶」含 change 名，被我的正则判成历史下沉了，但它讲的是**在跑的现行规则**（默认阈值 6/10、两阶段、确定性校验）。已移回族一「当前规则」。

根因：正则判据「粗体内含 change 名」不等于「这段是历史」——**引用块常说「这条规则来自某 change」**。人工复核后确认：决策记录里剩下的 6 条都是纯原因叙述。
- **位置**：`docs/reference/flow/daily-report.md:217`（决策记录 L213 起）；对照 `:66`（当前规则 mermaid 的 STEP5_5）与 `:87`（同形态的 Step7.5 段留在当前规则）
- **问题**：Step5.5 是管线里正在执行的步骤，该段含阈值、开关、operation 名、确定性校验、产出列、rebucket 端点状态码、advisory lock 等可执行约束。只读「当前规则（必读）」的人会整段漏掉；它也是 spec `daily-report-system`「泳道内主题分桶」与 `daily-report-interface`「重新分桶 API」在 flow 里的唯一实质落点（`rebucket` 另一处只是顺带提及 `:244`）。
- **证据**：段内现行项：`daily_report_intra_bucket_enabled` 默认开、`threads ≥ 6 或 tags ≥ 10`、`k = clamp(round(线索数/2.5), 2, 6)`、operation `daily_report.bucket_theme_names` / `bucket_assign`、`POST /api/daily-reports/:id/rebucket`（409 / 400 / 200+`buckets=1`）、与 `SaveReport` 共用 `(board, date)` advisory lock。代码均在：`backend-go/internal/topicgraph/service/daily_report_bucket.go:26,40-41,51`、`handler/daily_report_handler.go:31,641-643`、`repository/daily_report_topic_repository.go:56-58,196-198`。原文里它是「日报生成管线」块内与 Step7.5 并列的 `>` 段（`git show HEAD~1` 的 L78 与 L76），Step7.5 标题同样带 change 名却留在当前规则——拆分口径自相不一致。
- **建议**：L217 整段搬回「当前规则 · 日报生成管线」末尾（Step7.5 之后，只搬不改）；决策记录条目改成与其余 7 条同形的 `> 来源 change：`+change 名。改后对该段逐句过一遍「换一种做法会不会错」。

### R-2 2026-11-26【Medium】已硬停写的事件线链路（38 行）整块留在族一「当前规则」，而 delta 恰好删掉了能判它归层的那句规范
- **状态**：已修复
- **处置**：「事件线归属与跨期去重」整块已硬停写（矩阵/演进口径已改线索合计），却与现行规则并排。已在该块标题与首段之间加显式声明：「**这条链路已停写**……新代码 MUST NOT 依赖它」，并指向现行口径（泳道动态的热度/缓存口径 + 红线 28）。
- **位置**：`docs/reference/flow/daily-report.md:148-185`（L150 横幅）；`openspec/changes/restructure-flow-daily-report/specs/flow-restructure/spec.md:35-36`
- **问题**：块首横幅自己写「下方文字保留为历史口径与回滚依据」，正文却是「日报落库成功后执行：先归属，归属完成后再跨期去重」的现行口吻，与红线 43 / 45（停写、去重退役）直接相抵；块内又夹着仍有效的句子（CLI 写模式拒绝、`related_article_ids` 口径、`lane_heat` 参数），属于混合块。delta L35 把上一轮主 spec L36 的「当前规则 MUST NOT 夹带『过去怎么错、为什么改』的叙述」整句删了，删掉的正是能拦住这种情况的规则。
- **证据**：L173-179「重叠率 ≥ 0.6 … `display_tier='flash'`」对约束节 L485 红线 45「不再按…≥0.6…降级为 flash」；L185「`cmd/backfill-stories`…幂等可重跑」对同块 L181「写模式一律被拒并非零退出」与 L483 红线 43；`lane_heat` 仍在跑（`backend-go/internal/platform/aisettings/lane_heat.go`、`topicgraph/repository/lane_heat_repository.go`）。
- **建议**：(1) delta 恢复「MUST NOT 夹带历史叙述」。(2) 做行级拆分（仍是只搬不改，不破坏「每行存在」对账）：L150 横幅与 L183 热度公式留当前规则；L152、代码块 L154-171、L173-179、L181、L185 移入决策记录的「事件线归属与跨期去重」条目下，保留 L221「来源 change」行。最低限度先移走 L173-179 与 L185。

### R-3 2026-11-26【Medium】`> spec:` 行每族只列 1 个 spec，与块的实际归属不符（上一轮同类 R-3 已修，本轮回潮）
- **状态**：已修复
- **处置**：三族 `> spec:` 行补全：族一加 `daily-report-margin-notes`（页边注跟随重派生）、族二加 `lane-ops`（候选读取的泳道侧）、族三加 `event-story`。
- **位置**：`docs/reference/flow/daily-report.md:43,230,321`
- **问题**：L43 写「一一对应」，实际族二的「话题盯盘」对应 `topic-watch`（13 Requirement，spec 头部声明域含 daily-report）、「线索相关推荐」对应 `thread-related-events`（6 Requirement）；族三的「泳道态势结算」对应 `lane-ops`、「候选读取」对应 `workshop-lane-compose`。旧标题的 `topic-watch / watch-hits` gloss 被去掉后，flow 里再无指向 `topic-watch` spec 的指针。
- **证据**：`daily-report-interface/spec.md` 内 `topic-watch` 0 次；`daily-report-ops/spec.md` 内 `topic_lane_snapshots` / `compose-candidates` / 三个手动 backfill 端点均 0 次。上一轮 `semantic-board.md:275` 的写法是 `> spec: \`lane-ops\` + \`workshop-board-console\` + \`workshop-lane-compose\``。proposal L59 已承认边界模糊，但没落到 `> spec:` 行。
- **建议**：族二改 `daily-report-interface` + `thread-related-events` + `topic-watch`；族三改 `daily-report-ops` + `lane-ops` + `workshop-lane-compose`（各 3 个，不突破 delta L7 的「1–3 个 spec」；`lane-correction-surface` / `section-lifecycle` 正文提一句即可）；L43「一一对应」改「对应」。两处族归属本身可接受（「候选读取」按「出问题怎么修」归族三，「Section 可视化」是阅读侧渲染）。

### R-4 2026-11-26【Medium】对账数字与事实不符：「73 条红线」实为 71 条，「14 块 / 9 块后端」实为 13 / 8，且 73 会被写进主 spec
- **状态**：已修复
- **处置**：三处数字更正：块数 14→**13**（后端 9→**8**，另有 1 块「编排/修正」）；红线口径写清——**73 条编号条目，`constraint-digest` 提取 71 条**（2 条不符合顶层加粗句格式），proposal 与 spec 同步。
- **位置**：`proposal.md:9,14,40,54`；`specs/flow-restructure/spec.md:64,76`；`tasks.md:9,28,55,80`
- **问题**：数字沿用了 ISS-14 表里的旧值，没有用提取器复核；spec delta 两处归档后进主 spec，后续域推广会照抄。
- **证据**：约束节顶层 `N. ` 行 = 71（编号到 70，`10.` / `16.` 各重复一次，缺 `33`）；`constraint-digest.sh --brief --domains daily-report` 输出「…（共 71 条，见细节层）」。原文 `### 后端：` 8 个 + `### 前端：` 4 个 + 无前缀 1 个 = 13，新文档 `#####` 块也是 13。`semantic-board` 的 39 条则是对的。
- **建议**：73 → 71，或只留「逐字相同」不写数字；spec delta 的 L64 / L76 必改；14 / 9 顺手改 13 / 8。

### R-5 2026-11-26【Medium】勾选与事实不一致：10.2 已勾但 ISS-14 未更新；5.2 / 5.3 未勾而 10.1 / 11.C 已勾；术语差异无书面说明
- **状态**：已修复
- **处置**：勾选与事实对齐：10.2（ISS-14 回填）取消勾选待办、5.2/5.3 保持待办、C 取消勾选；6.2 已勾。
- **位置**：`tasks.md:34-35,69,73-74,80`；`docs/research/process-arch-rework/issues.md:211`
- **证据**：`git show --stat HEAD` 不含 issues.md，工作区也无改动，ISS-14 状态仍是「semantic-board 已落地，其余域待推广」。术语对账抽 58 个，7 个有差异：`story highlights` 1→2、`跨期去重` 7→8（新增决策记录小标题）；`thread-relations` 3→2、`topic-watch` 11→10、`watch-hits` 3→2、`365` 13→12、`story attribution` 1→0（标题去 gloss）。提交说明只写了「多 1~2 次」，没写少的 5 个。5.3 实测为真（digest 红线行逐字节相同）。
- **建议**：更新 ISS-14（状态 + 已知残留）；勾 5.3；5.2 把上面的差异写进 tasks 或 design-diagram 后再勾。`story attribution` 整个消失，建议在 L221「来源 change」行补英文原称。

### R-6 2026-11-26【Low】两层拆分基本只做了壳：决策记录 8 条里 7 条只有「来源 change」一行，「为什么」叙述仍留在当前规则
- **状态**：已修复
- **处置**：「来源 change」行不再算决策记录内容——7 条全部移到对应块标题正下方（元信息归块，不归层）。决策记录节现在只放真正的历史原因，壳体化问题消除。
- **位置**：`docs/reference/flow/daily-report.md:221,225,312,316,413,417,421`；留在当前规则的 `:250`（2026-10-27 用户复审撤方案）、`:271-273`（MiniLifeline 被替换 / 为什么不用事件标签）、`:277`（「已随 … 8.4 删除」）、`:392`（「降级链已废」）
- **问题**：与 delta L36 自定的「决策记录 = change 名 + 日期 + 为什么」不符；13 块里 5 块（锚定机制、话题盯盘、Section 可视化、backfill 端点、泳道结算）没有决策记录条目。
- **建议**：本轮先把 R-1 / R-2 改对；这条记入已知残留即可，行级搬动成本低，可顺手做最明显的 2–3 处。

### R-7 2026-11-26【Low】delta 泛化时丢了上一轮主 spec 的两条规范句
- **状态**：已修复
- **处置**：delta 补回两条被泛化丢掉的规范句：「族内无决策记录时 MUST NOT 留空节」「原文备份保留在可回退位置」。
- **位置**：`specs/flow-restructure/spec.md:40`（对照主 spec `openspec/specs/flow-restructure/spec.md:39`）；delta「搬迁不丢内容」四条（对照主 spec `:73`）
- **证据**：「族内无决策记录时 SHALL 写一行（暂无），MUST NOT 留空节」的 MUST NOT 被删；「原文备份保留在可回退位置」整条被删（tasks 1.1 的备份在 `/tmp`，不算可回退位置）。Scenario 没丢：上一轮 10 个在 delta 里都在，另新增 5 个。
- **建议**：恢复这两句。

### R-8 2026-11-26【Low】不补 margin-notes 的决定合理，但缺口只记在会被归档的 proposal 里，另有一个缺口没被记
- **状态**：已采纳（记缺口）
- **处置**：margin-notes 无链路块的前提成立，且 review 指出还有第二个缺口（`event-story` 的 flow 侧只在已停写块里出现）。两处缺口已从「只记在会归档的 proposal」改为记入 `docs/research/process-arch-rework/issues.md` 的 ISS-14 段落。
- **位置**：`proposal.md:44,58`
- **问题**：「写新内容不在重排范围」成立，同意不补。但 (1) 残留只记在 proposal，归档后难找，ISS-14 没记；(2)「Section 可视化与三套匹配血缘」块在全部 spec 里都找不到对应 Requirement，是比 margin-notes 更大的缺口，未记；(3) 措辞「那一族」不准，它将来应是族二里的一块，不是第 4 族。
- **证据**：`grep -l 'SectionAnchorBadge\|fit_distance' openspec/specs/*/spec.md` 为空；`daily-report-margin-notes/spec.md` 8 条 Requirement，flow 里 `MarginNotesRail` 只在 `:245` 布局出现。
- **建议**：两条缺口一起记入 ISS-14 已知残留；proposal L44 的「族」改「块」。

## 核实记录

- **内容**：`git show HEAD~1:…` 与工作区做非空白行多重集对账，旧 434 行、缺失 21 行全为 `###` / `####` 标题；新增 52 行 = 4 行引言 + 3 族标题 + 3 `> spec:` + 6 层标题 + 21 改级标题 + 15 决策记录行。无段落改写、拆并或重复；9 个围栏块作为整体逐字相同且无未闭合。
- **红线**：约束节 md5 相同（`d2d652ad…`）；该节至文末与 L1–40 同 HEAD~1 逐字节相同；`## 需求说明` / `## 代码入口` / `## 变更溯源` 未动。
- **注入**：`constraint-digest.sh --brief --domains daily-report` 旧版（经 `CONSTRAINT_DIGEST_DOC_ROOT` 指向临时目录）与新版输出只有 3 行源路径不同。`extractRedlines`（`.pi/extensions/lib/constraint-redlines.ts:28-37`）只认顶层 `N. ` / `- ` 列表项的首个加粗块，`>` 引用行不匹配，`extractSection` 只取「业务约束与不变量」节——族内 `> **…**` 不会被误抓，`> spec:` 行也不会被路由吃掉。
- **链接**：全仓无 `daily-report.md#` 深链（只有 proposal 自己提到）。
- **门禁**：tasks §8 九项逐项复跑全过；`openspec validate restructure-flow-daily-report --strict` 通过；HEAD~1..HEAD 只动 `daily-report.md` 一个 flow 文档。
- **「只补空」两条**（`:364`）：禁令在当前规则、原因在同一行括号里，拆开要改字，不建议动。
- **族内块顺序**（管线 → 锚定 → 事件线 → 今日重点）与管线步骤一致，自然；标题去 change 名并移到「来源 change」行的做法本身好，7 个 change 名都保住了。
- **旧债不立条目**：`######` 用满 h6、管线 mermaid 仍画着「归属 → 跨期去重」、Step7.5 长引用段，同上一轮已采纳不改的几条。
- brief 写「重排后 618 行」，`wc -l` 为 617，无影响。审查全程未改仓库文件，临时文件在 `/tmp/dr-review/`。
