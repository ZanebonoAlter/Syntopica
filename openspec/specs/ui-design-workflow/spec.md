# ui-design-workflow Specification

## Purpose

把用户可见界面的交互与视觉决策前移为可审查、可阻断、可追溯的 OpenSpec 契约，使前端实现开始前已有明确原型与布局基线，完成后能按同一基线验证而非临场发挥。

## Requirements


### Requirement: change 必须声明 UI 影响等级

使用项目新 workflow schema 创建的 change SHALL 在 proposal 头部携带且仅携带一个 `<!-- ui-impact: none|minor|major -->` 声明；该声明与 `complexity` 独立。`none` 仅用于不改变用户可见界面的 change；`minor` 用于在既有页面结构和交互模式内增加文案、字段或单步操作；新增页面、面板、弹窗、导航、多步流程、信息架构或布局模式变化 MUST 声明为 `major`。声明缺失、非法或与 change 内容明显矛盾时，系统 MUST 阻止进入实现并给出修复提示。

#### Scenario: 纯后端 change 声明 none
- **WHEN** change 只修改后端内部处理且 proposal 声明 `ui-impact: none`
- **THEN** UI 规划流程 SHALL 接受该声明，并允许以 N/A 形式完成 UI 设计制品

#### Scenario: 新面板必须声明 major
- **WHEN** proposal 或 tasks 包含新增治理面板、弹窗或多步交互，但声明为 `none` 或 `minor`
- **THEN** UI 入口门禁 MUST 报告影响等级不匹配并阻止实现

#### Scenario: 声明缺失或非法
- **WHEN** 新 schema change 的 proposal 缺少 `ui-impact`，或值不属于 `none|minor|major`
- **THEN** UI 入口门禁 MUST 阻止实现并指出合法值

### Requirement: ui-design 作为必经规划制品

项目 OpenSpec workflow SHALL 在 proposal 之后、specs 与技术 design 之前生成 `ui-design.md`，且 tasks MUST 依赖 specs、技术 design 与 UI 设计均完成。`ui-design.md` MUST 记录 UI 影响等级、页面入口、用户主任务、交互状态、布局类型、组件复用、响应式范围、原型路径和审批状态；`none` 可用带理由的最小 N/A 记录，`minor` MUST 写清现有模式的复用点与状态变化，`major` MUST 写完整信息架构、操作流程和状态矩阵。

#### Scenario: none 产生最小制品
- **WHEN** proposal 声明 `ui-impact: none`
- **THEN** `ui-design.md` SHALL 记录 N/A 理由、`ui-approval: not-required` 与 `ui-prototype: none`，无需制作原型

#### Scenario: minor 复用既有交互
- **WHEN** change 声明 `ui-impact: minor`
- **THEN** `ui-design.md` SHALL 指明所复用的页面、组件和布局模式，并覆盖 loading、empty、error、success 中受影响的状态，无需强制可视原型

#### Scenario: major 形成完整 UI 契约
- **WHEN** change 声明 `ui-impact: major`
- **THEN** `ui-design.md` MUST 包含页面入口、主次任务、信息层级、主次/危险操作、完整状态矩阵、布局模式、目标视口、组件复用映射和可丢弃原型引用

### Requirement: major UI 原型必须获得用户确认

major UI change MUST 在 change 目录的 `ui-prototype/` 中提供与真实接口解耦的可丢弃原型，并在 `ui-design.md` 使用机器可读 `<!-- ui-approval: pending|approved -->` 与 `<!-- ui-prototype: <relative-path> -->` 记录状态。只有用户在对话中明确确认后，agent 才可将审批改为 `approved`；任何改变信息架构、主流程或布局模式的修订 MUST 将审批重新置为 `pending`。审批 pending、原型文件不存在或引用越出 change 目录时，前端实现 MUST 被阻止。

#### Scenario: 原型待确认时暂停规划
- **WHEN** major change 已生成原型但 `ui-approval` 仍为 `pending`
- **THEN** propose/continue 流程 SHALL 向用户展示原型与待确认决策，并暂停后续会固化交互假设的制品创建

#### Scenario: 用户明确确认后放行
- **WHEN** 用户明确接受当前原型，agent 将 `ui-approval` 更新为 `approved`，且原型引用存在于当前 change 目录
- **THEN** specs、技术 design、tasks 与后续 apply SHALL 可继续

#### Scenario: 重大修订使审批失效
- **WHEN** 已批准原型的信息架构、主流程或布局模式发生变化
- **THEN** `ui-approval` MUST 重置为 `pending`，直至用户重新确认

### Requirement: 页面布局与弹窗尺寸采用统一契约

前端标准 SHALL 定义并要求新界面主动选择布局模式：`reader`（正文最大宽 760px）、`contained`（治理/设置内容最大宽 1120px 并居中）、`workspace`（利用全部可用宽度）、`split`（显式定义主从栏）；弹窗 SHALL 使用 `sm=420px`、`md=560px`、`lg=760px`、`xl=1040px` 四档并受可视区宽度约束。新建或重构界面 MUST 复用统一 page shell、dialog、button、input 等基础组件；需要自由宽度时 MUST 在 `ui-design.md` 记录理由和目标视口。

#### Scenario: 治理列表选择 contained
- **WHEN** 新增以浏览、筛选和治理为主且无需多栏联动的列表页面
- **THEN** UI 设计 SHALL 选择 `contained`，内容在宽屏居中且不随父容器无限拉宽

#### Scenario: 工作台选择 workspace 或 split
- **WHEN** 页面包含常驻侧栏、时间线或列表与详情联动
- **THEN** UI 设计 SHALL 选择 `workspace` 或 `split`，并记录各栏最小宽度与溢出策略

#### Scenario: 弹窗拒绝随手宽度
- **WHEN** 新弹窗可由四档尺寸之一容纳
- **THEN** 实现 MUST 复用统一弹窗尺寸档，不得在业务组件中另写自由 `width` 或重复 overlay

### Requirement: 实现入口门禁与存量兼容

新 workflow schema 的 apply 指令与 UI 入口门禁 SHALL 在 implementation 档对规划状态做同源检查。major change 未获批准时，门禁 MUST 阻止实现工具对项目代码的写入和实现子线程派发，同时允许继续修改当前 change 的 UI 设计与原型；`none` 或 `minor` 满足各自制品要求后正常放行。发布前已存在且仍绑定旧 `spec-driven` schema 的 change SHALL 保持可继续执行，不因缺少新制品被硬阻断，但触及前端时 SHALL 收到一次迁移提示。

#### Scenario: major pending 阻止实现写入
- **WHEN** implementation 档绑定 major change 且审批为 pending，agent 尝试编辑项目代码或派发实现子线程
- **THEN** 门禁 MUST 阻止操作，说明缺失项，并允许其返回 requirements 档完善 `ui-design.md` 与原型

#### Scenario: 完整规划正常放行
- **WHEN** UI 声明合法、`ui-design.md` 满足对应等级，且 major change 已 approved
- **THEN** apply 与实现工具 SHALL 正常放行且不产生成功噪声事件

#### Scenario: 旧 change 不被突然卡死
- **WHEN** rollout 前创建的 change 仍绑定旧 `spec-driven` schema 且缺少 `ui-design.md`
- **THEN** 系统 SHALL 允许继续执行；仅当其触及前端时给出一次迁移提示，不作硬阻断

### Requirement: UI 完成验收必须对照批准基线

major UI change 的完成验收 MUST 同时包含：至少一条 opencli 主交互链路的机械断言、1440×900 与 1920×1080 两档截图的视觉子代理检查，以及实现与已批准 `ui-design.md`/原型的差异说明；若产品明确支持窄屏，还 MUST 增加相应窄屏视口。minor change SHALL 至少映射到组件测试、opencli 或明确的人工验证之一。缺少 major UI 验收证据时，归档门禁 MUST 阻止归档。

#### Scenario: major UI 双层验收通过
- **WHEN** opencli 主链路断言通过，两个桌面视口的视觉报告无阻断项，且差异说明已记录
- **THEN** UI 验收 SHALL 通过并可纳入 Scenario→测试映射

#### Scenario: 只有单元测试不得完成 major UI
- **WHEN** major UI change 只有组件单测与编译结果，没有交互链路或视觉证据
- **THEN** 归档门禁 MUST 判定 UI 验收不完整并阻止归档

### Requirement: UI 门禁裁决进入事实库

UI 设计门禁的显著裁决 SHALL 写入 harness `policy.decision`，`policy=ui-design-gate`，action 使用既有 `block|warn|bypass|fail-open` 枚举，reasonCode 使用稳定有界值：`ui-impact-missing`、`ui-impact-mismatch`、`ui-design-missing`、`ui-prototype-missing`、`ui-approval-pending`、`ui-verification-missing`、`explicit-bypass`、`ui-gate-check-failed`。普通健康放行 MUST 零记录；记录失败 MUST NOT 改变原门禁裁决。

#### Scenario: pending 审批阻断被记录
- **WHEN** major change 因 `ui-approval: pending` 被阻止实现
- **THEN** 事实库 SHALL 追加 action=`block`、reasonCode=`ui-approval-pending` 且 change 绑定当前 change 的事件

#### Scenario: 存量提示被记录
- **WHEN** 旧 schema change 触及前端并收到迁移提示
- **THEN** 事实库 SHALL 追加 action=`warn`、reasonCode=`ui-design-missing` 的单次事件

#### Scenario: 健康放行零记录
- **WHEN** UI 规划与审批均满足要求且实现正常放行
- **THEN** 事实库 MUST NOT 产生 `policy.decision`
