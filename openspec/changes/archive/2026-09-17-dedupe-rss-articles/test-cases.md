# test-cases: dedupe-rss-articles

> 复杂档白盒账本。落点均为**拟定**，不代表已存在或已通过。每条映射 specs/rss-article-dedup/spec.md 的 Scenario；`人工` 行不设测试文件。

## 故事 S1：快讯不再重复入库（锚 Requirement: RSS 入库按 (feed_id, link) 判重）

| # | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1.1 | 同 feed 刷新两次，第二次条目 link 相同但 title 改变 | 快讯同 link 改标题不产生新文章 | articles 不新增行，已有条目不动（未走更新路径时） | service+DB | reader/service/feed_service_test.go |
| 1.2 | 两个不同 feed 各自收录同 link 条目 | 跨 feed 同 link 各自保留 | 每个 feed 各一条，互不干扰 | service+DB | 同上 |
| 1.3 | 并发两个刷新流同 feed 同 link（依赖唯一索引） | 同 feed 并发刷新不产生重复 | 仅一条成功，另一条 Create 失败被 continue 吞掉，刷新整体成功 | DB | platform/database 迁移测试（唯一索引冲突注入） |
| 1.4 | 上库人工核对 | 同上三个 | 部署后同 feed 重复组计数为 0 | 人工 | 验证 SQL（tasks 3.3） |

## 故事 S2：快讯滚动更新收敛为单条（锚 Requirement: link 命中已有条目时的更新语义）

| # | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 2.1 | link 命中，title/description 均未变 | 内容未实质变化时跳过 | 不 UPDATE 内容、不入队、无 AI 调用 | service | reader/service/feed_service_test.go |
| 2.2 | link 命中，title 变化 | 内容实质变化时更新并重走处理链 | UPDATE 内容字段；状态按 buildArticleFromEntry 重置；衍生字段清空；旧标签删除；enqueueArticleProcessing 被调 | service | 同上 |
| 2.3 | link 命中，description 变化（title 同） | 同上 | 同 2.2 | service | 同上 |
| 2.4 | feed 开关矩阵 ×4（firecrawl/summary 各开关）下内容更新 | 同上 | 状态字段四组合各自正确（pending/incomplete/complete 分布） | service | 同上（表驱动） |
| 2.5 | 更新后链条完成事件到达（模拟 firecrawl_completed job） | 处理链完成事件触发重打标 | existingCount=0，走 AI 正常打标，新标签基于新内容 | core 集成 | tagmanagement/service/core（tag job 处理测试） |
| 2.6 | 更新前已有旧标签 | 同上 | 旧 article_topic_tags 已删、孤儿 topic_tags 清理 | core | 同上 |

## 故事 S3：跨 feed 打标只烧一次 AI（锚 Requirement: 跨 feed 副本打标复用）

| # | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 3.1 | 副本 A 已打标（llm, 5 标签），副本 B（同 link 不同 feed）进入 tagArticle | 复制已有打标结果 | B 获得 5 条 article_topic_tags，source='reuse'，score 沿用；AI 调用次数 0 | core+DB | tagmanagement/service/core/article_tagger 测试 |
| 3.2 | 副本 B 已有 1 条标签（与 A 部分重叠）时复用 | 同上 | 重叠标签不重复插入（无 (article_id, topic_tag_id) 重复） | core+DB | 同上 |
| 3.3 | 无同 link 副本或副本无标签 | （反向） | 走原 AI 提取路径，source='llm'/'heuristic' | core | 同上 |
| 3.4 | 手动 RetagArticle（Force） | 手动重打标不复用 | 清旧标签后重新 AI 提取，不套用复用 | core | 同上 |
| 3.5 | 复用后查 articles.tag_count | （一致性） | tag_count 与实际标签行数一致 | core | 同上 |

## 故事 S4：存量重复归并（锚 Requirement: 存量重复文章归并迁移）

| # | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 4.1 | 构造重复组（副本有标签/正文/阅读行为，保留条更全） | 保留条按处理链完整度选择 | 打分最高者保留，其余删除 | DB 迁移 | platform/database 迁移测试（PG） |
| 4.2 | 混合 archived 组（副本 active、保留候选 archived） | 同上 | active 条保留（archived 副本删除），活跃窗口不丢文章 | DB 迁移 | 同上 |
| 4.3 | 副本标签与保留条标签部分重叠 | 关联数据无损合并 | 重叠去重、不重叠改指向；无 (article_id, topic_tag_id) 重复；tag_count 重算一致 | DB 迁移 | 同上 |
| 4.4 | 副本有 pending/processing 的 tag_jobs/firecrawl_jobs | 同上 | 改指向保留条，不悬挂 FK；completed 直接删 | DB 迁移 | 同上 |
| 4.5 | 迁移在已清理库重跑 | 迁移幂等 | no-op 无报错 | DB 迁移 | 同上 |
| 4.6 | 迁移中途注入失败 | （原子性） | 全回滚，库与迁移版本表均无残留 | DB 迁移 | 同上 |
| 4.7 | 迁移完成后直插同 feed 同 link 第二条 | 唯一索引随迁移建立 | 唯一索引拒绝（uq_articles_feed_link, link != '' 部分索引）；空 link 行不受约束 | DB 迁移 | 同上 |
