# PRD: 无概念兜底归属策略

> **状态**: 待讨论
> **优先级**: 低（标签系统功能完整后再考虑）
> **关联**: 标签层级放置流程 `PlaceTagInHierarchy`

## 背景

当前标签层级放置流程 (`PlaceTagInHierarchy`) 依赖 `MatchTagToConcept()` 匹配概念板块。如果某标签所属领域没有对应概念板块，标签获得 `action="no_matching_concept"` 并**永久悬浮**——不会被放置到层级树中任何位置。

随着标签库增长，无概念兜底的标签会持续累积为"孤岛"。

## 当前行为

```
新标签 → PlaceTagInHierarchy()
          ├─ 有匹配概念 → 放置到对应层级 ✅
          └─ 无匹配概念 → action="no_matching_concept" → 永久悬浮 ❌
```

## 需要讨论的方案

### 方案 A: 自动生成概念板块
当累积 ≥3 个相似无归属标签（embedding similarity ≥ 0.75）时，自动触发概念板块生成。

- 优点: 完全自动化，`GenerateAnchorSignals()` 已有类似逻辑
- 风险: 可能生成过多碎片化概念

### 方案 B: "未分类" 兜底板块
每类标签（event/keyword/person）维护一个系统级兜底概念板块，无归属标签默认放入。

- 优点: 简单可靠，保证所有标签可见
- 风险: 兜底板块可能积累过多标签

### 方案 C: 延迟放置 + 定期重试
现有 `RetryOrphanPlacements()` 已有重试逻辑，但没有概念板块时重试永远失败。可以改为：重试时检查是否有新概念板块匹配。

- 优点: 最小改动
- 风险: 如果永远不创建对应概念，标签永远悬浮

## 待决策问题

1. 是否需要兜底？还是接受"无概念则不归属"的设计？
2. 无归属标签对用户的影响有多大？（当前前端已有"未归属"折叠区展示）
3. 概念板块生成是否应该更积极（阈值更低、自动触发）？

## 影响范围

- `backend-go/internal/domain/tagging/hierarchy_placement.go` — 放置逻辑
- `backend-go/internal/domain/tagging/sector_generation.go` — 概念生成
- `backend-go/internal/jobs/tag_hierarchy_placement.go` — 调度器
