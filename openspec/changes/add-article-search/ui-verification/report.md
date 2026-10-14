# add-article-search UI 验收报告（任务 3.6 主链路 + 4.2 双视口）

- 验收时间：2026-10-14（agent-browser headless，Chromium，静态托管 http://localhost:5100）
- 断言方式：机械 DOM 断言优先（eval 查询选择器/class/文本），截图为人审证据
- 视口：主链路 1440×900；4.2 双档 1440×900 / 1920×1080
- 断言工具：agent-browser session `e2e-search-6d5795ec8cb0`

## 一、任务 3.6 主链路逐步断言

| # | 判据 | 结果 | 证据文件 |
| --- | --- | --- | --- |
| 1a | 侧边栏存在「文章搜索」入口且位于「发现订阅源」之后 | **pass**（DOM：sidebar-items 顺序 `["全部文章","收藏夹","叙事工坊","发现订阅源"(idx3),"文章搜索"(idx4),…]`，idx4=idx3+1；`data-onboarding="nav-search"`） | e2e-1-sidebar-entry-to-search.png |
| 1b | 点击入口 → URL 变为 /search | **pass**（`location.pathname === '/search'`，初始空态 `[data-test=search-initial-empty]` 渲染，页标题「文章搜索」） | e2e-1-sidebar-entry-to-search.png |
| 2a | 输入连续短语「特斯拉电池」回车 → 结果卡片 ≥1 | **pass**（12 张卡片；摘要「共 12 条结果 · 3.01s」；输入框值=特斯拉电池） | e2e-2-search-results.png |
| 2b | 命中 chip 存在（关键词命中语义） | **pass**（12 个 `.hit-chip`，全部 `hit-keyword`，文案样本「🔍 命中「特斯拉电池」」） | e2e-2-search-results.png |
| 2c | 归档角标存在 | **pass**（12 个 `.archived-badge`，文案「已归档」；本次查询 12/12 结果均为 archived，与 API `archived:true` 一致） | e2e-2-search-results.png |
| 3a | 点结果卡片 → 预览弹窗打开（AppDialog 可见）且 URL 不变 | **pass**（`.app-dialog-overlay` 可见、header「文章预览」、`location.pathname === '/search'`；两查询各测一次） | e2e-3-preview-modal-highlight.png |
| 3b | 弹窗正文命中词高亮（mark 元素） | **pass**（查询「安全带卡扣」→ `mark.apm-hit` ×3，文本样本「安全带卡扣」×2 + 正文句「特斯拉车主反映安全带卡扣断裂」） | e2e-3-preview-modal-highlight.png |
| 3b' | 「特斯拉电池」查询的弹窗高亮 | **观察**：mark=0——该查询 12 条命中均为 L3 trigram 模糊命中标题（score≈0.18），正文不含「电池」字样，无字面词可包；高亮机制本身经 3b 证明工作正常，属词组不在正文的自然结果，非缺陷 | e2e-3-preview-modal-highlight.png（安全带卡扣）/ 无（特斯拉电池为 0 mark） |
| 4 | 弹窗内点「阅读全文」→ 跳转阅读路径（URL 离开 /search）→ 返回 /search | **pass（复验通过（修复后）**，原 fail 详情保留于缺陷 D1）：弹窗 header 现含「阅读全文」按钮（`data-test=preview-read-full`），点击后 URL=`/?article=3170` 离开 /search，首页阅读路径 reading-col 渲染该文章（H1.article-title-full 标题可见）。残留子发现：浏览器 back 回 /search 后查询/列表为初始空态未保留——spec「阅读全文跳转」Scenario 不含返回保留要求，详见 D1 复验记录 | e2e-4-readfull-fixed.png + e2e-4-back-query-lost.png |
| 5 | Esc 关闭 → 返回列表、查询输入保留、滚动位置恢复 | **pass**（双查询变体：①「安全带卡扣」：modal 关闭、输入值=安全带卡扣、scrollTop 261→261 精确恢复、列表 9 卡完好；②「特斯拉电池」：输入值=特斯拉电池、scrollTop 300→300 精确恢复、12 卡完好） | e2e-5-esc-restore.png |
| 6a | 切「仅活跃」（关「含归档文章」）→ 触发重查 | **pass**（network requests 捕获 `GET /api/search?q=特斯拉电池&archived=active-only&expand=off&page=1&per_page=20`；结果 12→0 卡，空态「没有找到「特斯拉电池」的相关文章」） | e2e-6-filter-active-only.png |
| 6b | 无结果空态含「开启 LLM 扩展搜索再试」引导 | **pass**（`[data-test=enable-expand-retry]` 按钮存在） | e2e-6-filter-active-only.png |
| 6c | LLM 扩展开关存在且默认关闭 | **pass**（`.app-toggle` label「LLM 扩展搜索」`is-active=false`；「含归档文章」默认 `is-active=true`） | e2e-2-search-results.png（开关可见）+ e2e-6 |
| 附 | 语义降级提示条（spec「语义降级提示」Scenario 顺带验证） | **pass**（本环境 embedding 不可用，`[role=status]` 提示条「语义匹配暂不可用，已降级为纯关键词结果」展示，关键词结果正常，无错误弹窗） | e2e-6-filter-active-only.png |

## 二、任务 4.2 双视口断言（/search 有结果态，12 卡）

| 视口 | 判据 | 结果 | 证据文件 |
| --- | --- | --- | --- |
| 1440×900 | contained 容器（`.app-page-shell__inner`）宽 ≤1120px 且居中 | **pass**（offsetWidth=1120，left=160/right=160，左右差 0px） | viewport-1440.png |
| 1440×900 | 无横向溢出 | **pass**（`documentElement.scrollWidth=1440 ≤ clientWidth=1440`；`.search-main` scrollWidth=clientWidth=1440） | viewport-1440.png |
| 1920×1080 | contained 容器宽 ≤1120px 且居中 | **pass**（offsetWidth=1120，left=400/right=400，左右差 0px） | viewport-1920.png |
| 1920×1080 | 无横向溢出 | **pass**（`documentElement.scrollWidth=1920 ≤ clientWidth=1920`；`.search-main` scrollWidth=clientWidth=1920） | viewport-1920.png |

## 三、主链路 pass/fail 总结

- 步骤 1/2/3/5/6 **全 pass**（含双查询变体与空态/降级顺带验证）。
- 步骤 4 **fail**——spec Requirement「搜索结果预览弹窗交互」Scenario「阅读全文跳转」未实现（见缺陷 D1）。
- 双视口（4.2）**全 pass**。

## 四、缺陷清单

### D1（High）：预览弹窗无「阅读全文」入口，spec「阅读全文跳转」Scenario 未实现

- 判据：spec「搜索结果预览弹窗交互」——*弹窗内「阅读全文」SHALL 进入现有阅读路径*；tasks 3.6 步骤 4。
- 复现：/search 搜「特斯拉电池」→ 点任意结果卡 → 弹窗打开 → 遍历弹窗内所有 button/a/[role=button]（文本/title/aria-label），无「阅读全文」或等价站内阅读路径入口；可用动作仅 `在新窗口打开原文`（外部 link，非站内阅读路径）、收藏、全屏、切换内嵌网页、处理状态、关注标签。
- 源码佐证：`grep -rn "阅读全文" front/app` 零命中；`ArticlePreviewModal.vue`（复用自 tags 域）→ `ArticleContentView.vue` 全链无 router/navigate-to-reading 调用。
- 影响：搜索→预览→深读的主用户旅程断链，用户无法从预览进入阅读页。
- 处置建议：需产品决策——在 ArticlePreviewModal（或其 search 用法）补「阅读全文」动作跳转阅读路径，或修订 spec 移除该 Scenario。**本验收不修码，留待主线程处置。**
- **D1 复验（2026-10-14 修复后）：部分通过**——「阅读全文」入口+跳转 pass：弹窗 header 存在「阅读全文」按钮（`data-test=preview-read-full`，search.vue 内 `goReadFull` 经 `navigateTo` 跳转），点击后 URL=`/?article=3170` 离开 /search、首页阅读路径 reading-col 渲染该文章（标题可见），搜索→预览→深读主旅程恢复；back 返回子断言 **fail**：浏览器 back 回 /search（URL 正确）但查询输入为空、结果列表 0 卡、初始空态（2/2 稳定复现）——useSearch 状态为组件内 ref、路由离开即销毁；spec「阅读全文跳转」Scenario 仅要求跳转，返回保留属复验 brief 补充断言的新子发现。证据：e2e-4-readfull-fixed.png（跳转后阅读栏）+ e2e-4-back-query-lost.png（back 后空态）。一句话：D1 本体已修复，唯「阅读全文」离页后返回不保留查询/列表，是否需支持留主线程决策。主链路表步骤 4 已同步改为 pass（复验通过（修复后））。

### D2（Low/UX 观察）：trigram 模糊命中的高亮为空

- 复现：搜「特斯拉电池」→ 12 条命中均为 L3 trigram 标题模糊命中（score≈0.18），弹窗正文无一含「电池」→ mark=0。
- 判断：高亮实现按命中文本字面包裹文本节点，词组不在正文时无字面可包，机制本身正常（「安全带卡扣」查询 3 个 mark 佐证）；spec 只要求「保留命中词高亮」，未要求模糊命中的部分词高亮。属可接受的边界行为，记录备查；如需改进可考虑对 trigram 命中词做子词级（如「特斯拉」）高亮。

### D3（Info）：点击侧边栏/结果卡的 label 文本不触发动作

- AppToggle 的点击处理器绑定在 `.app-toggle__track` 上，点 label 文本 span 无响应（E2E 中 `find text "含归档文章" click` 无效，点轨道有效）。键盘/可达性角度 label 应可点（原生 label 包裹时点文本应等效）。低优先级可达性改进，记录备查。

## 五、证据文件清单（ui-verification/）

- e2e-1-sidebar-entry-to-search.png —— 首页侧边栏入口顺序 + /search 初始空态
- e2e-2-search-results.png —— 「特斯拉电池」12 结果、hit-keyword chip ×12、已归档角标 ×12
- e2e-3-preview-modal-highlight.png —— 预览弹窗（安全带卡扣，mark.apm-hit ×3，URL=/search）
- e2e-4-read-full-missing.png —— 弹窗动作全集（无「阅读全文」入口）
- e2e-5-esc-restore.png —— Esc 关闭后列表原位（查询保留、scrollTop 恢复）
- e2e-6-filter-active-only.png —— 仅活跃重查（archived=active-only 请求、空态+LLM 扩展引导+语义降级提示条）
- viewport-1440.png / viewport-1920.png —— 双视口 contained 1120 居中、无横向溢出
- e2e-4-readfull-fixed.png ——（D1 复验 2026-10-14 修复后）「阅读全文」跳转 /?article=3170 阅读栏渲染该文章
- e2e-4-back-query-lost.png ——（D1 复验 2026-10-14）back 回 /search 后查询/列表空态（back 保留子断言 fail 证据）

> 注：断言均为 DOM 机械断言（选择器/class/文本/网络请求），截图供人审像素级细节；验收模型无图像通道，未做截图视觉复看。
