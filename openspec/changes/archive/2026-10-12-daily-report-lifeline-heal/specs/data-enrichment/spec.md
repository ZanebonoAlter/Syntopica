## MODIFIED Requirements

### Requirement: 循环 A 触发与汇总算法

循环 A SHALL 定时触发（week 每周、month 每月、year 每年），**每个周期各产一条独立 period 行（新周期不覆盖旧周期）**。SHALL 内置检查自愈机制（扫描缺失历史 period 的 topic，按 period 逐个补），SHALL 支持手动重生成任意 period，SHALL 归档清理超期行（week>8 周、month>12 月）。

**每日补齐触发**：month 与 year 粒度的检查自愈 SHALL 额外在每日日报成功生成后异步触发一次（挂在日报 job 尾部，不改变日报自身的生成编排契约）。补齐 SHALL NOT 阻塞或延迟日报 job 的完成状态；单话题补齐失败 SHALL NOT 中断其他话题的补齐；当日补齐未完成（如模型层不健康、部分失败）SHALL 顺延至次日日报成功后重试，SHALL NOT 丢弃缺口。归档行齐全时补齐触发 SHALL NOT 发起任何 LLM 调用（稳态零成本）；补齐仅针对「有 section 素材但无归档行」的 (topic, period)，SHALL NOT 重写已存在的归档行。week 粒度 SHALL NOT 启用每日补齐（维持现有禁用状态）。

汇总算法：**每个 period 独立汇总成一条**——读「该 period 范围内的 sections」一次性汇总，SHALL NOT 与旧汇总合并覆盖。`all`（`period='all'`）为例外，滚动单行，读全部历史 sections + 旧 all 汇总增量合并。各 period SHALL 平行独立维护，不搞 week→month→year 层层金字塔合并（避免误差累积）。

#### Scenario: 每 period 独立汇总

- **WHEN** 每月定时刷新某 topic 的 month（period=2026-06）
- **THEN** SHALL 读 2026-06 范围内 sections 独立汇总成一条，SHALL NOT 覆盖 2026-05 的 month 汇总

#### Scenario: 历史周期可翻

- **WHEN** 用户在前端按周期筛选查看 2026-05 的 month
- **THEN** 系统 SHALL 返回该 period 独立存的汇总行（未被新周期合并覆盖）

#### Scenario: 检查自愈补遗漏 period

- **WHEN** 某 topic 从未生成过 2026-05 的 month 汇总
- **THEN** 检查自愈 SHALL 补生成该 period，`as_of_date` 顺序推进

#### Scenario: 用户手动首次生成缺失 period

- **WHEN** 用户在前端①选一个该 topic 从未生成过的 period（如 2026-W25，表1无对应行）
- **THEN** 系统 SHALL 允许手动触发该 period 的汇总（调 `POST .../contexts/:granularity/regenerate?period=...`），通过 Upsert 首次插入该 period 行，SHALL NOT 要求用户等待检查自愈排队补；生成后该 period 立即出现在可翻阅列表中。此入口与「翻历史」并列，覆盖「已存在重算」和「从未生成首次生成」两种情况

#### Scenario: 日报成功后补齐月报缺口

- **GIVEN** 某 active topic 存在当月 month 归档缺口（有 section 素材但 `topic_lifeline_context` 无该 period 行），且当日日报已成功生成
- **WHEN** 日报 job 尾部的每日补齐触发执行
- **THEN** 系统 SHALL 补生成该 period 的 month 归档行，且日报 job 的完成状态 SHALL NOT 因补齐执行而延迟

#### Scenario: 月初定时触发被跳过后由每日补齐兜底

- **GIVEN** `lifeline_monthly` 在月初 03:30 因模型层不健康被跳过，当月归档行全缺
- **WHEN** 当日 21:00 日报成功生成（日报成功即隐含模型层恢复健康）
- **THEN** 每日补齐 SHALL 在当日把当月缺口补上，SHALL NOT 等待下月定时触发

#### Scenario: 补齐失败顺延不丢

- **GIVEN** 每日补齐执行中模型层再次不健康，当日仅补齐部分 topic
- **WHEN** 次日日报成功生成
- **THEN** 每日补齐 SHALL 重试仍缺失的 (topic, period)，已补齐的行 SHALL NOT 被重写

#### Scenario: 稳态零成本

- **GIVEN** 所有 active topic 的当期 month/year 归档行齐全
- **WHEN** 日报成功生成后每日补齐触发执行
- **THEN** 扫描 SHALL 发现零缺口，SHALL NOT 发起任何 LLM 调用

#### Scenario: 年报年中新增话题由每日补齐覆盖

- **GIVEN** 2026 年 10 月某话题被激活（active），此前无 2026 year 归档行，且 2026 年 1-10 月有 section 素材
- **WHEN** 次日日报成功生成后每日补齐触发
- **THEN** 系统 SHALL 补生成该 topic 的 2026 year 归档行，SHALL NOT 等待次年 1 月 1 日的年度定时触发
