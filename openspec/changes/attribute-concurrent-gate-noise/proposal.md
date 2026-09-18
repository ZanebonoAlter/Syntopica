<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: 无（纯 harness 工具链 change，不涉业务域代码） -->

## Why

并发 change 共享同一棵工作树，但两个消费者仍以「**全树 git 脏文件**」为输入，而归属地图（`edit.map` / `concurrent-change-coordination`）只接在了 `doc-impact verify` 上：

- **`doc-impact.sh suggest` 口径错**：`cmd_suggest()` 直接吃全树 `changed_files`（`doc-impact.sh` 约 155-190 行）。apply 启动时的预勾选会命中并行的其他 change 的 `.go`/`.vue` 文件，导致 tasks.md 写下错的 `<!-- doc-impact: ... -->`。声明错域的连锁后果是双向的：声明多了，归档门禁逼你去改无关文档；声明少了（本 change 该声明的域被误判为「已声明」），`verify` 的「疑似遗漏」永远不报，真遗漏被掩盖。
- **`quality-gate` 并发误报 + 重复播报**：`turn_end` 的触发集来自全树脏文件的 mtime/size 变化，而门禁命令本身是**全仓** `golangci-lint run ./...` / `eslint .`；别的会话（或会话启动前就存在的）半成品会被算到本会话头上，还因为「我这会话上一回合是绿的」被打成 `[回归]`，诱导 agent 去改别人的文件（最坏是跨会话写冲突）。同时失败消息**无去重**：同一 `(cmd, diag)` 只要没转绿，每回合把整块 `tail(output, 30)` 重新注入一次，`stickyFailures` 还让纯对话回合也重跑重发。

事实库证据（2026-09-18 采集，`events.db`）：

| 观测 | 数字 |
| --- | --- |
| 近 21 天门禁失败行 | 2398 条 |
| 其中 ±30 分钟内存在**另一个 session** 报同一 `(cmd, diag)` 的失败行 | 1608 条（67%，并发同频上界代理） |
| 同一会话同一 `(cmd,diag)` 重复 ≥3 次的组 / 行数 | 151 组 / 1773 行，其中 **1622 行（91%）是第 2..N 次重复注入** |
| 单次事故极端样本 | 单会话 669 条失败中 666 条为重复（223 连击的 toolchain 事故，已由 `gate-toolchain-health` 收敛） |
| 2026-09-18 06:08–06:13 现场 | 两个并发 session 各 **19 次** `golangci-lint` 报同一个 `internal/tagmanagement/service/sourcestats/sourcestats.go:63`；该目录当时是 `??`（别人未跟踪的半成品），而当时挂账的 change 是纯运维脚本 change `add-pg-key-tables-backup`（proposal 自述「不涉业务域代码」）——门禁把别人的半成品算成它的回归 |

## What Changes

- **`doc-impact.sh suggest` 接上归属轨**：新增 change 名解析（`--change` 参数 → `PI_SESSION_ID` 查事实库最新 `mode.set` 的 `boundChange` → 空），有归属集合时预勾选**只吃归属集合**；输出改**三桶分列**（本 change / 其他 change / 无归属），**不静默过滤**——归属轨只会少不会多，静默过滤会制造漏声明（比噪声更糟）。
- **quality-gate 失败报告去重（与并发无关的独立收益）**：按 `(cmd, diag)` 指纹抑制重复——首次输出完整失败块，同指纹持续降为单行摘要，连续 ≥3 回合未变化附加「未修」标记，转绿输出一行收尾；**失败粘性重跑语义不变**（门禁不沉默，只是不再重复注入同样的字节）。
- **quality-gate 并发外部失败归因**：新增**会话启动基线**（现在 `snapshot` 每回合被覆盖，基线丢了）+ 三向归属判定（本会话触发集 ∪ 本 change 归属 / 会话启动基线 ∪ 其他 change 归属），只对**有正证据**的外部失败降级为 `[外部]`：不进粘性、不打 `[回归]/[中间态]`、一行提示 + 记 `policy.decision(action=warn, reasonCode=foreign-breakage)`；路径解析不出或归属混合时**维持现状**（保守，不开口子给假阴性）。
- 机制文档 `docs/reference/harness/pi-extensions.md` 同步（quality-gate 节补失败报告与并发归因小节）。

## Capabilities

### New Capabilities

- `gate-failure-reporting`: quality-gate 增量门禁的失败报告口径——失败指纹与重复抑制（报告收敛）、并发工作树下失败归属判定与外部失败降级（含归因记账）。

### Modified Capabilities

- `doc-impact-gate`: 新增 `suggest` 预勾选输入口径要求（归属轨优先 / 三桶输出 / 全树回退显式标注），既有「文档影响声明（apply 启动时）」「verify 归档对账」行为不变。

## Impact

- 受影响脚本/扩展：`scripts/harness/doc-impact.sh`（suggest + 两个只读查询函数）、`.pi/extensions/quality-gate.ts`、`.pi/extensions/lib/failure-classify.ts`（新增路径提取与归属判定纯函数）。
- 受影响测试：`scripts/harness/doc-impact.smoke.sh`、`.pi/extensions/tests/quality-gate.behavior.smoke.cjs`、`.pi/extensions/tests/failure-classify.smoke.cjs`。
- 不涉及前端产品代码、后端 Go 代码、数据库、API、部署形态；不动 `concurrency-status.sh`、不动 `spec-gate` 检查⑤'、**不收窄门禁命令范围**（见 design Non-Goals）。
- 部署后影响：agent 在 apply 启动看到的文档域预勾选更准；并行会话下门禁 steer 的上下文占用与误报催修显著下降；账本新增 `policy.decision(warn, foreign-breakage)` 事实（可被 harness-retro 回检）。无用户可见产品行为变化，无需用户手动操作或数据迁移。
