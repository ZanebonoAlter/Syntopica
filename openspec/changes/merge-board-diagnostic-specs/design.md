# Design: 版块诊断/研究两族合并

## D1 为什么不是 5 → 1

名字前缀相同不等于功能族相同。判断依据是**改动连带关系**：改 A 的时候是否几乎必然要动 B。

| 组合 | 改动连带 | 结论 |
|---|---|---|
| health-checkup ↔ direction-check | 都在判断「质量合不合格」：体检算贴合度/分诊信号，方向校验算 cosine 是否够。调阈值、加信号、改分诊闸门时两边一起动 | **同族** |
| signal-reports ↔ level-analysis | 都是「人工触发 → 异步产出 → 可核查报告」，共享候选/触发/幂等/失败恢复/工作台 IA 这套骨架 | **同族** |
| health ↔ research | 一个看内部构成质量，一个对外做研究；共用词只有「版块」 | 不同族 |
| 任何 ↔ radar-pipeline-status | radar 是页面状态块的展示契约（红/绿/文案/入口），后端分析改不动它 | 不同族 |

1579 行也超过 design D5 的 1500 红线，合并前就该拆。

## D2 direction-check 归诊断而不是匹配

这条最容易被质疑，先讲清楚：

`board-direction-check` 校验的是**匹配结果的质量**（tag identity embedding 与 board embedding 的 cosine < 阈值 → 标 `direction_mismatch`），消费方是日报（排除 mismatch 标签）与匹配详情 API。

但它**不参与匹配决策**——匹配已经用 `composite_hit/direct_hit/hit_rate/max_sim/weighted` 判完了，方向校验是事后质检。质检归诊断，决策归匹配。

**已知代价（R-2）**：`tag-matching`（上一族，已归档）里有 26 处 direction 内容，其中一句明确写着「执行方向校验（与间接匹配规则相同的三层 direction check：全库 gap 豁免 → 绝对门 → 相对门**（详见 board-direction-check）**）」。本次合并后这是死引用。同理还有 3 条消费方契约（日报排除 direction_mismatch、文章 API 过滤、match detail 返回 direction_sim）在 `tag-matching` 与 `board-diagnostics` 两侧各有落点。

**处置**：本 change 不改 `tag-matching`（改它的 Requirement 正文属语义修改，且会破坏已归档族的守恒）。在 `board-diagnostics` 的方向校验 Requirement 里保留完整算法原文，作为唯一权威；死引用修复登记为后续 change（tasks 7.5）。这是「分批合并」的固有代价——跨族引用要等相邻族都收口后再统一扫一遍。

## D3 族边界与命名

| 族 | 命名理由 | 吸收 |
|---|---|---|
| `board-diagnostics` | 「诊断」= 判定质量、产出信号/评分/分诊建议，不改变版块本身 | board-health-checkup、board-direction-check |
| `board-research` | 「研究」= 人工触发、异步产出、可核查报告，成本受用户控制 | board-signal-reports、board-level-analysis |
| `radar-pipeline-status` | 保留原名：它是**页面契约**（状态块字段/入口/二次确认），与后端能力不同层 | — |

`board-diagnostics` 与 `board-research` 的边界判据：**诊断自己算结论（评分/信号/标记），研究产出待用户判断的材料（简报/调查/报告）**。前者不许写版块，后者不许自动触发。

## D4 排序与分类

沿用 `merge-tag-spec-family` 的两段式：

1. business（行为契约）在前，保持来源顺序
2. impl（施工记录）在后，每条标 `<!-- kind: impl -->`

分类由 glm-5.4 批量判定 + 人工校验边界例。判据与上族一致：

| 类型 | 判据 | 例 |
|---|---|---|
| business | 约束系统对外产出什么 | 体检快照字段、方向阈值默认值、报告必须标注数据时效 |
| impl | 约束代码怎么组织 / 已完成的施工 | 包结构、缓存实现、测试迁移、一次性存量清理 |

**本族预判的边界例**（review 会重点看）：

- `DirectionSimThreshold configurable` / `DirectionRelativeMargin configurable` → business（配置项是对外可调契约，改默认值影响日报结果）
- `Direction check data loading` → impl（数据加载方式，属实现）
- `旧论文式版块报告兼容` / `旧研究历史只读访问` → business（向后兼容是对外承诺，用户还能访问）
- `简报与调查异步任务可区分` → business（API 契约：两个任务类型可区分）

## D5 归档机制（复用已验证流程）

1. `.openspec.yaml` 声明 `retire_capabilities: true`——否则空 spec 校验失败会整次中止
2. 归档前摘掉 3 个被退役 spec 的 `<!-- domain: -->` 注释——openspec 拒绝删除「含无法安全处理内容」的文件
3. 归档后给 2 个新族补 marker：`board-diagnostics` → `semantic-board`；`board-research` → `semantic-board, data-enrichment`（研究产出走数据增强链）

## D6 已知损失

指向 **4 个**被删 capability 的 archived delta 会落 `domain-history.sh` 第 6 段；只碰过这 4 个 capability 的 history change 会从反查里消失（与 `merge-tag-spec-family` 同一损失模式，根治需「已退役 capability → 族」别名表，仍未做）。

`board-health-checkup` 被子孙 change 反复改过（fix-board-health-llm-misread / board-health-triage-advice / fix-board-health-triage-degrade / fix-board-health-magnet-signal），`board-direction-check` 亦然（fix-digest-tag-leak / fix-board-direction-gap-exempt / fix-board-ghost-false-positive）——这些 change 若只碰过被删 capability 就会从反查里消失，具体条数在归档后由 `domain-history.sh` 第 6 段给出（tasks 5.1 按实测记数，不预填）。

`board-signal-reports` 原 marker 含 `data-enrichment`，合并后由 `board-research` 继承——数据增强域的人仍能查到研究族。
