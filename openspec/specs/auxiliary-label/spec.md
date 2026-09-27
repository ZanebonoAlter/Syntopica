## Purpose

辅助标签（Auxiliary Label）的提取、入库、治理。辅助标签是 event/person tag 的语义锚点，也用于 keyword tag 直入辅助池。

## Requirements

### Requirement: LLM 提取 tag 时同时生成辅助标签（event/person）
系统 SHALL 在 LLM 提取 event/person 标签时，为每个 tag 同时生成 3-5 个带 description 的辅助标签（auxiliary label），作为 tag 的语义锚点。辅助标签 SHALL 与 event/person tag 在同一个 event/person 提取调用中输出，不再为辅助标签发起额外 LLM 调用。keyword 标签不生成辅助标签，而是直接进入辅助标签池（见"Keyword 标签直接入辅助池"需求）。辅助标签必须与事件核心主体直接相关，是理解事件不可或缺的要素；文章中仅为背景提及、一笔带过的人物或国家不应作为辅助标签。

#### Scenario: event 标签提取含辅助标签
- **WHEN** 一篇文章被处理，LLM 提取到 event tag "伊朗袭击以色列"
- **THEN** LLM 同时输出辅助标签 [{"label": "伊朗", "description": "中东地区国家"}, {"label": "以色列", "description": "中东国家"}, {"label": "导弹袭击", "description": "军事打击行动"}]

#### Scenario: 辅助标签数量约束（event/person）
- **WHEN** LLM 提取的 event/person 标签结果中某个 tag 的辅助标签数量为 0 或超过 5
- **THEN** 系统 SHALL 拒绝该结果，返回错误

#### Scenario: 背景提及人物不作为辅助标签
- **WHEN** 事件为"日菲加强安保合作旨在牵制中国"，文中一笔带过提到"特朗普对此表示关注"
- **THEN** "特朗普" SHALL NOT 被提取为该事件的辅助标签，因为移除特朗普后事件描述仍然成立

### Requirement: Keyword 标签直接进入辅助标签池
系统 SHALL 将 category=keyword 的 tag 直接作为辅助标签入库，不再生成额外的 auxiliary_labels。tag 的 label 和 description 直接复用为辅助标签的 label 和 description。

#### Scenario: keyword 标签直入辅助池
- **WHEN** 一篇文章被处理，LLM 提取到 keyword tag "Claude Code"，description="Anthropic推出的AI编程助手工具"
- **THEN** 系统 SHALL 将 "Claude Code"（label）+ description 直接作为辅助标签入库，不调用 LLM 生成额外的 auxiliary_labels

#### Scenario: keyword 标签辅助标签数量不约束
- **WHEN** LLM 提取的 keyword 标签的 auxiliary_labels 为空
- **THEN** 系统 SHALL 不拒绝该标签，而是跳过辅助标签生成，直接走 keyword 直入逻辑

### Requirement: 辅助标签三级入库（embedding 分离）
系统 SHALL 对每个辅助标签按 L1→L2→L3 顺序处理入库。L1 为 slug/alias 精确匹配（零成本复用），L2 使用 merge_embedding（仅 label 文本）判断 ≥0.95 自动 merge，L3 新建辅助标签时同时生成 merge_embedding（label-only）和 storage embedding（label + description，写入 semantic_labels.embedding）。

#### Scenario: L1 精确匹配命中 alias
- **WHEN** 新辅助标签 "AI生成工具" 入库，已有 semantic_label "AI图像生成" 的 aliases 包含 "AI生成工具"
- **THEN** 系统 SHALL 直接关联到已有 semantic_label，不调用 embedding API

#### Scenario: L2 merge-embedding 判断
- **WHEN** 新辅助标签 "AI绘图" 的 merge_embedding（仅 label 文本）与已有 "AI图像生成" 的 merge_embedding cosine similarity 为 0.96（≥0.95）
- **THEN** 系统 SHALL 将 "AI绘图" merge 到 ref_count 更大的一方，小方 label 加入大方 aliases

#### Scenario: L3 新建辅助标签（storage-embedding）
- **WHEN** 新辅助标签 "量子计算"（description="利用量子力学原理进行计算的新型计算范式"）的 merge-embedding 与所有已有 semantic_label 的相似度均 <0.95，且 slug/alias 未命中
- **THEN** 系统 SHALL 创建新的 semantic_label（label_type="auxiliary"），description 写入 description 字段，merge_embedding 由 label 生成，storage embedding 由 label + ": " + description 生成并写入 embedding 字段

### Requirement: 辅助标签 description 写入
系统 SHALL 在辅助标签入库时写入 description 字段。event/person 的辅助标签 description 由 LLM 提取时生成；keyword 直入的辅助标签 description 复用 tag 已有的 description（由 tagger.go tag_description 操作生成）。

description SHALL 非空、长度不超过 500 字符，且不能只重复 label。系统 SHALL 拒绝缺少 description 的 event/person 辅助标签。keyword 标签直入时 SHALL 使用同一次 tag 提取结果中的 description；系统 SHALL NOT 依赖后续异步 description 生成结果完成 keyword 直入。

#### Scenario: event 辅助标签 description 来自 LLM
- **WHEN** LLM 提取 event tag "伊朗袭击以色列" 的辅助标签 "伊朗"（description="中东地区国家"）
- **THEN** 系统 SHALL 将 "中东地区国家" 写入 semantic_labels.description

#### Scenario: keyword 辅助标签 description 来自 tag
- **WHEN** keyword tag "PostgreSQL"（description="开源关系型数据库管理系统"）直入辅助池
- **THEN** 系统 SHALL 将 "开源关系型数据库管理系统" 写入 semantic_labels.description

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

### Requirement: Tag 提取单次调用输出 event/person 与 keyword 双数组

系统 SHALL 以单次 LLM 调用完成 tag 提取，输出同时包含 event/person 标签数组与 keyword 标签数组（schema 双数组约束）。event/person 标签 SHALL 携带 3-5 个辅助标签；keyword 标签 SHALL 携带 description，不输出 auxiliary_labels。

单次调用（含既有重试上限）整体失败时，系统 SHALL 回退到 heuristic keyword 提取。调用成功但 event/person 数组为空或全部未通过校验时，系统 SHALL 仅保留 keyword 产出并记录缺失原因，不视为失败、不触发全量 heuristic 回退。调用成功但 keyword 数组为空或全部未通过校验时，系统 SHALL 使用 heuristic keyword 作为展示兜底，但 heuristic keyword 因缺少同次 LLM description，默认不进入辅助标签池。

合并后的标签总数 SHALL 不超过 5 个；keyword 数组最多保留 3 个标签。若同一 slug 同时出现在多个 category 中，系统 SHALL 按 person > event > keyword 的优先级保留更具体的分类，并丢弃低优先级重复项。

#### Scenario: event/person 数组为空但 keyword 有产出

- **WHEN** 单次调用成功返回，event/person 数组为空或全部未通过校验，keyword 数组有产出
- **THEN** 系统 SHALL 保留 keyword 标签，记录 event/person 数组缺失原因，不生成 event/person 标签，且不触发全量 heuristic 回退

#### Scenario: keyword 数组为空但 event/person 有产出

- **WHEN** 单次调用成功返回，keyword 数组为空或全部未通过校验，event/person 数组有产出
- **THEN** 系统 SHALL 保留 event/person 标签，并使用 heuristic keyword 作为展示兜底；heuristic keyword 默认不写入辅助标签池

#### Scenario: 双数组间去重

- **WHEN** 同一次调用的 event/person 数组输出 person tag "Sam Altman"，keyword 数组也输出 keyword tag "Sam Altman"
- **THEN** 系统 SHALL 保留 person tag，并丢弃重复的 keyword tag

#### Scenario: 单次调用整体失败回退 heuristic

- **WHEN** 单次提取调用重试耗尽仍失败
- **THEN** 系统 SHALL 回退 heuristic keyword 提取（结果 source=heuristic），失败原因记录在提取结果错误信息中

#### Scenario: mono 文章提取调用数为 1

- **WHEN** 一篇 mono 文章完成打标且 LLM 提取路径成功
- **THEN** `ai_call_logs` 中该文章的 tag 提取调用（operation=tagmanagement.extractor_enhanced）恰为 1 次

### Requirement: Alias 变更不得触碰向量列

辅助标签的 alias 积累路径（L2 归并 addAlias、手动 alias 合并）SHALL 仅更新 aliases 列（及归属维护所需的 ref_count/状态列），SHALL NOT 写入或清空 embedding / merge_embedding 向量列。已入库向量的「吸引力」以既有向量为准，alias 变更不改变其物理表示（对应 flow 红线：L2 命中只 addAlias、不改 Label、不重算 MergeEmbedding）。

#### Scenario: L2 归并新别名后向量保持不变

- **WHEN** 新辅助标签文本 "高市早苗政府" 的 merge embedding 与已有标签 "高市早苗"（embedding/merge_embedding 均非 NULL）cosine ≥ 去重阈值，触发 L2 归并加入 aliases
- **THEN** 该已有标签的 embedding 与 merge_embedding 保持归并前的值不变，仅 aliases 列追加 "高市早苗政府"

#### Scenario: 手动 alias 合并后目标标签向量保持不变

- **WHEN** 用户将源辅助标签的 label/aliases 合并进目标辅助标签（目标标签向量非 NULL）
- **THEN** 目标标签的 embedding / merge_embedding 保持合并前的值不变

#### Scenario: 归并路径不产生 active 且向量缺失的行

- **WHEN** 任意数量的别名变体依次归并到同一已有标签（含并发打标批处理场景）
- **THEN** 该标签行始终满足 status=active ⇒ embedding IS NOT NULL（不出现 active 但向量为 NULL 的静默损坏行）

### Requirement: 死向量回填（active aux 向量重建）

系统 SHALL 提供手动触发的辅助标签死向量回填端点：异步扫描 `status=active AND label_type=auxiliary` 且 embedding 或 merge_embedding 为 NULL 的行，按 L3 同款生成规则重建向量（storage embedding 由 "label: description"（有 description 时，否则仅 label）生成、merge embedding 由 label 生成），逐行独立容错。该端点 SHALL 幂等：已有向量的行不重算，重复触发只处理仍缺失向量的行；单行生成失败 SHALL 跳过并记录，不阻塞其余行；全部行完成后任务结束，可再次触发续漏。

#### Scenario: 触发回填补齐死向量行

- **WHEN** 库中存在 active aux 标签 "高市早苗"（embedding/merge_embedding 为 NULL），用户调用回填端点
- **THEN** 系统异步为该行重建 embedding（由 "高市早苗: <description>" 生成）与 merge_embedding（由 "高市早苗" 生成），完成后该行两向量均非 NULL

#### Scenario: 已有向量的行不受回填影响

- **WHEN** 回填运行时某 active aux 标签两向量均非 NULL
- **THEN** 该行的向量与别名、描述等其余列保持原值，不重算

#### Scenario: 单行生成失败不阻塞批处理

- **WHEN** 回填过程中某行的 embedding 调用失败（如 LLM 网络错误）
- **THEN** 系统跳过该行并记录失败，继续处理其余行；任务结束后再次触发时仅重试仍缺向量的行

#### Scenario: 无死向量时回填为空操作

- **WHEN** 库中不存在 active 且向量缺失的 aux 标签，用户调用回填端点
- **THEN** 任务立即完成（零行处理），不产生任何写入或 LLM 调用
