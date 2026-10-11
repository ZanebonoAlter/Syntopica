## Context

巡检结论（详见 `docs/research/board-matching-leak-patrol/explore-findings.md`）：`loadActiveAuxiliaryLabels`（缓存加载，`Select` 排除 embedding/merge_embedding 两列，17.5k 标签的内存预算所迫）返回的结构体不含向量；L2 命中路径 `addAlias` 对该结构体执行 GORM `Save(label)` 全量保存，nil 指针字段写 NULL → 标签向量静默清零（status 仍 active）。全库 1505/17525 active aux 死向量，11/12 版块构成被截肢。约束：`docs/reference/flow/semantic-board.md` 红线 2（L2 命中只 addAlias、不改 Label、不重算 MergeEmbedding、向量恒定）——本 change 是把该红线从「文档约束」升级为「代码 + 测试不变量」。

## Goals / Non-Goals

**Goals:**
- addAlias 及同型 alias 写路径只动该动的列，向量列永不沾染。
- 存量死向量可一次性回填补齐（幂等、行级容错）。
- 回归测试把「归并后向量不变」钉死在 testcontainer PG 上。

**Non-Goals:**
- 不动 weighted 规则后门、方向校验阈值、版块匹配五级优先（巡检第二/三层根因，后续独立 change）。
- 不做回填进度面板/状态端点（fire-and-forget + 重触发幂等即可满足单用户运维）。
- 不重算 LLM merge_embedding 语义（向量生成规则与 L3 完全一致，不引入新 prompt/新 operation）。

## Decisions

### D1: addAlias 改列级 Update，而非让缓存携带向量或 Save+Omit

- **选定**：`addAlias` 中 `Save(label)` 改为 `Model(...).Where("id=?", label.ID).Update("aliases", label.Aliases)`（updated_at 由 GORM 钩子维护）。
- 理由：问题本质是「全量保存 + 字段子集加载」的组合陷阱，列级更新从根上消除列集合不对称；且语义上 addAlias 本来就只该动 aliases。
- 备选否决：① 缓存 Select 加回向量两列——17.5k×2×2560 维进进程内存（数百 MB），缓存设计初衷就是躲开这个；② `Save` + `Omit("Embedding","MergeEmbedding")`——仍依赖每次记得 Omit，且读者需知道隐藏契约；③ GORM 钩子在 BeforeSave 里回填旧向量——引入隐式魔法查询，更难推理。

### D2: MergeAuxiliaryLabelAlias 同步改为列级更新（防御性加固）

- 现状 `tx.First(&target)` 全列加载后 `tx.Save(&target)`，当前**恰好**安全（结构体带向量）。但这是巧合式安全：任何人日后把加载换成缓存结构体就复发。与 D1 同款改为列级 `Updates(map[string]any{"aliases": ...})`，把「不碰向量列」变成路径无关的硬事实。
- 审计范围：grep `semantic_labels` 上的全部 `Save(`/`Updates(` 写点，逐个确认向量列只在「生成向量的路径」（L3 新建、disable 置 NULL、回填写入）出现；结论记入 tasks 验证节。

### D3: 回填端点挂 `POST /api/auxiliary-labels/backfill-embeddings`，异步 goroutine + 幂等扫描

- 复用既有路由组（`POST /gc`、`POST /merge-alias` 邻居，`board_crud_handler.go` 的 `auxiliary` 组）与 handler 模式；不复用 embedding_queues（那是 topic_tag 向量队列，实体类型不同，混入会污染队列语义）。
- 行为：handler 查询 `status='active' AND label_type='auxiliary' AND (embedding IS NULL OR merge_embedding IS NULL)` 得 pending 数，立即 202 返回 `{pending: N}`；detached goroutine 分批（50 行/批）逐行处理：merge embedding 用 `DefaultAuxiliaryLabelEmbedder(label, merge)`、storage 用 `(label+": "+description 或 label, storage)`（与 L3 同款），列级 `Updates` 只写各自列——**单行只补缺的那列**（merge_embedding 尚在、embedding 缺失的行不重算 merge，反之亦然）。行级失败记 warn 跳过，批间无 sleep（airouter 自带限速/重试）。幂等由「只扫 NULL 列」天然保证，重触发只处理残余。
- 失败语义对齐 spec：全部失败不阻断、不标任务失败（单用户运维，重触发即重试）。

### D4: 回归测试落在 service 层 + testcontainer PG

- `addAlias`/`MergeAuxiliaryLabelAlias` 的向量保持用例走 `testutil.SetupTestDB(t)`（仓库标准，禁 SQLite）——断言「归并前后 embedding/merge_embedding 原样」。GORM Save-vs-Update 的列语义差异只有真 PG 才能钉实。
- 回填用例用注入 fake embedder（`AuxiliaryLabelEmbedder` 已是可注入函数类型）断言：只补 NULL 列、已有列不动、单行失败跳过、重触发幂等。不真调 LLM。

## Risks / Trade-offs

- [回填 1500×~2 次 embedding 调用消耗额度] → 行级只补缺列（多数行两列皆缺，少部分单缺）；airouter 既有计量；用户手动触发一次即止。
- [修 bug 后旧死向量不会自愈] → 回填端点即为此设计；tasks 将「触发回填 + 抽查恢复」列为部署后操作。
- [weighted 挂载不会因向量恢复自动重算] → 向量补齐后版块匹配缓存（auxiliaries/embeddings 随 InvalidateBoardData 失效）在下次打标/重算时自然吃到新向量；存量错挂治理可后续跑 rematch-all，属运维操作不进代码。
- [addAlias 列级更新后 updated_at 语义变化] → Update 单列仍触发 GORM updated_at 自动维护，无语义损失。

## Migration Plan

1. 合并部署（bug 修复 + 端点）→ 止血生效，新别名归并不再清向量。
2. 部署后手动 `POST /api/auxiliary-labels/backfill-embeddings` 一次，回填存量死向量（重触发至 pending=0）。
3. 回滚：代码回滚即可（回填写入的向量是合规 L3 同款向量，无需清理；端点无人调用则无副作用）。
