<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 影响面

本 change 的前端改动**只有一处文案**，无新组件、无新交互、无接口契约变化：

- `front/app/features/shell/components/FeedLayoutShell.vue`（`handleDeleteCategory` 的 `confirm` 文案）：由「这个操作不会删除分类下的订阅源。」改为「该分类下的订阅源及其文章也会一并删除，且不可撤销。」

## 为什么必须改（契约一致性，不是设计变更）

后端删除分类的实际语义 = 连带删除其下订阅源及其文章（真库靠存量遗留 FK `fk_categories_feeds` + `fk_feeds_articles` 级联；本 change 起代码显式删除，行为等价、与 FK 是否存在无关）。旧文案向用户承诺「不会删除订阅源」，与实现相反：真库（生产）**早就不成立**，无遗留 FK 的库里本 change 起也不成立。留着这条错误承诺会导致用户在一次不可撤销的破坏性操作上被误导，故随本 change 一并修正（用户 2026-09-17 明确选择「只改文案、保持删除语义」）。

## 复用契约（无新 UI 模式）

| 项 | 复用既有 |
| --- | --- |
| 确认交互 | 既有 `window.confirm` 原生确认框，不引入统一对话框（`unified-dialog` 化不在本 change 范围） |
| 文案风格 | 与既有破坏性操作文案一致（对照 `EditFeedDialog.vue` 删订阅源的「该订阅源下的文章也会一起删除」） |
| 空态/错误态/加载态 | 不变（失败仍 `alert(response.error || '删除失败')`） |

## 验收（双视口）

- 组件单测：本 change 不加新断言（文案无既有测试断言；改动为字符串替换，`pnpm exec nuxi typecheck` + `pnpm lint` 覆盖语法/类型）。
- 人工：设置页 → 侧栏分类右键「删除分类」 → 确认框文案为「该分类下的订阅源及其文章也会一并删除，且不可撤销。」（桌面 + 移动视口各看一次）。
