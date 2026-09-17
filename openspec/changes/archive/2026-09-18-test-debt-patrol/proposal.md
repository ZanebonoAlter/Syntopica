<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

单测跑红越积越多（用户 2026-09-17 报告，前后端都有存量红）。根因是测试体系只有"局部绿"保障：增量门禁（quality-gate 只跑影响包 + 前端 lint）与会话内 [回归] 修复运转良好，但"全局绿"在树莓派跑不动全量测试（test-scope-guard 事实库 30+ 条 full-go-test 劝退记录）后无人接盘——platform/骨架档改动不测、前端 typecheck/test:unit 留手动、基线按会话重置，三个盲区里的红测试可以无限期漂着；"不是本 change 改坏的"归因困境的真相是**历史上某个 change 改坏的，只是没有记账机制**。

## What Changes

- **受控摸底（第一步，纯取证）**：分片拉取前后端存量红清单；实测 `go test -short ./internal/...` 全后端耗时与前端 `test:unit --maxWorkers=2` 分片耗时，产出决定巡检分片粒度的数据
- **滚动巡检机制**：全量测试按分片拆解（后端按 domain 天然分片 / 前端按文件组），新增巡检脚本在会话低峰跑一两片、限并发防树莓派过载，N 天滚完一轮全量；分片进度与最近巡检结果可查
- **欠账台账**：巡检/归档撞见的非本 change 红测试登记台账（测试标识/首见时间/发现语境/状态），状态机为 红 → 已修 / 已豁免（附理由）；台账可查、可统计，接入 harness-retro 复盘
- **归档记账要求**：归档语境（§11 验证节实测 / 巡检）撞见非本 change 的红测试时，**登记台账后方可继续**——把"没人负责"变"有人记账"，堵住"发现即遗忘"的缺口
- **存量还债**：基于摸底清单开批量修复任务（本 change 内直接清欠或拆后续 fix change，视摸底规模定）

不做的事（明确出圈）：不改 quality-gate "turn_end 不跑 go test" 的门禁分层设计；不做反向依赖影响包分析（change-scope-gate 升级）——视摸底数据另立 change。

## Capabilities

### New Capabilities
- `test-debt-patrol`: 测试欠账治理——滚动分片巡检（触发/分片/限并发/进度可查）、欠账台账（登记/状态机/查询）、归档记账要求（发现非本 change 红 → 登记后放行）。与 `lint-zero-debt` 同构的"测试零漂没"契约。

### Modified Capabilities
- `harness-fact-log`: 巡检结果与台账登记写入事实库的事件词汇扩展（patrol 结果记账），维持"事件考古有账可查"的既有契约。

## Impact

- **脚本**：新增 `scripts/test-patrol.sh`（分片巡检 + 台账读写 + 进度查询），复用 change-scope.sh 的 domain 自动发现思路
- **台账存储**：`.pi/harness/` 下本机数据（独立 JSONL 或 events.db 单表，design 定）；`.pi/` 已 gitignore，修复行为仍走 git（fix change）
- **harness 扩展（可选联动）**：test-scope-guard / spec-gate 的记账提示语指引台账登记；不改门禁硬语义
- **文档**：`docs/reference/开发执行规范.md` §4.1 门禁分层（巡检通道定位）、`standard/backend/testing.md` + `standard/frontend/testing.md`（巡检用法）、`harness/pi-extensions.md`（事件词汇）
- **约束**：巡检分片必须限并发（`--maxWorkers=2`、不与 build/浏览器自动化并行、不与多 pi 会话叠加），遵守 `standard/frontend/testing.md` 的 load-106 事故教训
