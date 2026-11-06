# B2 独立 review 发现修复记录（tasks 2.2 复验）

> 2026-10-29。B2 独立 review（只读审不可变快照 `/tmp/board-health-b2-review-bKcdnuTb`，未改任何文件、未跑测试）结论 **BLOCK**：1 项 Medium + 2 项 Low。三项均属原合同范围，**无需用户补批**；按父线程指示：先暂撤 tasks 2.2 的完成勾并在 review.md 记录阻断，修复并专项复验后再恢复。
> 修复者：单写者（`minimax-cn/MiniMax-M3.1-Flash:max`）。写入范围仅限：本次新 handler 及其测试、repo p2 测试、review.md、tasks 2.2 勾选、本 handoff 与 seam-b2-api-handoff.md 的纠正记录。**未接生产 routes/wire、未部署、未改其它域。**

## 1. 发现 → 处置对照

| 编号 | 级别 | review 发现 | 处置 | 复验 |
| --- | --- | --- | --- | --- |
| R-1 | Medium | 新 handler 的 POST 用 `ShouldBindJSON`，Gin decoder 只 Decode 一次；有效对象后追加第二个对象或非空垃圾仍会创建任务（202 + 落 job/items），违背「非法 JSON→400」契约 | **仅该端点**改为「一次对象 Decode + 第二次必须 io.EOF」：新增 `decodeSingleJSONObject(body io.Reader, target any) error`（`json.Decoder` 解一个对象后，第二次 Decode 必须返回 `io.EOF`；尾随**空白仍合法**）。**不改全局 Gin 绑定器**，其它端点的绑定行为一字未动 | `TestAuxiliaryDescriptionRepairCreateRejectsTrailingJSON` → PASS（第二对象 / 尾随垃圾 / 尾随 `}` 三个子例均 400，且 job 与 items **零新增**；尾随空白仍 202） |
| R-2 | Low | 复用的 `parseUintParam` 接受完整 uint64，而 PG 的 label/job id 列是 signed bigint；越界 id 被驱动拒绝后落成 **500** 而不是 4xx | **仅这三个新端点**改用 `parsePositiveBigIntParam(c, name)`：正整数且 ≤ `math.MaxInt64`，否则 400。**不改共享 `parseUintParam`**、不改任何旧接口的类型或行为 | `TestAuxiliaryDescriptionRepairRejectsOutOfRangeIDs` → PASS（MaxInt64 → 404（合法但不存在）；MaxInt64+1 与 MaxUint64 → 400，覆盖 context / create / get 的 `:id` 与 `:job_id`；**无任务写入**） |
| R-3 | Low | `repository/auxiliary_description_repair_p2_test.go:92` 拿 `tag.ID`（topic_tags 的 id）比 `job.AuxiliaryLabelID`（semantic_labels 的 id），两表序列碰巧相等才通过，且没有证明冻结项存的是正确的 tag | 断言改为：job 属主与 **`label.ID`** 比；**夹具先种一个占位辅助标签**，使 `tag.ID ≠ label.ID`（并断言这一点，杜绝巧合）；再查持久 items 断言 **`TopicTagID == tag.ID`** | `TestAuxiliaryDescriptionRepairReplayAfterLabelHardDelete` → PASS（该用例单跑；未重跑 B1 整套） |

## 2. 精确验证命令与结果（串行，GOMAXPROCS=2 / -p 1）

| # | 命令 | 结果 |
| --- | --- | --- |
| 1 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepairCreateRejectsTrailingJSON\|TestAuxiliaryDescriptionRepairRejectsOutOfRangeIDs' -v ./internal/tagmanagement/handler` | **ok 20.5s**，2/2 PASS（4 个子例） |
| 2 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepair' -v ./internal/tagmanagement/handler` | **ok 6.0s**，8/8 PASS（12 个子例）——B2 既有语义（context 字段 / 纯读零任务 / disabled 可读 / 404 / 三规则 / 499-500 / 幂等重放 / 409 冲突与互斥 / 属主越界 / 硬删重放 / 500 不外泄）**未被修复破坏** |
| 3 | `cd backend-go && GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepairReplayAfterLabelHardDelete' -v ./internal/tagmanagement/repository` | **ok 11.3s**，PASS（仅该精确回归，未重跑 B1 整套） |
| 4 | `cd backend-go && GOMAXPROCS=2 golangci-lint run ./internal/tagmanagement/handler/... ./internal/tagmanagement/repository/...` | **0 issues** |
| 5 | `cd backend-go && GOMAXPROCS=2 go vet ./internal/tagmanagement/handler/ ./internal/tagmanagement/repository/` | ok |

**未跑**（按指示刻意不跑）：B2 已通过的整包复跑、外部门禁、全量 `go test ./...`、B1 整套、真库/环境 DSN/psql、dev 起停与部署。

## 3. 改动文件与指纹（backend-go/internal/ 下）

| 文件 | 改动 | SHA256 |
| --- | --- | --- |
| `tagmanagement/handler/auxiliary_description_repair_handler.go` | 新增 `decodeSingleJSONObject` + `parsePositiveBigIntParam`，三个端点改用之 | b71234172fad5a0bd328366e0b393be71d71779821e122bd9348fccb00fe87b0 |
| `tagmanagement/handler/auxiliary_description_repair_handler_test.go` | 新增两个用例（尾随 JSON 三子例、越界 id 覆盖四个位置） | 0760b8949f7b6f4f750cbae8f0f4730ed5e41f385bd94063938a06c439038ba9 |
| `tagmanagement/repository/auxiliary_description_repair_p2_test.go` | R-3 断言与夹具修正 | 59799f05a02e68d48fd2673698fa9bd3846d22392fd39ea628910b2f321d7083 |

## 4. 状态

- **tasks 2.2 的完成勾已恢复**（修复 + 专项复验全绿，见 tasks.md）。
- B2 接缝的其余结论不变：三项 P2（B-P2-1/2/3）修复与证据见 `seam-b2-api-handoff.md`；`review.md` 的 B-P2 表已带验证命令。
- **2.3 随后已交付**，见 `seam-b3-atomic-handoff.md`；2.4 / 2.5 / 2.6 与前端仍未完成。
- 本记录只解除 B2 接缝的 BLOCK，**不替整个 change 最终放行**（review.md「最终审查待办」不变）。
