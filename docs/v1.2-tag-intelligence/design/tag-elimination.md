# 标签淘汰机制设计

> 日期: 2026-04-18
> 状态: 已确认
> 灵感来源: evomap.ai 的 freshness 衰减 + epigenetic 生命周期模型

## 背景与动机

当前 `topic_tags` 只有 `active` 和 `merged` 两种状态。标签一旦创建就永远 active，即使关联的文章已删除或话题早已过气。随着标签数量持续增长，需要一种自动化的标签生命周期管理机制：

1. **活跃度衰减** — 长期无新文章关联的标签应逐渐降温、最终淘汰
2. **删除级联清理** — 文章删除后，孤儿标签应被及时处理

## 设计概览

引入 freshness_score 指数衰减模型（evomap 风格），配合三态生命周期：

```
active → dormant → retired
              ↑         |
              |         x (不可恢复)
              +─────────+
              新文章关联时自动恢复
```

## 1. 数据模型变更

### `topic_tags` 表新增字段

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `freshness_score` | `float64` | `1.0` | 活跃度分数 0~1，指数衰减 |
| `last_active_at` | `timestamp` | `NOW()` | 最后一次有文章关联的时间 |
| `dormant_at` | `timestamp nullable` | `null` | 进入 dormant 状态的时间 |

### `status` 字段扩展

| 状态 | 含义 |
|------|------|
| `active` | 正常标签，出现在所有查询和推荐中 |
| `merged` | 已合并到其他标签（现有逻辑不变） |
| `dormant` | 休眠，不再出现在默认查询中，但可被搜索到，可自动恢复 |
| `retired` | 已淘汰，不出现在任何查询中，不可恢复 |

## 2. Freshness 衰减公式

```
freshness = exp(-days_since_last_active / 90)
```

- 半衰期 ≈ 62 天
- `last_active_at` 更新时机：新文章关联该标签时（`article_topic_tags` INSERT）
- 每日定时巡检时重算

衰减示例：

| 天数 | freshness |
|------|-----------|
| 0 | 1.00 |
| 30 | 0.72 |
| 62 | 0.50 |
| 90 | 0.37 |
| 120 | 0.26 |
| 180 | 0.14 |
| 207 | 0.10 |
| 365 | 0.02 |

## 3. 状态转换规则

### active → dormant

满足任一条件即转换：
- `freshness_score < 0.1`（约 207 天无活动，定时巡检触发）
- 关联文章数 = 0（文章删除后立即触发）

### dormant → retired

- 已 dormant 超过 180 天（`dormant_at < NOW() - 180天`），且期间无新文章关联

### dormant → active（自动恢复）

- 有新文章关联该标签时自动恢复
- `freshness_score` 重置为 1.0
- `last_active_at` 更新为当前时间
- `dormant_at` 清空

## 4. 豁免规则

以下标签不参与淘汰流程：

| 豁免条件 | 判断方式 | 原因 |
|----------|----------|------|
| 抽象标签 | 在 `topic_tag_relations` 中作为 parent 存在 | 有子标签的父标签是分类基础设施 |
| watched 标签 | `is_watched = true` | 用户主动关注，不应自动淘汰 |
| 手动标签 | `source = "manual"` | 人工创建，不做自动淘汰 |

## 5. 文章删除级联处理

文章删除时执行以下步骤：

1. 删除 `article_topic_tags` 中该文章的所有关联记录
2. 对每个受影响的标签：
   - 若标签在豁免列表中 → 跳过
   - 查询剩余关联文章数
   - 若 = 0 → 立即设为 `dormant`，记录 `dormant_at`
   - 若 > 0 → 仅更新 `freshness_score`（重新计算，不改变状态）
3. 若受影响标签是抽象标签的子标签：
   - 父抽象标签本身不淘汰（豁免）
   - 前端可选择标记"低活跃"状态

## 6. 定时巡检任务

每日执行一次，接入现有 scheduler：

```
步骤 1: 扫描 status='active' 且不在豁免列表的标签
步骤 2: 重算 freshness_score
步骤 3: 执行 active → dormant 转换（freshness < 0.1）
步骤 4: 扫描 status='dormant' 且 dormant_at < NOW() - 180天
步骤 5: 执行 dormant → retired 转换
步骤 6: 记录日志（淘汰数量、标签列表）
```

## 7. 前端影响

| 场景 | 行为 |
|------|------|
| 层级图 (hierarchy) | dormant 标签灰显但可见，retired 标签不显示 |
| 标签详情页 | 显示 freshness_score、状态标签、last_active_at |
| 筛选 | 支持 `?status=active\|dormant\|retired` |
| 淘汰摘要（可选） | 展示近期被淘汰的标签列表 |

## 8. API 变更

| 端点 | 变更说明 |
|------|----------|
| `GET /api/topic-tags/hierarchy` | 默认只返回 active + dormant；支持 `?status=` 筛选 |
| `GET /api/topic-tags/:id` | 返回 freshness_score、last_active_at、status |
| 新增 `GET /api/topic-tags/retired` | 查看已淘汰标签列表（分页） |
| 文章删除 API | 内部级联处理标签状态，无需前端额外调用 |

## 9. 向后兼容

- 现有 `status = 'active' | 'merged'` 逻辑不受影响
- 新增的 dormant/retired 状态对现有查询天然兼容（现有查询都过滤 `status = 'active'`）
- 现有标签迁移：设 `freshness_score = 1.0`，`last_active_at` 取 `updated_at` 或 `created_at`

## 10. 风险与缓解

| 风险 | 缓解措施 |
|------|----------|
| 误杀暂时冷门但重要的标签 | 豁免 watched/manual/抽象标签；dormant 可自动恢复 |
| 定时任务性能（大量标签） | 分批处理，每次处理 500 个，避免长事务 |
| 文章删除批量操作的性能 | 批量删除后一次性收集受影响标签，批量检查 |
| retired 标签的 embedding 数据 | 保留不删除，retired 仅影响查询可见性 |
