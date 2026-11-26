
## 约束判据实测：绝对阈值不成立、相对偏好可分离

【方向 A「话题辅助标签约束」的判据实测结论 —— 决定实现形态的关键】

① **「同一个 tag 同时满足两个约束锚」不可行**：锚「以色列」↔「黎巴嫩」余弦距离仅 0.14(bare)/0.28(ev)/0.36(kw)；远程空间用 tag 锚（「以色列总理」identity vs「真主党」identity）在 event_keyword 空间检索，τ=0.50 时 6521~7078/7400（88%）event 标签「同时满足」两锚，命中全是纯以色列组阁新闻。机理：向量的「近」=「语义同类」≠「语义包含」——真正的双主体事件句「以色列国防部长卡茨警告黎巴嫩真主党」到「以色列」锚 d=0.538，比纯单侧主体词「以色列总理」（0.236）远一倍。

② **单一绝对距离阈值不可行**：本地空间 section 级模拟（107 个 tag 向量，V1_prod 文本 × kw 锚）τ=0.45 时正样本放行 42~58%、纯以色列误判 6~27%、纯黎巴嫩误判 55% —— ROC 不分离。

③ **【可行】相对偏好（margin）判据**：约束 i 被覆盖 ⟺ ∃tag t 使 d(t,A_i)+margin_i < d(t,A_j)（∀其它约束 j）；再叠加绝对上限 max_distance。实测 margin=0.05、锚编码 `term + " " + "event"` 时：正样本放行 **64.4%**、纯以色列误判 **0%**、纯黎巴嫩误判 **0%** —— 分离。分离度排序：V1_prod×event 64.4% > V1_prod×kw 53.6% > V2_bare×kw 46.8% > V1_prod×裸词 26.2%。

④ **margin 取值**：0.05 正样本 64.4%；0.10 掉到 25.2%；0.15 起基本清零。max_distance 默认 0.55（纯以色列 tag 到「黎巴嫩」锚 p50=0.593）。

⑤ **模型空间**：远程↔本地同文本余弦仅 0.74，且**换本地 Q4_K_M 后仍为 0.749**（而本地 Q6↔Q4 = 0.984）→ 差距**不是量化造成**，是 pooling/EOS/前缀或模型 size 差异；换量化档不改变结论。锚须与 tag 同空间 → 记录 embedding_model + 跨空间 tag 跳过（不计入覆盖也不判违反）。

⑥ **7 天 GC 是硬约束**：`internal/tagmanagement/service/core/edge_gc.go` DefaultTagEdgeRetentionDays=7；topic_tags 存活 11月 15398 个 vs 6~10 月合计 2392；有标签文章 11月 6696 篇 vs 10月 390 篇；「真主党」tag created_at=2026-11-12（首次产出）。→ 任何基于 tag id 的约束会自行失效；且 section.cluster_tag_ids 92.8% 悬空（14679/15820），判定输入必须走 article_topic_tags（悬空 0%）。

⑦ **降级污染实测为 0**：11-13 后无新日报（最新 period_date=2026-11-12），section/质心全在远程空间；仅 5 条本地 topic_tag_embeddings + 565 条本地 ai_embedding_cache（后者有 model 列）。远程恢复后无需重算。

【已开的 change】`openspec/changes/topic-lane-auxiliary-label-constraint`（complex / ui-impact minor，5/5 制品完成，validate+doc-impact verify+check-tasks-tail 全绿）。数据模型 `board_persistent_topic_constraints`（term / term_embedding 无维度限定 vector / embedding_model / mode / margin / max_distance）；闸门落 lane group 级（assembleLaneGroups 之后）；违例补一次 L2 裁决、仍违例则沿用「switch 集外 → 降级 new」；前端配置区阻塞于 `lane-manual-correction-loop`（0/60 任务）。

**引用**：backend-go/internal/topicgraph/service/daily_report_lane.go:assembleLaneGroups、backend-go/internal/topicgraph/service/daily_report_lane.go:parseL2Response、backend-go/internal/tagmanagement/service/core/embedding.go:buildTagEmbeddingText、backend-go/internal/tagmanagement/service/core/edge_gc.go、backend-go/internal/platform/airouter/router.go:Embed

<!-- pinned 2026-11-13T10:21:09Z -->

## 泳道双工作面 UI 现状 + 两条必撞规范

【泳道工作面 UI 现状（2026-11-24 摸底，此前多方假设已过期）】

## 一、change 谱系（别再引用旧前提）
`lane-manual-correction-loop` 已于 **2026-11-16 归档**，之后叠 6 个 UI 重构：
- 2026-11-17 `unify-lane-workbench`：两栏 → **三栏**（用户拍板）
- 2026-11-19 `replace-lane-lifeline-with-topic-events`
- 2026-11-21 `rework-lane-compose-swimlane-canvas`：泳道行画布 + 拖拽选入 + 散料折叠三档 + 检索态
- 2026-11-23 `lane-compose-topic-event-tags`：★ 只读投影 + 隐藏「显示已有泳道(只读)」「历史待启用话题」两入口
- 2026-11-23 `clarify-lane-correction-effect-timing`：生效时机徽标从页脚上移到标题行常驻
- 另有 2026-11-23 `fix-lane-row-gap-zoom` / `fix-lane-label-horizontal-fit`

## 二、结构（关键：不是 AppDialog）
`front/app/features/tags/components/console/BoardComposeOverlay.vue`（979 行）承载「新建泳道」+「编辑泳道(修正)」**两种模式**，根容器是 `Teleport > div.bco`（`position:fixed; inset:0; z-index:1000`，`:647-657`）——**全屏模态，不是 AppDialog，无 size 档位、无 layout mode**。
- 模板起点 `:309`；标题行 `:328-355`（新建=静态「新建泳道」；修正=泳道名就地编辑 `:330-353`）
- **修正模式 `:451-521`**：左 `aside.bco__left` `:452-481`（`LaneMemberList` `:457-467` + `LaneAnchorScope` `:468-480`）／中 `LaneCorrectionCanvas` `:483-511`／右 `aside.bco__right-correction` `:512-521`（`LaneRecommendationList` `:513-520`）
- 工具条：`.bco__range`（时间视窗，已占满一条）/ `.bco__tools`（取消聚焦 + 阈值三档 segmented，`:430-441`）
- 左栏 **≤1280px 塌到 240px**，新块必须纵向堆叠
- 新建模式泳道名只在最后命名弹窗输入（`AppDialog size="md"`，`:614-641`）

## 三、可复用先例：`LaneAnchorScope.vue`（唯一「话题级持久设置」块）
`console/LaneAnchorScope.vue`：独立 `<section class="las-panel">`（`:100`）+ 块头「标题 + 计数徽标 + **结论型生效徽标『次日日报生效』**」（`:101-105`）+ sticky 提交区（`:168`, `:352-353`）。
组件习惯：`AppInput`（`~/components/ui/AppInput.vue`，props modelValue/type/placeholder/disabled/error/step/min/max/maxlength/inputId）+ `AppButton`（variant/size/disabled/loading/type）+ 原生 `<button aria-pressed>`（segmented）。

## 四、两条规范会撞人
1. **「产品界面不承载解说性长句」**（`lane-correction-surface` spec + `interaction.md`）：禁「操作口径并列解释」「派生值机制科普」「图例长句」「界面内教学语」。→ 不能写「约束只影响吸附、不影响生命周期」这类句子。只留：动作标签 / 状态结论（徽标、空态、错误、计数）/ 必要确认与反馈。
2. **`ui-design.md` minor 档起必填两表**（`interaction.md`）：`## Interaction Decisions`（ID/决策/模式/forbidden/状态，模式取自 page|panel|drawer|dialog|inline-edit|canvas|command；pinned 不可被实现替换）+ `## Data Provenance`（每项用户可见数据一行：`api <path>` / `表.字段` / `派生 <公式>` / `none`）。
判据 P1-P5：P1 空间关系→canvas；P2 单字段→inline-edit；P3 破坏性→dialog；P4 保留列表上下文多选→panel/inline-edit；P5 并列比较→禁 dialog。

## 五、ui-impact 档位
major 判据 = 新页面/面板/弹窗/导航/多步/信息架构/布局模式。在既有 panel 族追加**同形态可折叠块**（复用 LaneAnchorScope 骨架、默认折叠、不新增入口/layout mode/弹窗档）→ 判 minor 成立（已在 ui-design 留判定依据表）。

## 六、前端 API 归属
`front/app/api/dailyReports.ts` 是 `/daily-reports/topics/:id/*` 的归属地（已有 anchor / key-events / detach-date / merge / split / lifeline 共 10+ 方法）。`persistentTopics.ts` 只负责编排态（compose-candidates / persistent-topics/manual）。新话题级端点加到 `dailyReports.ts`。

## 七、测试规范已变（2026-11 现行）
`docs/reference/standard/shared/test-design.md`：永久业务测试只收「API 主线」+「有来源的最小 Bug 回归」；**不要求** test-cases.md、分支表、边界值清单、逐 Scenario 用例表、变体走查（原文「不提交白盒账本或逐 Scenario 表」）。`test-assets.sh` 已把 test-cases.md 称作「旧」制品，无任何门禁强制它。
**但** `scripts/harness/scenario-trace.sh` 仍是归档门禁：要求 tasks.md「验证」节有 `| Scenario | 测试文件 |` 表，**每个 delta Scenario（含 MODIFIED 块内既有）都要映射**，且映射文件必须真实存在。

**引用**：front/app/features/tags/components/console/BoardComposeOverlay.vue:451-521、front/app/features/tags/components/console/LaneAnchorScope.vue:100-105、openspec/specs/workshop-lane-compose/spec.md、openspec/specs/lane-correction-surface/spec.md、docs/reference/standard/frontend/interaction.md:37-41

<!-- pinned 2026-11-24T05:43:29Z -->

## 辅助标签约束后端实现完成（含闸门与验证实测）

【后端实现已完成（tasks 1~5 组，24/45 项）—— 实现事实，供评审/归档/后续前端接线查阅】

## 文件清单
新增 6：
- `internal/platform/database/topic_lane_constraint_migration.go`（155 行）迁移 `20261124_0001`：`CREATE TABLE IF NOT EXISTS board_persistent_topic_constraints`（id bigserial / topic_id bigint FK CASCADE / term varchar(100) / **term_embedding vector NOT NULL 无维度限定** / embedding_model varchar(100) / mode varchar(16) default 'required' / **margin numeric NULL** / **max_distance numeric NULL** / created_at）+ CHECK（mode 三态、margin/max_distance 非空时 [0,1]）+ UNIQUE(topic_id,term) + INDEX(topic_id)，并播种 `persistent_topic_constraint_margin_default=0.05` / `..._max_distance_default=0.55`
- `internal/topicgraph/repository/daily_report_constraint_repository.go`（388 行）：列出/批量列出/全量替换（单事务幂等、非法项整体回滚）/删除；`ListLaneConstraintLabelVectors`（经 `article_topic_tags` 取活 tag 向量+模型）；`SampleLiveTagEmbeddingDimension`；`ResolveSectionValidTagIDs`；`GetTopic`
- `internal/topicgraph/service/daily_report_constraint.go`（486 行）：锚文本 `term + " event"`、`ComputeLaneConstraintAnchor`（走 airouter 当前顺位、返回模型名）、**纯函数 `EvaluateLaneConstraintCoverage`**、`ReplaceTopicLaneConstraints`、`LoadLaneConstraintOption`
- `internal/topicgraph/handler/daily_report_constraint_handler.go`（124 行）：GET/PUT 两条
- 测试：`repository/daily_report_topic_repository_test.go`（484 行）、`service/daily_report_constraint_test.go`（509 行）、`service/daily_report_constraint_preferred_test.go`

修改 9：`postgres_migrations.go`（注册 1 行）、`daily_report_models.go`（`TopicLaneConstraint`，**不注册 AutoMigrate**）、`daily_report_topic_repository.go`（config 两个默认值 key）、`daily_report_lane.go`（`ClusterTagsLane` 加可选 `opts ...LaneConstraintOption`、两条出口都接 `ApplyLaneConstraintGate`）、`daily_report_orchestrator.go`（1 行传入 option）、`daily_report_handler.go`（路由）、及两个测试文件。

## 闸门语义（`daily_report_lane.go:ApplyLaneConstraintGate`）
不在 `planTopicAssignments` 之前（那在保存事务里，LLM 不能进事务），而在 `ClusterTagsLane` 内紧跟 `assembleLaneGroups`（:394 与 :444 两条出口）。
- 无违例 → 原样返回（`len(opt.ConstraintsByTopic)==0` 直接 return，存量话题零开销零 LLM）
- 有违例 → **所有违例 group 合并成一轮** L2 调用（只 1 次），提示词加约束状态块（`buildLaneConstraintStatusBlock`）
- 复裁仍选中未覆盖话题 → `target=0` → `downgradeLaneGroupToNew`（`Lane="l3"`, `MatchedTopicID=nil`, `GroupName` 取首 tag label）
- LLM 调用失败 → 违例 group 全部降 l3 + warn（**不炸整份日报**，但 SHALL NOT 锚到违例话题）
- 未违例但复裁选中的话题最终 `Lane="l2"`（符合 spec：违例的 L1 直挂降级进 L2 band）

## 关键实现细节（易踩）
- 判定输入**只走 `article_topic_tags`**，全程不读 `cluster_tag_ids`（该列 92.8% 悬空）
- `margin`/`max_distance` 可空 → 空取 `ai_settings` 全局默认；改全局默认对存量约束立即生效
- PUT 中**短词未变则复用已落库锚向量**（幂等、不需要 embedding 可用、保住向量空间一致性）；重算锚 = 在 PUT 里省略该条再重新提交
- `preferred`：不拦截、不计入 required、**不作 required 相对偏好的参照锚**；只作为偏好提示进 L2 提示词（`laneConstraintPreferredTerms`，渲染为「偏好:…（仅提示不拦截）」）

## 验证实测（2026-11-24）
- `go build ./...` PASS / `go vet ./...` PASS / `golangci-lint run ./...` **0 issues**
- `go test ./internal/topicgraph/service` PASS 361s；`./repository` PASS 89s；`./handler` PASS 50s
- 闸门 7 条（`TestConstraintGate_*`）全 PASS，含「无约束时输出逐字一致 + 零 LLM 调用」「违例只补 1 次调用」「LLM 失败不锚违例话题」「畸形响应不绕过」
- 判定 18 条（`TestConstraint_*`）全 PASS；仓储 13 条（`TestTopicLaneConstraint*`）全 PASS，含 schema 物理形状核对
- `scenario-trace.sh` 46 个 Scenario 映射齐全；`openspec validate` valid（仅 1 条既有 requirement 超长 warning）
- **注意**：load average > 9 时 testcontainers 会超时抖动（handler 包曾 FAIL，重跑即绿）——树莓派上跑测试前先确认负载

## 未做/边界
- 前端配置块（第 6 组，用户决策延后至界面稳定）——落点已定 `BoardComposeOverlay.vue:481`
- 真库效果核对 + margin 标定（第 7.1，需真实数据 + 写副作用）
- 历史错挂 section 不回改（proposal 非目标）
- 无专用「重算锚」端点（靠省略+重提交等价实现）

**引用**：backend-go/internal/platform/database/topic_lane_constraint_migration.go、backend-go/internal/topicgraph/service/daily_report_constraint.go:EvaluateLaneConstraintCoverage、backend-go/internal/topicgraph/service/daily_report_lane.go:ApplyLaneConstraintGate、backend-go/internal/topicgraph/service/daily_report_lane.go:buildLaneConstraintStatusBlock、backend-go/internal/topicgraph/repository/daily_report_constraint_repository.go、backend-go/internal/topicgraph/handler/daily_report_constraint_handler.go

<!-- pinned 2026-11-24T07:37:15Z -->

## 新建泳道同事务建约束 + 草稿模式实现

【新建泳道可同事务建约束 + 前端草稿模式（2026-11-25 实现完成，design D12）】

## 后端（三层）
1. **仓储** `daily_report_manual_topic.go`：
   - `CreateManualTopicWithConstraints(boardID, label, sectionIDs, constraints []ManualTopicConstraint)`；`CreateManualTopic` 保留为三参包装（传 nil），存量调用零变化。
   - `ManualTopicConstraint{Term, Mode, Anchor(pgvector literal), Model, Margin, MaxDistance}`。
   - 约束行在 `tx` 内、`anchor override` 之后写入；插入失败 → 整个建话题回滚（不留「有话题、无约束」）。
2. **服务** `daily_report_constraint.go`：`PrepareManualLaneConstraints(ctx, reqs)` —— 复用 `laneConstraintInputsFromRequests`（与 PUT 同一校验真相源）；**锚向量在开事务前算**（embedding 是网络调用，不能进事务）；按 `SampleLiveTagEmbeddingDimension` 校维度，与 PUT 同口径。
3. **handler** `createManualTopic`：请求体加可选 `constraints []service.LaneConstraintRequest`；校验失败 → 400；锚算失败 → 500；都用 `LaneConstraintValidationError` / `ErrLaneConstraintAnchor` 分流。

## 前端
- `LaneConstraintScope.vue` 加 `draft` prop + `update:draftConstraints` emit。草稿模式：`enableDraft()` 把 phase 直接置 ready、`commit()` 只改本地行集不发请求。
- `useLaneConstraint.ts` 加 `draft` ref + `enableDraft()`；`load`/`commit` 在 draft 时短路。
- `BoardComposeOverlay.vue` 新建分支左栏改为 `div.bco__left-compose`（grid 两行：`minmax(0,1fr)` 候选链 + `auto` 约束块），底部挂 `<LaneConstraintScope draft>`；`draftConstraints` ref 经 `useLaneCompose` 的 `draftConstraints()` 回调在保存时读取。
- `useLaneCompose.save()`：**无约束时保持 3 参数调用形态**（与引入前逐字一致，既有测试不用改）；有约束才带第 4 个参数。
- `persistentTopics.createManualLane(boardId, label, sectionIds, constraints=[])`：空数组不发 `constraints` 字段。

## 验证实测
- `go build ./...` / `go vet ./...` / `golangci-lint run ./...` → 全绿 0 issues
- 新增后端测试（`daily_report_topic_integration_test.go`）：`TestManualTopic_CreateWithConstraints` PASS、`TestManualTopic_ConstraintFailureRollsBackTopic` PASS（非法 mode → CHECK 违例 → 话题不创建）
- 前端 `LaneConstraintScope.test.ts` 10/10（含 2 条草稿模式：零 GET/零 PUT、行集上报、删行后续上报）
- 回归：`BoardComposeOverlay.test.ts` + `.correction.test.ts` 共 79/79 全绿
- `nuxi typecheck` exit 0；`pnpm lint` 0 error
- 真库端到端：`POST .../persistent-topics/manual` 带 `mode:"optional"` → 400 `lane constraint "测试": invalid mode "optional"`，话题未创建、约束表仍 0 行
- spec 新增 requirement「手动建泳道可同时创建约束」+ 3 Scenario；`scenario-trace` 49 个 Scenario 全映射

## 关键约束（别踩）
- 锚必须事务外算；约束必须事务内写。
- 空约束时前端必须走 3 参数调用（不传 `constraints` 字段），否则既有「无约束行为逐字一致」被打破。
- 命名弹窗（`AppDialog size="md"`）只承载单字段，约束块不能塞进去；右栏「只做三件事」，也不能放。

**引用**：backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:CreateManualTopicWithConstraints、backend-go/internal/topicgraph/service/daily_report_constraint.go:PrepareManualLaneConstraints、backend-go/internal/topicgraph/handler/daily_report_handler.go:createManualTopic、front/app/features/tags/components/console/LaneConstraintScope.vue、front/app/features/tags/composables/useLaneConstraint.ts

<!-- pinned 2026-11-25T07:23:57Z -->
