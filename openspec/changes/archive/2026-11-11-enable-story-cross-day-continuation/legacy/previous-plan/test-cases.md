[spill] 完整输出 45494 字节已存 .pi/harness/spill/01a1c0a1-6f7a-76b1-aee8-fe8e47fb198c/1794312838821-read-a8089b324476513c.txt，需要完整内容时用 read 读取该路径（已省略 42933 字节）
--- 头部 2048 字节 ---
# test-cases: 事件线跨天延续（两层判定）

**故事**：同一件事在 11-03 和 11-04 各被报道一次时，读者在日报与态势图里看到的是**一条事件线**（而不是两条各活一天的死线）；而同一篇日报里切出来的两段互不相干的线索**仍然分开**（同日去重不放宽）。事件线一旦跨天，矩阵的每日格子与「今天可能升温」才有真实的分母；数值型事件（汇率中间价、指数点位）不再因为「数值命不中标签名的词」而永远不可信。

**测试单元**：Requirement 用户故事；spec Scenario 为断言片段。以下均为**待实现 / 待执行**用例，不表示当前实现已具备能力。

## 1. 主链路节拍

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 提取一条 event 标签，指纹里带 `object_type` | 同一次调用产出指纹 | 指纹六字段在**同一次**提取调用里产出；系统 SHALL NOT 为 `object_type` 另起一次 LLM 调用 | 函数单测 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go`；`backend-go/internal/tagmanagement/service/core/tagger_fingerprint_test.go` |
| 2 | 数值型事件的标签被判可信 | 参数型对象不再永久不可信 | `object=6.7253`、`object_type=value`、关键词为标签名用词时 → 指纹可信，不再因数值命不中关键词而永久不可信 | 函数单测 | `backend-go/internal/topicgraph/service/story_fingerprint_test.go` |
| 3 | 角色声明不成立时回落 | 参数型对象声明不成立时回落为实体型 | `object_type=value` 但 object 不以数字开头 → 按 entity 处理，回到关键词命中要求 | 函数单测 | `backend-go/internal/topicgraph/service/story_fingerprint_test.go` |
| 4 | 存量标签（无角色字段）走保守路径 | 存量指纹无角色字段时按实体型处理 | 缺 `object_type` → 按 entity；行为与今天一致，不猜角色 | 函数单测 | `backend-go/internal/topicgraph/service/story_finger
--- 尾部 513 字节 ---
赖向量单独判定（红线 14），故不新增「距离更近即合并」的用例。
- **话题（PersistentTopic）生命周期用例**：候选门槛、手动建话题、归档三条红线本 change **不碰**，不新增相关用例（红线 2/3/7 的既有用例作回归网）。
- **跨期去重口径用例**：重叠率 0.6 / 新 feed / `is_development` 三条判据本 change **不改**，只补一条「跨天延续标 `is_development` 后不被降级」的用例；不重写既有去重用例。
