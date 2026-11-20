## ADDED Requirements

### Requirement: 版块栏逐日线索条码

版块态势页左轨（版块栏）SHALL 用**逐日线索条码**表达每个版块，MUST NOT 把标签挂载数（`tag_count`）或任何单点计数作为行内主数字。

每个版块行 SHALL 为一段式布局：**版块名称**（超长省略号截断）+ **14 格逐日线索条码**（右侧）。

| 元素 | 口径 |
| --- | --- |
| 条码格值 | 该版块该日**非综合稿线索合计**（`GET /semantic-boards/overview` 的 `clue_days`） |
| 条码窗 | 固定 **14 天**，MUST NOT 随顶部观察档位（7/14/30/365）变化——跨版块可比性依赖固定窗 |
| 颜色分档 | 按**行内峰值**归一（与矩阵日格、雷达页版块小矩阵同一套色阶），MUST NOT 引入第二套色阶 |
| 标签挂载数 | MAY 退到行的 `title`，MUST NOT 占据行内数字位 |
| 解释性文字 | MUST NOT 在左轨常驻「数字 = …」「每格 = 一天」这类口径说明 |

行序 SHALL 保持 `display_order`（`GET /semantic-boards` 返回序），MUST NOT 因活跃度重排；活跃度差异 SHALL 只用视觉强调表达。

**共同轴对齐**：各版块的 `clue_days` 由后端按**各自锚点日**生成（`radarAxis(anchor)`），直接并列即假比较。实现 SHALL 按**全局锚点日**（overview 响应 `date`）把每行重对齐到同一 14 天轴 `[date-13 … date]`；该版块自身窗口覆盖不到的位置 SHALL 标**未取到**（斜纹占位），MUST NOT 补 0。于是「停更」与「安静」SHALL 在视觉上可区分，MUST NOT 用同一形态表达。

#### Scenario: 未取到不等于安静

- **WHEN** 某版块 overview 行标记 `data_unavailable`、或后端未下发 `clue_days`
- **THEN** 该行条码 SHALL 整行显示未取到占位（斜纹），MUST NOT 显示为空或全 0

#### Scenario: 停更版块的尾段留白

- **WHEN** 某版块锚点日落后全局锚点日 3 天
- **THEN** 该行条码前段 SHALL 显示真实值、末 3 格 SHALL 显示未取到占位；MUST NOT 用 0 补齐，也 MUST NOT 把末格当作「最近有动静」

#### Scenario: 行内无数字无说明

- **WHEN** 用户浏览左轨
- **THEN** 每行 SHALL 只显示名称与条码，MUST NOT 出现当日线索数等独立数字位，MUST NOT 出现口径解释文字

#### Scenario: 选中版块与动效

- **WHEN** 用户点击某版块行
- **THEN** 该行 SHALL 呈现选中态（左缘强调线），当日格 MAY 脉动；MUST NOT 让全部行同时脉动；`prefers-reduced-motion: reduce` 时 SHALL 关闭入场与脉动动效

## MODIFIED Requirements

### Requirement: 治理右轨与治理抽屉

版块态势页 SHALL 在右轨常驻只读治理与动态信号（≥1200px；768–1199 收进顶栏「治理」入口，抽屉头部仍显示体检分与趋势摘要），区块顺序为：**版块体检 → 当日日报 → 话题追踪 → 构成候选 → 来源构成**。

**版块体检** SHALL 为一行状态条：最新快照的真实 `llm_score`、体检时间与刷新入口（MUST NOT 以雷达关注度冒充健康、MUST NOT 随观察日期回溯历史评分）。「最新快照 · 非所选历史评分」口径 SHALL 在详情说明、右轨不重复（沿用 `simplify-workshop-matrix-copy` 已定的口径）。评语首句 SHALL NOT 常驻右轨；完整评语、趋势与依据（含挂载标签数、幽灵匹配数、向量缺失）SHALL 经入口按需查看。

**当日日报** SHALL 跟随**矩阵选中日**（唯一日期源），并用两层表达：

1. **点评层**：该日日报的真实编辑点评（`highlights`，2–3 条），每条 SHALL 显示标题与点评正文，`article_count` 缺省时 MUST NOT 显示「N 篇来源」。
   该日报的 `title` / `summary` **就是首条重点的标题与正文**（后端派生关系），SHALL NOT 与点评层重复展示。
2. **泳道层**：该日日报中归属于正式泳道（`topic_status_at_report = active`）的分区，每条 SHALL 显示泳道色点、泳道名、**该泳道当日的线索数**与一句代表线索（日报线索标题，无线索时退回分区概述）。
   线索数 SHALL 取自泳道逐日线索量（与矩阵日格**同源同值**），SHALL NOT 取日报分区线索数（后者含综合稿，与矩阵口径不同）；取不到时显示 `—`，MUST NOT 补 0。
   该层 SHALL 按线索数降序排列（并列时保持日报原序）；线索数未知的泳道排在末位。
   候选分区（「其他动态」）SHALL NOT 进右轨。

右轨 MUST NOT 提供任何日期选择器；日期 SHALL 只来自矩阵选中日，右轨块标题 SHALL 写明所跟日期。选中日无日报时 SHALL 显示诚实空态，MUST NOT 显示上一日内容或回落到锚点日。右轨取数失败 MUST NOT 阻塞矩阵与版块栏。

**构成候选** SHALL 列真实名称与实际清单数（top 3），措辞 MUST NOT 把高频共现候选表述为确定整改指令；构成候选（辅助标签收编）、话题 candidate（人工激活）、话题追踪（watch）为三类不同实体，SHALL 分列标清、MUST NOT 自动转正或写状态。**话题追踪** SHALL 显示真实条目名称与 active/paused 状态，管理入口 SHALL 仅以板块 `boardId` 为作用域（关注记录关联的 persistentTopicId 仅用于定位对应泳道）。**来源构成** SHALL 为独立滚动统计窗（7/30/90 天）并明示「独立统计，不随时间视窗」，未取数档显示未取数而非补 0。

需要动手的治理内容（构成标签质量表、组合标签、候选全量、来源全量、版块设置）SHALL 收进治理抽屉。右轨 MUST NOT 放置全局治理或重复的创建版块入口。

#### Scenario: 右轨展示体检信号

- **WHEN** 版块有一次体检快照，评分 85
- **THEN** 右轨 SHALL 显示 85、分档色与体检时间；最新快照口径在详情说明（右轨不重复），评语首句 SHALL NOT 出现在常驻状态条上（经体检详情按需查看）

#### Scenario: 无体检快照

- **WHEN** 版块尚无体检快照
- **THEN** 右轨 SHALL 显示「等待首轮体检」，构成管理入口仍可用

#### Scenario: 追踪管理仅板块作用域

- **WHEN** 用户在选中某泳道时打开话题追踪管理
- **THEN** 管理 SHALL 仅按当前板块读取全部关注记录，MUST NOT 默认按所选泳道过滤

#### Scenario: 右轨首屏顺序

- **WHEN** 用户以 1440×900 视口打开版块态势页
- **THEN** 右轨自上而下 SHALL 依次为体检状态条、当日日报、话题追踪、构成候选、来源构成

#### Scenario: 当日日报不重复日报标题

- **WHEN** 该日日报有 3 条重点
- **THEN** 右轨 SHALL 显示这 3 条各带标题与点评正文，MUST NOT 在其上方再单列日报标题（标题即首条重点）

#### Scenario: 当日日报跟随矩阵选中日

- **WHEN** 用户在矩阵把选中日切到 11-16
- **THEN** 右轨「当日日报」SHALL 显示 11-16 那期日报的点评与泳道线索，块标题 SHALL 写明 11-16

#### Scenario: 选中日无日报

- **WHEN** 用户选中的那一天该版块没有日报
- **THEN** 右轨 SHALL 显示诚实空态说明，MUST NOT 显示邻近日期的日报内容

#### Scenario: 泳道层不并排两套量纲

- **WHEN** 某泳道在当日日报中有分区
- **THEN** 该行 SHALL 显示泳道名、该泳道当日的线索数与一句代表线索；线索数 SHALL 与矩阵该泳道该日格的值相同，MUST NOT 使用含综合稿的日报分区线索数

#### Scenario: 泳道层按线索数排序

- **WHEN** 当日日报涉及 4 条正式泳道，其当日线索数分别为 1、4、2、0
- **THEN** 右轨 SHALL 按 4、2、1、0 的顺序列出这 4 条泳道；并列时 SHALL 保持日报原序

#### Scenario: 候选分区不进右轨

- **WHEN** 该日日报含 `topic_status_at_report = candidate` 的分区
- **THEN** 右轨 SHALL NOT 列出这些分区（由矩阵下方「当日其他话题」承载）

#### Scenario: 未取到不冒充安静

- **WHEN** overview 本次未取到某版块计数
- **THEN** 左轨该行 SHALL 显示未取到占位，MUST NOT 显示 0、MUST NOT 声称该版块无动态
