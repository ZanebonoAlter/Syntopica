<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

2026-09-23 测试欠账巡检扫出一轮 132 条红，逐层排查后确认其中 116 条为假红、1 次误报超时、2 条真红——三处独立根源均指向「测试运行环境不可信」，欠账台账被环境噪声污染后失去排期价值（事实链与复现步骤见 `docs/research/test-env-pitfalls/explore-findings.md`）：

1. **pi 会话 `NODE_ENV=production` 泄入 vitest**：pi agent 主进程以 production 模式运行，bash 子进程继承该变量 → vitest 用 Vue 生产构建 → 事件追踪被裁 → `wrapper.emitted()` 断言批量假红（fe-tags 85 / fe-features 22 / fe-components 4 / fe-discovery 5）。worktree 切到已知全绿提交仍红、`NODE_ENV=test` 前缀重跑立即全绿，已双重验证。
2. **`bd4f8847` 的 useState mock registry 用例间泄漏**：vitest.setup.ts 把 `useState` mock 改为按 key 全局单例 registry（为测 useConfirm 跨实例共享），但未做用例间清理——前一个用例写入的状态泄漏给后续用例，fe-composables 2 条「静默降级期望清零」断言读到残留值（二分验证引入点唯一为该提交）。
3. **be-skeleton 分片 DDL 锁竞争拖满 go test 默认 600s 超时**：`TestInitDBConnectsToPostgres` 跑生产 `InitDB`（含 GORM AutoMigrate），dev 后端并发运行分析任务持表锁时测试等到 panic（panic 输出无 FAIL 行，巡检报「runner 退出码 1 但无法解析失败明细」）；App 空闲时 3.65s 通过。巡检分片应快速失败并给出可解析的失败标识，而不是耗 10 分钟产出无法归账的超时。

## What Changes

- `front/vitest.config.ts`：`test.env` 钉死 `NODE_ENV: 'test'`（vitest 的 `test.env` 覆盖进程环境，任何入口——pi 会话 / 巡检脚本 / 手动终端——跑单测都安全；`pnpm build` / `pnpm dev` 不受影响）。
- `front/vitest.setup.ts`：useState registry 增加用例间清理（`beforeEach` 清空 registry），保留 bd4f8847 引入的 per-key 单例语义（useConfirm 类跨实例共享 composable 仍可测）。
- `scripts/harness/test-patrol.sh`：后端分片 go test 显式 `-timeout 120s`，DDL 锁竞争时 2 分钟内快速失败；失败输出保持可被巡检 FAIL 解析归账。
- 修复验收绑定欠账台账指标：fe-composables 2 条 open 欠账转 fixed、后续巡检 fe 分片假红为 0。

## Capabilities

### New Capabilities
- `frontend-test-env`: 前端 vitest 运行环境的确定性约束——NODE_ENV 不继承宿主进程、全局 mock 状态用例间隔离。

### Modified Capabilities
- `test-debt-patrol`: 滚动分片巡检增加「后端分片 go test 超时上界」要求（快速失败、失败可归账）。
