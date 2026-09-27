# test-cases.md — fix-alias-save-vector-wipe

测试单元 = 「辅助标签 alias 变更与向量回填」用户故事。spec delta 全部为 ADDED（无 MODIFIED/REMOVED），⓪ 继承与调整不适用：旧测试不断言旧契约，零继承处置。前端无涉（ui-impact: none），opencli 端到端豁免理由：纯后端 change，无用户可见交互。

## 主链路表

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 建行（带双向量）→ 触发 L2 归并新别名 | L2 归并新别名后向量保持不变 | aliases 追加且 embedding/merge_embedding 逐字节原样 | service / PG testcontainer | `auxiliary_label_service_test.go` TestAddAliasPreservesVectors |
| 2 | 双向量目标行 → MergeAuxiliaryLabelAlias(source,target) | 手动 alias 合并后目标标签向量保持不变 | 目标行向量不变仅 aliases 变；source 侧按既有语义 disabled+NULL | service / PG | TestMergeAuxiliaryAliasPreservesTargetVectors |
| 3 | 同一标签连续多次归并不同别名变体 | 归并路径不产生 active 且向量缺失的行 | 终态 status=active ⇒ 两向量均非 NULL | service / PG | 步1 用例参数化（3 个别名变体串行归并）|
| 4 | 预置 active+双 NULL 行 → BackfillMissingEmbeddings | 触发回填补齐死向量行 | 两列按 L3 同款生成并写入（fake embedder 断言入参：merge←label、storage←"label: description"）| service / fake embedder | TestBackfillMissingEmbeddingsFillsNulls |
| 5 | 预置一 NULL 一非 NULL 行 → 回填 | 已有向量的行不受回填影响 | 只补 NULL 列，非 NULL 列值不变且不为该列发起生成 | service / fake embedder | TestBackfillSkipsExistingColumns |
| 6 | 混入一行 embedder 必失败 → 回填 | 单行生成失败不阻塞批处理 | 失败行保持 NULL、其余行补齐、无 error 上抛 | service / fake embedder | TestBackfillRowFailureSkips |
| 7 | 全量健康库 → 回填；POST /backfill-embeddings | 无死向量时回填为空操作 | embedder 零调用；HTTP 202 `{"pending":0}` | service + handler / httptest | TestBackfillNoopHealthy + TestBackfillEndpoint202 |
| 8 | 死向量行 N 条 → POST 端点 | （端点行为，design D3） | 202 `{"pending":N}`，goroutine 异步处理不阻塞响应 | handler / httptest | TestBackfillEndpointPendingCount |

## 变体走查（五组固定清单）

| 组 | 变体 | 答案 / 处置 |
| --- | --- | --- |
| 输入 | 空串/纯空白/纯分隔符/单token/大小写/特殊字符 | 划除：addAlias 与回填输入均来自 ResolveAuxiliaryLabel 前置校验后的库行 label，无自由文本入口（空 label 在 Resolve 入口已拒） |
| 输入 | 超长 label | 有答案：L3 生成规则无截断，embedding 输入超长由 airouter 统一处理；回填不引入新截断逻辑，行为与 L3 一致（用例不单列，记答案留痕） |
| 前置 | 空集（零死向量） | 场景 7 覆盖 |
| 前置 | 重复（重触发回填） | 场景 6+幂等断言：二次触发 pending 只含残余行 |
| 前置 | 越界引用 | 划除：回填按 NULL 列扫描，无引用解析 |
| 时间窗口 | 边界/空窗/跨窗/归一化 | 划除：无时间窗口概念 |
| 幂等 | 重复执行 | 回填天然幂等（只扫 NULL），场景 4 重跑断言 |
| 幂等 | 部分失败重试 | 场景 6 后续触发 = 仅失败行重试，断言残余 pending=1 |
| 幂等 | 并发（并发归并同标签） | 有答案：两个并发 addAlias 读-改-写 aliases 存在丢一个 alias 的竞态——与旧 Save 行为一致（非本 change 引入、不恶化），向量列安全（列级 Update 不触碰）；不在本 change 修，留痕 |
| 可用性(UI必检前三) | 误输入/空态/错误态 | 划除：无 UI；端点错误态=库不可用 500，遵循框架默认 |

## 效果核对（真库量化）

- 触发原因：addAlias 清向量 bug 导致 1505 个 active aux 死向量（巡检量化）。
- 方法：部署后 POST 回填 → SQL 计数对比。
- 量化：`SELECT count(*) FROM semantic_labels WHERE label_type='auxiliary' AND status='active' AND embedding IS NULL` 从 ~1505 → 0；抽查 id=113731（高市早苗）向量非 NULL。
- 结论判据：计数归零 + 日本新闻版块构成 5 个死向量标签全部恢复。
