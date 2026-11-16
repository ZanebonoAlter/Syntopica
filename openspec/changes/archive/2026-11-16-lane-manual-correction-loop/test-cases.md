# Test Cases — lane-manual-correction-loop

> complex 档白盒用例文档（test-design ⓪-⑤全覆盖）。断言判据主线程定；实现侧只许按本表落点补测试，不得改判据。测试只跑影响包（change-scope.sh）。Scenario 名与 delta spec 逐字一致（§6 全量对账）。

## 0. 继承与调整（⓪ 改契约了吗 → 有，7 个 MODIFIED Requirement）

| MODIFIED Requirement（capability） | 契约变化 | 旧测试资产 | 处置 |
|---|---|---|---|
| PersistentTopic 持久化数据模型（persistent-topic） | 新增 `centroid_pending_recompute` 列、`topic_anchor_overrides` 表、`daily_report_sections` 留痕三列 | `daily_report_models.go` 相关模型测试 | EXTEND：AutoMigrate 断言加新列/新表；旧断言不动 |
| PersistentTopic 质心表示（persistent-topic） | 刷新范围 = 当日 touched ∪ `centroid_pending_recompute=true`；成功清标记、失败保留 | `daily_report_topic_repository_test.go`（UpdateCentroidOnSectionChange 族） | MODIFY：刷新集合断言改并集口径；旧「仅 touched」断言更新 |
| DailyReportSection 强制归属 PersistentTopic（persistent-topic） | 移除 = 四列 NULL（`persistent_topic_id`/`topic_match_distance`/`topic_match_confidence`/`topic_status_at_report`），`lane_tier` 保留；加回 = 人工归属写法 | `daily_report_assignment_test.go` | ADD：移除/加回 Scenario 测试；旧归属断言不动 |
| Board Embeddings Cache（board-match-cache） | `PUT /semantic-boards/:id` label/desc 变更后调 `InvalidateMatchCache()`；不自动重跑 `topic_tag_board_labels` | `board_crud_handler_test.go` / `semantic_board_cache_test.go` | ADD：失效断言（Cache invalidates on board label/description change）；旧断言不动 |
| 编排态入口与全屏画布（workshop-lane-compose） | 复用骨架增「修正模式」分支（入口/草稿隔离/只读边界） | `BoardComposeOverlay.test.ts` | ADD：修正模式分支；旧编排态断言不动 |
| 泳道动态视图入口（board-lane-dynamics） | 行头新增进修正工作面入口；不就地改生命周期 | `lane_snapshot_repository_test.go` / `daily_report_handler_test.go` | ADD：入口接线测试；旧聚合断言不动 |
| Board editing dialog（board-editing-ui） | 全量编辑改轻编辑（名称/描述/排序/受保护），字段抽取自 `BoardSettingsForm` | `board_crud_handler_test.go` | MODIFY：PUT 请求体断言改轻编辑字段集；删僵尸组件无测试资产 |

## 1. 主链路故事（交付账本：故事绿才算交付）

### 故事 A：重设首义（人工定义面）

| 步 | 动作 | 来源 Scenario（逐字引用） | 期望（断言判据） | 层 | 落点 |
|---|---|---|---|---|---|
| A1 | 以 7 天窗口从矩阵行 ⋯ 打开工作面 | lane-correction-surface#从矩阵行进入修正工作面 | 两栏可见、无右栏、左栏三块齐；不触发行选中 | e2e(agent-browser) | tests/e2e（新）+ ui-verification/report.md |
| A2 | 左栏「首义范围」勾选 n1 + 更早带 n9 | lane-correction-surface#首义范围手动勾选；lane-correction-surface#首义节点在窗口外时左缘折叠带展示 | 折叠带出现、勾选态双向同步；按钮计数=2 | Vitest + e2e | `LaneAnchorScope.test.ts`（新） |
| A3 | 确认条 →「继续重设」 | lane-manual-correction#选定节点范围重设首义 | `PATCH /api/daily-reports/topics/7/anchor` body `{section_ids:[101,109]}`；200 含 `topic_id`/`selected_sections`/人工改写次数/`next_report` | handler(PG) | `daily_report_anchor_handler_test.go`（新） |
| A4 | 服务端写 override | persistent-topic#首义改写留痕追加；lane-manual-correction#首义改写历史可查 | `topic_anchor_overrides` 追加一行，既有行不动（append-only）；**历史端点** `GET /api/daily-reports/topics/:id/anchor/history` 返回改写记录（改前/改后向量摘要 + 选定 section id 列表 + 输入模式 + 操作者 + 时间），**按时间升序**、行数 = 改写次数 | repository(PG) + handler(PG) | `daily_report_assignment_history_test.go`（新）+ `daily_report_history_handler_test.go`（新） |
| A5 | 置待重算标记 | persistent-topic#待重算标记强制纳入刷新范围；lane-manual-correction#无新内容命中也仍会重算 | `centroid_pending_recompute=true` | repository(PG) | 同上 |
| A6 | 次日日报生成 | lane-manual-correction#写入后质心次日生效；persistent-topic#重算成功后清标记；persistent-topic#重算失败保留标记 | 质心 = α·新首义均值 + (1-α)·滚动均值（α=0.5）；成功清标记、失败保留、日报不中断 | service(PG) | 同上 |
| A7 | 重新打开工作面 | lane-correction-surface#首义来源可辨 | 「当前首义来源：人工（2 节点）」列节点；auto 态只显示「自动（创建话题时写入）」 | Vitest | `LaneCorrectionCanvas.test.ts`（新） |

### 故事 B：移除误入节点 + 加回

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| B1 | 成员列表取消勾选 n4 | lane-correction-surface#取消勾选即移除；lane-manual-correction#移除不需要指定去处；lane-manual-correction#单条 section 解除归属 | 零弹窗；`PATCH /sections/:id/assignment` body `{topic_id:null}` | e2e + handler(PG) | `daily_report_assignment_handler_test.go`（新） |
| B2 | 服务端写移除 | persistent-topic#手动解除归属；lane-manual-correction#移除留痕可查 | 四列 NULL、`lane_tier` 保留、留痕三列 `action='detach'`（= 最近一次快照）；`section_assignment_overrides` **追加**一行（append-only，先前行不被覆盖）；**历史端点** `GET /api/daily-reports/sections/:id/assignment/history` 返回变更时间 / 变更前话题 id / 动作（detach / attach），**按时间升序**、行数 = 变更次数 | repository(PG) + handler(PG) | `daily_report_assignment_history_test.go`（新）+ `daily_report_history_handler_test.go`（新） |
| B3 | 事务末重建关系 | lane-manual-correction#归属变更触发关系重建；lane-manual-correction#关系重建失败整体回滚 | `RebuildBoardRelations(tx, boardID)`；失败归属与关系全还原 | repository(PG) | 同上 |
| B4 | 不动计数与状态 | lane-manual-correction#归属变更不动命中计数与状态 | `hit_count`/`consecutive_hits`/`status` 前后相等 | repository(PG) | 同上 |
| B5 | 「已移出（本次）」→「加回来」 | lane-correction-surface#本次移出的节点可加回；lane-manual-correction#加回本次被移除的节点；persistent-topic#手动加回被移除的节点；lane-manual-correction#加回后质心窗口重新纳入 | `{topic_id:7}`；`persistent_topic_id=7`、`confidence='manual'`、distance 实算、`topic_status_at_report=NULL`、`action='attach'`；目标话题 pending 置位 | repository(PG) + e2e | 同上 |
| B6 | 保存失败态 | lane-correction-surface#编辑版块弹窗（同款错误反馈口径）；10.4 前端断言 | 画布保留本轮改动 + 重试保存；无数据丢失 | Vitest | `LaneCorrectionCanvas.test.ts` |

### 故事 C：改名 / 归档 / 版块编辑（衍生三块）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| C1 | 标题点泳道名 → Enter | lane-correction-surface#画布标题就地改名；lane-correction-surface#改名的次要路径不弹窗 | 就地 `AppInput` 形态；Enter 保存、Esc/失焦取消；主路径 dialog 计数=0 | e2e / Vitest | `LaneCorrectionCanvas.test.ts` |
| C2 | 管理列表「重命名」 | lane-correction-surface#泳道管理入口命名（入口组） | `sm` 弹窗（420）保存生效（次要路径） | Vitest | `TopicManageDialog.test.ts`（重命名内聚在泳道管理弹窗，无独立组件） |
| C3 | 「归档」 | lane-correction-surface#归档泳道可逆 | `sm` 弹窗 + `useConfirm()` 二次确认 → `PATCH /api/daily-reports/topics/:id {status:'archived'}` → toast 含「不再吸附新内容」；不改 hit_count / consecutive_hits | handler(PG) + Vitest | `daily_report_handler_test.go` + `LaneArchiveDialog.test.ts` |
| C4 | 雷达行「编辑版块」 | board-editing-ui#雷达行提供编辑入口；board-editing-ui#edit board label；board-editing-ui#edit board description；board-match-cache#Cache invalidates on board label change；board-match-cache#Cache invalidates on board description change | `md` 弹窗（560）四字段；PUT 后 `InvalidateMatchCache()` 被调；`topic_tag_board_labels` 不自动重跑 | handler + 单元 | `board_crud_handler_test.go` |
| C5 | 批量移除某天 | lane-manual-correction#批量移除某一天 | `POST /topics/:id/detach-date` 逐条走 B2 口径；当天全移除 | handler(PG) | `daily_report_handler_test.go` |
| C6 | 归档后重开（**可逆**） | lane-correction-surface#归档泳道可逆 | 归档后 `PATCH /api/daily-reports/topics/:id {status:'active'}` 可恢复 → 泳道重新吸附新内容，**成员内容与归属不丢**（归档只改 status，SHALL NOT 清成员） | handler(PG) + Vitest | `daily_report_handler_test.go` + `LaneArchiveDialog.test.ts` |

### 故事 D：更早折叠带与选择池（画布展示口径）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| D1 | manual 首义含窗外节点 | lane-correction-surface#首义节点在窗口外时左缘折叠带展示 | 虚线分隔+节点数+首义虚线圆点堆叠（非比例）；节点卡可开；勾选同步 | Vitest + e2e | `LaneCorrectionCanvas.test.ts` |
| D2 | 带内 >6 节点 | 同上（上限 6 +「+M」） | 6 +「+M」；全量在左栏「更早 · N」 | Vitest | `LaneAnchorScope.test.ts` |
| D3 | 窗口切 30 天 | 同上（按日期判断） | 窗外节点落回时间轴、带消失 | Vitest | 同上 |
| D4 | auto 态 | lane-correction-surface#自动首义不虚构节点列表 | 无折叠带、无「更早」分组、只有文字标签 | Vitest | 同上 |
| D5 | 选择池 | lane-correction-surface#首义选择池边界 | 可选项覆盖本话题全部成员（窗口内 + 更早成员分组）；候选不入池；首义集成员带首义标 | Vitest | 同上 |
| D6 | 更早成员分组与计数 | lane-correction-surface#更早成员分组与计数口径 | 左栏「窗口 4 · 全部 12」徽标 + 「更早成员 · 8」按月分组、组内滚动、节点卡可开；选择池含全部 12；画布仍只画窗口内 4 条 | Vitest + e2e | LaneMemberList.test.ts / report.md |

### 故事 E：候选口径与改挂（素材来源放开）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| E1 | 窗口内含三类节点 | lane-correction-surface#候选口径含动态泳道节点 | 无归属节点 + 属 candidate 泳道节点 → 进候选区/关联推荐；属 active 泳道节点 → 只读参照不进候选 | Vitest | `useLaneCorrection.test.ts` |
| E2 | 候选行来源标注 | 同上 | candidate 来源的候选行显示「来自 X」泳道名（改挂来源看得见） | Vitest | 同上 |
| E3 | 从动态泳道改挂 | lane-manual-correction#从 candidate（动态）泳道改挂 | 归属改为本泳道 + 留痕记来源与目标 + **两条泳道**均置待重算 | Vitest(go) | `daily_report_assignment_test.go` |
| E4 | 从 active 泳道改挂 | lane-manual-correction#加回不接受从 active 泳道改挂 | 4xx 拒绝；归属与快照列不变；不写留痕 | Vitest(go) | 同上 |
| E5 | 首义面板分区 | lane-correction-surface#首义面板当前/更早分区 | 「窗口内 · N」默认展开 / 「更早成员 · N」默认折叠；徽标「窗口内 N · 全部 M」 | Vitest | `LaneAnchorScope.test.ts` |

## 2. 变体走查（五组固定清单）

| 组 | 变体 | 答案（断言） | 挂靠 Scenario |
|---|---|---|---|
| 输入 | anchor `section_ids` 空/非数组 | 400，无部分写入 | lane-manual-correction#选定节点范围重设首义 |
| 输入 | anchor 自然语言字符串 | 4xx | lane-manual-correction#首义不接受自然语言输入 |
| 输入 | `section_ids` 重复 id | 去重 200；selected_sections 去重 | 同上 |
| 输入 | assignment `topic_id` 字符串/负数/0 | 400 | lane-manual-correction#加回目标校验 |
| 输入 | 超长泳道名 >200 | 截断校验 4xx | lane-correction-surface#画布标题就地改名 |
| 前置 | anchor 目标 topic 不存在/已归档 | 404/409 无写入 | lane-manual-correction#选定节点跨话题时拒绝（同族校验） |
| 前置 | assignment 跨 board 目标 | 4xx 整体拒绝 | lane-manual-correction#加回目标校验 |
| 前置 | section 已属目标时加回 | 幂等 200 不重复留痕 | lane-manual-correction#加回本次被移除的节点 |
| 前置 | 单元素 section_ids | 200（均值退化为该节点向量） | lane-manual-correction#选定节点范围重设首义 |
| 时间窗口 | 重算落在日报生成中段 | pending 并入本轮刷新不遗漏 | persistent-topic#待重算标记强制纳入刷新范围 |
| 时间窗口 | 移除当日又有新命中 | touched ∪ pending 并集；次日按新口径 | lane-manual-correction#修正后受影响话题次日重算 |
| 幂等 | 同一 anchor 重复提交 | override 追加两行（历史保留）最新生效；次数 +2 | persistent-topic#首义改写留痕追加 |
| 幂等 | assignment 部分失败重试 | 整体回滚后重试成功；无半写 | lane-manual-correction#关系重建失败整体回滚 |
| 幂等 | 并发移除同一 section | 一个成功一个幂等 200/409；无双重留痕 | lane-manual-correction#单条 section 解除归属 |
| 可用性（UI） | 误输入反馈 | inline-error 不关界面 | 10.4 组件测试 |
| 可用性（UI） | 空态 | 成员 0 条空态文案；auto 无节点集不虚构 | lane-correction-surface#自动首义不虚构节点列表 |
| 可用性（UI） | 错误态 | 成员加载失败错误块+重试；保存失败保留改动+重试 | B6 |
| 可用性（UI） | 加载态 | 骨架 + 按钮禁用 | 10.4 |
| 可用性（UI） | 超长文本 | 节点标题两行截断+title 全名；ellipsis 不硬裁 | 10.4 |
| 可用性（UI） | 重复提交 | 请求期间按钮 disabled 防双写 | 10.4 |

## 3. 白盒附加（分支表 + 边界值）

| # | 分支 | 判据 |
|---|---|---|
| A1 | anchor：属本话题+同 board+embedding 非空维度一致 | 200 + override 追加 + pending 置位 |
| A2 | anchor：跨话题/跨 board | 4xx 整体拒绝（override 表 0 行） |
| A3 | anchor：embedding 空或维度不一致 | 4xx 整体拒绝 |
| A4 | anchor：watch 物化话题重设 | override 覆盖检索句向量；来源标签变人工 |
| B1 | assignment：topic_id=null | 四列 NULL + detach 留痕 + lane_tier 保留 |
| B2 | assignment：topic_id=数值 | 人工归属写法 + attach 留痕 + 目标 pending |
| B3 | assignment：无归属时传 null | 409（与 spec 校验表一致；单用户下无归属=状态已变，如实报告） |
| B4 | assignment：加回目标话题与该 section 现归属话题不一致，且**现归属为 active 泳道** | 4xx 整体拒绝（SHALL NOT 从已转正泳道改挂、不写留痕）；现归属与四列快照不变 |
| B6 | assignment：加回目标话题与该 section 现归属话题不一致，且**现归属为 candidate 泳道** | 允许改挂：归属改为目标 + 留痕(from=来源,to=目标) + 来源与目标两条泳道均置 pending；质心列不直写 |
| B5 | assignment：移除时目标 section 无既有留痕（此前从未人工变更过归属） | 本次 detach 仍须留下留痕（动作 = detach + 原话题 id 在案），SHALL NOT 因缺历史留痕跳过；留痕与四列置 NULL 同事务 |
| C1 | 质心刷新集合 | touched ∪ pending；成功清失败留 |
| C2 | 质心重算失败 | 标记保留、日报不中断 |
| C3 | 待重算标记三分支 | 标记 false 且当日未 touched → 不纳入刷新；标记 true → 强制纳入（当日无新命中也重算）；重算失败 → 标记保留不中断日报 |
| D1 | PUT board label/desc | InvalidateMatchCache 调用；不重跑 topic_tag_board_labels |
| D2 | PUT board 无字段变化 | 不失效 |
| E1 | RebuildBoardRelations 失败 | 整体回滚 |
| F1 | 折叠带渲染 | 仅人工 override 且含窗外节点；≤6 +「+M」 |
| F2 | 窗口切换 | 按日期比较；覆盖则归位带消失 |

边界值：首义集 1（退化均值）/ 6（上限）/ 7（+M 出现）；窗口首日当天 ∈ 窗口、首日前一天 ∈ 折叠带；α=0.5 固定；override 次数 1→2 两行；30 天窗口恰好覆盖最早节点 → 带空；待重算标记三态 false→true→清/留；无既有留痕行时 detach 仍写留痕。

不适用划除（留痕）：
- 输入组「纯分隔符/大小写」：结构化 JSON 数值/ID 列表，无自由文本解析——不适用
- 幂等组「并发线程安全声明」：无显式并发 API 契约，仅测条件更新行为（B3/并发移除行）——不适用

## 4. 效果核对（真库量化）

| 项 | 触发 | 方法 | 量化 |
|---|---|---|---|
| 人工重设首义次日生效 | 真库选 auto 泳道重设 2 节点首义 | 改前 centroid 存档 → 触发次日重算 → 比对 | 质心变化≠0 且方向=新首义均值方向（cos>0.9）；标记清空 |
| 无新命中话题也生效 | 选当日无命中泳道重设首义 | 同上 | pending 路径独立生效（旧实现会漏——修复点） |
| 版块缓存失效 | PUT board label 后立即跑方向校验 | 比对校验向量新旧 | 用新 label embedding（旧实现用缓存旧向量——修复点） |

## 5. UI 机械断言（agent-browser，静态 :5100）

改名就地 dialog 计数前后=0；归档/版块编辑 elementFromPoint 命中本体；移除后「已移出（本次）」出现且画布 ⊘、加回回成员区；1440×900 / 1920×1080 无横向溢出；375×667 画布禁用+提示+轻弹窗可用；A-10/A-11/A-12 逐条。

## 6. 全量 Scenario 对账（100 条逐条落位）

### lane-correction-surface（21）
从矩阵行进入修正工作面→A1｜查看与选入分离→10.4｜取消勾选即移除→B1｜本次移出的节点可加回→B5｜首义范围手动勾选→A2｜更早成员分组与计数口径→D6｜首义来源可辨→A7｜左栏超高时操作按钮仍可达→§5（A-10）｜首义节点在窗口外时左缘折叠带展示→D1-D3｜自动首义不虚构节点列表→D4｜首义选择池边界→D5（口径已扩：全部成员）｜窄屏禁用画布→§5｜画布标题就地改名→C1｜改名的次要路径不弹窗→C1｜编辑版块弹窗→C4｜归档泳道可逆→C3｜界面无解说性长句→10.4｜雷达行入口不误跳转→10.4｜矩阵行头入口不误选中→10.4｜泳道管理入口命名→C2｜无来源徽标不出现→10.4（A-11）｜接近度口径统一→10.4（A-11）

### lane-manual-correction（25）
选定节点范围重设首义→A3｜首义不接受自然语言输入→变体·输入｜选定节点跨话题时拒绝→变体·前置+白盒A2｜选定节点无向量时拒绝→白盒A3｜写入后质心次日生效→A6｜无新内容命中也仍会重算→A5｜重算失败保留待重算标记→A6+白盒C2｜首义列不被算法改写→回归面（回归断言）｜单条 section 解除归属→B1｜移除不需要指定去处→B1｜加回本次被移除的节点→B5｜加回后质心窗口重新纳入→B5｜加回目标校验→变体·输入｜加回不接受改挂→B1 口径断言｜归属变更触发关系重建→B3｜关系重建失败整体回滚→B3+白盒E1｜批量移除某一天→C5｜被移除内容仍可再参与候选→B5 后置断言（下次日报候选集含它）｜归属变更不动命中计数与状态→B4｜归属变更不立即重算质心→B2 断言（无 centroid 直写）｜无立即重算端点→路由断言（404/无注册）｜派生缓存不被人工直写→B2 断言（centroid 列不被 handler 写）｜修正后受影响话题次日重算→变体·时间窗口｜首义改写历史可查→A4｜移除留痕可查→B2

### persistent-topic（33）
候选话题创建→回归面｜话题状态约束→回归面｜来源约束→回归面｜历史 topic 默认来源→回归面｜待重算标记默认值与迁移→白盒C1 前置（默认 false + 迁移加法）｜首义改写留痕追加→A4｜质心按近期 section 计算→回归面｜质心随内容漂移→回归面｜section 不足退化首义向量→回归面｜质心增量更新→回归面｜首义不可用时回退纯滚动→回归面｜锚定权重配置边界→回归面（α 配置既有）｜存量质心重算→回归面（既有一次性端点不动）｜待重算标记强制纳入刷新范围→A5｜重算成功后清标记→A6｜重算失败保留标记→A6｜首义不被算法改写→回归面（与本 change 首义列不被算法改写同断言）｜L1 直挂命中（质心强匹配）→回归面｜candidate 近距离降级 L2（观察期门禁）→回归面｜门禁开关关闭回退旧行为→回归面｜vacuum active 不直挂（既有规则保持）→回归面｜L2 弱区 LLM 裁决留/换/新→回归面｜L2 裁决中观察中话题从严判断→回归面｜L2 keep 显式指定候选集内话题尊重归属→回归面｜L2 keep 集外/空 target 维持最近候选→回归面｜L3 新建候选（质心挂不上）→回归面｜L2 LLM 指定 topic 超出候选集降级→回归面｜embedding 为空标记未匹配→回归面｜手动改写归属→B5（ADD 新测试）｜手动解除归属→B2（ADD）｜手动加回被移除的节点→B5（ADD）｜同日多条 section 只记一次命中→回归面｜质心按 section 逐条计入→回归面

### workshop-lane-compose（7）
进入编排态→回归面｜查看与选入分离→回归面（10.4 顺带）｜进入修正模式→A1 前置（模式分支 ADD）｜取消勾选为破坏性动作→B1 视觉区分断言（ADD；**该 Scenario 已随原型 v4 改写为直接解除归属口径**——取消勾选即移除，不弹去处选择器）｜新建模式草稿不被修正模式污染→C1 后置断言（ADD）｜窄屏禁用编排→回归面｜右栏职责（已并入左栏口径）→回归面

### board-lane-dynamics（5）
默认展示泳道动态区→回归面｜只读语义→回归面｜行头提供编辑入口→A1 入口断言（ADD）｜行头操作不误触发行选中→10.4｜矩阵内不就地改生命周期→10.4（断言无就地编辑器）

### board-editing-ui（4）
雷达行提供编辑入口→C4｜edit board label→C4｜edit board description→C4｜行尾操作不误触发跳转→10.4

### board-match-cache（5）
Cache miss loads from DB→回归面｜Cache eliminates parsePgVector overhead→回归面｜Cache invalidates with auxiliaries→回归面｜缓存无 TTL 场景下的失效保证→C4 断言（PUT 后必失效，无 TTL 依赖）｜Cache hit returns cached→回归面｜Cache invalidates on upgrade confirm→回归面｜Cache invalidates on board label change→C4（ADD）｜Cache invalidates on board description change→C4（ADD）

> 回归面 = 本 change 不改该行为，由 12.2 定向测试（`go test ./internal/topicgraph/... ./internal/tagmanagement/...`）兜底，不新增用例；实现若触碰这些口径，须回主线程重审（红线：改动波及回归面 = 升级情形②）。
