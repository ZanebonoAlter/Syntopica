# Seam B4 交接 — 描述修复匹配执行与逐项持久进度（tasks 2.4）

> board-health-triage-advice seam B 第四个小批次。仅交付任务 2.4：**用既有匹配引擎处理冻结范围、逐项持久化结果与进度、计数口径（processed/failed/not_applicable/remaining）**。
> **不含**：2.5 完整 dispatcher/heartbeat/重启恢复、2.6 生产接线、前端（seam C）。不宣称整条修复链已交付。
> 模型 `minimax-cn/MiniMax-M3.1-Flash:max`（用户指定）；承接 seam-b3-atomic-handoff.md（B3 已交付，其 review 四项 R 的修复见 seam-b3-review-fixes.md）。

## 1. 新增/改动文件（backend-go/internal/ 下）

| 文件 | 状态 | 内容 |
| --- | --- | --- |
| `tagmanagement/service/auxrepair/matching.go` | **新增** | 匹配阶段编排：`ExecuteMatchingPhase(ctx, jobID, owner)`、阶段执行权领取、逐项「适用性判定 → 引擎调用 → 结果落库」、阶段可分辨错误、每次运行前失效 board cache |
| `tagmanagement/service/auxrepair/matching_pg_test.go` | **新增** | 临时 PG 回归（10 例 / 12 子例），含**真实既有引擎**的一条持久匹配用例 |
| `tagmanagement/repository/auxiliary_description_repair_matching.go` | **新增** | 匹配阶段 SQL/事务层：冻结范围读取、适用性判定（复用冻结谓词）、逐项结果 + 计数同事务落库、阶段收尾（completed / matching_failed）与执行权释放 |
| `tagmanagement/service/auxrepair/auxrepair.go` | 改动 | `Service` 增 `db` 与 `matcher`（默认 = 既有引擎，绑定本 service 的显式 DB）；`JobView` 增 `not_applicable` / `remaining`；`NewService(db, opts...)` 保持旧调用兼容 |
| `tagmanagement/models/auxiliary_description_repair.go` | 改动 | 新增 item 状态 `not_applicable`、job 计数列 `not_applicable`（nullable + default 0，**不加旧表 NOT NULL**） |
| `tagmanagement/service/service.go` | 改动（**仅追加** facade 段） | `WithAuxRepairMatcher`（可注入匹配引擎）；既有脏改逐字保留 |

**未改动**：handler / routes.go / wire.go / app 启动、迁移与旧表、boardhealth / A2、前端、匹配算法与阈值、`board` 包本身（只**调用**既有引擎）、全局 `repository.Repo`。

## 2. 实际入口与签名

```go
// service/auxrepair
type Matcher interface {
    MatchTopicTag(ctx context.Context, topicTagID uint) ([]board.SemanticBoardMatchResult, error)
}
func WithMatcher(m Matcher) Option                      // 默认 board.NewSemanticBoardMatchingService(db)
func (s *Service) ExecuteMatchingPhase(ctx context.Context, jobID uint, owner string) (*JobView, error)

// repository（auxiliary_description_repair_matching.go）
func (r *...) ListRepairScopeItems(ctx, jobID uint) ([]tagmodels.AuxiliaryDescriptionRepairItem, error) // 仅 pending/failed
func (r *...) GetScopeTagState(ctx, topicTagID uint) (RepairScopeTagState, error)                     // Exists/Active（复用冻结谓词）
func (r *...) RecordMatchingItemResult(ctx, outcome RepairItemOutcome) (*RepairCounters, error)       // item + counters 同一短事务
func (r *...) FinishMatchingPhase(ctx, jobID uint, owner, token string) (*Job, *RepairCounters, error)
```

**默认 matcher 是既有引擎**：`board.NewSemanticBoardMatchingService(db)` 显式绑定本 service 的 DB（**不落全局 Repo**、不改匹配阈值、不复制算法）。构造 `NewService` **不执行** matcher 也不执行 AI。

## 3. 执行语义

```
前置（零引擎调用）  owner+实时租约 → description_committed=true → 任务未完成
                    ↓ 任一不满足：拒绝（busy / owner lost / not committed / already completed）
领取阶段执行权      原子 CAS（与 B3 同一底座）：写新 token、status=matching、清空旧 error
失效 board cache    每次运行（含恢复/重试）前都失效（cache 无 TTL）
逐项（只取冻结 items 的 pending/failed）
  适用性判定        标签已删除 / disabled / merged（谓词与冻结范围同一份）→ 持久 not_applicable
                    → 不调引擎、不计成功、不计失败、不缩 total
  引擎调用          既有 MatchTopicTag；nil error = 成功（**合法零命中也是成功**）
                    error → 再查一次 DB：确认已不适用 → not_applicable；否则 → failed（继续兄弟项）
  结果落库          item 行 + job 计数**同一短事务**，owner+token+实时租约守卫，RowsAffected 检查
阶段收尾            计数聚合：remaining=0 且 failed=0 → completed（并清空 error）；有失败 → matching_failed
                    结束即释放 claim（owner 守卫，不动别人的 claim）与阶段执行权
```

- **只处理冻结 items**：`succeeded` 与 `not_applicable` 是终态，永不重跑；冻结后新增的 active tag **不入旧范围**（沿既有实时匹配路径）。
- **活跃谓词复用**：`activeTopicTagsPredicate`（`active` / legacy `''` / `NULL`；排 disabled/merged）与冻结范围同一份，未另造条件。
- **计数单源**：`processed/failed/not_applicable` 由**冻结 items 聚合**（每次落库在同事务内重算），不是内存自增；`remaining = total - processed - failed - not_applicable` 由服务端算出并随 DTO 给出，四者之和恒等于 total。
- **每项最多一份结果**：item 更新只作用于 `pending/failed`；已终态的 item 不会被改写 → 失败重试成功后 `failed` 减少、`processed` 增加，累计不重复。
- **不重复 embed/保存**：匹配阶段不调用 embedder；已 committed 的任务重入不重嵌不重存。
- **不改别的**：标签描述/别名/storage/merge 向量、升级建议、旧快照、日报全部不动；匹配表由**既有引擎**按其自身语义写（这正是本阶段的目的）；不自动体检。

## 4. DTO 字段变化（继承 B2 DTO，默认兼容）

`JobView` 新增两个字段，既有字段语义不变：

| 字段 | 含义 |
| --- | --- |
| `not_applicable` | 处理时确认已失效/删除而**持久跳过**的项数——**不是成功**；旧任务读作 0 |
| `remaining` | `total - processed - failed - not_applicable`（服务端算）；旧任务按同一持久计数计算 |

信封、字段命名（snake_case）、ID 数字 wire 均不变；前端按既有归一器消费（前端 normalizer 转 string 的裁定不变）。

## 5. 本批实跑验证（backend-go，串行，GOMAXPROCS=2 / -p 1）

`GOMAXPROCS=2 go test -count=1 -p 1 -v ./internal/tagmanagement/service/auxrepair` → **ok 15.0s，22/22 PASS**（B3 12 例 + B4 10 例；12 个子例组）。**计数更正（2026-10-29，按现源码核对）**：原文「21/21 = 13 + 8」与源码不符——`embedding_pg_test.go` 12 个测试函数、`matching_pg_test.go` 10 个，合计 22；未为数字重跑任何批次。B4 逐例：

| 用例 | 断言要点 |
| --- | --- |
| `...MatchingRunsWholeFrozenScope` | 冻结范围内**每一项**都进了引擎（含未归属者；legacy `''` 仍适用）；冻结后新增的 tag 不入范围；processed=3、failed=0、NA=0、remaining=0、status=completed；四数之和=total；每次运行前 cache 被失效 |
| `...MatchingMarksNotApplicable` | 删除/disabled/merged 三项持久记 `not_applicable`，**引擎只被调用 1 次**（仅存活项）；processed=1、NA=3、remaining=0、**total 不缩小**；全项有结论 → completed |
| `...MatchingWithRealEngine` | **真实既有引擎**（`board.NewSemanticBoardMatchingService(db)`，未注入桩）跑通并**持久化一条 direct_hit 挂载**（`topic_tag_board_labels` 行存在，match_reason=direct_hit）；任务 completed |
| `...MatchingPartialFailureAndRetry` | 一项失败**不打断兄弟项**（引擎被调用 3 次）；processed=2、failed=1、remaining=0 → status=`matching_failed`（**绝不 completed**）；**新 service 实例**（同 DB）重试只碰失败项（引擎再调用 1 次）→ failed=0、processed=3、累计不重复 → completed |
| `...MatchingRefusesWithoutLiveOwnership`（3 子例） | 缺 owner / 租约过期 → **零引擎调用**、状态不变；调用中 owner 被替换 → 拒绝状态提交：items 仍 pending、计数全 0、status 不变、**新 owner 的 claim_owner 不被踩掉** |
| `...MatchingRequiresCommittedDescription` | 描述未提交 → 拒绝，**零引擎调用** |
| `...MatchingEmptyScopeCompletes` | 零范围 → completed（无项可匹配不是失败） |
| `...MatchingDoesNotReembed` | 匹配阶段**不新增 embedding 调用**（保存阶段 1 次 → 匹配阶段仍 1 次）；描述与向量保持已提交值 |
| `...MatchingLeavesNeighboursAlone` | 标签描述/两向量/别名、升级建议、旧快照全部不动 |
| `...MatchingVanishedTagBecomesNotApplicable` | 引擎调用中标签消失且 DB 确认 → 该项记 `not_applicable`（不吞掉普通引擎失败） |

门禁：`golangci-lint run ./internal/tagmanagement/...` → **0 issues**；`go vet ./internal/tagmanagement/...` → ok；`go build ./...` → ok；`go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepair' ./internal/tagmanagement/handler` → ok（B2 DTO 用例仍绿）。

**未跑**（按纪律刻意不跑）：全量 `go test ./...`；B1/B2/B3 原批整套复跑；外部 board-signal 组合用例（已登记欠账）；真库/环境 DSN/psql；dev 起停/部署；未派生任何子线程。

## 6. 指纹（backend-go/internal/ 下，成稿时磁盘状态）

| 文件 | SHA256 |
| --- | --- |
| tagmanagement/service/auxrepair/matching.go | a6904656da8d7d30012512b5f5d44e823293e8825b1ba607eb572d8f44734cb3 |
| tagmanagement/service/auxrepair/matching_pg_test.go | fb4530ebdb235e2d0823bad9beaeff6a2470e43f11ba3fd3952aff60858d217f |
| tagmanagement/service/auxrepair/auxrepair.go | 1771ba28b0fe59dfc940252b1f5bacf01cc5b276206eef8d79fc554209294db6 |
| tagmanagement/repository/auxiliary_description_repair_matching.go | ea6cbbcf3ea66e67d812224ce1bb348ecce0579e05fb8696553073fd0106c8d4 |
| tagmanagement/repository/auxiliary_description_repair_stage.go（B3 修复后） | 7e6df3a3f2edc804433e3daa51af5677de78c674d6f33719ef60202a94ac1faf |
| tagmanagement/models/auxiliary_description_repair.go | 5b0686896b675955c1e3147723d6f580a617a2d26e6248c82294d47b8e016b20 |
| tagmanagement/service/service.go（含外部既有脏改） | e507061d2fb4abfdcba36573b43cfcefabfecefa48c1ef93a72b9ad0803edcbe |

> 续做时以 `sha256sum` 复核为准。

## 7. 2.5 还缺什么（交接必读）

- **dispatcher / worker 循环**：本批**没有**任何 goroutine、定时任务或后台领取；`ExecuteMatchingPhase` 只能被显式调用。
- **heartbeat / lease 续期**：租约靠调用方在 claim 时给定时长；没有续租心跳，长任务需要调用方保证时长足够（2.5 补）。
- **重启恢复**：没有「扫描未完成任务并续跑」的机制；恢复入口已具备（阶段执行权 + 冻结 items + 计数），但**谁来调用它**属 2.5。
- **失败任务的安全重试**：重试语义已通（只补 failed/未完成项、不重嵌不重存），但**没有**重试端点/定时器，也没有「运行中重试不得重复 worker」的调度保证。
- **claim 的完整语义**：接管作废旧 token、租约实时判定已就位；claim 扫描/超时清理/多执行者仲裁仍属 2.5。
- **honest 边界**：引擎调用在途不可中断——进程崩溃后该项保持 pending，重启后**至少一次**重放（引擎是替换式写入，幂等）；本批不宣称 exactly-once。

## 8. 给后续 seam 的注意事项

1. **数据隔离红线**：该修复链会重算**所在库全部 active tags** 的匹配——命名空间样本**不是**隔离手段。真实交互故事必须落在一次性 PG + 对应测试 API；不得把「批准实现」当作「批准在真库自动修复真实标签」。
2. **计数口径**：`not_applicable` 永远不是成功；`remaining` 由服务端算，前端不得自算或把缺字段当 0（见 data-map「计数字段来源」）。
3. **cache 失效**：本批在每次匹配运行前失效一次；2.5 的恢复/重试调用同一入口即自动再失效。
