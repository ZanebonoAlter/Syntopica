## Context

见 `proposal.md - Why`。当前默认 `spec-driven` schema 只有 `proposal → specs/design → tasks` 四类制品，`openspec/config.yaml` 也没有 UI 专用规则。`docs/reference/开发执行规范.md` §3 将强制原型限定在复杂逻辑，§5.3 的 opencli/k3 位于实现后验收；entry-gate 只在 implementation 回合末检查复杂档白盒用例。

`add-composite-labels` 是直接证据：规划制品完整描述了组合标签的数据、匹配和迁移，但前端只有“列表/创建/启停/过滤”等功能枚举，没有页面入口、主任务、状态流、布局模式或视觉确认；实际 `TagsPage` 主内容全宽，组合标签池直接撑满，创建弹窗又自写 Teleport 和 560px 宽度而未复用 `AppDialog`。事实库保留窗口内 130 次子线程派发仅 8 次描述命中 UI/视觉/原型；92 个出现前端门禁的 session 中只有 4 个同时出现视觉类派发，说明视觉前置尚未成为稳定流程。

本 change 横跨 OpenSpec schema、apply 编排、pi 门禁、事实库和前端规范；不声明业务 flow 域，不改变产品数据或业务行为。

## Goals / Non-Goals

**Goals:**

- 让 UI 影响在 proposal 时机械显式化，避免“逻辑不复杂但界面是新设计”的漏网。
- 让交互与布局在 specs/技术 design 固化前可视、可审查，major UI 必须获得用户确认。
- 给页面 shell、内容宽度和 dialog 尺寸建立少而稳定的选择集合。
- 在 apply 和 archive 两端建立机器门禁，并将显著裁决写入事实库。
- 对 rollout 前的旧 change 保持兼容，不突然阻断正在进行的工作。

**Non-Goals:**

- 本 change 不重做 `add-composite-labels` 或其他存量页面视觉；后续按新流程另开 UI 修订 change。
- 不引入 Storybook、Figma、远端设计服务或新的前端运行时依赖。
- 不把每个 minor UI 修改都升级为人工审批；人工确认只对 major 强制。
- 不统一迁移仓库里全部历史自由宽度；仅约束新建或主动重构的界面。
- 不让视觉模型替代机械交互断言，也不让 opencli 代替主观视觉判断。

## Decisions

### D1: fork 项目本地 schema，新增始终存在的 `ui-design` 制品

- **选择**：执行 `openspec schema fork spec-driven syntopica-ui`，形成项目内可版本化 schema；将依赖图改为：

  ```text
  proposal
      │
      ▼
  ui-design
    ├──────► specs ──┐
    └──────► design ─┼──► tasks ──► apply
                     ┘
  ```

  `ui-design` 始终生成 `ui-design.md`：none 为最小 N/A，minor 为轻量契约，major 为完整设计与原型审批。schema 的 apply instruction 在开始实现前再次读取该制品。
- **理由**：条件跳过 artifact 会让 OpenSpec 的静态依赖仍处于 blocked，状态难以解释；始终存在但按等级缩放，既能让依赖图稳定，也把 none 的成本压到几行。
- **备选（否决）**：只在 `design.md` 加 UI 章节——技术决策容易淹没交互决策，且无法在 specs 前形成独立审批点。
- **备选（否决）**：UI artifact 可选——依赖图与状态无法机械表达，仍会退回“记得就做”。

### D2: `ui-impact` 使用单一三级声明，并与 complexity 正交

- **选择**：proposal 头使用 `<!-- ui-impact: none|minor|major -->`；判定主信号是是否新增界面结构或交互模式，而不是代码行数。
  - none：无用户可见界面变化；
  - minor：在既有结构内增加文案、字段或单步动作；
  - major：新页面/面板/弹窗/导航、多步流程、信息架构或布局模式变化。
- **理由**：`complexity` 解决测试设计强度，`ui-impact` 解决交互设计强度。像新增治理面板可以算法简单但 UI major，不能再共用一个维度。
- **反向校验**：proposal/tasks 出现前端路径或 UI 关键词而声明 none，或 major 特征却声明 minor，门禁报告 mismatch。词表只做兜底，声明是主信号，避免无限扩关键词。
- **备选（否决）**：多布尔标签（layout/dialog/navigation 等）——表达精细但 proposal 成本高，且 gate 组合爆炸；细分信息放 `ui-design.md`。

### D3: `ui-design.md` 既是设计合同，也是审批状态的唯一持久化来源

模板固定头部：

```md
<!-- ui-impact: major -->
<!-- ui-approval: pending -->
<!-- ui-prototype: ui-prototype/index.html -->

## User Journey
## Information Architecture
## Interaction Contract
## State Matrix
## Layout Contract
## Component Reuse
## Prototype
## Acceptance
```

- none：保留三条 marker，approval=`not-required`、prototype=`none`，正文只写不适用理由。
- minor：写入口、受影响状态、复用组件、布局模式和验收；approval=`not-required`，prototype 可为 none。
- major：完整填写，并在 `ui-prototype/` 放静态 fixture 驱动的独立 HTML 原型；不连接真实 API、不修改产品代码。agent 用浏览器展示原型，只有用户明确确认后才将 marker 改为 approved。
- 重大修订（信息架构、主流程、布局模式）必须重置 pending；颜色微调和文案修正可记录差异而不重审。
- **理由**：对话确认会随 session 丢失；marker + change 内原型可被 resume、review、archive 和门禁共同读取。
- **备选（否决）**：审批只记在对话或事实库——对话不稳定，事实库有 TTL 且不是版本化设计依据。

### D4: 布局规则收敛为 page shell 与 dialog size tier

新增 `docs/reference/standard/frontend/layout.md`，并提供可复用实现：

| 模式 | 默认约束 | 典型用途 |
|---|---|---|
| reader | max 760px，居中 | 长文、报告正文 |
| contained | max 1120px，居中 | 设置、治理、表单、普通列表 |
| workspace | 填满可用宽度 | 看板、时间线、带常驻侧栏工作台 |
| split | workspace 基础上显式主从栏 min/max | 列表+详情、编辑器 |

Dialog 提供 `sm=420`、`md=560`、`lg=760`、`xl=1040`，统一限制为可视区宽度（目标 92vw）。`AppDialog` 新增 size API 并保留旧 width prop 兼容存量；新代码只能选 size，例外必须在 UI design 说明。page shell 优先做 `AppPageShell` 或同等单一入口，不允许每个页面复制 max-width。

- **理由**：当前主题系统统一了颜色，却没统一空间；把“居中还是宽屏”变为显式枚举，才能在原型和实现之间稳定传递。
- **备选（否决）**：只给推荐数字——业务组件仍会各写一套，无法 review 或复用。
- **备选（否决）**：立即迁移全部页面——范围过大且会混入大量用户可见变化。

### D5: schema apply 前置检查为主，pi 硬门禁为后备

采用两层执行：

1. **正常路径**：custom schema 的 apply instruction 要求先运行 UI 检查；major pending 时保持 requirements 档，提示展示/确认原型，不生成实现计划、不派发实现子线程。
2. **逃逸后备**：新增 `ui-design-gate` pi extension。在 implementation 档绑定新 schema change 时，拦截 Agent 实现派发与项目文件 mutation；仅允许继续修改该 change 的 `ui-design.md`、`ui-prototype/**` 及切回 requirements 所需操作。检查异常 fail-open，但必须记账并明显告警。

门禁判定抽到纯函数库，输入为 schema 名、proposal、ui-design、change 文件清单、mode 与目标工具；extension 只负责读上下文、调用判定、返回 block/custom message。对 bash 这类难以可靠判定写路径的工具以 apply 前置检查为主，不尝试实现通用 shell 解析器。

- **理由**：只写规范容易被忘，单靠 tool hook 又无法覆盖所有 shell 变体；正常编排 + 硬后备能以有限复杂度覆盖主要入口。
- **兼容**：旧 `.openspec.yaml` 仍绑定 spec-driven，不硬阻断；其触及前端时每 session/change 最多提示一次可迁移到新 UI 制品。
- **逃生口**：保留显式 `UI_DESIGN_GATE_BYPASS=1`，只用于门禁故障或紧急修复，放行必须留 policy.decision，不能默默绕过。

### D6: archive 检查验证证据，而非仅检查 marker

major UI 完成条件固定为：

- opencli 主交互路径，断言从 state/get 原始值机械判定；
- 1440×900 和 1920×1080 截图，由视觉子代理按布局、层级、溢出、主题和与原型差异输出结构化结论；
- tasks 验证节记录命令/证据路径与预期结果；
- `ui-design.md` Acceptance 记录实现差异，重大差异需重新审批。

spec-gate 在现有 doc-impact、standards、Scenario trace 等检查之外追加 UI 检查。minor 至少映射到组件测试、opencli 或人工验证；none 只校验 N/A 一致性。

- **理由**：lint/typecheck/unit test 只能证明代码健康，不能证明宽屏构图或任务流正确；视觉检查也不能替代数据与 DOM 断言。
- **备选（否决）**：所有 UI 都写 Playwright 回归——项目已有结论是复杂 SPA 交互脚本维护成本过高；继续用 opencli 现写现跑，CI 只留稳定 smoke。

### D7: 显著裁决复用 `policy.decision`，普通放行零记录

在 `harden-harness-policy-and-spill` 先归档的前提下，UI gate 使用 `policy=ui-design-gate` 与既有 action 枚举；reasonCode 固定为 spec 中的八个值。block/warn/bypass/fail-open 全记，健康通过不记，change 优先绑定明确目标。

- **理由**：需要回答“哪些前端 change 没做设计、为何被阻断、是否绕过”，但不能让每次正常工具调用淹没事实库。
- **依赖处理**：apply 开工先确认 `harden-harness-policy-and-spill` 已归档；若尚未归档，先完成它或将其已定义的 policy 事件契约同步为本 change 的前置，不并行改写同一主 spec。

## Risks / Trade-offs

- [所有 change 多一个 artifact，可能增加仪式成本] → none 只写机器 marker 与一句理由；minor 不强制原型，成本与影响等级成比例。
- [agent 可能把 major 错报 minor] → proposal/tasks 关键词与前端路径做反向质询，review 与 archive 再校验；不靠关键词直接自动升级以控制误报。
- [用户审批导致 one-step propose 暂停] → 这是刻意的产品决策点；none/minor 仍可一次完成，major 展示原型后等待一次明确确认再继续。
- [原型与真实组件风格漂移] → 原型必须引用布局 token、组件复用映射和现有 editorial/magazine 方向；实现验收记录差异。
- [硬门禁误伤旧 change] → 仅对新 schema 硬执行，旧 schema 只提示；提供显式、留痕的紧急 bypass。
- [tool_call 无法可靠识别任意 bash 写盘] → schema apply preflight 是主门，hook 只覆盖结构化 mutation 与 Agent 派发，不构造脆弱 shell parser。
- [与正在进行的 policy 事实库 change 冲突] → 将其归档列为实现前置，避免两个 active change 同时改同一事件词汇主 spec。
- [固定宽度档不适合少数特殊工作台] → workspace/split 保留弹性，例外允许但必须在 UI design 写理由和视口证据。

## Migration Plan

1. 确认并优先归档 `harden-harness-policy-and-spill`，锁定 `policy.decision` 契约。
2. fork `spec-driven` 为项目本地 `syntopica-ui` schema，新增 `ui-design` artifact/template、依赖边和 apply preflight；运行 schema validate。
3. 用临时 none/minor/major change 做 schema smoke：验证 artifact 顺序、最小 N/A、major pending 暂停和 approved 放行；清理临时 change。
4. 实现 UI 影响解析纯函数、pi `ui-design-gate` 与 smoke/unit tests，并同步 harness 代码快照和事实库文档。
5. 新增 layout 标准与 page shell/dialog size API；补组件测试，不迁移现有业务页面。
6. 更新开发执行规范、AGENTS 速查、ui-verify/frontend-design 使用边界和归档检查。
7. 将 `openspec/config.yaml` 默认 schema 切到 `syntopica-ui`；已有 change 继续读取各自 `.openspec.yaml` 的旧 schema，新 change 使用新流程。
8. 运行端到端门禁 smoke、OpenSpec strict validate、文档影响校验后归档。

回滚时将 `openspec/config.yaml` 默认 schema 改回 `spec-driven` 并停用 UI gate extension；项目本地 schema、`ui-design.md` 与布局组件可保留，不影响旧 change 或产品运行。事实库已有裁决为 append-only 历史，不删除。
