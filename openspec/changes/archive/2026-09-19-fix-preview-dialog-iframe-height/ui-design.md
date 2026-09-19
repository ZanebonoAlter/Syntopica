<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

- 入口不变：日报/版块页点文章 → `ArticlePreviewModal`（AppDialog 宿主，`width="90vw"` 存量兼容档）→ 工具栏「切换到内嵌网页」按钮。
- 变更仅限弹窗内容区高度解析方式（auto → 确定高度），入口、导航、控件全部不动。

## 受影响状态

- **success（iframe 模式）**：修复——iframe 撑满弹窗内容区（此前塌陷到 ~150px）。
- **success（文本预览模式）**：滚动容器从弹窗 body 移到内容区 `.preview-mode` 内部，工具栏/阅读进度条常驻不随内容滚走。
- **loading（iframe 加载中）**：`.iframe-loading` 绝对定位铺满内容区，随内容区获得确定高度自然修复，无需额外改动。
- **error（iframe 加载失败/无 src）**：`.iframe-error` 同上，铺满内容区。
- **empty**：无文章时弹窗不渲染内容（`v-if="selectedPreviewArticle"`），不受影响。

## 复用组件与布局模式

- 复用 `AppDialog`（存量 `width="90vw"` 档，本次不改外壳、不新开尺寸档——layout.md dialog 四档约束不涉及本修复）。
- 复用 `ArticleContentView` 全部视图模式（preview / iframe / fullscreen），不改其内部布局。
- 唯一新增：内容区确定高度 `calc(85vh - 110px)`（AppDialog `max-height: 85vh` − header ≈61px − body 上下 padding 40px，留 9px 余量防双滚动条）。

## 验收映射

- 组件测试（jsdom）：断言 `.preview-body` 携带内联确定高度样式（结构契约锚点）+ 文章选中时 ArticleContentView 渲染于其内。
- 人工/截图验证：日报文章预览切内嵌网页，iframe 撑满；文本模式滚动正常（本 change 验证节记录）。

无需原型（minor 档，复用既有布局契约）。
