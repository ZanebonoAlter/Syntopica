# Seam C1 复审修复记录（tasks 3.1）

> board-health-triage-advice seam C 第 1 批的**独立只读审查**修复。
> 审查对象：不可变快照 `/tmp/board-health-c1-review-jcfaqcuw`（10 文件）。
> 审查报告：`/home/zanebono/.pi/agent/sessions/--home-zanebono-software-Syntopica--/subagent-artifacts/outputs/a29dfeed-701a-4b41-bbf6-ff6f73b496bf/.pi/subagents/board-health-c1-independent-review.md`
> 判定：**BLOCK**（3 High + 5 Medium + 6 Low）。模型 `minimax-cn/MiniMax-M3.1-Flash:max`（用户指定）。
> 本批**只改** seam C1 的 API/normalizer/composable 与其精准测试 + 本 change 的
> review.md / tasks.md / test-cases.md / data-map.md；**未改**后端、Vue 组件、宿主、CSS、图标、
> 共享 client、其它 change 的脏文件；未做任何 Git 写操作；未跑 build / 浏览器 / 全量测试。

## 0. 勾选处置（账本纪律）

| 时点 | 动作 |
| --- | --- |
| 审查 BLOCK 后 | **tasks 3.1 勾先撤**（`[x]` → `[ ]`），本记录建账 |
| 本批修复完成且新增回归全绿后 | **恢复 3.1 勾**（见 tasks.md 该行的新证据） |

## 1. 已接受问题与处置

### H1（High）`loadContext` 自动解冻 + 采纳他键任务 → 可重复建任务

- **问题**：提交遇 500/网络错 → 冻结保留（键 K）；重载上下文时 `latest_task=null`（POST 未落库）
  或返回**他键**历史任务 → 旧实现 `clearFrozen` → 下次铸**新键 K'** → 服务端可能建第二个任务。
  违反 implementation-contract §3「单次 context 查询为空或只返回别的幂等键任务…不能自动解冻换新键」。
- **处置**（`useAuxiliaryDescriptionRepair.ts` `loadContext`）：
  - 只有**同键 + 同 payload + 归属一致**（`idempotency_key`/`new_description`/`expected_version`/`auxiliary_label_id`
    四项全等）的任务才证明"本次确认已被受理" → 采纳并**解冻**；
  - `latest_task=null` → **一律不解冻**（查询为空不是撤销授权）；
  - 他键/异 payload 任务 → **不采纳**为本次结果（`task=null` + 提示"结果未知，请重发原内容"）、
    **不解冻**、不换键；
  - 无冻结时（首次进入 / 重新进入恢复）才采纳服务端最新任务（contract §3「重新进入从 context.latest_task 恢复」）；
  - 另加**归属校验**：服务端返回的标签必须就是请求的标签（canonical 比较），否则上下文作废。
- **回归**：`useAuxiliaryDescriptionRepair.test.ts` →
  「网络错后重载上下文（latest=null）：不换键，重发仍是原键原 body」、
  「500 后重载上下文（latest=null）：同样不换键」、
  「重载上下文返回他键任务：不采纳为本次结果、不解冻、不换键」、
  「重载上下文返回同键同 payload 任务：确认已被受理 → 采纳并解冻」。
  幂等键工厂改为**递增**（key-1、key-2…）并断言调用次数（旧用例恒返 'key-1' 会掩盖换键 bug）。

### H2（High）卸载后迟到 POST/retry 仍重启轮询

- **问题**：POST 在途 → 卸载 → 200 到达 → 旧实现 epoch 仍相等 → 写 `task` 并 `startPolling` → 组件已卸载仍每 3s GET。
- **处置**：新增**显式生命周期**：
  - `dispose()`：永久结束（`disposed=true` + epoch 自增 + 停轮询）；此后**不再写任何 ref、不再轮询**；
  - `reset()`：**不是** dispose（清当前选择后可继续 `loadContext` 复用；父线程裁定）；
  - Vue 组件 / effect scope：自动 dispose（`getCurrentScope()` + `onScopeDispose`，不再假挂 `onUnmounted`）；
  - scope 外创建：dev 提示要求**显式 dispose()**；
  - 所有 await 之后与 `startPolling` 之前都查 `disposed`/epoch；
  - 停止本地查询**不取消**服务端已确认任务。
- **回归**：「scope.stop()（卸载）在途 POST 迟到：零后续 GET、当前 refs 不复活」、
  「scope.stop() 在途 poll 迟到：不写状态、不再排下一发」、
  「scope.stop() 在途 context 迟到：不复活上下文」、
  「scope.stop() 在途 retry 迟到：不写状态、不重启轮询」、
  「组件卸载后不再发请求（组件作用域自动 dispose）」、
  「scope 外创建：dev 提示要求显式 dispose；显式 dispose 后不再发请求」、
  「reset 不是 dispose：随后仍可加载新标签」。

### H3（High）同 aux 矛盾分诊 first-wins 保留可执行项

- **问题**：triage=[{aux:11,rewrite,suggestion},{aux:11,remove}] → 旧实现留第一条 → 视图给出
  `actionable=true` 的 rewrite，用户据此改写/摘除共享标签。
- **处置**（`api/normalizers/boardHealthTriage.ts`）：**按 aux 分组**，
  - 组内 category/evidence/suggestion **完全一致** → 视为重复记录，留一条（其余计入 dropped）；
  - 组内存在**任何矛盾** → **整组不可执行**：全部丢弃并按组内条目数计入 dropped（不保留可执行项、不猜哪个对）；
  - 不猜诊断、不额外请求、不改后端病型公式。
- **回归**：「一致重复：同标签留一条（其余计 dropped）」、
  「矛盾分类（同 aux 不同 category）：整组不可执行，全部丢弃并计 dropped」、
  「证据/建议矛盾（同 aux 同 category 但证据不同）：整组不可执行」。

### M1（Medium）408 等超时类 4xx 被当"确定拒绝"解冻

- **处置**：新增 `isDefiniteRejection(status)`：无 status（传输错）/ 5xx / **408、499（超时取消类）**
  → 结果未知（保留冻结）；其余 4xx → 确定拒绝（解冻）。**按状态码判定，不解析错误文案**。
- **回归**：「408（超时类）视结果未知：保留冻结，不解冻、不换键」。

### M2（Medium）`retryTask` 无本地并发锁

- **处置**：新增 `retrying` 闸门，与 `submitting` **互斥**（任一在途时另一动作被拒）；双击只发一次 HTTP。
- **回归**：「retry 双击只发一次请求；retry 在途时 submit 被拒（互斥）」。

### M3（Medium）前端"终态" ≠ 后端"未结束互斥"

- **事实**：后端 `models.AuxiliaryDescriptionRepairTerminalStatuses` = completed/conflict/unavailable；
  **三种失败态仍持同标签创建互斥**（partial unique index），铸新键新建必 409。
- **处置**：拆三集合（`api/normalizers/auxiliaryRepair.ts`）：
  - `AUXILIARY_REPAIR_POLL_STOP_STATUSES`（停止轮询）= 终态 + 三失败 + **unknown**；
  - `AUXILIARY_REPAIR_MUTEX_RELEASED_STATUSES`（解除新建互斥）= completed/conflict/unavailable；
  - `AUXILIARY_REPAIR_RETRYABLE_STATUSES` = 三失败。
  新确认只允许互斥已释放的终态；失败态与 unknown **保守禁止新确认**，只走 `retryTask`。
- **回归**：「失败态（后端仍持同标签互斥）：只能重试，不得铸新键新建」。

### M4（Medium）迟到响应跨标签污染

- **处置**：切标签/reset 后的迟到 POST 只**维护该标签自己的冻结记录**
  （成功/确定拒绝 → 解冻；未知 → 标记该标签冻结 uncertain），**不写**当前标签的
  `submitUncertain`/`submitError`/`task`/`conflict`；`submitUncertain` 改为**按当前标签冻结派生**的 computed。
- **回归**：「切标签后旧标签的 POST 迟到：只维护旧标签冻结，不写当前标签的全局 ref」
  （并验证切回旧标签可恢复未知意图）。

### M5（Medium）组件实例外使用无生命周期兜底

- **处置**：见 H2 生命周期段（scope 自动 dispose；scope 外 dev 提示 + 显式 dispose 契约）。

### Low 收口

| # | 问题 | 处置 |
| --- | --- | --- |
| L1 | unknown 状态未停轮询，与 handoff 不一致 | `AUXILIARY_REPAIR_POLL_STOP_STATUSES` 含 unknown；轮询遇 unknown → 停 + 标可恢复（**不当后台失败、不当完成**）。回归：「未知状态：停轮询并标可恢复」 |
| L2 | `reset()` 清 uncertain 却保留冻结，二者不同生同死 | `submitUncertain` 改为按冻结派生的 computed；reset 后切回旧标签可恢复未知意图。回归：「reset 后未知意图仍在」 |
| L3 | 冻结键用 `ctx.label.id`、解冻用入参 auxId，字符串不一致时永不解 | 新增 `canonicalAuxiliaryId()`（数字串去前导零，不做 Number 转换以免 int64 丢精度）；冻结存/查/解除、请求路径、上下文与任务归属校验统一用 canonical 值。回归：「canonical ID 一致性」 |
| L4 | total/processed/failed 缺字段补 0 | 改为 `number | null`（缺失/非法 → null = 未知）；`not_applicable` 维持旧 wire 兼容 0；`remaining` 缺/非法继续 null。回归：「total/processed/failed 缺失或非法：未知（null），不得捏造 0」 |
| L5 | 409 有四种语义，不应统一写成"版本变化" | 409 分支透传服务端 `message`（`auxiliary label has changed…` / `idempotency key was already used…` / 已有任务 / disabled），composable 不改写文案；仅置泛 `conflict=true`。回归：「409 冲突：…透传服务端 message」 |
| L6 | 测试缺口 + 恒定 key 掩盖换键 | 幂等键改递增工厂并断言调用次数；新增 12 条小故事（见 §2） |

## 2. 新增回归故事（全部落地）

| # | 故事 | 断言要点 |
| --- | --- | --- |
| N1 | 网络错 → ctx latest=null → 重发 | 仍原键原 body；key 工厂调用数不增 |
| N2 | 500 → ctx latest=null → 重发 | 同上（500 也是未知） |
| N3 | ctx 返回他键任务 | 不采纳、不解冻、不换键 |
| N4 | ctx 返回同键同 payload 任务 | 采纳并解冻 |
| N5 | 408 | 冻结保留、uncertain=true |
| N6 | 409 → loadContext 刷新 | 草稿仍保留、冲突标记清除 |
| N7 | POST 在途 scope.stop | 零后续 GET、refs 不复活 |
| N8 | poll 在途 scope.stop | 不写状态、不再排下一发 |
| N9 | context 在途 scope.stop | 不复活上下文 |
| N10 | retry 在途 scope.stop | 不写状态、不重启轮询 |
| N11 | 切标签后旧响应 | 不污染新标签；旧标签冻结仍可恢复 |
| N12 | reset → load 新标签 | 仍可用（reset ≠ dispose） |
| N13 | retry 双击 / retry↔submit 互斥 | 只发一次请求 |
| N14 | 失败态 | 只走 retry，不铸新键 |
| N15 | 矛盾 triage vs 一致重复 | 矛盾整组丢弃（dropped=2）；一致留 1 |
| N16 | 未知计数 | total/processed/failed → null，不捏造 0 |
| N17 | canonical ID | 请求路径/冻结键/归属同一形态 |

## 3. 验证

| # | 命令 | 结果 |
| --- | --- | --- |
| 1 | `cd front && pnpm test:unit app/api/normalizers/boardHealthTriage.test.ts app/api/normalizers/auxiliaryRepair.test.ts app/api/auxiliaryLabels.test.ts app/features/tags/composables/useAuxiliaryDescriptionRepair.test.ts --maxWorkers=2` | **ok**：4 文件 / **76 例**全 PASS（修复前 51 例） |
| 2 | `cd front && pnpm test:unit app/api/semanticBoards.test.ts app/api/normalizers/consoleRail.test.ts app/features/tags/components/BoardHealthPanel.test.ts app/api/client.test.ts app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/components/CompositeLabelEditDialog.test.ts --maxWorkers=2` | **ok**：6 文件 / 63 例全 PASS（受影响旧件） |
| 3 | `cd front && npx eslint <本批 9 文件>` | 0 问题 |
| 4 | `cd front && pnpm exec nuxi typecheck` | 本批文件**零错误**；余下报错全属其它 change 脏文件（`SignalReportView.test.ts`、`console/ObservationMatrixGrid.test.ts`、`console/OtherTopicReading.signals.test.ts`），本批未触碰未修 |
| 5 | 变异验证（用例拦截力）：① `loadContext` 在 latest=null 时解冻 → 3 例红；② `dispose()` 不停轮询 → 2 例红；③ triage 矛盾组不丢弃 → 2 例红；④ 计数捏造 0 → 由「未知计数」例红 | 通过（每项均能拦） |

未跑 `pnpm build` / 静态部署 / 浏览器 / 全量测试 / 后端门禁（按本批边界）。

## 4. 措辞纠正（对审查结论的更正，不掩盖）

- 审查写「可重复建任务」的**后果**：后端同标签互斥（partial unique index）与版本校验仍是服务端兜底，
  客户端重复建任务**是否一定落成两个任务**取决于互斥是否已释放（终态后互斥释放 → 可能落成两个任务，
  各自会 embedding + 全范围匹配）。本批**不声称**"已实证生产必双 embedding"；
  确证的违约是**客户端幂等语义**：结果未知时静默换键重建，违反 implementation-contract §3，
  且可能把**别的任务**当成本次确认的结果（错误任务冒充）。两者都必须修，已修。
- 审查 Low-1（unknown 停轮询）本批选择**对齐 handoff**（停 + 可恢复），而不是改成"后台失败"。
- 审查 Medium-5 的"dev 警告"与父线程裁定的"scope 外提示 + 显式 dispose 契约"一致；
  `reset()` 按父线程裁定**不是**永久 disposed（后续 `loadContext` 必须仍可用）。

## 5. 未决 / 移交 3.2-3.3

1. **`dropped` 呈现**：`triage.dropped > 0` 是否提示"本轮分诊数据异常（N 条已剔除）"——UI 决定。
2. **`magnet_flow` 视图**：仍只有 raw DTO，3.2 若要展示失败挂载流向需补 camelCase 视图。
3. **未知计数（null）在 UI 的呈现**：3.3 须显式处理 `total/processed/failed === null`（未知，不得当 0）。
4. **`retrying` 闸门暴露**：3.3 需在重试按钮上反映"重试中"，避免二次点击（composable 已拒绝第二次）。
5. **失败态的 UI 出口**：三失败态只能重试（不得出现"新建"入口），3.3 需按 `isAuxiliaryRepairRetryableStatus` 渲染。
