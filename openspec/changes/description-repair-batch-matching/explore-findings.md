
## A 前端进度轮询链路与排队任务数可见性

## A. 前端进度轮询链路（排队任务数怎么暴露）

### A1. 轮询不在 workspace.ts，而在 composable
- **`front/app/features/tags/composables/useAuxiliaryDescriptionRepair.ts`（675 行）= 唯一的轮询/状态机实现**
  - `DEFAULT_POLL_INTERVAL_MS = 3000`（L52）、`DEFAULT_MAX_POLL_ATTEMPTS = 60`（L53）→ 3 秒一次、最多 60 次（≈3 分钟预算）
  - `pollTarget: { auxId, taskId } | null`（L159）—— **同时只跟一个任务**；`startPolling`（L223）先 `stopPolling()` 再起新目标
  - `pollOnce()`（L233）→ `api.getDescriptionRepairJob(auxId, taskId)`（GET 只读）
  - 停止条件：`isAuxiliaryRepairPollStopStatus(job.status)`（normalizers/auxiliaryRepair.ts L135）终态停；`unknown` / 网络错 / 预算耗尽 → `pollPaused=true` + `taskError` 提示（**不当后台失败**），用户可 `resumePolling()`（L590）手动恢复（只查同一 job）
  - 世代守卫 `pollGen` / `contextEpoch` 防迟到响应串台；`reset()` 只清当前选择，`dispose()` 才永久结束
- **轮询端点**：`front/app/api/auxiliaryLabels.ts`（100 行）
  - `getDescriptionRepairContext(id)` → `GET /auxiliary-labels/:id/description-repair-context`
  - `createDescriptionRepair(id, body)` → `POST /auxiliary-labels/:id/description-repairs`
  - `getDescriptionRepairJob(id, jobId)` → `GET /auxiliary-labels/:id/description-repairs/:jobId`
  - `retryDescriptionRepair(id, jobId)` → `POST /auxiliary-labels/:id/description-repairs/:jobId/retry`

### A2. boardHealthWorkspace.ts（316 行）= 纯派生，无取数
- `boardHealthIssueStatus(status)`（L122）→ pending/embedding/saving/matching = `running`；completed = `completed`；三个 *_failed = `failed`；conflict/unavailable 各自；词表外 = `unknown`
- `boardHealthStageViews(task)`（L170）→ 三阶段视图（embedding/saving/matching），状态 done/current/error/pending，**只按服务端 status/stage 推导**
- `formatRepairCounter(value)`（L203）→ null/undefined/非有限 = 「未知」（**不补 0**）
- `boardHealthProgressText(task)`（L209）→ **「已处理 X/Y」的唯一来源**：`matching` 时拼 `正在更新版块匹配（已处理 ${processed}/${total}）`，`processed`/`total` 直接取 `AuxiliaryRepairJobView.processed` / `.total`（数字，缺字段时 null → 「未知」）。**前端不重算任何计数**
- `BoardHealthWorkspace.vue`（428 行）只做组合：`useBoardHealth(toRef(props,'boardId'))` + `useAuxiliaryDescriptionRepair()`（L~86），`watch(() => props.boardId)` 切版块时 `repair.reset()`；进度文案经 `boardHealthProgressText(repair.task.value)` 传给 `BoardHealthWorkspaceHead`，任务详情传给 `BoardHealthIssueDetail`，阶段条在 `BoardHealthRepairProgress.vue`（301 行），有 `pollPaused` → 「恢复查询」按钮 → `resume-poll` 事件

### A3. 后端 JobView（`backend-go/internal/tagmanagement/service/auxrepair/auxrepair.go`，246 行）
`JobView` 字段（JSON snake_case）：
`id, auxiliary_label_id, idempotency_key, expected_version, old_description, new_description, stage, status, description_committed, total, processed, failed, not_applicable, remaining, error, created_at, updated_at`
- `remaining` 由**后端**算（`total - processed - failed - not_applicable`，下钳 0），前端不重算
- `RepairContext`（同文件）：`auxiliary_label{id,label,description,status,version}` + `direct_boards[]` + `active_tag_count` + `latest_task`（**单个标签的最新一条任务**）

### A4. 关键结论：前端**无法**知道排队任务数
- 后端只有 **4 条** repair 路由（`backend-go/internal/tagmanagement/handler/auxiliary_description_repair_handler.go` L52-61 `RegisterAuxiliaryDescriptionRepairRoutes`）：
  1. `GET  /api/auxiliary-labels/:id/description-repair-context`（单标签上下文，含 latest_task）
  2. `POST /api/auxiliary-labels/:id/description-repairs`（建单任务）
  3. `GET  /api/auxiliary-labels/:id/description-repairs/:job_id`（**单 job**）
  4. `POST /api/auxiliary-labels/:id/description-repairs/:job_id/retry`
- **没有任何「列出版块全部 repair job」的端点**；repository 侧也只有 `GetLatestRepairJob(ctx, auxID)`（单标签最新一条，`auxiliary_description_repair.go` L431），无 list/按状态计数方法
- `GET /api/semantic-boards/:id/health-checkup`（`handler/board_health_handler.go` L40）**不含任何 repair 字段**——`backend-go/internal/tagmanagement/service/boardhealth/*.go` 里 grep `repair` 无命中，体检快照不带修复任务状态
- 前端 `useAuxiliaryDescriptionRepair` 单一 `task` ref 只装**当前选中标签**的任务；`statusOf`（BoardHealthWorkspace.vue）靠 `task.auxiliaryLabelId !== auxId` 判「别的标签 = todo」——**没任务的标签被当成「待核对」，而不是「排队中」**
- ⇒ 「排队任务数」必须**新增后端字段/端点**（如 JobView 加 `pending_jobs` / board 级 repair job 列表端点），前端无源可取

### A5. 与批量合并直接相关的事实
- 同一批 4 项 = 4 个**独立 job**（per-label 幂等键 + 同标签互斥，跨标签不互斥），每个 job 各自冻结**全量 active topic_tags 范围**（~19882 条）→ 4 次全量匹配
- 前端只轮询**选中的那一个 job**（`pollTarget` 单目标），其余 3 个 job 在 UI 上完全不可见 → 合并执行后前端需要新的聚合视图，否则「排队数/合并进度」无载体

**引用**：front/app/features/tags/composables/useAuxiliaryDescriptionRepair.ts:DEFAULT_POLL_INTERVAL_MS/pollOnce/startPolling、front/app/features/tags/components/console/board-health/boardHealthWorkspace.ts:boardHealthProgressText/boardHealthStageViews/boardHealthIssueStatus/formatRepairCounter、front/app/features/tags/components/console/board-health/BoardHealthWorkspace.vue、front/app/features/tags/components/console/board-health/BoardHealthRepairProgress.vue、front/app/api/auxiliaryLabels.ts:getDescriptionRepairJob/createDescriptionRepair/getDescriptionRepairContext/retryDescriptionRepair、front/app/api/normalizers/auxiliaryRepair.ts:normalizeAuxiliaryRepairJob/AuxiliaryRepairJobView/isAuxiliaryRepairPollStopStatus、backend-go/internal/tagmanagement/service/auxrepair/auxrepair.go:JobView/RepairContext/jobView、backend-go/internal/tagmanagement/handler/auxiliary_description_repair_handler.go:RegisterAuxiliaryDescriptionRepairRoutes/getRepairJob/createRepairJob、backend-go/internal/tagmanagement/repository/auxiliary_description_repair.go:GetLatestRepairJob

<!-- pinned 2026-11-06T01:38:53Z -->

## B 主 spec requirement 清单与待改写原文

## B. 主 spec 现有 requirement 措辞（delta 要基于它改写）

### B0. 关键定位事实
- **`openspec/specs/auxiliary-label-description-repair/spec.md` 不存在！**（`openspec/specs/` 下无此目录）——因为 `board-health-triage-advice` change **尚未归档**。
- 该 spec 的**现行文本**在 delta 里：`openspec/changes/board-health-triage-advice/specs/auxiliary-label-description-repair/spec.md`（**77 行**，全是 `## ADDED Requirements`，5 个 Requirement）。
- ⇒ 本 change 的 delta 形式应是 **`## MODIFIED Requirements`**（改写 triage change 里 ADDED 的 5 条），且**归档顺序有依赖**：triage change 必须先归档进 `openspec/specs/`，本 change 再 MODIFY 主 spec。若 triage change 先归档、本 change 还在改同一批 Requirement，会撞 spec-gate。
- 另一处相关 delta：`openspec/changes/board-health-triage-advice/specs/board-health-checkup/spec.md`（**158 行**）；主 spec `openspec/specs/board-health-checkup/spec.md`（**144 行**，已存在）。

### B1. `auxiliary-label-description-repair` delta 的 5 个 Requirement（全部 ADDED）与 Scenario 名
1. **Requirement: 共享描述修复须人工确认**
   - Scenario: 核对并确认共享影响
   - Scenario: 无效描述与禁用标签
2. **Requirement: 描述与 storage 向量原子更新**
   - Scenario: 生成失败保持旧数据
   - Scenario: 修改说明不改变标签身份
   - Scenario: 来源版本已变化
3. **Requirement: 修复任务持久且幂等**
   - Scenario: 双击与请求重试不重复执行
   - Scenario: 离开或服务重启后再进入
   - Scenario: 运行中与完成后的重试幂等
   - Scenario: 失败等待明确重试
4. **Requirement: 修复后更新匹配并独立重试** ← **本 change 的主要改写对象**
   - Scenario: 未归属标签也进入更新范围
   - Scenario: 匹配失败只重试匹配
   - Scenario: 冻结后标签不再适用
   - Scenario: 成功执行可合法零命中
5. **Requirement: 更新完成与效果验证分离** ← **进度/排队 UI 的改写对象**
   - Scenario: 结果不伪造健康改善

### B2. 与本 change 直接相关的 Requirement 原文（逐字）

**【Requirement: 修复后更新匹配并独立重试】**（改写对象①）
> 描述与新向量提交后 SHALL 失效 board 匹配缓存，并用既有匹配引擎更新任务范围内全部活跃 topic_tags 的版块匹配，包含当前未归属任何版块的标签；任务范围在执行起点固定并可恢复，后续新标签沿既有实时匹配路径处理。MUST NOT 仅枚举现有匹配行或只处理当前版块，MUST NOT 改匹配算法或重嵌全部文章。任务 SHALL 返回冻结总数 total、真实成功 processed、失败 failed、不再适用 not_applicable 与未完成 remaining；所有计数由服务端提供，四类之和 SHALL 等于 total。成功指实际调用匹配引擎且正常返回（包括合法零命中）；未调用、报错或失效跳过 MUST NOT 冒充成功。冻结后被删除、禁用或合并的范围项 SHALL 持久记录为不再适用，total 保持不变，不永久卡死任务。部分失败不得标全完成。匹配失败后仅重试失败或未完成匹配，不重嵌/再次保存描述。

关键 Scenario 原文：
- **未归属标签也进入更新范围**：*WHEN* 修复时某活跃 topic_tag 在匹配表没有任何行 *THEN* 该标签 SHALL 仍被纳入本次匹配范围并由既有匹配引擎处理
- **匹配失败只重试匹配**：*WHEN* 描述和向量已保存，但部分 tag 匹配失败 *THEN* 任务 SHALL 标部分完成并返回计数；用户重试只继续未完成匹配，embedding 调用次数不再增加
- **冻结后标签不再适用**：*WHEN* 冻结范围中的标签在处理前已删除、禁用或合并 *THEN* 系统 SHALL 不调用该项匹配，持久记录不再适用，not_applicable 增加而 processed/failed 不增加，total 不缩小；其余适用项仍正常执行
- **成功执行可合法零命中**：*WHEN* 既有匹配引擎对适用标签成功执行并返回空命中结果 *THEN* 系统 SHALL 按成功执行计入 processed，不伪造版块命中，不将正常空结果误报失败

**【Requirement: 修复任务持久且幂等】**（改写对象②——调度与互斥）
> 任务 SHALL 持久记录请求幂等键、标签及来源版本、用户确认的新描述、执行阶段、保存版本、匹配范围与进度、失败原因。相同幂等键重试 MUST 返回同一任务；同标签未结束任务不能并发覆盖。页面关闭不取消已确认任务，worker SHALL 绑定应用生命周期而非提交请求的 context。服务重启后须可查询并恢复失租的未结束阶段；明确失败态 SHALL 等待用户重试，不自动无限重跑。不得凭内存 job 或组件本地状态宣称可恢复。

关键 Scenario 原文：
- **双击与请求重试不重复执行**：*WHEN* 同一提交因双击/超时使用相同幂等键重发 *THEN* 系统 SHALL 返回同一任务，不重复重嵌、不重复提交描述
- **离开或服务重启后再进入**：*WHEN* 用户关闭页面或服务重启后查询已确认的任务 *THEN* 系统 SHALL 返回持久阶段与结果；已保存的描述不再次覆盖，未完成匹配可继续或明确重试
- **运行中与完成后的重试幂等**：*WHEN* 用户重复重试已运行或已完成的任务 *THEN* 系统 SHALL 返回202及同一任务当前状态，不再排队、不启动重复worker、不修改范围/计数/时间；conflict/unavailable则明确409要求重新核对，不借retry修改原描述请求或来源版本
- **失败等待明确重试**：*WHEN* 已确认任务进入embedding_failed、save_failed或matching_failed *THEN* 后台 SHALL 不自动无限重跑；用户retry只重新排入同一任务，已提交描述只续匹配，旧失败清空后展示新尝试

**【Requirement: 更新完成与效果验证分离】**（改写对象③——进度 UI）
> 界面 SHALL 展示生成向量、原子保存、匹配更新三个真实阶段及可执行恢复动作。仅在冻结范围所有仍适用项均匹配成功、其余项均有明确不再适用记录且无失败/未完成项后才显示「更新完成，效果待下一轮体检验证」，并引导处理下一项。旧体检快照、既有日报正文、历史评分 MUST 保持，不因修复被覆盖；不自动触发额外体检，不把复制、已排队、描述已保存或部分完成当成效果已改善。

Scenario 原文：
- **结果不伪造健康改善**：*WHEN* 修复任务成功结束 *THEN* 用户 SHALL 看到匹配更新完成与下一步处理入口，同时知道效果待下一轮体检验证；旧评分和旧快照不变

**【Requirement: 描述与 storage 向量原子更新】**（改写对象④——embedding/saving 前置阶段的合并）
> 系统 SHALL 使用最终确认的新描述生成 storage embedding（label + ': ' + description），生成成功后在事务中校验来源版本、类型与状态并原子保存 description+embedding。MUST NOT 清空旧向量后复用 NULL 回填；MUST NOT 修改 label、aliases、merge_embedding、组合标签向量。生成/保存失败时旧描述和旧向量均保持。并发修改冲突 MUST 明确拒绝并引导重新核对，不能覆盖较新描述。

**【Requirement: 共享描述修复须人工确认】**（改写对象⑤——批量确认语义）
> 系统 SHALL 提供从体检问题项进入的辅助标签描述修复能力：展示实时原描述、建议文案、可编辑新描述、直接引用版块及全局共享影响。描述须 trim 后非空、不仅重复 label、不超过 500 字符。只有用户明确确认才可创建修复任务；只查看建议/打开页面/运行体检 MUST NOT 创建任务。disabled、非 auxiliary 或不存在的标签 MUST 被拒绝。
> - 核对并确认共享影响：*WHEN* 用户确认将「上海」描述从商业场所表述改为城市定义 *THEN* 系统 SHALL 在提交前声明是共享标签、使用它的版块均受影响、匹配更新覆盖全部活跃话题标签，用户取消时零写入

### B3. `board-health-checkup` 相关 Requirement（描述修复动作侧）
主 spec `openspec/specs/board-health-checkup/spec.md`（144 行）Requirement 清单：
体检统计与标签贴合度归因 / 贴合度重算口径 / 事实标签画像与未收编候选 / LLM 综合评分与降级 / 体检快照持久化与趋势 / 异常通知打扰裁决 / 体检自动化与扩充手动边界 / 体检数据查询端点只读
→ **主 spec 里没有「描述修复动作」Requirement**（该能力属 auxiliary-label-description-repair spec）。

triage change delta `specs/board-health-checkup/spec.md`（158 行）里的相关条目：
- ADDED **Requirement: 分诊动作边界** — Scenario: 摘除经用户确认走既有接口 / 重写建议不自动执行 / 分诊不产生匹配写入 / 组合标签的分诊项只读（仅摘除可处理）
- ADDED **Requirement: 分诊以问题处理为主线** — Scenario: 先处理问题而非遍历四组卡片 / **修复完成不冒充体检验证**
- MODIFIED **Requirement: LLM 综合评分与降级**、**体检快照持久化与趋势**、**体检数据查询端点只读**
- 「修复完成不冒充体检验证」原文：*THEN* 修复完成只表示匹配已更新、效果待下一轮体检验证（旧快照与日报不改写）—— 合并批处理不得破坏这条不变量

### B4. 本 change delta 建议落点
- `openspec/changes/description-repair-batch-matching/specs/auxiliary-label-description-repair/spec.md`：`## MODIFIED Requirements` 改写上面 4 条（修复后更新匹配并独立重试 / 修复任务持久且幂等 / 更新完成与效果验证分离 / 描述与 storage 向量原子更新）
- 可能的 ADDED：「同批 job 合并匹配」新 Requirement + 「排队任务数可见」新 Requirement
- 依赖：先归档 `board-health-triage-advice`（其 delta 才能进 `openspec/specs/`），否则 MODIFIED 无基线

**引用**：openspec/changes/board-health-triage-advice/specs/auxiliary-label-description-repair/spec.md、openspec/changes/board-health-triage-advice/specs/board-health-checkup/spec.md、openspec/specs/board-health-checkup/spec.md、openspec/changes/description-repair-batch-matching/.openspec.yaml

<!-- pinned 2026-11-06T01:39:10Z -->

## C 生产接线、executor 调度模型与测试锚点

## C. 生产接线与测试锚点

### C0. 文件行数（记录）
| 文件 | 行数 |
| --- | --- |
| `backend-go/internal/app/runtime.go` | **593** |
| `backend-go/internal/tagmanagement/service/auxrepair/executor_pg_test.go` | **935** |
| `backend-go/internal/tagmanagement/service/auxrepair/matching_pg_test.go` | **595** |
（参考：`auxrepair.go` 246 / `embedding.go` 457 / `executor.go` 389 / `matching.go` 255 / `retry.go` 91 / `embedding_pg_test.go` 965；`front/.../boardHealthWorkspace.ts` 316、`useAuxiliaryDescriptionRepair.ts` 675、`front/app/api/auxiliaryLabels.ts` 100、`front/app/api/normalizers/auxiliaryRepair.ts` 370）

### C1. runtime.go 接线方式（593 行）
- **启动**：`runtime.go` L142-149（在 `tagging.StartAllWorkers()` 之后）：
  ```go
  appCtx, appCancel := context.WithCancel(context.Background())
  if err := startRepairExecutorFn(appCtx); err != nil {
      logging.Errorf("Failed to start the description-repair executor: %v", err)
  }
  ```
  → **worker 绑应用生命周期 ctx**（不是提交请求 ctx，关页面不取消已确认任务）；启动失败只记错误日志（链初始化失败已在 `InitDescriptionRepairChain` 处 Fatalf）。
- **注入缝**（L437-440，可替换的包级 var，单测注入 fake）：
  ```go
  startRepairExecutorFn = tagging.StartDescriptionRepairExecutor
  stopRepairExecutorFn  = func() { tagging.StopDescriptionRepairExecutor() }
  ```
- **关停**：`repairShutdownTimeout = 10 * time.Second`（L~429）—— cancel 后 join 超时上限。
- **实测断言点**：`backend-go/internal/app/runtime_repair_worker_test.go` L144 `require.True(t, tagging.DescriptionRepairExecutorRunning())`、L152 `tagging.StopDescriptionRepairExecutor()`。

### C2. config 从哪来（`backend-go/internal/tagmanagement/wire.go` L266-355）
- `InitDescriptionRepairChain(db)` → `service.NewAuxRepairService(db)`（默认 embedder=auxlabel.DefaultAuxiliaryLabelEmbedder、matcher=board.NewSemanticBoardMatchingService(db)、invalidateCache=board.InvalidateMatchCache、now=time.Now）+ `service.NewAuxRepairExecutor(svc, service.DefaultAuxRepairExecutorConfig())` → `InstallDescriptionRepairChain(svc, exec)` 装域单例
- **config 硬编码、无环境变量/配置项**：`DefaultExecutorConfig()`（executor.go）：
  `PollInterval=5s, LeaseDuration=60s, HeartbeatInterval=20s, HeartbeatTimeout=10s, MaxBatch=1`
  ⇒ **MaxBatch=1 就是「一次一个 job」的单槽来源**；批量合并若走「一批多 job」的路线，config 是第一个要动的地方（且 `Validate()` 强制 heartbeat < lease、MaxBatch ≥ 1）
- 顺序契约：DB ready 之后 → 路由注册之前 → worker 启动之前；不支持热替换已运行的链（换链须先停 worker 并重建 host）

### C3. executor 调度模型（`auxrepair/executor.go` 389 行）—— 批量合并要改的骨架
- `Executor.loop`（L~330）：每轮 `for i := 0; i < MaxBatch; i++ { runRound }`；有活就 continue 继续排干，空则等 `notify`（创建任务时唤醒）或 `PollInterval` tick
- `RunOnce` → `repo.ClaimNextRepairJob(ctx, owner, lease)`（**FOR UPDATE SKIP LOCKED**，`Order("id ASC").Limit(1)`）→ `ErrRepairNoClaimableJob` 时返回 (0, nil)
- `runJob`（L~230）**当前是「一个 attempt 内串完 embedding → saving → matching」**：
  ```go
  if job.DescriptionCommitted { ExecuteMatchingPhase } 
  else { ExecuteEmbeddingPhase; if committed { ExecuteMatchingPhase } }  // 同一 lease、同一 heartbeat
  ```
  ⇒ **批量合并的核心改动点**：把 `runJob` 拆成「先只跑 embedding/saving 的批」+「再跑一次合并 matching」两段；现在 4 个 job 各跑一次全量匹配 = 4× 浪费
- `ExecutorConfig.MaxBatch` 是「一轮执行几个任务」，与「批量合并匹配」是两个概念，需在 proposal 里区分清楚
- claim 语义：claimable = `status='pending'` **或** (`status IN (embedding,saving,matching)` 且 lease 过期)；显式失败态（embedding_failed/save_failed/matching_failed）**只等用户 retry**，worker 不自动重跑（对应 spec「失败等待明确重试」）

### C4. O(N²) 放大器已定位（`auxrepair/matching.go` 255 行 + repository）
- `ExecuteMatchingPhase` 逐条 `for _, item := range items` → 每条 `s.recordItem(...)` → `repo.RecordMatchingItemResult`（`repository/auxiliary_description_repair_matching.go` L115）
- `RecordMatchingItemResult` 在**一个短事务**里做两件事：① UPDATE item 行（只改 pending/failed）；② `r.aggregateRepairCounters(tx, outcome.JobID, job.Total)`（L143）
- `aggregateRepairCounters`（L269-298）= `SELECT count(*) FILTER (WHERE status=...) ... WHERE job_id = ?` —— **对全 job 的 item 行做全量聚合计数**
- ⇒ **N 条 item × 每次一次全量 count 扫描 = O(N²)**，N≈19882 时约 4 亿行扫描量级；每条 item 还额外 1-2 次 `GetScopeTagState`（每条 1 次 COUNT 查询，失败时再 1 次）
- 可批量化方向（评估用，未实施）：把计数改成**增量维护**（成功/失败/失效各 +1，DB 侧 update job 行）或**每 K 条聚合一次**；注意不变量「processed+failed+not_applicable+remaining == total」仍由服务端保证（红线 26）
- `FinishMatchingPhase`（L166）也调一次 aggregate（收尾一次，可接受）

### C5. 测试组织方式
**`executor_pg_test.go`（935 行）**
- 库：**testcontainers-go + 真实 Postgres**，`db := testutil.SetupTestDB(t)`（`backend-go/internal/platform/testutil/testutil.go` L179；容器 `startContainerAndConnect` L112，`openGorm` L161，`runTestMigrations` L452）；断言用 **testify `require`**；**绝不调真实 AI provider**（embedder 用 stub）
- 配置：`auxRepairTestExecutorConfig()`（L~44）—— PollInterval 20ms、Lease 短（测试可越过租期）、HeartbeatInterval 远小于 Lease、MaxBatch=1
- 辅助：`waitForCondition(t, timeout, cond, msg)`（L~55）—— 有界等待，不死等
- 20 个 `Test*` 函数（行号）：
  `ConcurrentClaimSingleWinner`(70) / `HeartbeatKeepsLeaseAcrossTTL`(111) / `HeartbeatLossDoesNotReviveLease`(163) / `RecoversCommittedTaskWithoutReembedding`(229) / `RecoversInterruptedUncommittedTask`(277) / `DoesNotAutoRetryFailedTask`(316) / `RetrySemantics`(376) / `ExecutorLifecycle`(445) / `TaskSurvivesRequestContext`(490) / `WritesOnlyJobAndItems`(526) / `ExecutorConfigValidation`(580) / `NotifyWithoutWorker`(608) / `ResumesTaskWithNullLease`(632) / `PanicInEmbeddingKeepsTaskUncommitted`(709) / `PanicInMatchingKeepsCommittedData`(746) / `KeepsWorkingAfterPanic`(787) / `LeaseUsesDatabaseClock`(822) / `PhaseAcquireRejectsForeignToken`(868) / `LeaseClockAheadStillCompletes`(900) / （+ConfigValidation 无 DB）
  ⇒ **多 job 行为的断言范式已有**：`ConcurrentClaimSingleWinner` 用两个 executor 抢同一 job，断言 `counts[0]+counts[1] == 1`；`DoesNotAutoRetryFailedTask` 断言扫描器不烧 AI quota；`ResumesTaskWithNullLease` 断言 lease 为 NULL 的任务可被接管

**`matching_pg_test.go`（595 行）**
- 同库同栈（testutil.SetupTestDB + testify require）；**自带 stub matcher**（`// ── stub matcher ──` 段）注入 `WithMatcher(...)` 驱动失败与合法零命中；另有一例 `TestAuxiliaryDescriptionRepairMatchingWithRealEngine`(218) 用**真引擎**验证版块挂载真的落库
- 辅助：`prepareCommittedRepairJob(t, svc, labelID, key, description)`（L78）造已提交描述的任务；`frozenItemStatuses(t, db, jobID)`（L90）读回 item 状态分布（**计数断言的抓手**）
- 10 个 `Test*`：`RunsWholeFrozenScope`(108) / `MarksNotApplicable`(168) / `WithRealEngine`(218) / `PartialFailureAndRetry`(263) / `RefusesWithoutLiveOwnership`(325) / `RequiresCommittedDescription`(413) / `EmptyScopeCompletes`(437) / `DoesNotReembed`(467) / `LeavesNeighboursAlone`(500) / `VanishedTagBecomesNotApplicable`(556)
  ⇒ **「匹配失败只重试匹配」「计数四类之和恒等 total」「未归属标签也进范围」已有用例**，批量合并后这些语义必须保持（红线 26）

### C6. 状态/阶段常量（`backend-go/internal/tagmanagement/models/auxiliary_description_repair.go`）
- Status：`pending / embedding / saving / matching / completed / embedding_failed / save_failed / matching_failed / conflict / unavailable`
- Stage：`embedding / saving / matching`
- 终态（互斥已释放）：`completed / conflict / unavailable`
- 批量合并要新增的状态（如 `queued_for_batch` / `batch_id`）会**新增到这张词表**，前端 `boardHealthIssueStatus` 的 switch 需同步（词表外状态走 `unknown` 分支——不会崩，但会显示「状态未知」）

**引用**：backend-go/internal/app/runtime.go:startRepairExecutorFn/stopRepairExecutorFn/repairShutdownTimeout、backend-go/internal/tagmanagement/wire.go:InitDescriptionRepairChain/InstallDescriptionRepairChain/StartDescriptionRepairExecutor/StopDescriptionRepairExecutor、backend-go/internal/tagmanagement/service/auxrepair/executor.go:DefaultExecutorConfig/ExecutorConfig/loop/runJob/RunOnce、backend-go/internal/tagmanagement/service/auxrepair/matching.go:ExecuteMatchingPhase/recordItem、backend-go/internal/tagmanagement/repository/auxiliary_description_repair_matching.go:RecordMatchingItemResult/aggregateRepairCounters/FinishMatchingPhase、backend-go/internal/tagmanagement/repository/auxiliary_description_repair_executor.go:ClaimNextRepairJob/RenewRepairLease、backend-go/internal/tagmanagement/models/auxiliary_description_repair.go、backend-go/internal/tagmanagement/service/auxrepair/executor_pg_test.go、backend-go/internal/tagmanagement/service/auxrepair/matching_pg_test.go、backend-go/internal/app/runtime_repair_worker_test.go

<!-- pinned 2026-11-06T01:39:35Z -->

## D 批量合并的技术依据：引擎是 per-tag 全局匹配，合并后调用数 4N→N 且语义更正确

## D. 批量合并的技术依据（决定性事实，建议 proposal 直接引用）

### D1. 匹配引擎是「per-tag 对全部版块」，不是 per-board
`backend-go/internal/tagmanagement/service/board/semantic_board_matching.go:107` `SemanticBoardMatchingService.MatchTopicTag(ctx, topicTagID)`：
1. `LoadTagAuxiliaries(topicTagID)`（该 tag 挂的 active auxiliary）
2. `LoadTagComposites(...)`
3. `loadBoardAuxiliaries(ctx)` / `loadBoardComposites(ctx)` / `loadBoardEmbeddings(ctx)` —— **全部版块（全局）**
4. `evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliariesFiltered, tagComposites, boardComposites, config, tagEmbedding, boardEmbeddings)`
5. `replaceTopicTagBoardLabels(ctx, topicTagID, matches)` —— **替换该 tag 的全部挂载**

⇒ **一次 `MatchTopicTag(t)` 计算的是「tag t ↔ 所有版块」**，与「正在修哪个辅助标签」无关。
⇒ 4 个 job 各自跑一遍全量匹配 = **对同一批 ~19882 个 tag 重复调用 4 次几乎等价的引擎**（后 3 次基本是纯重复计算；唯一差别是 board embedding 因前序 job 已保存而变化）。
⇒ 合并后引擎调用数：**4 × 19882 → 19882**（4 倍缩减），这就是本 change 的核心收益论证。

### D2. 合并后语义**更正确**（不只是省算力）
- 现状：job1 保存→立刻匹配（此时 job2/3/4 的新向量**还没生效**）→ job2 保存→再匹配（board embedding 又变了）→ …→ 结果是 4 个时间点不一致的匹配快照。
- 合并：4 个标签的 embedding/saving **全部先完成**，再跑一次全量匹配 ⇒ **一次评估、一致快照**。
- 这条应写进 spec 的 Requirement 措辞（「同批 job 合并后匹配须基于全部已提交向量做一次性评估」），否则会被当成纯性能优化被后续 reviewer 砍掉。

### D3. 范围冻结在**创建时**（不是执行时）
`repository/auxiliary_description_repair.go:190` `CreateRepairJob` 事务内：
- L269-276：`Pluck("id", &tagIDs)` where `activeTopicTagsPredicate`（= `status='active' OR status='' OR status IS NULL`）→ **冻结全部 active topic_tag**
- L278-303：建 job 行（`Total = len(tagIDs)`）+ `CreateInBatches(items, repairItemInsertBatchSize)` 写 **每个 job 独立的 item 行**
⇒ 4 个 job = **4 份 ~19882 行 item**（≈7.95 万行），且 4 份的 topic_tag_id 集合**几乎完全相同**（同一时刻冻结，差异仅毫秒级新增）。
⇒ 合并匹配时**天然去重**：按 topic_tag_id 去重后只需一次引擎调用，结果**回填到 4 个 job 的 item 行**（各 job 的 processed/failed/not_applicable 仍按各自 item 结算，四类之和恒等各自 total 的不变量不变）。
⇒ 也说明「4 份 item 行」是本次浪费的另一半（存储 + 每条 COUNT 聚合 = O(N²) 放大器，见 C4）。

### D4. 由此推出的实现边界（供 design 阶段取舍）
- 匹配阶段需要「**一批 job 共享一个匹配轮次**」的抽象：批内按 topic_tag_id 去重 → 跑引擎 → 把结果分发给批内每个 job 的 item 行。
- 每 job 的计数器仍**服务端单独结算**（红线 26：processed+failed+not_applicable+remaining == total，每 job 各自成立）。
- 失败语义要小心：某个 job 的某 item 失败**不能**连坐同批其它 job 的同 tag item（每 job 独立记 failed）——引擎失败是全局的（同一 tag 调用失败对所有 job 同样失败），所以「按 tag 一次判定、结果广播到批内各 job」在语义上是自洽的。
- 现状 `ExecuteMatchingPhase` 是 **per-job** 的（`ListRepairScopeItems(jobID)` → 逐条 → `RecordMatchingItemResult(jobID,...)`），批量化需引入跨 job 的批上下文（可能需要 `batch_id` 列或批内 job id 列表的进程内态——注意**重启恢复**要求：批上下文不能只放内存，否则重启后失租的批无法恢复；这是 design 阶段的硬约束）。

**引用**：backend-go/internal/tagmanagement/service/board/semantic_board_matching.go:MatchTopicTag/replaceTopicTagBoardLabels/evaluateSemanticBoardMatches/loadBoardEmbeddings、backend-go/internal/tagmanagement/repository/auxiliary_description_repair.go:CreateRepairJob/activeTopicTagsPredicate、backend-go/internal/tagmanagement/service/auxrepair/matching.go:ExecuteMatchingPhase/recordItem、backend-go/internal/tagmanagement/repository/auxiliary_description_repair_matching.go:ListRepairScopeItems/RecordMatchingItemResult/aggregateRepairCounters

<!-- pinned 2026-11-06T01:40:04Z -->
