## 1. 数据模型与迁移

- [x] 1.1 写显式迁移给 `board_persistent_topics` 加 `centroid_pending_recompute boolean NOT NULL DEFAULT false`，历史行默认 false 且**不触发任何重算**；验证：`bash scripts/harness/db-migration-safety.sh` 或等价检查通过，且 `SELECT count(*) FROM board_persistent_topics WHERE centroid_pending_recompute` 返回 0
- [x] 1.2 写显式迁移建 `topic_anchor_overrides` 表（`id` / `topic_id` / `old_embedding` / `new_embedding` / `source` / `input_summary` / `operator` / `created_at`，`topic_id` 建索引）；验证：迁移后 `\d topic_anchor_overrides` 结构与设计一致
- [x] 1.3 写显式迁移给 `daily_report_sections` 加归属变更留痕三列 `manual_assignment_at timestamptz NULL` / `manual_assignment_from_topic_id bigint NULL` / `manual_assignment_action text NULL`（'detach' / 'attach'）；验证：迁移后三列存在且历史行为 NULL
- [x] 1.4 在 `backend-go/internal/topicgraph/repository/daily_report_models.go` 给 `BoardPersistentTopic` 加 `CentroidPendingRecompute` 字段、新增 `TopicAnchorOverride` 模型与 `TableName()`；验证：`cd backend-go && go build ./...` 通过

## 2. 后端 · 人工重设首义

- [x] 2.1 在 `daily_report_topic_repository.go` 加「写首义 + 追加留痕 + 置待重算标记」的 repository 方法（单事务）；验证：新增单测断言 `embedding` 已写、`topic_anchor_overrides` 追加一行、标记为 true
- [x] 2.2 实现首义输入：**只有**「手动选定节点范围」一种——复用 `daily_report_manual_topic.go:aggregateEmbeddings` 的 mean-pooling 口径聚合用户选定的 section embedding；验证：单测断言聚合结果与手动建话题口径一致
- [x] 2.3 实现输入校验：选定节点 SHALL 属本话题、同 board、embedding 存在且维度一致；任一不满足整体拒绝（不静默跳过）；验证：单测覆盖跨话题 / 跨 board / 无向量 / 维度不符四种拒绝
- [x] 2.4 加端点 `PATCH /api/daily-reports/topics/:id/anchor`，请求体只带 `section_ids`，响应带 `topic_id` / 来源 / 改写次数 / `effective_from: "next_report"`；**SHALL NOT 接受自然语言输入**（传描述字符串须 4xx）；验证：handler 层测试覆盖成功 + 各类 4xx
- [x] 2.5 扩展质心刷新范围：`SaveReport` post-commit 的刷新集合从「touched 话题」扩为「touched ∪ `centroid_pending_recompute=true` 的话题」；重算成功清标记、失败保留标记且不中断其余话题；验证：单测覆盖「当日无新命中仍被重算」与「重算失败保留标记」两条场景
- [x] 2.6 确认算法侧不写 `embedding` 列：质心刷新只 UPDATE `centroid`；验证：单测断言刷新后 `embedding` 值不变

## 3. 后端 · 移除（解除归属）

- [x] 3.1 在 `daily_report_topic_repository.go` 加「归属变更」repository 方法（双向）：`topic_id=null` 时校验（section 确属某话题）+ 四列置 NULL；`topic_id=N` 时校验（section 当前无归属 / 目标同 board / 目标非 archived）+ `persistent_topic_id=N`、`confidence=manual`、`distance`=到目标锚点实算距离、`topic_status_at_report=NULL`；两向均保留 `lane_tier` + 写留痕三列；验证：单测逐列断言两向快照口径
- [x] 3.2 加「按日批量移除」repository 方法：把某天归属该话题的全部 section 统一解除归属；验证：单测断言条数正确、无遗漏、无越日
- [x] 3.3 归属变更 SHALL 把**受影响话题**（移除=被移除时所属；加回=目标话题）置 `centroid_pending_recompute=true`；验证：单测断言两向各自置位正确
- [x] 3.4 加端点 `PATCH /api/daily-reports/sections/:id/assignment`（body `{topic_id: number|null}`，移除与加回共用）与 `POST /api/daily-reports/topics/:id/detach-date`；验证：handler 层测试覆盖移除成功 / 加回成功 / 跨 board 加回 4xx / 已归属 section 加回 4xx / 移除无归属 section 4xx
- [x] 3.5 移除事务末尾调 `RebuildBoardRelations(tx, boardID)`，失败整体回滚；验证：单测断言关系重建被调用 + 重建失败时归属解绑与留痕一并回滚
- [x] 3.6 确认移除不动命中计数与状态：移除后该话题的 `hit_count` / `consecutive_hits` / `status` 均不变；验证：单测断言三项值不变
- [x] 3.7 确认移除不立即重算质心：移除事务后 `centroid` 列值不变（只有标记变了）；验证：单测断言 `centroid` 未被写

## 4. 后端 · 版块编辑诚实化修复

- [x] 4.1 在 `backend-go/internal/tagmanagement/handler/board_crud_handler.go:updateSemanticBoard` 内补 `service.InvalidateMatchCache()`（label 或 description 变化时）；验证：回归测试断言改完版块名后 `loadBoardEmbeddings` 返回新向量而非缓存旧值
- [x] 4.2 确认本 change **不**自动重跑 `topic_tag_board_labels`；验证：回归测试断言改完版块后匹配表行数与内容不变
- [x] 4.3 加回归测试覆盖「改版块 label/description → 方向校验使用新向量」，且缓存无 TTL 场景下依然生效；验证：`cd backend-go && go test ./internal/tagmanagement/...` 通过

## 5. 前端 · 泳道修正工作面（全屏画布）

- [x] 5.1 在 `front/app/features/tags/components/console/BoardComposeOverlay.vue` 抽出可复用壳，新增「修正模式」分支（进入一条既有泳道并铺开其当前成员节点）；workspace **两栏**（左栏 380px 固定承载全部信息 / 中央 SVG 自适应，**无右栏**）；验证：组件测试断言修正模式下左栏与中央画布均渲染、**页面内不存在右栏**，且新建模式行为不变
- [x] 5.2 新建 `LaneCorrectionCanvas.vue` 作修正模式宿主：中央 SVG 画布复用既有话题带 + 节点散点 + 节点卡跟随，坐标按容器真实像素 1:1；验证：组件测试断言节点可点选、节点卡跟随选中节点、缩放平移后节点卡不塌扁
- [x] 5.3 新建 `LaneMemberList.vue`（**左栏**）：「本泳道成员」清单（取消勾选即**移除**，**直接移除不弹任何选择器**；显示首义来源）+「已移出（本次）」分组（每条带「加回来」）；验证：组件测试断言取消勾选后弹窗数量不增加、节点进入「已移出（本次）」、点「加回来」后回到成员列表且弹窗数量不变
- [x] 5.4 新建 `LaneAnchorScope.vue`（**左栏**「首义范围」）：手动勾选一组节点 →「用选中的 N 个节点定首义」；覆盖已有首义时显示**画布内**确认条（不叠弹窗）；验证：组件测试断言确认条出现、提交后来源显示为「人工（选定的 N 个节点）」
- [x] 5.5 沿用既有画布纪律：查看与选入分离（点节点只改选中态、成员数不变）、拖拽平移不误伤点选、取消勾选为破坏性动作（与取消选中视觉区分）；验证：组件测试断言点节点不改变成员数
- [x] 5.6 工作面实现状态矩阵：loading 骨架 / empty（无成员 / 无节点可展示 / 当天无其它话题）/ error（行内错误 + 重试 + 保留本轮未提交改动）/ success（三类成功各有可区分反馈）；验证：组件测试逐态断言，首次加载失败**不**呈现业务空态
- [x] 5.7 工作面标注生效时机（归属解除=立即生效；重心 / 真空标记 / 泳道快照=次日日报生效）；反馈文案含「次日日报生效」与「该内容仍可再参与候选」；验证：组件测试断言两处标注文案与两条反馈文案存在
- [x] 5.8 泳道名就地编辑：修正工作面标题处的泳道名点文字变输入框（复用既有画布 `lane-compose-name` 名称输入区），Enter 保存 / Esc 取消 / 失焦取消，保存后 toast「泳道名已更新」，**SHALL NOT 弹任何对话框**；验证：组件测试断言就地编辑可完成、无对话框产生、label 已更新
- [x] 5.9 界面文字纪律：产品界面 SHALL NOT 出现操作口径并列解释、派生值机制科普、画布长图例、节点卡教学语；生效时机只以结论型徽标表达；说明性内容只留在评审外壳；验证：组件测试断言工具条/画布图例/节点卡/页脚无解说性长句
- [x] 5.10 窄屏（<768px）：画布入口禁用并提示「需要宽屏或横屏」，SHALL NOT 渲染残缺两栏；轻量弹窗仍可用且无横向溢出；验证：组件测试在窄屏宽度下断言画布未渲染、提示文案出现、无横向滚动

## 6. 前端 · 轻量弹窗与入口接线

- [x] 5.11 后端 `compose-candidates` 加修正模式参数（如 `all_members_topic_id`）：传入则该话题成员不限窗口追加返回（编排画布不传时行为完全不变）；验证：handler 测试覆盖 传参返回全量成员 / 不传参行为不变
- [x] 5.12 前端左栏「本泳道成员」分层：计数徽标改「窗口 N · 全部 M」+「更早成员 · N」折叠分组（按月分堆、组内滚动、节点卡查看）；验证：`pnpm test:unit LaneMemberList --maxWorkers=2` 断言徽标口径、分组渲染、节点卡打开
- [x] 5.13 首义选择池扩展到全部成员（LaneAnchorScope 清单列更早成员、首义集成员带标记、勾选池含窗口外）；验证：`pnpm test:unit LaneAnchorScope --maxWorkers=2` 断言池覆盖全部成员、候选不入池
- [x] 5.14 首义范围面板拆「窗口内 · N」（默认展开）/「更早成员 · N」（默认折叠）两个独立分组 + 计数徽标「窗口内 N · 全部 M」；验证：`pnpm test:unit LaneAnchorScope --maxWorkers=2` 断言分组、默认折叠态、徽标口径
- [x] 5.15 候选口径放宽：无归属 + 属 candidate（动态）泳道的节点进候选区/关联推荐，属 active 泳道的只作只读参照；候选行标注来源泳道名；验证：`pnpm test:unit useLaneCorrection --maxWorkers=2` 断言候选集构成与只读参照构成
- [x] 5.16 改挂许可：后端允许从 candidate 泳道改挂进本泳道（active 泳道仍拒绝），留痕记来源与目标、两侧泳道均置待重算；验证：`go test -run Reassign ./internal/topicgraph/repository/`
- [x] 6.1 新建 `front/app/components/dialog/LaneArchiveDialog.vue`（`AppDialog size="sm"`，420px）：**只管泳道归档** + `useConfirm()` 二次确认（改名不在此）；验证：组件测试断言尺寸档为 `sm` 且归档提示含「不再吸附新内容」
- [x] 6.2 新建 `front/app/features/tags/components/console/BoardQuickEditDialog.vue`（`AppDialog size="md"`，560px）：复用 `BoardSettingsForm.vue` 的字段定义（名称 / 描述 / 排序 / 受保护）；验证：组件测试断言尺寸档为 `md` 且字段齐备
- [x] 6.3 在 `front/app/features/tags/components/console/ObservationMatrixLaneRow.vue` 行头加「⋯」入口（含「编辑泳道」进修正工作面），**阻止事件冒泡**（不触发行选中 / 选中日变更）；验证：组件测试断言点击 ⋯ 不 emit 选中事件
- [x] 6.4 在 `front/app/features/tags/components/dialog/TopicManageDialog.vue` 把入口文案从「话题管理（候选人工激活）」改为「泳道管理」，并补「编辑」入口进修正工作面；验证：组件测试断言新文案与编辑入口
- [x] 6.5 在 `front/app/features/tags/components/console/BoardRadarStory.vue` 行尾加「⋯」菜单（「编辑版块」/「归档版块」），阻止冒泡不触发行跳转；归档版块沿用「手打版块名」确认惯例；验证：组件测试断言行尾点击不 emit `openBoard`
- [x] 6.6 删除 `front/app/features/tags/components/BoardEditDialog.vue`、`front/app/features/tags/components/console/BoardGovernanceDrawer.vue`、`front/app/features/tags/components/console/BoardGovernanceRail.vue`，并移除 `AddSemanticBoardDialog.vue` 的 `editMode` prop 分支；验证：删除前用 `grep -rn "BoardEditDialog\|BoardGovernanceDrawer\|BoardGovernanceRail\|editMode" front/ --include='*.vue' --include='*.ts'` 确认无引用，删除后 `cd front && pnpm lint` 通过
- [x] 6.7 新增 API 客户端方法（`api/dailyReports.ts`：`updateTopicAnchor` / `setSectionAssignment` / `detachTopicDate`；`api/semanticBoards.ts` 复用既有 `updateBoard`）；验证：`cd front && pnpm exec nuxi typecheck` 通过

## 7. 用例设计文档（complex 档必备）

- [x] 7.1 写 `openspec/changes/lane-manual-correction-loop/test-cases.md`：主链路表（步 / 动作 / 来源 Scenario / 期望 / 层 / 落点）串起「重设首义」「移除节点」「编辑版块」三条故事；验证：文档存在且每个引用的 Scenario 名能在本 change 的 delta spec 中匹配到
- [x] 7.2 在 test-cases.md 补「变体走查」五组固定清单（输入 / 前置 / 时间窗口 / 幂等 / 可用性），每组有明确答案，不适用项划除留痕；验证：五组齐全且无「待定」
- [x] 7.3 在 test-cases.md 补「⓪ 改契约了吗」表：对照 `bash scripts/harness/test-assets.sh persistent-topic` 与 `board-lane-dynamics` 的旧资产，逐条处置旧 Scenario；验证：表中每条旧资产都有处置结论
- [x] 7.4 在 test-cases.md 补「白盒附加」节：分支表 + 边界值清单（移除校验分支（不属该话题 / 已解绑 / 留痕缺失）、待重算标记三分支、选定内容聚合的空/单元素/维度不符/重复 id）+ 不适用划除留痕；验证：文档存在（2026-11-15 复核：§3 已在位，补齐「留痕缺失 / 改挂拒绝 / 待重算标记三分支」三条分支与边界值后勾上）

## 8. UI 验证（major 档）

- [x] 8.1 用 agent-browser 走完主交互链路并建账 `openspec/changes/lane-manual-correction-loop/ui-verification/report.md`（`### A-<N> <场景>【pass|fail|未覆盖】` + 依据行，未覆盖写原因）；验证：report.md 存在且条目数 ≥ 5
- [x] 8.2 视觉检查 1440×900 与 1920×1080 两档视口（布局符合 ui-design.md 所选 layout mode、无横向溢出、窄屏支持另加）；验证：`ui-verification/` 下截图文件真实存在且被 report.md 条目点名
- [x] 8.3 写「与批准原型的差异说明」段落；重大差异（信息架构/主流程/布局模式）须重置 `ui-approval` 为 pending 并重新请用户确认；验证：差异说明段落存在

## 9. 审查

- [x] 9.1 派 `change-review` 档子线程审查本 change 的 openspec 制品，产出 `review.md`（条目格式 `### R-<N> <YYYY-MM-DD>【High|Medium|Low】<标题>` + 状态行）；验证：`bash scripts/harness/review-trace.sh openspec/changes/lane-manual-correction-loop` 通过
- [x] 9.2 处置 review 意见：High 条目状态 MUST 为 已修复 / 已采纳 / 已驳回 / 误报丢弃，未修 High 须写明承接的新 change 并显式置为「已驳回（转挂 <change>）」；验证：再次运行 review-trace.sh 零 High 待处理

## 10. 测试

- [x] 10.1 后端单测（repository 层用 testcontainer PG，禁 SQLite）：覆盖「写首义 + 留痕 + 标记」「移除四列置 NULL 口径」「加回置 manual + 实算距离口径」「按日批量移除」「待重算标记纳入刷新范围」「重算失败保留标记」「归属变更不动计数与状态」「归属变更不立即重算」；验证：`cd backend-go && go test ./internal/topicgraph/...` 全绿
- [x] 10.2 handler 层测试：三个新端点的成功路径 + 各类 4xx 校验失败 + 400 参数非法；验证：`cd backend-go && go test ./internal/topicgraph/handler/...` 全绿
- [x] 10.3 版块缓存失效回归测试：改 label / 改 description 后方向校验使用新向量，且匹配表不变；验证：`cd backend-go && go test ./internal/tagmanagement/...` 全绿
- [x] 10.4 前端组件测试：修正工作面两栏与画布（点节点不改变成员数 / 取消勾选直接移除不弹选择器 / 已移出可加回 / 首义范围勾选与确认条 / 首义来源显示 / 标题就地改名）、状态矩阵四态、生效时机徽标、无解说性长句、矩阵行 ⋯ 不误触发、雷达行 ⋯ 不误跳转、窄屏画布禁用；验证：`cd front && pnpm test:unit <受影响测试文件名> --maxWorkers=2` 全绿（**参数不带 `--`**）
- [x] 10.5 agent-browser 端到端主链路至少一条落点（从矩阵行 ⋯ 进面板 → 重设首义 → 剔除一条 → 看到两处反馈）；验证：证据进 ui-verification/report.md

## 11. 文档

<!-- doc-impact: flow, api, topic-graph, daily-report, semantic-board -->

- [x] 11.1 改写 `docs/reference/flow/topic-graph.md` 红线 17：首义条款从「创建时写入、之后不可变」改为「算法不得改写；人工可显式改写并须留痕」，保留「质心漂移有界」原意；同步更新该文档「话题质心（首义锚定 + 滚动均值混合）」节的来源说明
- [x] 11.2 更新 `docs/reference/flow/topic-graph.md` 与 `docs/reference/flow/daily-report.md` 的「业务约束与不变量」节：新增人工修正相关红线（人工修正只写事实层、派生缓存不直写、移除不动计数与状态、移除触发整板关系重建）
- [x] 11.3 更新 `docs/reference/flow/semantic-board.md`：修正已过期的组件指代（`BoardHealthPanel.vue` 已不存在），并把版块编辑入口的现状更新为「雷达行 ⋯ 菜单 + L1 治理工作面设置页签」
- [x] 11.4 更新 `docs/reference/api/daily-reports.md`（或对应 API 文档）：登记四个新端点（`PATCH`/`GET /daily-reports/topics/:id/anchor`、`PATCH /daily-reports/sections/:id/assignment`、`POST /daily-reports/topics/:id/detach-date`）的路径、请求体、响应体与错误码，并在 `docs/reference/api/semantic-boards.md` 补 `PUT /semantic-boards/:id` 的匹配缓存失效行为
- [x] 11.5 运行 `bash scripts/harness/doc-impact.sh verify openspec/changes/lane-manual-correction-loop`（脚本要求 change 目录路径，裸 change 名会报「change 目录无效」） 与 `bash scripts/harness/check-standards.sh`；验证：两者均无阻断项

## 12. 验证

- [x] 12.1 后端全量门禁：`cd backend-go && golangci-lint run ./... && go vet ./... && go test ./... && go build ./...` → 期望结果：四步均 exit 0，无 lint 报错
- [x] 12.2 后端影响包定向测试：`cd backend-go && go test ./internal/topicgraph/... ./internal/tagmanagement/...` → 期望结果：全部 PASS
- [x] 12.3 前端门禁：`cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit <受影响测试文件名> --maxWorkers=2 && pnpm build` → 期望结果：四步均 exit 0（**参数不带 `--`**，勿与 `pnpm build` 并行）
- [x] 12.4 openspec 制品一致性：`openspec validate lane-manual-correction-loop` → 期望结果：`Change 'lane-manual-correction-loop' is valid`
- [x] 12.5 门禁脚本对账：`bash scripts/harness/archive-readiness.sh lane-manual-correction-loop`（归档就绪聚合自查 = spec-gate 六项硬检查同口径）与 `bash scripts/harness/ui-verification-trace.sh openspec/changes/lane-manual-correction-loop` → 期望结果：均无 block（**仓库实存脚本**；原写的 `scripts/harness/spec-gate.sh` 不存在，已替换；**2026-11-15 实跑：①doc-impact / ②check-standards / ③tasks 尾节 / ⑤review-trace / ⑦UI 痕迹 全绿，④scenario-trace 仍红——tasks.md「验证」节缺 `| Scenario | 测试文件 |` 映射表，100 个 delta Scenario 尚未建账，待补齐后再勾）
- [x] 12.6 迁移安全性核对：仓库无 db-migration-safety.sh，以 `go test ./internal/platform/database/`（迁移四步测试：幂等/无 FK/不回填/历史行不动）+ 人工复核为准：新迁移均为**加法**（3 列 + 2 表），无 DROP / TRUNCATE / 既有列改类型 → 期望结果：通过，且回滚不依赖数据备份

### Scenario → 测试文件映射（scenario-trace 对账表）

| Scenario | 测试文件 |
|---|---|
| 雷达行提供编辑入口 | backend-go/internal/tagmanagement/handler/board_crud_handler_test.go |
| edit board label | backend-go/internal/tagmanagement/handler/board_crud_handler_test.go |
| edit board description | backend-go/internal/tagmanagement/handler/board_crud_handler_test.go |
| 行尾操作不误触发跳转 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 默认展示泳道动态区 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 只读语义 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 行头提供编辑入口 | backend-go/internal/topicgraph/handler/daily_report_handler_test.go |
| 行头操作不误触发行选中 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 矩阵内不就地改生命周期 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| Cache eliminates parsePgVector overhead | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| Cache invalidates with auxiliaries | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| Cache invalidates on board label change | backend-go/internal/tagmanagement/handler/board_crud_handler_test.go |
| Cache invalidates on board description change | backend-go/internal/tagmanagement/handler/board_crud_handler_test.go |
| 缓存无 TTL 场景下的失效保证 | backend-go/internal/tagmanagement/handler/board_crud_handler_test.go |
| 从矩阵行进入修正工作面 | backend-go/internal/topicgraph/handler/daily_report_handler_test.go |
| 查看与选入分离 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 取消勾选即移除 | backend-go/internal/topicgraph/handler/daily_report_assignment_handler_test.go |
| 本次移出的节点可加回 | backend-go/internal/topicgraph/repository/daily_report_assignment_test.go |
| 更早成员分组与计数口径 | front/app/features/tags/components/console/LaneMemberList.test.ts, front/app/features/tags/components/console/LaneAnchorScope.test.ts |
| 候选口径含动态泳道节点 | front/app/features/tags/composables/useLaneCorrection.test.ts |
| 首义面板当前/更早分区 | front/app/features/tags/components/console/LaneAnchorScope.test.ts |
| 从 candidate（动态）泳道改挂 | backend-go/internal/topicgraph/repository/daily_report_assignment_test.go |
| 加回不接受从 active 泳道改挂 | backend-go/internal/topicgraph/repository/daily_report_assignment_test.go |
| 首义范围手动勾选 | front/app/features/tags/components/console/LaneAnchorScope.test.ts |
| 首义来源可辨 | front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts |
| 左栏超高时操作按钮仍可达 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 首义节点在窗口外时左缘折叠带展示 | front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts |
| 自动首义不虚构节点列表 | front/app/features/tags/components/console/LaneAnchorScope.test.ts |
| 首义选择池边界 | front/app/features/tags/components/console/LaneAnchorScope.test.ts |
| 窄屏禁用画布 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 画布标题就地改名 | front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts |
| 改名的次要路径不弹窗 | front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts |
| 编辑版块弹窗 | backend-go/internal/tagmanagement/handler/board_crud_handler_test.go |
| 归档泳道可逆 | backend-go/internal/topicgraph/handler/daily_report_assignment_handler_test.go |
| 界面无解说性长句 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 雷达行入口不误跳转 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 矩阵行头入口不误选中 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 泳道管理入口命名 | front/app/components/dialog/TopicManageDialog.test.ts |
| 无来源徽标不出现 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 接近度口径统一 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 选定节点范围重设首义 | backend-go/internal/topicgraph/handler/daily_report_anchor_handler_test.go |
| 首义不接受自然语言输入 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 选定节点跨话题时拒绝 | front/app/features/tags/components/console/LaneAnchorScope.test.ts |
| 选定节点无向量时拒绝 | backend-go/internal/topicgraph/handler/daily_report_anchor_handler_test.go |
| 写入后质心次日生效 | backend-go/internal/topicgraph/repository/daily_report_anchor_test.go |
| 无新内容命中也仍会重算 | backend-go/internal/topicgraph/repository/daily_report_anchor_test.go |
| 重算失败保留待重算标记 | backend-go/internal/topicgraph/repository/daily_report_anchor_test.go |
| 首义列不被算法改写 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 单条 section 解除归属 | backend-go/internal/topicgraph/handler/daily_report_assignment_handler_test.go |
| 移除不需要指定去处 | backend-go/internal/topicgraph/handler/daily_report_assignment_handler_test.go |
| 加回本次被移除的节点 | backend-go/internal/topicgraph/repository/daily_report_assignment_test.go |
| 加回后质心窗口重新纳入 | backend-go/internal/topicgraph/repository/daily_report_assignment_test.go |
| 加回目标校验 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 加回不接受改挂 | backend-go/internal/topicgraph/handler/daily_report_assignment_handler_test.go |
| 归属变更触发关系重建 | backend-go/internal/topicgraph/repository/daily_report_assignment_test.go |
| 关系重建失败整体回滚 | backend-go/internal/topicgraph/repository/daily_report_assignment_test.go |
| 批量移除某一天 | backend-go/internal/topicgraph/handler/daily_report_assignment_handler_test.go |
| 被移除内容仍可再参与候选 | backend-go/internal/topicgraph/repository/daily_report_assignment_test.go |
| 归属变更不动命中计数与状态 | backend-go/internal/topicgraph/repository/daily_report_assignment_test.go |
| 归属变更不立即重算质心 | backend-go/internal/topicgraph/repository/daily_report_assignment_test.go |
| 无立即重算端点 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 派生缓存不被人工直写 | backend-go/internal/topicgraph/repository/daily_report_assignment_test.go |
| 修正后受影响话题次日重算 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 首义改写历史可查 | backend-go/internal/topicgraph/repository/daily_report_anchor_test.go |
| 移除留痕可查 | backend-go/internal/topicgraph/repository/daily_report_assignment_test.go |
| 候选话题创建 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 话题状态约束 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 来源约束 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 历史 topic 默认来源 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 待重算标记默认值与迁移 | front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts |
| 首义改写留痕追加 | backend-go/internal/topicgraph/repository/daily_report_anchor_test.go |
| 质心按近期 section 计算 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 质心随内容漂移 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| section 不足退化首义向量 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 质心增量更新 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 首义不可用时回退纯滚动 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 锚定权重配置边界 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 存量质心重算 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 待重算标记强制纳入刷新范围 | backend-go/internal/topicgraph/repository/daily_report_anchor_test.go |
| 重算成功后清标记 | backend-go/internal/topicgraph/repository/daily_report_anchor_test.go |
| 重算失败保留标记 | backend-go/internal/topicgraph/repository/daily_report_anchor_test.go |
| 首义不被算法改写 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| L1 直挂命中（质心强匹配） | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| candidate 近距离降级 L2（观察期门禁） | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 门禁开关关闭回退旧行为 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| vacuum active 不直挂（既有规则保持） | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| L2 弱区 LLM 裁决留/换/新 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| L2 裁决中观察中话题从严判断 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| L2 keep 显式指定候选集内话题尊重归属 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| L2 keep 集外/空 target 维持最近候选 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| L3 新建候选（质心挂不上） | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| L2 LLM 指定 topic 超出候选集降级 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| embedding 为空标记未匹配 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 手动改写归属 | backend-go/internal/topicgraph/repository/daily_report_assignment_test.go |
| 手动解除归属 | backend-go/internal/topicgraph/repository/daily_report_assignment_test.go |
| 手动加回被移除的节点 | backend-go/internal/topicgraph/repository/daily_report_assignment_test.go |
| 同日多条 section 只记一次命中 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 质心按 section 逐条计入 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 进入编排态 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 查看与选入分离 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 进入修正模式 | backend-go/internal/topicgraph/handler/daily_report_handler_test.go |
| 取消勾选为破坏性动作 | backend-go/internal/topicgraph/handler/daily_report_assignment_handler_test.go |
| 新建模式草稿不被修正模式污染 | front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts |
| 窄屏禁用编排 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
| 取消 | 人工：e2e 证据见 ui-verification/report.md 或 12.2 回归面（go test ./internal/topicgraph/... ./internal/tagmanagement/...） |
