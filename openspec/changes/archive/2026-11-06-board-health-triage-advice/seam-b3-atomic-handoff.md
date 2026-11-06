# Seam B3 交接 — 描述修复执行阶段：事务外 embedding + 事务内原子保存（tasks 2.3）

> board-health-triage-advice seam B 第三个小批次。仅交付任务 2.3：**事务外生成 storage embedding + 事务内锁行原子保存 description+embedding+任务提交阶段**。
> **不含**：2.4 匹配执行、2.5 claim/heartbeat/重启恢复的完整流程、2.6 生产接线、前端（seam C）。不宣称整条修复链已交付。
> 模型 `minimax-cn/MiniMax-M3.1-Flash:max`（用户指定）；承接 seam-b2-api-handoff.md（2.2 已交付、ID 数字 wire 已由父线程裁定）。

## 1. 新增/改动文件（backend-go/internal/ 下）

| 文件 | 状态 | 内容 |
| --- | --- | --- |
| `tagmanagement/service/auxrepair/embedding.go` | 新增 | 执行阶段编排：可注入 embedder / cache-invalidator / clock（variadic Option）、owner/lease 前置校验、事务外单次 storage embedding、事务内原子保存的调用、阶段可分辨错误、commit 后 cache 失效 |
| `tagmanagement/service/auxrepair/embedding_pg_test.go` | 新增 | 临时 PG + 桩 embedder 回归（7 个用例 / 10 个子用例）：成功原子提交、embedding 失败、保存事务回滚、版本变化冲突、disabled/删除/非aux 不可用、owner 缺失/过期/被替换拒提交、已提交重入零调用 |
| `tagmanagement/repository/auxiliary_description_repair_stage.go` | 新增 | 执行阶段 SQL/事务层：claim CAS、owner-guarded 阶段推进、失败/终态标记、`CommitDescription` 原子提交（锁 job + 锁 aux 行 + 复核 + 同事务写三处） |
| `tagmanagement/models/auxiliary_description_repair.go` | 改动（仅新增 2 个内部字段） | `CommittedVersion *string` / `CommittedAt *time.Time`（nullable、`json:"-"`、不进任何 HTTP DTO）。既有字段一字未改 |
| `tagmanagement/service/auxrepair/auxrepair.go` | 改动（结构体 + 构造） | `Service` 增加 `embedder` / `invalidateCache` / `now` 三个协作者与默认值；`NewService(db, opts ...Option)` —— **`NewService(db)` 旧调用保持兼容** |
| `tagmanagement/service/service.go` | 改动（仅追加 facade 段） | `AuxRepairOption` 类型别名 + `WithAuxRepairEmbedder/WithAuxRepairCacheInvalidator/WithAuxRepairClock`；既有脏改（board archive / weak evidence）逐字保留 |

**B3原子保存功能未接线或改变HTTP契约**：routes.go / wire.go / app 启动、迁移与旧表、boardhealth / A2、前端均未改。此轮另外实施的B2审查修复确有handler/其测试及repo p2测试变更，详见seam-b2-review-fixes.md；不是「handler一字未动」。`CreateJob` 仍只入队，不执行 embedding。

**新列的上线方式**：`committed_version` / `committed_at` 是 **nullable** 列，由生产启动路径的 `AutoMigrate`（`tagmanagement/wire.go` 的 `database.RegisterModels` 已注册该模型）自动补齐——不需要显式迁移，也没有 NOT NULL 约束需要迁移兜底（code-style.md「显式迁移管的表不写 not null」）。

## 2. 实际入口与签名

### 2.1 service 层（`service/auxrepair`）

```go
// 构造（Option 全部可选；旧调用 NewService(db) 仍有效）
func NewService(db *gorm.DB, opts ...Option) (*Service, error)
type Option func(*Service)
func WithEmbedder(e Embedder) Option                       // 默认 auxlabel.DefaultAuxiliaryLabelEmbedder
func WithCacheInvalidator(fn func()) Option                // 默认 board.InvalidateMatchCache
func WithClock(fn func() time.Time) Option                 // 默认 time.Now
type Embedder = auxlabel.AuxiliaryLabelEmbedder            // func(ctx, input string, mode) (pgVector string, []float64, error)

// 前置原语（2.5 的最小底座，不是 2.5 的完整流程）
func (s *Service) ClaimJobForExecution(ctx, jobID uint, owner string, lease time.Duration) (*JobView, error)

// 执行阶段入口
func (s *Service) ExecuteEmbeddingPhase(ctx, jobID uint, owner string) (*JobView, error)
```

`owner` 是**内部执行者标识**（如 `executor-1`）：是 service 方法参数，**不进任何 HTTP DTO**；job 上的 `claim_owner/claimed_at/lease_expires_at` 也不在 `JobView` 里（2.2 的 handler 测试已断言 `claim_owner` 不出现在响应中）。

### 2.2 repository 层（`repository/auxiliary_description_repair_stage.go`）

```go
var ErrRepairOwnerRequired      = errors.New("repair executor owner is required")
var ErrRepairJobNotClaimable    = errors.New("repair job cannot be claimed by this executor")
var ErrRepairJobOwnerLost       = errors.New("repair job is not owned by this executor")
var ErrRepairJobAlreadyCommitted = errors.New("repair job already committed the confirmed description")

func (r *AuxiliaryDescriptionRepairRepository) ClaimRepairJob(ctx, jobID uint, owner string, leaseUntil time.Time) (*Job, error)
func (r *AuxiliaryDescriptionRepairRepository) GetRepairJobByID(ctx, jobID uint) (*Job, error)
func (r *AuxiliaryDescriptionRepairRepository) GetLiveAuxiliaryLabel(ctx, auxiliaryLabelID uint) (*models.SemanticLabel, error)
func (r *AuxiliaryDescriptionRepairRepository) MarkRepairEmbeddingStarted(ctx, jobID uint, owner string) error
func (r *AuxiliaryDescriptionRepairRepository) MarkRepairSavingStarted(ctx, jobID uint, owner string) error
func (r *AuxiliaryDescriptionRepairRepository) MarkRepairFailure(ctx, jobID uint, owner, status, message string) error
func (r *AuxiliaryDescriptionRepairRepository) MarkRepairTerminal(ctx, jobID uint, owner, status, message string) error

type CommitDescriptionParams struct { JobID uint; Owner string; Description string; Embedding string }
func (r *AuxiliaryDescriptionRepairRepository) CommitDescription(ctx, params CommitDescriptionParams) (*Job, error)
```

**守卫模型（写链的全部安全边界）**：每一次写都是 `WHERE id=? AND claim_owner=? AND lease_expires_at IS NOT NULL AND lease_expires_at > now() AND description_committed=false` 的 CAS；**租约过期以数据库时钟 `now()` 判定**（应用时钟只用于 lease 计算与前置快检）。`RowsAffected=0` → `ErrRepairJobOwnerLost` → 调用方**不得再写**。

`ClaimRepairJob` 的 CAS 允许：未 claim、同 owner 续 claim、租约已过期后接管；**终态 job 不可 claim**。claim 只管「所有权」，不管阶段——已 committed 的 job 也能被 claim（2.4 的匹配阶段正需要这一点），是否允许在某阶段运行由该阶段自己判。

## 3. 执行阶段的时序与阶段/状态

```
pre-check（零 AI）  claim/owner/lease → label 存在且是 auxiliary → active → 版本 == 用户确认的版本
                    ↓ 任一不满足：终态 conflict/unavailable（独立事务写状态），不做 AI 调用
stage=embedding     owner-guarded CAS（status=embedding）
embed（无事务）     1 次 storage embedding：input = label + ": " + trim(新描述)，mode=storage
                    ↓ 失败：status=embedding_failed（旧值不动，可重试）
stage=saving        owner-guarded CAS（status=saving）
commit（单事务）    锁 job 行 → 锁 aux 行 → 复核 owner/lease + active/auxiliary + 版本
                    → 同事务写：semantic_labels.description / embedding
                                job.description_committed=true / stage=matching / status=matching
                                job.committed_version（新令牌）/ committed_at（DB 时钟）
                    ↓ 业务冲突 → 事务回滚（状态不被回滚带走，见 §4）
cache 失效          提交成功之后（事务之外）调用 board.InvalidateMatchCache
```

- 阶段推进：`pending → embedding → saving → matching`；**本批不把 job 置 completed**，计数仍来自冻结范围（items 未动）。
- 保存成功后 job 停在 `stage=matching / status=matching / description_committed=true` → **2.4/2.5 可据此识别「从 matching 续，不倒退、不重嵌、不重存」**。

## 4. 故障窗与状态语义（已 PG 验证）

| 故障窗 | 行为 | 任务状态 | 标签 |
| --- | --- | --- | --- |
| embedding 失败（provider 报错/空向量） | 旧描述与旧向量不动 | `embedding_failed`（非终态，仍持互斥） | 未改 |
| 保存事务内失败（DB 错误/注入 CHECK 约束） | 整个事务回滚：描述、向量、提交标记全部不落 | `save_failed` | 未改 |
| 生成期间标签被编辑（版本变化） | 拒绝覆盖，新值保留 | `conflict`（终态，释放互斥） | 保留**别人改过**的描述与原向量 |
| 生成期间标签被 disabled | 拒绝写入 | `unavailable`（终态） | 未改（fixture 自己的 disable 除外） |
| 生成期间标签被删除 | 拒绝写入（任务行保留，无 FK） | `unavailable`（终态） | 仍是删除态 |
| 生成期间标签被改成非 auxiliary | 拒绝写入 | `unavailable`（终态） | 未改 |
| owner 缺失 / 租约过期 / owner 被替换 | **零 embedding 调用、零写入**，不踩掉新 owner | 不变（仍 pending） | 未改 |
| 已 committed 的 job 重入该阶段 | 不再 embed、不再保存、阶段不倒退 | 仍 `matching` | 未改 |

**「事务里返回业务冲突会把状态一起回滚」的陷阱已处理**：冲突/不可用是在**提交事务回滚之后**用**独立事务**写进 job 的（`MarkRepairTerminal`）。PG 用例断言：conflict/unavailable 的状态**确实落库**（可被查到），同时标签保持当前原值。DB/ctx 已不可写时，`markTerminal/markFailure` 返回 false，service 返回的错误会明说「the terminal/failure state could not be persisted」——**不伪报失败已持久**。

## 5. AI 调用纪律

- 默认 embedder = `auxlabel.DefaultAuxiliaryLabelEmbedder(ctx, input, auxlabel.AuxiliaryLabelEmbeddingModeStorage)`：内部 `airouter.NewRouter().Embed`，operation=`tagmanagement.auxlabel_embedding`、metadata.operation=`auxiliary_label_storage_embedding` → 走既有 airouter 日志链（ai-logging.md 要求），**无直打 HTTP、无额外 LLM 调用**。
- 一次执行 = **一次** storage embedding 调用（桩 embedder 计数断言 = 1）。
- **不**重算 merge 向量、**不**调 `EnsureVectorDimension*`、**不**先清空旧向量再补（失败时旧向量原样保留）。
- 构造 / `GetRepairContext` / `CreateJob` **绝不触发 AI**（本批新增入口只有 `ExecuteEmbeddingPhase` 会调 embedder）。
- 输入形态：`label + ": " + trim(用户确认的新描述)`（用例断言 `"上海: 中国东部沿海的直辖市"`）。

## 6. 只写该写的东西

- `semantic_labels`：仅 `description` + `embedding`（storage）。**`label` / `slug` / `aliases` / `merge_embedding` / `status` / `display_order` 等一律不动**（用例逐项断言）。
- `auxiliary_description_repair_jobs`：`description_committed` / `stage` / `status` / `committed_version` / `committed_at`（+ 阶段推进时的 `error`）。
- **不写**：`topic_tag_board_labels`（匹配表）、`board_upgrade_suggestions`、`board_health_snapshots`、日报、其它标签、items（冻结范围由 2.4 推进）。用例对匹配行/升级建议/旧快照做了计数与内容断言。

## 7. 本批实跑验证（backend-go，串行，GOMAXPROCS=2 / -p 1）

1. `go test -count=1 -p 1 -v ./internal/tagmanagement/service/auxrepair` → **ok 7.9s**，7/7 PASS（10 个子用例全绿，桩 embedder + 真实 testcontainer PG）。逐例：
   - `TestAuxiliaryDescriptionRepairExecuteCommitsAtomically`：1 次 storage 调用、input/mode 断言、只有 description+embedding 变、label/aliases/merge_embedding/slug 不变、job committed+stage/status=matching+committed_version 新于确认版本、cache 失效**恰一次且发生在提交之后**（回调里读到新描述与 committed=true）、匹配/建议/快照/冻结范围不变、**embedding 期间 `pg_stat_activity` 无 idle-in-transaction**（证明生成在事务外）。
   - `TestAuxiliaryDescriptionRepairExecuteEmbeddingFailureKeepsOldValues`：embedding_failed；旧描述/旧向量/merge 向量不动；无 cache 失效。
   - `TestAuxiliaryDescriptionRepairExecuteSaveFailureRollsBack`：注入 CHECK 约束让保存事务失败 → 整体回滚（旧描述/旧向量保持）、job=save_failed、未提交、无 cache 失效。
   - `TestAuxiliaryDescriptionRepairExecuteConflictOnVersionChange`：生成期间改描述 → conflict 落库、标签保留别人改过的描述与原向量。
   - `TestAuxiliaryDescriptionRepairExecuteUnavailableOnDisabledOrDeletedLabel`（disabled / deleted / 非aux 三个子例）：unavailable 落库、标签未被写入。
   - `TestAuxiliaryDescriptionRepairExecuteRefusesWithoutLiveOwnership`（缺失 / 过期 / 被替换三个子例）：ErrStageOwnerLost、**embed 调用数 = 0**、零写入、owner 被替换时新 owner 的 claim_owner 不被踩掉。
   - `TestAuxiliaryDescriptionRepairExecuteSkipsAlreadyCommittedJob`：重入零新调用、阶段不倒退。
2. 受影响包回归：`go test -count=1 -p 1 ./internal/tagmanagement/repository` → ok 21.1s；`go test -count=1 -p 1 ./internal/tagmanagement/handler` → ok 17.6s（B2 的三端点用例仍绿）；`go test -count=1 -p 1 -short ./internal/tagmanagement/...` → 全 ok。
3. `golangci-lint run ./internal/tagmanagement/...` → **0 issues**；`go vet ./internal/tagmanagement/...` → ok；`go build ./...` → ok。

**未跑**（按纪律刻意不跑）：全量 `go test ./...`；原批命令的另行逐条复跑（上面的影响包整包回归包含既有B1/B2用例，不等于这些用例未重跑）；外部 board-signal 组合用例（已登记欠账）；任何真库 / 环境 DSN / `psql`；未启 dev 服务、未部署、未派生子线程。

## 8. 指纹（backend-go/internal/ 下）

| 文件 | SHA256 |
| --- | --- |
| tagmanagement/service/auxrepair/embedding.go | e8c4996ed29af3173c062797d11cfd5ae02ecd2a3cb6dac0877b8d7392187038 |
| tagmanagement/service/auxrepair/embedding_pg_test.go | 6e944f6d4be4c4e84b1e991072a7885b9334d6aa34b67770e2647b6f39b81383 |
| tagmanagement/service/auxrepair/auxrepair.go | 558d24447b715a9328364ece72648788c6a30eb6399e9017c017331fe7c47123 |
| tagmanagement/repository/auxiliary_description_repair_stage.go | c85af27024c3c4250eb4b46544170f967dac9d14ca6a93b09b2ea3e4b2291b9c |
| tagmanagement/models/auxiliary_description_repair.go | 7bb62e86d3d97992f088428cdcc88a22aef1c28d9bf30c274e6b7549a792b27e |
| tagmanagement/service/service.go（含外部既有脏改） | 88b4790d8007c959f1c462552f0e7037d431946da5bd76a57aab7a8236ac081c |

> 指纹为成稿时的磁盘状态；续做时以 `sha256sum` 复核为准。

## 9. 2.4 / 2.5 还缺什么（交接必读）

**2.4 匹配执行（未做）**
- 用既有 `MatchTopicTag` 处理**冻结范围内全部 active topic_tags**（含无匹配行的未归属者），逐项写 items 的 succeeded/failed，计数（processed/failed/total）由服务端单源推进；部分失败不得标全完成。
- **匹配执行前必须再次失效 board 匹配缓存**（board cache 无 TTL；本批已在保存成功后失效一次，但 2.4 匹配前与重试时都要再失效）。
- 匹配范围内后来 disabled/删除的 tag 要明确记「不再适用」，不得让任务永久卡死。
- 匹配完成后 job 才能进 `completed`（本批不置 completed）。

**2.5 claim/heartbeat/重启恢复（未做）**
- 本批只给了**底层原语**：`ClaimRepairJob`（CAS claim）、owner-guarded 阶段写、owner/lease 校验。**完整的 claim/heartbeat/lease 续期/重启恢复流程没有实现**——没有 worker、没有 goroutine、没有定时任务、没有重试端点。
- 2.5 需要补：worker 循环（领取 pending/失败态任务）、heartbeat 续租、lease 过期后的接管、进程重启后从 `stage`/`status` 恢复（`description_committed=true` → **不得重嵌/重存**，直接续 matching）、失败任务的安全重试（只补未完成项）。
- 本批的状态机已为此留好位置：`claim_owner/claimed_at/lease_expires_at` 三列现在真的被写入与校验；`stage` 停在 `matching` 即「描述已提交、等匹配」。

**2.6 生产接线（未做）**
- `RegisterAuxiliaryDescriptionRepairRoutes` 仍未接进 `tagmanagement/routes.go` / `app/router.go`；执行阶段**没有** HTTP 触发端点（刻意不加假 retry/execute 路由）。因此这三个端点与执行入口**当前线上都不可达**——这是批次边界，不是遗漏。
- 前端零改动（seam C）。

## 10. 给后续 seam 的注意事项

1. **ID 类型**：沿用父线程裁定——HTTP wire 保持数字，前端 API normalizer 转 string；本批未改任何既有 DTO。
2. **版本令牌**：`AuxiliaryVersionToken` 产出 43 字符（`v1_`+40 hex），不透明、不可解码；`committed_version` 存的是**保存之后**的新令牌（内部字段，不进 DTO）。
3. **ai-logging 登记**（文档欠账，归 tasks 6.x）：本批复用既有 operation `tagmanagement.auxlabel_embedding`（已带 airouter 日志），但 `standard/backend/ai-logging.md` 的「已接入 AI 调用的功能清单」尚未补「辅助标签描述修复 storage embedding」一行——文档阶段补上。
4. **不要把本批的方法与 PG 用例当成完整 UI 故事**：任务链的真实用户故事（确认→三阶段→部分失败恢复→完成→下一项）仍需 seam C 的前端与 5.3 的 agent-browser 落点。
