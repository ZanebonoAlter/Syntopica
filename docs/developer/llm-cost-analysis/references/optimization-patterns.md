# LLM 调用优化模式参考

## 模式 1: Batch 化处理

**适用场景**: 多个独立的 LLM 判断调用可以合并

**当前案例**:
| 旧 Operation | 新 Operation | 收益 |
|--------------|--------------|------|
| `adopt_narrower_abstract` (N次) | `adopt_narrower_abstract_batch` (1次) | N倍 → 1倍 |
| `tag_judgment` (逐条) | `batch_tag_judgment` (批量) | 多次 → 1次 |
| `judge_abstract_relationship` (逐个) | `judge_abstract_relationship_batch` (批量) | 多次 → 1次 |
| `resolve_multi_parent` (逐个) | `batch_resolve_multi_parent` (批量) | 多次 → 1次 |

**实施要点**:
1. 在 `topicanalysis/` 中找到对应批量函数
2. 使用队列机制累积 pending 任务
3. 达到阈值或定时触发时批量处理
4. 单条时 fallback 到逐条调用

**风险**: batch 成功率通常低于单条，需监控

---

## 模式 2: 提取阶段直接生成

**适用场景**: 后续需要单独调用 LLM 生成的附加信息，可以在初始提取时一并输出

**当前案例**:
```
旧: extractCandidates(LLM) → 创建标签 → go generateTagDescription(再次LLM)
新: extractCandidates(LLM with description) → 创建标签时直接写入 description
```

**收益**: 减少 ~1600 次 tag_description 调用/周

**实施要点**:
1. 修改 ExtractedTag 结构体添加 Description 字段
2. 在 extraction prompt 中要求输出 description（可选字段）
3. 创建新标签时直接使用 tag.Description
4. 复用已有标签时若 description 已存在则不覆盖
5. person 标签保持单独路径（需要结构化属性）

**代码位置**:
- 结构体: `topictypes/types.go`
- Prompt: `topicextraction/extractor_enhanced.go` buildExtractionSystemPrompt
- 创建: `topicextraction/tagger.go` findOrCreateTag

---

## 模式 3: 阈值调整

**适用场景**: embedding 相似度匹配进入 LLM 判断的候选过多

**当前阈值**:
- ≥0.97: 直接复用（不调用 LLM）
- 0.78~0.97: 进入 LLM 判断
- <0.78: 新建标签（不调用 LLM）

**优化方案**:
- 对层级匹配（judge_abstract_relationship）提高到 0.82~0.85
- 对新标签判断（tag_judgment）保持 0.78

**风险**: 可能漏掉一些有效的匹配

---

## 模式 4: 结果缓存

**适用场景**: 相同输入会产生相同输出的判断

**当前案例**:
- `judge_abstract_relationship`: 相同标签对的父子关系判断
- `judge_cross_layer_duplicate`: 相同标签对的跨层重复判断
- `find_similar_existing_abstract`: 相同标签的相似抽象查找

**实施要点**:
1. 使用内存缓存（sync.Map 或 LRU cache）
2. Key 为组合键（如 parentID+childID 或 label+category）
3. TTL 设置为 24-48 小时（标签关系变化不频繁）
4. 标签合并/删除时清除相关缓存

**代码位置**: 在对应 service 中添加缓存层

---

## 模式 5: 异步队列 + 批量处理

**适用场景**: 非实时必须的 LLM 调用

**当前案例**:
- `abstract_tag_update_queues`: 抽象标签刷新队列
- `adopt_narrower_queues`: 收养更窄标签队列
- `merge_reembedding_queues`: 合并后重新 embedding 队列

**实施要点**:
1. 任务入队时去重（检查是否已有 pending/processing）
2. Worker 轮询处理（3-5 秒间隔）
3. 调度器 Phase 批量处理所有 pending（确保定时完成）
4. 失败时可重试，有 retry_count 保护

**代码位置**: `topicanalysis/abstract_tag_update_queue.go`, `adopt_narrower_queue.go`

---

## 模式 6: 错误分类与降级

**适用场景**: LLM 调用有失败风险

**错误分类**:
| 错误码 | 含义 | 处理策略 |
|--------|------|----------|
| 500 | 模型内部错误/Context 超限 | 重试 1-2 次，截断上下文后重试 |
| 1302 | 模型输出解析失败 | 重试 1 次，调整 prompt |
| 1301 | 模型不可用 | 延迟重试 |
| 503 | 服务不可用 | 延迟重试 |
| network_error | 网络问题 | 立即重试 |

**降级策略**:
1. LLM 调用失败 → 使用 embedding 精确匹配
2. embedding 匹配失败 → 使用 slug 精确匹配
3. 全部失败 → 标记任务为 failed，可手动重试

**代码位置**: `findOrCreateTag` 中的 fallback 逻辑

---

## 模式 7: Prompt 精简

**适用场景**: prompt 过长导致耗时增加或 context 超限

**当前问题**:
- person 标签的 metadata（country/organization/role/domains）导致 prompt 过长
- 部分 articleContext 过长（截断到 800-2000 字符）

**优化方案**:
1. 对 person 标签的结构化属性设置长度上限（每个字段 max 50 字符）
2. articleContext 二次截断（已有截断但可能不够）
3. 移除冗余的示例说明（保留最核心的 2-3 个）
4. 使用更简洁的 JSON Schema（减少 token 消耗）

**代码位置**: `tagger.go` 中的 prompt 构建函数

---

## 模式 8: 调度频率优化

**适用场景**: 定时任务的执行频率过高或过低

**当前调度器**:
- `tag_hierarchy_cleanup`: 7 阶段清理，每天或每 12 小时
- `embedding_queue_worker`: 3 秒轮询
- `adopt_narrower_worker`: 5 秒轮询

**优化方向**:
1. Phase 4-5 批量处理可以改为"仅在 pending > N 时执行"
2. Worker 轮询间隔可以根据负载动态调整
3. 大量 pending 时批量锁定处理，减少锁竞争

**代码位置**: `jobs/tag_hierarchy_cleanup.go`, 各 worker 文件
