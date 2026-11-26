<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

`add-dispatch-constraint-digest`（2026-11-04 归档）把「派发前取红线」从一条没有方法的 MUST 变成「一条命令 + 固定 brief 格式」，并在 design D7 留下观察窗口与判据：**present/(present+absent) ≥ 70% → 规范引导路线成立；< 70% → 另开 change 只放开「无档编辑路径 JIT」最小口子并复测**。

判据现在已经触发：

- `harness-retro` ⑦G 实测 **29%（present 31 / 可判定 107，未知 62）**，可判定派发已 107 次（窗口门槛 30 次）。
- 该 change 的非目标写明「未激活档仍只注入索引」——`planInjection` 的三处闸（`matchText` / JIT / `allowedKeywordDocs`）全部以 `mode` 为开关，`state.mode === null` 时只有 `constraints-index.md` 这一份索引，模型读多少代码都不会自愈。

两个后果：

1. **主会话裸奔**：开发执行规范豁免清单里的 hotfix / 单文件小修 / 纯配置任务天然没有 opsx 档位，主会话改业务代码时拿不到该域红线。
2. **子线程连带缺纸条**：主会话自己不知道域，写 brief 就只能凭空猜或省略（29% 的一部分原因），子线程也没有 `before_agent_start` 通道可自愈。

另有路径反查覆盖不足的结构性因素（实测）：

- 一级目录 36 个中 **21 个无任何标签覆盖**；
- 后端业务相关目录漏网：`backend-go/internal/platform/ws`、`platform/notification`、`admin/repository` 反查均「未命中任何标签」；
- 前端业务辖区（`front/app/features/*`、`front/app/pages/*`）只命中 `standard/frontend/layout.md`（布局规约，非业务域），flow 域标签没有前端辖区；
- 命中失败的路径只能回退 `--domains`（人工补业务域名），而实践中不会有人回退。

## What Changes

- **放开无档（`mode = null`）编辑路径 JIT（最小口子）**：`tool_call` 的 write/edit 命中 `doc-impact-applies` 路径标签时，未激活会话也把对应文档追加进注入计划（动态层 steer）。**关键词匹配 `matchText` 与域限定 `allowedKeywordDocs` 保持 gate 不动**——只开「路径精确命中」这一条零歧义通道，不引入「聊到日报就注日报」的误命中面。
- **JIT 投递 gate 同步放开**：`if (jitAdded && state.mode && ...)` 改为只判 `jitAdded`（否则无档下命中只记 map、不投递，等于白改）。
- **补 `doc-impact-applies` 路径标签**：按「该域业务约束实际管得着的代码」盘点缺口，只补有域红线的真实业务路径，基础设施目录（`cmd/`、`scripts/`、`stores/`、`utils/`、`types/` 等）不硬塞。目标：`constraint-digest.sh --paths` 在业务路径上的命中率可测提升（盘点前后对比进 review 证据）。
- **可观测口径**：`constraint.inject` payload 追加 append-only 字段 `tier`（`none` / `requirements` / `implementation`）——现有 `mode` 字段已表示文档形态（`section`/`digest`/`full`），不可复用；历史记录无 `tier` → 计「未知」并剔除分母。`harness-retro` 增「⑦H 无档 JIT」探针（条数/会话数/平均域数/送达字节/降级占比；不统计原始字节）。
- **spec 不变量修订**：「未激活档仅索引」改为「未激活档**稳定层**仅索引；编辑路径命中的文档 SHALL 以动态层追加注入」。
- **不做**：不改关键词通道、不改 `allowedKeywordDocs` 域限定、不动子线程继承链路（`deliverChildInitStable` / 快照 key）、不改 brief 契约与 `constraint-digest.sh` 输出格式。

## Capabilities

### New Capabilities

无（本 change 不引入新 capability）。

### Modified Capabilities

- `constraint-injection`: 修订「未激活档仅索引」requirement 为「稳定层仅索引 + 编辑路径 JIT 动态追加」，新增无档 JIT 的注入形态（全节）、预算降级顺位与记账场景；并补充「无档不跑关键词/不绑定 change」的边界。

## Impact

| 面 | 影响 |
| --- | --- |
| `.pi/extensions/constraint-injection.ts` | `tool_execution_start` handler 内两处 JIT gate（约 L1949 扫描 `args.path`、L1983 投递）；`constraint.inject` payload 追加 `tier`；`planInjection` 无需改（`state.jitDocHits` 追加本就无 mode gate，仅写入端被挡） |
| `.pi/extensions/tests/` | 新增无档 JIT 用例（命中/未命中/去重/预算降级/子线程自愈）；既有 smoke 回归（历史 2 项预存在红：`semantic-board` / `reading` 标签指向已删路径，台账已登记，不属本 change） |
| `docs/reference/flow/*.md` | 补头部 `doc-impact-applies` 标签（仅前 15 行生效、一文档仅首条标签）；不动「业务约束与不变量」正文；**顺带修复两项孤儿标签**（semantic-board 7 条 / reading 3 条指向已删路径，改后 `test-patrol --resolve` 清账）；`docs/reference/standard/shared/doc-authoring.md` 同步标签语义 |
| `docs/reference/harness/pi-extensions.md` | JIT 通道小节补「无档可注入」语义、观测口径与回滚开关说明 |
| `AGENTS.md` / `docs/reference/开发执行规范.md` | 修正「未激活=仅索引」的旧表述，补无档 JIT 的行为说明（避免规范与实现打架） |
| `scripts/harness/` | 不改 `constraint-digest.sh`（标签补全后 `--paths` 自动受益）；新增 `constraint-coverage.sh`（业务路径→命中域矩阵与覆盖率盘点，只读）；`harness-retro.sh` 增无档 JIT 探针段 |
| 运行时成本 | 无档会话新增动态注入受既有 `budgetBytes`（默认 32768B）约束。实测节字节 3.4–46.3KB：**daily-report（46.3KB）单域即超预算**，命中时按既有顺位降为占位行（标题 + read 路径）；带标签文档共 17 份，注入条数上界按实际编辑路径与去重收缩 |

**回滚开关**：无需新增配置——若观察期发现无档注入噪声过大，把两处 gate 恢复 `state.mode &&` 即回到现状（一行级回滚）。
