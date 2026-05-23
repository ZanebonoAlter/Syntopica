## Why

当前叙事系统直接从散装 event 标签生成叙事，缺少中间的组织层。当事件数量较多时，叙事平铺展示难以浏览和筛选，用户无法快速定位感兴趣的领域。需要一个"版块"聚合层，将叙事按主题分组，提供折叠/钻取的浏览体验，同时简化后端 LLM 调用链路。

## What Changes

- **新增版块（Board）概念**：LLM 动态生成的主题聚合层，类似 BBS 论坛版块，包含多个叙事
- **两阶段 LLM 生成流程**：Pass 0 生成版块划分 + 抽象标签归类；Pass 1 每版块并行生成叙事
- **抽象标签映射为叙事卡**：已有抽象标签直接作为版块内的叙事卡片展示，status 根据子标签文章活动计算
- **跨天版块连接**：从叙事 parent_ids 推导版块间的延续/分裂/合并关系
- **全局版块合并**：LLM 判断跨 feed 分类的同名/相似版块合并为全局版块
- **前端双层 Canvas**：版块大节点 → 点击展开叙事小节点，叙事层级的连线关系
- **BREAKING 删除旧流程**：移除 GenerateCrossCategoryNarratives、GenerateWatchedTagNarratives、散装 event 直接生成叙事的旧 Pass 2
- **BREAKING 数据清空**：现有 narrative_summaries 数据已清空，使用新版块模型重新生成

## Capabilities

### New Capabilities
- `narrative-boards`: 版块的生成、持久化、跨天匹配、全局合并、抽象标签归类
- `narrative-board-canvas`: 前端版块-叙事双层 Canvas 交互（版块大节点 + 展开叙事小节点）

### Modified Capabilities

## Impact

- **后端**：`backend-go/internal/domain/narrative/` 大幅重构（service、generator、collector），新增 board 相关文件和模型，`jobs/narrative_summary.go` 调度器重写
- **前端**：`front/app/features/topic-graph/components/` NarrativePanel 和 NarrativeCanvas 重写
- **API**：新增版块相关端点（timeline 包含版块层级），现有叙事 API 响应结构变更
- **数据模型**：新增 `narrative_boards` 表，`narrative_summaries` 表新增 `board_id` 字段和 `source` 扩展值
- **LLM 调用**：从 4 层调用简化为 2 层 + fallback，prompt 全部重写
