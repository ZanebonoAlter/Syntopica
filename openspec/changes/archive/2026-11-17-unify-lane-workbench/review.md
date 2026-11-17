# review.md · unify-lane-workbench

- 审查时间：2026-11-16
- 审查档位：`change-review` 只读档
- 审查范围：制品（proposal / design / ui-design / test-cases / tasks / specs / ui-verification）+ 实现 diff

## 总评

实现整体质量高：三栏拆分、节点卡写操作、相关高亮、时间视窗、365 轻量曲线、OQ 收口五项均已真实落地；`openspec validate --strict`、`scenario-trace`（30/30）、`check-tasks-tail` 全过，定点抽测 5 个测试文件 84 用例全绿，7 条不变量 grep 全部通过，旧能力去向表无「遗漏」。

共 **1 High / 3 Medium / 4 Low**：

- **High×1**：API 归一层把 `early_members[].embedding` 整个丢掉，「窗口 0 成员时更早成员撑起推荐质心」（design D6 / tasks 5.1 / test-cases B11、E4、S3.6）在真实运行时不可达——单测因 mock 了 API 模块而假绿。
- **Medium×3**：delta spec 残留两处「左栏加入入口」文字；tasks.md 与实现严重脱节（51 项仅 1 勾 + 文档节未做）；major 档写操作无真实落库证据（X-1）。
- **Low×4**：陈旧注释×2、质心池合成 section 的脆弱展开、日期校验双实现、design 的 Scenario 计数失真。

**建议**：修复 R-1 后再归档；R-2/R-3 归档前必须收口（R-3 本身就会被归档门禁拦住）；R-4 建议部署前补一次真写验证。其余实现与制品一致性良好。

**处置结果（2026-11-16，实现子线程）**：R-1 / R-2 / R-3 / R-5 / R-6 / R-7 / R-8 已修复并逐条取证（见各条「处置取证」与文末处置状态汇总）；R-4 保持待处理——真写落库验证要写库，等主线程批准后由具写权限线程补，不阻塞本次收口。

---

### R-1 2026-11-16【High】归一层丢 `early_members[].embedding`，更早成员永远进不了推荐质心

- **状态**：已修复
- **定位**：`front/app/api/persistentTopics.ts:212-226`（`ComposeEarlyMemberPayload` 无 `embedding` 字段；`normalizeEarlyMember` 不拷贝）→ `front/app/features/tags/composables/useLaneCorrection.ts:391-397`（`buildEarlyMembers` 读 `row.embedding`，恒为 undefined）
- **问题**：期望（design D6「编辑质心继续含有效更早成员」、tasks 5.1、test-cases B11/E4/S3.6「窗口 0、全部 M>0 → 推荐质心仍含有效更早成员」）：后端 `early_members` 每行带向量（`ComposeEarlyMember.Embedding`，`daily_report_manual_topic.go:162`，仅向量可用者入集），前端用它扩容质心池。实际：`normalizeEarlyMember` 新造对象只拷 6 个字段，`embedding` 既不在 payload 类型里也没拷贝，`earlyEmbeddings` 恒空——窗口内成员为 0（或向量全无效）时质心为 null，**候选区/右栏推荐全空**，与 B11 契约相反。
- **依据**：`grep -n "embedding" front/app/api/persistentTopics.ts` → 只有 sections 侧（L34/106/166）有 embedding，`ComposeEarlyMemberPayload`（L212）与 `normalizeEarlyMember`（L285-297）无；后端 `getComposeEarlyMembers`（`daily_report_manual_topic.go:445-493`）SELECT `ds.embedding` 并解析后放入响应。**测试假绿原因**：`useLaneCorrection.test.ts:36` `vi.mock('~/api/persistentTopics')`，L83-84 的 EARLY_MEMBERS 直接以「已归一形态」带 `embedding: [0.9, 0.1]` 注入，绕过了真实归一层；`persistentTopics.test.ts` 无 early-member embedding 透传用例。本轮实跑 `useLaneCorrection.test.ts`（29 用例）全绿，证实 mock 层掩盖。
- **建议**：① `ComposeEarlyMemberPayload` 加 `embedding?: number[]`，`normalizeEarlyMember` 补 `if (p.embedding?.length) out.embedding = p.embedding`；② `persistentTopics.test.ts` 补「early_members[].embedding 透传 / 缺省为 undefined」用例；③ 复跑 `pnpm test:unit app/api/persistentTopics.test.ts app/features/tags/composables/useLaneCorrection.test.ts --maxWorkers=2`。
- **处置取证**（2026-11-16）：① `front/app/api/persistentTopics.ts:209` `ComposeEarlyMemberPayload` 增 `embedding?: number[]`，`:302` `normalizeEarlyMember` 补 `if (p.embedding?.length) out.embedding = p.embedding`（空数组不塞 `[]`，缺省留 `undefined`）；② `front/app/api/persistentTopics.test.ts` 新增 describe「early_members[].embedding 透传」两用例（有值透传 `toEqual([0.9, 0.1])` / 缺省与 `[]` 均 `undefined` 且断言 `embedding` 键不存在）；③ 实跑 `cd front && pnpm test:unit app/api/persistentTopics.test.ts app/features/tags/composables/useLaneCorrection.test.ts --maxWorkers=2` → 16 + 29 = 45 用例全绿。至此 `buildEarlyMembers` 的 `row.embedding` 有真实来源，B11/E4/S3.6 在真实运行时可达。


### R-2 2026-11-16【Medium】delta spec 残留两处「左栏加入入口」文字，与 D5 迁移后现实不符

- **状态**：已修复
- **定位**：`openspec/changes/unify-lane-workbench/specs/lane-correction-surface/spec.md:16`（动作表「加入节点」行：「…或**左栏对候选节点勾选**」）与 `spec.md:23`（「写操作入口 SHALL 有两处…① 左栏（成员勾选 / **「＋加入」** / 「加回来」/ 首义勾选）」）
- **问题**：期望（D5/OQ-3 收口：推荐与加入入口迁右栏，左栏只留已选侧）：加入入口 = 右栏「＋加入」+ 节点卡两处。实际：实现正确（`LaneMemberList` 无候选勾选、无加入按钮；加入只在 `LaneRecommendationList` 与节点卡），但 delta spec 仍声称左栏有「对候选节点勾选」和「＋加入」两个不存在的入口——OQ-3 收口记录只改了「左栏/同栏关联推荐」的位置措辞，漏掉这两处枚举。按此 spec 未来写测试/回归会对着一个不存在的入口失败。
- **依据**：`grep -rn "左栏.*勾选\|＋加入" specs/lane-correction-surface/spec.md`；对照实现 `LaneMemberList.vue`（emits 仅 remove/restore/focus）与 `BoardComposeOverlay.vue:517-526`（加入入口在右栏）。
- **建议**：修 delta spec 两处：动作表「加入节点」删「或左栏对候选节点勾选」；「写操作入口…① 左栏（成员勾选 / 「加回来」/ 首义勾选）」删「＋加入」（加入入口归②节点卡与右栏，三处共享同一份状态）。纯文字修订，无需重审原型。
- **处置取证**（2026-11-16）：`specs/lane-correction-surface/spec.md:16` 动作表「加入节点」行删「或左栏对候选节点勾选」（保留右栏「＋加入」/节点卡两处）；`:23` 「写操作入口」改为「① 左侧已选区（成员勾选 / 「加回来」/ 首义勾选）；② 节点卡；右栏「关联推荐」的「＋加入」与节点卡走同一加入写路径」，并把「两处」改「两类」以免与右栏入口计数冲突。复跑 `grep -n "左栏.*勾选\|＋加入" specs/lane-correction-surface/spec.md` → 左栏勾选已无命中，右栏「＋加入」/「关联推荐」表述仍在；`openspec validate unify-lane-workbench --type change --strict --no-interactive` → `Change is valid`。


### R-3 2026-11-16【Medium】tasks.md 与实现严重脱节：51 项仅 1 项勾选、头部仍声明「全部未执行」、文档节（任务 10）未做

- **状态**：已修复
- **定位**：`openspec/changes/unify-lane-workbench/tasks.md:3`（头部「本轮只建立计划，全部未执行」）+ 全文 `- [x]` 计数 1 / `- [ ]` 计数 51；`docs/reference/api/semantic-boards.md`（compose-candidates 节无 `clue_count`/`omit_embeddings`/365 上限记载）；`docs/reference/flow/semantic-board.md`（工作树零改动，任务 10.2 要求更新）
- **问题**：期望（§11.1 条件 2/3 + §0.6 apply 流程）：tasks 勾选反映实现、验证节命令实测、文档同步。实际：工作树已有完整实现 + `ui-verification/report.md`（16 pass 建账），但 tasks.md 全未勾、头部声明过时、任务 10 的 api/flow 文档确实没写——归档门禁（doc-impact verify / 11.15）必然拦截，且误导后续会话以为实现未开始。
- **依据**：`grep -c "\- \[x\]" tasks.md` → 1；`grep -c "\- \[ \]"` → 51；`git status --short docs/reference/flow/semantic-board.md` → 无输出（未改）；`grep -n "clue_count\|omit_embeddings" docs/reference/api/semantic-boards.md` → 0 命中。
- **建议**：归档前逐项核对勾选（含把头部声明改为实现态）、完成任务 10.1（semantic-boards.md 补 `section.clue_count` 非综合稿口径、0 必返、`omit_embeddings=1` 轻量参数、365 上限）与 10.2（flow/semantic-board.md、flow/daily-report.md 增补），再跑 `doc-impact.sh verify`。
- **处置取证**（2026-11-16）：**(a) tasks 勾选**：按代码事实逐条核对 52 项 → 48 项 `[x]`（关键落点：`laneComposeLogic.ts:402 pointAppearance` / `:116 relatedNodeIds(…, recommendedIds?)` / `:507 proximityOf` / `:580 candidatePoolDayTotals` / `:628 validateWorkbenchRange`；`repository/daily_report_manual_topic.go:97 ClueCount`（`json:"clue_count"` 无 omitempty）/ `:77 ComposeWindowMaxDays=365` / `:337 OmitEmbeddings`；`ObservationRangeControls.vue:60-82` 四个可选 props；`BoardComposeOverlay.vue:469-522` 三处统一走 composable；`LaneCorrectionCanvas.vue:822/830/838` 卡上三按钮），4 项保留 `[ ]` 并逐行注明「未实现：<原因>」（8.2 待 R-4 批准；9.7 / 11.13 真库 SELECT 与 11.10 部署受本线程红线与副作用限制）；头部「本轮只建立计划，全部未执行」改为实现态声明（批次 + 48/52 计数）。**(b) 文档 10.1**：`docs/reference/api/semantic-boards.md` 新增 `### GET /semantic-boards/:id/persistent-topics/compose-candidates` 节——`section.clue_count` 非综合稿口径 + 0 必返回 + 旧响应降级为未知；`omit_embeddings=1` 用途与实测（25,772,225 B / 1.45 s → 553,570 B / 0.55 s，约 46.8 倍）；365 上限（`ComposeWindowMaxDays`）与 7 条错误码/文案；明写「没有单独的 365 可选池端点」。**(c) 文档 10.2**：`flow/semantic-board.md` 新增「泳道工作面（三栏 380/自适应/300、节点卡显式写操作、模态本地时间窗、两曲线口径）」节 + 变更溯源行；`flow/daily-report.md` 新增「编排 / 修正工作面的候选读取（365 窗口 + 轻量档）」节、同步约束 40 的 90→365 数字（旧文字 `>90 截 90` 已与代码不符）+ 变更溯源行。**验证**：`bash scripts/harness/doc-impact.sh verify openspec/changes/unify-lane-workbench` → 通过（声明 flow, api；3 文件）；`check-standards.sh` 304 通过 / 0 失败；`check-tasks-tail.sh` exit 0。


### R-4 2026-11-16【Medium】X-1：major 档写操作无真实落库证据（A-3~A-5 为浏览器内拦截回放）

- **状态**：待处理
- **定位**：`openspec/changes/unify-lane-workbench/ui-verification/report.md`（X-1 未覆盖；A-3/A-4/A-5 声明「写请求浏览器内回放、不触网、DB 零写入」）
- **问题**：期望（§11.1-6 major UI 验收 + ui-design Acceptance-1「至少一条端到端落点」）：加入/移出/加回至少一条真实 PATCH → DB → 再读取证。实际：真实链路（前端参数序列化 → 真实 PATCH 信封 → DB → 写后 `reloadCurve`/清单刷新的真实时序 → 真实 4xx 错误路径渲染）从未整体走过。**判定**：不算「验收证据失真」——报告如实声明了拦截方式、未把 mock 写冒充落库 pass，UI 状态机断言本身真实；且写路径后端零改动、由 DB 级测试兜底（`TestSetSectionAssignment_*` 9 个用例），前端参数由组件测试断言（`toHaveBeenCalledWith(103, 7)`）。但作为 major 档核心交付（节点卡写操作当场生效），这条缝值得在部署前缝上：真实信封/错误码不匹配、写后刷新联动这类 bug 只有真写才能暴露。
- **依据**：report.md X-1 节原文；`BoardComposeOverlay.correction.test.ts:162-195`（mock 层断言调用参数与失败重试）；`backend-go/.../daily_report_assignment_test.go:574` `TestSetSectionAssignment_LeavesHitCountersAndStatusUntouched`（后端行为兜底）。
- **建议**：部署（`deploy-frontend.sh`）前由具写权限线程在主线程批准的 fixture 泳道上重跑 A-3~A-5 各一次真写（含一次故意失败路径），把结果追加进 report.md 的 X-1；不必阻塞本次 review 收口。
- **处置取证**（2026-11-16）：未处置——保持「等主线程批准真写验证」：本线程禁止真写库（红线：不跑 SQL、不碰数据库），故 A-3~A-5 的真实 PATCH → DB → 再读仍缺证据；R-4 不阻塞本次收口，建议部署前由具写权限线程补。


### R-5 2026-11-16【Low】两处陈旧/错误注释与行为矛盾

- **状态**：已修复
- **定位**：`front/app/features/tags/components/console/LaneCorrectionCanvas.vue:476`（节点卡注释仍写「画布只读，卡内无写操作按钮」——本 change 卡上恰恰有加入/移出/加回三按钮）；`backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:341` 附近（「画布路径**不传谈**参数」错字，应为「不传该参数」）
- **问题**：注释与实现相反/错字，误导后续维护。
- **依据**：`sed -n 476p` 与卡片模板 L816-843（三个写按钮在场）；Go diff 轻量取数注释原文。
- **建议**：改为「节点卡（跟随选中节点；卡上显式写操作）」与「画布路径不传该参数」。
- **处置取证**（2026-11-16）：`front/app/features/tags/components/console/LaneCorrectionCanvas.vue:477` 注释改「节点卡（跟随选中节点；卡上带显式写操作按钮）」，与卡上 `lane-card-add/remove/restore` 三按钮一致；`backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:336` 错字「不传谈参数」→「不传该参数」。后端 `golangci-lint run ./internal/topicgraph/repository` → 0 issues。


### R-6 2026-11-16【Low】质心池扩容用 `...sections.value[0]!` 合成更早成员 section，写法脆弱

- **状态**：已修复
- **定位**：`front/app/features/tags/composables/useLaneCorrection.ts:258-268`（`centroidPool.push({ ...sections.value[0]!, id: node.id, …, embedding })`）
- **问题**：依赖「展开窗口第一条 section 再覆盖字段」合成早成员对象——`sections.value` 为空时展开 `undefined`（得 `{}`），当前恰因 `centroidRecommendations` 只读 `embedding/id/persistentTopic` 而不出事；字段读取面一变即成隐性 bug。
- **依据**：`centroidRecommendations`（`laneComposeLogic.ts:297-314`）现只消费三个字段；合成处覆盖了这三个，故现行为正确。
- **建议**：显式构造最小对象（id/embedding/persistentTopicId/persistentTopic: undefined/periodDate/clusterLabel），不借道 `sections.value[0]`。
- **处置取证**（2026-11-16）：`front/app/features/tags/composables/useLaneCorrection.ts:260-268` 改为显式构造最小对象（`id / embedding / persistentTopicId / persistentTopic: undefined / periodDate / clusterLabel`），不再 `...sections.value[0]!`；`useLaneCorrection.test.ts` 29 用例绿（含窗口 0 成员、更早成员撑质心用例），`nuxi typecheck` exit 0（`ComposeCandidate` 必填字段满足）。


### R-7 2026-11-16【Low】自选日期校验双实现：`validateCustomRange` 未复用共享 `validateWorkbenchRange`

- **状态**：已修复
- **定位**：`front/app/features/tags/composables/useLaneCompose.ts:121-125,316-336`（自有 `MAX_WINDOW_SPAN_DAYS=364` + 校验逻辑）vs `laneComposeLogic.ts:628` `validateWorkbenchRange`（useLaneCorrection 已用）
- **问题**：任务 1.4 要求日期限界纯函数收进 laneComposeLogic；两份实现当前数值一致（365），但属可漂移的重复（改上限时容易只改一边——正是 OQ-2 强调「三处一致」要防的事，现在实际是四处）。
- **依据**：`grep -n "MAX_WINDOW_SPAN_DAYS\|validateWorkbenchRange"`。
- **建议**：`validateCustomRange` 内部改为委托 `validateWorkbenchRange(from, to, 365, serverWindow.value?.latestCompletedDate)`，删本地常量与重复分支。
- **处置取证**（2026-11-16）：`front/app/features/tags/composables/useLaneCompose.ts:320-322` `validateCustomRange` 改为一句话委托 `validateWorkbenchRange(from, to, COMPOSE_WINDOW_MAX_DAYS, serverWindow.value?.latestCompletedDate)`；删掉本地 `MAX_WINDOW_SPAN_DAYS=364` 与四条重复分支，常量改为 `COMPOSE_WINDOW_MAX_DAYS = 365`（语义是「含首末的总天数」，与后端同名口径一致）；从 `laneComposeLogic` 补 import。文案与边界不变：实跑 `useLaneCompose.test.ts` 41 用例全绿，含「366 天拒绝 + 文案含 365」「365 天边界可用」「开始早于取数窗」用例。


### R-8 2026-11-16【Low】design.md Context 的 Scenario 计数失真（28/27，实际 30/29）

- **状态**：已修复
- **定位**：`openspec/changes/unify-lane-workbench/design.md` Context 末段「两份 delta 共 28 个 Scenario 实例 / 27 个不同标题」
- **问题**：OQ-5 收口补入 ADDED Requirement（+3 Scenario）后该段未同步；实测 `grep -c "^#### Scenario:"` = 21+9=30 实例、去重 29 标题（仅「查看与选入分离」重名）。机器对账（scenario-trace 30/30）不受影响，纯制品数字失真。
- **建议**：Context 改为 30 实例 / 29 标题（或删掉具体数字只留「重名一条」说明）。

---

## 审毕无发现项

- 审毕无发现（审查范围：7 条不变量 grep 验证——点画布本体不改成员、写路径不碰命中计数、候选排除 active、曲线轻量取数、接近度方向、成员尺寸不编码接近度、本 change 无 DDL；输出见下清单，全部通过）。
- 审毕无发现（审查范围：D1~D8 决策落点可解析性 + OQ-1/2/4/5 收口落实；OQ-3 位置措辞已改但存 R-2 残留；逐条结论见文末表）。
- 审毕无发现（审查范围：旧能力去向复核——左栏推荐七项能力全部迁入右栏或显式退役、成员三分组/sticky/勾选同步无丢失、旧「推荐在左栏」断言已改为反向断言而非删除；去向表无「遗漏」行）。
- 审毕无发现（审查范围：后端 `clue_count` 投影（0 必序列化、composite 排除、查询失败即失败）、365 窗口三处一致性、`omit_embeddings` 轻量参数与实测留证（tasks 2.5：0.55 MB/46.8 倍压缩）；编辑 365 档拉全量向量 25.77 MB < 30 MB 阈值且为画布所需，符合 OQ-2 定案边界）。
- 审毕无发现（审查范围：定点测试实跑——`useLaneCorrection.test.ts`(29) + `LaneRecommendationList.test.ts`(10) + `persistentTopics.test.ts`(14) + `LaneMemberList.test.ts`(16) + `BoardComposeOverlay.correction.test.ts`(15) 共 84 用例全绿，`--maxWorkers=2`）。
- 审毕无发现（审查范围：ui-verification 其余判定——A-3/4/5 浏览器内 mock 写**不构成证据失真**（如实声明、未冒充落库、UI 状态机断言真实，残余风险并入 R-4）；C-5 四路由未覆盖**可接受**（`/search`、`/discovery`、`/settings`、`/research` 不挂载本次改动的 `BoardComposeOverlay`，报告如实标未覆盖并给原因）；环境问题附录（dev 代理回落 SPA、静态旧版）确非本 change 缺陷）。

---

## 7 条不变量 grep 验证结果

1. **点画布本体不改成员**：`grep -n "onNodeClick\|emit('add'\|emit('remove'" LaneCorrectionCanvas.vue` → `onNodeClick`(L548-551) 仅 `emit('focus', node.id)`（带 250ms 拖拽守卫）；`add/remove/restore` 只出现在卡片按钮 `@click.stop`（L823/831/839）。**通过**。
2. **人工变更归属不碰命中计数/状态**：前端写面 仅 `setSectionAssignment`(L634/L669)、`updateTopicAnchor`(L735)、`updateTopic`(L769，仅 `{label}`)；后端 `SetSectionTopicAssignment`（`daily_report_topic_repository.go:1671`）由 `TestSetSectionAssignment_LeavesHitCountersAndStatusUntouched`（assignment_test.go:574）锚定。**通过**。
3. **只从 candidate 改挂，不从 active**：useLaneCorrection.ts L277 `if (section.persistentTopicId && ownerStatus !== 'candidate') continue`（active/archived/disabled 均出候选）+ L238 readonly 判据 + laneComposeLogic.ts:311 `unassigned` 过滤 `status === 'active'`；`TestSetSectionAssignment_ReassignFromActiveLaneRejected` 后端兜底。**通过**。
4. **曲线只用轻量取数**：`grep -rn "days: 365" front/app` → 取曲线仅两处：useLaneCorrection.ts:578 `loadLaneDynamics(boardId, 365)`（lane-dynamics 本身不含向量）、useLaneCompose.ts:469 `getComposeCandidates(…, { days: 365, omitEmbeddings: true })`；无其他 365 全量向量路径。**通过**。
5. **接近度方向一致**：五文件（两画布 + inspector + 左栏 + 右栏）用户可见文案 grep「距离」→ 0 命中（唯一命中是 L374 代码注释）；新建卡片/检视均改 `估算接近度 {{ proximityOf(distance).toFixed(2) }}`；无 `1 - cos` 当接近度用法（`cosineFor` 返回距离，展示统一 `1 − 距离`）。**通过**。
6. **成员尺寸不编码接近度**：`pointAppearance` → `memberBase = flags.memberRadius ?? max(min(cap,10), cap*0.85)`，不读 `proximity`/`distance`；`radius = selected ? memberBase : recommend`；`recommendCap = min(cap*0.72, memberBase - 0.5)` 保证候选恒小于成员。**通过**。
7. **无新迁移**：`git status | grep -iE "migrat|\.sql"` → 命中的 `lane_manual_correction_migration*.go` 与 `postgres_migrations.go` 注册行均属 P1（文件头注释明标 `lane-manual-correction-loop`，四步 20261113_*）；本 change 的 `clue_count` 为运行时投影，无 DDL。**通过（本 change 范围内）**。

---

## 旧能力去向表

> 旧入口核对方式：`LaneMemberList.vue`/`useLaneCorrection.ts` 等均为未跟踪新文件（P1 实现未提交），`git show HEAD:` 不可用；旧能力清单以 P1 归档制品（`archive/2026-11-16-lane-manual-correction-loop/specs/lane-correction-surface/spec.md`）+ 本 change test-cases「旧断言冲突清单」为基准核对。

| 旧能力 | 旧入口（P1 归档 spec / 组件） | 新承接 | 判定 | 证据 |
| --- | --- | --- | --- | --- |
| 推荐列表渲染（日期/标题行） | 左栏「关联推荐」分组 | `LaneRecommendationList.vue` `lrl-list` | 已迁移 | RN 组件 + RN 测试 10 用例全绿 |
| 来源泳道徽标 | 左栏推荐行「候选行 SHALL 标注来源泳道名」 | `lrl-tag`「来自 X」（无来源不显示） | 已迁移 | RN 测试「来源泳道名可见；无来源不显装饰性标签」 |
| 估算接近度标注 | 左栏推荐半边 | `估算接近度 x.xx`（右栏）；快照半边留 LM | 已迁移 | LM 测试断言不含「估算」+ RN 断言「估算接近度」在场 |
| 「＋加入」 | 左栏推荐行按钮 | 右栏行按钮 + 节点卡 `lane-card-add`（同一 `addMember`） | 已迁移 | `BoardComposeOverlay.correction.test.ts:198-207`（同一写路径） |
| 左栏勾选候选成员加入 | P1 动作表「左栏勾选候选成员」 | 右栏按钮/节点卡承接（勾选形态退役） | 主动退役 | delta spec 动作表仍残留该入口文字 → **R-2** |
| pending 态 | 左栏推荐行 pending | `:disabled="isPending(id)"`（共享 pendingIds） | 已迁移 | RN 测试「pending 同步禁用」 |
| 推荐空态 | 左栏推荐空态 | `lrl-empty`「没有新的关联推荐。」 | 已迁移 | RN 测试 |
| 推荐错误态 | 左栏 | `lrl-error`（props.error = loadError，保旧数据） | 已迁移 | RN 测试「错误态：行内错误展示」 |
| 定位/聚焦推荐节点 | 左栏点行 | `lrl-peek`「定位」→ emit focus（只改选中态） | 已迁移 | RN 测试「点定位只抛 focus」 |
| 成员/更早/已移出三分组（滚动、按月分堆、加回、勾选） | LaneMemberList（P1） | 原样保留（收窄为三组） | 已保留（无丢失） | LM 测试 16 用例 |
| 首义 sticky 提交按钮 | LaneAnchorScope（P1） | 仍挂左栏 aside 底部（Overlay L483-497） | 已保留 | Overlay 模板 + correction 测试「左栏承载四块」 |
| 勾选态双向同步（左栏↔画布↔卡片） | P1 | remove/restore/toggleAnchor 共享 composable 单一状态 | 已保留 | correction 测试「右栏推荐行与节点卡同一语义」 |

**「遗漏」行数：0。**

---

## 8 个决策落点逐条核对

| 决策 | 落点核对 | 结论 |
| --- | --- | --- |
| D1 编辑曲线独立取数 | `laneDynamicsWindow.ts:useLaneDynamicsWindowApi` ✓；`useLaneCorrection.ts:useLaneCorrection` ✓；tasks 6.3 ✓ | 可解析 |
| D2 clue_count | `ComposeCandidateSection.ClueCount`（`clue_count` 无 omitempty）✓；批量复用 `listLaneSectionThreads().Clues` ✓；`normalizeCandidate` 归一（0 保留/null/非法→null）✓ | 可解析 |
| D3 窗口上限 | `composeWindowMaxDays = repository.ComposeWindowMaxDays(365)` ✓；报错文案引用常量 ✓；`validateCustomRange` 上限 365 ✓（双实现见 R-7） | 可解析 |
| D4 时间窗归模态本地 | `switchWindow`（有草稿确认/失败保旧）✓；Overlay 双 `ObservationRangeControls` 本地接线、零 router/query import ✓ | 可解析 |
| D5 推荐迁右栏 | `LaneRecommendationList.vue` 新建 ✓；`LaneMemberList.vue` 收窄三组 ✓；三栏 380/1fr/300 ✓；旧断言改反向 ✓ | 可解析 |
| D6 relatedNodeIds 唯一入口 | `relatedNodeIds(relations, seeds, recommendedIds?)` 扩展 ✓；两画布均经此入口 ✓。注：更早成员入质心的数据通道断在 R-1 | 可解析（实现有 R-1 缺陷） |
| D7 pointAppearance 单一入口 | `pointAppearance(…, flags)` + `LanePointFlags` ✓；`proximityOf`/`scaledPointRadius`/`cosineFor` ✓；两画布 + Inspector 全走它 | 可解析 |
| D8 卡片只派发既有动作 | `addMember/removeMember/restoreMember` ✓；卡上 `@click.stop` ✓；active/归属未知 `cardWriteKind = null` ✓；pending 禁用 ✓ | 可解析 |

**OQ 收口表 5 项**：

- **OQ-1 ✓**：`withDefaults` 缺省 `tiers: [...SOURCE_TIER_DAYS]`、`selectableBounds: null`、`maxSpanDays: null`、`caption: null`；矩阵宿主 `BoardConsoleStage.vue:114-124` 未传任何新 props；测试覆盖「不传 maxSpanDays 手势全程可用」。
- **OQ-2 ✓**：三处一致——`repository.ComposeWindowMaxDays = 365`、handler 报错文案引用常量、前端 `MAX_WINDOW_SPAN_DAYS=364 → "不能超过 365 天"`；E6 边界双侧测试。
- **OQ-3 基本 ✓（残留见 R-2）**：「左栏/同栏关联推荐」措辞已全部改右栏；「取消」已拆为「新建弃草稿 / 编辑返回不撤销已落库」。
- **OQ-4 ✓**：新建分支栏位（候选/画布/检视 + `起名并创建`）与 grid `300px minmax(0,1fr) 320px` 未动。
- **OQ-5 ✓**：`lane-correction-surface` delta ADDED Requirement「新增泳道的时间视窗曲线口径」+ 2 Scenario 存在；scenario-trace 30/30 映射齐。

---

## 核实记录（环境与口径备注）

- **diff 归属混杂**：工作树未提交改动为 P1（lane-manual-correction-loop）+ P2（fix-lane-metric-provenance）+ 本 change 三者叠加（`LaneMemberList.vue`、`useLaneCorrection.ts` 等 P1 产物均为未跟踪新文件，`git show HEAD:` 不可用）；本审查按 brief 给定文件清单 + P1/P2 归档制品区分归属。
- **proposal.md 的 P1/P2 前置描述与「后端仅当…才改」条件为过时文字**：design.md Context 已显式声明为历史残留并以 OQ-2 定案推翻，不另开条目。
- **验证命令实测**：`openspec validate unify-lane-workbench --type change --strict --no-interactive` → exit 0；`scenario-trace.sh` → 30/30 exit 0；`check-tasks-tail.sh` → exit 0；定点 vitest 5 文件 84 用例全绿。
- **编辑曲线断档日显示为未知（null）而非已知 0**：lane-dynamics 的 timeline 仅含有 section 的日期，实现把缺失日映射 null（保守不补 0），与 State Matrix 一致；与新增池曲线「无条目日 = 已知 0」口径不同但各有依据，不判问题。

## 处置状态汇总

| 编号 | 等级 | 状态 | 处置 | 取证（2026-11-16） |
| --- | --- | --- | --- | --- |
| R-1 | High | 已修复 | 归一层透传 `early_members[].embedding` + 补两用例 | `persistentTopics.ts:209/302`；`persistentTopics.test.ts` +2 用例；45 用例绿 |
| R-2 | Medium | 已修复 | delta spec 删两处「左栏加入入口」 | `specs/lane-correction-surface/spec.md:16/23`；`openspec validate` valid |
| R-3 | Medium | 已修复 | tasks 48/52 勾选（4 项注明未实现）+ 文档 10.1/10.2 | tasks 头部改实现态；api + 两个 flow 文件增补；`doc-impact verify` 通过、`check-standards` 304/0 |
| R-4 | Medium | 待处理 | **等主线程批准真写验证**（本线程禁真写库，不阻塞收口） | 未取证 |
| R-5 | Low | 已修复 | 两处陈旧/错字注释 | `LaneCorrectionCanvas.vue:477`、`daily_report_manual_topic.go:336` |
| R-6 | Low | 已修复 | 质心池 section 显式构造 | `useLaneCorrection.ts:260-268` |
| R-7 | Low | 已修复 | 日期校验委托共享函数 | `useLaneCompose.ts:320-322`；41 用例绿（366 拒 / 365 过） |
| R-8 | Low | 已修复 | Scenario 计数 30/29 | `design.md:21` |
- **处置取证**（2026-11-16）：`design.md:21` 改为「共 **30 个 Scenario 实例 / 29 个不同标题**（仅「查看与选入分离」重名）」，与 `grep -c "^#### Scenario:" specs/*/spec.md`（21+9）及 `sort -u`（29）一致；`scenario-trace.sh` 报「30 个 Scenario 映射齐全」。

