# 真实样本效果验证（E1；E2/E3 待用户看完 E1 后决定）

> 用户授权（2026-10-19）：先跑 E1 一例看效果再决定是否继续。预算上限 3 发现+3 研究、单研究 40 轮、外搜≤9、网页≤15。
> 环境：隔离验收库 `syntopica_e1`（生产库 `pg_dump` 只读克隆，板块 15220「全球科技巨头动态」在克隆库开启 enrichment_enabled，生产库零写入）；独立后端实例 :5101；真实 LLM（airouter 路由）+ 真实 Bocha 配置。

## E1：AI 基建发债是否挤压信用市场（候选 18）

原计划样本 article149928（政治资金）不在任何板块材料中（执行前复查证实 test-cases 预判），改用同构主题候选：**「美国科技巨头密集发债近5000亿美元，IMF 警告债券收益率无序上涨」**（资金来源/去向、需外部数据、partial 出口——与原 E1 验证目标一致）。

### 执行记录（3 次研究 = 1 次失败 + 1 次修复验证 + 1 次终验）

| 轮 | 结果 | 轮次/取数 | tokens | 耗时 | 说明 |
| --- | --- | --- | --- | --- | --- |
| 发现 | 5 候选（含 brief） | — | 31,273 | 90s | question-v1 brief 全部生成：unknown 具体、verification 指向真实数据源、source_ids 冻结 n1~n22 |
| 研究#1 | ❌ abandoned（stop=compose） | 23/8 | 1,726,604 | 642s | **发现真缺口**：compose prompt 未告知 answer 契约——模型先把 `unanswered` 写成字符串，回注后干脆丢字段，三连拒 |
| 研究#2 | ✅ 报告 22 | 11/6 | 695,969 | 294s | 修复（system prompt question-v1 增补显式 answer JSON 形状）后通过；**发现低危缺口**：附录 gap 混入他候选（美光）冻结缺口（批次快照共享） |
| 研究#3 | ✅ 报告 23（终验） | 10/5 | 486,431 | 196s | gap 过滤修复后终验 |

### 报告 23 核对（通过判据逐项）

- **answer**：partial；reason 诚实（"企业发债规模、科技行业细分数据缺失，无法验证个体风险与发行量"）；unanswered 3 条具体（官方库无公司债发行量序列、高收益 OAS 取数失败、科技债 CDS 无官方日度序列）。
- **先读门**：n19/n20（本地冻结）先读后才放行 FRED 取数（text_calls/read 顺序符合 read-first）。
- **外搜/网页**：0 次（模型判断本地+数值源足够；预算未动，Bocha 零调用）。合法（外搜是可选补证）。
- **引用可定位**：[[source:]] 引用 0 条（正文用 data/calc/news 引用）→ 0/0，N/A 通过；data/calc 引用全部可解析（悬空 0）。
- **无「没取到=没发生」**：未出现；缺口如实进 unanswered 与附录。
- **旧路径回归**：研究全程 airouter 留痕（45 次 research 调用 + 5 次 compose + 1 次 detect，session_id 串联完整可重建）。
- **失败进展保留**：研究#1 的 progress 行 superseded 保留可查（stop_reason=compose，错误信息完整）。

### 由此修复的代码（已进 change）

1. `signal_compose.go`：`signalComposeAnswerPrompt`（question-v1 system prompt 增补：answer 必填、unanswered 是字符串数组、三枚举语义）+ `signalComposeSystemPromptOf` 按协议路由；旧协议逐字不变。测试 `TestQuestionComposePromptCarriesAnswerContract`。
2. `signal_compose.go`：`isForeignCandidateGap`（附录/prompt 过滤他候选冻结缺口）；测试 `TestQuestionReportScopesFreezeGapsToCandidate`。

### 成本汇总（E1 全部）

发现+研究共 4 个 session、52 次 LLM 调用、2.94M tokens、约 20 分钟。其中 2 次为发现缺陷后的修复验证轮（若一次通过约 1.2M tokens / 5 分钟）。

## E2/E3

未执行——等用户看 E1 证据后决定（授权口径：先跑一例看效果）。
