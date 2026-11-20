# UI 验收证据（fix-reader-lede-blank-space）

> 场景：智源社区（feed 1）文章 `id=181787`「Anthropic今晚冲刺市值1.5万亿美元…」。
> 环境：本地静态托管 `http://127.0.0.1:5100/?article=181787`（同源入口），agent-browser（Chromium headless）。
> 修复前基线（修复前同一篇文章实测）：导语段 `.lede` 高 **8046 px**、正文 `.markdown-article` 从第 **8787 px** 才开始，标题下方一整屏空白；页面可见文本含 `aaaa` = false。

### A-0 修复后主场景（重点截图）【pass】
依据：标题下导语段为一行纯文本（浅色细左线，无空白），正文段落紧接其后；整页无空白区块。截图 `lede-after-fix.png`。

### A-1 导语段不再产生不可见空白【pass】
依据：修复后同一篇文章 `.lede` 实测高 **30 px**（修复前 8046 px），`.markdown-article` 起始 top 由 8787 px 提前到 **771 px**；截图为标题 + 细左线导语 + 正文紧接其下，首屏直接可见正文。断言依据：`getBoundingClientRect().height` 与 `top` 实测值对比。截图 `lede-after-fix.png`（对照 `lede-before-fix.png`）。

### A-2 导语以纯文本渲染、不把源站 HTML 注入 DOM【pass】
依据：`document.querySelector('.lede').innerHTML` 返回纯文本串（`内容中包含的图片若涉及版权问题，请及时与我们联系删除`），不含任何标签；渲染路径为 `v-text` + `toPlainText`，源站 `<div id="js_content" style="opacity: 0">` 已不再进入 DOM。

### A-3 页面可见文本不含「aaaa」【pass】
依据：`document.body.innerText.includes('aaaa')` 实测 `false`；全库剥标签后搜索 `aaaa` 0 命中（用户所见的「aaaa」为配图 CDN 文件名哈希 `.../hub-detail/3a8fed6844ebaaaae1a18986c50781fa1790748001235.webp`）。

### A-4 原文源渲染前清洗【pass】（证据为单测，入口验证见 A-6）
依据：内容源切换入口在本次验证时被**并发 change** 改动（`ArticleStatusMenu.vue` / `ArticleContentToolbar.vue` 在工作树中属其它 active change 的脏文件，菜单里已不再渲染源切换分段控件），故通过等价的**行为断言**验证：`front/app/utils/articleContentHtml.test.ts` 7 例全绿，覆盖——`opacity:0` 根节点包裹的正文清洗后仍可见且输出不含 `opacity`（智源形态）、`display:none` 块连同子树消失、`on*` 事件属性不残留、`<iframe srcdoc>` / `<form>` / `javascript:` 被剔除、清洗排在外链图片代理之前、Base64 占位图被剔除、空输入返回空串。修复前同一路径实测 `.markdown-article` 高 10634 px、可见文本 4819 字（截图 `original-source.png` 为修复前留证）。

### A-5 切回「Firecrawl 全文」默认源后正文链不变【pass】
依据：重点后 `.markdown-article` 首元素为 markdown 渲染产物（`<h5>`/`<p>` 结构），`.lede` 高度仍为 30 px，内容源取值链无回归；`useArticleContentView.ts` 的 firecrawl 分支仍走 `marked.parse`（源码锚 + `ArticleContentView.test.ts` 14 例全绿）。

### A-6 内容源切换入口的原机核对【未覆盖】
依据：验证时该入口被其它 active change 从工具栏菜单移走（`ArticleStatusMenu.vue` / `ArticleContentToolbar.vue` 均为工作树中他人会话的脏文件，本 change 未触碰），故无法在真机上点开「原始内容」源。未覆盖原因：入口不在本 change 职责内，且并发改动使其在当前构建中不可达；等价行为已由 A-4 的单测覆盖。待并发 change 收尾后，入口恢复即可人工复验。

### A-6 智源类「整体隐藏」文章的导语内容【未覆盖】
依据：智源文章的正文整体位于 `opacity:0` 的根 div 内，纯文本摘要按契约剔除隐藏子树，故导语只落到文章尾部可见的一句版权提示（「内容中包含的图片若涉及版权问题，请及时与我们联系删除」）。已实测确认**不再出现空白**（A-1），但该句作为导语语义价值低（属源站可见尾注，非内容摘要）。未覆盖原因：进一步治理需引入「可见首段抽取」或尾注黑名单，属超出本 change 范围的独立优化；本 change 的契约只要求「不可见内容不形成空白占位、第三方 HTML 不直出 DOM」，两条均已满足。
