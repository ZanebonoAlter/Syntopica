## 1. 后端：删除级联解绑（用例先行）

- [x] 1.1 改写复现测试 `backend-go/internal/admin/handler/ai_handler_test.go`：将 `TestDeleteProviderBlocksLinkedProvider` 改为级联解绑语义——构造 provider 挂在 2 条线路（含不同 capability）上，调 `DeleteProvider`，断言 200、`ai_route_providers` 中该 provider 关联数为 0、provider 行已删、message 含解绑数量（先跑确认红：现实现 409）。验证：`cd backend-go && go test ./internal/admin/handler -run TestDeleteProvider -count=1`
- [x] 1.2 修改 `DeleteProvider`（`backend-go/internal/admin/handler/ai_handler.go`）：按 design D1/D2，单事务内先查并删除 `AIRouteProvider` 关联（记录条数）再删 provider；成功 message 为 `provider deleted (detached from N route(s))`（N>0）或 `provider deleted`（N=0）；移除 409 拦截分支。验证：`go test ./internal/admin/handler -run TestDeleteProvider -count=1` 全绿

## 2. 前端：删死代码与文案

- [x] 2.1 `front/app/features/ai/composables/useAIRouterSettings.ts` 的 `deleteBackupProvider`：删除成功后的 `removeProviderFromRoute` 循环死代码（design D3，`loadData()` 已重建线路选择）；确认弹窗文案追加「将从所有线路解绑」。验证：`cd front && pnpm test:unit useAIRouterSettings --maxWorkers=2`
- [x] 2.2 `useAIRouterSettings.test.ts` 补/改断言：删除挂线路 provider 成功路径不调用 `removeProviderFromRoute`、调用 `loadData`（若现有 mock 场景需同步调整则一并更新）。验证：同上命令全绿

## 2b. 前端补漏（2026-09-23 用户验收反馈：按钮 disabled 死锁语义 + 原生 confirm）

- [x] 2b.1 `front/app/composables/useConfirm.ts` + `front/app/components/ui/AppConfirmDialog.vue`（design D6）：promise 式 `confirm(options): Promise<boolean>`，内部复用 `AppDialog`（size=sm、不点遮罩关、无右上关闭钮、Escape=取消），footer 用 `AppButton`（cancel=secondary / confirm=danger?danger:primary）；挂载 `front/app/app.vue`。验证：新增 `useConfirm.test.ts` + `AppConfirmDialog.test.ts` 全绿（附带：vitest.setup.ts 的 useState mock 改为按 key registry，对齐 Nuxt 全局单例语义，否则组件与测试代码各拿各的 state）
- [x] 2b.2 `AIRouterBackupProviders.vue`：移除删除按钮 `:disabled="ctx.isProviderLinked(provider.id)"`；提示行改「挂在线路上，删除将自动从所有线路解绑」。验证：`AIRouterBackupProviders.test.ts` 新增删除入口 describe（2 用例），全文件 8 用例全绿
- [x] 2b.3 `useAIRouterSettings.ts` 的 `deleteBackupProvider`：弃用原生 `confirm()` 改 `useConfirm().confirm({ title, message: 现有文案, confirmText: '删除', danger: true })`；`useAIRouterSettings.test.ts` 改 mock `useConfirm`。验证：`pnpm test:unit useConfirm AppConfirmDialog AIRouterBackupProviders useAIRouterSettings --maxWorkers=2` 全绿（29 用例）

## 3. 测试

- [x] 3.1 后端影响包测试：`cd backend-go && go test ./internal/admin/handler -count=1`（handler 包全量，覆盖周边用例无回归）。期望：全部 PASS
- [x] 3.2 前端受影响文件测试：`cd front && pnpm test:unit useAIRouterSettings --maxWorkers=2`。期望：全部 PASS
- [x] 3.3 冒烟验证（可选，需本地后端跑起）：挂线路的 provider 经管理页删除成功、线路面板同步为空；摘空能力的 AI 调用返回既有「无可用 provider」类错误

## 4. 文档

<!-- doc-impact: flow, api -->
<!-- doc-impact-excuse: database=测试文件存量 AutoMigrate 字样（setupAIAdminTestDB 既有代码），本 change 无 schema 变更 -->

- [x] 4.1 检查 `docs/reference/flow/ai-summary.md`：删除语义无描述、无需修正；变更溯源行归档时补（§12）。验证：`grep -n "删除\|409" docs/reference/flow/ai-summary.md` 无与新行为冲突的描述
- [x] 4.2 检查 `docs/reference/api/`：provider 删除接口无 409/拦截描述、无需更新。验证：`grep -rn "still used" docs/reference/` 无残留

## 5. 验证

| Scenario | 测试文件 |
| --- | --- |
| 删除挂在线路上的 provider | backend-go/internal/admin/handler/ai_handler_test.go |
| 删除未被引用的 provider | backend-go/internal/admin/handler/ai_handler_test.go |
| 线路被摘空后调用该能力 | backend-go/internal/platform/airouter/store_test.go |
| 挂线路 provider 的删除入口与告知 | front/app/features/ai/components/AIRouterBackupProviders.test.ts |
| 删除确认弹窗 | front/app/components/ui/AppConfirmDialog.test.ts |

- [x] 5.1 `cd backend-go && golangci-lint run ./internal/admin/handler/... && go vet ./internal/admin/handler/...`。期望：无输出（0 错误）
- [x] 5.2 `cd backend-go && go build ./...`。期望：编译通过（注：工作区存在他人会话 `dataenrichment` 中间态改动，若其未完成导致 build 红属并发归属，本 change 只需 `go build ./internal/admin/...` 绿）
- [x] 5.3 `cd front && pnpm exec nuxi typecheck`。期望：0 错误
- [x] 5.4 补漏后复验：`cd front && pnpm lint`（0 error，7 存量 warning 均在非本 change 文件）+ `pnpm exec nuxi typecheck`（0 错）+ `pnpm test:unit useConfirm AppConfirmDialog AIRouterBackupProviders useAIRouterSettings --maxWorkers=2`（29 用例全绿）+ `pnpm build`（成功）；已跑 `bash scripts/dev/deploy-frontend.sh` 铺静态托管并重启后端（健康 200）
- [x] 5.5 归档前：`bash scripts/harness/doc-impact.sh verify openspec/changes/fix-provider-delete-route-deadlock`（通过，声明 flow,api）+ `bash scripts/harness/check-standards.sh`（201/0 全绿）
