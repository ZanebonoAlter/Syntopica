<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI Design Contract（minor：复用/状态契约）

## 入口与入口变更

入口不变：版块控制台 → 右轨体检卡片（`BoardHealthWorkspace.vue` 428 行，既有组合层）→ 问题项处理流（`BoardHealthIssueIndex` / `BoardHealthIssueDetail` / `BoardHealthRepairEditor` / `BoardHealthRepairProgress`）。本 change 不新增页面、面板、弹窗或导航；唯一入口级变化是**进度文案的信息量**：匹配阶段进度从「正在更新版块匹配（已处理 X/Y）」追加排队信息「· 另有 M 项排队」（M=0 时不追加，保持现状文案）。

新增字段的数据链（只加信息通道，不改取数结构与轮询节奏）：

- 后端 `GET /api/auxiliary-labels/:id/description-repairs/:jobId` 响应 JobView 追加 `queued_jobs`（同版块未结束任务数，int，不含当前任务；语义在 specs delta 锚定）；
- `front/app/api/auxiliaryLabels.ts:getDescriptionRepairJob` 透传；
- `front/app/api/normalizers/auxiliaryRepair.ts:normalizeAuxiliaryRepairJob` → `AuxiliaryRepairJobView` 增字段（缺失容错为 null）；
- `boardHealthWorkspace.ts:boardHealthProgressText`（「已处理 X/Y」唯一派生点）拼接排队段；
- `BoardHealthWorkspaceHead` / `BoardHealthRepairProgress` 既有展示位原样渲染新文案，不改模板结构。

## 受影响状态

| 状态 | 受影响 | 说明 |
| --- | --- | --- |
| 进度中（embedding/saving） | 否 | 文案不变；排队段只在 matching 阶段出现（embedding/saving 秒级即过，避免阶段切换文案抖动） |
| 进度中（matching） | **是** | 文案追加「· 另有 M 项排队」（M≥1 时）；这就是用户「以为卡死」的场景，让排队可见 |
| pollPaused（预算耗尽/网络错恢复态） | 否 | 「恢复查询」按钮行为照旧；恢复后首个含 `queued_jobs` 的响应即生效 |
| error（`*_failed` / conflict / unavailable） | 否 | 失败态展示与重试动作不变 |
| empty（无任务/未确认） | 否 | 无进度文案可追加 |
| success（completed） | 否 | 「更新完成，待下一轮体检验证」文案与「处理下一项」引导不变 |

normalizer 容错：`queued_jobs` 缺失/非有限数字 → null → 文案静默降级不追加排队段（与 `formatRepairCounter` 对 processed/total 缺失标「未知」的策略不同：排队段是增量信息，缺失时不标错、不打扰）。

## 复用组件与布局模式

- **零新组件、零新布局**：全部复用 `BoardHealthWorkspace.vue`（组合）、`BoardHealthWorkspaceHead.vue`（头部文案位）、`BoardHealthRepairProgress.vue`（阶段条，含 pollPaused 恢复按钮）、`boardHealthWorkspace.ts`（纯派生）。
- 布局模式不变：体检卡片在既有右轨结构内，无 layout mode 变更、无 dialog、无自由宽度。
- 文案派生纪律不变：**前端不重算任何计数**——`queued_jobs` 由服务端提供、前端只拼接（与 processed/total 同一纪律）。
- 字段语义锚（test-design ⑤）：`queued_jobs` 为用户可见计数字段，来源语义在本 change specs delta 的 MODIFIED「更新完成与效果验证分离」Requirement（进度真实性与排队可见性）锚定；specs 制品因基线依赖（board-health-triage-advice 未归档）按用户决策留后撰写，锚定关系在此先行声明。

## 验收映射

- **派生层**：`front/app/features/tags/components/console/board-health/boardHealthWorkspace.model.test.ts` 新增用例——matching 阶段 task 含 `queued_jobs=2` → 文案含「另有 2 项排队」；`queued_jobs=0`/null/缺失 → 文案与现状逐字一致（回归锚）。
- **composable 层**：`front/app/features/tags/composables/useAuxiliaryDescriptionRepair.test.ts` 新增用例——轮询响应含 `queued_jobs` 时 `AuxiliaryRepairJobView` 透传该字段；缺字段时为 null 不报错。
- **展示层回归**：`front/app/features/tags/components/console/board-health/BoardHealthWorkspace.repair.test.ts` 既有进度展示断言保持绿（文案追加不破坏既有形态）。
- **真实页面核对**（静态 :5100，agent-browser 或人工，落点在 tasks 验证节定）：连确认两项描述修复后，先进入 matching 的任务进度文案出现排队段；批全部完成后文案回到现状形态。
