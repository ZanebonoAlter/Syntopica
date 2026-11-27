## 1. 仓储层：板块贴题信号查询

- [x] 1.1 新增按 report id 读本期正文板块贴题信号的轻量只读查询（字段：id / cluster_index / bucket_index / cluster_tag_ids / lane_tier / topic_status_at_report；写法仿 `ListReportEventThreads`，不要走 `GetReportByID`；`cluster_tag_ids` 按数字数组解析），验证：`cd backend-go && go build ./...` 通过，且该查询被单测或 2.3 的接线夹具真实覆盖（是否留永久仓储测试按 `docs/reference/standard/backend/testing.md` 的准入判断）。

## 2. 服务层：贴题档、候选池、后排与接线

- [x] 2.1 新增纯函数「贴题档」与「候选池（先筛）」（见 design「核心逻辑（伪代码）」；贴题档以聚簇完整成员标签对照板块标签集合，跨板块取最好），验证：新增表驱动单测覆盖 delta spec 的场景「贴题簇充足时不贴题的大簇不进候选」「贴题簇不足时按贴题档补位到三个」「贴题簇为空时维持原候选口径」「候选按簇内文章数截断」「跨板块取最好一档」全部通过；夹具用「中东地缘政治与美伊关系」2026-11-27（贴题簇 3 个：伊朗与美韩波斯湾表态、沙特炼油厂、伊朗霍尔木兹；中美关税 = 中档 + 候选话题，不进池）与 2026-11-26（贴题簇 3 个，含一个手动关注命中；两条关税簇都不进池）的真实形态。
- [x] 2.2 新增纯函数「后排（兜底）」：模型选中的重点按同一个贴题档稳定排序，验证：新增表驱动单测覆盖场景「贴题档低的大簇让位」「手动关注命中排在近档之后」「候选话题同档靠后且缺失不惩罚」「正文无对应板块的重点排最后」全部通过。
- [x] 2.3 在 `finalizeDailyReportHighlights` 的聚簇模式里接线：用既有比较器排出完整候选序（不截断）→ 读板块信号 → 组候选池 → 模型挑选（提示词、校验、重试不变）→ 后排 → 落库；读信号失败时候选池退回既有候选序前 8、跳过后排、记一条含 report id 与原因的 Warn；标签回退模式不接，验证：`cd backend-go && go test ./internal/topicgraph/service -run 'Highlight|CandidatePool'` 全绿，且 `TestHighlightsClusterPromptAndSnapshot` 明确断言「板块信号为空时候选池 = 既有候选序前 8、重点顺序 = 候选序」。
- [x] 2.4 自检范围没有外溢：既有候选序比较器的三键与截断语义不变（实现时把比较器函数体抽成 `OrderEventClusterCandidates`，`RankEventClusterCandidates` 只做截断委托，既有 `TestRankEventClusterCandidatesOrderAndLimit` 未动即通过）、提示词、快照字段、`FinalizeReportHighlights`、前端文件均无语义变化，验证：`git diff --stat` 里代码改动只有 `daily_report_story_highlights.go`、`report_event_clusters.go`（比较器抽取，三键逐字未变）、新仓储文件与测试文件。

## 3. Review

- [x] 3.1 派 `change-review` 子线程聚焦 review（审查制品 + diff）：对照 delta spec 两条 Requirement 的新增与改写场景、design D2 / D6 / D8（跨板块取最好、补位与为空的分支、不用 `GetReportByID`）与 flow 红线 5 / 46 / 41，重点看贴题档是否用聚簇完整成员标签、补位是否真的挤不掉贴题簇、同档是否仍只比文章数、失败分支是否真的退回现状且不阻断、有没有顺手改了提示词或既有比较器；意见按 High / Medium / Low 处置后在 `review.md` 建账，验证：`bash scripts/harness/review-trace.sh openspec/changes/rank-report-highlights-by-board-relevance` 退出码 0。

## 4. 测试

- [x] 4.1 影响包测试：`cd backend-go && go test ./internal/topicgraph/service ./internal/topicgraph/repository`（只跑影响包；同包有 `lane-first-recall` 的中间态，红了先分清归属再处理）。
- [x] 4.2 永久测试只留主线与回归：2.1 / 2.2 的表驱动单测 + 2.3 的一条接线断言；实现期的临时白盒用完即弃。

## 5. 文档

<!-- doc-impact: flow -->

- [x] 5.1 更新 `docs/reference/flow/daily-report.md`：「今日重点的选题与热度（story highlights）」节补贴题档（0–5）与候选池规则（贴题簇优先、不足补位到 3、为空维持前 8）、挑完再排、查询失败回退、只调序不过滤；红线 46 的候选口径与「输出顺序按服务端候选序而非模型序」改为新口径；「代码入口」清单补新的查询与函数。
- [x] 5.2 归档后按 §12 在该 flow 文档补「变更溯源」链接。

## 6. 验证

每条「命令 → 期望结果 → 实际」，实际结果为主线程 2026-11-27 亲眼实跑。

- `cd backend-go && go build ./... && golangci-lint run ./internal/topicgraph/... && go vet ./internal/topicgraph/...` → 零报错 → **OK / `0 issues` / OK**
- `cd backend-go && go test -short -count=1 ./internal/topicgraph/...`（`change-scope.sh` 机械判定的影响范围）→ PASS → **handler / repository / service 全 ok**；动手前基线同为 ok，无既有红
- `cd backend-go && go test -short -count=1 ./internal/topicgraph/service -run 'CandidatePool|RankPicked|RelevanceRank|Highlight' -v` → 候选池与排序场景全 PASS → **64 PASS / 0 FAIL**；11-27 夹具候选池 = 伊朗与美韩波斯湾表态 / 沙特炼油厂 / 伊朗霍尔木兹（不含中美关税），11-26 夹具候选池 = 2 近档 + 1 手动关注（两条关税不进池）
- `cd backend-go && go test -count=1 ./internal/topicgraph/repository -run 'TestListReportSectionSignals|TestOrderEventClusterCandidatesIsFullOrder' -v` → PASS → **PASS**（testcontainer PG：jsonb 数字数组与历史 NULL 读取）
- `cd backend-go && go test -count=1 ./internal/topicgraph/service -run 'TestHighlightsClusterSignalErrorKeepsCandidateOrder|TestHighlightsClusterFinalizeAndTagFallback|TestHighlightsCompositeOnlyReportNotTagFallback'` → PASS → **3 PASS**（信号读取失败退回候选序、零候选走标签回退、全 composite 不回退）
- 部署后首期核对的只读 SQL（**重启后端后执行，结果待记**；是否手动重跑 11-27 / 11-26 那两期由用户决定）：

  ```sql
  -- 「中东地缘政治与美伊关系」(board 1974) 自部署日起：每条重点 → 所属板块的泳道档 / 话题状态
  select r.period_date, e.ord, left(e.h->>'title', 24) as title,
         (select string_agg(distinct s.lane_tier, '/') from daily_report_sections s
           where s.report_id = r.id and exists (select 1 from jsonb_array_elements_text(s.cluster_tag_ids) x
             where x in (select jsonb_array_elements_text(e.h->'tag_ids')))) as lane_tiers,
         (select string_agg(distinct s.topic_status_at_report, '/') from daily_report_sections s
           where s.report_id = r.id and exists (select 1 from jsonb_array_elements_text(s.cluster_tag_ids) x
             where x in (select jsonb_array_elements_text(e.h->'tag_ids')))) as topic_status
  from board_daily_reports r cross join lateral jsonb_array_elements(r.highlights) with ordinality as e(h, ord)
  where r.semantic_board_id = 1974 and r.period_date >= '<部署日>'
  order by r.period_date desc, e.ord;
  ```

  期望：当天贴题板块 ≥3 时重点里没有 `l2_llm` / `l3_new` 的簇，首条为 `l1_direct` 或 `watch_*`。
- `bash scripts/harness/doc-impact.sh verify openspec/changes/rank-report-highlights-by-board-relevance` → PASS → **通过（声明 flow，文件 1 个）**
- `bash scripts/harness/check-standards.sh --change rank-report-highlights-by-board-relevance` → A–D / F / G 段零失败 → **通过 357 / 失败 0 / 警告 0**
- `bash scripts/harness/archive-readiness.sh rank-report-highlights-by-board-relevance` → **九项全绿**
- `bash scripts/harness/test-patrol.sh --report` → 本 change 影响包（be-topicgraph）无欠账；台账 38 条 open 均为其他 change 的既有欠账，不属本 change。
