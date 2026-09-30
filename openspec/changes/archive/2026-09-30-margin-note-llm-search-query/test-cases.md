# Test Cases: 页边注问答 LLM 自由选词搜索

用户故事：读者在台湾局势段落落锚批注，问「蓝白合是什么」——系统先让 LLM 看着问题与划选原文自由生成搜索词（而非机械截断原文），联网检索概念解释，回答带可点网络来源；追问「2024 大选对两岸关系什么影响」时搜索词随问题变化；联网链路任何一步失败都静默回退，问答永不阻断。

## ⓪ 继承与调整（改契约：MODIFIED `daily-report-margin-notes`·页边注问答与引用）

旧 Scenario 处置（旧测试资产：`backend-go/internal/topicgraph/service/margin_notes_qa_test.go`、`handler/margin_notes_handler_test.go`，来自 2026-09-26-daily-report-margin-notes）：

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 提问获得带引用回答 | 保留不变（回答契约不变） | margin_notes_qa_test.go（open_notebook 路由断言） | 补 stub `marginNoteQueryGenFn` 后重跑；断言不动 |
| 无文章上下文时明示 | 保留不变 | margin_notes_qa_test.go | 补 stub 后重跑 |
| 生成失败可重试不丢问题 | 保留（指回答调用；query 生成失败不报错、走回退） | 前端组件 + handler 测试 | 零改动重跑（前端零改动） |
| 追问 | 保留不变 | margin_notes_qa_test.go | 补 stub 后重跑 |
| 联网补强回答 | 保留（白名单池从单 query 结果扩为合并池，语义等价） | margin_notes_qa_test.go（联网 5 用例：拼 prompt/失败降级/白名单剔除/不标纯模型/均空才标） | **重点调整**：每用例显式 stub query 生成（防「未 stub 走默认 airouter 失败→静默回退锚文本」的意外绿）；白名单剔除用例改从合并池断言 |
| 搜索失败静默降级 | 保留（SearXNG 侧失败语义不变） | margin_notes_qa_test.go（失败降级用例） | stub query 生成成功 + 搜索钩子报错 → 断言回答照常 |
| 编造 URL 白名单剔除 | 保留（「结果集」→「合并结果池」） | margin_notes_qa_test.go | 输入改为多 query 合并池场景断言 |
| 纯模型知识标注口径 | 保留不变 | margin_notes_qa_test.go | 补 stub 后重跑 |
| 其余 Requirement（锚定/术语/零回归/管理页/空态窄屏） | 不在本 change 辖区，零改动 | 各自测试文件 | 仅回归跑，不动断言 |

## 主链路表（节拍）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 读者在批注卡提问「蓝白合是什么」 | 提问获得带引用回答 | query 生成调用发出：输入含锚文本+问题+线索上下文，输出 1~3 条精炼中文搜索词（贴问题语义，非原文截断） | 函数单测（stub 钩子） | margin_notes_qa_test.go `TestMarginNoteQueryGen` |
| 2 | 生成的 query 并行调 SearXNG | 搜索词随问题语义变化 | 各 query 独立超时；URL 去重保序合并，池子截前 5 进 prompt | 函数单测 | margin_notes_qa_test.go `TestMergeSearchResults` |
| 3 | LLM 回答并引用网络来源 | 联网补强回答 | 回答尾部网络来源 chips；web_sources 全部落在合并池白名单内 | 函数单测 | margin_notes_qa_test.go（白名单用例，合并池场景） |
| 4 | 读者追问「2024 大选对两岸关系什么影响」 | 搜索词随问题语义变化 | 第二轮 query 生成输出与第一轮不同且贴新问题（stub 两次不同返回，断言搜索入参随之变化） | 函数单测 | margin_notes_qa_test.go |
| 5 | query 生成调用失败/超时/输出空 | query 生成失败回退划词搜索 | 静默回退锚文本（截前 80 runes）单 query 搜索，问答照常无报错 | 函数单测 | margin_notes_qa_test.go `TestAskMarginNoteDegrade` |
| 6 | SearXNG 全部 query 失败/未配置 | 搜索失败静默降级 | 无联网参考块，回答照常、无网络来源行、无错误提示；未配置时 query 生成钩子零调用 | 函数单测 | margin_notes_qa_test.go `TestAskMarginNoteDegrade` |
| 7 | 回答落库（cited_web_sources jsonb） | 纯模型知识标注口径 | 落库契约不变（空写 `[]`）；本地引用与网络来源均空才标 | 既有 repo/handler 测试 | margin_notes_repository_test.go（零改动回归） |
| 8 | 真机：同一批注先后两问 | （效果核对，见下） | ai_call_logs 两条 `daily_report.margin_note_search_query` 的 queries 互不相同且贴各问题语义 | 人工走查 | tasks 4.3（psql 留证据） |

## 变体走查（五组固定清单）

| 组 | 变体 | 答案 |
| --- | --- | --- |
| 输入 | query 生成输出：空串/纯空白（全角、tab）/纯分隔符 | 逐条过滤后为空 → 整体视为生成空 → 回退锚文本（同失败路径） |
| 输入 | 单 token（单条 query） | 合法，1 条也走搜索（下限 1） |
| 输入 | 返回 5 条（超上限 3） | 截前 3，多余丢弃（Warn 不抛） |
| 输入 | 单条超长 query | 截 ≤60 runes 再搜 |
| 输入 | 大小写/特殊字符 | query 原样透传 SearXNG（`BuildQuery` 只做 trim+截断）；LLM 提示要求中文词组，不做二次过滤 |
| 前置 | endpoint 未配置 / enabled=false / 配置但空串 | 预检直接跳过整个联网链路，query 生成钩子零调用（省 LLM 调用） |
| 前置 | 锚文本超长（>200 runes） | 进 query 生成 prompt 前截 200；回退路径搜索时截 80（现状规则） |
| 前置 | 线索标题/摘要缺失 | prompt 省略该块，query 生成照常（无 thread 批注） |
| 时间窗口 | —— | 不适用（无时间窗口语义），划除 |
| 幂等 | 重复执行/部分失败重试/并发 | 问答为追加轮次无幂等键（现状契约）；每轮联网链路独立走降级树，重试即整轮重试（前端既有按钮）；并发追问走 airouter 信号量排队（design Risks 已记）——不适用自动断言，划除留痕 |
| 可用性 | UI 必检前三 | 不适用（前端零改动，无 UI 状态变化），划除 |

## 效果核对（效果依赖 LLM 行为：query 选词质量）

- **触发原因**：搜索词质量由 LLM 决定，单测 stub 无法证明真实选词贴问题语义。
- **方法**：真机走查（tasks 4.3）——SearXNG 已配置环境，同一批注先后问两个语义不同问题，psql 查 `ai_call_logs`（operation=`daily_report.margin_note_search_query`）。
- **量化结果**：两条记录 queries 无重叠且各自包含问题核心实体（如「蓝白合/政党联盟」vs「2024台湾大选/两岸关系」）；对比改造前（固定锚文本 query，两问完全相同）。
- **结论**：写回 tasks 4.3（人工验收措辞），不满足则回 D6 调 prompt 再走查。

## 白盒附加（complex 档：D5 降级决策树分支表）

| # | 分支条件 | 期望 | 落点 |
| --- | --- | --- | --- |
| B1 | endpoint 未配置/禁用 | 跳过联网链路，query 生成钩子**零调用**，回答无联网参考块 | TestAskMarginNoteDegrade |
| B2 | query 生成成功（1~3 条）× 搜索全成功 | 合并去重保序 top5 进 prompt | TestAskMarginNoteDegrade |
| B3 | 生成成功 × 部分搜索失败 | 成功 query 的结果照用，失败 query 静默丢弃 | TestAskMarginNoteDegrade |
| B4 | 生成成功 × 全部搜索失败 | 无联网参考块；**不**再回退锚文本（SearXNG 同轮已挂） | TestAskMarginNoteDegrade |
| B5 | 生成成功 × 各 query 结果全空 | 同 B4 | TestAskMarginNoteDegrade |
| B6 | 生成调用失败/超时 | 静默回退锚文本（截 80 runes）单 query 搜索 | TestAskMarginNoteDegrade |
| B7 | 生成输出空/全空白/解析失败 | 同 B6 | TestMarginNoteQueryGen |
| B8 | 回退锚文本后搜索失败/空 | 无联网参考块（双保险耗尽，静默终态） | TestAskMarginNoteDegrade |
| 边界 | query 数 1/3/4 | 1 与 3 合法；4 截前 3 | TestMarginNoteQueryGen |
| 边界 | 合并池长度 5/6 | 截前 5（`marginNoteSearxngResultLimit` 不变） | TestMergeSearchResults |
| 边界 | 同 URL 跨 query 重复/单 query 内重复 | trim 精确匹配去重，首次出现位次保序 | TestMergeSearchResults |
| 边界 | 锚文本 200 runes 边界 | prompt 截 200；回退搜索截 80 | TestMarginNoteQueryGen |
