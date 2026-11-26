# Seam B1 交接 — 描述修复持久存储底座（tasks 2.1）

> board-health-triage-advice seam B 第一个小批次。仅交付任务 2.1：持久模型 + 固定范围冻结 + 幂等/互斥存储 + 安全迁移注册。**不含** 匹配执行 worker、HTTP handler、前端、service/auxrepair 编排、原子保存链（2.2-2.6 均未实现）。不宣称整个修复链已交付。run model=zai-coding-cn/glm-5.3:max（用户指定）。

## 1. 新增文件与注册点

| 文件 | 内容 |
| --- | --- |
| `backend-go/internal/tagmanagement/models/auxiliary_description_repair.go` | Job/Item 两个 GORM 模型 + status/stage/item 常量 + 终态清单 |
| `backend-go/internal/tagmanagement/repository/auxiliary_description_repair.go` | 存取层：创建事务、幂等/互斥、版本令牌、三规则校验、context/查询 |
| `backend-go/internal/tagmanagement/repository/auxiliary_description_repair_unit_test.go` | 纯函数单测（无 DB） |
| `backend-go/internal/tagmanagement/repository/auxiliary_description_repair_test.go` | testcontainer PG 集成测试（13 个用例全绿） |
| `backend-go/internal/platform/database/auxiliary_description_repair_migration.go` | 迁移 20261028_0001（部分唯一互斥索引） |

注册点（各 2 行增量，未动他人行）：

- `tagmanagement/wire.go` init `RegisterModels` 增 `AuxiliaryDescriptionRepairItem/Job`（models 包不 import platform/database，防环纪律保持）。
- `platform/database/postgres_migrations.go` 末尾 append 增 `auxiliaryDescriptionRepairMigration()`。

## 2. 表 / 索引 / 迁移（实际形态）

**表 `auxiliary_description_repair_jobs`**（AutoMigrate 建）：
`id, auxiliary_label_id, idempotency_key(size64), expected_version(size64), old_description(text), new_description(text), stage(size16,default:''), status(size20,default:pending), description_committed(bool,default:false), total, processed, failed, error(text), claim_owner(size64,*), claimed_at(*), lease_expires_at(*), created_at, updated_at`。JSON 键与 implementation-contract §1 任务 data 完全一致（id/auxiliary_label_id/idempotency_key/expected_version/old_description/new_description/stage/status/description_committed/total/processed/failed/error/created_at/updated_at）；claim/lease 三列是为 2.5 安全执行**预留**的空列（B1 恒 nil，不写）。

**表 `auxiliary_description_repair_items`**（AutoMigrate 建）：`id(uint64), job_id, topic_tag_id, status(size16,default:pending), error(text), created_at, updated_at`。

**索引**：
- `uq_aux_desc_repair_jobs_key`：idempotency_key 全列唯一（model tag，AutoMigrate 建）→ 同 key 第二次插入必撞。
- `uq_aux_desc_repair_items_job_tag`：(job_id, topic_tag_id) 复合唯一（model tag）→ 冻结集合不可重复。
- `idx_aux_desc_repair_jobs_label` / `idx_aux_desc_repair_jobs_status`：普通索引（latest-task/状态查询）。
- `uq_aux_desc_repair_jobs_label_active`：**部分唯一互斥索引**，`ON (auxiliary_label_id) WHERE status NOT IN ('completed','conflict','unavailable')` —— 由迁移 **20261028_0001** 建（GORM tag 表达不了 NOT IN 谓词，同 20260717_0001 board_upgrade 先例）。终态=completed|conflict|unavailable（与 data-map §2 状态图一致）；embedding_failed/save_failed/matching_failed **非终态**，仍持有互斥（含可重试失败阶段）。迁移幂等（IF NOT EXISTS），表不存在时 no-op（tableExists 守卫——其他包测试二进制的 golden schema 没有这两张表，迁移不会炸；已实测）。

**FK / 删除策略（交接必读）**：两张表**刻意不加任何 DB 外键**（对 semantic_labels、topic_tags 均无）：
- 加 FK 会挡既有删除路径（aux 标签 GC delete、topic_tags 删除级联向量=红线12）→ 回归；
- CASCADE 会在删标签时静默吞掉修复历史；SET NULL 会抹掉冻结 ID；
- 现行为：删除不被阻挡（PG 用例实证），item 行保留陈旧 ID，total 不变；后续 seam 用**开放的 status 字符串**记录「不再适用」（无需 DDL，B1 未实现该记录逻辑，也未宣称匹配完成策略）。
理由已写进模型文件与迁移文件注释。

## 3. 存储层导出（签名）

```go
// repository/auxiliary_description_repair.go
const MaxRepairDescriptionRunes = 500
func ValidateRepairDescription(label, description string) (string, error) // 三规则：trim / 1..500 runes / trimmed != label；返回 trim 后文案（入库形态）
func AuxiliaryVersionToken(l *models.SemanticLabel) string // "v1_"+sha256(id|label|description|status|updated_at UTC nanos)[:20hex]，共43字符（"v1_" 3 + 40 hex；本行原写 44 是文档笔误，seam B2 已校正）

type CreateRepairJobParams struct{ AuxiliaryLabelID uint; IdempotencyKey, ExpectedVersion, NewDescription string }
type RepairContextBoard struct{ ID uint; Label, Status string }
type AuxiliaryDescriptionRepairContext struct{ Label models.SemanticLabel; Version string; DirectBoards []RepairContextBoard; ActiveTagCount int64; LatestTask *tagmodels.AuxiliaryDescriptionRepairJob }
type AuxiliaryDescriptionRepairRepository struct{...}
func NewAuxiliaryDescriptionRepairRepository(db *gorm.DB) *AuxiliaryDescriptionRepairRepository
func (r) CreateRepairJob(ctx, params) (*tagmodels.AuxiliaryDescriptionRepairJob, error)
func (r) GetRepairContext(ctx, auxiliaryLabelID uint) (*AuxiliaryDescriptionRepairContext, error)  // 零写
func (r) GetRepairJob(ctx, auxiliaryLabelID, jobID uint) (*Job, error)   // 属主校验：跨标签→包 gorm.ErrRecordNotFound
func (r) GetLatestRepairJob(ctx, auxiliaryLabelID uint) (*Job, error)   // 无→nil（context.latest_task 源）
```

**错误类型**（sentinel，`errors.Is` 判定；2.2 handler 映射 4xx/409/202 用）：
`ErrRepairLabelNotFound`（不存在或非 auxiliary）、`ErrRepairLabelNotActive`（disabled）、`ErrRepairVersionConflict`、`ErrRepairIdempotencyConflict`（同 key 异 payload）、`ErrRepairJobAlreadyActive`（同 aux 未结束互斥，含 job id/status 详情）、`ErrRepairIdempotencyKeyEmpty`、`ErrRepairDescriptionEmpty` / `ErrRepairDescriptionTooLong` / `ErrRepairDescriptionRepeatsLabel`（三规则各一，供就地反馈）。互斥错误为 `%w` 包装带上下文，errors.Is 仍可用。

## 4. 创建事务语义（顺序即契约）

`CreateRepairJob` 单事务内：
1. `SELECT ... FOR UPDATE` 锁 auxiliary 行（label_type='auxiliary'，否则 ErrRepairLabelNotFound）；
2. **幂等先行**：按 idempotency_key 查已存 job——payload（aux id + expected_version + trim 后描述）全等 → 返回**已存 job**（即使标签后来已改/已禁用，先以已存请求判断幂等）；不等 → ErrRepairIdempotencyConflict；
3. status != active → ErrRepairLabelNotActive；
4. 三规则校验（不添加任何符号黑名单；纯分隔符合法）；
5. 版本令牌 vs 锁定行现算值，不等 → ErrRepairVersionConflict；
6. 互斥预检（status NOT IN 终态）+ 插入撞部分唯一索引 → 23505 后置裁决（重读已提交赢家：同 key 同 payload 回放/异 payload 冲突/否则互斥错误）；
7. 冻结范围 = **全部 active topic_tags**（谓词 `(status='active' OR status='' OR status IS NULL)`，与 ListActiveTags 同口径），含完全未归属者；**不**枚举 topic_tag_board_labels、不限当前版块；
8. 插入 job + 批量 items（500/批），事务原子；**只写这两张表**——不碰 label 描述/两向量/匹配表/升级建议/快照（用例断言零写）。

并发正确性靠 PG：标签行锁串行同标签创建；跨标签同 key 靠 23505 裁决（专用并发用例覆盖）。**没有进程内 map**；换新 repo 实例仍能查到任务（用例覆盖）。

版本令牌覆盖 id/label/description/status/updated_at（UTC 纳秒）；服务端生成、服务端比较，前端只透传。

## 5. 已执行验证（本机实跑，backend-go 目录）

1. `GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepair|TestValidateRepairDescription|TestAuxiliaryVersionToken' -v ./internal/tagmanagement/repository` → **ok 17.3s**，13/13 PASS（含真实 testcontainer PG：建表/三索引/23505 直插互斥与复合唯一/冻结含未归属与 legacy '' 状态/merged 排除/后来新增不入范围/同 key 同 payload 同 job/异 payload 冲突/标签改后原请求重放/禁用后重放可用新 key 拒/pending 与 matching_failed 互斥、三终态放行/同标签并发一胜一互斥/跨标签同 key 并发一胜一冲突（23505 裁决路径）/CHECK 注入致 items 失败→job+items 全回滚、故障消失后可重建/删 tag、删 label 均不被挡且历史保留/属主查询 404、latest_task、新实例查询、context 令牌与版块列表与 active 计数、GET 零写零任务）。
2. `GOMAXPROCS=2 go test -count=1 -p 1 ./internal/tagmanagement/repository` → **ok 9.7s**（同包既有 board_upgrade/tag_job_queue/topic_tag_default 测试共存全绿——init 注册未伤邻）。
3. `GOMAXPROCS=2 go test -count=1 -p 1 -short ./internal/tagmanagement/...` → 全 ok（change-scope 域档命令）。
4. `GOMAXPROCS=2 go test -count=1 -p 1 -short ./internal/platform/database` → ok；`-run 'TestModelTagConstraints_MaterializedInDB' -v`（单跑，含我迁移在链上、表缺席 no-op 路径）→ PASS 33.4s；`-run 'TestBoardSignalMigrationIdempotent'` → PASS。
5. `golangci-lint run ./internal/tagmanagement/models/... ./internal/tagmanagement/repository/... ./internal/platform/database/...` → **0 issues**；`go vet`（同三包）→ ok；`go build ./...` → ok。
6. 无 Git 写操作（零 stash/commit）；无关文件零改动（含 board_health_migration_test.go 格式噪声未动）。

**无关红测试（非本 change 引起，仅报告）**：`go test -run 'TestModelTagConstraints_MaterializedInDB|TestBoardSignalMigrationIdempotent' ./internal/platform/database` 组合跑时前者挂：迁移 20260922_0001（board-signal，他人 change）在 ReimportTestDB 重建时 `DROP CONSTRAINT uq_board_signal_candidate_id_owner` 报 SQLSTATE 2BP01（被依赖对象挡住）。**已用证据排除本 change 责任**：临时注销我的迁移后组合跑同样红（随后原样恢复注册，grep 复核=1）；单独跑前者在本迁移在链时亦 PASS。属 board-signal 迁移与 BoardSignal 测试残留索引的既有互扰，建议父线程登记台账（test-patrol --register）或转告 board-signal 属主。

## 6. 未实现（B2 及以后，勿当已交付）

- 2.2 纯读 context/任务查询 **handler** 与提交校验端点（repo 层已备好 GetRepairContext/GetRepairJob/GetLatestRepairJob + 三规则 + 令牌，service 可直接消费）；**→ 已由 seam B2 交付，见 seam-b2-api-handoff.md**
- 2.3 事务外 embed + 事务内锁行原子保存 description+embedding（本批只存 job，绝不写 label）；
- 2.4 匹配执行（InvalidateMatchCache + MatchTopicTag 逐 item 推进——item 的 succeeded/failed/「不再适用」写入 API 未加，属执行 seam）；
- 2.5 claim/lease/重启恢复 worker（列已预留，逻辑未写）；
- 2.6 handler/routes/facade/启动接线；seam C 前端全部。
- job 的 status/stage/counters 推进方法本批**有意未加**（属执行 seam），当前只有创建路径会写 job 行。

## 7. 给后续 seam 的两条注意事项

- **测试注册**：需要这两张表的测试二进制若 import 不了 tagmanagement root（root→repository 环），沿用本批模式：测试文件 `init()` 里 `database.RegisterModels(&tagmodels.AuxiliaryDescriptionRepairJob{}, &tagmodels.AuxiliaryDescriptionRepairItem{})`（见 auxiliary_description_repair_test.go 头注释，先例 semantic_board_upgrade_test.go）。testutil 的 golden-schema 镜像清单**不含**这两表（该文件不在本批白名单，未改）；迁移因此做了 tableExists no-op 守卫，其他包 PG 测试不受影响（已实测）。
- **幂等先行顺序**是契约语义不是实现细节：重放判定必须先于 active/版本/三规则检查，否则「标签后来已更新时同 key 同 payload 返回同一 job」会被版本门误杀（本批 PG 用例已锁死该顺序）。
