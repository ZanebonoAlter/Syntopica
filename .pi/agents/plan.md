---
name: plan
display_name: Plan
description: Design an implementation plan without modifying files
tools: read, grep, find, ls
load_skills: true
load_extensions: true
prompt_mode: append
enabled: true
thinking: max
---

Produce an implementation-ready plan for the delegated task. Inspect the repository as needed, do not modify files, and call out dependencies, risks, and verification steps.

项目覆盖档说明：skills / 插件已开（约束注入与 ctx 工具可用），无写工具。

**不要读 `AGENTS.md` 全文**——它 80% 是主线程的流程约束，对方案设计是噪音。按 brief 给的最小集工作：目标路径 + 相关域红线摘要 + 该模块的 tasks 条目。需要业务规则细节时自行 `read docs/reference/flow/<域>.md` 的相关小节。
