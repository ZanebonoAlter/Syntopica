<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 只增加一个**只读**的 harness 事实账本消费脚本（`scripts/harness-retro.sh`）、其 fixture 冒烟测试与一份 agent 侧 skill（`.agents/skills/harness-retro/SKILL.md`），并只读 `.pi/harness/events.db`。不新增页面、面板、对话框、导航或交互流程，不触及 `front/` 下任何组件、样式、路由、状态或文案，用户可见的产品界面零变化。

依据 `docs/reference/standard/frontend/layout.md` 的分档判据（page shell 四模式 / dialog 四档 / 双视口验收）均不适用，proposal 声明 `ui-impact: none`，无界面需要设计，故不制作原型。
