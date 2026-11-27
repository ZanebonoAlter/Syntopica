
## 日报召回链路 + 参数标定

【日报召回链路事实 + 参数标定（2026-11-26 实测，实现阶段直接照此动手）】

## 管线顺序（决定性问题位置）
```
daily_report_orchestrator.go:GenerateDailyReport
  Step 1   collectBoardTags(boardID, date)   // 主查询(已挂载) + fallback(未挂载 Limit 50, 无 ORDER BY)
  Step 2   DeduplicateTags(...)
  Step 2.5 filterTagsByQuality(tags)         // ★ 二元硬开关，新标签在这里死；L1/L2/L3 在 Step 3 才跑
  Step 3   ClusterTagsLane(...)              // line 111
  Step 3后 ApplyLaneConstraintGate(...)
```

## filterTagsByQuality 真实语义
- kept = match_reason ∈ {composite_hit, direct_hit, hit_rate, max_sim}；其余进 weighted
- `len(kept) < 10` → **全部 weighted 放回**（噪声也进）
- `len(kept) >= 10` → **weighted 全部丢弃**；kept>30 再按 tier/score/疑似AI占比 截到 30
- 单篇新事件恒为 weighted（max_sim 需 score≥0.8 + hits≥min(2,N) + hitRate≥0.3，hits=1 时 score 只有 0.60~0.73）

## collectBoardTags 口径
- 主查询：`topic_tag_board_labels.semantic_board_id=?` AND NOT direction_mismatch AND status='active' AND category='event' AND pub_date ∈ [当日00:00, 次日00:00)
- 排他：`NOT EXISTS(stronger.score >= 本板score + semantic_board_report_exclusive_gap)`；gap=0 关闭。**gap 必须走 Go 侧分支，不能内联成 SQL 参数**（pgx 会把左侧参数绑定判错）
- fallback：未挂载 event 标签 `Limit(50)` 无 ORDER BY

## 向量空间与索引（实现必读）
- 管线用 `embedding_type='semantic'`（`ListTagSemanticEmbeddings`，daily_report_topic_repository.go:505）
- 现成 HNSW：`idx_topic_tag_embeddings_semantic_halfvec_hnsw` on `((embedding)::halfvec(2560)) halfvec_cosine_ops WHERE embedding_type='semantic'`
- 查询必须用同表达式 `(embedding)::halfvec(2560) <=> $1::halfvec(2560)` 才命中索引；最终裁决用原列 float32
- `board_persistent_topics.embedding` 无向量索引（61 条 active 泳道，够小，不必加）

## 标定结果（12 版块 / 61 泳道 / 122 lane-day / 307 对人工样本）
- K×τ 召回量：K=8 时 τ0.15→106、τ0.17→156、τ0.20→237；K=8→12 只 +22%（K=8 拐点）
- 人工精度：<0.15 ≈95%；0.15–0.17 ≈72%；0.17–0.195 ≈65%
- K=8 τ=0.16 放行构成：约 30% 已质量 / 30% 救 weighted / 40% 救未挂载
- **默认 `lane_recall_top_k=8`、`lane_recall_max_distance=0.16`**
- 量纲警告：全部为**标签级**距离；section 级门槛 L1=0.18 / L2=0.30 不可搬用

## 反例（禁止用 board score 做判据）
11-26 board 15220：`英伟达暂停东南亚芯片交付`=0.700 与 `索尼公布PS+会免名单`=0.700、`微软弃用 Windows 11 蓝牙`=0.700、`绿媒态度急转弯`=0.700 **完全同分**；按 score Top-8 会捞进 Google 刷脸(0.733)/PS+/Windows

## 泳道侧数据形状
- 122 条 lane-day 中 71 条（58%）rank1 ≥0.20 → 当天确实没有专属新闻
- 干净泳道 rank1：沙特胡塞 0.075 / 美伊博弈 0.082 / 以色列选举 0.108 / 黄仁勋 0.076 / 芯片算力 0.103
- 被污染泳道 rank1：以黎冲突 0.241（拉进 海湾/美伊/加沙/俄乌）、旅游 0.264、能源 0.247 → «没新闻就空着» 是正确行为

**引用**：backend-go/internal/topicgraph/service/daily_report_orchestrator.go:collectBoardTags、backend-go/internal/topicgraph/service/daily_report_orchestrator.go:filterTagsByQuality、backend-go/internal/topicgraph/repository/daily_report_topic_repository.go:ListTagSemanticEmbeddings、backend-go/internal/topicgraph/service/daily_report_lane.go:ClusterTagsLane

<!-- pinned 2026-11-26T16:08:48Z -->
