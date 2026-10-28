# 人工效果核对（tasks 5.3）— 真实结算产物抽查

> 核对时间：2026-10-28。数据源：本地真库 `topic_lane_snapshots`（该 change 上线后已跑了 10-25/26/27 三个日报日，全部为新单版结算产物）。
> 只读 SQL 抽查，无写入。判据来自 tasks 5.3：阶段带驱动语义 / 事件仅转折标记 / 无逐日罗列 / 无预测 / 量级引用准确。

## 1. 样本与总体

| 指标 | 结果 |
| --- | --- |
| 结算后快照总数 | 46（其中 38 条为上线后新写，`updated_at > 2026-10-24`） |
| 新写 `rolling_detail` 为空串 | 38 / 38 ✓（长版停止产出生效） |
| 字数区间 | 38/38 均 ≥ 200 rune（全部顶到 `laneSnapshotMaxRunes=200` 上限） |
| 以句末标点收尾 | **1 / 38**（37 条在字中间被切断） |

抽样 8 条（板块 15220 / 3030 / 2272 各若干）逐条对照讨论判据：

| 判据 | 结果 | 证据（节选） |
| --- | --- | --- |
| 阶段划分 + 日期锚 | ✓ | 「10月14日至17日为发酵期」「第一阶段（10-13至10-17）」「近14天…可划为三个阶段」 |
| 每阶段驱动/转折语义 | ✓ | 「以AI交易退潮为驱动」「Skill加载仅读description字段的机制缺陷被点出…构成转折」 |
| 事件仅作转折标记、不展开 | ✓ | 未见按日复述事件全文；均为「起点/转折/驱动」式引用 |
| 无逐日罗列 | ✓ | 无逐日日期条目串（对比旧长版实测逐日复述） |
| 无未来预测 | ✓ | 全部为「已发生走向」陈述 |
| 量级引用准确 | ✓ | 抽查 topic 1011「16日达到4条的峰值」→ 真库 10-16 thread=4 ✓；topic 1010「单日1条的低位」「22日升至2条」→ 真库 10-18/19=1、10-22=2 ✓ |
| **句子完整性** | **✗** | 「…美国土安全部指示联邦民事机构停用并移除DeepSeek」（截断）、「…SK」（截断）、「…OpenAI营收预」（截断） |

## 2. 结论与处置

- 内容判据（叙事形态、禁项、量级）**全部通过**；形态判据「读完一句完整的话」**不通过**：200 字预算装不下「阶段→驱动→落点→量级」四段，模型稳定写到 250-300 字后被机械截断，读者在泳道趋势区看到半截话。这是「机械截断保护」从兜底变成常态路径。
- 处置（按 5.3 「不合格回改 prompt 而非放宽判据」+ 用户 2026-10-28 裁决）：
  1. prompt 加篇幅硬约束：总长（含标点）≤200 字、必须以句号收尾、写不下压缩/合并阶段；
  2. 解析端 `clampLaneSnapshotRunes`：超限优先在窗口内最后一个句末标点（`。！？…`）收口，标点过早（< 60% 预算）或整段无标点时退回 rune 硬切；
  3. 只改上述两处，**不放宽 200 字口径**、不改 spec 字数区间。
- 复验计划：下个日报日（2026-10-29 凌晨）结算后重跑同一组 SQL——期望「以句末标点收尾」≥ 90%、字数仍 ≤200；合格后把 tasks 5.3 勾选并归档本 change。

## 3. 复验用 SQL（只读）

```sql
-- 句末收尾比例与顶格比例
select count(*) n,
       count(*) filter (where right(rolling_summary,1) in ('。','！','？','…')) as ends_clean,
       count(*) filter (where char_length(rolling_summary) >= 200) as at_cap
from topic_lane_snapshots
where updated_at > '<复验日 00:00>';

-- 量级引用抽查（换 topic id / 日期）
select s.persistent_topic_id, r.period_date, count(t.id) threads
from daily_report_sections s
join board_daily_reports r on r.id = s.report_id
left join daily_report_threads t on t.section_id = s.id
where s.persistent_topic_id in (1010, 1011)
  and r.period_date between '2026-10-12' and '2026-10-28'
group by 1, 2 order by 1, 2;
```
