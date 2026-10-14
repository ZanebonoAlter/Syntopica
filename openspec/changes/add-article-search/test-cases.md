# test-cases: add-article-search

本文件是实现与验收计划（用例先行），不是已运行测试报告。规格 7 Requirement / 20 Scenario 逐条落点见各故事主链路表；新测试文件在实现阶段创建；断言判据由主线程裁定（见文末白盒附加）。搜索端到端主链路必须走静态托管 :5100 的 agent-browser（任务 3.6）。

## 故事 S1: 我用中文关键词找回那篇文章（锚 Requirement: 混合搜索聚合端点、查询预处理四层管线、归档文章默认含入搜索）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 输入「国漫崛起」搜索 | 关键词路径命中并返回解释 | 结果含标题带「国漫」的文章，`hit_reasons` 有 type=keyword 项（命中文本+分数）；mock embedding 不可用时同样命中（不依赖语义路径） | HTTP/service | search/handler_test.go（sqlite 关键词路径）、repository_pg_test.go |
| 2 | 输入词重组「科技股AI泡沫」 | 词典词重组查询命中 | L1 切出词典词逐词命中标题「AI 泡沫下的科技股定价逻辑」（换序），不因 L2 整串不匹配而漏 | PG testcontainer | search/repository_pg_test.go |
| 3 | 输入连续短语「特斯拉电池自燃」 | 连续短语的 trigram 兜底 | L3 召回标题「特斯拉Model Y地下车库自燃」，hit_reasons 携带相似度分数 | PG testcontainer | search/repository_pg_test.go |
| 4 | 输入空串 / 纯空格 / 全角空格 | 空与纯空白查询拒绝 | 400，不产生任何召回动作（SQL 零执行） | HTTP | search/handler_test.go |
| 5 | 输入含 `%` `_` `\` 或 201+ 字符 | 特殊字符与超长查询 | 通配符转义按字面匹配；截断 200 字符 + warn，不报错 | 函数 | search/preprocess_test.go |
| 6 | 不带 archived 搜索命中归档文章 | 默认含归档 | 归档文章出现且带 `archived: true` 标记 | PG testcontainer | search/repository_pg_test.go |
| 7 | `archived=active-only` 同词重查 | 仅活跃过滤 | 归档文章不出现 | PG testcontainer | search/repository_pg_test.go |
| 8 | `per_page=0` / `page=-1` | 非法分页参数 | 400，不执行搜索 | HTTP | search/handler_test.go |

### 变体走查
| 组/条目 | 期望答案 | 层/落点 |
| --- | --- | --- |
| 输入：空串/纯空白（半角、全角 U+3000、tab）/纯通配符串/超长（201+） | 空/纯空白 400；纯通配符转义后按字面匹配（可零命中）；超长截断+warn 不报错 | preprocess_test.go、handler_test.go |
| 前置：空库（零文章）/无标签库（词典空） | 均返回 200 空结果不报错；词典空时 L1 自动跳过，L2/L3 照常 | repository_pg_test.go（空库种子）、service 单测 |
| 时间：无窗口语义 | ~~划除不适用~~：搜索为全库静态匹配，无时间窗参数与时效语义 | — |
| 幂等：同查询重复执行（10min 内 expand 重查） | LLM 扩展结果 TTL 缓存命中，零重复调用；关键词/语义结果确定（同库同查询两次结果一致） | expand_test.go |
| 可用性：误输入反馈/空态/错误态 | 400 就地提示保留输入；初始空态引导；API 非 2xx 错误条+重试保留输入 | useSearch.test.ts、pages/search.test.ts |

### 效果核对（问句④必做）
- 触发原因：中文漏召回源于 tsvector `simple` 分词器整句成块，是真实库数据分布问题，mock/单测测不出漏召回量。
- 方法：部署后真库只读核对：`curl --noproxy '*' 'http://localhost:5100/api/search?q=特斯拉' | jq '.results | length'`，同口径对比 `GET /api/articles?search=特斯拉` 修复前后命中数（修复前 tsvector 路径实测 2 篇；LIKE 基线 67 篇）。
- 量化结果：待部署后填，**不填假数字**；期望 search 端点 ≥50（任务 6.5 验收线）。
- 结论：待真实核对；若不达标先查词典覆盖率与 trigram 阈值分布，不默默调参交付。

## 故事 S2: 我搜概念也能发现相关文章（锚 Requirement: 语义召回约束与降级）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 概念词命中语义路径 | 语义路径命中返回标签与分数 | hit_reasons 含 type=semantic 项（标签名+cosine 分数）；响应 `semantic` 块列出命中标签 | service（mock Embed） | search/semantic_test.go、handler_test.go |
| 2 | 构造边界数据（cosine 0.57 排第 9 / cosine 0.50 排第 1） | 双重约束边界 | 排名越界与分数越界均不参与召回 | 函数（白盒） | semantic_test.go（边界值表见文末） |
| 3 | mock airouter Embed 失败/挂起 3s+ | embedding 不可用降级 | 纯关键词结果 + `semantic.skipped=true` + HTTP 200，无错误 | service | semantic_test.go（context 超时） |
| 4 | expand=on 且 LLM 正常 | LLM 扩展结果区隔 | 扩词补跑 L1/L2，命中文章带 type=expand hit_reasons；排序融合、UI 区隔展示 | service+组件 | expand_test.go、pages/search.test.ts |
| 5 | expand=on 且 LLM 失败 | （spec 同节）LLM 失败主结果正常 | 主结果 200 正常，扩展区缺失，提示一次 | service | expand_test.go（mock 失败分支） |

### 变体走查
| 组/条目 | 期望答案 | 层/落点 |
| --- | --- | --- |
| 输入：LLM 返回非法 JSON/空数组/词数 <5 或 >8/含重复词 | 解析容错：有效词 1~4 个仍执行扩展（有总比无好），>8 截前 8，去重；整体解析失败降级主结果+提示一次 | expand_test.go |
| 前置：topic_tag_embeddings 无 identity 行 / article_topic_tags 空表 | 语义零命中但不 skipped（块为空标签列表，区别于服务不可用） | semantic_test.go |
| 时间：3s 超时边界 | context 控制：超时即弃语义路，主结果不等 | semantic_test.go |
| 幂等：10min TTL 内同 query 重复 expand | 缓存命中，airouter chat 调用计数不变 | expand_test.go |
| 可用性：语义降级提示条 / 扩展失败 toast | 降级条不阻塞不弹窗；扩展失败提示一次不重复 | pages/search.test.ts |

### 效果核对
- 触发原因：语义召回质量依赖真实 embedding 与 42K 标签向量分布，mock 只能测管线与边界约束。
- 方法：部署后人工抽查概念词（如「电动车电池安全」），核对命中标签合理性与文章相关性；对照 hit_reasons 分数。
- 量化结果：待部署；记录 top 命中标签 cosine 分布（p50/p90）与语义路径命中占比。
- 结论：待真实核对；召回不足时的升级路径是 design Non-Goal 的影子边表，不在本 change 内调。

## 故事 S3: 搜索页好用——入口、状态与预览（锚 Requirement: 搜索结果预览弹窗交互、搜索页入口与状态）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 看侧边栏并点击入口 | 搜索页入口与状态 | 「发现订阅源」之后出现「文章搜索」入口（分隔线之前），点击跳 /search | 组件 | AppSidebarView.test.ts + agent-browser |
| 2 | 输入连续短语回车 | （spec 同节）状态矩阵 | 骨架屏 → 结果渲染：命中 chip（keyword/semantic/expand 三型）与归档角标存在 | 组件+E2E | pages/search.test.ts、ArticleCardView.test.ts、agent-browser 3.6 |
| 3 | 三路零结果且 expand=off | 无结果引导扩展 | 空态 + 「开启 LLM 扩展搜索再试」按钮；点击 → 开启扩展并自动重查 | 组件 | pages/search.test.ts、useSearch.test.ts |
| 4 | 响应 semantic.skipped=true | 语义降级提示 | 顶部黄色降级提示条，关键词结果正常展示，无错误弹窗 | 组件 | pages/search.test.ts |
| 5 | 点结果卡片 | 点击卡片打开预览 | 预览弹窗开（AppDialog 90vw/85vh 双模式契约），正文命中词高亮，URL 不变 | 组件+E2E | ArticlePreviewModal.test.ts、agent-browser 3.6 |
| 6 | 弹窗内点「阅读全文」 | 阅读全文跳转 | 进入现有阅读路径 | 组件+E2E | ArticlePreviewModal.test.ts、agent-browser 3.6 |
| 7 | Esc 关闭弹窗 | 关闭返回原位 | 返回列表，查询词与滚动位置保持 | 组件+E2E | pages/search.test.ts、agent-browser 3.6 |
| 8 | editorial/dark 切换 | 双主题渲染 | 全语义 token 切换，无未适配区块；chip 三色（info/success/warning）与归档角标正确 | 组件+人工 | pages/search.test.ts + 人工走查 6.6 |

### 变体走查
| 组/条目 | 期望答案 | 层/落点 |
| --- | --- | --- |
| 输入：长查询折行 / 多 chip 折行 / 窄屏 375 | 无横向溢出；过滤行折行堆叠 | pages/search.test.ts + 双视口截图（任务 4.2） |
| 前置：卡片无封面 | 降级 feed 图标，行高不变（既有行为回归） | ArticleCardView.test.ts（回归） |
| 时间：无窗口语义 | ~~划除不适用~~：无时效过滤 | — |
| 幂等：进行中重复提交 / 输入不变不重查 / 加载更多 offset 追加 | 按钮禁用+输入不变跳过；追加页不重置已有结果 | useSearch.test.ts |
| 可用性：错误重试保留输入 / 语义降级条可自动恢复 / 扩展分隔条区分主结果 | 错误条+重试；下次查询自动重试语义；扩展条带扩展词回显 | pages/search.test.ts |

### 效果核对
视觉与交互用 agent-browser 截图断言 + 双视口检查（1440×900 / 1920×1080，contained ≤1120 居中、无横向溢出），证据落 `ui-verification/report.md`（任务 4.2）；无模型质量依赖，不设语义质量核对项。

## 故事 S4: 老入口一并修好，旧路径清退干净（锚 Requirement: articles 列表端点 search 参数修复）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | `GET /api/articles?search=特斯拉`（活跃集） | 中文关键词不再漏召回 | 命中数与 `title LIKE '%特斯拉%'` 同一活跃集口径一致，不再只剩整块 token 命中 | HTTP+PG | article_handler_test.go（新增用例）、reader/handler PG 集成用例 |
| 2 | 列表 search 带归档数据 | 归档排除保持 | 归档默认排除，行为与修复前一致 | HTTP | article_handler_test.go（既有契约回归） |
| 3 | 迁移 A 执行 | （tasks 1.1） | `\d articles` 断言 title/description 两列 GIN trgm 索引存在；`show_trgm('特斯拉电池')` 返回多 trigram | PG testcontainer | database/pg_trgm_migration_test.go |
| 4 | 迁移 B 执行 | （tasks 1.2） | search_vector 列、articles_search_vector_trigger 触发器、旧索引均不存在 | PG testcontainer | database/search_vector_drop_migration_test.go |
| 5 | 归档超限触发 cleanup | （tasks 1.2） | feed_service 归档 updates 不再引用 search_vector，TestCleanup 通过 | service | feed_service_cleanup_test.go（调整后） |

### 变体走查
| 组/条目 | 期望答案 | 层/落点 |
| --- | --- | --- |
| 输入：非 PG（sqlite 测试库）分支 | LIKE 分支保持不变（切换只动 postgres 分支） | article_handler_test.go |
| 前置：迁移 B 后列不存在 | 归档 updates 无 search_vector 引用不报错 | feed_service_cleanup_test.go |
| 时间：n/a | ~~划除不适用~~：迁移一次性执行 | — |
| 幂等：迁移重跑 | IF NOT EXISTS / IF EXISTS 全幂等；重复执行无副作用 | 迁移测试 |
| 可用性：n/a | ~~划除不适用~~：无 UI 面 | — |

### 效果核对
真库 `articles?search=` 修复前后命中数对比（修复前 2 篇 → 期望与 LIKE 口径一致），与故事 S1 效果核对同批执行（任务 6.5）。

## 继承与调整（⓪ article-list-projection 旧测试资产处置）

`bash scripts/harness/test-assets.sh article-list-projection` 反查结果逐行处置：

| 资产 | 处置 |
| --- | --- |
| 主 spec article-list-projection 9 Scenario（字段投影契约：列表不带正文大字段、excerpt ≤200、per_page 超限语义） | **继承不动**：与 search 匹配行为替换正交；search 新端点结果项沿用同一投影契约（复用 articleListColumns + excerpt，不含 content/firecrawl_content/ai_content_summary） |
| article_handler_test.go 现有 8 个 Test 函数 | **继承不动**：均无 search 分支断言；新增中文 search 用例进同文件（任务 2.8） |
| feed_service_cleanup_test.go TestCleanupOldArticlesClearsDerivedData（sqlite stub search_vector 列断言 NULL） | **调整**：随迁移 B 移除 stub 列 setup 与 NULL 断言（保留 ReadingBehavior 清除断言），其余 4 个 Test 继承 |
| useAutoRefresh.test.ts / ArticleContentPreviewPanel.test.ts 相关映射行 | **继承不动**：与本 change 无交集 |

## 白盒附加（断言判据，主线程裁定）

### 阈值边界值表
| 参数 | 边界点 | 判据 |
| --- | --- | --- |
| `search_trigram_threshold`（默认 0.10） | 0.09 / 0.10 / 0.11 | similarity=0.10 的行：阈值 0.09、0.10 时召回，0.11 时不召回；配置非法（负数/非数字/>1/空串）回退 0.10 + warn 日志可断言。（实现期修订：L3 仅作用于 title 列，见 design D2 修订注） |
| `search_semantic_min_cosine`（默认 0.55） | 0.54 / 0.55 / 0.56 | cosine=0.55 的标签：阈值 0.54、0.55 时参与，0.56 时不参与 |
| `search_semantic_top_n`（默认 8） | 第 8 名 / 第 9 名 | 分数均达标时第 8 名参与、第 9 名不参与；分数达标排名越界 → 不召回（spec 双重约束） |
| 查询长度 | 199 / 200 / 201 runes | 201 截断为 200 + warn；200、199 不截断；rune 计数（中文 1 rune）非字节 |
| 词典词长（topic_tags.label） | 2 / 3 / 8 / 9 runes | 3~8 进词典；2、9 不进；含空白的 label 不进（词内无空白） |
| LLM 扩词数 | 0 / 4 / 5 / 8 / 9 | 有效词 1~4 仍执行扩展召回；5~8 全用；>8 截前 8；去重后 0 个 → 降级主结果+提示一次 |
| 语义超时 | 3s | context deadline：≥3s 即弃语义路（skipped=true），关键词结果不等待 |

### 命中层级加权排序断言（design D4）
| 对比组 | 判据 |
| --- | --- |
| 词典多词命中 vs 词典单词命中 | 同基础条件下多词（系数 0.85）排在单词（0.6）之前 |
| 整串命中 vs 词典多词 | 整串（1.0）> 词典多词（0.85） |
| title 命中 vs description 命中（同层级） | title（权重 1.0）> description（0.7） |
| trigram 层 | 无固定系数，用原始 similarity 作为层级分 × 字段权重 |
| 融合 | RRF k=60：`score = Σ 1/(60 + rank_i)`，关键词榜与语义榜各自排名后求和 |

### 词典切词白盒（design D2 正向最大匹配）
| 输入 | 判据 |
| --- | --- |
| 「人工智能大模型」（词典含两词） | 切出 [人工智能, 大模型] |
| 词典同时含「人工」「人工智能」 | 最长优先：匹配「人工智能」 |
| 空串 / 纯空白 | 空切（不产生 L1 词） |
| 纯英文 "AI agent" | L0 已按空格拆；词典无命中 → L1 空切 |
| 「XX人工智能YY」YY 非词典词 | 切出 [人工智能]，剩余片段交 L2/L3 |
| 词典加载 | 3~8 字 label 全集；标签写入后缓存失效（stale-while-revalidate：旧词立即返回 + 后台单飞重载，`EnsureFresh` 同步等待；实现期修订见 design D2） |

### 响应契约断言（handler）
- 结果项字段：沿用 article-list-projection 投影（camelCase）+ `archived: bool` + `hit_reasons: [{type: keyword|semantic|expand, text, score?}]`。
- `semantic` 块：`{skipped: true}` 或 `{skipped: false, tags: [{label, score}]}`。
- `q` 缺失/空/纯空白 → 400；`per_page=0`、`page<1`、`per_page` 超上限 → 400（不执行搜索）；`archived`/`expand` 非法枚举值 → 400 或回退默认（判据：与既有端点校验风格一致，回退默认 + 不报错，仅分页数值类硬 400——按 spec 只要求分页 400 与空 q 400）。
