# Review — add-article-search（2026-10-14）

审查范围：`backend-go/internal/reader/search/` 全包（dict/default/expand/semantic/fusion/repository/preprocess/config/list_filter/handler）、`postgres_migrations.go` 仅 `pgTrgmMigration`/`searchVectorDropMigration` 两新函数、`article_handler.go` search 分支 + `article_excerpt.go` 导出、`feed_service.go` 归档 updates、`routes.go` 装配、`airouter/router.go` 白名单一行、`tagger.go`/`hard_merge.go` InvalidateDict 接线、前端 `features/search/` + `pages/search.vue` + `api/search.ts` + `types/search.ts` + ArticleCardView/ArticlePreviewModal/AppSidebarView、docs 登记粗验、测试断言面粗验（不逐行审测试实现）。树上其它 change 脏文件（keyEventMarkersMigration、dataenrichment/topicgraph、daily-report-watch 等）未审。

不变量 grep 验证（均通过）：
- ✅ 红线 12 零写入：search 包生产代码无任何 `.Create/.Save/.Update/.Delete` 数据写；唯一 `.Exec` 为 `set_config(..., true)`（SET LOCAL 事务级 GUC，非数据写入，repository.go:254）
- ✅ 红线 10 归档默认含入：`parseArchivedParam` 缺省 include，repository 语义路均仅 `active-only` 时加过滤
- ✅ 红线 11 关键词路永远在线：语义/扩展任何 error → `skipped` 降级 200；L3 失败仅 warn 降级 LIKE-only；关键词 SQL 硬错误如实 500
- ✅ 列表端点归档默认排除未变（article_handler.go L138-141，`archived = c.Query("archived") == "true"` 默认 false）
- ✅ 红线 9 窄投影：`articleSearchSelect` 无 content/firecrawl_content/ai_content_summary，excerpt 源 substr 2000 + 复用 buildExcerpt ≤200
- ✅ 红线 5：`magnify`/`arrow-left` 均已在 iconify-subset.json（子集键无 `mdi:` 前缀，按 icons 键名验证）
- ✅ SQL 全参数化占位符；escapeLikePattern 与 `ESCAPE '\'` 配对；pgvector 经 `?::vector` 参数绑定
- ✅ 语义 goroutine 无泄漏：buffered channel(1) + 内部 3s WithTimeout；expand 缓存 mutex 覆盖完整、时钟可注入；DefaultDict 按 DB 指针换绑有锁保护
- ✅ handler 参数校验：空/纯空白 q → 400（不产生召回动作）；page/per_page 非整数或 <1 → 400；per_page>100 钳 100+warn；archived/expand 非法静默回退
- ✅ ArticlePreviewModal 高亮用 `mark.textContent` + DocumentFragment 切分（无 innerHTML 拼接，无 XSS 面）
- ✅ 前端 id 字符串化/snake→camel 映射收敛在 api/search.ts 归一层，组件层零散落映射
- ✅ useSearch seq 过期响应丢弃（runSearch/loadMore 双路径）、loadMore 可被新首查打断
- ✅ 迁移注册顺序（20261013_0001 → 0002）、破坏性守卫 MIGRATIONS_ALLOW_DESTRUCTIVE、DROP 顺序 trigger→function→index→column
- ✅ RegisterRoutes 在 `database.InitDB` 之后（main.go:50 → SetupRoutes），装配时 database.DB 非 nil

---

### R-1 2026-10-14【High】expand=on 主响应串行等待无超时上限的 LLM 调用，最坏挂起 ~600s
- **状态**：已修复（runExpand 入口 context.WithTimeout 10s〔expandPathTimeout，var 可测缩〕，超时走 LLM 失败同款降级；回归测试 TestRunExpandTimeoutBound）
- 位置：`backend-go/internal/reader/search/handler.go` `HandleSearch` 的 `runExpand` 调用（主响应路径）+ `expand.go` `AirouterExpander.ExpandQuery`；超时事实源：`internal/platform/airouter/store_vendor.go:231`（`TimeoutSeconds <= 0 → 600`）、`openai_compatible.go:303/576`（每请求 `WithTimeout(vendor)`）。
- 问题：语义路有 3s `WithTimeout`（semantic.go），但 expand 路的 LLM 调用只用请求 ctx，无自身超时上限；airouter vendor 默认超时 600s（为后台批处理设计的量级）。`runExpand` 在关键词+语义融合之后、JSON 响应之前**串行执行**——LLM 慢（非失败）时整个 `/api/search` 响应（含关键词主结果）被阻塞到 LLM 返回或 600s。design「Risks/Trade-offs」明文「扩展结果异步区隔展示，**不阻塞首屏**」，实现与制品意图冲突。
- 缓解项（不改变结论）：默认 off；10min TTL 缓存同查询免重复调用；浏览器断开会经 ctx cancel 中断调用。
- 建议：`runExpand` 入口 `ctx, cancel := context.WithTimeout(ctx, 10*time.Second); defer cancel()`，超时按 LLM 失败同路径降级（`expand.words` 置空、主结果正常返回）——一行级改动即可满足 design 意图。

### R-2 2026-10-14【Medium】dict.go 重载在途发生的 InvalidateDict 会被 reload 完成时的无条件 stale=false 吞掉
- **状态**：已修复（gen 代数计数：reload 记起始 gen，完成时仅 gen 未变才清 stale，窗口内失效保 stale 待下轮重载）
- 位置：`backend-go/internal/reader/search/dict.go` `EnsureLoaded` 后台单飞分支 + `reload` 尾部 `d.stale = false`。
- 问题：时序——T0 `InvalidateDict`（stale=true）→ T1 查询触发后台重载（reloading=true）→ T2 重载的 DB 读已在途时又发生标签写入 + `InvalidateDict`（stale 仍 true，幂等置位）→ T3 reload 完成读到**不含 T2 新词**的快照，却无条件 `stale=false` → T2 的失效丢失，新词最长**无限期**缺席词典（直到下次任意标签写入触发新失效）。打标活跃期写入频繁可自愈，但「恰逢重载窗口的最后一次写入」是真实可达的丢失场景，与注释宣称的 stale-while-revalidate 语义有偏差。
- 影响：L1 切词漏新词（增强层，L0/L2/L3 兜底仍在，不违红线 11），非正确性故障。
- 建议：代际计数（generation）：`InvalidateDict` 时 `d.gen++` 并记录；后台重载启动时记下起始 gen，完成时仅当 `gen == 起始 gen` 才清 `stale=false`，否则保持 stale 让下个查询再触发一轮重载。

### R-3 2026-10-14【Medium】cleanupOrphanTags 删除孤儿标签后未接 InvalidateDict（接线不完整）
- **状态**：已修复（article_tagger.go 孤儿删除成功分支补 search.DefaultDict().InvalidateDict()，含注释）
- 位置：`backend-go/internal/tagmanagement/service/core/article_tagger.go:556`（orphan 标签批量 Delete）。
- 问题：本 change 接线 3 处失效钩子（tagger.go Save/Create 两处、hard_merge.go 一处），但运行时第 4 条标签删除路径——orphan 清理——删了 `topic_tags` 行却未调 `search.DefaultDict().InvalidateDict()`。被删标签的 label 残留切词候选（无限期，直到其它标签写入）。`repository.DeleteTag` 无调用方（非用户路径），当前运行时删除点仅 hard_merge 与 orphan 清理两处，后者漏了。
- 影响：词典陈旧方向是「多切词」（多一轮 LIKE），不是漏召回；量级为孤儿标签数。属一致性缺口而非故障。
- 建议：orphan 清理成功分支后补一行 `search.DefaultDict().InvalidateDict()`（与 hard_merge 同款「删除后失效」）。

### R-4 2026-10-14【Medium】列表端点 L3 臂 `similarity() >= ?` 显式谓词不可走 GIN 索引，OR 组合存在整查询 seqscan 退化风险
- **状态**：已驳回（真库实测留档：2026-10-14 `GET /api/articles?search=特斯拉` 531ms/246ms 两次实测〔L3 title-only 后〕，title 相似度全表算对 38K 行的代价被短 title（无 TOAST）控住，秒级退化未复现；持续观测交日常使用，超 1s 再换 GUC+% 事务方案）
- 位置：`backend-go/internal/reader/search/list_filter.go` L3 分支（`similarity(articles.title, ?) >= ?`）。
- 问题：分层路径（/api/search 的 `mergeTrigramLayer`）用 `%` 算子 + 事务内 `set_config` 走 gin_trgm_ops 索引（实测 ~95ms）；list_filter 因「无法包事务」改用显式 `similarity()` 谓词——该形态 PG 无法用 GIN 索引，必须逐行计算。而它是无条件附加的 OR 臂：PG 的 BitmapOr 要求全部臂可索引，一臂不可索引 → 整条 WHERE 退化为 seqscan。列表端点默认 active-only（~2K 行）可部分先过滤，但 38K 行全表扫描 + 每行 trigram 计算 + 3 组 LIKE 在树莓派上可能把 `GET /api/articles?search=` 从原 tsvector GIN 的毫秒级拉到秒级（proposal 实测裸 LIKE 全表 ~3s），且 count 查询同 filter 双倍。tasks 6.5 只核对 /api/search 延迟，列表端点无实测项。
- 建议：① 真库实测 `GET /api/articles?search=特斯拉` 延迟（含 count）留档；② 若秒级成立，考虑 L3 臂改为 `articles.title % ? AND similarity(articles.title, ?) >= ?` 双谓词（`%` 按 PG 默认 GUC 0.3 走索引圈候选，显式谓词守住 0.10 语义；代价是 [0.10,0.3) 区间在列表端点漏召回——列表与 /api/search 口径将不完全一致，需权衡）或干脆列表端点去 L3 臂（L0-L2 已覆盖 proposal 实测的 67/67 中文命中场景）。

### R-5 2026-10-14【Medium】双命中文章的分区归属：主路命中的文章因叠加 expand 命中被整卡挪进扩展区
- **状态**：已修复（search.vue 分区判据改 isExpandOnlyHit：hitReasons 含 expand 且不含 keyword/semantic 才进扩展区；双命中卡留主区双 chip 展示；search.test.ts 补双命中归属用例）
- 位置：`front/app/pages/search.vue` `mainResults`/`expandResults` computed（`h.hitReasons.some(r => r.type === 'expand')` 二分）。
- 问题：后端 Fuse 的 mergeReason 会把同文章的 keyword/semantic/expand 命中聚合进同一 `hit_reasons`；前端以「是否含 expand 型 reason」分区，导致**同时命中关键词路与扩展路**的文章整卡进入「以下 N 条来自扩展搜索」区——用户输入词直接命中标题的文章从主结果区消失。且分区展示破坏了后端 RRF「排序上融合」的名次（扩展区整体脱离融合序）。spec「与主结果在排序上融合但在 UI 上区隔展示」未定义双归属文章的归属，当前实现是取了更反直觉的一侧。
- 建议：分区判据改为「含 keyword 或 semantic 型 reason 即主区；**纯** expand 命中才进扩展区」（`mainResults = hits.filter(h => !h.hitReasons.some(r => r.type !== 'expand'))` 方向），并在 test-cases/组件测试补双归属断言。

### R-6 2026-10-14【Medium】ArticlePreviewModal 命中高亮属渲染后 DOM 注入，弹窗内响应式更新会静默丢弃 mark
- **状态**：已修复（watch 改按引用盯 selectedPreviewArticle + nextTick 重施；另加 bodyRef MutationObserver 兜底不经 props 的内部重渲染〔v-html 重设〕，disconnect 防自触发、条款为空不挂观察者〔tags 域零开销〕；补同 id 引用替换与内部重渲染两条用例）
- 位置：`front/app/features/tags/components/ArticlePreviewModal.vue` `applyHighlights` + `front/app/pages/search.vue` `handleFavorite`。
- 问题：高亮通过直接替换 Vue 管辖子树内的 Text 节点实现（`replaceChild(fragment, textNode)`）。弹窗打开期间任何触发 `ArticleContentView` 重渲染的响应式更新都会让 Vue 按 vnode patch 覆盖/孤儿化注入的 mark：具体触发链——modal 内点收藏 → `handleFavorite` 里 `selectedPreviewArticle.value = { ...selectedPreviewArticle.value, favorite: next }`（引用替换，id 不变，高亮 watch 源 `[visible, id, terms]` 不重触发）→ 子树重渲染 → 高亮消失；且后续对已脱离 DOM 的原 Text 节点的 patch 不可见。不 crash、tags 域（不传 highlightTerms）零影响，属短生命周期 UX 缺陷。
- 建议（择一）：① favorite 等字段更新改为浅层赋值不动对象引用（`selectedPreviewArticle.value.favorite = next`，需确认 ArticleContentView 不依赖引用变化）；② watch 源加入 `loadingPreviewArticle` 与 selectedPreviewArticle 引用（`activeTerms` 之外补 article 引用），重渲染后重跑 `applyHighlights`；③ 在 ArticlePreviewModal 内对 mark 注入区包一层非响应式容器隔离 patch 域。

### R-7 2026-10-14【Medium】semantic.go 白名单注释与 router.go 实际登记矛盾（陈旧注释误导）
- **状态**：已修复（注释改为已登记事实，删「遗留给 2.6/2.8」行）
- 位置：`backend-go/internal/reader/search/semantic.go` `semanticQueryOperation` 常量注释 vs `backend-go/internal/platform/airouter/router.go` `embeddingCacheOperations`。
- 问题：注释写「airouter 的 ai_embedding_cache 白名单暂不含本 operation——遗留给 tasks 2.6/2.8 接线时一行登记」，但 router.go 本 change 已实际登记 `article_search_query`（router.go:448）。注释是接线前的中间态残留，与代码事实相反，会误导后来者重复登记或误判缓存行为。小项，顺修即可。

---

## 整体结论

实现质量整体扎实：三路召回/降级链/参数化 SQL/窄投影/前端归一层/状态机竞态防护/迁移幂等与破坏性守卫均符合 spec 与红线，四条不变量 grep 全过。核心风险集中在**增强层的时序与超时边界**：expand 路的 600s 挂起上限（R-1，High，一行超时可修）需在归档前处置；R-2/R-3 为词典失效语义的边角缺口；R-4 需一次真库实测数据收口；R-5/R-6 为前端交互语义边角；R-7 顺修。无阻塞架构问题，全部处置后可进入归档门禁。
