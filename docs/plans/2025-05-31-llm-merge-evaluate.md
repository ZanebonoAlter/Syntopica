# LLM 合并建议评估 - 实施计划

## 前置 bug 修复（已完成）
- [x] HardMergeTags 清理 embedding_queues / merge_reembedding_queues 外键
- [x] MergeTagsWithCustomNameHandler 嵌套事务自死锁 → 直接用 HardMergeTags(tx)
- [x] SSE EventSource URL 硬编码 → 改用 getApiOrigin()

## 任务清单

### Task 10: DB + 后端 LLM 评估逻辑
**文件**: 
- `backend-go/internal/domain/models/topic_graph.go` — TagMergeSuggestion 加 `LLMVerdict` 字段
- `backend-go/internal/domain/tagging/tag_merge_suggest.go` — 新增 EvaluateMergeSuggestions 函数
- `backend-go/internal/domain/tagging/tag_merge_suggest_test.go` — 补充测试

**改动**:
1. `TagMergeSuggestion` 加 `LLMVerdict string gorm:"type:text" json:"llm_verdict"`
2. `EvaluateMergeSuggestions(ctx)`:
   - 查询 `status=pending` 的 suggestions
   - 按 `existing_tag_id` 分组
   - 每组构建 prompt，调用 `airouter.Chat`
   - 解析 JSON 响应，更新每条 suggestion 的 `llm_verdict`
   - 通过 progress channel 推送进度（复用 SSE 模式）
3. 并发保护：`atomic.Bool` 防止重复触发

**LLM Prompt 模板**:
```
你是标签合并专家。以下标签都与「{existing_label}」相似：

{existing_label} (id:{existing_id}) 是目标标签，有 {existing_articles} 篇文章。

相似标签候选：
- {new_label} (id:{new_id})，相似度 {similarity}，有 {new_articles} 篇文章
...

请判断每对是否应该合并到「{existing_label}」。输出 JSON：
{
  "verdicts": [
    {
      "new_tag_id": 123,
      "should_merge": true,
      "suggested_name": "共产党员",
      "reason": "简称和全称的关系"
    },
    ...
  ]
}
```

### Task 11: 后端评估 API 端点
**文件**: `backend-go/internal/domain/tagging/tag_merge_preview_handler.go`

**改动**:
1. `POST /merge-preview/evaluate` — 触发 LLM 评估，返回 202 / 409
2. `GET /merge-preview/evaluate/stream` — SSE 推送评估进度
3. `RegisterTagMergePreviewRoutes` 注册两个新路由

**EvaluateProgress 结构**:
```go
type EvaluateProgress struct {
    Status        string `json:"status"`         // evaluating, done, error
    TotalGroups   int    `json:"total_groups"`
    Completed     int    `json:"completed"`
    CurrentTarget string `json:"current_target"` // 正在评估的 target 标签名
}
```

### Task 12: 后端分组查询 API
**文件**: `backend-go/internal/domain/tagging/tag_merge_preview_handler.go`

**改动**: 修改 `ScanMergePreviewHandler` 或新增端点，返回**按 target 分组**的数据格式：

```json
{
  "groups": [
    {
      "target_tag_id": 123,
      "target_label": "共产党员",
      "target_slug": "gong-chan-dang-yuan",
      "target_articles": 71,
      "category": "keyword",
      "suggestions": [
        {
          "id": 1,
          "new_tag_id": 456,
          "new_label": "共产党",
          "similarity": 0.95,
          "new_articles": 50,
          "llm_verdict": {"should_merge": true, "suggested_name": "共产党员", "reason": "..."},
          "source": "full_scan"
        }
      ]
    }
  ],
  "total_groups": 15
}
```

只返回含 `should_merge=true` verdict 的建议（如果已评估）。未评估时返回全部 pending。

### Task 13: 前端 API 层
**文件**: 
- `front/app/types/tagMerge.ts` — 新增类型
- `front/app/api/tagMergePreview.ts` — 新增方法

**新增类型**:
```typescript
interface MergeSuggestionGroup {
  targetTagId: number
  targetLabel: string
  targetSlug: string
  targetArticles: number
  category: string
  suggestions: MergeSuggestion[]
}

interface MergeSuggestion {
  id: number
  newTagId: number
  newLabel: string
  similarity: number
  newArticles: number
  llmVerdict: LLMVerdict | null
  source: string
}

interface LLMVerdict {
  should_merge: boolean
  suggested_name: string
  reason: string
}

interface EvaluateProgress {
  status: string
  totalGroups: number
  completed: number
  currentTarget: string
}
```

**新增 API 方法**:
- `triggerEvaluate()` → POST /merge-preview/evaluate
- `createEvaluateEventSource(callback)` → EventSource for /merge-preview/evaluate/stream
- `getMergeGroups(params)` → GET /merge-preview 返回分组数据

### Task 14: 前端 TagMergePreview.vue 重写
**文件**: `front/app/features/topic-graph/components/TagMergePreview.vue`

**UI 结构**:
```
┌─ 标签合并预览 ──────────── [AI评估] [全量扫描] [×] ─┐
│                                                      │
│  AI 评估进度条 (SSE)                                  │
│                                                      │
│  ┌─ 🎯 共产党员 (71篇) ──────────────────────────┐   │
│  │  ✅ 共产党   0.95  50篇  → 共产党员            │   │
│  │     简称和全称的关系                            │   │
│  │  ✅ 党员     0.82  30篇  → 共产党员            │   │
│  │     党员是共产党员的简称                        │   │
│  │  ─────────────────────────────────────         │   │
│  │  [🔍 添加标签]           [确认合并 (2)]         │   │
│  └────────────────────────────────────────────────┘   │
│                                                      │
│  ┌─ 🎯 人工智能 (120篇) ────────────────────────┐   │
│  │  ...                                          │   │
│  └────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────┘
```

**交互**:
1. 打开面板 → 调 `getMergeGroups()` 加载分组数据
2. 点"AI评估" → `triggerEvaluate()` + SSE 进度
3. 评估完成后自动刷新 → 只显示 should_merge=true 的
4. "添加标签" → 弹出搜索框，搜索现有标签，选中后加入该组
5. "确认合并 (N)" → 批量调 merge-with-name，合并组内所有建议
6. 每条建议可单独移除或修改建议名称

### Task 15: 添加标签到组
**文件**: 
- 后端: `POST /merge-preview/add-to-group` — body: `{target_tag_id, new_tag_id}`
- 前端: 搜索框 + 下拉选择

**逻辑**: 
1. 搜索用已有 `GET /topic-tags/search?q=xxx`
2. 选中后在 suggestion 表插入一条 pending 记录
3. 前端刷新组数据
