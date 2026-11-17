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

新 workflow schema 的 apply 指令与 UI 入口门禁 SHALL 在 implementation 档对规划状态做同源检查。major change 未获批准时，门禁 MUST 阻止实现工具对项目代码的写入，以及**具备写能力的实现子线程**派发；门禁 MUST NOT 阻断只读档（勘察/审查）与原型编辑档的派发——只读档（`explore`/`plan`：无写工具；`change-review`：只读工具 + shell，shell 通道为既有边界）不产生前端实现，原型编辑档服务于「修改当前 change 的 UI 设计与原型」这一被允许的路径，且其经 `edit`/`write` 工具的写入仍逐个过门禁（越出当前 change 目录者照常阻止；shell 类通道不在本门禁覆盖范围——既有边界）。门禁 SHALL 允许继续修改当前 change 的 UI 设计与原型；`none` 或 `minor` 满足各自制品要求后正常放行。发布前已存在且仍绑定旧 `spec-driven` schema 的 change SHALL 保持可继续执行，不因缺少新制品被硬阻断，但触及前端时 SHALL 收到一次迁移提示。

#### Scenario: major pending 阻止实现写入
- **WHEN** implementation 档绑定 major change 且审批为 pending，agent 尝试编辑项目代码，或派发具备写能力的实现子线程
- **THEN** 门禁 MUST 阻止操作，说明缺失项，并允许其返回 requirements 档完善 `ui-design.md` 与原型

#### Scenario: major pending 放行只读档与原型编辑档派发
- **WHEN** implementation 档绑定 major change 且审批为 pending，agent 派发只读档（勘察/审查）或原型编辑档
- **THEN** 门禁 MUST NOT 阻断该派发，并按 `ui-dispatch-allowed` 留痕

#### Scenario: 原型编辑档写入越界仍被阻止
- **WHEN** 原型编辑档子线程在合同未就绪期间经 `edit`/`write` 工具写入目标越出当前 change 目录（如 `front/` 下的文件）
- **THEN** 该写入 MUST 被阻止并记 block；写入 change 目录内的 `ui-prototype/**` 与规划制品 MUST 放行

#### Scenario: 完整规划正常放行
- **WHEN** UI 声明合法、`ui-design.md` 满足对应等级，且 major change 已 approved
- **THEN** apply 与实现工具 SHALL 正常放行且不产生成功噪声事件

#### Scenario: 旧 change 不被突然卡死
- **WHEN** rollout 前创建的 change 仍绑定旧 `spec-driven` schema 且缺少 `ui-design.md`
- **THEN** 系统 SHALL 允许继续执行；仅当其触及前端时给出一次迁移提示，不作硬阻断

### Requirement: UI 完成验收必须对照批准基线

major UI change 的完成验收 MUST 同时包含：至少一条 agent-browser（无头浏览器驱动真实浏览器）主交互链路的机械断言、1440×900 与 1920×1080 两档截图的视觉检查，以及实现与已批准 `ui-design.md`/原型的差异说明；若产品明确支持窄屏，还 MUST 增加相应窄屏视口。上述证据 MUST 以 `ui-verification/report.md` 建账（格式见 ui-verification-trace-gate 能力契约），条目点名的截图 MUST 落 `ui-verification/` 目录；「未覆盖」场景 MUST 显式写明原因。minor change 的验收映射 MUST 指名道姓：组件测试须写出真实存在的测试文件路径，agent-browser/人工验证须在 `ui-verification/report.md` 有 ≥1 条判定条目。归档门禁 MUST 以 `scripts/harness/ui-verification-trace.sh` 为单一事实源判定证据齐全性；缺少证据时阻止归档。

#### Scenario: major UI 双层验收通过

- **WHEN** agent-browser 主链路断言通过，两个桌面视口的视觉证据以判定条目建账于 `ui-verification/report.md`，点名截图真实存在，且差异说明已记录
- **THEN** UI 验收 SHALL 通过并可纳入 Scenario→测试映射

#### Scenario: 只有单元测试不得完成 major UI

- **WHEN** major UI change 只有组件单测与编译结果，没有交互链路或视觉证据
- **THEN** 归档门禁 MUST 判定 UI 验收不完整并阻止归档

#### Scenario: 字样齐备但无实物不得通过

- **WHEN** tasks.md 验证节写了 agent-browser 与 1440/1920 字样，但无 `ui-verification/` 目录、report.md 或点名截图
- **THEN** 归档门禁 MUST 依脚本结论阻断归档（关键词字样不再构成放行条件）

#### Scenario: minor 指名映射通过

- **WHEN** minor change 验收节写出真实存在的组件测试文件路径，或 agent-browser/人工验证在 `ui-verification/report.md` 有合法判定条目
- **THEN** minor 验收 SHALL 通过

#### Scenario: minor 裸关键词不再放行

- **WHEN** minor change 验收节仅出现「组件测试/人工验证」等关键词，未挂测试文件路径或建账记录
- **THEN** 归档门禁 MUST 阻断并给出指名道姓格式指引

### Requirement: UI 门禁裁决进入事实库

UI 设计门禁的显著裁决 SHALL 写入 harness `policy.decision`，`policy=ui-design-gate`，action 使用既有 `block|warn|bypass|fail-open` 枚举，reasonCode 使用稳定有界值：`ui-impact-missing`、`ui-impact-mismatch`、`ui-design-missing`、`ui-prototype-missing`、`ui-approval-pending`、`ui-verification-missing`、`ui-dispatch-allowed`、`explicit-bypass`、`ui-gate-check-failed`。合同未就绪时放行的派发 SHALL 记 `action=warn` 且 `reasonCode=ui-dispatch-allowed` 并绑定当前 change；普通健康放行 MUST 零记录；记录失败 MUST NOT 改变原门禁裁决。

#### Scenario: pending 审批阻断被记录
- **WHEN** major change 因 `ui-approval: pending` 被阻止实现
- **THEN** 事实库 SHALL 追加 action=`block`、reasonCode=`ui-approval-pending` 且 change 绑定当前 change 的事件

#### Scenario: 存量提示被记录
- **WHEN** 旧 schema change 触及前端并收到迁移提示
- **THEN** 事实库 SHALL 追加 action=`warn`、reasonCode=`ui-design-missing` 的单次事件

#### Scenario: 合同未就绪时的放行派发被记录
- **WHEN** 合同未就绪且只读档或原型编辑档的派发被放行
- **THEN** 事实库 SHALL 追加 action=`warn`、reasonCode=`ui-dispatch-allowed` 且 change 绑定当前 change 的事件

#### Scenario: 健康放行零记录
- **WHEN** UI 规划与审批均满足要求且实现正常放行
- **THEN** 事实库 MUST NOT 产生 `policy.decision`

### Requirement: 原型生产阶段必须可机器识别且按阶段限制写入域

`ui-design.md` SHALL 支持机器可读标记 `<!-- ui-stage: l0|skeleton|interaction|data|approved -->`，与 `ui-impact` / `ui-approval` / `ui-prototype` 三条 marker 同层同形。标记缺失 SHALL 视为 legacy，SHALL NOT 因此阻断任何写入。

阶段语义为单调阶梯，`approved` 等价「全放行」：

| stage | 允许写入（相对 change 根） |
|---|---|
| `l0` | change 根下任意路径，**除 `ui-prototype/`** |
| `skeleton` | 同上，另放行 `ui-prototype/skeleton/` |
| `interaction` | 全部 |
| `data` | 全部（与 `interaction` 写域相同；区别只在记账意图：表示进入填数阶段） |
| `approved` | 全部，且不记账 |

阶段检查 SHALL 由**写入路径**触发，SHALL NOT 依赖会话档位（`mode`）或 change 绑定：目标路径落在 `openspec/changes/<name>/` 下即 SHALL 触发，change 身份 SHALL 由路径本身解析。

本能力当前 SHALL 只记账、不阻断：标记缺失、取值非法、写入越域三类事件 MUST 记为 `policy=ui-design-gate`、`action=warn` 的事实库事件，SHALL NOT 改变任何既有 `block` / `pass` 裁决。同一会话 × change × reasonCode SHALL 至多记账一次。

将越域记录升级为阻断 SHALL 作为该能力的独立变更处理，并同步扩充阻断码白名单。

#### Scenario: requirements 档写入原型目录也会被记账
- **WHEN** 会话处于 `requirements` 档（或无档位），向 `openspec/changes/<name>/ui-prototype/index.html` 写入
- **THEN** 系统 SHALL 按路径解析出 change 名并记录一条 `action=warn` 事件
- **AND** SHALL NOT 阻断该写入

#### Scenario: 缺少 stage 标记视为 legacy
- **WHEN** change 的 `ui-design.md` 没有 `ui-stage` 标记
- **THEN** 系统 SHALL 记 `reasonCode=ui-stage-missing` 一次
- **AND** SHALL NOT 阻断，SHALL NOT 要求该 change 补标记

#### Scenario: 骨架阶段写业务数据越域
- **WHEN** `ui-stage: skeleton` 的 change 向 `ui-prototype/fixture.json` 写入
- **THEN** 系统 SHALL 记 `reasonCode=ui-stage-scope-out`
- **AND** 本阶段 SHALL NOT 阻断

#### Scenario: 已批准阶段不再记账
- **WHEN** `ui-stage: approved` 的 change 向任意 change 内路径写入
- **THEN** 系统 SHALL NOT 产生 stage 相关事件

#### Scenario: marker 取值非法
- **WHEN** `ui-design.md` 的标记写成 `<!-- ui-stage: design -->` 或同一文件出现两条 `ui-stage`
- **THEN** 系统 SHALL 记 `reasonCode=ui-stage-invalid` 一次
- **AND** SHALL NOT 阻断
