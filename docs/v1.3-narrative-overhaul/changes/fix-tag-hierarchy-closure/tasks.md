## 1. 断点复现与基线测试

- [x] 1.1 为 Template 保存流程添加后端测试：preview 不保存 config、不创建 rebuild_job、不删除 Node/relations
- [x] 1.2 为现有 rebuild WebSocket 添加测试或最小验证，证明前端监听协议与后端发送协议不一致
- [x] 1.3 为 `PlaceTagInHierarchy` 添加测试：匹配 Sector 但无 parent 时当前返回 `unplaced` 且不创建 Node
- [x] 1.4 为 cold-start 添加测试：无 active Sector 且 unplaced Tags 超阈值时 placement scheduler 当前不会触发 `AutoGenerateSectors`
- [x] 1.5 为 LLM Sector confirm 添加测试：后端当前只返回 message，无法表达逐项成功/失败

## 2. 后端闭环编排

- [x] 2.1 新增 hierarchy orchestration service，定义 category closure status、bootstrap、placement、validation、refresh summary 的核心接口
- [x] 2.2 实现 category closure status：active Sector count、unplaced Tag count、pending count、active rebuild job、blocker counts
- [x] 2.3 将 `AutoGenerateSectors` 接入 orchestration bootstrap step，并按 category 阈值和 active job 状态限流
- [x] 2.4 修改 `TagHierarchyPlacementScheduler`，通过 orchestration flow 处理 cold-start 和 placement retry
- [x] 2.5 修改 `RebuildService`，在 batch placement 前调用必要的 bootstrap/closure preflight

## 3. Template Preview/Apply 重构

- [x] 3.1 拆分或重定义 hierarchy config API，使 preview 请求无副作用
- [x] 3.2 实现 apply/confirm 路径：保存 template、删除该 category abstract Nodes/relations/embeddings、创建 rebuild job、启动执行
- [x] 3.3 确保 apply 只影响发生变化的 category，不对未修改模板重复触发重建
- [x] 3.4 更新 config impact 返回结构，包含 affected Tag count、estimated rebuild duration、violation summary
- [x] 3.5 补充 API 测试覆盖 preview cancel、confirmed apply、active rebuild conflict

## 4. Placement 与 Node 创建闭环

- [x] 4.1 修改 `PlaceTagInHierarchy` 返回结构，增加 blocker reason 和可诊断 action
- [x] 4.2 在无合适 parent Node 时调用 Node 创建决策，而不是静默返回 `unplaced`
- [x] 4.3 将信息增益校验接入 Node 创建路径：候选子 Tag 数、文章 Jaccard、leaf-to-depth ratio
- [x] 4.4 Node 创建成功后生成 embedding、建立 relation、返回 `created_node` action
- [x] 4.5 Node 创建失败时返回结构化 blocker：`insufficient_siblings`、`low_information_gain`、`no_anchor_context` 等
- [x] 4.6 更新或移除旧的第二 Node 创建路径，确保 `PlaceTagInHierarchy` 是唯一 Node 创建入口

## 5. Anchor Signal 策略

- [x] 5.1 决定 anchor signal 落地方式：持久化表/metadata 或删除该承诺
- [x] 5.2 若保留 anchor signal，新增存储结构和 TTL 清理逻辑
- [x] 5.3 修改 `GenerateAnchorSignals`，将 signal 写入可消费位置而不是只返回内存结果
- [x] 5.4 修改 `searchAnchors` / placement context，使当前 Tag 的 anchor signal 成员参与 parent 选择或 Node 创建
- [x] 5.5 不适用：已选择保留并持久化 anchor signal，Phase 7 统计/文案继续指向可消费 signal

## 6. Rebuild Event 协议统一

- [x] 6.1 修改 `RebuildService` progress/complete/failure broadcast，统一发送 `type: "hierarchy_rebuild"`
- [x] 6.2 统一 payload 字段：job_id、category、status、processed、total、failed_count、estimated_remaining_seconds、current_tag、error
- [x] 6.3 确保 rebuild 失败也广播 failed event 并持久化 error_detail
- [x] 6.4 更新 `useWebSocketRebuild` 消费新协议
- [x] 6.5 添加前端或 composable 测试，验证 processing/completed/failed event 更新状态

## 7. Sector 审批与执行结果

- [x] 7.1 定义 Sector diff execution result DTO，覆盖 add、merge、split 的逐项结果
- [x] 7.2 修改 `LLMExecuteSectorDiff`，返回每个操作的 status、affected_tag_count、created_ids、moved_tag_count、error
- [x] 7.3 修改 `confirmRegenerateSectorsHandler`，返回真实 execution result 而非单一 message
- [x] 7.4 更新 `SectorApprovalPanel`，展示后端真实结果并支持部分失败
- [x] 7.5 确认 Sector 创建、删除、合并、拆分后刷新 closure status 和相关 Tag concept assignments

## 8. PendingChange 审批语义修正

- [x] 8.1 梳理现有 `HierarchyPendingChange` change_type 和可执行 payload
- [x] 8.2 修改 approval 逻辑，按 change_type 执行明确动作，不再统一调用 `PlaceTagInHierarchy` 重新猜测
- [x] 8.3 对缺少 payload 或无法执行的 change 返回 failed + reason
- [x] 8.4 更新 `PendingChangePanel` 展示成功/失败数量和失败原因
- [x] 8.5 添加后端测试覆盖单条审批、批量审批、缺失 payload、部分失败

## 9. /tags 用户闭环 UI

- [x] 9.1 在 `/tags` 页面加载 category closure status，并展示 unplaced count、blocker summary、active rebuild 状态
- [x] 9.2 修改 `TemplateSettingsDialog`：保存按钮只请求 preview，确认按钮才调用 apply
- [x] 9.3 Template apply 后使用 WebSocket progress 更新底栏，并在 completed 后刷新 hierarchy/Sector/pending/closure status
- [x] 9.4 Sector create/delete/LLM confirm 后刷新 Sector 列表、hierarchy tree、timeline、pending count、closure status
- [x] 9.5 在 unplaced section 或页面摘要中展示 placement blocker 聚合原因

## 10. 验收测试与文档

- [x] 10.1 更新或创建 acceptance story：进入 `/tags` 查看未闭合状态 → 创建/生成 Sector → 触发放置 → 层级树刷新
- [x] 10.2 更新 acceptance story：Template preview → cancel 无副作用 → confirm 后 rebuild progress 可见 → completed 后刷新
- [x] 10.3 更新 acceptance story：LLM Sector 审批部分失败时 UI 展示真实失败结果
- [x] 10.4 更新 `docs/reference/architecture/`，记录 hierarchy closure 状态机和 orchestration 边界
- [x] 10.5 更新标签生命周期文档，说明 Sector bootstrap、Node 创建、PendingChange 审批和 rebuild 的闭环关系

## 11. 验证

- [x] 11.1 后端 targeted tests：`go test ./internal/domain/tagging ./internal/domain/narrative ./internal/jobs -run "Hierarchy|Rebuild|Sector|Placement|Pending" -v`
- [ ] 11.2 后端全量验证：`golangci-lint run ./...`、`go vet ./...`、`go test ./...`、`go build ./...`
- [ ] 11.3 前端验证：`pnpm lint`、`pnpm exec nuxi typecheck`、`pnpm test:unit`、`pnpm build`
- [ ] 11.4 验收验证：运行 `acceptance-testing` change 中的 `unify-tag-hierarchy` stories 或等价手动验收脚本
- [x] 11.5 更新 `./docs` 知识库并记录已知限制与后续改进项
