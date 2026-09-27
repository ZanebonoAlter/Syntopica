# Design: harden-constraint-injection-channel

## Context

见 proposal.md「Why」。摘要：现状在 `before_agent_start` 每 turn 重建注入块并**追加进 system prompt**（`return { systemPrompt: event.systemPrompt + block }`），存在两类实证缺陷——①system prompt 任何字节变化（含尾部追加）使其后全部 history 前缀缓存失效；②多 change 并行下绑定污染（隐性绑定无记账 / 抢绑横跳 / 关键词跨域误伤）。

约束条件（决定方案边界）：

- pi 的 `before_agent_start` 返回值支持 `{ systemPrompt, message }`：`message` 为持久化消息（存入 session、进 LLM 上下文、**追加顺序**），`systemPrompt` 为替换式。
- 消息追加是 append-only → 前缀缓存友好；system prompt 替换 → 其后全部历史缓存失效。
- 消息参与 compaction（会被摘要），system prompt 不参与 → 抗压缩性只在 system prompt。
- `pi.sendMessage(msg, { deliverAs: "steer", triggerTurn })`：streaming 时在当前 assistant turn 工具调用间隙送达（`triggerTurn:false` 不额外触发 turn），idle 时 `triggerTurn:true` 才触发新 turn。
- harness 事实库 `.pi/harness/events.db` 为 append-only 事件账本，`mode.set` / `constraint.inject` 语义变更属 `harness-fact-log` capability。

## Goals / Non-Goals

**Goals:**

- system prompt 在档位生命周期内字节恒定（新内容一律走追加通道），消除约束注入引起的 history 缓存失效。
- 多 change 并行下绑定确定、可归因：任何绑定变化都有 `mode.set` 记账，read 不抢绑，同 turn 绑定锁定。
- 关键词命中的注入范围与当前 change 域相关（声明域 ∪ 栈相关 ∪ 索引），跨域词不误拉。
- 红线层（最高遵从度内容）仍每 turn 在场且抗 compaction。
- 记账语义与新的投递节奏一致（送达时记、不重复记）。

**Non-Goals:**

- 不改 pi 本身、不改 quality-gate / spec-gate 等其他扩展（它们已是事件驱动 steer 模式）。
- 不引入语义检索/向量匹配来决定注入（关键词命中仍是词表机制，只收窄范围）。
- 不解决 steer/消息在长会话中的历史累积（受 pi 无删除消息能力限制，见 Risks）。
- 不做存量事实库数据迁移（append-only，历史行不重写）。

## Decisions

### D1 通道分层边界：稳定层 = 「档位生命周期内字节恒定」的内容

**决策**：稳定层（system prompt）= 索引文档 + mode-base 基础文档 + **声明域红线层**。动态层（追加消息）= 关键词命中全节 + JIT 命中全节 + change 级文件（explore-findings / 词汇表）+ 稳定层差异通知。

**理由**：分层的判据不是「重要程度」而是**变化频率**与**抗压缩诉求**的交集。

- 红线层是「必须每 turn 在场、且整段生命周期稳定」的内容 → 放 system prompt：既保住最高遵从度与抗压缩，又因内容恒定而不产生缓存失效（只在档位切换时变一次）。
- 关键词/JIT 全节是「会话中途高频变化」的内容 → 放追加通道：变化不再触碰 system prompt，缓存零失效。
- change 级文件（findings 会被 pin_finding 持续追加）同样高频变化 → 追加通道。

**备选与否决**：

- 全量走 steer 消息（最初设想）——否决：红线层移出 system prompt 后①每 turn 在场性靠历史消息承载，长会话注意力衰减；②compaction 后红线丢失需频繁重发。红线是「管知道」里最不能掉的一层。
- 全量留 system prompt（现状）——否决：正是本 change 要修的缓存问题。

### D2 稳定层快照缓存：key 化记忆化，内容不变则不重算不替换

**决策**：维护模块级 `stableSnapshot = { key, block }`。`key = hash(mode, boundChange, 声明域列表, 各稳定层文档内容 hash)`。`before_agent_start` 时先算 key：

- key 未变 → 直接复用 `block`（**不读文件、不重算**），返回值与上 turn 字节一致 → 无缓存失效；
- key 变化（档位切换 / 绑定修正 / 声明域变更 / 文档内容变更）→ 重算 stable 层并整体变化一次；
- 声明域**红线层重解析结果**若与快照不同但 key 判定为「同一档位生命周期」（如 proposal 编辑新增一域）→ 走 D3 动态层差异通知，**不动 stable 快照**（stable 保持激活时刻语义，避免 system prompt 反复变）。

**理由**：缓存失效只允许发生在「档位生命周期切换」这一个低频事件上；proposal 编辑等中途变化不应反复触碰 system prompt。差异走追加通道既保缓存又保信息不丢。

### D3 动态层 diff 指纹 + 双投递通道

**决策**：维护 `sentFingerprints: Map<itemKey, contentHash>`（itemKey = 文档路径 + 注入形态）。计算「当前应送达集合」后与已发送指纹比：

- 增量 = 新增 item 或 contentHash 变化的 item；
- 增量非空 → 投递（只发增量条目，不发全量）；
- 增量空 → **零投递**（不产生任何消息）。

投递通道按触发时机二选一：

- **turn 起点**（`before_agent_start`，关键词命中/findings 更新/compact 快照）→ 在 handler 返回值里带 `message`（pi 契约：持久化消息、追加进 session、进本 turn 上下文）；
- **turn 中途**（`tool_execution_start` 的 JIT 命中）→ `pi.sendMessage(msg, { deliverAs: "steer", triggerTurn: false })`，在当前工具调用间隙送达（语义正是「继续编辑相关文件前拿到约束」），且不额外触发 turn。

两条通道共用同一指纹状态与同一 `renderDynamicMessage(items)`，避免重复投递；投递成功即写入指纹。

**理由**：`before_agent_start` 返回 `message` 是 append-only 投递，天然缓存友好；JIT 需要「编辑动作发生时的即时性」，只有 `sendMessage` + `steer` 能在 turn 中途送达。

**风险与实测点**：`before_agent_start` 返回值中的 `message` 是否在**本 turn** 的首次 LLM 调用即生效，需实现时实测；若不生效，退化方案为在该 handler 内改用 `pi.sendMessage(..., { deliverAs: "steer" })`（消息仍在 append-only 通道，缓存收益不变，仅时序晚一拍）。

### D4 compaction 补偿：合并到下一注入时机重发快照

**决策**：监听 `session_compact`，置 `pendingSnapshot = true`。下一次 `before_agent_start` 时，若 `pendingSnapshot`：计算快照（稳定层摘要 + 动态层当前有效集合），**无视指纹**投递一条消息（payload 记账附 `source:"compact-resend"`），然后重建指纹（`sentFingerprints` 重置为「快照已发送」状态），清除 `pendingSnapshot`。

**理由**：消息会被 compaction 摘要掉（红线虽在 system prompt 安然，但动态层细节会丢）；快照重发让模型在压缩后重新拿到当前有效约束集合。选择「合并到下一注入时机」而非 compact 事件里立即发，避免打断 compact 流程与产生额外 turn。

**备选**：在 `session_before_compact` 里把约束块并入 summary（自定义 compaction）——否决：耦合 pi 压缩实现细节，且 summary 长度受限。

### D5 抢绑条件化 + turn 绑定锁定（纯函数化判定）

**决策**：抽出纯函数 `resolveBindAction({ tool, path, currentBound, boundExists }) → "noop" | "rebind"`：

- `tool === "read"` → `noop`（读其他 change 属参考性访问）；
- 路径不命中 `openspec/changes/<name>/` → `noop`；
- 命中且 `currentBound` 为空或 `boundExists === false` → `rebind`（兜底绑定，`source:"edit-dir"`）；
- 命中且当前绑定健康 → `noop`（**不抢绑**）。

turn 锁定：模块级 `turnBindLocked: boolean`，在 `before_agent_start` 重置为 false；turn 内首个生效的绑定变化（命令/skill/edit-dir/兜底）后置 true，同 turn 后续绑定请求一律 `noop`。bind 判定与锁定组合成另一个纯函数 `shouldApplyBind(action, locked)`，便于 smoke 直跑。

**理由**：把「读参考」与「写入」区分开，消除 s=01a0aabf 一毫秒 4 连绑的竞态（并行工具调用完成顺序不定 → 绑定非确定）。turn 粒度的确定性优先于「即时切换绑定」的便利：需要切换的正当场景（连续处理另一 change）会在下一个 turn 首事件完成。

**权衡**：多 change 轮转的同一 turn 内若要连续处理两个 change，绑定不会中途切换——这是有意交换（确定性 > 同 turn 灵活性）；跨 turn 自然恢复。

### D6 记账：送达时记 + source 字段 + 去重

**决策**：

- `logModeSet(cwd, sessionId, mode, boundChange, source)`——`source ∈ {command, skill, edit-dir, recover, inherit, fallback}`，**所有**绑定变化路径调用（含恢复、兜底、继承），消除隐性绑定。
- `constraint.inject` 改为**投递成功时**按条目记（稳定层：快照重算投递时；动态层：消息实际发出时；compact 快照：附 `source:"compact-resend"`）。同 session 内相同 `{path, mode, reason, bytes}` 未变化则不重复记（以指纹状态为准，天然去重）。
- `pin.read` 保留原「(sessionId|title) 会话内去重」语义。

**理由**：记账语义对齐真实投递事件；事实库瘦身（现状每 turn 全量重复记）。

### D7 关键词命中域限定

**决策**：`matchKeywordDocs` 增加 `allowedDocs` 过滤集 = 声明域 flow 文档 ∪ 栈相关条件文档 ∪ 索引文档；命中项不在集合内则跳过（不写入粘性集合）。词边界匹配与「只增不减」粘性集合语义保留。

**理由**：修 2026-09-16 实测的跨域误伤（聊 harness 机制含 "discovery" 误拉 ~8KB discovery.md）。声明域是 change 自己声明的相关域，栈相关文档是当前编辑栈的必读文档——两者之外的关键词命中与本 change 无关。保底：索引文档始终注入，模型可自行 `read` 补取（注入块已含取回指引）。

**权衡**：削弱「跨域联想」能力（做 ai-summary 时聊到 scheduler 词不再自动拉 scheduler 约束）。判断：跨域约束的正确获取途径是显式 `read` 或声明域，而非对话词碰撞的隐式推断——后者正是污染源。

### D8 配置与回退开关

**决策**：新增配置（`.pi/constraint-injection.json`，缺省即启用新行为）：

- `channel`：`"split"`（缺省，混合通道）/ `"legacy"`（旧的每 turn system prompt 全量，应急回退）；
- `dynamicDisplay`：动态层消息 TUI 展示控制（`"compact"` 缺省 / `"full"`），映射到 `message.display` 与正文精简。

**理由**：万一新通道在真实 provider 上出现兼容性问题，一条配置即可回退，无需改代码；也便于 A/B 观察缓存收益。

## Risks / Trade-offs

- **[R1] `before_agent_start` 返回 `message` 的本 turn 时序未验证** → 实现首步实测（task 含最小验证）；不生效则退化到 `pi.sendMessage(steer)`（缓存收益不变）。
- **[R2] 动态层消息在长会话中累积**（pi 无删除消息能力，每次 findings 更新追加一条）→ 缓解：只发增量条目 + budgetBytes 预算约束 + 指纹去重（稳态零消息）+ compaction 自然压缩。残留影响：极长会话中动态层历史体积增长，属可接受代价（换取 system prompt 恒定）。
- **[R3] 红线层在 system prompt，档位切换仍失效一次** → 接受：档位切换是低频事件（一会话 1~2 次），且切换后整个生命周期恒定；这是「保遵从度 + 抗压缩」的必要成本。
- **[R4] 关键词域限定削弱跨域自动联想** → 接受（见 D7 权衡）；索引文档 + 取回指引兜底。
- **[R5] turn 锁定使同 turn 无法切换绑定** → 接受（见 D5 权衡）；跨 turn 恢复，命令/提及可显式切换。
- **[R6] 记账语义变更影响既有查询脚本/技能文档**（`constraint.inject` 频次下降、`mode.set` 新增 source）→ 缓解：`harness-facts` skill 与 `docs/research/harness事实库.md` 同步更新；查询契约（枚举值）保持兼容（新增字段不删旧字段）。
- **[R7] 稳定层快照 key 计算开销**（每 turn 读需 hash 的文档）→ 缓解：key 只需对稳定层文档（索引 + baseDocs + 声明域）做 mtime+size 快速判定或短 hash，文档数量小（个位数）。

## Migration Plan

1. **部署**：更新 `.pi/extensions/constraint-injection.ts` + 配置缺省 `channel:"split"`；无需数据库迁移（事实库 append-only，旧行保留、TTL 自然清扫）。
2. **回退**：配置 `channel:"legacy"` 恢复旧行为；`mode.set` / `constraint.inject` 的新字段对旧消费方向后兼容（新增不删旧），无需回滚数据。
3. **观察**：部署后看事实库 `constraint.inject` 频次是否显著下降（每 turn 重复消失）+ 会话中 system prompt 是否稳定（可用 harness 事件与人工观察 widget）。
4. **文档同步**：AGENTS.md「pi 扩展全景」表 constraint-injection 行、`harness-facts` skill（事件语义）、`docs/research/harness事实库.md`。

## Open Questions

- 动态层消息在历史中的**保留策略**（是否需要定期「压缩为一条摘要消息」以控制体积）——留给观测数据决定，不影响本期实现与 spec。
- `dynamicDisplay:"compact"` 的具体文案形态（一行摘要 vs 折叠块）——实现时按 TUI 渲染效果定，属表现层细节。
