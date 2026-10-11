<!-- complexity: complex -->
<!-- ui-impact: major -->
<!-- constraint-domains: semantic-board, daily-report, scheduler -->

## Why

版块的构成标签（board composition 里的 aux/composite）挂载后没有任何质量反馈：哪些标签在给版块贡献低分/方向不匹配的噪声文章、哪些语义已经事实上被版块成员高频携带却没被收编为构成标签，用户全看不出来。同时现有扩充建议的召回范围是全库 aux 池，处理范围大、证据弱。需要一个自动、纯读的「版块体检」机制让版块治理有数据可依，并以体检产出的成员携带证据（member-derived）作为扩充建议的高质量候选来源。

## What Changes

- **版块体检 job（每日自动，纯读）**：挂日报生成流程之后执行，对每个活跃版块计算健康度统计——
  - 标签贴合度分布（现场重算构成标签 embedding × 全部挂载 tags 的 identity embedding cosine，**不依赖单 aux 挂载关系**）的均值/中位数/分位数/低分占比；
  - 版块级指标：match_reason 分布、direction_mismatch（幽灵匹配）计数、向量缺失计数；板块太小或向量全缺的构成标签显示「数据不足」，不硬凑；
  - 事实标签画像 vs 官方构成 diff：已归入版块的 tags 携带的、不在构成中的高频 aux 清单（member-derived 扩容候选，只存候选不落建议）。
- **LLM 综合评分**：体检数据快照交给 LLM，产出评分 + 一句评语（不做规则加权合成）。
- **体检快照落表**：每次体检结果持久化，支撑版块健康度趋势展示（验证收编 aux 前后的效果）。
- **阈值通知**：体检评分异常时由 LLM 决定是否值得打扰用户，值得则经现有 Notification 系统发通知（LinkType=board 可跳转到版块）。
- **member-derived 扩充候选接入现有扩充建议管道（手动触发）**：用户在版块上手动触发生成扩充建议时，候选来源为体检存好的 member-derived 高频 aux（而非全库召回），仍走 LLM 裁决 + suggestion_hash 幂等 + dismiss 冷却管道；确认执行时用户逐条勾选子集（现有 per-row 勾选能力）。**红线 10 不动：定时任务仍不得自动生成扩充建议，扩充建议落库必须用户手动触发。**
- **前端：体检治理面板（形态 A，吸收构成管理，BoardCompositionPanel 退役）**：原「板块内容」tab 的构成面板（aux chips 纯展示 + 添加/移除 + 组合标签管理）与体检评价合并为单一治理主面板——评分徽章 + LLM 评语 + 指标 chips + 构成标签质量表（含行内移除，ref_count 展示废弃）+ 组合标签区（含创建入口）+ 评分趋势 + 未收编候选（带「收编」快捷动作 = 现有手动挂载接口，不走 LLM 建议管道）；header「＋ 添加构成」收编原 picker；无体检快照时质量列显「待体检」占位、构成管理照常可用；泳道动态（LaneDynamicsPanel）与来源构成（BoardSourcePanel）位置不动；评分低时 tab 顶部警告条。

## Capabilities

### New Capabilities

- `board-health-checkup`: 版块体检——每日纯读统计（标签贴合度分布/幽灵匹配/事实画像 diff）、LLM 综合评分与评语、快照持久化与趋势、异常通知（LLM 决定是否打扰）、版块内容 tab 体检卡展示。

### Modified Capabilities

- `board-upgrade-expand`: 扩充候选召回新增 member-derived 来源路——手动触发生成扩充建议时，召回候选改为（或并入）体检产出的事实画像高频未收编 aux，替代原有全库相似路+共现路的默认行为。

## Impact

- **后端**：`internal/tagmanagement/`（体检统计服务、member-derived 候选查询、扩充召回接入）、`internal/platform/notification/`（复用，发 LinkType=board 通知）、日报生成链路或 scheduler（体检 job 挂载点）、新迁移（体检快照表）。
- **前端**：`app/features/tags/`（版块内容 tab 体检卡组件 + 顶部警告条 + 趋势展示，参考 BoardSourcePanel 与 FeedSourceQualityPill/Block 模式）。
- **规范**：`docs/reference/flow/semantic-board.md`（体检机制、member-derived 召回路、红线新增体检相关条目）；红线 10 保持不变。
- **LLM 成本**：每日每活跃版块一次综合评分调用 + 异常时通知裁决调用；扩充建议生成仍为手动触发的 LLM 裁决（候选集变小，成本低于现有全库召回）。
