# 浏览器端到端验证（tasks 3.4 / 32 / test-cases 节拍 4–5）

工具：`agent-browser`（headless Chromium，CDP）；会话 `AGENT_BROWSER_SESSION=syntopica-*`；视口 **1440×900**；目标 = 静态托管 `http://127.0.0.1:5100/`（新二进制 + 新静态产物）。
方法：用 `network route "*/api/articles/*" --abort` **拦截详情接口**，把「详情未就绪的首帧」变成可稳定观测的状态；断言全部走 DOM 机械读取（`.lede` / `.markdown-article` / `.article-row`）与 API 对照，不靠肉眼。

## 1. 补丁前实测（2026-09-24，窄投影首版）

| 相位 | 场景 | 结果 |
|---|---|---|
| P1 | 首页文章 + 详情被拦（首帧） | `.lede` 文本 **=== 列表 `excerpt`**（200 字符，逐字相等）；`.markdown-article` 文本长度 0（正文未到，无占位块） |
| P2 | 首页文章 + 详情放行 | 正文非空（4717 字符）；**`.lede` 消失** |
| P3 | V2EX 短帖 + 详情被拦 | `.lede` === excerpt（26 字符） |
| P4 | V2EX 短帖 + 详情放行 | 正文非空；**`.lede` 消失** |

P2/P4 的 `.lede` 消失不是 bug，而是阅读页既有去重 guard（`shouldShowArticleDescription`）在详情到达后判「导语与正文重复 → MUST NOT 渲染」——但它暴露了一个 spec 未预见的行为：

- 非归档文章 **1560/1704（92%）** `description == content`（RSS 全文源），另有 26 条 `description` 是 `content` 子串、191 条 `description` 空；
- 改动前这类文章**从来没有导语**（列表行自带正文，guard 首帧即命中）；窄投影后首帧没有正文 → guard 放行 → 点选时**导语闪现一下再消失**（远程链路上闪现时长 = 详情请求耗时）；
- 首屏 20/20 全部属于这一类。

→ 用户 2026-09-24 决策：**后端不下发「与正文重复」的导语**（方案 B）。两轮补丁见 tasks.md 2.6 / 2.7。

## 2. 补丁后实测

（见下方「补丁后」小节，含 P1/P2 复测、feed 2 正向路径、截图与控制台错误核对。）

### 补丁后（两轮补丁均已上线）

| 相位 | 场景 | 结果 |
|---|---|---|
| P1 | 首页文章（`description == content`）+ 详情被拦 | `ledePresent: false`（`.lede` 不存在）、正文 0 字符、列表项 `excerpt == ""` → **首帧即无导语，无闪现** |
| P2 | 首页文章 + 详情放行 | 仍 `ledePresent: false`、正文非空（253 字符）→ 首帧与终态一致 |
| P5b | feed 2（阮一峰的网络日志，导语与正文不同的真导语）+ 详情被拦 | `.lede` 存在且文本 **逐字 === 列表 `excerpt`**（50 字符）；拦截生效已单独验证（浏览器内 `fetch('/api/articles/2358')` → `Failed to fetch`） |
| P6 | 同上 + 详情放行 | `.lede` 仍在（同一文本，详情 `description` 与 excerpt 同源）、正文非空（4979 字符） |

页面错误：`agent-browser errors` **空**；控制台 error 级日志 **0 条**。

截图（`evidence/`）：
- `screenshot-lede-present-feed2.png`：导语正常常驻（feed 2，50 字符导语 + 正文）
- `screenshot-reading-no-lede.png`：首页文章（`description == content`）阅读页无导语、正文正常
- `screenshot-list-after.png`：列表页（行渲染字段不变）

### 诚实留痕（观测边界）

1. **P5b 的正文非 0 不代表详情已到**：正文来自「内容补全状态接口」（`GET /api/content-completion/articles/:id/status`）的 `liveStatus`，是既有路径（`useArticleContentView.mergedArticle`）；P1 的文章无该状态内容故正文为 0。
2. **真库无「导语文本可区分首帧/详情」的样本**：实测所有非空 excerpt 长度 ≤ 50 字符（< 200 上限），即 description 文本本身就短，详情返回前后导语文本相同，无法用文本区分两段式；该切换由单测 fixture 覆盖（`ArticleContentPreviewPanel.test.ts`「详情未就绪时首帧导语用列表 excerpt」/「详情返回后导语以详情 description 为准」），浏览器侧只断言「导语存在且 == excerpt」与「无导语闪现」。
3. **补丁前遗留**：首屏 20/20 文章属 `description == content` 类，补丁前点选会闪现导语；补丁后实测无导语（P1/P2）。
