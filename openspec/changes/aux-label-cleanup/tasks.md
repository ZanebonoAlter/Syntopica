## 1. Phase A: ref_count 修正

- [ ] 1.1 在 `AuxiliaryLabelService` 新增 `RecountRefs(ctx, ids []uint) error` 方法
- [ ] 1.2 在 `CleanupOrphanedTags`（article_tagger.go）中，删除 topic_tag 前收集 aux label IDs，删除后调用 `RecountRefs` 重算
- [ ] 1.3 在 `HardMergeTags`（hard_merge.go）中，删除 source tag 前收集 aux label IDs，删除后调用 `RecountRefs` 重算
- [ ] 1.4 编写 `RecountRefs` 和 `CleanupOrphanedTags` 修正逻辑的单元测试
- [ ] 1.5 编写一次性存量校准脚本 `scripts/recalculate_aux_refcounts.go`

## 2. Phase C: GC 服务

- [ ] 2.1 在 `AuxiliaryLabelService` 新增 `GC(ctx, req AuxLabelGCRequest) (*AuxLabelGCResult, error)` 方法
- [ ] 2.2 在 `semanticBoardHandler` 新增 `POST /api/auxiliary-labels/gc` 端点
- [ ] 2.3 编写 GC 逻辑的单元测试

## 3. Phase C: 定时任务

- [ ] 3.1 新建 `backend-go/internal/jobs/aux_label_cleanup.go`，实现 `AuxLabelCleanupScheduler`（仿照 LogCleanupScheduler）
- [ ] 3.2 在 `runtimeinfo/schedulers.go` 声明 `AuxLabelCleanupSchedulerInterface`
- [ ] 3.3 在 `handler.go` schedulerDescriptors 中注册 `aux_label_cleanup`
- [ ] 3.4 在 `runtime.go` 中初始化并启动 `AuxLabelCleanupScheduler`，注册到 graceful shutdown
- [ ] 3.5 编写调度器单元测试

## 4. 前端集成

- [ ] 4.1 在 `front/app/utils/schedulerMeta.ts` 添加 `aux_label_cleanup` 的 displayName/icon/color
- [ ] 4.2 在 `front/app/api/auxiliaryLabels.ts` 添加 `triggerGc` API 方法
- [ ] 4.3 前端 lint + typecheck 验证

## 5. 存量数据校准

- [ ] 5.1 在数据库手动执行存量 ref_count 校准脚本，验证修正结果

## 6. 验证

- [ ] 6.1 运行 `golangci-lint run ./...` 和 `go vet ./...`
- [ ] 6.2 运行 `go test ./internal/domain/tagging/... ./internal/jobs/...`
- [ ] 6.3 运行 `go build ./...`
- [ ] 6.4 前端 `pnpm lint` + `pnpm exec nuxi typecheck` + `pnpm build`
