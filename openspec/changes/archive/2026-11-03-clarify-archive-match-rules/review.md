# Review 建账 · clarify-archive-match-rules

> 审查人：change-review-glm 子线程（2026-11-03，审查制品 + 本 change 全部代码 diff）
> 报告全文见会话记录；本表为逐条处置账。

### R-1 2026-11-03【Medium】composite_hit 结论行叠字「⓪组合命中命中」
- **状态**：已修复
- 处置：verdictLine composite 分支改为「→ ⓪组合命中即停，得分 …」；chains 为空（composite_hits 缺失的异常数据）时退化为「组合标签交集非空」表述；补 composite 结论行组件断言。

### R-2 2026-11-03【Medium】组件内重复实现交集去重口径，违背 design D1「组件只做展示」
- **状态**：已修复
- 处置：`archiveMatchFlow.ts` 导出 `overlapOf(detail)` 唯一实现，组件 verdictLine 取用，删除组件内 `new Set(...)` 逻辑。

### R-3 2026-11-03【Medium】test-cases §2 声明的多个「组件用例」未落地
- **状态**：已修复（②④⑤⑥ 补用例；①③ 改留痕）
- 处置：补「空集：该标签无辅助标签」「loading 时无判定过程节点」「match-detail 失败后重开成功给真实数值」「入板阶段名与 RULE_COPY 同源」四条断言；①超长（实现为 flex-wrap 换行兜底 + 仅 .amp-stage-label 有 ellipsis，改 test-cases 措辞如实留痕）、③越界引用（行为由 auxRows 映射天然保证，标注无独立用例）。

### R-4 2026-11-03【Low】② 参照行与头区指标行两套格式（，/=三位 vs ·/两位）
- **状态**：已修复
- 处置：② 参照行 result 改用与 spec 引文/头区一致的「相似达标 x/N · R x.xx · Smax x.xx」格式，纯函数断言同步。

### R-5 2026-11-03【Low】spec Scenario 引文「未走到（①已命中）」缺「即停」
- **状态**：已修复
- 处置：delta Scenario 引文补「即停」，与 requirement 正文逐字一致。

### R-6 2026-11-03【Low】ui-design mockup 与 tasks 2.6 信息排序描述漏「描述」块
- **状态**：已修复
- 处置：两处措辞补「{类别}描述」（实现与 flow 文档已含，行为合规）。

### R-7 2026-11-03【Low】test-cases §3「结论」占位未回填
- **状态**：已修复
- 处置：回填 pass 结论（引用 ui-verification/report.md）。

### R-8 2026-11-03【Low】ui-verification 报告用例数失真（45 → 实际 26+19+11=56）
- **状态**：已修复
- 处置：更正为「三文件合计 56 用例（26+19+11）」。

### R-9 2026-11-03【Low】composite 命中步公式展开仍渲染 R 公式（免相似度计算口径相悖）
- **状态**：已修复
- 处置：match_reason=composite_hit 时跳过 rateFormula 渲染。

### R-10 2026-11-03【Low】reached 字段组件未消费 / step-result testid 多行复用
- **状态**：已驳回（保留现状）
- 理由：reached 是 tasks 1.2 明确要求的「是否已走过」输出字段，作为纯函数契约保留（组件用 state 过滤，语义等价）；testid 复用发生在同一步骤容器结构内，现有测试按步骤容器定位无歧义，不构成维护负担。

### R-11 2026-11-03【High】用户实机报障：白天模式公式文字与背景同色不可见
- **状态**：已修复
- 根因：`front/app/components/KaTeXRender.vue` scoped 样式写死 `color: rgba(255,245,235,0.88)`（近白色，仅适配深色主题），editorial 浅底上对比度 1.06。存量 bug，本 change 将公式从折叠块提到命中步就地展开后曝光。共享组件——MatchDetailPanel / MatchingConfigDialog 同样受益。
- 处置：改用主题 token `var(--color-text-primary)`；实页复测：editorial 对比度 1.06→15.32，dark 17.38 无回归；已重新构建铺盘。

- 审毕发现总计 11 条：High 1 / Medium 3 / Low 7；已修复 10 / 已驳回（保留现状）1 / 待处理 0。
