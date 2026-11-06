# 4.1 正式评审修复批次（M1 必修 + C3 三条 Low）

> 承接 `.pi/subagents/board-health-formal-review.md`（4.1 全 change 正式评审，0 High / 1 Medium / 3 Low）。
> 本批只做 **M1 必修** 与 **C3 三条 Low 收口**；模型 `minimax-cn/MiniMax-M3.1-Flash:max`（用户指定）。
> 本批**未启服务 / 未构建 / 未部署 / 未跑浏览器 / 未跑全量测试**，**未做任何 Git 写操作**，
> **未触碰其它 change 的脏文件**（`BoardConsoleShell.vue` 的脏行与 `useConsoleReading*` / `ObservationMatrix*` /
> `OtherTopicReading*` / `SignalReportView*` 等一律未碰）。

## 1. 处置结论

| 编号 | 级别 | 评审意见 | 本批处置 |
| --- | --- | --- | --- |
| **M1** | Medium | 组合（composite）构成标签进 triage 覆盖，其 rewrite 项前端 `actionable=true`，但描述修复链全程 `label_type='auxiliary'` 过滤 → repair-context 404 死路；wire 无 `label_type`，前端无从辨别原因 | **已修**（见 §2）：① 后端 `metrics.labels[]` 逐条透传 `label_type`；② 前端 normalizer 接管 `label_type`，非 auxiliary 的 rewrite 项标为**不可执行**并给出明确文案（证据不隐藏、不当它健康）；③ remove 不受影响（走既有 DELETE 链，且**组合标签的构成关系现可被摘除**——见 §2.3）；④ specs / data-map / implementation-contract 补实际语义；⑤ 补用例（rewrite 不可执行且无修复入口、remove 仍可摘除） |
| **C3-L1** | Low | `removedAuxIds` 只增不减：同一标签被重新挂回后「已摘除」徽标仍在、摘除按钮仍灰着 | **已修**：徽标改为「会话摘除记录 ∩ **不在当前构成**」，重挂后徽标自动撤销、按钮恢复（会话记录本身不撤销） |
| **C3-L2**（评审报告 L2） | Low | `matching_failed` 在 `failed=null` 时渲染「有 未知 项匹配失败」，措辞生硬 | **已修**：改为「有匹配更新失败项（数量未知）」，不提具体数（`boardHealthProgressText` 与 `BoardHealthRepairProgress` 两处一致） |
| **C3-L3** | Low | JobView 对外含 `idempotency_key/expected_version`（单用户无风险、合同未禁） | **记录备查，不改**（见 `review.md` 台账） |

> 评审报告另列 **L1**（`degraded` 无法区分「LLM 之前的历史快照」）：存量快照均有 LLM、实际影响近零，
> **本批不改**（见 `review.md` 台账的归属说明）。
>
> **与评审报告最小修法的差异（有意）**：报告括号里建议把非 auxiliary 的 rewrite 项「归入其余/说明行」；
> 本批按处置要求**保留在需核对索引中**并标「只读」+ 明确说明（索引类目标签「建议重写描述 · 只读」、
> 处理区给「组合标签暂不支持描述修复」）——**证据不隐藏、不当它健康**，用户能看到该项存在与依据，
> 但界面上没有任何可执行入口。

## 2. 改动明细

### 2.1 后端：`metrics.labels[]` 透传 `label_type`（M1-①）

| 文件 | 改动 |
| --- | --- |
| `backend-go/internal/tagmanagement/models/board_health.go` | `BoardHealthLabelMetrics` 增 `LabelType string \`json:"label_type"\``（auxiliary / composite；旧快照缺字段 = 空串 = 未知） |
| `backend-go/internal/tagmanagement/service/boardhealth/metrics.go` | `loadCompositionLabels` 把已 SELECT 的 `l.label_type` 填进指标行（构成清单混有组合标签，本就不过滤） |
| `backend-go/internal/tagmanagement/service/boardhealth/repo_pg_test.go` | 新增 `TestLoadCompositionLabelsCarriesLabelType`（PG：辅助标签 → `auxiliary`、组合标签 → `composite`） |
| `backend-go/internal/tagmanagement/handler/board_health_handler_test.go` | 既有 triage 透传用例补 `label_type` 断言（auxiliary + composite 两行） |

**口径纪律**：体检链纯读不变（红线 20）；标签类型是**既有查询结果**的透传，不是新的统计口径；
分诊覆盖面不变（组合标签仍逐条被裁决，不截断、不跳过）。

### 2.2 前端：normalizer 接管 `label_type`，非 aux 的 rewrite 不可执行（M1-②）

| 文件 | 改动 |
| --- | --- |
| `front/app/api/semanticBoards.ts` | `BoardHealthLabelMetric` 增 `label_type?: string`（DTO 纯增量） |
| `front/app/api/normalizers/boardHealthTriage.ts` | 归一器从 `metrics.labels[]` 建 `auxId → labelType` 映射；分诊项视图增 `labelType`；**rewrite 项仅在 `labelType === 'auxiliary'` 时可执行**（composite / 类型未知 → `actionable=false`）；remove 项不受类型影响 |
| `front/app/features/tags/components/console/board-health/boardHealthWorkspace.ts` | 视图模型增 `notExecutableReason` 与纯函数 `boardHealthRepairUnsupportedReason`（composite → 「组合标签暂不支持描述修复」；类型未知 → 「标签类型未知，无法执行描述修复」）；`splitTriageItems` 据此落 `actionable`；`boardHealthProgressText` 的 matching_failed 措辞（failed 未知不提数） |
| `front/app/features/tags/components/console/board-health/BoardHealthIssueDetail.vue` | rewrite 分支：不可执行项渲染只读说明块（`bhw-unsupported-repair`）**替代**编辑器（无 textarea、无提交入口）；标题/引导语按「只读」改写；证据块照常可展开 |
| `front/app/features/tags/components/console/board-health/BoardHealthIssueIndex.vue` | 索引项类别标签对不可执行项标「建议重写描述 · 只读」（不说得像能直接改） |
| `front/app/features/tags/components/console/board-health/BoardHealthWorkspace.vue` | 选中不可执行项时**不拉**必然 404 的 repair-context，改为清空当前视图（不残留上一项的任务/上下文）；证据照常展示 |
| `front/app/features/tags/components/console/board-health/BoardHealthRepairProgress.vue` | matching_failed 措辞与进展摘要一致（failed 未知不提数） |

### 2.3 后端：组合标签的构成关系现可摘除（M1-③，修复链的前置缺口）

- **发现**：`AuxiliaryLabelService.RemoveBoardComposition` 只受理 `label_type='auxiliary'`，而挂载侧
  `addBoardComposition` 受理 `auxiliary|composite`（`board_composition.auxiliary_label_id` 列复用）——
  **摘除侧与挂载侧不对称**：组合标签的挂载关系实际无法解除（既有 UI 的「移除组合挂载」同一条 DELETE 链也会 400）。
  M1 要求「composite remove → 仍可摘除」，故本批把摘除侧对齐挂载侧。
- **改动**：`backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go`
  → `label_type IN ('auxiliary','composite')`（1 行 + 注释）；新增
  `TestAuxiliaryLabelServiceRemoveBoardCompositionAcceptsComposite`（PG：只摘请求的那一行，辅助标签行不动）。
- **口径**：只删**本版块**的构成关系 + 该标签在**本版块**的匹配行；不删标签本身、不动其它版块、不即时回填历史匹配。
- **已知边界（与既有「历史匹配不会自动回填」一致）**：组合标签本身不挂在 tag 上（组件辅助标签才挂），
  因此摘除组合挂载**不会**连带删掉该板由 composite_hit 产生的历史匹配行——它们要等下一次手动回填才重算；
  界面文案沿用既有「历史匹配尚未重算」提示。

### 2.4 前端：C3-L1「已摘除」徽标语义

- `front/app/features/tags/components/console/BoardWideWorkface.vue`：新增 `effectiveRemovedAuxIds`
  = `removedAuxIds` ∩ **不在当前构成**（`labels` + `composites` 同源壳层的 `compositionLabels`），
  再传给工作面（索引徽标 + 处理区「已摘除」注记 + 摘除按钮禁用态一并随之恢复）。
- **未动壳层**：`BoardConsoleShell.vue` 的 `removedAuxIds` 状态与脏行 174/229/230 均未触碰。

## 3. 验证

| # | 命令 | 结果 |
| --- | --- | --- |
| 1 | `cd backend-go && GOMAXPROCS=2 go test -p 1 -count=1 ./internal/tagmanagement/...` | **ok**：13 包全 PASS（boardhealth / auxlabel / handler / models / repository / board / core / sourcestats / auxrepair …） |
| 2 | `cd backend-go && golangci-lint run ./internal/tagmanagement/...` | **0 issues**（含 gofmt；只对本批改动文件做 gofmt，未做全局格式化） |
| 3 | `cd front && pnpm test:unit app/api/normalizers/boardHealthTriage.test.ts app/features/tags/components/console/board-health/boardHealthWorkspace.model.test.ts app/features/tags/components/console/board-health/BoardHealthWorkspace.test.ts app/features/tags/components/console/board-health/BoardHealthWorkspace.repair.test.ts app/features/tags/components/console/BoardWideWorkface.test.ts app/features/tags/components/console/BoardConsoleShellRepair.test.ts --maxWorkers=2` | **ok**：6 文件 / **87 例**全 PASS（新增：normalizer 组合标签 rewrite/remove/类型未知 3 例、视图模型 4 例、组件 2 例、工作面 1 例） |
| 4 | `cd front && npx eslint app/api/semanticBoards.ts app/api/normalizers/boardHealthTriage.ts app/api/normalizers/boardHealthTriage.test.ts app/features/tags/components/console/board-health/ app/features/tags/components/console/BoardWideWorkface.test.ts` | **0 错误**（`.vue` 不在 eslint 配置覆盖范围 → 报「no matching configuration」提示，非问题） |
| 5 | `cd front && pnpm exec nuxi typecheck` | 本批改动行**零错误**；全局仅剩 `app/features/tags/components/SignalReportView.test.ts` 7 处报错——属**另一 change** 的脏文件，未触碰未修，**不宣称全局绿** |

> 未跑 `pnpm build` / 静态部署 / 浏览器 / 全量前端测试 / 后端全量门禁（按本批边界）。

## 4. 未决 / 遗留

1. **L1（`degraded` 与「LLM 之前的历史快照」同形）**：存量快照均有 LLM，实际影响近零，本批不改；
   若真机遇到远古快照文案误导，再在 `boardHealthWorkspaceState` 补一档文案兜底。
2. **L3（JobView 对外含 `idempotency_key/expected_version`）**：单用户无风险、合同未禁，记录备查不改。
3. **C3-L3（工作面与右轨各持一次幂等 GET health-checkup）**：doc-only 记录为已知行为，不改结构。
4. **3.5 几何/截图 与 5.3 真机故事**仍待办（真实几何/双视口验收、agent-browser 真后端流程）。
