<!-- complexity: medium -->
<!-- ui-impact: none -->

## Why

本 change 不改产品代码，只改「UI 设计怎么被规定」这件事本身。动因是三处结构性缺口，均有实测数据支撑（事实库 30 天窗口 + `openspec/changes/**/ui-prototype/**` 全量扫描）：

**症状一 · 交互模式的词表是偏的，不是 agent 审美不行。**

`docs/reference/standard/frontend/layout.md` 把 `dialog` 规训得最细（`sm=420`/`md=560`/`lg=760`/`xl=1040` 四档 + 3 个 Scenario + 浮层层级契约 + 嵌套抬升规则），而 `canvas` 只有一条「按容器真实像素 1:1 绘制」——**说的是怎么画，没说什么时候该画**。被规训得最细的模式最容易被选中：agent 选弹窗不是偏好，是弹窗在契约里「有据可依」。用户指定画布、agent 交弹窗堆叠，是这条缺口的直接后果（实例：`lane-manual-correction-loop` 需专文写「改名的唯一路径是就地编辑…SHALL NOT 弹对话框」才压住，说明该判据在规范层缺失、只能逐 change 手工打补丁）。

**症状二 · 没有数据来源契约，于是「丰富」变成唯一可交付的丰富度。**

实测：22 个 change 有原型，101 个原型 HTML 里可见文字只占 **0.10%**，`<script>` 占 **82.4%**；单文件最大 11MB（内联整份数据集）。假数据词频 Top-1 是 `add-home-pipeline-status-page`（442 次 / 78 文件，`fake` 163、`示例` 249）。`ui-design.md` 契约里没有任何一处要求说明「这个字段从哪来」，所以原型里凭空造出来的卡片，直到实现阶段才被发现后端没有数据。

**症状三 · 制品没有预算，分批生产直接乘起来。**

`add-home-pipeline-status-page` 原型目录：**796 个文件 / 174MB，其中 61 份 md 共 10,360 行、33 份 html 共 65,600 行、402 张带版本号的截图**。这不是 agent 自嗨——`human-feedback.md` 的 FB-2 是用户下的分批令（「各自把每个模块都做一遍，然后统一让我看看」），`module-batch-plan.md` 是串行 01→09。**分批本身没错，错在分批时没有契约、且把审阅推到最后一刻**：每个子线程拿到一个模块边界，必须独立回答"什么交互、什么数据、怎么说明"，于是每个模块产出一份 `node-spec.md` + 一份 `parts-table.md` + 一批截图。同期的 `rework-workshop-into-radar-and-desk` 攒了 21 个 `revision-*` 目录 / 45MB，而事实库里对应的审批往返只有 1 次——**21 轮改动，人只出现了一次**。

**共性根因**：`ui-design.md` 现在是「八节散文」契约，散文没有上限、不要求数据血缘、不约束交互模式，因此 agent 的每一步"更完整"都在合法地增加体积；而 `ui-design-gate` 只在 `implementation` 档介入，实测 34 次 `ui-design` 档派发 **100% 发生在 `requirements`/无档位**，门禁对原型工作零覆盖。

## What Changes

1. **新增** `docs/reference/standard/frontend/interaction.md`：交互模式固定词表（`page` / `panel` / `dialog` / `drawer` / `inline-edit` / `canvas` / `command`），每类给「何时该用 / 何时禁止 / 本项目实例」；界面文字纪律（只承担动作标签 / 状态结论 / 必要确认反馈三类职责）；交互决策台账形态（`pinned` / `forbidden`）；原型预算与数据血缘要求；画布类交互的「状态图 + 操作表」制品形态。
2. **修改** `docs/reference/standard/frontend/layout.md`：新增「交互模式选择判据」小节并交叉引用 `interaction.md`；原文的画布 1:1 渲染条款保留不动（那是渲染保真，不是模式选择）。
3. **修改** `docs/reference/开发执行规范.md` §3.1：写入 L0–L3 四层制品阶梯（L0 契约 → L1 骨架 → L2 交互 → L3 数据）与对应的 R0–R3 评审阶梯，规定 **R0–R2 禁止业务文案**、每轮人只裁一个决策。
4. **修改** `openspec/specs/ui-design-workflow/spec.md`（经本 change 的 delta）：`ui-design.md` 在八节之外 MUST 含「交互决策台账」「数据 provenance」「明确不做」三节；major 原型形态扩为「可点 HTML **或** 状态图 + 操作表」；原型目录受预算约束。
5. **新增** capability `frontend-interaction-contract` 的 spec 基线（词表与文字纪律的行为契约）。

## Non-Goals

- **不写检查脚本**。本 change 是纯文档级，四条机械判据（文字预算 / forbidden 模式 / provenance 完整性 / 原型体积）本次只落规范文本，由 `change-review` 按清单人工核；脚本化作为后续 change 的钩子写在 `design.md` D6，不在本次实现。理由：`ui-design-gate` 目前只在 `implementation` 档生效，脚本必须挂到 apply 入口 + 归档门禁（照 `ui-verification-trace.sh` 的位置）才有意义，那是一次独立的门禁改造。
- 不改任何 `front/` 下的产品代码、不改 `.pi/extensions/`。
- 不追溯改造历史 change 的 `ui-design.md`。
- 不涉及业务域：本 change 只改前端设计规范与编排纪律，`docs/reference/flow/*.md` 无对应域，故不声明 `constraint-domains`。

## Impact

- 受影响制品：`ui-design.md` 模板结构（八节 → 八节 + 三节）与原型形态；后续所有 `ui-impact: major|minor` 的 change 受约束。
- 不受影响：产品运行时行为、API、数据模型、既有 change 的可继续性（旧 `ui-design.md` 不做硬校验，仅在 review 时提示）。
- 部署后可见行为：**用户界面零变化**。变化只体现在 agent 产出 `ui-design.md` 与原型时的形态。

## 存量处理（与本 change 并行，不在本 change 的 tasks 内）

`add-home-pipeline-status-page` 原型目录按「冻结瘦身、保留契约」处理：整目录已打包至 `~/backups/ui-prototype-add-home-20261115.tar.gz`（160MB / 847 条目，该目录 0 文件入 git，删除不可逆），61 份 md 合并为单份 `node-decisions.md`，其余 md / 重复 html / 版本截图删除。
