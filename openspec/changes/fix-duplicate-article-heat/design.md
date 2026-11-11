## Context

见 `proposal.md` · Why 的实证数据。当前状态与约束：

- 关键字物化轨的唯一生产调用点在 `daily_report_orchestrator.go` Step 7.5；同段落的 sentence 轨（`MaterializeSentenceWatch`）已经接收 `date`，keyword 轨没有——两条轨的日期口径在签名层就分叉了。
- `scanBoardDayArticles` 在 `date.IsZero()` 时静默改用 `time.Now()`。这个"兜底"让调用方漏传参数不会报错，只会静默产出错误数据（本次事故的直接放大器）。
- `story_signals` 主键 `(story_id, article_id)`，一篇文章一个 feed 副本一行；`participant_key = feed:<id>` 被当作参与者身份的唯一来源，热度、`participant_count`、雷达「N 个来源」、矩阵颜色分档、lane_heat 全部按它 `DISTINCT` 计数。
- `articles` 唯一键 `(feed_id, link)` 是既有设计（`rss-article-dedup`：跨 feed 副本共享打标、feed 归属独立），本期不动。
- 红线（`docs/reference/flow/daily-report.md` 约束 44/45、`semantic-board.md` 雷达口径）：热度不按 `archived` 过滤；热度不从日报派生；`related_article_ids` 生成口径不改。

## Goals / Non-Goals

**Goals:**
- 补档 / 重建历史日报时，关键字物化轨与「当天按时生成」得到同一发布窗口（消除跨天重复选文）。
- 同一篇内容的多 feed 副本在**所有**参与者消费面（signals、participant_count、story_heat、lane_heat、雷达来源数、矩阵分档）只算 1 个参与者，且口径只有一份实现。
- 存量虚高数据可幂等重算，且不触碰任何日报正文。

**Non-Goals:**
- 不改 `articles` 入库层（`(feed_id, link)` 唯一、副本打标复用保持不变）。
- 不合并「同标题不同 URL」的转载（本期只认规范化链接）。
- 不重建历史报告正文（section / thread 一律不动）。
- 不改事件线身份判定（`subject_key` 三层匹配、`subjectPreFilter` 一票否决）。
- 不改热度公式（48h/24h 半衰期、`HeatRuleVersion` 保持 `heat-v1-48h-hl24h`）。

## Decisions

### D1. 关键字物化轨显式接收报告日期，并删除零值兜底

- **结论**：`MaterializeKeywordWatches` 增加 `date time.Time` 入参（与 `MaterializeSentenceWatch` 对齐），生产调用点传 orchestrator 的 `date`；`scanBoardDayArticles` 的 `date.IsZero() → time.Now()` 分支删除，改为返回错误（显式失败优于静默错数据）。
- **落点**：`backend-go/internal/topicgraph/service/watch_materialize_keyword.go:MaterializeKeywordWatches`
- **取舍**：否掉「保留签名 + 在 orchestrator 里设置包级"当前报告日期"」——隐式全局状态在并发/多板补档时不可靠，且同样会静默降级；否掉「只在 orchestrator 里预过滤文章再传入」——召回层职责会分裂到两个包。

### D2. 内容身份落成 `story_signals` 的持久列，而不是查询期表达式

- **结论**：`story_signals` 新增可空列 `content_key`（规范化链接；不可判定时为空串），写入时与 `participant_key` 同源产出；迁移只加列 + 索引，不做长事务回填。
- **落点**：`backend-go/internal/platform/database/postgres_migrations.go`
- **取舍**：否掉「热度查询里 join `articles` 用 SQL 表达式现算」——本次 bug 的成因就是同一口径在多处各写一遍（signals 写入 / heat 加载 / radar 计数），现算会让口径继续散落且无法索引；否掉「新建 content_identity 映射表」——多一层 join，而副本身份本来就是 link 的函数，无独立生命周期。

### D3. 去重键统一为 `COALESCE(content_key, participant_key)`

- **结论**：参与者去重的唯一键定为 `COALESCE(NULLIF(content_key,''), participant_key)`；`story_heat`、`lane_heat`、`participant_count`、雷达来源数、矩阵分档全部改用该键，保证「1 篇内容 = 1 个参与者」只有一处定义。
- **落点**：`backend-go/internal/topicgraph/repository/story_heat.go:loadBoardHeatEvidence`
- **取舍**：否掉「只改 `participant_count` 不改 `story_heat`」——会出现热度按 2 计、来源数按 1 计的更严重不一致（红线要求同源）；否掉「直接把 `participant_key` 改写成 `content:<hash>`」——会丢掉 feed 维度的可解释性（哪个源先报），且需要重写全部历史证据行。

### D4. 链接规范化只有一个 Go 实现，迁移与运行期共用

- **结论**：新增 `NormalizeArticleLink(link string) string`：`TrimSpace` → 斩 `#` 后段 → 斩 `?` 后段 → scheme/host 小写、path 保留原样；空输入返回空串。写入 signal、存量回填、测试断言全部调用它，SQL 侧不重写规则。
- **落点**：`backend-go/internal/platform/textutil/normalize.go`
- **取舍**：否掉「SQL 里 `regexp_replace` 实现」——大小写折叠与 URL 解析在 PG 里写起来脆，且立刻形成第二份口径；否掉「用 url.Parse 全量规范化」——会改 path 语义（尾斜杠、编码），与 `rss-article-dedup` 的 `(feed_id, link)` 原值判重口径打架。

### D5. 存量重算走既有运维入口，幂等且不重建报告

- **结论**：迁移只加列；存量 `content_key` 回填与受影响事件线的 signals / participant_count 重算复用 `cmd/backfill-stories --rebuild-signals`（补齐按内容键去重后的计数），分批、可重入、失败仅告警。
- **落点**：`backend-go/cmd/backfill-stories/main.go`
- **取舍**：否掉「迁移里一次性全量回填 + 重算」——树莓派上长事务会阻塞启动，且迁移失败不可重试；否掉「重建 21 份受污染报告」——用户已拍板不动正文，重建会改变历史阅读内容。

### D6. 修正存量报告的记忆，只改数字不改正文

- **结论**：受影响报告（`watch_keyword` 混入非当天文章的 21 份）不做任何 section/thread 改写；其虚高数字由 D5 重算 signals + 计数自然回落。
- **落点**：Scenario「补档内容不含晚于报告日的文章」
- **取舍**：否掉「删除历史报告里非当天的 thread」——会破坏已读内容的可回溯性，且 `related_article_ids` 被删后无法复原；否掉「标记脏数据可见」——产生新的展示契约，超出 none 档 UI 影响。

### D7. 日期语义加测试防线，防回潮

- **结论**：补档场景进自动化用例——报告日 ≠ 执行日时，keyword 物化只取报告日文章；`scanBoardDayArticles` 收到零值必须报错。测试同时覆盖「当天按时生成」与「补档重建」两条路径产出同一窗口。
- **落点**：`backend-go/internal/topicgraph/service/watch_materialize_integration_test.go`
- **取舍**：只加断言不加护栏（保留 `time.Now()` 兜底）——同一个坑会再踩一次；否掉「用 e2e 覆盖」——补档是调度路径，函数层集成测试更便宜且能精确断言窗口。

## Risks / Trade-offs

- [热度整体下降，雷达阈值（P80/P20 = 3.3/0.7）可能失准] → 重算后跑 `cmd/calibrate-heat` 只读采样核对分档分布；阈值是配置，不属本次 spec 变更。
- [规范化链接误合并两篇不同内容（极端：站点用同一 URL 换稿）] → 只按 link 判定，与 `rss-article-dedup` 的副本语义一致；不做标题/指纹归并，避免更激进的误合并。
- [`content_key` 回填期间新旧口径并存，数字短暂跳变] → 回填脚本幂等，重算完成前只影响展示数值，不影响身份与正文。
- [历史 signals 的 `content_key` 为空串会退化按 feed 计] → 回填覆盖全部存量 `articles.link`；空 link 文章本就无内容身份，退化是明示行为。
- [报告正文仍含跨天混入的 7 篇文章（历史事实）] → 用户已确认不重建；本次只保证后续补档不再发生，并在重算后让来源数/热度如实反映。

## Migration Plan

1. 部署新代码 + 迁移（加 `content_key` 列与 `(story_id, content_key)` 索引，可空、可回滚）。
2. 执行 `cmd/backfill-stories --rebuild-signals`：回填 `content_key` → 逐事件线重算 signals / participant_count（分批、幂等）。
3. 只读核对：受影响事件线（示例 5204）参与者数由 2 回落为 1；全库 `participant_key ≠ content_key` 的重复组归零。
4. 回滚：列可保留不读；代码回滚后旧口径（按 feed）立即恢复，无需数据回滚。

## Open Questions

（无。剩余选择均属实现细节，不改变 spec 或任务拆分。）
