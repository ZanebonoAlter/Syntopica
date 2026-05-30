# #6 - 无聚合标签时未归属层级树不展示

## What to build

`TagHierarchy.vue` 中 `th-unplaced-section`（第 585 行）在 `v-else` 分支内（第 564 行），该分支条件是 `sortedNodes.length > 0`。当没有任何已归属的聚合标签/Node 时（`sortedNodes` 为空），渲染走 `v-else-if="sortedNodes.length === 0"` 显示空状态文案，**unplaced section 永远不会渲染**。

将 `th-unplaced-section` 移到条件分支外独立渲染，使未归属标签在任何情况下都能展示。

## Acceptance criteria

- [ ] `sortedNodes` 为空且 `unplacedTags` 有数据时，未归属 section 正常显示
- [ ] `sortedNodes` 有数据时，未归属 section 仍正常显示在树下方
- [ ] 两者都为空时，显示原有空状态文案
- [ ] 展开/收起未归属列表功能正常

## Blocked by

None - can start immediately.
