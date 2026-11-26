# 审查报告：merge-lane-ops-specs

- **审查者**：MiniMax M3.1 Flash（只读）
- **审查日期**：2026-11-26
- **结论**：需改后进
- **放行判断**：内容搬运与守恒都成立，但按现设计归档会在 `openspec archive` 阶段两次 Aborted（实测），且 lane-ops 的族级 domain marker 漏 topic-graph、会把「改这族必须读的红线」整域丢掉——两处补齐后可进。

---

## 发现

### R-1 2026-11-26【High】按现设计归档会中止：摘 domain marker 不够，lane-manual-correction 的表格与尾部散段同样会让 retire 路径拒绝删除
- **状态**：已修复
- **处置**：tasks 拆为 4.3a（摘 5 个 marker）+ 4.3b（清 lane-manual-correction 的 20 行表格，内容已在 lane-ops ADDED 里逐字保留），并注明「不要删空 spec」与「六项门禁不含 dry-run」。design 新增 D7 记录这个 openspec 归档陷阱
- **位置**：`openspec/changes/merge-lane-ops-specs/design.md` D5 第 2 条 / `tasks.md` 4.3、8.2；受影响文件 `openspec/specs/lane-manual-correction/spec.md`
- **问题**：D5 只安排了「归档前摘掉 6 个被退役 spec 的 `<!-- domain: -->`」，这**是必要但不充分**的。openspec 2.1.1 的 retire 路径在删除一个被清空的 main spec 前，会对**整份原文件**做一次「哪些行能被合并流程点名」的审计：能被点名的只有标题、`## Purpose` 段、`## Requirements` 头、以及每个 requirement 块自己的头 + 首段陈述 + scenario 子弹。**markdown 表格行**和**首段之后的额外散文段**都不在可点名范围内 → 判为「unaccounted content」→ 拒绝删除该 spec。`lane-manual-correction` 正好两类都有（20 行表格 + 1 段尾部散段）。`tasks.md` 没有任何任务覆盖这一步，4.3 做完仍会中止。
- **证据**（/tmp 副本实测，非预演）：
  1. 原样归档 → `Aborted. No files were changed.`，报 `'board-lane-dynamics' declares retire_capabilities, but the spec holds content the merge cannot safely account for and deleting the file would take with it: "<!-- domain: semantic-board, daily-report -->"`。
  2. 摘掉 6 个 domain marker 后再归档 → 仍 `Aborted.`，报 `'lane-manual-correction' ... ": "| 字段 | 含义 |", "|---|---|", "| `source` | ...", and 17 more line(s)`。
  3. 再把 `lane-manual-correction` 的 20 行表格删掉 → 仍 `Aborted.`，报 `"加回的边界口径：section 已属目标话题时 SHALL 幂等成功（200，不追加留痕、不重复置标记）；..."`（首段之后的尾部散文段）。
  4. 再删掉该散文段 → 归档成功：`Totals: + 38, ~ 0, - 38, → 0`，6 个旧 spec 目录全部删除，`openspec validate --specs` → `146 passed, 0 failed`。
  - 源码依据：`@fission-ai/openspec@2.1.1` `dist/core/specs-apply.js` 的 `contentTheMergeCannotName(parts)`（注释明写 "Deliberately whole-file"）与 `dist/core/archive.js` 的 `decideSpecOutcome` / `isRetirementCandidate`（`built.unaccountedContent.length === 0` 是 retire 的前置条件）。
  - 关键风险点：`scripts/harness/archive-readiness.sh` 的六项门禁（doc-impact / check-standards / check-tasks-tail / review-trace / ui-verification-trace / constraint-read-trace）**不含 dry-run 归档**，实测六项可全绿而归档仍中止 → 这个坑只会在 tasks 8.2 才暴露。
- **建议**：
  1. tasks 4.3 拆成两条：4.3a 摘 6 个 domain marker；**4.3b 清 `lane-manual-correction` 的 20 行表格 + 1 段尾部散文**（内容已逐字保留在 `lane-ops` 的 ADDED 块里，实测「加回的边界口径」一句在 lane-ops 中存在且逐字一致，删除不丢契约）。
  2. 其余 5 个被退役 spec 已核过**无表格行**（board-lane-dynamics / lane-trend-overview / lane-heat-metric / workshop-radar / workshop-article-archive 均为 0 行），无需处理。
  3. 归档前加一步 dry-run 验证（`openspec archive <change> --yes` 在副本上跑，或至少 `openspec validate --specs` 前先跑一次预演），把「归档会中止」从 8.2 提前到 4.x 暴露。
  4. 注意顺序：**不要**先把 `lane-manual-correction` 的 spec 删空再归档——实测那样 `counts.removed = 0`，`decideSpecOutcome` 会返回 `write` 而非 `retire`，归档同样中止（"REMOVED requirement ... is not in the current spec; treating it as already removed" + "Spec must have at least one requirement"）。必须保留 requirement 块、只删不可点名的行。

### R-2 2026-11-26【High】lane-ops 的族级 domain marker 漏 topic-graph：违反族级 marker 取舍判据，且会让该域红线整域查不到这一族
- **状态**：已修复
- **处置**：lane-ops marker 改为 `semantic-board, daily-report, topic-graph`（3 域，正好上限）；proposal 的 constraint-domains 同步补 topic-graph。design D5 补取舍说明：lane-manual-correction 原 marker 含 topic-graph，且族内有 topic-graph 红线 23/24/25 对应条款，按「改这族必须读哪些 flow 红线」判据不可省
- **位置**：`design.md` D5 第 3 条、`tasks.md` 8.3a；被吞掉的来源 `openspec/specs/lane-manual-correction/spec.md` 的 `<!-- domain: semantic-board, topic-graph -->`
- **问题**：D5 写「`lane-ops` → `semantic-board, daily-report`」，但 4 个成员的原 marker 并集是 **{semantic-board, topic-graph, daily-report}**（lane-manual-correction = `semantic-board, topic-graph`；board-lane-dynamics = `semantic-board, daily-report`；lane-trend-overview = `daily-report, semantic-board`；lane-heat-metric = `semantic-board`）。`topic-graph` 被静默丢弃，design D5 的括号只解释了 daily-report 的来由，对 topic-graph 一字未提。
  这不是「成员并集装不下」的取舍问题——3 域正好是 J 段上限。族级 marker 的判据写在 `docs/reference/standard/shared/doc-authoring.md` 第 55 行：「**族级 domain marker 按『改这族必须读哪些 flow 红线』取舍，不按『成员碰过哪些域』**」，而 lane-ops 里恰好躺着 topic-graph 红线 18/19/23/24/25 的对应条款：
  - topic-graph 红线 25「滚动窗口取数 SHALL 单点实现（`ListCentroidWindowSections`），质心计算与首义读端点 MUST 共用同一实现，且该读路径 MUST 零写副作用」→ 对应 lane-ops 的「滚动窗口成员取数的单点实现」；
  - topic-graph 红线 23「质心刷新范围 SHALL 为『当日 touched 话题 ∪ `centroid_pending_recompute=true` 的话题』」→ 对应「人工修正的派生值刷新时机」；
  - topic-graph 红线 24「手动建话题 SHALL 在建话题同一事务内追加一行首义留痕」→ 对应「人工重设泳道首义」。
  按判据，topic-graph 是「改这族必读」的域，不该被舍弃。
- **证据**：
  - `sed -n '1,8p'` 逐个读取 6 个被退役 spec 的 marker：lane-manual-correction = `semantic-board, topic-graph`（唯一带 topic-graph 的成员）。
  - `docs/reference/flow/topic-graph.md`「业务约束与不变量」第 23/24/25 条 + 第 18/19 条（`grep -n 'lane-manual-correction-loop' docs/reference/flow/topic-graph.md` 命中 18、19 两行）。
  - `bash scripts/harness/constraint-digest.sh --brief --domains topic-graph` → topic-graph 红线层共 25 条，含上述 23/24/25。
  - **实测（/tmp 归档后）**：`domain-history.sh topic-graph` 第 1 段「本域 main spec」里 `lane-manual-correction`（317 行）消失，`lane-ops` **不出现**（归档后 marker 全丢，见核实记录）；即使按 D5 补上 `semantic-board, daily-report`，topic-graph 域的现行契约清单里也永远不会出现这一族 → 「泳道归属变更」这一族在 topic-graph 域完全查不到。
- **建议**：
  1. `tasks.md` 8.3a 改为 `<!-- domain: semantic-board, daily-report, topic-graph -->`（3 域，正好触顶但合法）。
  2. design D5 补一句取舍说明：为什么留 semantic-board / daily-report / topic-graph 三个（主业务域 = 版块/泳道口径；次域 = 日报落库与派生缓存口径；topic-graph = 话题锚定/质心/滚动窗口口径），并注明「成员并集恰好 3 域，无舍弃」。
  3. change 自身的 `<!-- constraint-domains: semantic-board, daily-report -->`（proposal.md 头部）同步补 `topic-graph`——本次搬运的就是 topic-graph 域的契约，注入时必须能取到 topic-graph 红线层（见 R-9）。

### R-3 2026-11-26【Medium】「唯一口径出处」不成立：关注度/分档阈值条款在两族各写一遍，且 lane-ops 内部出现自指的死指针
- **状态**：已修复（部分）
- **处置**：① board-lane-dynamics 的「（热度口径见 lane-heat-metric）」已改为「（见本族『泳道当天热度口径』）」，消除同文件自指；② 分档阈值条款在 lane-ops 与 workshop-radar 各写一遍**登记不修**——改正文属语义变更，归入「跨族引用清扫」批次；③ workshop-views 已不存在，其死指针随之消失
- **位置**：`design.md` D2 第 1 条、D6 第 2 条；`openspec/changes/merge-lane-ops-specs/specs/lane-ops/spec.md` 第 3 行（Purpose）与第 395 行；`specs/workshop-views/spec.md` 第 32 行
- **问题**：三处叠加，使「lane-heat-metric 是唯一口径出处」这个说法在合并后**站不住**：
  1. **同一条款两族各写一遍**：「分档阈值 SHALL 为可配置项，且 SHALL 按新关注度量纲下的真实分布标定……MUST NOT 直接沿用事件线热度量纲下的 3.3/0.7，也 MUST NOT 沿用更早的 section 计数量纲下的 50/15」在 `lane-heat-metric`（→ lane-ops）和 `workshop-radar`（→ workshop-views）里各写了一遍；「关注度 = 各泳道 lane_heat 之和，保留一位小数」「低于阈值折叠为常温摘要」「分档不靠颜色表达」也是两处重复。合并后这组分档契约**横跨两个族**——正是本 change 想消灭的碎片形态被固化下来。
  2. **lane-ops 内部自指**：board-lane-dynamics 的「展示范围与排序」原文带「（热度口径与权重见 `lane-heat-metric`）」，逐字搬进 lane-ops 后变成 **同一个文件里指向一个已退役 capability 名**的指针（该文件第 702 行起就是「泳道当天热度口径」本体）。
  3. **workshop-views 指向死名**：radar 的「版块行构成与分档」带「热度口径与权重见 `lane-heat-metric`」，合并后指向已退役 capability；正确目标应改为 `lane-ops`。
- **证据**：
  - `grep -n 'lane-heat-metric' openspec/changes/merge-lane-ops-specs/specs/{lane-ops,workshop-views}/spec.md` → lane-ops:395、workshop-views:32（另 lane-ops 的 `<!-- from: -->` 溯源注释 3 处，属正常）。
  - 归档后实测：`openspec/specs/lane-ops/spec.md:389` 与 `openspec/specs/workshop-views/spec.md:32` 两处死指针落在 main spec 里（/tmp 副本）。
  - 条款重复对照：`lane-heat-metric`「版块关注度与分档」第 2 段 vs `workshop-radar`「版块行构成与分档」第 2 段，语义等价、措辞几乎相同。
  - 分档阈值的**标定值在两个 spec 里都没有**：flow 红线 29 写「P80/P20，当前 14.5 / 2.3」，而 `semantic-board.md` 第 27 行的 L0 雷达详述段写的是「分档阈值临时 10/3」——flow 文档自身两处不一致，spec 侧则完全没落数字。
- **建议**：
  1. 本 change 不改契约文本的约束下，允许做的最小修正是**只改指针**：`lane-ops` 内改成「（热度口径与权重见本文件『泳道当天热度口径』）」，`workshop-views` 改成「（见 `lane-ops`『泳道当天热度口径』）」。这两处是指针而非契约，改动不破坏「逐字搬运」不变量（需要把「38 条逐字一致」的口径放宽为「38 条契约文本逐字一致，2 处 capability 指针随合并改写」，并在 tasks 里写明）。
  2. 若坚持零文本改动，则至少在 tasks 7.4「跨族引用清扫」批次里**逐条登记**这 2 处新死指针（当前设计只登记了 board-console 一处，且描述有误，见 R-4）。
  3. 「关注度 + 分档阈值」整组条款二选一收口（建议收进 lane-ops，radar 侧留指针），并把 flow 红线 29 的 14.5/2.3 标定值落进 spec——否则 lane-ops 的 Purpose「唯一口径出处」这句自我声明要么删掉，要么补上真正唯一的出处。

### R-4 2026-11-26【Medium】跨族引用清单不完整，且 D2/D6 对「引用性质」的描述失准
- **状态**：已修复
- **处置**：跨族引用清单以本 review 的证据为准（lane-ops 内部 1 处已修、workshop-radar 1 处登记）；design D6 的「引用性质」描述已按实测更正
- **位置**：`design.md` D2 第 3 条、D6 第 2 条；`tasks.md` 3.2、7.3、7.4
- **问题**：design 与 tasks 都声称做过跨族引用检查，但清单不完整、且把「重复条款」误认成「指针」：
  1. **漏了两处 main spec 指针**：`openspec/specs/persistent-topic/spec.md:64`「（后者见 lane-manual-correction 能力）」、`openspec/specs/related-events/spec.md:77`「（见 `lane-heat-metric`「线索计数的内容身份去重口径」）」——两处都是明确的 capability 名引用，归档后成为死指针，而 design D6 与 tasks 3.2 只提了 workshop-board-console 一处。
  2. **workshop-board-console 的「泳道热度口径（线索数单一口径）」不是指针，是重复条款**：D6 写「它的『泳道热度口径』那条……合并后该引用仍指向 lane-ops 的对应条款，不会断」，但 board-console 第 267 行起是一条**完整自洽的规范条款**（矩阵日格 / 格色深 / 演进节点 / 行序 / 综合稿判据 / section 数不参与计算），没有任何 `见 lane-heat-metric` 字样。它与 lane-heat-metric 的「泳道当天热度口径」+「版块关注度与分档」构成同一口径的两份独立表述——D6 说「不会断」是对的，但原因不是「指针指向 lane-ops」，而是**它压根没指过**。这条重复正是 D3 用来论证「board-console 内聚、不该拆」的那条，它同时也是「矩阵排序要改就得开两个文件」的实锤。
  3. **文档侧的引用无需动**：`docs/reference/api/semantic-boards.md`、`docs/reference/api/daily-reports.md`、`docs/reference/database/tables/daily-report-watch.md`、`docs/reference/standard/frontend/interaction.md`、各 flow 文档里出现的 `lane-manual-correction` 全部是**变更名**（`lane-manual-correction-loop` 等）指向 archived change，归档后链接仍有效——tasks 7.3 扫描时需要区分「capability 名」与「change 名」，否则会产生一批假阳性的清扫条目。
- **证据**：
  - 全量扫描（`grep -rln` over `openspec/specs` + `docs` + `scripts`，排除 `openspec/changes/`）后逐条判定 capability 名 vs change 名；6 个旧名里，**真正以 capability 名被引用**的只有 4 处：persistent-topic:64、related-events:77、board-lane-dynamics:65（→ lane-ops:395）、workshop-radar:29（→ workshop-views:32）。
  - `grep -n '泳道热度口径' openspec/specs/workshop-board-console/spec.md` → 第 267 行整条 Requirement；同文件内无 `lane-heat-metric` 字样（`grep -n 'lane-heat-metric' openspec/specs/workshop-board-console/spec.md` 命中 0）。
- **建议**：
  1. tasks 7.3 的扫描结果按「capability 名 / change 名」两栏落盘，逐条给 file:line，别只给结论。
  2. 把 4 处 capability 名引用全部登记进「跨族引用清扫」批次，并在 design D6 里改写为事实描述（含 persistent-topic、related-events 两处，以及 board-console 那条是重复条款而非指针）。
  3. 更彻底的做法：把 board-console 的「泳道热度口径（线索数单一口径）」降为指针（指向 lane-ops），从而让「唯一口径出处」真正唯一——但这属于改 spec 契约文本，需单独 change，不在本 change 范围内。

### R-5 2026-11-26【Medium】workshop-radar 与 workshop-article-archive 的归族依据不成立：两者零交叉引用，且归档视图实为 board-console 页面内的同级视图，不是「平级视图」
- **状态**：已修复
- **处置**：**切法已改**：不再新建 `workshop-views`，改为 `workshop-article-archive` 并入 `workshop-board-console`（就地扩容，1106 行）——archive 原文写着「与『矩阵』同层切换」，是同一页面内的视图；`workshop-radar` 保持独立（跨版块首屏，与被并者双向 grep 零命中）。proposal/design/tasks/PURPOSE 字典已同步
- **位置**：`proposal.md`「What Changes」表与判据第 2 条、`design.md` D1 表格第 2 行、8.3b 对应的 `scripts/harness/merge-spec-family.sh` PURPOSE 字典 `workshop-views` 条
- **问题**：归族理由（「两者都是叙事工坊的平级视图，同一个 change 创建，共享版块行/文章行的入口与跳转联动」）只有「同一个 change 创建」这一条可查证，其余两条与 spec 文本不符：
  1. **不是平级视图**：radar 是 `/tags` **未选中版块时**的首屏（跨版块列表，点版块行 → 路由跳 `/tags/:boardId` 渲染矩阵视图）；archive 是**版块态势页内**的视图——原文「版块态势页 SHALL 提供『归档』视图，与『矩阵』同层切换」。两者一个在版块之上、一个在版块之内。
  2. **零交叉引用**：radar 全文不提「归档」，archive 全文不提「雷达」（双向 grep 命中数均为 0）。真正与 archive 耦合的是 workshop-board-console：board-console 的矩阵条款明写「『打开当日日报』『查看归档』两个既有动作 SHALL 保留可达」，且 board-console 的面包屑条款明写「路由 SHALL 回到 `/tags` 并渲染雷达」。
  3. 真实耦合拓扑是 **radar ← board-console → archive**（board-console 在中间），本 change 把拓扑两端的两个视图捆成一个族，把中间那个留在一族之外——新族内部没有任何共享条款。
  4. 该错误断言会被**逐字写进 main spec**：merge-spec-family.sh 的 PURPOSE 字典里 workshop-views 写「叙事工坊的**两个平级视图**……**两者共享版块行/文章行的入口与跳转联动**」——这是 archive 时进入 `openspec/specs/workshop-views/spec.md` 的 Purpose 全文，且是读者进这个族看到的第一段话。
- **证据**：
  - `grep -n '归档' openspec/specs/workshop-radar/spec.md` → 0 命中；`grep -n '雷达' openspec/specs/workshop-article-archive/spec.md` → 0 命中。
  - `openspec/specs/workshop-article-archive/spec.md:10`「版块态势页 SHALL 提供『归档』视图，与『矩阵』同层切换」；`openspec/specs/workshop-board-console/spec.md:69`「『打开当日日报』『查看归档』两个既有动作 SHALL 保留可达」；`openspec/specs/workshop-board-console/spec.md:11-16` 面包屑回 `/tags` 渲染雷达。
  - 两者的 `<!-- from: -->` 溯源注释都写 `2026-11-02 rework-workshop-into-radar-and-desk` → 「同一 change 创建」属实。
- **建议**：
  1. 最小修法（保 6→2 不变）：把 workshop-views 的 Purpose 改成事实描述——「① 雷达首屏：`/tags` 未选中版块时的跨版块视图；② 文章档案：**版块态势页内**与『矩阵』同层的归档视图（入口在矩阵历史格『看归档』）」，并删掉「两个平级视图」「共享入口与跳转联动」这两句无据断言。归族理由改为「同一 change 创建、同属叙事工坊、都是工坊的对外读视图」，并明说「两者之间无直接契约耦合」——不改判据也把话说准。
  2. 备选切法（值得评估，见总评 E-3）：`workshop-board-console` + `workshop-article-archive` 同族（同一页面内的两个视图，实测 861 + 245 = 1106 行，仍在 1200 以内），radar 单独或并入 lane-ops（radar 的关注度分档消费 lane-heat-metric 的口径）→ 泳道组 9 个 spec 收敛成 4 族而非 5 族。本 change 若改切法需重跑 delta 生成，代价高于改文案。

### R-6 2026-11-26【Medium】「1200 预警线」在仓库里查无出处，design D3 还把出处误引成 D5
- **状态**：已修复
- **处置**：design D3 的「1200 预警线」改为引用 doc-authoring 的族级约定（单族 >1200 行且含 ≥3 个不相干功能考虑拆、>1500 必须拆），不再自指 D5
- **位置**：`proposal.md`「不做的事」第 3 条、`design.md` D3 第 1 句
- **问题**：不并 `lane-correction-surface`(718) + `workshop-lane-compose`(567) 的**唯一量化理由**是「合并后 1300 行超 1200 预警线」，但：
  1. 全仓找不到这条 1200 预警线。`docs/reference/` 全域、`openspec/specs/`、`scripts/harness/` 全仓 grep `1200` 的命中只有 `docs/v1.3-narrative-overhaul/changes/database-docs-global-perspective/design.md` 一处——那是 2025 年一篇讲 `DATABASE_FIELDS.md` 的旧设计稿，与 spec 族规模无关。
  2. design D3 写「超 **D5** 的 1200 预警线」，但 D5 是「归档机制」，里面没有任何行数阈值——这是**同文档内的自引错误**。
  3. 真实体量也没有 1300：718 + 567 = **1285**（`wc -l`）。
- **证据**：`grep -rn --include='*.md' --include='*.sh' -E '\b1200\b' docs/ openspec/specs/ scripts/ AGENTS.md` → 仅 1 条历史无关命中；`wc -l` 实测两 spec 为 718 / 567。
- **建议**：
  1. 要么把 1200 预警线**补进规范文档**（`docs/reference/standard/shared/doc-authoring.md` 或 `开发执行规范.md`）并给出出处，再在 design 里按规范引用；要么把 D3 的理由改写成不依赖阈值的表述：「两支画布是各自完整的工作面（编排态 vs 修正态），共享的只有外壳 3~5 条；合并后单文件近 1300 行会超过本仓库单 spec 的实际体量上限（当前最大 main spec = workshop-board-console 861 行 / tag-matching 1136 行）」——用**仓库现状**而不是一条无出处的数字来立论。
  2. 顺带把「约 3 条 80 行」改成实测值：共享项实际是 吸顶标签列（lane-correction-surface 的 2 条 + workshop-lane-compose 的 1 条）+ 高度贴合容器（2 条）+ 缩放与滚动外壳同源（1 条），合计 **5~6 条、约 125 行**，不是 3 条 80 行。数字落在 80 行还是 125 行不改变结论，但会影响「共享条款占比只有 6%」这种说法的可信度。

### R-7 2026-11-26【Low】两族行数三份口径互不相同，且都不是归档后的实际体量
- **状态**：已采纳
- **处置**：三份行数口径（来源 spec 之和 / delta 行数 / 归档后实测）已在 tasks 与 design 注明；归档后以实测为准
- **位置**：`proposal.md`「What Changes」表（lane-ops 778 行 / workshop-views 465 行）、`tasks.md` 2.2（lane-ops 817 行 / workshop-views 499 行）
- **问题**：同一批行数在两处给出两个值，且都不是实际值。实测（/tmp 归档后 main spec）：**lane-ops = 798 行 / 23 req / 102 scenario，workshop-views = 491 行 / 15 req / 66 scenario**；两份 delta 文件本身是 815 / 497 行。proposal 的 778 / 465 是 4+2 个**源 spec 的行数之和**（源 spec 用「文件末尾无换行 → 行数 +1」的约定：317+247+107+103 = 774、218+245 = 463，再按 +1 惯例凑成 778/465），tasks 的 817/499 则近似 delta 文件长度。
- **证据**：`wc -l` 与 `awk 'END{print NR}'` 对 6 个源 spec、2 份 delta、归档后 2 个 main spec 的实测值；`awk '/^### Requirement:/'` 与 `/^#### Scenario:/'` 计数 = 23/102 与 15/66。
- **建议**：把 proposal 表与 tasks 2.2 的行数统一成**归档后 main spec 的实测值 798 / 491**（或直接删掉行数列，只留 req / scenario 数），并注明「行数按归档后 main spec 计」。同批 proposal 表里 9 个 spec 的行数（318/248/108/104/219/246/877/728/572）也整体比 `wc -l` 多 1，「9 个 spec 共 3392 行」实际为 3383。

### R-8 2026-11-26【Low】business / impl 分类 38 / 0：判定成立，无漏判
- **状态**：已采纳
- **处置**：分类 38/0 经审查确认无漏判，留档
- **位置**：`spec-classification.txt`（38 行全 business）、`design.md` D4 预判清单
- **问题**：无（记录核实结论，供 D4 预判销账）。三条重点核过：
  1. **「滚动窗口成员取数的单点实现」判 business 成立**——标题里的「单点实现」指的是**契约层面的唯一性要求**（质心计算与首义读端点 MUST 共用同一实现、MUST NOT 出现第二份窗口筛选口径、读路径 MUST 零写副作用），不是施工记录；正文里出现的是配置项名 `persistent_topic_centroid_window`（默认 30）这类**契约参数**，不是文件路径 / 函数名 / 表结构。
  2. **「泳道动态聚合端点」「版块总览聚合端点」判 business 成立**——两者都是端点契约：请求形状 + 响应字段语义 + 降级语义（`data_unavailable` / `null` 未知 ≠ 0 / `other_topics` 真实零 / 不完整清单标注）。端点存在本身不是实现说明，判 impl 才会错。
  3. **无施工记录漏判**：对两份 delta 全文 grep 实现特征（`.go` / `.tsx` / `internal/` / `backend-go` / `front/` / `Handler` / `func (` / `Repository` / `.sql`）→ **0 命中**；grep 施工记录词（`已实现` / `施工记录` / `重构前` / `原实现` / `TODO`）→ 仅命中「本次质心刷新」「本次会话」「本次数据未取全」等业务语义用法，非施工记录。6 个源 spec 本身 `kind: impl` 标记数均为 0，合并后无需 impl 段。
- **证据**：`grep -cE '^\s*\|' openspec/specs/{6 个源}/spec.md` → 0（无表格化施工记录）；两份 delta 的实现特征 grep 均为 0 命中；`spec-classification.txt` 行数 38 = lane-ops 23 + workshop-views 15，格式与 `archive/2026-11-26-merge-topic-specs/spec-classification.txt` 同构。
- **建议**：D4 的两条预判（「滚动窗口成员取数的单点实现」偏 impl、「泳道动态聚合端点」偏 interface）在 tasks 3.1 处显式销账为「判 business，理由：契约级唯一性要求 / 端点契约含降级语义」，避免后续 change 再翻案。

### R-9 2026-11-26【Low】change 自身的 `constraint-domains` 声明漏 topic-graph
- **状态**：已修复
- **处置**：proposal 头部 `constraint-domains` 补 `topic-graph`
- **位置**：`proposal.md` 头部 `<!-- constraint-domains: semantic-board, daily-report -->`
- **问题**：本 change 搬运的 lane-manual-correction 5 条要求属于 topic-graph 域（红线 18/19/23/24/25），但 change 头部只声明了 semantic-board + daily-report，实现/评审阶段的约束注入取不到 topic-graph 红线层。
- **证据**：`docs/reference/flow/topic-graph.md` 第 184/185 行（红线 18/19）明写「（lane-manual-correction-loop）」；`constraint-digest.sh --brief --domains topic-graph` 确认该域红线层存在且含 23/24/25。
- **建议**：proposal 头部改为 `<!-- constraint-domains: semantic-board, daily-report, topic-graph -->`（`stage.md` 头部同理，只增不减）。

### R-10 2026-11-26【Low】tasks 缺项清单（补进 tasks.md 即可，不影响制品内容）
- **状态**：已修复
- **处置**：tasks 缺项已补（4.3b 清表格、8.3a/b marker 更正、4.5 数字 146、5.4 未触碰清单）
- **位置**：`tasks.md` 第 4/7/8 节
- **问题**：对照 R-1/R-2/R-3/R-4/R-5，下列动作在 tasks 里没有对应条目：
  1. 4.3b：清 `lane-manual-correction` 的表格 + 尾部散段（R-1，**缺了归档必中止**）。
  2. 8.3a 的 marker 值修正为含 topic-graph（R-2）。
  3. design D5 补族级 marker 取舍说明（R-2）。
  4. 7.3 的扫描结果分「capability 名 / change 名」两栏落盘，4 处 capability 名引用逐条登记（R-4）。
  5. 2 处族内/跨族死指针的改写或登记（R-3）。
  6. `workshop-views` 的 PURPOSE 文案修正（R-5）——注意该文案在 `scripts/harness/merge-spec-family.sh` 的 PURPOSE 字典里（本次已改，见 `git diff scripts/harness/merge-spec-family.sh`），修正要改字典而不是 change 内的 delta。
  7. proposal 表 / tasks 2.2 的行数口径统一（R-7）。
- **证据**：逐条比对 tasks.md 第 4/5/7/8 节与本报告发现；`git diff scripts/harness/merge-spec-family.sh` 显示两族 PURPOSE 已写入共享脚本字典。
- **建议**：按上列 7 条补进 tasks.md；其中第 1 条是归档硬阻塞，其余为质量项。

---

## 逐项结论（对应审查重点）

### A. 「不合并」的判断

- **A-1 不并 workshop-board-console（861 行 / 25 req）——判断成立，保留。** 关键事件视图侧实测是 **12 条 / 396 行**（第 466 行起），不是 brief 估的 6 条 254 行；矩阵侧 13 条 / 456 行。两视图的耦合是**状态级**的，不是措辞级：关键事件视图条款明写「两视图 MUST 共用同一条观察轴（同一时间视窗控件与同一份 `visibleAxis`）、同一份持久化话题行序与受控选中状态（`selectedLane` / `selectedDate`）；切换视图 MUST NOT 重置其中任何一项」「写入与矩阵同一个 `selectedDate`，MUST NOT 维护第二份选中日状态」「按矩阵行序，含休眠分组」。拆开会把这组共享状态契约变成跨族指针——而 workshop-lane-compose 已经示范了跨族指针维护成本有多高（吸顶标签列那条）。另有一个反向信号：board-console 的最后一个 Requirement「矩阵其他话题整行」（第 824 行）是矩阵侧条款却排在 12 条关键事件条款之后，说明这份 spec 已经是两段混排。**建议**：保留不拆，但把「下一批拆分候选」写成一个有触发条件的待办（拆点＝矩阵侧 13 条 / 关键事件侧 12 条；触发＝共享状态契约先抽成独立条款再拆），而不是「等它长到 1000+」——861 行的 spec 再加 6 条矩阵口径就会触发，而触发时共享状态条款还没抽出来。
- **A-2 不并 lane-correction-surface(718) + workshop-lane-compose(567)——结论成立，但理由要重写（R-6）。** 「1300 超 1200」这条线查无出处、且 D3 误引 D5；真实体量 1285。真正的理由应该是：两者是**两个各自完整的工作面**（编排态 vs 修正态），共享的只是外壳（吸顶标签列 / 高度贴合容器 / 缩放滚动外壳同源，实测 5~6 条约 125 行，占比 ~9%），而 `workshop-lane-compose` 已经有一条「吸顶标签列是两支画布共用的单元」把跨族共享表达清楚了。design D3 提出的「未来抽 canvas-shell（约 100 行）」比合并两个工作面更对路。**唯一要改的是把无出处的行数阈值换成可复现的判据**。
- **A-3 lane-heat-metric 归 lane-ops——归族可以，但「改矩阵排序要跨两个文件」这个担心成立，且合并后已经成立。** 实测：矩阵行序公式（`lane_heat` 降序 → 全窗线索合计降序 → topic id 升序）写在 **workshop-board-console:116 与 :269**，`lane-heat-metric` 自己**并没有**写行序公式（只在 Purpose 里声称自己是「泳道行序与矩阵排序的唯一口径出处」）；board-lane-dynamics 的「展示范围与排序」（→ lane-ops:395）才是第三个出处。所以矩阵排序今天就要开 2 个文件（board-console + 矩阵口径），合并后 lane-heat-metric 进 lane-ops，**矩阵排序仍然要开 2 个文件**（board-console + lane-ops）。结论：把 lane-heat-metric 放进 lane-ops **不解决**矩阵排序的跨文件问题，只是把它挪了个位置；D2「放视图族会变成矩阵族里的口径条款」的顾虑成立（board-console 自己已经有一份口径条款），但 D2 给出的理由（口径定义不是视图）不足以回答「改矩阵排序」这个问题。**建议**：不改归族（D2 的取舍可接受），但把 D2 的论证改成「矩阵排序口径的落点问题单独记为待办」——要么把行序公式也收进 lane-ops 并让 board-console 降为指针（R-4 建议 3），要么删掉 lane-ops Purpose 里「唯一口径出处」这句自我声明。二选一，不能既声明唯一又承认三处并存。
- **A-4 workshop-radar 与 workshop-article-archive 同族——判断不成立（R-5）。** 两者之间零交叉引用、不是平级视图、真实耦合在 board-console 上。建议至少改正 Purpose 与 design 措辞；更优切法见 E-3。

### B. business / impl 分类（38 / 0）

- 成立，无漏判。逐条依据见 R-8。design D4 的两条预判建议在 tasks 3.1 显式销账。

### C. 归档可行性

- **C-1 六个被退役 spec 的 `<!-- domain: -->`：6/6 都有 marker，无「已无 marker」的。** lane-manual-correction = `semantic-board, topic-graph`；board-lane-dynamics = `semantic-board, daily-report`；lane-trend-overview = `daily-report, semantic-board`；lane-heat-metric = `semantic-board`；workshop-radar = `semantic-board`；workshop-article-archive = `semantic-board`。6 个都要摘，design D5 第 2 条的数量与对象正确。**但摘完还不够**——`lane-manual-correction` 还有 21 行不可点名内容会挡住 retire（R-1）。
- **C-2 两族 marker 方案**：`workshop-views → semantic-board, ui` 与仓库既有惯例一致（`tagging -> semantic-board, ui` 是同一形状；全仓 26 个 spec 用 `ui` 作域），**采纳**。`lane-ops → semantic-board, daily-report` **不采纳**：成员并集是 3 域（semantic-board / daily-report / topic-graph），按 `doc-authoring.md` 第 55 行「按改这族必须读哪些 flow 红线取舍」，topic-graph 是必读域（红线 18/19/23/24/25 逐条对应本族 5 条要求），3 域正好不触顶，无舍弃理由（R-2）。
- **C-3 marker 会不会保留——实测结论：不会，全丢。** 实测方式：`cp -a openspec /tmp/arc-test/`（openspec 2.1.1，`openspec --version` = 2.1.1），在副本上跑真实 `openspec archive merge-lane-ops-specs --yes`。结果：
  - 归档成功后 `openspec/specs/lane-ops/spec.md` 与 `openspec/specs/workshop-views/spec.md` 的**头部 8 行内没有任何 `<!-- domain: -->`**（文件从 `# lane-ops Specification` 直接开始）。源码依据：`dist/core/specs-apply.js` 的 `buildSpecSkeleton()` 生成新 main spec 时只写 `# <name> Specification\n\n## Purpose\n\n<purpose>\n\n## Requirements\n`，**不携带 delta 的任何头部注释**。
  - `check-standards.sh` J 段在归档后报：`[WARN] 全仓 146 个 main spec 中 2 个缺 domain marker（domain-history 会漏查）`。
  - 所以 **tasks 8.3a / 8.3b 是必做项，不是可选项**——本报告已实测确认，任务表里的写法是对的。
  - 补充实测：`domain-history.sh semantic-board` 第 6 段在归档后确实列出了指向 6 个被退役 capability 的孤儿 delta（tasks 5.1 的预期成立）；第 1 段「本域 main spec」在补 marker 前不含 lane-ops / workshop-views，补完即出现。
  - **同时实测到的既有基线异常（非本 change 造成，但会影响「146 全过」的判读）**：当前工作树相对 HEAD 有 9 个 main spec 已删未提交（board-article-api / board-editing-ui / board-management-api / board-upgrade-expand / daily-report-match-fallback / radar-pipeline-status / topic-key-event-markers / upgrade-candidate-time-window / watch-materialized-topic）。工作树实际 spec 数 = 150（HEAD = 159）。本 change 归档后 = **146**，与 proposal 一致；但提交时这 9 个删除必须一并带上，否则 master 上会是 155。

### D. 遗漏

- **D-1 6 个旧 capability 名的引用清单**（完整版见 R-4）：main spec 侧 **4 处**以 capability 名引用——`persistent-topic:64`、`related-events:77`、`board-lane-dynamics:65`（→ lane-ops:395）、`workshop-radar:29`（→ workshop-views:32）；`docs/` 侧的命中全是**变更名**（`lane-manual-correction-loop` 等）指向 archived change，归档后仍有效，无需动。**brief 里点名的「lane-heat-metric 被 workshop-board-console 引用」经核实不成立**：board-console 全文 0 次出现 `lane-heat-metric`，它的「泳道热度口径（线索数单一口径）」是**独立重复条款**而非指针。
- **D-2 `lane-correction-surface` 的 TBD Purpose——不影响 domain-history，也不阻断归档。** 实测：`domain-history.sh` 只读 `<!-- domain: -->` marker，与 Purpose 无关；`openspec validate lane-correction-surface` → `Specification 'lane-correction-surface' is valid` + 一条 Purpose 占位符 **WARNING**（非 ERROR），全仓 `validate --specs` 同样只是 warning，且同类 warning 有 10+ 条（TBD Purpose 是全仓普遍现象）。design D6 把它登记为内容问题、tasks 3.3 标注「本次不修」，处置得当。补充一条：`lane-manual-correction` 的 Purpose **也是** TBD（`TBD - created by archiving change lane-manual-correction-loop`），但它随 spec 一起被退役，archive 过程中会额外打一条 Purpose 占位符 warning（不影响成功），D6 只提了 lane-correction-surface，建议在 D6 里把两者并列为「历史遗留 Purpose 占位符」。
- **D-3 tasks 该做没做的**：见 R-10（7 条）。

### E. 风险

- **E-1 lane-ops 798 行 / 23 req（实测）——体量合理。** 23 条 = 人工改 5（首义重设 / 节点归属变更 / 派生值刷新时机 / 留痕可追溯 / 滚动窗口单点实现）+ 动态视图 9 + 趋势区块 6 + 热度口径 3，主题自洽（改 / 看 / 量），比 board-console 的 25 条窄。**但两条要注意**：(1) 它把「人工修正」的数据语义与「读视图」放在一起，与 `lane-correction-surface`（修正工作面的 UI 契约）之间只靠 Purpose 一句「画布见 workshop-lane-compose / lane-correction-surface」维系，符合本仓库既有的跨族指针形态，可接受；(2) 文件里同时有「修正」的写语义和「读」的量纲，规模再涨就会逼近需要拆分的临界点，建议在 lane-ops 的 Purpose 里写一句「超过 N 条/行即评估拆出 lane-metrics」的触发条件。
- **E-2 只减 4 个 capability（150 → 146）——单开一个 change 值得，但价值不在「减 4」这个数字上。** 值得的理由是：① 它是 ISS-04 的第 6 族（`docs/research/process-arch-rework/issues.md` 第 111 行「剩余：泳道画布 9 个」），有明确排期；② 前 4 族（merge-tag-spec-family / merge-board-diagnostic-specs / merge-board-upgrade-api-specs / merge-topic-specs）都建立了同一套机制与验收口径，本族沿用是低风险复刻；③ 本仓 150 个 spec 里 126 个 < 300 行（平均 154 行，ISS-13 实测），碎片化是真问题，6→2 是这条线上的一步。**不值得的理由**（需要如实记下）：本 change **一行契约都没减少**，只搬了行、多了 2 段 Purpose；同时还新增了 2 处族内/跨族死指针、1 处族级 marker 取舍错误、1 处归档硬阻塞。**净收益主要体现在导航面，而代价集中在归档现场**。建议在 proposal 的 Impact 段把收益口径写准（不是「spec 数量 150 → 146」，而是「泳道/工坊域的读入口从 9 个收敛到 5 个」），并把 R-1/R-2 的修补成本计入本 change 的工时。
- **E-3 更好的切法？** 泳道/工坊 9 个 spec 的可能切法：
  1. **本 change 的切法（9 → 5）**：lane-ops(4) + workshop-views(2) + board-console + correction-surface + lane-compose。问题见 A-4（radar/archive 无耦合）。
  2. **9 → 4**：把 `workshop-board-console`(861) + `workshop-article-archive`(245) 并成「版块态势页」族（**1106 行**，实测同一页面内的矩阵视图 + 归档视图，实耦合最强），radar 并入 lane-ops（radar 消费 lane-heat-metric 的关注度口径，798+218 ≈ 1016 行）；correction-surface + lane-compose 保持两族。得到 4 族：lane-ops(1016) / board-page(1106) / correction-surface(718) / lane-compose(567)。**这是比本 change 更贴「改 A 必动 B」判据的切法**，代价是 radar 的族归属要重判、delta 要重跑。
  3. **9 → 3**：`board-page` 再吃 radar → 1324 行，越过任何合理的单文件上限，不建议。
  4. **把两支画布合成 canvas 族**：1285 行 + 共享条款只有 5~6 条，合并后反而制造「两个工作面 + 6 条外壳条款」的怪异结构，不如 design D3 提的「抽 canvas-shell」。
  **建议**：本 change **不改切法**（delta 已生成、守恒已验证、改切法要重跑全链），但把「9 → 4 的备选切法」写进 design 的「后续」一节作为下一批候选——因为 A-1 提到的 board-console 拆分触发点（共享状态契约先抽出）恰好也是这条备选切法的前置条件，两件事可以合并做。

---

## 核实记录（只读验证，均已实跑）

| 验证项 | 命令 / 方法 | 结果 |
| --- | --- | --- |
| ADDED 逐字一致 | 自写 python：按 `### Requirement:` 切块，归一化空白后逐条比对 6 个源 spec 与 2 份 ADDED | 38 / 38 一致，**0 处差异**；标题集合相同；Scenario **168 = 168** |
| delta 结构 | `openspec validate merge-lane-ops-specs` | valid |
| 归档实测（marker 会否保留） | `cp -a openspec /tmp/arc-test/` → `openspec archive merge-lane-ops-specs --yes`（openspec **2.1.1**） | **新 main spec 无 domain marker**（`buildSpecSkeleton` 不带 delta 头部注释）；`check-standards` J 段报「146 个中 2 个缺 domain marker」 |
| 归档实测（retire 阻塞） | 同上副本，逐轮补前置条件 | ①原样→abort（domain marker）；②摘 6 marker→abort（lane-manual-correction 表格 17+ 行）；③删表格→abort（尾部散文段）；④删散文段→**成功**（+38/-38，6 目录全删，`validate --specs` 146 passed / 0 failed） |
| 归档后 spec 数 | `ls openspec/specs/*/spec.md \| wc -l`（副本） | 150 → **146**（150 − 6 + 2） |
| 归档后族体量 | `wc -l` / `grep -c` | lane-ops **798 行 / 23 req / 102 scenario**；workshop-views **491 行 / 15 req / 66 scenario**（delta 815 / 497 行） |
| 域 marker 并集 | 逐个读 6 个源 spec 头部 8 行 | lane-manual-correction = `semantic-board, topic-graph`（唯一带 topic-graph 者）；并集 = semantic-board + daily-report + **topic-graph** |
| topic-graph 红线归属 | `docs/reference/flow/topic-graph.md` 161-193 行 + `constraint-digest.sh --brief --domains topic-graph` | 红线 18/19/23/24/25 逐条对应 lane-manual-correction 的 5 条要求（红线 25 = 「滚动窗口取数的单点实现」） |
| domain-history 前后对比 | 真实仓（归档前）vs /tmp 副本（归档后）`domain-history.sh topic-graph` | 第 1 段：归档前有 `lane-manual-correction` 317 行；归档后消失，`lane-ops` 不出现（补 marker 前） |
| domain-history 孤儿 delta | /tmp 副本 `domain-history.sh semantic-board` 第 6 段 | 列出了指向 6 个被退役 capability 的孤儿 delta（tasks 5.1 预期成立） |
| radar ↔ archive 耦合 | 双向 `grep -n '归档' workshop-radar` / `grep -n '雷达' workshop-article-archive` | 命中数 **0 / 0**；archive 自述「版块态势页 SHALL 提供『归档』视图，与『矩阵』同层切换」 |
| board-console 侧耦合 | `grep -n '归档' workshop-board-console` / `grep -n '雷达' workshop-board-console` | 「查看归档」动作可达（:69）、面包屑回雷达渲染（:11-16） |
| 旧 capability 名引用 | `grep -rln` over `openspec/specs` + `docs` + `scripts`（排除 `openspec/changes/`） | capability 名 **4 处**（persistent-topic:64、related-events:77、board-lane-dynamics:65、workshop-radar:29）；`docs/` 侧全是**变更名**引用 |
| board-console 是否引用 lane-heat-metric | `grep -n 'lane-heat-metric' openspec/specs/workshop-board-console/spec.md` | **0 命中**——它那条是独立重复条款，不是指针 |
| 1200 预警线出处 | `grep -rn -E '\b1200\b' docs/ openspec/specs/ scripts/ AGENTS.md` | 仅 1 条历史无关命中（2025 年 DATABASE_FIELDS.md 设计稿）；规范/脚本中**无此阈值** |
| 分类完整性 | 两份 delta grep 实现特征（`.go`/`.tsx`/`internal/`/`Handler`/`func (`/`.sql`）与施工记录词 | 实现特征 **0 命中**；6 个源 spec `kind: impl` 计数均为 **0** |
| lane-correction-surface 的 TBD Purpose | `openspec validate lane-correction-surface` | valid + Purpose 占位符 **WARNING**（非 ERROR）；全仓同类 warning 10+ 条；不阻断归档、不影响 domain-history |
| 归档就绪六道门禁 | `bash scripts/harness/archive-readiness.sh merge-lane-ops-specs` | ① doc-impact ✓ ② check-standards ✓ ③ tasks-tail ✓ ⑤ ui-trace ✓ ⑥ constraint-read ✓ ⑦ feedback ✓；**仅 ④ review-trace 红**（本报告 review.md 落盘后转绿）。**六道门禁不含 dry-run 归档 → 测不出 R-1 的阻塞** |
| 域 marker 规范判据 | `docs/reference/standard/shared/doc-authoring.md` 第 55 行 | 「族级 domain marker 按『改这族必须读哪些 flow 红线』取舍，不按『成员碰过哪些域』……已知代价：被舍弃的次域查询不到该族」 |
| 工作树基线异常（非本 change 造成） | `git status --porcelain` + `git ls-tree -r HEAD` | 9 个 main spec 已删未提交（HEAD 159 → 工作树 150）；本 change 归档后 146；**提交时须一并带上这 9 个删除** |

> 说明：本报告只写入 `openspec/changes/merge-lane-ops-specs/review.md` 一个文件，未修改任何实现文件、spec 正文、脚本或 main spec。归档实测在 `/tmp/arc-test` 副本中进行，仓库工作树未受归档影响。
