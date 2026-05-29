## Context

当前板块升级建议系统位于 `backend-go/internal/domain/tagging/semantic_board_upgrade.go`。`clusterCandidates()` 方法执行两阶段聚类：
- Phase A：将候选与已有板块匹配（cosine distance ≤ threshold → 归入该板块簇）
- Phase B：未被 Phase A 匹配的候选走纯自聚类

Phase A 的副作用是：同一语义空间的候选可能被已有板块"截走"，限制新板块发现。LLM 同时做 create_new / merge_into_existing / skip 三选一，merge 判断质量不稳定。

前端 `UpgradeSuggestionPanel.vue` 展示建议卡片，用户只能"确认执行"，无法修改决策或选择合并目标。

## Goals / Non-Goals

**Goals:**
- 将聚类与已有板块解耦：所有候选统一走纯自聚类（Phase B only）
- Phase A 降级为纯元数据计算（board_affinities），不影响聚类
- LLM 只做 create_new / skip 二选一
- 前端展示 board_affinity 参考信息，提供"合并到..."下拉按钮让用户手动触发 merge
- 后端 ConfirmSuggestion API 的 merge_into_existing 路径保持不变

**Non-Goals:**
- 不改变 ConfirmSuggestion API 的接口定义
- 不改变候选收集逻辑（collectCandidates）
- 不改变 co-tag 事件上下文加载逻辑
- 不新增独立的 board affinity API 端点（affinity 数据随 cluster 返回）
- 不实现自动 merge 建议

## Decisions

### D1: Phase A → 纯元数据注释

**决策**：`clusterCandidates()` 中，将 Phase A（`closestBoardContext` 匹配）从聚类逻辑中移除。改为聚类完成后，对每个纯自聚类簇计算与已有板块的亲和度。

**理由**：保持 Phase A 的信息价值（用户需要知道哪些已有板块相似），但不让它干扰聚类结果。

**实现方式**：新增 `BoardAffinity` 结构体，聚类后遍历每个簇的候选，计算与每个已有板块的距离，统计 matching_candidates 数量和 avg_distance。同时清理 `SemanticBoardUpgradeCluster` 中 `ExistingBoardID` 等 Phase A 遗留字段。

**替代方案**：完全移除 Phase A，不提供 board affinity 信息 → 用户失去了重要参考，增加了人工判断负担。

### D2: BoardAffinity 计算时机

**决策**：在 `clusterCandidates()` 返回后、LLM 调用前，作为 `clusterCandidates()` 的最后一步计算并附加到每个 cluster。

**理由**：保持 `clusterCandidates()` 作为唯一的聚类+元数据准备函数，调用方无需额外步骤。

**替代方案**：独立函数 `computeBoardAffinities()` → 增加了调用方复杂度，且与 cluster 结构紧密耦合。

### D3: LLM prompt 简化

**决策**：修改 `buildSemanticBoardUpgradePrompt()`，移除 `merge_into_existing` 选项和 `target_board_id` 字段。Prompt 只要求 LLM 判断 create_new 或 skip。同时将 `board_affinities` 信息注入 prompt（相似板块名称、匹配候选数、平均距离），让 LLM 在判断 skip 时有参考依据。

**理由**：LLM 在 merge 判断上不可靠（需要理解已有板块边界），且用户现在有 board_affinity 信息可以自行判断。但完全移除相似板块信息会影响 create_new vs skip 的判断质量。

### D4: 前端 merge dropdown

**决策**：在 `UpgradeSuggestionPanel.vue` 的每个 create_new 建议卡片上增加"合并到..."下拉按钮。下拉选项来自 suggestion 内嵌的 `board_affinities`（按 avg_distance 升序）。用户选择后，前端构造新的 `{decision: "merge_into_existing", target_board_id, auxiliary_label_ids}` 请求体，调用现有 ConfirmSuggestion API。

**理由**：复用现有 API，无需后端变更。board_affinity 信息直接内嵌在 suggestion 中（后端在 `suggestionsToDTO` 中根据 auxiliary_label_ids 汇总相关 cluster 的 affinities），前端无需做 cluster→suggestion 映射。

### D5: BoardAffinity 数据结构

**决策**：
```go
type BoardAffinity struct {
    BoardID          uint
    BoardLabel       string
    MatchingCandidates int    // 簇内与该板块距离 ≤ threshold 的候选数
    AvgDistance      float64  // 这些候选的平均距离
}
```

前端 TypeScript 对应接口：`{ board_id: number; board_label: string; matching_candidates: number; avg_distance: number }`

**理由**：`MatchingCandidates` 和 `AvgDistance` 提供了足够的参考信息。不包含候选 ID 列表以保持响应体积可控。

## Risks / Trade-offs

- **[LLM 不再产出 merge 建议]** → 用户可能遗漏合理的合并机会 → 通过 board_affinity 展示（matching_candidates 多、avg_distance 低）引导用户注意
- **[Phase A 移除后聚类结果变化]** → 原来被 Phase A 截走的候选现在参与自聚类，可能产生更大的簇 → 这是期望行为（更完整的语义聚类），但如果簇过大可能影响 LLM 判断 → 现有阈值机制控制簇大小
- **[前端增加 merge dropdown 复杂度]** → 需要从 cluster 到 suggestion 的映射 → 前端已有 cluster 数据，通过 index 关联即可

## Post-Implementation Fix: Two-Pass Clustering

### 问题

实现后发现 `clusterCandidates()` 的单遍贪心聚类存在两个缺陷：

1. **第一簇过度吸积**：Pass 1 使用 running-mean centroid（`updateCentroid`），随着成员加入 centroid 会漂移。先出现的簇像黑洞一样越吸越大，把本不属于一类的标签也吸进来。
2. **大量单标签簇**：相近标签被大簇抢走后，剩余标签只能各自成簇。`break` 导致只匹配第一个满足阈值的簇，即使候选更接近后来的簇。

### 修复

增加 **Pass 2 重分配迭代**：

1. Pass 1（不变）：贪心初始分群 + running-mean centroid
2. 用每个簇的全部成员重新计算**稳定 centroid**（`computeStableCentroid`）
3. Pass 2：每个候选重新分配到距离最近的稳定 centroid（距离 > threshold 则独立成簇）
4. 删除空簇，重算最终 centroid

### 关键代码

- `computeStableCentroid(candidates)`: 对所有候选 embedding 取算术平均，不依赖累积状态
- Pass 2 循环使用 `origIdx` 临时字段追踪 Pass 1 的簇索引（不序列化到 JSON）
- 仅当 `len(clusters) > 1` 时才执行 Pass 2（单簇无需重分配）

### 测试

- `TestClusterCandidatesPass2Reassignment`: 验证候选被重新分配到最近的稳定 centroid
- `TestClusterCandidatesPass2SplittingPreventsGiantFirstCluster`: 验证链式嵌入不会全被第一簇吞掉

## Follow-up Diagnosis: Centroid Strategy Is Still Insufficient

### 触发背景

两趟 centroid 聚类上线到实际候选数据后，仍观察到：

- 第一个/最大簇包含大量无关标签
- 仍有若干单标签簇
- 聚类观感与旧算法差异不大

因此做了一次只读诊断：从本地 PostgreSQL 读取真实候选标签（`label_type = auxiliary`、`status = active`、`ref_count >= 5`、未归入已有 board、embedding 非空），用当前阈值 `semantic_board_upgrade_cluster_distance_threshold = 0.35` 对比多种聚类策略。

### 诊断数据

真实候选数：`288`

当前 `centroid + Pass2` 结果：

```text
簇数 = 25
最大簇 = 198
top10 = [198, 36, 12, 5, 4, 4, 3, 2, 2, 2]
单标签簇 = 10
```

最大簇示例标签：

```text
美元、布伦特原油期货、WTI原油、纳斯达克 100 指数、道琼斯工业平均指数、
墨西哥、美股、纽约尾盘、乌克兰、亚马逊、谷歌、商务部、黎巴嫩、
救援工作、Meta、北京、蔚来、Android、微信、雷军、脑机接口、阿里云...
```

最大簇内部 pairwise 质量：

```text
size = 198
pairwise median = 0.469
pairwise mean   = 0.462
pairwise p90    = 0.531
pairwise max    = 0.624
距离 > 0.35 的标签对比例 = 95.6%
```

这说明最大簇内绝大多数标签彼此并不相似；它们只是都离某个 centroid 不远。

### 根因判断

当前问题不是 Pass 2 实现错误，而是 **centroid 阈值聚类不适合当前 embedding 分布**。

诊断发现，全局平均向量存在明显 hub 效应：

```text
到全局均值距离 median = 0.270
到全局均值距离 mean   = 0.274
距离全局均值 <= 0.25:  93 / 288
距离全局均值 <= 0.30: 217 / 288
距离全局均值 <= 0.35: 274 / 288
距离全局均值 <= 0.40: 284 / 288
```

也就是说，大多数候选标签都离“全局平均语义中心”较近。用 centroid 判断入簇时，会出现：

```text
不是标签之间真的互相相似，
而是它们都离“平均中心”不远。
```

因此 centroid 会变成“通用新闻语义中心”，导致大簇持续吸收无关标签。

### 阈值扫描

单纯调小/调大阈值也不理想：

```text
th = 0.20: 簇数=211, 最大=11,  单标签=175
th = 0.25: 簇数=136, 最大=30,  单标签=85
th = 0.30: 簇数=73,  最大=46,  单标签=38
th = 0.32: 簇数=48,  最大=73,  单标签=25
th = 0.35: 簇数=25,  最大=198, 单标签=10
th = 0.38: 簇数=10,  最大=255, 单标签=4
th = 0.40: 簇数=8,   最大=275, 单标签=4
```

结论：

- 阈值过小：大量单标签簇
- 阈值稍大：迅速形成巨型簇
- 仅靠阈值无法稳定得到“人类可读”的语义分组

### 策略对比

同一批真实候选数据、同一阈值 `0.35` 下：

| 策略 | 簇数 | 最大簇 | 单标签簇 | 观察 |
|------|------|--------|----------|------|
| 当前 centroid + Pass2 | 25 | 198 | 10 | 最大簇严重混杂 |
| pairwise 图连通分量 | 14 | 274 | 12 | 链式连通导致更大巨型簇 |
| complete-link 贪心 | 113 | 15 | 37 | 簇很干净，但偏碎 |
| average-link 贪心 | 84 | 18 | 29 | 较平衡，观感最好 |
| medoid + 约束（试验） | 81 | 12 | 36 | 控制住大簇，但部分簇仍需调参 |

`average-link 贪心` 示例簇：

```text
LangGraph、ReAct、GPT-5.5、qwen3.6、DeepSeek V4-Pro、OpenClaw、
Gemini 3.5 Flash、工具调用、Transformer、国产大模型、大模型、Vibe Coding...
```

```text
恒生科技指数、纳斯达克 100 指数、道琼斯工业平均指数、美股、纽约尾盘、
A 股、纳斯达克、日经225指数、证监会、恒指、深成指、创业板指...
```

### 后续方向

下一步不建议继续修补 centroid。建议将 `clusterCandidates()` 的自聚类核心替换为 **average-link greedy**：

候选加入簇时，不再只判断“离 centroid 是否足够近”，而是判断：

1. 候选至少和簇内一个真实成员距离 `<= threshold`
2. 候选与簇内所有成员的平均距离 `<= avgThreshold`
3. 若多个簇满足条件，选择平均距离最小的簇

接地气地说：

```text
旧规则：你离这个班的“平均脸”像不像？
新规则：你和班里真实同学像不像？你进来后这个班会不会变得太杂？
```

这会牺牲一点簇数量（更多小簇），但能避免一个大筐装进大量无关标签，更符合人工审核和 LLM prompt 的使用场景。
