# test-cases: replace-lane-lifeline-with-topic-events

> 复杂档（`complexity: complex`）。故事层账本；方法级单测允许，但交付验收锚在本文的主链路落点。
> 锚 Requirement：`thread-related-events`（线索相关事件读模型 / 候选集与相似度打分 / 排序与截断 / 来源文章与已确认标记 / 缺失向量的处置 / 推荐读模型只读）；`daily-report-system`（话题卡片段落）。

## 故事 S1: 我读日报时想知道这条线之前讲过什么

**用户**：日报读者。**场景**：展开「关心的话题」，读到某条线索。**期望**：线索下方有一两个 chip 提示这是有来头的；悬停能判断值不值得点；点开直接读原文；没来头的线索下面什么都没有。

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 请求 `days=7`，日报 `period_date=2026-11-17` | 查询 7 天窗口 | `window_start=2026-11-10`、`window_end=2026-11-16`；当期线索不作来源 | repository（PG） | `thread_relations_pg_test.go` |
| 2 | 候选与当前线索余弦 0.890 | 同话题高相似线索入选 | 入选，`score=0.890`、`level=strong` | repository（PG） | `thread_relations_pg_test.go` |
| 3 | 候选余弦 0.72 | 低于阈值不入选 | 不出现 | repository（PG） | `thread_relations_pg_test.go` |
| 4 | 两条候选 0.86（旧）/ 0.80（新） | 分数不做二次加权 | score 分别为 0.86 / 0.80，不被时间改写 | repository（PG） | `thread_relations_pg_test.go` |
| 5 | 两条候选内容高度相似 | 不做语义归并 | 各自成条 | repository（PG） | `thread_relations_pg_test.go` |
| 6 | 两条 score 与日期都相同 | 稳定排序 | 按 `thread_id` 升序；两次查询等价 | repository（PG） | `thread_relations_pg_test.go` |
| 7 | 某线索 5 条合格候选 | 全部返回 | 响应含 5 条（后端不截断） | repository（PG） | `thread_relations_pg_test.go` |
| 8 | 候选的 event 标签在本话题有 confirmed 注记 | 已确认线索加星 | `confirmed=true` | repository（PG） | `thread_relations_pg_test.go` |
| 9 | 只有 suggested / dismissed 注记 | 只有 AI 建议不加星 | `confirmed=false` | repository（PG） | `thread_relations_pg_test.go` |
| 10 | 候选无 `related_article_ids` 或首篇已删 | 来源文章不可用 | `source_article=null`，标题摘要仍返回 | repository（PG） | `thread_relations_pg_test.go` |
| 11 | 当期某线索无 `embedding` | 当期线索无向量 | 该线索不进 `threads`；`skipped_no_embedding>0`、`status=partial` | repository（PG） | `thread_relations_pg_test.go` |
| 12 | 窗口内某历史线索无 `embedding` | 历史线索无向量不进候选 | 它不是任何线索的推荐项 | repository（PG） | `thread_relations_pg_test.go` |
| 13 | 前端展开话题卡片 | 线索行渲染相关事件 chip | 每条线索最多 3 chip +「还有 N 条」；整份日报只发 1 次请求 | 组件 | `ThreadRelatedEvents.test.ts` + `DailyReportTopicSection.test.ts` |
| 14 | 某线索无合格候选 | 无相关事件的线索不渲染 chip 行 | DOM 无 chip 容器、无占位、无空态文案 | 组件 | `DailyReportTopicSection.test.ts` |
| 15 | 悬停 / Tab 聚焦 chip | chip 悬停出预览 | 浮层含标题、摘要、来源文章、档位+分数、日期 | 组件 | `ThreadRelatedEvents.test.ts` |
| 16 | 点击 chip | 点击 chip 打开阅读预览 | emit 打开文章事件（带 articleId + reportId）；路由不变 | 组件 | `ThreadRelatedEvents.test.ts` |
| 17 | 7 天切 30 天再切回 | 7/30 档位切换与缓存 | 切 30 天请求一次；切回命中缓存 | composable | `useThreadRelatedEvents.test.ts` |
| 18 | 请求失败 | 推荐加载失败不阻断阅读 | chip 行不渲染；正文/文章/关键事件动作照常 | 组件 | `ThreadRelatedEvents.test.ts` |
| 19 | 正文列 690–900px + 长标题 | 窄容器排版不崩坏 | 无横向溢出；换行自然；无 `gridColumn: N / span 2` | 组件 + agent-browser | `DailyReportEvents.layout.test.ts` |
| 20 | 不存在的 report | 日报不存在 | 404 | handler | `thread_relations_handler_test.go` |
| 21 | 全无达标候选 | 无任何推荐 | `threads={}`（非 null）、`status=empty` | handler | `thread_relations_handler_test.go` |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入·`days=` 空串 | 回落 7 并回显 `days=7` | handler | `thread_relations_handler_test.go` |
| 2 | 输入·`days=30` | 合法，窗口 30 天 | handler | 同上 |
| 3 | 输入·`days=14` / `365` / `abc` / `0` / `-1` | 一律回落 7，不报 400 | handler | 同上 |
| 4 | 前置·空集（当期无 thread） | `threads={}`、`status=empty`、200 | handler | 同上 |
| 5 | 前置·单元素（1 条合格候选） | 1 个 chip，无「还有 N 条」（`related` 档浮层写「相关」） | 组件 | `ThreadRelatedEvents.test.ts` |
| 6 | 前置·重复元素（同候选出现在多期？不可能） | 按 `thread_id` 去重，不重复出现 | repository | `thread_relations_pg_test.go` |
| 7 | 前置·越界引用（reportId 不存在） | 404 | handler | `thread_relations_handler_test.go` |
| 8 | 前置·部分满足（当期部分线索缺向量） | 有向量的正常出候选；`status=partial` | repository | `thread_relations_pg_test.go` |
| 9 | 时间·窗口边界：`report_date-7` 含；`report_date` **不含** | 左闭右开（不含当期） | repository | 同上 |
| 10 | 时间·空窗口（窗口内无其它 section） | 无候选；全无则 `threads={}` | repository + handler | 两个后端测试 |
| 11 | 时间·跨窗口（第 8 天的候选，7 天档看不到、30 天档看得到） | 7 天档无、30 天档有 | repository | `thread_relations_pg_test.go` |
| 12 | 幂等·重复执行两次 | 响应等价、DB 无写 | repository | 同上 |
| 13 | 幂等·部分失败后重试 | entry 置 error；重试成功后覆盖 | composable | `useThreadRelatedEvents.test.ts` |
| 14 | 幂等·并发（同 key 两次 ensure） | 只发一次，共享同一 promise | composable | 同上 |
| 15 | 可用性·加载态 | **不渲染骨架占位**（实施决策，见下「与 ui-design 状态矩阵的差异」）：首载时无法预知是否有候选，骨架会在多数无线索的行上闪现后消失 | 组件 | `ThreadRelatedEvents.test.ts` |
| 16 | 可用性·错误态 | 整行不渲染（不阻断阅读） | 组件 | 同上 |
| 17 | 可用性·空态 | 整行不渲染、无文案 | 组件 | 同上 |
| 18 | 可用性·超长文本 | chip 单行 ellipsis + `title` 全文；浮层摘要 2 行 clamp | 组件 | 同上 |
| 19 | 可用性·重复提交（连点档位） | 同 key 只一次；迟到响应按 key 归属不串档 | composable | `useThreadRelatedEvents.test.ts` |
| 20 | 可用性·误输入反馈 | 本视图无自由文本输入 —— 不适用（划除留痕） | — | — |
| 21 | 前置·列表长度 0 / 1 / 3 / 4 / 20（前端） | 0 = 整行不渲染；1–3 = 逐个 chip 无「还有」；4/20 = 3 个 chip +「还有 1 条 / 17 条」，展开后补齐全部且按钮消失 | 组件 | `ThreadRelatedEvents.test.ts` |
| 22 | 前置·悬空来源（`source_article=null`） | chip 仍渲染（`cursor: default` + `tre__chip--static`），点击不 emit、不弹空弹窗 | 组件 | `ThreadRelatedEvents.test.ts` |
| 23 | 前置·同一候选出现在多条当期线索下（thread 身份不合并） | 各线索各自的 chip 组独立渲染，行为不互相影响 | 组件（数据层由 repository 覆盖） | `ThreadRelatedEvents.test.ts`（`threads` 只有别的线索 → 本线索不渲染） |
| 24 | 时间·7 天档看不到、30 天档看得到（前端档位） | 切 30 天 emit `changeDays`；切回 7 天命中缓存不再请求；两档数据互不污染 | composable | `useThreadRelatedEvents.test.ts` |
| 25 | 可用性·键盘 Esc | 只关浮层、线索行与焦点不动；再聚焦仍能打开 | 组件 | `ThreadRelatedEvents.test.ts` |
| 26 | 可用性·浮层裁剪 | 浮层 Teleport 到 `body` + `position: fixed` + 视口宽度上限，不被线索行 `overflow` 裁剪 | 组件 | `ThreadRelatedEvents.test.ts`「展示合理性锚」 |
| 27 | 布局·正文列 760/900px | chip 行是 `.drm-thread-row` 的**兄弟后继**（不在两列 grid 内）；全仓无 `gridColumn` 残留 | 组件树 + 源码扫描 | `DailyReportEvents.layout.test.ts` |

### 效果核对（问句④）

- **触发原因**：关注度取决于「有候选的线索占比」。若多数线索在窗口内找不到 ≥0.75 的同话题线索，chip 会大面积缺席，功能形同虚设。
- **核对方法**：本地 Docker PG 只读统计（`BEGIN READ ONLY`）：最近 10 期日报、3 个 active 话题，逐条线索算窗口内是否有 ≥0.75 的候选，统计有候选的线索占比与平均候选数。
- **量化结果**：**实现完成后填入**。判据：有候选的线索占比 < 40% 视为未达预期（选型实测里 topic 502 当期 6 条线索全部有 0.75+ 候选，但要更大样本才能确认）。
- **结论**：仅作价值判断，不改变本 change 的契约（无候选即不渲染、缺向量即 partial）。

### 与 ui-design 状态矩阵的差异（实施留痕）

| 项 | 已批准状态矩阵 | 实施 | 理由 |
| --- | --- | --- | --- |
| loading 态 | 2 个骨架 chip（高度固定，不引起跳动） | **整行不渲染**（`v-if` 在最外层，仅 `success && 有候选` 才出） | 骨架只在「已知该线索会有 chip」时才有意义；首载时无法预知，而多数线索没有候选（0.75 阈值下候选稀疏）→ 骨架会在几乎每条线索下闪现后消失，比不渲染更吵。实现按派发 brief「无数据整行不渲染、error 同样不渲染、`v-if` 在最外层」执行，其余 6 条主操作与 5 条禁止项与契约一致。 |

### 继承与调整（问句⓪，MODIFIED Requirement）

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 展开并缓存话题生命线 | 改语义（不再调 `getTopicLifeline`；改由 `thread-relations` 承担） | `front/app/features/tags/composables/useDailyReportReader.test.ts` | 改断言：删 lifeline 缓存用例，补「日报不调用 getTopicLifeline」 |
| identity 连线跨越空白日期 | 废止（不再渲染连线；identity 数据仍由后端保留） | `front/app/features/tags/components/daily-report/dailyReportMagazine.test.ts`、`DailyReportTopicSection.test.ts` | 删除 `buildLifelineWindow` 用例（函数退役）；组件侧改断言「模板中无 svg 连线」 |
| lifeline 加载失败 | 废止（该调用已退役） | `DailyReportReader.test.ts`、`useDailyReportReader.test.ts` | 删除断言；错误态改由 `ThreadRelatedEvents` 的「不渲染」承担 |
| 进入完整话题生命线 | 废止（侦探墙入口移除） | `DailyReportDeskShell.test.ts`、`DailyReportTopicSection.test.ts` | 删除断言并留痕（本表即留痕）；矩阵侧入口测试不动 |

### 白盒附加（复杂档）

**候选与打分分支表**

| 分支 | 条件 | 期望 |
| --- | --- | --- |
| B1 | 余弦 0.90 | 入选，`level=strong` |
| B2 | 余弦 0.85（边界） | 入选，`level=strong` |
| B3 | 余弦 0.8499 | 入选，`level=related` |
| B4 | 余弦 0.75（边界） | 入选，`level=related` |
| B5 | 余弦 0.7499 | **不入选** |
| B6 | 余弦 < 0 | 不可能（余弦负值按 0 处理，不入选） |
| B7 | 当期 thread `embedding` 为空 | 跳过该线索，`skipped_no_embedding += 1` |
| B8 | 候选 thread `embedding` 为空 | 不进候选池 |
| B9 | 候选 `related_article_ids` 为空 | `source_article=null` |
| B10 | 候选首篇文章已被删 | `source_article=null` |
| B11 | 候选标签有 confirmed 注记 | `confirmed=true` |
| B12 | 候选标签只有 suggested/dismissed | `confirmed=false` |
| B13 | score 与 period_date 都相同 | 按 `thread_id` asc |
| B14 | 窗口右端 = report_date | 不含当期（不推荐自己） |
| B15 | 全部候选低于阈值 | `threads={}`、`status=empty` |

**边界值清单**

- `days` ∈ {空、`7`、`30`、`14`、`0`、`-1`、`365`、`abc`} → 只有 7/30 通过。
- 阈值边界：0.7499 / 0.75 / 0.8499 / 0.85 / 0.9999 / 1.0000。
- 候选条数：0 / 1 / 3 / 4 / 20（前端 3 + 「还有 N 条」）。
- `title` 长度：短 / 24 字（ellipsis 边界）/ 80 字。
- `skipped_no_embedding`：0 / 1 / 全部（此时 `threads={}` 且 `status` 优先级为 empty）。
- 候选 `period_date`：窗口首日 / 窗口末日（`report_date-1`）/ 窗口外一天（不进）。

**不适用划除**

- 时区换算 → 全链路用 `DATE`（`period_date`）+ 本地日归一，不测跨时区。
- 并发写 → 只读模型，无写无锁，不声称线程安全。
- 分页 → 未实现（design「不做」），不测。
