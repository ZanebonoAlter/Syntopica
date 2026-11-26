# Review 记录：fix-reader-lede-blank-space

> 2026-11-20 派 `change-review` 子线程对制品（proposal / ui-design / design / specs / tasks / test-cases）与实现 diff 做聚焦 review；报告按 High / Medium / Low 分类，处置如下。
> 结论：5 High / 8 Medium / 2 Low（另 3 条不计入本 change 缺陷），High 全部已修复，Medium 全部处置（修复或明确驳回理由）。

## High

### R-1 2026-11-20【High】刷新判重被清洗打断 —— 每轮刷新都重跑 Firecrawl+LLM+打标链
- **状态**：已修复
- `feed_service.go:refreshExistingArticle` 原先用 `existing.Description == entry.Description` 判「内容未变」，但库里是清洗后的 ≤300 纯文本、`entry` 是源站原文，对「摘要即整篇 HTML」的源永远不等 → 每轮刷新都重置状态位、删标签、重跑处理链（违反 `rss-article-dedup` 主 spec 与 content-enrichment 约束 #11；影响面含智源 2148、掘金 4023、金融界 3501、华尔街见闻 995 篇）。
- 修复：改为先 `fresh := s.buildArticleFromEntry(feed, entry)`，比较 `fresh.Title` / `fresh.Description` / `fresh.Content`。**必须带 `content` 比较**——清洗后摘要只剩 300 字，正文变化无法从摘要检出。决策入 design D11。

### R-2 2026-11-20【High】Go 与 TS 清洗口径相反（实体处理顺序）
- **状态**：已修复
- Go `PlainText` 先解实体再解析（`&lt;c&gt;` 被当标签丢弃），TS `toPlainText` 用浏览器语义先解析（`<c>` 保留为字面量）。两套单测各自绿、行为相反，违反 delta spec「同源同口径」，且 design D5 声明的防漂移锚实际未落地。
- 修复：口径统一为「先解实体 → 再解析 → 取文本」（保幂等，代价是转义标记会被当标签丢弃，已写进 spec 正文），前端改用 `decodeEntitiesOnce` 前置；两侧写入**同一组 5 个固定样本**（实体 / 视觉隐藏 / 标签分隔 / 纯文本 / display:none），断言逐字相同（Go `TestPlainText_SharedSamplesWithFrontend` ↔ TS `跨实现共享样本`）。决策入 design D10。

### R-3 2026-11-20【High】Go `SanitizeHTML` 顶层 `<script>`/`<style>` 未移除，单测假绿
- **状态**：已修复
- `sanitizeNode` 只对子节点判 `isDroppedElement`，顶层节点自身不判 → `SanitizeHTML("<script>…</script><p>x</p>")` 原样返回带 `<script>`；既有单测把 script 嵌在 `<div>` 内所以通过（假绿）。
- 修复：`SanitizeHTML` 顶层也走 `isDroppedElement` 判定（且判定在属性清洗之前，否则 `display:none` 声明会先被剥掉导致元素存活）；补顶层脚本/样式/隐藏块断言。
- 备注：该函数目前**无生产调用点**（渲染路径用前端 TS 版），属潜伏问题；已在 design D10 注明它是未来接线点。

### R-4 2026-11-20【High】前端 `sanitizeHtml` 残留可执行嵌入与危险 URL 方案
- **状态**：已修复
- 原文源是未受信的源站全文 HTML，原实现只移除 script/style/`on*`/隐藏块，`<iframe srcdoc>`、`<form>`、`href="javascript:…"`、`data:text/html` 均可执行。
- 修复：移除 `iframe`/`object`/`embed`/`form`/`applet`/`base`/`frame`/`frameset` 与 `srcdoc`；对 `href`/`src`/`xlink:href`/`action`/`formaction`/`poster`/`data` 过滤 `javascript:`/`vbscript:`/`data:`（`data:image/*` 放行），判定前先剥控制字符（`java\tscript:` 也拦）。spec 补对应 Scenario。

### R-5 2026-11-20【High】整理稿兜底输入被清空（Firecrawl 失败时永久拿不到摘要）
- **状态**：已修复
- `content_completion_service.go` 的降级路径回退 `article.Description`；摘要纯文本化后，智源形态（整篇正文在 `opacity:0` 根节点内）清洗结果为空串 → Firecrawl 一失败就报 `no content available for summary`，文章永久 failed。这破坏了 content-enrichment 的降级链设计意图。
- 修复：降级改用 `htmltext.PlainTextAll(article.Content)`（正文源字段的文本投影，**保留隐藏子树文本**——源站把正文藏起来是它的样式决定，不代表内容不存在）；`description` 仅在 `content` 也为空时再用。新增单测 `TestCompleteArticle_FirecrawlFailedFallsBackToBodySourceWhenSummaryEmpty`（断言摘要 complete 且 AI 请求体含正文文字）。决策入 design D12。

## Medium

### R-6 2026-11-20【Medium】回填候选口径与「是不是 HTML」的真实判定不一致
- **状态**：已修复
- `LIKE '%<%'` 会把「纯文本里提到标签」的行也纳入（`使用 <br> 标签`）。实测本地库 63303 行含 `<` 者全部含真标签（当前无受害行），但口径必须对齐。
- 修复：SQL 保留方言中立的粗筛（`LIKE '%<%' OR length(description) > ?`），真实判定移到 Go 侧用既有的 `htmltext.HasMarkup`（`<`+字母+…+`>`）；超长纯文本行按上限截断以对齐「摘要 ≤300」不变量（实测仅 2 行）。补两个单测：`a < b 是正常比较符号` 不被改写、350 字纯文本被截断且带省略号。决策入 design D13。
- 注：刻意**不用** Postgres 正则 `~`——测试跑 sqlite，两方言不一致。

### R-7 2026-11-20【Medium】跨上限共享测试是空转的
- **状态**：已修复
- 5 个样本全 < 200 rune → `PlainSummary(raw,200)` 与 `(raw,300)` 必然相等，对「口径是否共享」零信息量。
- 修复：换成 250 rune 样本，断言「200 版 == 300 版前 199 rune + …」「300 版不截断」」。

### R-8 2026-11-20【Medium】「原文源渲染前清洗」只有源码字符串断言
- **状态**：已修复
- 原断言匹配源码文本，无法证明「隐藏块真的消失、事件属性真的没绑」。
- 修复：把原文源加工链提成纯函数 `buildOriginalContentHtml`（`front/app/utils/articleContentHtml.ts`，放 utils 层是因为 composable 顶层 import pinia stores，测试环境拉不起来），新增 `articleContentHtml.test.ts` 7 例**行为断言**；`ArticleContentView.test.ts` 另加 5 例行为断言。

### R-9 2026-11-20【Medium】design.md 落点指向错误的任务号
- **状态**：已修复
- D4 原写「落点：3.1」（实为前端 DOMParser，应为 4.1）、D7 原写「落点：4.1」（实为回填 CLI，应为 5.1）。
- 修复：D4 → `4.1`、D7 → `5.1`、D9 落点改为「该文件保持不变」的明确措辞；新增 D10–D13 承接 review 后的四项决策。

### R-10 2026-11-20【Medium】excerpt 口径变了但 `article-list-projection` 主 spec 未同步
- **状态**：已修复
- excerpt 现在额外剔除隐藏子树、还原全部 HTML 实体（原来只还原 6 个具名实体），但对「description 即整篇 HTML」的源 excerpt 会变成空串——这是有意收敛，两个主 spec 必须一致。
- 修复：新增 delta `specs/article-list-projection/spec.md`（MODIFIED「excerpt 承载列表层导语」），把清洗口径写全（还原实体 → 解析 → 去脚本样式 → 剔隐藏子树 → 去标签 → 折叠空白；截断不带省略号；无字母数字即空串），并补「整体隐藏的源其导语为空串」Scenario。

### R-11 2026-11-20【Medium】flow 文档规模数字与实测基线不一致
- **状态**：已修复
- 文档写「13602 行含 HTML / 276 MB」，实测是「63303 行含真实标签 / 381.0 MiB（按字符计 276 MB）」，行数差一个量级、字节与字符混写。
- 修复：`reading.md` 与 `content-enrichment.md` 统一为实测口径（63303 行 / 381.0 MiB，注明按字符计 276 MB）；`pin_finding` 的历史快照保留原值不改写（当时的粗略口径）。

### R-12 2026-11-20【Medium】`rss_parser.go` 混进另一 change 的功能改动
- **状态**：已采纳
- 该文件除本 change 的字段注释外，还含 `httpclient.WithCrawlScope()` 的出站代理作用域改动（归属 `narrow-outbound-proxy-scope`）。
- 处置：本 change 提交时按 hunk 拆分，只提交注释那一段；功能改动留给归属它的 change。已在 tasks.md 的「脏文件」提示与提交纪律中记录。

### R-13 2026-11-20【Medium】回填范围与「≤300 字符」红线对不齐
- **状态**：已修复（与 R-6 同批）
- 不含 `<` 但 > 300 字符的纯文本行原本不会被回填 → 回填后表里仍混着超长摘要，与 content-enrichment 新红线 #14 不符。
- 修复：候选增加长度臂（`length(description) > maxRunes`），此类行按上限截断；单测覆盖。

## Low

### R-14 2026-11-20【Low】excerpt 与摘要有第二处口径差异（实质文本地板）
- **状态**：已采纳
- excerpt 有「无字母数字 → 空串」的地板（`"，，，。"` 在 excerpt 为 `""`、在摘要为 `"，，，。"`），而 spec 原写「仅允许长度上限不同」，字面上不成立。
- 处置：spec 措辞放宽为「仅长度上限与既有的『实质文本地板』不同」，地板保留（是 excerpt 的既有契约）。

### R-15 2026-11-20【Low】`display:none !important` 前后端都漏判
- **状态**：已采纳（不改）
- Go 的 `isDisplayNone` 与 TS 的 `value === 'none'` 都不剥 `!important`，但**两侧一致**，不构成漂移；记此备查，后续如遇真实样本再修。

### R-16 2026-11-20【Low】`ui-design.md` 写 `.reading-col` max-width 840px，reading 域红线 #7 写「≤760px」
- **状态**：误报丢弃
- 属既有文档/代码不一致（`ArticleContent.css` 实际是 840px，红线文字是历史表述），本 change 未触碰版式，不属本 change 缺陷。

---

## 审毕无发现的范围

- `articles.content` 零改动：diff 只有 `Content: entry.Content` 一行未变；回填 CLI 只 `UpdateColumn("description", …)`。
- 回填默认 dry-run 成立；分批事务、批间让出、信号可中断、`openDB` 不走 `InitDB`（不迁移不 seed）。
- 既有 excerpt 契约三条（≤200 / 无字母数字为空 / 与正文重复为空）全部保留，handler 单测全绿。
- `shouldShowArticleDescription` guard 未被改动（design D9 守住）；`v-if="showDescription && ledePlainText"` 双条件兜住「guard 说有内容但清洗后为空」。
- 导语已彻底不再 `v-html` 直出 `description`；`grep -rn 'v-html="article.description"' front/app` 无命中。
- 两类隐藏分流（语义不展示移除子树 / 视觉隐藏剥声明留内容）在 Go 与 TS 两侧判定一致（含 `0`/`0.0`/`0px`/`0%`、`visibility:collapse`、`hidden` 属性）；纯文本侧两侧都「任一隐藏声明子树整体剔除」。
- 文档红线句格式合规（顶层列表项 + 首词组加粗自含句 + `：` 接细节）。
