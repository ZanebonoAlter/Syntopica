# Review：anchor-topic-centroid-with-embedding

审查人：change-review 子线程（MiniMax M3.1 Flash:max，2026-11-12），制品 4 件 + delta spec + 核心提交 d42bef55/39f73983。

### R-1 2026-11-12【High】归档门禁 review 痕迹为零
- **状态**：已修复（本文件建账，R-1..R-9 全条目处置）

### R-2 2026-11-12【High】spec delta 配置表名错（embedding_config → 实为 ai_settings）
- **状态**：已修复（delta 5 处 + design.md D3 改回 ai_settings；系提案阶段引入的错误——`LoadPersistentTopicConfig` 实读 `models.AISettings`，旧 spec 原文正确，主 spec 未被污染，归档前已纠正）

### R-3 2026-11-12【Medium】「单话题失败不中断」零真覆盖（畸形维度话题走的是降级分支）
- **状态**：已修复（测试注入真失败话题：0 section + NULL 首义 → 空串写 vector 列触发 SQLSTATE 22P02 → 断言 updated=3/failed=1 且其余话题已写；scoped 重跑同断言）

### R-4 2026-11-12【Medium】新端点未进 API 参考
- **状态**：已修复（docs/reference/api/daily-reports.md 端点索引 + 正文章：board_id/400/processing/异步幂等语义）

### R-5 2026-11-12【Medium】效果核对数字口径不自洽（模拟=聚合向量距离 vs 实测=逐条平均）
- **状态**：已采纳（tasks.md 5.4 注明两口径度量差异、方向一致；补 268151 复查 0.1239≈模拟 0.1247，未误伤结论成立）

### R-6 2026-11-12【Low】recompute 30min ctx 只覆盖首次 List，逐话题循环无 ctx
- **状态**：待处理（实跑 1915 话题秒级完成、幂等可重跑，风险极低；后续如话题量级增长再做 WithContext）

### R-7 2026-11-12【Low】窗口混入短维 section 时 meanPgVectors 截断 → 质心维失配 → 该话题静默永不命中
- **状态**：待处理（仅换 embedding 模型后的存量场景可触发；后续可按首义维过滤窗口内 section）

### R-8 2026-11-12【Low】α 线性口径措辞（cosine 匹配下实际锚定强于标称）
- **状态**：已修复（delta 配置项节 + configuration.md 注明「线性混合权重，实际锚定效果略强于标称比例」）

### R-9 2026-11-12【Low】死配置键被 MODIFIED 块重新背书 + assignment 注释漂移
- **状态**：已修复（delta 中 `persistent_topic_decay_window` 标注为迁移种子遗留、实际 key 为 `persistent_topic_candidate_decay_window`；daily_report_assignment.go topicAnchor 注释更新为混合公式语义）

## 审毕范围外记录

- 公式正确性/并发异步/flow 一致性/UI 影响面：审查未发现（见审查报告「未发现」节）。
- 审查人备注的 commit 定位偏差（brief 误写 4f48a0b，实为 d42bef55）：误报丢弃。
