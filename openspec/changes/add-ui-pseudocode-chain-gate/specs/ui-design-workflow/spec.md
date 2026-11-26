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
