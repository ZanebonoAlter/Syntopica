
## 组A 落地事实与 gutter 考古

【任务 1.1/1.2 落地事实（组A，2026-09-18 20:46 核验）】
- `front/app/composables/useMediaQuery.ts`：useMediaQuery(query) / useIsNarrowViewport() / NARROW_VIEWPORT_QUERY='(max-width: 767.98px)'；SSR 守卫 token `__mediaQueryClient`（测试可覆写）；onMounted 注册 + onUnmounted 清理；9/9 测试过
- `front/app/components/FeedLayout.css`：原规则零触碰，新增 31 行全在 @supports(height:100dvh) 与 @media(max-width:767.98px) 块内
- **gutter 考古**：`.feed-layout` 现状无任何 padding/gutter——宽屏 24px 是 AppPageShell 的 --shell-gutter 契约值，但主工作台不用 AppPageShell。窄屏 gutter 已做 `--feed-gutter: 16px` 变量于 .feed-layout，目前仅 .content-panel 右缘一处消费；**组D 需在 ArticleListPanelView（scoped 样式）自行取用该变量**
- `.main-content > * { min-width: 0 }` flex 收缩保护已加（窄屏块内）
- 侧栏隐藏/单栏化未做（归组D 2.3）——tasks 1.2 的「缩窗侧栏隐藏」验证项等组D 落地后补勾
- 本机 happy-dom@20.8.4 + VTU 下 wrapper.emitted() 失效（NotificationPanel.test.ts 头注释已文档化；AppDialog.test.ts 3 个交互用例预存挂）——交互测试统一用 onClose spy 模式（NotificationPanel.test.ts:175 先例）

<!-- pinned 2026-09-18T12:46:45Z -->
