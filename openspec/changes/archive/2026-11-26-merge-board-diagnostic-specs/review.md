# 审查报告：merge-board-diagnostic-specs

- **审查者**：glm-5.4（只读）
- **结论**：需改后进
- **放行判断**：守恒与逐字零改动全量通过、归档机制经 openspec 2.1.1 源码与只读探针验证可行；最大风险是两族 delta 的 `## Purpose` 为空——不补就归档，openspec 会把「TBD - created by archiving change …」占位写进两个新主 spec，而 tasks 3.1/3.2 已勾选「Purpose 复核」（R-1）。

## 总评

| 清单项 | 结论 | 要点 |
|---|---|---|
| A 守恒 | 通过 | ADDED 22+36=58，REMOVED 12+10+15+21=58；4/4 REMOVED 条数等于各自 spec 的 Requirement 数；Scenario 72+37=109、63+59=122，合计 231；`<!-- from -->` 58 条 |
| B 语义零改动 | 通过 | 未抽查，做了全量：58/58 块去掉 from/kind 注释与空行后与原 spec 逐字一致；另列 8 条抽样明细（跨两族四来源） |
| C 来源注释 | 通过 | 58 条只指向 4 个创建 change：目录存在、排序最前、delta 类型均为 ADDED；无「MODIFIED 当来源」；脚本 `first_change()` 已只认 ADDED |
| D business/impl | 通过 | 56/2 与 design、tasks、`spec-classification.txt` 一致；2 条 impl 判定成立且后置条件在 business 段有落点；56 条 business 逐条看过标题与可疑正文，未见反向漏判；4 条配置项、2 条兼容条判 business 成立 |
| E 族边界 | 成立，论证有保留 | 体检磁铁信号在代码里只读消费 `direction_mismatch`（boardhealth/signals.go）；研究两族共享 analysisRunner / 409 互斥 / 不可变快照骨架；radar 保留成立；D2 的第二条理由不成立、且没盘点 tag-matching 既有条款与 3 条消费方契约的跨族重复（R-2） |
| F 归档可行性 | 可行（待 R-1） | validate 非 strict 通过；REMOVED 标题 58/58 逐字同序；`retire_capabilities: true` 已写；tasks 4.3 摘 marker 必要且充分（源码 + 探针）；marker 方案零丢域；两族 Purpose 归档后为 TBD 占位（R-1） |
| G 遗漏 | 3 处小缺口 | Purpose 没有任务（R-1）；路径引用实为 1 处非 0（R-5）；data-enrichment 溯源表未列（R-5） |
| H 风险 | 可控 | 全部制品可 `git revert`（change 目录尚未提交，4 个 spec 干净）；board-research 归档后 833 行 < 1200；备选切法见 R-2 |

红线复核（brief 附带的 1–8）：红线 1–3 的落点是上一族的 `tagging` / `tag-matching`，不在本次 4 个成员内，合并前后不受影响。红线 4 在合并稿 `board-diagnostics` 内完整可搜：L444「对每个匹配成功的 tag-board 结果（composite_hit 除外）」、L452「direct_hit 同样执行上述判定」、L483/L487「match still recorded with original score」（仍记录）、L548「Daily report excludes direction_mismatch tags」（不计日报）、L564「getBoardArticles … 默认排除」；唯一半句「前端默认隐藏」的落点在 `tag-matching` L588「默认隐藏 direction_mismatch 标签」，合并前也不在 board-direction-check 里。红线 5–8 不在这 4 个 spec 内。

## 发现

### R-1 2026-11-26【Medium】两族 delta 的 Purpose 为空，归档后两个新主 spec 会带 TBD 占位
- **状态**：已修复
- **处置**：`merge-spec-family.sh` 的 PURPOSE 字典补两族族级文案，重新生成后 delta Purpose 非空（实测首句「诊断 = 判定版块质量…」/「研究 = 人工触发…」）。归档前可再验一次：`sed -n "/^## Purpose$/,/^## ADDED/p" <delta>` 非空
- **位置**：`specs/board-diagnostics/spec.md:1-5`、`specs/board-research/spec.md:1-5`；tasks 3.1 / 3.2（均已勾选「Purpose 复核」）；design D3；`scripts/harness/merge-spec-family.sh` 的 `PURPOSE` 字典
- **问题**：两个 delta 的 `## Purpose` 下面只有三个空行。脚本 `PURPOSE` 只有 tagging / auxiliary-label / tag-matching 三个键，delta 输出取 `PURPOSE.get(fam, "")`，新族没登记就为空；tasks 2.1「复用脚本」直接把它带进制品，3.1/3.2 的「Purpose 与边界复核」没有文案可复核。openspec 2.1.1 对新建 spec 会把 delta 的 Purpose 写进主 spec（上一族 tagging / tag-matching 即如此），为空时 `buildSpecSkeleton` 落占位符。归档后两个新族的首段就是「TBD - created by archiving change merge-board-diagnostic-specs. Update Purpose after archive.」；`validate --specs` 与 check-standards J 段都不拦（占位符超过 50 字符），只能靠人事后手改，而 tasks 8.3 只列了补 marker。
- **证据**：
  - `sed -n '1,5p' openspec/changes/merge-board-diagnostic-specs/specs/board-diagnostics/spec.md` → `## Purpose` + 3 空行 + `## ADDED Requirements`（board-research 同）
  - 只读探针（见核实记录 F4）`buildUpdatedSpec` 重建稿首 5 行：`# board-diagnostics Specification` / `## Purpose` / `TBD - created by archiving change merge-board-diagnostic-specs. Update Purpose after archive.`；board-research 同
  - `dist/core/specs-apply.js:1242-1248`：`const purposeBody = purpose?.trim() || PURPOSE_PLACEHOLDER_PREFIX + changeName + PURPOSE_PLACEHOLDER_SUFFIX`
  - 对照上一族：`openspec/changes/archive/2026-11-26-merge-tag-spec-family/specs/tagging/spec.md:3` 有族级文案，其 tasks 3.x 写「Purpose 已由脚本写入」
- **建议**：归档前把 D3 的边界句（「诊断自己算结论，不改变版块；研究产出待用户判断的材料，不自动触发」）扩写成两段 Purpose 写进两个 delta 的 `## Purpose`，并补进脚本 `PURPOSE` 字典以保持可重跑；tasks 3.1/3.2 补一条「补族级 Purpose」；8.3 顺手断言归档后两族 Purpose 不含 `TBD`。

### R-2 2026-11-26【Medium】D2 归族论证缺一半：「已归档不重开口子」不成立，且未盘点 tag-matching 既有方向校验条款与 3 条消费方契约的跨族重复
- **状态**：已采纳
- **处置**：design D2 补「已知代价」段：承认 tag-matching 里 26 处 direction 内容 + 「详见 board-direction-check」死引用 + 3 条消费方契约跨族重复；本 change 不改已归档族，登记 tasks 7.5 为后续 change（跨族引用等相邻族收口后统一扫）
- **位置**：design.md D1/D2；proposal.md「不做的事」末条；`specs/board-diagnostics/spec.md:548/559/574`；`openspec/specs/tag-matching/spec.md:588/602/648`；`openspec/specs/daily-report-match-fallback/spec.md:28`
- **问题**：
  1. D2 第二条理由「`tag-matching` 已归档，不再重开口子」不成立——归档的是 change 不是 spec，后续 change 随时可对 tag-matching 追加 delta（上一族本身就是往已归档的 capability 上合并的）。归属只能靠第一条语义论证，这句话应删。
  2. 第一条论证只看了两端（方向校验定义 vs 匹配决策），没盘点 tag-matching 里已经依赖方向校验的条款：direct_hit 降级方向校验场景（L31-39）、`默认隐藏 direction_mismatch 标签`（L588）、`匹配详情面板降级说明和方向校验展示`（L602）、`getTagMatchDetail 返回 direction_sim`（L648），全文 26 处 `direction_mismatch` / `direction_sim`。合并后同一个 API 字段在两族各有一条：board-diagnostics L574 `Match detail API returns direction_sim` 与 tag-matching L648 `getTagMatchDetail 返回 direction_sim`，Scenario 名都叫 `direction_sim available` / `direction_sim unavailable`；「日报排除 mismatch」同样成对：board-diagnostics L548 与 daily-report-match-fallback L28 `日报排除 direction_mismatch 标签`。这 3 条消费方契约放在定义旁边可以自圆其说，但 D2 应写明「与消费方 spec 重复」并登记去重，否则下一拨人会以为是合并引入的。
  3. D1「调阈值、加信号、改分诊闸门时两边一起动」偏弱：调方向阈值不需要动体检阈值，反向亦然；两者的真实连带只有一条——体检磁铁信号只读消费 `direction_mismatch`。
- **证据**：
  - `grep -n -E 'direction_mismatch|direction_sim' openspec/specs/tag-matching/spec.md | wc -l` → 26；`grep -n -E '^### Requirement: (默认隐藏|匹配详情面板|getTagMatchDetail)' openspec/specs/tag-matching/spec.md` → 588 / 602 / 648
  - `sed -n '574,585p' specs/board-diagnostics/spec.md` 与 `sed -n '648,661p' openspec/specs/tag-matching/spec.md`：两条都规定 `direction_sim`（float64/null）+ 同名两 Scenario
  - `sed -n '548,556p' specs/board-diagnostics/spec.md`（collectBoardTags 主查询与 fallback 排除）vs `sed -n '28,40p' openspec/specs/daily-report-match-fallback/spec.md`（同口径）
  - 磁铁信号消费：`backend-go/internal/tagmanagement/service/boardhealth/signals.go:59-61`「healthy side (good) = direction_mismatch=false mounts; failed side = direction_mismatch=true mounts」
  - 结论本身（质检归诊断）站得住，所以定 Medium 落在「论证与登记」而不是「切法错误」
- **建议**：D2 删掉「不再重开口子」，补「已知跨族重复」三条清单；proposal「不做的事」同步改口径。本次不动制品（守恒 58=58 已核），登记后续 change：三条消费方契约与 tag-matching / daily-report-match-fallback 去重，保留一处、另一处留指针。备选切法一并记录：direction-check 整体并入 tag-matching（匹配管线自带的事后质检，配置键 `semantic_board_match_direction_*`，impl 加载条目上一族已在 tag-matching），diagnostics 只剩体检——本族选了「质检归诊断」，可接受。

### R-3 2026-11-26【Low】退役数写成 3（实为 4），孤儿 delta 影响没有数字，5.1 无法断言
- **状态**：已修复
- **处置**：proposal「退役 3 个」→「退役 4 个」；design D6 明确 4 个 + 列出被反复改过的子孙 change（health 4 个 / direction 3 个），孤儿具体条数改由 tasks 5.1 归档后实测记数不预填
- **位置**：proposal.md:47-48；design.md:60、65；`.openspec.yaml:3`；tasks 5.1 / 验收 C
- **问题**：退役的是 4 个 capability，三处写「3 个」；proposal 自己的算式 158 − 3 + 2 = 157 对不上 156（tasks 4.5 的「158 − 4 + 2」才对）。D6「指向 3 个被删 capability 的 archived delta 会落第 6 段」既少数又过度承诺：第 6 段只列「仍能经其它 capability 命中本域」的 change，只碰过被删 capability 的整条消失（上一族 R-7 同一模式）。D6 没给数字，5.1 只能靠人现算。
- **证据**：
  - `grep -n '3 个' {proposal.md,design.md,.openspec.yaml}` → proposal:47「退役 3 个 + 新建 2 个」、proposal:48、design:60「摘掉 3 个被退役 spec」、design:65、`.openspec.yaml:3`「3 个旧 capability（…四个名字…）」
  - 按 `domain-history.sh:150-186` 两跳逻辑模拟合并后状态：指向 4 个旧 capability 的 archived delta 17 个文件 / 16 个 change（health 5、direction 4、signal 4、level 4）；`semantic-board` 仍可见 9 个（会进第 6 段），7 个彻底不可见：2026-10-14-add-fast-frequency-datasources、2026-10-24-fix-board-direction-gap-exempt、2026-10-24-fix-board-ghost-false-positive、2026-10-24-fix-board-health-llm-misread、2026-11-06-board-health-triage-advice、2026-11-09-fix-board-health-triage-degrade、2026-11-10-fix-board-health-magnet-signal；`data-enrichment` 可见 5 / 不可见 2（add-fast-frequency-datasources、add-topic-key-event-markers）
  - `scripts/harness/domain-history.sh:179-186`：`orphans` 只在 `matched=1` 分支自增
- **建议**：三处改 4；D6 / 5.1 / 验收 C 写明 17 文件 / 16 change、semantic-board 9 可见 7 不可见、data-enrichment 5 / 2；「已退役 capability → 族」别名表沿用上一族的后续 change 登记。

### R-4 2026-11-26【Low】行数四个口径打架；4.4/4.5 排在归档前却只能在归档后观察；4.3 与 8.2 应同提交
- **状态**：已修复
- **处置**：design D2 表格行数改为三口径并列（来源 spec 615/769、delta 672/866、归档后 655/833）；tasks 4.4/4.5 标注「归档后」；4.3 明确「与 8.2 同一个提交」
- **位置**：proposal.md「What Changes」表；tasks 1.1、2.2、4.3、4.4、4.5
- **问题**：
  1. 615 / 769 是两个来源 spec 行数之和（405+210、387+382），不是族稿行数；delta 实测 672 / 866 行（tasks 2.2 写 673 / 867），归档重建稿 655 / 833 行。
  2. tasks 4.4「归档后断言已退役」、4.5「`--specs` 156 全过」排在第 4 节，但只能在 8.2 归档后观察；归档前 `ls openspec/specs | wc -l` 仍是 158。
  3. 4.3 摘 marker 改的是 4 个已跟踪的 main spec，应与 8.2 归档同一提交；中途失败要 `git checkout` 恢复 marker，否则 domain-history 把 4 个 spec 当孤儿、check-standards J 段报 WARN。
- **证据**：`wc -l openspec/specs/board-{health-checkup,direction-check,signal-reports,level-analysis}/spec.md` → 404 / 209 / 386 / 381（无尾换行，+1 即 brief 的 405 / 210 / 387 / 382）；`wc -l …/specs/board-{diagnostics,research}/spec.md` → 671 / 865；探针 `lines=655` / `lines=833`；`git status --short` → 仅 `?? openspec/changes/merge-board-diagnostic-specs/`，4 个 spec 与脚本干净。
- **建议**：统一写成「来源 1384 行 → 族稿 delta 672 / 866、归档后 655 / 833（含 from 注释）」；4.4/4.5 挪到第 8 节或注明「归档后」；4.3 注明与 8.2 同提交。

### R-5 2026-11-26【Low】旧 capability 路径引用实为 1 处（在途 change 的探索笔记）；data-enrichment 溯源表没有任务
- **状态**：已修复
- **处置**：tasks 7.4 更正：docs/reference 40 余处均为 change 名溯源不需改，实质路径引用仅 1 处在在途 change 的探索笔记里（非 main spec，不阻断）
- **位置**：tasks 7.3 / 7.4 / 第 10 节；`openspec/changes/description-repair-batch-matching/explore-findings.md:116`；`docs/reference/flow/data-enrichment.md:404`
- **问题**：7.4「旧 capability 名的路径引用实测 0 处」不完全准。路径形态确有 1 处——在途 change `description-repair-batch-matching` 的探索笔记写「主 spec `openspec/specs/board-health-checkup/spec.md`（144 行）」，归档后成断链；该 change 若据此给 board-health-checkup 提 delta 会落空（已核：当前除本 change 外无在途 change 对 4 个旧 capability 提 delta）。docs/reference 里 40 余处都是 change 名溯源（`board-signal-reports`、`add-board-health-checkup` 等），归档后仍有效，不需改。另外 board-research 的 marker 含 `data-enrichment`、4 个退役 spec 有 2 个原本就带该域，7.3 只给 `semantic-board.md` 溯源表加行。
- **证据**：`grep -rn -E '(openspec/)?specs/(board-health-checkup|…)(/spec\.md|\b)' .`（排除 archive、本 change、.pi/harness/spill）→ 唯一命中 `openspec/changes/description-repair-batch-matching/explore-findings.md:116`；`for c in …; do ls -d openspec/changes/*/specs/$c; done` → 只有本 change 的 4 个；`grep -n '变更溯源' docs/reference/flow/data-enrichment.md` → 404。
- **建议**：7.4 改「1 处，归档同一提交把该行改指 board-diagnostics」；7.3 顺带给 `data-enrichment.md` 溯源表加一行（check-standards E 段不强制，属一致性）。

### R-6 2026-11-26【Low】内容问题登记（继承自原 spec，本次不修）
- **状态**：已采纳
- **处置**：内容问题（混合条目、mag 信号排在定义前、L533 配置项第二 bullet 偏 impl）登记在案不修——本 change 不改语义
- **位置**：两族 delta（归档后即 main spec）
- **问题**：按 proposal「内容问题只记录不修」，登记：① `Match detail API returns direction_sim`（L574）与 tag-matching `getTagMatchDetail 返回 direction_sim` 重复（R-2）；② `Daily report excludes direction_mismatch tags`（L548）与 daily-report-match-fallback L28 重复（R-2）；③ `DirectionSimThreshold configurable`（L533，正文 L538）「与现有 12 个匹配参数一致」计数过时；④ `Direction check data loading`（L645）Scenario 仍写「skipped for all max_sim matches」，与 business 段「composite_hit 除外全部校验」口径不一致；⑤ `版块分析工作台信息架构`（L760）Scenario「单一下拉选择泳道」WHEN 讲选择研究/背景、THEN 讲不存在旧选择器，文不对题；⑥ Scenario 格式不统一：board-research 的 122 个 Scenario 里 105 个标题与 WHEN 之间多一空行（自 L54 起），17 个紧邻（如 L7 起首条 Requirement 的 8 个、L799 起两条的 8 个）；board-diagnostics 109 个全部紧邻；⑦ 7 条英文 Requirement 无 SHALL/MUST（strict WARNING 来源）。
- **证据**：行号见上；`openspec validate merge-board-diagnostic-specs --strict` 17 条 WARNING 与之对应。
- **建议**：作为后续「内容修订 change」的输入，本次不动。

### R-7 2026-11-26【Low】strict 的 17 条 WARNING 全部继承自原条款；归档门禁当前只差 review 痕迹
- **状态**：已采纳
- **处置**：strict 的 17 条 WARNING 全部继承自原条款（超长 11 / 缺 SHALL 6），归档门禁不含 strict
- **位置**：`openspec validate merge-board-diagnostic-specs --strict`；`bash scripts/harness/archive-readiness.sh merge-board-diagnostic-specs`
- **问题**：strict exit 1，17 条 WARNING（diagnostics 13：超长 7 + 缺 SHALL/MUST 6；research 4：超长），全部来自原条款正文（B 段已证逐字未动），非合并引入；非 strict 通过 exit 0；`--specs` 158 passed。archive-readiness 六项当前只有 ④ review-trace 红（本报告落盘后转绿），①②③⑤⑥⑦ 已绿。
- **证据**：见核实记录 F1 / F5。
- **建议**：留痕即可，不阻断（门禁六项不含 strict）。

## 核实记录

审查日期 2026-11-26。全程只读：未改任何仓库文件，唯一写入是本报告；探针在 `/tmp/review-mbds/`（仓库外，已清理），只调用 openspec 的 `findSpecUpdates` / `buildUpdatedSpec` / `isRetirableSpec`，不调用 `writeUpdatedSpec` / `retireSpec`，未跑 `openspec archive`。

### A 守恒（实测）

```
C=openspec/changes/merge-board-diagnostic-specs/specs
for d in $C/*/; do printf '%-24s Req=%s Scen=%s\n' "$(basename $d)" "$(grep -c '^### Requirement:' $d/spec.md)" "$(grep -c '^#### Scenario:' $d/spec.md)"; done
```

| delta | Req | Scen | | main spec | Req | Scen | `wc -l` |
|---|---:|---:|---|---|---:|---:|---:|
| board-diagnostics（ADDED） | 22 | 109 | | board-health-checkup | 12 | 72 | 404 |
| board-research（ADDED） | 36 | 122 | | board-direction-check | 10 | 37 | 209 |
| board-health-checkup（REMOVED） | 12 | 0 | | board-signal-reports | 15 | 63 | 386 |
| board-direction-check（REMOVED） | 10 | 0 | | board-level-analysis | 21 | 59 | 381 |
| board-signal-reports（REMOVED） | 15 | 0 | | radar-pipeline-status（不动） | 5 | — | 194 |
| board-level-analysis（REMOVED） | 21 | 0 | | | | | |

- ADDED 58 = REMOVED 58；4/4 REMOVED 条数等于各自 main spec 的 Requirement 数；Scenario 72+37=109、63+59=122，合计 231。
- `ls openspec/specs | wc -l` = 158；4.5 的 156 只在归档后成立（R-4）。

### B 语义零改动（全量 + 8 条抽样）

方法：对每个来源 spec 的 Requirement 块与 delta 同标题块比较，delta 侧只删 `<!-- from: … -->` / `<!-- kind: impl -->` 两类注释行，再去空行、去行尾空白。

```
exact byte-equal blocks（仅删注释行）: 0；去空行后一致: 58 / 58
```

抽样 8 条（`diff <(main 块去空行) <(delta 块去注释去空行)` 非空行差 0）：

| delta 位置 | main 位置 | Requirement | 非空行 | Scenario |
|---|---|---|---:|---|
| board-diagnostics:7 | board-health-checkup:10 | 体检统计与标签贴合度归因 | 10/10 | 3/3 |
| board-diagnostics:278 | board-health-checkup:257 | 标签分诊信号计算 | 37/37 | 10/10 |
| board-diagnostics:439 | board-direction-check:9 | Direction check for non-direct_hit matches | 67/67（7419 B） | 19/19 |
| board-diagnostics:623 | board-direction-check:192 | DirectionGapExemptMargin configurable | 12/12（1221 B） | 3/3 |
| board-research:7 | board-signal-reports:9 | 周期材料与固定候选快照 | 26/26 | 8/8 |
| board-research:129 | board-signal-reports:119 | 四十轮问题驱动研究 | 25/25 | 8/8 |
| board-research:541 | board-level-analysis:105 | 深度调查必须评估竞争假设 | 7/7 | 2/2 |
| board-research:843 | board-level-analysis:362 | 旧研究历史只读访问 | 11/11 | 3/3 |

阈值样例（默认 0.10 / 1e-9 / DirectionFloor / 默认 0.5 / 默认 0.15）在两侧均逐字相同。

### C 来源注释

`grep -h '^<!-- from:' $C/{board-diagnostics,board-research}/spec.md | sort | uniq -c` → 58 条、4 个来源：

| 来源 | 条数 | 目录存在 | 排序最前 | delta 类型 | 后续 change 也碰过 |
|---|---:|---|---|---|---|
| board-health-checkup（2026-10-10 add-board-health-checkup） | 12 | 是 | 是 | ADDED（8 条） | fix-board-health-llm-misread / board-health-triage-advice / fix-board-health-triage-degrade / fix-board-health-magnet-signal |
| board-direction-check（2026-05-29 board-direction-check-and-board-editing） | 10 | 是 | 是 | ADDED（7 条） | fix-digest-tag-leak（RENAMED）/ fix-board-direction-gap-exempt / fix-board-ghost-false-positive |
| board-signal-reports（2026-09-26 board-signal-reports） | 15 | 是 | 是 | ADDED（11 条） | add-fast-frequency-datasources / add-topic-key-event-markers / redesign-enrichment-research-workbench |
| board-level-analysis（2026-08-31 board-level-deep-analysis） | 21 | 是 | 是 | ADDED（16 条） | fix-board-analysis-material / add-evidence-backed-cross-board-relations / redesign-enrichment-research-workbench |

无「MODIFIED 当来源」：`merge-spec-family.sh` `first_change()` 已改为只认 `## ADDED Requirements`。备注（约定所致，非本次缺陷）：注释写的是 capability 的创建 change，58 条里有 18 条实际由后续 change 引入（如 `DirectionGapExemptMargin configurable` ← 2026-10-24、`分诊合理性闸门` ← 2026-11-10），读的时候按「来自 capability X（创建于 Y）」理解。

### D business/impl

`spec-classification.txt`：56 business / 2 impl，序号 14（Direction check data loading）、16（direction_mismatch persisted）与 delta 的 `<!-- kind: impl -->` 位置一致，两条都排在族末。

- `Direction check data loading`：正文是 `MatchTopicTag` 加载 `topic_tag_embeddings` / `semantic_labels` 并传入 `evaluateSemanticBoardMatches`；两条 Scenario 的行为后置条件（缺 embedding 跳过校验）在 business 段 L444 块「tag identity embedding 缺失时跳过全部判定…挂载版块自身 embedding 缺失时跳过对应门」与 Scenario「tag or board embedding unavailable」有落点；上一族 tag-matching 把同源的「MatchTopicTag 加载方向校验数据 / evaluateSemanticBoardMatches 接收方向校验数据」判 impl，口径一致。
- `direction_mismatch persisted`：schema 列 + `replaceTopicTagBoardLabels` 写入，属施工记录；其行为后置条件在 L548（日报排除）、L559（文章 API 过滤）有 business 落点。混合条目，可接受（同上一族 R-12）。
- 4 条配置项（L533 DirectionSimThreshold / L589 DirectionRelativeMargin / L608 Direction floor / L623 DirectionGapExemptMargin）：默认值与「≤0 关闭」语义直接决定 mismatch 判定，改默认值影响日报结果 → business 成立；`DirectionGapExemptMargin configurable` 第二个 bullet（LoadConfig 白名单、parse 语义）偏 impl，属混合。
- `旧论文式版块报告兼容` / `旧研究历史只读访问`：向后兼容的用户可见承诺（红线 41）→ business 成立。
- 反向漏判：56 条 business 标题全读，可疑正文（`成功快照与无评审输出` 的 schema_version/NULL 列、`调查综合只修复可证明的单一根终止符` 的 JSON 修复算法、`失败恢复与周期查询` 的分页参数）都带对外行为契约，未见纯施工记录。倾斜成因：3 个 spec 成文于 2026-08 之后的行为契约时期，只有 2026-05 的 direction-check 带施工味条目。

### E 族边界

- E2 耦合：`boardhealth/metrics.go:55` SQL 读 `COALESCE(ttbl.direction_mismatch, false)`；`signals.go:59-61` 磁铁信号 good/failed 两侧按 `direction_mismatch` 切分。体检确实只读消费方向校验的产物，合并后定义与消费同卷（但 L278 的磁铁信号排在 L439 的定义之前，原样搬运不允许加指针，读的时候要往后翻）。
- E3 共性：`dataenrichment/handler/analysis_runner.go:43-49` 同一 runner 挂 `board_brief` / `board_investigation` / `board_signal_discovery`，409 互斥、job id 轮询、不可变快照、可核查引用是真实共享骨架；分析链各走各的（七源 agent loop + calculate + compose vs 内部泳道 + 多假设 + 可选 web_search），产品面按红线 41 刻意分开（独立工作台、历史列表不混 signal_report）。共性成立但只在基础设施层。
- E4 radar：`openspec/specs/radar-pipeline-status/spec.md` marker `semantic-board, daily-report`，全文无 checkup / direction 字样；与体检的唯一连带是「手动生成日报成功后触发体检」，走的是日报域（radar 的 marker 已含 `daily-report`）。保留独立成立；proposal「与后端分析无改动连带」略夸，但不影响结论。
- E1：见 R-2。

### F 归档可行性

F1：
```
openspec validate merge-board-diagnostic-specs            → Change 'merge-board-diagnostic-specs' is valid（exit 0，17 WARNING，0 ERROR）
openspec validate merge-board-diagnostic-specs --strict   → exit 1（17 WARNING：超长 11 / 缺 SHALL-MUST 6）
openspec validate --specs                                 → Totals: 158 passed, 0 failed
```

F2：4 个 REMOVED delta 的标题序列与各自 main spec 的 `### Requirement:` 序列逐字同序一致（58/58），openspec 解析器消费成功（探针 counts.removed = 12 / 10 / 21 / 15）。

F3：`.openspec.yaml` 含 `schema: syntopica-ui` + `retire_capabilities: true`。

F4：tasks 4.3「摘 marker」必要性——`dist/core/archive.js:78-81` `isRetirementCandidate` 要求 `unaccountedContent.length === 0`；`specs-apply.js:642-690` `contentTheMergeCannotName` 把 `## Requirements` 之前除标题与 Purpose 节以外的任何非空行（含 `<!-- domain: -->`）算作 leftover。探针（`node`，import openspec dist，只读）：
```
board-health-checkup   removed=12 noReqBlocks=true unaccountedContent=["<!-- domain: semantic-board -->"] retirable=true
board-direction-check  removed=10 noReqBlocks=true unaccountedContent=["<!-- domain: semantic-board -->"] retirable=true
board-signal-reports   removed=15 noReqBlocks=true unaccountedContent=["<!-- domain: semantic-board, data-enrichment -->"] retirable=true
board-level-analysis   removed=21 noReqBlocks=true unaccountedContent=["<!-- domain: semantic-board, data-enrichment -->"] retirable=true
board-diagnostics      added=22 exists=false lines=655  Purpose="TBD - created by archiving change merge-board-diagnostic-specs. Update Purpose after archive."
board-research         added=36 exists=false lines=833  Purpose="TBD - created by archiving change merge-board-diagnostic-specs. Update Purpose after archive."
```
leftover 只有 marker 一行 → 摘掉即充分；不摘则走 'write' → 空 spec 校验失败 → 整次中止。4.3 成立。

F5：`bash scripts/harness/archive-readiness.sh merge-board-diagnostic-specs` → ①②③⑤⑥⑦ ✓，④ ✗（缺 review.md）；`check-standards.sh --change merge-board-diagnostic-specs` → 通过 338 / 失败 0 / 警告 2（J 段：两个新族尚无 main spec，预期内）。

F6 marker 方案：4 个退役 spec 的 marker 为 `semantic-board`（health、direction）与 `semantic-board, data-enrichment`（signal、level）。board-diagnostics → `semantic-board`、board-research → `semantic-board, data-enrichment` 恰为成员并集，零丢域（design D6 只提了 signal-reports 的 data-enrichment，level-analysis 原本也带，结论不变）。

### G 遗漏

G1：路径引用 1 处（R-5）；docs/reference 40 余处均为 change 名溯源，不需改。G2：`radar-pipeline-status` 无交叉引用要改。G3：tasks 该列未列——补 Purpose（R-1）、data-enrichment 溯源表（R-5）、孤儿数字（R-3）、4.4/4.5 时序与 4.3 同提交（R-4）。

### H 风险

H1：change 目录未跟踪、4 个 spec 与脚本干净，提交后 `git revert` 可完整恢复（删 4 / 新 2）。工作树另有 81 条与本 change 无关的脏项（62 个已跟踪文件：`topic-lane-auxiliary-label-constraint` 等在途 change 制品、`board-concept-management` / `board-lane-dynamics` 等 5 个其它 main spec、backend/front 文件；均非本审查所改）——归档门禁会对并发脏文件 warn，提交时只挑本 change 的文件。H2：board-research 归档后 833 行、board-diagnostics 655 行，均低于上一族 D5 的 1200 / 1500 档（design D1 的「design D5 的 1500 红线」指的是上一族 D5 / doc-authoring「族内碎片 >1500 行」，本 design 的 D5 是归档机制，自指易误读）。H3：备选切法见 R-2。

### brief 与现实的冲突（如实记录）

- brief 期望族稿「615 / 769 行」，实为来源 spec 之和；delta 672 / 866、归档后 655 / 833（R-4）。
- brief D3 只点名 4 条配置项；`DirectionGapExemptMargin configurable` 含 impl 味第二 bullet，按混合条目判 business。
- brief F5 问 board-level-analysis 原 marker：`semantic-board, data-enrichment`，与 signal-reports 相同，不丢域。
- brief H2「board-research 867 行」：delta 实测 866（`wc -l` 865），归档后 833。
- 本报告按 brief 落在 `review.md`，与门禁 `review-trace.sh` 同名，无冲突。
