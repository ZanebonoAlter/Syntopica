
## 多 change 并行注入污染取证与机制

事实库（.pi/harness/events.db）2026-09-16 取证，三个污染根源：

**P1 隐性绑定无记账**（现场：s=01a0aabd 做 ai-health-heartbeat-reprobe，15:02:50 被注入 dedupe-rss-articles 全套——mode-base + declaration 红线 content-enrichment/topic-graph + keyword discovery.md + change-file findings 全文；该 session 全程零 mode.set 记录，绑定来源不可考）。代码位置：
- session_start handler（constraint-injection.ts ~L1164-1210）：reason="startup" 时若 currentMode===null 按同 sessionId 查 mode.set 恢复（不记账）；注释明确 startup 双面孔=①进程冷启动恢复 ②pi-subagents 子线程派发共用模块实例继承主会话档位（继承也不记账）
- planInjection（~L720-740）：analysisChange = bound ?? fallbackChange，implementation 档无绑定时 mtime 兜底（detectActiveChange）不记账不显式——隐性绑定
**P2 抢绑横跳+并行竞态**（现场：s=01a0aabf 于 15:05:10.396~397 一毫秒内 4 连绑 ai-health→add-same-origin→loading-progress-tips→offline-catchup；gate.check/edit.map 归因全程漂移）。代码：tool_execution_start 第三分支（~L1295-1305）对 read/write/edit 命中 openspec/changes/<X>/ 路径无条件 modeBoundChange=X；并行工具调用时最后写入者赢，非确定
**P3 关键词跨域误伤**：对话输入含"discovery"等词即全节注入 discovery.md（~8KB），与当前 change 域无关（matchKeywordDocs 全库关键词匹配，命中源=recentInputs 窗口 5 条）

修复方向：P1 所有绑定变化强制 logModeSet + payload 加 source 字段（command/skill/edit-dir/recover/inherit/fallback）；隐性兜底显式化或移除。P2 read 不抢绑；write/edit 仅当绑定不健康（null/目录消失）时抢；同 turn 锁定。P3 关键词命中限定声明域∪栈相关文档

<!-- pinned 2026-09-16T15:13:05Z -->

## constraint-injection 注入链路基线与 steer 通道事实

**现状注入链路（混合改造的基线）**：
- 挂点 before_agent_start（~L1307-1368）：每 turn 调 planInjection 重建注入块，`return { systemPrompt: event.systemPrompt + plan.block }`——注入块追加在 system prompt 尾部
- 注入块构成分层：①索引（constraints-index.md，未激活档唯一内容）②mode-base 基础文档（档位激活）③声明域红线层（proposal 头 constraint-domains → flow 约束节红线句逐行，extractRedlines，0 条或 <512B 回退全节）④keyword 命中全节（粘性集合 keywordDocHits 只增不减）⑤jit-path 命中全节（粘性集合 jitDocHits，write/edit 路径命中 doc-impact-applies 标签）⑥change 级文件（implementation 档：explore-findings.md + ubiquitous-language.md，digest 阈值可配）
- 预算：applyBudget（budgetBytes 默认值在 config，超限降级 digest/placeholder）
- 记账：每 turn 对全部 docEntries 记 constraint.inject（同 turn 重复 N 次），pin.read 按 (sessionId|title) 会话内去重
- 粘性集合"只增不减保前缀缓存"注释存在认知盲区：system prompt 尾部追加仍使 system prompt 之后的全部 history 缓存失效（前缀=[system,history...]）

**quality-gate steer 模式参照**（quality-gate.ts L241-247/L402-417）：pi.sendMessage({customType, content, display}, {deliverAs:"steer", triggerTurn:true})；deliverAs 三档 steer/followUp/nextTurn；display 可控渲染（消音）；customType 可配 registerEntryRenderer

**pi 事件时序**：session_before_compact / session_compact / session_compact_failed 事件可监听（compaction.md），compact 后可重发约束快照；steer 消息参与 LLM 上下文但会被 compaction 摘要（system prompt 不参与 compaction——抗压缩是现设计选 system prompt 的核心理由）

**目标混合架构**：稳定层（索引+mode-base+声明域红线层，档位生命周期内恒定）留 system prompt；动态层（keyword 全节/jit-path 全节/findings 更新）走 steer 消息事件驱动 diff 发送；session_compact 后重发快照；记账改为发送时记（constraint.inject 去重瘦身）

<!-- pinned 2026-09-16T15:13:15Z -->
