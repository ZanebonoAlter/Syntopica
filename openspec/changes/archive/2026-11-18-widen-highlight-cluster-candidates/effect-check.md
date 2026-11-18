# effect-check — widen-highlight-cluster-candidates

## 触发原因

「今日重点」效果依赖测试断言之外的因素（当天标签数据形态 + LLM 实际选择），绿灯后必须真库量化核对（test-design 问句④ / backend testing「绿灯 ≠ 功能有效」）。锚点按 design D3：断言服务端可确定的部分——重建/重算 2026-11-12 board 15220 后**候选白名单第一簇 = 黄仁勋簇**，LLM 头条选择仅记录不强绑。

## 方法

**重建路径未执行（如实记录）**：核对日 **2026-11-15**，11-12 距今 3 天、在 7 天重建窗口内，`POST /api/daily-reports/generate` 会被**放行**。放行场景下重建 = ① 覆盖现存 report 1473（误伤证据报告，重建即抹掉）② 产生真实 LLM 调用 ③ 还需先把 :5100 在跑的旧代码后端换成本 change 新二进制重启。三项副作用均超出本任务授权，改走只读 SQL / 服务层测试路径；**部署新后端后重建该板**是更自然的端到端验证时机（留待主会话决策）。

已执行（全部只读 SELECT，真库 localhost:5432/syntopica）：

1. `board_daily_reports` / `daily_report_threads`：确认 report 1473（board 15220, 2026-11-12）线索清单与现行 highlights。
2. 新口径候选模拟：按 `ListReportEventThreads`（不再按 scope 排除）展开 report 1473 全部 12 条线索的 tag→article 映射，`topic_tags` 过滤 category=event，按内容身份（归一 link，feed 兜底）去重计每标签簇文章数。
3. 黄仁勋簇并集口径：thread 17828 的存活 event 标签集合 → 携带任一标签的线索的 evidence 并集 → 去重计数。
4. 聚簇可行性代理：`topic_tag_embeddings`（embedding_type=semantic，15/15 有向量）两两余弦相似度，0.86 阈值边图 union-find 连通性（tier-2 兜底层同款向量；tier-1 现算 related-event-v1 向量需真实 AI 调用，未烧）。**首位判定对降级层不敏感**：17 个标签全部挂在同一条线索（17828）上，无论 tier-1/tier-2/主体近似哪一层兜底，其标签与证据都只可能聚入同一候选簇，簇内去重篇数（20）与次名（2 篇）的断层不受降级层选择影响；差异仅在簇边界（是否并入相邻小簇，见遗留观察）。
5. 代码路径端到端：stub embedder + stub LLM 的 testcontainer 服务层测试（`TestDailyReportEventClusterCompositeBigThreadRanksFirst` 等）。

## 量化结果

| 事实 | 数值 |
| --- | --- |
| report 1473 现行 highlights | 3 条，article_count 全部 = 1（豆包接入新模型 / 腾讯裁员 / 恒生指数），报告标题「豆包接入新模型」 |
| thread 17828（黄仁勋线索） | scope=composite、tag_ids 17 个（15 个存活 event 标签，2 个已删除标签不参与）、related_article_ids 20 篇 |
| 黄仁勋簇并集去重文章数（新口径） | **20 篇**（携带 BIG 标签的线索数 = 1，即 17828 本身） |
| 当天最强竞争候选 | 2 篇（白宫未干预 / 马斯克声援 / 黄仁勋面临罢免风险三条 composite 线索的标签），其余 single 候选全部 1 篇 |
| 旧口径候选池（对照组） | composite 一刀切后仅剩 single 线索：9 个候选 × 1 篇 → 白名单首位文章数 = 1 |
| 语义向量 0.86 边图 | 15 节点 67 条边、min pair 0.770 / avg 0.881，**连通分量 = 1**（15 标签全部连通，transitive union-find 判定） |
| 排序判定 | 20 ≫ 2：黄仁勋簇按「簇内去重文章数 DESC」排候选白名单**首位**（次键不触发） |

## 结论

**达标交付（服务端可断言部分）**：新口径下 2026-11-12 board 15220 的候选白名单第一簇 = 黄仁勋簇（15 标签、去重 20 篇），对次名候选（2 篇）形成断层优势；旧口径下该簇被 `scope <> composite` 整体排除、头条被 1 篇小簇占据（现行 highlights 为证）。聚簇可行性由语义向量连通性佐证（tier-2 兜底即可成单簇；tier-1 现算向量需部署后重建观察）。LLM 头条实际选择（是否挑黄仁勋簇为第一条）留待部署后重建该板时按 D3 记录——候选序即展示序、白名单首位即大簇，LLM 波动不作为硬门槛。

遗留观察项：同事件近邻标签 274754（黄仁勋面临罢免风险，2 篇）可能并入大簇（簇数升至 ~22），不影响首位判定。
