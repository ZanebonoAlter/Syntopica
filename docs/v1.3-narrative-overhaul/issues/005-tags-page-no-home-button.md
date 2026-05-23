# #5 - /tags 页面缺少返回首页按钮

## What to build

`TagsPage.vue` 是独立全屏页面，自带 topbar/sidebar/content/bottombar，不使用任何 layout wrapper。用户从首页 sidebar 进入 `/tags` 后，没有任何 UI 元素可以返回首页，只能靠浏览器后退按钮。

在 topbar 左侧（图标+标题之前）添加一个 home/arrow 图标按钮，点击 `navigateTo('/')`。

## Acceptance criteria

- [ ] topbar 左侧有返回首页按钮，视觉与现有 topbar 风格一致
- [ ] 点击后导航到 `/`
- [ ] 不影响现有 category tabs 和设置按钮布局

## Blocked by

None - can start immediately.
