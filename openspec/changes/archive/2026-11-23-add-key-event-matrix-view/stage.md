<!-- stage: verify -->
<!-- constraint-domains: semantic-board, topic-graph -->

# 阶段与范围

- 制品：proposal / ui-design / specs / design / tasks 已完成；当前阶段 implement（实现 + 验收）。
- UI：`ui-impact: major`，原型 `ui-prototype/index.html` 已完成，`ui-approval: approved`（用户已确认原型）。
- 实现期若发现新业务域或布局决策，只在本文件追加，不新建第二份阶段记录。

## 2026-11-23 用户实机反馈修订（追加，不新建记录）

用户在实机（2560×1271，30 天档、观察窗 09-29~11-22 共 55 天）提出两条反馈，已写入制品并纳入实现：

1. **「超过 30 天啥都看不见」** → 根因：事件标签收起判据是「可见日数 > 31」一刀切，55 天窗口整体收成光点 + ★计数。修订（design D7 / ui-design Layout Contract / delta spec Scenario「长窗仍看得见事件名称」）：**收起判据改为堆叠层数上限（4 层 / 行高封顶 246px）**，长窗照常显示事件名称。
2. **「我选了右侧的泳道等应该跳转回矩阵追踪视角」** → 根因：候选话题在关键事件视图不占行，右轨选中是死选择。修订（design D14 / ui-design Interaction Decisions D4 / delta spec Scenario「右轨选中切回矩阵视图」）：**右轨发起的话题选中一律切回「矩阵 / 追踪」视图，并把该泳道滚入视野 + 高亮**；本视图内部的选中仍留在本视图。
- 两处修订都只动交互与降级规则，**信息架构 / 主流程 / 布局模式未变** → `ui-approval` 不重置；原型差异照实写进 `ui-verification/report.md`。
