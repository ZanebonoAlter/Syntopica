# 归档视图重做（8G）独立只读 review 报告

- **审查日期**：2026-11（本次会话）
- **审查对象**：`rework-workshop-into-radar-and-desk` change 的 **8G 归档视图重做段**
- **合同基线**：`ui-design-archive-rework.md`（v1→v10）+ `specs/workshop-article-archive/spec.md`（delta 验收合同）+ `specs/board-article-api/spec.md`（delta）
- **审查性质**：**只读**。除本报告外未修改任何文件；未 commit、未部署、未动数据库。
- **范围边界**：只评价 8G 段，不对整 change 判归档。

---

## 0. 验证记录（本轮实际跑过的命令与结果）

| 命令 | 结果 |
| --- | --- |
| `cd front && pnpm test:unit app/features/tags/components/console/BoardArchiveView.test.ts app/features/tags/components/console/ArchiveMatchPopover.test.ts app/features/tags/composables/useBoardArchive.test.ts app/utils/matchQuality.test.ts --maxWorkers=2` | **4 文件 / 60 用例全绿**（18 + 16 + 14 + 12） |
| `cd backend-go && go test ./internal/tagmanagement/...` | **全绿**（含 handler / service/board 包） |
| `git diff --stat -- front/app/features/tags/components/MatchDetailPanel.vue .../BoardTimelinePanel.vue .../BoardConsoleShell.vue .../BoardConsoleStage.vue` | **空**（旧证据面板 / 旧时间线页 / Shell / Stage 本轮均未改动） |
| `git diff --ignore-all-space --stat -- front/app/assets/css/main.css` | 真实功能改动 **19 行**（含 composite token ×2） |

**结论前提**：自动化全绿 ≠ 合同满足。本轮发现的 2 个 High 全部落在**当前测试没有覆盖的口径**上（见 R-10）。

---

## 1. spec 对齐清单（Requirement / Scenario → 实现落点 → 判定）

图例：✅ 满足 ｜ ⚠️ 部分满足/有偏差 ｜ ❌ 未满足

### 1.1 `workshop-article-archive` delta

| Requirement / Scenario | 实现落点 | 判定 |
| --- | --- | --- |
| **归档视图入口与日期筛选** | `BoardArchiveView.vue:64,76,101-140`（today / watch / 预设 + AppInput 起止） | ⚠️ 见 R-06（顶栏入口不回锚点当天） |
| Scenario 默认只看数据日 | `BoardArchiveView.vue:64` `props.asOf`；`BoardConsoleStage.vue:102` `:as-of="latestReportDate \|\| anchorDate"` | ⚠️ 见 R-06 |
| Scenario 自由日期范围 | `useBoardArchive.ts:200-240`（start_date/end_date 重取） | ✅ |
| Scenario 无文章 | `BoardArchiveView.vue:344-349`（`archive-empty`） | ✅ |
| **按日期平铺文章**（不按话题分组） | `useBoardArchive.ts:144-170`（Map 聚合，不碎组）；`BoardArchiveView.vue:471-533` | ✅ |
| Scenario 同一天多篇 | `useBoardArchive.test.ts:77-88`（交错日期不碎组） | ✅ |
| Scenario 一篇多标签（各自规则色/分数、降级半透明） | `BoardArchiveView.vue:196-222,494-512`；`boardArchiveView.css:388-404` | ✅（降级口径见 R-13） |
| Scenario 主点击读原文 | `BoardArchiveView.vue:474` `@click="emit('open-article', row.id)"` → `BoardConsoleStage.vue:105` → `BoardConsoleShell.vue:363` → `useConsoleShellOverlays.ts:46-52` → `ArticlePreviewModal`（`BoardConsoleShell.vue:435`） | ✅（链路完整；仅组件级断言，见 R-10） |
| Scenario 标签色彩编码可扫描（最强匹配列 = **最强证据**） | `BoardArchiveView.vue:224-245`（`strongestTag` 取**最高分**） | ❌ 见 **R-01** |
| **未纳入日报标记与筛选** | `useBoardArchive.ts:78-92,97-104`；后端 `board_match_handler.go:415-424` | ✅（口径有偏差见 R-07） |
| Scenario 筛选遗漏 / 弱证据筛选 | `BoardArchiveView.vue:378-390`（三 chip）；`useBoardArchive.test.ts:117-132` | ✅ |
| **事件卡 popover**（锚定、Esc/点外可关、窄屏底部档） | `ArchiveMatchPopover.vue:345-360`；`archiveMatchPopover.css:6-33`；`BoardArchiveView.vue:262-282`（锚点=currentTarget） | ✅（遮罩/焦点语义见 R-14） |
| Scenario 解释弱证据（相似度 x.xx / 已降级 ↓ / **弱证据说明**） | `ArchiveMatchPopover.vue:365-372`（rule+score+已降级↓） | ⚠️ 无「弱证据」措辞（R-11） |
| Scenario 方向校验标记（含「会被降级隐藏」说明） | `ArchiveMatchPopover.vue:373`（`方向校验未通过 ⊘`） | ⚠️ 缺后半句（R-11） |
| Scenario 标签描述展开 | `ArchiveMatchPopover.vue:376-384`；后端 `board_match_handler.go:270-272,296`（`tt.description`） | ✅ |
| Scenario 事件辅助×版块收编（✓/✗ + 最相似对 + M/N） | `ArchiveMatchPopover.vue:106-142`（`auxRows`/`auxSummary`） | ✅ |
| **入板过程 popover**（五阶段、纯数据、锚定形态） | `ArchiveMatchPopover.vue:156-172,424-470` | ⚠️ 结构满足，**命中步推导错**（R-01） |
| Scenario 强证据文章判定（①阶段标入板依据、无冗余文字） | `ArchiveMatchPopover.vue:443-452` | ✅ |
| Scenario 各阶段得分分组（空阶段给未命中原因） | `archiveMatchFlow.ts:266-310`（`whyMissText`）；`ArchiveMatchPopover.vue:454-462` | ✅ |
| Scenario 命中即停的阶段语义 | `ArchiveMatchPopover.vue:164`（`skipped`） | ⚠️ 命中步取错（R-01） |
| Scenario 通用匹配流程说明入口 | `BoardArchiveView.vue:396-402,585-615`；`archiveMatchFlow.ts:105-160` | ✅ |
| Scenario 事件私有判定行 | `ArchiveMatchPopover.vue:414-421` | ✅ |
| Scenario **仅弱证据文章判定**（如实标「仅弱证据」，不显示进版块 ✓） | `ArchiveMatchPopover.vue:443-452` —— 弱文章仍给 max_sim/降级标签打「入板依据」，popover 无「仅弱证据」标注 | ❌ 见 **R-03** |
| Scenario **无标签**（灰 chip ✅ ＋ **口径说明「不会进入日报」**） | `BoardArchiveView.vue:521`（灰 chip ✅）；口径说明 `BoardArchiveView.vue:357` 只讲「已入」定义 | ❌ 见 **R-04** |
| Scenario 计算明细就地展开（不渲染到 popover 外） | `ArchiveMatchPopover.vue:429-447` | ✅ |
| Scenario 历史计算缺失如实标注 | `ArchiveMatchPopover.vue:442-446`（取不到时如实标） | ⚠️ 只覆盖「取不到」，未覆盖「取到但实为重算」（R-05） |
| **归档排序**（组间日期倒序 + 组内默认质量可切时间） | 后端 `board_match_handler.go:219-221,330-366`；前端 `useBoardArchive.ts:144-170,247-253` | ❌ 见 **R-02**（跨页失效）、**R-01**（tier 口径打架） |
| Scenario 日期优先 | `useBoardArchive.ts:163-165`（组间 `b.date.localeCompare(a.date)`） | ✅ |
| Scenario 组内质量默认 | — | ❌ 见 **R-02** |
| Scenario 切换时间序 | `useBoardArchive.ts:126-132,247-249`；后端 `:219` | ✅（但每次切换整表重拉，见 R-08） |
| **归档只读边界**（无补进日报 / 重跑 / 改匹配） | 8G 全部调用均为 GET（`getBoardArticles`/`getMatchingConfig`/`getMatchDetail`/`getTagAuxiliaryLabels`/`getComposition`/`getArticle`）；模板内无写操作控件 | ✅ |

### 1.2 `board-article-api` delta（8G.3a）

| Requirement / Scenario | 实现落点 | 判定 |
| --- | --- | --- |
| `filtered_tags` 补 `description`（缺失为空串而非字段缺失） | `board_match_handler.go:270-272`（Select 加 `tt.description`）、`:25-31`（DTO 字段）、`:296`（装配）；前端 `semanticBoards.ts:253-254` | ✅ |
| Scenario 标签描述随标签返回 | `board_article_handler_test.go:240-262`（真断：断言原文 + 空串） | ✅ |
| `in_report` / `weak_evidence` 后端派生，前端只读 | `board/archive.go:60-135`（`LoadReportMembership`）、`:148-163`（`ApplyWeakEvidenceFilter`）；前端 `useBoardArchive.ts:78-92` | ✅（口径偏差见 R-07） |
| Scenario 标记未入日报 / 按遗漏过滤 / 弱证据过滤 | `board_article_handler_test.go:174-238` | ✅ |
| `report_status` 过滤（all/not_in_report/weak） | `board_match_handler.go:490-517` | ✅（后端有能力，**前端未使用**，见 R-07 建议） |

---

## 2. 问题清单

### R-01 【High】「最强匹配」与「入板步号」按**最高分**推导，与后端的**最强证据层级**口径打架

**位置**
- `front/app/features/tags/components/console/BoardArchiveView.vue:224-230`（`strongestTag`：只比 `tag.score`）
- `front/app/features/tags/components/console/BoardArchiveView.vue:232-245`（`strongestView` → 行尾「最强匹配」列 + 「入板」chip 步号）
- `front/app/features/tags/components/console/ArchiveMatchPopover.vue:76-84`（`strongestTag`，同一套逻辑）
- `front/app/features/tags/components/console/ArchiveMatchPopover.vue:156-172`（`stages` → `bestIdx`/`skipped` → 「未走到（第 N 步命中即停）」）
- `front/app/features/tags/components/console/ArchiveMatchPopover.vue:246-249`（article 模式下 `getMatchDetail` 打给「最高分标签」）
- 后端对照：`backend-go/internal/tagmanagement/handler/board_match_handler.go:293-294`（`ArchiveEvidenceRank`，层级优先）、`:330-366`（文章级 `MatchTier`）、`:584-595`（`MatchTier` 定义）

**事实**：仓库里现在有**三套互不一致的「最强」口径**：

| 口径 | 位置 | direct_hit / composite_hit / hit_rate / weighted / max_sim |
| --- | --- | --- |
| `ArchiveEvidenceRank`（组内标签序） | `service/board/archive.go:166-180` | 0 / 1 / 2 / 3 / 4 |
| `MatchTier`（文章质量序） | `board_match_handler.go:584-595` | 0 / **3** / 1 / 3 / 2 |
| 前端「最高分」 | `BoardArchiveView.vue:224` | 只看 `score`，不看层级 |

两点后果：
1. `MatchTier` **不认 composite_hit**（落 default=3，与 weighted 同档），而 `ArchiveEvidenceRank` 给 composite_hit=1（第二强）——同一份数据两套层级。
2. 前端取最高分，与两者都可能不一致。

**可复现的错显**：文章标签 `[direct_hit 0.62, weighted 0.91]`
- 后端：`ArchiveEvidenceRank` 把 direct_hit 排第一（`filtered_tags[0]`）；文章级 `MatchTier=0` → 该文在组内按**强证据**排位。
- 前端：`strongestTag` 选中 weighted(0.91) → 行尾显示「**最强匹配 综合 0.91**」「**入板 ⑤ 加权**」；入板过程 popover 因 `bestIdx=4` 把 ① 阶段标成「未走到（第 ⑤ 步命中即停）」——**而 ① 其实命中了**。
- 这正是 v6/v7 修订想根治的「扫一眼即知本文靠第几步入板块（①=强快速通道、④⑤=弱通道）」被重新打乱。
- composite_hit 文章（score=1.0）同理：行内显示「最强匹配 组合命中 1.00」，却被 `MatchTier` 排到 weighted 同档、组内靠后。

**违反的合同条款**
- delta `workshop-article-archive`「按日期平铺文章」Scenario「标签色彩编码可扫描」：「**最强匹配列 SHALL 以规则色文字+分数展示最强证据**」——展示的不是最强证据。
- delta「入板过程 popover」：「**入板依据标签标记**」「命中步之后的阶段 SHALL 标『未走到（第 N 步命中即停）』」——N 取错。
- ui-design-archive-rework.md「Visual Design」：「最强匹配：行尾独立一列，规则色文字 + 分数……**质量排序时即扫描锚点**」。

**建议**
1. 前端**改用 `filtered_tags[0]`**（后端已按 `ArchiveEvidenceRank` 排好序）作为「最强匹配」与「入板步号」来源——**不新增公式、不复制 BestTier/BestScore**，正好符合合同「BestTier/BestScore 前端不复制」。改动点：`BoardArchiveView.vue:224-245`、`ArchiveMatchPopover.vue:76-84,156-172,246-249`。
2. 后端统一层级口径：`MatchTier` 补 `composite_hit` 分支，或归档文章级排序直接复用 `service.ArchiveEvidenceRank`（`archive.go` 已注释「与 MatchTier 分开维护……避免跨域口径漂移」，正说明这个漂移风险已被预见但未闭环）。
3. 补测试（见 R-10）。

---

### R-02 【High】后端 quality 排序是**页内**排序 → 跨页失效，「组内默认质量排序」在 >1 页窗口不成立

**位置**
- `backend-go/internal/tagmanagement/handler/board_match_handler.go:217-222`：SQL 先 `ORDER BY articles.id ASC` + `OFFSET/LIMIT` **切页**
- `backend-go/internal/tagmanagement/handler/board_match_handler.go:326-366`：内存 `sort.SliceStable(articles, ...)` 在**取页之后**才按 tier/score 排
- `front/app/features/tags/composables/useBoardArchive.ts:207-238`：前端按页序 `collected.push(...)` 拼接 → 全局序 = **页序**，不是质量序
- `front/app/features/tags/composables/useBoardArchive.ts:245-249`：quality 模式刻意不重排（守住了「前端不复制口径」，但也把后端的页内序原样放行）

**事实**：`sort=quality` 时，一页（`per_page=100`）内部按 tier/score 排好，但**页与页之间**是 `articles.id ASC` 的插入序。所以：

- 窗口内 >100 篇（7 天冻结样本 `articles-1974-7d.json` 有 163 篇；`ARCHIVE_PER_PAGE=100`、`ARCHIVE_MAX_PAGES=20`）时，page2 的 direct_hit 文章会排在 page1 的 max_sim 文章之后。
- 同一日期组跨页时，组内顺序 = 页序 → **「一天几十篇强弱混排」的老问题在 8G 重做后依然存在**，而这恰是本次重做的主打卖点之一（v3 问题 3）。

**归属说明（重要）**：主 spec `openspec/specs/board-article-api/spec.md:43` 原文就是「排序在 Go 端**内存中**完成」——页内排序是既有设计，不是 8G 新引入的 bug。但 8G 的 delta **新增**了用户可见承诺「组内 SHALL 默认按质量排序（后端按最佳匹配层级与分数派生的强证据在前）」，把它从「接口实现细节」升格为「归档视图行为合同」。因此闭环责任在 8G 段。

**建议**（择一，前端都不动）
1. **把排序键下推到 SQL**：用 CTE/lateral join 聚合 `topic_tag_board_labels` 求 `best_tier` 与对应 `best_score`，`ORDER BY best_tier, best_score, pub_date DESC` **之后再** `LIMIT/OFFSET`。这是唯一能让「分页 + 全局质量序」同时成立的做法，且顺带修掉主 spec 的既有缺陷。
2. quality 模式改为「先算全量排序键 → 内存排 → 再切页」（窗口大时内存成本上升，需评估）。
3. 若坚持不改后端，则**必须**把 delta 的「组内默认按质量排序」降级/改写（如同 R-01 一并处理），否则合同与实现长期背离。

---

### R-03 【Medium】delta Scenario「仅弱证据文章判定」未满足：popover 无「仅弱证据」标注，还给弱标签打「入板依据」

**位置**
- `front/app/features/tags/components/console/ArchiveMatchPopover.vue:443-452`：`<span v-if="item.id === bestTagId" class="amp-win">入板依据</span>`
- `front/app/features/tags/components/console/ArchiveMatchPopover.vue:420-470`：article 模式模板通篇无「弱证据」字样

**事实**：一篇只带 `max_sim` 或已降级标签的文章（= 弱证据），入板过程 popover 会给那个 `max_sim`/降级标签打上「**入板依据**」徽标，且**没有任何「仅弱证据」提示**。行内虽然有「弱证据」小标（`BoardArchiveView.vue:530`），但 delta Scenario 明确要求的是「**面板** SHALL 如实标注『仅弱证据』，MUST NOT 显示进版块 ✓」——现状正好是该 Scenario 想防的那一面：弱文章在判定面板里看起来像有明确入板依据。

**违反条款**：delta「入板过程 popover」Scenario「仅弱证据文章判定」。

**建议**：article 模式下，若 `article.weak_evidence === true`（或最强标签为 max_sim/降级），在头区显式标「**仅弱证据**」并把该标签的徽标从「入板依据」改为「弱证据命中（不计入强证据）」；强文章才用「入板依据」。

---

### R-04 【Medium】delta Scenario「无标签」后半句未满足：口径说明缺「这类文章不会进入日报」

**位置**
- `front/app/features/tags/components/console/BoardArchiveView.vue:521`（灰 chip「无标签」✅）
- `front/app/features/tags/components/console/BoardArchiveView.vue:355-359`（`ar-window` 口径说明只有「已入 = 出现在范围内任一日报分区线索（后端派生）」）
- `front/app/features/tags/components/console/ArchiveMatchPopover.vue:472-478`（article 模式只写「本文未打标签。」）

**违反条款**：delta「入板过程 popover」Scenario「无标签」——「行内 SHALL 显示『无标签』灰 chip，**文章弹窗/口径说明 SHALL 说明这类文章不会进入日报**」。

**建议**：统计带口径行或「无标签」chip 的 `title` 补一句「无命中标签的文章不会进入日报」；popover 的「本文未打标签」后补同一句。

---

### R-05 【Medium】popover 的「代入式 / 命中数值」是**按当前规则实时重算**，却以历史证据口径呈现（可能展示假等式）

**位置**
- 数据源：`backend-go/internal/tagmanagement/handler/board_match_handler.go:344-392` —— `getTagMatchDetail` 用**当前** auxiliaries + **当前** config **现场重算** `hits/hit_rate/max_similarity/pairs`，但 `score/match_reason/downgraded` 取 DB 冻结值 `stored.Score`（`:373-380`）
- 展示：`front/app/features/tags/components/console/ArchiveMatchPopover.vue:429-447`（规则代入式）、`front/app/features/tags/components/console/archiveMatchFlow.ts:344-360`（`substitutionFormula` / `rateFormula`）

**事实**：`substitutionFormula` 把**重算**出来的 `S_max`/`R` 代入公式，并宣称结果等于**冻结**的 `score`，形如 `score=0.60×0.840+0.40×0.667=0.770`。若重算值与冻结 score 不一致（改过阈值、辅助标签变动、embedding 更新都会导致），页面展示的就是一条**不成立的等式**。

**违反条款**：delta「入板过程 popover」Scenario「历史计算缺失如实标注」——「MUST NOT 拿当前规则计算结果冒充历史明细」。当前实现只覆盖了「match-detail **取不到**」的分支（`ArchiveMatchPopover.vue:442-446`），没覆盖「取到了、但它是当前规则的实时重算」这一更常见的情形。

**建议**（择一）
1. **前端标注（成本最低）**：代入式区与私有判定行加一行「下列 S_max / R / 交集 为**按当前匹配配置实时计算**；score 为匹配时冻结值」；两者不一致时显式提示「重算值与冻结 score 不一致」。
2. **后端补字段**：match-detail 同时返回 `computed_score` 与 `stored_score` + `computed_at`，前端据实展示二者差异（需要动 `board-article-api` spec，成本更高）。

---

### R-06 【Medium】顶栏「文章归档」不再回数据锚点当天（`initialDate` 粘滞）

**位置**
- `front/app/features/tags/components/console/BoardArchiveView.vue:64`：`today = props.initialDate || props.asOf || todayInShanghai()` —— `initialDate` **优先级高于** `asOf`
- `front/app/features/tags/components/console/useConsoleShellActions.ts:50`：`archiveInitialDate.value = date || archiveInitialDate.value`（只写不清）
- `front/app/features/tags/components/console/useConsoleShellOverlays.ts:101`：`archiveInitialDate` **只在切板块**时清空
- `front/app/features/tags/components/console/BoardConsoleHeader.vue:102-109`：顶栏「文章归档」走 `emit('setView','archive')`，**不带日期**

**事实**：用户从矩阵日格点「看归档」（带入日期 D）→ 归档显示 D ✅（这是 review.md R-7 的修复，保留）；但随后切回矩阵、再点顶栏「文章归档」→ `archiveInitialDate` 仍是 D → 归档**继续锁在 D**，而不是数据锚点当天。

**违反条款**：delta「归档视图入口与日期筛选」——「归档 SHALL **默认落在数据锚点当天**」；ui-design-archive-rework.md User Journey 第 1 条「默认数据锚点当天」。

**建议**：顶栏入口显式清 `archiveInitialDate`（`handleSetView` 里对 `view === 'archive'` 且来源是顶栏时置 null），或把 `initialDate` 改成「一次性」语义（进入时消费即清）。

---

### R-07 【Medium】`weak` 口径与 delta 文本不一致（方向校验被算进弱证据）；前后端回退口径也不同

**位置**
- delta 文本：`specs/board-article-api/spec.md` —「`weak` 表示**无匹配标签或匹配规则为 `max_sim` 或已降级**」
- 后端：`backend-go/internal/tagmanagement/service/board/archive.go:148-163`（`ApplyWeakEvidenceFilter`）—— 默认（`includeDirectionMismatch=false`）把**方向未通过**的标签也从「强」里剔除
- 后端：`archive.go:185-188`（`IsWeakEvidence` = `max_sim || downgraded`，**不含**方向校验）—— 后端自身两处就不一致
- 前端：`front/app/features/tags/composables/useBoardArchive.ts:78-92`（`isWeakArticle` 回退分支同样不看 `direction_mismatch`）

**事实**：一篇只带「方向未通过的 direct_hit」标签的文章，按 delta 文本应算 strong，后端默认却给 `weak_evidence=true`（因为该标签默认连 `filtered_tags` 都不展示，用户看到的是「无标签」）。而当用户打开「显示方向不符标签」开关时，后端 `strongEvidence` 又会把方向不符标签算成强 → **同一个文章在开关前后强弱结论翻转**，统计带「弱证据」计数与「弱证据」筛选结果随这个「显示」开关漂移。

**建议**：二选一并写进 spec——
(a) delta 补一句「未通过方向校验的匹配不算强证据」，同时把 `ApplyWeakEvidenceFilter` 与 `IsWeakEvidence` 统一成同一判据；
(b) weak 口径严格按 delta 三条判，方向校验不参与，并把「显示方向不符」降级为纯展示开关（不改 `weak_evidence`）。
另：前端目前**完全没用**后端 `report_status`（`useBoardArchive.ts:200-238` 未传该参数），弱证据筛选是纯前端过滤——这符合 delta「筛选只影响当前文章列表」，但意味着后端的 `report_status=weak` 分支在真实链路里没被走到，只被单测覆盖。

---

### R-08 【Medium】排序切换整表重拉、无缓存（合同写的是「切换仅重排 / 切缓存」）

**位置**
- `front/app/features/tags/composables/useBoardArchive.ts:196-198`（`setSortMode` 改 ref）、`:256`（`watch([..., sortMode])` → `load()` 全量重取）
- `front/app/features/tags/components/console/BoardArchiveView.vue:331-334`（`v-if="loading"` 整块替换为 loading 态）

**事实**：切一次「质量 ↔ 时间」= 重新拉全部页（最多 20 页 × 100 篇），且**整个列表塌成 loading 态**（滚动位置丢失）。ui-design-archive-rework.md「Interaction & States」明确写：「排序切换 | 质量默认；**切换仅重排不重新拉数据**（同窗两序后端各拉一次，**切换切缓存**）」。

**建议**：按 `sortMode` 缓存最近一次结果（`Map<'quality'|'time', ArchiveArticle[]>`），切换命中缓存则不重拉、只重排；同时把 `loading` 与「首次加载」区分开（已有数据时不整块替换）。

---

### R-09 【Medium】筛选覆盖面仍用**原生 `<select>`**——正是本次重做合同点名要清的「三代风格混杂」之一

**位置**
- `front/app/features/tags/components/console/BoardArchiveView.vue:561-578`（`.ar-select select` ×2：来源、构成标签）
- `front/app/features/tags/components/console/boardArchiveView.css:175-184`

**事实**：`ui-design-archive-rework.md` 背景第 1 条把「原生 `<select>/<input type=date>/裸 checkbox`（违反主合同 Component Reuse『不自建第二套控件』）」列为本次要修的**四类问题之一**；Visual Design 要求「控件全部走产品档：AppToggle（排序/方向不符）、AppInput（日期）、**AppSelect 或覆盖面内列表**（来源/标签）」。

现状盘点：
- 日期输入 → 已换 `AppInput` ✅
- 方向不符 → 已换 `AppToggle` ✅
- **来源 / 构成标签 → 仍是原生 `<select>`** ❌
- 排序两档 → 手写 `.ar-seg` 段控（合同写的是 AppToggle，但 `AppToggle` 是布尔开关、做不了两档 —— **合同表述有误，此处不算实现错**）

**说明**：仓库里**没有** `AppSelect`（`front/app/components/ui/` 现有：AppButton / AppDialog / AppDrawer / AppInput / AppPageShell / AppSectionHeader / AppSidebarDrawer / AppToggle / ThemeToggle / Notification*）。所以要么按合同允许的「**覆盖面内列表**」形态落地（AppDialog 内用可点列表替代下拉），要么新建 `AppSelect`（跨组件库改动，需评估是否越出 8G 辖区）。

**建议**：在筛选覆盖面里把两个 `<select>` 换成产品档列表（AppDialog 内可点选项 + 已选项 chip），避免在 8G 段新建公共组件。

---

### R-10 【Medium】新增的「归档排序」Requirement **三个 Scenario 零自动化断言**（测试诚实性）

**位置**
- 后端 `backend-go/internal/tagmanagement/handler/board_article_handler_test.go`：7 个用例（`:174/:193/:216/:229/:240/:264/:273`）**没有一条**断言 `sort=quality` 或 `sort=time` 的顺序；fixture 里 20 篇强证据文章标签同质（`setupArchiveFixture`，`:97-160`），**结构上就测不出跨 tier 顺序**
- 前端 `front/app/features/tags/composables/useBoardArchive.test.ts:89-96`（「quality 模式组内保持后端原序」）：喂进去的数组本就有序，断言的只是「前端不重排」——**不能证明后端产出了质量序**
- 前端 `BoardArchiveView.test.ts:216-226`（「切换组内排序为时间」）：只断言「重拉并带 sort=time」，没断言组内顺序

**后果**：delta「归档排序」的三个 Scenario（日期优先 / 组内质量默认 / 切换时间序）**没有任何自动化证据**，而 R-01 / R-02 两个 High 正好落在这个洞里 —— 单测全绿但功能不成立。

**建议**（补完才算闭环）
- 后端：加一条 fixture —— 混合 `direct_hit / hit_rate / max_sim / downgraded / composite_hit` + 跨日 + **>100 篇跨页**，断言 `sort=quality` 返回的**全局**顺序（含页边界）与 `sort=time` 的 `pub_date DESC, id DESC`。
- 前端：加一条「后端给出跨 tier 质量序 → 组内相对序保持不变」的用例（喂乱序 + 断言组内不重排），以及一条「文章级 tier 与标签级 tier 口径一致」的断言。

---

### R-11 【Low】popover 缺两句合同要求的说明

1. delta Scenario「方向校验标记」要求「**且说明该文会被降级隐藏**」——`ArchiveMatchPopover.vue:373` 只有「方向校验未通过 ⊘」，没有后半句。
2. delta Scenario「解释弱证据」要求「**并给出弱证据说明**」——popover 头部有「相似度 x.xx」「已降级 ↓」，但没有「这是弱证据/建议人工核对」的措辞（`ruleMeta('max_sim').semantics` 里有类似语义，但只用于行内 chip 的 `title`，popover 里没用到）。

**建议**：两处各补一行短说明。

---

### R-12 【Low】「+N」展开后无法收起；展开态不随范围/筛选清理

- `front/app/features/tags/components/console/BoardArchiveView.vue:250-256`（`toggleTags` 双向，但按钮只在 `hiddenTags > 0` 时渲染 → `:513-520`）→ 展开后「+N」消失，**无法收起**
- `expandedIds` 只增不减：换日期范围 / 切筛选 / 切排序后，旧的 article id 仍留在 Set 里（内存与状态卫生问题）

**建议**：展开态改成「折叠/展开」双向按钮（或展开后显示「收起」）；`range`/`filter`/`sortMode` 变化时清空 `expandedIds`。

---

### R-13 【Low】降级 chip 的半透明口径：行内与 popover 不一致，且没用合同说的 color-mix

- 合同：「降级 = 色彩 50% 透明（**color-mix**）」
- `front/app/utils/matchQuality.ts:33-38` 已提供 `matchReasonColor(reason, downgraded)` 的 color-mix 变体
- `front/app/features/tags/components/console/BoardArchiveView.vue:195-197` 调 `matchReasonColor(tag.match_reason)` **不传 downgraded**，改用 `boardArchiveView.css:398-400` 的 `.ar-tag.is-deg { opacity: .55 }`（整 chip 连文字一起降 45%）
- `front/app/features/tags/components/console/ArchiveMatchPopover.vue:90` **传了** downgraded → 同一标签在行内（浅 45%）与 popover（色相减淡 50%）里深浅不一致

**建议**：行内改用 `matchReasonColor(reason, tag.downgraded)`（规则色 50% color-mix），去掉 `opacity`；或两处统一为同一种降级表达。

---

### R-14 【Low】popover 用全屏透明遮罩拦截整页交互 + 缺无障碍语义

- `front/app/features/tags/components/console/archiveMatchPopover.css:6-11`（`.amp-mask` fixed + inset:0）
- `front/app/features/tags/components/console/ArchiveMatchPopover.vue:348`（遮罩盖住整页）、`:352-356`（`role="dialog"` 无 `aria-modal` / 无 `aria-labelledby`）

后果：popover 打开时**整页不可点**（连回到原 chip 再点一次都不行），而锚定 popover 的语义本应是「轻量浮层 + 点外关闭」。关闭时也没有把焦点交回触发 chip（`useConsoleShellOverlays.ts:46-52` 的 `articleFocus` 兜的是 ArticlePreviewModal，不是本 popover）。

**建议**：改为事件委托「点外关闭」（不铺遮罩），或保留遮罩但补 `aria-hidden="true"` + `aria-modal="false"` + 关闭后 `anchor?.focus()`。

---

### R-15 【Low】chip 提示文案：无条件补「…」，描述为空时拿规则语义冒充描述

- `front/app/features/tags/components/console/BoardArchiveView.vue:204-207`：`const summary = (tag.description || meta.semantics).slice(0, 60)` + 固定拼 `…`
- 结果：描述只有 10 个字也显示「短描述…（点击展开完整描述…）」；`description` 为空时提示里出现的是**规则语义**（如「标签的辅助标签与版块构成对得上，证据强…」）而非描述，读者会误以为是标签描述

**建议**：有 description 才截断并补省略号；无 description 时只显示规则 + 分数 + 「（暂无描述）」。

---

### R-16 【Low】`asOf` 未到达时先打一次「今天」窗口的请求，且可能重置用户已改的日期

- `front/app/features/tags/components/console/BoardArchiveView.vue:64,76-79`：`asOf` 为空时 `today` 兜底 `todayInShanghai()`；`asOf` 异步到达后 `watch(today)` 把起止重置为锚点日 → 多一次错窗口请求 + 列表闪烁；若用户在锚点到达前手改过日期，改动被静默重置
- 触发条件：`BoardConsoleStage.vue:102` 的 `latestReportDate || anchorDate` 中 `anchorDate` 可为 null（`consoleStageTypes.ts:23`）

**建议**：`asOf` 为空时不发请求（等锚点），或起止只在锚点**首次到达**时初始化一次，之后不再被 `watch(today)` 覆盖。

---

### R-17 【Low】「构成标签」筛选的语义未在 UI 说明（辅助标签 ≠ topic_tag 挂载）

- `front/app/features/tags/components/console/BoardArchiveView.vue:569-576`（选项来自 `labels` prop = 版块 `board_composition` 挂载的**辅助标签**）
- 后端语义：`board_match_handler.go:207-210` —— `auxiliary_label_id` 过滤的是「文章命中了**挂载了该辅助标签的 topic_tag**」
- 而归档视图里被展示/排序的「命中标签」是 `topic_tag_board_labels`（topic_tag 层）。两者不是同一层，界面无任何解释

**建议**：在筛选项旁加一行说明，或把选项标题改为「构成辅助标签」。

---

### R-18 【Low】`main.css` 整文件行尾 CRLF → LF，功能改动被 diff 淹没

- `front/app/assets/css/main.css`：真实功能改动 **19 行**（`--color-match-composite` 明暗两套 ×2、`--font-serif`、`html/body` 底色），但因整文件行尾从 CRLF 改成 LF，`git diff --stat` 显示 **1495 行变更**（890+/739-）
- 这会显著抬高该文件的 review/回滚成本，也容易让后续真改动被漏看

**建议**：把行尾改回 CRLF（或在 `.gitattributes` 统一），让 8G 的真实改动只剩 19 行。

---

### R-19 【Low】time 模式下前端重复排序（冗余，低危）

`front/app/features/tags/composables/useBoardArchive.ts:247-249`（`orderedArticles` 在 time 模式全量重排）与 `:165-172`（`groupArchiveArticles` 内又按组重排一次）；而后端 `board_match_handler.go:219` 的 `sort=time` 已在 SQL 里全局排好。功能无误，但同一份排序做了两遍，且让「组内保持后端原序」的口径在前端看起来不统一。

**建议**：time 模式直接信任后端序（后端已全局有序），前端只做分组，不重排。

---

## 3. 可归档性结论（仅评价 8G 段）

### 3.1 判定

**8G 归档视图重做段：暂不可归档（Not archivable as-is）。**

### 3.2 理由

1. **两个 High 都在打 8G 段自己的卖点上**：
   - R-01 让「四色 + 最强匹配 + 入板步号」这套新语言在真实数据上**自相矛盾**——行内可能显示「入板⑤加权（弱通道）」，而该文其实是靠 ① 直接命中进板的；这正是 v3 问题 2/3 与 v6/v7 修订反复要治的病。
   - R-02 让「组内默认质量排序（强证据在前）」在窗口 >100 篇时**不成立**——重做主打卖点之一落空。
   - 两者叠加的观感就是「同一个视图里强的排在后面、最强匹配显示的是弱通道」，与「报刊风、可扫描」的设计目标相反。
2. **3 个 delta Scenario 未满足**：仅弱证据文章判定（R-03）、无标签口径说明（R-04）、历史计算缺失如实标注只覆盖一半（R-05）。
3. **新增的「归档排序」Requirement 零自动化断言**（R-10）——本轮单测全绿，但绿灯覆盖不到这次新增的核心行为，因此「已验证」这一说法目前**不成立**。

### 3.3 归档前建议的最小收口清单

| 优先级 | 项 | 说明 |
| --- | --- | --- |
| **必做** | R-01 | 前端改用 `filtered_tags[0]` 作为最强匹配/入板步号来源（不复制公式）；后端统一 `MatchTier` 与 `ArchiveEvidenceRank` 的 composite_hit 层级 |
| **必做** | R-02 | 排序键下推到 SQL（`ORDER BY` 后再 `LIMIT/OFFSET`），或明确改写 delta 降级该承诺 |
| **必做** | R-10 | 补后端跨 tier / 跨页顺序断言 + 前端「不重排」断言 |
| **必做** | R-03 / R-04 | 补「仅弱证据」标注与「无标签不会进入日报」口径说明（两条 delta Scenario） |
| **强烈建议** | R-05 / R-06 / R-07 | 代入式标注「实时重算」；顶栏归档回锚点当天；weak 口径与 delta 对齐 |
| **建议** | R-08 / R-09 | 排序切缓存；筛选覆盖面去原生 `<select>` |
| **可延后** | R-11 ~ R-19 | 均为文案 / 一致性 / 冗余级，不阻塞归档，但 R-13、R-18 建议顺手带上（前者影响观感一致性，后者影响后续 review 成本） |

### 3.4 对整 change 的影响评估

- 本报告只覆盖 8G 段，**不对整 change 判归档**。
- 但 R-01/R-02 涉及的排序键下推（R-02 建议方案 1）会改动 `board-article-api` 的既有行为（主 spec「排序在 Go 端内存中完成」），属于**跨 spec 的口径变更**，建议：
  - 走 `board-article-api` 的 delta（哪怕只是把「内存排序」改成「SQL 排序」），不要在 8G 段里静默改行为；
  - 或者接受「主 spec 现状 = 8G 现状」，把 workshop-article-archive delta 的「组内默认按质量排序」改写为「组内按后端返回序（页内质量序）」并向用户说明限制。
- R-06（顶栏归档回锚点）与 R-07（weak 口径）会影响 Shell / 后端 `archive.go`，建议在 8G 段内一并收口，避免留到归档后再改。

### 3.5 已确认无回归的部分（供归档核查直接引用）

- `MatchDetailPanel.vue` / `BoardTimelinePanel.vue`（旧证据抽屉宿主）**本轮零改动**（`git diff --stat` 为空），旧页面不受影响。
- 新归档视图**不再挂载** MatchDetailPanel（`grep` 全仓仅 `BoardTimelinePanel.vue:201` 引用），FB-3「渲染在抽屉外、掉出视口」的宿主问题随组件退役消失。
- `ArticlePreviewModal` 挂载契约未破坏：`BoardConsoleShell.vue:435` 未改，归档行 → `open-article` → `useConsoleShellOverlays.ts:46-52` → `openArticlePreview(id)` 链路完整，focus 归还逻辑在 Shell 侧未被本段改动影响。
- 归档**全链路只读**：本段调用的 6 个接口全为 GET；模板内无补进日报 / 重跑 / 改匹配动作（delta「归档只读边界」满足）。
- 自动化现状：`front` 4 文件 60 用例绿、`backend-go/internal/tagmanagement/...` 全绿（见 §0）。
