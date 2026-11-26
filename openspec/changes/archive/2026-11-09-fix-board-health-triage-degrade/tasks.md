## 1. Review

- [x] 实现完成后派子线程聚焦 review（审查制品 + diff，重点看「补齐不给可执行建议」「守卫不误杀」「超时不吃兄弟板预算」三条），意见按 High/Medium/Low 处置后在 `review.md` 建账（1 High + 4 Medium + 5 Low 全处置，见 review.md R-1~R-10）

## 2. 契约与校验（漏判补齐）

- [x] 2.1 `backend-go/internal/tagmanagement/service/boardhealth/llm.go`：`validateTriage` 改为 `normalizeTriage(triage, labels) ([]TriageItem, error)` —— 致命集保留（非法 category / 未知或重复 aux_id / evidence 空白 / rewrite 缺 suggestion / 覆盖率低于下限），漏判走补齐
- [x] 2.2 补齐项构造：category 恒为 `insufficient`，evidence 固定文案含「本轮 LLM 未覆盖该标签（返回 N/M 条），下轮再判」，suggestion 留空；MUST NOT 猜 keep/remove/rewrite
- [x] 2.3 新增包级常量 `minTriageCoverage = 0.5`（覆盖率 `<` 该值整轮降级），并在注释里写明实证依据（47–98 标签板漏判 2–28 条，覆盖率实测 71%–96%）
- [x] 2.4 `run.go:198 adjudicateBoardHealth` 改为调用 `normalizeTriage` 并把补齐后的清单写回 `result.Triage`；`run.go:86` 的降级分支语义不变（仍只处理「有 err」）
- [x] 2.5 `llm.go:28` 与 `validateTriage` 顶部注释同步改口径（原注释写「an invalid list degrades the whole verdict」）

## 3. 日志口径

- [x] 3.1 `run.go` 每板一行（成功也打）：`[board-health] board=%d labels=%d covered=%d filled=%d degraded=%v elapsed=%s`
- [x] 3.2 整轮结束一行：`[board-health] round finished boards=%d degraded=%d filled=%d skipped_fresh=%d elapsed=%s`
- [x] 3.3 整轮降级行带 `reason=`（覆盖率不足时含 `covered=%d/%d (%.1f%%) below min coverage`）；沿用 `logging.Warnf`，不新引日志库

## 4. 触发面（三条路径 + 新鲜度守卫）

- [x] 4.1 `boardhealth/run.go` 新增 `RunCheckupWithOptions(ctx, db, llm, Options{BoardIDs []uint; MinInterval time.Duration})`；`RunCheckup(ctx, db, llm)` 保留为全量零守卫包装（CLI 与既有调用点行为不变）
- [x] 4.2 新鲜度守卫：批量查 `MAX(checked_at) GROUP BY semantic_board_id`（一次 SQL），距今 `< MinInterval`（默认 6h）的板跳过；跳过板不调 LLM、不写快照，计数进 round 日志
- [x] 4.3 `tagmanagement/wire.go` 导出 `RunBoardHealthCheckupWithOptions` 包装（供 scheduler 与 handler 共用），保留既有 `RunBoardHealthCheckupWrapped`
- [x] 4.4 `backend-go/internal/admin/scheduler/job_daily_report.go`：整轮预算 `10*time.Minute` → `30*time.Minute`（`:55`）；回填阶段 `backfilled > 0` 后追加一次带守卫触发（同一晚主轮跑过则全部被跳过）
- [x] 4.5 `backend-go/internal/topicgraph/handler/daily_report_handler.go`：`generateSingleBoard` / `generateAllBoards` / `generateSelectedBoards` 在 `saved > 0` 时异步触发体检，scope = 本次板块集合；用 `context.Background()` 派生 30 分钟预算（生成 goroutine 的 ctx 已结束），panic 在 goroutine 边界 recover 并只记日志
- [x] 4.6 后缀注释一并纠正 `job_daily_report.go:408` 那句「Manual TriggerNowWithDate flows through … so the manual path triggers the checkup」——手动端点走的是 handler 的 generate*，不是这条尾步

## 5. 两级时间预算

- [x] 5.1 `run.go` 单板子超时 6 分钟：`checkupOneBoard` 调用外包 `context.WithTimeout`，超时按单板失败处理（日志含 elapsed）
- [x] 5.2 确认整轮 ctx 取消时行为与现状一致：未跑到的板不写快照，轮次正常返回，不 panic

## 6. 测试

- [x] 6.1 `llm_test.go`：`TestValidateTriage` 按 test-cases.md §0 改写（漏覆盖 → 补齐成功；多余未知 aux 仍降级），新增 `TestNormalizeTriageCoverage`（分支表：0% / 49.9% / 50% / 96% / 100%、m=1 且 n=0、全角空格 evidence）
- [x] 6.2 `run_test.go`：桩 LLM 返回 45/47 条 → 快照 score 非空且 triage 长度 47、缺项 insufficient；覆盖率 20% → 整轮降级；日志断言（每板一行含 filled/degraded）
- [x] 6.3 `boardhealth/run_test.go`：守卫用例（6h 内跳过零 LLM 调用、5h59m/6h01m 边界、BoardIDs 限定 scope、非 board id 不产快照）；单板超时（注入慢 seam + 覆盖超时常量）
- [x] 6.4 `job_daily_report_test.go`：尾步 ctx deadline ≈30min；回填 `backfilled > 0` 触发尾步、`backfilled == 0` 不触发
- [x] 6.5 `topicgraph/handler/daily_report_handler_test.go`：手动生成成功触发体检 seam（scope 正确）、saved==0 不触发、panic 被隔离不影响 HTTP 响应
- [x] 6.6 `repo_pg_test.go`：补齐后的 triage 落库可读（PG 集成，沿用现有 testcontainer 模式）

## 7. 文档

<!-- doc-impact: semantic-board -->

- [x] 7.1 `docs/reference/flow/semantic-board.md`：体检触发（三条路径 + 6h 新鲜度守卫）、降级与补齐口径（覆盖率下限）、两级预算，写入对应节并更新「变更溯源」
- [x] 7.2 `docs/reference/flow/semantic-board.md` 的「业务约束与不变量」补一条红线（红线 27）：**分诊清单漏判只能补 ⚪ 证据不足，MUST NOT 补成可执行建议；MUST NOT 因漏判丢弃评分**
- [x] 7.3 已核 `docs/reference/api/` 与 `docs/reference/configuration.md` 无需改动（无新端点、无新配置项，体检触发时机不在其描述范围）

## 8. 验证

| Scenario | 测试文件 |
| --- | --- |
| LLM 产出评分与评语 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| 模型漏判部分标签仍保留整轮裁决 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| 补齐项不被猜成可执行建议 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 覆盖率过低仍整轮降级 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 非法分诊项整轮降级 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| LLM 失败降级不丢统计 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| 降级与补齐都有日志 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| 短实体标签不因低 cosine 被判摘除 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 磁铁标签凭失败子集证据判摘 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 证据不足标签归入下轮再判 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 相对垫底但绝对健康的标签不判分化 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 无组合挂载的板不因 reason_dist 判病 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 真分化仍被拦截 | backend-go/internal/tagmanagement/service/boardhealth/llm_test.go |
| 体检在日报后自动执行 | backend-go/internal/admin/scheduler/job_daily_report_test.go |
| 手动生成日报后也能触发体检 | backend-go/internal/topicgraph/handler/daily_report_handler_test.go |
| 回填补出日报后触发体检 | backend-go/internal/admin/scheduler/job_daily_report_test.go |
| 新鲜度守卫避免重复体检 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| 降级快照不把板块算成新鲜 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| 主轮尾步不受新鲜度守卫约束 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| 单板超时不影响其它板块 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| 体检不改匹配数据 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| 扩充建议仍需手动触发 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |

- [x] 8.1 `cd backend-go && go test ./internal/tagmanagement/service/boardhealth ./internal/tagmanagement ./internal/admin/scheduler ./internal/topicgraph/handler` → 4 包全绿（含 review 修复后复跑；`topicgraph/handler` 在 4 包并行负载下偶发既有 flake `TestKeyEventPut_MarkIdempotentAndRevisionConflict`，单独跑 3 次全绿，与本 change 无关）
- [x] 8.2 `cd backend-go && golangci-lint run ./...` → 期望 0 issue
- [x] 8.3 `cd backend-go && go build ./...` → 期望退出码 0
- [x] 8.4 `bash scripts/harness/check-tasks-tail.sh openspec/changes/fix-board-health-triage-degrade` → 期望输出通过（三固定尾节 + doc-impact 标记 + review 行齐备）
- [x] 8.5 `openspec validate fix-board-health-triage-degrade --strict` → 期望 valid
- [x] 8.6 `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-board-health-triage-degrade` → 期望无 FAIL
- [x] 8.7 `bash scripts/harness/check-standards.sh` → 本 change 在 F 段通过；全局 266/2，两条 FAIL 属其它未归档 change（`add-question-driven-news-research`、`board-health-triage-advice` 的 doc-impact 声明对账），与本 change 无关，已记入交付报告
- [x] 8.8 真库效果核对（**写快照属写操作，用户 2026-11-09 确认重跑**）：2026-11-04 首轮因本地 Qwen 不可达 12/12 全降级（基础设施不可用，非本 change 缺陷）。**2026-11-09 14:31–14:49 重跑**（Qwen 已恢复，`/api/ai/health` healthy=true）：`round finished boards=13 processed=13 degraded=4 filled=10 skipped_fresh=0 elapsed=17m34s`。只读对账：9/13 板 `llm_score` 非空（68/72/72/72/72/82/85/85）+ `triage` 为数组，`labels` 全部照常落库；4 板整轮降级（1974/2242/15220/285673，`llm_score` NULL、`triage` null）且 reason 明确：1974「duplicate aux id 180059」、2242「triage.25.suggestion 类型非法（array≠string）」、15220「duplicate aux id 1372」、285673「triage.0.aux_id 类型非法（string≠uint）」——均为**模型输出结构性非法**（spec 红线 27 要求整轮降级的情形），非本 change 缺陷。历史 6 个漏判板（47/50/54/58/96/98 标签）中 **4 板恢复出分并走 filled 补齐**（2197 filled=4 / 2272 filled=2 / 1980 filled=2 / 3030 filled=2），另 2 板（1974、15220）因上述模型输出重复 aux_id 降级；降级快照未挡后续新鲜度重生（守卫按设计生效）。验收期望「12/12 全非空」未达，如实按 test-cases 结论口径记录：非法项属上游模型输出问题，本 change 的漏判补齐路径已在真库验证。
- [x] 8.9 归档顺序检查：`board-health-triage-advice` 已于 2026-11-06 归档（`openspec/changes/archive/2026-11-06-board-health-triage-advice`），主 spec 已含 triage 文本 → 顺序满足。
