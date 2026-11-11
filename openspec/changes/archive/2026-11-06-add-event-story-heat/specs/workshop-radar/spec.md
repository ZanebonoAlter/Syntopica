## MODIFIED Requirements

### Requirement: 版块行构成与分档
每个雷达行 SHALL 包含：版块名、当日事件线数、近三日与前三日的热度环比、该版块前 3 条泳道的 14 天事件线密度、最新日报标题（无日报时 SHALL 显示「今天暂无日报」）、以及进入版块的动作。

「关注度」SHALL 由 lane 热度派生（该版块当日 `lane_heat`，保留一位小数），SHALL NOT 由 `section` 数、`事件` 数或分区数加权得到。行 SHALL 按关注度分档：热点行带「热点」分档标记；关注度低于阈值的版块 SHALL 折叠为一行「常温」摘要，不逐行占据注意力。

分档阈值 SHALL 为可配置项，且 SHALL 按 `lane_heat` 的真实分布标定（标定依据记录在 design 或实现文档中），MUST NOT 直接沿用旧 `section` 计数量纲下的 50 / 15。分档数字 SHALL 在界面上可读（hover 或行内），MUST NOT 只以颜色表达。

#### Scenario: 热点行带理由与密度

- **WHEN** 某版块当日 `lane_heat` 为全站最高且环比上升、且达到热点阈值
- **THEN** 该行 SHALL 排在最前、带「热点」分档标记，并展示最新日报标题与泳道事件密度条

#### Scenario: 常温版块折叠

- **WHEN** 某版块近 14 天没有任何事件（无事件线且无非综合稿事件）
- **THEN** 该版块 SHALL 只出现在「常温」摘要行中，不展开为独立雷达行

#### Scenario: 关注度不因分区数虚高

- **WHEN** 版块 A 当日 8 个 section 但只对应 1 个事件线（2 个来源），版块 B 当日 2 个 section 对应 2 个事件线（共 6 个来源）
- **THEN** B 的 `lane_heat` SHALL 高于 A，雷达排序 SHALL 把 B 排在 A 之前，MUST NOT 因 A 的 section 数更多而反转

### Requirement: 今日该读与升温提示
雷达 SHALL 在侧栏展示跨版块「今日该读」条目（取关注度最高的若干版块的当日日报标题、文章数、事件数），并展示「今天可能升温」清单：某版块当日 `story_heat` 合计与近三日日均值之比 ≥ 2 且当日事件线数 ≥ 3 时列入，并标出倍数。两项 SHALL 只读，不得触发日报生成或匹配。

「事件数」SHALL 为当日去重后的事件线数；未回填时 SHALL 按非综合稿事件数作上界并标注降级，MUST NOT 用 `section` 数代替。倍数 SHALL 按热度计算，SHALL NOT 按分区数计算。

#### Scenario: 升温判定

- **WHEN** 版块当日 3 个事件、热度合计 6.0，近三日日均热度 2.0
- **THEN** 该版块 SHALL 出现在「今天可能升温」清单并显示 3.0×

#### Scenario: 分区多但事件少不升温

- **WHEN** 版块当日 8 个 section 但只有 1 个事件、热度合计 1.2，近三日日均 1.0
- **THEN** 该版块 SHALL NOT 出现在升温清单，MUST NOT 因 section 数 ≥ 3 而列入

#### Scenario: 无升温

- **WHEN** 当日没有版块的事件线热度超过 2 倍阈值
- **THEN** 清单 SHALL 显示「暂未触发升温条件」而不是空白
