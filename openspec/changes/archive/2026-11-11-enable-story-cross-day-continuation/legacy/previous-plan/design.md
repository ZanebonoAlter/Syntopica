[spill] 完整输出 39079 字节已存 .pi/harness/spill/01a1c0a1-6f7a-76b1-aee8-fe8e47fb198c/1794312838796-read-556d4edaad358328.txt，需要完整内容时用 read 读取该路径（已省略 36519 字节）
--- 头部 2048 字节 ---
# Design — 事件线跨天延续（两层判定）

## Context

见 proposal.md「Why」——用户要的是「事件线真的能跨天」，而今天的事件线是「当天的一条线索」。本设计只回答**判定层怎么改**，不回答界面（ui-impact: none，见 ui-design.md）。

实施现状（实读代码，非推测）：

| 事实 | 落点 |
|---|---|
| 归属判定是**单层五层规则**，同日与跨天共用同一套判据；`object` 严格相等是硬否决 | `backend-go/internal/topicgraph/service/story_assign.go:classifyStoryCandidate` |
| 信任判据要求 `subject` 与 `object` **各自**能在 `event_keywords` + `aliases` 里双向命中；`object` 命不中即 `FingerprintReasonObjectMiss` | `backend-go/internal/topicgraph/service/story_fingerprint.go:ValidateEventFingerprint` |
| 指纹由**同一次提取调用**产出（`subject`/`subject_key`/`action`/`object`/`occurred_at` 五字段），存 `topic_tags.metadata.event_fingerprint` | `backend-go/internal/tagmanagement/service/core/extractor_section.go:EventFingerprintSchema`、`core/tagger.go:mergeFingerprintMetadata` |
| 阶段推进的日期判据用**事件线的建线原点日**（`board_stories.occurred_at`，建线当天写入，后续 attach 不更新） | `backend-go/internal/topicgraph/service/story_assign.go:stageDatesWithinOneDay` |
| 候选召回只带出**最新一条证据**的报告日（`evidence_report.period_date`），正是「最近证据日」 | `backend-go/internal/topicgraph/repository/story_repository.go:RecallStoryCandidates` |
| `board_stories.participant_count` 是**生命周期累计**（`recomputeStoryParticipantsWith` 按全部 signals 去重），矩阵日格颜色直接用它 | `backend-go/internal/topicgraph/repository/story_repository.go:recomputeBoardStoryCountsWith`、`repository/lane_snapshot_repository.go:foldDay` |
| 雷达侧「来源数」已按**当日窗口**从 signals 现算（不受累计值影响） | `backend-go/internal/topicgraph/repository/board_radar_stories.go:radarStor
--- 尾部 512 字节 ---
体的延续」覆盖率过低（例如 <10%），再单开 change 放宽到 N 天 —— 届时用数据决定 N，**不在本 change 里预先放宽**（避免无依据的过拟合）。
- **`--reattribute` 的默认日期窗**：现按 `--from/--to` 显式指定；是否给一个「最近 30 天」的默认值，等实施期看命令行实际使用体验再定。
- **人工复核入口的产品形态**（按钮位置、批量拆分、复核队列）：属另开 change 的范围，本 change 不预判。
