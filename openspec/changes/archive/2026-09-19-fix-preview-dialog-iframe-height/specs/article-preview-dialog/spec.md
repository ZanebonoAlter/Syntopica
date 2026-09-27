## Purpose

文章预览弹窗（tags 域日报/版块共用的 `ArticlePreviewModal`）的能力契约：AppDialog 宿主内嵌 `ArticleContentView`，弹窗内容区持有确定高度，文本预览与内嵌网页（iframe）两种视图模式都撑满弹窗可用高度，不因宿主高度链断裂而塌陷。

## ADDED Requirements

### Requirement: 预览弹窗内容区持有确定高度

文章预览弹窗的内容区 MUST 携带显式确定高度（基于 AppDialog 85vh 上限扣除 header 与 body 内边距），MUST NOT 依赖 `flex: 1` 从非 flex 宿主解析高度；弹窗总高度 MUST NOT 超出 AppDialog 的 85vh 上限（内容区高度 + header + body padding ≤ 85vh），MUST NOT 出现弹窗 body 与内容区双滚动条。

#### Scenario: 切换到内嵌网页模式

- **WHEN** 用户在文章预览弹窗中点击「切换到内嵌网页」
- **THEN** iframe 撑满弹窗内容区可用高度（而非浏览器默认 ~150px），加载中/失败态浮层同样铺满内容区

#### Scenario: 文本预览模式内部滚动

- **WHEN** 用户在文章预览弹窗中查看文本预览且内容超长
- **THEN** 滚动发生在内容区内部，弹窗总高度不超过 85vh，工具栏保持可见

#### Scenario: 高度契约结构锚点

- **WHEN** 渲染 `ArticlePreviewModal` 且选中文章存在
- **THEN** 内容区元素携带显式高度声明（组件测试可断言的结构契约），`ArticleContentView` 渲染于内容区内
