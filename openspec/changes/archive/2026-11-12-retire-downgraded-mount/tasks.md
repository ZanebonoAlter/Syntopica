# Tasks — retire-downgraded-mount

<!-- doc-impact: flow, api, database -->

## 后端

- [x] 1. 匹配层：删 `MatchResult.Downgraded` 与置位/落库逻辑（semantic_board_matching.go :87/:477-486/:494/:845），min(2,N) 门槛保留
- [x] 2. 模型层：删 `TopicTagBoardLabel.Downgraded` 字段（tag_models.go:78）
- [x] 3. 归档口径：`ApplyWeakEvidenceFilter` 去 NOT downgraded 项；`ArchiveEvidenceRank`/`IsWeakEvidence` 签名简化（archive.go）
- [x] 4. Handler：DTO/SQL/赋值清理 + `MatchTier` max_sim 统一档 + ORDER BY 同步（board_match_handler.go）
- [x] 5. topicgraph：TagInput/SELECT/GROUP BY/quality_breakdown 的 downgraded 全清（daily_report_models.go、daily_report_orchestrator.go、daily_report_rebucket.go）
- [x] 6. 后端测试：匹配三规则/MatchTier/归档弱证据/quality_breakdown 的 downgraded 断言改为「字段不存在」反向守卫；`go test ./internal/tagmanagement/... ./internal/topicgraph/...` 全绿

## 迁移与部署

- [x] 7. 迁移脚本 `scripts/db/drop-downgraded-20261112.sql`（ALTER TABLE ... DROP COLUMN IF EXISTS downgraded，幂等）；先发码重启 → 后跑迁移（design D3）
- [x] 8. 重启后端 + 验证：真库只读核对挂载行为不变（同 tag 挂载集合/分数与迁移前一致）、psql \d 确认列已删

## 前端

- [x] 9. 删「已降级 ↓」徽标/「↓」角标/ruleColor 参数（ArchiveMatchPopover.vue）、「⚠降级匹配」后缀（archiveMatchFlow.ts）、弱文章判定条项（useBoardArchive.ts）
- [x] 10. 前端测试：受影响 test 文件断言更新；`pnpm test:unit <受影响文件> --maxWorkers=2` + `pnpm lint` + typecheck

## 文档与收尾

- [x] 11. `docs/reference/flow/semantic-board.md` L1 归档描述去「降级」徽标引用；`docs/reference/api/semantic-boards.md` 示例 JSON 去 downgraded 键；`docs/reference/database/` 两处列描述清理；`doc-impact.sh verify` + `check-standards.sh` 通过（check-standards 唯一失败为 2026-11-09 别 change 的既有溯源欠账，非本 change）
- [x] 12. 汇报部署影响：用户可见变化 = 前端「已降级」徽标消失，其余行为零变化；回滚路径见 proposal Impact

## Review

- [x] 13. 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账
  - 审查结论：High 5 / Medium 4 / Low 4；**核心承诺「挂载行为完全不变」核实成立**（downgraded 从来不是写入条件，逐分支核对无行因删标记而少写；min(2,N) 与 max_sim 判定条件逐字未动；N=1 单 aux 仍挂载有测试守）
  - 处置：H1-H5 全部修（M1/M2 改承诺口径文档不改代码）、M1-M4 修、L1-L3 修、L4 观察（不修）
  - 连带修出 2 个既有 bug：`cmd/dump-sanitizer` 的 `analysis_methods` / `reference_roles` stale 表条目（表已被迁移 DROP，工具此前必然报 `relation does not exist`）、`demo/seed/seed.sql` 按新列清单重新生成

## 复检

- [x] 14. 处置后全量门禁复检：openspec validate --strict / go build / go vet / golangci-lint 0 issues / tagmanagement+topicgraph 测试 / platform-database testcontainer 集成测试 / pnpm lint + typecheck + 8 文件受影响单测（112 passed）/ doc-impact verify —— 全绿（check-standards 唯一失败为 11-09 别 change 的既有溯源欠账）

## 14. 测试

- [x] 14.1 后端：`go test -short ./internal/tagmanagement/... ./internal/topicgraph/...` 全绿（匹配三规则 / MatchTier / 归档弱证据 / quality_breakdown 键全清的反向守卫）
- [x] 14.2 后端集成：`go test ./internal/platform/database/ -count=1` 通过（testcontainer PG，43.5s；golden schema 用例已同步删 `downgraded` 行）
- [x] 14.3 前端：`pnpm test:unit` 跑 9 个受影响文件 + 2 个新增/扩充用例文件（archiveMatchFlow 补 max_sim 无降级文案断言、MatchDetailPanel.maxsim 补 ④ 步骤方向校验断言），全绿
- [x] 14.4 前端静态门禁：`pnpm lint`（0 error，8 warning 为存量他文件）、`pnpm exec nuxi typecheck` 通过
- [x] 14.5 后端静态门禁：`go build ./...`、`go vet ./...`、`golangci-lint run ./...`（0 issues）

## 15. 文档

<!-- doc-impact: flow, api, database -->

- [x] 15.1 `docs/reference/flow/semantic-board.md`：L1 归档描述去「降级」徽标引用（业务约束与不变量节的 min(2,N) 放宽准入红线不变）
- [x] 15.2 `docs/reference/api/semantic-boards.md`：`filtered_tags` 示例 JSON 去 `downgraded` 键
- [x] 15.3 `docs/reference/database/tables/semantic-labels.md`：列清单去 `downgraded` 行 + 退役说明；`docs/reference/database/tables/_conventions.md`：连接表字段描述同步
- [x] 15.4 `docs/reference/standard/backend/testing.md`：回归守卫样本测试名同步（`TestCollectBoardTags_PGHonorsGroupBy`）
- [x] 15.5 openspec 主 spec：6 个 capability 的 delta（`tag-to-board-matching` / `match-detail-ondemand` / `board-article-api` / `match-score-visualization` / `workshop-article-archive` / `daily-report-system`），场景名与主 spec 逐一对齐
- [x] 15.6 `bash scripts/harness/doc-impact.sh verify openspec/changes/retire-downgraded-mount` 通过；`bash scripts/harness/check-standards.sh` 通过 287 / 失败 1（唯一失败为 2026-11-09 `fix-board-health-triage-degrade` 的既有溯源欠账，非本 change）

## 16. 验证

- [x] 16.1 `openspec validate retire-downgraded-mount --strict`，期望 delta 规范有效（openspec 的 MODIFIED 块按场景名对齐，6 个 capability 全部对齐）
- [x] 16.2 `bash scripts/harness/scenario-trace.sh openspec/changes/retire-downgraded-mount`，期望 62 个 delta Scenario 映射齐全
- [x] 16.3 `bash scripts/harness/review-trace.sh openspec/changes/retire-downgraded-mount`，期望 review 留痕有效、High 均已处置
- [x] 16.4 `bash scripts/harness/ui-verification-trace.sh openspec/changes/retire-downgraded-mount`，期望 minor 档验收映射指名道姓（见 ui-design.md Acceptance 节）
- [x] 16.5 真库核对（只读）：迁移前后 `topic_tag_board_labels` 行数 3627 → 3627（零删除）；`downgraded=true` 1001 行 / `false` 2626 行；迁移后 `information_schema` 0 行；后端 `/health` 200
- [x] 16.6 真库行为核对（只读）：board 1980 的关键单 aux 标签挂载与分数**完全未变**——Claude 1.000 / 英伟达 1.000 / Transformer 0.854 / SGLang 0.808 / Coding 0.881 / AI Agent 1.000（hit_rate），印证「只删标记不动挂载行为」
- [x] 16.7 部署核对：前端静态托管已铺盘（`scripts/dev/deploy-frontend.sh`）、后端重启后 `/health` 200、`/tags` 200

| Scenario | 测试文件 |
| --- | --- |
| Scenario | 测试文件 |
| --- | --- |
| 质量序跨分页稳定 | backend-go/internal/tagmanagement/handler/board_article_handler_test.go |
| 标记未入日报 | backend-go/internal/tagmanagement/handler/board_article_handler_test.go |
| 标签描述随标签返回 | backend-go/internal/tagmanagement/handler/board_article_handler_test.go |
| 按遗漏过滤 | backend-go/internal/tagmanagement/handler/board_article_handler_test.go |
| 弱证据过滤 | backend-go/internal/tagmanagement/handler/board_article_handler_test.go |
| 跨版块排他剔除弱归属标签 | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| 排他不影响分差不足的双挂载 | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| 排他剔除不参与保底回捞 | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| 质量筛选过滤 weighted | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| 保底机制 | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| 截断机制 | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| fallback 路径同样应用排他 | backend-go/internal/topicgraph/service/collect_board_tags_test.go |
| 计算 best_tier | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| 前端按质量排序聚类 | backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go |
| 非 direct_hit 场景返回完整 pairs | backend-go/internal/tagmanagement/handler/semantic_board_match_detail_test.go |
| direct_hit 场景同时返回精确匹配和完整 pairs | backend-go/internal/tagmanagement/handler/semantic_board_match_detail_test.go |
| max_sim 降级匹配返回 downgraded 标记 | backend-go/internal/tagmanagement/handler/semantic_board_match_detail_test.go |
| 正常匹配返回 downgraded=false | backend-go/internal/tagmanagement/handler/semantic_board_match_detail_test.go |
| 文章有三种匹配方式的标签 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 所有标签同一种匹配方式 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 正常匹配 tag chip 显示 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 降级匹配 tag chip 显示 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| direct_hit 和其他规则不受降级影响 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 显示分数 | front/app/utils/matchQuality.test.ts |
| direct_hit 分数 | front/app/utils/matchQuality.test.ts |
| matchReasonColor | front/app/utils/matchQuality.test.ts |
| matchInfoLabel | front/app/utils/matchQuality.test.ts |
| 降级匹配流程步骤 | front/app/features/tags/components/MatchDetailPanel.maxsim.test.ts |
| 正常匹配流程步骤不变 | front/app/features/tags/components/MatchDetailPanel.maxsim.test.ts |
| 方向校验通过 | front/app/features/tags/components/MatchDetailPanel.maxsim.test.ts |
| 方向校验未通过 | front/app/features/tags/components/MatchDetailPanel.maxsim.test.ts |
| 命中率超阈值直接挂载（multi-aux） | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| max_sim 超阈值且双因子满足（正常匹配） | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| max_sim 超阈值但 hits 不足 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| max_sim 超阈值但 hit_rate 不足 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| N=1 时降级匹配 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| N=2 时正常匹配 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| 加权综合分挂载（无降级概念） | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| 无任何 board 匹配 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| 多视角挂载含降级标记 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| 超过归属上限时截断 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| 同一天多篇文章 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 一篇多标签 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 主点击读原文 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 标签色彩编码可扫描 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 筛选遗漏 | front/app/features/tags/composables/useBoardArchive.test.ts |
| 弱证据筛选 | front/app/features/tags/composables/useBoardArchive.test.ts |
| 解释弱证据 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 方向校验标记 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 标签描述展开 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 事件辅助×版块收编层级 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 标签卡判定过程逐条呈现 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 判定过程前置失败步给差距 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 收编与交集口径分家 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 判定数值缺失 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 命中步就地展开公式 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 强证据文章判定 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 各阶段得分分组 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 命中即停的阶段语义 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 通用匹配流程说明入口 | front/app/features/tags/components/console/BoardArchiveView.test.ts |
| 仅弱证据文章判定 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
| 无标签 | front/app/features/tags/components/console/ArchiveMatchPopover.test.ts |
