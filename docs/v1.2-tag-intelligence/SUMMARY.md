# Milestone v1.2 — 标签智能收敛与关注推送总结

**生成时间:** 2026-04-15
**用途:** 团队入职和项目回顾
**状态:** 5/8 阶段完成，Phase 3/4/5 因设计方向变更跳过

---

## 1. 项目概览

**项目名称:** RSS Reader 标签智能系统 (v1.2)
**核心价值:** 通过智能标签系统帮助用户高效消费信息——语义收敛减少噪音，关注机制聚焦兴趣，抽象标签层级帮助从碎片走向结构。
**部署模式:** 个人/单用户，无认证系统，PostgreSQL + pgvector 持久化。

本里程碑的核心目标是建立一套完整的标签智能系统：

- 新文章入库时，通过 embedding 语义相似度自动合并相近标签，解决标签碎片化
- 用户关注感兴趣的标签，首页推送关注标签关联的文章
- LLM 从中间地带标签中提取共同概念，建立抽象标签层级树
- 标签图谱和标签树交互优化，包括简介生成、时间筛选、节点手动归类

Phase 3（日报周报重构）、Phase 4（标签历史趋势）、Phase 5（相关标签推荐）因设计方向变更跳过。

---

## 2. 核心业务流程

### 2.1 标签匹配与收敛流程

新文章入库时，`findOrCreateTag` 是标签去重的核心入口，执行三级匹配：

```
新文章标签 (slug, label, category)
        │
        ▼
  ┌─ Step 1: Exact Match ──► 精确匹配 slug + category ──► 复用
  │       (miss)
  │       ▼
  ┌─ Step 2: Alias Match ──► 别名数组包含 slug ──► 复用
  │       (miss)
  │       ▼
  ┌─ Step 3: Embedding Match ──► pgvector <=> 余弦距离
  │       │
  │       ├─ ≥ 0.97 (HighSimilarity) ──► 复用已有标签
  │       │     如果匹配到抽象标签 → 创建新标签作为子标签，删除子标签 embedding
  │       │
  │       ├─ 0.78~0.97 (Middle band)
  │       │     匹配到普通标签 → LLM 提取共同抽象标签，删除两个子标签 embedding
  │       │     匹配到抽象标签 → 创建新标签作为子标签，删除子标签 embedding
  │       │
  │       └─ < 0.78 (LowSimilarity) ──► 创建全新标签 + 异步生成 embedding
  │
  ▼
  (embedding 不可用时降级为 exact match)
```

**关键设计：**
- Embedding 生成是 fire-and-forget goroutine（defer/recover），永不阻塞标签创建
- 已有标签缺少 embedding 时，复用时会触发异步 backfill
- 合并标签标记 `status='merged'`，不物理删除，保留历史追溯
- 中间地带标签的 embedding 在合并/抽象化后删除，控制向量数量增长

### 2.2 标签合并事务 (MergeTags)

5 步原子事务：

```
BEGIN TRANSACTION
  1. 迁移 article_topic_tags: source → target (dedup-before-update)
  2. 迁移 ai_summary_topics: source → target
  3. 标记 source 标签 status='merged', merged_into_id=target
  4. 删除 source 标签的 embedding 向量
  5. 重算 target 标签的 feed_count
COMMIT
```

**dedup-before-update 模式：** 迁移前检查 `(article_id, target_tag_id)` 是否已存在，存在则删除 source 链接（避免 unique constraint 违反）。

### 2.3 抽象标签提取

当中间地带匹配触发时：

```
候选标签 (0.78~0.97 相似度)
        │
        ▼
  LLM 提取共同概念 (via CapabilityTopicTagging)
        │
        ├─ 成功 → 创建抽象标签 (abstract tag)
  │        ├─ 关联子标签到抽象标签 (topic_tag_relations)
  │        ├─ 删除子标签 embedding
  │        └─ 异步为抽象标签生成 embedding
  │
  └─ 失败 → 降级创建普通标签
```

抽象标签创建后，立即搜索相似抽象标签建立更高级层级：
- **≥ 0.97**: 新抽象标签成为已有抽象标签的子标签
- **0.78~0.97**: AI 判断父子方向，建立层级
- **< 0.78**: 无操作

**数据模型：** `topic_tag_relations` 表
- `parent_id` (抽象标签)
- `child_id` (子标签)
- `relation_type` (abstract / synonym / related)
- `similarity_score`

### 2.4 关注标签与首页推送

```
用户关注标签 ──► is_watched=true, watched_at=now
        │
        ▼
首页 GetArticles?watched_tag_ids=1,2,3&sort_by=relevance
        │
        ├─ 关注标签展开: 抽象标签自动包含子标签文章
        │
        └─ 相关度排序: SUM(CASE WHEN tag_id IN (...))
              抽象标签权重 2x
```

**前端交互：**
- 心形图标点击即时翻转本地状态 + API 同步（乐观更新）
- 侧边栏"关注标签"分组：全部关注 + 按单个标签筛选
- 无关注标签时显示引导横幅，首页保持默认时间线

### 2.5 标签合并交互界面

```
用户触发扫描 ──► GET /api/topic-tags/merge-preview
        │
        ▼
  展示候选合并对 (源→目标, 相似度, 文章数)
        │
        ├─ 逐对确认 / 跳过
        ├─ 修改合并后标签名称
        └─ 一键全部合并 ──► POST /api/topic-tags/merge-with-name
        │
        ▼
  展示合并结果汇总
```

扫描使用与自动合并调度器相同的 pgvector cross-join SQL，源/目标判定用文章数启发式（多的为目标）。

### 2.6 标签质量评分 (quality_score)

```
定时调度器 ──► 计算每个标签的 quality_score
        │
        ├─ 评分维度: 文章数、embedding 质量、时间衰减等
        │
        ▼
  后端 API 按 quality_score 排序
  前端图谱默认过滤低质量标签
```

---

## 3. 架构与技术决策

### 技术栈

| 层 | 技术 |
|----|------|
| Frontend | Nuxt 4, Vue 3, TypeScript, Pinia, Tailwind CSS v4 |
| Backend | Go (Gin, GORM), PostgreSQL + pgvector |
| Embedding | pgvector `vector(1536)` + HNSW 索引, OpenAI 兼容 API |
| 标签提取 | TagJobQueue 异步队列 + TagMatch 三级匹配 |
| LLM 抽象标签 | airouter (CapabilityTopicTagging) |

### 关键技术决策

- **Decision:** pgvector vector 列替代 JSON 文本存储 embedding
  - **Why:** SQL 级 `<=>` 余弦距离搜索替代 Go 侧循环遍历全表，性能从 O(n) 降到索引查询
  - **Phase:** 01 (INFRA-01)

- **Decision:** 复用 airouter provider 框架的 CapabilityEmbedding
  - **Why:** EmbeddingClient 和 provider 管理已存在，避免重复造轮子
  - **Phase:** 01 (INFRA-02)

- **Decision:** 三级标签匹配 (exact → alias → embedding)
  - **Why:** 精确匹配和别名匹配作为快速路径，embedding 匹配作为兜底，平衡准确性和性能
  - **Phase:** 01 (CONV-01)

- **Decision:** 标签合并使用事务安全操作，merged 状态保留历史
  - **Why:** 防止 article_topic_tags 引用悬空，保留合并历史可追溯
  - **Phase:** 01 (CONV-02, CONV-04)

- **Decision:** Middle band (0.78-0.97) 跳过 AI 判定，降级为创建新标签
  - **Why:** 简化入库流程，后续由抽象标签机制统一处理中间地带
  - **Phase:** 01 (CONV-03)

- **Decision:** 关注标签扩展到抽象标签子标签文章
  - **Why:** 关注抽象标签时自动包含其所有子标签的文章，用户体验更自然
  - **Phase:** 02 (FEED-01)

- **Decision:** 抽象标签通过 LLM 从候选标签中提取共同概念
  - **Why:** 自动建立标签层级关系，减少标签碎片
  - **Phase:** 07 (NEW-01)

- **Decision:** 标签 quality_score 用于排序和低质量标记
  - **Why:** 帮助用户识别有价值的标签，过滤噪音
  - **Quick Task:** 260415-gls

---

## 4. 交付阶段

| Phase | Name | Status | Summary |
|-------|------|--------|---------|
| 01 | 基础设施与标签收敛 | ✅ 完成 (passed) | pgvector 迁移、三级匹配、合并事务、Embedding 配置 UI |
| 02 | 关注标签与首页推送 | ✅ 完成 (passed) | 关注 CRUD API、首页关注文章流、侧边栏分组、心形图标 |
| 03 | 日报周报重构 | ⏭️ 跳过 | 设计方向变更 |
| 04 | 标签历史趋势 | ⏭️ 跳过 | 设计方向变更 |
| 05 | 相关标签推荐 | ⏭️ 跳过 | 设计方向变更 |
| 06 | 标签合并交互界面 | ✅ 完成 (passed) | 全量扫描预览、自定义名称合并、批量操作确认 |
| 07 | Middle-band 抽象标签提取 | ✅ 完成 (passed) | LLM 抽象概念提取、层级树 API、前端递归组件 |
| 08 | 标签树增强与图谱交互优化 | ✅ 完成 (passed) | Description 生成、时间筛选、图谱发光可视化、节点手动归类、合并预览迁移 |

### Quick Tasks

| # | Description | Date | Status |
|---|-------------|------|--------|
| 260413-p2t | 后端队列处理 Tab (embedding + 重算队列) | 2026-04-13 | ✅ |
| 260413-r4v | 标签自动合并调度器 | 2026-04-13 | ✅ |
| 260414-ok6 | 后端 Go 日志 info/error 分流 | 2026-04-14 | ✅ |
| 260414-pg-alias | PostgreSQL JSON 数组别名查询修复 | 2026-04-14 | ✅ |
| 260415-0gc | 区分抽象/普通标签阈值匹配 + 多级分层 | 2026-04-15 | ✅ |
| 260415-gls | 标签 quality_score 方案 | 2026-04-15 | ✅ |

---

## 5. 需求覆盖度

### ✅ 已满足 (15/22 original + 8 emerged = 23/23 completed)

| ID | Description | Phase |
|----|-------------|-------|
| INFRA-01 | pgvector vector 列替代 JSON 文本 | 01 |
| INFRA-02 | Embedding 模型名从 provider 动态读取 | 01 |
| INFRA-03 | 收敛阈值可配置 (API) | 01 |
| CONV-01 | findOrCreateTag 三级匹配集成 | 01 |
| CONV-02 | 标签合并事务安全迁移 | 01 |
| CONV-03 | 中间地带跳过 AI 判定 | 01 |
| CONV-04 | merged 状态保留历史 | 01 |
| WATCH-01 | 标签列表页关注开关 | 02 |
| WATCH-02 | 后端关注 CRUD API | 02 |
| WATCH-03 | watched_at 时间记录 | 02 |
| FEED-01 | 首页关注标签文章流 | 02 |
| FEED-02 | 按关注标签筛选文章 | 02 |
| FEED-03 | 相关度排序 | 02 |
| NEW-01 | Middle-band 抽象标签提取 | 07 |
| NEW-02 | 标签层级树前端展示 | 07 |
| NEW-03 | TopicTag Description 字段 | 08 |
| NEW-04 | 时间筛选 API + UI | 08 |
| NEW-05 | 图谱抽象标签可视化 | 08 |
| NEW-06 | 合并预览迁移至设置页 | 08 |
| NEW-07 | 标签树节点手动归类 | 08 |
| NEW-08 | quality_score 方案 | Quick |

### ⏭️ 跳过 (7/22)

| ID | Description | Reason |
|----|-------------|--------|
| DIGEST-01~04 | 日报周报重构 | 设计方向变更 |
| TRENDS-01~03 | 标签历史趋势 | 设计方向变更 |
| REC-01~02 | 相关标签推荐 | 设计方向变更 |

---

## 6. 关键决策记录

| ID | Decision | Phase | Rationale |
|----|----------|-------|-----------|
| D-01 | pgvector vector 列 + HNSW 索引 | 01 | SQL 级余弦距离搜索替代 Go 循环 |
| D-02 | 复用 airouter CapabilityEmbedding | 01 | EmbeddingClient 已存在 |
| D-03 | 三级匹配 exact → alias → embedding | 01 | 快速路径 + 语义兜底 |
| D-04 | 合并标签 merged 状态保留 | 01 | 历史可追溯 |
| D-05 | Middle band 降级创建新标签 | 01 | 简化入库流程 |
| D-06 | 关注标签扩展抽象标签子标签 | 02 | 用户体验更自然 |
| D-07 | 抽象标签 LLM 提取共同概念 | 07 | 自动建立层级关系 |
| D-08 | 抽象标签 3D 发光可视化 | 08 | 图谱中区分抽象/普通标签 |
| D-09 | TagMergePreview 迁移至设置页 | 08 | 与合并队列放在一起更直观 |
| D-10 | custom:YYYY-MM-DD:YYYY-MM-DD 时间格式 | 08 | 保持现有 API contract |
| D-11 | quality_score 用于排序和低质量标记 | Quick | 识别有价值标签 |

---

## 7. 数据模型

### topic_tags 表

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| slug | string | URL 安全标识 |
| label | string | 显示名称 |
| aliases | jsonb | 别名数组 |
| category | string | 分类 |
| status | string | active / merged |
| merged_into_id | *uint | 合并目标标签 ID |
| is_watched | bool | 是否被关注 |
| watched_at | *time | 关注时间 |
| description | string | LLM 生成的标签简介 |
| quality_score | float | 标签质量评分 |

### topic_tag_embeddings 表

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| topic_tag_id | uint | 标签 ID |
| vector | jsonb | 遗留 JSON 向量（双写兼容） |
| embedding_vec | vector(1536) | pgvector 向量列 |

### topic_tag_relations 表

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| parent_id | uint | 父标签（抽象标签） |
| child_id | uint | 子标签 |
| relation_type | string | abstract / synonym / related |
| similarity_score | float | 相似度分数 |
| created_at | time | 创建时间 |

### embedding_config 表

| 字段 | 类型 | 说明 |
|------|------|------|
| key | string | 配置键 (high_similarity_threshold, low_similarity_threshold, embedding_model, embedding_dimension) |
| value | string | 配置值 |
| description | string | 配置说明 |

## 8. API 参考

### Embedding 配置

| Method | Path | 说明 |
|--------|------|------|
| GET | /api/embedding/config | 获取所有配置项 |
| PUT | /api/embedding/config/:key | 更新单个配置项 |

### 关注标签

| Method | Path | 说明 |
|--------|------|------|
| GET | /api/topic-tags/watched | 列出关注标签（含抽象标签子标签展开） |
| POST | /api/topic-tags/:id/watch | 关注标签 |
| POST | /api/topic-tags/:id/unwatch | 取消关注 |

### 标签合并预览

| Method | Path | 说明 |
|--------|------|------|
| GET | /api/topic-tags/merge-preview | 扫描高相似度标签对（不执行合并） |
| POST | /api/topic-tags/merge-with-name | 自定义名称合并标签 |

### 抽象标签层级

| Method | Path | 说明 |
|--------|------|------|
| GET | /api/topic-tags/hierarchy | 获取标签层级树 |
| PUT | /api/topic-tags/:id/abstract-name | 重命名抽象标签 |
| POST | /api/topic-tags/:id/detach | 解除父子关系 |
| POST | /api/topic-tags/:id/reassign | 手动归类到其他抽象标签 |

### 文章查询扩展

| Method | Path | 说明 |
|--------|------|------|
| GET | /api/articles?watched_tag_ids=1,2,3&sort_by=relevance | 按关注标签筛选 + 相关度排序 |

## 9. 技术债务与延期项

### 已知 Gap

1. **quality_score 与收敛流程联动** — quality_score 已实现计算和排序，但与标签自动收敛的集成尚未完善
2. **日志门面覆盖不完整** — 仅覆盖 auto_refresh/auto_tag_merge/content_completion，其余调度器仍用标准库 log
3. **Phase 3/4/5 功能** — 日报周报重构、标签历史趋势、相关标签推荐因设计方向变更跳过，如需要可在新里程碑重新定义

### 其他已知问题

- 标签匹配中抽象标签多级分层的边界条件（0.78-0.97 抽象标签 vs 抽象标签）需更多真实数据验证
- `topic_tag_embeddings` 表仍保留遗留 JSON 字段双写，后续可清理

---

## 10. 快速开始

### 运行项目

```bash
# Backend (需要 PostgreSQL + pgvector)
docker run -d --name rss-postgres -p 5432:5432 \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=rss_reader \
  pgvector/pgvector:pg18-trixie

cd backend-go
go run cmd/server/main.go    # http://localhost:5000

# Frontend
cd front
pnpm install
pnpm dev                      # http://localhost:3000
```

### 关键目录

| 目录 | 说明 |
|------|------|
| `backend-go/internal/domain/topicanalysis/` | Embedding 匹配、标签收敛、抽象标签、关注标签 |
| `backend-go/internal/domain/topicextraction/` | 标签提取队列、quality_score |
| `backend-go/internal/domain/topicgraph/` | 主题图谱构建 |
| `backend-go/internal/domain/models/topic_graph.go` | TopicTag 模型（含 embedding、status、description） |
| `backend-go/internal/domain/models/topic_tag_relation.go` | 抽象标签层级关系模型 |
| `front/app/features/topic-graph/` | 标签图谱和标签树前端组件 |
| `front/app/api/watchedTags.ts` | 关注标签前端 API |
| `front/app/api/embeddingConfig.ts` | Embedding 配置前端 API |

### 测试

```bash
# Backend tests
cd backend-go
go test ./internal/domain/topicanalysis -v
go test ./internal/domain/topicextraction -v
go test ./...                        # 全量测试

# Frontend tests
cd front
pnpm test:unit                       # Vitest 单测
pnpm exec nuxi typecheck             # 类型检查
```

### 首先阅读

1. `backend-go/internal/domain/topicanalysis/embedding.go` — 三级匹配和 TagMatch 核心逻辑
2. `backend-go/internal/domain/topicanalysis/abstract_tag_service.go` — 抽象标签提取和层级管理
3. `backend-go/internal/domain/topicextraction/tagger.go` — 标签提取入口和 findOrCreateTag
4. `front/app/features/topic-graph/components/TopicGraphPage.vue` — 标签图谱主页面

---

## 统计信息

- **时间线:** 2026-04-13 → 2026-04-15 (3 天)
- **阶段:** 5 完成 / 8 总 (3 跳过)
- **Plans:** 20 完成
- **Quick Tasks:** 6
- **提交:** 122
- **文件变更:** 269 (+31,081 / -2,764)
- **贡献者:** zanebonoalter
