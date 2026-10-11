## MODIFIED Requirements

### Requirement: 日报质量筛选
`collectBoardTags` 查询携带 `match_reason` 和 `score`（包括 fallback 路径产生的标签）。生成管线 SHALL 在聚类前按以下规则筛选标签：

1. **跨版块排他**：同一 tag 在其他版块存在 `score ≥ 本版块 score + exclusive_gap`（`semantic_board_report_exclusive_gap`，默认 0.3，ai_settings 可配；设为 0 时排他关闭）的挂载时，本版块 SHALL NOT 收该标签。排他剔除的标签 SHALL NOT 参与后续任何保底回捞（「有更强归属」是确定性判断，与弱规则保底语义不同）。主查询与 fallback 路径（现场计算匹配后择本版块结果）SHALL 应用同一规则。
2. 过滤 `direction_mismatch = true`
3. 保留 `match_reason ∈ {direct_hit, hit_rate, max_sim}`（含 downgraded）
4. 过滤 `weighted`（最弱规则）
5. 如果剩余 < 10 个标签，把 weighted 也拉回来（保底；被排他剔除的标签不回捞）
6. 如果剩余 > 30 个标签，按 `(tier, score)` 排序后截断到 top-30

Fallback 标签同等对待：fallback 路径产生的标签也携带 `match_reason`/`score`，筛选规则完全一致（含跨版块排他）。

#### Scenario: 跨版块排他剔除弱归属标签
- **WHEN** tag「美国非农数据超预期降温」挂载「美国新闻」score=1.0（max_sim）与「中国国内新闻」score=0.7（direct_hit），分差 0.3 ≥ exclusive_gap=0.3，生成「中国国内新闻」日报
- **THEN** 该标签 SHALL NOT 进入「中国国内新闻」日报的聚类候选（「美国新闻」日报不受影响）

#### Scenario: 排他不影响分差不足的双挂载
- **WHEN** tag「中美贸易谈判」挂载「美国新闻」score=0.8 与「中国国内新闻」score=0.7，分差 0.1 < exclusive_gap=0.3
- **THEN** 两个版块日报 SHALL 均可收该标签（真跨界场景保留多版块呈现）

#### Scenario: 排他剔除不参与保底回捞
- **WHEN** 某版块当日收集 12 个标签，其中 3 个被跨版块排他剔除、5 个为 weighted，剩余 4 个 (< 10)
- **THEN** 系统 SHALL 把 weighted 拉回（9 个进入聚类），被排他剔除的 3 个标签 SHALL NOT 回捞

#### Scenario: 质量筛选过滤 weighted
- **WHEN** 收集到 20 个标签，其中 5 个 weighted
- **THEN** 系统 SHALL 过滤掉 weighted 标签，15 个进入聚类

#### Scenario: 保底机制
- **WHEN** 收集到 12 个标签，过滤 weighted 后只剩 8 个 (< 10)
- **THEN** 系统 SHALL 把 weighted 标签拉回，12 个全部进入聚类

#### Scenario: 截断机制
- **WHEN** 收集到 40 个标签，过滤后剩 35 个 (> 30)
- **THEN** 系统 SHALL 按 (tier, score) 排序后截断到 top-30

#### Scenario: fallback 路径同样应用排他
- **WHEN** fallback 现场计算某无挂载 tag 的匹配，结果含「美国新闻」score=1.0 与「中国国内新闻」score=0.7，当前正在生成「中国国内新闻」日报
- **THEN** fallback SHALL NOT 把该标签以 0.7 分收进「中国国内新闻」候选
