## MODIFIED Requirements

### Requirement: 展示范围与排序

泳道动态区 SHALL 展示「关注（active）」与「我在追踪（watch 关联）」的泳道，按**近 14 天有锚定 section 的天数**（同日多条只计一次）降序排列，二次排序按最近命中日期倒序、再按 topic id 升序；watch 关联泳道 SHALL 带追踪标识。近 14 天无锚定 section 的 active 泳道（沉寂）SHALL NOT 展示。candidate 泳道 SHALL NOT 出现在主卡片区。

排序口径 SHALL NOT 使用 section 条数：泳道内主题分桶会把同一话题同一天的内容拆成多条 section，按条数排序会让"当天拆得多的泳道"虚高。

#### Scenario: 活跃排序

- **GIVEN** 泳道 A 近 14 天有 6 天出现锚定 section（其中某天 3 条），泳道 B 有 3 天（每天 1 条）
- **WHEN** 渲染泳道动态区
- **THEN** A 卡 SHALL 排在 B 卡前（6 天 > 3 天），SHALL NOT 因为 A 的 section 条数被拆分放大而进一步加权

#### Scenario: 同日多条只计一天

- **GIVEN** 泳道 A 仅有 1 天出现锚定 section，但该天被拆成 4 条 section；泳道 B 有 2 天、每天 1 条
- **WHEN** 渲染泳道动态区
- **THEN** B 卡 SHALL 排在 A 卡前（2 天 > 1 天）

#### Scenario: watch 标识

- **WHEN** 某泳道被用户 watch（追踪）关联
- **THEN** 该泳道卡 SHALL 带追踪标识（与普通 active 卡视觉区分）

#### Scenario: 沉寂不展示

- **WHEN** 某 active 泳道近 14 天无任何锚定 section
- **THEN** 该泳道 SHALL NOT 渲染卡片

#### Scenario: 候选不进主区

- **WHEN** 某 candidate 泳道近 14 天有锚定 section
- **THEN** 该泳道 SHALL NOT 出现在主卡片区（仅可能出现在候选栏）
