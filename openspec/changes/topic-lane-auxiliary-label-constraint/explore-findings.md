
## 约束判据实测：绝对阈值不成立、相对偏好可分离

【方向 A「话题辅助标签约束」的判据实测结论 —— 决定实现形态的关键】

① **「同一个 tag 同时满足两个约束锚」不可行**：锚「以色列」↔「黎巴嫩」余弦距离仅 0.14(bare)/0.28(ev)/0.36(kw)；远程空间用 tag 锚（「以色列总理」identity vs「真主党」identity）在 event_keyword 空间检索，τ=0.50 时 6521~7078/7400（88%）event 标签「同时满足」两锚，命中全是纯以色列组阁新闻。机理：向量的「近」=「语义同类」≠「语义包含」——真正的双主体事件句「以色列国防部长卡茨警告黎巴嫩真主党」到「以色列」锚 d=0.538，比纯单侧主体词「以色列总理」（0.236）远一倍。

② **单一绝对距离阈值不可行**：本地空间 section 级模拟（107 个 tag 向量，V1_prod 文本 × kw 锚）τ=0.45 时正样本放行 42~58%、纯以色列误判 6~27%、纯黎巴嫩误判 55% —— ROC 不分离。

③ **【可行】相对偏好（margin）判据**：约束 i 被覆盖 ⟺ ∃tag t 使 d(t,A_i)+margin_i < d(t,A_j)（∀其它约束 j）；再叠加绝对上限 max_distance。实测 margin=0.05、锚编码 `term + " " + "event"` 时：正样本放行 **64.4%**、纯以色列误判 **0%**、纯黎巴嫩误判 **0%** —— 分离。分离度排序：V1_prod×event 64.4% > V1_prod×kw 53.6% > V2_bare×kw 46.8% > V1_prod×裸词 26.2%。

④ **margin 取值**：0.05 正样本 64.4%；0.10 掉到 25.2%；0.15 起基本清零。max_distance 默认 0.55（纯以色列 tag 到「黎巴嫩」锚 p50=0.593）。

⑤ **模型空间**：远程↔本地同文本余弦仅 0.74，且**换本地 Q4_K_M 后仍为 0.749**（而本地 Q6↔Q4 = 0.984）→ 差距**不是量化造成**，是 pooling/EOS/前缀或模型 size 差异；换量化档不改变结论。锚须与 tag 同空间 → 记录 embedding_model + 跨空间 tag 跳过（不计入覆盖也不判违反）。

⑥ **7 天 GC 是硬约束**：`internal/tagmanagement/service/core/edge_gc.go` DefaultTagEdgeRetentionDays=7；topic_tags 存活 11月 15398 个 vs 6~10 月合计 2392；有标签文章 11月 6696 篇 vs 10月 390 篇；「真主党」tag created_at=2026-11-12（首次产出）。→ 任何基于 tag id 的约束会自行失效；且 section.cluster_tag_ids 92.8% 悬空（14679/15820），判定输入必须走 article_topic_tags（悬空 0%）。

⑦ **降级污染实测为 0**：11-13 后无新日报（最新 period_date=2026-11-12），section/质心全在远程空间；仅 5 条本地 topic_tag_embeddings + 565 条本地 ai_embedding_cache（后者有 model 列）。远程恢复后无需重算。

【已开的 change】`openspec/changes/topic-lane-auxiliary-label-constraint`（complex / ui-impact minor，5/5 制品完成，validate+doc-impact verify+check-tasks-tail 全绿）。数据模型 `board_persistent_topic_constraints`（term / term_embedding 无维度限定 vector / embedding_model / mode / margin / max_distance）；闸门落 lane group 级（assembleLaneGroups 之后）；违例补一次 L2 裁决、仍违例则沿用「switch 集外 → 降级 new」；前端配置区阻塞于 `lane-manual-correction-loop`（0/60 任务）。

**引用**：backend-go/internal/topicgraph/service/daily_report_lane.go:assembleLaneGroups、backend-go/internal/topicgraph/service/daily_report_lane.go:parseL2Response、backend-go/internal/tagmanagement/service/core/embedding.go:buildTagEmbeddingText、backend-go/internal/tagmanagement/service/core/edge_gc.go、backend-go/internal/platform/airouter/router.go:Embed

<!-- pinned 2026-11-13T10:21:09Z -->
