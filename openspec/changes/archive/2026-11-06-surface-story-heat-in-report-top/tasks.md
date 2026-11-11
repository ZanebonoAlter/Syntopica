# Tasks

> 本阶段仅完成制品，以下实现/验收任务均未执行。不修改实现或 docs/reference、不提交。执行前先评审 design、test-cases、ui-design。

## 1. 模型与契约

- [x] 1.1 在 `backend-go/internal/topicgraph/repository/daily_report_models.go` 扩展 Highlight 的可空 story_id 与服务端快照字段；`front/app/api/dailyReports.ts` 同步重点/线程 story_id 与派生 is_progress 类型。旧字段缺失与真实 0 必须区分，highlights 恒数组，无 DDL。
- [x] 1.2 在 `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go` 和 `front/app/api/laneDynamics.ts` 增加 hottest_story_id/hottest_story_title；明确赢家同源、存活 id 同分排序、无身份空值，保持 max_participant_count 旧统计范围。
- [x] 1.3 apply 前运行 `bash scripts/harness/doc-impact.sh suggest` 与 `bash scripts/harness/doc-impact.sh context`，按结果核对声明；在 change 下做可丢弃 fake 原型核验排序、同步取消与修订条件，记录结果后销毁，禁止用真库验证写入。

## 2. 后端

- [x] 2.1 在 `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go` 聚合中绑定获胜 id/title/max，合并解析后去重；`board_radar_repository.go` 只做回归核对，不修改雷达阈值与排序。
- [x] 2.2 **独立时序前移任务**：修改 `backend-go/internal/topicgraph/service/daily_report_orchestrator.go`、`daily_report_watch.go`，移除前置并行 highlights 和重复 detached 归属。SaveReport → 有界归属 → 去重 → 候选 → highlights → title/summary → 终态；不改 watch 独立失败边界。
- [x] 2.3 在 `backend-go/internal/topicgraph/service/story_attribute.go` 在线调用链传递 60s ctx，覆盖 DB、embedding/classifier；超时/错误停止在线归属，无迟到写入，保留回填逐条继续策略；`daily_report_story_dedup.go` 仍沿用原 7 天/0.6 判据。
- [x] 2.4 复用 `backend-go/internal/topicgraph/repository/story_heat.go` 的证据算法实现本 report 候选：t=报日北京时间23:59:59，48h/24h，原始 H/P/id 稳定 top8；body 仅筛展示资格，不影响热度。先选择故事/标签模式，归属/去重/候选失败或无可定位线回退标签，禁止伪造身份。
- [x] 2.5 修改 `backend-go/internal/topicgraph/service/daily_report_llm.go` prompt/schema/解析，白名单 story/tag、拒绝无事实条目、重复 story 保首条、合法 [] 不重试、非法最多一次重试；服务端补快照，不让模型填来源/heat。升级 promptVersion，保留 operation/SessionID 与日志。
- [x] 2.6 `backend-go/internal/topicgraph/repository/daily_report_repository.go` 最终同事务写 highlights/title/summary，以本轮保存修订条件拒绝迟到写；详情批量解析存活 id、精确前日重点集合，派生 is_progress，线程 story_id DTO 同步归一，不覆盖人工字段或历史 JSON。失败保持现有标题兜底与 [] 健壮性。

## 3. 前端

- [x] 3.1 `front/app/features/tags/components/console/observationLanes.ts` / `ObservationMatrixGrid.vue` / `ObservationMatrix.vue` 消费 hottest 字段，title/aria 同源、空标题用 #ID、缺身份不冒名，图例替换固定文案；保留断档/零值/点击行为。
- [x] 3.2 `front/app/features/tags/components/daily-report/DailyReportMasthead.vue` 显示进展徽标、来源快照及热度可读说明，旧数据保持静态文案；合法空态不消失。仅可定位时使用键盘可达按钮，保留文案划选。
- [x] 3.3 `front/app/features/tags/components/DailyReportReader.vue` 与 `daily-report/DailyReportTopicSection.vue` 接定位意图、两分支 thread 锚点、按当前 DOM 顺序展开定位首条、focus/2s 单高亮及 report 切换清理；缺目标不跳近似 section，复用轻提示，reduced-motion 不平滑。

## 4. 测试

- [x] 4.1 按 `test-cases.md` 功能/边界/白盒用例更新下表真实既有测试，先证明旧契约断言被改写。仓储只用隔离 PostgreSQL/testcontainer，不用 SQLite，不碰真库写入。
- [x] 4.2 fake LLM/时钟/阻塞依赖验证：归属失败且标签模型返回有效重点时报告仍完成、highlights 非空且 story_id 为空；60s 取消不迟到写；同证据/模型响应重建两次一份报告且候选/重点一致。
- [x] 4.3 保持 null→[]、非法响应一次重试、合法 [] 零重试、空态、composite、人工归属、display_tier 与去重旧断言；添加候选/合并/昨日/定位分支。
- [x] 4.4 人工：使用 agent-browser 在静态 :5100 串行执行完整交互故事与 1440×900、1920×1080/窄屏检查，建账 `ui-verification/report.md`，没有证据不得勾选；对真实已有数据只读抽样量化字段覆盖与故事命中，不重建超窗报告。

## 5. 文档

<!-- doc-impact: flow, api, database -->

- [x] 5.1 实现阶段更新 `docs/reference/flow/daily-report.md` 与 `docs/reference/flow/semantic-board.md`：生成时序、候选/昨日重点/定位与矩阵点名口径；明确来源第一版按 feed、回退和旧数据降级。**当前制品任务不执行这些 reference 改动**。
- [x] 5.2 实现阶段更新 `docs/reference/api/daily-reports.md` 的 lane-dynamics/日报字段与兼容契约；更新 `docs/reference/database/tables/daily-report-watch.md` 的 highlights JSON 扩展（无新列），实施前核对实际文档落点，不另建平行权威文档。
- [ ] 5.3 归档后按规范补两域 flow 的永久 archive 变更溯源链接；完成报告写部署影响/所需操作/旧报降级，无自动历史回填。

## 5b. Review

- [x] 5b.1 派只读子线程聚焦 review（审查制品 + diff，重点：时序前移的降级与迟到写入、候选截断顺序、重建 signals 重算），意见按 High/Medium/Low 处置后 review.md 建账，验证：`bash scripts/harness/review-trace.sh openspec/changes/surface-story-heat-in-report-top` 退出码 0

## 6. 验证

- [x] 6.1 `openspec validate surface-story-heat-in-report-top --strict` → 输出 valid；本阶段仅此制品校验，不声称实现测试通过。
- [x] 6.2 `bash scripts/harness/change-scope.sh` → 机械判定影响包；按结果只测试影响包，禁止顺手全量。
- [x] 6.3 `cd backend-go && go test ./internal/topicgraph/service ./internal/topicgraph/repository` → 相关测试通过（含归属失败标签回退非空、固定证据重建两次一致）；测试 PG 使用一次性容器/库。
- [x] 6.4 `cd front && pnpm test:unit app/features/tags/components/console/observationLanes.test.ts app/features/tags/components/console/ObservationMatrix.test.ts app/features/tags/components/console/ObservationMatrixGrid.test.ts app/features/tags/components/daily-report/DailyReportMasthead.test.ts app/features/tags/components/daily-report/DailyReportTopicSection.test.ts app/features/tags/components/DailyReportReader.test.ts --maxWorkers=2` → 定点组件/定位测试通过，不加参数分隔双横杠。
- [x] 6.5 `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm build` → 全绿，串行执行，不与测试/浏览器并行；实现部署时 `bash scripts/dev/deploy-frontend.sh` → 健康检查通过且静态 :5100 可见新行为。
- [x] 6.6 `bash scripts/harness/doc-impact.sh verify && bash scripts/harness/check-standards.sh` → reference 文档对账及标准校验通过（归档前）；`git diff --cached --name-only` → 本 change 无暂存；`git diff --check -- openspec/changes/surface-story-heat-in-report-top` → 无空白错误。
- [x] 6.7 人工：按 test-cases 效果核对记录真实样本数/字段覆盖率/结论与归属耗时，完成 UI 证据，不执行写库 SQL 探测，不承诺本阶段已取得运行效果。

| Scenario | 测试文件 |
|---|---|
| daily-report-system / 创建日报记录 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| daily-report-system / 日报记录唯一性 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| daily-report-system / 并发落库同板同日不炸唯一约束 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| daily-report-system / 日报关联昨日报告 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| daily-report-system / 线程存储在独立表中 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| daily-report-system / 兄弟桶共享聚类序号 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| daily-report-system / 未分桶 section 取默认桶序号 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| daily-report-system / highlights 落库恒为数组 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| daily-report-system / 历史 null highlights 读出归一为数组 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| daily-report-system / 事件落库时 scope 由事件标签数推导 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| daily-report-system / 事件未归组不影响日报 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| daily-report-system / 并行生成成功 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` |
| daily-report-system / Thread 输出不含 status | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` |
| daily-report-system / 昨日日报不存在 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` |
| daily-report-system / 完整流水线执行 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` |
| daily-report-system / 物化失败不阻断流水线 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` |
| daily-report-system / 生成失败 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` |
| daily-report-system / 分桶失败不阻断流水线 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` |
| daily-report-system / 归属失败标签回退仍出报告 | `backend-go/internal/topicgraph/service/daily_report_watch_integration_test.go` |
| daily-report-system / 归属超时不迟到写入 | `backend-go/internal/topicgraph/service/daily_report_watch_integration_test.go` |
| daily-report-system / 同板同日重建可重算 | `backend-go/internal/topicgraph/service/daily_report_watch_integration_test.go` |
| daily-report-system / 候选按热度而非桶数截断 | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| daily-report-system / 同分与精度不反转 | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| daily-report-system / 来源窗口与半衰期 | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| daily-report-system / 只可从候选事实写文案 | `backend-go/internal/topicgraph/service/daily_report_llm_test.go` |
| daily-report-system / 无事件线走标签回退 | `backend-go/internal/topicgraph/service/daily_report_llm_test.go` |
| daily-report-system / 少量候选与合法空返回 | `backend-go/internal/topicgraph/service/daily_report_llm_test.go` |
| daily-report-system / 重点快照与旧数据兼容 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| daily-report-system / 昨日同线再入选 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| daily-report-system / 缺昨日不借前日 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| daily-report-system / 合并与坏引用 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| daily-report-system / 点击第一条可见正文线程 | `front/app/features/tags/components/DailyReportReader.test.ts` |
| daily-report-system / 失去目标与旧重点 | `front/app/features/tags/components/DailyReportReader.test.ts` |
| daily-report-system / 重复定位与减弱动画 | `front/app/features/tags/components/DailyReportReader.test.ts` |
| daily-report-system / 展示筛选不改变热度 | `backend-go/internal/topicgraph/service/daily_report_story_dedup_test.go` |
| workshop-board-console / 同日分桶不冒充分区热度 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| workshop-board-console / 矩阵格值是桶峰值 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| workshop-board-console / 颜色表达参与者数 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| workshop-board-console / 多线格点名获胜线 | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` |
| workshop-board-console / 来源同分稳定点名 | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` |
| workshop-board-console / 无身份不伪造标题 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| workshop-board-console / 空标题与零来源 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| workshop-board-console / 渲染矩阵 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| workshop-board-console / 图例可见 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| workshop-board-console / 断档可读 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| workshop-board-console / 零线索可开 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| 完整 UI 故事：矩阵点名→重点依据/进展→首条正文定位 | 人工：agent-browser 静态 :5100，证据写本 change ui-verification/report.md |
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
| 并行生成成功 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` |
| Thread 输出不含 status | `backend-go/internal/topicgraph/service/daily_report_llm_test.go` |
| 昨日日报不存在 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` |
| 完整流水线执行 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` |
| 物化失败不阻断流水线 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` |
| 生成失败 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` |
| 分桶失败不阻断流水线 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` |
| 归属失败标签回退仍出报告 | `backend-go/internal/topicgraph/service/daily_report_story_highlights_integration_test.go` |
| 归属超时不迟到写入 | `backend-go/internal/topicgraph/service/daily_report_story_highlights_integration_test.go` |
| 同板同日重建可重算 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| 候选按热度而非桶数截断 | `backend-go/internal/topicgraph/repository/report_story_highlights_test.go` |
| 同分与精度不反转 | `backend-go/internal/topicgraph/repository/report_story_highlights_test.go` |
| 来源窗口与半衰期 | `backend-go/internal/topicgraph/repository/report_story_highlights_test.go` |
| 只可从候选事实写文案 | `backend-go/internal/topicgraph/service/daily_report_story_highlights_test.go` |
| 无事件线走标签回退 | `backend-go/internal/topicgraph/service/daily_report_story_highlights_test.go` |
| 少量候选与合法空返回 | `backend-go/internal/topicgraph/service/daily_report_story_highlights_test.go` |
| 重点快照与旧数据兼容 | front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts |
| 昨日同线再入选 | `backend-go/internal/topicgraph/repository/report_story_highlights_test.go` |
| 缺昨日不借前日 | `backend-go/internal/topicgraph/repository/report_story_highlights_test.go` |
| 合并与坏引用 | `backend-go/internal/topicgraph/repository/report_story_highlights_test.go` |
| 点击第一条可见正文线程 | front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts |
| 失去目标与旧重点 | front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts |
| 重复定位与减弱动画 | front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts |
| 展示筛选不改变热度 | `backend-go/internal/topicgraph/repository/report_story_highlights_test.go` |
| 同日分桶不冒充分区热度 | `front/app/features/tags/components/console/observationLanes.test.ts`, `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 矩阵格值是桶峰值 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| 颜色表达参与者数 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| 多线格点名获胜线 | front/app/features/tags/components/console/observationLanes.test.ts |
| 来源同分稳定点名 | front/app/features/tags/components/console/ObservationMatrixGrid.test.ts |
| 无身份不伪造标题 | front/app/features/tags/components/console/observationLanes.test.ts |
| 空标题与零来源 | front/app/features/tags/components/console/observationLanes.test.ts |
| 渲染矩阵 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 图例可见 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 断档可读 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 零线索可开 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
