# Review：enable-unbound-jit-injection

> 格式契约见 `openspec/specs/review-trace-gate/spec.md` 与 `scripts/harness/review-trace.sh`：
> 标题行 `### R-<N> <YYYY-MM-DD>【High|Medium|Low】<标题>` + 紧随状态行 `- **状态**：<五值封闭>`。
> 规划期三轮审查意见 + 实现期新发现问题逐条建账。

## 规划期审查（三轮）

### R-1 2026-11-06【High】`constraint.inject` 的 `mode` 字段语义冲突（已是文档形态）
- **状态**：已采纳（改用 append-only 的 `tier` 字段，`mode` 语义不动）

`mode` 在 `InjectedDocEntry` 里表示**文档形态**（`section` / `digest` / `full`），不是会话档位；复用它做档位归因会污染既有统计口径。改为 payload 追加 `tier`（`none` / `requirements` / `implementation`，注入器自报），历史无 `tier` 的记录计「未知」并剔除分母。

### R-2 2026-11-06【Medium】事件名 `tool_call` 写错
- **状态**：已采纳（事件名 `tool_execution_start`，路径取 `event.args.path`）

JIT 写入与投递两处 gate 实际挂在 `tool_execution_start`（pi `ToolExecutionStartEvent` 的参数字段是 `args` 不是 `input`）；制品里的 `tool_call` 会让实现改错事件。实现按 `args.path` 落。

### R-3 2026-11-06【Medium】预算量化前提错误（daily-report 46.3KB、上界 17 份）
- **状态**：已采纳（design D4 换成实测表）

原稿把「业务约束与不变量」节字节与注入条数上界写成估值。实测：daily-report 46.3KB / semantic-board 22.4KB / data-enrichment 27.1KB / reading 15.4KB / discovery 14.7KB / ai-summary 12.5KB / topic-graph 8.3KB / content-enrichment 8.1KB / scheduler 7.4KB / research-data-sources 3.4KB；默认预算 32768B → **daily-report 单域即超预算**，命中即降为占位行（标题 + read 路径）。带标签文档 17 份才是注入**条数**上界。

### R-4 2026-11-06【Medium】标签扩展名碰撞 / 多域重叠 / 覆盖率口径
- **状态**：已采纳（design D3 原则改写）

- 扩展名碰撞：`X.ts` 信号会命中 `X.tsx` 路径 → 单文件信号只在无法用目录表达时使用，且必须写完整文件名。
- 多域重叠是**合法结果**（topicgraph 同属 daily-report / semantic-board / topic-graph），不算误匹配；盘点输出单列「合法多域重叠」段，避免被误当误命中。
- 覆盖率分母 = **脚本内固定的样本清单长度**（不是所有一级目录），保证改前/改后两次盘点可重复对账。

### R-5 2026-11-06【Medium】7.3 路径写法不命中（原路径为目录/占位写法）
- **状态**：已采纳（换成 5 条真实存在的路径）

验收路径改为 `front/app/features/articles/components/ArticleCardView.vue`、`front/app/pages/tags/`、`front/app/features/discovery/`、`front/app/features/tags/components/`、`backend-go/internal/platform/airouter/embedding.go`，按输入顺序逐条等于 reading / semantic-board / discovery / semantic-board / ai-summary。

### R-6 2026-11-06【Medium】生命周期边界未闭合
- **状态**：已采纳（design D2 表 + spec 四条场景 + smoke 五条断言）

补齐：主会话 compact 重发；**无 `before_agent_start` 的子线程 compact 不重发**（该事件在子线程 SDK 不存在，不伪造通道）；同进程 fork 继承 `jitDocHits` 与指纹 → 父已送达不重投；legacy 通道不即时 steer、下一 turn 落 system prompt；reload / resume 状态重建 → `jitDocHits` 清零。

### R-7 2026-11-06【Medium】smoke 验收不可执行
- **状态**：已采纳（退出码 0 + stdout 含 `SMOKE OK`）

验收命令改为 `bash .pi/extensions/tests/run-smoke.sh` → **退出码 0** 且 stdout 含 `SMOKE OK`；不使用「管道后接 grep/tail」判定（会掩盖退出码）。

### R-8 2026-11-06【Low】快照键 `null|null`
- **状态**：已采纳（`none|none`）

`stableSnapshotKey(mode, boundChange)` 实参为 null 时返回 `none|none`（不是 `null|null`）；实现期 smoke 断言按 `none|none` 口径。

## 实现期新发现

### R-9 2026-11-06【Medium】子线程/fork 用例在仓库根 cwd 跑会写真实事件库
- **状态**：已修复（改跑 tmp 夹具 cwd + 清理误写行）

实现初版把「子线程自愈 / fork 不重投」用例放在仓库根 cwd 且带 `sessionManager` → `logEvent(ctx.cwd, …)` 落进 `.pi/harness/events.db`（真实事实库），违反「测试用临时 DB/夹具」约束。已把这批用例（含 tier 记账断言）改到 tmp 夹具 cwd（`unbound-dom.md` 夹具域），并删除误写的 12 行（`session_id LIKE 'ci-smoke-unbound%'`）；真实库现无测试残留（`ci-smoke%` 计数 0）。

### R-10 2026-11-06【Medium】孤儿标签不止任务清单里的两条
- **状态**：已修复（信号全部指向真实路径，smoke 断言绿）

盘点时又发现两条信号指向已删路径：`reading.md` 的 `front/app/composables/useReadingTracker.ts`（真实位置 `front/app/features/preferences/composables/`）、`topic-graph.md` 的 `front/app/features/tags/components/BoardThreadBrowser.vue`（文件已删）。前者去掉（已被目录信号覆盖），后者换成 `front/app/features/tags/components/TopicGraphMergeDialog.vue`。现在 10 篇 flow 文档的全部信号均 `existsSync` 为真。

### R-11 2026-11-06【Low】覆盖盘点脚本的域名拼接按字节替换会切坏 UTF-8
- **状态**：已修复（`tr ',' '、'` 改 `sed 's/,/、/g'`）

`tr` 按字节处理，`、`（3 字节 UTF-8）被拆成乱码，矩阵表读不出域名。改用 `sed` 整串替换。

### R-12 2026-11-06【Low】覆盖盘点矩阵的「预期未覆盖」负样本原会被计成失败
- **状态**：已修复（负样本单列，不入覆盖率分子、不计失败）

盘点脚本初版把「零命中」一律计未覆盖失败 → 无法表达负样本（期望不被任何域覆盖的路径）。改为：期望为空且命中为空 → 标「➖ 预期未覆盖」，单列计数、不入分子、不算失败；期望非空而零命中仍算未覆盖失败。

## 任务 2.1 / 2.4 / 7.3 证据（盘点改前改后对账）

盘点脚本：`scripts/harness/constraint-coverage.sh`（只读；样本清单固定在脚本内，49 条）。同一份样本清单，分别对**改前标签**（`git show HEAD:docs/reference/flow/*.md` 落到临时目录）与**改后标签**跑盘点：

| | 覆盖 | 未覆盖 | 期望不符 |
| --- | --- | --- | --- |
| 改前（HEAD 标签） | 14 / 49（28.6%） | 33 | 2 |
| 改后（本 change 标签） | **49 / 49（100%）** | 0 | 0 |

改前未覆盖的 33 条集中在四类缺口（与 design D3 预判一致）：

- 后端业务包无标签：`platform/aisettings/`、`platform/notification/`、`platform/scheduler/`、`platform/ws/`、`admin/repository/`；
- 前端业务辖区无 flow 标签：`front/app/features/{ai,articles,discovery,feeds,preferences,search,shell,tags}/`、`front/app/pages/*`、`front/app/api/{discovery,scheduler,dailyReports,datasources,aiAdmin}.ts`（改前只命中 `standard/frontend/layout.md`，属「非业务域」）；
- 改前 2 条「期望不符」= `admin/handler/scheduler_handler.go` 与 `ai_handler.go` 被 `reading.md` 的**目录级**信号 `backend-go/internal/admin/handler/` 一网打尽（改后 reading 收窄为 `preferences_handler.go` 单文件信号）。

**5 条验收路径的命中来源**（`constraint-digest.sh --paths`，改后；顺序与输入一致）：

```
front/app/features/articles/components/ArticleCardView.vue → docs/reference/flow/reading.md（信号：front/app/features/articles/）
front/app/pages/tags/ → docs/reference/flow/semantic-board.md（信号：front/app/pages/tags/）
front/app/features/discovery/ → docs/reference/flow/discovery.md（信号：front/app/features/discovery/）
front/app/features/tags/components/ → docs/reference/flow/semantic-board.md（信号：front/app/features/tags/）
backend-go/internal/platform/airouter/embedding.go → docs/reference/flow/ai-summary.md（信号：backend-go/internal/platform/airouter/）
```

同一次取数另有 standard 文档命中（`standard/frontend/layout.md` × 4、`standard/backend/ai-logging.md` × 1）——standard 文档同走 JIT（不限 flow），不计入业务域集合。

**任务 1.4 「不必改」清单只读核对结论**（逐项确认后**未改**）：

| 项 | 结论 |
| --- | --- |
| `planInjection` 的 `jitDocHits` 追加（~L1075） | 本就**无 mode gate**（`for (const e of state.jitDocHits.values())` 无条件追加 `reason: "jit-path"`）——缺的只是写入端与投递端两处 gate，不改 |
| `splitPlanItems`（~L1585） | `jit-path` 本就归动态层，不改 |
| `applyBudget`（~L825） | 降级顺位（keyword → jit-path → change-file → declaration）本就含 jit-path，无档会话同样受 `budgetBytes` 约束，不改 |
| `bindAction` / `resolveBindAction` | 无档不绑 change 的判定本就靠 `state.mode` 外层 gate（该 gate 未动），不改 |
| `stableSnapshotKey(null, null)` | 返回 `none|none`（非 `null|null`），无档 JIT 不改快照键 → 稳定层字节恒定，不改 |
| legacy 落 system prompt 路径（~L2050） | legacy 分支本就「每 turn 全量进 system prompt」，无档 JIT 计划在下一 turn 随计划回 system prompt，不改 |
