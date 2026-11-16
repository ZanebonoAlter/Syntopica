# Review — lane-manual-correction-loop

> 审查档位：实现/评审（change-review，只读）
> 审查日期：2026-11-15
> 审查对象：`proposal.md` / `design.md` / `ui-design.md` / `tasks.md` / `test-cases.md` / `specs/**`（7 份 delta）/ `ui-verification/report.md`
> 对照实现（只读抽查）：`backend-go/internal/topicgraph/`（handler + repository 新增件）、`backend-go/internal/tagmanagement/handler/board_crud_handler.go`、`backend-go/internal/platform/database/lane_manual_correction_migration.go`、`front/app/features/tags/components/console/Lane*`、`front/app/features/tags/composables/useLaneCorrection.ts`、`front/app/api/dailyReports.ts`
> 红线基线：`docs/reference/flow/topic-graph.md` 17–22、`daily-report.md` 50–54、`semantic-board.md` 35

## 总评

实现侧与红线对齐得很干净：四列 NULL / `lane_tier` 保留、留痕追加不覆盖、命中计数与 status 不动、不立即重算质心、刷新范围 = touched ∪ pending、缓存失效不重跑匹配表、自然语言 4xx —— 逐条抽查均命中实现与真实测试（repository 层 8 组新断言 + handler 层 11 组 + 前端 11 个测试文件）。三条迁移纯加法，flow 红线与 API 文档已同步，`openspec validate --strict` 通过。

问题集中在**制品层**，不在代码层：同一 change 的三份制品对「取消勾选 = 移除」给出相反契约；delta spec 的两个「留痕可查」Scenario 在实现里没有对应查询能力（其中一个在数据模型上就不可满足）；ui-design 的首义范围「可跨话题」与 spec 的「必须属本话题」相反；ui-verification 的 `fail 0` 统计与它自己记录的 D-1（1440×900「节点不重叠压字」不达标）相矛盾。**这些不修就会随归档写进主 spec**，建议归档前先处置 R-1～R-4。

## 问题清单

### R-1 2026-11-15【High】同一 change 内三份制品对「取消勾选 = 移除」的交互给出相反契约
- **状态**：已采纳（文档侧已统一为「取消勾选即直接解除归属」：workshop-lane-compose delta Scenario 改写 + ui-design 浮层层级删「目标话题选择器」行 + test-cases §6 加注）

**问题**
本 change 对「取消勾选成员」这一主动作存在两套互斥契约：

| 制品 | 口径 |
|---|---|
| `specs/lane-correction-surface/spec.md` Scenario「取消勾选即移除」 | SHALL **直接移除**，SHALL NOT 弹出目标话题选择器、SHALL NOT 要求指定去处 |
| `specs/workshop-lane-compose/spec.md` Scenario「取消勾选为破坏性动作」 | SHALL **就地展开目标话题选择器**（不叠弹窗），SHALL 与「取消选中」视觉区分 |
| `ui-design.md`「浮层层级」 | 「剔除时的目标话题选择器：**就地展开**，**不叠第二层弹窗**」 |

而 `ui-design.md`「已记录的原型修正 v4」又写「『移除』不再选归处」——ui-design 自身内部也打架。

实现按 `lane-correction-surface` 走（`useLaneCorrection.ts` 的 `removeMember()` 只发 `PATCH /sections/:id/assignment {topic_id:null}`，全程无选择器），`ui-verification/report.md` A-2 实测弹窗数恒 0。`test-cases.md` §6 全量对账把 workshop 的该 Scenario 静默映射到「B1 视觉区分断言」，期望写的是「零弹窗」——矛盾被用例账本掩盖掉了。

**影响**
归档时 `workshop-lane-compose` 的 delta 会把「就地展开目标话题选择器」写进主 spec，与 `lane-correction-surface` 主 spec 正面冲突；后续按主 spec 实现的人会重新引入去处选择器，直接违反 D4「移除不需要指定去处」与用户评审 v4 结论。

**证据**
- `specs/lane-correction-surface/spec.md`（Scenario 取消勾选即移除）
- `specs/workshop-lane-compose/spec.md`（Scenario 取消勾选为破坏性动作）
- `ui-design.md`「浮层层级」段 + 「已记录的原型修正 v4」段
- `front/app/features/tags/composables/useLaneCorrection.ts`（`removeMember`）
- `openspec/changes/lane-manual-correction-loop/ui-verification/report.md` A-2

**建议**
把 `workshop-lane-compose` 的该 Scenario 改写为「取消勾选即直接解除归属（不弹去处选择器），SHALL 与『取消选中』在视觉与交互上明确区分」；同步删掉 `ui-design.md`「浮层层级」里「剔除时的目标话题选择器」一行，并在 `test-cases.md` §6 注明该 Scenario 已随 v4 改写。

---

### R-2 2026-11-15【High】delta spec 的两个「留痕可查」Scenario 在实现里没有对应能力（一个在数据模型上就不可满足）
- **状态**：已采纳（归属留痕 append-only 表 section_assignment_overrides + operator + 两个历史查询端点已落地，spec/API 文档已同步；测试覆盖升序/多行/404）)

**问题**
`specs/lane-manual-correction/spec.md` Requirement「人工修正留痕与可追溯」明写「两者 SHALL 携带时间戳与操作者标识」，并给了两个查询型 Scenario：

- 「首义改写历史可查」：改写 2 次后**查询该话题的首义改写历史** → SHALL 返回 **2 条**留痕记录（改前值、改后值、输入模式、section id 列表、时间），SHALL 按时间升序。
- 「移除留痕可查」：section #501 **曾被从 #8 移除、之后又被加回** → 查询该 section 的留痕 → SHALL 返回变更时间、变更前话题 id 与动作（detach / attach）。

实现侧：首义留痕只有写（`Create`）与「计数 + 取最新一行」读（`daily_report_anchor_read.go`），**没有任何历史列表查询**——`GET /topics/:id/anchor` 只返回 `override_count` 与最新一行的 `section_ids`，拿不到 2 条记录，也拿不到改前/改后向量。归属留痕侧：`manual_assignment_at / _from_topic_id / _action` 三列在 detach / attach / 批量 detach 三条路径上**互相覆盖**（`daily_report_topic_repository.go` 的 detach、attach、批量 detach 三处 Updates），一次「加回」就把先前的 `detach` 痕迹连同 `from_topic_id=8` 一起抹掉，且三列**没有 operator 列**，也没有任何读出口。

**证据**
- `specs/lane-manual-correction/spec.md`（Requirement「人工修正留痕与可追溯」+ 两个留痕 Scenario）
- `backend-go/internal/topicgraph/repository/daily_report_anchor_read.go:63-85`（只有 count + latest）
- `backend-go/internal/topicgraph/repository/daily_report_topic_repository.go:1288`（写）、`:1663-1665`（detach 写）、`:1739-1741`（attach 覆盖写）、`:1813-1815`（批量 detach 写）
- `backend-go/internal/topicgraph/handler/daily_report_handler.go:68-75`（只有 PATCH/GET anchor，无 history 路由）
- 反证（实现自己留下的痕迹）：`ui-verification/report.md` §5「真库残留」——section 8174 走完「移除 → 加回」后只剩 `manual_assignment_action='attach'`，原先的 detach 记录已不存在
- 用例账本把缺口掩盖了：`test-cases.md` §1 A4 把「首义改写历史可查」映射成「`topic_anchor_overrides` 追加一行，既有行不动（append-only）」——写侧断言，不是查询侧断言

**影响**
「这条内容为什么被移出」在一次加回后就再也答不出来——正是 Requirement 自己承诺的可追溯性；两个 Scenario 随归档进主 spec 后即为未兑现的契约。

**建议**
二选一并同步改 spec / 红线 / 用例：① 归属留痕也改成 append-only 追加表（与首义留痕同构），补 operator 列，并新增一个留痕查询端点（首义历史 + 归属历史）；② 若坚持三列覆盖写，则把两个 Scenario 改写成「最近一次变更可查」，并把 Requirement 的「可追溯」承诺与「两者 SHALL 携带操作者标识」收窄到与设计一致（去 operator、补「仅保留最近一次」的诚实措辞）。

---

### R-3 2026-11-15【High】ui-design「首义范围可跨话题」与 delta spec「必须属本话题否则整体拒绝」相反
- **状态**：已采纳（ui-design 主操作表「设定首义范围」改为「限本泳道节点 + 当前首义集；跨话题节点服务端 4xx 拒绝」并补边界说明段）

**问题**
`ui-design.md`「Interaction Contract → 主操作」表里「设定首义范围」一栏写的是：在左栏「首义范围」勾选一组节点（**可跨话题**）→ 服务端按这组节点的 embedding 取均值写入首义。

而权威 delta spec 写的是：被选定的 section SHALL 满足「属于同一 board、**当前归属该话题**、embedding 存在且维度一致」，任一条不满足 SHALL **拒绝该次写入**（整体不落库）。实现按 spec 走（`loadAnchorSectionVectors` 命中 `not_in_topic` 即 400 拒绝）。

**证据**
- `ui-design.md`「主操作（全部在画布内）」表 设定首义范围 行
- `specs/lane-manual-correction/spec.md` Requirement「人工重设泳道首义」第 3 段 + Scenario「选定节点跨话题时拒绝」
- `backend-go/internal/topicgraph/repository/daily_report_topic_repository.go`（`loadAnchorSectionVectors` → `Reason: "not_in_topic"`）

**影响**
major 档 UI 契约制品与 spec 正面矛盾：后续实现者按 ui-design 做跨话题选择池，用户点提交只会拿到 4xx；反之按 spec 做则 ui-design 需要回写。归档后 ui-design 作为权威制品继续误导。

**建议**
把 ui-design 该单元格改为「勾选一组节点（限本泳道节点 + 当前首义集；跨话题节点服务端 4xx 拒绝）」，并在该行补一句与 spec 一致的边界说明。

---

### R-4 2026-11-15【High】ui-verification 统计「fail 0」与自身记录的 D-1（1440×900 节点压字不达标）相矛盾
- **状态**：已修复（D-1 已按主线程裁定②「标题描边光晕」落地并双视口复核，report.md §5 记「已修」）

**问题**
`ui-design.md`「目标视口 → 1440×900 验收要点」明写一条硬要求：**画布节点不重叠压字**。

`ui-verification/report.md` §5 D-1 自陈该条**不达标**：1440×900 与 1920×1080 下，42 个节点标题里 **22 个**与成员勾号圆发生几何重叠（「ui-design『1440×900 视口验收要点』明写『中央画布节点不重叠压字』→ 该条不达标」）。

但报告 §2 的 A-1~A-12 结论表里**没有任何条目承载「视觉检查（两档视口）」这条验收要点**（A-1~A-12 全是机械断言），统计口径是「pass 8 / fail 0 / 未覆盖 4」；`scripts/harness/ui-verification-trace.sh` 只数条目不校内容（实测输出「✓ 通过：report.md 建账 12 条（pass 8 / fail 0 / 未覆盖 4）」）。即：一个已知不达标的验收项在门禁账本里既没有 fail 条目、也没进「视觉检查」条目，只有 §5 的缺陷清单记着。

**证据**
- `ui-design.md`「目标视口」表 1440×900 行
- `ui-verification/report.md` §2 结论表 + §5 D-1（含 22/42 的量化）
- `bash scripts/harness/ui-verification-trace.sh openspec/changes/lane-manual-correction-loop` 实跑输出

**影响**
归档门禁按「fail 0」放行，实际带着一条明文不达标的视觉验收项；只读 §2 的读者（主线程 / 用户 / 后续 change）会得到「视觉验收全过」的错误结论。

**建议**
在 report.md 里把「视觉检查 1440×900 / 1920×1080」立为独立建账条目（结论按实测写 fail 或 pass-with-caveat），并与 D-1 双向绑定；D-1 的三条修法需要主线程 / 用户拍板（报告已按「不自处置」纪律上报），归档前必须有处置结论（修 / 显式豁免并写明理由）。

---

### R-5 2026-11-15【Medium】ui-design 仍以「顶栏保存 + 提交本轮全部修正」描述工作面，与已裁定的逐条即时落库不一致
- **状态**：已采纳（ui-design 已改为「逐条即时落库（无全局保存动作）」：主操作表 / 区块划分 / State Matrix / Component Reuse + 信息架构图去掉顶栏「保存」）

**问题**
`ui-design.md` 里至少三处仍按「先改后保存」描述修正工作面：

- 「主操作」表末行：**保存** → 顶栏「保存」→ 提交本轮全部修正 → toast「已保存，次日日报生效」；
- 「State Matrix → 画布」：success · 保存 → toast「已保存，次日日报生效」；画布退出修正态；error · 保存失败 → 画布底部行内错误，保留本轮未提交的改动；
- 「Component Reuse」：修正工作面外壳复用「左栏 + SVG 画布 + 窗口控制 + 名称 / **保存**」。

`ui-verification/report.md` §4 ① 已把「顶栏无保存按钮（逐条即时落库）」记为主线程裁定的既定差异，但 **ui-design.md 本身没有回写**。

**证据**
- `ui-design.md`「主操作」表保存行 / 「State Matrix」success·保存 与 error·保存失败 行 / 「Component Reuse」首行
- `ui-verification/report.md` §4 差异说明 ①

**建议**
在 ui-design 里把这三处改成「逐条即时落库（移除 / 加回 / 首义各自一次请求）」，并把 State Matrix 的 success·保存 / error·保存失败 换成「success·单条操作 / error·单条操作失败（保留本轮未提交改动）」；与 R-1 一并回写。

---

### R-6 2026-11-15【Medium】test-cases 故事 C3「归档泳道」的判据错位（把批量移除某日当归档落点，且丢掉「可逆」断言）
- **状态**：已采纳（test-cases C3 判据改为 `sm` 弹窗 + `useConfirm()` 二次确认 → `PATCH /daily-reports/topics/:id {status:'archived'}`，批量移除某日归位 C5，新增 C6 归档可逆断言）

**问题**
`test-cases.md` §1 故事 C3 行：

| 步 | 动作 | 来源 Scenario | 期望（断言判据） | 层 | 落点 |
|---|---|---|---|---|---|
| C3 | 「归档」 | lane-correction-surface#归档泳道可逆 | `sm` 手打名称确认；**`POST /topics/:id/detach-date`**；不改 status/hit_count | handler(PG) + Vitest | `daily_report_handler_test.go` |

`POST /topics/:id/detach-date` 是**批量移除某日**（故事 C5 的动作），不是归档。Scenario「归档泳道可逆」要求的是「二次确认后置 `status=archived`」且「该操作 SHALL 可逆（切回启用即恢复）」——C3 的判据既指错了端点，也**完全没有承载「可逆」这条断言**（账本里没有任何一条用例断言「归档后能切回 active」）。

实现是对的（`LaneArchiveDialog.vue` 走 `updateTopic(id, {status:'archived'})` → `PATCH /daily-reports/topics/:id`），但用例账本会误导后续补测。

**证据**
- `test-cases.md` §1 故事 C3 / C5 两行
- `specs/lane-correction-surface/spec.md` Scenario「归档泳道可逆」
- `front/app/components/dialog/LaneArchiveDialog.vue:58`（`updateTopic(props.topicId, { status: 'archived' })`）

**建议**
C3 判据改为「`sm` 弹窗 + `useConfirm()` 二次确认 → `PATCH /daily-reports/topics/:id {status:'archived'}` → toast 含『不再吸附新内容』；不改 hit_count / consecutive_hits」，并新增一条「归档后 `PATCH {status:'active'}` 可恢复（可逆）」的断言与落点。

---

### R-7 2026-11-15【Medium】ui-design A-3 验收判据与 delta spec 口径冲突（确认条只在「覆盖已有首义」时出现）
- **状态**：已采纳（ui-design Acceptance A-3 拆两段：首次设定直接提交 / 覆盖已有首义才出画布内确认条）

**问题**
- `ui-design.md` Acceptance A-3：勾选 2 个节点 → 点「用选中的 2 个节点定首义」→ **确认条出现** → 提交 → toast 含「次日」字样（写成无条件出现）。
- `specs/lane-correction-surface/spec.md` Scenario「首义范围手动勾选」：系统 SHALL 弹出画布内确认条（**覆盖已有首义时**）。
- 实现按 spec 走（`requestAnchorApply()` 仅在 `anchorSource==='manual' && anchorSectionIds.length` 时出确认条）；自动首义泳道点按钮直接提交。

`ui-verification/report.md` A-3 把「未覆盖」的原因写成两条：真库首义全为 auto → 点按钮直接落库 → 按写边界不点提交；以及「自动首义场景下点按钮会直接落库，无法停在『确认条出现』这一步」。真实原因里还叠了一层**判据本身与契约不一致**：A-3 把「确认条出现」当成流程必经步，而契约只要求覆盖时出现。

**证据**
- `ui-design.md` Acceptance 表 A-3 行
- `specs/lane-correction-surface/spec.md` Scenario「首义范围手动勾选」
- `front/app/features/tags/composables/useLaneCorrection.ts`（`requestAnchorApply`）
- `ui-verification/report.md` A-3

**建议**
把 A-3 判据改成两段：「自动首义 → 点按钮直接提交 → toast 含『次日』」与「人工首义（覆盖）→ 确认条出现 → 确认后提交」；在 report.md A-3 的「未覆盖」里把「判据与契约不一致」与「真库写边界」分开陈述，避免用写边界掩盖判据问题。

---

### R-8 2026-11-15【Low】删除 `BoardEditDialog.vue` 后，`docs/reference/flow/data-enrichment.md` 仍把它列为活组件
- **状态**：已采纳（data-enrichment.md 组件指代改为现状：治理工作面「设置」页签 `console/BoardSettingsForm.vue` 持自动发现开关 + 雷达行 ⋯ 轻编辑 `console/BoardQuickEditDialog.vue`，并补变更溯源行）

**问题**
task 6.6 删除了 `front/app/features/tags/components/BoardEditDialog.vue`，design 的风险条要求「删除前以 grep 全仓复核（含 `front/` 与 `docs/`）」，但只要求同步修正 `docs/reference/flow/semantic-board.md`；`docs/reference/flow/data-enrichment.md:326` 仍把 `BoardEditDialog.vue`（自动发现开关，默认关闭）列为「前端跨版块关系」的现役组件。

**证据**
- `docs/reference/flow/data-enrichment.md:326`
- git status：`D front/app/features/tags/components/BoardEditDialog.vue`
- `docs/reference/flow/semantic-board.md:28`（已正确记为「三套僵尸实现已删除」）

**建议**
把 data-enrichment.md 该行的组件指代改为删除后的实际落点（自动发现开关的现役位置），或直接去掉该括注。

---

### R-9 2026-11-15【Low】前端仍有 5 处注释指向已删组件，task 11.3 的「修正过期组件指代」只覆盖了 flow 文档
- **状态**：已采纳（前端 5 处过期组件注释已清，全仓 grep 零残留）)

**问题**
task 11.3 的验收口径是「修正已过期的组件指代」，实际只改了 `docs/reference/flow/semantic-board.md`；前端源码注释里仍有 5 处指向已删文件：`front/app/features/tags/composables/useBoardSources.ts:10`（`BoardGovernanceRail`）、`front/app/features/tags/components/BoardSourcePanel.test.ts:7`（`console/BoardGovernanceDrawer.test.ts`）、`front/app/features/tags/components/console/BoardSettingsForm.vue:42` 与 `:73`（`BoardEditDialog`）、`front/app/components/ui/AppDrawer.vue:11`（`BoardGovernanceDrawer`）。

**证据**
- 上述 5 处 `grep -rn "BoardEditDialog\|BoardGovernanceDrawer\|BoardGovernanceRail" front/ --include='*.vue' --include='*.ts'` 命中（全部为注释）

**建议**
同一批把这几处注释改成现役组件名，避免下一个接手的人照着注释去找已删文件。

---

### R-10 2026-11-15【Low】tasks 勾选与制品实况有两处对不齐
- **状态**：已采纳（test-cases §3 补「留痕缺失 / 改挂拒绝 / 待重算标记三分支」分支与边界值后勾 7.4；12.5 命令改为仓库实存脚本 `archive-readiness.sh` + `ui-verification-trace.sh`）

**问题**
1. task 7.4「在 test-cases.md 补『白盒附加』节」未勾，但 `test-cases.md` §3「白盒附加（分支表 + 边界值）」已经存在（A1–E1 分支表 + 边界值清单 + 不适用划除留痕）——只是缺任务点名的「留痕缺失」分支，属于交付物已到位却没勾（勾选与实况反向不符）。
2. task 12.5 点名的 `bash scripts/harness/spec-gate.sh` **在仓库里不存在**（`scripts/harness/` 下实际是 `archive-readiness.sh` / `ui-verification-trace.sh` / `check-standards.sh` / `doc-impact.sh` 等，无 `spec-gate.sh`）；执行前需替换为仓库等价脚本，否则该验证命令永远跑不通。

**证据**
- `tasks.md` 第 61 行（7.4）、第 98 行（12.5）
- `test-cases.md` §3
- `ls scripts/harness/`（无 `spec-gate.sh`）

**建议**
7.4 补齐「留痕缺失」分支后勾上；12.5 把命令改成仓库实存脚本（如 `bash scripts/harness/archive-readiness.sh <change-dir>` + `ui-verification-trace.sh`）。

---

### R-11 2026-11-15【Low】`centroid_pending_recompute` 清标记是无条件写 false，存在把人工刚置的标记抹掉的窄窗口竞态
- **状态**：已采纳（清标记改 correctionTrailVersion 条件更新，快照读不到保留标记为安全方向；3 条并发回归用例绿）)

**问题**
`refreshCentroidsIsolated` 的顺序是「重算质心 → 清标记」，而 `clearCentroidPendingFlag` 是无条件 `UPDATE ... SET centroid_pending_recompute = false`。若一次人工修正恰好落在「重算完成」与「清标记」之间，本次清标记会把**刚刚置上的新标记**抹掉 → 该次人工修正的质心刷新被静默跳过（且此后没有 pending 标记兜底，泳道当天若无新命中就长期停在旧质心）。窗口很窄，但这个标记正是 D2「人工修正最终落地」的唯一保障。

**证据**
- `backend-go/internal/topicgraph/repository/daily_report_topic_repository.go`（`refreshCentroidsIsolated` / `clearCentroidPendingFlag`）

**建议**
清标记改为条件更新（仅当标记仍是本轮开始时读到的值才清），或改为「重算前记录该话题的 `updated_at` / 标记版本，清标记时带上版本条件」；补一条并发用例（重算期间置位 → 标记不被清）。

---

### R-12 2026-11-15【Low】前端 API 客户端的响应类型声明与后端实际返回不符
- **状态**：已采纳（三个 TS 响应类型已按后端 json tag 补齐，typecheck 0 error）)

**问题**
`front/app/api/dailyReports.ts` 里三个新方法的返回类型声明与后端真实响应不一致：

- `updateTopicAnchor` 声明 `ApiResponse<PersistentTopic>`，后端实际返回 `{ topic_id, source, selected_sections, override_count, effective_from }`；
- `setSectionAssignment` 声明 `{ section_id, topic_id }`，后端实际还返回 `previous_topic_id / action / changed / topic_match_distance / board_id`；
- `detachTopicDate` 声明 `{ topic_id, period_date }`，后端实际还返回 `detached / section_ids / board_id`。

当前 `useLaneCorrection.ts` 只读 `res.success` 与本地状态，功能不受影响，但类型层会把后续按类型消费的调用方（如想直接读 `override_count` 或 `previous_topic_id` 做 UI）引到错误的字段上。

**证据**
- `front/app/api/dailyReports.ts:391`、`:412`、`:417`
- `backend-go/internal/topicgraph/repository/daily_report_topic_repository.go`（`AnchorFromSectionsResult` / `AssignmentChangeResult` / `BatchDetachResult` 的 json tag）

**建议**
按后端 json tag 补三个 TS 响应类型（`TopicAnchorWriteResult` / `AssignmentChangeResult` / `BatchDetachResult`），或直接复用 `docs/reference/api/daily-reports.md` 已登记的响应体定义。

---

## 核实记录

**已复现的验证命令（只读）**

| 命令 | 结果 |
|---|---|
| `openspec validate lane-manual-correction-loop --strict` | ✅ `Change 'lane-manual-correction-loop' is valid` |
| `bash scripts/harness/ui-verification-trace.sh openspec/changes/lane-manual-correction-loop` | ✅ 建账 12 条（pass 8 / fail 0 / 未覆盖 4），点名截图 10 张全在盘（但见 R-4：统计口径漏掉视觉检查条目） |
| `bash scripts/harness/check-tasks-tail.sh openspec/changes/lane-manual-correction-loop` | ✅ 尾三节与 doc-impact 声明标记齐全 |
| `bash scripts/harness/review-trace.sh openspec/changes/lane-manual-correction-loop` | 审查前 ❌（零痕迹），本文件建账后应通过 |
| `cd backend-go && go build ./...` | ✅ exit 0，无输出 |
| `cd backend-go && go vet ./internal/topicgraph/... ./internal/tagmanagement/...` | ✅ exit 0（含测试文件编译） |
| `cd front && pnpm exec nuxi typecheck` | ✅ exit 0（仅 node DEP0205 弃用告警，无 TS 错误） |

**未复现（留待主线程按影响包跑，未在本次只读审查内执行）**

- `go test ./internal/topicgraph/...`、`go test ./internal/tagmanagement/...`（需 testcontainers 起 PG，树莓派上并行易撞 Docker 超时；本次只做编译级核验）
- `pnpm test:unit`（12 个受影响文件）、`pnpm lint`、`pnpm build`
- `bash scripts/harness/db-migration-safety.sh`（tasks 12.6 仍未勾）

**契约-实现逐条核对（抽查结论：实现侧全部符合）**

| 契约点 | 实现落点 | 结论 |
|---|---|---|
| 移除四列置 NULL、`lane_tier` 保留 | `daily_report_topic_repository.go` detach 三处 Updates（不含 lane_tier）+ 测试 `DetachClearsSnapshotAndKeepsLaneTier` 用裸 SELECT 复核「真 NULL」 | ✅ |
| 加回 `confidence=manual` + 实算距离（centroid 优先 / embedding 兜底） | `attachSection` + `topicAnchorVec` + 测试 `AttachWritesManualSnapshot`（按质心算距离，断言 1.0） | ✅ |
| 加回幂等（已属目标 → 200，不写列、不追加留痕） | `attachSection` 幂等分支 `Changed=false` + 测试 `AttachToSameLaneIsIdempotent` | ✅ |
| 不提供改挂 | `ErrSectionAlreadyAssigned` → 409 + handler 测试 `RejectsReparent` | ✅ |
| 不动 hit_count / consecutive_hits / status | 测试 `LeavesHitCountersAndStatusUntouched`（5 / 2 / active 前后相等） | ✅ |
| 不立即重算质心（只置标记） | `markTopicCentroidPending` + 测试 `FlagsAffectedTopicForRecompute`（centroid 值不变） | ✅ |
| 关系重建在同一事务、失败整体回滚 | `repo.rebuildRelations` 可注入 + 测试 `RebuildsRelationsAndRollsBackOnFailure` | ✅ |
| 留痕 append-only（首义） | `topic_anchor_overrides` 无 FK、追加写 + 测试 `AppendsHistoryOnRepeatedRewrite` | ✅（读侧缺口见 R-2） |
| 刷新范围 = touched ∪ pending；成功清标记 / 失败保留 | `RefreshBoardCentroids` + `refreshCentroidsIsolated` + 测试 `IncludesPendingTopicWithoutTouch` / `FailedTopicKeepsPendingFlag` | ✅ |
| 算法不写首义列 | `UpdateCentroidOnSectionChange` 只 UPDATE centroid + 测试 `DoesNotTouchEmbedding` | ✅ |
| 自然语言输入 4xx | handler 只认 `section_ids`；空数组 → 400、非数组 → 400 + 测试 `RejectsNonSectionIDBodies` | ✅ |
| 版块 label/desc 变更同请求内失效缓存、不重跑匹配 | `board_crud_handler.go` `boardVectorChanged` 分支调 `invalidateMatchCache()` + 3 个回归测试 | ✅ |
| 红线：人工修正只写事实层、不提供立即重算端点 | 三个写端点均不写 centroid / is_vacuum / 快照表 | ✅ |
| 红线：质心刷新范围按 board 收敛 | `ListPendingCentroidTopicIDs(boardID)` + 测试 `IsBoardScoped` | ✅ |
| 迁移纯加法 | `lane_manual_correction_migration.go` 三步：ADD COLUMN IF NOT EXISTS ×4 / CREATE TABLE IF NOT EXISTS + INDEX，无 DROP / TRUNCATE / 改类型 | ✅ |
| 入口不误触发（行头 / 行尾「⋯」） | `RowActionMenu.vue` 触发器 `@click.stop` + 测试「点 ⋯ 不 emit 行选中 / 不 emit openBoard」 | ✅ |
| 画布两栏、无右栏 | 修正模式 `bco__workbench--correction` 两子节点 + 测试「页面内不存在右栏」 | ✅ |

**未覆盖项正当性评估（report.md A-3 / A-6 / A-8 / A-12）**

四条「未覆盖」均源于真库写边界（首义重设不可逆 + 污染审计流；改名 / 改版块写真库标签列），主线程已裁定该边界，处置方式（只验证到弹窗 / 输入框出现 + 用单测替代提交步）合理，**理由站得住**。两点补充：① A-3 的「确认条出现」另有一层原因是判据本身与 spec 不一致（R-7），不只是写边界；② A-12 的替代单测（折叠带渲染 / ≤6 + 「+M」/ 选择池边界）覆盖度足够，人工首义侧未做 e2e 的风险可接受。四条均不影响归档判断，但 A-3 需按 R-7 改判据。

**tasks 勾选诚实度抽查（5 条）**

| task | 抽查方式 | 结论 |
|---|---|---|
| 10.1 / 10.2（后端单测 / handler 测试） | 测试函数逐个核对断言与落点（8 组 repository + 11 组 handler，均含真实断言） | ✅ 非虚勾 |
| 10.4（前端组件测试） | 11 个测试文件 103 个 it 用例逐条核对（两栏无右栏 / 点节点不改成员数 / 零弹窗 / 加回 / 首义确认条 / 就地改名 / 窄屏 / 无解说长句 / 菜单不误触发） | ✅ 非虚勾 |
| 11.4（API 文档登记） | `docs/reference/api/daily-reports.md` 四个端点表 + 三节正文；`semantic-boards.md` 缓存失效段 | ✅ |
| 8.1 / 8.2（UI 建账 + 截图） | 12 条目、10 张截图全在盘且被点名 | ✅（账本口径问题见 R-4） |
| 1.1–1.3（迁移） | 三个迁移文件 + `postgres_migrations.go` 注册行 | ✅ |

**红线符合性总览**

`topic-graph.md` 17–22、`daily-report.md` 50–54、`semantic-board.md` 35 六条新红线与实现逐条一致，未发现红线冲突；本 change 对既有红线文本的唯一改写（红线 17 首义条款）已完成且保留了「质心漂移有界」原意。实现侧未发现契约违反。
