## Context

`ArticlePreviewModal`（`front/app/features/tags/components/ArticlePreviewModal.vue`）在 `AppDialog` 内渲染 `ArticleContentView`。高度链现状：

```
.app-dialog（flex column，max-height: 85vh，高度由内容撑开）
└─ .app-dialog__body（block，flex:1，overflow-y:auto —— 不是 flex 容器）
   └─ .preview-body（flex:1 无效 → 高度 auto）
      └─ .article-content h-full（父 auto → 百分比失效 → auto）
         ├─ .preview-mode flex-1 overflow-y-auto（文本模式：内容自然流，弹窗 body 滚动，看似正常）
         └─ .iframe-mode flex-1 > iframe h-full（iframe 模式：塌陷到浏览器默认 ~150px）
```

主界面不受影响：`FeedLayoutShell` 的 `.feed-layout` 根部 `100vh` → `.main-content` flex:1 → `.content-panel` flex:1（stretch 得确定高度）→ `h-full` 可解析。

## Goals / Non-Goals

**Goals:**
- 弹窗内 iframe 模式撑满内容区；文本预览模式内部滚动、总高不超 85vh。
- 修复范围最小化：只动 `ArticlePreviewModal.vue`（+ 新测试）。

**Non-Goals:**
- 不改 `AppDialog`——通用外壳，其余十余个弹窗依赖内容撑高，给它加固定高度/强制 flex 会波及全部弹窗。
- 不改 `ArticleContentView` / `ArticleIframeView` / `ArticleContent.css`——组件本身契约正确，问题在宿主。
- 不迁移 `width="90vw"` 存量档到 size 四档（独立议题，与本 bug 无关）。

## Decisions

**D1：内容区给确定高度 `height: calc(85vh - 110px)`，而不是改造 AppDialog 为 flex。**
- 备选 A（改 AppDialog body 为 flex column）：不可行——弹窗整体高度仍是内容撑开（max-height 只是上限），`.preview-body` 的 `flex:1` 在 auto 高度的 flex 容器里同样解析不出确定高度，修不掉塌陷；且影响全部弹窗。
- 备选 B（本做法）：高度在弹窗这一层落地，链路上游唯一需要确定高度的点被满足，下游 `h-full`/`flex:1` 全部自然解析。110px = header（16+16 padding + ~28px 关闭钮 + 1px 边框）≈61px + body 上下 padding 40px，留 9px 余量防双滚动条。
- 高度用内联 `:style` 绑定而非 scoped CSS：jsdom 不算布局，内联样式是组件测试可断言的结构契约锚点（测试设计 §层选择：jsdom 层测结构契约，真实布局留给人工/截图验收）。

**D2：复现测试 = 组件测试断言结构契约。** Bug 修复用例先行（§2 不可豁免）：jsdom 无法度量真实像素高度，以「内容区必须带显式高度声明」为可断言契约（修复前该断言失败=复现，修复后通过）；iframe 撑满的视觉效果由人工/截图验收兜底。

## Risks / Trade-offs

- header 高度变化（如未来加标题行）会使 110px 失准——余量设计为「宁小勿大」：偏小只浪费几像素空间，偏大会触发 85vh 截断 + 双滚动条。已在样式中注释推导式。
- 极矮视口下 `calc(85vh - 110px)` 可能过小（如视口 500px 时内容区 ≈315px）——可用性可接受，与主界面窄屏行为一致，不做额外断点。
