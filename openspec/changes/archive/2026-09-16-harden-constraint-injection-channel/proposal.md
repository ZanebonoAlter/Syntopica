<!-- ui-impact: none -->
<!-- complexity: complex -->

# Proposal: harden-constraint-injection-channel

## Why

constraint-injection 目前在 `before_agent_start` 每 turn 重建注入块并追加进 system prompt，有两类实证问题：

1. **前缀缓存破坏**：LLM 请求前缀 = [system prompt, history...]，system prompt 尾部追加仍使之后全部 history 缓存失效（代码注释「只增不减保前缀缓存」只保护了 system prompt 内部，保护不了 history）。档位切换、关键词/JIT 新命中、agent 编辑 proposal/findings 都触发失效——实现档长会话失效十次八次不稀奇，每次全量 KV 重算。
2. **多 change 并行注入污染**（事实库 2026-09-16 取证，见 explore-findings）：10 个活跃 change 并行下，①隐性绑定无记账（会话 s=01a0aabd 被静默绑上他窗口刚激活的 dedupe-rss-articles，全套约束+findings 灌入且零 mode.set 记录，来源不可考）；②抢绑横跳（s=01a0aabf 一毫秒内 4 连绑——并行工具调用同时碰多个 change 目录，read/write/edit 无条件抢绑，非确定）；③关键词跨域误伤（对话聊到 "discovery" 即全节注入 ~8KB discovery.md，与本 change 域无关）。

## What Changes

- **注入通道分层（混合架构）**：
  - 稳定层（索引 + mode-base + 声明域红线层）保留 system prompt 注入——档位生命周期内字节恒定，只在档位切换时失效一次；
  - 动态层（关键词命中全节、JIT 命中全节、change 级文件 explore-findings/词汇表）改走 steer 消息（`pi.sendMessage` + `deliverAs:"steer"`），事件驱动 diff 发送（内容 hash 变化才发），不再进 system prompt；
  - `session_compact` 后重发一次当前约束快照（补偿 steer 消息被 compaction 摘要）；
  - steer 消息 `display` 受控渲染，避免 5-10KB 约束块在 UI 刷屏。
- **绑定污染修复**：
  - 所有绑定变化路径（命令/skill/写目录/恢复/继承/mtime 兜底）强制 `mode.set` 记账，payload 增加 `source` 字段（command/skill/edit-dir/recover/inherit/fallback），隐性绑定可归因；
  - read 不再抢绑；write/edit 命中其他 change 目录仅在当前绑定不健康（null 或 change 目录已消失）时兜底绑上，健康绑定时 SHALL NOT 抢绑；
  - 同一 turn 内绑定锁定（turn 首个绑定事件定绑，后续工具调用不切换），消除并行竞态。
- **关键词命中域限定**：关键词命中文档范围限定为「声明域 ∪ 栈相关 ∪ 索引」，跨域词不再全库匹配误拉。
- **记账语义同步**：`constraint.inject` 从每 turn 重复记改为送达时记（稳定层档位切换记、动态层消息发送记），事实库瘦身；`pin.read` 去重逻辑保留。

## Capabilities

**Modified Capabilities**：

- `constraint-injection`——「每 turn system prompt 强制注入」Requirement 重写为混合通道语义（稳定层 system prompt + 动态层 steer + compact 重发）；「档位识别与 change 绑定」Requirement 增加抢绑条件化、turn 锁定、记账 source；关键词命中域限定。
- `harness-fact-log`——`constraint.inject` 记账时机（每 turn → 送达时）与 `mode.set` 激活来源枚举扩展（+source 字段，恢复/继承/兜底路径记账）。

**New Capabilities**：无。

## Impact

- `.pi/extensions/constraint-injection.ts`（主体重构：planInjection 拆稳定/动态两层、新增 steer 发送与 diff 状态、compact 监听）
- `.pi/extensions/tests/constraint-injection.smoke.cjs`（新增纯函数用例：diff 计算、抢绑条件、turn 锁定、关键词域过滤）
- `.pi/constraint-injection.json`（若需新增配置：steer 消息开关/display 控制）
- `harness-facts` skill 文档与 `docs/research/harness事实库.md`（事件语义变化说明）
- AGENTS.md「pi 扩展全景」表 constraint-injection 行（注入机制描述更新）
- 风险：steer 消息参与 LLM 上下文但会被 compaction 摘要——compact 重发快照补偿；红线层仍在 system prompt 保住遵从度与抗压缩。向后兼容：配置缺省行为 = 混合架构启用，无旧配置迁移需求。
