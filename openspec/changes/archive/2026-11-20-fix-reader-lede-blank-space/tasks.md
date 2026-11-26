# Tasks：fix-reader-lede-blank-space

> 设计依据 design.md D1-D9；用例锚点 specs delta（`article-description-sanitization` 新增 3 个 Requirement / 9 个 Scenario，`reading-article-pane` 改写 2 个 Requirement）；事实来源见 proposal.md - Why（智源 2148/2148 篇 description==content，均值 66KB，最大 864KB；全库 13602 行 description 含 HTML，合计 276MB；实测导语段高 8046px、正文从 8787px 开始）。

> ⚠️ 工作树存在其它会话的脏文件（"归属其他 active change" 桶 1195+ 个）。本 change 只碰下列清单里的文件，提交时按归属地图精确 `git add`，不得 `git add -A`。

## 1. 用例先行

- [x] 1.1 撰写 `test-cases.md`：主链路表（步/动作/来源 Scenario/期望/层/落点），覆盖 specs delta 全部 Scenario——`article-description-sanitization` 的「整篇 HTML 型摘要入库为纯文本」「隐藏节点内容不进入摘要」「纯文本摘要不被改写」「清洗幂等」「正文源字段不被改写」「摘要导语以纯文本呈现」「不可见内容不占位」「事件属性不被执行」「正文源切换后正文可见」「默认不写库」「显式写入后摘要为纯文本」「重复执行无二次改动」，以及 `reading-article-pane` 改写后的「原始内容源渲染前清洗」「HTML 型详情摘要不产生空白」等既有 Scenario（既有 Scenario 回归行）。**⓪ 继承与调整表**：跑 `bash scripts/harness/test-assets.sh article-description-sanitization`（无则跑 `article-list-projection`）反查旧测试资产，逐行列「旧 Scenario × 处置 × 旧测试 × 动作」；白盒附加节出分支表（清洗器：空输入/纯文本/无标签/超长截断/实体嵌套/隐藏子树嵌套/script 内含标签/幂等）。验证：文件存在且 `grep -c "Scenario" test-cases.md` ≥ 12。

## 2. 后端：清洗包（口径唯一实现）

- [x] 2.1 新建 `backend-go/internal/platform/htmltext/htmltext.go`：`PlainSummary(raw string, maxRunes int) string`（剥 `<script>`/`<style>` 内容 → 剔除隐藏子树（自身 `style` 含 `display:none` / `visibility:hidden` / `opacity:0` / `font-size:0` 的元素及其后代）→ 去标签取文本 → 还原 HTML 实体（`&nbsp;`/`&amp;`/`&lt;`/`&gt;`/`&quot;`/`&#39;`/数字实体）→ 折叠连续空白为单空格 → 按 rune 截断并以「…」收尾）；`SanitizeHTML(raw string) string`（保守清洗：移除 script/style 元素、移除 `on*` 事件属性、移除隐藏元素、剥离 `style` 中的隐藏声明，保留其余结构与排版）；两者 MUST 幂等。配套 `htmltext_test.go`：用智源真实样本片段（`<div id="js_content" style="visibility: visible; opacity: 0;">…<p>正文</p>…</div>`）断言输出不含 `<`、不含 `opacity`、不含隐藏子树文本、长度 ≤上限、二次清洗结果相同；纯文本输入原样通过（仅空白折叠）；实体还原正确。验证：`cd backend-go && go test ./internal/platform/htmltext -count=1` 通过。

- [x] 2.2 `backend-go/internal/reader/handler/article_excerpt.go`：把既有 `stripExcerptMarkup` / `decodeHTMLEntities` 的清洗逻辑上提到 `htmltext` 包（handler 改为调用 `htmltext.PlainSummary` 或其内部的等价导出函数），保证列表 `excerpt` 与文章摘要**同源同口径**（仅长度上限不同：excerpt ≤200 / 摘要 ≤300）。既有 excerpt 单测断言保持绿（可调整调用方式，不改断言语义）。验证：`cd backend-go && go test ./internal/reader/handler -count=1` 通过，且 `grep -rn "stripExcerptMarkup\|decodeHTMLEntities" backend-go/internal/reader/handler` 无残留实现（只允许调用点）。

## 3. 后端：入库清洗

- [x] 3.1 `backend-go/internal/reader/service/feed_service.go:buildArticleFromEntry`：`Description: entry.Description` 改为 `Description: htmltext.PlainSummary(entry.Description, descriptionSummaryMaxRunes)`（常量 300，定义处写明"文章摘要上限，与列表 excerpt 200 上限同源不同参"）；`Content: entry.Content` **保持不动**（正文源不动）。在 `ParsedEntry.Description` 字段注释里写明其为未清洗的源站原文。验证：`cd backend-go && go test ./internal/reader/service -count=1` 通过；新增单测：条目 description 为整篇 HTML 时组装出的 `models.Article.Description` 为 ≤300 字符纯文本、`Content` 逐字等于源站原文。

## 4. 前端：清洗工具与渲染护栏

- [x] 4.1 新建 `front/app/utils/htmlSanitize.ts`：`sanitizeHtml(html: string): string`（浏览器 `DOMParser` 解析 → 移除 `script`/`style` 元素、移除 `on*` 事件属性、移除隐藏元素（`style` 含 `display:none`/`visibility:hidden`/`opacity:0`/`font-size:0` 或带 `hidden` 属性）→ 序列化回字符串；解析失败时降级为转义后的纯文本）；`toPlainText(html: string, maxRunes?: number): string`（去 script/style → 去标签 → 还原实体 → 折叠空白 → 可选截断）。配套 `front/app/utils/htmlSanitize.test.ts`：隐藏子树文本不出现、`on*` 属性被移除、`<script>` 内容不出现、实体还原、截断 ≤300 带省略号、幂等。验证：`cd front && pnpm test:unit utils/htmlSanitize.test.ts --maxWorkers=2` 通过（参数不带 `--`）。

- [x] 4.2 `front/app/features/articles/components/ArticleContentPreviewPanel.vue`：导语段由 `<div v-if="article?.description" v-html="article.description" />` 改为纯文本渲染——取值 `toPlainText(article?.description, 300)`，`v-if` 保持"有实质内容才渲染"，`v-text` 输出（**不得**再出现 `v-html="article.description"`）。`front/app/features/articles/components/ArticleContentView.test.ts` 增断言：description 含 `<img onerror=...>` / 含 `opacity:0` 包裹时，渲染出的导语节点不含任何元素子节点（纯文本）、不含 `onerror` 属性、导语文本 ≤300 字符。验证：`cd front && pnpm test:unit components/ArticleContentView.test.ts --maxWorkers=2` 通过。

- [x] 4.3 `front/app/features/articles/composables/useArticleContentView.ts`：`displayContent` 计算的 `original` 分支对 `resolvedContent` 施加 `sanitizeHtml()` 后再交给渲染（`firecrawl` 分支的 markdown 渲染链不变）；既有 `proxyImagesInHtml` 与 Base64 图片剔除顺序保持不变（清洗在最前）。验证：`cd front && pnpm test:unit features/articles/ --maxWorkers=2` 通过（既有内容源切换取值链断言不变）。

## 5. 存量回填

- [x] 5.1 新建 `backend-go/cmd/clean-article-descriptions/main.go`：扫描 `articles` 中 `description` 含 `<` 的行（分批读取），按 `htmltext.PlainSummary(raw, 300)` 清洗并 UPDATE；**默认 dry-run**（只打印待清洗行数、示例与预计释放体积，不写库），`--apply` 才写入；分批提交（每批独立事务，批间让出）；**MUST NOT 修改 `content` 与其它字段**；重复执行时已清洗行不再被判定为待清洗（幂等）。验证：`cd backend-go && go build ./cmd/clean-article-descriptions` 退出 0；`go run ./cmd/clean-article-descriptions`（dry-run）输出待清洗行数且 DB 无变化（用 `count(*)` 前后对比留证）；`--apply` 在本地库执行一次后再跑一次，第二次受影响行数为 0。

## 6. Review

- [x] 6.1 派 `change-review` 子线程聚焦 review（审查制品 proposal/design/specs/tasks 与实现 diff），重点核对：清洗口径前后端是否一致、隐藏节点判定是否覆盖 `opacity:0` 这类真实样本、`content` 是否被误改、回填是否默认 dry-run、导语取值优先级是否被破坏；意见按 High/Medium/Low 处置后在 `review.md` 建账。验证：`review.md` 存在且每条意见有处置行（`bash scripts/harness/review-trace.sh openspec/changes/fix-reader-lede-blank-space` 退出 0）。

## 7. Review 修复项（review 报告 2026-11-20：5 High / 8 Medium / 5 Low）

- [x] 7.1 【High H1】`backend-go/internal/reader/service/feed_service.go:refreshExistingArticle`：内容未变判定改为比较入库同口径的值（`fresh := buildArticleFromEntry(...)` → 比 `fresh.Title`/`fresh.Description`/`fresh.Content`；**必须带 content 比较**，清洗后摘要只剩 300 字检不出正文变化），否则「摘要即整篇 HTML」的源每轮刷新都重置状态、删标签、重跑 Firecrawl+LLM+打标链（违反 `rss-article-dedup` 主 spec 与 content-enrichment 约束 #11）。验证：新增单测——HTML 型摘要连续刷新不重跑处理链；content 变化仍走更新分支。
- [x] 7.2 【High H2】清洗口径跨实现对齐：`htmltext.PlainText`（Go，解实体优先）与 `toPlainText`（TS，现为浏览器语义）输出相反（`&lt;c&gt;` 一个丢标签一个保留），违反 delta spec「同源同口径」。**口径定为「先解实体 → 再解析 → 取文本」**（保幂等），前端改；两侧用同一组 5 个固定样本（智源/实体/混合可见性/纯文本/display:none）断言**逐字相同的输出**。验证：两侧单测各含同一张样本表，期望值逐字一致。
- [x] 7.3 【High H3】`backend-go/internal/platform/htmltext/htmltext.go`：`SanitizeHTML` 顶层元素未走 `isDroppedElement` → 顶层 `<script>`/`<style>` 残留（现有单测嵌在 `<div>` 内，属假绿）。修顶层判定 + 补顶层断言 + 删掉假绿断言。验证：`go test ./internal/platform/htmltext -count=1` 通过。
- [x] 7.4 【High H4】`front/app/utils/htmlSanitize.ts:sanitizeHtml`：补可执行嵌入与危险 URL 方案——移除 `iframe`/`object`/`embed`/`form`/`applet`/`base`/`frame`/`frameset` 与 `srcdoc`；`href`/`src`/`xlink:href`/`action`/`formaction`/`poster`/`data` 上的 `javascript:`/`vbscript:`/`data:`（`data:image/*` 除外）移除该属性。验证：`pnpm test:unit utils/htmlSanitize.test.ts --maxWorkers=2` 绿。
- [x] 7.5 【High H5】`backend-go/internal/reader/service/content_completion_service.go`：Firecrawl 失败时降级用的 `article.Description` 现在可能为空（智源形态整篇在 `opacity:0` 根节点内）→ 文章永久拿不到整理稿。改为用 `articles.content` 的**保留隐藏子树文本**的纯文本投影（`htmltext` 新增该投影），`description` 仅在 `content` 为空时再用。验证：新增单测（firecrawl 失败 + 摘要空 + content 有正文 → 整理稿能生成）。
- [x] 7.6 【Medium M1/M8】`backend-go/cmd/clean-article-descriptions/main.go`：候选口径从「含 `<`」改为「含真实标签标记（`<[a-zA-Z]`）或纯文本超 300 字符」，避免把正常纯文本摘要里的尖括号当标记改写（实测本地库 63303 行含 `<` 者全部含真标签，现无受害行，但口径必须对齐）；超长纯文本行按上限截断以对齐「摘要 ≤300」不变量（实测仅 2 行）。验证：补单测（纯文本含 `<` 无真标签不被改写 / 超长纯文本被截断）+ dry-run 行数复核。
- [x] 7.7 【Medium M3】`backend-go/internal/platform/htmltext/htmltext_test.go`：跨上限共享测试是空转的（5 个样本全 < 200 rune → 两上限必然相等）→ 换成 > 200 rune 样本，断言「200 版 == 300 版的前 199 rune（200 版带省略号、300 版不带）」。
- [x] 7.8 【Medium M4】`front/app/features/articles/components/ArticleContentView.test.ts`：「原文源渲染前清洗」只有源码字符串断言 → 补 `displayContent` 的**行为断言**（`opacity:0` 内容仍可见、`display:none` 块消失、无 `on*` 属性、firecrawl 源仍是 markdown 产物）。
- [x] 7.9 【Medium M2】design.md 落点修正：D4 → `4.1`、D7 → `5.1`、D9 落点措辞改为「该文件保持不变」；新增 D10（清洗顺序与跨实现锚）/ D11（刷新判重口径）/ D12（整理稿兜底投影）/ D13（回填候选口径）。验证：`bash scripts/harness/check-standards.sh` 与 design-anchor 检查通过。
- [x] 7.10 【Medium M5/M6/L1】delta 补 `article-list-projection` 的 MODIFIED（excerpt 清洗口径写全）；flow 文档规模数字改为实测口径（63303 行 / 381.0 MiB，按字符 276 MB）；spec「仅允许长度上限不同」放宽为「仅长度上限与既有实质文本地板不同」。验证：`bash scripts/harness/doc-impact.sh verify` 退出 0。

## 8. 测试

- [x] 8.1 后端影响包门禁：先用 `bash scripts/harness/change-scope.sh` 机械判定影响包（预期含 `internal/platform/htmltext`、`internal/reader/service`、`internal/reader/handler`、`cmd/clean-article-descriptions`），再对影响包跑 `go test <影响包> -count=1` 全 PASS、`go vet ./...` 退出 0、`golangci-lint run <影响包>` 0 issues、`go build ./...` 退出 0；树莓派上不顺手跑全量 `go test ./...`（`internal/reader/search` 的 PG 向量维度红测试与本 change 无关：经 `go list -deps` 验证该包不依赖本次改动包）。验证：命令与退出码记录在本节证据行。
- [x] 8.2 前端门禁：`cd front && pnpm lint` 0 错误、`pnpm exec nuxi typecheck` 0 error、受影响单测（4.1-4.3 + 7.2/7.4/7.8）带 `--maxWorkers=2` 全 PASS；不与 `pnpm build` 并行。验证：命令与退出码记录在本节证据行。

## 9. 文档

<!-- doc-impact: flow, database -->

- [x] 9.1 `docs/reference/flow/reading.md`：导语段链路补「摘要入库即纯文本（≤300）、导语以去标签纯文本渲染（禁 v-html 直出第三方 HTML）」；业务约束与不变量节加红线句（**摘要 MUST 以去标签纯文本呈现，不得把源站 HTML 直接注入 DOM；不可见内容 MUST NOT 形成空白占位**）；「代码入口」节补 `front/app/utils/htmlSanitize.ts` 与 `backend-go/internal/platform/htmltext/`。验证：`bash scripts/harness/doc-impact.sh context` 对应段核对一致。
- [x] 9.2 `docs/reference/flow/content-enrichment.md`：链路设计补「RSS 条目 description 入库即纯文本化（`buildArticleFromEntry` → `htmltext.PlainSummary`，≤300 字符；`content` 正文源不动）」与存量回填命令用法；业务约束与不变量节加红线句（**文章摘要字段 MUST 是 ≤300 字符纯文本，源站整篇 HTML MUST NOT 入库为摘要**）。验证：`bash scripts/harness/doc-impact.sh context` 对应段核对一致。
- [x] 9.3 `docs/reference/database/tables/content.md`：`articles.description` 字段说明补「入库即纯文本化（≤300 字符，清洗口径与回填命令）」，`articles.content` 补「保留源站原文、不被摘要清洗改写；原文源渲染前在前端清洗」。验证：`bash scripts/harness/doc-impact.sh verify openspec/changes/fix-reader-lede-blank-space` 无 database 域 FAIL。

## 10. 验证

- [x] 10.1 真实页面核对（agent-browser，静态 :5100）：打开一篇智源文章（feed 1，`/?article=<智源文章 id>`），核对——① 导语段呈现纯文本摘要而非空白；② 首屏直接可见正文（不再被 8000px 空白顶下去）；③ 切换内容源到「原文」后正文可见、无隐藏占位；④ 页面 DOM 中 `document.querySelector('.lede').innerHTML` 不含未清洗的源站标签。证据写入 `ui-verification/report.md`（条目格式 `### A-<N> <场景>【pass|fail|未覆盖】` + 依据行，截图文件名参与实物对账）。验证：`bash scripts/harness/ui-verification-trace.sh openspec/changes/fix-reader-lede-blank-space` 退出 0。
- [x] 10.2 文档与一致性校验：`bash scripts/harness/doc-impact.sh verify` 与 `bash scripts/harness/check-standards.sh` 退出 0；`grep -rn "v-html=\"article.description\"" front/app` 无命中（导语不再 v-html 直出）。验证：命令与退出码记录在本节证据行。
