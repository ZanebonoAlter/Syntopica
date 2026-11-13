# test-cases: dedupe-thread-article-copies

## 故事 S1: 用户展开一条日报线索时，看到的是「这篇线索关联的几篇文章」而不是同一篇内容的重复副本（锚 Requirement: 日报查询 API）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 打开某篇日报详情（关心的话题页） | 查询日报详情含线程 | 响应含 sections[].threads[]，每条含 id/title/summary/tag_ids/confidence/related_article_ids，无 status/prev_*_id | 集成（testcontainer PG） | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 2 | 该 thread 的关联文章含两个跨 feed 同 link 副本 | 线索下同 link 副本折叠 | 响应里该 thread 的 `related_article_ids` 只剩首个出现的 id，长度减一、顺序不变 | 集成（testcontainer PG） | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 3 | 折叠后回查库表 | 线索下同 link 副本折叠 | `daily_report_threads.related_article_ids` 落库值仍是未折叠的两个 id | 集成（testcontainer PG） | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 4 | 两篇文章 link 仅差 fragment / query / host 大小写 | 归一后 link 相同也折叠 | 归一后判为同内容，折叠为一条 | 单元（纯函数，无 DB） | backend-go/internal/topicgraph/repository/article_identity_test.go |
| 5 | thread 关联文章中有一篇 `link` 为空 | link 为空不折叠 | 该 id 原样保留在响应数组里 | 单元（纯函数，无 DB） | backend-go/internal/topicgraph/repository/article_identity_test.go |
| 6 | 两篇文章标题相同但 link 不同（多源转载） | 同标题不同 link 不折叠 | 两个 id 都保留（本口径只认归一 link 相同） | 单元（纯函数，无 DB） | backend-go/internal/topicgraph/repository/article_identity_test.go |
| 7 | 用户在「关心的话题」页展开该线索 | 同标题不同 link 不折叠 + 线索下同 link 副本折叠 | 展开后不出现重复标题条目；「N 篇」计数与实际展示条数一致 | 端到端（agent-browser，静态托管 :5100） | 人工：agent-browser 打开 /tags/<boardId>/<date>，展开 thread 17700，确认标题只出现 1 次且计数为「1 篇」 |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入：空串（`link` = ""） | 不参与折叠，原样保留 | 单元 | article_identity_test.go |
| 2 | 输入：纯空白（`link` = "   "，含全角空格/tab） | `TrimSpace` 后为空 → 不参与折叠 | 单元 | article_identity_test.go |
| 3 | 输入：纯分隔符（`link` = "///"） | 归一后非空（"///"）→ 参与折叠，与另一个 "///" 判为同内容 | 单元 | article_identity_test.go |
| 4 | 输入：单 token（`link` = "abc"） | 归一后为 "abc"，参与折叠 | 单元 | article_identity_test.go |
| 5 | 输入：大小写（`HTTPS://EXAMPLE.COM/a` vs `https://example.com/a`） | 归一后相同 → 折叠 | 单元 | article_identity_test.go |
| 6 | 输入：特殊字符（引号 / emoji / 正则元字符） | 归一是纯字符串处理，不受影响 | 单元 | article_identity_test.go |
| 7 | 输入：超长 link | 归一不截断（既有 `NormalizeArticleLink` 语义），行为一致 | 单元 | article_identity_test.go |
| 8 | 前置：空集（thread 无 `related_article_ids`） | 原样返回空数组，不报错 | 单元 | article_identity_test.go |
| 9 | 前置：单元素 | 原样返回该元素 | 单元 | article_identity_test.go |
| 10 | 前置：重复元素（同一 id 出现两次） | 折叠为一条 | 单元 | article_identity_test.go |
| 11 | 前置：越界引用（id 指向不存在的文章行） | 查不到 link → 不参与折叠、保持原样，不崩、不删 | 单元 + 集成 | article_identity_test.go；daily_report_repository_test.go（TestGetReportByID_KeepsDanglingArticleIDReference） |
| 12 | 前置：部分满足（部分 id 有 link、部分查不到） | 有 link 的按 link 折叠；查不到的保持原样 | 单元 | article_identity_test.go |
| 13 | 时间窗口：边界两端（当天算不算 / 前后各一步） | 折叠与时间无关：判据无时间项，同 link 即折叠（跨天/同日无差别） | 单元 | article_identity_test.go（用例名「变体#13 折叠与时间无关：同 link 即折叠（判据无时间项）」） |
| 14 | 时间窗口：空窗口 / 跨窗口 | 不适用——折叠不依赖时间窗口，无窗口概念 | — | 划除留痕：判据无时间项 |
| 15 | 幂等：重复执行两次 | 两次结果一致（纯函数 + 读路径不改库） | 单元 | article_identity_test.go |
| 16 | 幂等：部分失败后重试 | link 查询失败 → 降级不折叠；重试成功后正常折叠 | 集成 | daily_report_repository_test.go（TestGetReportByID_LinkQueryFailureThenRetryFolds） |
| 17 | 幂等：并发 | 不适用——读路径无共享可变状态，不声称线程安全需另行声明 | — | 划除留痕：无共享状态 |
| 18 | 可用性：空态 | 线索无关联文章时既有空态不变（回归） | 集成 + 端到端 | daily_report_repository_test.go；人工：agent-browser 看无文章的线索 |
| 19 | 可用性：错误态 | 折叠用的 link 查询失败 → 详情接口仍 200 且数组未折叠（降级可见） | 集成 | daily_report_repository_test.go |
| 20 | 可用性：误输入反馈 / 加载态 / 超长文本 / 重复提交 | 不适用——本 change 无前端输入与交互改动 | — | 划除留痕：读路径改动，前端零改动 |

### 效果核对（触发问句④时）

- 触发原因：折叠效果依赖真实库里「同一篇内容被存成几行」的分布，断言通过不等于库里真有重复、折叠真生效。
- 核对方法：真库只读（`SELECT`，不写库）统计 `daily_report_threads.related_article_ids` 展开后「同 thread 同归一 link 出现 >1 次」的组数与多余文章数；与折叠后 API 返回的数组长度对账。
- 量化结果（2026-11-12 排查实测，折叠前）：全库 136 个重复组、影响 134 条线索、多出 145 篇文章（30 组同 link 跨 feed 副本 / 106 组同标题不同 link 转载）；折叠后同 link 组应归零。
- 结论：保守档只覆盖 30/136 组（其余为多源转载，本口径明确不折叠），收益以「零误伤」为前提取舍。

### 继承与调整（涉及 MODIFIED Requirements 时必填）

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 查询日报详情含线程 | 改语义 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go | 照跑（回归网）+ 补折叠断言（步 2/3） |
| 查询板块日报列表 | 继承 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go | 照跑 |
| API 查询超过 30 天 | 继承 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go | 照跑 |
| 无日报时返回空 | 继承 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go | 照跑 |
| 展开 thread 文章并复用缓存 | 继承 | front/app/features/tags/composables/useDailyReportReader.test.ts；front/app/features/tags/components/DailyReportReader.test.ts | 照跑（前端零改动） |
| 点击线索文章直达预览弹窗 | 继承 | front/app/features/tags/components/daily-report/DailyReportDeskShell.test.ts | 照跑（前端零改动） |
| 一线索多篇依据只标一条 | 继承 | backend-go/internal/topicgraph/repository/key_event_markers_pg_test.go | 照跑（关键事件注记直读库表，不受读路径折叠影响） |

### 白盒附加（复杂档：折叠算法分支表 + 边界值）

**分支表**（`FoldThreadArticleCopies` 的判定分支，断言判据=返回值）：

| # | 分支条件 | 期望行为 | 边界值/样例 |
| --- | --- | --- | --- |
| B1 | thread 数组为空 | 原样返回空 | `[]` |
| B2 | `related_article_ids` 为空 / 非法 JSON | 原样保留（不折叠、不删） | `[]`、`null`、非数组 |
| B3 | 元素 id 在 link 映射中不存在 | 该 id 原样保留 | id=999999 无对应行 |
| B4 | link 去空白后为空 | 该 id 不参与折叠，原样保留 | `""`、`"   "`、`"\u3000"` |
| B5 | 归一 link 非空且首次出现 | 保留 | keeper |
| B6 | 归一 link 非空且与前面某 id 相同 | 折叠（丢弃当前 id） | 副本 |
| B7 | 归一 link 与前面 id 相同但当前 id 排在前面 | 保留当前（首个出现为准） | 顺序敏感 |
| B8 | 两个 id 归一后不同（不同 link / 同标题不同 link） | 都保留 | 转载样本 |
| B9 | 归一后 host 大小写不同 | 判为相同 → 折叠 | `EXAMPLE.com` vs `example.com` |
| B10 | 归一后仅 query 不同 | 判为相同 → 折叠 | `?utm_source=x` |
| B11 | 归一后仅 fragment 不同 | 判为相同 → 折叠 | `#reply12`（`#!` hashbang 保留） |
| B12 | 同一 id 在数组中重复出现 | 折叠为一条 | `[7,7]` |
| B13 | link 映射整体为空（查询失败降级） | 原样返回（不折叠） | `map[uint]string{}` |

**边界值清单**

| 项 | 值 |
| --- | --- |
| id 数量 | 0 / 1 / 2 / 3（首个出现为准需 ≥2 才可判折叠） |
| link 字符 | `""` / `" "` / `"\u3000"` / `"///"` / `"abc"` / `"HTTPS://EXAMPLE.COM/a"` |
| 归一等价对 | fragment 差 / query 差 / host 大小写差 / 三者同时差 |
| 折叠后长度 | 期望 = 原长度 − 同归一 link 的多余个数 |

**不适用划除留痕**

- 状态机：不适用——折叠是**无状态纯函数**（输入 threads + link 映射 → 输出 threads），无状态迁移、无合法/非法状态。
- 协议：不适用——无跨进程/跨服务协议变更；HTTP 响应字段形状不变（仍是 article id 数组），只是元素数变少。
- 时间窗口：不适用——判据无时间项（见变体 #13/#14）。
