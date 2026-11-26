<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI Design — tolerate-schema-envelope-extraction-output

## 1. 变更面

**无。** 本 change 不触碰任何前端文件。

## 2. 契约影响

- **无界面元素变化**：修复的是「模型输出形态 → 解析器」这一层，界面消费的仍是同一套 `topic_tags` / `article_topic_tags` 数据。
- 用户可感知的变化是**间接的**：打标成功率回升 → 更多文章拿到 event 标签 → 日报文章数恢复。这属于既有界面的数据变化，不是界面改形。

## 3. 验收

**none 档 = N/A 一致**：本 change 无 UI 制品可验；无需 agent-browser 走查、无需截图、无需原型审批。

## N/A Reason

本 change 声明 `ui-impact: none`，理由：

1. **零前端文件改动**——不新增、不删除、不改动任何组件/样式/交互；`front/` 目录一字未动。
2. **无布局契约影响**——不涉及 `AppPageShell` 四模式、`AppDialog` 尺寸档、`Z_INDEX` 浮层档位表、窄屏断点。
3. **无界面元素增删**——修复发生在「模型响应 → 解析器」这一层；界面消费的仍是同一套 `topic_tags` / `article_topic_tags` 数据，字段与语义均未变。
4. **用户可感知的变化是间接的**——打标成功率回升 → 更多文章拿到 event 标签 → 日报文章数恢复。这属既有界面的数据变化，不是界面改形，无需原型、无需双视口截图、无需 agent-browser 走查。
