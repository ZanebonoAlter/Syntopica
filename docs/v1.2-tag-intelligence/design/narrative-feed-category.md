# 叙事按 Feed 分类视角设计

## 目标

当前叙事（narrative）仅按日期聚合，没有 feed 分类维度。用户希望每个 feed 分类能独立生成自己的叙事脉络，作为该分类的局部观察视角，同时保留全局叙事作为主线。

## 方案选择

采用“全局叙事 + 分类独立叙事”方案：
- 全局叙事保持现有逻辑不变，作为首页主视图
- 每个 feed 分类独立生成叙事，作为第二层钻取
- 不引入全局叙事与分类叙事的映射关系（留给后续增强）

## 一、数据模型

### narrative_summaries 新增字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `scope_type` | string(20), default 'global' | 叙事作用域：`global` / `feed_category` |
| `scope_category_id` | uint, nullable | 当 `scope_type=feed_category` 时记录 feed 分类 ID；全局为 null |
| `scope_label` | string(100), nullable | 分类名快照，便于展示和排查，非强依赖 |

一条叙事要么是全局的（`scope_type=global`），要么属于某个 feed 分类（`scope_type=feed_category`）。

### 唯一索引/查询索引

- 复合索引：`(scope_type, scope_category_id, period_date)` 用于按作用域+日期查询
- 不在第一版引入全局叙事与分类叙事的映射表

### 不做

- 不加 `global_narrative_id` / `parent_global_id`
- 不加分类叙事与全局叙事的关联表
- 不改现有字段结构

## 二、生成链路

叙事生成仍然在 `narrative_summary` 调度任务内统一执行，不拆成多个调度器。

### 生成顺序

1. 生成全局叙事（保持现有逻辑，`scope_type=global`）
2. 遍历当天有数据的 feed 分类，逐个生成分类叙事（`scope_type=feed_category, scope_category_id=x`）
3. 记录本次运行摘要（分类数量、每分类叙事数）

### 分类叙事生成

- 输入：该 feed 分类作用域内的 tag inputs
- 调用：复用现有 `GenerateNarratives`（不改 generator 本体）
- 输出：`NarrativeOutput` → 保存时写入 `scope_type` + `scope_category_id` + `scope_label`

### 分类叙事生成门槛

避免小分类硬凑噪音，需满足以下条件才生成：
- 分类当天文章数 ≥ 阈值（默认 5）
- 分类当天有效 tag 数 ≥ 阈值（默认 3）
- 单分类最多生成叙事数：固定上限（默认 5）
- 分类内只有单一标签簇时不生成

### 手动触发

- `TriggerNow` / `TriggerNowWithDate` 行为不变，执行同一流程
- 也可以扩展支持按单个分类触发（可选）

## 三、查询接口

### 现有接口兼容

| 接口 | 变化 |
|------|------|
| `GET /api/narratives?date=...` | 新增可选参数 `scope_type`, `category_id` |
| `GET /api/narratives/timeline?date=...&days=...` | 新增可选参数 `scope_type`, `category_id` |
| `GET /api/narratives/:id/history` | 不变 |
| `DELETE /api/narratives?date=...` | 新增可选参数 `scope_type`, `category_id`，可只删分类叙事 |

默认行为（不传 scope 参数）：`scope_type=global`，和现有行为一致。

### 新接口

`GET /api/narratives/scopes?date=YYYY-MM-DD`

返回当天有哪些分类有叙事，以及每个分类的叙事数量，给前端做分类列表入口。

响应示例：
```json
{
  "success": true,
  "data": {
    "date": "2026-04-18",
    "global_count": 4,
    "categories": [
      {
        "category_id": 1,
        "category_name": "投资资讯",
        "category_icon": "mdi:chart-line",
        "category_color": "#60a5fa",
        "narrative_count": 3,
        "last_generated_at": "2026-04-18T03:00:00Z"
      },
      {
        "category_id": 2,
        "category_name": "科技媒体",
        "category_icon": "mdi:chip",
        "category_color": "#34d399",
        "narrative_count": 2,
        "last_generated_at": "2026-04-18T03:01:00Z"
      }
    ]
  }
}
```

## 四、前端交互

### 作用域切换器

在叙事面板标题区和叙事列表之间加一行胶囊式筛选条：
- 样式：`[ 全部 ] [ 按分类 ]`
- 默认激活“全部”

### 作用域=全部（默认）

体验和现有叙事面板一致，零改动：
- 时间线 canvas
- 叙事卡片列表
- 详情浮层
- “重新整理”按钮

### 作用域=按分类

**一级视图：分类列表**

- 调用 `/narratives/scopes?date=...`
- 渲染为纵向卡片列表，每个卡片包含：
  - 左侧：feed 分类图标（复用分类已有 icon/color）
  - 中间：分类名称
  - 右侧：叙事数量 badge
- 点击进入二级视图

**二级视图：某分类叙事时间线**

- 调用 `/narratives/timeline?scope_type=feed_category&category_id=x`
- 复用现有 `NarrativeCanvas.client.vue` 渲染
- 复用现有详情浮层
- 顶部加返回按钮：`< 全部叙事`
- 分类名作为子标题，如：“投资资讯 · 2026-04-18 叙事脉络”

### “重新整理”按钮

- 作用域=全部：重新生成全局叙事
- 作用域=按分类→某分类二级：只重新生成该分类的叙事
- 作用域=按分类→分类列表：不展示按钮

### 状态保留

- 从分类二级返回分类列表时，列表状态保留
- 日期变更时，整个分类面板重载

### 加载与空状态

| 场景 | 展示 |
|------|------|
| 分类列表加载中 | 3-5 张骨架屏卡片 |
| 分类列表为空 | “当天无分类叙事，请先完成一次叙事整理” |
| 某分类叙事为空 | “该分类当天未生成叙事（文章数或标签数不足）” |

## 五、第一版不做

- 全局叙事和分类叙事自动对齐 / 映射
- 跨分类比较视图
- 分类叙事单独的状态体系
- 按单个 feed 维度生成叙事（只做到 feed 分类层级）
- 分类叙事中预览标签标题等额外信息

## 六、涉及文件

### 后端
- `backend-go/internal/domain/models/narrative.go` — 新增字段
- `backend-go/internal/domain/narrative/service.go` — 分类生成逻辑 + 作用域查询
- `backend-go/internal/domain/narrative/handler.go` — 新参数 + scopes 接口
- `backend-go/internal/jobs/narrative_summary.go` — 调度链路补充分类生成

### 前端
- `front/app/api/topicGraph.ts` — 新增 `getNarrativeScopes` + 参数扩展
- `front/app/features/topic-graph/components/NarrativePanel.vue` — 作用域切换器 + 分类列表视图
