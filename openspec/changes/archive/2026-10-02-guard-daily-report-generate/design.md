# Design — guard-daily-report-generate

## Context

9/30 事故链（见 proposal Why + `docs/research/daily-report-generate-concurrency/explore-findings.md`）：日报生成现有两个触发面——调度（`admin/scheduler/job_daily_report.go` 的 `DailyReportJob`）与手动端点（`topicgraph/handler/daily_report_handler.go:96` `triggerGenerateDailyReport`）。调度层有自己的 `isExecuting`（`BaseScheduler.TrySetExecuting`，`TriggerNowWithDate` 已接），但手动端点在 topicgraph 域、拿不到 scheduler 实例，裸 `go generateAllBoards(...)`。两域只能通过共享 import 的包协调：`admin/scheduler` 已 import 域根包 `daily_report "syntopica-backend/internal/topicgraph"`（调 `CollectBoardIDsForDate` 等）。

## Goals / Non-Goals

**Goals**
- 全系统同一时刻至多一条日报生成管线；手动端点与调度作业收敛到同一把非阻塞闸。
- 被拒触发返回结构化 409，且不产生第二条终态通知。
- 闸 panic 安全（defer 释放）。

**Non-Goals**
- 不做「当日已存在报告 → 二次确认/force」语义（同日窗口内重建=整份覆盖是既有设计，保持不变）。
- 不做跨进程/分布式锁（单用户单进程部署，进程内原子标志足够）。
- 不改通知中心的去重逻辑本身——重复通知是双跑的症状，闸修好即消失。
- 不改 `waitForDailyReportWindow` 队列感知等待逻辑。

## Decisions

### D1：闸放 `internal/topicgraph/service` 子包，域根 wire.go re-export（不放 scheduler、不注入 handler）

新增 `internal/topicgraph/service/generation_gate.go`：`atomic.Bool` + `TryAcquireDailyReportGeneration() bool` / `ReleaseDailyReportGeneration()`（CAS 语义）；域根 `wire.go` 按既有 re-export 模式（`CollectBoardIDsForDate` 同型）转发。

- 备选 A：复用 `BaseScheduler.isExecuting`——需把 scheduler wrapper 注入 topicgraph handler（跨域构造注入），且「调度层防 tick 重入」与「生成管线互斥」两个语义混在一把锁上，边界糊。弃。
- 备选 B：PG advisory lock——单进程系统杀鸡用牛刀，还引入 DB 依赖。弃。
- 备选 C：闸本体放域根包 `internal/topicgraph`——域根包 import handler（wire.go 转发路由），handler 反向 import 域根包即循环导入（Go 编译器硬拒），handler 摸不到域根闸；除非 wire init() 往 handler 注入 seam（引入初始化时序耦合与危险默认值）。apply 阶段用户拍板弃。
- service 子包与两调用面依赖全通：handler 已 import service（直接调）、scheduler 已 import 域根（wire re-export 拿转发），零新增依赖边，无循环。

**闸语义**：域闸是生成互斥的**唯一权威**；`BaseScheduler.isExecuting` 保留原职（调度 tick 防重入 + `TriggerNowWithDate` 快速拒绝），不承担端点防护，二者是非阻塞 Try 串联、无死锁面。

### D2：两个触发面的接闸位置

- 调度：`DailyReportJob` 闭包体开头 `TryAcquire`，失败→`JobResult{skipped: "already_running"}`（复用现有 skip 机制）；成功→`defer Release`，**先于** `waitForDailyReportWindow`——窗口等待期（队列未清空的长等待）也占闸，与 9/30「等待中手点必然双跑」的现场对齐。`generateAndSaveReport` seam（测试桩替换点）不受影响。
- 手动：`triggerGenerateDailyReport` 在 rebuild-window 校验之后 `TryAcquire`，失败→`409 {success:false, error:"日报生成正在执行中，请稍后再试。", reason:"already_running"}`；成功→把 Release 挂进 `generateSingleBoard`/`generateAllBoards` goroutine 的 defer（含 panic 路径——现有 `broadcastDone` 之后、函数返回前）。

### D3：前端零改动

`DailyReportGenerateDialog.vue` / `LaneDynamicsPanel.vue` 均已有 `res.success === false` 错误 toast 通道，透出后端 `error` 字段即得「正在生成中」提示。不新增状态、不新增界面结构（ui-impact: none 的依据）。

## Risks / Trade-offs

- [窗口等待期占闸，用户手点被 409，需等调度管线自己出] → 单用户可接受；409 文案与 `TriggerNowWithDate` 现行文案一致，无新概念。
- [panic 泄闸 → 永久 409 直到重启] → 两条触发路径的 Release 全部 `defer`（panic 也执行）；测试覆盖 panic 释放场景。
- [调度 skip `already_running` 时 `scheduler_tasks.last_execution_result` 记为 skip 而非 success，运维语义微变] → 与暂停期 skip 同型，属既有 skip 语义的自然延伸。

## Migration Plan

无 DB 迁移、无配置项。部署即生效；回滚 = 回退二进制。旧数据零降级。

## Open Questions

（无）
