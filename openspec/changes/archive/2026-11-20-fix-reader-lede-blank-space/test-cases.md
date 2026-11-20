# 用例设计 — fix-reader-lede-blank-space

> 测试单元 = 一个 Requirement 的用户故事。本文件把 spec Scenario 串成完整故事；断言判据由实现线程给出；复杂档附白盒分支表与边界值。
> 故事锚点：**用户打开一篇智源社区文章 → 标题下方是导语而不是 8046px 空白 → 正文首屏直接可读 → 列表导语与详情导语口径一致 → 存量数据可一次性回填。**
> 涉及能力：`article-description-sanitization`（新增，3 Requirements / 12 Scenarios）+ `reading-article-pane`（改写，2 Requirements / 9 Scenarios）。
> 线程分工：**后端** = `internal/platform/htmltext` + `internal/reader/handler` + `internal/reader/service` + `cmd/clean-article-descriptions`；**前端** = `front/app/utils/htmlSanitize.ts` + 导语段渲染 + 原文源清洗（本表前端行由前端线程落地）。

## 0. 继承与调整（⓪ 契约变更反查）

反查命令：`bash scripts/harness/test-assets.sh article-description-sanitization`（该 capability 是本次新增，主 specs 与 archive 均无 → 命令报「无匹配」）；回退跑 `bash scripts/harness/test-assets.sh article-list-projection`（命中 change `2026-09-24-slim-article-list-payload`，含 test-cases.md）。

| 旧 Scenario（capability） | 处置 | 旧测试资产 | 动作 |
|---|---|---|---|
| `article-list-projection` / Requirement「excerpt 承载列表层导语」/ Scenario「长 HTML 源被截断」 | **语义保留、调用方式变更**：清洗器上提到 `internal/platform/htmltext`，handler 改调共享实现；截断语义（≤200 rune、无省略号）不变 | `backend-go/internal/reader/handler/article_handler_test.go`（`TestExcerpt`） | 既有断言语义不动（逐条绿）；白盒分支迁到 `internal/platform/htmltext/htmltext_test.go` |
| `article-list-projection` / Requirement「excerpt 承载列表层导语」/ Scenario「空源返回空串」 | 语义保留 | 同上（`TestExcerpt` "empty source"） | 回归跑，不改 |
| `article-list-projection` / Requirement「excerpt 承载列表层导语」/ Scenario「与正文重复的导语不下发」 | 语义保留（去重口径随清洗器同步，去重判定本身不变） | 同上（`TestExcerpt` + `TestArticleListExcerptDedupe`） | 回归跑，不改 |
| `article-list-projection` / Requirement「excerpt 承载列表层导语」/ Scenario「正文兜底导语不下发」 | 语义保留 | 同上 | 回归跑，不改 |
| `article-list-projection` / Requirement「列表响应不携带正文类大字段」/ Scenario「列表响应体积上限」 | **受影响（正面）**：`description` 入库即纯文本，详情响应体积从 276 MB 级降到 MB 级 | `article_handler_test.go` | 回归跑 + 真库量化见 §4 |
| `reading-article-pane` / Requirement「简介导语段按实质内容呈现」/ Scenario「有实质内容」 | **语义保留、数据来源变更**：导语取值仍是详情 `description` 优先 / `excerpt` 回退；渲染方式由 `v-html` 改纯文本 | `front/app/features/articles/components/ArticleContentPreviewPanel.test.ts` | 前端线程改断言：节点无元素子节点、文本 ≤300 |
| `reading-article-pane` / Requirement「简介导语段按实质内容呈现」/ Scenario「无实质内容」 | 不变（guard 判定语义不动，design D9） | 同上 | 回归跑 |
| `reading-article-pane` / Requirement「简介导语段按实质内容呈现」/ Scenario「详情未就绪时的首帧导语」 | 语义保留（回退 `excerpt` 不变） | 同上 | 回归跑 |
| `reading-article-pane` / Requirement「简介导语段按实质内容呈现」/ Scenario「详情返回后以详情为准」 | 语义保留 | 同上 | 回归跑 |
| `reading-article-pane` / Requirement「手动操作与内容源切换收进更多菜单」/ Scenario「按能力显隐」「操作进行中」「双源切换」 | 不受影响（菜单行为不动） | `ArticleContentPreviewPanel.test.ts` / `useArticleContentView` 相关测试 | 回归跑 |
| `reader/handler/article_excerpt.go` 中 `stripExcerptMarkup` / `decodeHTMLEntities` 的白盒断言 | **按新契约调整**：实现上提到 `internal/platform/htmltext`，handler 只保留调用点 | `article_handler_test.go`（无直接白盒断言，走 `buildExcerpt` 间接覆盖） | 迁到 `htmltext_test.go`（§3 白盒分支表） |
| `reader/service` 入库链路断言（`ParsedEntry.Description` 原样入库） | **按新契约调整**：`buildArticleFromEntry` 对 description 施加 ≤300 纯文本；`Content` 仍原样 | `internal/reader/service/feed_service_unit_test.go` | 新增 `TestBuildArticleFromEntryCleansDescription`；旧用例不依赖 description 形态，无破坏 |

## 1. 主链路表（节拍）

| # | 步 / 动作 | 来源 Scenario | 期望 | 层 | 落点 | 线程 |
|---|---|---|---|---|---|---|
| 1 | RSS 刷新拉到一个条目，其 description 是整篇文章 HTML（智源形态：根节点 `style="visibility: visible; opacity: 0;"`，含内联样式与图片） | `article-description-sanitization` / Requirement「文章摘要入库即纯文本化」/ Scenario「整篇 HTML 型摘要入库为纯文本」 | 入库后 `articles.description` 为不含任何标签的纯文本，长度 ≤300 字符，超长处以「…」收尾 | Go service 单测 | `backend-go/internal/reader/service/feed_service_unit_test.go`（`TestBuildArticleFromEntryCleansDescription`） | 后端 |
| 2 | 同一条目的正文源字段 `content` | `article-description-sanitization` / Requirement「文章摘要入库即纯文本化」/ Scenario「正文源字段不被改写」 | `content` 与源站原文逐字相同（摘要清洗不得碰正文源） | Go service 单测 | 同上（`require.Equal(t, wholeArticleHTML, article.Content)`） | 后端 |
| 3 | 条目 description 的根节点声明 `opacity: 0` 包裹全文 | `article-description-sanitization` / Requirement「文章摘要入库即纯文本化」/ Scenario「隐藏节点内容不进入摘要」 | 该隐藏子树内容不进入摘要（摘要为空或只剩子树外可见文本） | Go 单测 | `backend-go/internal/platform/htmltext/htmltext_test.go`（`TestPlainText_RemovesHiddenSubtrees`）+ 上条 service 单测 | 后端 |
| 4 | 条目 description 是 ≤300 字符的普通纯文本 | `article-description-sanitization` / Requirement「文章摘要入库即纯文本化」/ Scenario「纯文本摘要不被改写」 | 入库摘要与源文本一致（仅空白折叠），不被截断或改写 | Go service 单测 | `feed_service_unit_test.go`（"plain text summary is preserved"） | 后端 |
| 5 | 对已入库摘要再跑一次同一套清洗 | `article-description-sanitization` / Requirement「文章摘要入库即纯文本化」/ Scenario「清洗幂等」 | 结果与已存值逐字相同（重复回填不产生二次改动） | Go 单测 | `htmltext_test.go`（`TestPlainSummary_Idempotent`）+ `cmd/clean-article-descriptions/main_test.go`（`TestRunSecondPassAffectsNothing`） | 后端 |
| 6 | 列表接口 `GET /api/articles` 返回导语 `excerpt` | `article-list-projection` / Requirement「excerpt 承载列表层导语」/ Scenario「长 HTML 源被截断」（既有回归）+ `article-description-sanitization` Requirement「文章摘要入库即纯文本化」/「与 excerpt 同源同口径」 | excerpt 与摘要同源清洗，仅上限不同（200 vs 300）；既有 `TestExcerpt` 断言全部绿 | Go handler 单测 | `backend-go/internal/reader/handler/article_handler_test.go`（`TestExcerpt` 回归，不改断言） | 后端 |
| 7 | 阅读页打开一篇智源文章（存量未回填数据） | `article-description-sanitization` / Requirement「第三方 HTML 渲染前必须经清洗」/ Scenario「摘要导语以纯文本呈现」+ `reading-article-pane` / Requirement「简介导语段按实质内容呈现」/ Scenario「HTML 型详情摘要不产生空白」 | 导语段呈现去标签后的纯文本（≤300 字符），MUST NOT 出现空白占位，MUST NOT 把该 HTML 作为标签注入 DOM | Vitest 组件 | `front/app/features/articles/components/ArticleContentView.test.ts`（新增断言：导语节点无元素子节点、无 `onerror`、文本 ≤300） | 前端 |
| 8 | 阅读页首屏 | `article-description-sanitization` / Requirement「第三方 HTML 渲染前必须经清洗」/ Scenario「不可见内容不占位」 | 不可见内容不保留占位空间，页面首屏直接可见正文 | 端到端（静态 :5100） | `ui-verification/report.md`（条目 A-2） | 前端 |
| 9 | 详情接口返回的 description 仍含 `<img onerror=...>` | `article-description-sanitization` / Requirement「第三方 HTML 渲染前必须经清洗」/ Scenario「事件属性不被执行」 | 清洗后不保留事件属性，渲染 DOM 不绑定处理器 | Vitest 组件 + Go 单测 | `front/.../ArticleContentView.test.ts`；后端 `htmltext_test.go`（`TestSanitizeHTML_KeepsStructureAndDropsDangerousParts`） | 前端 + 后端 |
| 10 | 用户切到「原始内容」内容源 | `reading-article-pane` / Requirement「手动操作与内容源切换收进更多菜单」/ Scenario「原始内容源渲染前清洗」+ `article-description-sanitization` / Requirement「第三方 HTML 渲染前必须经清洗」/ Scenario「正文源切换后正文可见」 | 渲染结果为清洗后的可见正文；隐藏节点不造成空白占位；事件属性不被绑定；保留可见正文结构 | Vitest composable | `front/app/features/articles/composables/useArticleContentView.ts` 相关测试 | 前端 |
| 11 | 不带写入开关跑回填命令 | `article-description-sanitization` / Requirement「存量摘要字段可一次性回填」/ Scenario「默认不写库」 | 数据库不被修改；命令输出待清洗行数与预计释放体积 | Go CLI（dry-run + DB 前后比对） | `cmd/clean-article-descriptions/main_test.go`（`TestRunDryRunDoesNotWrite`）+ §4 真库 dry-run 输出 | 后端 |
| 12 | 带写入开关跑回填命令 | `article-description-sanitization` / Requirement「存量摘要字段可一次性回填」/ Scenario「显式写入后摘要为纯文本」 | 含 HTML 的历史摘要被更新为清洗后的纯文本；正文源字段保持不变；只改 `description` 一个字段 | Go CLI | `main_test.go`（`TestRunApplyOnlyTouchesDescription`） | 后端 |
| 13 | 在已清洗数据上再次跑回填 | `article-description-sanitization` / Requirement「存量摘要字段可一次性回填」/ Scenario「重复执行无二次改动」 | 受影响行数为 0，数据保持不变 | Go CLI | `main_test.go`（`TestRunSecondPassAffectsNothing`） | 后端 |

**负向节拍（SHALL NOT）**：任何一步都不得出现「`articles.description` 仍存整篇第三方 HTML」「`articles.content` 被摘要清洗规则改写」「回填默认写库」——由步骤 1/2/11/12 的断言与 §4 的真库前后对比覆盖。

## 2. 变体走查

| 组 | 变体 | 答案 |
|---|---|---|
| 输入 | 空串 / 纯空白（含全角）/ 纯符号 / 单 token / 含实体 / 含转义标签 / 超长 / 隐藏子树 / script / style / 图片-only | 逐条期望见 §3 白盒分支表（清洗器分支表） |
| 前置 | 条目无 description / description 为空标签 / description 为隐藏根节点包裹全文 / description 与 content 同源 | 无 description → 摘要空串；空标签 → 空串；隐藏根节点 → 子树外文本或空串；与 content 同源 → 摘要纯文本，`content` 不变 |
| 幂等 | 重复执行清洗 / 重复执行回填 / 部分批次失败重跑 | 清洗幂等（§3）；回填二次运行受影响行数 0；批次事务独立，已提交批次不回滚 |
| 存量 | 未回填 / 回填中 / 已回填 | 未回填 → 后端清洗保证新数据干净，前端护栏保证渲染安全；回填中 → 前端已生效，不依赖回填；已回填 → 二次运行 0 变更 |
| 大小写/边界 | 摘要恰好 300 / 301 rune；excerpt 恰好 200 / 201 rune | 300 → 不截断；301 → 299 +「…」；excerpt 200 → 不截断，201 → 截 200（无省略号，既有契约） |

## 3. 白盒附加（复杂档）

### 3.1 清洗器分支表（`internal/platform/htmltext`）

| 分支 | 输入 | 期望 |
|---|---|---|
| 空输入 | `""` | `""` |
| 纯文本 | `"abc"` | `"abc"` |
| 纯空白 | `"  \t\n "` / `"　　"` | `""` |
| 纯符号 | `"，，，。"` | `"，，，。”`（摘要层保留；excerpt 层由「无字母数字→空串」契约清空） |
| 无标签文本含转义 | `"a&amp;b&lt;c"` | `"a&b<c"` |
| 实体还原 | `&nbsp; &amp; &lt; &gt; &quot; &#39; &#20013; &#x4E2D;` | 空格 / `&` / `<` / `>` / `"` / `'` / `中` / `中` |
| 单次解码 | `"a&amp;lt;b"` | `"a&lt;b"`（不二次解码成 `<`） |
| script 内容 | `"<script>alert(1)</script>text"` | `"text"` |
| style 内容 | `"<style>p{color:red}</style>ok"` | `"ok"` |
| 隐藏子树（4 种声明 + hidden 属性） | `display:none` / `visibility:hidden` / `opacity:0` / `font-size:0` / `hidden` | 子树及其后代文本全部剔除 |
| 隐藏子树嵌套 | `<div style="opacity:0"><p>深</p></div><p>可见</p>` | `"可见"` |
| script 内含标签 | `<div style="display:none"><script>x</script></div><p>可见</p>` | `"可见"` |
| 标签分隔 | `"<p>a</p><p>b</p>"` | `"a b"`（不是 `"ab"`） |
| 空白折叠 | `"  a \t\n b  "` | `"a b"` |
| rune 截断 | 501 个「导」 | 300 rune，尾部「…」 |
| 恰好上限 | 300 个「导」 | 原样，不加省略号 |
| 上限为 1 / 0 | maxRunes=1 / 0 | `"…"` / `""` |
| 幂等 | 上表全部输入各跑两次 | 两次结果逐字相同 |
| 真实样本（智源） | 根节点 `visibility: visible; opacity: 0;` 包裹全文 | 摘要不含 `<`、不含 `opacity`、不含子树文本、长度 ≤ 上限 |

### 3.2 `SanitizeHTML` 分支表

| 分支 | 输入 | 期望 |
|---|---|---|
| script / style 元素 | `<script>…</script><style>…</style>` | 元素与其内容都移除 |
| 事件属性 | `onerror="…"` / `onload="…"` | 属性移除，元素保留 |
| display:none 元素 | `<div style="display:none">广告</div>` | 元素与子树移除 |
| hidden 属性元素 | `<div hidden>…</div>` | 元素与子树移除 |
| 隐藏声明剥离 | `style="color:red;opacity:0"` | 只剩 `color:red`，元素与文本保留 |
| 其它内联样式 | `style="color:#333;font-size:16px"` | 原样保留 |
| 结构保留 | `<p>` / `<img src>` | 保留 |
| 真实样本（智源） | 根节点 `visibility: visible; opacity: 0;` 包裹全文 | 正文文本保留（「正文第一段」在输出里）、`opacity` 不在输出里（声明被剥离） |
| 幂等 | 清洗结果再清洗 | 逐字相同 |

### 3.3 入库点分支表（`buildArticleFromEntry`）

| 分支 | 条目 description | 期望 |
|---|---|---|
| 整篇 HTML | 智源样本 | 摘要 ≤300 rune 纯文本；`content` 逐字等于源站原文 |
| 长可见 HTML | `<p>` + 900×「导」 | 摘要 300 rune，尾部「…」 |
| 普通纯文本 | `"普通中文摘要"` | 原样 |
| 空 | `""` | `""` |

## 4. 效果核对（真库量化）

- **触发原因**：`description` 体积与内容形态依赖真库（单条从数百字节到 864 KB 不等），fixture 无法代表。
- **方法**：只读回填 dry-run（`go run ./cmd/clean-article-descriptions`），统计候选行数、受影响行数、清洗前后字节数；对照 = `psql` 只读计数。
- **实测基线（2026-11-20，本机真库；库在自动刷新，行数随新文章微增）**：
  - `articles` 总行数 **74303**；`description` 含 `<` 的行 **63300**（其中含真实标签的同为 63300）。
  - 清洗前 `description` 合计 **381.0 MiB**（按字节；PG `length()` 按字符计为 276 MB，与 proposal 的口径一致）。
  - 清洗后预计 **32.3 MiB**，**预计释放 348.8 MiB**。
  - dry-run 输出 `updated=0`，dry-run 后复查 `description LIKE '%<%'` 仍为 63300 → 未写库 ✓。
- **达标判据**：`--apply` 后 `description LIKE '%<%'` 的行数显著下降、二次运行受影响行数为 0、`content` 列字节数不变。

## 5. 验证命令（实现线程执行）

| 命令 | 结果 |
|---|---|
| `cd backend-go && go test ./internal/platform/htmltext -count=1` | PASS |
| `cd backend-go && go test ./internal/reader/handler -count=1` | PASS |
| `cd backend-go && go test ./internal/reader/service -count=1` | PASS |
| `cd backend-go && go test ./cmd/clean-article-descriptions -count=1` | PASS |
| `cd backend-go && go vet ./internal/platform/htmltext ./internal/reader/... ./cmd/clean-article-descriptions` | 0 问题 |
| `cd backend-go && golangci-lint run ./internal/platform/htmltext/... ./internal/reader/... ./cmd/clean-article-descriptions/...` | 0 问题 |
| `cd backend-go && go run ./cmd/clean-article-descriptions`（dry-run） | 见 §4 实测基线 |
