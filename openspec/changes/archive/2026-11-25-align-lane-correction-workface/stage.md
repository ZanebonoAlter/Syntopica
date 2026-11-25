<!-- stage: implement -->
<!-- constraint-domains: daily-report, topic-graph -->

# align-lane-correction-workface — 阶段载体

## 当前进度

| 制品 | 状态 | 备注 |
|---|---|---|
| proposal | ✅ 完成 | `complexity: complex` / `ui-impact: minor` |
| ui-design | ✅ 完成 | minor 复用契约，`ui-approval: not-required`、`ui-prototype: none` |
| specs | 进行中 | 4 个 delta：lane-correction-surface / lane-manual-correction / workshop-lane-compose / persistent-topic |
| design | 待写 | 关键决策见下 |
| tasks | 待写 | 含 Review 节（complex 档强制） |

## 用户已拍板的决策（2026-11-24）

1. **拖拽落点语义**：拖入成员区 = 加入/加回，成员拖出成员区 = 移出，松手即落库；只读参照与「更早」折叠带节点不可拖。
2. **新建画布余量分配**：填满容器，多出的高度全部给底部散料区，本次泳道行与候选链行位置不变。
3. **UI 档位**：minor，直接实现，不出原型（做完截图验收）。
4. **手动建泳道的首义集合**：建话题时选中的节点即首义集合，在建话题事务内写 `topic_anchor_overrides` 留痕；历史手动建的泳道不回填，显示「创建时写入 · 来源不可考」。

## 待定 / 实现期确认

- 相对归一的成员基准半径调整幅度（会牵动 `NODE_BLOCK_H/nodeRowGap/membersBottom/zones` 整条布局链，需与画布高度贴合容器同批验证）。
- `ListCentroidWindowSections` 抽出后 `ComputeTopicCentroid` 的等价性验证方式（当前窗口 SQL 含 `ORDER BY period_date DESC, s.id DESC LIMIT window`）。
