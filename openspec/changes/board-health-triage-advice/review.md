# 聚焦 Review 处置账本

> 分阶段记录，不代表 tasks 4.1 的全change最终审查已完成。用户已批准v2；原型审查不替代产品code/story审查。全部代码审查只读，不顺手修无关脏改或重跑已通过整套测试。

## Seam A — 独立审查

- reviewer run：e96e730c-e5e3-4785-b55a-3407681628f4，zai-coding-cn/glm-5.3。
- 范围：boardhealth模型/配置/三病信号/LLM/快照与handler透出测试；不含B1未完成时的存储代码。
- 结论：High=0，Medium=0，Low=2，核心口径/SQL/完整triage/60证据护栏/纯读边界已核实。

| 编号 | 级别 | 定位（修复前）与问题 | 作者处置 | 验证 |
| --- | --- | --- | --- | --- |
| A-L1 | Low | boardhealth.go:130 parseFloatDefault放行NaN，新磁铁阈值可能静默失效 | 接受，拒绝非有限值回退默认；保持正常配置及0/100既有边界 | 已修复：`GOMAXPROCS=2 go test -p 1 -count=1 -run 'TestParseFloatDefaultNonFiniteGuard|TestConvertTriageClearsNonRewriteSuggestion' ./internal/tagmanagement/service/boardhealth` → ok（A2批，2026-10-28；NaN含大小写/带符号/首尾空白、±Inf/infinity全回退，0/0.08/100边界不变，越界/坏文本不变） |
| A-L2 | Low | run.go:171 convertTriage原样复制非rewrite suggestion，违背非rewrite不输出该字段的接缝 | 接受，在转换持久化时清空非rewrite suggestion，不因此丢弃合法分类与证据 | 已修复：同上命令 → ok（keep/remove/insufficient多余文案被清空，rewrite保留，JSON省略非rewrite suggestion，输入slice与类别/证据原值保持） |

不把真实LLM质量当作已被字符串测试证明：短名remove误判=0、rewrite文案/证据正确性仍按tasks5.4核对。tier2/tier3截断优先级与异维历史向量属于后续审查/效果观察项，未据此宣称系统无缺陷。

## Seam B1 — 存储底座

任务2.1已交接，实际表/索引/导出方法和PG证据见 seam-b1-handoff.md。独立审查run=cffff836-a888-43a6-b438-7901b844bc4e（zai-coding-cn/glm-5.3）已完成，存储主路/并发/范围/部署顺序正确，提出3项P2，均接受在2.2小批修复，不把规范改成例外。执行worker/原子保存/恢复与前端尚未实现。

| 编号 | 审查级别 | 问题 | 处置与验收 |
| --- | --- | --- | --- |
| B-P2-1 | P2 | aux硬删后同key原请求在锁行前404，未回放已存任务 | 接受：幂等预查询置于活体锁行前，并保留锁后复查/唯一冲突裁决；补建任务→删标签→同key同payload仍同job、异payload冲突、新key404的PG/HTTP回归。**已在2.2批修复并验证**（见下节「Seam B2 — API 接缝」） |
| B-P2-2 | P2 | 首次AutoMigrate失败时迁移no-op却记完成，重启补表后可能永缺部分唯一索引 | 接受：每次成功AutoMigrate后仅自愈该新索引（IF NOT EXISTS），不改全局失败政策；补先无表记迁移、后建表且版本已存在仍补索引的临时PG回归，不删除/清理历史任务。**已在2.2批修复并验证**（见下节「Seam B2 — API 接缝」） |
| B-P2-3 | P2（2.2责任） | 幂等键超过64字符落到DB22001，缺4xx校验 | 接受：service/handler入参边界先拒绝，补65/64边界及无新任务断言；不暴露原始DB错误。**已在2.2批修复并验证**（见下节「Seam B2 — API 接缝」） |

B1报告令牌长度为44是文档笔误，实际`len("v1_")+40=43`；令牌保持不透明，前端不得按长度/结构解码。后续交接校正该笔误（已在 seam-b1-handoff.md §3 与 seam-b2-api-handoff.md 校正）。

## Seam B3 — 执行阶段：事务外 embedding + 事务内原子保存（tasks 2.3）

> **2026-10-29：B3 独立 review 判定 BLOCK → 已接受，待修复后复验。** 审查（只读审 `/tmp/board-health-b3-review-NfMkzs2r` 的 B2-fixed/B3 快照）给出 2 项 Medium + 2 项 Low：
> - **R1（Medium）** 租约校验用事务开始固定的 `now()`，且在等 aux 行锁**之前**检查 → 跨过期仍可保存。修复方向：取得 job/aux **全部锁后**用数据库**实时**时钟 `clock_timestamp()` 复核；最终 job 提交标记的 UPDATE 必须带 owner+实时租约条件并检查 RowsAffected，失权/过期时**整体回滚**已写的 label/向量。
> - **R2（Medium）** 同有效 owner 并发两个 `ExecuteEmbeddingPhase` 时，只靠 owner/lease/未committed 条件的阶段标记挡不住，**两者都会 embed**。修复方向：用原子 CAS 领取本阶段执行权（pending/明确失败后可进；已 embedding/saving 的运行者拒绝 → 失败方直接 busy/owner 错误返回，**不得顺手改坏真实运行者的状态**）；已提交仍零调用；恢复重入需显式授权（过期后新 owner/token 接管），同 token 续有效租约不得隐式重启阶段。
> - **R3（Low）** 开始新尝试与成功提交未清空旧 error → 补 embedding_failed → 成功 matching 时持久 Error 为空。
> - **R4（Low）** 旧 storage 与桩新 storage 用了同一个向量 → 漏测「根本没更新」；需换成不同但合法的同维向量，并断言 `CommittedVersion ==` 回读 PG 标签算出的 `AuxiliaryVersionToken`。
>
> 处置：2.3 的完成勾已暂撤（见 tasks.md），修复与专项复验证据见 `seam-b3-review-fixes.md`，复验后再恢复。**B2 的三项 R 修复已独立复审关闭**（见 `seam-b2-review-fixes.md`）。

**修复结果（2026-10-29，四项全部落地并复验，2.3 完成勾已恢复）**：

| 编号 | 处置 | 复验 |
| --- | --- | --- |
| R1 | 租约一律用数据库**实时** `clock_timestamp()`（`now()`/`statement_timestamp()` 固定在事务/语句开始，均不可用）；`CommitDescription` 在取得 job+aux **全部锁之后**再复核；最终提交标记带 `claim_owner + phase_token + 实时租约` 条件并检查 RowsAffected，失权/过期整体回滚（含已写 label/vector） | `TestAuxiliaryDescriptionRepairExecuteRejectsLeaseExpiredWhileWaiting`（另一事务真实持有 aux 行锁→保存进入等待并跨过租约→释放后拒绝，旧 desc/vector 不变、committed=false）+ `TestAuxiliaryDescriptionRepairExecuteRollsBackWhenCommitMarkerFails`（label 写处注入延时→租约到期→提交标记条件失败→连 label/vector 一起回滚）→ PASS |
| R2 | 新增**阶段执行权原子 CAS**（`AcquireRepairPhase`）：要求描述状态匹配、持 claim 且实时租约有效、**无人持有 phase token**、状态在允许来源集合内；领取写新 token 并清空旧 error；失败方返回 busy，**不写失败状态**；claim 接管作废旧 token，旧在途调用凭 token 失配无法提交 | `TestAuxiliaryDescriptionRepairExecuteRefusesConcurrentSameOwner`（第一次阻塞在 embedder→第二次同 owner busy 拒绝、embed 调用数仍 1、第一次状态未变→释放后仅一次原子保存）+ `TestAuxiliaryDescriptionRepairExecuteAuthorizedRecoveryAfterLeaseExpiry` + `TestAuxiliaryDescriptionRepairExecuteRetryAfterFailure` → PASS |
| R3 | 领取阶段执行权时清空 error；成功提交也写 `error=''` | `TestAuxiliaryDescriptionRepairExecuteRetryAfterFailure`（embedding_failed→重试成功→持久 Error 为空）→ PASS |
| R4 | 桩改用不同但合法同维向量；断言旧!=新、保存后==新；断言 `CommittedVersion ==` 回读 PG 标签算出的 `AuxiliaryVersionToken` | `TestAuxiliaryDescriptionRepairExecuteCommitsAtomically` → PASS |

`GOMAXPROCS=2 go test -count=1 -p 1 -v ./internal/tagmanagement/service/auxrepair` → **ok 15.0s，21/21 PASS**；`golangci-lint run ./internal/tagmanagement/...` → 0 issues；`go vet`/`go build ./...` → ok。逐项证据见 `seam-b3-review-fixes.md`。**2.5 的完整 dispatcher/heartbeat/重启恢复仍未实现**（本批只是底层 CAS 原语）。

## Seam B4 — 匹配执行与逐项持久进度（tasks 2.4）

任务 2.4 已交接：冻结范围经**既有**匹配引擎逐项处理、结果与计数同事务持久化、`not_applicable`/`remaining` 计数口径。实际入口/守卫/默认真实 matcher/owner 边界与 2.5 缺口见 `seam-b4-matching-handoff.md`。**2.5 完整恢复流程 / 2.6 生产接线 / 前端仍未实现**。

## 独立复审台账（B3-fixed / B4 / B5，2026-10-29）

- **B2 复审（Codex 之前的 MiniMax 轮）**：三项 R 已修复并**独立复审关闭**（见 `seam-b2-review-fixes.md`）。
- **B3/B4/B5 独立复审**（只读审固定快照 `/tmp/board-health-b5-review-orPVR6ud`，MiniMax 通道，run `db933a23`）：结论 **OK with notes**——B3 旧四 R **全部真实关闭**、B4 正确、B5 正确（含 **NULL 租约明确已支持**，无需为该猜测改代码）；新增 **1 Medium + 4 Low**。
- **另一路复审（Codex run `43672238`）因 usage limit 失败，无结论**——**不记作「审毕无发现」**。

| 编号 | 级别 | 处置 | 证据 |
| --- | --- | --- | --- |
| R5-1 | Medium（panic 隔离） | **已修**：阶段级 + 执行器级 + 心跳 goroutine 三层 recover，日志含 job/阶段/stack，生命周期 defer 仍收束 HB 与槽位；**纠正复审报告的错误断言**——匹配阶段描述**已提交**，panic 后保留 `description_committed=true` 与新描述/向量并落 `matching_failed`，embedding/保存前 panic 才保持未提交；失败落库仍走 owner+token+实时租约守卫，失权/DB 不可写时不踩新 owner、不假报已记录 | `seam-b5-review-fixes.md` §1 R5-1（3 条受控 panic 桩 PG 回归 → PASS） |
| R5-2 | Low（租约用应用时钟写） | **已修**：租约期限改由数据库算（`clock_timestamp() + make_interval(secs => ?)`，参数化），领取与心跳续租统一；应用时钟不再决定 DB 租约的最终权威；`ClaimJobForExecution` 外部签名不变 | `seam-b5-review-fixes.md` §1 R5-2（时钟落后 1 小时的精准回归 → PASS） |
| R5-3 | Low（阶段 CAS 未查 RowsAffected） | **已修**：0 行 → `ErrRepairJobBusy`，不写状态、不调 AI | `seam-b5-review-fixes.md` §1 R5-3（`phase_token` 非 NULL 空串的防御回归 → PASS） |
| R5-4 | Low（retry 接受 JSON `null`） | **已修**：区分真空体（EOF）与 JSON null（→ 400）；`{}` 与真空体仍合法 | `seam-b5-review-fixes.md` §1 R5-4（handler 新增 `null` 负向 → PASS） |
| R5-5 | Low（逐项聚合 O(N²)） | **驳回（不采纳）**：逐项结果与计数**同事务**是本 change 的明示合同（implementation-contract §1 + data-map「计数字段来源」），「每 K 项再刷新」会静默弱化它；计数单源与精确即时计数保留。当前**无量化瓶颈证据**，后续有实测再单独立项优化——本轮不新增实现、不另起 change | — |

## B6 独立复审台账（2026-10-29，run `db933a23`）

B6 独立复审（只读审固定快照 `/tmp/board-health-b6-review-62WReNwQ`）结论 **OK with notes，无 High/Medium 阻断**：B3 旧四 R、B4、B5 四项修复**经本次复审确认真实关闭**；B6 接线未发现阻断缺陷（显式初始化、单 service、app 生命周期、关停收束、完整故事测试均已核实）。新提 **4 项 Low**，**全部采纳并已修复**（证据见 `seam-b6-review-fixes.md`）：

| 编号 | 级别 | 发现 | 处置与证据 |
| --- | --- | --- | --- |
| F1 | Low | 租约权威已移到 DB 时钟，但 service 预检仍用**应用时钟**（领先 DB 时会误判失权 → 任务活锁） | **已修**：预检只判 owner，租约交给 SQL `clock_timestamp()`（阶段领取在引擎/AI 调用之前 → 过期仍零调用）；应用时钟降级为诊断，`WithClock` 兼容保留。回归：`TestAuxiliaryDescriptionRepairExecutorLeaseClockAheadStillCompletes`（应用时钟领先 1 小时 → 一次 RunOnce 跑完两阶段、completed、embedding 调用数=1、无热扫）→ PASS |
| F2 | Low | 关停 repair 步 `Stop()` 无有界超时 | **已修**：先 cancel 应用 ctx → 后台 join → `repairShutdownTimeout`（10s，单测 200ms）超时只告警并继续 registry/workers，**不谎称已 join**。回归：`TestRepairWorkerShutdownIsBounded`（阻塞 stop 缝 → 总耗时受限、后续步仍执行、ctx 已 cancel）→ PASS |
| F3 | Low（覆盖缺口） | app→域**真实 Start 绑定**无用例 | **已修**：新增真绑定回归（不替换 start 缝；一次性 PG 装真实域链 → 生产缝启动真实执行器 → 断言 worker 在跑、**被取消的请求 ctx 不停 worker**、取消应用 ctx 后 join 立即返回；全程不排任务、不调 provider）→ `TestRepairWorkerRealDomainBinding` → PASS |
| F4 | Low（API 形状） | `InstallDescriptionRepairChain` 允许注册后替换 → 旧 router / 新 worker 两扇门 | **已修（不重构）**：顺序约束写进 API 文档注释（DB ready 之后 / 路由注册之前 / worker 启动之前；运行中替换需先停旧链并重建宿主，**不支持热替换**），并加轻量顺序回归 → `TestDescriptionRepairWiringOrderContract` → PASS |

**复审边界**：本台账只覆盖 B6 接线的 4 项 Low；**前端（seam C）、参考文档对账、真实部署验证仍未完成**，整个 change **未审毕**。

## Seam B6 — 生产源代码接线（tasks 2.6）

四个资源接入生产路由（域根持有同一 service 实例）、app 启动/关停接线（应用生命周期 ctx + 关停先 cancel/join）、域级完整后端故事回归。实际生命周期接口/路径/命令与未解决项见 `seam-b6-wiring-handoff.md`。**本批只做源代码接线**：未启服务、未部署、未写静态产物；写链只发生在一次性 testcontainer PG。

## Seam B5 — 安全执行 / 心跳 / 重启恢复 / 明确重试（tasks 2.5）

任务 2.5 已交接：显式生命周期的执行器（扫描领取 + 租约 + 心跳 + 重启恢复）、用户的明确 retry 与第四个 HTTP 资源。实际导出/默认配置/Start-Stop 约束/扫描与 retry 语义/故障窗与 2.6 接线要求见 `seam-b5-executor-handoff.md`。**2.6 生产接线与前端仍未实现**。

| 边界 | 实现要点 |
| --- | --- |
| 生命周期 | `NewService` 不自启；`Start(appCtx)` 幂等（无双 worker）、`Stop()` 取消并收束 loop/心跳且幂等；worker ctx = 应用生命周期（关页面不取消已确认任务）；通知只唤醒已有 worker，不为每次 POST 起 goroutine；默认单执行槽 + 有界轮询，空队列/DB 错误退避等待 |
| 扫描领取 | 队列即数据库（无内存 map）；只领 pending 与「失租未结束阶段」；明确失败（embedding_failed/save_failed/matching_failed）**不自动重跑**；completed/conflict/unavailable 不扫描；`FOR UPDATE SKIP LOCKED` + CAS → 多实例只一方领取；每次执行新唯一 owner token 并作废旧 phase token；已 committed=true 直续匹配，绝不重嵌/重存 |
| 心跳 | 只在当前唯一 owner 且租约**未过期**时续租（数据库实时 `clock_timestamp()`）；不复活过期租约、不重置 phase_token、不清别人 owner；失权/不确定 DB 故障→取消本次 phase ctx（老 actor 不能提交/标进度）；心跳 SQL 有有限 ctx；收尾停 ticker/goroutine |
| retry | 同一任务重新排入（不新建 job、不改 expected_version/new_description/冻结范围、清旧 error、作废旧执行权）；已 committed 只续匹配；运行中/已完成 → 202 no-op（连 UpdatedAt 都不变）；conflict/unavailable → 409；越界/不存在 → 404；body 只允许空/`{}`，任何参数/非法 JSON → 400；并发 retry 与扫描由 DB 仲裁；retry 不提前查活体标签（来源再变化由阶段校验拒写） |

证据：`GOMAXPROCS=2 go test -count=1 -p 1 -v ./internal/tagmanagement/service/auxrepair` → **ok 27.1s，34/34 PASS**（B3 12 + B4 10 + B5 12）；`go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepair' -v ./internal/tagmanagement/handler` → ok 6.3s，9/9 PASS；`golangci-lint run ./internal/tagmanagement/...` → 0 issues；`go vet`/`go build ./...` → ok。**诚实边界**：匹配调用在途不可中断 → 崩溃后至少一次幂等重放（不宣称 exactly-once）；本批**未接生产**（2.6）。

## Seam B2 — API 接缝（tasks 2.2）

> **2026-10-29：B2 独立 review 判定 BLOCK → 已修复并复验，2.2 完成勾已恢复。** 独立 review（只读审不可变快照 `/tmp/board-health-b2-review-bKcdnuTb`，未改代码）给出 1 项 Medium + 2 项 Low：① 新 handler 的 POST 用 `ShouldBindJSON`，Gin decoder 只 Decode 一次 → 有效对象后追加第二个对象或非空垃圾仍会创建任务；② 复用的 `parseUintParam` 接受完整 uint64，而 PG 的 label/job id 列是 signed bigint → 越界 id 被驱动拒绝后落成 500；③ `repository/auxiliary_description_repair_p2_test.go` 拿 topic_tag 的 id 比 label 的 id（碰巧等值的假阳性），且未证明冻结项存的是正确的 tag。
> 处置：① 仅该端点改为「一次对象 Decode + 第二次必须 io.EOF」（尾随空白仍合法），**不改全局绑定器**；② 仅这三个新端点限制为正 int64（MaxInt64 合法→404，MaxInt64+1/MaxUint64→400），**不改共享 `parseUintParam` 与旧接口**；③ 断言改与 `label.ID` 比并让两种 id 故意不等，再查持久 items 断言 `TopicTagID == tag.ID`。
> 复验：`GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepair' -v ./internal/tagmanagement/handler` → 8/8 PASS（12 子例）；`go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepairReplayAfterLabelHardDelete' -v ./internal/tagmanagement/repository` → PASS；`golangci-lint run ./internal/tagmanagement/handler/... ./internal/tagmanagement/repository/...` → 0 issues；`go vet` → ok。逐项证据见 `seam-b2-review-fixes.md`。

任务2.2已交接：三个 HTTP 资源（context / create / job 查询）+ 三项 P2 修复。实际文件、导出符号、JSON 形态、错误映射与验证命令见 seam-b2-api-handoff.md。**2.3 原子保存 / 2.4 匹配执行 / 2.5 恢复重试 / 2.6 生产接线与前端均未实现**，本节不宣称整条修复链已交付。

| 编号 | 级别 | 问题 | 处置与验收 |
| --- | --- | --- | --- |
| B-P2-1 | P2 | aux硬删后同key原请求在锁行前404 | 已修复：幂等判定前置到活体锁行之前（`checkStoredRequest` 复用同一判定函数，锁前/锁后各判一次，23505 仍走原裁决路径），DB 互斥索引与活体锁行均保留。回归：`GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepairReplayAfterLabelHardDelete' -v ./internal/tagmanagement/repository` → PASS（同key同payload回同一job、异payload→ErrRepairIdempotencyConflict、新key→ErrRepairLabelNotFound、任务数仍1、冻结范围保留）；HTTP 侧 `go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepair' -v ./internal/tagmanagement/handler` → PASS（删除后重放仍202且job id相同、异payload 409、新key 404） |
| B-P2-2 | P2 | 迁移no-op被记完成 → 重启补表后永缺部分唯一索引 | 已修复：`ensureAuxiliaryDescriptionRepairMutexIndex`（`CREATE UNIQUE INDEX IF NOT EXISTS`）由迁移 20261028_0001 的 Up 与 `RunAutoMigrate` 成功后的自愈 hook **共用同一 helper**；hook 位于成功 AutoMigrate 之后、无关 discovery 补全之前（补全报错不得掩盖自愈），只补本 change 的新 job 索引，不动其它历史迁移/表，不清理历史任务，未改全局 InitDB 的 AutoMigrate 非致命失败政策。回归：`GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepairMutexIndexSelfHeal' -v ./internal/platform/database` → PASS（无表→迁移 no-op 且版本已记录→建表→RunAutoMigrate 仍补索引→重复自愈幂等且索引仍为1条，断言为 partial unique 且排除三个终态） |
| B-P2-3 | P2 | 幂等键/版本令牌越界落到DB22001 → 500 | 已修复：边界在 service/handler 入参侧先拒（key 空/超64字符、version 缺失/超64字符 → 400；描述三规则 → 400），DB 22001 不再出现；未知错误一律 500 且不外泄原始DB详情。回归：repository `TestAuxiliaryDescriptionRepairRequestBoundaryRejections` → PASS（64字符键被接受、65字符键/空键/版本缺失/版本65字符各自命中对应 sentinel、任务数仍1）；handler `TestAuxiliaryDescriptionRepairCreateEndpoint` → PASS（400 分支齐、无新任务）；`TestAuxiliaryDescriptionRepairUnknownErrorStaysInternal` → PASS（存储故障 → 500 且响应体只有 "internal server error"，明细只进日志） |

补充口径：`expected_version` 是不透明令牌，只校验「必填 + 容量」，任何解码尝试都是错的（B1 报告的 44 字符是笔误，实际 43）。

外部测试欠账已登记（不算本change导致，也不修外部代码）：

`backend-go/internal/platform/database:TestModelTagConstraints_MaterializedInDB+TestBoardSignalMigrationIdempotent`

组合运行因既有20260922_0001 board-signal约束依赖报2BP01；本change迁移注销后仍复现，注册恢复后单测各自PASS。详证据见B1交接§5。已使用test-patrol --register，domain=database，context=board-health-triage-advice，status=open；不因其重复运行组合或扩大修复范围。

## Seam C1 — 前端 API/归一器/有界轮询（tasks 3.1）

> 独立只读审查：快照 `/tmp/board-health-c1-review-jcfaqcuw`（10 文件），判定 **BLOCK**（3 High + 5 Medium + 6 Low）。
> 报告：`…/outputs/a29dfeed-701a-4b41-bbf6-ff6f73b496bf/.pi/subagents/board-health-c1-independent-review.md`。
> 处置：本批（3.1）完成勾**先撤**、修复后恢复；逐条处置与证据见 `seam-c1-review-fixes.md`。
> 只改 seam C1 的 API/normalizer/composable 与其精准测试 + 本 change 文档；未动后端/组件/宿主/共享 client。

| 编号 | 级别 | 问题 | 作者处置 | 验证 |
| --- | --- | --- | --- | --- |
| C1-H1 | High | `loadContext` 以 latest=null 或他键任务自动解冻 → 结果未知时静默换键重建；并把别的任务当本次结果 | 解冻的唯一凭据 = **同键 + 同 payload + 归属一致**；latest=null 一律不解冻；他键任务不采纳不解冻；无冻结时才按 latest_task 恢复；另加上下文归属校验 | 已修复：`pnpm test:unit app/features/tags/composables/useAuxiliaryDescriptionRepair.test.ts --maxWorkers=2` → 38 例 PASS（含 latest=null/他键/同键三故事 + 递增 key 工厂调用数断言）；变异（latest=null 时解冻）→ 3 例红 |
| C1-H2 | High | 卸载后迟到的 POST/retry 仍写 refs 并重启轮询 | 新增显式生命周期：`dispose()` 永久结束、`reset()` 可复用切清、组件/effect scope 自动 dispose（onScopeDispose）、scope 外 dev 提示 + 显式 dispose 契约；await 后与 startPolling 前均查 disposed/epoch | 已修复：同命令 → 38 例 PASS（POST/poll/context/retry 在途 scope.stop 四例 + 组件卸载一例 + scope 外 dev 提示一例 + reset≠dispose 一例）；变异（dispose 不停轮询）→ 2 例红 |
| C1-H3 | High | 同 aux 矛盾类别 first-wins 保留可执行项 | 按 aux 分组：组内完全一致才去重留 1；任何矛盾（类别/证据/建议）→ 整组丢弃并按组内条目数计 dropped | 已修复：`pnpm test:unit app/api/normalizers/boardHealthTriage.test.ts --maxWorkers=2` → 14 例 PASS（一致重复留 1、矛盾类别 dropped=2、矛盾证据 dropped=2）；变异（矛盾不丢弃）→ 2 例红 |
| C1-M1 | Medium | 408 等超时类 4xx 被当"确定拒绝"解冻 | 新增 `isDefiniteRejection(status)`：无 status / 5xx / 408 / 499 → 结果未知（保留冻结）；其余 4xx → 确定拒绝；按状态码判定不解析文案 | 已修复：38 例 PASS（含 408 例） |
| C1-M2 | Medium | `retryTask` 无本地并发锁（双击两次 POST） | 新增 `retrying` 闸门，与 `submitting` 互斥 | 已修复：38 例 PASS（retry 双击 + retry↔submit 互斥） |
| C1-M3 | Medium | 前端"终态"≠后端"未结束互斥"（三失败态仍持锁） | 拆三集合：停止轮询集（含 unknown）/ 互斥释放集（completed·conflict·unavailable）/ 可重试集（三失败）；新确认只允许互斥已释放态 | 已修复：38 例 PASS（失败态只走 retry、未知计数、canonical ID 等） |
| C1-M4 | Medium | 迟到响应跨标签污染全局 ref | 迟到 POST 只维护**该标签**的冻结记录；`submitUncertain` 改为按当前标签冻结派生 | 已修复：38 例 PASS（切标签后旧响应不污染 + 切回可恢复未知意图） |
| C1-M5 | Medium | scope 外使用无生命周期兜底 | 见 C1-H2 生命周期段（自动 dispose / dev 提示 / 显式 dispose 契约） | 已修复：38 例 PASS |
| C1-L1..L6 | Low | unknown 未停轮询；reset 与冻结不同生同死；冻结键与解冻 ID 形态不一致；计数缺字段捏造 0；409 文案统一化；测试缺口与恒定 key 掩盖换键 | 逐条收口（unknown 停轮询+可恢复；submitUncertain 派生；`canonicalAuxiliaryId()`；total/processed/failed → nullable、NA 维持 0、remaining 维持 null；409 透传服务端 message；递增 key 工厂 + 17 条新故事） | 已修复：4 文件 76 例 PASS；`npx eslint` 0 问题；`pnpm exec nuxi typecheck` 本批零错误（余下报错属其它 change 脏文件） |

**措辞纠正**：审查推断"可重复建任务"的后果，本批不声称"已实证生产必双 embedding"——服务端同标签互斥与版本校验仍是兜底，客户端重复建任务是否落成两个任务取决于互斥是否已释放。确证的违约是**客户端幂等语义**（结果未知时静默换键重建、可能错误任务冒充），已按 implementation-contract §3 修复。

**边界**：本批未跑 build / 部署 / 浏览器 / 全量前端测试 / 后端门禁；3.2-3.3（组件）尚未开始；C1 的 UI 故事与真机验收仍待 tasks 5.3。

## Seam C1 复审放行 + C2 组件批（tasks 3.2 / 3.3）

### C1 独立复审（定点复核 3 High / 5 Medium / 6 Low）

- 复审基线：`/tmp/board-health-c1-fixed-review-OSCvEkR0`；报告：
  `…/outputs/a29dfeed-701a-4b41-bbf6-ff6f73b496bf/.pi/subagents/board-health-c1-independent-review.ce06fd10.md`。
- **判定：OK —— seam C1 / tasks 3.1 可放行**（仅本批；不替整个 change 放行：3.2/3.3、宿主接线、真机故事与后端门禁不在其范围）。
- manifest 比对确认修复只动 6 个文件（2 normalizer + 2 测试 + composable + 其测试），**未碰 HTTP 层/共享 client**；
  implementation-contract §3 未改，修复按原文执行。3 High / 5 Medium / 6 Low **全部真实关闭**
  （逐条结论见该报告；处置与证据见 `seam-c1-review-fixes.md`）。
- **措辞纠正已接受**：原「可重复建任务」的后果推断收窄——后端同标签互斥与 version 校验仍是服务端兜底，
  客户端是否落成两个任务取决于互斥是否已释放；**确证**违约是客户端幂等语义（结果未知时静默换键重建、可能错误任务冒充）。

### C1 复审新提 1 条 Low（随 C2 处理，不阻断 C1）

| 编号 | 级别 | 问题 | 作者处置 | 验证 |
| --- | --- | --- | --- | --- |
| C1-L7 | Low | composable 提示「…请先重发原内容，**或加载最新版本重新核对**」误导：加载最新版本**不解冻**，照此提示会再次撞同一拒绝；真实出口只有「先同键重发原内容，待服务端明确接受/拒绝后再改内容」 | ① 文案改为「提交结果尚未确认，请先重发原确认内容，待后台确认后再修改。」（不展示技术 key 值、不把刷新说成已撤销/确认未受理）；② C1 **最小扩展** `resendFrozen()`：显式「重发原确认内容」——严格用冻结的幂等键 + 冻结的 payload，用户当前草稿**不参与也不被清空**，与新确认**共用同一发送管线**（`postRepair`：互斥/epoch/生命周期守卫一致，不复制第二套提交逻辑）；③ 结果确定后才解冻，来源仍服务端，不自动新创建 | 已修复：C1 composable 用例 → **40 例**全 PASS（新增「草稿已变但显式重发仅旧 body 同 key 且草稿不丢」+「无冻结时重发直接拒绝」2 例）；C2 组件用例「提交结果尚未确认」断言只读保留原确认内容 + 「重发原确认内容」入口 + 同键同 body |

### C2 组件批（tasks 3.2 / 3.3）

| 编号 | 级别 | 内容 | 处置与验证 |
| --- | --- | --- | --- |
| C2-OK | — | 获批 v2 原型落成真实组件：紧凑主从工作面（顶部一行待核对/进展 + 左 240px 索引 + 右单项处理区），keep/insufficient 与评分统计趋势默认折叠但完整保留 | 已交付：`front/app/features/tags/components/console/board-health/`（Workspace 根 + WorkspaceHead + IssueIndex + IssueDetail + RepairEditor + RepairProgress + 纯派生 `boardHealthWorkspace.ts`；全部新组件 < 500 行 / < 15KB）；组件故事 **40 例**全 PASS（挂载**真实 C1 composable**，只 mock API 四资源） |
| C2-OK2 | — | 复用 C1 归一器 + 新增 `normalizeBoardHealthMagnetFlow`（服务端已算好的流向只搬运，不前端推 top1/可靠命中） | 已交付：`boardHealthTriage.ts` + 3 例精准单测（搬运 / topBoard 未知保持 null / 缺字段 null） |
| C2-OK3 | — | C1 最小扩展：`directBoards`/`activeTagCount` 改 nullable（**范围缺失 = 未知**，不谎称 0 范围） | 已交付：`auxiliaryRepair.ts` + 2 例（C1 归一器 20 例全 PASS） |
| C2-OK4 | — | 旧能力→承接：评分/趋势**复用既有 `BoardHealthSignal`**（零额外请求、零重复统计）；逐标签质量表/组合/候选仍由既有页签承载（不空占位、不复制实现） | 已交付：见 `seam-c2-workspace-handoff.md` §3「旧能力 → 承接」表（以 HEAD 真实入口为证据） |
| C2-OK5 | — | 3.4 接入契约（props/emits）与 3.5 缺口（真实几何/截图/双主题复核） | 已交付：handoff §4（`boardId` / `boardLabel` / `removedAuxIds` + `remove-composition` 事件签名）+ §6（窄屏断点已按 767.98px 落地，**JSDOM 不构成尺寸验收**） |

**边界**：本批**未接宿主**（`BoardWideWorkface` 的 health 页签仍渲染 `RailHealthBrief`）——未接宿主不等于整故事交付；
真机完整故事仍待 tasks 5.3 的 agent-browser；未启动服务/未构建/未部署/未跑全量测试。

## Seam C2 复审（tasks 3.2 / 3.3）——修复后放行

> 独立只读审查：快照 `/tmp/board-health-c2-review-B4ApztsQ`（17 文件；manifest 比对确认 `semanticBoards.ts`
> 哈希与 C1 一致 → **wire 未动**）。报告：
> `…/outputs/a29dfeed-701a-4b41-bbf6-ff6f73b496bf/.pi/subagents/board-health-c1-independent-review.779b5d47.md`。
> 判定：**修复后放行（4 Medium + 2 Low；无 High）**；3.4 宿主未接、3.5 几何未验按已知边界处理，不计缺陷。
> 逐条处置与证据见 `seam-c2-review-fixes.md`。

| 编号 | 级别 | 问题 | 作者处置 | 验证 |
| --- | --- | --- | --- | --- |
| C2-M1 | Medium | 降级轮被误标「历史快照」，`'degraded'` 分支真 wire 不可达（后端 `run.go:95`：降级轮 score/comment 均 NULL 且 `metrics.triage` 为 JSON null） | 降级判定**提前**到 `!triage.available` 之前；「历史快照」只留「有评分无 triage」；夹具改真 wire（`triage:null`），不再用 `triage:[]` 冒充降级 | 已修复：组件故事 51 例 PASS（新增「真 wire 降级轮」+「有评分无 triage」两例） |
| C2-M2 | Medium | 正常轮没有「本轮体检依据」折叠区（评分/统计/趋势不可达，违反 ui-design 与已批原型） | `showBasis` 改为「快照存在即渲染」（默认折叠），内容仍是既有 `BoardHealthSignal`（零额外请求） | 已修复：组件故事新增「正常轮也有依据折叠区」例 |
| C2-M3 | Medium | remove 项处理区无证据（原型对每项都渲染证据块） | 证据 `<details>` 移到四形态共用位置（`v-if="!task"`，有任务时由执行态折叠摘要承载，避免重复） | 已修复：组件故事新增「remove 项也能展开证据（磁铁/流向）再确认摘除」例 |
| C2-M4 | Medium | 非 409 的提交失败被吞（用户零反馈）；范围未知时确认按钮仍可点；行内确认静默关闭 | 根保存 `submitRepair()` 的 error → 独立错误行（服务端 message 透传）；失败时**不收起**行内确认；范围未知/未加载/读取失败 → 禁用确认并说明 | 已修复：组件故事新增「非 409 失败独立错误行 + 行内确认不静默关闭」「共享范围未知禁用确认并说明」两例 |
| C2-L1 | Low | 切项瞬间编辑器闪现上一项草稿 | 切项在 `await loadContext()` 前先 `setDraft('')`（同项重选仍保留草稿），`draftTouched` 语义不变 | 已修复：组件故事新增「切项先清草稿：不闪现上一项内容」例 |
| C2-L2 | Low | stale 用例未断言提示行（提示是否真出现未被拦） | 测试改为真正触发一次同版块刷新失败（新增 `defineExpose({ reload })` 供宿主/测试触发），断言 `bhw-stale-hint` 文案与旧快照仍在；顺带把读取失败判定改为「无旧快照才进错误态」（有旧快照按 stale 保留） | 已修复：组件故事「刷新失败保留上次快照（stale）：给出提示行且旧快照仍在」例 |

**顺带补的最小能力**：`defineExpose({ reload })`——同版块刷新体检快照（失败按 stale 处理，不创建任何任务）；
给 3.4 一个刷新入口，也让 stale 提示行成为可达状态。

**验证**：组件故事 3 文件 / **51 例**全 PASS（修复前 40 例）；C1 composable 40 例全 PASS；
C1 归一器 + 旧件回归 6 文件 / 69 例全 PASS；合并 10 文件 / **160 例**全 PASS；
`npx eslint`（本批目录）→ 0 问题；`pnpm exec nuxi typecheck` → 本批零错误（全局 20 处报错全属**其它 change 并行中的脏文件**，未触碰未修、不宣称全局绿）。
未跑 build / 部署 / 浏览器 / 全量测试 / 后端门禁。

**边界**：本批**未接宿主**（`BoardWideWorkface` 的 health 页签仍渲染 `RailHealthBrief`）——未接宿主不等于整故事交付；
真机完整故事仍待 tasks 5.3 的 agent-browser；3.5 几何/截图未验。

## Seam C2 定点复审放行 + Seam C3 宿主接线（tasks 3.4）

### C2 定点复审（run `4f444981`）

- 新基线 `/tmp/board-health-c2-fixed-review-XijARpUo`；manifest 比对：**仅 7 个文件变**
  （根/Detail/Editor/`boardHealthWorkspace.ts` + 3 测试），C1 归一器与 composable、Head/Index/Progress **哈希未动**
  → C1 行为与 wire 未被顺带改写 ✓。
- **判定：C2 放行**（原 4 Medium + 2 Low 全部关闭；新增 1 条 Low 边界说明）。
- 逐项关闭证据（M1 降级轮判定顺序 / M2 正常轮依据折叠 / M3 remove 证据 / M4 失败不吞 + 范围禁确认 /
  L1 切项清草稿 / L2 stale 提示行）见该报告与 `seam-c2-review-fixes.md`。

### C2 复审新提 Low（本批随 C3 修）

| 编号 | 级别 | 问题 | 作者处置 | 验证 |
| --- | --- | --- | --- | --- |
| C2-L7 | Low | `useBoardHealth` 不看状态码 → 版块已删（404/无权 403）带旧快照也被当 stale：说成「刷新失败，显示上次快照」，旧清单仍可操作 | `useBoardHealth` **透出 `errorStatus`**；工作面纯函数在 **404/403** 判新形态 `gone`（版块可能已不存在 → **不展示可操作清单**、**不再显示误导性 stale 文案**）；网络错/5xx 仍按 stale；写入侧不变（C1 实时 context + 版本校验） | 已修复：组件用例「404（版块可能已删除）：不再说「刷新失败显示上次快照」，旧清单不再可操作」+ 纯函数用例（404/403 → `gone`） |

### C3 宿主接线（tasks 3.4）

| 编号 | 内容 | 处置与验证 |
| --- | --- | --- |
| C3-OK | 工作面接进既有宿主 | `BoardWideWorkface` 的**体检依据页签改渲染 `BoardHealthWorkspace`**（原 `RailHealthBrief variant=full` 的完整依据收进工作面内默认折叠的「本轮体检依据」，复用 `BoardHealthSignal` 零额外请求）；新增 prop `removedAuxIds` + emit `remove-composition`；**构成/组合/候选/来源/设置页签原样保留**；**右轨 brief 不动**（轻结论，不塞清单），其「完整评语与依据」落点改到「体检依据」页签（获批原型同款） |
| C3-OK2 | 摘除走既有 DELETE 链 | 工作面 `remove-composition`（canonical 字符串）→ 壳层 `handleRemoveComposition` → `Number(auxId)` → 既有 `useBoardComposition.removeLabel` → `DELETE /semantic-boards/:boardId/composition/:auxId`；成功 notify 一次 + 重取构成 + 体检重取；失败 notifyError **一次**（壳层不叠加 toast）；**只有真实成功才回填 `removedAuxIds`**（失败不谎称已摘除） |
| C3-OK3 | 旧能力去向表 | 以 HEAD 真实入口为证据登记（评分/趋势 → 工作面折叠依据；质量表/组合/候选 → 既有页签；右轨 → 不动；摘除 → 既有链），见 `seam-c3-host-handoff.md` §3 |

**验证**：宿主 + 旧件 7 文件 / **60 例**全 PASS（新增 `BoardConsoleShellRepair.test.ts`：工作面 → 既有 DELETE → 成功才回填 / 失败不回填）；
C2 工作面 + C1 + 旧件回归 11 文件 / **168 例**全 PASS；`npx eslint`（本批目录 + 两个 composable）→ 0 问题；
`pnpm exec nuxi typecheck` → 本批改动行零错误（全局外部红 `BoardConsoleShell.vue:173 isNarrow` 属**另一 change** 在本批期间重写
`useConsoleShellOverlays.ts` 引入，与本批改动行 202/208/342/397/419/423 无关——未触碰未修，不宣称全局绿）。

**边界**：本批**未接真机**（未启服务/未构建/未部署/未跑浏览器）；3.5 几何/截图与 5.3 agent-browser 真后端故事仍待办；
右轨落点改动（`openWorkface('quality')` → `openWorkface('health')`）是本批唯一行为改动，回退只需改壳层一行。

## 4.1 正式评审（全 change，tasks 4.1）

> 评审：独立只读子线程（`zai-coding-cn/glm-5.3:max`），2026-10-29；报告原文
> `.pi/subagents/board-health-formal-review.md`。结论 **0 High / 1 Medium / 3 Low**，
> 判定「修 M1 后进 5.x 门禁」。本节记录 **M1 处置 + 三条 Low 归属 + 验证命令与结果**。
> 修复批次明细（逐文件）见 `review-4.1-fixes.md`。

### 4.1 台账

| 编号 | 级别 | 问题 | 处置 | 验证 |
| --- | --- | --- | --- | --- |
| **M1** | Medium | 组合（composite）构成标签进 triage 覆盖；其 rewrite 项前端 `actionable=true`，但描述修复链全程 `label_type='auxiliary'` 过滤 → repair-context 404 死路；wire 无 `label_type`，前端无从辨别原因 | **已修（5 项一并落地）**：① 后端 `metrics.labels[]` 逐条透传 `label_type`（`BoardHealthLabelMetrics.LabelType` + `loadCompositionLabels` 填充）；② 前端 normalizer 接管 `label_type`——非 auxiliary 的 rewrite 项 `actionable=false`，工作面渲染只读说明「组合标签暂不支持描述修复」（类型未知 → 「标签类型未知，无法执行描述修复」），**证据不隐藏、不当它健康**，且不拉必然 404 的修复上下文；③ remove 不受影响（走既有 DELETE 链；并修掉摘除侧只受理 `auxiliary` 的不对称 → 组合标签的构成关系现可摘除）；④ `specs/board-health-checkup` / `data-map` / `implementation-contract` 补实际语义；⑤ 补用例（rewrite 不可执行且无修复入口 / remove 仍可摘除 / 类型未知不猜） | 后端：`go test -p 1 -count=1 ./internal/tagmanagement/...` 13 包全 PASS；`golangci-lint run ./internal/tagmanagement/...` 0 issues。前端：6 文件 / **87 例**全 PASS；eslint 0 错误；typecheck 本批零错误 |
| **C3-L1**（评审 L2 同源措辞项除外） | Low | `removedAuxIds` 只增不减：同一标签被重新挂回后「已摘除」徽标仍在、摘除按钮仍灰着 | **已修**：`BoardWideWorkface` 计算 `effectiveRemovedAuxIds` = 会话摘除记录 ∩ **不在当前构成**（`labels` + `composites`，同源壳层 `compositionLabels`）→ 重挂后徽标自动撤销、按钮恢复；**会话记录本身不撤销**；**未动壳层**（`BoardConsoleShell.vue` 脏行未碰） | `BoardWideWorkface.test.ts` 新增用例（辅助标签重挂 + 组合挂载重挂两种）；宿主回归 `BoardConsoleShellRepair.test.ts` 2 例 PASS |
| **C3-L2 / 评审 L2** | Low | `matching_failed` 在 `failed=null` 时渲染「有 未知 项匹配失败」，措辞生硬 | **已修**：`boardHealthProgressText` 与 `BoardHealthRepairProgress` 统一改为「有匹配更新失败项（数量未知）」，不提具体数 | `boardHealthWorkspace.model.test.ts` 新增用例（断言含「数量未知」且不含「未知 项」） |
| **C3-L3** | Low | 工作面与右轨各持一个 `useBoardHealth` 实例 → 同一只读端点被请求两次 | **记录为已知行为（doc-only）**：两次都是只读幂等的同一 GET（端点 MUST NOT 写库、不触发体检），无副作用、无状态分歧；合并成一次需跨宿主/工作面的缓存共享（属另一 seam 的结构改动），本批不改 | `seam-c3-host-handoff.md` §8「C3-L3」已写明 |
| **评审 L1** | Low | `degraded` 形态无法区分「LLM 之前的历史快照」（两者 wire 同形：score/comment 均 null） | **记录不改**：存量快照均有 LLM，实际影响近零；真机若遇远古快照文案误导，再补文案兜底 | — |
| **评审 L3** | Low | `JobView` 对外含 `idempotency_key` / `expected_version`（单用户无风险、合同未禁） | **记录备查，不改** | — |

### 4.1 验证命令与结果

| # | 命令 | 结果 |
| --- | --- | --- |
| 1 | `cd backend-go && GOMAXPROCS=2 go test -p 1 -count=1 ./internal/tagmanagement/...` | **ok**：13 包全 PASS（boardhealth / auxlabel / handler / models / repository / board / core / sourcestats / auxrepair / merge / …） |
| 2 | `cd backend-go && golangci-lint run ./internal/tagmanagement/...` | **0 issues**（gofmt 只对本批改动文件，未做全局格式化） |
| 3 | `cd front && pnpm test:unit app/api/normalizers/boardHealthTriage.test.ts app/features/tags/components/console/board-health/boardHealthWorkspace.model.test.ts app/features/tags/components/console/board-health/BoardHealthWorkspace.test.ts app/features/tags/components/console/board-health/BoardHealthWorkspace.repair.test.ts app/features/tags/components/console/BoardWideWorkface.test.ts app/features/tags/components/console/BoardConsoleShellRepair.test.ts --maxWorkers=2` | **ok**：6 文件 / **87 例**全 PASS |
| 4 | `cd front && npx eslint app/api/semanticBoards.ts app/api/normalizers/boardHealthTriage.ts app/api/normalizers/boardHealthTriage.test.ts app/features/tags/components/console/board-health/ app/features/tags/components/console/BoardWideWorkface.test.ts` | **0 错误**（`.vue` 不在 eslint 配置覆盖范围 → 「no matching configuration」提示，非问题） |
| 5 | `cd front && pnpm exec nuxi typecheck` | 本批改动行**零错误**；全局仅剩 `app/features/tags/components/SignalReportView.test.ts` 7 处——属**另一 change** 的脏文件，未触碰未修，**不宣称全局绿** |

> 本批**未启服务 / 未构建 / 未部署 / 未跑浏览器 / 未跑全量测试**，**未做 Git 写操作**；
> 3.5 几何/截图与 5.3 真机故事仍待办。

## 最终审查待办

B1独立审查、后续B2/C的API/持久恢复/真实UI故事、文档与部署对账仍未完成；不得将本阶段账本当成最终放行。

## 5.1 后端门禁（2026-10-29）

- 受影响包 `GOMAXPROCS=2 go test -p 1 -count=1 ./internal/tagmanagement/... ./internal/app/...` 首跑两红：
  - `TestShutdownSequence_SummaryLineContract`（internal/app）——**本 change 回归**：B6/#15 关停汇总行加 `repair=` 字段，契约测试正则未跟上。由编排者修（测试侧两处：正则加 repair 组并把组号/下界纳入 total 校验；TC-21 补 `repairShutdownTimeout=10s` 默认+可覆写断言）。修后整包 PASS、lint 0 issues。
  - `TestSourceStatsPostgres`（sourcestats）——docker 高负载 SLOW SQL 抖动，复跑两次均 ok，非本 change（未触该包），不登记欠账。
- `go vet ./...` 0、`go build ./...` 0、`golangci-lint` 受影响范围 0 issues。tasks 5.1 已勾（证据行见 tasks.md）。
- 5.2 前端：`pnpm lint` 退出 0（8 条既有 warning 非本 change 文件）；`pnpm exec nuxi typecheck` 全局仅剩 `SignalReportView.test.ts` 7 处 error——属活跃 change `add-question-driven-news-research`（文件 mtime 10-29 00:58），本 change 文件零错。5.2 勾选待该外部红消除后完成。

## 6.x 文档（tasks 6.1 / 6.2 / 6.3）

> 2026-10-29 文档批（模型 `minimax-cn/MiniMax-M3.1-Flash:max`，用户指定）：**纯文档改动**——只改 `docs/reference/` 与本 change 的 `tasks.md`/`review.md`；未碰任何代码文件、未跑测试/build/浏览器/服务、**未做 Git 写操作**。

### 改动清单

| 任务 | 文件 | 改动 |
| --- | --- | --- |
| 6.1 | `docs/reference/flow/semantic-board.md` | 新增小节「版块体检三病分诊与共享描述修复（board-health-triage-advice）」（三病口径 / 体检纯读与人工修复隔离 / 共享影响面 / 任务状态机与恢复 / 旧快照与日报不改写 / composite 只读 / 端点与代码入口）；「业务约束与不变量」补 **24/25/26** 三条红线句；「代码入口」补 auxrepair 一行；「变更溯源」补本 change 一行（未归档，归档后由编排者补永久链接） |
| 6.2 | `docs/reference/api/semantic-boards.md` | 「端点汇总」补 4 行；新增「## 辅助标签描述修复（board-health-triage-advice）」节（四资源 / 任务字段集 / 状态与 stage 闭集 / 计数语义表 / 错误码一览 / 幂等键语义 / retry 语义 / 内部列不进 wire）；`GET /semantic-boards/:id/health-checkup` 响应补 `metrics.triage[]`、`metrics.board.magnet_flow`、`metrics.labels[].label_type/description/magnet/neighbors/desc_flags` 与降级轮说明 |
| 6.2 | `docs/reference/database/tables/semantic-labels.md` | 新增 §5.6 `auxiliary_description_repair_jobs`、§5.7 `auxiliary_description_repair_items`（字段/约束/索引/生命周期/计数不变量 + 无 DB 级 FK 的理由）；索引表补 5 条；ER 图补两实体与逻辑关系边；§5.1 补 label_type 必要说明一行 |
| 6.2 | `docs/reference/database/_index.md` | 完整表清单补两张表（业务表计数 58 → 60） |
| 6.3 | `docs/reference/configuration.md` | `ai_settings` 键表补 `board_health_magnet_gap`(0.08) / `board_health_magnet_min_failed`(5) 两行；新增小节「### 版块体检阈值（`ai_settings`，board-health-triage-advice）」（6 键默认值表 + 读取方 + 非法值回退 + 执行边界四句） |

### 对账要点（逐条）

1. **三病口径**（flow）＝ `specs/board-health-checkup/spec.md`「标签分诊信号计算」的三类确定性信号：磁铁（`all_mean − failed_mean ≥ board_health_magnet_gap` 且失败样本 ≥ `board_health_magnet_min_failed`）、最近邻（全库 active auxiliary top-3，排除自身）、description 事件性词根预筛；实现口径核自 `service/boardhealth/signals.go`（`Insufficient = AllSamples < MinSampleSize || FailedSamples < MagnetMinFailed`，`Suspicious` 用 `>=` 语义）。
2. **纯读边界**（flow）＝ 红线 20 原文 + `specs` 「分诊动作边界」：体检轮次/查询端点不写匹配结果表、不落扩充建议 pending、不改 description/embedding；摘除走既有 `DELETE /semantic-boards/:id/composition/:auxiliary_label_id`。
3. **composite 只读**（flow + api + database）＝ 4.1 M1 的实际语义：`metrics.labels[]` 逐条透传 `label_type`（`auxiliary`/`composite`，历史快照缺字段＝类型未知），描述修复链只覆盖 auxiliary；`remove` 项不受类型限制。
4. **四资源契约**（api）＝ `handler/auxiliary_description_repair_handler.go` 的 `RegisterAuxiliaryDescriptionRepairRoutes` 四条路径 + `repairErrorStatus` 的 400/404/409 映射 + `respondRepairInternalError` 的 500 sanitized（`internal server error`，DB 细节只进日志）；DTO 字段＝ `service/auxrepair/auxrepair.go` 的 `RepairContext`/`JobView`（含 `remaining` 由服务端算）；wire ID 为数字（前端归一为字符串视图）。
5. **幂等与重试**（api + flow）＝ `implementation-contract.md` §1：同键同载荷回同任务、同键异载荷 409、同标签未结束互斥（部分唯一索引 `uq_aux_desc_repair_jobs_label_active`）、三失败态等显式 retry 且只续匹配、conflict/unavailable 409、运行中/已完成 retry 为幂等 no-op。
6. **计数不变量**（api + database）＝ `total = processed + failed + not_applicable + remaining`；`not_applicable` 是失效跳过项（**不是成功**）、`total` 是创建时冻结值不缩小；来源＝ `models/auxiliary_description_repair.go` 注释 + `data-map.md` §3。
7. **表结构**（database）＝ `models/auxiliary_description_repair.go` 的列名/类型/gorm tag（含 `not_applicable` 为加列兼容而**无 NOT NULL**、内部列 `committed_version`/`committed_at`/`phase_token` 不进任何 DTO）+ 迁移 `database/auxiliary_description_repair_migration.go` `20261028_0001` 的部分唯一索引；**无 DB 级 FK** 及其删除策略理由照模型注释写入。
8. **配置无漂移**（configuration + flow）＝ `service/boardhealth/boardhealth.go` 的 `LoadConfig` 键名与 6 个默认常量逐项对齐；此前 flow 只列 4 个 `board_health_*` 键（缺两个磁铁键），本次在 flow 与 configuration 两处补全；另核 `alert_threshold` 为「严格小于才通知」、`member_candidates_top` 为手动扩充主路取数深度（快照固定持久化 top 50）。

### 门禁记录

| # | 命令 | 结果 |
| --- | --- | --- |
| 1 | `bash scripts/harness/check-standards.sh` | **通过 256 / 失败 2**：A/B/C/D/E/G/H/I 段全绿，G 段导航层死链为空；两处失败均在 F 段 doc-impact——① `add-question-driven-news-research`（**另一 change**，非本批）；② `board-health-triage-advice`：`backend-go/internal/app/runtime.go`（B6 生产接线）命中 architecture 启发式，属 **tasks 7.5** 待办（本批未改声明、未动 architecture 文档，故保持 FAIL 不掩盖） |
| 2 | `bash scripts/harness/doc-impact.sh verify openspec/changes/board-health-triage-advice` | **1 FAIL**（`疑似遗漏: 改了 architecture 相关代码未声明 architecture`），与 6.x 文档改动无关，改动前后一致 |

> **未决**：`bash scripts/check-standards.sh` 实际路径是 `scripts/harness/check-standards.sh`（tasks 6.3/7.5 文案里的短路径需编排者订正）；architecture 启发式命中的处置（补 `architecture` 域声明 + 更新 `docs/reference/architecture/` 或加 `<!-- doc-impact-excuse: domain=architecture -->`）留给 7.5 裁定。

## 5.2 / 7.2 前端门禁（2026-10-29）

- 外部阻塞解除：另一 change `add-question-driven-news-research` 的 `SignalReportView.test.ts` 7 处类型错（TS18048/TS2532）经用户授权由本会话代修——4 处非空断言（sections[0]!、items[1]!、links[0]!×4、plain[0]!），纯测试类型守卫、零逻辑改动，均有 toHaveLength 前置保证。该文件 `pnpm test:unit SignalReportView.test.ts --maxWorkers=2` → 37 例全 PASS。
- 门禁：typecheck 退出 0 零 error；lint 退出 0；受影响单测（BoardHealthPanel/BoardWideWorkface/BoardContextRail + 全部新增 API/normalizer/composable/board-health 文件）190 例全 PASS。tasks 5.2/7.2 已勾。
