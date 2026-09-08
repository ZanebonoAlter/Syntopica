<!-- complexity: complex -->
<!-- constraint-domains: semantic-board -->

## Why

语义版块归类存在结构性漏网之鱼：挂载体系的基本单元是中性辅助标签（CPI、收益率、国债等"具体语义锚点"），缺乏指向性。指向信息在 LLM 提取辅助标签那一刻就丢了，导致「中国国债收益率」的文章能借 {国债, 收益率} 重叠 direct_hit 直进「美国国债」版块。现有 direction check（tag identity embedding vs board embedding cosine）度量的是主题相关性而非指向一致性（反义词在 embedding 空间是近邻），调阈值治不了；且 direct_hit 恰好豁免方向校验——最强匹配路径最盲。升级建议同源带病：中性标签聚类产出的新板天然还是中性的，一代代自我复制问题。

引入**组合标签**（composite label）：把「美国国债 × 收益率」这类高频共现的中性标签组合升级为有指向性的语义单元（「美债收益率」），让挂载从"中性概念"升级为"指向性主题"，从根上解决特异性误归类（A 类痛点；走势方向 B 类暂不在本 change 范围）。

## What Changes

### 一期核心闭环

- **组合标签实体**：`semantic_labels` 新增 `label_type=composite`（复用现有表与挂载关系），新表 `composite_components` 存有序组件引用（组件=已有 auxiliary label）；组合 embedding 由 LLM 对组合短语重新 embed（禁止组件向量合成——合成结果仍是中性域向量）。
- **去重 canonical 化**：组件先归一到 aux canonical ID 再比集合（L1）；组合 embedding 相似兜底（L2）；组合空间是 O(n²)，从根上防碎片化。
- **升级建议新增 compose 决策**：co-tag 高频共现对（现有 `loadCoTagEventContext` 统计基础设施，30 天窗口）→ 建议创建组合标签，复用现有 suggestion 生命周期（pending/watch/dismissed/confirmed + suggestion_hash 幂等 + 冷却期）。语义粒度阶梯：aux → composite → board。
- **匹配规则升级**：composite direct_hit 为最强信号（score=1.0，指向一致天然免检）；单 aux 重叠 direct_hit 降级为弱信号（score 打折并强制走 direction check）——堵住「中性标签重叠免检直进」漏洞。
- **存量迁移**：匹配规则变更影响存量归类，需全量重跑 board 匹配 backfill。
- **治理面板**：组合标签的查看/手动创建/禁用（禁用即弃向量红线继承）；升级建议面板渲染 compose 类型建议与确认执行。

### 开放问题（有意推迟，见 design.md 详述）

- 提取侧 LLM 直接识别组合（文章明说"美债收益率"时 LLM 标记组合、组件 L1 匹配防碎片）——待升级建议产线验证组合标签质量后再评估。
- 组合标签成簇后升级为版块的完整链路（composite → board 的第二级升级建议）。
- 走势方向槽位（B 类：上行/下行作为组合第三槽位；反义词 embedding 近邻，需受控枚举而非 embedding 去重）。
- 存量高频共现对的初始回填（一次性建议批量生成 vs 等定时任务自然涌现）。

## Capabilities

### New Capabilities

- `composite-label`: 组合标签实体全生命周期——数据模型（label_type=composite + composite_components 有序引用）、LLM embedding 生成、去重 canonical 化（组件集合 L1 + 组合 embedding L2）、禁用即弃向量、治理面板管理操作。

### Modified Capabilities

- `tag-to-board-matching`: 匹配规则升级——composite direct_hit 新增为最强信号；单 aux 重叠 direct_hit 从免检 score=1.0 降级为打折弱信号并强制 direction check；存量重跑 backfill 后归类结果变化。
- `board-upgrade`: 升级建议新增 compose 决策类型——co-tag 高频共现对产出「建议创建组合标签」，复用现有生命周期（hash 幂等/冷却期/watch/确认执行事务联动）；LLM 提示词与前端面板适配新决策类型。
- `semantic-label-model`: `semantic_labels` 统一数据模型扩展第三种 label_type=composite；挂载关系表（topic_tag_semantic_labels / board_composition）对组合标签的复用语义。

## Impact

- **后端**：`backend-go/internal/tagmanagement/service/board/`（匹配四规则、升级建议生成/确认执行、co-tag 统计）、`backend-go/internal/tagmanagement/service/auxlabel/`（组合标签入库/去重/embedding）、`backend-go/internal/models/semantic_label.go`、handler 层（匹配详情/升级建议/组合标签 CRUD）、`internal/platform/database/`（迁移 + backfill）。
- **数据库**：新表 `composite_components`；`semantic_labels` 复用（新 label_type 值）；升级建议表 suggestion 数据兼容（decision 新值 compose）；一次性全量重跑匹配 backfill。
- **前端**：治理面板（组合标签管理）、升级建议面板（compose 决策渲染/确认）、匹配详情（composite 命中展示）。
- **文档**：`docs/reference/flow/semantic-board.md`（匹配规则/粒度阶梯/新约束）、`docs/reference/api/`、`docs/reference/database/`。
- **兼容性**：单 aux direct_hit 降级改变存量归类行为（用户可见），随 backfill 生效；不做 compose 建议时系统行为与现状一致。
