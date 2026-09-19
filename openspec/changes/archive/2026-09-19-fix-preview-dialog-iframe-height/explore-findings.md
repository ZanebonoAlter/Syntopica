
## 预览弹窗 iframe 高度塌陷根因与修复锚点

**根因链**（jsdom 不可测布局，真实像素验收走人工/截图）：
`AppDialog`（front/app/components/ui/AppDialog.vue）`.app-dialog` 是 flex column 但高度由内容撑开（max-height:85vh 仅上限）；`.app-dialog__body` 是普通 block（非 flex）。`ArticlePreviewModal.vue` 的 `.preview-body` 写了 `flex:1; min-height:0`，父级非 flex → 无效，高度 auto。下游 `ArticleContentView`（front/app/features/articles/components/ArticleContentView.vue）根节点 `.article-content h-full`（父 auto → 百分比失效）→ iframe 模式下 `ArticleIframeView.vue` 根 `.iframe-mode flex-1` 内 `iframe.h-full` 塌陷到浏览器默认 ~150px（「一小坨」）。文本预览模式看似正常因内容自然流+弹窗 body 滚动。

**主界面不受影响**：FeedLayoutShell → `.feed-layout` 根 100vh → `.main-content`(flex:1) → `.content-panel`(flex:1, stretch 定高) → h-full 可解析。全屏模式 `.fullscreen-article fixed inset-0` 也定高。仅弹窗宿主中招。

**修复锚点**（design.md D1）：只动 ArticlePreviewModal.vue——`.preview-body` 改内联 `:style` 绑定 `height: calc(85vh - 110px)`（85vh − header≈61px − body 上下 padding 40px，留 9px 余量；宁小勿大防双滚动条），删除无效 `flex:1`。内联样式是 jsdom 组件测试可断言的结构契约锚点。不改 AppDialog（会波及全部弹窗）、不改 ArticleContentView/ArticleContent.css（reading.md 红线 145：ArticleContent.css 是共享宿主不得动）。

**复现测试**：新增 front/app/features/tags/components/ArticlePreviewModal.test.ts，mount 时 stub AppDialog/ArticleContentView/teleport，断言 `.preview-body` 存在 + 带 height 内联样式 + ArticleContentView 在其内。AppDialog 用 Transition+Teleport，测试需 stubs: { teleport: true }（参考 BoardEditDialog.test.ts 写法）。

**引用**：front/app/features/tags/components/ArticlePreviewModal.vue、front/app/components/ui/AppDialog.vue、front/app/features/articles/components/ArticleContentView.vue、front/app/features/articles/components/ArticleIframeView.vue

<!-- pinned 2026-09-19T13:31:42Z -->
