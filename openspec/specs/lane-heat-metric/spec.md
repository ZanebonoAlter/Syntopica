<!-- domain: semantic-board -->

# lane-heat-metric Specification

## Purpose

定义事件线退役后的当天热度量纲：泳道热度 = 该泳道当天线索数按话题状态加权，版块关注度 = 各泳道热度之和。该量纲只表达「今天有多少线索」，不衰减、不跨天、不依赖任何事件身份，是雷达关注度分档、泳道行序与矩阵排序的唯一口径出处。

## Requirements

### Requirement: 泳道当天热度口径

系统 SHALL 以「该泳道当天线索数」为唯一输入计算泳道当天热度：活跃（active）泳道的当天线索数按权重 `1.0` 计，候选（candidate）泳道的当天线索数按权重 `lane_heat_candidate_weight` 计（默认 `0.5`）。

当天 SHALL 为 Asia/Shanghai（UTC+8）日历日，与日报 `period_date` 同一日界。线索数 SHALL 为该泳道在该日锚定 section 下的 thread 数；综合稿（`scope=composite`）的 thread SHALL NOT 计入线索数。

系统 SHALL NOT 把事件线数、事件数、section 数、主题桶数、参与者数、半衰期衰减或任何 LLM 输出作为热度输入。热度 SHALL 为确定性函数：同一份日报数据重复计算 SHALL 得到同一结果；任意历史日 SHALL 可回算（热度不落历史表）。

候选权重 SHALL 经 `ai_settings` 在线可调，有效值为 `[0,1]` 内有限数；缺项 SHALL 回退默认 `0.5`，坏存量值 SHALL 告警并回退默认，非法提交 SHALL 整体拒绝且不覆盖原值。候选权重 SHALL NOT 参与话题生命周期判定：candidate 连续命中与转正资格仍 SHALL 只由命中计数与转正门槛决定。

#### Scenario: 活跃与候选按不同权重计

- **WHEN** 某活跃泳道当天有 6 条线索，某候选泳道当天有 4 条线索，候选权重为 0.5
- **THEN** 活跃泳道当天热度 SHALL 为 6.0
- **AND** 候选泳道当天热度 SHALL 为 2.0

#### Scenario: 候选权重在线调整后立即生效

- **WHEN** 把候选权重从 0.5 调到 0.2 并重新读取同一泳道热度
- **THEN** 该候选泳道热度 SHALL 按 0.2 重算为 0.8
- **AND** 活跃泳道热度 SHALL 保持 6.0 不变

#### Scenario: 热度不因分桶或事件标签数量变化而变

- **WHEN** 同一泳道同一天的线索数不变，但打标模型把当天内容拆成不同数量的主题桶或产出不同数量的事件标签
- **THEN** 该泳道当天热度 SHALL 保持不变

#### Scenario: 只表达当天不跨天累积

- **WHEN** 某泳道前一天有 20 条线索、当天 0 条线索
- **THEN** 该泳道当天热度 SHALL 为 0
- **AND** SHALL NOT 包含前一天的任何衰减残留或半衰期分量

#### Scenario: 坏权重存量回退默认

- **WHEN** `lane_heat_candidate_weight` 的存量值为越界数、非有限数或空串
- **THEN** 系统 SHALL 回退默认 0.5 并留下告警
- **AND** SHALL NOT 因该坏值拒绝渲染泳道或版块行

#### Scenario: 候选权重不影响话题状态机

- **WHEN** 把候选权重调到 0
- **THEN** 候选话题的命中计数、连续命中资格与转正门槛 SHALL 保持原判定
- **AND** 候选话题 SHALL NOT 因热度为 0 被隐藏、归档或转正

### Requirement: 版块关注度与分档

版块关注度 SHALL 为该版块各泳道当天热度之和，保留一位小数。版块行 SHALL 按关注度降序排列；关注度低于关注阈值的版块 SHALL 折叠为一行「常温」摘要。

分档阈值 SHALL 为可配置项，且 SHALL 按新关注度量纲下的真实分布标定，标定依据 SHALL 记录在 design 或实现文档中。阈值 MUST NOT 直接沿用事件线热度量纲下的 `3.3` / `0.7`，也 MUST NOT 沿用更早的 section 计数量纲下的 `50` / `15`。

分档信息 SHALL 以可读形式表达（行内徽标或文字 + hover 提示），MUST NOT 只以颜色表达。关注度 SHALL NOT 被用于话题状态判定，也 SHALL NOT 因候选泳道权重调高而被解释为话题活跃度。

#### Scenario: 关注度为泳道热度之和

- **WHEN** 某版块有两个活跃泳道当天热度分别为 6.0 与 2.0
- **THEN** 该版块关注度 SHALL 为 8.0 并按一位小数呈现

#### Scenario: 分档阈值按新分布标定

- **WHEN** 新关注度量纲下真实分布的分位点与旧 3.3 / 0.7 不一致
- **THEN** 分档阈值 SHALL 以新分布重新标定并留痕标定依据（样本、日期、分位点）
- **AND** SHALL NOT 沿用旧数值，也 SHALL NOT 沿用更早的 50 / 15

#### Scenario: 分档可读不靠颜色

- **WHEN** 某版块落入「关注」档
- **THEN** 界面 SHALL 以文字徽标或行内提示表达该档位
- **AND** SHALL NOT 仅以颜色区分档位

### Requirement: 线索计数的内容身份去重口径

对外呈现的「来源数 / 文章数 / 参与篇数」SHALL 按**内容身份**去重：同一篇内容（规范化链接相同）被多个 feed 各抓一份 SHALL 只计 1 篇。

去重键 SHALL 为 `COALESCE(NULLIF(content_key,''), participant_key)`，其中 `content_key` = 规范化文章链接（去 `#fragment`、去 `?query`、scheme/host 小写、path 原样），`participant_key` 只作 feed 粒度兜底。内容身份不可判定（链接为空）时 SHALL 退化为 feed 粒度，SHALL NOT 静默按 0 计。

该口径 SHALL 是所有对外计数的唯一实现点，SHALL NOT 在不同消费面各写一份等价式。

#### Scenario: 同一内容多 feed 只算一篇

- **WHEN** 同一篇文章被 3 个 feed 各抓一份、规范化链接相同
- **THEN** 该簇的来源篇数 SHALL 为 1 而不是 3

#### Scenario: 内容身份不可判定时退化为 feed 粒度

- **WHEN** 某文章链接为空、内容身份不可判定
- **THEN** 该篇 SHALL 按 feed 粒度单独计 1 篇
- **AND** SHALL NOT 被丢弃或与其他文章合并

#### Scenario: 不同消费面口径一致

- **WHEN** 同一份数据分别被矩阵来源数、聚簇文章数与候选排序消费
- **THEN** 三处 SHALL 得出同一去重口径下的同一计数
