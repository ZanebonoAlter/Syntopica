---
name: llm-cost-analysis
description: >
  分析 my-robot 项目中 LLM 调用频率、耗时和成功率，结合代码流程文档定位优化方向。
  适用于：诊断 LLM 调用瓶颈、分析 tag_description / topic_tagging / embedding 等各类 capability 的调用分布、
  结合 docs/guides/tagging-flow.md 等流程文档给出优化建议、监控 ai_call_logs 表发现异常调用模式。
  当用户询问"LLM 调用太频繁"、"优化 AI 调用"、"分析 ai_call_logs"、"哪个 operation 调用最多"、
  "为什么 LLM 这么慢"、"tag_description 能不能优化"时触发此 skill。
---

# LLM 调用成本分析 Skill

## 快速诊断流程

### Step 1: 获取全局概览

```sql
-- 按 capability 统计（最近 7 天）
SELECT 
  capability,
  COUNT(*) as cnt,
  ROUND(AVG(latency_ms)::numeric, 0) as avg_ms,
  ROUND(100.0 * SUM(CASE WHEN success THEN 1 ELSE 0 END) / COUNT(*)::numeric, 1) as success_pct
FROM ai_call_logs 
WHERE created_at > NOW() - INTERVAL '7 days'
GROUP BY capability 
ORDER BY cnt DESC;
```

### Step 2: 定位高频 Operation

```sql
-- topic_tagging 的 operation 细分（最近 7 天）
SELECT 
  (request_meta::jsonb)->>'operation' as operation,
  COUNT(*) as cnt,
  ROUND(AVG(latency_ms)::numeric, 0) as avg_ms,
  ROUND(100.0 * SUM(CASE WHEN success THEN 1 ELSE 0 END) / COUNT(*)::numeric, 1) as success_pct
FROM ai_call_logs 
WHERE capability = 'topic_tagging' 
  AND created_at > NOW() - INTERVAL '7 days'
  AND request_meta IS NOT NULL 
  AND request_meta != ''
GROUP BY (request_meta::jsonb)->>'operation'
ORDER BY cnt DESC;
```

### Step 3: 分析错误模式

```sql
-- 错误分布
SELECT 
  (request_meta::jsonb)->>'operation' as operation,
  error_code,
  COUNT(*) as cnt
FROM ai_call_logs 
WHERE success = false 
  AND created_at > NOW() - INTERVAL '7 days'
GROUP BY operation, error_code
ORDER BY cnt DESC;
```

### Step 4: 结合代码流程文档

查询到高频/高耗时 operation 后，结合 `docs/guides/tagging-flow.md` 定位对应环节：

| Operation | 对应文档章节 | 优化方向 |
|-----------|-------------|----------|
| `tag_description` | 第 3 节 LLM 批量判断后 | 提取阶段直接生成 description |
| `tag_description_person` | 第 3 节 person 标签 | 截断人物属性避免 context 超限 |
| `judge_abstract_relationship` | 第 6 节 抽象层级匹配 | 提高 embedding 阈值、缓存结果 |
| `batch_tag_judgment` | 第 3 节 LLM 批量判断 | 保持 batch 模式 |
| `resolve_multi_parent` | 第 7 节 多父冲突 | 使用 batch_resolve_multi_parent |
| `abstract_tag_label_description_refresh` | 第 9 节 抽象标签刷新 | 减少刷新频率、累积批量处理 |
| `adopt_narrower_abstract` | 第 10 节 收养更窄标签 | 使用 adopt_narrower_abstract_batch |
| `tree_review` | 第 12 节 Phase 6 整树审查 | 非高频，暂不优化 |

## 关键指标解读

### 耗时分布
- **embedding**: ~500ms（正常，向量生成快）
- **topic_tagging**: ~36s（慢，需要优化）
- **article_completion**: ~65s（慢，成功率高但失败也高）
- **summary**: ~51s（可接受）

### 成功率红线
- **>99%**: 正常
- **95-99%**: 关注，检查 error_code
- **<95%**: 严重，需要立即修复
- **<50%**: 阻塞，如 tag_description_person（43.5%）

### 高频调用阈值
- **>1000 次/周**: 优先优化（如 judge_abstract_relationship 2784 次）
- **100-1000 次/周**: 次优先
- **<100 次/周**: 不紧急

## 优化策略速查

### 策略 1: 减少调用次数
- **batch 化**: 单条 → batch（如 adopt_narrower_abstract_batch）
- **阈值调整**: 提高 embedding 相似度阈值，减少进入 LLM 的候选
- **结果缓存**: 对 judge_abstract_relationship 等确定性判断结果缓存 24-48h

### 策略 2: 降低单次耗时
- **prompt 精简**: 移除冗余上下文（person 标签截断属性）
- **模型路由**: 简单判断用轻量模型（如本项目只有一个本地模型则不可行）
- **异步化**: 非关键 description 改为异步批量生成

### 策略 3: 提高成功率
- **错误重试**: 对 500/1302 错误加入指数退避重试
- **context 截断**: 超长 articleContext 截断到安全长度
- **降级策略**: LLM 失败时回退到 embedding 精确匹配或 slug 匹配

## 完整查询参考

详见 [references/queries.md](references/queries.md)

## 优化模式参考

详见 [references/optimization-patterns.md](references/optimization-patterns.md)
