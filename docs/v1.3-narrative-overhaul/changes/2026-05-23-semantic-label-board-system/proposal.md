## Why

当前标签→板块的匹配依赖 embedding 余弦相似度直接比对，存在三个核心问题：(1) 新词/OOV（如 happyhorse）embedding 模型不认识导致语义归类失败；(2) 事件标签本身的语义（如"霍尔木兹海峡"）与板块主题（如"地缘政治"）关联弱，匹配不准确；(3) 板块是按 event/keyword/person 三类独立管理的，分类僵化且丢失了跨视角的叙事能力。需要引入辅助标签作为 tag 和 board 之间的统一语义中介层，从根本上重构匹配和板块生成机制。

## What Changes

- **新增辅助标签（Auxiliary Label）体系**：event/person tag 由 LLM 提取时同时生成 3-5 个带 description 的辅助标签；keyword tag 直接以 label+description 进入辅助标签池，不再额外生成辅助标签
- **Tag 提取双分支调用**：event/person 与 keyword 使用独立 prompt/schema 并行提取，避免 auxiliary_labels 对 keyword 无意义、对 event/person 必填造成 batch fail-fast；支持部分成功和分支级错误记录
- **辅助标签 description 增强**：storage-embedding 输入为 label+description，显著提升跨域区分度，根治 embedding 跨域误判
- **Embedding 分离**：新增 `merge_embedding`（仅 label，用于 L2 merge 判断），现有 `embedding` 作为 storage embedding（label+description，用于 board 推荐、匹配、升级聚类和回填）
- **统一语义标签池**：辅助标签和板块共存于同一张 `semantic_labels` 表，辅助标签通过聚类+LLM 升级为板块
- **新匹配逻辑**：tag → board 匹配从 embedding 直接比对改为通过辅助标签交集计算（直接命中 + 间接匹配双因子）
- **板块升级机制**：辅助标签积累到阈值后，预聚类 + LLM 判断（补充 co-tag 事件上下文）生成新板块
- **多板块归属**：一个 tag 可属于多个 board（多视角叙事），默认最多 3 个；同一事件文章允许在多个 board 中重复出现
- **辅助标签 merge**：embedding ≥0.95 自动合并，积累 alias，支持 L1 精确匹配复用
- **语义板块与每日叙事板分层**：`SemanticBoard` 全局共享；`NarrativeBoard` 仍按 global/feed_category 每日生成，并通过 `semantic_board_id` 续接
- **冷启动手动建议**：冷启动允许短期无 board；辅助标签池累计到阈值后，由用户手动触发 LLM 建议并确认执行
- **最小修正能力**：支持禁用辅助标签、手动合并 alias、从 board composition 移除辅助标签
- **BREAKING 删除层级体系**：移除 hierarchy_template、hierarchy_placement、hierarchy_cleanup、abstract_tag 全套逻辑
- **BREAKING 删除旧板块体系**：移除 board_concepts 表及 concept/bootstrap、concept/matcher
- **BREAKING 取消旧热点路径**：不再通过 abstract tree → hotspot board 生成每日板，所有每日叙事板均来自 SemanticBoard
- **前端改造**：板块详情页增加辅助标签筛选维度，标签卡片显示多板块归属

## Capabilities

### New Capabilities
- **辅助标签**: 辅助标签的提取、入库（L1/L2/L3 三级）、merge（≥0.95 + alias 积累）、统一池子管理、description 增强、keyword 直入
- `semantic-label-model`: semantic_labels 统一数据模型及 topic_tag_semantic_labels 关联表
- `board-matching`: tag → board 的新匹配逻辑（直接命中 + 间接匹配：命中率 / max_sim / 加权综合）
- `board-upgrade`: 辅助标签聚类 + LLM 判断（含 co-tag 事件）生成新板块，含手动回填队列
- `board-management-api`: 板块及辅助标签的 CRUD API，含升级建议、回填触发、参数配置
- `semantic-board-narrative`: SemanticBoard → NarrativeBoard 的每日生成、续接、重复事件展示规则

### Modified Capabilities

## Impact

- **数据模型**：新增 semantic_labels 表（替代 board_concepts，含 storage `embedding` 与 `merge_embedding`）、topic_tag_semantic_labels、topic_tag_board_labels、board_composition；删除旧 board_concepts 及层级相关表字段
- **核心逻辑**：tagger.go 提取 prompt 改造（keyword 不生成辅助标签，event/person 辅助标签带 description）；embedding.go 改为辅助标签入库（分离 merge-embedding 和 storage-embedding）；sector_generation.go 改造为升级机制；concept 包重构
- **删除代码**：hierarchy_template/config/placement/cleanup/dedup/aggregation/handler/orchestration/prompts（约 15 个文件）；abstract_tag_*.go（约 6 个文件）；adopt_narrower_queue；tree_bridge；multi_parent_resolve_queue；concept/bootstrap.go；concept/matcher.go
- **API**：新增辅助标签、语义板块、板块升级、回填、人工辅助标签推荐、composition 添加等接口；删除层级相关接口；现有板块 API 适配新模型
- **前端**：板块详情页增加辅助标签筛选 chips；标签卡片显示多板块归属；新增升级建议面板、回填触发、参数配置 UI
- **配置**：新增命名空间化匹配参数（semantic_board_match_*）；升级阈值（semantic_board_upgrade_ref_count_threshold≥5）；多归属上限（semantic_board_match_max_boards=3）
- **历史数据**：不做历史迁移，开发阶段由用户手动清空旧数据后重建
