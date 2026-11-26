# Tasks: fix-board-direction-gap-exempt

## 1. 判定核心改造

- [x] 1.1 `semantic_board_matching.go`：`computeGlobalBestBoards` 更名 `computeGapExemptBoards` 并增加 margin 参数，集合条件 `bestCos-cos < max(margin, 1e-9)`；`SemanticBoardMatchConfig` 新增 `DirectionGapExemptMargin float64`（默认 0.10，D2）。调用点（directionMismatch 闭包、applyDirectionRelativeGate）同步换名。验证：`cd backend-go && go build ./...` 通过
- [x] 1.2 `MatchTopicTag` 内传入 `config.DirectionGapExemptMargin`；margin ≤ 0 时行为与现版逐位一致（max 归一 1e-9）。验证：`go test ./internal/tagmanagement/service/board -run TestDirection -v` 既有豁免/非最优/地板线用例全绿（回归不改意图）

## 2. 配置键

- [x] 2.1 `LoadConfig` switch 白名单新增 `semantic_board_match_direction_gap_exempt_margin`（`parseDirectionRelativeMargin` 复用语义：负→0 关闭、>1/非法拒绝保持现值——复刻 floor 模式，floor/relative_margin 均不进 config API 白名单，同口径）。验证：`go test ./internal/tagmanagement/service/board -run TestConfig -v` 含默认/自定义/非法三用例通过

## 3. 测试（用例与测试资产）

- [x] 3.1 test-cases.md 已建（主链路 6 步 + ⓪ 继承表 + 变体走查 + 白盒分支表）；按表落地单测：near-best 豁免（差值 0.066）、beyond margin 仍标（0.214）、差值恰=margin 不豁免（严格 <）、rank-1 恒豁免、margin=0 回退并列口径、floor 收回豁免、单板/孤板挂载。验证：`go test ./internal/tagmanagement/service/board -v` 全绿且新增用例 ≥ 7
- [x] 3.2 回归确认：fix-board-ghost-false-positive 的豁免三连（global-best mount exempted / tied global best / floor overrides exemption）不改意图全绿。验证：`go test ./internal/tagmanagement/service/board -run 'TestGlobalBest|TestFloor' -v` 通过

## 4. 真库重算与效果核对

- [x] 4.1 部署后手动触发 rematch-all（D3，distinct tag=2173 实际口径）。验证：重算成功 0 失败；`SELECT count(*) FROM topic_tag_board_labels WHERE direction_mismatch;` 结果 ~166±10%（166=实证 gap10 预期）；mismatch 行 join identity embedding 无空行（82 陈旧清零）
  - 实测（2026-10-22）：2173/2173 成功 0 失败；全库 229（6.9%）；日本新闻 1/54（1.9%）、美国 48/425（11.3%）、科技巨头 159/439；陈旧行 0 ✅
  - 与实证差异留痕：229 - 166 = +63，其中科技巨头 +54（实证口径内 105≈96 ±10% ✓）全部来自相对门路径（mounted_cos≥0.6 但同轮 gap≥0.15 的次优挂载，如「OpenAI 发布 GPT-6.1 Sol」挂科技巨头 0.638 vs 真家生成式AI 0.831）——fix-digest-tag-leak 既有语义，抽样确认非冤案；SQL 模拟器只模拟豁免层+绝对门、未含相对门独立触发，预测口径缺口，非实现偏差
- [x] 4.2 次日体检快照与日报核对：日本新闻(2128) `metrics->'board'->>'ghost_matches'` ≤ 5；日报页恢复高市早苗/央行类 tag（人工留痕本节）。验证：SQL + 日报页截图/文字留痕
  - 实测（2026-10-24 核对）：ghost 快照 10-21 25 → **10-22 三轮均为 3**（≤5 达标，llm_score 65→85）；日报 event_tag_count 10-20 事故期 1 → 10-22 **8**（含高市早苗）→ 10-23 **5**（含央行）✅。字面核对方式：`SELECT metrics->'board'->>'ghost_matches', llm_score FROM board_health_snapshots WHERE semantic_board_id=2128 ORDER BY checked_at DESC LIMIT 3`；日报 SQL：`board_daily_reports` highlights/dynamics LIKE 关键词 + event_tag_count 对比
- [x] 4.3 fix-board-ghost-false-positive 4.2 验收衔接：其预期「ghost 回落个位数」以本 change 4.2 数据为准留痕（D4）。验证：该 change tasks.md 4.2 行补注引用本节结果
  - 已补注：fix-board-ghost-false-positive tasks.md 4.2 行与 test-cases.md 4.2 节均补「以 fix-board-direction-gap-exempt tasks 4.2 数据为准」衔接说明（该 change 单靠全库最优豁免日本余 ghost 21~25，gap 豁免后方达 3）

## 5. 文档

<!-- doc-impact: flow -->

- [x] 5.1 `docs/reference/flow/semantic-board.md`：红线 4 全库最优豁免表述升级为「全库 gap 豁免」（差值 < `semantic_board_match_direction_gap_exempt_margin` 默认 0.10，≤0 回退并列口径），变更溯源表补本 change 行。验证：`grep -c "gap 豁免" docs/reference/flow/semantic-board.md` 期望 ≥ 2
- [x] 5.2 主 spec delta 与 design D2 默认值措辞对齐（0.10 甜点依据引 proposal 实证表）。验证：`grep -c "0.10" openspec/changes/fix-board-direction-gap-exempt/specs/board-direction-check/spec.md` 期望 ≥ 3

## 6. 验证

- [x] 6.1 `cd backend-go && golangci-lint run ./internal/tagmanagement/...`，期望 0 issues
- [x] 6.2 `cd backend-go && go vet ./internal/tagmanagement/service/board`，期望无输出
- [x] 6.3 `cd backend-go && go test ./internal/tagmanagement/service/board`，期望 ok（含 testcontainers DB 用例）
- [x] 6.4 `bash scripts/harness/scenario-trace.sh openspec/changes/fix-board-direction-gap-exempt`，期望退出码 0
- [x] 6.5 `bash scripts/harness/archive-readiness.sh fix-board-direction-gap-exempt`，期望全绿后再归档（归档顺序：fix-board-ghost-false-positive 先行，D4）
  - 2026-10-24 执行：六项全绿（doc-impact 首跑 flow FAIL 为声明域 key 写法，已改 `<!-- doc-impact: flow -->`；归档顺序遵守：ghost 先行)

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| near-best mount within gap margin exempted | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| mount beyond gap margin still flagged | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| rank-1 mount always within gap margin | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| gap margin disabled falls back to tie-only exemption | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| floor revokes gap exemption | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| hit_rate match passes direction check | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| hit_rate match fails direction check | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| weighted match fails direction check | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| relative gate catches multi-mount cross-domain leak | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| legal dual-mount within margin survives | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| single-board mount unaffected by relative gate | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| composite_hit exempt | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| tag or board embedding unavailable | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| direct_hit match | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| custom margin applied | backend-go/internal/tagmanagement/service/board/semantic_board_matching_config_test.go |
| default margin without setting | backend-go/internal/tagmanagement/service/board/semantic_board_matching_config_test.go |
| invalid margin rejected | backend-go/internal/tagmanagement/service/board/semantic_board_matching_config_test.go |

## 7. Review

- [x] 7.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账（High 零发现；M1/M2 已修复、M3 建账留痕归档时拆 commit、L1-L7 已处置，见 review.md R-1~R-8）
