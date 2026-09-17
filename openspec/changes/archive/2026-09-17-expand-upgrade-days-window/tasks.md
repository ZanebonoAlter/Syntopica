# Tasks: expand-upgrade-days-window

## 1. 后端召回时间窗

- [x] 1.1 `UpgradeGenerateRequest` 的 days 透传到 `generateExpandSuggestions` 与 `generateExpandCompose` 调用链（`semantic_board_upgrade.go:186-196` 分发处），验证：`go build ./...` 通过且分发处四个分支均能引用 days
- [x] 1.2 相似路近期活跃过滤：`recallExpandAuxCandidates` 相似路 simHits 收集后、排序截断前，days>0 时按「窗口内有文章引用的标签 ID 集」批量 EXISTS 查询（`IN` 一次执行）过滤内存集合；days=0 跳过。验证：新增单测——days=7 时相似达标但无近期文章的标签被剔除、days=0 时保留（构造测试数据断言候选集差异）
- [x] 1.3 共现路窗口收紧：`loadExpandCooccurrence` 的 cutoff 计算改为 `min(days, CoTagWindowDays)`（days>0 时），days=0 保持全局配置。验证：新增单测——days=3 且 CoTagWindowDays=30 时 cutoff 等效 3 天窗口；days=0 时等效 30 天
- [x] 1.4 compose 段共现窗口同步收紧（`generateExpandCompose` 引用的共现查询），验证同 1.3 的单测口径覆盖 compose 路

## 2. 前端放开显隐与携带

- [x] 2.1 `UpgradeSuggestionPanel.vue:266` v-if 放开为恒显（四格可用），`:67` days 携带条件放开到扩充方向。验证：`pnpm lint` + `pnpm exec nuxi typecheck`（Windows cmd）通过
- [x] 2.2 组件测试更新/新增：扩充方向下 days 下拉可见、生成请求 emit 载荷含 days（data-testid=gen-days 存在性 + emit 断言）。验证：`pnpm test:unit` 相关用例绿

## 3. 收口

- [x] 3.1 影响包全量验证：`bash scripts/change-scope.sh` 判定后端影响包（tagmanagement）跑 `go test ./internal/tagmanagement/...`；前端 `cmd.exe` 跑 `pnpm build`；`golangci-lint run ./...` + `go vet ./...` 全绿

## 4. 测试

- 后端（影响包 tagmanagement，真库 testcontainer）：
  - `go test ./internal/tagmanagement/...` → 全部 ok（含新增 TestRecallExpandAuxCandidatesDaysSimFilter / TestRecallExpandAuxCandidatesDaysCooccurWindow / TestGenerateExpandComposeDaysWindow；既有 expand 用例同步更新签名后全绿）
- 前端：
  - `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm test:unit"` → 848 passed（UpgradeSuggestionPanel.gen.test.ts 按 test-design ⓪ 更新旧契约断言：四格均携带 days）

## 5. 文档

<!-- doc-impact: flow, api -->

- [x] `docs/reference/flow/semantic-board.md`：days 生效范围改为四格恒携、扩充召回双路收紧规则、前端面板分区 days 下拉恒显描述
- [x] `docs/reference/api/semantic-boards.md`：`days` 查询参数语义（create×aux 过滤 / expand 双路收紧 / create×composite 忽略）
- 变更溯源链接待 archive 后按 §12.2 补入 flow/semantic-board.md

## 6. 验证

| 命令 | 期望 | 实测 |
| ------ | ------ | ------ |
| `cd backend-go && go build ./internal/tagmanagement/...` | 通过 | ✅ 2026-09-12 |
| `cd backend-go && golangci-lint run ./internal/tagmanagement/...` | 0 issues | ✅ 0 issues |
| `cd backend-go && go vet ./internal/tagmanagement/...` | 通过 | ✅ |
| `cd backend-go && go test ./internal/tagmanagement/...` | 全部 ok | ✅ 7 包 ok |
| `cd front && pnpm lint` | 0 error | ✅（7 存量 warning 均在无关文件） |
| `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"` | 本次改动文件零错误 | ✅（全仓仅剩并行 change 的 discovery/CandidateEditDialog.vue 2 个中间态错误，非本 change 文件；归档前复跑需对方收口） |
| `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm test:unit"` | 全部通过 | ✅ 848 passed (73 files) |
| `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm build"` | Build complete | ✅ |

Scenario → 测试映射（路径为仓库根相对，用例名见对应文件内同名 Test 函数/用例标题）：

| Scenario | 测试文件 |
| ------ | ------ |
| 扩充方向显示时间窗选择器 | front/app/features/tags/components/UpgradeSuggestionPanel.gen.test.ts |
| 用户切换时间窗口 | front/app/features/tags/components/UpgradeSuggestionPanel.gen.test.ts |
| 组合标签方向不受时间窗影响 | front/app/features/tags/components/UpgradeSuggestionPanel.gen.test.ts |
| days 收紧相似路 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| days 收紧共现窗口 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| days=0 等于现状 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 相似标签被召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 共现标签被召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 已挂载标签不召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 无关标签不被召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 时间窗收紧相似路召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 时间窗收紧共现窗口 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 组合路候选相关性过滤 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 召回为空 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
