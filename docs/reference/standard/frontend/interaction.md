# 前端交互契约（Interaction）

<!--
doc-impact-applies: front/app/features/, front/app/pages/, front/app/components/ui/, front/app/components/dialog/, front/app/components/common/, front/app/composables/ | section=Requirements
-->

> **权威源**：本文件是「交互模式怎么选、界面文字承担什么职责、原型长什么样」的唯一权威。**状态标记位置、可观测性展示分层与数值/日期呈现格式**见 [interaction-conventions.md](interaction-conventions.md)（两份文件分工：本文件管「选哪个模式」与「文字职责边界」，那边管「状态与数字怎么摆」）；页面布局模式（`reader`/`contained`/`workspace`/`split`）、弹窗尺寸档与浮层层级见 [layout.md](layout.md)；UI 工作流分档与原型审批见《开发执行规范》§3.1。
> 来源 change：`add-ui-contract-ladder`（design D1–D5），文字纪律提炼自 `lane-manual-correction-loop`（其 `ui-design.md` 的「界面文字纪律」节）；占位界线、状态文案与视觉否决项见 [prototype-content.md](prototype-content.md)（`add-prototype-slop-filter` 拆出）。
> **本文档预算：≤180 行**（超了先拆文件，不要继续加）。

## 为什么需要交互契约

`layout.md` 把 `dialog` 规训得最细——四档尺寸、三个 Scenario、浮层层级、嵌套抬升——而 `canvas` 只有一条「按容器真实像素 1:1 绘制」，说的是**怎么画**，没说**什么时候该画**。结果是：被规训得最细的模式最容易被选中。用户指定画布，agent 交弹窗堆叠，不是审美分歧，是判据缺失。

同一缺口的另外两面：`ui-design.md` 不要求声明数据来源，于是「丰富」成了唯一可交付的丰富度（实测某 change 原型内联 163 处 `fake` 数据）；制品没有预算，于是按模块分批生产直接乘出 61 份散装设计说明。

## Requirements

### Requirement: 交互模式必须从固定词表选择

每个需要用户操作的位置 MUST 指定下列模式之一。SHALL NOT 在 change 内临时发明模式名（「轻量面板」「侧边浮窗」「快捷卡片」一律不合法）；需要新模式时 MUST 先扩充本表并给出判据。

| 模式 | 何时该用 | 何时禁止 | 本项目实例 |
|---|---|---|---|
| `page` | 独立信息架构、可深链、有独立 URL | 只需要在一屏内改一处状态时 | `/tags` 雷达、`/tags/:boardId` 版块态势 |
| `panel` | 页面内的独立分区，需与其他分区**并列可见** | 需要压倒性聚焦且会丢失上下文时 | 版块治理工作面右轨、走廊侧栏 |
| `drawer` | 从边缘滑出、需**保留主区上下文**的次级操作 | 操作本身就是主任务时 | 窄屏主工作台降级（`layout.md`「抽屉 + 单栏切换」） |
| `dialog` | **破坏性**操作需二次确认；或与当前页面无关的独立小任务 | 单字段修改、并列比较、依赖空间关系的操作 | 归档版块（手打版块名）、`AddSemanticBoardDialog` |
| `inline-edit` | 改**单个字段**且改动时需看见上下文 | 字段之间存在联动校验时 | 修正工作面标题就地改名 |
| `canvas` | 操作依赖**多个对象的空间关系或相对位置**（拖动、编排、连线、区域框选） | 只有一两个对象、无空间语义时 | 泳道编排画布、雷达点阵 |
| `command` | 跨页面跳转/操作的高频入口 | 操作目标在主区内已可见时 | 暂无实例；引入时在此登记 |

强制判据（可被 review 机械核对）：

| # | 条件 | 必须 |
|---|---|---|
| P1 | 操作依赖多个对象的空间关系或相对位置 | `canvas`；SHALL NOT 用 `dialog` |
| P2 | 改单个字段且需看见上下文 | `inline-edit`；SHALL NOT 用 `dialog` |
| P3 | 破坏性且不可逆、或需二次确认 | `dialog`（`sm`/`md` 档） |
| P4 | 需在保留列表上下文前提下多选/批量 | `panel` 或 `inline-edit`；SHALL NOT 用 `dialog` |
| P5 | 操作对象超过一个且需并列比较 | SHALL NOT 用 `dialog`（串行弹窗逐个呈现即违反） |

选择理由 MUST 是一条判据，SHALL NOT 是「实现方便」「组件已有」「弹窗好写」。

#### Scenario: 并列比较不得塞进弹窗
- **WHEN** 用户需同时看多个候选逐一比较后选一个
- **THEN** 设计 MUST 用 `panel` 或 `canvas` 并列呈现

#### Scenario: 单字段修改不得弹窗
- **WHEN** 用户只改一个名称，且该字段在页面上已有可见位置
- **THEN** 设计 MUST 用 `inline-edit` 就地修改

### Requirement: 界面文字只承担三类职责

产品界面文字 SHALL 只属于三类：**动作标签**（按钮、菜单项、表头）、**状态结论**（徽标、空态、错误、计数）、**必要确认与反馈**（二次确认文案、toast）。

下列四类 SHALL NOT 出现在产品界面：

| 不得出现 | 例子 | 由什么承担 |
|---|---|---|
| 操作口径的并列解释 | 「点节点 = 选中查看（细圈）｜勾选 = 选入本泳道（实心 + 勾）」 | 交互自身的视觉差异 + tooltip |
| 派生值机制科普 | 「重心 = α·首义 + (1-α)·滚动均值」 | 只保留结论徽标：「立即生效」「次日生效」 |
| 图例长句 | 「大小 / 深浅 = 接近程度（不是置信度）」 | 极简符号图例（≤8 字/项）或省略 |
| 界面内教学语 | 「画布只做展示。要移除请在左栏勾选。」 | 按钮本身的命名与位置 |

解说性内容 SHALL 只出现在 `ui-design.md` 与原型评审区，SHALL NOT 进入产品界面。

#### Scenario: 节点卡不得承载教学语
- **WHEN** 原型在节点卡内写入「要移除请在左栏勾选」
- **THEN** 该文本 MUST 删除，移除能力由左栏按钮的命名与位置表达

### Requirement: 交互决策台账带 pinned / forbidden 语义

`ui-design.md` MUST 含 `## Interaction Decisions` 表（**none 档豁免；minor 档起必填**），每行一个决策：

| ID | 决策 | 模式 | forbidden | 状态 |
|---|---|---|---|---|
| D1 | 泳道内容的增删改入口 | `canvas` | `dialog`, `drawer` | `pinned` |
| D2 | 泳道重命名 | `inline-edit` | `dialog` | `pinned` |

- `状态: pinned` = 用户已定的方向。实现与原型 **MUST NOT 替换、简化、合并**，也 MUST NOT 因为「弹窗更好实现」而改写台账。具体禁用四种偏离：① 拆成 `pinned(部分)` + `open(其余)`；② 不换模式但改写该决策的交互形态（触发方式 / 时序 / 位置）；③ 放宽该行的 `forbidden` 列；④ 把该行降级为 `open`。
- 唯一合法的变更路径：**用户在对话中明确改口后**，由 agent 写回台账并追加「变更来源：用户 <日期>」；除此之外一律视为违反契约。
- 认为必须偏离时，回问的**最小形式** = 偏离理由 + 两种候选方案；在用户答复并写回台账前 MUST NOT 继续实现。
- `forbidden` 取值 MUST 取自上面 7 个模式；核对对象固定为三处：台账、`interaction-table.md` 的「触发 / 结果」两列、原型可见文本与控件形态——任一处出现被禁模式即命中。
- `状态: open` = agent 可提案，但提案 MUST 附判据编号（P1–P5）。

#### Scenario: pinned 被实现阶段"优化"掉
- **WHEN** 实现阶段把 `pinned` 的 `canvas` 改成 `dialog` 并同步改了台账
- **THEN** 判定为违反契约；正确做法是停止实现并向用户提出偏离

### Requirement: 数据来源必须显式声明

`ui-design.md` MUST 含 `## Data Provenance` 表（**none 档豁免；minor 档起必填**），每个用户可见数据项一行。**枚举口径**：以「界面上独立可指认的数字、列表、徽标或图表」为一项——同一表格的同一列只计一项，装饰性静态文案不计：

| 数据项 | source | 说明 |
|---|---|---|
| 泳道当日热度 | `api GET /daily-reports/topics` | 已存在端点 |
| 节点归属计数 | `daily_report_sections.assignment` | 表字段 |
| 「预计升温」标签 | `派生 近 3 日均值 > 阈值` | 前端派生，无后端字段 |
| 「本周关注度」环形图 | `none` | `needs-approval`：后端无数据源 |

- `source` MUST 取 `api <path>`、`表.字段`、`派生 <公式>`、`none` 之一。
- `api <path>` **必须可核**：路径 MUST 能在 `front/app/api/**` 或后端路由注册处 grep 到；grep 不到就只能填 `none` + `needs-approval`（这一条是本规范最容易形式化的地方，审核时优先核它）。
- 取 `none` 者 MUST 标 `needs-approval` 并在对话中获用户确认后才可进实现。
- 原型数据 MUST 与 provenance 表一致，SHALL NOT 出现表中没有的字段。

#### Scenario: 造了字段却没有来源
- **WHEN** 原型展示「本周关注度」环形图，而 provenance 表无对应行
- **THEN** 制品 MUST 判定为不完整：要么补来源，要么标 `none` 并取得批准

### Requirement: 原型目录受预算约束

`ui-prototype/` MUST 满足下列上限。**计数口径**：「短语」= 一个独立可见文本节点（按钮名、标签、徽标、标题各计 1，不按字数拆），按字符数计长（标点计入）；「屏」= 一个 HTML 视图或一张状态图（画布类三张状态图算 3 屏）；例外仅限空态、错误态、二次确认文案：

| 项 | 上限 |
|---|---|
| 设计说明类 md | 非画布类：≤1 份；**画布类：≤2 份**（`interaction-table.md` 必需 + `states/README.md` 可选）；每份 ≤300 行。禁止按模块散装的 `node-spec.md` / `parts-table.md` / `*-verification.md` 堆积 |
| 界面可见文字 | ≤12 个短语/屏；禁止 >40 字完整句 |
| 数据 | MUST 来自 `fixture.json`；禁止 HTML 内联 >50KB `<script>` 数据集 |
| 版本 | 只留当前版本；禁止 `revision-*/` 目录与 `v49`/`v50` 快照 |
| 截图 | 只留决策表与 `states/` 引用的那几张；未引用者清理 |

> 决策表**不进** `ui-prototype/`：唯一落脚点是 `ui-design.md` 的 `## Interaction Decisions` 节。原型目录里只放原型本身与它的操作表。

#### Scenario: 按模块分批乘出散装文档
- **WHEN** 一个 major change 拆 9 个模块分批生产
- **THEN** 产出 MUST 收敛为 1 份决策表 + 1 个入口原型 + `fixture.json`；MUST NOT 每模块各产出一份设计说明书

### Requirement: 画布类交互的原型形态

主要模式为 `canvas` 的界面，原型 MUST 采用「状态图 + 操作表」，SHALL NOT 用可点 HTML 模拟：

- `states/` 至少三张：空态、编辑中、已确认；
- `interaction-table.md`：`操作` / `触发` / `前置` / `结果` / `反馈` / `失败`，每操作一行；
- `反馈` 列 MUST 写清动效与吸附规则（「松手后 120ms 归位并高亮目标区域」），SHALL NOT 写「有反馈」；
- 审批与验收的比对基准 = 三张状态图逐态对账 + `interaction-table.md` 六列逐行对账，**不需要可交互页面**。画布类原型的 V3–V5（焦点/键盘/减动）由 `interaction-table.md` 末尾的「可访问与动效声明」四行承接，见 [prototype-content.md](prototype-content.md)。

理由：静态 HTML 里「装得像能交互」最便宜的手段是弹窗，强制可点 HTML 会把画布设计系统性推向弹窗堆叠。

#### Scenario: 画布用弹窗模拟被拒
- **WHEN** 台账把泳道内容编辑钉为 `canvas`，原型却是「点节点 → 弹窗逐条勾选」
- **THEN** 判定为偏离契约

## 与 layout.md 的边界

`reader` / `contained` / `workspace` / `split` 是**布局模式**（决定容器宽度与栏数），本文件的 7 个是**交互模式**（决定操作以什么形态发生）。同一个页面可以同时是 `split` 布局 + `inline-edit` 交互。`layout.md` 的「画布 1:1 绘制」是**渲染保真**要求，不构成选画布的理由；反之选画布也不豁免 1:1 要求。
