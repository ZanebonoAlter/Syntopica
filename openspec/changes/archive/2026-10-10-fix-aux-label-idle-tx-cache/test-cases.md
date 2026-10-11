# Test Cases — fix-aux-label-idle-tx-cache

> 复杂档白盒附加见文末。本 change 为 skip_specs（实现层优化，L1/L2/L3 语义不变），故事锚点取 design.md 决策（D1–D5）。

## 主链路故事：批量打新标签 → 无 idle in transaction、无重复全量拉、语义不变

| 步 | 动作 | 锚点 | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | attach 3 个全新标签 | D1 | 批量 embedder 恰好 2 次调用（merge 批 1 + storage 批 1），非逐标签 6 次 | 函数单测（mock 计数） | `auxiliary_label_service_test.go` |
| 2 | 同上 | D1 | 事务闭包内零 embedder 调用；Begin..Commit 区间只含 DML | 函数单测（事务 hook/mock） | 同上 |
| 3 | attach + embedder 延迟 2s | D5 | `pg_stat_activity` 全程无 `state='idle in transaction'` | testcontainer PG | `attach_idle_tx_test.go` |
| 4 | 连续 attach 多批新标签 | D2 | 缓存 `mergeEmbeddingsAt` 不被重置、map 大小线性增长、`SELECT id, merge_embedding` 仅冷启动 1 次 | 函数单测（时间戳断言） | 缓存单测 |
| 5 | `DisableAuxiliaryLabel` | D2 | map 条目删除 + `activeLabels` 过滤，无全量重拉 | 函数单测 | 同上 |
| 6 | `MergeAuxiliaryLabelAlias`（source 禁用） | D2 | source 向量删除、target 保留 | 函数单测 | 同上 |
| 7 | 阶段二注入写失败 | D1 原子性 | 全部回滚：无 link、无新建行、alias 未追加 | testcontainer PG | `attach_atomicity_test.go` |
| 8 | 并发双 attach 同名标签 | D3 | slug 唯一索引冲突冒泡、表中无重复行 | testcontainer PG | `attach_slug_conflict_test.go` |
| 9 | 全 L1 命中的 attach（既有标签） | D1 | 零 embedder 调用、缓存零重拉、行为与现状一致 | 函数单测 | 回归 |
| 10 | `semantic_board_upgrade` 建议确认执行 | D1 | 事务内无 embedder 调用，板向量事务前算好 | 函数单测 | board 包测试 |

## 变体走查（五组固定清单）

- **输入**：空 `labels` 切片（早退 nil ✓ 现状保留，单测回归）；纯空白 label（校验拒绝 ✓ 现状）；slug 归一化为空的 label（拒绝 ✓ 现状）；大小写/全半角变体走 L1 normalizeKey（现状逻辑，回归）
- **前置**：空标签池冷启动（首次全量拉 1 次后缓存）；单元素批；批内重复 label（attach 前已 dedupe + OnConflict DoNothing 兜底，回归）
- **时间窗口**：TTL 边界 9:59 命中缓存 / 10:01 过期重拉（任务 1.4）；`mergeEmbeddingsAt` 与写操作交错（锁内更新）
- **幂等**：重复 attach 同一标签（link OnConflict DoNothing、ref_count 仅新 link 递增 ✓ 现状回归）；阶段一失败重试（无副作用可安全重试）；阶段二失败重试（事务回滚后全批重做）
- **可用性**：~~误输入反馈/空态/错误态/加载态/超长文本/重复提交~~ 不适用（无 UI 变更，划除留痕）

## 效果核对（真库量化，任务 5.2）

- **触发原因**：数据库健康检查发现 idle in transaction 4–40s 与 216MB 级 `merge_embedding` 反复全量拉（proposal Why，生产实测基线）
- **方法**：①改造后 testcontainer PG + 2s 延迟批量 embedder 跑 3 标签 attach，100ms 间隔轮询 `pg_stat_activity`（旧实现病灶窗口 = 整段 attach 时长 ≥4s，必被轮询命中）；②缓存侧断言连续 3 次 L3 后 `mergeEmbeddingsAt` 不重置、`mergeEmbeddings` map 大小不变（2026-10-07 FB-1 修订：map 不原位写，无锁读者约束）、`activeLabels` 线性 +3，冷启动重拉仅 1 次（TTL 边界测试）
- **量化结果**：
  - 改造前（生产基线）：idle in transaction 最长 **40s**；全量拉 = 每个新建标签 1 次（一篇 10 新标签文章 ≈ 10 次 × 216MB 文本传输 + ~4500 万次 ParseFloat，TTL 10 分钟从未活到过期）
  - 改造后（testcontainer 实测，`TestAttachIdleTxNoneDuringSlowEmbedder` / `TestAuxLabelCacheCreateIsIncremental` / `TestAuxLabelCacheTTLExpiryReloads`）：**0 次** >500ms idle in transaction 命中（BeginProbe 另证零 embedder 在 BEGIN 后调用）；连续 3 次 L3 后 **0 次**创建触发的全量重拉，全量拉仅冷启动 + TTL 到期后下一次 L2 匹配各 **1 次**（与新建个数无关）；批量 embedder 恰 2 次调用（旧串行 6 次）
  - 附带：阶段二原子性（触发器注入写失败 → 整体回滚，`TestAttachPhaseTwoWriteFailureRollsBackAll`）与并发 slug 冲突冒泡无重复行（表锁+栅栏确定性构造，`TestAttachConcurrentSameLabelSlugConflictBubbles`）均命中
- **结论**：事务挂起归零（4–40s → 0，亚秒纯写事务不再持 xmin）、创建触发的全量拉 N→1（新建向量 L2 可见性延迟 ≤TTL，FB-1 修订语义）；生产复测项转任务 7.3 人工（重启后端 + recreate postgres 后观察）

## 白盒附加（复杂档）

**分支表**：

| 分支 | 覆盖用例 |
| --- | --- |
| L1 命中（slug/alias/normalizeKey 三路） | 步 9 + 回归 |
| L1 miss → L2 命中（sim ≥ 阈值，取 ref_count 最大） | 步 1、4 |
| L1 miss → L2 miss → L3 新建 | 步 1、7 |
| L2 未过阈值 → continue（无 best） | 阈值边界用例 |
| 批量 embedder 部分失败（供应商限流） | 步 1 扩展：错误冒泡、无半成品写入 |
| 阶段二 INSERT slug 唯一冲突 | 步 8 |
| 阶段二 alias UPDATE 失败 | 步 7 |
| disable 后缓存删除 | 步 5 |
| TTL 过期重拉 | 任务 1.4 |
| ~~pgvector 维度变更/HNSW 路径~~ | Non-Goal，划除 |

**边界值**：`auxiliary_label_dedupe_sim` 阈值 ε 邻域（恰等于/略小于）；TTL 9:59/10:01；空批次（步 9 的零调用断言）；批大小上限切片（32，实现细节若引入则补断言）

## ⓪ 继承与调整（旧测试资产处置）

skip_specs 无 spec delta；旧测试断言的**行为**（L1/L2/L3、alias 积累、ref_count、禁用清理）全部保留，实际处置（逐行核对完毕，全量 tagmanagement 树 testcontainer 跑绿）：

| 旧资产 | 处置 | 动作 |
| --- | --- | --- |
| `auxiliary_label_service_test.go` 既有用例 | 保留 | 直接绿（L1 alias/normalizeKey、L2 阈值/高 ref 优先、L3 双向量、attach 幂等、disable/merge 向量清理全过）；embedder mock 零适配——自定义单标签 embedder 由构造器包装为批量序列执行，逐输入命中不变 |
| `auxiliary_label_backfill_test.go`（同文件 backfill 用例） | 保留 | backfill 路径未动，直接绿 |
| `composite_label_service_test.go` / `composite_label_handler_test.go` | 保留 | Plan/apply 拆分后 `CreateCompositeLabel` 语义等价（写路径多包一层短事务，原子性只增不减），直接绿 |
| `article_tagger` 相关测试 | 保留 | 调用方签名不变，直接绿（core 包全量过） |
| `semantic_board_upgrade_test.go` / `semantic_board_compose_test.go` | 保留 | ConfirmSuggestion 阶段前置后错误语义不变（含 embedder 失败建议保持 pending、失效标签软过滤），直接绿 |

新增白盒资产：`aux_label_cache_test.go`（D2 增量缓存 + TTL 边界 9:59/10:01）、`attach_two_phase_test.go`（批量计数 2 次/BeginProbe 事务外断言/批内去重/失败冒泡/chunkStrings 边界）、`attach_idle_tx_test.go`、`attach_atomicity_test.go`、`attach_slug_conflict_test.go`、board `confirm_tx_probe_test.go`（create_new + compose 双方向事务内零 embedder）。
