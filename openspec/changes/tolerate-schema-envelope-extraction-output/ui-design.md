# UI Design — tolerate-schema-envelope-extraction-output

> ui-impact: **none**（纯后端解析层修复，无任何界面新增/删除/改形）

## 1. 变更面

**无。** 本 change 不触碰任何前端文件。

## 2. 契约影响

- **无界面元素变化**：修复的是「模型输出形态 → 解析器」这一层，界面消费的仍是同一套 `topic_tags` / `article_topic_tags` 数据。
- 用户可感知的变化是**间接的**：打标成功率回升 → 更多文章拿到 event 标签 → 日报文章数恢复。这属于既有界面的数据变化，不是界面改形。

## 3. 验收

**none 档 = N/A 一致**：本 change 无 UI 制品可验；无需 agent-browser 走查、无需截图、无需原型审批。
