# Tasks

## 1. direct_hit 占比门槛（tagmanagement）

- [x] 1.1 `semantic_board_matching.go`：`SemanticBoardMatchConfig` 增加 `DirectHitMinRatio float64`；`LoadConfig` 默认 0.5 + key `semantic_board_match_direct_hit_min_ratio`（clamp 0~1，非法回退）；Priority 2 判定追加 `len(tagAuxiliaryIDs) > 0 && overlap/len(tagAuxiliaryIDs) >= DirectHitMinRatio` AND 条件（ratio 不足不 append、落回 Priority 3）。验证：`go test ./internal/tagmanagement/service/board/ -run TestSemanticBoardMatching`（新增用例）
- [x] 1.2 `board_match_handler.go`：matching-config GET 映射表与 PUT 白名单加入 `semantic_board_match_direct_hit_min_ratio`。验证：`go test ./internal/tagmanagement/handler/ -run TestMatchingConfig`（既有用例过 + 断言新 key 出现）
- [x] 1.3 单测：占比 0.4（2/5）不 direct_hit 且退回相似度路径（相似度不足时不挂载）；占比 0.5（2/4）照常 direct_hit；ratio=0 关闭门槛回到旧行为；tag 无辅助标签时行为不变。验证：`go test ./internal/tagmanagement/service/board/`

## 2. 日报收集排他（topicgraph）

- [x] 2.1 `daily_report_orchestrator.go`：新增 `LoadDailyReportCollectConfig()`（读 `semantic_board_report_exclusive_gap`，默认 0.3，非法回退；gap ≤ 0 关闭）；`collectBoardTags` 主查询 Where 追加 NOT EXISTS 子查询（`stronger.score >= score + gap`，`stronger.semantic_board_id <> 本板`）。验证：`go test ./internal/topicgraph/service/ -run TestCollectBoardTags`
- [x] 2.2 fallback 路径排他：择板循环处若 `matches[0].Score - boardMatch.Score >= gap` 则不收。验证：单测覆盖 fallback 排他场景（`TestCollectBoardTags_FallbackCrossBoardExclusivity`：归档前补齐——复刻 fallback 全链路端到端夹具，gap 0.32 排他 / gap 0.167 双保留 / 主查询回落路径三向验证）
- [x] 2.3 单测（testcontainer PG）：强归属 tag（他板 1.0 本板 0.7）被排他剔除且不参与 weighted 保底回捞；分差 0.1 双挂载均保留；gap=0 排他关闭全收。验证：`go test ./internal/topicgraph/service/ -run TestCollectBoardTags`

## 3. spec 同步与效果核对

- [x] 3.1 `openspec validate fix-board-match-cross-domain` 通过；两份 delta spec 与实现行为一致
- [x] 3.2 效果核对（真库）：SQL 模拟排他后统计各版块日报候选变化——确认 tag 196587（非农）在 10.4 数据集下不再进入「中国国内新闻」候选、双挂载真跨界 tag（分差 <0.3）不受影响；量化「被排他剔除的 tag 数」写入完工汇报。验证：核对 SQL + 结果数字留档
- [x] 3.3 影响包测试收尾：`bash scripts/harness/change-scope.sh` 确认范围，跑 `golangci-lint run ./internal/tagmanagement/... ./internal/topicgraph/...` + 受影响包 `go test`

## 4. 测试

- [x] 4.1 影响包测试全绿（归档前复跑）：`go test ./internal/tagmanagement/service/board -count=1`（含 TestSemanticBoardMatchingDirectHitRatio{Gate,Half,Disabled,BlockedButSimilar} 占比四场景）+ `go test ./internal/topicgraph/service -run CollectBoardTags -count=1`（testcontainer 排他三场景：强归属剔除不回捞 / 分差 0.1 双挂载保留 / gap=0 全收）+ `go test ./internal/tagmanagement/handler -count=1`（config 白名单既有用例回归）；`go vet` / `golangci-lint` 0 issue、`go build ./...` 0 error
- [x] 4.2 真库只读 SQL 效果核对（任务 3.2，test-cases.md 效果核对节四要素齐：160 弱挂载 / 91 排他可救 / 69 源头拦截 / tag 196587 模拟剔除）

## 5. 文档

- [x] 5.1 `docs/reference/flow/semantic-board.md` 约束 3 补 direct_hit 占比门槛语义（含证据不换赛道）+ §变更溯源行
- [x] 5.2 `docs/reference/flow/daily-report.md` 新增约束 28（跨版块排他，主查询与 fallback 同规则、不参与保底回捞）+ §变更溯源行
- [x] 5.3 `docs/reference/api/semantic-boards.md` matching-config GET 示例补 `semantic_board_match_direct_hit_min_ratio`（顺补同表既缺的 `direct_hit_min_overlap`）

## 6. 验证

<!-- doc-impact: flow, api -->
<!-- doc-impact-excuse: database=无任何 schema 变更：daily_report_orchestrator.go 的 NOT EXISTS 子查询为查询层过滤（topic_tag_board_labels 表结构不变），启发式按文件内容含 SQL 关键词误报 -->

Scenario→测试映射（spec delta 两个 capability 共 21 个场景）：

| Scenario | 测试文件 |
| --- | --- |
| 组合标签直接命中 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 直接命中 board 构成标签 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 交集数达标但占比不足退回相似度 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| direct_hit_min_overlap=1 向后兼容 | backend-go/internal/tagmanagement/service/board/semantic_board_backfill_test.go |
| 降级 direct_hit 方向校验不通过 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 组合命中与单标签重叠同时存在 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 无组合标签时行为 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 交集数不足退回相似度匹配 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 占比拦下但剩余标签真相似仍可挂载 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 占比门槛不影响交集数不足的判定 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 用户修改阈值 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| 用户修改 min_hits | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| 用户修改 min_hit_rate | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| 用户调整 direct_hit 占比门槛 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 跨版块排他剔除弱归属标签 | backend-go/internal/topicgraph/service/collect_board_tags_exclusivity_test.go |
| 排他不影响分差不足的双挂载 | backend-go/internal/topicgraph/service/collect_board_tags_exclusivity_test.go |
| 排他剔除不参与保底回捞 | backend-go/internal/topicgraph/service/collect_board_tags_exclusivity_test.go backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| 质量筛选过滤 weighted | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| 保底机制 | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| 截断机制 | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| fallback 路径同样应用排他 | backend-go/internal/topicgraph/service/collect_board_tags_exclusivity_test.go |

- [x] 6.1 `openspec validate fix-board-match-cross-domain` 通过；两份 delta spec 与实现行为一致（占比 AND 条件、重叠标签不作退回相似度证据、NOT EXISTS 排他、fallback 择板同规则、保底不回捞）
- [x] 6.2 归档门禁四项（doc-impact verify / check-standards / check-tasks-tail / scenario-trace）全过
- [ ] 6.3 部署后观察：下一轮日报生成核对「生成式AI 与大模型厂商」候选量（FB-1 观察点：被剔 35 个最多，若变薄 gap 往 0.35~0.4 调，两 key 均 ai_settings 在线可调）——**人工**
