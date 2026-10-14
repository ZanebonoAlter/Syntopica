## Purpose

站内文章混合搜索能力：关键词（词典切词+整串+trigram 模糊）与语义（标签向量）双路召回、RRF 融合排序、可选 LLM 查询扩展；覆盖含归档在内的全部文章；提供独立搜索页与文章预览弹窗交互；统一修复 articles 列表端点 search 参数对中文的失效。

## ADDED Requirements

### Requirement: 混合搜索聚合端点

`GET /api/search` SHALL 接收查询参数 `q`、`archived`（include|active-only，默认 include）、`expand`（off|on，默认 off）、`page`/`per_page`，执行关键词路径与语义路径召回，按 RRF（k=60）融合排序后返回分页结果；每个结果项 MUST 携带命中解释 `hit_reasons`（type: keyword|semantic|expand，含命中文本与分数）；响应 MUST 携带 `semantic` 块（命中标签列表或 `skipped: true`）。首屏延迟目标 P50 < 1s（语义路径并行执行，不阻塞关键词结果）。

#### Scenario: 关键词路径命中并返回解释
- **WHEN** 客户端请求 `GET /api/search?q=国漫崛起`
- **THEN** 返回结果中标题含「国漫」的文章，其 `hit_reasons` 含 type=keyword 项，且不依赖语义路径可用性

#### Scenario: 语义路径命中返回标签与分数
- **WHEN** 查询命中语义路径（query embedding 与标签向量 cosine 达标）
- **THEN** 结果项 `hit_reasons` 含 type=semantic 项（标签名与 cosine 分数），响应 `semantic` 块列出命中标签

#### Scenario: 非法分页参数
- **WHEN** 客户端请求 `per_page=0` 或 `page=-1`
- **THEN** 返回 400 错误，不执行搜索

### Requirement: 查询预处理四层管线

搜索服务 SHALL 按以下顺序对查询串做字面预处理，各层独立产生召回：L0 空格显式拆分（多词 AND）；L1 标签概念词典（3~8 字 `topic_tags.label` 全集，内存缓存，随标签系统自动进化）正向最大匹配切词，切出词逐词 AND LIKE（title/description）；L2 整串子串 LIKE；L3 trigram 模糊匹配（阈值 `search_trigram_threshold`，默认 0.10，`ai_settings` 可配，非法值回退默认并记 warn）。查询串为空、纯空白或剥离全半角空白后无有效字符时 MUST 拒绝执行（400）并保留前端输入。

#### Scenario: 词典词重组查询命中
- **WHEN** 用户输入「科技股AI泡沫」而文章标题为「AI 泡沫下的科技股定价逻辑」（词序重组）
- **THEN** L1 切出词典词后逐词命中该文章，不出现在 L2 整串结果中的遗漏

#### Scenario: 连续短语的 trigram 兜底
- **WHEN** 用户输入「特斯拉电池自燃」且标题为「特斯拉Model Y地下车库自燃」（无词典词、不连续）
- **THEN** L3 trigram 相似度 ≥ 阈值时该文章被召回，hit_reasons 携带相似度分数

#### Scenario: 空与纯空白查询拒绝
- **WHEN** `q` 为空串、纯空格或全角空格
- **THEN** 返回 400，响应不产生任何召回动作

#### Scenario: 特殊字符与超长查询
- **WHEN** `q` 含 LIKE 通配符（% _ \\）或长度 > 200 字符
- **THEN** 通配符被转义按字面匹配；超长被截断到 200 字符并记 warn，不报错

### Requirement: 语义召回约束与降级

语义路径 SHALL 将查询串 embedding 后与 `topic_tag_embeddings`（embedding_type=identity）计算 cosine，按双重约束命中标签：cosine ≥ `search_semantic_min_cosine`（默认 0.55）且排序后取前 `search_semantic_top_n`（默认 8）；经 `article_topic_tags` 召回文章，文章语义分取其命中标签的 max cosine。embedding 服务不可用或超时（上限 3s）时 SHALL 静默跳过语义路径：不报错、不阻塞，响应标记 `semantic.skipped=true`，前端提示降级。LLM 扩展（expand=on）SHALL 将查询串交 LLM 产出 5~8 个扩展词，补跑 L1/L2 字面召回，结果区隔为扩展结果（type=expand）；LLM 失败时主结果正常返回，仅扩展区缺失并提示一次。

#### Scenario: 双重约束边界
- **WHEN** 某标签 cosine 0.57 且排名第 9（top_n=8）
- **THEN** 该标签不参与召回（排名越界）；cosine 0.50 的标签无论排名均不参与（分数越界）

#### Scenario: embedding 不可用降级
- **WHEN** airouter embedding 调用失败或超时
- **THEN** 返回纯关键词结果，`semantic.skipped=true`，HTTP 200，无错误弹出

#### Scenario: LLM 扩展结果区隔
- **WHEN** expand=on 且 LLM 返回扩展词
- **THEN** 扩展词命中的文章携带 type=expand 的 hit_reasons，与主结果在排序上融合但在 UI 上区隔展示

### Requirement: 归档文章默认含入搜索

搜索 SHALL 默认覆盖全部文章（含 archived=true，其 title/description/content 均保留）；`archived=active-only` 时 SHALL 仅返回未归档文章。归档文章的结果项 MUST 携带归档标记供前端渲染角标。此行为与 reader 列表端点的归档排除（article-retention 契约）互不影响。

#### Scenario: 默认含归档
- **WHEN** 不带 archived 参数搜索，某归档文章标题命中
- **THEN** 该文章出现在结果中并带 `archived: true` 标记

#### Scenario: 仅活跃过滤
- **WHEN** `archived=active-only` 搜索同一关键词
- **THEN** 归档文章不出现

### Requirement: 搜索结果预览弹窗交互

搜索页结果卡片点击 SHALL 打开文章预览弹窗（复用 `ArticlePreviewModal` 的 AppDialog 90vw/85vh 高度链与文本/内嵌网页双模式契约），MUST NOT 路由跳转；预览正文 MUST 保留命中词高亮；弹窗内「阅读全文」SHALL 进入现有阅读路径；关闭（Esc/遮罩/关闭钮）SHALL 返回列表并保留查询与滚动位置。

#### Scenario: 点击卡片打开预览
- **WHEN** 用户点击结果卡片
- **THEN** 预览弹窗打开，正文命中词高亮，URL 不变化

#### Scenario: 阅读全文跳转
- **WHEN** 用户在预览弹窗点击「阅读全文」
- **THEN** 跳转该文章现有阅读路径

#### Scenario: 关闭返回原位
- **WHEN** 用户按 Esc 关闭预览弹窗
- **THEN** 返回结果列表，查询词与滚动位置保持不变

### Requirement: 搜索页入口与状态

侧边栏「发现订阅源」之后 SHALL 新增「文章搜索」入口，指向新页面 `/search`；页面 SHALL 使用 contained 布局模式（≤1120px 居中），editorial/dark 双主题零特判；SHALL 呈现完整状态矩阵：初始空态（未搜索引导）、加载骨架屏、无结果（含「开启 LLM 扩展搜索再试」引导，一键开启并重查）、错误（重试按钮，保留输入）、语义降级提示条（不阻塞）。LLM 扩展开关默认关闭。

#### Scenario: 无结果引导扩展
- **WHEN** 三路召回均零结果且 expand=off
- **THEN** 展示空态与扩展引导按钮；点击后开启扩展并自动重查

#### Scenario: 语义降级提示
- **WHEN** 响应含 `semantic.skipped=true`
- **THEN** 页面顶部显示降级提示条，关键词结果正常展示，无错误弹窗

#### Scenario: 双主题渲染
- **WHEN** 用户在 editorial 与 dark 主题间切换
- **THEN** 搜索页与结果卡片样式随语义 token 切换，无未适配区块

### Requirement: articles 列表端点 search 参数修复

`GET /api/articles?search=` 的匹配路径 SHALL 从 tsvector（`simple` 分词器，对中文失效）切换为与搜索服务一致的 trigram/子串匹配，中文关键词 MUST 不再系统性漏召回（原实测漏 97%）；归档排除行为保持 article-retention 契约不变（默认不含归档）。

#### Scenario: 中文关键词不再漏召回
- **WHEN** `GET /api/articles?search=特斯拉`
- **THEN** 命中数与 `title LIKE '%特斯拉%'` 在同一活跃集上口径一致，不再只剩整块 token 命中的少数结果

#### Scenario: 归档排除保持
- **WHEN** 列表端点带 search 查询
- **THEN** 归档文章仍按现有契约默认排除，行为与修复前一致
