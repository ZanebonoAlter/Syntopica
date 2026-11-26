<!-- complexity: medium -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board, topic-graph -->

# Proposal: 标签族 spec 合并（23 → 3）

## Why

`semantic-board` 域有 52 个 spec、12142 行（占全仓 spec 体量 44%），其中**标签相关的 23 个 spec 共 2771 行讲的是同一套标签体系**，平均每个 120 行。改一次标签匹配要打开 21 个文件，而这 21 个文件里没有任何一个能告诉你「现在总的规矩是什么」。

这是 capability 划分方式的副作用：每个 change 新建一个 capability，老 capability 不断 MODIFIED 变长，**只增不减**。域索引（`add-spec-domain-index`）解决了「从 177 个里找到相关的 12 个」，但没解决「12 个还是读不完」。

实测压缩空间：23 个 spec 的 Requirement 标题一共 148 条，全列出来约 194 行——**2771 行的正文，目录只要 194 行**。这说明碎片是文件边界造成的，不是内容真的需要 2771 行。

## What Changes

按业务分层把 23 个 capability 收敛到 3 个族级 capability（其中 `auxiliary-label` 就地扩容、保留原名；详见 design.md D1–D2、D8）：

| 新 capability | 层 | 吸收的旧 capability | 行数 |
|---|---|---|---:|
| `tagging` | L0 原子概念 | tagging-domain, aggregate-tagging, semantic-label-model, composite-label, article-tag-watch-toggle, tag-queue-scheduling, tag-queue-progress-chip | 1042 |
| `auxiliary-label` | L0 原子概念 | auxiliary-label, aux-label-gc, auxiliary-label-description-repair, tag-hierarchy-quality, tag-merge-suggestions, tag-merge-ui, tag-match-identity-protection | 678 |
| `tag-matching` | L2 加工能力 | tag-to-board-matching, tag-embedding-management, board-affinity-metadata, board-match-auto-trigger, board-match-cache, match-score-visualization, match-detail-ondemand, source-board-hit-rate, cross-board-relation-discovery | 1051 |

合并规则：

1. **Requirement 原样搬运，不改语义**——本次只是重组文件边界，不是改契约。任何措辞调整必须单独提 change。
2. **每条 Requirement 带来源注释** `<!-- from: <旧capability> (<归档change>) -->`，合并后仍能查「这条哪来的」。
3. **按功能逻辑重排**，不按来源文件堆叠：同族的 Requirement 按「输入 → 处理 → 输出 → 边界」顺序组织。
4. **Scenario 跟随其 Requirement 整体搬运**，不重新分配。

## Impact

- **spec 数量**：178 → 158（退役 22 个 + 新建 `tagging` / `tag-matching` 2 个；`auxiliary-label` 就地扩容保留）
- **历史追溯**：指向 22 个被删 capability 的 delta 共 47 个文件、涉及 36 个 archived change。其中 15 个 change 仍能经其它 capability 命中本域、进 `domain-history.sh` 第 6 段；**21 个只碰过被删 capability 的 change 会彻底不可见**（已知损失，根治需别名表，见 design D6）。来源注释是读 spec 时的补偿手段
- **门禁**：`openspec validate --specs` 必须 158 全过；归档需 `.openspec.yaml` 的 `retire_capabilities: true`（否则整次中止，见 design D7）；新 main spec 需补 domain marker（取舍见 design D9）
- **代码**：零改动。本次不动 `backend-go/` 与 `front/`
- **文档交叉引用**：12 处 `docs/reference/` 与 main spec 里引用旧 capability 路径的指针需改指新族（tasks 7.4）
- **用户可见行为**：无
- **回退**：`git revert` 单个 commit；旧 spec 目录从 git 历史可完整恢复

## 不做的事

- 不合并 `semantic-board` 其余 29 个 spec（版块诊断、泳道画布、日报）——它们先保持原样，等本次验证流程后再逐族推进
- 不拆分巨型族（`tagging` 1042 行 / `tag-matching` 1051 行先接受，超过 design D5 阈值再拆）
- 不修正 spec 内容本身的问题（重复、过时条款），发现即记录，另开 change
