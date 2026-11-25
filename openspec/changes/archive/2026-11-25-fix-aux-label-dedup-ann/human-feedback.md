# Human Feedback — fix-aux-label-dedup-ann

> **定位**：记录用户在 change 生命周期内对产物（proposal/design/ui-design/原型/实现/文档）的反馈，作为本 change 修订依据；也是 harness-retro `--lifecycle` 体检卡「人类反馈段」的计量输入。人写给人的修订账本，不进 events.db（文件随 change 归档永久保存）。
> **消费方**：① 本 change 修订（条目 → 对应制品增量）；② harness-retro --lifecycle 计量（条数/归因分布/未处置数）；③ 归档对账（archive-readiness 顺带 feedback.sh verify——未处置条目提醒处置，不许静默丢）。
> **格式**（条目级契约，一行正则全解析）：条目标题行 `### FB-<N> <日期>【<类型>|<归因>】<标题>`；条目含状态行 `- **状态**：<状态>`。词表见 `bash scripts/harness/feedback.sh menu`。

## 反馈清单

### FB-1 2026-11-24【流程|fact-check】design D3 直接用 halfvec 近似距离裁决阈值，偏离 core/embedding.go:findSimilarByVector 既有的「halfvec 召回 + float32 精排」范式，自造精度翻转风险

- **状态**：已消化（design D3 改两段式：halfvec 召回 + 原列 float32 精排，落点 sqlMergeMatcher）
- **原文要点**：design D3 直接用 halfvec 近似距离裁决阈值，偏离 core/embedding.go:findSimilarByVector 既有的「halfvec 召回 + float32 精排」范式，自造精度翻转风险
- **已核实事实**：embedding.go:findSimilarByVector 已是两段式并注明「ranking 与旧全表扫描一致、阈值不变」；原 design 直接用 halfvec 距离裁决， fp16 误差 ~1e-3 会让 0.95 边界翻转
- **影响面**：归并判定准确性（多建重复标签 / 误归并），对账 mismatch 无法归零
- **后续动作**：已写回 design D3 / spec「阈值判定不受 halfvec 近似误差影响」/ tasks 2.2；设计阶段先 grep 同类既有范式再定方案

### FB-2 2026-11-24【流程|fact-check】design 未设 hnsw.ef_search：LIMIT 32 贴着默认值 40，D6 的 LIMIT 128 兜底实际拿不到行；既有先例 embedding.go:302 已有 SET LOCAL 写法

- **状态**：已消化（design D6 补 ef_search：32 配 100、兜底 128 配 200，只读事务内 SET LOCAL）
- **原文要点**：design 未设 hnsw.ef_search：LIMIT 32 贴着默认值 40，D6 的 LIMIT 128 兜底实际拿不到行；既有先例 embedding.go:302 已有 SET LOCAL 写法
- **已核实事实**：pgvector 默认 hnsw.ef_search=40、iterative_scan 默认 off；embedding.go:302 的先例是 pool 50 配 ef_search 120 并注明「必须 ≥ pool」
- **影响面**：召回率与 LIMIT 128 兜底有效性
- **后续动作**：已写回 design D6 / spec / tasks 2.3、2.4；部署后对账看执行计划与 mismatch

### FB-3 2026-11-24【流程|test-gap】tasks 3.1 写「无残留（测试除外）」，而 aux_label_cache_test.go 有 27 处引用待删字段会让测试包编不过；3.2 的 -run 模式匹配不到任何现有测试会假通过

- **状态**：已消化（tasks 3.1 改为含测试零命中并列出要删改的测试；3.2 的 -run 模式改为 TestAuxiliaryLabelServiceL1）
- **原文要点**：tasks 3.1 写「无残留（测试除外）」，而 aux_label_cache_test.go 有 27 处引用待删字段会让测试包编不过；3.2 的 -run 模式匹配不到任何现有测试会假通过
- **已核实事实**：aux_label_cache_test.go 6 个测试共 27 处引用 merge 缓存；现有 L1 测试名为 TestAuxiliaryLabelServiceL1*，`-run TestAuxiliaryLabelL1` 匹配 0 个
- **影响面**：实现子线程会在测试包编译失败处卡住，或拿到假绿验证结果
- **后续动作**：已写回 tasks 3.1 / 3.2；写验证命令前先 grep 现有测试名
