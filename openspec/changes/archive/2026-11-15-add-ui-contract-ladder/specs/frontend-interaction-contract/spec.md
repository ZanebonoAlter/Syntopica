# frontend-interaction-contract — 交互契约 delta（新 capability）

## Purpose

为「用户可见界面用什么模式交互、界面上允许出现多少文字、原型长什么样」建立可审查的基线，避免模式选择偏斜（凡被规训最细的模式最易被选中）与原型无界膨胀。

## ADDED Requirements

### Requirement: 交互模式必须从固定词表显式选择

每一处需要用户操作的位置 MUST 从固定词表显式指定模式，词表为：`page`、`panel`、`drawer`、`dialog`、`inline-edit`、`canvas`、`command`。SHALL NOT 在 change 内临时发明模式名（如「轻量面板」「侧边浮窗」）；需要新模式时 MUST 先在 `docs/reference/standard/frontend/interaction.md` 扩充词表并给出判据。

模式选择 MUST 能被一条判据支持，SHALL NOT 以「实现方便」「组件已有」为理由。以下判据为强制：

- 操作依赖**多个对象的空间关系或相对位置**（拖动、编排、连线、区域框选）→ MUST 用 `canvas`，SHALL NOT 用 `dialog`；
- 修改**单个字段且修改时需要看见上下文**（名称、标题、少量属性）→ MUST 用 `inline-edit`，SHALL NOT 用 `dialog`；
- 操作**破坏性**且不可逆或需二次确认 → MUST 用 `dialog`（`sm`/`md` 档）；
- 需要在**保留列表上下文**的前提下多选或批量操作 → MUST 用 `panel` 或 `inline-edit`，SHALL NOT 用 `dialog`；
- 操作对象**超过一个且需并列比较** → SHALL NOT 用 `dialog`。

#### Scenario: 并列比较不得塞进弹窗
- **WHEN** 用户需要同时看多个候选并逐一比较后选一个
- **THEN** 设计 MUST 选择 `panel` 或 `canvas`，把候选并列呈现；MUST NOT 用串行弹窗逐个呈现

#### Scenario: 单字段修改不得弹窗
- **WHEN** 用户只改一个名称或标题，且该字段在页面上已有可见位置
- **THEN** 设计 MUST 采用 `inline-edit` 就地修改

#### Scenario: 自创模式名被拒
- **WHEN** `ui-design.md` 的决策台账里出现 `模式: 轻量面板`
- **THEN** 该行 MUST 被要求改写为词表取值

### Requirement: 界面文字只承担三类职责

产品界面文字 SHALL 只属于三类：**动作标签**（按钮、菜单项、表头）、**状态结论**（徽标、空态、错误提示、计数）、**必要的确认与反馈**（二次确认文案、toast）。

以下四类 SHALL NOT 出现在产品界面，其职责由交互本身承担：

- 操作口径的并列解释（如「点节点=选中查看｜勾选=选入本泳道」）——由视觉差异与 tooltip 承担；
- 派生值机制科普（如「重心 = α·首义 + (1-α)·滚动均值」）——只保留结论型徽标（「立即生效」「次日生效」）；
- 图例长句（超过 8 字/项）——压缩为极简符号或省略；
- 界面内的操作教学语（如「要移除请在左栏勾选」）——由按钮命名与位置承担。

解说性内容 SHALL 只出现在 `ui-design.md` 与原型评审区，SHALL NOT 进入产品界面。

#### Scenario: 节点卡不得承载教学语
- **WHEN** 原型在节点卡内写入「画布只做展示，要移除请在左栏勾选」
- **THEN** 该文本 MUST 被删除；移除能力由左栏按钮的命名与位置表达

#### Scenario: 机制解释压缩为结论徽标
- **WHEN** 设计需要让用户知道某项修正何时生效
- **THEN** 界面 MUST 只出现「立即生效」或「次日生效」这类结论，MUST NOT 出现计算链路说明

### Requirement: 原型目录受预算约束

`ui-prototype/` 目录 MUST 受以下预算约束，超出即视为制品不合格。**计数口径**：「短语」= 一个独立可见文本节点（按钮名/标签/徽标/标题各计 1），按字符数计长（标点计入）；「屏」= 一个 HTML 视图或一张状态图（画布类三张状态图计 3 屏）：

- 设计说明类 markdown：非画布类 SHALL ≤1 份，**画布类 SHALL ≤2 份**（`interaction-table.md` 必需 + `states/README.md` 可选），每份 SHALL ≤300 行；SHALL NOT 出现按模块/节点散装的 `node-spec.md`、`parts-table.md`、`*-verification.md` 长期堆积；决策表唯一落脚点是 `ui-design.md` 的 `## Interaction Decisions`；
- 原型界面可见文字 SHALL ≤12 个短语/屏，SHALL NOT 出现超过 40 字的完整句子（例外仅限空态、错误态、二次确认文案）；
- 原型数据 MUST 来自同目录的 `fixture.json`，SHALL NOT 在 HTML 中内联超过 50KB 的 `<script>` 数据集；
- 目录 SHALL 只保留当前版本，SHALL NOT 出现 `revision-*/` 目录或 `v49`/`v50` 这类版本快照堆积；
- 截图 SHALL 仅保留决策表引用到的那几张；未被引用的截图 MUST 清理。

#### Scenario: 按模块分批不得乘出散装文档
- **WHEN** 一个 major change 拆成 9 个模块分批生产
- **THEN** 产出 MUST 仍收敛为 1 份决策表 + 1 个入口原型 + 该原型的 `fixture.json`；MUST NOT 每个模块各产出一份设计说明书

#### Scenario: 版本快照堆积被拒
- **WHEN** `ui-prototype/` 下出现 21 个 `revision-*` 目录
- **THEN** 制品 MUST 被判定为不合格，只保留决策表所引用的最终版本

### Requirement: 画布类交互的原型形态

主要模式为 `canvas` 的界面，原型 MUST 采用「状态图 + 操作表」形态而非可点 HTML：

- `states/` 目录下至少三张状态图：空态、编辑中、已确认；
- `interaction-table.md`：列含 `操作` / `触发` / `前置` / `结果` / `反馈` / `失败`，每个操作一行；
- `反馈` 列 MUST 写清动效与吸附规则（如「松手后 120ms 归位并高亮目标区域」），SHALL NOT 写「有反馈」这类无对象措辞。

理由：静态 HTML 中「装得像能交互」最便宜的手段是弹窗，强制可点 HTML 会系统性把画布设计推向弹窗堆叠。

#### Scenario: 画布交互用弹窗模拟被拒
- **WHEN** 决策台账把泳道内容编辑钉为 `canvas`，原型却提供「点击节点 → 弹窗逐条勾选」的可点页面
- **THEN** 该原型 MUST 被判为偏离契约

#### Scenario: 状态图缺已确认态
- **WHEN** `states/` 只有空态与编辑中两张图
- **THEN** 制品 MUST 判定为不完整，补足已确认态
