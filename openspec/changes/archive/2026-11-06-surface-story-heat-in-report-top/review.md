# Review 记录 — surface-story-heat-in-report-top

> 审查执行：2026-11-06，只读审查子线程（模型 `openai-codex/gpt-6.1-terra:high`，档 `reviewer`）。
> 审查范围：proposal / design / ui-design / tasks / test-cases / specs delta + 后端实现 diff（前端部分由另一轮验收覆盖）。

### R-1 2026-11-06【High】重建日报后旧 signals 残留，热度与来源数被高估
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/repository/daily_report_repository.go`（`SaveReport` 删除旧线程后的收尾）
- **事实**：删旧线程后只调 `recomputeBoardStoryCounts`，而它读取的是**现存** `story_signals`；真正按当前线程重建证据的 `recomputeSignalsForStoryWith` 未被调用。
- **影响**：重建后，旧线程已不再覆盖的文章/feed 仍贡献热度与来源数 → 矩阵颜色、雷达排序、日报重点候选全部失真；违反「重建不累计旧证据」的契约。
- **处置**：在受影响 story 循环里先 `RecomputeSignalsForStory` 再 `recomputeBoardStoryCountsWith`（顺序敏感：participant_count 读新 signals），仍 best-effort 不阻断保存。新增 PostgreSQL 隔离容器测试：重建移除某来源后断言旧 signal 消失、行数/参与者数/热度同步下降。
- **复验**：`go test ./internal/topicgraph/repository/ -run 'Story|Heat|Signal'` 通过；测试先红（重建后 signals 仍 2 行）后绿（1 行）。

### R-2 2026-11-06【High】候选事实只认常规聚类标签，物化线索被误剔除
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/service/daily_report_story_highlights.go`、`repository/report_story_highlights.go`
- **事实**：传给重点生成的 `inputs.tags` 在 watch 物化**之前**固定；物化分支另行检索事件标签。`eligibleStoryFacts` 只认前者中的 ID，且仓储**先截 top 8 再过滤**。
- **影响**：有正文的物化故事被剔除；若前 8 条全被剔除，会错误回退标签模式，且不会补入第 9 条可用候选。
- **处置**：① 新增 `storyHighlightCandidates`，按候选线程标签并集**批量**取事实（覆盖物化标签）；② 截断移到事实确认**之后**（`maxStoryHighlightCandidates=8`）。新增两用例：物化故事能入选；前 8 全无事实时补入第 9 条。

### R-3 2026-11-06【Medium】`CollectBoardIDsForDate` 在 SQLite 单测库报 `no such table: topic_tags`
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/repository/daily_report_repository.go`
- **事实**：新函数 join 了标签相关表，而部分 SQLite 单测只迁移日报表 → 门禁 `go test -short ./internal/topicgraph/...` 红灯（[回归]）。
- **处置**：入口加 `Migrator().HasTable` 守卫（任一依赖表缺失即返回空结果、不报错），沿用 `SaveThreads` 的既有降级契约；未逐个补测试库迁移清单（避免同类问题反复踩坑）。连带把依赖「DROP TABLE 注入失败」的既有测试改为缺列空表，测试契约不变。

### R-4 2026-11-06【Low】`design.md` 与实现的截断顺序表述不一致
- **状态**：已采纳
- **事实**：design §4.1 字面写「先截 8 再验资格」，实现（及修复后语义）是「先确认事实可用再截 8」。
- **处置**：以实现为准，文档（`flow/daily-report.md`）按「先确认事实再截 8」写；design 保留原表述作为决策记录，差异在此条目留痕。

### 记录在案的取舍
1. **时序前移**：归属从 detached goroutine 改为 `SaveReport` 后**有界同步**（≤60s），换取重点生成能读到事件线与热度；失败/超时回退标签路径，`story_id` 为空。迟到写入用 `WHERE id = ? AND updated_at = ?` 修订条件拒绝覆盖。
2. **旧报告不补造**：历史报告不重算 `story_id`/热度快照，前端在缺字段时完全不显示来源数与「进展」徽标（`ui-verification/report.md` A-7 已验）。
3. **UI 未覆盖一项**：新生成日报才能端到端看到「进展」徽标与点击定位（需真实 LLM + 写库），本 change 明确不做历史回填；分支逻辑由单测覆盖（A-8 记录为未覆盖并写明原因）。
