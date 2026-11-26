<!-- domain: semantic-board -->

## Purpose

辅助标签是可人工编辑的标签描述层，作为 event/person tag 的语义锚点（keyword tag 直入辅助池），用于提升版块匹配质量。

本族定义它怎么维护——模型与生命周期（提取入库/别名/禁用）、层级质量校验（slug 规范化/抽象树/多父）、合并建议与合并界面、清理回收（GC/描述修复）、身份保护。

标签**怎么产生**见 `tagging`；**怎么匹配到版块**见 `tag-matching`。

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

**【2026-11-12 本 change 起生效】单次调用（含既有重试上限）整体失败时，系统 SHALL 直接判失败，SHALL NOT 回退到 heuristic keyword 提取**——失败错误上抛给编排层，使打标任务进入队列重试，文章保持无标签。调用成功但 event/person 数组为空或全部未通过校验时，系统 SHALL 仅保留 keyword 产出并记录缺失原因，不视为失败。调用成功但 keyword 数组为空或全部未通过校验时，系统 SHALL NOT 生成 heuristic keyword 兜底（原「使用 heuristic keyword 作为展示兜底」条款与实现脱节——合并提取的 `mergeExtractedTags` 从未实现该兜底，本 change 一并作废）。

合并后的标签总数 SHALL 不超过 5 个；keyword 数组最多保留 3 个标签。若同一 slug 同时出现在多个 category 中，系统 SHALL 按 person > event > keyword 的优先级保留更具体的分类，并丢弃低优先级重复项。

#### Scenario: event/person 数组为空但 keyword 有产出

- **WHEN** 单次调用成功返回，event/person 数组为空或全部未通过校验，keyword 数组有产出
- **THEN** 系统 SHALL 保留 keyword 标签，记录 event/person 数组缺失原因，不生成 event/person 标签，且不触发任何规则词兜底

#### Scenario: keyword 数组为空但 event/person 有产出

- **WHEN** 单次调用成功返回，keyword 数组为空或全部未通过校验，event/person 数组有产出
- **THEN** 系统 SHALL 保留 event/person 标签，SHALL NOT 生成 heuristic keyword 作为展示兜底（原「使用 heuristic keyword 作为展示兜底」条款作废——规则词兜底已整体退役）

#### Scenario: 双数组间去重

- **WHEN** 单次调用成功返回，同一 slug 同时出现在 event/person 数组与 keyword 数组
- **THEN** 系统 SHALL 按 person > event > keyword 的优先级保留更具体的分类，丢弃低优先级重复项

#### Scenario: 单次调用整体失败不回退 heuristic

- **WHEN** 单次提取调用重试耗尽仍失败
- **THEN** 系统 SHALL 返回失败并把错误上抛给编排层（不产出任何标签、不回退 heuristic keyword 提取），使打标任务标记为失败并进入队列重试

#### Scenario: mono 文章提取调用数为 1

- **WHEN** 一篇 mono 文章走完整打标流程
- **THEN** 提取 SHALL 只发起 1 次 LLM 调用（双数组在同一次响应中返回），无额外调用

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

### Requirement: 辅助标签语义去重走数据库 ANN 召回

系统 SHALL 通过 `semantic_labels.merge_embedding` 上的 halfvec HNSW 部分索引召回 L2 候选，SHALL NOT 在进程内全量加载辅助标签向量逐条计算余弦；召回 SHALL 设置 `hnsw.ef_search` 不小于本次候选上限，召回查询所在事务 SHALL NOT 包含 embedder 调用。

- 索引 SHALL 只覆盖 `label_type='auxiliary' AND status='active' AND merge_embedding IS NOT NULL` 的行；召回查询的 WHERE 谓词 SHALL 与该部分索引谓词一致。
- 索引表达式 SHALL 内嵌当前运行时 embedding 维度（`halfvec(dim)`）；维度变化时 SHALL 在建索引前清理存量旧维度行后重建，SHALL NOT 继续使用旧维度表达式索引；列维度过渡（`ALTER COLUMN … TYPE`）执行前 SHALL 先删除该索引。

#### Scenario: L2 候选召回走索引而非全量遍历

- **WHEN** 一次打标签请求产生 N 个 L1 未命中的辅助标签
- **THEN** 每个标签的 L2 判定 SHALL 由数据库索引检索完成，扫描行数 SHALL 与 active 辅助标签总数无关
- **AND** 进程 SHALL NOT 因去重而持有全部 active 辅助标签的 `merge_embedding` 向量副本

#### Scenario: 召回候选被截断时的兜底

- **WHEN** 首次召回的候选上限条候选经精确重排后全部不低于去重阈值
- **THEN** 系统 SHALL 以更大的候选上限（并相应提高 `hnsw.ef_search`）重查一次后再裁决

#### Scenario: 维度变化后索引重建

- **WHEN** embedding 运行时维度由 2560 变为其它值（更换模型）
- **THEN** 系统 SHALL 在存量旧维度行清理后重建索引以匹配新维度
- **AND** 存量维度与运行时维度不一致时 SHALL 显式报错并给出清理指引，SHALL NOT 声称「降级为顺序扫描且结果正确」
- **AND** SHALL NOT 继续使用旧维度表达式的索引
- **AND** 若需执行列维度过渡语句，该语句执行前旧索引 SHALL 已被删除

### Requirement: L2 阈值裁决使用原列精确距离

系统 SHALL 以召回候选在原 `vector` 列上的精确余弦距离做去重阈值裁决，SHALL NOT 用 halfvec 近似距离裁决。

- 候选裁决沿用既有口径：cosine ≥ 去重阈值、`RefCount` 大者优先、同 `RefCount` 取 `ID` 小者。
- 归并路径的其它不变量 SHALL 保持不变：只 addAlias、不改 Label、不重算 MergeEmbedding、不触碰 `embedding` / `merge_embedding` 列。

#### Scenario: 阈值判定不受 halfvec 近似误差影响

- **WHEN** 某候选的 halfvec 近似距离与精确 float32 距离分别落在去重阈值两侧
- **THEN** 系统 SHALL 以精确 float32 距离为准判定是否命中

#### Scenario: 归并结果与全量遍历一致

- **WHEN** 同一批样本标签分别用旧实现（进程内全量余弦）与新实现（召回 + 精确重排）判定
- **THEN** 阈值内的候选裁决 SHALL 一致（同一 label 归并到同一目标 ID，或同样判定为 L3 新建）
- **AND** 对账中出现的差异 SHALL 逐条记录并归因，SHALL NOT 默许

#### Scenario: 归并路径仍不触碰向量列

- **WHEN** L2 命中并执行 alias 归并
- **THEN** `embedding` 与 `merge_embedding` 列 SHALL 保持原值不变（沿用既有的「Alias 变更不得触碰向量列」要求）

### Requirement: L1 索引增量写入对并发读者安全

L1 精确匹配索引的增量写入 SHALL 与无锁读者互斥，SHALL NOT 在读者可能并发访问时原地写 map。

#### Scenario: 并发打标签不崩进程

- **WHEN** 打标队列并发执行多个 attach，其中一个走到 L3 新建（增量吸收）而其它正在做 L1 匹配
- **THEN** 系统 SHALL 不出现 `concurrent map read and map write` 致命错误

### Requirement: 辅助标签垃圾回收（GC）

<!-- from: aux-label-gc -->

系统 SHALL 支持扫描并清理无活跃 topic_tag 引用的辅助标签。清理时 SHALL 跳过 protected=true、创建时间在 grace_days 内、或 status 非 active 的标签。清理策略支持四种模式：dry_run（仅预览）、disable（软删除）、delete（硬删除）、recalculate（仅重算 ref_count）。

#### Scenario: GC dry_run 预览
- **WHEN** 用户调用 GC API mode=dry_run, grace_days=1
- **THEN** 系统 SHALL 返回符合条件的标签数量和前 20 个示例列表，不修改任何数据

#### Scenario: GC disable 软删除
- **WHEN** 用户调用 GC API mode=disable, grace_days=1
- **THEN** 系统 SHALL 将符合条件的辅助标签 status 更新为 "disabled"，同时删除 `board_composition` 中引用这些标签的行，并返回实际影响的标签数量

#### Scenario: GC delete 硬删除
- **WHEN** 用户调用 GC API mode=delete, grace_days=1
- **THEN** 系统 SHALL 硬删除符合条件的辅助标签记录（含 embedding 向量，CASCADE 自动清理 board_composition），并返回实际删除的标签数量

#### Scenario: GC recalculate 存量校准
- **WHEN** 用户调用 GC API mode=recalculate
- **THEN** 系统 SHALL 对所有 label_type='auxiliary' 的 semantic_labels 执行 ref_count 全量重算（基于 topic_tag_semantic_labels 中的实际计数），并返回重算的标签数量和修正数量

#### Scenario: 跳过有活跃引用的标签
- **WHEN** auxiliary label "AI" 在 topic_tag_semantic_labels 中有关联行
- **THEN** 该标签 SHALL NOT 被 GC 清理

#### Scenario: 跳过 protected 标签
- **WHEN** auxiliary label "AI" 的 protected=true，即使无任何 topic_tag 引用
- **THEN** 该标签 SHALL NOT 被 GC 清理

#### Scenario: 跳过 grace_days 内新建标签
- **WHEN** auxiliary label "量子计算" 创建于 12 小时前，无任何 topic_tag 引用，grace_days=1
- **THEN** 该标签 SHALL NOT 被 GC 清理

#### Scenario: 清理后减少匹配负载
- **WHEN** 100 个无引用的 auxiliary label 被 GC disable
- **THEN** 后续 `loadActiveAuxiliaryLabels` 和 board 匹配 SHALL 排除这些 disabled 标签

#### Scenario: GC disable 清理 board_composition
- **WHEN** auxiliary label #42 被 GC disable，且 `board_composition` 中存在 board_id=5, auxiliary_label_id=42 的行
- **THEN** 该 `board_composition` 行 SHALL 被删除，board #5 的 composition 页面不再显示该标签

### Requirement: AuxLabelCleanup 定时任务

<!-- from: aux-label-gc -->

系统 SHALL 提供每小时执行一次的 AuxLabelCleanup 定时任务，自动对无活跃引用的辅助标签执行 disable 清理。任务 SHALL 通过现有调度器框架管理，支持查看状态、手动触发、重置统计、修改间隔。

#### Scenario: 定时自动执行
- **WHEN** AuxLabelCleanup 调度器运行中，到达下一次执行时间
- **THEN** 系统 SHALL 执行 mode=disable, grace_days=1 的 GC 清理，记录执行结果到 SchedulerTask 数据库记录

#### Scenario: 手动触发执行
- **WHEN** 用户在 GlobalSettings 定时任务面板点击 "手动执行" AuxLabelCleanup
- **THEN** 系统 SHALL 立即执行一次 GC 清理，返回执行结果

#### Scenario: 调度器状态查询
- **WHEN** 用户打开 GlobalSettings 定时任务面板
- **THEN** AuxLabelCleanup 任务 SHALL 显示名称、状态、执行次数、上次执行时间、执行耗时等统计信息

#### Scenario: 调度器跳过重复执行
- **WHEN** AuxLabelCleanup 正在执行中，到达下一次定时触发时间
- **THEN** 系统 SHALL 跳过本次执行，等待下次周期

### Requirement: 手动 GC API

<!-- from: aux-label-gc -->

系统 SHALL 提供 `POST /api/auxiliary-labels/gc` 端点，允许手动触发 GC 操作，支持指定 mode（dry_run / disable / delete / recalculate）和 grace_days 参数。

#### Scenario: 手动 dry_run
- **WHEN** POST /api/auxiliary-labels/gc body={"mode":"dry_run","grace_days":1}
- **THEN** 返回 eligible_count 和 preview 列表，不修改数据

#### Scenario: 手动 disable
- **WHEN** POST /api/auxiliary-labels/gc body={"mode":"disable","grace_days":1}
- **THEN** 符合条件的标签 status 设为 "disabled"，关联 board_composition 行删除，返回 affected_count

#### Scenario: 手动 delete
- **WHEN** POST /api/auxiliary-labels/gc body={"mode":"delete","grace_days":1}
- **THEN** 符合条件的标签被硬删除，返回 affected_count

#### Scenario: 手动 recalculate
- **WHEN** POST /api/auxiliary-labels/gc body={"mode":"recalculate"}
- **THEN** 所有 auxiliary label 的 ref_count 被重算，返回 total_count 和 corrected_count

#### Scenario: 参数校验
- **WHEN** mode 不为 dry_run / disable / delete / recalculate 之一
- **THEN** 返回 400 错误

#### Scenario: grace_days 默认值
- **WHEN** 请求未提供 grace_days
- **THEN** 系统 SHALL 使用默认值 1

#### Scenario: recalculate 忽略 grace_days
- **WHEN** mode=recalculate
- **THEN** 系统 SHALL 忽略 grace_days 和 protected/status 过滤条件，对所有 auxiliary label 执行 ref_count 重算

### Requirement: 共享描述修复须人工确认

<!-- from: auxiliary-label-description-repair (2026-11-06 board-health-triage-advice) -->

系统 SHALL 提供从体检问题项进入的辅助标签描述修复能力：展示实时原描述、建议文案、可编辑新描述、直接引用版块及全局共享影响。描述须 trim 后非空、不仅重复 label、不超过 500 字符。只有用户明确确认才可创建修复任务；只查看建议/打开页面/运行体检 MUST NOT 创建任务。disabled、非 auxiliary 或不存在的标签 MUST 被拒绝。

#### Scenario: 核对并确认共享影响
- **WHEN** 用户确认将「上海」描述从商业场所表述改为城市定义
- **THEN** 系统 SHALL 在提交前声明是共享标签、使用它的版块均受影响、匹配更新覆盖全部活跃话题标签，用户取消时零写入

#### Scenario: 无效描述与禁用标签
- **WHEN** 用户提交空白、超过 500 字符、仅重复标签名的描述，或标签已 disabled/非 auxiliary
- **THEN** 系统 SHALL 返回明确错误且不修改描述、向量或匹配，不将任务显示为成功

### Requirement: 描述与 storage 向量原子更新

<!-- from: auxiliary-label-description-repair (2026-11-06 board-health-triage-advice) -->

系统 SHALL 使用最终确认的新描述生成 storage embedding（label + ': ' + description），生成成功后在事务中校验来源版本、类型与状态并原子保存 description+embedding。MUST NOT 清空旧向量后复用 NULL 回填；MUST NOT 修改 label、aliases、merge_embedding、组合标签向量。生成/保存失败时旧描述和旧向量均保持。并发修改冲突 MUST 明确拒绝并引导重新核对，不能覆盖较新描述。

#### Scenario: 生成失败保持旧数据
- **WHEN** 修复任务生成新向量失败
- **THEN** 原描述与原 storage/merge 向量 SHALL 保持，任务记录失败原因并可重试

#### Scenario: 修改说明不改变标签身份
- **WHEN** 新描述与 storage 向量提交成功
- **THEN** label/aliases/merge_embedding SHALL 原值不变，description 与 storage 向量均为对应的新版本，不出现半更新

#### Scenario: 来源版本已变化
- **WHEN** 用户核对后、事务保存前标签已被其他操作修改或禁用
- **THEN** 系统 SHALL 拒绝覆盖，记录冲突或不可用状态；界面保留草稿并要求加载最新版本重新核对

### Requirement: 修复任务持久且幂等

<!-- from: auxiliary-label-description-repair (2026-11-06 board-health-triage-advice) -->

任务 SHALL 持久记录请求幂等键、标签及来源版本、用户确认的新描述、执行阶段、保存版本、匹配范围与进度、失败原因。相同幂等键重试 MUST 返回同一任务；同标签未结束任务不能并发覆盖。页面关闭不取消已确认任务，worker SHALL 绑定应用生命周期而非提交请求的 context。服务重启后须可查询并恢复失租的未结束阶段；明确失败态 SHALL 等待用户重试，不自动无限重跑。不得凭内存 job 或组件本地状态宣称可恢复。

#### Scenario: 双击与请求重试不重复执行
- **WHEN** 同一提交因双击/超时使用相同幂等键重发
- **THEN** 系统 SHALL 返回同一任务，不重复重嵌、不重复提交描述

#### Scenario: 离开或服务重启后再进入
- **WHEN** 用户关闭页面或服务重启后查询已确认的任务
- **THEN** 系统 SHALL 返回持久阶段与结果；已保存的描述不再次覆盖，未完成匹配可继续或明确重试

#### Scenario: 运行中与完成后的重试幂等
- **WHEN** 用户重复重试已运行或已完成的任务
- **THEN** 系统 SHALL 返回202及同一任务当前状态，不再排队、不启动重复worker、不修改范围/计数/时间；conflict/unavailable则明确409要求重新核对，不借retry修改原描述请求或来源版本

#### Scenario: 失败等待明确重试
- **WHEN** 已确认任务进入embedding_failed、save_failed或matching_failed
- **THEN** 后台 SHALL 不自动无限重跑；用户retry只重新排入同一任务，已提交描述只续匹配，旧失败清空后展示新尝试

### Requirement: 修复后更新匹配并独立重试

<!-- from: auxiliary-label-description-repair (2026-11-06 board-health-triage-advice) -->

描述与新向量提交后 SHALL 失效 board 匹配缓存，并用既有匹配引擎更新任务范围内全部活跃 topic_tags 的版块匹配，包含当前未归属任何版块的标签；任务范围在执行起点固定并可恢复，后续新标签沿既有实时匹配路径处理。MUST NOT 仅枚举现有匹配行或只处理当前版块，MUST NOT 改匹配算法或重嵌全部文章。任务 SHALL 返回冻结总数 total、真实成功 processed、失败 failed、不再适用 not_applicable 与未完成 remaining；所有计数由服务端提供，四类之和 SHALL 等于 total。成功指实际调用匹配引擎且正常返回（包括合法零命中）；未调用、报错或失效跳过 MUST NOT 冒充成功。冻结后被删除、禁用或合并的范围项 SHALL 持久记录为不再适用，total 保持不变，不永久卡死任务。部分失败不得标全完成。匹配失败后仅重试失败或未完成匹配，不重嵌/再次保存描述。

#### Scenario: 未归属标签也进入更新范围
- **WHEN** 修复时某活跃 topic_tag 在匹配表没有任何行
- **THEN** 该标签 SHALL 仍被纳入本次匹配范围并由既有匹配引擎处理

#### Scenario: 匹配失败只重试匹配
- **WHEN** 描述和向量已保存，但部分 tag 匹配失败
- **THEN** 任务 SHALL 标部分完成并返回计数；用户重试只继续未完成匹配，embedding 调用次数不再增加

#### Scenario: 冻结后标签不再适用
- **WHEN** 冻结范围中的标签在处理前已删除、禁用或合并
- **THEN** 系统 SHALL 不调用该项匹配，持久记录不再适用，not_applicable 增加而 processed/failed 不增加，total 不缩小；其余适用项仍正常执行

#### Scenario: 成功执行可合法零命中
- **WHEN** 既有匹配引擎对适用标签成功执行并返回空命中结果
- **THEN** 系统 SHALL 按成功执行计入 processed，不伪造版块命中，不将正常空结果误报失败

### Requirement: 更新完成与效果验证分离

<!-- from: auxiliary-label-description-repair (2026-11-06 board-health-triage-advice) -->

界面 SHALL 展示生成向量、原子保存、匹配更新三个真实阶段及可执行恢复动作。仅在冻结范围所有仍适用项均匹配成功、其余项均有明确不再适用记录且无失败/未完成项后才显示「更新完成，效果待下一轮体检验证」，并引导处理下一项。旧体检快照、既有日报正文、历史评分 MUST 保持，不因修复被覆盖；不自动触发额外体检，不把复制、已排队、描述已保存或部分完成当成效果已改善。

#### Scenario: 结果不伪造健康改善
- **WHEN** 修复任务成功结束
- **THEN** 用户 SHALL 看到匹配更新完成与下一步处理入口，同时知道效果待下一轮体检验证；旧评分和旧快照不变

### Requirement: Slug generation normalizes whitespace

<!-- from: tag-hierarchy-quality (2026-05-10 fix-tag-hierarchy-inflation) -->

The `Slugify` function SHALL collapse consecutive whitespace characters into a single space before applying any other transformations, so that labels differing only in whitespace (e.g., "DeepSeek首轮融资" and "DeepSeek 首轮融资") produce the same slug.

#### Scenario: Labels with space versus no-space produce same slug
- **WHEN** `Slugify("DeepSeek首轮融资")` and `Slugify("DeepSeek 首轮融资")` are called
- **THEN** both SHALL return `deepseek 首轮融资`

#### Scenario: Multiple consecutive spaces collapse to single space
- **WHEN** `Slugify("foo   bar")` is called
- **THEN** the result SHALL be `foo bar`

#### Scenario: Leading and trailing whitespace trimmed
- **WHEN** `Slugify("  hello world  ")` is called
- **THEN** the result SHALL be `hello world` (spaces preserved between words, punctuation removed)

### Requirement: Abstract tag creation requires minimum information gain

<!-- from: tag-hierarchy-quality (2026-05-10 fix-tag-hierarchy-inflation) -->

Before creating a new abstract tag, the system SHALL verify that:
1. At least 2 distinct child tags are proposed
2. No pair of proposed child tags shares more than 70% of their associated articles (Jaccard similarity)
3. The resulting tree SHALL have a leaf-to-depth ratio of at least 1.5

If any of these checks fail, the abstract tag creation SHALL be rejected and logged.

#### Scenario: Too few children rejected
- **WHEN** LLM proposes an abstract tag with only 1 candidate child
- **THEN** the system SHALL reject creation and log a warning

#### Scenario: High article overlap rejected
- **WHEN** two candidate child tags share 80% of their associated articles
- **THEN** the system SHALL reject abstract creation and log a warning suggesting merge instead

#### Scenario: Acceptable abstract passes all checks
- **WHEN** LLM proposes an abstract with 3 children whose maximum pairwise Jaccard is 0.3 and the resulting tree leaf-to-depth ratio is 2.0
- **THEN** the system SHALL proceed with abstract tag creation

### Requirement: Existing whitespace-variant duplicate tags are cleaned up

<!-- from: tag-hierarchy-quality (2026-05-10 fix-tag-hierarchy-inflation) -->

A scheduled cleanup task SHALL detect active tags whose slugs would be identical after whitespace normalization (`\s+` → single space) and merge them, migrating article associations, embeddings, and hierarchy relations to the surviving tag.

#### Scenario: Whitespace variant pair merged
- **WHEN** cleanup runs and finds two active tags "DeepSeek首轮融资" (slug: `deepseek首轮融资`) and "DeepSeek 首轮融资" (slug: `deepseek 首轮融资`) that are the same after normalization
- **THEN** the tag with fewer article associations SHALL be merged into the other (marking it `status=merged`, `merged_into_id` set), all article_topic_tags SHALL be migrated, and duplicate article-tag pairs SHALL be skipped

### Requirement: Degenerate abstract trees are flattened

<!-- from: tag-hierarchy-quality (2026-05-10 fix-tag-hierarchy-inflation) -->

A scheduled cleanup task SHALL detect abstract tag chains where the leaf-to-depth ratio falls below 1.5 and flatten them by removing intermediate abstract nodes, relinking children to the nearest ancestor that provides meaningful grouping.

#### Scenario: Four-level chain with 5 leaves flattened
- **WHEN** cleanup encounters A→B→C→D with D's children being 5 leaf tags
- **THEN** intermediate nodes B and C SHALL be deactivated and their children linked directly to A

### Requirement: Multi-parent assignments are prevented

<!-- from: tag-hierarchy-quality (2026-05-10 fix-tag-hierarchy-inflation) -->

When linking a child tag to a new abstract parent via `linkAbstractParentChild()`, if the child already has an active abstract parent and the new parent's article set overlaps with the existing parent's article set (Jaccard > 0.3), the second parent assignment SHALL be rejected.

#### Scenario: Second parent rejected due to overlap
- **WHEN** tag X has parent A covering articles {1,2,3} and a new parent B covering articles {1,2,4} is proposed (Jaccard = 0.5)
- **THEN** the system SHALL reject assigning B as parent and log the conflict

#### Scenario: Second parent accepted for non-overlapping sets
- **WHEN** tag X has parent A covering articles {1,2} and a new parent B covering articles {3,4,5} is proposed (Jaccard = 0)
- **THEN** the system SHALL accept B as an additional parent

### Requirement: tag_merge_suggestions 表

<!-- from: tag-merge-suggestions -->

系统 SHALL 维护 `tag_merge_suggestions` 表，记录可能需要合并的相似标签对。

#### Schema
- `id`: PK, auto-increment
- `new_tag_id`: FK → topic_tags，被提取的新标签
- `existing_tag_id`: FK → topic_tags，相似的已有标签
- `new_label`: text，新标签的 label
- `existing_label`: text，已有标签的 label
- `category`: text，标签类别
- `similarity`: float，embedding 相似度 (0-1)
- `status`: text，枚举 `pending` / `merged` / `dismissed`，默认 `pending`
- `source`: text，来源 `incremental` / `full_scan`
- `created_at`: timestamp
- `updated_at`: timestamp

#### Constraints
- UNIQUE (`new_tag_id`, `existing_tag_id`)
- INDEX (`status`, `similarity` DESC)

### Requirement: 增量记录合并建议

<!-- from: tag-merge-suggestions -->

`findOrCreateTag` 创建新 tag 时，如果 `TagMatch` 返回 candidates，SHALL 将候选对写入 `tag_merge_suggestions`。

#### Scenario: 新标签有相似候选
- **WHEN** `findOrCreateTag` 的 `TagMatch` 返回 `candidates`
- **AND** 新 tag 创建成功（获得 new_tag_id）
- **THEN** 对每个 candidate 写入一条 `tag_merge_suggestion`（status=pending, source=incremental）
- **AND** 以 `(new_tag_id, candidate_tag_id)` 为唯一键，已存在则 skip

#### Scenario: 新标签无相似候选
- **WHEN** `TagMatch` 返回 `no_match` 或 `exact`
- **THEN** 不写入 suggestion

### Requirement: 异步全量扫描

<!-- from: tag-merge-suggestions -->

用户可手动触发全量扫描，遍历所有 active tag，每个 tag 调用 `FindSimilarTags` 查找相似对，结果写入 `tag_merge_suggestions`。

#### Scenario: 触发全量扫描
- **WHEN** 收到 `POST /merge-preview/scan` 请求
- **THEN** 后端在 goroutine 中启动扫描，立即返回 202
- **AND** 遍历所有 active tag，每个 tag 调用 `FindSimilarTags(tag, category, 10, 'semantic')`
- **AND** 相似度 ≥ LowSimilarity 的结果写入 suggestion（source=full_scan）
- **AND** 已存在的对 skip，不重复写入

#### Scenario: 并发保护
- **WHEN** 已有一个扫描任务在运行
- **AND** 收到新的 `POST /merge-preview/scan` 请求
- **THEN** 返回 409 "scan already in progress"

### Requirement: SSE 进度推送

<!-- from: tag-merge-suggestions -->

全量扫描通过 `GET /merge-preview/scan/stream`（SSE）实时推送进度。

#### Message Format
```json
{
  "status": "scanning",
  "total": 590,
  "scanned": 342,
  "current_category": "keyword",
  "new_suggestions": 23
}
```

扫描完成后发送最终消息：
```json
{
  "status": "done",
  "total": 590,
  "new_suggestions": 47
}
```

#### Scenario: 客户端连接 SSE
- **WHEN** 前端创建 `EventSource('/api/topic-tags/merge-preview/scan/stream')`
- **THEN** 收到 Content-Type: text/event-stream 响应
- **AND** 扫描进行中持续收到 progress 事件
- **AND** 扫描完成后收到 done 事件并关闭

### Requirement: 查询合并建议

<!-- from: tag-merge-suggestions -->

`GET /merge-preview` 改为查询 `tag_merge_suggestions` 表。

#### Scenario: 查询 pending 建议
- **WHEN** 前端调用 `GET /merge-preview`
- **THEN** 返回 `tag_merge_suggestions` 中 status=pending 的记录
- **AND** 按 similarity DESC 排序
- **AND** 附带 source/new_tag 的文章数量统计

### Requirement: 合并后标记 suggestion

<!-- from: tag-merge-suggestions -->

合并操作完成后，SHALL 将相关的 suggestion 标记为 merged。

#### Scenario: 合并后标记
- **WHEN** `POST /merge-with-name` 完成合并（source→target）
- **THEN** 更新 `tag_merge_suggestions` 中 `new_tag_id=source` 或 `existing_tag_id=source` 的 pending 记录为 merged
- **AND** 同时更新涉及 target 的 pending 记录为 merged（因为 target 可能被重命名）

### Requirement: TagsPage 提供标签合并入口

<!-- from: tag-merge-ui -->

TagsPage 左侧栏操作按钮区 SHALL 包含"标签合并"按钮，点击后显示 `TagMergePreview` 组件。

#### Scenario: 点击标签合并按钮
- **WHEN** 用户在 TagsPage 点击左侧栏"标签合并"按钮
- **THEN** 弹出 TagMergePreview 面板
- **AND** 自动从 `tag_merge_suggestions` 表加载 pending 状态的候选对
- **AND** 用户可逐个确认合并、跳过、交换合并方向、编辑合并后名称

#### Scenario: 合并完成
- **WHEN** 用户完成合并操作并关闭面板
- **THEN** TagsPage 刷新标签数据以反映合并结果
- **AND** 已合并的 suggestion 被标记为 `merged` 状态

#### Scenario: 忽略建议
- **WHEN** 用户点击某个候选对的"忽略"按钮
- **THEN** 该 suggestion 被标记为 `dismissed` 状态
- **AND** 不再出现在候选列表中

### Requirement: 异步全量扫描 + SSE 进度展示

<!-- from: tag-merge-ui -->

用户可手动触发全量扫描，发现老标签之间的相似对。扫描异步执行，进度通过 SSE 实时推送到前端。

#### Scenario: 触发全量扫描
- **WHEN** 用户点击"全量扫描"按钮
- **THEN** 后端启动异步扫描任务（同时只允许一个任务运行）
- **AND** 前端通过 SSE 接收进度更新

#### Scenario: SSE 进度推送
- **WHEN** 全量扫描正在运行
- **THEN** 前端实时展示：进度条（scanned/total）、当前处理的类别、已发现的新建议数
- **AND** 扫描完成后自动刷新候选列表

#### Scenario: 扫描已完成时再次触发
- **WHEN** 用户在扫描进行中再次点击"全量扫描"
- **THEN** 返回错误提示"扫描正在进行中"

### Requirement: Embedding 高相似度匹配降级为 candidates

<!-- from: tag-match-identity-protection -->

`TagMatch` 方法中，当 embedding 余弦相似度达到 `HighSimilarity` 阈值时，SHALL 返回 `MatchType: "candidates"`（而非 `"exact"`），将匹配结果作为候选列表返回。`findOrCreateTag` 对 `candidates` 的已有行为是 fall through 到创建新 tag。

#### Scenario: embedding 高相似度不再返回 exact
- **WHEN** 一个标签通过 embedding 搜索匹配到已有标签，cosine 相似度为 0.96（≥ HighSimilarity 0.97）
- **THEN** `TagMatch` 返回 `MatchType: "candidates"`，`ExistingTag` 为 nil，`Candidates` 包含匹配结果
- **AND** `findOrCreateTag` 创建新的独立 tag

#### Scenario: slug 精确匹配仍返回 exact
- **WHEN** `TagMatch` 通过 slug+category 在数据库中找到完全匹配的记录
- **THEN** 返回 `MatchType: "exact"`，`ExistingTag` 为匹配到的标签
- **AND** `findOrCreateTag` 正常合并（更新 label/slug 等）

#### Scenario: alias 精确匹配仍返回 exact
- **WHEN** `TagMatch` 通过 alias 字段匹配到已有标签
- **THEN** 返回 `MatchType: "exact"`，`ExistingTag` 为匹配到的标签
- **AND** `findOrCreateTag` 正常合并

### Requirement: 删除 keyword 类别阈值覆盖

<!-- from: tag-match-identity-protection -->

`CategoryThresholdOverrides` map SHALL NOT 包含 `"keyword"` 键。所有类别的 embedding 匹配统一使用 `DefaultThresholds`（`HighSimilarity: 0.97, LowSimilarity: 0.78`）。

#### Scenario: keyword 类别使用默认阈值
- **WHEN** `ThresholdsForCategory("keyword")` 被调用
- **THEN** 返回 `DefaultThresholds`（`HighSimilarity: 0.97, LowSimilarity: 0.78`）
