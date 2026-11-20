<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board -->

## Why
矩阵/追踪首屏多处重复解释日期格式、线索统计口径和治理规则，遮挡主任务。

## What Changes
保留原布局和操作，用轴单位、短图例、状态和入口替代常驻说明；规则移到详情或操作旁。

## Capabilities
### Modified Capabilities
- `workshop-board-console`：观察窗、矩阵图例、治理右轨的文案。

## Impact
前端 console 组件与回归测试；不改接口、统计、日期计算、权限或数据。
