<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

- **settings → 订阅源管理（SettingsSectionFeeds）→ feed 详情（FeedDetailEditor）**：本次补齐能力的唯一编辑界面。新增"AI 总结"主开关、"刷新后自动总结"（原误导名"内容补全"改名+改文案）、最大重试次数、RSS 地址编辑。
- **主界面（FeedLayoutShell）"编辑订阅源"入口**：由弹窗 `EditFeedDialog` 改为跳转 settings 深链 `/settings?feed=<id>&section=feeds`，落地后自动选中该 feed 详情。
- **移除**：`EditFeedDialog.vue` 整体删除（其删除订阅源能力 settings 已有同款确认流程）。

## 受影响状态

- **loading**：深链跳转后 settings feeds 列表加载中——复用现有列表 loading 态，feed 参数在列表就绪后自动选中并滚动定位；列表加载失败时深链参数保留在 URL，重试成功后仍能定位。
- **empty**：无 feed 时深链目标不存在——落到订阅源管理空态（现有），忽略失效 feed 参数，不报错。
- **error**：toggle 更新失败——复用 `updateFeedSetting` 现有错误提示（toast），开关回退到实际值；URL/重试次数保存失败同现有表单错误态。
- **success**：开关切换即时生效（现有 PATCH 单字段模式），无额外成功提示；词汇表文案按 proposal 统一表替换。

## 复用组件与布局模式

- 全部复用 `FeedDetailEditor` 既有结构：`feed-detail__toggle-row`（toggle 行：图标+名称+说明+AppToggle）、`feed-detail__label` 表单行、settings 页现有 contained 布局模式，不引入新布局 mode、不新增自由宽度。
- 新增字段沿用现有控件：AppToggle（两个新开关行）、AppInput / number input（重试次数）、AppInput url（RSS 地址）。
- 深链用 Nuxt 路由 query（`useRoute`/`navigateTo`），无新组件。
- 主界面入口跳转复用 `navigateTo`，菜单项文案不变。

## 验收映射

- 组件测试：FeedDetailEditor 补齐字段的渲染与 update-feed 事件断言（现有测试文件扩展）；EditFeedDialog 删除后相关引用与测试清理；stores/api 映射默认值回退测试（`?? false` 统一）。
- 人工验证：settings 打开任一 feed 详情确认三开关+重试+URL 可用；主界面菜单点"编辑订阅源"跳转并定位；两处文案与词汇表一致。
