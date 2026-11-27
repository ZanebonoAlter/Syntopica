# Human Feedback — rank-report-highlights-by-board-relevance

> **定位**：记录用户在 change 生命周期内对产物（proposal/design/ui-design/原型/实现/文档）的反馈，作为本 change 修订依据；也是 harness-retro `--lifecycle` 体检卡「人类反馈段」的计量输入。人写给人的修订账本，不进 events.db（文件随 change 归档永久保存）。
> **消费方**：① 本 change 修订（条目 → 对应制品增量）；② harness-retro --lifecycle 计量（条数/归因分布/未处置数）；③ 归档对账（archive-readiness 顺带 feedback.sh verify——未处置条目提醒处置，不许静默丢）。
> **格式**（条目级契约，一行正则全解析）：条目标题行 `### FB-<N> <日期>【<类型>|<归因>】<标题>`；条目含状态行 `- **状态**：<状态>`。词表见 `bash scripts/harness/feedback.sh menu`。

## 反馈清单

### FB-1 2026-11-27【产品|backend】用户指出只改输出序赶不走跑题的（模型挑出来就那 3 条），改为先筛后挑：候选池按贴题档（近档 + 手动关注命中）优先、不足补位到 3、为空维持前 8，挑完再按贴题档排一遍

- **状态**：已消化（方案已按反馈改为先筛后挑并实现，2026-11-27）
- **原文要点**：用户指出只改输出序赶不走跑题的（模型挑出来就那 3 条），改为先筛后挑：候选池按贴题档（近档 + 手动关注命中）优先、不足补位到 3、为空维持前 8，挑完再按贴题档排一遍
- **已核实事实**：只读回放 board 1974 近 15 期——11-26 模型挑的 3 条里 2 条是关税，只改输出序只能挪后不能赶走；贴题板块（l1 + watch）≥3 的有 14/15 期，先筛后挑可让关税等跑题簇不进候选（见 explore-findings「方案调整」）
- **影响面**：proposal / specs（两条 Requirement 的候选池与展示序）/ design（D3–D7 重写）/ tasks（2.1–2.4）/ flow 红线 46；模型挑选、提示词、快照字段不变
- **后续动作**：已随本 change 落地；若之后仍想让模型在池内更偏向贴题的，可另开 change 把贴题档加进提示词（本 change 明确不做）
