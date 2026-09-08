# harness-quick-wins Design

## Context

见 proposal.md Why——四项性能/记账浪费 + 两项流程修正案（同根因短路、steer 分级），实测依据已 pin 在 explore-findings.md（粘性失败真实形态：波次红绿 2-5 次转绿、build failed 三红同根因）。quality-gate.ts 现状：turn_end 挂钩、会话内快照差分增量路由、lint→vet→build→test 串行 `for...await`、stickyFailures 粘性重跑、steer 消息固定强语气。代码在 `.pi/extensions/`（gitignored），入库快照 `docs/research/quality-gate.ts`。

## Goals / Non-Goals

**Goals**：门禁墙钟 -60%+（lint cache 为大头）；事件库写放大 -80%（成功采样）；编译失败期不跑必红命令；steer 语气区分中间态与回归；扩展全景入档。

**Non-Goals**：不做失败退避（数据不支持复读机形态）；不动 stickyFailures 粘性语义；不动 pin.read；不动 spec-gate / entry-gate 行为；不新建 quality-gate capability spec（行为契约锚在 AGENTS.md/开发执行规范，本次仅同步文档）。

## 决策

### D1：eslint 缓存放门禁调用侧，不动 `pnpm lint` 脚本

quality-gate 经 `cmd.exe` 调 Windows 侧 eslint（实测 WSL DrvFS 跨文件系统 I/O 慢 ~17 倍：热缓存 WSL 35s vs Windows 原生 2.1s，瓶颈是文件系统而非 eslint 计算），命令形如 `cd /d <front> && pnpm exec eslint . --cache --cache-location node_modules/.cache/eslint/.eslintcache`；`front/package.json` 的 `lint` 脚本保持全量语义（人工排查 / pre-push / 归档验收需要绝对正确）。理由：`--cache` 对 flat config 变更存在不失效的已知坑，门禁语义=快速反馈可容忍；而 lint 脚本消费场景不能容忍。兜底：quality-gate 会话内 git diff 命中 `front/eslint.config.*` 变更时，该会话内门禁去 `--cache` 全量跑（配置变更回合不信任缓存）。缓存文件在 node_modules 下天然 gitignore。

### D2：lint 先行做短路哨兵，vet/build 并行，domain tests 保持串行

执行序改为：golangci-lint 先跑（~2.6s）→ 输出命中编译失败特征（窄匹配：`typechecking error` 或 `[build failed]` 精确子串——不用 truncateDiagGate 宽特征表，防 unused/gofmt 等 lint 规则错误误触发短路）则本轮短路返回（vet/build/test 不跑、不记账）；否则 go vet / go build `Promise.all` 并行，domain go test 保持串行（总预算 5min 逐包检查语义不破坏 + cmd.exe 并发峰值保守）。理由：lint 是编译失败的最快信号（比 build 快且信息更聚焦）；短路判定确定性字符串匹配，不做跨命令状态。

### D3：成功采样 = （session, cmd）连续成功计数 + 翻转锚点

会话内 per-cmd 状态（与 stickyFailures 同族的一个 Map）：`{ consecOk, lastWasFail }`。规则：ok=false → 全记，计数清零；ok=true 且 lastWasFail → 记（payload `flip:true`）；ok=true 且计数达 N（缺省 5）→ 记（payload `sampled:true, n:5`）；否则不落库。粘性重跑照旧触发命令，只是成功时少记账——记账与执行解耦，stickyFailures 不动。统计口径见 spec delta（锚点计 1、采样条 ×N 还原分母）。N 为代码常量（`GATE_OK_SAMPLE_EVERY = 5`），不做配置面（调参需求出现时再开）。

### D4：steer 分级信号 = 会话内该命令是否曾绿

会话内 per-cmd `everGreen` set（与 D3 同一状态对象）。失败 steer 文案两档：

- 曾绿变红（everGreen 命中）：`[回归] <cmd> 上回合尚绿，必须修复` —— 保持既有强语气；
- 从未绿过：`[中间态] <cmd> 失败（新代码中间态可能正常）；若正在推进可继续，回合末复检` —— 不打断 TDD 节奏。

AGENTS.md「agent 见到失败消息必须修」的门禁描述同步改为分级表述（任务 6.2）。**注意**：spec-gate 归档门禁与 §11 验收不受分级影响——归档前全绿仍是硬要求，分级只作用于 turn_end steer 语气。

### D5：扩展全景档 = AGENTS.md 一张矩阵表 + skill 写入方清单

AGENTS.md 增「pi 扩展全景」表：8 扩展 ×（挂点 / 触发条件 / 软硬 / fail 策略 / 记账 kind）。harness-facts skill 的写入方清单从 3 个补到实际（constraint-injection / quality-gate / harness-telemetry / tool-output-spill 的 spill.write）。不逐个开 spec（见 Non-Goals）。

## 备选方案与否决理由

- **采样改「段末聚合事件」（记一条带 successCount）**：否决——"段结束"无可靠钩子（会话结束事件不可靠），且改变事件粒度破坏既有 SQL 配方兼容性。
- **N=10 或概率采样**：否决——N=5 已使成功事件量降 ~80%，更稀的采样让"最近一次成功何时"失真；概率采样非确定性难测。
- **短路判定用 go build 而非 lint**：否决——build ≈2s 且错误信息与 vet/test 重叠度高，lint 的 typechecking error 是最快且特征最稳定的信号（实测 diag 序列验证）。

## Open Questions

（无——D1~D5 均已定案；eslint flat config 缓存失效坑按 D1 兜底处理）
