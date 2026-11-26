# 审查报告：merge-tag-spec-family（标签族 spec 合并）

- **审查者**：glm-5.4（只读）
- **结论**：需改后进
- **放行判断**：条数与正文守恒做得干净（133 条全量逐字一致、零丢失），但按 tasks 现状归档跑不通——22 个被清空的 capability 需要 `.openspec.yaml` 的 `retire_capabilities: true` 才会退役，缺它 `openspec archive` 会在校验处中止；另有 2 条行为契约（composite_hit 产生条件、merge embedding 缓存失效）被误标为 impl。

## 总评

| 清单项 | 结论 | 要点 |
|---|---|---|
| A 守恒 | 通过 | ADDED 55+23+55=133，REMOVED 合计 133；133+15=148；22/22 个 REMOVED 条数等于各自 spec 的 Requirement 数；Scenario 150+61+170=381，加 auxiliary-label 留在原位的 34 条 = 415 |
| B 语义无改动 | 通过 | 未抽查，做了全量：133/133 条与原文逐字一致（仅块首尾空行不同）；无正文重复、无行级近似重复 |
| C 来源注释 | 部分不实 | 6 个零引用 capability 确未编造日期；16 个带来源中 14 个指向创建它的 ADDED delta，2 个把「首个 MODIFIED 它的 change」当来源，殃及 7 条（R-4） |
| D business/impl | 2 条误判 | 116/32 与 design 一致；32 条 impl 全部读过，2 条是行为契约（R-2），1 条后置条件只在 impl 段（R-3） |
| E 就地扩容 | 结构通过，Purpose 不落地 | ADDED 23 条不含自身、与原 15 条标题零交集，归档后 38 条；族级 Purpose 被 openspec 忽略（R-5） |
| F 格式与归档 | 归档会中止 | validate（非 strict）通过；22/22 REMOVED 标题与 main spec 逐字同序一致；缺 retire 授权（R-1） |
| G 门槛与遗漏 | 4 项缺口 | retire 授权 / aux Purpose / 12 处跨文件引用无任务（R-8）/ marker 取舍未论证（R-6）；孤儿 delta 影响被低估（R-7） |
| H 风险 | 见右 | 不在 git 的东西（R-9）、归族切法与阈值（R-13、R-14） |

红线复核（brief 附带的 1–8）：红线 1–5 的关键词在合并后的可搜索面里都在（三级去重/addAlias 在 auxiliary-label 原 15 条，composite_hit/direct_hit/MaxBoards/mismatch/max_sim 在 tag-matching）。红线 6/7/8（suggestion_hash、dismiss 冷却、观察池）属于 `board-upgrade` spec，不在本次 23 个成员内，合并前后都不受影响。

## 发现

### R-1 2026-11-26【High】归档缺 retire 授权，`openspec archive` 会中止；tasks 4.3 的「空壳清理」流程与 openspec 2.1.1 实际行为不符
- **状态**：已修复
- **处置**：新增 `.openspec.yaml`（`retire_capabilities: true`）；tasks 4.0 / 4.3 改为「归档后断言已自动退役，不留空壳」；design D7 补退役授权说明
- **位置**：`openspec/changes/merge-tag-spec-family/tasks.md` 8.2、4.3；change 目录（缺 `.openspec.yaml`）；design.md D7
- **问题**：22 个旧 capability 的 Requirement 被 REMOVED 清空后，openspec 2.1.1 默认走「写入重建稿 → 校验」，重建稿因「no requirements」校验失败而整次中止。只有 change 元数据声明 `retire_capabilities: true`，归档才会直接删除这 22 个 spec 文件并修剪空目录——不会留下 4.3 所说的「只剩 `# 标题` 的空壳」。tasks 没有补声明这一步，4.3 描述的是一条走不到的流程。
- **证据**：
  - `ls -la openspec/changes/merge-tag-spec-family/` → 只有 design/proposal/tasks/ui-design/specs，无 `.openspec.yaml`
  - openspec 2.1.1 `dist/core/archive.js` `decideSpecOutcome`：未声明 → `'write'`；`!report.valid` → `Aborted. No files were changed.`，并提示 `add retire_capabilities: true to the change's .openspec.yaml`
  - 用 openspec 自身库函数探针（`buildUpdatedSpec` + `isRetirableSpec`，不落盘）：`可退役(归档将删除 spec): 22 个`，`将写入: 3 个 → auxiliary-label(+23/-0,valid=true), tag-matching(+55/-0), tagging(+55/-0)`；aggregate-tagging 重建稿只剩 marker + `# Aggregate Tagging` + Purpose + 空 `## Requirements`
  - /tmp 沙盒实跑（仓库外副本）：`openspec archive -y` 返回非 0，沙盒 specs 仍 178、无 archive 目录（与中止一致；该次日志被 `set -e` 吞掉未留存）。以 spec-gate 可识别的形式重跑时被门禁阻断，**未使用** `--force`/`SPEC_GATE_BYPASS`，结论以源码与探针为准
  - 唯一能产生「空壳再手删」的路径是 `openspec archive --no-validate`（help 标注 not recommended）
  - 先例：`openspec/changes/archive/2026-11-20-retire-story-adopt-event-metrics/.openspec.yaml` 含 `schema: syntopica-ui` + `retire_capabilities: true`
- **建议**：新增任务「归档前在 change 目录写 `.openspec.yaml`（`schema: syntopica-ui`、`retire_capabilities: true`）」；4.3 改为「归档后断言 22 个 `openspec/specs/<cap>/` 已由 openspec 自动退役」，验收清单 C 同步；design D7 补一句退役授权的说明。

### R-2 2026-11-26【High】D4 分类把 2 条行为契约标成 impl
- **状态**：已修复
- **处置**：改判 104（匹配输入加载组合标签）、128（Merge Embedding 向量缓存）为 business——两者都带失效契约；重新生成后 tagging 13 impl / tag-matching 15 impl
- **位置**：`specs/tag-matching/spec.md`：`匹配输入加载组合标签`（tag-to-board-matching）、`Merge Embedding 向量缓存`（board-match-cache）；design.md D4
- **问题**：两条都带 `<!-- kind: impl -->`、被排到 tag-matching 末尾，被当作施工记录。前者定义匹配输入并承载 composite_hit 的产生条件与隐式组合推导；后者是缓存失效契约，与同族标为 business 的 `Active Auxiliary Labels Cache (TTL)` 结构相同，却落在了另一类。D4 自己的边界裁定是「失效契约 → business」，这里没有执行。
- **证据**：
  - `匹配输入加载组合标签` Scenario：「THEN 匹配过程 SHALL 识别交集并产出 composite_hit」「匹配 SHALL 视为该 tag 挂载此组合（显式关联 ∪ 推导命中）」「board composition 的组合标签挂载发生变更 → board match cache SHALL 失效」
  - `Merge Embedding 向量缓存` Scenario：「缓存在标签新增时失效」「缓存在标签禁用时失效」「添加别名不影响 merge embedding 缓存」（6 个 Scenario，其中 3 个是失效契约）
  - 对照：`[business] Active Auxiliary Labels Cache (TTL)` 的 Scenario 是「Cache invalidated on label creation / label disable」
  - 关联红线：红线 3（composite_hit 判定）、红线 11（disabled 必须清空 merge_embedding——缓存不失效等于继续用被清空的向量）
- **建议**：两条移回 business 段并去掉标记；若要保留 impl 段，可把纯实现部分拆出去（缓存存 `map[uint][]float64`、`parsePgVector` 仅加载时调用），失效契约单独成条留在 business。

### R-3 2026-11-26【Medium】`聚合标签复用既有入库链路` 标 impl，其行为后置条件没有 business 落点
- **状态**：已修复
- **处置**：29（聚合标签复用既有入库链路）改判 business，保留其「必须 enqueue embedding」后置条件
- **位置**：`specs/tagging/spec.md`（来自 aggregate-tagging）
- **问题**：正文是「复用既有链路、SHALL NOT 另建入库通道」，按字面属代码组织；但它的 Scenario 断言的是系统行为——聚合路径的 event 标签必须入 embedding 队列、命中既有标签不得新建重复。这条后置条件在合并稿里只此一处，标 impl 后读者会当它不是现行契约。
- **证据**：Scenario「event 标签照常进入 embedding 队列：THEN 6 个标签均被 enqueue embedding，与 mono 路径行为一致」；`grep -nE "enqueue|入队" openspec/specs/aggregate-tagging/spec.md` 只命中这一条。
- **建议**：保持 business；或拆成「聚合标签入库后置条件（business）」+「复用链路（impl）」两条。

### R-4 2026-11-26【Medium】来源注释有 7 条把「首个 MODIFIED 它的 change」当成来源
- **状态**：已修复
- **处置**：`first_change()` 只认 `## ADDED Requirements`；board-match-cache 6 条与 match-detail-ondemand 3 条改为无来源注释；不可考从 6 个更正为 8 个
- **位置**：`specs/tag-matching/spec.md` 中 `from: board-match-cache (2026-11-16 lane-manual-correction-loop)` 的 6 条、`from: match-detail-ondemand (2026-05-29 board-direction-check-and-board-editing)` 的 3 条；`scripts/harness/merge-spec-family.sh` `first_change()`；design.md D3；tasks 1.2
- **问题**：脚本取「archive 里目录名排序最前、含 `specs/<cap>` 的 change」，没有区分 delta 类型。这两个 capability 在 archive 里的首个 change 是 MODIFIED，说明它们在此之前就已存在，真正的创建 change 不可考。注释写法却让读者以为这些 Requirement 由该 change 引入。
- **证据**：
  - `grep -nE '^(## |### Requirement:)' openspec/changes/archive/2026-11-16-lane-manual-correction-loop/specs/board-match-cache/spec.md` → 只有 `## MODIFIED Requirements` + `### Requirement: Board Embeddings Cache` 一条；其余 5 条（Board Auxiliary Labels Cache / AI Config Cache / Merge Embedding 向量缓存 / Thread Safety / Active Auxiliary Labels Cache）与该 change 无关
  - `…/2026-05-29-board-direction-check-and-board-editing/specs/match-detail-ondemand/spec.md` → 只 MODIFIED `getTagMatchDetail returns direction_sim`；其余 2 条无关
  - `git log --diff-filter=A --format='%ad %h %s' --date=short -- openspec/specs/board-match-cache/spec.md | tail -1` → `2026-06-14 c15e0e3a`；match-detail-ondemand → `2026-05-26 81a885b8`，均早于注释所写的日期
  - 其余 14 个带来源的 capability：目录存在、是首个、delta 类型为 ADDED，属实
- **建议**：`first_change()` 只认 `## ADDED Requirements`；未命中时写 `<!-- from: <cap> -->`（与 6 个零引用同等处理）；design D3 / tasks 1.2 的「6 个零引用」改为「8 个创建 change 不可考」。

### R-5 2026-11-26【Medium】auxiliary-label 的族级 Purpose 不会落地；原有 15 条里的 1 条 impl 既不标记也不靠后
- **状态**：已修复
- **处置**：tasks 4.4c 新增「归档后手改 auxiliary-label 的 Purpose」；design D8 写明该条 impl 不带标记、不重排
- **位置**：`specs/auxiliary-label/spec.md` 的 `## Purpose`；tasks 4.4；design.md D4/D8
- **问题**：openspec 归档时，delta 的 Purpose 只用于新建 spec，对已存在的 spec 忽略并告警。就地扩容后 main spec 的 Purpose 仍是旧文案（只讲提取/入库/治理），不覆盖新并入的合并建议、合并界面、层级质量、身份保护。tasks 只给两个新 spec 补 marker，没有更新 aux Purpose 的任务。另外 design 说 auxiliary-label 是「37 business / 1 impl」，但那 1 条 impl 在保留的原 15 条里，没有 `<!-- kind: impl -->`、也不在末尾。
- **证据**：
  - 探针输出：`auxiliary-label: delta Purpose ignored; auxiliary-label already has one. Edit .../auxiliary-label/spec.md directly to change it.`
  - 重建稿 Purpose：`辅助标签（Auxiliary Label）的提取、入库、治理。辅助标签是 event/person tag 的语义锚点，也用于 keyword tag 直入辅助池。`
  - `/tmp/spec-merge/classify-output.txt` 中 impl 序号 70 = auxiliary-label 原 main 第 15 条 `L1 索引增量写入对并发读者安全`；`grep -c 'kind: impl' openspec/specs/auxiliary-label/spec.md` → 0
- **建议**：4.4 增加一步「归档后手动改写 auxiliary-label main spec 的 Purpose 为族级文案」；D8 如实写明该条不带标记、不重排。

### R-6 2026-11-26【Medium】族 domain marker 的取舍没有在 design 论证，三族都丢了成员的域
- **状态**：已修复
- **处置**：design 新增 D9：三族 marker 取舍表 + 判据（改这族必须读哪些 flow 红线）+ 已知代价
- **位置**：tasks 4.4；design.md（全文未提 marker）；`openspec/specs/spec-domain-index/spec.md`「历史 change 的域归属两跳推导」
- **问题**：domain-history 靠 main spec 的 marker 给历史与未来的 change 归域。合并后一个族只能有一个 marker，且 J 段上限 3 个域，装不下成员 marker 的并集。tasks 4.4 直接给了 2/1/2 个域，没有说明为什么丢掉 scheduler、ai-summary、reading 等。auxiliary-label 的 marker 保持 `semantic-board`，新增 6 个成员里的 scheduler（aux-label-gc、tag-hierarchy-quality）与 ui（tag-merge-ui）都掉了。
- **证据**：
  - 成员 marker 并集：tagging 7 个（ai-summary, db, reading, scheduler, semantic-board, topic-graph, ui）；auxiliary-label 新增 6 成员 4 个（scheduler, semantic-board, topic-graph, ui）；tag-matching 6 个（api, db, discovery, semantic-board, topic-graph, ui）
  - tasks 4.4 拟定：tagging `semantic-board, ui`；tag-matching `semantic-board, topic-graph`
  - `check-standards.sh` J 段：合法词表含 `ui`/`topic-graph`，`超过 3 个域` 判非法；J 段只 WARN 不阻断
- **建议**：design 增一条「族 marker 取舍」，判据写清（改这族的人必须读哪些 flow 红线）；例如 tag-queue-scheduling 对应 topic-graph 红线 9 与 scheduler，是否进 tagging 的 marker 要有明确取舍。J 段对拟定值会通过（词表内、≤3 域），不阻断。

### R-7 2026-11-26【Medium】孤儿 delta 的影响被低估：21 个历史 change 会从 domain-history 的所有段消失
- **状态**：已修复
- **处置**：design D6 更正为 47 delta / 36 change / 15 可见 / 21 不可见并列出 5 个典型；tasks 5.1 与验收 C 同步；别名表登记为后续 change
- **位置**：design.md D6；proposal.md Impact；tasks 5.1；验收清单 C
- **问题**：D6 说「第 6 段会列出所有指向它们的 archived delta（预计 ~40 条）」。实际 `domain-history.sh` 的第 6 段只统计「仍能经其它 capability 命中本域」的 change；只碰过被删 capability 的 change 匹配不到任何域，时间线和孤儿段都不会出现。来源注释只能指到「首个 change」，补不上这块。
- **证据**（按 domain-history.sh 的两跳逻辑模拟合并后状态）：
  - 指向 22 个被删 capability 的 archived delta 文件 47 个，涉及 36 个 change（archive 共 284 个 change）
  - 合并后仍命中 `semantic-board`、会进第 6 段的：15 个 change
  - 合并后彻底不可见的：21 个，含 `2026-05-13-backend-package-restructure`、`2026-08-22-analysis-remediation`、`2026-09-24-night-window-alignment`、`2026-10-28-guard-tag-exemplar-leakage`、`2026-11-25-retire-person-extraction`（全表见核实记录 G3）
  - `scripts/harness/domain-history.sh` 第 143–190 行：`orphans` 只在 `matched=1` 的分支里自增
- **建议**：更正 D6 / 5.1 / 验收 C 的数字（22 个 capability、36 个 change、第 6 段约 15 个、21 个不可见）；后续 change 给 domain-history.sh 加「已退役 capability → 族」别名表（可由本 change 的 REMOVED delta 推导），才能兑现 spec-domain-index「孤儿 delta 显式列出」。

### R-8 2026-11-26【Medium】旧 capability 名的 12 处跨文件引用没有任务处理
- **状态**：已修复
- **处置**：tasks 7.4 新增：6 处 docs/reference 指针改指新族 + 4 个 main spec 的仅改指针 MODIFIED
- **位置**：proposal.md Impact；tasks 7.x（只有 7.3 给 flow/semantic-board.md 加一行溯源）
- **问题**：docs/reference 与其余 main spec 里有 12 处按旧 capability 名指向「唯一权威」契约，合并后都是断链。
- **证据**：
  - `docs/reference/api/feeds.md:55`：`唯一权威 openspec/specs/source-board-hit-rate/spec.md`
  - `docs/reference/api/semantic-boards.md:128,153`、`docs/reference/flow/semantic-board.md:500,522`（红线 17、35）、`docs/reference/flow/reading.md:212`：指向 `board-match-cache` / `source-board-hit-rate`
  - main spec：`board-concept-management/spec.md:5,10,34`（→ semantic-label-model）、`board-health-checkup/spec.md:312`（→ auxiliary-label-description-repair）、`analysis-pause-control/spec.md:87`（→ tag-queue-scheduling）、`board-editing-ui/spec.md:17`（→ board-match-cache）
- **建议**：tasks 增加一项：归档的同一提交内把 docs/reference 的 6 处改指新族；其余 4 个 main spec 的指针要么纳入本 change 作仅改指针的 MODIFIED delta，要么列为紧随其后的 change；proposal Impact 补「文档引用」。

### R-9 2026-11-26【Low】分类输入不在仓库里，脚本不能复现 tagging / tag-matching 制品
- **状态**：已修复
- **处置**：分类结果入库为 `spec-classification.txt`，脚本可幂等重跑
- **位置**：`scripts/harness/merge-spec-family.sh`；tasks 2.x；`/tmp/spec-merge/classify-output.txt`
- **问题**：business/impl 的 148 个标签只存在于 /tmp。R-2 若要改判，要么手改 delta，要么重跑脚本，而重跑没有 kind 文件就得不到同样的两段式产物。脚本头部的用法行也没列 `--kind-file` / `--kind-offset` / `--delta-out`。
- **证据**：不带 kind 文件重跑脚本后与制品 diff（忽略 `<!-- kind: impl -->` 行）：22 个 REMOVED 与 auxiliary-label ADDED 差异 0 行；tagging 452 行、tag-matching 568 行（全部来自 impl 重排与标记）。
- **建议**：把 `classify-output.txt` 存进 change 目录；或让脚本支持从既有制品读 `kind` 标记做幂等重跑；补全用法说明。

### R-10 2026-11-26【Low】制品之间的数字与计数表述不一致
- **状态**：已修复
- **处置**：统一为退役 22 / 新建 2 / 最终 158；行数改按 `wc -l` 口径
- **位置**：proposal.md（Why / What Changes / Impact）、tasks.md 1.1 / 4.2 / 5.1 / 验收 C
- **问题**：不影响执行，但对照时会打架。
- **证据**：
  - proposal「178 → 158（删 23 加 3）」 vs tasks 4.5「178 − 22 + 2」（auxiliary-label 就地扩容，不删不加）
  - tasks 4.2「生成 23 个旧 capability 的 REMOVED」、5.1 / 验收 C「23 个旧 capability」，实际 REMOVED 22 个
  - 行数：proposal 2771 / 1042 / 678 / 1051，`wc -l` 实测 2748 / 1035 / 671 / 1042；归档后重建稿实测 tagging 1132、auxiliary-label 675、tag-matching 1137
- **建议**：统一为 22 / 158（178 − 22 + 2），行数注明口径（源文件 / delta / 归档后）。

### R-11 2026-11-26【Low】`openspec validate --strict` 不过，22 条 WARNING 全部继承自原 spec
- **状态**：已采纳
- **处置**：strict 的 22 条 WARNING 全部继承自原 spec，本 change 不改语义故不修；已在 tasks 备注留痕
- **位置**：`openspec validate merge-tag-spec-family --strict`
- **问题**：非 strict 通过（`Change 'merge-tag-spec-family' is valid`，exit 0）；`--strict` exit 1，原因是「Requirement text is very long (>500 characters)」与「should contain SHALL or MUST」，都是原样搬运带来的。归档门禁六项不含 strict，不阻断；proposal「不做的事」已声明不修内容。
- **证据**：见核实记录 F1。
- **建议**：在 design 或 review 留痕里记一句「strict 的 WARNING 为存量」，避免后续被当成合并引入。

### R-12 2026-11-26【Low】`匹配工具函数` 标 impl 可接受，但它是混合条目
- **状态**：已采纳
- **处置**：混合条目接受：impl 部分是函数签名，用户可见输出在 business 段 `Tag chip 分数文字显示` 有落点，不重复
- **位置**：`specs/tag-matching/spec.md`（来自 match-score-visualization）
- **问题**：正文规定前端工具函数签名（impl），Scenario 却断言用户可见输出（`"#f59e0b"`、`"命中率 0.75"`、不输出 "↓"）。「不追加 ↓」在 business 条目 `Tag chip 分数文字显示` 里有重复落点，所以风险低。
- **证据**：`grep -nE "↓|downgraded" openspec/specs/match-score-visualization/spec.md` → 第 35 行（business）与第 61 行（本条）各一处。
- **建议**：可不动；若拆分，展示契约归 business。

### R-13 2026-11-26【Low】归族切法有两处存疑
- **状态**：已采纳
- **处置**：两处归族存疑记入后续拆分候选，本次不动（改了要重跑 133 条守恒）
- **位置**：design.md D2
- **问题**：
  1. `tag-match-identity-protection` 约束的是 `TagMatch` / `findOrCreateTag` 在标签创建路径上的身份判定，离 tagging 的「标签怎么产生」更近，却落在 auxiliary-label。
  2. `source-board-hit-rate`（命中统计）与 `cross-board-relation-discovery`（跨版块关系，红线 13 明确「不影响 tag 版块归属」）是匹配结果的下游消费方，不是「标签匹配到版块」本身。
- **证据**：`sed -n '1,36p' openspec/specs/tag-match-identity-protection/spec.md`（两份 Requirement 都以 `TagMatch` / `CategoryThresholdOverrides` 为主语）。
- **建议**：不阻断本次。记入后续拆分候选：前者并入 tagging，后者可与 sourcestats 一起另立「匹配结果消费」族。

### R-14 2026-11-26【Low】D5 第二档阈值对 tag-matching 已经贴边，判据不可操作
- **状态**：已修复
- **处置**：tasks 4.4d 新增两族标题风格修正；D5 阈值「不相干功能」判据登记为后续 change 细化
- **位置**：design.md D5
- **问题**：归档后 tag-matching 1137 行，距「>1200 行且含 ≥3 个不相干功能 → 考虑拆」只差 63 行，而它现在已经同时装着匹配内核、缓存、前端展示、命中率统计、跨版块关系。「不相干」没有判据，这条阈值实际上无法触发。新 main spec 标题会是 `# tagging Specification`（小写 capability id），与存量 `# Aggregate Tagging` 这类标题风格不一致。
- **证据**：探针重建稿行数 1132 / 675 / 1137；`buildSpecSkeleton` 生成 `# ${specFolderName} Specification`。
- **建议**：D5 把第二档改成可判的（例如「同一 change 连续 N 次只改族内固定子集」）；标题大小写归档后顺手改。

## 核实记录

审查日期 2026-11-26。全程只读：未改任何仓库文件，唯一写入是本报告。沙盒实验在 `/tmp` 的仓库外副本进行，已清理。

### A 守恒

```
C=openspec/changes/merge-tag-spec-family/specs
for d in $C/*/; do printf '%-36s Req=%s Scen=%s\n' "$(basename $d)" "$(grep -c '^### Requirement:' $d/spec.md)" "$(grep -c '^#### Scenario:' $d/spec.md)"; done
```

| delta | Req | Scen | | delta | Req | Scen |
|---|---:|---:|---|---|---:|---:|
| tagging (ADDED) | 55 | 150 | | tag-embedding-management | 14 | 0 |
| auxiliary-label (ADDED) | 23 | 61 | | tagging-domain | 22 | 0 |
| tag-matching (ADDED) | 55 | 170 | | tag-hierarchy-quality | 5 | 0 |
| aggregate-tagging | 8 | 0 | | tag-match-identity-protection | 2 | 0 |
| article-tag-watch-toggle | 3 | 0 | | tag-merge-suggestions | 6 | 0 |
| auxiliary-label-description-repair | 5 | 0 | | tag-merge-ui | 2 | 0 |
| aux-label-gc | 3 | 0 | | tag-queue-progress-chip | 5 | 0 |
| board-affinity-metadata | 3 | 0 | | tag-queue-scheduling | 2 | 0 |
| board-match-auto-trigger | 1 | 0 | | tag-to-board-matching | 12 | 0 |
| board-match-cache | 6 | 0 | | source-board-hit-rate | 4 | 0 |
| composite-label | 6 | 0 | | match-detail-ondemand | 3 | 0 |
| cross-board-relation-discovery | 6 | 0 | | match-score-visualization | 6 | 0 |
| semantic-label-model | 9 | 0 | | | | |

- ADDED 合计 133；REMOVED 合计 133；原 23 个 main spec：Requirement 148、Scenario 415、`wc -l` 2748 行；`ls -d openspec/specs/*/ | wc -l` = 178。
- A4：22 个 REMOVED 条数与各自 main spec 的 Requirement 条数全部相等（22/22，不只是抽查 5 个）。
- A5：brief 期望的「415 − 0」不成立——auxiliary-label 就地扩容，自己的 34 条 Scenario 不进 ADDED，所以 ADDED 里是 381；归档后 150 + (34+61) + 170 = 415 守恒。

### B 语义无改动（全量）

方法：对 22 个来源 spec 的每个 Requirement 块与 delta 中同标题块比较，delta 侧只删 `<!-- from: … -->`、`<!-- kind: impl -->` 两类注释行；先做 `strip()` 比较，再做保留块内空白的严格比较。

```
[tagging] delta 55 / main 55，scenario 150/150，business 41 impl 14
[auxiliary-label] delta 23 / main 23，scenario 61/61，business 23 impl 0
[tag-matching] delta 55 / main 55，scenario 170/170，business 38 impl 17
总计对比 133 条, 不一致 0 条；严格比较 133/133；重复标题: 无；正文完全相同: 无；行级 Jaccard ≥ 0.5 的条目对: 无
```

原 23 个 spec 的二级标题只有 `## Purpose` ×23 与 `## Requirements` ×23，没有会被脚本截掉的其他节；非 Scenario 的四级标题只有 tag-merge-suggestions 的 `#### Schema` / `#### Constraints` / `#### Message Format`，随块搬运，探针重建稿里仍是 3 个。

### C 来源注释

`grep -h '^<!-- from:' $C/{tagging,auxiliary-label,tag-matching}/spec.md | sort | uniq -c` → 133 条、22 个不同来源。逐个核对 archive：

| 来源 | 条数 | 目录存在 | 是首个 | delta 类型 |
|---|---:|---|---|---|
| tagging-domain (2026-05-13 backend-package-restructure) | 22 | 是 | 是 | ADDED |
| tag-to-board-matching (2026-05-10 narrative-concept-boards) | 12 | 是 | 是 | ADDED |
| semantic-label-model (2026-08-22 analysis-remediation) | 9 | 是 | 是 | ADDED |
| tag-embedding-management (2026-08-22 analysis-remediation) | 14 | 是 | 是 | ADDED |
| aggregate-tagging (2026-08-22 aggregate-article-tagging) | 8 | 是 | 是 | ADDED |
| composite-label (2026-09-04 add-composite-labels) | 6 | 是 | 是 | ADDED |
| cross-board-relation-discovery (2026-09-05 add-evidence-backed-cross-board-relations) | 6 | 是 | 是 | ADDED |
| match-score-visualization (2026-05-26 board-interaction-overhaul) | 6 | 是 | 是 | ADDED |
| tag-hierarchy-quality (2026-05-10 fix-tag-hierarchy-inflation) | 5 | 是 | 是 | ADDED |
| auxiliary-label-description-repair (2026-11-06 board-health-triage-advice) | 5 | 是 | 是 | ADDED |
| tag-queue-progress-chip (2026-09-17 add-notification-center) | 5 | 是 | 是 | ADDED |
| source-board-hit-rate (2026-09-18 add-source-board-hit-rate) | 4 | 是 | 是 | ADDED |
| article-tag-watch-toggle (2026-05-10 article-tag-follow-status) | 3 | 是 | 是 | ADDED |
| match-detail-ondemand (2026-05-29 board-direction-check-and-board-editing) | 3 | 是 | 是 | **MODIFIED** |
| tag-queue-scheduling (2026-09-24 night-window-alignment) | 2 | 是 | 是 | ADDED |
| board-match-cache (2026-11-16 lane-manual-correction-loop) | 6 | 是 | 是 | **MODIFIED** |
| aux-label-gc / board-affinity-metadata / tag-match-identity-protection / tag-merge-ui（无来源） | 3/3/2/2 | — | — | archive 含 `specs/<cap>` 的 change 数 = 0 |
| tag-merge-suggestions（无来源） | 6 | — | — | 0 |

6 个无来源的 capability 确实零引用，未编造日期。两个 MODIFIED 的见 R-4。

### D 分类

`awk '{print $2}' /tmp/spec-merge/classify-output.txt | sort | uniq -c` → 116 business / 32 impl。32 条 impl 标题全部列出读过：

- tagging 14 条：Tagging domain package structure、Domain package singular naming、Models ownership by domain、Unified worker lifecycle、No behavior change、Embedding 写入只使用 pgvector 列、FindSimilarTags 使用原始浮点数组构建查询、数据库 vector 列被移除、移除 auxiliary_label_service 中的 SQLite fallback 代码、不引入新的 SQLite 依赖路径、存量病态时间锚标签的可修复性、存量示例词泄漏关联的一次性清理、聚合标签复用既有入库链路、删除旧概念和层级字段
- tag-matching 17 条：evaluateSemanticBoardMatches 接收方向校验数据、MatchTopicTag 加载方向校验数据、MatchTopicTag 使用缓存数据、**匹配输入加载组合标签**、存量匹配重算、3 条测试迁移到 Postgres、Board/Merge Embeddings 缓存存储已解析数据、Tag Embedding 不缓存、存量孤儿 embedding 一次性清理、队列任务批量调用 embedding、AI Config Cache (TTL)（D4 裁定的纯 TTL，归类正确）、**Merge Embedding 向量缓存**、Thread Safety、匹配工具函数

其中 2 条误判（R-2）、1 条存疑（R-3）、1 条混合（R-12），其余与 D4 判据相符。business 侧扫了三个族全部标题，没有明显的施工记录；`状态机字段的 GORM default tag 保留`、`embedding 模型名列宽不窄于模型注册表` 偏代码/schema 层，但正文论证了行为后果，按 business 可接受。

### E 就地扩容

- delta ADDED 的来源分布：tag-merge-suggestions 6、auxiliary-label-description-repair 5、tag-hierarchy-quality 5、aux-label-gc 3、tag-merge-ui 2、tag-match-identity-protection 2 = 23；不含 `from: auxiliary-label`
- 原 main 15 条与 delta 标题交集为空；探针重建稿：Requirement 38、Scenario 95、`<!-- from:` 23、domain marker 保留
- Purpose 未更新（R-5）

### F 格式与归档

F1：
```
openspec validate merge-tag-spec-family            → Change 'merge-tag-spec-family' is valid（exit 0，22 WARNING，0 ERROR）
openspec validate merge-tag-spec-family --strict   → exit 1（WARNING 计失败：very long >500 characters / should contain SHALL or MUST）
openspec validate merge-tag-spec-family --specs    → Totals: 178 passed, 0 failed
```

F2：22 个 REMOVED delta 的标题序列与 `openspec/specs/<cap>/spec.md` 的 `### Requirement:` 序列逐字、同序一致（22/22）。

F3：每条 REMOVED 为三行 `### Requirement:` / `**Reason**:` / `**Migration**:`，与先例 `archive/2026-05-10-narrative-concept-boards/specs/narrative-board-generation/spec.md` 第 62 行起写法一致；openspec 解析器成功消费全部 133 条 REMOVED（探针 counts）。

F4：探针（node，import openspec dist 的 `core/specs-apply.js`、`core/archive.js`、`core/validation/validator.js`，只调用 `findSpecUpdates` / `buildUpdatedSpec` / `isRetirableSpec`，不写盘）：
```
可退役(归档将删除 spec): 22 个 → aggregate-tagging, …, tagging-domain
将写入: 3 个 → auxiliary-label(exists=true,+23/-0,valid=true), tag-matching(exists=false,+55/-0,valid=true), tagging(exists=false,+55/-0,valid=true)
auxiliary-label: delta Purpose ignored; auxiliary-label already has one.
```
沙盒实跑与门禁：见 R-1。归档门禁当前红项（spec-gate 输出原文）：① `doc-impact verify` 未声明；③ tasks.md 缺「测试 / 文档 / 验证」尾三节与 `<!-- doc-impact:` 标记；④ `review-trace` 无 `review.md`。

### G 门槛与遗漏

G1/G2：`check-standards.sh` J 段词表 `semantic-board topic-graph daily-report content-enrichment data-enrichment ai-summary discovery reading scheduler research-data-sources harness ui dev api db`，上限 3 域，只 WARN。拟定 marker 均通过。并集与取舍见 R-6。

G3：合并后对 `semantic-board` 不可见的 21 个 archived change：
```
2026-05-10-article-tag-follow-status            ← article-tag-watch-toggle
2026-05-10-fix-tag-hierarchy-inflation          ← tag-hierarchy-quality
2026-05-13-backend-package-restructure         ← tagging-domain
2026-08-22-aggregate-article-tagging            ← aggregate-tagging tagging-domain
2026-08-22-aggregate-tagging-resilience         ← aggregate-tagging
2026-08-22-analysis-remediation                 ← semantic-label-model tag-embedding-management
2026-08-23-fix-quality-audit-p0                 ← tagging-domain
2026-08-24-restore-gorm-default-tags            ← tagging-domain
2026-09-17-add-notification-center             ← tag-queue-progress-chip
2026-09-18-add-source-board-hit-rate            ← source-board-hit-rate
2026-09-19-long-form-sampled-tagging            ← tagging-domain
2026-09-24-night-window-alignment               ← tag-queue-scheduling
2026-09-25-fix-tagging-pollution                ← tagging-domain
2026-10-10-fix-event-tag-time-anchor            ← tagging-domain
2026-10-28-guard-tag-exemplar-leakage           ← tagging-domain
2026-11-03-fix-embedding-model-column-width     ← tag-embedding-management
2026-11-05-speed-up-tag-embedding                ← tag-embedding-management
2026-11-11-normalize-event-subject-entities     ← aggregate-tagging
2026-11-12-tolerate-schema-envelope-extraction-output ← tagging-domain
2026-11-23-fix-feed-refresh-churn               ← tag-queue-scheduling
2026-11-25-retire-person-extraction             ← tagging-domain
```
被引用最多的被删 capability：tagging-domain 12、tag-to-board-matching 8、aggregate-tagging 6。

G4：tasks 未列之事——retire 授权（R-1）、aux Purpose（R-5）、跨文件引用（R-8）、kind 文件（R-9）。

### H 风险

H1：`git status --short` 显示 change 目录与 `scripts/harness/merge-spec-family.sh` 目前都是未跟踪（`??`），提交后 `git revert` 可完整恢复 spec 文件（22 删 / 2 新 / 1 改都在 git 跟踪内）。不在 git 里的：`/tmp/spec-merge/{classify-input.md,classify-output.txt}`（R-9）；另有 7 个 main spec 带未提交改动（CRLF→LF 加 marker，含成员 article-tag-watch-toggle、tag-hierarchy-quality；`git diff --ignore-cr-at-eol` 后实质差异只有 marker 行），需与本 change 一同或先行提交。

H2/H3：见 R-13、R-14。

### brief 与现实的冲突（如实记录）

- brief 指定输出 `review-merge.md`；归档门禁 `scripts/harness/review-trace.sh` 只认 `review.md`（格式 `### R-1 <YYYY-MM-DD>【等级】<标题>` + 状态行）或 proposal 头部 `<!-- review: waived -->`。本报告按 brief 落在 `review-merge.md`，是否并入 `review.md` 由主线程决定，未自行改需求。
- brief 的 A5 期望值「415 − 0」与 D8 就地扩容矛盾，按实际 381 报告。
- 首次沙盒实跑的日志因 `set -e` 未留存；重跑被 spec-gate 阻断，未绕过。
