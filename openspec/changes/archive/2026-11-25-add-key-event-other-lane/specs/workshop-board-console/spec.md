## MODIFIED Requirements

### Requirement: 关键事件视图切换与行集合

版块态势页主舞台 SHALL 提供两个同级视图：「矩阵 / 追踪」（现状）与「关键事件」。两视图 MUST 共用同一条观察轴（同一时间视窗控件与同一份 `visibleAxis`）、同一份持久化话题行序与受控选中状态（`selectedLane` / `selectedDate`）；切换视图 MUST NOT 重置其中任何一项。时间范围 SHALL 由用户在同一个时间视窗控件里选择（7 / 14 / 30 / 365 档或自由区间），关键事件视图 MUST NOT 维护第二套跨度状态。

行集合 SHALL 由两部分组成，顺序固定为：持久化话题行（active，按矩阵行序，含休眠分组）→ 一个「其他」兜底行。候选话题（candidate）MUST NOT 单独占行；其确认事件 MUST NOT 被丢弃，SHALL 全部归入「其他」行（见「关键事件『其他』兜底行」）。

视图头 SHALL 提供「所属日期」选择器（候选日只列观察窗内的日期），选定后写入与矩阵同一个 `selectedDate`，MUST NOT 维护第二份选中日状态，并显示该日在全部泳道（含「其他」行）内的确认事件条数。

选中日落到可见轴之外时，本视图 SHALL 取消聚焦，MUST NOT 呈现窗外日期的节点、标签或卡片。

#### Scenario: 切换视图保留选中态

- **WHEN** 用户在矩阵中选中「大模型商业化路径」与 11-19，然后切到「关键事件」
- **THEN** 关键事件视图 SHALL 保持该行选中、11-19 列选中与当前观察窗，MUST NOT 回落到默认选中

#### Scenario: 时间视窗两侧共用

- **WHEN** 用户在矩阵把时间视窗切到 30 天，再切到「关键事件」
- **THEN** 关键事件视图 SHALL 直接使用这 30 天轴，MUST NOT 出现第二个跨度控件或不同的日期范围

#### Scenario: 候选话题不占行

- **WHEN** 观察窗内有 13 个候选话题各自带有确认事件
- **THEN** 本视图行集合 SHALL 不新增 13 行，MUST NOT 生成候选行；这 13 个话题的确认事件 SHALL 全部出现在「其他」行内，一条不丢

#### Scenario: 右轨选中切回矩阵视图

- **WHEN** 用户在关键事件视图里从右轨（当日其他话题 / 话题条目）选中一个话题
- **THEN** 主舞台 SHALL 切回「矩阵 / 追踪」视图，并把该话题泳道滚入视野 + 高亮，MUST NOT 在关键事件视图里留下一个无对应行的死选择
- **AND** 本视图内部的选中（点事件节点）SHALL 仍留在本视图，MUST NOT 把用户踢出时间轴上下文

#### Scenario: 视图头日期选择器写入同一个选中日

- **WHEN** 用户在关键事件视图头把「所属日期」改为 11-18
- **THEN** 写入的 SHALL 是与矩阵同一个 `selectedDate`，该日在所有有事件的泳道行（含「其他」行）内展开日聚焦卡片列，MUST NOT 产生第二份本视图专属的选中日状态

#### Scenario: 选中日落到窗外取消聚焦

- **WHEN** 观察窗变化使当前选中日不再位于可见轴内
- **THEN** 本视图 SHALL 不展开任何日聚焦卡片列，MUST NOT 呈现该窗外日期的节点、标签或卡片

## ADDED Requirements

### Requirement: 关键事件「其他」兜底行

本视图 SHALL 在休眠泳道组之后提供且仅提供一个「其他」兜底行，收容窗口内所有无法归属到任一持久化话题行的事件：未关注候选话题、已归档话题、话题已删除（`persistent_topic_id` 为空）、以及窗口内无日报分区而未成行的话题。兜底行 SHALL 独立常显（有事件才渲染），MUST NOT 参与休眠泳道的折叠/展开，MUST NOT 生成多个兜底行。

兜底行的事件来源 MUST 为同一份 `confirmed` 事件响应（与正式泳道同一个接口与同一观察窗），MUST NOT 触发额外请求、MUST NOT 纳入 `suggested` / `dismissed` 注记。行内节点、标签、日聚焦卡片列 SHALL 与正式泳道同一套渲染与布局规则。

无兜底事件时 MUST NOT 渲染该行，也不得占位显示「0 条」；事件层读取失败或 stale 时 MUST NOT 用空兜底行冒充成功或 0 条。

#### Scenario: 未关注候选话题的确认事件显形

- **WHEN** 版块「开发者工具与开源生态」的 14 天窗内有 3 条 confirmed 注记属于未关注的候选话题（11-18 两条、11-24 一条）
- **THEN** 本视图 SHALL 在休眠组之后渲染「其他」行，11-18 列与 11-24 列各落对应节点与标签，行头显示 `3 事件 · 最近 MM/DD`

#### Scenario: 无兜底事件不占行

- **WHEN** 窗口内所有 confirmed 注记都归属到当前已显示的持久化话题行
- **THEN** 本视图 MUST NOT 渲染「其他」行，MUST NOT 出现空行或「0 条」占位

#### Scenario: 不参与休眠折叠

- **WHEN** 休眠泳道组处于收起状态且「其他」行有事件
- **THEN** 「其他」行 SHALL 仍然可见，MUST NOT 因休眠组收起而隐藏

#### Scenario: 无持久化话题但有兜底事件

- **WHEN** 版块暂无任何持久化话题行（`no-lanes`），但窗口内存在未归属的 confirmed 注记
- **THEN** 本视图 SHALL 同时保留「该版块暂无持久化话题」说明行与「其他」行，MUST NOT 因无正式行而吞掉兜底事件

### Requirement: 关键事件「其他」兜底行的读数与交互

读数口径 SHALL 与正式泳道一致并包含兜底行：视图头「MM/DD · N 条」、兜底行行头「N 事件 · 最近 MM/DD」与当日条数、整窗空态判定 MUST 计入兜底行事件；MUST NOT 出现「行内有节点、视图头读 0 条」的矛盾。

兜底行混装多话题，事件卡 SHALL 继续显示确认时冻结的所属话题名（`topic_label_snapshot`），MUST NOT 反查实时话题名。

兜底行 MUST NOT 写入壳层选中话题：点该行行头 MUST NOT 触发视图切换、MUST NOT 产生无对应矩阵行的死选中；点该行节点或标签 SHALL 仍写入与矩阵同一个选中日并保持在读视图内。

#### Scenario: 读数含兜底行

- **WHEN** 选中日只有「其他」行存在确认事件
- **THEN** 视图头当日读数 SHALL 显示该日条数（如 `11/18 · 2 条`），MUST NOT 显示 0 条

#### Scenario: 兜底行不产生死选中

- **WHEN** 用户点击「其他」行的行头
- **THEN** 本视图 SHALL 保持当前视图与选中日，MUST NOT 把 `__other__` 之类的占位写入壳层 `selectedLane`，MUST NOT 触发切回矩阵视图

#### Scenario: 无归属历史注记不丢失

- **WHEN** 某条 confirmed 注记的 `persistent_topic_id` 已被置空（话题删除后 `ON DELETE SET NULL`）
- **THEN** 该注记 SHALL 出现在「其他」行，事件卡显示其冻结的话题名快照，MUST NOT 被丢弃
