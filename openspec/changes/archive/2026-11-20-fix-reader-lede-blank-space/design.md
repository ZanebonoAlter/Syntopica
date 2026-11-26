## Context

现状三条事实（详见 proposal.md - Why）：

1. `articles.description` 存的是源站原文，部分源是整篇文章的 HTML（智源 2148/2148 篇 description 与 content 逐字相同，均值 66 KB，最大 864 KB）。入库路径唯一：`reader/service/rss_parser.go` 解析 → `reader/service/feed_service.go:buildArticleFromEntry` 组装 `models.Article` 落库。
2. 阅读页导语段用 `v-html` 直出 `description`（`front/app/features/articles/components/ArticleContentPreviewPanel.vue`），无任何清洗；智源文章的公众号 HTML 根节点带 `opacity: 0` → 内容不可见却占 8046 px。
3. 列表 `excerpt` 已有成熟清洗逻辑（`reader/handler/article_excerpt.go:stripExcerptMarkup` + `decodeHTMLEntities`，≤200 字符），但它只服务列表投影，详情接口的 `description` 完全没走这套口径。

约束：

- `articles.content` 是正文源切换与 Firecrawl 兜底链路的依赖，不可随意改写（`reader/service/fallback_crawler.go` 与 `dataenrichment/service/signal_sources.go` 都取 `content`，后者明确不取 `description`）。
- 前端不引入新依赖是项目既有偏好（`front/package.json` 目前只有 `marked` 一个渲染相关依赖）。
- `docs/reference/flow/reading.md` 约束 #7、#9 锁死了阅读页版式与列表窄投影契约，本 change 不得破坏。

## Goals / Non-Goals

**Goals:**

- 摘要字段在入库时即为 ≤300 字符纯文本，从源头消除"整篇 HTML 冒充摘要"。
- 渲染路径自带护栏：无论数据是否已回填，第三方 HTML 都不会未经清洗进入 DOM。
- 摘要清洗口径与列表 `excerpt` 同源同实现，避免两套正则漂移。
- 存量数据可一次性、幂等、可审计地回填，且回填不触碰 `content`。

**Non-Goals:**

- 不改变 `articles.content` 的内容与语义。
- 不改 `shouldShowArticleDescription` 的 guard 判定语义（清洗后摘要若与正文开头逐字重复而不显示导语，是既有契约的正确行为）。
- 不动 AI 整理稿（`ai_content_summary`）与 Firecrawl 抓取链、降级链。
- 不改 API 路径、响应字段集与列表投影字段集。

## Decisions

> D10–D13 为 review 后追加的决策（编号接续 D1–D9，按追加位置排列）。

### D1. 清洗口径落在 `internal/platform/htmltext` 单一实现，excerpt 清洗器上提复用

- **结论**：新建 `backend-go/internal/platform/htmltext` 包，提供 `PlainSummary(raw string, maxRunes int) string`（去 script/style → 剔除隐藏子树 → 去标签 → 还原实体 → 折叠空白 → 截断加省略号）与 `SanitizeHTML(raw string) string`（保守清洗：去 script/style、`on*` 事件属性、**视觉隐藏剥声明 / 语义不展示移除子树**，保留其余结构）；把 `reader/handler/article_excerpt.go` 现有的 `stripExcerptMarkup` / `decodeHTMLEntities` 逻辑上提进该包，handler 改为调用它。
- **落点**：backend-go/internal/reader/handler/article_excerpt.go
- **取舍**：不新建 `reader/internal/htmltext` 私有包——回填命令（`cmd/`）与 `reader/handler`、`reader/service` 三处都要用，platform 层是三者共同的下游；不复制两套正则——两套口径必然漂移（列表与详情对同一篇文章给出不同导语）。代价：`article_excerpt.go` 的既有单测要跟着调整调用方式（断言不变）。

### D2. 摘要清洗落在入库点 `buildArticleFromEntry`，不在解析层

- **结论**：在 `reader/service/feed_service.go:buildArticleFromEntry` 组装 `models.Article` 时，对 `entry.Description` 施加 `htmltext.PlainSummary(entry.Description, 300)`；`rss_parser.go` 保持纯解析（仍原样透传条目 description）。
- **落点**：backend-go/internal/reader/service/feed_service.go
- **取舍**：不在 `rss_parser.go` 清洗——解析层应保持"忠实反映源"，语义收敛是入库层的职责，且解析层被其它调用方复用（如 feed 预览）；不在 handler 层——handler 只处理已入库数据，管不到新文章。代价：`ParsedEntry.Description` 仍是原始 HTML，调用方需知其为未清洗值（函数注释写明）。

### D3. 摘要上限 300 字符，列表 `excerpt` 上限 200 字符不变

- **结论**：`description` 清洗后长度上限 300 字符；`excerpt` 维持既有 ≤200 字符契约。两者共用同一清洗实现，仅长度参数不同。
- **落点**：openspec/specs/article-list-projection/spec.md
- **取舍**：不把两者统一成同一个数——`excerpt` 有"列表响应 <100 KB"的传输上限契约（spec `article-list-projection`），详情导语是完整摘要场景，300 更合适；不把两者统一成 300——会破坏列表窄投影的既有契约与 spec。

### D4. 前端清洗用浏览器原生 `DOMParser`，不引入 DOMPurify

- **结论**：新增 `front/app/utils/htmlSanitize.ts`，导出 `sanitizeHtml(html: string): string`（保守清洗，保留可见结构）与 `toPlainText(html: string, maxRunes?: number): string`（去标签取纯文本 + 截断）；实现基于 `DOMParser` + 元素遍历（移除 script/style、移除 `on*` 属性、移除隐藏元素与 `style` 中的隐藏声明）；另需移除可执行嵌入元素（iframe/object/embed/form/applet/base/frame/frameset）与 `srcdoc` 属性、过滤 `javascript:`/`vbscript:`/`data:` 危险 URL 方案。
- **落点**：4.1
- **取舍**：不引入 DOMPurify——项目前端依赖极少（现只有 `marked`），加一个运行时依赖需要评估构建体积与供应链；不用正则剥标签——`<img onerror>` 这类嵌套与属性结构用正则无法可靠处理（这正是本 bug 的成因形态）。代价：需要 `DOMParser`（浏览器环境可用；SSR/Node 测试环境需 stub，Vitest 默认 jsdom 环境可用）。

### D5. 导语段改纯文本渲染，前端清洗独立于后端回填生效

- **结论**：`ArticleContentPreviewPanel.vue` 的导语段由 `v-html="article.description"` 改为 `v-text="toPlainText(article.description, 300)"`（`article.description` 为空时回退 `article.excerpt`，与既有取值优先级一致）；清洗在渲染路径生效，不依赖数据是否已回填。
- **落点**：front/app/features/articles/components/ArticleContentPreviewPanel.vue
- **取舍**：只改后端不动前端——存量 276 MB 未回填前，导语仍是整篇 HTML 直出，空白照旧；只改前端不动后端——DB 体积（276 MB）与详情接口传输体积不解决，且列表/详情两处口径仍会漂移。代价：清洗在两端各做一次（后端为权威口径、前端为护栏），需靠单测保证两者对同一输入产出一致文本。

### D10. 清洗顺序统一为「先解实体 → 再解析」，并用跨实现共享样本锚定

- **结论**：纯文本清洗路径（后端 `htmltext.PlainText` / 前端 `toPlainText`）统一为「**先还原 HTML 实体 → 再按 HTML 解析 → 取文本 → 折叠空白**」。该顺序保证清洗幂等（输出里残留的尖括号不会在二次清洗时被当标签吞掉），代价是源里写成转义形式的标记（`&lt;b&gt;`）会被当标签丢弃——刻意接受的取舍。结构清洗路径（后端 `SanitizeHTML` / 前端 `sanitizeHtml`）解析**原始串**（不解实体），两侧同口径，但两者是**各自独立的实现**：前端版是渲染路径护栏（生产路径），后端版是未来接线点（本次无生产调用点）。两侧 MUST 用同一组固定样本（智源样本 / 实体样本 / 混合可见性样本 / 纯文本样本 / display:none 样本）断言**逐字相同的输出**，作为防漂移锚。
- **落点**：backend-go/internal/platform/htmltext/htmltext_test.go
- **取舍**：不用浏览器语义（先解析再解码）——它不幂等（`&lt;b&gt;` 残留的 `<b>` 会在二次清洗时被吞掉），回填就会二次改写数据；两端共用一份实现（Go 供后端、TS 供前端）不可能——运行时不同语言，只能靠共享样本锚定一致性。

### D11. 刷新判重比较清洗后的摘要 + 未改写的正文源字段

- **结论**：`feed_service.go:refreshExistingArticle` 的「内容未变」判定 MUST 改为比较入库同口径的值：先 `fresh := s.buildArticleFromEntry(feed, entry)`，再判 `existing.Title == fresh.Title && existing.Description == fresh.Description && existing.Content == fresh.Content`。**必须带 `content` 比较**——清洗后的摘要只剩 ≤300 字符，正文变化无法再从摘要检出。
- **落点**：backend-go/internal/reader/service/feed_service.go
- **取舍**：直接比较 `entry.Description`（现状）——库内存的是清洗值、源站是原文，对「摘要即整篇 HTML」的源永远判为「已变」→ 每轮刷新都重置状态、删标签、重跑 Firecrawl + LLM + 打标链（违反 `rss-article-dedup` 主 spec 与 content-enrichment 约束 #11）；只比较清洗后的摘要（不加 content）——正文变化会被漏掉，文章内容陈旧。代价：正文源字段参与比较后，源站 HTML 的任何字节变化都会触发一次更新（与修复前「description==content」时的触发面等价）。

### D12. 整理稿兜底改用正文源字段的文本投影

- **结论**：`content_completion_service.go` 的整理稿降级路径（Firecrawl 抓取失败时）MUST 改用**正文源字段**（`articles.content`，源站原文、未清洗）的纯文本投影，而非清洗后可能为空或过短的 `description`。投影口径：去脚本/样式、去标签、还原实体、折叠空白，但**保留隐藏子树的文本**（被视觉藏起来的正文对「总结正文」这个用途是有效内容，隐藏与否是源站样式问题）。
- **落点**：backend-go/internal/reader/service/content_completion_service.go
- **取舍**：继续用 `description`——智源形态整篇位于 `opacity:0` 根节点内，摘要清洗后为空串 → Firecrawl 一失败文章就永久拿不到整理稿（破坏了 content-enrichment 的降级链设计意图，且 design 的 Non-Goals 明写「不动降级链」）；用 `content` 原文直喂 LLM——把整篇 HTML（含标签与样式噪音）喂给模型，成本与质量都差。代价：`htmltext` 多一个投影函数（`PlainTextAll`），「隐藏内容不进摘要」与「隐藏内容可进整理稿」两种口径并存，MUST 在 spec 与代码注释里写清各自的用途。

### D13. 回填候选按「真实标签」判定，不按「含尖括号」判定

- **结论**：回填命令的候选集 MUST 与清洗器的真实判定对齐：含真实标签标记（`<[a-zA-Z]…>`）的行被清洗；**不含真实标签标记的行 MUST NOT 被改写**（避免把正常纯文本摘要里的尖括号当标记清理掉）；超长纯文本行按长度上限截断以对齐「摘要 ≤300」不变量。实测本地库：含 `<` 的 63303 行**全部**含真实标签（无误伤行）；不含 `<` 但超 300 字符的纯文本行仅 2 行。
- **落点**：4.1
- **取舍**：沿用「含 `<` 即候选」——口径与清洗器真实判定不一致，清洗器一旦改判定就会误伤正常纯文本（本次实测无受害行，但口径必须对齐）；候选含超长纯文本——为对齐长度不变量，这些行按上限截断（仅 2 行，代价可忽略）。

### D6. 原文源做保守清洗（两类隐藏分开处理），不改成纯文本

- **结论**：`useArticleContentView.ts` 的 `displayContent` 在 `activeContentSource === 'original'` 分支对 `content` 施加 `sanitizeHtml` 后再渲染，保留 `<p>`/`<img>`/标题等结构与大部分内联排版。隐藏处理**分两类**：① **视觉隐藏声明**（`visibility:hidden` / `opacity:0` / `font-size:0`）→ **剥离声明、保留元素与内容**（内容有效只是被藏起来，剥声明后重新可见）；② **语义不展示**（`display:none` / `hidden` 属性）→ **移除元素连同子树**（本就不展示，且此类块常藏 SEO 关键词堆砌）。脚本、样式、事件属性一律移除。
- **落点**：front/app/features/articles/composables/useArticleContentView.ts
- **取舍**：原文源也转纯文本——会丢掉图片与排版，阅读体验退化到"纯文字版"，与"原文"源的语义相悖；两类隐藏一律移除子树——智源整篇正文被公众号根 div 的 `opacity:0` 包裹，移除后原文源正文变空（本地库统计：无 firecrawl 正文且 content 含隐藏声明的 29 篇会变空，智源 feed 1 内 48 篇），用户点「原文」看到空页面，比空白更糟；两类隐藏一律保留——SEO 隐藏关键词块会跟着显示出来。代价：视觉隐藏与语义不展示靠 style 属性文本匹配判定，边角样式（如 `opacity:0.01`）可能判不准，但这些样式本身就在边缘地带。

### D7. 存量回填做成独立 CLI 命令，默认 dry-run

- **结论**：新增 `backend-go/cmd/clean-article-descriptions`：扫描 `articles` 中 `description` 含 `<` 的行，按 `htmltext.PlainSummary` 清洗并 UPDATE；默认只统计不写库，`--apply` 才写入；分批提交（每批 N 行一个事务），输出受影响行数与释放体积；可重复执行（已清洗的行不再被判定为待清洗）。
- **落点**：5.1
- **取舍**：做成 DB migration——migration 是 schema 变更语义，数据清洗与之不匹配，且 migration 无法"默认不写"；做成 HTTP 端点——需要鉴权与前端入口，单用户系统里 CLI 更简单、可审计、可随时重跑。代价：需要人工执行一次（不在自动路径上），因此**前端护栏必须先于回填上线**，保证回填未执行时用户也不受影响。

### D8. `articles.content` 不动

- **结论**：本 change 不改写 `content` 字段的存量内容与入库语义；回填命令只处理 `description`。
- **落点**：Scenario: 正文源字段不被改写
- **取舍**：一并清洗 `content`——`content` 是正文源切换与 Firecrawl 兜底链路的数据源（`fallback_crawler.go`、`dataenrichment/service/signal_sources.go` 均取 `content`），改写会破坏兜底与信号提取；只清洗 `description`——智源的 136 MB 冗余（description 与 content 逐字相同）只消除一半，另一半仍需后续单独决策。

### D9. 不改 `shouldShowArticleDescription` 的 guard 语义

- **结论**：导语是否渲染仍由 `front/app/utils/articleContentGuards.ts:shouldShowArticleDescription` 判定（归一化后与正文重复则不显示）；本 change 只改"渲染什么"与"怎么渲染"，不改"渲不渲染"。
- **落点**：front/app/utils/articleContentGuards.ts（该文件保持不变——D9 的决策即「不动它」，此落点用于锁定「保持不变」的对象）
- **取舍**：不改 guard——清洗后摘要（正文开头 300 字）与 firecrawl 正文若逐字重复，guard 判定重复 → 导语不显示，这是既有契约（"导语与正文重复就不渲染"）的正确行为，不应另立规则；改 guard（让摘要总显示）——会与 spec `reading-article-pane` 的既有 Scenario 冲突并制造首帧导语闪现。

## Risks / Trade-offs

- [摘要清洗把「Firecrawl 失败时用 RSS description 降级生成整理稿」的输入源改空] → 降级路径改用正文源字段的文本投影（D12），保留「隐藏子树不进摘要」与「隐藏子树可进整理稿」两种口径并存。
- [清洗口径在 Go 与 TS 两套实现间漂移] → 统一为「先解实体 → 再解析」的顺序（D10），并用跨实现共享样本锚定逐字相同的输出。
- [智源文章的摘要就是正文开头 300 字，guard 可能判定"与正文重复" → 导语不显示] → 这是既有契约的正确行为（不显示重复导语优于显示重复段落）；D5 已确保即便不显示也不会出现空白占位。
- [前端对 66 KB HTML 做 DOMParser 解析的回填前成本] → 单次解析在毫秒级；回填上线后详情接口只传 ≤300 字符，成本归零。
- [回填 UPDATE 13602 行在树莓派上可能长时间持锁] → 分批 + 每批独立事务 + 批间 sleep；命令默认 dry-run，可择机执行。
- [清洗后的摘要与 firecrawl 正文文字存在细微差异（标点/空白），可能影响 `excerptRepeatsBody` 的重复判定] → 既有 excerpt 契约（spec `article-list-projection`）不依赖描述与正文逐字相同，只依赖"归一化后相同或互相包含"，清洗后仍落在同一判定框架内；既有 excerpt 单测保持绿作为回归锚。
- [后端与前端各有一份清洗实现，可能漂移] → 以同一组固定样本（智源真实 HTML 片段）分别在 Go 与 TS 单测中断言相同输出，作为防漂移锚。

## Migration Plan

1. **上线后端**（`htmltext` 包 + 入库清洗 + 回填命令）：新入库文章的 `description` 即为 ≤300 字符纯文本。存量不动。
2. **上线前端**（`htmlSanitize.ts` + 导语改 `v-text` + 原文源清洗）：**存量文章即刻不再出现空白**（清洗在渲染路径生效），不依赖回填。
3. **择机回填**（可选，只为体积）：`cd backend-go && go run ./cmd/clean-article-descriptions` 先看统计，确认后加 `--apply` 执行；幂等，可重跑。
4. **回滚**：前端回滚即恢复旧显示（旧前端 `v-html` 渲染纯文本 description 与渲染 HTML 表现一致，不会出现新的破图或错版）；后端 `htmltext` 包与入库清洗可保留（纯文本摘要对两版前端都安全）；回填不可逆但无副作用（description 变短、语义不变），必要时可从备份恢复。

## Open Questions

无。回填执行时机（是否立刻跑、是否择机）由用户在部署后决定，不影响 spec、方案与任务拆分。
