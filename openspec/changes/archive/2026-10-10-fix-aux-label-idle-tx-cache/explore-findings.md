
## idle in transaction 根因链：AttachAuxiliaryLabels 事务内外部 AI 调用

问题链（两根因同链）：
1. idle in transaction 元凶 = auxiliary_label_service.go:160 `AttachAuxiliaryLabels`：`s.db.WithContext(ctx).Transaction(...)` 内循环 `txService.ResolveAuxiliaryLabel(ctx, item.Label, item.Description)`（txService 由 `NewAuxiliaryLabelService(tx, s.embedder)` 构造，走事务连接）。每个 L1 miss 标签在事务内调 1-2 次外部 AI API：L2 前 `s.embedder(ctx, label, AuxiliaryLabelEmbeddingModeMerge)`；L3 新建前再调 `s.embedder(ctx, label+": "+description, AuxiliaryLabelEmbeddingModeStorage)`（L3 代码在 line ~232-258）。观察值 idle in transaction 4-40s 与 2 次 API × N 标签完全吻合。
2. 全量拉放大器 = 缓存全清失效：line 748 `SELECT id, merge_embedding FROM semantic_labels WHERE label_type='auxiliary' AND status='active' AND merge_embedding IS NOT NULL`（17.5k 行 × 2560 维 ≈ 216MB 文本传输 + strings.Split/ParseFloat ~4500 万次；注释自证 "Eliminates the 216 MB DB transfer on every call"——指缓存命中时）。L3 成功后 `invalidateOnCreate()`（line 257）把 mergeEmbeddings 与 activeLabels 双双置 nil → 循环内下一个标签的 L2 匹配又全量拉。TTL auxMergeEmbeddingsTTL = 10 分钟（line 72）在批量写场景从未活到过期。

次要事务源：semantic_board_upgrade.go 事务闭包内 `s.embedder != nil` 分支调 `s.embedder(ctx, input, AuxiliaryLabelEmbeddingModeStorage)`（约 line 393-400 区域），同模式需移出。

调用方（预期签名不变）：article_tagger.go:253 与 :260，`AuxServiceFactory(repository.Repo.DB(), nil).AttachAuxiliaryLabels(ctx, dbTag.ID, tag.AuxiliaryLabels)`；桥接接口 core/bridge.go:21。

批量能力：airouter.Embed 的 EmbeddingRequest.Input 为 []string，批量现成。embedder 现签名 `func(ctx, input string, mode AuxiliaryLabelEmbeddingMode) (string, []float64, error)`（DefaultAuxiliaryLabelEmbedder，line ~790），拆分时批量单元需可 mock。

竞态事实：semantic_labels.slug 有唯一索引 idx_semantic_labels_slug（models/semantic_label.go:12）；UniqueSemanticLabelSlug（line 840）为查后插入，TOCTOU 窗口现状已存在；打标主写入路径 embedding_queue.go 单 goroutine worker 串行。

**引用**：backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:AttachAuxiliaryLabels、backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:ResolveAuxiliaryLabel、backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:packageAuxLabelCacheGetOrLoad、backend-go/internal/tagmanagement/service/board/semantic_board_upgrade.go、backend-go/internal/tagmanagement/service/core/article_tagger.go:253

<!-- pinned 2026-10-06T14:47:02Z -->

## auxLabelCache 结构与全部失效入口清单

缓存结构：auxLabelCache{mu, activeLabels []models.SemanticLabel, activeLabelsAt, mergeEmbeddings map[uint][]float64, mergeEmbeddingsAt}，包级单例 packageAuxLabelCache（line 95），TTL 常量 auxMergeEmbeddingsTTL=10min（line 72）、activeLabelsTTL 同文件。activeLabels 加载 Select 列集合刻意排除向量列（line ~641：id,label,slug,label_type,aliases,ref_count,description,status,protected,source,display_order,created_at,updated_at）——增量 append 副本须对齐该列集。

全部失效入口清单（增量改造逐点核对）：
- service 内：invalidateOnCreate（line 654，L3 后调用于 line 257）、invalidateOnDisable（line 663，DisableAuxiliaryLabel 内 line 273 调用）、invalidateOnAliasChange（line 672，addAlias 内 line 785 调用，只清 activeLabels ✓ 已正确）
- MergeAuxiliaryLabelAlias（line 285 起）：事务内 source 行 Updates 置 disabled（line 336），事务外需补 delete(mergeEmbeddings, sourceID)——注意该方法自带事务，失效调用需在事务提交后
- handler/composite 直改路径（不经 service invalidate，靠 TTL 兜底，本 change 不恶化不处理）：board_crud_handler.go:330（删除 board 连带 disable）、composite_label_service.go:240、auxiliary_label_service.go:610（另一处 disable Updates）

parse：ParsePgVector = strings.Split+ParseFloat（line ~825 起）；增量塞缓存时直接用 embedder 返回的 []float64（L3 已有 mergeVector），不必再 Parse。

ai_settings 阈值：loadAuxLabelDedupeSim 每次 L2 评估查一次 ai_settings(key='auxiliary_label_dedupe_sim')，默认 defaultAuxLabelDedupeSim=0.95——两阶段拆分后归阶段一，行为不变。

**引用**：backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:auxLabelCache、backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:invalidateOnCreate、backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:MergeAuxiliaryLabelAlias、backend-go/internal/tagmanagement/handler/board_crud_handler.go:330

<!-- pinned 2026-10-06T14:47:15Z -->
