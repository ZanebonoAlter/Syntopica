## ADDED Requirements

### Requirement: LLM 提取 tag 时同时生成辅助标签
系统 SHALL 在 LLM 提取标签时，为每个 tag 同时生成 3-5 个辅助标签（auxiliary label），作为 tag 的语义锚点。辅助标签 SHALL 与 tag 一起在单次 LLM 调用中输出，不额外增加 API 调用。

#### Scenario: 正常提取含辅助标签
- **WHEN** 一篇文章被处理，LLM 提取到 tag "happyhorse"
- **THEN** LLM 同时输出辅助标签 ["AI", "图像生成", "开源模型"]，并与 tag 一起返回

#### Scenario: 辅助标签数量约束
- **WHEN** LLM 提取标签结果中某个 tag 的辅助标签数量为 0 或超过 5
- **THEN** 系统 SHALL 拒绝该结果，返回错误

### Requirement: 辅助标签三级入库
系统 SHALL 对每个辅助标签按 L1→L2→L3 顺序处理入库。L1 为 slug/alias 精确匹配（零成本复用），L2 为 embedding ≥0.95 自动 merge，L3 为新建辅助标签。

#### Scenario: L1 精确匹配命中 alias
- **WHEN** 新辅助标签 "AI生成工具" 入库，已有 semantic_label "AI图像生成" 的 aliases 包含 "AI生成工具"
- **THEN** 系统 SHALL 直接关联到已有 semantic_label，不调用 embedding API

#### Scenario: L2 embedding merge
- **WHEN** 新辅助标签 "AI绘图" 的 embedding 与已有 "AI图像生成" 的 cosine similarity 为 0.96（≥0.95）
- **THEN** 系统 SHALL 将 "AI绘图" merge 到 ref_count 更大的一方，小方 label 加入大方 aliases

#### Scenario: L3 新建辅助标签
- **WHEN** 新辅助标签 "量子计算" 的 embedding 与所有已有 semantic_label 的相似度均 <0.95，且 slug/alias 未命中
- **THEN** 系统 SHALL 创建新的 semantic_label（label_type="auxiliary"），生成并存储 embedding

### Requirement: Merge 时保留 ref_count 更大的一方
系统 SHALL 在 merge 两个辅助标签时，始终保留 ref_count 更大的 label，将较小方的 label 文本加入较大方的 aliases。

#### Scenario: 新标签 merge 到已有标签
- **WHEN** 新辅助标签 "LLM"（ref_count=0）与已有 "大语言模型"（ref_count=15）的 sim=0.96
- **THEN** 保留 "大语言模型"，将 "LLM" 加入 aliases，tag 关联到 "大语言模型"

### Requirement: Alias 自动积累不做修正
系统 SHALL 在 merge 时自动将被合并的 label 加入 alias 列表。系统 SHALL 同时提供最小修正能力：禁用辅助标签、手动合并 alias、从 board composition 移除辅助标签。

#### Scenario: Alias 持续增长
- **WHEN** 辅助标签 "AI图像生成" 先后 merge 了 "AI生成工具" 和 "AI绘图"
- **THEN** aliases 列表更新为 ["AI生成工具", "AI绘图"]

### Requirement: 辅助标签可禁用
系统 SHALL 允许用户将低质量、泛化或错误的辅助标签标记为 disabled。disabled 辅助标签 SHALL 不参与后续 board 匹配和升级候选。

#### Scenario: 禁用泛化辅助标签
- **WHEN** 用户禁用辅助标签 "事件"
- **THEN** 后续 board 匹配和升级候选 SHALL 排除该辅助标签

### Requirement: 辅助标签 alias 可手动合并
系统 SHALL 允许用户将一个辅助标签合并为另一个辅助标签的 alias，并迁移 tag 关联。

#### Scenario: 手动合并 alias
- **WHEN** 用户将辅助标签 "AI绘图" 合并到 "AI图像生成"
- **THEN** 系统 SHALL 将 "AI绘图" 加入 "AI图像生成" 的 aliases，并将相关 topic_tag_semantic_labels 迁移到目标辅助标签

### Requirement: Board composition 可移除辅助标签
系统 SHALL 允许用户从 SemanticBoard 的 board_composition 中移除错误辅助标签。移除后系统 SHALL 不自动重算历史 tag-board 归属，用户可手动触发回填。

#### Scenario: 移除错误构成标签
- **WHEN** 用户从 SemanticBoard "能源安全" 中移除辅助标签 "体育赛事"
- **THEN** 后续匹配不再把 "体育赛事" 视为该 board 的构成标签
