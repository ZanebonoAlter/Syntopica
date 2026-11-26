## Context

- 全库唯一 tsvector 列 `articles.search_vector` 用 `simple` 分词器，中文整句成块导致全文路径漏召回 97%（实测 2 vs 67 篇）；裸 LIKE 全表扫 ~3s（树莓派实测），均不可用。
- pg_trgm 扩展未安装（现装扩展仅 plpgsql/vector）；SD 卡余 60G，库 3.2G，articles 表 299MB。
- pgvector 与 embedding 管线（airouter 缓存路由 + embedding_queues）已就位；`topic_tag_embeddings` 42,200 条 2560 维向量（identity/semantic/event_keyword 三型）。
- 归档文章（36,331 篇，94.6%）文本字段全保留、标签边仅保留 7 天（`tag_edge_retention_days` GC，实测归档边 100% ≤7 天）；挂签文章仅 7,011 篇（18%）。
- 查询预处理实验结论（/tmp/search-exp，2,059 篇 + 8,293 词词典）：词典切词对词重组/概念/混合场景召回 0~2%→100%；trigram 目标分数 p50=0.143、非目标 p99=0（阈值 0.10 召回 26%、均命中 0.4 篇）；span 场景组合召回 ~45%，余量交语义路径。
- 现有 `ArticlePreviewModal`（AppDialog 90vw/85vh 高度链、文本/内嵌网页双模式）为 tags 域共用组件，搜索直接复用。

## Goals / Non-Goals

**Goals:**
- 中文为主的站内搜索达到可用召回质量：四层字面管线（空格拆分/词典切词/整串/trigram）+ 标签语义路径 + RRF 融合
- 查询路径毫秒级（走索引），首屏 P50 < 1s（语义并行不阻塞）
- 覆盖含归档的全部文章；三路（关键词/语义/LLM）独立降级，关键词路径永远在线
- 修复 `articles?search=` 中文失效，消灭最后 tsvector 消费点

**Non-Goals:**
- 不给文章生成 embedding（38K 回填不做；语义召回走标签向量间接路径）
- 不做影子边表（`article_tag_history`，归档时复制标签边）——未来演进项，一期验证召回不足再上
- 不做 content 列 trgm 索引（"深度搜索"二期视需求）
- 不做 BM25/tsvector 重建、不做专业中文分词器（zhparser/jieba，arm64 维护成本不划算）
- 不动 retention 体系（归档逻辑、7 天边 GC 原样）

## Decisions

### D1: 关键词匹配走 pg_trgm GIN 索引，弃用 tsvector 路径

- `CREATE EXTENSION IF NOT EXISTS pg_trgm` + `articles.title`/`articles.description` 两列 `gin_trgm_ops` GIN 索引（估算 100~200MB）
- 查询侧 `LIKE '%q%'` 由 trigram 索引加速（contrib 自带、零编译）；`similarity()` 用 pg_trgm 原生函数
- **替代方案否决**：zhparser/pg_jieba 需 arm64 编译维护；裸 LIKE 实测 3s 不可用
- 旧 `search_vector` 列、触发器与 GIN 索引随迁移 DROP（最后消费点已切走）；`feed_service` 归档置 NULL 逻辑同步移除该字段引用

### D2: 查询预处理在服务端 Go 实现四层管线

- L0 空格拆分（unicode 空白，含全角）；L1 词典= `topic_tags.label` 中 3~8 字词（启动加载 + 标签写入时失效重载的内存缓存），正向最大匹配；L2 整串 LIKE；L3 `similarity()` ≥ 0.10（`search_trigram_threshold` 可配）
- **实现期修订（2026-10-14 真库实测）**：① L3 仅作用于 `title` 列——`description` 列 GIN recheck 需 detoast 数万候选行，单层 5.9~8.5s 而三词实测独占召回 0 行，砍掉后热查询 ~400ms 达 P50<1s（L0-L2 LIKE 仍覆盖 desc，走同一对 GIN 索引无性能问题）；② 词典缓存失效改 stale-while-revalidate——16.5K 标签 DISTINCT 加载 ~2s，打标活跃期同步重载会把失效后首查拖到 2s+，改为旧词立即返回 + 后台单飞重载（`EnsureFresh` 同步版供测试）；③ HandleSearch 中 LLM 扩展路与语义路同时发车（原串行：语义 join 后才跑扩词）——慢 embedding（3s 封顶）+ 慢 chat（10s 封顶）不再叠加，端到端延迟≈两路最大值而非之和，实测冷查询 11.2s→3.0s、全路热查询 ~600ms；失败静默降级语义不变，扩展路超时上限（review R-1）不变
- LIKE 通配符（% _ \\）统一转义（复用 discovery 域 `escapeLikePattern` 模式）；查询超 200 字符截断
- 层间去重合并：同文章多路命中时 hit_reasons 聚合，关键词分数取最高层（整串 1.0 / 词典词 0.85 / trigram 原始分）

### D3: 语义路径 = identity 标签向量 + 双重约束 + max 聚合

- query embedding（airouter，缓存键含查询串）→ 与 identity 型 `topic_tag_embeddings` cosine（pgvector `<=>`），`ORDER BY distance LIMIT 8` + `WHERE 1 - distance >= 0.55` 双约束（配置键 `search_semantic_min_cosine`/`search_semantic_top_n`）
- 标签→文章：`article_topic_tags IN (命中标签)` 一次 JOIN，文章语义分取 max(cosine)，命中标签全量进 hit_reasons
- 并行执行：goroutine 并跑关键词 SQL 与语义路径，语义超时 3s 即弃（context 控制），响应标 `skipped`
- **替代方案否决**：semantic 型双查并集（二期）；文章直连向量（Non-Goal）

### D4: 融合排序 RRF + 命中层级加权

- 基础 RRF：`score = Σ 1/(60 + rank_i)`，关键词榜与语义榜各自排名
- 关键词榜内部排序：`命中字段权重（title 1.0 > description 0.7）× 命中层级系数（整串 1.0 / 词典多词 0.85 / 词典单词 0.6 / trigram 原始分）`——补偿单词 AND 放宽召回（实验 mix 场景均 16 篇）的精度损失，多词命中 > 单词命中

### D5: LLM 扩展按需调用，结果区隔

- expand=on 时查询串交 airouter LLM（chat 路由），prompt 约束输出 JSON 数组 5~8 词；扩词补跑 D2 管线，type=expand
- 同查询扩展结果内存 TTL 缓存（10min），会话内重复搜索不重复调用
- LLM 失败不影响主结果；默认 off，无结果空态提供一键开启

### D6: 新增 search 服务挂 reader 域，不新建顶级包

- `internal/reader/search/`：handler（GET /api/search）+ service（管线编排/RRF）+ repository（SQL + 词典缓存）
- 路由注册进 `internal/app/router.go`；前端 `features/search/`（页面 + 组合式函数），入口改 `AppSidebarView.vue`
- 复用 `ArticleCardView`（新增可选 props hitReasons/archived，缺省行为不变）与 `ArticlePreviewModal`（新增可选命中词高亮 prop）

### D7: articles 端点修复与 search_vector 清退同迁移

- 迁移 A：装 pg_trgm + 建 2 个 GIN 索引（`db-migration-execution` 红线，一次性 DDL，入库前 count 留底）
- 迁移 B：DROP `search_vector` 列 + 触发器 + 旧索引；`article_handler.go` 的 tsquery 分支替换为 D2 同款管线（复用 service，仅 active 集 + 分页）
- 回滚策略：迁移 A/B 独立成号可分别回退；代码回滚后旧列已 DROP 的场景——保留迁移 A（索引无害），search_vector 不可恢复（该列数据可由 title/description 重算，属可再生数据）

### D8: 前端 /search 页与状态机

- `AppPageShell mode="contained"`；stores 走组合式函数 `useSearch()`（fetch + 状态机 + 扩展开关 + 分页 cursor 用 offset）
- 预览弹窗复用：命中词列表传入 `ArticlePreviewModal` 做正文高亮；关闭回滚滚动位置（记录 scrollY + 查询 state 不卸载列表组件）
- 侧边栏入口：`navigateTo('/search')`，与"发现订阅源"同款交互

## Risks / Trade-offs

- [trigram 索引写放大：每篇文章入库/更新维护 GIN] → 索引仅 title/description 两列；文章写入频率低（日增百级），实测影响可忽略；监控迁移后 INSERT 耗时
- [词典切词质量依赖标签库词汇密度：词典外词（如"特斯拉"）切不中] → L3 trigram 兜底 + 语义路径；上线后统计 L1 切空率，若高再评估通用词表补充
- [语义路径仅覆盖 18% 文章且归档边 7 天滚动：老文章语义缺席] → 预期管理进文档（proposal 已述）；影子边表为未来演进项
- [LLM 扩展延迟 1~3s] → 默认关 + 无结果才引导开启；扩展结果异步区隔展示，不阻塞首屏
- [search_vector DROP 后旧端点回滚不可逆] → 列数据可再生（title/description 重算）；迁移 B 单独成号，代码切换与 DDL 同 PR 内原子合入
- [ArticlePreviewModal 跨域复用（tags→search）引入耦合] → 仅新增可选 props，不改既有行为；tags 域现测试回归验证

## Migration Plan

1. 迁移 A（pg_trgm 扩展 + 2 GIN 索引）——先行合入，对旧代码零影响
2. 后端 search 服务 + 端点 + articles 端点切换（D7 代码部分）
3. 迁移 B（DROP search_vector 及触发器）与代码切换同批
4. 前端页面 + 侧边栏入口 + 预览复用
5. 验证：中文召回对比（修复前后同词命中数）、延迟 profile、双主题/状态矩阵走查

## Open Questions

- 扩展词缓存的存储位置（内存 TTL vs 复用 ai_embedding_cache 表）——实现时按 airouter 现有缓存模式选，不改行为契约
- `ArticlePreviewModal` 命中词高亮的实现层（组件内 prop vs 包装组件处理）——实现时定，契约只要求正文高亮可见
