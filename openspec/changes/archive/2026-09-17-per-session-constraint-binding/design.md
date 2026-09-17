# Design — per-session-constraint-binding

## 背景（实测，2026-09-17）

`harden-constraint-injection-channel` 归档后，**跨会话**污染仍在：绑定状态是进程级全局，多会话共用 pi 进程时互相串味。事实库证据（`constraint.inject` / `mode.set`）：

- 会话 `01a0aaf4` **全程零 `mode.set`**（无自身绑定），注入归属却依次是 `linux-native-dev-environment`（16:02）→ `dedupe-rss-articles`（17:10，**另一会话 `01a0ab11` 在 17:09:58 刚绑的**，+6 秒）→ `harness-retro-loop`（23:50）。
- 17:10:04 那次还带上了 dedupe 声明的域（`content-enrichment` / `topic-graph` 红线层）——本会话真正在做的是 `heal-dangling-article-refs`（16:11:32 的 pin.write 可证）。
- 另有 10+ 会话「有注入、零 mode.set」。
- 2026-09-17 建本 change 期间再次现场复现：本会话上下文被注入 `failover-dead-proxy-to-direct` 的 explore-findings。

## D1 现状：哪些状态是「会话语义、全局存储」

`constraint-injection.ts`：

| 行 | 变量 | 语义 | 现状 |
| --- | --- | --- | --- |
| 141 | `currentMode` | 本会话档位 | 模块级全局 |
| 143 | `modeBoundChange` | 本会话绑定的 change | 模块级全局 |
| 145 | `recentInputs` | 本会话最近输入窗（关键词/栈命中源） | 全局 |
| 147 | `jitDocHits` | 本会话 JIT 命中集 | 全局 |
| 150 | `keywordDocHits` | 本会话关键词命中集 | 全局 |
| 177 | `turnBindLocked` | 本会话 turn 内绑定锁 | 全局 |
| — | 稳定层快照 / 指纹 / `pendingSnapshot` / `pinReadSeen` | 本会话投递状态 | 全局（`resetChannelState()` 整体重置） |

读写点（迁移清单，实现时逐处改为走会话 state）：`1549-1588`（startup/resume/reload 恢复与清零）、`1599-1601`（输入入窗）、`1605-1623`（命令绑定）、`1644-1656`（skill 绑定）、`1656-1690`（JIT 命中 + 绑定修正）、`1695-1701`（JIT 即时投递）、`1724-1753`（turn 收尾 / 绑定健康检查 / mtime fallback）、`1790`（稳定层快照 key）、`1946-1951`（declaration 域选择）。

## D2 目标状态模型

```ts
type SessionState = {
  mode: Mode | null;
  boundChange: string | null;
  turnBindLocked: boolean;
  recentInputs: string[];
  jitDocHits: Map<string, DocEntry>;
  keywordDocHits: Map<string, DocEntry>;
  pinReadSeen: Set<string>;
  channel: ChannelState;      // 稳定层快照/指纹/pendingSnapshot
  lastUsedAt: number;         // 淘汰用
};
const states = new Map<string, SessionState>();
const FALLBACK_KEY = "__no-session__"; // 无 sessionId（smoke stub / 非真实会话）
function stateFor(ctx): SessionState;  // sessionId 缺失 → FALLBACK_KEY（保持既有单例行为）
```

要点：`stateFor` 是**唯一**入口；`sessionId` 取 `ctx.sessionManager?.getSessionId?.()`（既有取法）。历史 `mode.set` 的 `recover` 查询仍按 sessionId 走事实库（不变）。

## D3 生命周期与继承规则（本次修复的核心）

| 场景 | 行为 | 记账 |
| --- | --- | --- |
| 会话有自身 state | 用自身的 | 无 |
| `startup`（冷启动/重启恢复） | 按**同 sessionId** 的 `mode.set` 历史恢复（既有 recover 语义）；无记录 → 未绑定 | `source=recover`（不变） |
| `resume` / `reload` | 同上（同 sessionId 单段恢复） | `source=recover`（不变） |
| `new` | 只清本会话条目，新建空 state | 无 |
| `fork` / pi-subagents 子线程（同一模块实例、`reason=startup` 但**不是**冷启动） | **显式继承父会话 state**（快照注入需带完整约束块，既有 @bugfix 语义必须保住） | `source=inherit`（新增） |
| 无自身 state 且**无法确定父会话** | **未绑定（仅索引）**；MUST NOT 借用任意他会话 state | 无（不记账 = 没有隐性绑定） |

**父会话如何确定（实现前置实测，见 tasks 1.x）**：优先 `ctx.sessionManager` 暴露的父/来源标识（session file 路径形如 `<parent-session-dir>/forks/<ts>_<parentId>-*.jsonl`，可从路径反解父 sessionId）；拿不到则退回**显式注册表**——`subagent` 工具调用事件（`tool_execution_start` 的 `toolName === "subagent"`）时把当前会话 state 登记为「待继承」，子会话 `startup` 时认领。

**关键差异（与现状）**：现状是「无 state 就沿用全局值」＝借任意他会话；新规则是「只在能证明父子关系时继承」，其余一律未绑定。

## D4 回收与内存有界

- 条目上限（默认 32，可配）+ LRU 淘汰（`lastUsedAt`），淘汰时不做额外副作用。
- extension rebind（`session_start` 的 new/resume/fork/reload 分支）只重置**本会话**条目；`states` 容器在模块重新加载时天然重建。
- 单测/烟测语境的 `FALLBACK_KEY` 条目同样参与 LRU。

## D5 必须保住的不变量（回归红线）

1. **稳定层字节恒定**：同一会话同一 `mode|boundChange` 生命周期内快照字节不变（快照 key 现含会话维度后，他会话绑定变化不再刷新本会话快照——这既是修复也是缓存收益）。
2. **turn 内绑定锁定**：`turnBindLocked` 语义按会话独立（他会话工具调用不再影响本会话绑定时机）。
3. **子线程继承**：apply 中派子线程后主会话档位/命中集不受影响，子线程能拿到完整约束块（既有 @bugfix，smoke 已覆盖，必须继续绿）。
4. **无 sessionId 语境**（烟测 stub）：行为与现状等价（单例兜底）。
5. **legacy channel**：`channel:"legacy"` 回退路径同样按会话隔离（不额外破坏）。
6. **`mode.set` 记账**：所有绑定变化仍全量记账，新增 `source=inherit` 取值（spec 枚举补齐）。

## D6 风险与回滚

| 风险 | 处理 |
| --- | --- |
| 父会话识别不可靠 → 子线程继承失效（apply 中派子线程拿不到约束块） | 实现前**先实测** pi-subagents 子线程可见的 ctx 字段；拿不到就启用「派发即登记」回退；smoke 覆盖 fork 继承用例 + tasks 真实会话观测（9.x） |
| 多 pane 共享同一 sessionId（用户开两个视图看同一会话） | 视为同一会话，共享 state 是**期望**语义 |
| 长跑进程 Map 增长 | D4 LRU + 上限 |
| 污染没修干净（仍有借用路径） | 验收用事实库口径：观测窗口内每个会话的 `constraint.inject.change` 必须等于该会话自身最近一次 `mode.set.boundChange`（无 mode.set 的会话则必须为 `null`） |

**回滚**：`.pi/extensions/` 单文件改动，`git revert` 该提交即可；无 schema/数据迁移。

## 不做

- 不改注入内容选择算法、域限定规则、混合通道协议与消息形状。
- 不改 `.pi/harness/events.db` schema（`source=inherit` 是既有 payload 字段的新取值）。
- 不引入跨进程一致性（多 pi 进程各自独立，本就无共享）。
