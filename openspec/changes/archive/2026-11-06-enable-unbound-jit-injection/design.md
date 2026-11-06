# Design: enable-unbound-jit-injection

## 0. 问题边界（代码事实）

约束注入仍以 `mode` 为总开关，三处闸门与 `add-dispatch-constraint-digest` 归档时一致：

| 闸门 | 定位 | 无档位时行为 |
| --- | --- | --- |
| 关键词命中 | `planInjection`：`matchText = mode ? state.recentInputs.join("\n") : ""`（`.pi/extensions/constraint-injection.ts:980`） | 不扫描任何域关键词 |
| 编辑路径 JIT 写入 | `tool_execution_start` handler：`if (state.mode && (write\|edit))`（~L1949） | 编辑业务代码不记 `jitDocHits` |
| 域限定 D7 | `allowedKeywordDocs`：无 mode 时集合只含 `indexDoc`（~L1044） | 即使关键词命中也被丢弃 |

关键事实（决定改动面）：**JIT 的消费端本来就没有 mode gate**——`planInjection` 里 `for (const e of state.jitDocHits.values())`（~L1075）无条件追加 `reason: "jit-path"`；`splitPlanItems`（L1573）把它归到动态层；`deliverDynamic`（L1695）不看 mode；预算降级顺序里 `jit-path` 是第二顺位（`applyBudget`，L825）。所以「无档 JIT」缺的只是**写入端那一个 gate** 与**投递触发端那一个 gate**。

无档时 `planInjection` 的稳定层仍只有 `indexDoc`（`ordered = [{doc: indexDoc, reason: "index"}]`），JIT 只走动态层；`stableSnapshotKey(null, null)` 返回 `none|none`，快照键与指纹 regime 不因 JIT 改变。

子线程侧：pi-web 子线程 SDK 无 `before_agent_start`，动态层命中计算同样 gated on mode；父为 null 时子线程只能拿到索引，读多少代码都不自愈。放开写入+投递 gate 后，子线程按自己的真实编辑路径即可拿到对应域（不依赖主线程写 brief）。

**事件与参数形状**（审查修正）：JIT handler 挂在 `tool_execution_start`，路径取 `event.args.path`（不是 `tool_call`）。现有 smoke L278-285 即按此形状回放。

## D1 只开「编辑路径」通道，不动关键词

| 方案 | 误命中面 | 覆盖 | 结论 |
| --- | --- | --- | --- |
| 只开编辑路径 JIT | 零（文件系统事实） | 改业务代码的会话 | **采用** |
| 同时开关键词匹配（无档） | 大（聊到「日报」「标签」就注全节，实测曾误拉 8KB） | 纯问答语境 | 不做 |
| 全放开（含 `allowedKeywordDocs`） | 更大（跨域命中） | — | 不做 |

路径标签是**人工维护的文件级声明**（`doc-impact-applies`），命中即「这个文件辖区属于该域」，不存在语义猜测；关键词是对话文本的推测，无档时没有 change 声明域做收敛，放开就是噪音。

## D2 最小改动集与生命周期边界

| 改动 | 位置 | 说明 |
| --- | --- | --- |
| JIT 写入 gate 去掉 `state.mode &&` | `tool_execution_start` ~L1949 | `if (event.toolName === "write" \|\| event.toolName === "edit")` |
| JIT 投递 gate 去掉 `state.mode &&` | 同 handler ~L1983 | `if (jitAdded && cfg.channel !== "legacy")` |
| 注释更新 | 两处附近 + `planInjection` 头部 | 「未激活=仅索引」表述改为「稳定层仅索引 + 编辑路径 JIT 追加」 |

**不改**：`planInjection` 的 `jitDocHits` 追加逻辑（本就无 gate）、稳定层快照键（无档仍 `none|none`）、`bindAction`（无档不绑 change）、`applyBudget` 顺位、`deliverChildInitStable`（子线程稳定层仍只投索引，JIT 走 `tool_execution_start` 即时 steer）。

**生命周期与兼容边界**（审查补充，写进 spec 与测试）：

| 场景 | 无档行为 | 依据 |
| --- | --- | --- |
| 主会话 compact | `session_compact` 置 `pendingSnapshot`，下一 `before_agent_start` 重发索引 + 当前 JIT 集合 | L2165-2174 |
| 无 `before_agent_start` 的子线程 compact | **不重发**（该事件在子线程 SDK 不存在）——已知边界；同文档重复编辑也不重投（去重），丢失内容靠显式 read 或父会话 brief | L1753-1787 注释 |
| 同进程 fork / 子线程继承 | `jitDocHits` 与指纹随 channel 拷贝；子线程首投只投稳定层，父已命中的文档不重投（同文档重复编辑也不重投——去重语义） | L317-339、L1753 |
| legacy 通道 | 无档 JIT **不即时 steer**；下一 turn `before_agent_start` 把含 jit-path 的完整计划放回 system prompt | L2050-2068 |
| reload / resume | 会话状态重建，`jitDocHits` 清零——回滚只需恢复 gate 即可，无需清数据 | `stateFor` 新建 |

**回滚**：把两处 gate 恢复 `state.mode &&`（一行级），无需配置开关；已注入的历史消息不受影响，重载会话后按旧行为走。

## D3 路径标签补全：先量后补，多域重叠是合法结果

现状（`doc-impact-applies` 只扫 flow + standard 文档头 15 行、一文档仅首条标签生效）：

- 一级目录 36 个，**21 个无任何标签覆盖**；
- 真实业务路径漏网（实测）：`backend-go/internal/platform/ws` / `platform/notification` / `admin/repository` 反查均未命中；`front/app/features/*` 与 `front/app/pages/*` 只命中 `standard/frontend/layout.md`（非业务域），flow 域无前端辖区；
- 对照：`backend-go/internal/admin/scheduler/job_daily_report.go` → daily-report + scheduler；`backend-go/internal/topicgraph/repository` → daily-report + semantic-board + topic-graph（机制可用，缺覆盖）；
- 带标签文档共 17 份（flow 10 + standard 7），所以「最多 10 条 steer」的上界说法不成立。

策略：

1. **新增盘点脚本 `scripts/harness/constraint-coverage.sh`**（只读）：输入是**脚本内固定的业务路径样本清单**（显式列出 `backend-go/internal/**` 业务包 + `front/app/features/**` 功能目录），输出「路径 → 命中业务域 / 未覆盖」矩阵与覆盖率；分母 = 样本清单长度（不是所有一级目录），保证可重复对账。
2. **只补「域约束管得着」的路径**：判定标准 = 该目录有 flow 文档「业务约束与不变量」节里能落地的红线。基础设施目录（`cmd/`、`scripts/`、`stores/`、`utils/`、`types/`、`middleware/`、`platform/{config,database,logging,...}`）**不硬塞**。
3. **子串匹配的风险与规则**：匹配是 `p.includes(s)`，因此
   - 目录信号一律带尾斜杠（`backend-go/internal/platform/ws/`）；
   - 文件信号写**更长/更具体**的形式——`X.ts` 会误中 `X.tsx`，反之不会，所以单文件标签只在无法用目录表达时使用，并用盘点脚本的矩阵断言「该路径命中域集合 = 预期集合」；
   - **多域重叠是合法结果**（topicgraph 同时属 daily-report / semantic-board / topic-graph），注入条数 = 命中域数，不算 bug；盘点输出需能看出重叠，避免把重叠误当误匹配。
4. **验收**：改前/改后各跑一次盘点，输出覆盖率与 5 条探针路径的命中域集合，贴 `review.md`；不追求目录数指标，只要求「业务样本中未被任何域覆盖的条目减少，且新增命中经逐条确认」。
5. **顺带清账**：现有两项预存在红（`标签: semantic-board.md 路径真实存在（7 条）`、`标签: reading.md 路径真实存在（3 条）`）就是孤儿标签；本 change 既然改标签，一并修掉并用 `test-patrol.sh --resolve` 清账，避免「先改后豁免」的双标。

## D4 预算与噪声（实测口径）

「业务约束与不变量」节实测字节（`## 节` 原文，UTF-8）：

| 文档 | 节字节 | 文档 | 节字节 |
| --- | --- | --- | --- |
| daily-report | **46.3KB** | semantic-board | 22.4KB |
| data-enrichment | **27.1KB** | reading | 15.4KB |
| discovery | 14.7KB | ai-summary | 12.5KB |
| topic-graph | 8.3KB | content-enrichment | 8.1KB |
| scheduler | 7.4KB | research-data-sources | 3.4KB |

默认 `budgetBytes = 32768`（32KB）。因此：

- **daily-report 单域即超预算** → 无档 JIT 命中它时按既有顺位降级为占位行（标题 + read 路径 + 降级通知），模型需自行 read 补取；data-enrichment 与 semantic-board 在同时命中第二域时同样会降级。
- 单条注入字节**不是 8–13KB**，范围 3.4–46.3KB；量化必须分别记录「原始字节 / 送达字节 / 是否降级」。
- 注入条数上界 = **当前唯一命中的带标签文档数**（现有 17 份），不是消息/事件总数：compact 强制重发、内容或降级结果变化都会再次投递；无档会话实际命中数取决于编辑路径，去重（`jitDocHits`）只保证「同文档首次命中一条」。
- **价值判断**：巨域降级为占位行仍提供「该域有约束，路径在 X，请 read」的信息，比现状（完全没有）有增益；若后续要提升，另开 change 让 JIT 走红线层（1–3KB/域），本 change 不做（最小口子原则）。

## D5 可观测与回检判据

记账字段冲突（审查 High）：`constraint.inject` payload 里 `mode` 已表示**文档形态**（`section` / `digest` / `full`，见 `InjectedDocEntry` L904），不是会话档位，不能复用。

**方案**：`constraint.inject` payload 追加 append-only 字段 `tier`（`"none" | "requirements" | "implementation"`），由约束注入器自报当前会话档位；历史记录无该字段 → 计「未知」并从分母剔除（与 `subagent.dispatch` 的 `constraintDigest` 三态同款口径，不回溯伪造）。

- 无档 JIT 事件特征：`reason = "jit-path"` 且 `tier = "none"`。
- 可能与同会话档位激活后的 JIT 混在同一统计窗，探针按 `tier` 分组，不与有档 JIT 混算。
- `harness-retro.sh` 新增「⑦H 无档 JIT」子组：按日统计注入条数 / 会话数 / 平均域数 / **送达字节**（`bytes` 即送达字节，不统计原始字节）/ 降级占比；compact 重发（`source=compact-resend`）**计入**注入条数但不重复计会话；域名统计只取 `path` 以 `docs/reference/flow/` 开头的条目（standard 文档计为非业务域，不进平均域数）；同一会话同一 `path` 只计一次（去重后算域数）。
- 查询落点：`⑦H` 直接用 sqlite（`.pi/harness/events.db`）聚合（P95 用窗口函数或脚本内排序计算），口径写进段头；`m7.*` 键沿用 retro 现有命名风格；smoke 用 sqlite 夹具断言段输出字段非空。

**回检判据**（窗口 2 周或 ≥20 次「无档会话编辑命中」先到）：

| 指标 | 目标 | 处置 |
| --- | --- | --- |
| 无档会话 JIT 命中会话数 / 无档会话数 | 观测基线（无目标） | 仅记录 |
| 平均命中域数 / 会话 P95 | ≤ 3 | 超线 → 检查是否标签过宽（D3 原则 2） |
| 降级占比（degraded / 全部 jit-path 条） | 记录基线，不设上限 | 若 >50% 且用户抱怨信息不足 → 另开「JIT 红线层」change |
| 标签误命中（review 或用户报告） | 0 | 立即收窄标签 |

## D6 明确不做

- 不改关键词通道、不改 `allowedKeywordDocs` 域限定（D1）。
- 不改子线程继承链路 / 稳定层快照 / `mode.set` 语义。
- 不改 brief 契约 / `constraint-digest.sh` 输出格式（标签补全使其自动受益）。
- 不把 JIT 形态改成红线层（D4 价值判断，留观察窗口）。
- 不引入新的配置项或环境变量（回滚靠代码级 revert）。

## D7 风险与边界

| 风险 | 说明 | 缓解 |
| --- | --- | --- |
| 标签过宽导致噪音 | 子串匹配 + 目录前缀写太粗 | D3 原则 2/3 + 盘点矩阵逐条确认 |
| 文件扩展名碰撞 | `X.ts` 标签命中 `X.tsx` 路径 | 单文件标签尽量避免；矩阵断言预期域集合 |
| 巨域降级 | daily-report 46.3KB > 预算 | 占位行 + 降级通知（D4），数据进 ⑦H |
| 无档会话连续编辑多域 | 上界 17 份文档，实际受编辑路径约束 | D5 判据（P95 ≤ 3 域）+ 去重 + 预算降级 |
| 与旧文档表述冲突 | `doc-authoring.md` 仍写「仅档位激活时生效」 | 同步改该行 + `pi-extensions.md` + 代码注释 |
| 稳定层缓存失效 | 无档时 `mode/boundChange` 均 null → key `none|none` | 不改快照键；smoke 断言无档 JIT 后 systemPrompt 字节不变 |
| 子线程 compact 不重发 | 子线程无 `before_agent_start` | 已知边界（D2 表），不做额外通道 |
| legacy 通道 | 无即时 steer | 保留 `channel !== "legacy"` 判断；下一 turn 放回 system prompt |
