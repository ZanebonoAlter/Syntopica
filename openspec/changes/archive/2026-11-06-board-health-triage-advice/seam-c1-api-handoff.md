# Seam C1 交接 — 前端 API/归一器/有界轮询 composable（tasks 3.1，review 修复版）

> board-health-triage-advice seam C 第 1 批。交付**只读+状态机层**：体检分诊的 raw DTO→camelCase 视图、
> 共享描述修复的四个 HTTP 资源、任务/上下文归一器、以及有界轮询 composable。
> **不含**任何 Vue 组件/宿主模板/页面/CSS/图标改动（3.2/3.3 才是 UI）；不启动 dev server、不构建、不部署。
> 模型 `minimax-cn/MiniMax-M3.1-Flash:max`（用户指定）；承接 seam-b6-wiring-handoff.md。
> 后端 2.1-2.6 源码已交付（账本 11/29），本批**未改任何后端文件**。
> **本版含独立审查 BLOCK 的全部处置**（3 High + 5 Medium + 6 Low）：逐条处置与证据见 `seam-c1-review-fixes.md`。

## 1. 改动文件

| 文件 | 改动 |
| --- | --- |
| `front/app/api/semanticBoards.ts` | **纯增量**扩展体检快照 DTO：`metrics.triage`、`metrics.board.magnet_flow`、`metrics.labels[].description/magnet/neighbors/desc_flags` + 四个新 raw 类型。全部 optional → 历史快照/旧夹具零改动 |
| `front/app/api/normalizers/boardHealthTriage.ts` | **新增**：分诊与标签级信号的归一器（视图类型 + `normalizeBoardHealthTriage` / `normalizeBoardHealthLabelSignals`） |
| `front/app/api/normalizers/auxiliaryRepair.ts` | **新增**：修复上下文/任务的 raw wire 类型、camelCase 视图类型、三集合状态词表、归一器、`canonicalAuxiliaryId`、描述三规则校验器 |
| `front/app/api/auxiliaryLabels.ts` | 追加四个修复资源方法 + raw 类型 re-export；**未动**既有 `AuxiliaryLabel` 数字 ID DTO |
| `front/app/features/tags/composables/useAuxiliaryDescriptionRepair.ts` | **新增**：私有 composable（装载/已知任务/提交冻结/重试/有界轮询/恢复/生命周期） |
| `front/app/api/normalizers/boardHealthTriage.test.ts` | **新增**（14 例） |
| `front/app/api/normalizers/auxiliaryRepair.test.ts` | **新增**（19 例） |
| `front/app/api/auxiliaryLabels.test.ts` | **新增**（5 例） |
| `front/app/features/tags/composables/useAuxiliaryDescriptionRepair.test.ts` | **新增**（38 例） |

**未改动**：任何 `.vue` 组件、宿主模板、CSS、图标、`useBoardHealth.ts`、`features/tags/public.ts`（该 feature 无 public.ts，3.2/3.3 同 feature 内消费，无需 facade）、共享 ApiClient。
**未触碰**其它 change 的脏改（`git status` 中 front/ 下的其它脏文件原样保留）；**未做任何 Git 写操作**。

## 2. 实际导出与形态

### 2.1 体检分诊（`api/normalizers/boardHealthTriage.ts`）

```ts
export type BoardHealthTriageCategory = 'remove' | 'rewrite' | 'keep' | 'insufficient'

export interface BoardHealthTriageItemView {
  auxId: string          // 字符串 ID（wire 是数字，边界转 string）
  category: BoardHealthTriageCategory
  evidence: string
  suggestion: string     // 仅 rewrite 语义持有；其它类别恒空串
  actionable: boolean    // remove/rewrite 且证据（rewrite 还需建议）齐备
}

export interface BoardHealthTriageView {
  available: boolean     // false = 历史快照缺 triage 字段（旧形态）
  items: BoardHealthTriageItemView[]
  dropped: number        // 被剔除/不可执行的条目数（供上层提示"分诊数据异常"）
}

export function normalizeBoardHealthTriage(snapshot: BoardHealthSnapshotDTO | null): BoardHealthTriageView
export function normalizeBoardHealthLabelSignals(snapshot: BoardHealthSnapshotDTO | null): BoardHealthLabelSignalView[]
```

`BoardHealthLabelSignalView` = `{ auxId, label, description, magnet: {allMean, failedMean, allSamples, failedSamples, suspicious, insufficient} | null, neighbors: {auxId,label,cosine}[] | null, descFlags: string[] }`。

**降级规则（逐条可测）**：
- 快照缺 `metrics.triage` → `available=false`、条目空（**不**用统计反推诊断）；
- `metrics.triage` 存在但为空数组 → `available=true`、条目空（降级轮次，与旧形态可区分）；
- **按 aux 分组**：组内 category/evidence/suggestion **完全一致** → 视为重复记录，留一条（其余计入 `dropped`）；
  组内存在**任何矛盾** → **整组不可执行**：全部丢弃并按组内条目数计入 `dropped`（不保留任何可执行项、不猜哪个对）；
- 非法分类 / aux 不在 `metrics.labels`（非法引用）/ 证据为空 → 该条丢弃（`dropped++`）；
- `rewrite` 缺 suggestion → 条目保留但 `actionable=false`（无文案无从执行）；
- `keep`/`insufficient` 的 suggestion 一律清空、`actionable=false`（只读结论不带可执行建议）。

### 2.2 修复上下文/任务（`api/normalizers/auxiliaryRepair.ts`）

raw wire：`AuxiliaryRepairLabelRaw` / `AuxiliaryRepairBoardRaw` / `AuxiliaryRepairJobRaw` / `AuxiliaryRepairContextRaw`（字段全 optional，容忍缺字段）。

视图：`AuxiliaryRepairLabelView` / `AuxiliaryRepairBoardView` / `AuxiliaryRepairJobView` / `AuxiliaryRepairContextView`（**全部字符串 ID，canonical 形态**）。

```ts
export type AuxiliaryRepairJobStatus =
  | 'pending' | 'embedding' | 'saving' | 'matching'
  | 'completed' | 'embedding_failed' | 'save_failed' | 'matching_failed'
  | 'conflict' | 'unavailable' | 'unknown'

export type AuxiliaryRepairStage = '' | 'embedding' | 'saving' | 'matching' | 'unknown'

/** 停止轮询集 = 终态 + 三失败 + unknown（unknown 不当后台失败、不当完成）。 */
export const AUXILIARY_REPAIR_POLL_STOP_STATUSES
/** 后端同标签互斥释放集 = completed/conflict/unavailable（三失败态仍持锁 → 只能 retry）。 */
export const AUXILIARY_REPAIR_MUTEX_RELEASED_STATUSES
/** 可重试集 = embedding_failed/save_failed/matching_failed。 */
export const AUXILIARY_REPAIR_RETRYABLE_STATUSES

export function isAuxiliaryRepairPollStopStatus(s): boolean
export function isAuxiliaryRepairNewConfirmAllowedStatus(s): boolean
export function isAuxiliaryRepairMutexReleasedStatus(s): boolean
export function isAuxiliaryRepairRetryableStatus(s): boolean
export function canonicalAuxiliaryId(value: string | number | null | undefined): string
export function normalizeAuxiliaryRepairJob(raw): AuxiliaryRepairJobView | null
export function normalizeAuxiliaryRepairContext(raw): AuxiliaryRepairContextView | null
export function mapAuxiliaryRepairContextResponse(res): AuxiliaryRepairContextResponse
export function mapAuxiliaryRepairJobResponse(res): AuxiliaryRepairJobResponse
export const REPAIR_DESCRIPTION_MAX_RUNES = 500
export function validateRepairDescription(label, description): {ok:true,value:string} | {ok:false,error:string}
```

**计数与状态纪律**：
- `total/processed/failed` 服务端单源、前端**不重算**；缺失或非法 → **null（未知）**，**不得捏造 0**（否则会造出完成进度）；
- `notApplicable` 单列（**不是成功**），旧 wire 缺字段按 **0** 读（既定兼容）；
- `remaining`：wire 缺/非法 → `null`（未知，不展示、**不补 0**、不用差值反推）；
- `version`/`expectedVersion` 不透明只透传；未知 status/stage → `unknown`（不猜语义）；
- 私有执行元数据（`claim_owner`/`phase_token`/`committed_version`/`committed_at`/`claimed_at`/`lease_expires_at`）
  **按白名单逐字段映射丢弃**，即便 wire 混入也不透出；
- 缺 `auxiliary_label` → 上下文 `null`（不造版本）；ID 一律经 `canonicalAuxiliaryId()`（数字串去前导零，
  不做 Number 转换以免 int64 丢精度）→ 冻结键、请求路径、归属校验同一形态。

### 2.3 四资源（`api/auxiliaryLabels.ts`，ApiClient 唯一 HTTP 边界）

| 方法 | 请求 |
| --- | --- |
| `getDescriptionRepairContext(id)` | `GET /auxiliary-labels/{id}/description-repair-context` |
| `createDescriptionRepair(id, {description, expected_version, idempotency_key})` | `POST /auxiliary-labels/{id}/description-repairs` |
| `getDescriptionRepairJob(id, jobId)` | `GET /auxiliary-labels/{id}/description-repairs/{jobId}` |
| `retryDescriptionRepair(id, jobId)` | `POST /auxiliary-labels/{id}/description-repairs/{jobId}/retry`（body `{}`，无参数） |

`id`/`jobId` 接受 `number | string`（前端边界已转 string，URL 与数字 wire 一致）。
返回 `ApiResponse<Raw…>`（`status` 已由既有 ApiClient 填充，**未改 shared client**，无 toast）。

### 2.4 composable（`features/tags/composables/useAuxiliaryDescriptionRepair.ts`）

```ts
export function useAuxiliaryDescriptionRepair(options?: {
  pollIntervalMs?: number   // 默认 3000；非正数回落默认
  maxPollAttempts?: number  // 默认 60；非正数回落默认
  createIdempotencyKey?: () => string  // 默认 crypto.randomUUID
})
```

状态：`context` / `contextLoading` / `contextError` / `task` / `taskError` / `submitting` / `retrying` /
`submitError` / `conflict` / `submitUncertain`（**computed**，按当前标签冻结派生）/ `draft` /
`polling` / `pollPaused` / `frozenSubmit`（computed）。
动作：`loadContext(auxId)` / `setDraft(text)` / `submitRepair()` / `retryTask()` / `resumePolling()` /
`stopPolling()` / `reset()` / `dispose()`。
返回类型 `AuxiliaryRepairActionResult = { ok, error?, uncertain?, conflict?, task? }`。

**幂等与冻结边界（3.1 核心）**：
- 一次确认 = 一个 `idempotency_key` + 一份**冻结** payload（`description` + `expected_version`），按标签存；
- **网络错 / 5xx / 408·499 等超时取消类 = 结果未知**：保留冻结 → 重发必须**同键同体**（逐字段相同）；
- **改了描述** → 拒绝（`uncertain=true`），**不**复用旧键、**不**自动换键重建（防重复创建）；
- **4xx（除超时取消类，含 409）= 服务端确定拒绝**（未建任务）→ 冻结解除，可改正后用新键重提；
  409 额外置 `conflict=true`、**保留草稿**、**透传服务端 message**（不统一写成"版本变化"）；
- 状态判定只用 HTTP `status`，**不解析错误文案**；出错时**已知任务保留**，不重置为"新提交"。

**冻结解除的唯一凭据（implementation-contract §3 澄清）**：
- 只有**同键 + 同 payload + 归属一致**（`idempotency_key`/`new_description`/`expected_version`/`auxiliary_label_id`
  四项全等）的任务才证明"本次确认已被受理" → 采纳并解冻；
- `latest_task=null` → **一律不解冻**（查询为空不是撤销授权）；他键/异 payload 任务 → **不采纳**、
  **不解冻**、不换键、不把别的任务当本次结果；
- 无冻结时（首次进入 / 重新进入恢复）才采纳服务端最新任务（contract §3「重新进入从 context.latest_task 恢复」）；
- 另加**上下文归属校验**：服务端返回的标签必须就是请求的标签（canonical 比较），否则上下文作废。

**互斥边界（前端终态 ≠ 后端未结束互斥）**：
- 后端 partial unique index 的终态集 = completed/conflict/unavailable；**三失败态仍持同标签锁** →
  前端**不得**铸新键新建，只能走 `retryTask`；
- 允许显式新确认的只有互斥已释放的终态；`unknown` 保守禁止。

**轮询边界**：
- 按**明确 task id** 轮询，只发 GET（不写任何东西、不再 POST 建任务）；
- 正间隔（默认 3s）+ 有限预算（默认 60 次）；
- 终态 + 三失败 + **unknown** 停止（unknown 标可恢复，**不当后台失败、不当完成**）；
- 网络错 / 预算耗尽 → 保留已知任务、置 `pollPaused=true`（查询暂停·可恢复），**不伪造后台失败或完成**；
- 手动恢复 `resumePolling()` **只查同一 job**；
- `stopPolling()` / `reset()` / `dispose()` / 卸载停 timer；切标签与迟到响应由 `contextEpoch` + `pollGen` + `submitSeq` 三重守卫丢弃，不串台。

**生命周期**：
- 组件 / effect scope 内创建 → 作用域结束**自动 dispose**（`getCurrentScope()` + `onScopeDispose`，不再假挂 `onUnmounted`）；
- scope 外创建 → dev 提示要求**显式 `dispose()`**；
- `dispose()` = 永久结束（此后不再写任何 ref、不再轮询）；`reset()` = **可复用**的"清当前选择"（随后 `loadContext` 仍可用）；
- 各标签的冻结记录在 `reset()` 后**保留**（未知意图不因离开而消失，切回该标签可恢复）；
- 停止本地查询**不取消**服务端已确认任务。

## 3. 验证（当前 Linux 本机直跑；未跑 build/部署/浏览器）

| # | 命令 | 结果 |
| --- | --- | --- |
| 1 | `cd front && pnpm test:unit app/api/normalizers/boardHealthTriage.test.ts app/api/normalizers/auxiliaryRepair.test.ts app/api/auxiliaryLabels.test.ts app/features/tags/composables/useAuxiliaryDescriptionRepair.test.ts --maxWorkers=2` | **ok**：4 文件 / **76 例**全 PASS |
| 2 | `cd front && pnpm test:unit app/api/semanticBoards.test.ts app/api/normalizers/consoleRail.test.ts app/features/tags/components/BoardHealthPanel.test.ts app/api/client.test.ts app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/components/CompositeLabelEditDialog.test.ts --maxWorkers=2` | **ok**：6 文件 / 63 例全 PASS（受影响旧件） |
| 3 | `cd front && npx eslint <本批 9 个文件>` | 0 问题 |
| 4 | `cd front && pnpm exec nuxi typecheck` | 本批文件**零错误**；剩余报错全属其它 change 的脏文件（`SignalReportView.test.ts`、`console/ObservationMatrixGrid.test.ts`、`console/OtherTopicReading.signals.test.ts`），本批未触碰未修 |
| 5 | 变异验证：① latest=null 时解冻 → 3 例红；② `dispose()` 不停轮询 → 2 例红；③ triage 矛盾组不丢弃 → 2 例红 | 通过（用例确有拦截力） |

> 未跑 `pnpm build` / 静态部署 / 浏览器 / 全量前端测试 / 后端门禁（按 seam 边界与任务约束）。

## 4. 3.2 / 3.3 接入契约（**C2 必遵守**）

**3.2（问题索引 + 单项处理）**
- 数据入口不变：`useBoardHealth(boardId)` 的 `snapshot`（含 `metrics.triage`、`metrics.labels[].magnet/neighbors/desc_flags`）。
- 消费方 `computed(() => normalizeBoardHealthTriage(snapshot.value))` 与 `computed(() => normalizeBoardHealthLabelSignals(snapshot.value))` 取得 camelCase 视图（沿用 `normalizeRailHealth` 既有范式：normalizer 在 `api/normalizers/`，组件只 import）。
- **不新增分诊请求**：triage 只来自既有 `GET /semantic-boards/:id/health-checkup`；旧快照（`available=false`）按旧形态展示，不展示问题清单、不反推诊断。
- 问题索引只列 `remove`/`rewrite`（`actionable=true`）；keep/insufficient 与统计按需展开。
- 摘除仍走既有 `DELETE /semantic-boards/:id/composition/:auxiliary_label_id`（3.4 范围，本批未动）。

**3.3（编辑/确认/真实进度/重试/冲突草稿/重进恢复）**
- 用 `useAuxiliaryDescriptionRepair()`：`loadContext(auxId)` → `draft` 绑定输入 → `submitRepair()` → `task`/`polling` 渲染阶段与计数 → `retryTask()`（仅失败态）→ `resumePolling()`（`pollPaused` 时）→ `reset()`（离开处理区）。
- 组件**不直发请求**、不自行算完成判据（终态词表已导出）。
- **计数展示口径**：`processed`（真实成功）/ `failed` / `notApplicable`（跳过，**不是成功**）/`remaining`（可能 `null` → 不展示）；
  `total/processed/failed` 可能为 **null（未知）** → UI 必须显式处理，不得当 0；
  `descriptionCommitted=true` 只代表描述+向量已原子提交，**不等于**匹配全部完成。
- **失败态只给"重试"**：三失败态（后端仍持同标签锁）不得出现"新建/再次确认"入口；用 `isAuxiliaryRepairRetryableStatus` 渲染。
- **冲突流**：`conflict=true` → 保留 `draft` → 调 `loadContext(auxId)` 取最新版重新核对 → 用户重新确认（新键、新 expected_version）；409 的提示文案用服务端 message 原文。
- **结果未知流**：`submitUncertain=true` / `frozenSubmit` 非空 → 用**同一份草稿**再次 `submitRepair()`（同键同体）；
  若用户改了内容，composable 会拒绝并提示"先重发原内容或加载最新版本重新核对"（**不得**由 UI 私自换键）。
- **生命周期**：组件内 `setup` 直接创建即可（随组件自动 dispose）；组件外/工具函数里创建请放进 `effectScope` 或显式 `dispose()`。
- 布局/文案按已批准的 `ui-design.md` v2（1040px 单级工作面、主从结构）；本批**未做任何布局决定**（layout 契约不适用）。

## 5. 未决 / 待 3.2-3.3 决策

1. **`dropped` 的呈现**：`triage.dropped > 0` 时 3.2 是否在问题索引上方显示"本轮分诊数据异常（N 条已剔除）"——UI 决定，本批只提供计数。
2. **`magnet_flow` 视图**：本批只把它加进 raw DTO（`metrics.board.magnet_flow`），**未**做 camelCase 视图（3.2 的"证据"区若要展示流向，需在 `boardHealthTriage.ts` 补一个 `normalizeBoardHealthMagnetFlow`）。
3. **未知计数（null）的 UI 呈现**：3.3 须显式处理 `total/processed/failed === null`（未知，不得当 0）。
4. **`retrying` 闸门暴露**：3.3 需在重试按钮上反映"重试中"，避免二次点击（composable 已拒绝第二次）。
5. **`magnet_flow`/`neighbors` 的展示密度**：证据区按需展开，具体折叠层级由 ui-design v2 决定。
6. **无 `features/tags/public.ts`**：本 feature 无 facade 文件；3.2/3.3 若需跨 feature 暴露再补（当前同 feature 内消费，不需要）。
