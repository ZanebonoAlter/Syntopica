# test-cases 节拍勾对（tasks 5.3）

change: `slim-article-list-payload`｜对账日期 2026-09-24｜`test-cases.md` 主链路 9 节拍逐行落点与通过证据。

| # | 节拍 | 落点测试 / 证据 | 命令与结果 |
|---|---|---|---|
| 1 | 列表首屏体积与字段集 | `TestGetArticles_Projection`（8 分支）+ 真库 curl | `go test ./internal/reader/handler/ -count=1 -run TestGetArticles` → PASS；`curl '…/api/articles?per_page=20'` → **11007 B**（< 102400）、`"content"` 0 次、`"excerpt"` 20 次（`evidence/volume-after.txt`） |
| 2 | 各筛选分支字段集一致 | `TestGetArticles_Projection` 表驱动 8 分支（sqlite）+ 真库 8 分支探针（postgres 的 DISTINCT/Group 路径） | 分支：无筛选 / `feed_id` / `category_id` / `watched_tag_ids&sort_by=relevance` / `watched_tag_ids`（DISTINCT）/ `concept_id` / `auxiliary_label_id` / `archived=true` → 全部 **21 key、禁列 0 命中**（`evidence/volume-after.txt`） |
| 3 | 列表行渲染不变（回归） | `ArticleCardView.test.ts`（含在 91 passed）+ 截图 | `pnpm test:unit app/features/articles app/features/feeds app/stores --maxWorkers=2` → **9 files / 91 tests passed**；`evidence/screenshot-list-after.png` |
| 4 | 点选文章（详情未就绪首帧） | `ArticleContentPreviewPanel.test.ts` 新增 2 用例 + 浏览器 P1/P5b | 组件测试 10 passed；浏览器：首页文章 `.lede` **不存在**（补丁后，无闪现）、feed 2 文章 `.lede` 文本 === 列表 `excerpt`（`evidence/e2e-browser.md`） |
| 5 | 详情返回（导语与正文以详情为准） | 浏览器 P2/P6 + `useArticleContentView` 单测（既有） | P2：无导语 + 正文 253 字符；P6：导语仍在 + 正文 4979 字符；`agent-browser errors` 空、控制台 error 0（`evidence/e2e-browser.md` + `screenshot-reading-no-lede.png` / `screenshot-lede-present-feed2.png`） |
| 6 | 单 feed 定时刷新完成 | `useAutoRefresh.test.ts`「单 feed 刷新完成」+ `useArticlePagination.test.ts` 5 用例 | 9 tests passed；断言 `per_page ≤ 100`、`page`/选中行不变、失败不改动列表 |
| 7 | 23 feed 同周期 | `useAutoRefresh.test.ts`「多 feed 同周期：同一分钟至多 1 个刷新」 | PASS：fake timers 推进 60 分钟，`starts.length ≥ 20` 且按分钟分组每组 ≤ 1（既证明确实在跑，也证明不变量成立） |
| 8 | 刷新请求失败 | `useAutoRefresh.test.ts`「刷新失败不终止调度」 | PASS：首个 feed reject 后，feed 2/3 仍被触发 + `console.error` 记录 |
| 9 | `per_page=10000` | `TestPerPageOverLimit` + 真库 curl + WARN 留痕 | 测试 5 档全 PASS；`curl '…per_page=10000'` → 100 条；`grep -c 'per_page=10000' backend-go/logs/app.log` → **4**（≥1） |

**负向节拍（SHALL NOT）**
- 「不得出现 `per_page > 100` 的文章列表请求」：`useAutoRefresh.test.ts`「不发起全量重拉：刷新流程不请求文章列表」（mock `~/api/articles` 的 `getArticles`，断言从未调用）PASS；`grep -n 'fetchArticles({ per_page: 10000' front/app/features/feeds/composables/useAutoRefresh.ts` → 无命中（exit 1）。
- 「不得同分钟 20+ 并发刷新」：节拍 7 的分钟分组断言 ≤ 1。

**白盒附加（test-cases §5）**
- §5.1 投影分支表：`TestGetArticles_Projection`（8 分支）+ 真库探针（含 `per_page=0 → 20`、`101 → 100 + WARN`、`10000 → 100 + WARN`、`100` 边界无 WARN）全绿。
- §5.2 `excerpt` 边界值：`TestExcerpt` **27 子用例**全 PASS（含空/纯空白/全角空格/纯图片/纯符号/199-200-201 字符/50KB HTML/script 剥离/description 空回退/同源抑制/子串抑制/兜底短帖抑制/有 Firecrawl 正文保留/短导语保留）。
- §5.3 划除留痕：时间窗口组（不改日期语义）、并发/线程安全（未声称）、DB 迁移层（无迁移）→ 不适用。
- 效果核对（§4 真库量化）：`evidence/volume-before.txt`（144850 B）→ `evidence/volume-after.txt`（11007 B，**−92%**）。

**场景 → 测试文件映射**：见 `tasks.md` 验证节末尾表格（16 个 Scenario 全部有落点）。
