# 跨分类叙事整合设计

## 问题

当前全局叙事（`scope_type = global`）直接从标签采集生成，输入是所有根抽象节点 + 未分类活跃标签（上限 100 个）。随着标签体系膨胀，存在：

1. LLM prompt 过长，生成质量下降
2. 叙事过于发散，关联性弱
3. 与分类叙事职责重叠

## 方案

全局叙事改为**分类叙事摘要的二次整合**，只负责发现跨分类的关联线索。

### 生成流程

```
分类叙事（各 category 独立生成，已有）
           ↓
  CollectCategoryNarrativeSummaries
           ↓
  GenerateCrossCategoryNarratives
           ↓
  全局叙事（scope_type = global）
```

严格顺序：先完成所有分类叙事，再以分类叙事摘要为输入生成全局叙事。

### 改动范围

#### 后端 `collector.go`

新增 `CollectCategoryNarrativeSummaries(date)` 函数：
- 查询当日 `scope_type = 'feed_category'` 且 `status != 'ending'` 的所有叙事
- 按 category 分组，每个 category 最多取 5 条叙事（按 generation DESC, id DESC 排序截断）
- 总输入上限：所有分类合计最多 30 条叙事摘要，超过时按 category 文章数降序优先保留
- 构建结构化输入：
  ```
  CategoryInput {
    CategoryName    string
    CategoryIcon    string
    Narratives      []CategoryNarrativeBrief
  }
  CategoryNarrativeBrief {
    ID             uint64
    Title          string
    Summary        string
    RelatedTags    []TagBrief
  }
  ```

#### 后端 `generator.go`

新增 `GenerateCrossCategoryNarratives(ctx, categoryInputs, prevGlobalNarratives)` 函数：
- 独立的 system prompt，明确要求只输出横跨≥2个分类的关联叙事
- JSON schema 新增 `CrossCategoryNarrativeOutput` 结构体，扩展自 `NarrativeOutput`：
  ```go
  type CrossCategoryNarrativeOutput struct {
      NarrativeOutput
      SourceCategoryIDs []uint `json:"source_category_ids"`
  }
  ```
- `source_category_ids` 不持久化到数据库，仅用于 prompt 约束和生成后校验（验证确实跨≥2分类）
- LLM 输出的 `related_tag_ids` 来自分类叙事摘要中的 `RelatedTags`，需从分类叙事的 tag 列表中提取有效 tag ID，传入 validTagIDs 做校验
- `related_article_ids` 的 `resolveArticleIDs` 保持现有逻辑，基于 `related_tag_ids` 查询当日文章
- `parent_ids` 指向分类叙事 ID（溯源关联）

#### 后端 `service.go`

修改 `GenerateAndSave(date)`：
1. 先调用 `GenerateAndSaveForAllCategories(date)` 生成分类叙事
2. 对每个分类叙事结果触发 `go feedbackNarrativesToTags(categoryOutputs)`（从分类级别触发 tag feedback，替代原来的全局级触发）
3. 调用 `CollectCategoryNarrativeSummaries(date)` 收集摘要
4. 如果分类叙事为 0，跳过全局叙事，但仍触发 `go GenerateWatchedTagNarratives(date)` 后返回
5. 收集前一日全局叙事 `CollectPreviousNarratives(date, models.NarrativeScopeTypeGlobal, nil)` 用于代际追踪
6. 调用 `GenerateCrossCategoryNarratives` 生成全局叙事
7. 保存时 `scope_type = global`，`parent_ids` 指向分类叙事 ID

#### 代际追踪方案（Option B）

全局叙事的 `parent_ids` 指向分类叙事（溯源），代际追踪不依赖 `parent_ids`，改为：

- **generation 计算**：保存前查询 `scope_type = 'global'` 且 `period_date = 前一日` 的叙事，取 `MAX(generation)`，新记录 generation = MAX + 1。若无前一日全局叙事则 generation = 0。在 `saveNarratives` 中为 global scope 增加此逻辑。
- **markEndedNarratives**：改为查询前一日 `scope_type = 'global'` 的叙事，与今日全局叙事的 parent_ids（分类叙事 ID）对比不再适用。改为：前一日全局叙事中，如果其 `related_tag_ids` 与今日任意全局叙事的 `related_tag_ids` 交集为空，则标记为 ending。若无前一日全局叙事则跳过。
- 分类叙事的代际逻辑不变：分类叙事 `parent_ids` 仍然只指向同分类的前一日叙事，`CollectPreviousNarratives(date, models.NarrativeScopeTypeFeedCategory, &categoryID)` 和 `resolveGeneration` 逻辑不变。

#### `RegenerateAndSave(date)` 修改

- 删除+重建顺序不变（`DeleteByDate(date, "", nil)` 清除全部）
- 只调用 `GenerateAndSave(date)`，不再额外调用 `GenerateAndSaveForAllCategories`
- `GenerateAndSave` 内部已先跑分类再跑全局，无需重复

#### 调度器 `runNarrativeCycle` 修改

- scheduled 路径：只调用 `GenerateAndSave(targetDate)`，移除额外的 `GenerateAndSaveForAllCategories` 调用
- manual 路径：只调用 `RegenerateAndSave(targetDate)`，同上
- `GenerateWatchedTagNarratives(date)` 从 `GenerateAndSave` 中移出，改到调度器层面触发：
  ```go
  // runNarrativeCycle 末尾
  savedCount, err = ...
  go narrative.GenerateWatchedTagNarratives(targetDate)
  ```

#### 错误处理

- 分类叙事全部失败但分类数量 > 0：视为分类叙事为 0，跳过全局叙事生成，日志记录警告
- 单个分类生成失败：已有 `continue` 跳过，不影响其他分类
- 分类叙事数量过多触发截断：在 `CollectCategoryNarrativeSummaries` 中记录截断日志

#### 数据模型

`NarrativeSummary` 无字段变更。`source_category_ids` 不持久化。

#### 前端

`NarrativePanel.vue` 无需改动。`全部/按分类` 切换已实现，全局叙事的 parent 关系可被历史面板正常展示。

### Prompt 设计

全局叙事 system prompt 核心要求：

```
你是一名专业的新闻叙事分析师。你收到了各分类频道独立生成的叙事摘要。
你的任务是发现横跨多个分类频道的关联叙事线索。

规则：
1. 只输出横跨至少 2 个分类的叙事
2. 每条叙事标注来源分类（source_category_ids，填入分类 ID）
3. 标题必须是带判断的短句，不超过 30 字
4. 摘要 200-400 字，说明跨分类的因果/影响/主题关联
5. 不要重复分类内部已发现的叙事
6. 数量不固定，没有跨分类关联就返回空数组
7. related_tag_ids 从输入的分类叙事摘要中的标签列表选取
```

### 向后兼容

- `CollectTagInputs` 和 `GenerateNarratives` 保留，不删除，用于分类叙事
- 现有 API 端点不变
- 前端查询逻辑不变（`scope_type` 过滤依然生效）

### 调度变更

`NarrativeSummaryScheduler`：
- scheduled/manual 路径均移除额外的 `GenerateAndSaveForAllCategories` 调用
- `GenerateWatchedTagNarratives` 改到调度器末尾触发
- 其余调度逻辑（cron、status、stats）不变

### 文件改动清单

| 文件 | 改动类型 | 说明 |
|------|---------|------|
| `collector.go` | 新增函数 | `CollectCategoryNarrativeSummaries` |
| `generator.go` | 新增函数 | `GenerateCrossCategoryNarratives`，新增 `CrossCategoryNarrativeOutput` |
| `service.go` | 修改 | `GenerateAndSave` 改为分类+全局两阶段；`RegenerateAndSave` 移除重复分类调用；新增全局 generation 计算和 markEnded 逻辑 |
| `narrative_summary.go` (jobs) | 修改 | `runNarrativeCycle` 移除重复分类调用，移入 `GenerateWatchedTagNarratives` |
| `models/` | 无变更 | `NarrativeSummary` 结构体不变 |
| 前端 | 无变更 | — |
