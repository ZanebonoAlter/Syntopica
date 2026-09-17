<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

`harden-constraint-injection-channel`（2026-09-16 归档）治好了**同一会话内**的隐性绑定与抢绑横跳，但它假设「绑定状态属于会话」。实际上绑定状态是**进程级全局**：`constraint-injection.ts:141` `let currentMode` / `:143` `let modeBoundChange`（连同 `turnBindLocked:177`、`recentInputs:145`、`jitDocHits:147`、`keywordDocHits:150`、稳定层快照/指纹）。`session_id` 只用于 `mode.set` 记账与 `recover` 查询，注入归属、快照 key、`declaration` 域选择全部读全局 → **多会话共享一个 pi 进程时互相串味**。

事实库取证（2026-09-17，`constraint.inject` / `mode.set`）：

| 时间 | 会话 | 事件 | 归属 change |
| --- | --- | --- | --- |
| 16:02:56 | `01a0aaf4`（本会话） | inject | `linux-native-dev-environment` ❌ |
| 16:11:32 | `01a0aaf4` | pin.write | `heal-dangling-article-refs` ← 真正在做的事 |
| 17:09:58 | **另一会话** `01a0ab11` | mode.set（source=edit-dir） | `dedupe-rss-articles` |
| 17:10:04 | `01a0aaf4` | inject（**+6 秒**） | `dedupe-rss-articles` ❌ + 注入其声明的 `content-enrichment`/`topic-graph` 红线层 |
| 23:50:17 | `01a0aaf4` | inject | `harness-retro-loop` ❌ |

`01a0aaf4` **全程零 `mode.set`**（无自身绑定），归属却随他会话游走；另有 10+ 会话「有注入、零 mode.set」。同形污染在 2026-09-17 本 change 创建期间再次现场复现（本会话上下文里出现 `failover-dead-proxy-to-direct` 的 explore-findings）。

后果：①注入**别人的域约束、漏自己的**；②稳定层快照 key 用全局绑定 → 他会话绑定变化刷新本会话快照（前缀缓存失效）；③`jitDocHits`/`keywordDocHits`/`recentInputs` 全局 → 跨会话关键词与 JIT 互相污染；④`turnBindLocked` 全局 → 他会话工具调用锁住本会话绑定时机。故「隐性绑定不存在」的承诺在**跨会话**维度不成立。

## What Changes

- **会话作用域状态容器**：按 `sessionId` 分桶的 `Map<string, SessionState>`（无 `sessionId` 的 stub/烟测语境走单例兜底 key，保持既有行为），承载 `mode` / `boundChange` / `turnBindLocked` / `recentInputs` / `jitDocHits` / `keywordDocHits` / 稳定层快照与指纹 / `pendingSnapshot`。
- **所有读写点改走会话 state**：`before_agent_start` / `input` / `tool_execution_start` / `session_start` / `session_compact`，注入归属与 `declaration` 域选择只读本会话 state；**绝不使用他会话的绑定**。
- **生命周期语义显式化**：`startup`/`new` 清零新建；`fork`/`resume`/`reload` 仅从**同 sessionId 或父会话**显式继承并记 `mode.set source=inherit`；他会话状态永不成为来源。
- **有界回收**：会话条目数上限 + 最近使用淘汰（防长跑进程 Map 无界）；extension rebind 时清空容器。
- **smoke 回归**：新增跨会话隔离用例（两会话交叉绑定、绑定变化不刷新他会话快照、fork 继承只认父会话、命中集隔离、GC 边界）。
- 不改动：注入内容选择算法、域限定规则、混合通道协议与 `legacy` 回退语义（legacy 同样按会话隔离）、`.pi/harness` 记账 schema。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `constraint-injection`：「档位识别与 change 绑定」Requirement 增加**会话作用域**不变量（绑定状态、注入归属、快照 key 按会话隔离；无自身绑定时不得借用他会话）；「混合注入通道」Requirement 增加命中集/指纹/待发快照按会话隔离的约束。
