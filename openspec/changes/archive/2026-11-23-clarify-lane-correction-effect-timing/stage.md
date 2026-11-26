<!-- stage: implement -->
<!-- constraint-domains: semantic-board -->

# clarify-lane-correction-effect-timing — 阶段载体

- 当前阶段：**implement**（proposal / ui-design / design / specs / tasks 已落盘；ui-impact=minor、ui-approval=not-required，无原型待批）。
- 用户拍板口径（2026-11-22 对话）：**不是要加保存按钮，是「看不出改动已经生效」**——选「工作面顶部加一行常驻说明」方案；按规范落到**结论型徽标**形态（禁解说性长句，见 `standard/frontend/interaction.md` 与 `lane-correction-surface`）。
- 探索期结论已落本文件与 proposal（生效时机原只存在于底部 10px 页脚徽标，常被视口裁掉）。
- **2026-11-22 用户二次确认**：三枚结论型徽标定稿 =「立即生效」（实线 accent）/「次日日报生效」（虚线）/「无需保存」（中性），置于工作面顶部标题行右侧；用户确认原话「就这三枚」。该确认补齐 `interaction.md`「取 none 者 MUST 标 needs-approval 并在对话中获用户确认」的要求（对应 review R-2）。
- review（change-review 档）结论：R-1 High / R-2、R-3 Medium 已处置，R-4~R-7 Low 已修或已采纳并记判断，见 `review.md`。
