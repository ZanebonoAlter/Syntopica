# Test Cases: offline-catchup

> 复杂档白盒+故事测试用例文档。测试单元 = Requirement 的用户故事，本表由 specs 12 个 Scenario 串成完整故事；spec Scenario 只是断言片段，交付账本在故事层。

## 主链路故事

树莓派常驻采集 + PC 间歇开机的用户，停机 5 天后回来：恢复 → 打标积压 drain（队列清空）→ 期间文章陆续打标挂边（边在 7 天窗内）→ 次日 21:00 定时日报跑完当天报告后自动补齐 5 天缺档 → 想手动重建 10 天前的报告被窗口守卫拒绝（边已回收，防止空报告覆盖好报告）。归档文章不再因时序运气丢边，边由统一时间窗 GC 回收。

## 主链路节拍表

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 文章超限归档 | article-retention/MODIFIED「衍生数据清除」 | behaviors 删、search_vector NULL、**article_topic_tags 边保留**（含迟到打标挂的边） | service (SQLite) | `internal/reader/service/feed_service_cleanup_test.go`（改造既有） |
| 2 | 归档文章搜索不可见 | 同上「归档文章不被全文搜索命中」 | 不出现在搜索结果（语义不变，防回归） | service (SQLite) | 同上 |
| 3 | 边满 7 天被 GC 回收 | article-retention/ADDED「超窗边被回收」 | 边删除；tag 无剩余边则被孤儿清理回收，有剩余边则保留 | service (SQLite) | `internal/tagmanagement/service/core/edge_gc_test.go`（新建） |
| 4 | 恢复后 drain 挂的边在窗内不删 | 同上「窗口内边保留供补档消费」 | 窗内边保留，日报候选可匹配 | service (SQLite) | 同上 |
| 5 | 配置缺失/非法回退默认 | 同上「配置非法回退默认」 | 回退 7 + warn，不拒绝执行 | 纯函数单测 | `internal/tagmanagement/service/core/edge_gc_test.go`（config reader 部分） |
| 6 | AI 暂停时 GC 照常跑 | 同上「回收不受分析暂停影响」 | 维护类 job 不经 PauseAware 门禁 | 结构+job 单测 | `internal/admin/scheduler/pause_test.go` 模式断言（aux_label_cleanup 不在跳过清单） |
| 7 | 队列清空后次日自动补档 | daily-report/ADDED「停机缺档次日自动补齐」 | 窗口内缺失 (board,date) 逐个重建，零人工 | job 单测 (SQLite) | `internal/admin/scheduler/job_daily_report_test.go`（新建） |
| 8 | 队列未清空顺延 | 同上「队列未清空顺延」 | 当天报告照常，补档跳过，次日再试 | job 单测 (SQLite) | 同上 |
| 9 | 已有报告不重建 | 同上「只补缺不重建已有」 | (board,date) 存在则跳过 | job 单测 (SQLite) | 同上 |
| 10 | 超窗日期 API 拒绝 | daily-report/ADDED「超窗日期拒绝重建」 | POST generate 返回 4xx，错误消息含窗口说明；既有报告不被覆盖 | handler 单测 (httptest) | `internal/topicgraph/handler/daily_report_handler_test.go`（新建或并入既有） |
| 11 | 窗口内日期正常重建 | 同上「窗口内日期正常重建」 | 照常异步触发 | handler 单测 (httptest) | 同上 |
| 12 | 调度器指定日期同口径 | 同上「调度器指定日期触发同口径」 | TriggerNowWithDate 超窗 accepted=false，与 API 口径一致 | job 单测 | `internal/admin/scheduler/job_daily_report_test.go` |

## 继承与调整（⓪ 改契约了吗——article-retention 有 MODIFIED Requirement）

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 「衍生数据清除」（旧行为：归档删边 + CleanupOrphanedTags） | MODIFIED：边保留，孤儿清理移交窗口 GC | `feed_service_cleanup_test.go` `TestCleanupOldArticlesClearsDerivedData`（L219，断言 victim edges=0、orphan tag 被删、shared tag 存活） | 断言反转：victim edges==2 保留、orphan tag **保留**（不再由归档路径清）、shared tag 保留；改名或改注释明确新语义 |
| 「归档文章不被全文搜索命中」 | 语义不变 | 无独立断言（search_vector 置 NULL 路径在主链路步 2 覆盖） | 不动，防回归即可 |
| 其余 article-retention requirement（超限归档/favorite 免死/无上限跳过/幂等/窗口计数/可见性/读取豁免） | 未动 | 同文件既有 4 个测试函数 | 不动 |
| scheduler：aux_label_cleanup job 语义扩展（加边 GC 步） | 无独立旧测试 | 无 | 新增 |
| scheduler：daily_report job 语义扩展（尾部补档） | 无独立旧测试 | 无 | 新增 |

## 变体走查（五组固定清单）

| 组 | 变体 | 答案 | 落点 |
| --- | --- | --- | --- |
| 输入 | 配置 `"abc"` / `""` / `"0"` / `"-3"` / 键缺失 | 全部回退默认 7 + warn 日志，不拒绝执行 | edge_gc_test（表驱动） |
| 输入 | date 参数格式非法（generate API） | 走既有 400 路径（不新增分支，守卫在格式解析之后） | handler 既有行为，不新增用例 |
| 前置 | 队列 pending → 补档 | 跳过，JobResult 记跳过原因 | job_daily_report_test |
| 前置 | 队列 leased → 补档 | 同 pending，跳过（leased 也是未完成积压） | job_daily_report_test |
| 前置 | 队列空 → 补档 | 执行扫描 | job_daily_report_test |
| 前置 | tag 有剩余边 / 无剩余边（GC 后） | 有 → tag 保留；无 → 孤儿清理回收 | edge_gc_test |
| 时间窗口 | 边恰好 created_at = now()-7d | **保留**（删除条件 `created_at < 下界`，等号不删）；与守卫「date==下界放行」同口径 | edge_gc_test 边界值 |
| 时间窗口 | 边 created_at = now()-7d-1ms | 删除 | edge_gc_test 边界值 |
| 时间窗口 | 补档扫描窗口 | (today-retentionDays, today) 左闭右开——**不含今天**（今天由主流程生成） | job_daily_report_test |
| 时间窗口 | 守卫 date = today-7d | 放行（"早于下界"才拒绝） | handler/TriggerNowWithDate 测试 |
| 时间窗口 | 守卫 date = today-8d | 拒绝 4xx / accepted=false | 同上 |
| 幂等 | EdgeGC 连跑两次 | 第二次删除 0 条（无残留超窗边），孤儿判定稳定 | edge_gc_test |
| 幂等 | 补档重跑 | 已有报告跳过（只补缺），不重复重建烧 LLM | job_daily_report_test |
| 幂等 | 队列空但窗口内无缺档 | 扫描 0 补 0，JobResult 正常返回 | job_daily_report_test |
| 可用性 | （UI 无改动，前三组豁免） | 超窗 4xx 错误消息文案 = 用户可见 API 文案，spec 已锚定「标签边已按窗口回收、候选不全」 | handler 测试断言消息含窗口说明 |

不适用划除：前端可用性变体（误输入反馈/空态/错误态/加载态/超长文本/重复提交）——本 change ui-impact: none，无前端改动。

## 白盒附加节（复杂档）

### 边 GC 状态矩阵（边 × tag 四格）

| 边状态 | tag 有剩余边 | tag 无剩余边 |
| --- | --- | --- |
| 边 ∈ 窗内 | 不删，tag 保留 | n/a（边在即有边） |
| 边 ∈ 窗外 | 删边，tag 保留 | 删边 + CleanupOrphanedTags 回收 tag |

边界值：created_at ∈ {now()-7d+1s, now()-7d, now()-7d-1ms, now()-8d}；批量删除含多 tag 混合状态（部分孤儿部分存活）。

### 补档前置状态机（队列 × 报告六格）

| 队列 | (board,d) 报告存在 | 报告缺失 |
| --- | --- | --- |
| pending / leased | 跳过补档（整轮，含当天） | 跳过补档（顺延次日） |
| 空 | 跳过该格（不重建已有） | **GenerateAndSaveReport 重建** |

附：当天报告生成失败时补档是否继续？——继续扫补档（当天失败已有错误处理，补档独立推进，单板块失败不阻塞兄弟板块，松耦合红线）。

### 守卫判定

`date < NormalizeReportDate(today) - retentionDays*24h` → 拒绝。两入口（handler / TriggerNowWithDate）共用同一判定函数，错误消息同文案。

## 效果核对（⑤d 真库量化）

| 问题 | 方法 | 结论位 |
| --- | --- | --- |
| 停机恢复后缺档是否真的自动补齐（依赖 drain 时序与 LLM 行为） | 人工：本地停 AI 数日（或模拟 stop AI → 累积 tag_jobs → 恢复）→ 次日 21:00 观察补档日志/报告列表 | 任务 3.2，人工时间线记录 |
| 存量泄漏边（>7 天的 1542 篇归档带边）首轮 GC 收敛 | 部署后观察 aux_label_cleanup job 日志边回收计数 | 任务 3.2 附记 |

## 展示字段盘点（⑤e）

无新增用户可见展示字段。JobResult Data 新增运维可见键（边回收计数、补档计数、跳过原因），无前端消费，随 scheduler 任务结果接口透出（既有机制）。超窗拒绝错误消息为 API 用户可见文案，spec 已锚定语义。

## Scenario → 测试文件映射表

| Scenario | 测试文件 |
| --- | --- |
| 衍生数据清除（边保留） | backend-go/internal/reader/service/feed_service_cleanup_test.go |
| 归档文章不被全文搜索命中 | backend-go/internal/reader/service/feed_service_cleanup_test.go |
| 超窗边被回收 | backend-go/internal/tagmanagement/service/core/edge_gc_test.go |
| 窗口内边保留供补档消费 | backend-go/internal/tagmanagement/service/core/edge_gc_test.go |
| 配置非法回退默认 | backend-go/internal/tagmanagement/service/core/edge_gc_test.go |
| 回收不受分析暂停影响 | backend-go/internal/admin/scheduler/pause_test.go（扩展断言） |
| 停机缺档次日自动补齐 | backend-go/internal/admin/scheduler/job_daily_report_test.go |
| 队列未清空顺延 | backend-go/internal/admin/scheduler/job_daily_report_test.go |
| 只补缺不重建已有 | backend-go/internal/admin/scheduler/job_daily_report_test.go |
| 超窗日期拒绝重建 | backend-go/internal/topicgraph/handler/daily_report_handler_test.go |
| 窗口内日期正常重建 | backend-go/internal/topicgraph/handler/daily_report_handler_test.go |
| 调度器指定日期触发同口径 | backend-go/internal/admin/scheduler/job_daily_report_test.go |

## 真实效果人工核对（任务 3.2 时间线）

（待回填：停机窗口 / 恢复时刻 / drain 完成时刻 / 补档触发时刻与结果 / 超窗手动请求结果）
