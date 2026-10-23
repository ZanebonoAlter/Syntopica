<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI Design: optimize-search-semantic-index

## N/A Reason

本 change 是纯后端语义召回性能修复（数据库索引 + SQL 重写），不新增/修改任何用户可见界面、交互或布局；搜索页现有「语义匹配暂不可用」降级提示条的行为契约不变（语义路径恢复后该提示自然减少），因此 ui-impact 声明为 none，无需 UI 原型或视觉验收。
