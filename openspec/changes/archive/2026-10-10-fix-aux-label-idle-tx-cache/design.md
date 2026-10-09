# Design — fix-aux-label-idle-tx-cache

## Context

现状链路（动机见 proposal.md - Why）：`article_tagger` → `AttachAuxiliaryLabels`（单事务）→ 循环 `ResolveAuxiliaryLabel`：L1 内存匹配 → L2 miss 时调外部 embedding API + 全量拉 `merge_embedding` 缓存 → L3 新建再调一次 API，且 `invalidateOnCreate` 全清缓存使下一个标签再全量拉。

约束事实：

- 打标主写入路径由单 worker 串行驱动（`embedding_queue.go` 单 goroutine），并发写标签的场景罕见；
- `semantic_labels.slug` 有唯一索引（`idx_semantic_labels_slug`），`UniqueSemanticLabelSlug` 是查后插入（TOCTOU 窗口现状已存在）；
- `airouter.Embed` 的 `EmbeddingRequest.Input` 为 `[]string`，批量能力现成；
- `packageAuxLabelCache` 同时缓存 `activeLabels`（L1 用，Select 列集合刻意排除向量列）与 `mergeEmbeddings`（L2 用，`map[uint][]float64`）；
- 除 service 方法外，`board_crud_handler.go` / `composite_label_service.go` 存在直改 `status='disabled'` 的 Updates 路径，**不经过** invalidate（现状即如此，靠 TTL 兜底）。

## Goals / Non-Goals

**Goals**

- 消除打标链路的 idle in transaction（AI 调用全部移出事务，事务只剩毫秒级纯写）；
- 消除批量打标场景的重复全量拉（缓存失效改增量维护）；
- 保持 L1/L2/L3 去重语义、`AttachAuxiliaryLabels` 整体原子性（全有或全无）与对外 API 不变。

**Non-Goals**

- 不改 pgvector 下推/HNSW（2560 维无法索引，Go 侧余弦是既定架构决策）；
- 不改 embedding 维度或降维（MATRYOSHKA 截断属另一量级的改动）；
- 不处理 postgres 端口绑定问题（用户决策另行处理，与本 change 无关）；
- 不统一 handler 直改路径的缓存失效（保持 TTL 兜底，行为不恶化即可）。

## Decisions

### D1 两阶段拆分：AI 调用移出事务

`AttachAuxiliaryLabels` 重构为：

```
阶段一（无事务，慢 I/O 区）
  1. loadActiveAuxiliaryLabels（缓存）逐标签 L1 匹配 → 命中者直接得 label ID
  2. 收集 L1 miss 标签 → 一次批量调 embedder(label 列表, mode=Merge)
  3. 对 merge 向量做 L2 余弦匹配（读增量缓存）
  4. L2 未命中的 → 一次批量调 embedder(label+description 列表, mode=Storage)
     （merge 向量在步骤 2 已得，直接复用）
  产出：每个标签的决策 = {命中已有 ID | 新建(label/slug/两个向量)}
阶段二（短事务，毫秒级纯写）
  5. L2 命中 → addAlias 的 UPDATE aliases（复用现有列级更新）
  6. 新建 → INSERT（含双向量）
  7. link OnConflict DoNothing Create + ref_count 递增（现有逻辑原样搬入）
  8. 增量更新缓存（见 D2）
```

- 原子性：阶段二仍为单事务，任一写失败全部回滚——与现状"全有或全无"一致。
- 替代方案否决：①每标签独立短事务——破坏整体原子性且增加事务数；②异步队列化标签创建——引入最终一致窗口，L1/L2 语义变化，超出本 change 范围。
- `ResolveAuxiliaryLabel` 保留为单标签入口（供既有调用方），内部复用阶段一的逻辑单元；`AttachAuxiliaryLabels` 走批量路径。批量 embedder 以新接口方法或独立函数注入，测试仍可 mock。
- 同模式清理 `semantic_board_upgrade.go`：事务前预计算 composite/board embedding，事务内纯写（该路径频率低，但模式相同，顺手治理）。

### D2 缓存失效增量化（2026-10-07 修订，FB-1）

> **修订原因**：初版方案的「`mergeEmbeddings` 原位写/删」引发生产 `fatal error: concurrent map read and map write` 两次宕机（2026-10-07，崩溃点 `sqlMergeMatcher`——读者经 GetOrLoad 拿到 map 引用后**无锁遍历读**，「锁内写 + 锁外读同一 map」在 Go 不可恢复）。修订后增量语义只保留 slice 半边；map 半边回退 TTL 全量重拉。

| 触发 | 现状 | 改为 |
| --- | --- | --- |
| L3 新建成功 | `mergeEmbeddings=nil` + `activeLabels=nil` 全清 | **仅** `activeLabels` append 轻量副本（对齐 Select 列集合）；`mergeEmbeddings` **不得原位写**（无锁读者约束），新向量经 TTL 到期重拉自然可见 |
| `DisableAuxiliaryLabel` | 全清 | **仅** `activeLabels` 换新切片过滤（fresh slice，不原位 filter）；`mergeEmbeddings` 不动，陈旧向量随 TTL 重拉消失 |
| `MergeAuxiliaryLabelAlias`（source 禁用） | 全清 | 同上：activeLists 换新切片（drop source + 刷新 target aliases） |
| alias 变更 | 只清 `activeLabels` ✓ | 不变 |
| handler 直改 disabled（不经 service） | 无失效，TTL 兜底 | 不变（不恶化） |

- **并发纪律（红线级）**：`mergeEmbeddings` map 存在无锁读者（`sqlMergeMatcher` 持引用遍历），任何路径不得原位写/删；读者安全的增量只有两种——slice append 超读者快照长度、slice 整头替换为 fresh 数组（旧读者继续持有旧底层数组）。
- TTL 10 分钟保留：兜底外部直改 DB、handler 直改路径与**新标签向量可见性**（≤TTL 延迟，正确性无损——L2 去重在延迟窗口内退化为 L3 新建+后续手动合并治理，罕见场景）。
- 替代方案否决：① 版本号/失效队列——单进程单实例场景过度设计；② 写时复制（整 map 拷贝替换引用）——17.5k 行向量每次 L3 拷贝成本待评估，若业务确需即时可见再启用；③ 原位写——已被生产事故否决，永不再回。

### D3 竞态窗口：slug 唯一索引兜底，行为与现状对齐

阶段一决策与阶段二写入之间存在理论竞态（并发创建相同标签 → 双双走 L3 → INSERT 撞唯一索引）。现状事务内同样是查后插入，TOCTOU 窗口早已存在；且主写入路径单 worker 串行。决策：INSERT 冲突错误照常冒泡（打标 job 记 warn 跳过，`article_tagger` 现有错误处理已如此），不加 ON CONFLICT 重试复杂度。

### D4 PG 防呆：`idle_in_transaction_session_timeout=60s`

`docker-compose.pg.yml` 的 postgres command 追加该参数。事务瘦身后最长正常事务应为亚秒级；唯一可能的长事务是 `HardMergeTags` 大合并（纯 SQL 行迁移，秒级）——60s 余量充足。误杀表现是事务被 PG 掐断报错，可观察可回滚参数。**部署时需 recreate 容器（连接闪断一次），tasks 中独立任务并在完工汇报中提示。**

### D5 效果验证方式（testcontainer 真库）

核心回归断言用 testcontainer PG：mock embedder 注入 2s 延迟，打标期间轮询 `pg_stat_activity` 断言不存在 `state='idle in transaction'` 的会话；全量拉次数用缓存时间戳断言（批量 N 个新标签后 `mergeEmbeddingsAt` 未被重置、map 大小线性增长）。

## Risks / Trade-offs

- [阶段一/二之间进程崩溃 → 标签没挂上但 embedding 花费已发生] → 可接受：无状态损失，下次打标重做（现状事务内 embedding 失败同样浪费调用）。
- [增量缓存与 DB 漂移（handler 直改路径）] → TTL 10 分钟兜底，窗口与现状完全一致。
- [`idle_in_transaction_session_timeout` 误杀未知长事务] → 60s 宽裕值；若出现误杀，日志可见 `terminating connection due to idle-in-transaction timeout`，调大或移除参数即可（compose 一行）。
- [批量 embedding 单请求变大（一批几十个标签）] → airouter 已有批量语义与供应商限流处理；单批上限如需要可在实现时加 32 切片，属实现细节。

## Migration Plan

1. 部署：重启后端（Go 代码无 schema 变更，无数据迁移）；D4 需 `docker compose -f docker-compose.pg.yml up -d`（recreate，连接闪断）。
2. 回滚：回退代码重启后端即可；D4 回滚为移除 command 参数再 recreate。
3. 旧数据：无降级问题（缓存是进程内存，重启即重建）。

## Open Questions

（无——D4 的 60s 取值可在观察期后调整，不阻塞任务拆解。）
