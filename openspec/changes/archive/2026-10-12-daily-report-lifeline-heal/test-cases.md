# Test Cases: daily-report-lifeline-heal

> complex 档测试设计。主 spec delta：`specs/data-enrichment/spec.md`（MODIFIED「循环 A 触发与汇总算法」，新增每日补齐触发契约 + 5 个 Scenario）。

## ⓪ 继承与调整（改契约 M：data-enrichment「循环 A 触发与汇总算法」）

`bash scripts/harness/test-assets.sh data-enrichment` 反查主 spec 该 Requirement 下 4 个旧 Scenario，处置如下：

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 每 period 独立汇总 | 保留不变（MODIFIED 版未改此节拍） | `internal/dataenrichment` 既有 lifeline refresh 测试 | 不动，作回归守护（task 3.1） |
| 历史周期可翻 | 保留不变 | 同上 | 不动 |
| 检查自愈补遗漏 period | 保留不变（`HealMissing` 原语义未动，容错版为新增函数） | `scheduler_jobs_test.go` 既有用例 | 不动 |
| 用户手动首次生成缺失 period | 保留不变（本 change 不触及 regenerate API） | handler 层既有用例 | 不动 |

旧测试全部保留且契约未破坏——本 change 是纯增量触发路径，无旧断言需改写。

## 主链路故事

**故事**：月报断供后的一夜——日报照常生成，尾部自愈把月/年缺口补上，第二天用户打开泳道趋势区看到最新月报。

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 日报 job 成功完成当日报告（LLM 通路健康） | 日报成功后补齐月报缺口 (GIVEN) | job 返回 success 且 JobResult.Data 带 `lifeline_heal.triggered=true` | job 单测 | task 2.3 |
| 2 | 尾部 heal 轮启动，扫 month 粒度：topic A 有当月素材无归档行 | 日报成功后补齐月报缺口 (WHEN/THEN) | 补生成 2026-10 行；日报 job 完成状态不因 heal 延迟（异步断言：job 先返回） | 函数单测（mock LLM） | task 1.1/1.2 |
| 3 | 扫 year 粒度：话题 B 十月激活、无 2026 年行、1-10 月有素材 | 年报年中新增话题由每日补齐覆盖 | 补生成 2026 year 行 | 函数单测（mock LLM） | task 1.1/1.2 |
| 4 | 月初 03:30 monthly job 曾被 model_unhealthy 跳过（当月全缺） | 月初定时触发被跳过后由每日补齐兜底 | 当日 21:00 日报成功后当月缺口全补，不等下月 | job 层触发断言 + 函数层缺行即补组合覆盖 | task 2.3 + 1.2 |
| 5 | heal 轮中 topic C 的 LLM 调用失败 | 补齐失败顺延不丢 | C 记 failed，A/B 照常补；次日轮只重试 C（A/B 行不被重写） | 函数单测（mock LLM 失败注入） | task 1.2 |
| 6 | 次日日报成功，全部归档行已齐 | 稳态零成本 | 扫描零缺口，mock LLM 调用计数 = 0 | 函数单测 | task 1.2 |

## 变体走查（五组固定清单）

- **输入**：
  - 空串/纯空白/纯分隔符：素材行由 SQL 拼接生成，无自由文本输入面——不适用（划除，无用户输入路径）。
  - 单 token/大小写/特殊字符/超长：素材为日期+标签+标题拼接，超长由既有 ReadSections LIMIT 5 标题约束——不适用（划除，非本 change 变更面）。
- **前置**：
  - 空集（无 active 话题）：heal 轮零 topic，直接返回 healed=0/failed=0，无 LLM 调用——函数单测覆盖（task 1.2 补充用例）。
  - 单元素（仅 1 话题缺 1 行）：正常补齐——task 1.2 已含。
  - 重复（已存在行的 topic）：不重写（Upsert 语义 + heal 扫描排除已有行）——task 1.2 已含。
  - 越界引用（素材引用已归档话题 archived）：lister 只取 active，archived 不补——函数单测 `TestHealMissingTolerant_TopicNotListedNotHealed`（review R-1 已落地）。
  - 部分满足（month 齐 year 缺）：两轮独立，year 轮单独补——task 1.2 已含（步骤 3）。
- **时间窗口**：
  - 边界两端（当期算不算）：period 上限 = 当前月/年（`HealMissing` 既有「up to current period, no future」），当日素材属当期——函数单测 `TestHealMissingTolerant_MonthBoundaryDates`（review R-1 已落地：跨月切换夜 23:59/00:00 分属两期各自成行，period 归属沿用既有 ParsePeriodRange，不新写时间逻辑）。
  - 空窗口（无任何素材的 topic）：「有 section 素材但无行」判定天然排除——不补（函数单测 `TestHealMissingTolerant_NoMaterialNoHeal`，review R-1 已落地零调用负向断言）。
  - 跨窗口（历史缺口跨多个月，如 2026-09+2026-10 双缺口）：双向回填逐期补——函数单测（task 1.2 补充用例）。
  - 归一化：period 格式由 `FormatMonth/FormatYear` 统一产出，无自由输入——不适用（划除）。
- **幂等**：
  - 重复执行（连续两晚 heal 轮）：第二晚零调用（稳态零成本）——task 1.2 步骤 6。
  - 部分失败重试：只重试仍缺的 (topic, period)——task 1.2 步骤 5。
  - 并发：单 goroutine 顺序执行，不声称线程安全，无并发用例（划除，不适用——设计决策①）。
- **可用性（UI 必检前三）**：本 change 无 UI 面（ui-impact: none）——整组划除。

## 效果核对（④：效果依赖断言外因素）

- **触发原因**：月报断供根因是月初定时触发被 model_unhealthy 跳过且无重试（scheduler_tasks 实证）；本 change 的效果 = 断供窗口缩到一天。
- **方法**：单测层用 mock LLM 调用计数断言稳态零调用与缺口收敛；真库效果靠部署后观察。
- **量化结果（部署后人工核对，task 7.5）**：首晚日志 healed 计数应 ≈ 历史缺口数（2026-09 期 ~55 行 + 2026-10 期 ~59 行 + year 空洞）；一周后 `topic_lifeline_context` 中 2026-10 月期行数 = active 话题数（59）。
- **结论**：若一周后月期行数 < active 数，检查 heal 失败日志（failed 计数持续 > 0 的 topic 需人工看素材异常）。

## 白盒附加（complex 档）

容错补齐函数分支表：

| 分支 | 输入/边界值 | 期望 | 用例落点 |
| --- | --- | --- | --- |
| lister 失败 | DB 查询 active topics 出错 | 返回 error，零 LLM 调用 | task 1.2 |
| 单 topic 读取 sections 失败 | topic #3 读素材 err | failed++，继续 #4 | task 1.2 |
| 单 period LLM 失败 | LLM 返回 err | failed++，同 topic 下一 period 继续 | task 1.2 步骤 5 |
| 零缺口 | 全部行已存在 | healed=0/failed=0，LLM 计数 0 | task 1.2 步骤 6 |
| period 上界 | 素材日期晚于当前期（时钟回拨场景） | 不补未来期（既有 ParsePeriodRange 约束） | task 1.2 边界用例 |
| 混合缺口 | month 齐/year 缺 × topic 交叉 | 两轮独立计数正确 | task 1.2 |
| heal goroutine panic | 注入 panic | recover 捕获记日志，日报 job 不受影响 | task 2.3 |
| 日报主流程失败 | 生成返回 err | heal 不触发 | task 2.3 |

不适用划除：week 粒度分支（设计 Non-Goal，weekly 禁用）；并发分支（不声称线程安全）。
