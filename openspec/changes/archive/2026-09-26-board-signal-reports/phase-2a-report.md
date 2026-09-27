# 阶段2a交付报告：发现闭环（phase-2a-report）

> 2026-09-22，develop 主仓库直改（树上其他 change 脏改未触碰、未还原）；阶段1 合同文件（datasources/repository/signal_material）零改动。任务范围=2.3 detect + 2.4 discovery handler/job 扩展（discovery 部分）；research/compose/calculate 全部留给 2b，未顺手做。

## ① 改动文件清单

| 文件 | 改动 |
| --- | --- |
| `backend-go/internal/dataenrichment/service/signal_detect.go` | **新增**。`SignalDetector.Detect`（严格 schema 校验+白名单证据过滤+score 门槛+最多 2 次尝试）、`SignalDiscoveryService.DiscoverSignals`（prepare→detect→原子保存编排 + `SignalStageError` 阶段错误）、`GenerateSignalDiscoverySessionID`、`SignalDetectionsToCandidates`、detect prompt 装配。detector/service 均不持有 tool registry（发现零取数/零计算/零成文，编译级保证） |
| `backend-go/internal/dataenrichment/service/signal_detect_test.go` | **新增**。12 用例覆盖 SD-1~5 + session 格式 + 零工具面断言 + 候选行映射 |
| `backend-go/internal/dataenrichment/handler/analysis_runner.go` | **共享基建扩展**（旧 kind 输出零变化）：① `AnalysisStatus`/`analysisJob` 增 phase/outcome/error_stage/granularity/period/discovery_id/candidate_id/candidate_count（全部 `omitempty`，只经 `SignalJobPatch` 写入）；② 新 kind 常量 `board_signal_discovery`/`board_signal_report`（后者语义属 2b，仅保证状态契约完整）+ phase/outcome 常量组；③ `Start` 重构为薄包装（签名与行为逐字节不变），新增 `StartSignal`（fn 收 goroutine 安全的进度 reporter）与 `launch` 共同路径；④ 超时/panic 等未显式报告终态的信号任务由 runner 兜底 `outcome=failed`；⑤ `RunningSignalReportForCandidate`（O(1) 查 board 槽位 live research job，派生「研究中」用） |
| `backend-go/internal/dataenrichment/handler/signal_discovery.go` | **新增**。`triggerSignalDiscovery`（400 无 job/400 开关/409 互斥/202 帧；job fn：phase 透传 → outcome=discovered\|no_signal\|failed+error_stage → 恒返 0 无 result_id）、`listSignalCandidates`（游标分页+派生状态）、`SignalDiscoveryRunner` 接口、`SetSignalDiscovery`/`SetSignalDiscoveryOnInstance`（post-construction 装配） |
| `backend-go/internal/dataenrichment/handler/handler.go` | 增量：`EnrichmentHandler` 加 `signals` 字段；board 分析组（`:167`）注册 `POST /signal-discoveries`、`GET /signals` |
| `backend-go/internal/dataenrichment/wire.go` | 增量追加（预存脏改之上）：`signalDiscovery := service.NewSignalDiscoveryService(airouter.NewRouter(), CapabilityAnalysis, service.NewSignalMaterialBuilder(db, lifelineReader), repo)` + `handler.SetSignalDiscoveryOnInstance(...)`。InitHandler 既有参数未动 |
| `backend-go/internal/dataenrichment/handler/export_test.go` | **新增（测试专用）**：`StartHangingSignalReportJob` 给派生状态测试挂 live report job（带可见性等待，消除竞态） |
| `backend-go/internal/dataenrichment/handler/signal_discovery_test.go` | **新增**。11 用例覆盖 API-1/2、S1、JB-1/3/4（discovery 部分）；stub runner + stub LLM，不起真模型 |
| `openspec/changes/board-signal-reports/tasks.md` | 勾选 2.3/2.4（4.4 research 侧留 2b）；本报告 |

## ② 新 API 契约（给阶段3 前端消费）

### 路由（挂现有 board 分析组）

```
POST /api/semantic-boards/:id/enrichment/analysis/signal-discoveries
GET  /api/semantic-boards/:id/enrichment/analysis/signals?granularity=&period=&before_id=&limit=
```

job 轮询沿用现有 `GET /api/enrichment/analysis-status?job_id=`（及 `?scope=board&id=` 恢复）。

### POST /signal-discoveries

- 请求 body：`{"granularity":"month|year","period":"YYYY-MM|YYYY"}`
- **400**（无 job 产生）：周期非法/未来（`ParseSignalPeriod` 错误原文即消息）；增强开关未开启（同旧 trigger 文案，含 `not enabled` 可机判前缀）
- **409**：同板块任一任务在跑（与旧 brief/investigation 共享互斥），`data` 携 running job 完整身份（`job_kind` 可区分是谁在跑）
- **202** `data`：`{status:"started", job_id, job_kind:"board_signal_discovery", scope:"board", target_id, granularity, period}`

### job 状态新增字段（signal job 专用，旧 kind 输出零变化）

| 字段 | 出现时机 | 值 |
| --- | --- | --- |
| `phase` | 运行中 | `prepare`（材料装配）→ `detect`（detect LLM）；终态停留最后 phase（discovery 为 `detect`） |
| `outcome` | 终态 | `discovered`（候选≥1）/ `no_signal`（0 条，正常完成）/ `failed`（失败，不伪装零信号） |
| `error_stage` | 仅 failed 且 stage 可知 | `prepare` / `detect` / `save` |
| `discovery_id` / `candidate_count` | 终态 | 批次 id / 保存候选数（`no_signal` 时 `candidate_count`=0 省略键） |
| `result_id` | **discovery 永不出现**（2b 的 research job 才会有，仅成功） | — |

前端轮询判定：`finished && outcome=="no_signal"` → 安静空态不清列表；`failed` → 错误态可重试；`discovered` → 刷新候选列表。

### GET /signals

- 参数：`granularity`（必填 month|year）、`period`（必填，形状随 granularity）、`before_id`（可选，候选 id 游标，排他）、`limit`（默认 20，>100 截断为 100，非法 400）
- 响应 `data`：候选数组，按批次/id 倒序；每行：

```json
{
  "id": 3, "discovery_id": 2, "granularity": "month", "period": "2026-08",
  "signal": "…", "why_it_matters": "…", "research_question": "…",
  "evidence_refs": ["31","32"], "score": 8, "rationale": "…",
  "discovery_created_at": "…",           // 发现时间来自批次
  "status": "pending|researching|reported", // 服务端派生，前端只读勿自行推断
  "latest_result_id": 12                  // 可空 null：仅 reported 有值
}
```

- 空新批次天然不清旧记录（只查候选表）；空态=空数组。
- 派生状态语义：`researching`=该候选有 live running research job（不持久化，重启不会卡死）；`reported`=已有成功 signal_report；否则 `pending`。

### 2b 预留（本批未实现，前端勿消费）

`POST /signals/:candidateId/research`、`GET /signal-reports`、`GET /signal-reports/:rid`、job kind `board_signal_report`、phase `research|compose`、outcome `succeeded`、job 字段 `candidate_id`（已在状态结构中预留，researching 派生已依赖它）。

## ③ detect service 公开 API（给下一批 2b）

```go
// service/signal_detect.go
func GenerateSignalDiscoverySessionID(boardID uint) string        // board_signal_discovery_{id}_{hex8}
type SignalDetection struct { Signal, WhyItMatters, ResearchQuestion string; EvidenceRefs []string; Score int; Rationale string }
func NewSignalDetector(router AirRouter, capability airouter.Capability) *SignalDetector
func (d *SignalDetector) Detect(ctx, material *SignalMaterial, sessionID string) ([]SignalDetection, error)
    // 空材料零 LLM；非法 schema/路由错误恰 2 次尝试后 error（调用方转 job failed）
func SignalDetectionsToCandidates([]SignalDetection) ([]*repository.BoardSignalCandidate, error)

// 发现编排（handler 的 job fn 消费；2b 的 research job fn 可对齐此模式）
func NewSignalDiscoveryService(router AirRouter, capability airouter.Capability, builder *SignalMaterialBuilder, repo *repository.Repository) *SignalDiscoveryService
func (s *SignalDiscoveryService) DiscoverSignals(ctx, boardID uint, granularity, period string, hook func(stage string)) (*SignalDiscoveryOutcome, error)
    // SignalDiscoveryOutcome{DiscoveryID, CandidateCount, SessionID}
    // hook 只发 phase（prepare|detect）；失败返回 *service.SignalStageError{Stage: prepare|detect|save}
type SignalStageError struct{ Stage, Err }   // handler 用 errors.As 取 error_stage
```

**2b 可直接复用的 job 基建现状**（`handler/analysis_runner.go`）：

- `runner.StartSignal(scope, id, kind, timeout, fn func(ctx, report func(SignalJobPatch)) error)`——fn 里用 report 发布 `SignalJobPatch{Phase/Outcome/ErrorStage/Granularity/Period/DiscoveryID/CandidateID/CandidateCount}`；零值字段不覆盖；终态后 patch 被忽略；超时/panic 未报告时兜底 `outcome=failed`（仅 signal kind）。
- phase 常量已备好：`SignalPhaseResearch`/`SignalPhaseCompose`；outcome 常量 `SignalOutcomeSucceeded/Failed`。
- research POST 需要的同板互斥/409 帧、30 分钟超时（`analysisJobTimeout`）、重启 404 语义全部现成；research 成功后 job fn 自己写 result（repository 层 4.2 已交付），成功时经 `SignalJobPatch` 报 `Outcome: succeeded` 并 `return resultID`（`StartSignal` 的 launch fn 已支持返回 resultID）。
- 派生状态查询 `RunningSignalReportForCandidate(boardID, candidateID)` 已被 GET /signals 消费；research job fn **必须**在启动时 `report(SignalJobPatch{CandidateID: id})`，否则候选不会显示「研究中」。
- handler 装配：`handler.NewHandler(...)` 后 `h.SetSignalDiscovery(runner)`；生产走 `SetSignalDiscoveryOnInstance`（wire.go 已接）。

## ④ 测试结果（命令 + 退出码）

| 命令（cwd=backend-go） | 退出码 | 说明 |
| --- | --- | --- |
| `go test -short ./internal/dataenrichment/... -count=1` | 0 | 影响包（change-scope 判定本 change 路径全落 dataenrichment 域）；4 包 ok，含新 23 用例与既有全量回归（旧 kind 状态输出零变化由既有 brief/investigation/topic 用例钉住） |
| `go test ./internal/dataenrichment/handler/ -short -run TestSignal -count=1` ×8 连跑 | 0 | 稳定性复跑（修掉 2 处测试竞态后 8/8 绿） |
| `golangci-lint run ./...` | 0 | 0 issues |
| `go vet ./...` | 0 | — |
| `go build ./...` | 0 | — |

测试全部 stub（stub AirRouter / stub SignalDiscoveryRunner），零真实 LLM 调用；handler 层按 testing.md 分层用内存 SQLite（repository 包测试未被触碰、仍为隔离 PG）。

## ⑤ 未做项 / 残余风险

- **2b 辖区零涉及**：research/compose/calculate/policy、`POST /signals/:candidateId/research`、signal_report result 写入、allowedTools 四源授权——全部未动。`board_signal_report` 常量与 phase/outcome 枚举仅为契约占位。
- **4.4 不勾**：research job 侧（result_id 仅成功存在、candidate_id 生命周期、重启恢复的 research 分支）留 2b 完成后勾选。
- **detect 输入规模**：材料 JSON 全量进 prompt（SignalMaterial 含切片 thread_titles）。月度窗口约 53 篇/泳道 × 泳道数的切片粒度远小于全文量，但年度窗口（year 粒度 12 个月切片累积）可能放大——首版未做材料 token 预算裁剪，若真实样本出现超长 prompt，需在 2b 或效果核对阶段回补（记为待观察项，非本批缺陷）。
- **`DiscoverSignals` 的 `now` 取 `time.Now()`**：period/cutoff 校验用真实时钟，测试经种子历史周期（2020-01）绕开未来拒绝；服务未注入时钟缝（构造器保持 4 参最小面）。若 2b 需要 deterministic 时间，再加 setter 即可。
- **错误路径的真库行为**：handler 测试为 SQLite 轻量层；`CreateSignalDiscoveryBatch` 的 PG 约束行为由阶段1 隔离 PG 用例守护，本批未新增 PG 用例（未改任何 SQL）。
- **wire.go 仍带其他 change 的未提交脏改**（research tools 注册等）：本批只增量追加，未还原未覆盖。

## ⑥ 裁决请求

无阻塞项。两点备案：

1. **detect 无材料预算裁剪**（见⑤）：design §2 只规定了「复用预算/排序思想」，未给 discovery 材料 token 上限的具体数字。当前实现按材料原样进 prompt；若需要硬上限（如切片数截断/标题截断），请给数值，属小改动。
2. **`no_signal` 时 `candidate_count` 键省略**：`omitempty` 使 0 值不出现在 job 状态 JSON（前端以 `outcome=="no_signal"` 判断，不看计数）。若阶段3 希望显式 `"candidate_count":0`，需把该字段改 `*int`——属一分钟改动，等前端反馈。
