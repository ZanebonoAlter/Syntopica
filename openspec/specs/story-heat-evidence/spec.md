# story-heat-evidence Specification

## Purpose
定义「热度」为**确定性函数**，输入是原始观察证据（哪篇文章、哪个来源、什么时候），输出是可直接比较、可回算、可解释的数值。热度 SHALL 不从日报派生——日报是热度的消费者，不是生产者。这样改提示词、改分桶策略、改去重规则都不会改变历史热度曲线。术语与图谱见 [`../data-map.md`](../data-map.md) §6。

## Requirements

### Requirement: 热度证据数据模型
系统 SHALL 使用 `story_signals` 表承载热度证据。每行 SHALL 表示「一个参与者在某个信源时间讨论过某条事件线」，主键为 `(story_id, article_id)`。字段至少包含：`story_id`、`article_id`、`feed_id`、`participant_key`、`kind`（`editorial` / `signal`）、`observed_at`。

`observed_at` SHALL 取 `articles.pub_date`（信源时间），SHALL NOT 取采集时间、日报生成时间或入库时间。`pub_date` 缺失时 MAY 回退到 `created_at`，但该行 SHALL 标记 `observed_at_source=fallback` 以便区分。

写入 SHALL 幂等：同一 `(story_id, article_id)` 重复写入 SHALL 收敛为一行（保留最新 `observed_at`），SHALL NOT 产生重复证据。

#### Scenario: 信源时间优先
- **WHEN** 某报道 `pub_date` 为 11-05 09:00，实际采集于 11-06 02:00
- **THEN** 其 signal 的 `observed_at` SHALL 为 11-05 09:00

#### Scenario: 重复写入收敛
- **WHEN** 同一事件线与文章因重新归组被写入两次
- **THEN** `story_signals` SHALL 仍只有一行，`observed_at` 取最新值

#### Scenario: 归属变更时证据迁移
- **WHEN** 某文章从事件线 A 改判到事件线 B
- **THEN** A 的对应 signal SHALL 删除、B 的 signal SHALL 写入，两侧热度 SHALL 在下次计算时反映变更

### Requirement: 参与者身份与去重

> **适用范围（2026-11-19 补注，review R-1）**：本 Requirement 描述事件线热度的**历史口径**。事件线在线生产已随 change `retire-story-adopt-event-metrics` 硬停写（flow `daily-report.md` 红线 43）——不再产生新的 `board_stories` / `story_signals` / `story_id`，下列 SHALL 只适用于存量只读数据与只读标定（`cmd/calibrate-heat --dry-run`），**不描述任何现行生产行为**，也 SHALL NOT 被重新开启为写路径。内容身份去重口径本身的现行承接方是 `lane-heat-metric`「线索计数的内容身份去重口径」（簇内文章数复用同一键），归档顺序上本 change SHALL 先于 `retire-story-adopt-event-metrics`（后者 REMOVE 本 Requirement，反序会使本 MODIFIED 找不到目标而硬失败）。

`participant_key` SHALL 标识「一个独立参与方」，用于回答「有多少不同来源在说这件事」。参与者身份 SHALL 按**内容身份优先**判定：同一篇内容（`articles.link` 经规范化后相同）的多个 article 副本 SHALL 只贡献一个参与者；内容身份不可判定（link 为空或规范化后为空）时 SHALL 退化为 feed 粒度（`feed:<feed_id>`）。

link 规范化口径 SHALL 固定为：去首尾空白 → 去掉 `#` 之后的 fragment（**`#!` hashbang 除外**：沿用 `textutil.StripURLFragment` 既有语义保留，SPA 路由身份不被吞掉）→ 去掉 `?` 之后的 query → scheme 与 host 转小写（path 保持原样，不做大小写折叠）。该口径 SHALL 只用于参与者身份与热度计数，SHALL NOT 改写 `articles.link`，SHALL NOT 变更入库判重键 `(feed_id, link)`。

热度计算 SHALL 对每个参与者只计一次：同一参与者在窗口内多次报道同一事件线 SHALL NOT 叠加热度。

同一篇内容被多个 feed 抓取时 SHALL 只记一个参与者；不同内容的独立报道 SHALL 各记一个参与者，与 feed 数量无关。原「同一篇 article 被多个 feed 转载时各记一个参与者（第一版不识别转载）」的已知简化条目 SHALL 废止。

参与者计数的所有消费面（`story_signals`、`participant_count`、`story_heat`、`lane_heat`、雷达「N 个来源」、矩阵颜色分档）SHALL 使用同一份内容身份判定，SHALL NOT 出现某处按 feed 计、某处按内容计的混用。

#### Scenario: 同一 feed 多条不叠加
- **WHEN** feed X 在 48 小时内发布 5 条关于同一事件线的文章，且 5 条 link 规范化后相同（互为副本）或均不可判定
- **THEN** 该事件线的热度 SHALL 只按 1 个参与者计算
- **AND** 若 5 条 link 规范化后互不相同，则 SHALL 按 5 个独立参与者计——参与者粒度是内容身份，不是 feed（见本 Requirement 正文「与 feed 数量无关」）

#### Scenario: 不同 feed 各计一次
- **WHEN** 5 个不同 feed 各发布 1 条关于同一事件线的文章，且 5 条 link 规范化后互不相同
- **THEN** 该事件线的参与者数 SHALL 为 5

#### Scenario: 排除不可见证据
- **WHEN** 某 article 已被撤回、或被人为标记为不参与统计
- **THEN** 其 signal SHALL NOT 计入热度
- **AND** 仅因轮转出 feed 活跃窗口而 `archived=true` 的文章 SHALL NOT 被排除（归档不是撤回）

#### Scenario: 同链接多 feed 副本只算一个参与者
- **WHEN** 同一 URL 的文章被 feed A 与 feed B 各抓取一份（两行 `articles`、同标题同 `pub_date`），并都被归属到同一事件线
- **THEN** 该事件线的参与者数 SHALL 为 1
- **AND** 热度 SHALL 只按 1 个参与者的 `observed_at` 计算（示例：24h 半衰期下 12.5 小时后 SHALL 为 0.70，SHALL NOT 为 1.40）

#### Scenario: 规范化链接覆盖 fragment 与 query 差异
- **WHEN** 同一篇文章的副本 link 分别为 `https://example.com/p/1?utm=x` 与 `https://EXAMPLE.com/p/1#comments`
- **THEN** 两副本 SHALL 判为同一内容，只计 1 个参与者

#### Scenario: 链接缺失退化按 feed
- **WHEN** 某事件线的两篇证据文章 link 均为空
- **THEN** 参与者身份 SHALL 退化为 `feed:<feed_id>` 粒度，SHALL NOT 因内容身份不可判定而合并不同 feed

#### Scenario: 归档不影响参与者计入
- **WHEN** 参与者的文章已归档（轮转出 feed 活跃窗口）
- **THEN** 该参与者 SHALL 照常计入热度，SHALL NOT 因 `archived` 被过滤，参与者数亦 SHALL NOT 因 `archived` 减少

### Requirement: 热度公式与窗口
事件线热度 SHALL 按时间衰减求和计算：

```
heat(story, t) = Σ over participant p:  0.5 ^ (Δt_p / HALF_LIFE_HOURS)
其中 Δt_p = t − max(observed_at of p 在窗口内)
```

`HALF_LIFE_HOURS` SHALL 对事件线级取 24 小时，`WINDOW_HOURS` SHALL 取 48 小时。话题级热度 SHALL 为窗内该话题下所有事件线热度之和，半衰期 SHALL 取 72 小时，窗口 SHALL 取 14 天。

公式参数 SHALL 版本化（`rule_version`，初始 `heat-v1-48h-hl24h`），计算结果的存储 SHALL 带该版本号，使得参数调整后能区分新旧口径。

#### Scenario: 半衰期衰减
- **WHEN** 某参与者最后一次报道在 24 小时前，当前时间参与计算
- **THEN** 该参与者贡献 SHALL 为 0.5；48 小时前最后一次贡献 SHALL 为 0.25

#### Scenario: 窗口外不计入
- **WHEN** 某参与者的最后一次报道在 49 小时前
- **THEN** 该参与者 SHALL NOT 计入事件线的 48 小时热度

#### Scenario: 话题热度为事件线热度之和
- **WHEN** 某话题在窗内有 3 条事件线，热度分别为 2.5、1.0、0.5
- **THEN** 该话题热度 SHALL 为 4.0

### Requirement: 热度不从日报派生
热度计算 SHALL 只依赖 `story_signals`、`board_stories`、事件归属关系与 `articles.pub_date`，SHALL NOT 依赖 `daily_report_sections` 的条数、事件行数、分区数量、桶数量或任何 LLM 输出字段。

因此：新增/移除/降级事件、改变桶数 `k`、重述降级、调整日报 prompt SHALL NOT 改变未变更证据的热度值。

#### Scenario: 桶数变化不影响热度
- **WHEN** 同一版块同一天因分桶参数调整从 3 个分区变为 5 个分区，文章与信号未变
- **THEN** 事件线热度与话题热度 SHALL 保持不变

#### Scenario: 重述降级不影响热度
- **WHEN** 日报把某事件线的重述条目从正文降级为快讯
- **THEN** 该事件线的热度 SHALL 不变

#### Scenario: 事件线合并后热度合并
- **WHEN** 事件线 A 合并进事件线 B
- **THEN** B 的热度 SHALL 在下次计算时等于两者证据的并集去重结果

### Requirement: 可重算与落后源处理
因为热度是确定性函数，任一历史时刻的热度 SHALL 可重算且结果一致。系统 SHALL 在计算时考虑采集落后：某源超过 `max(3 × refresh_interval, 90 分钟)` 未成功采集时，该源相关的小时 SHALL 标记为不完整，并在源恢复后重算受影响的区间。

#### Scenario: 历史重算一致
- **WHEN** 对同一时刻 t 连续计算两次
- **THEN** 两次得到的热度值 SHALL 完全一致（不依赖随机数、模型或当前时间之外的输入）

#### Scenario: 落后源标记后重算
- **WHEN** feed X 在 11-05 03:00–09:00 采集失败，恢复后补抓到 11-05 的文章
- **THEN** 受影响区间的热度 SHALL 被重算，补抓文章按 `pub_date` 计入对应小时

### Requirement: 热度消费口径一致
所有对外展示的「热」SHALL 来自同一套热度值，SHALL NOT 各自用条数近似：

- 矩阵日格数字 SHALL 为当日该话题的事件线数，SHALL NOT 为事件数、分区数或桶峰值。
- 矩阵日格颜色 SHALL 表达当日该话题最热事件线的参与者数，分档 SHALL 有图例且 SHALL 提供带数字的 title/aria（颜色不得是唯一信息载体）。
- 雷达关注度 SHALL 由话题热度派生（可含四舍五入与分档），SHALL NOT 直接用分区数加权。
- 「今天可能升温」SHALL 基于事件线热度环比，SHALL NOT 基于分区数环比。

#### Scenario: 矩阵数字为事件线数
- **WHEN** 某话题当日 2 条事件线（分别有 3、7 条事件）
- **THEN** 格内数字 SHALL 为 2；颜色 SHALL 按参与者数 7 对应的档位渲染

#### Scenario: 无事件但有分区
- **WHEN** 某话题当日有分区但所有事件均为综合稿或未归组
- **THEN** 格内数字 SHALL 为 0 或按降级口径给出的上界，并 SHALL 提供「无可计数事件」的可读说明，SHALL NOT 显示空白

#### Scenario: 雷达排序与热度一致
- **WHEN** 版块 A 的话题热度为 12.0、版块 B 为 9.5
- **THEN** 雷达 SHALL 把 A 排在 B 之前，SHALL NOT 因 A 的分区数更少而反转
