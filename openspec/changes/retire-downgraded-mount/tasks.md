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
