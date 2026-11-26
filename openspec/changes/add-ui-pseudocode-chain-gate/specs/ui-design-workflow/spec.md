## MODIFIED Requirements

### Requirement: UI 门禁裁决进入事实库

UI 设计门禁的显著裁决 SHALL 写入 harness `policy.decision`，`policy=ui-design-gate`，action 使用既有 `block|warn|bypass|fail-open` 枚举，reasonCode 使用稳定有界值：`ui-impact-missing`、`ui-impact-mismatch`、`ui-design-missing`、`ui-prototype-missing`、`ui-approval-pending`、`ui-verification-missing`、`ui-dispatch-allowed`、`ui-pseudocode-missing`、`ui-chain-out-of-order`、`explicit-bypass`、`ui-gate-check-failed`。合同未就绪时放行的派发 SHALL 记 `action=warn` 且 `reasonCode=ui-dispatch-allowed` 并绑定当前 change；普通健康放行 MUST 零记录；记录失败 MUST NOT 改变原门禁裁决。

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

## MODIFIED Requirements

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
- **THEN** 该写入 MUST 被阻止并记 block；写入 change 目录内的 `ui-prototype/**` 与规划制品 MUST 放行，**但 `ui-prototype/**` 的写入 SHALL 先过原型产出物依赖链**（见「原型产出物依赖链」：major 且缺前一层产物时按 `ui-chain-out-of-order` 阻断，不因「在 change 目录内」而豁免）

#### Scenario: 完整规划正常放行
- **WHEN** UI 声明合法、`ui-design.md` 满足对应等级，且 major change 已 approved
- **THEN** apply 与实现工具 SHALL 正常放行且不产生成功噪声事件

#### Scenario: 旧 change 不被突然卡死
- **WHEN** rollout 前创建的 change 仍绑定旧 `spec-driven` schema 且缺少 `ui-design.md`
- **THEN** 系统 SHALL 允许继续执行；仅当其触及前端时给出一次迁移提示，不作硬阻断

## ADDED Requirements

### Requirement: 业务先定：major 批准时伪代码节必须在

`ui-impact: major` 且 `ui-approval: approved` 时，`design.md` SHALL 含 `## 核心逻辑（伪代码）` 节。

- 节不存在、或为空，SHALL 阻断为 `ui-pseudocode-missing`
- 节写 `N/A：<理由>` 时 SHALL 通过（理由至少有实质内容，`N/A：` 后为空则阻断）
- `ui-approval: pending` 期 MUST NOT 检查本条（原型探索期允许业务口径边改边定）
- `ui-impact: none|minor` MUST NOT 检查本条

理由的**正确性**不判——机器只判「有没有写」，理由成立与否归 review 与看图的人。

#### Scenario: 批准时缺伪代码

- **WHEN** major 档 `ui-approval` 从 pending 改为 approved，但 `design.md` 无伪代码节
- **THEN** 后续写入 SHALL 被阻断，reasonCode=`ui-pseudocode-missing`，并提示「业务口径必须先定稿」

#### Scenario: N/A 需要理由

- **WHEN** `## 核心逻辑（伪代码）` 节内容为 `N/A：`（无理由）
- **THEN** SHALL 判 `ui-pseudocode-missing`；写 `N/A：只改布局常量，无控制流变化` 则通过

#### Scenario: pending 期不查

- **WHEN** major 档仍为 pending，且 `design.md` 无伪代码节
- **THEN** 阻断原因为 `ui-approval-pending`（不是 `ui-pseudocode-missing`）——审批优先

### Requirement: 原型产出物依赖链

`ui-impact: major` 时，写入 `ui-prototype/**` SHALL 按产出物顺序放行：

| 顺序 | 产出物 | 判据 |
|---|---|---|
| L1 | `ui-prototype/skeleton/` ≥1 文件 | 写非 skeleton 文件前必须已在 |
| L2 | `ui-prototype/index.html` | 写 `fixture.json` 前必须已在 |

缺前一层 SHALL 阻断为 `ui-chain-out-of-order`，提示缺哪一层与怎么补。

判据 MUST 基于**磁盘上的文件存在性**，MUST NOT 依赖 agent 自报的阶段 marker（无申报即无法判定）。`ui-impact: none|minor` MUST NOT 受本条约束。

出口：`ui-design.md` 写 `<!-- ui-skip: skeleton(理由) -->` 或 `<!-- ui-skip: data(理由) -->` 且理由非空时，SHALL 豁免对应环节；理由为空 MUST NOT 豁免。

#### Scenario: 越级写完整原型

- **WHEN** major 档 `ui-prototype/skeleton/` 为空，写入 `ui-prototype/index.html`
- **THEN** SHALL 阻断，reasonCode=`ui-chain-out-of-order`，提示先产出骨架

#### Scenario: 骨架自身不设前置

- **WHEN** 写 `ui-prototype/skeleton/index.html`
- **THEN** SHALL 放行（否则第一块骨架永远写不进去）

#### Scenario: 数据需要原型宿主

- **WHEN** `ui-prototype/index.html` 不存在，写入 `ui-prototype/fixture.json`
- **THEN** SHALL 阻断，提示先出可点原型

#### Scenario: 显式跳过

- **WHEN** `ui-design.md` 含 `<!-- ui-skip: skeleton(纯样式微调，无新增区块) -->`
- **THEN** 写 `ui-prototype/index.html` SHALL 放行；该 marker 理由为空时 MUST NOT 豁免

#### Scenario: minor 不被骨架链约束

- **WHEN** `ui-impact: minor` 的 change 写 `ui-prototype/index.html` 且无 skeleton
- **THEN** MUST NOT 因依赖链阻断
