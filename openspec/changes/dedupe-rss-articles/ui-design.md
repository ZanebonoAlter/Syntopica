<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 纯后端（RSS 入库判重、打标复用、数据迁移），不改任何前端页面、接口契约与交互。proposal 已声明 `ui-impact: none`。前端可见的唯一"变化"是数据修复效果：打标记录/AI 调用记录里同一篇文章不再重复出现——这是存量数据治理的自然结果，非 UI 行为变更。
