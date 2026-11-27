# 审查 brief：lane-first-recall（未提交，worktree 增量）

只读审查。你只输出报告，禁止修改任何文件、禁止执行改变状态的命令。允许 read/grep/find/ls 与只读验证（`go test ... -run` 精确匹配、`git diff`、`git status`）。**机器是 4 核树莓派、load 常年 10+，禁止 `go test ./...` 全量**；只跑下列指定包的 `-run` 精确匹配。

## 〇、worktree 噪音声明（重要）

当前 worktree 有**其他会话**的无关改动，一律不在审查范围：

- `openspec/changes/*` 的大批删除 + `openspec/changes/archive/2026-11-26-*` / `2026-11-27-configure-subagent-model-routing` 的新增 —— 是别的会话在归档 change
- `openspec/specs/subagent-model-routing/`、`.pi/extensions/tests/.pi/`、`docs/reference/configuration.md` 的改动同理

**只审下表列出的文件**。

## 一、审查对象

**背景**：2026-11-26「全球科技巨头动态」日报只收 10 篇（前一日 33、再前日 65），当天所有英伟达算力管制新闻一条未进；泳道「黄仁勋风波及英伟达股价、产品发布」零 section。归因（design.md 有完整证据）：
① `filterTagsByQuality` 是二元硬开关——高质量标签 ≥10 个时所有 `weighted`（单篇新事件天然落这里）整批丢弃；
② `collectBoardTags` 的 fallback 对未挂载标签用 `Limit(50)` 且无 `ORDER BY`，11-26 的 363 个未挂载标签里 313 个不可见。

本 change 的方案：**召回单元从版块改为泳道**——每条 active 泳道用 ANN 在当日全量 event 标签池里取 Top-K（默认 8）+ 距离上限（默认 0.16）；泳道证据替代版块统计证据放行；经泳道召回但 L2 判 `new` 的标签直接丢弃、不进 L3。

**改动清单**（只审这些）：

| 层 | 文件 |
| --- | --- |
| 迁移 | `backend-go/internal/platform/database/lane_recall_migration.go`（新增）+ `postgres_migrations.go`（登记） |
| 迁移测试 | `backend-go/internal/platform/database/lane_recall_migration_test.go` |
| 召回查询 | `backend-go/internal/topicgraph/repository/daily_report_lane_recall.go`（新增，ANN + 原列复核 + 参数读取） |
| 召回测试 | `backend-go/internal/topicgraph/repository/daily_report_lane_recall_test.go` |
| 收集接线 | `backend-go/internal/topicgraph/service/daily_report_orchestrator.go`（`collectBoardTags` 并入泳道召回、`filterTagsByQuality` 认泳道证据） |
| 聚类防护 | `backend-go/internal/topicgraph/service/daily_report_lane.go`（L2 判 new 的泳道召回标签不回 L3） |
| 新测试 | `daily_report_lane_recall_test.go`、`daily_report_filter_test.go`、`daily_report_lane_gate_test.go`（同 service 包） |
| 改签名后的既有测试 | `collect_board_tags_test.go`、`collect_board_tags_exclusivity_test.go`、`daily_report_ai_detection_test.go`、`daily_report_orchestrator_test.go` |
| 文档 | `docs/reference/flow/daily-report.md`（红线 71/72）、`docs/reference/flow/topic-graph.md`（参数）、`docs/reference/database/tables/daily-report-watch.md`（ai_settings 两 key） |

契约权威源：`openspec/changes/lane-first-recall/specs/lane-first-recall/spec.md`（3 Requirement / 7 Scenario）；设计权威源 `openspec/changes/lane-first-recall/design.md`（D1~D6 + 边界不变量）；任务 `tasks.md`。

## 二、必查维度（按 design/spec 逐条对）

1. **ANN 是否真的命中索引**
   - 查询表达式是否与索引一致：`(embedding)::halfvec(2560) <=> $1::halfvec(2560)`，且带 `embedding_type='semantic'` 条件（索引是 partial index，条件必须写）
   - ANN 之后是否**用原列 float32 距离复核并截断**（design D5 / semantic-board 红线 38 的范式），而不是直接信 ANN 返回的距离
   - 建议只读验证：`EXPLAIN` 查询计划里是否出现 `idx_topic_tag_embeddings_semantic_halfvec_hnsw`

2. **不写持久事实层**
   - 泳道召回的整条路径 MUST NOT 写 `topic_tag_board_labels`（design D5 / spec「SHALL NOT 写入」）。grep 确认新增代码里没有对该表的 Insert/Save/Update

3. **跨版块排他仍然生效**
   - 泳道召回结果是否也过 `semantic_board_report_exclusive_gap`（design D6 / tasks 3.2）
   - **`gap` 是否走 Go 侧分支而非内联 SQL 参数**（既有代码注释明确说 pgx 会把左侧参数绑定判错）

4. **双通道合并**
   - 同一标签被两条通道召回时是否只保留一条、且保留版块通道的 `MatchReason`/`Score`
   - `laneRecalled` 标记是否只对「仅靠泳道进来」的标签置位——若把版块通道的质量标签也标成泳道召回，会让 `filterTagsByQuality` 的判据失真（重点查这个）

5. **质量分级零回归**
   - `len(kept) < 10` 的既有保底行为是否完全保持（全部 weighted 放回）
   - `len(kept) >= 10` 时：泳道召回标签进 `kept`，其余 `weighted` 照旧丢弃
   - 截断到 30 的排序逻辑是否未被改动

6. **L3 防护边界**
   - L2 成功返回 `new` + 属 `laneRecalled` → 丢弃，不 append `l3pool`
   - 非泳道标签被判 `new` → 照旧进 L3
   - **L2 调用失败**的降级路径是否保持「整批降 L3 + warn、不中断」（这条不能被新逻辑改成丢弃）

7. **参数与降级**
   - `lane_recall_top_k` / `lane_recall_max_distance` 缺失、非法、越界是否各自回退默认且不中断
   - 泳道 `embedding IS NULL` / 维度不符是否跳过该泳道 + warn 而非报错
   - 迁移是否幂等、是否会把用户已改的值覆盖回默认（**这是重点**：`ON CONFLICT DO NOTHING` 还是 `DO UPDATE`）

8. **测试对准 Scenario**
   - 7 个 Scenario 是否都有对应测试；特别看「当天没有专属新闻的泳道保持空白」和「L2 调用失败仍按既有行为降级」这两条是否真有断言（不是只写了个 happy path）

9. **文档红线表述**
   - `daily-report.md` 新增红线 71/72 是否与 spec 语义一致、有无夸大（例如把「默认 0.16」写成硬约束）

## 三、允许的只读验证命令

```
cd backend-go && go build ./...
cd backend-go && go vet ./...
cd backend-go && go test ./internal/topicgraph/service -run 'TestLaneRecall|TestFilterTagsByQuality' -count=1
cd backend-go && go test ./internal/topicgraph/repository -run 'TestLaneRecall' -count=1
cd backend-go && go test ./internal/platform/database -run 'TestLaneRecall' -count=1
bash scripts/harness/scenario-trace.sh openspec/changes/lane-first-recall
git diff -- backend-go/internal/topicgraph/service/daily_report_orchestrator.go
```

若 lint/build 因 load 被 SIGTERM 杀掉，注明「环境噪声」不要当缺陷报。

## 四、输出要求

写 `openspec/changes/lane-first-recall/review.md`，结构：

1. **结论**（通过 / 有条件通过 / 不通过 + 一句话理由）
2. **必须修（High）**：会致数据错误、污染、红线违反、回归的项，每条给 `文件:行` + 为什么 + 怎么改
3. **建议修（Medium）**：可维护性、边界未覆盖、测试断言不足
4. **可忽略（Low）**：风格、命名
5. **已验证通过**：逐条列出你实际跑过/读过的检查与结果
6. **未验证项**：说明为什么没验（环境限制等）

要求：**不要复述我给的 brief**，直接给判断；每条缺陷必须能落到具体代码位置；不确定的写「不确定」并说明缺什么信息。
