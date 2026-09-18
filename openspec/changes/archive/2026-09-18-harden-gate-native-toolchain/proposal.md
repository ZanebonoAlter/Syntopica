<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

2026-09-17 早晨环境迁移当天，两个并发 pi 会话在 native 模式下因启动环境 PATH 缺 go/golangci-lint，quality-gate 每回合三连跑（lint/vet/build）秒败并经失败粘性放大，22 分钟产生 945 条假失败 gate.check——占 7 天复盘窗口全部失败的 56%、「并发/环境冲突」桶的 90%，且 [回归] 分级 steer 误导 agent 去修不存在的代码问题。现有防护只覆盖 windows 模式（interop 探测 + 特征归因，见 `gate-interop-health` spec），native 模式「不探测、不短路」是有意设计但存在盲区：它防的是「探测误伤」，没料到 native 自己也会工具链缺失。

## What Changes

- quality-gate native 模式新增**事前工具链可达性探测**：PATH 上找不到 go / golangci-lint（前端 pnpm）时短路该侧门禁（fail-open），记 `policy.decision(action=fail-open, reasonCode=toolchain-down)`，steer 提示环境问题与恢复方向（与 interop-down 同款套路）；探测结果会话内缓存（对齐「平台判定会话内稳定」既有约束）
- 新增**事后特征归因兜底**：`lib/failure-classify.ts` 增加 `isToolNotFound()`（对齐 `isInteropFailure()` 先例），native 模式下命令输出含 `command not found` 特征的失败不进粘性集合、不按 [回归]/[中间态] 分级，归因为环境问题
- 上述行为写入新 spec `gate-toolchain-health`；`docs/reference/harness/pi-extensions.md` 扩展机制文档同步补一节

## Capabilities

### New Capabilities

- `gate-toolchain-health`: quality-gate native 模式工具链健康防护——工具链不可达时短路门禁并显式区分"环境烂"与"代码烂"，防止 command-not-found 假失败经粘性放大污染账本与误导修复

### Modified Capabilities

（无——`gate-interop-health` 的 Requirements 不动：其边界仍限 cmd.exe interop 链路，其「native 下不含 interop 特征的失败按代码失败处理」的语义与本 change 新增的 command-not-found 特征归因正交不冲突）

## Impact

- `.pi/extensions/quality-gate.ts`：turn_end step 3.5 之后新增 native 工具链探测分支；`gateLog` 环境故障归因分支扩展
- `.pi/extensions/lib/failure-classify.ts`：新增 `isToolNotFound()` 特征函数
- `.pi/extensions/tests/`：新增/扩展 smoke 用例
- `docs/reference/harness/pi-extensions.md`：机制文档补节
- 不影响 windows 模式链路；不改 harness-retro.sh（其 `command not found` 归并分类在修复后自然失去输入）
