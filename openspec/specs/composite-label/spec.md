## Purpose

组合标签（composite label）实体全生命周期：由多个中性辅助标签组合而成、具备指向性的语义单元（如「美国国债 × 收益率」→「美债收益率」），覆盖数据模型、embedding 生成、去重 canonical 化与治理操作。

## Requirements

### Requirement: 组合标签数据模型
系统 SHALL 在 `semantic_labels` 表中以 `label_type="composite"` 存储组合标签，并使用 `composite_components` 关联表存储有序组件引用。每条组件记录 SHALL 包含 composite_id、component_label_id（指向 `label_type="auxiliary"` 的 semantic_labels）、position（组件顺序，从 1 开始）。单个组合标签 SHALL 包含 2-5 个组件。组合标签 SHALL 复用 semantic_labels 的通用字段（label、slug、embedding、aliases、ref_count、description、source、status 等），source 取值 "upgrade_suggest" 或 "manual"。

#### Scenario: 升级建议确认创建组合标签
- **WHEN** 用户确认 compose 建议「美债收益率」（组件：美国国债、收益率）
- **THEN** 创建 semantic_labels 记录（label_type="composite", source="upgrade_suggest", status="active"）及 2 条 composite_components 记录（position=1 指向「美国国债」，position=2 指向「收益率」）

#### Scenario: 手动创建组合标签
- **WHEN** 用户在治理面板手动创建组合标签「中国CPI」，选择组件 [中国, CPI]
- **THEN** 创建 semantic_labels 记录（label_type="composite", source="manual", status="active"）及对应组件引用

#### Scenario: 组件数量约束
- **WHEN** 创建组合标签时提供 6 个组件
- **THEN** 系统 SHALL 拒绝创建并返回明确错误

#### Scenario: 组件必须指向辅助标签
- **WHEN** 创建组合标签时某组件引用的 semantic_label 不是 label_type="auxiliary"
- **THEN** 系统 SHALL 拒绝创建并返回明确错误

### Requirement: 组合标签 embedding 由 LLM 对组合短语生成
系统 SHALL 在组合标签创建时，以「组合标签 label + 组件序列化文本」为输入调用 LLM embedder 生成 embedding。系统 SHALL NOT 通过组件向量加权/平均合成组合 embedding——合成结果丢失组合的指向性语义。

#### Scenario: 创建时生成 embedding
- **WHEN** 组合标签「美债收益率」被创建
- **THEN** 系统调用 embedder 对该组合标签的语义短语生成 embedding 并写入 embedding 字段

#### Scenario: embedder 失败则创建失败
- **WHEN** 创建组合标签时 embedder 调用失败
- **THEN** 创建事务回滚，组合标签与组件引用均不落库，错误返回给调用方

### Requirement: 组合标签去重 canonical 化
系统 SHALL 对新建组合标签执行两级去重：L1——组件引用先归一到各组件 aux label 自身的 canonical ID，比较 canonical 组件 ID 集合（无序集合相等）是否与既有组合相同，相同则复用既有组合（ref_count++）；L2——组件集合不同但组合 embedding cosine ≥ composite_dedupe_sim（默认 0.95，ai_settings 可配）时，将新组合名作为 alias 追加到既有组合（ref_count++），不新建行。L1/L2 均未命中才新建。系统 SHALL NOT 对组合标签使用 merge_embedding 做去重。

#### Scenario: L1 canonical 集合相等复用
- **WHEN** 创建组合标签「美债收益率」（组件 canonical 集合 {美国国债, 收益率}）时，已存在组合「美国国债收益率」组件 canonical 集合同为 {美国国债, 收益率}
- **THEN** 系统 SHALL NOT 新建，复用既有组合并 ref_count++

#### Scenario: L2 embedding 相似追加 alias
- **WHEN** 新组合「美债利率」组件集合 {美国国债, 利率} 与既有组合 {美国国债, 收益率} 集合不同，但组合 embedding cosine=0.96 ≥ 0.95
- **THEN** 系统 SHALL NOT 新建，将「美债利率」追加到既有组合的 aliases 并 ref_count++

#### Scenario: 均未命中新建
- **WHEN** 新组合的 canonical 组件集合与所有既有组合不同，且组合 embedding 与所有既有组合 cosine < 0.95
- **THEN** 系统新建 label_type="composite" 的 semantic_labels 记录

### Requirement: 组合标签挂载复用关联表
系统 SHALL 允许 topic_tag 通过 `topic_tag_semantic_labels` 关联组合标签，允许 SemanticBoard 通过 `board_composition` 挂载组合标签，关联表结构不变。组合标签的 ref_count SHALL 由挂载关系维护。

#### Scenario: tag 挂载组合标签
- **WHEN** tag「美债收益率创两周新高」关联组合标签「美债收益率」
- **THEN** 创建 topic_tag_semantic_labels 记录，组合标签 ref_count++

#### Scenario: 版块 composition 挂载组合标签
- **WHEN** 用户将组合标签「美债收益率」加入版块「美债观察」的构成
- **THEN** 创建 board_composition 记录指向该组合标签

### Requirement: 组合标签禁用即弃向量
组合标签的禁用 SHALL 继承「禁用即弃向量」红线：任何将组合标签 status 置为 disabled 的路径 MUST 同事务将 embedding 置 NULL（行本体、组件引用与 aliases 保留）；重新启用时由系统重算 embedding。

#### Scenario: 禁用组合标签
- **WHEN** 用户禁用组合标签「美债收益率」
- **THEN** 该行 embedding 置 NULL，composite_components 引用与 aliases 保留，后续匹配不再使用该组合

### Requirement: 组合标签治理操作
系统 SHALL 提供组合标签的列表查询（含组件展示、ref_count、状态过滤）、手动创建（校验组件类型与数量、去重）、禁用/启用 API。列表 SHALL 展示每个组合的组件标签序列。

#### Scenario: 列表展示组件
- **WHEN** 用户查看组合标签列表
- **THEN** 每个组合标签条目 SHALL 展示 label、组件序列（按 position）、ref_count、status

#### Scenario: 手动创建触发去重
- **WHEN** 用户手动创建的组合标签命中 L1/L2 去重
- **THEN** 系统 SHALL 返回复用/合并结果而非报错，并提示既有组合标签

#### Scenario: 组件候选推荐排序
- **WHEN** 用户打开手动创建对话框加载组件候选
- **THEN** 系统 SHALL 按推荐度返回 active 辅助标签候选：被 active 版块挂载者优先（挂载数多者在前），其后按 ref_count，同分按 label；每个候选 SHALL 携带挂载版块名列表，默认列表有限长度，搜索时 SHALL 支持全量模糊检索

#### Scenario: 版块上下文创建组合
- **WHEN** 用户从版块详情的「组合标签」区发起创建（board_id 上下文）
- **THEN** 组件候选 SHALL 将该版块已挂载的辅助标签置顶并标记「本版块」；创建成功后系统 SHALL 自动将组合标签挂载到该版块；版块详情 SHALL 展示已挂载组合（名称 + 组件链）并支持移除

#### Scenario: 已选组件共现联动重排
- **WHEN** 用户在创建对话框选中一个组件
- **THEN** 组件候选 SHALL 按与该组件的同 tag 共现频次实时重排（共现高者上浮并标记共现次数），引导用户发现可组合的搭档组件

#### Scenario: 已选组件提示现有组合
- **WHEN** 用户已选至少一个组件且存在含该组件的现有组合标签
- **THEN** 前端 SHALL 优先展示相关现有组合（名称 + 组件链）供参考；选中集合与某组合组件集完全一致时 SHALL 提示创建将复用该组合
