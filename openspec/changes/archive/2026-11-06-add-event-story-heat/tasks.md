## 1. 数据模型与迁移

- [x] 1.1 `backend-go/internal/topicgraph/repository/story_models.go`（新增）：定义 `BoardStory` 与 `StorySignal` GORM 模型（字段见 design §6.1，含 `SubjectKey` / `SubjectEmbedding`），`BoardStory.TableName() = "board_stories"`、`StorySignal.TableName() = "story_signals"`；`DailyReportThread` 增加 `StoryID *uint`、`Scope string`、`IsDevelopment bool`、`StoryManual bool`、`DisplayTier string`（body/flash/hidden，跨期去重展示层载体），验证：`grep -n "story_id\|scope\|is_development" backend-go/internal/topicgraph/repository/daily_report_models.go` 命中三列，`grep -n "subject_key\|subject_embedding" backend-go/internal/topicgraph/repository/story_models.go` 命中两列
- [x] 1.2 `backend-go/internal/platform/database/postgres_migrations.go`：新增版本迁移创建 `board_stories`、`story_signals` 两张表与索引（`board_stories_recall_idx`、`story_signals_time_idx`、`story_signals_story_idx`），并给 `daily_report_threads` 加五列（story_id/scope/is_development/story_manual/display_tier，display_tier 含 CHECK body/flash/hidden）+ `daily_report_threads_story_idx`；全部 `ADD COLUMN`/`CREATE TABLE IF NOT EXISTS`，无表重写，验证：迁移在空库与存量库各执行一次均成功，重复执行幂等
- [x] 1.3 迁移测试：存量库执行后 ① 新表存在且索引齐备；② `daily_report_threads` 行数不变；③ 五列默认值正确（`scope='single'`、`is_development=false`、`story_id=NULL`、`story_manual=false`、`display_tier='body'`），验证：`cd backend-go && go test ./internal/platform/database/... -run StoryHeat` 退出码 0

## 2. 事件指纹产出与校验

- [x] 2.1 `backend-go/internal/tagmanagement/service/core/extractor_section.go`：融合提取 schema 与 prompt 增加 `subject` / `subject_key` / `action` / `object`（`occurred_at` 可空），`event` 标签必填、`keyword`/`person` 不输出；prompt 明确要求 `subject_key` 格式为 `类型:规范名`（person/company/org/product/other）并由模型完成别名归并（Trump/川普/特朗普 → `person:trump`），验证：`grep -n "subject_key" backend-go/internal/tagmanagement/service/core/extractor_section.go` 命中 schema、prompt 与解析
- [x] 2.2 `backend-go/internal/tagmanagement/service/core/extractor_enhanced.go`：同步字段（若该路径仍产出 event 标签），保证两条路径结构一致，验证：`go test ./internal/tagmanagement/service/core/...` 退出码 0
- [x] 2.3 指纹一致性校验：新增校验函数，用 `metadata.event_keywords` 比对 `subject`/`object`（包含关系 + `topic_tags.aliases` 展开 + lower/NFKC 归一），不通过标 `fingerprint_trusted=false` 并 warning，验证：单测覆盖「一致 / 不一致 / 关键词缺失」三种输入
- [x] 2.4 迁移/回写：`fingerprint_trusted` 的存储位置（`topic_tags.metadata` 或独立列）按 1.1 的最终模型实现；存量标签不回填指纹，只在新提取时写入，验证：`grep -rn "fingerprint_trusted" backend-go/internal --include=*.go` 命中定义与写入点
- [x] 2.5 主体归一化：实现 design §4 的三层匹配（`subject_key` 精确 → `subject_key` embedding cos ≥ 0.92 → `subject` 字面归一），未命中登记新主体候选；`subject_embedding` 只为已建立的事件线计算一次，不逐篇算，验证：单测覆盖 test-cases §5.2 的 5 个分支 + 边界值（0.92 含 / 0.919 不含）

## 3. 事件线归属判定

- [x] 3.1 `backend-go/internal/topicgraph/service/story_assign.go`（新增）：实现 design §5.1 召回（pgvector，14 天窗口，`merged_into IS NULL`）与 §5.2 五层判定（subject_key 一票否决 / 全同 / 阶段 / object 不同 / 指纹不可信走 LLM）；反向动作表（发布 vs 召回/下架/否认）命中时跳过阶段规则，验证：单测覆盖 test-cases §5.1 的 7 个分支
- [x] 3.2 综合稿拦截：`scope='composite'` 的事件直接返回不建身份；`scope` 由 `tag_ids` 中 event 标签数推导（≥2 = composite），验证：单测断言 composite 事件的 `story_id` 始终为 NULL
- [x] 3.3 合并与地址稳定：事件线合并时写 `merged_into`、`status='merged'`；读取路径用一跳解析（不允许两层链）；合并条件为同一事件以 `SAME_OCCURRENCE` 命中多条事件线（规则层三重指纹全同）；本仓库无 groupReview 多 provider 复核，模型路径按单分类器置信度 ≥0.75 保守处理（偏离记入 review.md），验证：单测覆盖「合并后读取解析到存活者」「两层链被拒绝」「人工线程不被移动」
- [x] 3.4 人工纠正：`story_manual=true` 或 `board_stories.origin='manual'` 的事件，归属判定与回填跳过，验证：单测断言自动判定不改写人工指派，人工拆分（story_id=NULL + story_manual=true）不被回填重挂
- [x] 3.5 调度接入：日报落库后异步执行 `attributeStories(boardID, periodDate)`（复用 `daily_report_watch.go` 的 detached goroutine 模式），失败记录并在下次回填重试，验证：`grep -n "attributeStories" backend-go/internal/topicgraph/service/*.go` 命中调用点

## 4. signals 与热度

- [x] 4.1 `backend-go/internal/topicgraph/repository/story_signals_repository.go`（新增）：按事件的 `related_article_ids` 展开写 signals，`observed_at = articles.pub_date`（缺失回退 `created_at` 并标 `fallback`），`participant_key = "feed:" + feed_id`，`ON CONFLICT (story_id, article_id) DO UPDATE` 收敛，验证：仓储测试断言「pub_date 优先」「重复写入收敛」「归属变更时迁移」
- [x] 4.2 `backend-go/internal/topicgraph/repository/story_heat.go`（新增）：实现 design §5.2 的事件线热度与 lane 热度 SQL，参数版本化 `heat-v1-48h-hl24h`，验证：仓储测试断言半衰期衰减（24h → 0.5、48h → 0.25）、窗口外不计、lane 热度为事件线求和
- [x] 4.3 排除不可见证据：撤回/归档/人工排除的 article 不计入热度；`participation_mode` 若存在则沿用现有排除规则，验证：仓储测试断言排除行不进 heat
- [x] 4.4 落后源标记与重算：feed 超过 `max(3×refresh_interval, 90min)` 未成功采集时标 `incomplete`，恢复后重算受影响日期，验证：单测模拟落后与恢复，断言重算结果与正常计算一致

## 5. 展示口径

- [x] 5.1 `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go`：`LaneDynamicsTimelineDay` 与 section 响应补充 `story_count` 与 `max_participant_count`；`GetBoardLaneDynamics` 的 lane 排序从「活跃天数」改为 `lane_heat` 降序 → 事件数 → id，验证：`go test ./internal/topicgraph/repository/... -run LaneDynamics` 退出码 0
- [x] 5.2 `backend-go/internal/topicgraph/repository/board_radar_repository.go`：`buildRadarRow` 的 `Attention` 改为 `lane_heat`（保留一位小数），移除 Attention 中的 `len(day.Sections)` 累加；legacy `days`/`counts`/`today_sections` 字段按契约保留（grep 仍会命中 `counts[i] = len(day.Sections)`，但 Attention 不再引用），验证：`go test ./internal/topicgraph/repository/... -run Radar` 退出码 0 + Attention 表达式不含 section
- [x] 5.3 雷达阈值标定：跑全量历史计算，取当日活跃版块 `lane_heat` 的 P80/P20 作为热点/常温初始阈值，写入配置文件并记录分布到 design 附录，验证：标定命令输出与写入值记入 `review.md`
- [x] 5.4 前端矩阵：`ObservationMatrixGrid.vue` 格值渲染改为 `story_count`，新增颜色 class（按 `max_participant_count` 分 4 档），title/aria 同时给事件线数/最热来源数/未归组数/主题桶数，`ObservationMatrix.vue` footnote 补图例与「事件身份未回填」降级说明，验证：`cd front && pnpm test:unit app/features/tags/components/console/ObservationMatrixGrid.test.ts app/features/tags/components/console/ObservationMatrix.test.ts --maxWorkers=2` 通过
- [x] 5.5 前端雷达：`BoardRadarView.vue` 字段标签从「当日分区数」改为「当日事件线数」，`boardRadarHelpers.ts` 阈值常量换量纲（临时 10/3，待 5.3 标定）且注释标明依据，验证：`cd front && pnpm test:unit app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/components/console/boardRadarHelpers.test.ts app/features/tags/composables/useBoardRadarOverview.test.ts --maxWorkers=2` 通过
- [x] 5.6 前端接口映射：`front/app/api/normalizers/boardRadar.ts` 增加 `story_count` / `max_participant_count` / `lane_heat` 等字段归一（缺字段时按降级口径），验证：`cd front && pnpm test:unit app/api/normalizers/boardRadar.test.ts --maxWorkers=2` 通过

## 6. 跨期去重

- [x] 6.1 `backend-go/internal/topicgraph/service/daily_report_story_dedup.go`（新增，勿覆盖既有标签去重 `daily_report_dedup.go`）：新增跨期重复判定——查 `story_id` 最近 7 天 `display_tier='body'` 是否进过正文，按「文章集合重叠率 ≥0.6 且无新 feed」判重述降级 `flash`、「重叠 <0.6 或有新 feed 或 is_development」维持 `body`；`story_id` 为空或 composite 维持现状，验证：单测覆盖 test-cases 第 6 节四类输入
- [x] 6.2 降级只作用于展示：重述事件落库照常、signals 照常写、热度不变；降级结果在读取路径可区分（正文/快讯/未展示），验证：单测断言「降级前热度 == 降级后热度」
- [x] 6.3 编排接入：在 `story_attribute.go` 的 `attributeStoriesSafe` 归属完成后应用去重判定（story_id 归属后才存在，故不能在 orchestrator 组装 section 前判定；偏离记入 review.md），验证：`go test ./internal/topicgraph/service/... -run Dedup` 退出码 0

## 7. 回填与降级

- [x] 7.1 `backend-go/cmd/backfill-stories/main.go`（新增）：参数 `--board` / `--from` / `--to` / `--dry-run`；按版块 × 日期升序处理 `story_id IS NULL` 的非综合稿事件；`--dry-run` 输出将新建/合并/跳过计数，验证：`cd backend-go && go build ./cmd/backfill-stories` 退出码 0
- [x] 7.2 回填幂等与并发：已有 `story_id` 跳过；版块级 advisory lock；人工指派不覆盖，验证：集成测试连续执行两次，事件线表计数不变
- [x] 7.3 未回填降级：读取路径在 `story_id` 全空时按「非综合稿事件数」作事件数上界、`COUNT(DISTINCT feed_id)` 算参与者，响应带 `stories_backfilled=false`，验证：单测断言降级标记与数值
- [x] 7.4 回填执行与抽检：在一次性恢复库上 `--dry-run` 看计数 → 正式执行 → 抽检 20 条事件线人工确认（记入 `review.md`），验证：核对输出含「抽检 20 条 / 误合并数」

## 8. 测试

- [x] 8.1 后端影响包测试，验证：`cd backend-go && go test ./internal/topicgraph/... ./internal/tagmanagement/service/core/... ./internal/platform/database/...` 退出码 0
- [x] 8.2 前端影响文件测试，验证：`cd front && pnpm test:unit app/features/tags/components/console/ObservationMatrixGrid.test.ts app/features/tags/components/console/ObservationMatrix.test.ts app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/components/console/boardRadarHelpers.test.ts app/features/tags/composables/useBoardRadarOverview.test.ts app/api/normalizers/boardRadar.test.ts --maxWorkers=2` 通过
- [x] 8.3 质量门禁，验证：`cd backend-go && golangci-lint run ./...` 退出码 0、`go vet ./...` 退出码 0、`go build ./...` 退出码 0
- [x] 8.4 前端 lint 与类型，验证：`cd front && pnpm lint` 退出码 0、`pnpm exec nuxi typecheck` 退出码 0
- [x] 8.5 效果核对（真库量化，按 test-cases §4 七项指标执行并记录），验证：`review.md` 含七项指标实测值与通过/未通过结论

## 9. 文档

<!-- doc-impact: daily-report, semantic-board, topic-graph -->

- [x] 9.1 `docs/reference/flow/daily-report.md`：同步事件模型新增 `story_id`/`scope`/`is_development`、跨期重复降级规则、事件身份与事件的关系；核对「related_article_ids 是 tag 对应文章并集」这条既有事实未被改写
- [x] 9.2 `docs/reference/flow/semantic-board.md`：同步矩阵格值（事件数）、颜色（参与者数）、行序（lane_heat）、雷达关注度与升温判据的新口径；更新「日格=桶峰值」旧表述
- [x] 9.3 `docs/reference/flow/topic-graph.md`：同步 event 标签新增结构化指纹字段与校验规则、`fingerprint_trusted` 语义
- [x] 9.4 `docs/reference/database/`：补 `board_stories`、`story_signals` 表结构与索引、`daily_report_threads` 三列
- [x] 9.5 `docs/reference/api/`：若矩阵/雷达响应新增字段（`story_count`/`participant_count`/`lane_heat`），补字段说明
- [ ] 9.6 变更溯源：archive 后按 §12.2 补 flow 变更溯源链接

## 10. 验证

- [x] 10.1 `cd backend-go && go test ./internal/topicgraph/... ./internal/tagmanagement/service/core/... ./internal/platform/database/...` → 退出码 0
- [x] 10.2 `cd backend-go && golangci-lint run ./...` → 退出码 0
- [x] 10.3 `cd backend-go && go vet ./...` → 退出码 0
- [x] 10.4 `cd backend-go && go build ./...` → 退出码 0
- [x] 10.5 `cd front && pnpm lint && pnpm exec nuxi typecheck` → 退出码 0
- [x] 10.6 `cd front && pnpm test:unit <8.2 六个文件> --maxworkers=2` → 通过
- [x] 10.7 `openspec validate add-event-story-heat --strict` → 退出码 0
- [x] 10.8 `bash scripts/harness/scenario-trace.sh openspec/changes/add-event-story-heat` → 退出码 0
- [x] 10.9 `bash scripts/harness/doc-impact.sh verify openspec/changes/add-event-story-heat` → 退出码 0
- [x] 10.10 `bash scripts/harness/check-standards.sh --change add-event-story-heat` → 退出码 0
- [x] 10.11 `bash scripts/harness/check-tasks-tail.sh openspec/changes/add-event-story-heat` → 退出码 0
- [x] 10.12 端到端（agent-browser，静态 :5100）：打开某版块矩阵 → 确认格值可读、图例可见、title/aria 带具体数字 → 打开雷达确认排序与分档 → 以 1440×900 与 1920×1080 两档截图，验证：证据建账 `openspec/changes/add-event-story-heat/ui-verification/report.md`
- [x] 10.13 Review：派 `change-review` 子线程只读审查全量制品与 diff（重点审：subject 一票否决是否被向量覆盖、阶段规则与反向动作表是否漏分支、signals 的 observed_at 是否真的取 pub_date、热度是否仍有从 section/事件派生的残留、回填幂等与人工指派保护），意见按 High/Medium/Low 处置后在 `review.md` 建账，验证：`bash scripts/harness/review-trace.sh openspec/changes/add-event-story-heat` 退出码 0

### Scenario → 测试文件

| Scenario | 测试文件 |
| --- | --- |
| 每片一次调用产出候选 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go` |
| 同一次调用产出事件指纹 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go` |
| 指纹校验失败不阻断 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go` |
| 关键词缺失时指纹不可信 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go` |
| 单片失败不影响其余片 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go` |
| 单标签 aux 校验失败降级保留 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go` |
| keyword 缺 description 单标签跳过 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go` |
| 同一次调用产出指纹 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go` |
| 指纹与关键词一致 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go` |
| 指纹校验失败不阻断标签 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go` |
| 无关键词可校验时降级 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go` |
| 事件未归组时字段为空 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| 事件线跨天聚合 | `backend-go/internal/topicgraph/service/story_assign_test.go` |
| 被合并的事件线保留地址 | `backend-go/internal/topicgraph/service/story_assign_test.go` |
| 主体不同一票否决 | `backend-go/internal/topicgraph/service/story_assign_test.go` |
| 同一次发生合并 | `backend-go/internal/topicgraph/service/story_assign_test.go` |
| 阶段推进标记为进展 | `backend-go/internal/topicgraph/service/story_assign_test.go` |
| 同主体不同对象分开 | `backend-go/internal/topicgraph/service/story_assign_test.go` |
| 指纹缺失走模型且保守处理 | `backend-go/internal/topicgraph/service/story_assign_test.go` |
| 三事件综合稿不建身份 | `backend-go/internal/topicgraph/service/story_assign_test.go` |
| 单事件文章正常归组 | `backend-go/internal/topicgraph/service/story_assign_test.go` |
| 人工指派不被自动覆盖 | `backend-go/internal/topicgraph/service/story_assign_test.go` |
| 人工拆分后不回滚 | `backend-go/internal/topicgraph/service/story_assign_test.go` |
| 回填幂等 | `backend-go/internal/topicgraph/service/story_assign_test.go`, `backend-go/cmd/backfill-stories/main_test.go` |
| 回填不阻塞在线读写 | `backend-go/cmd/backfill-stories/main_test.go` |
| 未回填时保守降级 | `backend-go/internal/topicgraph/repository/story_heat_test.go`, `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 创建日报记录 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| 日报记录唯一性 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| 并发落库同板同日不炸唯一约束 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| 日报关联昨日报告 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| 线程存储在独立表中 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| 兄弟桶共享聚类序号 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| 未分桶 section 取默认桶序号 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| highlights 落库恒为数组 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| 历史 null highlights 读出归一为数组 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| 事件落库时 scope 由事件标签数推导 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| 事件未归组不影响日报 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| 重述降级为快讯 | `backend-go/internal/topicgraph/service/daily_report_dedup_test.go` |
| 新来源判为进展 | `backend-go/internal/topicgraph/service/daily_report_dedup_test.go` |
| 当事方新动作判为进展 | `backend-go/internal/topicgraph/service/daily_report_dedup_test.go` |
| 未归组事件不参与去重 | `backend-go/internal/topicgraph/service/daily_report_dedup_test.go` |
| 降级不改变热量 | `backend-go/internal/topicgraph/service/daily_report_dedup_test.go` |
| 读取路径只回正文线程 | `backend-go/internal/topicgraph/service/daily_report_story_dedup_test.go` |
| 信源时间优先 | `backend-go/internal/topicgraph/repository/story_signals_test.go` |
| 重复写入收敛 | `backend-go/internal/topicgraph/repository/story_signals_test.go` |
| 归属变更时证据迁移 | `backend-go/internal/topicgraph/repository/story_signals_test.go` |
| 排除不可见证据 | `backend-go/internal/topicgraph/repository/story_signals_test.go` |
| 同一 feed 多条不叠加 | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| 不同 feed 各计一次 | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| 半衰期衰减 | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| 窗口外不计入 | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| 话题热度为事件线热度之和 | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| 桶数变化不影响热度 | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| 重述降级不影响热度 | `backend-go/internal/topicgraph/service/daily_report_dedup_test.go` |
| 事件线合并后热度合并 | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| 历史重算一致 | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| 落后源标记后重算 | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| 矩阵数字为事件线数 | `backend-go/internal/topicgraph/service/lane_snapshot_test.go`, `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| 无事件但有分区 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 雷达排序与热度一致 | `backend-go/internal/topicgraph/repository/board_radar_repository_test.go`, `front/app/features/tags/components/console/BoardRadarView.test.ts` |
| 同日分桶不冒充分区热度 | `front/app/features/tags/components/console/observationLanes.test.ts`, `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 矩阵格值是桶峰值 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| 颜色表达参与者数 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| 渲染矩阵 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 图例可见 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 断档可读 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 零线索可开 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 热点行带理由与密度 | `front/app/features/tags/components/console/BoardRadarView.test.ts` |
| 常温版块折叠 | `front/app/features/tags/components/console/BoardRadarView.test.ts` |
| 关注度不因分区数虚高 | `front/app/features/tags/components/console/boardRadarHelpers.test.ts`, `front/app/features/tags/composables/useBoardRadarOverview.test.ts` |
| 升温判定 | `front/app/features/tags/components/console/boardRadarHelpers.test.ts` |
| 分区多但事件少不升温 | `front/app/features/tags/components/console/boardRadarHelpers.test.ts` |
| 无升温 | `front/app/features/tags/components/console/BoardRadarView.test.ts` |
| 跨语言别名归并 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go` |
| 同公司不同写法归并 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go` |
| 不同主体不因向量相近而合并 | `backend-go/internal/topicgraph/service/story_assign_test.go` |
| 三层均不命中时新建 | `backend-go/internal/topicgraph/service/story_assign_test.go` |
| 反向动作不走阶段规则 | `backend-go/internal/topicgraph/service/story_assign_test.go` |
| 跨语言别名归一为同一主体键 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go` |
