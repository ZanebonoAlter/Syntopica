<!-- stage: implement -->
<!-- constraint-domains: semantic-board, daily-report, topic-graph -->

# rework-lane-compose-swimlane-canvas — 阶段载体

- 当前阶段：**design**（proposal + ui-design 已落盘，`ui-approval: pending` 待用户批准原型；specs / design / tasks 待办）。
- 原型形态：canvas 主模式 → `ui-prototype/states/` 三态图（01-empty / 02-editing / 03-confirmed）+ 辅助态 04-search + `interaction-table.md`，**不做可点 HTML**。
- 探索期结论已落 `explore-findings.md`（实现期自动注入，无需重探）。
- 形态已由用户拍板（2026-11-20 对话，逐条确认）：
  1. 阈值只门控散料；候选链成员 / 已选 / 只读参照永不隐藏。
  2. 阈值 UI = 三档（仅高相关 / 标准 / 全部），默认标准。
  3. 关键词只切**检索态**，逐条人工选入，不做批量。
  4. 关键词检索下沉后端（含正文/摘要）**单独立项**，本期用现有纯前端标题过滤。
  5. 点击回归"只看"；选入/移出靠**拖拽**（点=逐条，带=整组）。
  6. 形态 β：带即行，成员排在自己行下面（不是"容器盒子"）。
  7. 卡片 ＋/⊘ 按钮保留（键盘/无障碍通道）。
  8. 移出 = 拖出泳道行松手即可（不需要精确目标）。
- 原型 v3 修订（2026-11-20 用户反馈）：
  1. 「按标题找」范围收窄为**未归类**（不属于画布上任何候选链的节点），命中只在地板层浮现。
  2. **本次泳道不绑候选链 / 事件线**：取空的链行消失，无「已采纳」摘要，无来源构成统计。
  3. 节点下**常显 ≤5 字截断标题**（列宽 <40px 整档关闭；散料不常显）。
  4. 同日多节点**竖直堆叠**（每槽 24px，行高随条数增长），不做折叠计数点。
  6. **删掉横条胶囊**（用户 2026-11-20："红线划出来的是不是没必要，因为没有意义"）：横向位置已由成员自身表达，条长是冗余；整组拖动把手改为行标签（hover 整行提亮）。
  5. **右栏骨架恒定**：① 当前事件线索 / ② 可补充的事件 / ③ tab 区（已选事件 · 按标题找）三段在任何态都渲染；空态 ③ 默认落「按标题找」。修掉 v2 里 01/02 漏画 tab 区、歧视为「检索态独吞右栏」的缺陷（违反母 spec 右栏三段契约）。
- **L2 交互沙盒**：`ui-prototype/sandbox/index.html`（非合同，仅验手势手感；用户 2026-11-20 明确要求，按优先级宪法执行并留痕）。L1 合同仍是 `states/` 三态图 + `interaction-table.md`，`ui-approval: approved` 指向 L1。
- 规划制品全部完成（proposal / ui-design / specs / design / tasks），当前已进 apply。
- 沙盒 `ui-prototype/sandbox/index.html` **布局有重叠缺陷，不作为验收基准、也不作为实现几何基准**（用户 2026-11-20 明确）；几何基准 = `states/01-empty.svg` / `02-editing.svg` / `03-confirmed.svg`（1440×900）。
- 派发记录（2026-11-20）：实现线程 `commandcode/deepseek/deepseek-v4.1-flash`，范围 tasks 1–6；review 线程按用户指定用 `zai-coding-cn/glm-5.3:max`（本 change 无后端改动，review 时仍用该模型审 change 级差异）。
- 约束域声明与 `proposal.md` 头部并集一致；实现期发现新域在本文件头部补（只增不减）。
