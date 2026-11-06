# Seam B2 交接 — 描述修复 API 接缝与三项 P2 修复（tasks 2.2）

> board-health-triage-advice seam B 第二个小批次。仅交付任务 2.2：三个 HTTP 资源（纯读 context / 创建任务 / 纯读任务查询）+ B1 独立审查提出的 3 项 P2 的**真正修复与验证**。**不含** 原子保存（2.3）、匹配执行（2.4）、claim/恢复/重试（2.5）、生产接线与前端（2.6 / seam C）。不宣称整条修复链已交付。
> 承接检查点：seam-b2-api-checkpoint.md（GLM 额度阻塞前的落盘状态）；模型 `minimax-cn/MiniMax-M3.1-Flash:max`（用户指定）。

## 1. 新增/改动文件（backend-go/internal/ 下）

| 文件 | 状态 | 内容 |
| --- | --- | --- |
| `tagmanagement/handler/auxiliary_description_repair_handler.go` | 新增 | 三个端点的 handler + 错误→状态码映射表（只绑参数/错误/DTO，不写 SQL） |
| `tagmanagement/handler/auxiliary_description_repair_handler_test.go` | 新增 | HTTP 契约回归（testcontainer PG）：字段形态/纯读/空态/disabled/坏 JSON/非法 ID/三规则/长键/冲突/越界 job/删除后重放/未知错误 500 不外泄 |
| `tagmanagement/repository/auxiliary_description_repair_p2_test.go` | 新增 | B-P2-1（硬删后重放/异 payload/新 key）与 B-P2-3（key/version 容量边界、无新任务）的 PG 回归 |
| `platform/database/auxiliary_description_repair_migration_test.go` | 新增 | B-P2-2（无表记迁移 → 建表 → 仍补索引 → 重复幂等）的临时 PG 回归 |
| `tagmanagement/service/service.go` | 改动（**仅追加** facade 段） | `AuxRepairService/AuxRepairContext/AuxRepairJobView/AuxRepairCreateRequest` 类型别名 + `NewAuxRepairService` 构造导出；文件内既有脏改（board archive / weak evidence，另一 change）**逐字保留未动** |
| `tagmanagement/repository/auxiliary_description_repair.go` | 未改（B1 落盘代码已满足 B-P2-1/P2-3 语义） | 幂等预判定已在活体锁行之前；key/version 容量校验已在事务之前 |
| `platform/database/auxiliary_description_repair_migration.go`、`platform/database/migrator.go` | 未改（B1 落盘代码已满足 B-P2-2） | 自愈 hook 与原迁移共用 `ensureAuxiliaryDescriptionRepairMutexIndex` |

**未接线**：本批**没有**改 `tagmanagement/routes.go` / `wire.go` / `app/router.go`——生产路由注册属任务 2.6。`RegisterAuxiliaryDescriptionRepairRoutes(rg, svc)` 是可注入 service 的独立注册函数，测试直接调用它。

## 2. 三个 HTTP 资源（实际形态）

前缀 `/api`，沿用 `{"success":bool,"data":...}` / `{"success":false,"error":"..."}` 信封；`:id` 沿用现有 `/auxiliary-labels` 组的通配名（与既有 `/:id/disable` 同节点），任务子资源用 `:job_id`。

### 2.1 `GET /api/auxiliary-labels/:id/description-repair-context`（纯读）

```json
{"success":true,"data":{
  "auxiliary_label":{"id":1,"label":"上海","description":"上海知名商业综合体","status":"active","version":"v1_<40 hex>"},
  "direct_boards":[{"id":2,"label":"中国地理","status":"active"}],
  "active_tag_count":2,
  "latest_task":null}}
```

- `version` = 服务端不透明令牌（`v1_` + 40 hex = **43 字符**；B1 报告的 44 是笔误，已校正），客户端只透传、不构造、不解码。
- `direct_boards` 恒为数组（无引用版块时为 `[]`）；引用它的版块含 disabled（带 status 展示）。
- `active_tag_count` = active topic_tag 总体数（与冻结范围同一谓词）。
- `latest_task` = 最新任务 DTO；无任务为 `null`；来源标签删除后历史任务仍可查。
- **零写**：不创建任务、不改标签/向量/匹配。
- 不返回向量与 claim/lease 私有列。

### 2.2 `POST /api/auxiliary-labels/:id/description-repairs`（202）

body `{"description":"...","expected_version":"v1_...","idempotency_key":"..."}` → `202` + 任务 DTO：

```json
{"success":true,"data":{"id":1,"auxiliary_label_id":1,"idempotency_key":"k1","expected_version":"v1_...",
 "old_description":"...","new_description":"...","stage":"","status":"pending","description_committed":false,
 "total":2,"processed":0,"failed":0,"error":"","created_at":"...","updated_at":"..."}}
```

- 本批**只落 job/items**：不改标签描述、不改 storage/merge 向量、不写匹配表、不跑 embed、不跑匹配。
- 幂等由**已存请求**判定（storage 层单源）：同 key + 同 payload → 返回同一 job（202，job id 相同）；同 key + 异 payload → 409；同标签已有未结束任务（异 key）→ 409。
- handler **不预先查询活体标签**：预查会误杀「标签后来被删/被停用后的合法重放」（B-P2-1）。

### 2.3 `GET /api/auxiliary-labels/:id/description-repairs/:job_id`（纯读）

`200` + 同一任务 DTO；job 属于别的标签 → 404（属主校验，不跨标签泄漏）；不存在 → 404；来源标签已删除 → 历史 job 仍 200。

### 2.4 错误映射（handler 单表，语义仍来自 storage sentinel）

| 情形 | 状态码 | 响应 error |
| --- | --- | --- |
| 非法 JSON / 缺 body / id 非正整数 / job_id 非正整数 | 400 | `invalid request body` / `invalid id` |
| `idempotency_key` 空 | 400 | `idempotency_key is required` |
| `idempotency_key` > 64 字符 | 400 | `idempotency_key must be at most 64 characters` |
| `expected_version` 缺失或 > 64 字符 | 400 | `expected_version must be a non-empty string of at most 64 characters` |
| 描述为空 / 超 500 字符 / 仅重复标签名 | 400 | `description must not be empty` / `... at most 500 characters` / `... must not only repeat the label` |
| 标签不存在或非 auxiliary | 404 | `auxiliary label not found` |
| job 不存在或属主越界 | 404 | `repair job not found` |
| 标签 disabled | 409 | `auxiliary label is disabled` |
| 来源版本已变 | 409 | `auxiliary label has changed, reload and confirm again` |
| 同 key 异 payload | 409 | `idempotency key was already used with a different request` |
| 同标签已有未结束任务 | 409 | `an unfinished description-repair job already exists ... (job N, status X)` |
| 其它（存储故障等） | 500 | `internal server error`（原始 DB 明细只进日志 `logging.Errorf`） |

三描述规则、版本、幂等、互斥的**判定公式只有一份**（`repository/auxiliary_description_repair.go`）；handler 只做状态码映射，service 不复算。

## 3. 导出的新符号

```go
// handler 包
func RegisterAuxiliaryDescriptionRepairRoutes(rg *gin.RouterGroup, svc *service.AuxRepairService)

// service 门面（service/service.go 新增段，handler 不直接 import service/auxrepair）
type (
    AuxRepairService       = auxrepair.Service
    AuxRepairContext       = auxrepair.RepairContext
    AuxRepairJobView       = auxrepair.JobView
    AuxRepairCreateRequest = auxrepair.CreateJobRequest
)
var NewAuxRepairService = auxrepair.NewService   // func(*gorm.DB) (*auxrepair.Service, error)

// service/auxrepair（B1 落盘，本批复核后沿用）
func NewService(db *gorm.DB) (*Service, error)
func (s *Service) GetRepairContext(ctx, auxiliaryLabelID uint) (*RepairContext, error)
func (s *Service) CreateJob(ctx, auxiliaryLabelID uint, req CreateJobRequest) (*JobView, error)
func (s *Service) GetJob(ctx, auxiliaryLabelID, jobID uint) (*JobView, error)
```

`NewService` 显式收 DB、nil DB 直接报错（绝不回落全局连接）；service 内无内存态、无 goroutine。

## 4. 三项 P2 的修复与证据

| 编号 | 修法 | 证据（命令见 §5） |
| --- | --- | --- |
| B-P2-1 | 幂等判定前置到活体锁行**之前**（`checkStoredRequest` 锁前/锁后各判一次；23505 仍走原「读赢家」裁决：同 key 同 payload 回放 / 异 payload 冲突 / 否则互斥）；DB 互斥索引与行锁都保留 | repository：硬删标签后同 key 同 payload 回同一 job、异 payload→`ErrRepairIdempotencyConflict`、新 key→`ErrRepairLabelNotFound`、任务数仍 1、冻结范围保留；HTTP：删除后重放仍 202 且 job id 相同、异 payload 409、新 key 404 |
| B-P2-2 | `ensureAuxiliaryDescriptionRepairMutexIndex`（`CREATE UNIQUE INDEX IF NOT EXISTS`）由迁移 20261028_0001 的 Up **与** `RunAutoMigrate` 成功后的自愈 hook 共用；hook 位置＝成功 AutoMigrate 之后、无关 discovery 补全之前（补全报错不得掩盖自愈）；只补本 change 的新 job 索引；不改全局 InitDB 的 AutoMigrate 非致命失败政策；不动其它历史迁移；不清理历史任务 | 临时 PG：无表→迁移 no-op 且版本已记录→建表→`RunAutoMigrate` 仍补出索引→重复自愈幂等（索引恒 1 条）且断言 partial unique + 排除三个终态 |
| B-P2-3 | key/version 容量在 service/handler 入参侧先拒（空 / >64 字符 / 版本缺失 / 版本 >64 字符 → 400），描述三规则同样 400；DB 22001 不再出现；未知错误 500 且不外泄明细 | repository：64 字符键被接受、65 字符键/空键/版本缺失/版本 65 字符各命中对应 sentinel、任务数仍 1；HTTP：400 分支齐、无新任务；存储故障→500 且响应体只有 `internal server error` |

## 5. 本批实跑验证（backend-go 目录，串行）

1. `GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepairReplayAfterLabelHardDelete|TestAuxiliaryDescriptionRepairRequestBoundaryRejections' -v ./internal/tagmanagement/repository` → **ok 17.4s**，2/2 PASS（含真实 testcontainer PG）。
2. `GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepair' -v ./internal/tagmanagement/handler` → **ok 17.5s**，6/6 PASS（context/create/500-字符/job 归属/硬删重放/未知错误 500）。**2026-10-29 review 修复后复跑：ok 6.0s，8/8 PASS（12 子例，新增尾随 JSON 与越界 id 两组，见 §9）**。
3. `GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestAuxiliaryDescriptionRepairMutexIndexSelfHeal' -v ./internal/platform/database` → **ok 18.3s**，PASS。
4. 受影响包回归：`go test -count=1 -p 1 ./internal/tagmanagement/repository` → ok 11.2s；`go test -count=1 -p 1 ./internal/tagmanagement/handler` → ok 14.4s；`go test -count=1 -p 1 -short ./internal/tagmanagement/...` → 全 ok；`go test -count=1 -p 1 -short ./internal/platform/database` → ok。
5. `golangci-lint run ./internal/tagmanagement/handler/... ./internal/tagmanagement/repository/... ./internal/tagmanagement/service/... ./internal/platform/database/...` → **0 issues**；`go vet`（同范围）→ ok；`go build ./...` → ok。

**未跑**（按纪律刻意不跑）：全量 `go test ./...`；未另行单独执行B1原批命令；但上文§5-4的本批受影响repository整包回归包含既有B1用例，不能理解为这些用例完全没有重跑；外部 board-signal 组合迁移互扰用例（已登记欠账，`test-patrol --register` 状态 open，不修不重跑）；任何真库 / 环境 DSN / `psql` 命令。

## 6. 指纹（backend-go/internal/ 下，仅作续做参照）

| 文件 | SHA256 |
| --- | --- |
| tagmanagement/handler/auxiliary_description_repair_handler.go | b71234172fad5a0bd328366e0b393be71d71779821e122bd9348fccb00fe87b0（review 修复后） |
| tagmanagement/handler/auxiliary_description_repair_handler_test.go | 0760b8949f7b6f4f750cbae8f0f4730ed5e41f385bd94063938a06c439038ba9（review 修复后） |
| tagmanagement/repository/auxiliary_description_repair_p2_test.go | e2c14011b18868a9c9c06f1500685bb27eb2e0f9fb9e7a7b842fef415fba5570 |
| platform/database/auxiliary_description_repair_migration_test.go | 911d6712371ffa6a99ed0887420dd7d4bea60ac23f67ed2037abb682b411cab5 |
| tagmanagement/service/service.go（含外部既有脏改） | 04300e671fa724729d73ca8e16cdcb4ca3da1902361fc94aff720fabb9ff473c |
| tagmanagement/service/auxrepair/auxrepair.go（B1 落盘，未改） | 503e2b5bbc0b1fcea1ea925e30cb321da89a28cc82c0f40b379e1d8483253718 |
| tagmanagement/repository/auxiliary_description_repair.go（B1 落盘，未改） | bdd1450383848eb9e5b2872d2a43986b624e58187713341bd08dbf8257c90a57 |
| platform/database/auxiliary_description_repair_migration.go（B1 落盘，未改） | 3d05ade2a7deb1cf2414969ff4de7a55b8256c06ee9bde4a5d38ab97c42cc077 |
| platform/database/migrator.go（B1 落盘，未改） | 42c3e64db2511d8b0ca64df480b6f24c8ef6eee00748c923e56cd7feaeb3d6ef |

## 7. 未实现 / 边界（勿当已交付）

- **2.3 原子保存**：事务外生成 `label + ': ' + description` 的 storage embedding、事务内锁行校验并原子写 description+embedding+任务提交阶段——未实现（本批只落 job/items）。
- **2.4 匹配执行**：失效 board cache、用既有 `MatchTopicTag` 处理冻结范围全部 active tag（含未归属）、逐项持久化成功/失败——未实现。
- **2.5 claim/lease/恢复/重试**：`claim_owner/claimed_at/lease_expires_at` 三列仍是预留空列；无 worker、无 goroutine、无 retry 端点（**故意不加假 retry 端点**）。
- **2.6 生产接线**：`RegisterAuxiliaryDescriptionRepairRoutes` 未接进 `tagmanagement/routes.go` / `app/router.go`，前端零改动（seam C）。因此**这些端点当前线上不可达**，这是刻意的批次边界，不是遗漏。
- 前端所需的一切（DTO 归一、轮询 composable、问题处理区 UI）属 seam C，未动。

## 8. 给后续 seam 的两条注意事项

1. **ID 类型（父线程已裁定）**：HTTP wire保持既有数字ID，前端API边界normalizer转换为string视图；implementation-contract §1已明确该分层。无需修改这三个HTTP DTO或全局AuxiliaryLabel.id数字DTO，本change前端新增归一类型独立采用字符串ID，组件不直接使用raw数字/snake_case。
2. **版本令牌**：`AuxiliaryVersionToken` 产出 43 字符（`v1_`+40 hex），前端不得按长度/结构解码；冲突响应保留用户草稿由前端负责重新核对（2.3+ 的事务内二次校验尚未实现）。

## 9. 纠正记录（2026-10-29，B2 独立 review BLOCK 后的修正）

B2 独立 review（只读审不可变快照 `/tmp/board-health-b2-review-bKcdnuTb`）判定 BLOCK：1 项 Medium + 2 项 Low。三项均在原合同范围内，修复与专项复验证据见 `seam-b2-review-fixes.md`。

- **R-1（Medium）尾随 JSON 仍会创建任务**：POST 端点原用 `ShouldBindJSON`，Gin decoder 只 Decode 一次 → 有效对象后追加第二个对象或非空垃圾仍 202 并落 job/items。**已修**：仅该端点改为 `decodeSingleJSONObject`（一次对象 Decode + 第二次必须 `io.EOF`，尾随空白仍合法），**全局绑定器未动**。回归：`TestAuxiliaryDescriptionRepairCreateRejectsTrailingJSON`（第二对象 / 尾随垃圾 / 尾随 `}` 均 400 且 job/items 零新增）。
- **R-2（Low）越界 id 落 500**：原复用 `parseUintParam`（接受完整 uint64），而 PG 的 label/job id 列是 signed bigint → 越界 id 被驱动拒绝后成 500。**已修**：仅这三个新端点改用 `parsePositiveBigIntParam`（正整数且 ≤ MaxInt64，否则 400），**共享 `parseUintParam` 与旧接口未动**。回归：`TestAuxiliaryDescriptionRepairRejectsOutOfRangeIDs`（MaxInt64→404；MaxInt64+1/MaxUint64→400，覆盖 context/create/get 的 `:id` 与 `:job_id`；无任务写入）。
- **R-3（Low）假阳性断言**：`auxiliary_description_repair_p2_test.go` 曾拿 topic_tag 的 id 比 label 的 id（碰巧等值）。**已修**：夹具先种占位辅助标签使两种 id 故意不等，断言改与 `label.ID` 比，并查持久 items 断言 `TopicTagID == tag.ID`。
