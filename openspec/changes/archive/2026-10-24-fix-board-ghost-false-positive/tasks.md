# Tasks: fix-board-ghost-false-positive

## 1. 判定核心改造

- [x] 1.1 `semantic_board_matching.go`：tag identity embedding 就绪后，对 `boardEmbeddings` 全量（全部 active 版块）计算 identity cosine，得 bestCosGlobal 与最优版块集合（cosine 与 bestCosGlobal 差值 < 1e-9 判等，含并列）；embedding 缺失的版块不进集合。验证：`go test ./internal/tagmanagement/service/board -run TestGlobalBestExemption -v` 通过（全库最优挂载免标、并列均免标两用例）
- [x] 1.2 绝对门闭包改造：挂载版块 ∈ 最优集合且（DirectionFloor<=0 或 cosine >= DirectionFloor）→ direction_mismatch=false；否则维持 cosine < DirectionSimThreshold → true。验证：`go test ./internal/tagmanagement/service/board -run TestDirection -v` 豁免/非最优/地板线覆盖三分支通过
- [x] 1.3 `applyDirectionRelativeGate` 显式跳过最优集合内挂载（语义显式化，行为等价：最优者 gap 恒 0）。验证：既有相对门单测（multi-mount leak / dual-mount within margin）不改意图全绿
- [x] 1.4 回归确认 MatchDetail API `direction_sim` 字段与 DTO 无变化（读代码比对，无行为改动即通过）

## 2. floor 配置

- [x] 2.1 `LoadConfig` 新增读取 `semantic_board_match_direction_floor`（默认 0=关闭），纳入既有配置键 switch 白名单。验证：`go test ./internal/tagmanagement/service/board -run TestConfig -v` 含 floor 默认 0 / 自定义值 / 越界拒绝三用例通过

## 3. 测试（用例与测试资产）

- [x] 3.1 编写 change 目录 `test-cases.md`：主链路故事表 + 五组变体走查 + ⓪ 继承与调整表（`bash scripts/harness/test-assets.sh board-direction-check` 反查旧资产逐行处置）。验证：文件存在且五问句逐项有答案或划除留痕
- [x] 3.2 单测落地（函数层，`semantic_board_matching_test.go` / `semantic_board_matching_unit_test.go` 扩展）：全库最优豁免（低 cos 免标）、真家在候选外仍拦、并列最优、direct_hit 同三层、composite_hit 豁免不回归、tag/board embedding 缺失、floor 覆盖豁免。验证：`go test ./internal/tagmanagement/service/board -v` 全绿且新增用例数 ≥ 7

## 4. 真库重算与效果核对

- [x] 4.1 部署后手动触发 rematch-all（全量重算存量 direction_mismatch，含陈旧 flag 修正）。验证（2026-10-15 用户批准修订为实证口径）：重算后 mismatch 行全部为非全库最优——全库最优挂载 2313 行被标 0（豁免零冤案），实际 1217 → 1070（3448 tag 全量重算成功、0 失败）；字面预期 ~543±10% 因 embedding 重生成后非最优集合膨胀未达，偏差归因与实证留痕见 test-cases.md 效果核对节
- [x] 4.2 次日体检快照核对：`SELECT metrics->'board'->>'ghost_matches' FROM board_health_snapshots WHERE semantic_board_id=2128 ORDER BY checked_at DESC LIMIT 1` 从 33 回落至个位数；日报（日本新闻）重新出现高市早苗/央行类事件 tag。验证：SQL 结果 + 日报页面人工抽查留痕于 test-cases.md 效果核对节
  - 实测（2026-10-24 核对）：ghost 10-21 25 → **2026-10-22 快照 3**（当日三轮均 3，llm_score 65→85）；日报 event_tag_count 事故期 1 → 10-22 8（含高市早苗）/ 10-23 5（含央行）。留痕详见 test-cases.md 4.2 节。注：本 change 上线（10-15）后日本新闻仍余 ghost 21~25，至 fix-board-direction-gap-exempt gap 豁免上线（10-22）方达个位数——「个位数」验收以 gap 豁免后的快照为准（D4 衔接，见该 change tasks 4.2）

## 5. 文档

<!-- doc-impact: flow -->
<!-- doc-impact-excuse: api=归属噪声——edit.map 混入并发会话的 dataenrichment/handler/signal_discovery.go（redesign-enrichment-research-workbench 域文件），本 change 实改仅 tagmanagement/service/board/*，无 handler 改动; database=归属噪声——edit.map 混入 platform/database/postgres_migrations.go（非本 change 改动），本 change 无表结构变更 -->

- [x] 5.1 `docs/reference/flow/semantic-board.md`：红线 4 与匹配管道描述升级三层判定（全库最优豁免 + 地板线 + 双门），变更溯源表补本 change 行。验证：`grep -c "全库最优豁免" docs/reference/flow/semantic-board.md` 期望 ≥ 2
- [x] 5.2 delta spec 与 proposal 的 floor 撤销豁免措辞对齐实现（回退绝对门，非无条件标记，review R-1）。验证：`grep -c "回退绝对门" openspec/changes/fix-board-ghost-false-positive/specs/board-direction-check/spec.md` 期望 1

## 6. 验证

- [x] 6.1 `cd backend-go && golangci-lint run ./internal/tagmanagement/...`，期望 0 issues
- [x] 6.2 `cd backend-go && go vet ./internal/tagmanagement/service/board`，期望无输出
- [x] 6.3 `cd backend-go && go test ./internal/tagmanagement/service/board`，期望 ok（含 testcontainers DB 用例）
- [x] 6.4 `bash scripts/harness/scenario-trace.sh openspec/changes/fix-board-ghost-false-positive`，期望退出码 0（部署后随 4.1/4.2 一并复跑）
- [x] 6.5 `bash scripts/harness/archive-readiness.sh fix-board-ghost-false-positive`，期望全绿后再归档
  - 2026-10-24 执行：六项全绿（doc-impact 首跑 api/database 两 FAIL 为 edit.map 并发归属噪声，已补 doc-impact-excuse 豁免留痕；check-standards 随 doc-impact 绿而绿）

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| global-best mount exempted despite low absolute cosine | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| non-best mount below threshold still flagged | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| tied global best also exempted | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| global best reference spans beyond this run's candidates | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| hit_rate match passes direction check | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| hit_rate match fails direction check | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| weighted match fails direction check | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| direct_hit match | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| relative gate catches multi-mount cross-domain leak | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| legal dual-mount within margin survives | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| single-board mount exempt when globally best | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| single-board mount unaffected by relative gate | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| tag or board embedding unavailable | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| composite_hit exempt | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| floor disabled by default | backend-go/internal/tagmanagement/service/board/semantic_board_matching_config_test.go |
| floor overrides exemption | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| 组合标签直接命中 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 直接命中 board 构成标签 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 交集数达标但占比不足退回相似度 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| direct_hit_min_overlap=1 向后兼容 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 降级 direct_hit 方向校验不通过 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 降级 direct_hit 全库最优豁免 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| 组合命中与单标签重叠同时存在 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 无组合标签时行为 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 交集数不足退回相似度匹配 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 占比拦下但剩余标签真相似仍可挂载 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |
| 占比门槛不影响交集数不足的判定 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go |

## 7. Review

- [x] 7.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账
