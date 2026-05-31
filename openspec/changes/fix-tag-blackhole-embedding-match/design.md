## Context

标签系统使用 `findOrCreateTag` 为文章提取的标签在数据库中查找或创建对应记录。匹配流程为三级：精确 slug/alias 匹配 → embedding 相似度匹配 → 创建新标签。

当前 `TagMatch` 方法在 embedding 相似度达到 `HighSimilarity` 阈值（keyword=0.90）时，返回 `MatchType: "exact"`。`findOrCreateTag` 对所有 `exact` 结果统一处理：覆盖已有标签的 label、slug、aliases 等字段并 `Save`。这导致：

1. 标签 A 被 embedding 匹配到标签 B → B 的 label/slug 被改为 A
2. B 的 embedding 失效（text_hash 变化）→ embedding queue 重新生成
3. 新 embedding 又吸引其他标签匹配到 B → 循环

此外，embedding 高相似度合并还导致**文章被错误关联**：即使不覆盖 identity，similarity 匹配返回已有 tag 引用后，不相关文章的 `article_topic_tags` 仍然指向错误的 tag。

并发度 3、batchSize 20 的 tag worker 加剧了竞态条件：多个 goroutine 同时对同一 tag 执行 Save。

## Goals / Non-Goals

**Goals:**
- 阻止 embedding 相似度匹配自动合并标签（降级为 candidates，创建独立 tag）
- 清理膨胀的 embedding 记录，防止同一 tag 同一 type 记录无限增长
- 增量记录合并建议，替代全量扫描 O(n²) cross-join
- 提供标签合并 UI（读 suggestion 表，支持异步全量扫描 + SSE 进度）
- 修复被污染的 article_topic_tags 数据

**Non-Goals:**
- 不重新设计标签匹配架构（保持三级匹配流程）
- 不修改 tag worker 并发模型
- 不做全量 retag（只修复已知的污染数据）
- 不修改提取 prompt 或版块配置（同义词问题的源头治理是独立工作）

## Decisions

### Decision 1: Embedding 高相似度匹配降级为 candidates

在 `TagMatch` 中，当 embedding 相似度达到 `HighSimilarity` 阈值时，返回 `MatchType: "candidates"` 而非 `"exact"`。`findOrCreateTag` 对 `candidates` 的已有行为是 fall through 到创建新 tag。

**效果**：embedding 只负责"找相似的"，标签合并只发生在 slug/alias 精确匹配时。彻底消除黑洞循环和错误关联。

**替代方案 A（原方案）**：在 `TagMatchResult` 中新增 `MatchReason` 字段区分来源，`findOrCreateTag` 按 reason 分支处理。问题：(1) similarity 匹配返回已有 tag 引用仍有错误关联；(2) cache 会静默错误关联（cache["义诊"] = tag94712）；(3) 改动更多但覆盖面更窄。

**替代方案 B**：维持高相似度合并但更严格限制覆盖行为。问题：没有解决文章被错误关联到不相关 tag 的根本问题。

**风险**：同义词标签（如"AI"/"人工智能"）会各自创建独立 tag，需手动合并。但 keyword 类别的标签提取很少出现同义词场景，且已有 merge 功能兜底。

### Decision 2: 删除 keyword 类别阈值覆盖

embedding 高相似度已不再走 exact 路径，`CategoryThresholdOverrides` 中 keyword 条目的 `HighSimilarity` 不再有意义（0.90 和 0.95 走的都是 candidates）。删除 keyword override，统一使用默认阈值（`HighSimilarity: 0.97, LowSimilarity: 0.78`）。

`LowSimilarity` 仍然有意义：它控制"多少算有候选"还是"完全没匹配"，决定返回 `candidates` 还是 `no_match`。

### Decision 3: SaveEmbedding 清理旧记录

`SaveEmbedding` 在保存新 embedding 时，删除同一 `topic_tag_id + embedding_type` 下 text_hash 不匹配的旧记录。

**替代方案**：保留旧 embedding 作为历史 — 但这会导致查询结果混乱（`FindSimilarTags` 可能匹配到过时的 embedding，且同一 tag 多条 embedding 会占据多个 top-N 位置）。选择清理。

### Decision 4: 合并建议增量记录

新增 `tag_merge_suggestions` 表，双通道写入：

**通道 1（增量，零成本）**：`findOrCreateTag` 创建新 tag 后，如果 `TagMatch` 返回了 candidates，调用 `RecordMergeSuggestions(newTagID, candidates)` 将候选对写入 suggestion 表。以 `(new_tag_id, existing_tag_id)` 为唯一约束，已存在则 skip。

**通道 2（异步全量扫描，手动触发）**：遍历所有 tag，每个 tag 复用 `FindSimilarTags` 做 ANN 查询（单 tag 毫秒级），结果增量写入同一张表（同样 skip 已存在的对）。通过 SSE 实时推送扫描进度。

**替代方案**：保留原 `ScanSimilarTagPairs` 的 cross-join 全表扫描 — O(n²)，标签多了卡死。

**为什么选双通道**：
- 增量通道覆盖日常场景（提取时自动积累），零额外计算成本
- 全量扫描作为补充（老标签对之间可能相似但从未被同时提取），按需触发

### Decision 5: 全量扫描进度用 SSE 推送

全量扫描通过 `GET /merge-preview/scan/stream`（SSE 端点）实时推送进度，前端用 `EventSource` 接收。避免轮询。

SSE 端点使用 Gin 的 `c.Stream()` + `c.SSEvent()` 实现，全局单例 channel 推送进度消息。

**消息格式**：
```json
{"status": "scanning", "total": 590, "scanned": 342, "current_category": "keyword", "new_suggestions": 23}
{"status": "done", "total": 590, "new_suggestions": 47}
```

**替代方案**：前端轮询 `GET /merge-preview/scan/status` — 可行但不够干净，SSE 天然适合单向进度推送。

### Decision 6: 接入标签合并 UI

在 TagsPage 左侧栏添加"标签合并"按钮，复用已有的 `TagMergePreview` 组件。`TagMergePreview` 改为从 `tag_merge_suggestions` 表读取候选对（`GET /merge-preview`），并支持触发全量扫描（`POST /merge-preview/scan`，进度通过 SSE 展示）。

合并操作（`POST /merge-with-name`）完成后，将相关 suggestion 标记为 `merged`。

### Decision 7: 数据修复策略

- 清理 tag 94712 的冗余 embedding（保留最新 1 对 identity+semantic）
- 逐条审查 tag 94712 的 article_topic_tags 关联，只保留 LLM 真正提取了"共产党员"的文章
- 检查是否有其他 tag 存在类似污染（embedding 数量异常多的 tag）

## Risks / Trade-offs

- **[同义词碎片化]** "AI"/"人工智能"、"特朗普"/"trump" 等同义词会各自独立创建 → 增量记录自动捕获候选对，用户通过合并 UI 手动处理
- **[数据修复不完整]** 手动清理可能遗漏被间接污染的关联 → 修复后观察 embedding 数量监控
- **[并发竞态仍存在]** 多 goroutine 同时 `findOrCreateTag` 写同一 tag → embedding 不再合并后风险降低（不会有多个 goroutine 争抢修改同一个 tag 的 identity），但 slug 精确匹配的并发写入仍需注意。如需彻底解决可加分布式锁，但当前不优先
- **[SSE 连接管理]** 全量扫描期间客户端断连 → 后端继续执行，结果写入 DB，客户端重连后读表即可
