<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI Design

- **ui-impact**: none
- **status**: not-required
- **prototype**: none

## N/A Reason

本 change 只改三样东西，零产品界面：

1. `.pi/agents/stage-*.md` —— 六个子线程档位定义（frontmatter 里的模型与工具白名单）
2. `scripts/harness/stage-agent.sh` —— 命令行脚本，输出一个 agent 名
3. `AGENTS.md` / `开发执行规范.md` —— 给人和模型看的映射表

用户可见的变化是「派子线程时它跑在哪个模型上」，这不是界面改动。没有页面、没有组件、没有交互状态，因此不需要原型、不需要视觉验收、不涉及双视口。
