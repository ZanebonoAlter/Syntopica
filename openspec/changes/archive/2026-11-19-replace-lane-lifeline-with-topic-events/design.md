# Design: 线索行内「相关事件」推荐（thread 向量版）

## 背景与两次否决

被替换对象：`DailyReportMiniLifeline.vue` 的 7 天折线（窄容器下详情面板塌成一字一行）。

**否决 1（形态）**：v1 是「话题卡片底部新增『话题事件』区块，按天然日分组列出最近 7/30 天的所有事件标签卡片」。用户否决原话：「不要堆列表吧，这可读性太差了，界面是给人用的，不是给 agent 看的」。那是把数据摊开供检索，不是给人读。

**否决 2（匹配口径）**：v2 用「事件标签 id 精确匹配」找相关事件。实测有候选的线索只有 **1.6%**——event 标签 id 基本不跨天复用，且 7 天孤儿清理会把零边标签硬删。随后评估「事件标签向量」，实测 top-3 与 thread 向量**零重叠**（见 D3）。

**定稿**：不新增区块、不占版面。每条当天线索下方一行 chip，hover 出预览，点击直接读原文；匹配口径用 **thread 向量**。

## D1 单位是「线索」，不是「话题」

推荐对象是**当天的 thread**（线索），不是话题整体。话题级聚合必须按天分组，天然会长成列表墙；线索级只针对「我正在读的这一条」，天然收敛成 2–3 个 chip。

## D2 批量端点，避免 N+1

`GET /api/daily-reports/:reportId/thread-relations?days=7|30`，一次返回该期日报**所有**线索的推荐，按 `thread_id` 分组：

```json
{
  "report_id": 88,
  "days": 7,
  "window_start": "2026-11-10",
  "window_end": "2026-11-16",
  "status": "ready|partial|empty",
  "threads": {
    "18669": [
      {
        "thread_id": 14356,
        "title": "企业级 Agent 平台与组织机制探索",
        "summary": "……",
        "period_date": "2026-11-02",
        "score": 0.890,
        "level": "strong",
        "confirmed": false,
        "source_article": { "id": 789, "title": "…", "feed": "…", "published_at": "…" }
      }
    ]
  },
  "total": 12,
  "skipped_no_embedding": 3
}
```

窗口 `[report_date - days, report_date - 1]`，**不含当前期**。`days` 只接受 7/30，其它回落 7 并回显实际值。

## D3 候选集：thread 向量余弦（附选型实测）

**候选**：窗口内、同一 `persistent_topic_id` 的其它 section 所辖 thread，与当前 thread 的 `embedding` 余弦相似度 ≥ **0.75**。

**为什么不是事件标签**（2026-11-17 实测，可复现）：

| 口径 | 历史候选池 | 平均 top-3 重合 | top1 一致 |
| --- | --- | --- | --- |
| thread 向量 | 235 条 | — | — |
| 标签向量·平均 | 44 条（19%） | **0.00/3** | **0/6** |
| 标签向量·最大 | 44 条 | **0.00/3** | **0/6** |

典型反例：

- 「企业级 Agent 落地需补齐托管与度量基建」：thread 0.890 →「企业级 Agent 平台与组织机制探索」（同一件事）；标签 0.754 →「Agent 协作架构向角色分工与路由拆分演进」（泛）。
- 「插件复用历史会话降低 AI 编程代理 token 消耗」：thread 0.854 →「本地记忆库打通多个 AI 编码 agent 的会话孤岛」（高度相关）；标签 0.888 →「编程 Agent 修复后仍持续执行多余步骤」（无关且分更高）。
- 标签 max 出现 **1.000** 推「Agent 客户端与浏览器操作能力外延」——不同标签共享同一段关键词向量导致满分，区分度归零。

三条结构性原因：

1. **候选池差 5 倍**：历史 `thread.tag_ids` 引用的 4173 个唯一标签里，65% 的行已被孤儿清理硬删，只剩 1181 个（28.3%）能算向量；thread 是主表列，94.7% 在。
2. **聚合必然丢信息**：一条线索 2–3 个标签，平均把强相关稀释、最大把巧合关键词放大。
3. **比的东西不同**：标签比「两个概念像不像」，thread 比「是不是同一条线的进展」。

> 附：标签**确实**有向量（`topic_tag_embeddings.embedding_type='event_keyword'`，8084 条 / 覆盖 88.4% 活标签 / 有 HNSW 索引），瓶颈不在覆盖率而在「历史引用所指的行已被 GC」。这一点在 v2 评审时曾被我误读为「标签没有向量」，此处更正记录。

**thread 向量的来源**：`daily_report_threads.embedding`，由日报生成流程按 **thread title** 批量计算（`service/daily_report_thread_fit.go`，原本用于 fit_distance）。不额外调 embedding、不写回。

**跳过的线索**：当前 thread 无 embedding（约 5.3%）→ 该线索不产出推荐，计入 `skipped_no_embedding`；历史 thread 无 embedding → 不进候选池。

## D4 打分与档位

```
score = 1 - (current.embedding <=> candidate.embedding)   // 余弦相似度，0..1
level = score >= 0.85 ? "strong" : "related"              // 只有 ≥0.75 才进入列表
```

- 分数**不取整、不二次加权**。相似度就是相似度，不掺时间衰减——「三天前但高度相关」比「昨天但只是同领域」更值得推。
- 阈值 0.75 的标定：实测同话题 30 天内 top-3 落在 0.75–0.89；低于 0.75 的候选在人工抽查里已明显泛化。
- 排序：`score` 降序 → `period_date` 降序 → `thread_id` 升序。后端返回全部（前端只渲染前 3）。

## D5 来源文章与 confirmed

- **来源文章**：取候选 thread 的 `related_article_ids` 首篇，一次批量查 `articles` 拿 title/feed/published_at；查不到则 `source_article` 为 `null`（前端该 chip 不可点，浮层不显示来源行）。
- **confirmed**：候选 thread 的 `tag_ids` 中若有 event 标签在本话题存在 `state='confirmed'` 的注记（`topic_key_event_markers.tag_ids_snapshot`），`confirmed=true` → chip 加 ★。只读，不改注记。

## D6 状态三态

| status | 条件 |
| --- | --- |
| `ready` | 正常返回，至少一条推荐且无跳过 |
| `partial` | 有当前 thread 因缺 embedding 被跳过（`skipped_no_embedding > 0`） |
| `empty` | 窗口内没有任何线索有推荐（`threads` 为空对象 `{}`） |

**不再有「悬空标签」这一态**——thread 向量不依赖标签行是否存活，失效标签问题在候选集层面消失。

## D7 前端结构与缓存

```
useDailyReportReader（既有，拿 section/thread）
  └── useThreadRelatedEvents（新，模块级单例）
        key = `${reportId}:${days}`
```

- 话题卡片首次展开时 `ensure(reportId, days)` 拉一次，覆盖整份日报；
- 切 7/30：cache 命中不重取；
- 推荐失败不阻断阅读：chip 行不渲染，卡片其余照常。

## D8 移除清单与回归面

| 项 | 处置 |
| --- | --- |
| `DailyReportMiniLifeline.vue` | 删除 |
| `buildLifelineWindow` / `buildBezierPath` | grep 确认无消费方后删除 |
| `useDailyReportReader.ts#ensureLifeline` + `TopicLifelineData` | 日报侧调用链退役；`getTopicLifeline` API 方法保留 |
| 后端 `lifeline` 端点 | 保留不动 |
| `selectedDayByTopic` / `selectLifelineDay` / `historicalSections` / `historicalPending` / `#details` 插槽 | 随折线一并退役 |
| `useTopicKeyEvents` | 不动；★ 由后端读 confirmed 判定 |

## 风险与取舍

| 风险 | 处置 |
| --- | --- |
| thread 向量召回的是「同领域相关」而非「同一事件」，可能被试读成「这两条是同一件事」 | chip 措辞用「相关」不用「同一事件」；tooltip 显示完整标题与摘要，让人自己判断 |
| 0.75 阈值下仍可能推出泛化条目 | 只渲染前 3；阈值常数集中在代码一处，便于按真实分布调整 |
| 当前 thread 无 embedding 时无推荐（5.3%） | 静默跳过（计入 `skipped_no_embedding`），不渲染 chip 行 |
| 同话题内候选很多（235 条） | 查询走 `persistent_topic_id` 索引限定 section 后收敛，单期 30 条 thread 量级可控 |

## 不做

- 不做「最近 N 天事件时间轴/列表」（v1，已否决）。
- 不用事件标签向量做匹配（v2，实测零重叠）。
- 不调 embedding、不写 `related_event_embeddings`、不复用 `related_event_similarity_threshold`。
- 不做分页/虚拟滚动。
- 不新增数据库索引（`persistent_topic_id` / `section_id` 够用；`daily_report_threads.embedding` 不加索引——按 section 收敛后是小集合精确算）。
