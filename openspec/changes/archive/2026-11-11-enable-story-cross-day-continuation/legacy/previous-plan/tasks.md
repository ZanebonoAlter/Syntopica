[spill] 完整输出 34329 字节已存 .pi/harness/spill/01a1c0a1-6f7a-76b1-aee8-fe8e47fb198c/1794312838809-read-95c8341665d6a603.txt，需要完整内容时用 read 读取该路径（已省略 31769 字节）
--- 头部 2048 字节 ---
# Tasks

> 本阶段以制品为主（proposal / ui-design / specs / design / tasks / test-cases）。**第 0 节（前置：召回盲区修复）已于 2026-11-10 实现并实测**（含代码改动、`docs/reference/` 同步与一次存量回填）；其余实现任务仍为待办（`- [ ]`），执行前先评审 design 与 test-cases。

## 0. 前置：修复召回盲区（无向量事件线永不召回）

- [x] 0.1 在 `backend-go/internal/topicgraph/service/story_attribute.go:createStoryForThread` 建线时若 `thread.Embedding` 为空，用事件 `title` 现算 embedding 写入 `board_stories.embedding`；现算失败只记 warning、不阻断建线；召回口径不变（仍只算 `embedding <=> ?`、`dist ≤ 0.5` top10）；验证：`cd backend-go && go test ./internal/topicgraph/service -run 'Attribute'` 通过。
  - 实现（2026-11-10）：新增包级注入点 `storyTitleEmbedder`（生产实现 `defaultStoryTitleEmbedder` → `airouter.NewRouter().Embed`，`Operation: "topicgraph.story_title_embedding"`、`CapabilityEmbedding`、30s 超时，与 `storySubjectEmbedder` 同形）；`createStoryForThread` 在 `thread.Embedding == "" && thread.Title != ""` 时现算一次写入 `story.Embedding`，失败只 `logging.Warnf`、仍以 NULL 建线（不阻断）。另导出 `service.EmbedStoryTitle(ctx, title)` 供 CLI 回填复用同一 operation（存量回填与在线建线必须同空间）。新增用例 `TestCreateStoryTitleEmbeddingFallback` / `TestCreateStoryTitleEmbeddingFailureDoesNotBlock` / `TestCreateStoryWithThreadEmbeddingSkipsTitleEmbed`；`go test ./internal/topicgraph/service` 通过。
- [x] 0.2 在 `backend-go/cmd/backfill-stories/main.go` 增加 `--fill-story-embeddings`：只补 `embedding IS NULL` 的存量事件线、不覆盖已有向量、`--dry-run` 只统计条数；验证：`cd backend-go && go test ./cmd/backfill-stories` 通过（新增用例覆盖「只补 NULL」与「dry-run 不写库」）。
  - 实现（2026-11-10）：新增 `cmd/backfill-stories/fill_story_embedding
--- 尾部 512 字节 ---
ge）。
- [ ] 10.3 前端暴露：tags console 的「匹配规则」面板新增该阈值项（展示 `ai_settings` 的生效值、可编辑并保存）；面板须显式标明「影子期：只记日志不合并」。
- [ ] 10.4 文档同步：`docs/reference/configuration.md`（默认值表 + `ai_settings` 在线覆盖口径）、`docs/reference/flow/topic-graph.md`（红线 14/15 的阈值与影子期口径）、`docs/reference/flow/daily-report.md`（事件线归属判据表）按实现结果同步。
