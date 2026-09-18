const TASK = `无人值守文档任务：Syntopica 仓库（cwd /home/zanebono/software/Syntopica）。产出一份**人类便于阅读**的讲解文档：当前项目的 harness 完整约束体系在开发生命周期中如何起作用。配图必须用 **diagram-design 技能**画（用户不可达，无法中途确认——按任务书内的既定决策直接执行，并在文档末尾记录假设与取舍）。

## 画图技能（必须先读再用）
技能文件：/home/zanebono/.pi/agent/git/github.com/cathrynlavery/diagram-design/skills/diagram-design/SKILL.md（references 相对同目录）。先完整读 SKILL.md，再按选型读对应 references。
- **风格已由用户明确指定用默认（explicit default choice）**：按 skill 规则跳过 first-time setup gate 的 onboarding 询问，直接用默认 editorial 皮肤（cool editorial palette），不定制 style guide。
- 三张图选型（可在读 references 后微调，但须在文档里记录理由）：
  1. 开发生命周期 × 各 harness 扩展介入点全景 → **Process**（type-process.md，multi-actor sequential process with handoffs）
  2. 单 turn 内约束注入 + turn_end 门禁增量检查 → **Sequence**（type-sequence.md）
  3. retro 复盘闭环（记账→报告→改进项→改规则→基线回检）→ **Loop**（type-loop.md，reinforcing cycle / flywheel）
- 硬约束（skill 原文为准）：每图 ≤9 节点/≤12 箭头/≤2 coral 焦点，超了拆 overview+detail；六条连接线规则（正交圆角 elbow、标签 6-10px 间隙、不重叠、同边 attach 点 fan-out ≥12px、不穿非端点盒、标签 mask 不压后画节点）；SVG 可访问性契约（role="img"、带前缀的 title/desc ID）；图例放底部水平条。
- 标签是中文：读 style-guide.md 的 Non-Latin labels 节，本项目为**简体中文**，选合适的简体中文字体族（Noto Sans SC）补进字体链接，人读的名称用 sans、技术子标签用 mono。
- 产出为**自包含 HTML**（内联 CSS + 内联 SVG，无外部依赖除 Google Fonts），静态无动画。

## 信息源（按可信度排序，源码为准）
1. docs/reference/harness/pi-extensions.md —— 机制全貌（扩展全景表/注入通道/门禁分层/记账口径）
2. .pi/extensions/ 源码逐个核对行为：constraint-injection.ts、quality-gate.ts、spec-gate.ts、ui-design-gate.ts、quota-gate.ts、dev-process-guard.ts、harness-telemetry.ts、entry-gate.ts、test-scope-guard.ts、tool-output-spill.ts
3. AGENTS.md「pi harness 扩展」节与「UI 分档速查」节；openspec 的 constraints-index.md 注入索引
4. docs/reference/开发执行规范.md §0.6（编排六步）/§4（门禁分层）/§11（归档门禁）

## 文档必须讲清
- 开发生命周期全景：需求探索 → openspec proposal（声明 ui-impact / constraint-domains）→ apply 六步编排 → 编辑与 turn_end 门禁 → 归档门禁 → 归档后溯源与 retro 复盘闭环
- 每个阶段哪些扩展介入、走什么通道（system prompt 约束注入 / PreToolUse 拦截 / turn_end 增量检查 / 会话结束清理）、agent 与用户各自感知到什么（约束文本出现 / 门禁 [回归] steer / block 理由 / 逃生口 --force 等）
- 事实闭环：events.db 记了什么 → harness-retro 报告怎么消费 → 改进项 → 规则修改 → 基线对比回检

## 产出（共 4 个文件，全在此目录）
/home/zanebono/software/Syntopica/docs/research/harness-lifecycle-2026-09-18/
- README.md —— 主讲解文档（中文大白话，面向想理解这套体系的新读者；内嵌三张图的相对链接与每图两三句导读；描述行为时若源码与 reference 文档有出入，以源码为准并在文中标注差异；文档末尾记录 diagram 选型假设与删减取舍）
- diagrams/lifecycle-panorama.html（图1）
- diagrams/turn-gate-sequence.html（图2）
- diagrams/retro-loop.html（图3）

## 硬边界
只写上述 4 个文件，不改任何代码/配置/正式 reference 文档，不开 change。若三张图超出复杂度预算，拆分为 overview+detail 并放入同一 diagrams 目录（文件数可超过 3，但都在该目录内）。最终回复=产出目录路径 + 文件清单 + 3 句话内摘要。`

const run = await runs.run("write-doc", { agent: "delegate", task: TASK })

return { output: run.output }
