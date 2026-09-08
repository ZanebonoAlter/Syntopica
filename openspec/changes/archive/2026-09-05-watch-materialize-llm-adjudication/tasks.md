# Tasks

<!-- doc-impact: flow database -->

## 1. 分流修复（物化轨误入 label 轨 bug）

- [x] 1.1 补 `WatchTypeLabel` 常量（若无显式定义，对齐迁移 CHECK 取值），`evaluateWatchHitsWithChat` 分流改显式三路：`label`→AI 判定、`keyword`→文本匹配、`keyword_topic`/`sentence_topic`→跳过；`daily_report_watch_test.go` 加分流单测（三类 watch 各验证落组/跳过），跑 `go test ./internal/topicgraph/...` 相关包通过
- [x] 1.2 存量违规 hits 一次性清理（幂等 SQL：物化轨 watch 的 `topic_watch_hits` 行），集成环境执行后验证：物化轨 watch 在 `topic_watch_hits` 无残留、label/keyword 轨历史 hits 不受影响

## 2. 裁决核心

- [x] 2.1 `LoadWatchMaterializeConfig`（`ai_settings` 键 `watch_materialize_llm_filter_enabled` 默认 true / `watch_materialize_candidate_limit` 默认 40，沿用 `LoadWatchSentenceConfig` 模式），单测覆盖缺省回退/非法值忽略/正常覆盖
- [x] 2.2 `adjudicateWatchArticles` 共用裁决函数（system prompt 含「意图限定词 + 实质因果链」标准、JSONMode+JSONSchema `{verdicts[], section_title}`、`validArticleIDs` 幻觉过滤、每篇 title+summary 截 200 runes、候选上限截断 Warn），单测（mock chat）覆盖：正常裁决、related=false 剔除、全幻觉 verdict 过滤、坏 JSON 返回 error、候选超限截断
- [x] 2.3 keyword 轨接入：`MaterializeKeywordWatches` 在 `matchKeywordArticles` 后过裁决，通过文章聚合 + `thread.Confidence` 写裁决置信度 + 板块名保持固定派生；裁决 error 时回退全量聚合（Confidence=1.0）+ Warn；单测覆盖裁决过滤/降级回退/开关关闭零 AI 全量
- [x] 2.4 sentence 轨接入：`MaterializeSentenceWatch` 在 `ListArticlesByTagsForDay` 并集后过裁决，同 2.3 的置信度/降级语义；全剔时不产 section（专属话题按既有规则自然衰减）；单测同 2.3 三态
- [x] 2.5 板块当日标题：verdict 同批 `section_title` 写入 `cluster_label`（通过文章事实锚），空/降级时兜底 watch 名；`watch_materialize_sentence_test.go` / `watch_materialize_keyword_test.go` 补标题两态断言

## 3. 前端装饰与透出

- [x] 3.1 `DailyReportSection` 加 `WatchLabel string`（`gorm:"-"` transient），日报详情读路径对 `lane_tier=watch_*` 板块回填 watch 名（keyword 轨固定名解析 / sentence 轨按归属 watch 回填），后端 handler/repository 测试验证序列化透出且不写库
- [x] 3.2 `SectionWatchBadge` 扩展显示 watch 名装饰（「关键字物化板块 · harness」式，可选 prop），组件测试覆盖有/无 watch 名两态渲染

## 4. 测试

- [x] 4.1 `watch_materialize_integration_test.go` 扩展裁决端到端三态（开启过滤 / 关闭全量 / AI 失败降级），连库跑通过
- [x] 4.2 本地生成一期含物化板块的日报（Docker DB + scheduler 手动触发或 runNow），人工核对：板块标题贴合当日内容、badge 装饰、裁决剔除率日志；`golangci-lint run ./...`、`go vet ./...`、影响包 `go test` 全绿；前端 `pnpm lint` + `cmd.exe` 跑 `nuxi typecheck` + `pnpm test:unit` 通过
- [x] 5.1 文档更新：`docs/reference/flow/daily-report.md`（§1 Step7.5 裁决层描述、业务约束节新增物化裁决红线句、代码入口补裁决函数）+ 变更溯源表补行；`doc-impact.sh verify` + `check-standards.sh` 通过

## 5. 文档

## 6. 验证

- [x] 5.1 `cd backend-go && go test ./internal/topicgraph/... ./internal/platform/database/... -count=1` — 全部 PASS（含连库集成）
- [x] 5.2 `cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...` — 0 issues
- [x] 5.3 `cd front && pnpm lint`（0 错误，5 个 warning 为存量无关文件）+ `cmd.exe /C pnpm exec nuxi typecheck` + `pnpm test:unit`（73 文件 847 用例全绿）+ `pnpm build` — 通过
- [x] 5.4 实弹验证（真库真 AI，board 1980/1974，2026-09-05 期）：美伊板块 7 候选剔 4 留 3、标题「美伊局势升级扰动能源金价」、confidence 0.8 落库；harness 板块 2 候选剔 1 留 1、标题「DeepSeek 获新 harness 插件」、confidence 0.95；零降级；watch_id 持久化；存量违规 hits 清零（迁移 20260905_0001 已应用）

### Scenario → 测试文件映射（scenario-trace-gate 对账表）

| Scenario | 测试文件 |
| --- | --- |
| 含关键字文章聚合为固定话题 | backend-go/internal/topicgraph/service/watch_materialize_integration_test.go |
| 漏网文章可被捞回 | backend-go/internal/topicgraph/service/watch_materialize_integration_test.go backend-go/internal/topicgraph/service/watch_materialize_keyword_test.go |
| 裁决批量单次请求 | backend-go/internal/topicgraph/service/watch_adjudicate_test.go |
| 无命中不产空 section | backend-go/internal/topicgraph/service/watch_materialize_integration_test.go |
| 裁决关闭回退机械聚合 | backend-go/internal/topicgraph/service/watch_materialize_integration_test.go |
| 检索命中并物化 | backend-go/internal/topicgraph/service/watch_materialize_integration_test.go |
| 阈值过滤 | backend-go/internal/topicgraph/service/watch_materialize_sentence_test.go |
| 意图限定词参与裁决 | 人工：真 LLM 裁决质量无法单测断言，tasks 6.4 实弹验证（美伊板块剔 4 留 3 均为市场关联）+ watch_adjudicate_test.go 解析层断言 |
| 因果链判定 | 人工：真 LLM 裁决质量无法单测断言，tasks 6.4 实弹验证 + watch_adjudicate_test.go 裁决标准 prompt 单测（TestAdjudicateWatchArticles_HappyPathSingleCall） |
| 检索句更新后缓存失效 | backend-go/internal/topicgraph/repository/topic_watch_repository_test.go |
| 单轨失败不阻断 | backend-go/internal/topicgraph/service/watch_materialize_integration_test.go |
| 裁决失败回退召回全量 | backend-go/internal/topicgraph/service/watch_materialize_integration_test.go |
| LLM 生成当日标题 | backend-go/internal/topicgraph/service/watch_materialize_integration_test.go |
| 标题兜底链 | backend-go/internal/topicgraph/service/watch_materialize_integration_test.go |
| 历史板块不回刷 | backend-go/internal/topicgraph/service/watch_materialize_integration_test.go |
| 命中记录 | backend-go/internal/topicgraph/service/daily_report_watch_test.go |
| 不走双重确认 | backend-go/internal/topicgraph/service/daily_report_watch_test.go |
| 批量单次请求 | backend-go/internal/topicgraph/service/daily_report_watch_test.go |
| 物化轨关注不进入判定 | backend-go/internal/topicgraph/service/daily_report_watch_test.go |
| 物化轨无命中记录 | backend-go/internal/topicgraph/service/daily_report_watch_test.go backend-go/internal/topicgraph/service/watch_materialize_integration_test.go |
| 删除关键字轨保留历史 | backend-go/internal/topicgraph/handler/topic_watch_handler_pg_test.go |
| 删除一句话轨确认归档 | backend-go/internal/topicgraph/handler/topic_watch_handler_pg_test.go |
