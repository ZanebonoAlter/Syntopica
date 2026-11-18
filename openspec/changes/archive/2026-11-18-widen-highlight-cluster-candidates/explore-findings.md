
## dry-run修正：composite误伤是主根因，分区墙保留

2026-11-15 dry-run（真库 11.12/11.14 全球科技巨头动态 board 15220）修正了最初诊断：

1. **用户实际案例是 11.12（report 1473），不是 11.14**：11.12 头条（highlights[0]）=「豆包接入新模型」article_count=1；11.14（report 1486）正文无豆包话题、旧排序黄仁勋组本就是组内第一。
2. **今日重点层主根因 = composite 误伤**：黄仁勋线索 17828（20 篇文章）挂 17 个同事件不同表述的 event 标签（黄仁勋承认出轨事件/黄仁勋承认出轨/黄仁勋出轨丑闻/黄仁勋与金伯利·鲍威尔婚外情/黄仁勋婚外情事件…），scope 推导规则「event 标签数≥2 → composite」误判；ListReportEventThreads（report_event_clusters.go:30-55）按 scope<>'composite' 过滤 → 大热点整体出局，候选池只剩 1 篇小簇，头条被豆包占据。11.12 该板 scope 分布：composite 3 条线索 24 篇 / single 9 条线索 9 篇。
3. **正文层 = 分区墙而非排序公式**：11.12 正文豆包话题（tier 0/0.785）在 active 分区、黄仁勋（tier 0/**0.935** 全场最高）在 candidate 分区，buildQualityZones 的 active→candidate→other 分区顺序把它压在第二屏；纯质量全局排序黄仁勋直接 no1（连量加权都不需要）。11.14 同样：黄仁勋 tier 0/0.880 candidate，active 分区只有 tier 3 的半导体/监管两条。
4. **用户拍板（2026-11-15）**：只修重点层（放宽聚簇 composite 排除），正文分区墙保留为产品语义，正文排序公式（质量+量加权）方向废弃。
5. **数据事实**：板表 = semantic_labels（label_type='board'），无 semantic_boards 表；topic_tags 无 article_count 列（运行时算）；1473 头条三条（豆包/腾讯裁员/恒生指数）article_count 全=1。

落点：新 change = widen-highlight-cluster-candidates（改动点 ListReportEventThreads 一处过滤删除）；旧 change rank-topics-by-quality-and-volume 已删。

<!-- pinned 2026-11-15T07:17:33Z -->
