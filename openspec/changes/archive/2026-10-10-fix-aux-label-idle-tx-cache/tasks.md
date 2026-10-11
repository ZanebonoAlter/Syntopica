# Tasks — fix-aux-label-idle-tx-cache

## 1. 缓存失效增量化（design D2）

- [x] 1.1 `invalidateOnCreate` 改增量：L3 路径仅 `activeLabels` append 轻量副本（对齐现有 Select 列集合）；`mergeEmbeddings` map 不得原位写（2026-10-07 修订，FB-1：无锁读者 concurrent map fatal，新向量由 TTL 重拉自然可见），verify: 单测断言 create 后 map 大小不变、`mergeEmbeddingsAt` 未重置、activeLabels 线性 +1
- [x] 1.2 `invalidateOnDisable` 与 `MergeAuxiliaryLabelAlias` 的 source 禁用路径改 `activeLabels` fresh-slice 过滤（不原位 filter），`mergeEmbeddings` 不动（同 FB-1 约束），verify: 单测断言 disable 后 activeLabels 移除、无全量重拉（mergeEmbeddingsAt 不变）
- [x] 1.3 alias 变更路径维持只清 `activeLabels`（回归确认现状不回退），verify: 单测断言 alias 追加后 mergeEmbeddings 保留
- [x] 1.4 TTL 过期兜底路径回归：过期后首次 L2 匹配重新全量拉一次，verify: 单测注入时钟/时间戳断言过期边界（9:59 命中 / 10:01 重拉）

## 2. 两阶段拆分：AI 调用移出事务（design D1）

- [x] 2.1 批量 embedder 注入点：新增批量调用单元（基于 `airouter.Embed` Input []string），保持可 mock，verify: 单测 mock 断言一批 N 标签只发生 2 次调用（merge 批 + storage 批）
- [x] 2.2 `AttachAuxiliaryLabels` 重构两阶段——阶段一（无事务）L1 预解析 + 批量 embedding + L2 匹配决策；阶段二（短事务）纯写：alias UPDATE / INSERT / link OnConflict Create / ref_count 递增，verify: 单测断言阶段二事务闭包内零 embedder 调用
- [x] 2.3 `ResolveAuxiliaryLabel` 复用拆分后的逻辑单元（单标签入口行为不变，供既有调用方），verify: 既有 auxlabel 单测全绿（L1 命中 alias / L2 去重 / L3 新建三场景）
- [x] 2.4 `semantic_board_upgrade.go` 事务内 `s.embedder` 调用移出事务（事务前预计算，事务内纯写），verify: board 包单测全绿 + grep 确认事务闭包内无 embedder 调用

## 3. 回归与效果验证（design D5）

- [x] 3.1 testcontainer PG：mock embedder 注入 2s 延迟跑 attach，轮询 `pg_stat_activity` 断言全程无 `state='idle in transaction'` 会话，verify: `go test ./internal/tagmanagement/service/auxlabel -run IdleTx -count=1` 绿
- [x] 3.2 testcontainer PG：阶段二任一写失败（注入 INSERT 错误）→ 全部回滚，无部分挂载/部分新建残留，verify: `go test ./internal/tagmanagement/service/auxlabel -run Atomicity -count=1` 绿
- [x] 3.3 testcontainer PG：并发双 attach 同名新标签 → slug 唯一索引冲突冒泡、无重复行（design D3），verify: `go test ./internal/tagmanagement/service/auxlabel -run SlugConflict -count=1` 绿

## 4. PG 防呆参数（design D4）

- [x] 4.1 `docker-compose.pg.yml` postgres command 追加 `idle_in_transaction_session_timeout=60s`，verify: recreate 后 `SHOW idle_in_transaction_session_timeout` 返回 60s（完工汇报提示用户 recreate 与连接闪断；compose config 已验证参数生效，运行容器当前仍 0 待 recreate）

## 5. 测试

- [x] 5.1 test-cases.md 主链路故事全绿（含变体走查与白盒附加节、⓪ 继承与调整表逐行处置），verify: `bash scripts/harness/change-scope.sh` 判定的影响包测试全绿（tagmanagement 全树 testcontainer 跑绿，其余域 -short 门禁全绿）
- [x] 5.2 效果核对（test-cases.md 效果节）真库量化完成：改造前基线（idle in transaction 最长时长 / 全量拉次数）vs 改造后数据落档，verify: test-cases.md 效果核对节含触发/方法/量化结果/结论四要素（生产复测转 7.3 人工）

## 6. 文档

<!-- doc-impact: flow, standard, deployment -->
<!-- doc-impact-excuse: database=本 change 无任何 schema 变更：auxiliary_label_service.go 命中的 ALTER TABLE 字样系既有启动期向量定维 helper（EnsureSemanticLabelVectorDimension，非本 change 引入），启发式按文件内容而非 diff 判定所致误报 -->

- [x] 6.1 `docs/reference/flow/semantic-board.md` 辅助标签入库节补两阶段事务边界与增量缓存说明 + 溯源行，verify: `doc-impact.sh verify` 通过（变更溯源表行按 §12 归档时补链接，节内两段 callout 已携带 change 归属）
- [x] 6.2 `docs/reference/standard/backend/` 若涉及事务模式约定（慢 I/O 移出事务红线）补充或确认已有覆盖，verify: check-standards.sh 对应段通过（code-style.md Anti-Patterns 新增事务内慢 I/O 硬禁条目，230/230 绿）

## 7. 验证

- [x] 7.1 后端影响包全绿：`cd backend-go && golangci-lint run ./internal/tagmanagement/... && go vet ./internal/tagmanagement/... && go test ./internal/tagmanagement/service/auxlabel ./internal/tagmanagement/service/board ./internal/tagmanagement/service/core` — 期望 0 fail（以 change-scope.sh 判定为准，域外红登记台账）——实测全树 tagmanagement 九包 testcontainer 全绿 + lint/vet 0 issue
- [x] 7.2 构建通过：`cd backend-go && go build ./...` — 期望 0 error
- [ ] 7.3 部署后手验：重启后端 + recreate postgres 后跑一次真实打标（或回填触发），`psql` 查 `pg_stat_activity` 无长时间 `idle in transaction`；观察 `SELECT id, merge_embedding` 仅冷启动出现一次 — 期望两项均命中（**人工**，结果记入 test-cases.md 效果节）
