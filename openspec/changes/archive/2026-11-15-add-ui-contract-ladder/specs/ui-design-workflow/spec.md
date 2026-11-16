# ui-design-workflow — 契约结构化 delta

## MODIFIED Requirements

### Requirement: ui-design 作为必经规划制品

项目 OpenSpec workflow SHALL 在 proposal 之后、specs 与技术 design 之前生成 `ui-design.md`，且 tasks MUST 依赖 specs、技术 design 与 UI 设计均完成。`ui-design.md` MUST 记录 UI 影响等级、页面入口、用户主任务、交互状态、布局类型、组件复用、响应式范围、原型路径和审批状态；`none` 可用带理由的最小 N/A 记录，`minor` MUST 写清现有模式的复用点与状态变化，`major` MUST 写完整信息架构、操作流程和状态矩阵。

在 `major` 与 `minor` 两个档位上，`ui-design.md` 除上述内容外 MUST 另含三节结构化声明，缺任一节即视为制品不完整：

- `## Interaction Decisions` — 交互决策台账，每条决策一行，列含 `ID` / `决策` / `模式` / `forbidden` / `状态`；`模式` MUST 取自 `docs/reference/standard/frontend/interaction.md` 的模式词表；`状态` 取 `pinned` 或 `open`；
- `## Data Provenance` — 每个用户可见数据项一行，列含 `数据项` / `source` / 说明；`source` MUST 取 `api <path>`、`表.字段`、`派生 <公式>`、`none` 之一；取 `none` 者 MUST 标注 `needs-approval` 并在对话中获得用户确认后方可进入实现；
- `## 明确不做` — 显式排除项列表，每条 MUST 可被读成「不出现在本界面里的具体东西」，SHALL NOT 写成「暂不支持」这类无对象措辞。

`pinned` 决策 SHALL NOT 被实现或原型替换、简化或合并；实现阶段若认为必须偏离，MUST 返回 requirements 档向用户提出，SHALL NOT 自行改写台账。

#### Scenario: none 产生最小制品
- **WHEN** proposal 声明 `ui-impact: none`
- **THEN** `ui-design.md` SHALL 记录 N/A 理由、`ui-approval: not-required` 与 `ui-prototype: none`，无需制作原型，三节结构化声明 SHALL NOT 被要求

#### Scenario: minor 复用既有交互
- **WHEN** change 声明 `ui-impact: minor`
- **THEN** `ui-design.md` SHALL 指明所复用的页面、组件和布局模式，覆盖 loading、empty、error、success 中受影响的状态，并 MUST 补齐 `## Interaction Decisions` 与 `## Data Provenance` 两节

#### Scenario: major 形成完整 UI 契约
- **WHEN** change 声明 `ui-impact: major`
- **THEN** `ui-design.md` MUST 包含页面入口、主次任务、信息层级、主次/危险操作、完整状态矩阵、布局模式、目标视口、组件复用映射、可丢弃原型引用，以及 `## Interaction Decisions`、`## Data Provenance`、`## 明确不做` 三节

#### Scenario: 决策台账缺模式词表取值
- **WHEN** `## Interaction Decisions` 中某行的 `模式` 不在 `interaction.md` 词表内（如自创「侧边浮窗」「轻量面板」）
- **THEN** 制品 SHALL 判定为不完整，review MUST 要求改用词表取值或先提案扩展词表

#### Scenario: pinned 决策被实现阶段改写
- **WHEN** 实现阶段为赶进度把 `pinned` 的 `canvas` 改成 `dialog` 并更新了台账
- **THEN** 该改写 MUST 被判定为违反契约；正确做法是停止实现并在 requirements 档向用户提出偏离

### Requirement: major UI 原型必须获得用户确认

major UI change MUST 在 change 目录的 `ui-prototype/` 中提供与真实接口解耦的可丢弃原型，并在 `ui-design.md` 使用机器可读 `<!-- ui-approval: pending|approved -->` 与 `<!-- ui-prototype: <relative-path> -->` 记录状态。只有用户在对话中明确确认后，agent 才可将审批改为 `approved`；任何改变信息架构、主流程或布局模式的修订 MUST 将审批重新置为 `pending`。审批 pending、原型文件不存在或引用越出 change 目录时，前端实现 MUST 被阻止。

原型形态 SHALL 按交互模式分流：以 `canvas` 为主要模式的界面，原型形态 MUST 为 `states/` 下的状态图（至少空态、编辑中、已确认三态）加一份 `interaction-table.md`（列含 `操作` / `触发` / `前置` / `结果` / `反馈` / `失败`）；其余模式 SHOULD 提供可点 HTML。两类形态均 SHALL 可丢弃，SHALL NOT 引入构建依赖。

`canvas` 类的审批展示与验收比对 MUST 降级为「逐张状态图走查 + `interaction-table.md` 六列逐行对账」，SHALL NOT 要求可交互页面；差异说明的比对基准同此口径。

#### Scenario: 原型待确认时暂停规划
- **WHEN** major change 已生成原型但 `ui-approval` 仍为 `pending`
- **THEN** propose/continue 流程 SHALL 向用户展示原型与待确认决策，并暂停后续会固化交互假设的制品创建

#### Scenario: 用户明确确认后放行
- **WHEN** 用户明确接受当前原型，agent 将 `ui-approval` 更新为 `approved`，且原型引用存在于当前 change 目录
- **THEN** specs、技术 design、tasks 与后续 apply SHALL 可继续

#### Scenario: 重大修订使审批失效
- **WHEN** 已批准原型的信息架构、主流程或布局模式发生变化
- **THEN** `ui-approval` MUST 重置为 `pending`，直至用户重新确认

#### Scenario: 画布类界面强行用可点 HTML 模拟
- **WHEN** `## Interaction Decisions` 把某操作钉为 `canvas`，而原型提供的是一个「点击打开弹窗逐条编辑」的可点 HTML
- **THEN** 该原型 MUST 被判定为偏离契约：弹窗模拟既不是画布的结构表达，也不是状态图形态
