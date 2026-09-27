## 1. NODE_ENV 钉死（frontend-test-env · Scenario 1-2）

- [x] 1.1 `front/vitest.config.ts` 顶层 `process.env.NODE_ENV = 'test'`（vitest test.env 实为 `??=` 语义不覆盖已有变量，实测无效后改 config 顶层强制赋值，见 design 决策 1；作用域仅单测进程，不动 `pnpm build` / `pnpm dev`）
- [x] 1.2 验证（环境注入复现原故障）：在 pi 会话 bash（宿主 `NODE_ENV=production`）执行 `cd front && pnpm test:unit app/components/ui/AppDialog.test.ts --maxWorkers=2` → 退出码 0、`Tests 13 passed`（修复前同命令 3 failed，报 `expected undefined to deeply equal [false]`）
- [x] 1.3 验证（不回退）：`cd front && pnpm test:unit app/composables/useConfirm.test.ts --maxWorkers=2` → 全绿（bd4f8847 引入的跨实例共享用例保持通过；2026-09-24 归档前复跑 5 passed）

## 2. useState registry 用例间清理（frontend-test-env · Scenario 3-4）

- [x] 2.1 `front/vitest.setup.ts` 的 useState registry 加用例间隔离：`beforeEach` 遍历重置各 ref 到 init 初始态、引用保留（初版全量 clear 打断模块顶层 composable 引用致 AppConfirmDialog 6 条假红，实测后改为重置值方案，见 design 决策 2；从 vitest 导入 beforeEach；注释含 2026-09-23 假失败事实链指向 docs/research/test-env-pitfalls/）
- [x] 2.2 验证（原 2 条真欠账转绿）：`cd front && NODE_ENV=test pnpm test:unit app/composables --maxWorkers=2` → 0 failed（修复前 useNotifications / useTagQueueProgress 2 条「静默降级期望清零」用例失败，读到前一用例残留值）
- [x] 2.3 验证（共享语义不回退）：同命令下 `useConfirm.test.ts`、`useNotifications.test.ts` 其余用例全绿

## 3. 巡检后端分片超时上界（test-debt-patrol · 新 Scenario「后端分片锁竞争时有界失败」）

- [x] 3.1 `scripts/harness/test-patrol.sh` 全部 6 个后端分片的 `go test` 参数追加 `-timeout 120s`（分片定义集中静态枚举，一处模式统一改；前端分片不动）
- [x] 3.2 验证：`bash scripts/harness/test-patrol.sh --shard be-skeleton` → `✓ [be-skeleton] 全绿`（正常耗时 12-15s，远低于新上界）
- [x] 3.3 验证（脚本自身冒烟）：`bash scripts/harness/test-patrol.smoke.sh` → 通过（分片定义变更不破坏既有冒烟契约）

## 4. 整轮回归与欠账台账回检

<!-- 注：4.5 在 4.4 之前执行（2026-09-24 顺手修复），编号按提出顺序追加 -->

- [x] 4.1 修复后完整巡检一轮：`bash scripts/harness/test-patrol.sh --shards 12` → 12 片中 11 片全绿、fe-core 仅剩已知 iconify subset 1 条真欠账（fe-composables 2 条转绿；该欠账已于 4.5 清掉）
- [x] 4.2 欠账台账：`bash scripts/harness/test-patrol.sh --report` → fe-composables 域 open 计数 = 0（对应 2 条记录迁 fixed，fixed_by=patrol）
- [x] 4.3 前端收尾门禁：`cd front && pnpm lint && pnpm exec nuxi typecheck` → 均通过（2026-09-24 归档前复跑：lint 0 error / typecheck exit 0）
- [x] 4.5 顺手清掉 fe-core 唯一真欠账（2026-09-24 会话确认非环境假红：SignalResearchTrace.vue 新增 2 个 mdi 图标引用后未重生成清单）→ 跑 `pnpm generate:icons` 重生成 iconify-subset.json、复跑测试全绿、`--shard fe-core` 归账后台账 open 计数 5→4

## 5. 测试

<!-- 注：本 change 为 simple 档，无独立 test-cases.md（纯环境/脚本修复，行为故事即修复验收本身，用例以场景验收内联在各任务行）；scenario-trace 对账表见 §7 验证节 -->

## 6. 文档

<!-- doc-impact: none(测试运行环境修复，未触及 reference 文档管辖面；事实链存 docs/research/test-env-pitfalls/，确定性约束由 frontend-test-env / test-debt-patrol spec 承载，归档时同步主 specs) -->

- 无 reference 文档更新：改的是测试运行环境与巡检脚本参数，`standard/frontend/testing.md` 无 NODE_ENV/超时上界相关表述需同步（已 grep 核实）；确定性约束由新 capability `frontend-test-env` 与 `test-debt-patrol` 修改节承载，归档时同步主 specs。
- 无 flow 影响（纯测试环境/工具链修复，不涉业务域，§12.2 变更溯源豁免）

## 7. 验证

<!-- 注：4.4 为归档后义务，转回检指标挂账，不占勾选框 -->

- [x] V1 环境注入复现：pi 会话 bash（宿主 `NODE_ENV=production`）跑 AppDialog 组件测试 13 passed（任务 1.2，修复前后对照）
- [x] V2 不回退：useConfirm.test.ts 5 passed（任务 1.3）
- [x] V3 registry 隔离：`pnpm test:unit app/composables --maxWorkers=2` 0 failed（任务 2.2/2.3）
- [x] V4 巡检：be-skeleton 分片 12-15s 全绿、test-patrol.smoke.sh 通过、整轮 12 片 11 绿（任务 3.2/3.3/4.1）
- [x] V5 台账：fe-composables open 计数 = 0，2 条迁 fixed（任务 4.2）
- [x] V6 前端收尾门禁：lint 0 error + typecheck 通过（任务 4.3）
- [x] V7 `openspec validate fix-test-env-false-reds --strict` → 通过；`doc-impact.sh verify` / `scenario-trace.sh` / `check-tasks-tail.sh` 均退出码 0

### 回检指标（归档后 7 天窗口，`harness-retro` 复查）

| 回检指标 | 基线（2026-09-23/24） | 期望 | 观察窗口 |
| --- | --- | --- | --- |
| fe 分片环境性失败（NODE_ENV 泄入 / registry 泄漏） | 116 条假红 | 0 | 上线后 7 天 |
| be 分片拖满 600s 超时且无法归账 | 1 次 | 0 | 上线后 7 天 |

（7 天后 `bash scripts/harness/harness-retro.sh --days 7` 确认；基线 .pi/harness/retro-baseline.json 已于 2026-09-23 存档，可 `--baseline` 对比）

### Scenario→测试文件映射（scenario-trace 对账）

| Scenario | 测试文件 |
| --- | --- |
| 宿主进程携带 production 时单测仍可断言组件事件 | front/app/components/ui/AppDialog.test.ts |
| 构建与 dev server 不受影响 | 人工（vitest.config.ts 顶层赋值仅单测进程内生效，不入 build/dev 链路；V6 构建链验证留痕） |
| 前一用例的状态不泄漏给后续用例 | front/app/composables/useNotifications.test.ts front/app/composables/useTagQueueProgress.test.ts |
| 同一用例内跨实例共享不回退 | front/app/composables/useConfirm.test.ts |
| 单分片巡检落账 | scripts/harness/test-patrol.smoke.sh |
| 资源红线——低并发执行 | scripts/harness/test-patrol.smoke.sh |
| 后端分片锁竞争时有界失败 | scripts/harness/test-patrol.smoke.sh |
| 分片内测试全绿 | scripts/harness/test-patrol.smoke.sh |
