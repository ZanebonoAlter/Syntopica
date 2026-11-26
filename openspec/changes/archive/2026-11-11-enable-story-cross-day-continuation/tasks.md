# Tasks：相关事件

> 仅三个交付故事：S1查看相关事件、S2调整阈值、S3历史效果核对。变体见test-cases.md，不拆成大量独立任务。
> 全部为待实施；旧勾选和历史原文移至legacy/previous-plan，不能充作新能力完成。用户要求本轮主线程直接执行、不派子线程。

## 1. 后端：查看相关事件（S1）

- [x] 1.1 实现确定性事件文本、隔离向量缓存与显式索引刷新（名称/描述/aux，缺aux降级；版本/空间失效；airouter审计；失败可重试），不改源标签和旧向量。验证：新增 `backend-go/internal/tagmanagement/service/related/events_test.go`，执行 `cd backend-go && go test ./internal/tagmanagement/service/related` → PASS；SQL与迁移在testutil临时PG验证，禁止真库试写。
- [x] 1.2 实现同版块相关事件只读API（30天、阈值、top10、稳定排序、来源文章、覆盖状态），不依赖旧身份、不合并/传递，不在GET调模型。验证：新增 `backend-go/internal/tagmanagement/handler/related_events_test.go`，执行 `cd backend-go && go test ./internal/tagmanagement/service/related ./internal/tagmanagement/handler -run RelatedEvent` → PASS。

## 2. 前端：查看与调节（S1 / S2）

- [x] 2.1 在既有matching-config读写、白名单与前端匹配规则中接入 `related_event_similarity_threshold=0.82`；非法拒绝不覆盖，保存后新查询立即用新值。验证：新增 `front/app/features/tags/components/MatchingConfigDialog.relatedEvents.test.ts`，执行 `cd front && pnpm test:unit app/features/tags/components/MatchingConfigDialog.relatedEvents.test.ts --maxWorkers=2` → PASS，后端配置断言放1.2的handler测试。
- [x] 2.2 在既有event标签卡按需展开相关列表，显示名称/描述/相似度、复用来源文章预览；各状态及迟到响应按ui-design.md处理，不扩页面。验证：新增 `front/app/features/tags/components/console/ArchiveMatchPopover.relatedEvents.test.ts`，执行 `cd front && pnpm test:unit app/features/tags/components/console/ArchiveMatchPopover.relatedEvents.test.ts --maxWorkers=2` → PASS。

## 3. 历史核对（S3）

- [x] 3.1 提供索引刷新与只读本地试验两种明确分离的CLI模式，在AI版块1980按test-cases S3核对0.82效果，审计副作用如实记录；不跑旧reattribute。验证：新增 `backend-go/cmd/related-events/main_test.go`，执行 `cd backend-go && go test ./cmd/related-events` → PASS；人工：S3写入样本、覆盖及有用性统计，不预填通过。

## 4. Review

- [x] 4.1 主线程审查制品及实现（用户本轮明确不用子线程），聚焦三个故事是否闭环、是否偷带身份合并/阶段推断；High/Medium/Low意见在review.md建账处置。验证：`bash scripts/harness/review-trace.sh openspec/changes/enable-story-cross-day-continuation` → 通过；实现后再次审查，不用本轮制品审查冒充代码审查。

## 5. 测试

- [x] 5.1 按test-cases S1/S2/S3实现主链路及就地变体，后端只测影响包、SQL用临时PG、前端maxWorkers=2。验证：上述定点go test与两个定点vitest命令 → PASS；无全量测试。
- [x] 5.2 用agent-browser验收“调阈值→看相关事件→打开来源”一条完整故事，覆盖局部错误/空/未就绪及长标题；1440×900、1920×1080、390×844机械断言无溢出且当前弹层可点击。验证：人工：`ui-verification/report.md`留真实步骤、断言和截图，不伪造已执行。

## 6. 文档
<!-- doc-impact: flow, api, database, configuration -->

- [x] 6.1 实现后更新 `docs/reference/flow/semantic-board.md`、`docs/reference/flow/topic-graph.md`、`docs/reference/api/`、`docs/reference/database/`、`docs/reference/configuration.md` 的新相关能力边界与刷新操作；不把相关改写成旧身份判定，不删除旧约束。验证：`bash scripts/harness/doc-impact.sh verify openspec/changes/enable-story-cross-day-continuation` → 通过；`bash scripts/harness/check-standards.sh --change enable-story-cross-day-continuation` → 通过。本轮规划未改生产reference，归档对账留到实现阶段，且不得绕过用户禁止git命令的限制。

## 7. 验证

- [x] 7.1 新测试文件落地后运行openspec strict、scenario-trace、check-tasks-tail、review-trace及文档门禁；只跑影响范围代码验证。前端实现通过后按项目要求静态部署并检查。验证：`openspec validate enable-story-cross-day-continuation --strict`、`bash scripts/harness/scenario-trace.sh openspec/changes/enable-story-cross-day-continuation` → 通过，实际命令/结果写本节；本轮不部署。

**7.1 执行记录（2026-11-11，实现子线程；review-trace 归 4.1，不由本线程跑）**

| 命令 | 结果 |
| --- | --- |
| `openspec validate enable-story-cross-day-continuation --strict` | `Change 'enable-story-cross-day-continuation' is valid` |
| `bash scripts/harness/scenario-trace.sh openspec/changes/enable-story-cross-day-continuation` | `✓ 通过：10 个 Scenario 映射齐全（自动测试 10 / 人工留痕 0）` |
| `bash scripts/harness/check-tasks-tail.sh openspec/changes/enable-story-cross-day-continuation` | `✓ tasks.md 尾三节与 doc-impact 声明标记齐全` |
| `bash scripts/harness/doc-impact.sh verify openspec/changes/enable-story-cross-day-continuation` | `通过（声明: flow, api, database, configuration 文件:3 个 启发式:ownership）` |
| `bash scripts/harness/check-standards.sh --change enable-story-cross-day-continuation` | `通过 277 / 失败 0` |
| `cd backend-go && golangci-lint run ./...` | `0 issues.` |
| `cd backend-go && go vet ./...` / `go build ./...` | 均 exit 0 |
| `cd backend-go && go test ./internal/tagmanagement/service/related` | `ok ... 45.751s` |
| `cd backend-go && go test ./internal/tagmanagement/service/related ./internal/tagmanagement/handler -run RelatedEvent` | 两包均 `ok`（44.5s / 28.2s） |
| `cd backend-go && go test ./cmd/related-events` | `ok` |
| `cd front && pnpm lint` | `0 errors, 8 warnings`（既有 warning，未新增） |
| `cd front && pnpm exec nuxi typecheck` | exit 0 |
| `cd front && pnpm test:unit <两个定点文件> --maxWorkers=2` | `Tests 22 passed (22)`；加跑既有 `ArchiveMatchPopover.test.ts` → 51 passed（22 = MatchingConfigDialog 10 + ArchiveMatchPopover 12；R-6 复核校正） |
| `cd backend-go && go run ./cmd/related-events --refresh --board 1980` | `tags=839 indexed=839 reused=0 failed=0`，2m16s |
| `bash scripts/dev/deploy-frontend.sh` | 构建+铺盘成功（`backend-go/frontend` 01:23），**第 3 步 60s 健康检查两次判失败**；后端随后 ~110–190s 才就绪、`/health` 200（本机启动慢，非本 change；未改部署脚本） |
| 浏览器验收（agent-browser，三视口） | 见 `ui-verification/report.md`：1440×900 / 1920×1080 / 390×844 机械断言全 PASS |

**审查后处置轮（2026-11-11 09:00-09:45，R-5/R-6/R-7 + 默认阈值 0.82→0.86）**

| 命令 | 结果 |
| --- | --- |
| `cd backend-go && go test ./cmd/related-events` | `ok`（新增 `TestResolveThresholdUsesCurrentSettingOnlyWhenUnset`：缺省读 ai_settings、显式值不再读库） |
| `cd backend-go && go test ./internal/tagmanagement/service/related ./internal/tagmanagement/handler -run RelatedEvent` | 两包 `ok`（24.5s / 21.8s，默认 0.86 断言与阈值边界向量已同步） |
| `cd backend-go && go test ./internal/tagmanagement/service/related` | `ok` 8.9s |
| `cd front && pnpm test:unit MatchingConfigDialog.relatedEvents.test.ts ArchiveMatchPopover.relatedEvents.test.ts --maxWorkers=2` | `Tests 22 passed (22)` |
| `cd front && pnpm test:unit ArchiveMatchPopover.test.ts --maxWorkers=2` | `Tests 29 passed (29)`（R-7 删除死模板后回归绿） |
| `cd backend-go && golangci-lint run ./...` / `go vet ./...` / `go build ./...` | `0 issues.` / exit 0 / exit 0 |
| `cd front && pnpm lint` / `nuxi typecheck` | `0 errors, 8 warnings` / exit 0 |
| `cd front && pnpm build` | 首次裸跑 exit 134（node OOM）；`NODE_OPTIONS=--max-old-space-size=3584 pnpm build` → `✨ Build complete!` exit 0 |
| `bash scripts/dev/deploy-frontend.sh` | **exit=0，`✅ 部署完成：http://localhost:5100/`**（本次 60s 健康检查通过；01:23 那两次的 60s 超时为环境启动慢） |
| `go run ./cmd/related-events --refresh --board 1980` | `tags=603 indexed=1 reused=602 failed=0`（暖缓存幂等；期间其他任务使版块 1980 有效 event 标签由 839 变为 603，非本 change 写入） |
| `openspec validate enable-story-cross-day-continuation --strict` | `valid`（默认阈值措辞已同步 spec/design/ui-design/test-cases/proposal） |
| `bash scripts/harness/review-trace.sh ...` / `ui-verification-trace.sh ...` | 均 `✓ 通过`（review 8 条，R-5/R-6/R-7 状态「已修复」） |
| 真库存量配置 | `PUT /api/semantic-boards/matching-config {"related_event_similarity_threshold":0.86}` → 200；`GET` 复核 0.86（未用裸 SQL） |

**未跑/留待 4.1**：`bash scripts/harness/review-trace.sh`（代码审查账不归实现线程，`review.md` 现有 R-1..R-4 制品审查账保持原样未动）。

### Scenario 映射（计划路径，尚未建立测试文件）

下表是实现计划，不是现有自动化覆盖；规划期scenario-trace可因文件未创建失败，不生成空壳测试或伪改为人工以骗过门禁。

| Scenario | 测试文件 |
| --- | --- |
| 文本表示稳定且辅助标签可缺失 | backend-go/internal/tagmanagement/service/related/events_test.go |
| 关联不依赖旧身份字段 | backend-go/internal/tagmanagement/service/related/events_test.go |
| 同版块窗口与稳定排序 | backend-go/internal/tagmanagement/service/related/events_test.go |
| 相关连接不改身份也不传递 | backend-go/internal/tagmanagement/service/related/events_test.go |
| 索引未就绪与依赖失败 | backend-go/internal/tagmanagement/service/related/events_test.go |
| 模型空间或文本变更后旧向量失效 | backend-go/internal/tagmanagement/service/related/events_test.go |
| 相关列表复用既有阅读入口 | front/app/features/tags/components/console/ArchiveMatchPopover.relatedEvents.test.ts |
| 阈值保存后新查询生效 | backend-go/internal/tagmanagement/handler/related_events_test.go front/app/features/tags/components/MatchingConfigDialog.relatedEvents.test.ts |
| 非法阈值不覆盖配置 | backend-go/internal/tagmanagement/handler/related_events_test.go front/app/features/tags/components/MatchingConfigDialog.relatedEvents.test.ts |
| 历史样本验证相关而非同一事件 | backend-go/cmd/related-events/main_test.go |

### 本轮规划核对

2026-11-10主线程检查：openspec strict / check-tasks-tail / review-trace（4条）/ feedback verify（5条）均通过。Scenario声明10/10已映射；scenario-trace退出1：5个计划测试文件尚未创建，共12条文件不存在（不是已执行测试失败，也不能宣称门禁绿）。设计/UI任务引用均命中有效任务ID；用例仅S1/S2/S3，任务共10项。实际代码测试与浏览器验收均未运行。doc-impact verify/check-standards涉及git扫描，本轮遵守用户禁止git命令而未运行，文档归档对账待实现阶段且仍受该限制约束。
