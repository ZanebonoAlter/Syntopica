<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->

## Why

数据库健康检查发现两个互相关联的反模式，根源都在辅助标签入库链路（`AttachAuxiliaryLabels`）：

1. **大量 idle in transaction（4–40 秒）**：`AttachAuxiliaryLabels` 在一个事务内循环处理标签，每个 L1 未命中的标签要调 1–2 次**外部 AI embedding API**（merge embedding + storage embedding，网络往返秒级），事务开着等 HTTP 响应。事务持 xmin 阻塞 vacuum，与刚做过的 autovacuum 调优（optimize-pg-storage）直接对抗，是 TOAST bloat 的帮凶。
2. **merge_embedding 反复全量拉取（≈216MB/次）**：L2 去重匹配依赖 `packageAuxLabelCacheGetOrLoad`（17.5k 行 × 2560 维向量，代码注释自证 "216 MB DB transfer"），虽有 10 分钟 TTL 缓存，但失效策略是**一写全清**（`invalidateOnCreate`/`invalidateOnDisable` 置 nil）——批量打标时每个新标签把缓存炸一次，下一个标签又全量拉一遍 + 4500 万次 `ParseFloat`。TTL 从未活到过期。

两者叠加：一篇有 10 个新辅助标签的文章 ≈ 20 次 AI API 调用 + 10 次 216MB 全量拉，全部塞在一个事务里。

## What Changes

- **事务瘦身（AI 调用移出事务）**：`AttachAuxiliaryLabels` 重构为两阶段——
  - 阶段一（无事务）：L1 内存预解析，收集 L1 miss 的标签，**批量**调 embedding API（`airouter.Embed` 原生支持 `[]string`），完成 L2 余弦匹配决策；
  - 阶段二（短事务，毫秒级）：纯写操作——L2 命中的 alias 追加、L3 新建 INSERT、link 创建与 `ref_count` 递增。原有"全有或全无"原子性保留。
- **同模式清理次要事务源**：`semantic_board_upgrade.go` 事务内的 `s.embedder` 调用同样移出事务。
- **缓存失效改增量**：`invalidateOnCreate` 从"全清"改为把新标签向量直接塞进缓存 map；`invalidateOnDisable`（含 merge 路径）改为 `delete(map, id)`；alias 变更维持只清 labels（现状已正确）；TTL 10 分钟保留作外部直改 DB 的兜底漂移窗口。
- **PG 防呆兜底（design 阶段定夺）**：是否给 docker-compose 的 postgres command 追加 `idle_in_transaction_session_timeout`（如 60s），作为"未来再有人把慢调用塞进事务"的硬保险。涉及容器 recreate（连接闪断），单独决策。

不改变：L1/L2/L3 去重语义、`auxiliary_label_dedupe_sim` 阈值、alias 自动积累行为、`MergeAuxiliaryLabelAlias`/`DisableAuxiliaryLabel` 对外行为、任何 API 路由与 DTO。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

（无——L1/L2/L3 三级入库的 spec 级行为（匹配层级、阈值语义、alias 积累、原子性）均不变；本 change 是事务边界与缓存策略的实现层优化，`auxiliary-label` spec 无需 delta。`.openspec.yaml` 设置 `skip_specs: true`。）

## Impact

- **代码**：
  - `backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go`（主战场：`AttachAuxiliaryLabels`、`ResolveAuxiliaryLabel` 拆分、`invalidateOnCreate/Disable` 增量化）
  - `backend-go/internal/tagmanagement/service/board/semantic_board_upgrade.go`（事务内 embedder 移出）
  - `backend-go/internal/tagmanagement/service/core/article_tagger.go`（调用方，预期签名不变）
- **依赖**：`airouter.Embed` 的批量输入能力（已有，无需改动）。
- **可观察收益**：idle in transaction 4–40s → 预期 <1s；批量打标 N 个新标签从 N 次 216MB 全量拉 → 0 次（内存增量维护）；树莓派内存/CPU 峰值显著下降。
- **运维影响**：vacuum 不再被长事务卡 xmin；若采纳 PG 防呆项需 recreate postgres 容器（连接闪断一次）。
- **不涉及**：前端、API 契约、数据迁移（无 schema 变更）。
