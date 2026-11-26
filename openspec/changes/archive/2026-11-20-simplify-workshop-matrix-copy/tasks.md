## 1. 实现
- [x] 1.1 精简时间视窗与矩阵图例，保留状态、共享工作面口径与功能。
- [x] 1.2 精简右轨，将体检和收编说明放回详情/操作上下文。

## 2. Review
- [x] 2.1 聚焦审查改动和说明去向，已记录于 review.md，无阻塞项。

## 3. 测试
- [x] 3.1 更新受影响的组件回归测试。

## 4. 验证
- [x] 4.1 受影响前端回归 205 条通过，增量 lint、类型检查和 OpenSpec 严格校验通过。

| Scenario | 测试文件 |
| --- | --- |
| 曲线与矩阵格同口径 | front/app/features/tags/components/console/ObservationRangeControls.test.ts |
| 拖选不重排 | front/app/features/tags/components/console/ObservationMatrix.test.ts |
| 常驻口径说明可见 | front/app/features/tags/components/console/ObservationMatrix.test.ts |
| 右轨展示体检信号 | front/app/features/tags/components/console/BoardContextRail.test.ts |

## 5. 文档
<!-- doc-impact: semantic-board -->
- [x] 5.1 核对业务文档并补文案变更溯源。
