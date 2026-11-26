<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 是**层级缺陷修复**，不引入任何新的 UI 结构、交互路径、状态机或视觉设计：

- 界面元素一个不加、一个不改、不挪位置：组合标签创建弹窗的 DOM、样式、文案、字段、按钮全部保持现状；唯一变化是遮罩的 z-index 从「写死 60」改为「取档位表的嵌套档 1100」。
- 修复的是「弹窗被盖住、用户看不见」这一**已存在缺陷**，不是新增界面或重排信息架构；用户可见行为回到设计本意（弹窗出现在宿主工作面之上），与 `standard/frontend/layout.md` 既有弹窗契约一致。
- 契约层（`layout-contract.ts` 的 `Z_INDEX` 档位表 + `layout.md` 增节）属代码规约，不构成 UI 设计变更。

因此按 `ui-impact: none` 归档，`ui-design.md` 为最小 N/A 记录节；UI 验收走组件层机械断言（层级数值比较）+ 浏览器实机 `elementFromPoint` 命中弹层本体（见 tasks 验证节），不产出原型。

## 层级契约（随契约落地的口径，非设计变更）

| 档位 | 值 | 用途 | 本次是否动用 |
| --- | --- | --- | --- |
| `dialog` | 1000 | 基础弹窗遮罩（AppDialog 默认） | 复用（数值不变） |
| `dialogNested` | 1100 | 弹窗内再开的嵌套浮层 | ✅ 组合创建弹窗改走此档 |
| `readingDialog` | 9100 | 阅读层弹窗（文章预览 / 批注确认） | 不动 |
| `readingFullscreen` | 9200 | 弹窗内全屏层 | 不动 |
| `toast` | 9999 | 全局提示 / 菜单 | 不动 |

规则一句话：**弹窗内再开的浮层，层级必须严格高于宿主弹窗遮罩**；宿主在更高档时按 `nestedOverlayZ(hostZ)` 抬升。
