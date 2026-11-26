<!-- domain: topic-graph -->

## Purpose

管理 DailyReportSection 之间跨天关系的三阶段二分图最优匹配。通过匈牙利算法（Phase 1）求解相邻天的 1:1 全局最优分配，再通过 Phase 2 split/merge 检测和 Phase 3 skip-day 补查，覆盖所有有意义的 section 间延续关系。相比旧增量贪心匹配，从根本上解决了累积误差和 merge 泛滥问题。

## Requirements

### Requirement: 三阶段相邻天二分图匹配

系统 SHALL 对每对相邻天（Day_i, Day_{i+1}）的 sections 执行三阶段匹配，生成 section 间关系。

**Phase 1：匈牙利 1:1 分配（带 penalty）**

输入：left_sections（较早天的 sections）和 right_sections（较晚天的 sections）。

系统 SHALL 用单条 SQL cross-join 一次性获取所有 pair 的 embedding cosine distance（截断 query_cutoff=0.35）：
```sql
SELECT s1.id, s2.id, s1.embedding <=> s2.embedding AS dist
FROM daily_report_sections s1, daily_report_sections s2
WHERE s1.report_id 指向 Day_i AND s2.report_id 指向 Day_{i+1}
  AND s1.embedding IS NOT NULL AND s2.embedding IS NOT NULL
  AND s1.cluster_label IS NOT NULL AND s1.cluster_label != ''
  AND s2.cluster_label IS NOT NULL AND s2.cluster_label != ''
  AND s1.embedding <=> s2.embedding < 0.35
```

注意：`cluster_label` 非空过滤必须保留，否则空标签但已有 embedding 的 section 会参与匹配，产生无意义关系。

系统 SHALL 构建扩展方阵（size = max(left, right) × 2），其中：
- 真实 pair 的代价 = embedding distance（超过 penalty=0.28 的设为 INF）
- left → dummy 的代价 = penalty（允许左侧不匹配）
- dummy → right 的代价 = penalty（允许右侧不匹配）
- dummy → dummy 的代价 = penalty × 0.5

系统 SHALL 执行匈牙利算法（O(n³)）求最小代价全局最优 1:1 分配，仅保留真实 left → 真实 right 且 distance ≤ penalty 的匹配作为 primary matches。

**Phase 2：Split/Merge 检测**

对 Phase 1 未匹配的 right section（emerging 候选）：
- 在已匹配的 left sections 中找最近邻，distance ≤ split_ceiling(0.30)
- 若该最近邻的 distance 与该 left section 的 primary match distance 的 gap < split_gap(0.03) → 标记为 split，写入关系
- 否则该 section 保持 emerging

对 Phase 1 未匹配的 left section（ending 候选）：
- 在已匹配的 right sections 中找最近邻，distance ≤ split_ceiling(0.30)
- 若 gap < split_gap(0.03) → 标记为 merge，写入关系
- 否则该 section 保持 ending 候选

**Phase 3：Skip-Day 补查**

对 Phase 1+2 后仍完全未匹配的 section，检查**隔一天**（Day_i → Day_{i+2}）的 section：
- from_section 在中间天（Day_{i+1}）无任何延续关系（即 from_section 不出现在任何 Day_{i+1} → Day_{i+1} 之后的 relation 中，也不出现在 Day_i → Day_{i+1} 的 Phase 1+2 结果中）
- embedding distance < skip_day_threshold(0.20)
- 满足条件则写入 skip-day 关系
- **仅检查隔一天**，不检查更远距离。隔两天以上的 skip-day 语义过弱，且实际数据中几乎所有有价值的 skip-day 均为隔一天

#### Scenario: Phase 1 完美 1:1 延续
- **WHEN** Day_i 有 sections [A, B, C]，Day_{i+1} 有 [D, E, F]
- **AND** 距离矩阵为 A→D=0.01, B→E=0.02, C→F=0.03（其余均 > 0.28）
- **THEN** Phase 1 SHALL 产生 3 条 primary：A→D, B→E, C→F

#### Scenario: Phase 1 penalty 阻止不相关匹配
- **WHEN** Day_i 有 [A, B]，Day_{i+1} 有 [C]
- **AND** A→C=0.31, B→C=0.32（均 > penalty=0.28）
- **THEN** Phase 1 SHALL 不产生任何匹配，C 为 emerging，A/B 为 ending 候选

#### Scenario: Phase 2 split 检测
- **WHEN** Phase 1 中 left section A primary match → right section D (dist=0.15)
- **AND** 未匹配的 right section E 与 A 的 distance=0.17（gap=0.02 < 0.03, ≤ split_ceiling 0.30）
- **THEN** Phase 2 SHALL 写入 split 关系 A→E (dist=0.17)

#### Scenario: Phase 2 split gap 过大不写入
- **WHEN** Phase 1 中 A→D (dist=0.10)
- **AND** 未匹配的 E 与 A distance=0.25（gap=0.15 ≥ 0.03）
- **THEN** Phase 2 SHALL 不写入 A→E，E 保持 emerging

#### Scenario: Phase 2 merge 检测
- **WHEN** Phase 1 中 left section A 未匹配
- **AND** 已匹配的 right section D 的 primary match 来自 B (dist=0.12)
- **AND** A→D distance=0.14（gap=0.02 < 0.03）
- **THEN** Phase 2 SHALL 写入 merge 关系 A→D (dist=0.14)

#### Scenario: Phase 3 skip-day 补查
- **WHEN** left section S1 (Day_i) 在 Phase 1+2 后未匹配
- **AND** Day_{i+1} 上 S1 无任何延续关系
- **AND** S1 与 Day_{i+2} 的 section S3 distance=0.09 (< 0.20)
- **THEN** Phase 3 SHALL 写入 skip-day 关系 S1→S3 (dist=0.09)

#### Scenario: Phase 3 skip-day 有中间延续则跳过
- **WHEN** left section S1 (Day_i) 未匹配，但 S1 在 Day_{i+1} 有延续关系 S1→S2
- **THEN** Phase 3 SHALL 不对 S1 做 skip-day 补查

#### Scenario: 首日报告无匹配
- **WHEN** board 的第一份报告保存，无 Day_{i-1}
- **THEN** 所有 section SHALL 为 emerging，不执行任何匹配

#### Scenario: 一天多个 section 均无匹配
- **WHEN** Day_{i+1} 的所有 sections 与 Day_i 的 distance 均 > penalty
- **THEN** Day_{i+1} 所有 sections SHALL 为 emerging，Day_i 的所有 sections SHALL 为 ending 候选

### Requirement: 匈牙利算法实现

系统 SHALL 实现纯 Go 的 O(n³) 匈牙利算法（Kuhn-Munkres），用于求解最小代价二分图 1:1 分配。

输入：n×n 代价矩阵（float64），INF(=1e6) 表示不可匹配。选择 1e6 而非 10.0 是为了确保即使未来 penalty 参数调大，INF 仍远大于任何有效代价。
输出：一组 (row, col) 分配对及其代价。

算法 SHALL 正确处理非方阵（通过 padding 到 max(left, right) × max(left, right)）。

#### Scenario: 方阵最优分配
- **WHEN** 代价矩阵为 [[0.01, 0.30], [0.25, 0.02]]
- **THEN** 算法 SHALL 返回 [(0,0,0.01), (1,1,0.02)]

#### Scenario: 非方阵 padding
- **WHEN** left 有 3 sections，right 有 2 sections
- **THEN** 算法 SHALL padding 到 3×3 矩阵（或 5×5 带 dummy），正确分配

#### Scenario: 全 INF 矩阵
- **WHEN** 所有 pair distance > penalty，矩阵全为 INF
- **THEN** 算法 SHALL 返回空分配

### Requirement: 阈值参数可配置

以下阈值 SHALL 定义为具名常量或配置变量：

| 参数名 | 默认值 | 用途 |
|--------|--------|------|
| `MatchPenalty` | 0.28 | 匈牙利算法的不匹配代价上限 |
| `SplitGap` | 0.03 | Phase 2 split/merge 的 gap 阈值 |
| `SplitCeiling` | 0.30 | Phase 2 候选的最大距离 |
| `SkipDayThreshold` | 0.20 | Phase 3 skip-day 距离阈值 |
| `QueryCutoff` | 0.35 | SQL cross-join 距离截断 |

#### Scenario: 使用默认阈值
- **WHEN** 系统启动，未提供自定义参数
- **THEN** 系统 SHALL 使用上述默认值

#### Scenario: 常量定义位置
- **WHEN** 开发者需要修改阈值
- **THEN** 阈值 SHALL 可在 `repository.go` 文件顶部找到并修改
