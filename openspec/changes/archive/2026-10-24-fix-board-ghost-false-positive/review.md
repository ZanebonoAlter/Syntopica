<!-- review: fix-board-ghost-false-positive -->
<!-- 审查方式：change-review-glm 子线程只读聚焦 review（审查制品 + 5 文件 diff），2026-10-15 -->

# Review 账本

结论：0 High / 2 Medium / 4 Low。判定结构、D1 参照系、边界、配置、回归面共 12 项核实通过（详见子线程报告「已确认无问题清单」）；无 High 级缺陷。

### R-1 2026-10-15【Medium】delta spec floor 撤销豁免语义两处互斥（ADDED SHALL 句比实现强）
- **状态**：已修复（实现「回退绝对门」= design D3 意图且与 MODIFIED Scenario/测试一致，改制品不改码：ADDED 句改「豁免不生效、回退绝对门与相对门判定」，MODIFIED 层 1/层 2 改「未获豁免的挂载」，proposal 措辞同步；分歧场景 floor>threshold 时回退语义更合理，floor 是下限保险丝不是第二把绝对刀）

### R-2 2026-10-15【Medium】域红线文档 semantic-board.md 红线 4 未随三层化更新（constraint-injection 注入源，后续 change 会被误导）
- **状态**：已修复（红线 4 与 L53 管道描述补「全库最优豁免 + 地板线」层、括号史补本 change 结论，变更溯源表补一行；tasks.md 补文档节 5.1 勾选）

### R-3 2026-10-15【Low】computeGlobalBestBoards 注释写 "plus that maximum" 但签名只返回集合
- **状态**：已修复（删多余短语）

### R-4 2026-10-15【Low】floor 等值边界用例 DirectionFloor=0.515 与 relVec(0.515) 实际 cosine 处 ulp 敏感区（arm64 FMA 舍入方向可能翻转）
- **状态**：已修复（floor 改用与门内同一表达式 `CosineSimilarity(xAxis, floorVec)` 构造，bitwise 等值消除抖动；同文件 "gap equals margin" 既有先例）

### R-5 2026-10-15【Low】hit_rate passes 用例挂载板平凡为全局最优，spec Scenario「not the global best AND cos≥0.5 AND gap<margin」前提未被钉住（false 可由豁免层给出）
- **状态**：已修复（板 10 emb 加第三维失配 cos≈0.949，候选外加 cos=1.0 更高板；false 现由「非最优 + 绝对门过 + 相对门不触发」路径给出）

### R-6 2026-10-15【Low】floor/margin 两键不在 board 配置管理 API 白名单（isSemanticBoardConfigKey / configToMap），只能直写 ai_settings
- **状态**：已驳回（超出本 change 范围：task 2.1 明确只做 LoadConfig 读取；direction_relative_margin 前例（fix-digest-tag-leak）同样缺席白名单，属沿袭现状而非新引入缺口，行为面一致；扩白名单 + DTO 属行为面变化，建议另开 change 或随配置管理专项处理）
