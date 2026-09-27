# Test Cases — add-source-board-hit-rate（复杂档白盒用例）

<!-- 对应 proposal.md 头部 complexity: complex；本文件是 case-first-testing（docs/reference/开发执行规范.md §2）复杂档要求的白盒用例清单。

用途：把「源 × 板块命中统计」的口径与端点契约机械展开为可判定分支，覆盖两份 delta spec 的全部 28 个 Scenario。
本文件只做机械枚举，不写实现、不写测试代码；实现阶段逐条落成断言，命名若与建议缝不一致须回改本文件。

判据来源：proposal.md 三条硬约束 + design.md D1–D7 + 2026-09-18 真库实测快照（docs/research/source-board-hit-rate/explore-findings.md）。 -->

## 0. 判据来源与落点

| 判据来源 | 取用内容 |
| --- | --- |
| proposal 硬约束 1 | 分母按文章去重（错算实证：1662 → 5193） |
| proposal 硬约束 2 | 窗口含已归档（实证：1671 篇里 1574 篇归档，437 篇命中全在归档区） |
| proposal 硬约束 3 | 必须限窗口（老文章打标覆盖 15%：20035 篇仅 3074 篇有标签） |
| design.md D2 | Go 侧算 cutoff 传参；`COUNT(DISTINCT)` + `SUM(CASE WHEN …)`；`EXISTS` 而非 JOIN |
| design.md D3 | 未打标两分取 `models.JobStatusPending/Leased` |
| design.md D4 | 窗口白名单 {7,30,90}，非法 400；`{success, data}` 信封；板块 404 |
| ui-design.md State Matrix | 指标 pill 三态 / 样本少 / 无新文 / 打标关闭 / 加载失败重试 |

**落点**：

| 组 | 层 | 测试文件（建议） |
| --- | --- | --- |
| B1–B3 | 聚合服务（sqlite 内存库） | `backend-go/internal/tagmanagement/service/sourcestats/sourcestats_test.go` |
| B4–B5 | 端点（httptest + gin） | `backend-go/internal/reader/handler/feed_board_stats_handler_test.go`、`backend-go/internal/tagmanagement/handler/board_source_breakdown_handler_test.go` |
| B6 | 前端组件 | `front/app/features/settings/components/FeedSourceQualityBlock.test.ts`、`FeedMasterList.test.ts`、`front/app/features/tags/components/BoardSourcePanel.test.ts` |
| B7 | 真库不变量抽检 | `backend-go/internal/tagmanagement/service/sourcestats/sourcestats_pg_test.go`（`database.InitDB` 既有范式，`-short` 下 skip） |

## 1. 前置实现契约（白盒用例依赖的缝）

| 缝 | 语义（固定） | 建议名 |
| --- | --- | --- |
| 源视角聚合 | 输入 (ctx, db, windowDays)，输出按 feed_id 的记录集合，含 `articles/in_board/tagged_no_board/untagged_pending/untagged_settled/hit_rate/tagging_enabled/boards[]` | `sourcestats.FeedBoardHitStats` |
| 板块视角聚合 | 输入 (ctx, db, boardID, windowDays)，输出 `{total_articles, source_count, sources[]}`，每条含 `feed_id/title/articles/share/feed_articles/feed_hit_rate` | `sourcestats.BoardSourceBreakdown` |
| 窗口解析 | 白名单 {7,30,90}，缺省 7，非法 → 参数错误（handler 映射 400）；同一解析函数供两个端点复用 | `sourcestats.ParseWindow(raw string) (int, error)` |
| 截止时间 | Go 侧计算 `time.Now().AddDate(0,0,-windowDays)`，作为 SQL 参数；测试可注入固定 now | 聚合函数参数或包内可替换 clock |
| 命中判定（唯一口径） | 文章 ≥1 标签经 `topic_tag_board_labels` 挂到 `semantic_labels.label_type='board' AND status='active'` | SQL 片段（单处定义） |

**掩蔽契约**：窗口下界比较用 `>=`（含边界时刻当天 00:00 的文章计入）。

## 2. B1 — 命中口径与去重

| ID | 输入 | 期望 |
| --- | --- | --- |
| TC-B1-01 | 文章 A 有 3 个标签，其中 2 个分别挂到 active 板块 P、Q | `in_board` 计 1；`boards[P]=1`、`boards[Q]=1`（合计 2 > `in_board` 1） |
| TC-B1-02 | 文章 A 有 3 个标签，全部只挂到同一板块 P | `in_board` 计 1；`boards[P]=1` |
| TC-B1-03 | 文章 A 的标签只挂到 `status='disabled'` 的板块 | 不计命中；归入 `tagged_no_board` |
| TC-B1-04 | 文章 A 的标签只挂到 `label_type='auxiliary'` / `'composite'` 的记录 | 不计命中（辅助/组合标签不是板块） |
| TC-B1-05 | 文章 A 有标签但 `topic_tag_board_labels` 无行 | 归入 `tagged_no_board`，不计命中 |
| TC-B1-06 | 文章 A 无标签 | 归入 `untagged_pending` 或 `untagged_settled`（按任务状态，见 B2） |
| TC-B1-07 | 窗口内 100 篇，其中 80 篇 `archived=true`，仅 5 篇命中且全在归档区 | `articles=100`、`in_board=5`、`hit_rate=0.05`（MUST NOT 为 0） |
| TC-B1-08 | 源窗口内 0 篇 | 该源仍出现在结果中：`articles=0`、`in_board=0`、`hit_rate=0`、`boards=[]` |
| TC-B1-09 | 文章 `pub_date` 为 NULL、`created_at` 在窗口内 | 该文章计入（用 `coalesce(pub_date, created_at)`） |
| TC-B1-10 | 文章 `pub_date` 在窗口外、`created_at` 在窗口内 | `coalesce` 取 `pub_date` → 不计入窗口（口径固定，不加或条件） |
| TC-B1-11 | 文章 `pub_date` 恰为窗口下界时刻 | 计入（`>=` 语义） |
| TC-B1-12 | 文章命中 2 个板块且 `boards[]` 求和 | 断言 SUM(boards.articles) ≥ in_board（不变量），且各板块计数为去重文章数 |

## 3. B2 — 未打标两分

| ID | 输入 | 期望 |
| --- | --- | --- |
| TC-B2-01 | 文章无标签，存在 `tag_jobs.status='pending'` 的行 | `untagged_pending += 1`，`untagged_settled` 不变 |
| TC-B2-02 | 文章无标签，存在 `tag_jobs.status='leased'` 的行 | `untagged_pending += 1` |
| TC-B2-03 | 文章无标签，`tag_jobs.status='completed'` | `untagged_settled += 1` |
| TC-B2-04 | 文章无标签，`tag_jobs.status='failed'` | `untagged_settled += 1`（失败任务不再占用"排队中"） |
| TC-B2-05 | 文章无标签且无任何 `tag_jobs` 行（源关闭打标） | `untagged_settled += 1`；该源 `tagging_enabled=false` |
| TC-B2-06 | 文章无标签，同时存在 `pending` 与 `completed` 两条任务行 | `untagged_pending += 1`（取存在未完成任务），计数不重复（`EXISTS` 语义） |
| TC-B2-07 | 文章有标签 | 两个 untagged 计数都不增加 |
| TC-B2-08 | 恒等式检查 | 对每个源：`articles == in_board + tagged_no_board + untagged_pending + untagged_settled` |

## 4. B3 — 窗口参数与边界

| ID | 输入 | 期望 |
| --- | --- | --- |
| TC-B3-01 | `window=7` | 只统计 `coalesce(pub_date,created_at) >= now-7d` 的文章 |
| TC-B3-02 | `window=30` / `window=90` | 同上按对应天数 |
| TC-B3-03 | 不传 `window` | 等同 `window=7` |
| TC-B3-04 | `window=14` / `window=0` / `window=abc` | 参数错误（端点 400），**不得**静默回退默认值 |
| TC-B3-05 | `window=-7` | 参数错误（400） |
| TC-B3-06 | 7 天窗口与 30 天窗口同源对比 | `articles(30d) >= articles(7d)`（窗口单调不变量） |
| TC-B3-07 | 注入固定 now，文章时间恰在 7 天前 1 秒 | 不计入；恰在 7 天前 0 秒（边界）→ 计入（`>=`） |

## 5. B4 — 源视角端点契约

| ID | 输入 | 期望 |
| --- | --- | --- |
| TC-B4-01 | `GET /api/feeds/board-hit-stats?window=7`，库内 3 个源 | 响应 `success=true`，`data.items` 长度 3 |
| TC-B4-02 | 同请求 | 每条含 `feed_id/title/tagging_enabled/articles/in_board/tagged_no_board/untagged_pending/untagged_settled/hit_rate/boards` 全字段 |
| TC-B4-03 | 源窗口内 67 篇、命中 56 | `hit_rate == 56/67`（浮点误差内），`boards` 为数组（可为空） |
| TC-B4-04 | 源 `tagging_enabled=false` | 记录照常返回且 `tagging_enabled=false`（后端不过滤、不特殊处理） |
| TC-B4-05 | 连续调用两次同一窗口 | 两响应除时间戳外一致；库内 `tag_jobs` 行数与 `feeds` 配置不变（只读无副作用） |
| TC-B4-06 | 库内无任何文章 | 响应 `items` 含全部源且 `articles=0`（不是空数组） |
| TC-B4-07 | 端点与既有 `GET /api/feeds/:feed_id` 路由共存 | 两者均可访问（静态段与参数段不冲突），`/api/feeds/board-hit-stats` 不被当作 feed_id |

## 6. B5 — 板块视角端点契约

| ID | 输入 | 期望 |
| --- | --- | --- |
| TC-B5-01 | 板块 #5 窗口内命中 437 篇、8 个源 | `total_articles=437`、`source_count=8`、`sources[].articles` 之和 = 437 |
| TC-B5-02 | 同请求 | 每条含 `feed_id/title/articles/share/feed_articles/feed_hit_rate`，`share == articles/total_articles` |
| TC-B5-03 | 源在板块内 12 篇，其窗口内总量 162、整体命中率 44% | `feed_articles=162`、`feed_hit_rate≈0.44` |
| TC-B5-04 | 请求不存在的板块 id | 404 |
| TC-B5-05 | 请求 `label_type='auxiliary'` 的 id | 404（非 board 类型） |
| TC-B5-06 | 板块窗口内无命中文章 | `total_articles=0`、`source_count=0`、`sources=[]`，HTTP 200（非错误） |
| TC-B5-07 | `window=abc` | 400 |
| TC-B5-08 | 同一文章命中板块 #5 与 #8 | 在两个板块的来源统计中各计一次（各自去重后为 1） |

## 7. B6 — 前端状态与交互

| ID | 输入 | 期望 |
| --- | --- | --- |
| TC-B6-01 | 源 7 天 67 篇 / 命中 56 | 列表 pill 文案「84%」，类名/样式为高指标档 |
| TC-B6-02 | 源 7 天 1662 篇 / 命中 437 | pill 文案「26%」，低指标档 |
| TC-B6-03 | 源 7 天 499 篇 / 命中 160 | pill 文案「32%」，中指标档（边界 30% 取中档、60% 取高/中按 spec 三分取上界包含） |
| TC-B6-04 | 源 7 天 3 篇 | pill 文案「样本少」，DOM 内不含 `%` |
| TC-B6-05 | 源 7 天 0 篇 | pill 文案「无新文」 |
| TC-B6-06 | 源 `tagging_enabled=false` | pill 文案「打标关闭」，不含百分比 |
| TC-B6-07 | 聚合请求 pending | pill 显示「—」占位（宽度与数字态一致，无布局抖动） |
| TC-B6-08 | 聚合请求失败 | pill 保持「—」，不抛错、不影响列表其它信息 |
| TC-B6-09 | 点「分类内 · 入板块率升序」 | 分类分组结构不变；分类内顺序为率升序；0 篇源排在末尾 |
| TC-B6-10 | 点「分类内 · 篇数降序」 | 分类内按窗口篇数降序 |
| TC-B6-11 | 点「分类内 · 杂音量降序」 | 分类内按 `articles - in_board` 降序 |
| TC-B6-12 | 开「只看低命中」 | 仅保留 篇数 ≥5 且率 <30% 且 `tagging_enabled=true` 的源；空分类隐藏 |
| TC-B6-13 | 开筛选后无匹配 | 显示空结果提示与复位入口；复位后恢复全量列表 |
| TC-B6-14 | 列表切窗口 30 天 | 详情块窗口标签同步为 30 天（同一状态源） |
| TC-B6-15 | 详情块渲染 1662/437/739/486 | 堆叠条四段宽度比例正确、图例文案含四个数值、率文案「26.3%（437 / 1662）」 |
| TC-B6-16 | 板块分布 8 个 | 显示 Top 6 chips + 「其他 2 个板块」 |
| TC-B6-17 | 详情块窗口内 0 篇 | 显示「近 7 天没有新文章」，不渲染堆叠条 |
| TC-B6-18 | 详情块窗口内 3 篇 | 显示「样本不足（3 篇）」 |
| TC-B6-19 | 源 `tagging_enabled=false` | 块内文案「该源已关闭打标，不参与板块归属」，不含 0% |
| TC-B6-20 | 块内统计失败 | 块内「统计加载失败」+ 「重试」；点击重试重新发请求；设置表单元素仍可交互 |
| TC-B6-21 | `/tags` 板块页「板块内容」tab | tab 栏五项不变（无「来源」tab）；面板位于板块构成之后；快速切换板块后表格来源与当前板块一致（竞态不覆盖）；切到其它 tab 面板卸载 |
| TC-B6-22 | 板块来源面板渲染 | 汇总行含本板块篇数/来源数/最大来源；表格 5 列齐全；行内无任何按钮动作 |
| TC-B6-23 | 板块来源面板空态 | 显示「近 7 天没有文章归入本板块」+ 切窗口提示 |
| TC-B6-24 | 面板排序切「按该源入板块率 ↑」 | 行顺序按 `feed_hit_rate` 升序 |
| TC-B6-25 | 面板统计失败 | 显示失败提示与「重试」；不影响上方板块构成面板；切换其它 tab 后再切回仍正常 |

## 8. B7 — 真库不变量抽检（对照实测快照，不冻结数字）

| ID | 断言 | 期望 |
| --- | --- | --- |
| TC-B7-01 | 全源恒等式 | 每个源满足 `articles == in_board + tagged_no_board + untagged_pending + untagged_settled` |
| TC-B7-02 | 去重不变量 | 每个源 `SUM(boards[].articles) >= in_board`，且 `SUM(boards[].articles)` 等于「按板块计次」的命中篇数 |
| TC-B7-03 | 归档口径 | 取任一近 7 天含归档文章的源（现网：华尔街见闻-实时快讯-要闻），其 `in_board > 0`——若实现误加 `archived=false` 过滤，该断言必失败 |
| TC-B7-04 | 板块合计 | 对任一 board，`SUM(sources[].articles) == total_articles` |
| TC-B7-05 | 窗口单调 | 同一源 `articles(7d) <= articles(30d) <= articles(90d)` |
| TC-B7-06 | 人工快照（一次性核对，不入自动化） | 2026-09-18 真库 7 天窗口：华尔街见闻-实时快讯-要闻 1662/437/739/486（率 26.3%）、凤凰网 499/160/268/64、最热文章 67/56/7/4、V2EX-技术 162/71/92/0 |

## 9. Scenario → 用例覆盖映射

| delta spec | Scenario | 覆盖用例 |
| --- | --- | --- |
| source-board-hit-rate | 一篇文章多标签多板块只计一次 | TC-B1-01、TC-B1-02 |
| source-board-hit-rate | 归档文章计入窗口统计 | TC-B1-07、TC-B7-03 |
| source-board-hit-rate | 非法窗口值被拒绝 | TC-B3-04、TC-B3-05、TC-B5-07 |
| source-board-hit-rate | 打标未完成与零标签可区分 | TC-B2-01..05 |
| source-board-hit-rate | 返回全量源的统计 | TC-B4-01、TC-B4-06 |
| source-board-hit-rate | 板块分布可超过命中数 | TC-B1-01、TC-B1-12、TC-B4-03 |
| source-board-hit-rate | 只读端点无副作用 | TC-B4-05 |
| source-board-hit-rate | 来源篇数合计等于板块总数 | TC-B5-01、TC-B7-04 |
| source-board-hit-rate | 上下文列帮助识别只偶尔命中的源 | TC-B5-03 |
| source-board-hit-rate | 板块不存在 | TC-B5-04、TC-B5-05 |
| source-board-hit-rate | 空板块 | TC-B5-06、TC-B6-23 |
| source-board-hit-rate | 显示来源构成 | TC-B6-22 |
| source-board-hit-rate | 切换板块重新拉取 | TC-B6-21 |
| source-board-hit-rate | 空态引导切窗口 | TC-B6-23 |
| source-board-hit-rate | 面板无写动作 | TC-B6-22 |
| feed-settings-ui | 高命中源显示高指标 | TC-B6-01 |
| feed-settings-ui | 高流量低命中源显示低指标 | TC-B6-02 |
| feed-settings-ui | 样本不足不显示百分比 | TC-B6-04 |
| feed-settings-ui | 打标关闭的源不显示 0% | TC-B6-06、TC-B6-19 |
| feed-settings-ui | 排序在分类内生效 | TC-B6-09、TC-B6-10、TC-B6-11 |
| feed-settings-ui | 窗口切换双向同步 | TC-B6-14 |
| feed-settings-ui | 筛出杂音源 | TC-B6-12 |
| feed-settings-ui | 打标关闭源不被判为杂音 | TC-B6-12 |
| feed-settings-ui | 筛选无结果可复位 | TC-B6-13 |
| feed-settings-ui | 展示三分解与率 | TC-B6-15 |
| feed-settings-ui | 展示板块分布 | TC-B6-16 |
| feed-settings-ui | 打标关闭不等于零命中 | TC-B6-19 |
| feed-settings-ui | 统计失败不阻断设置 | TC-B6-20、TC-B6-25 |
