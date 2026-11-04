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
- **THEN** 该写入 MUST 被阻止并记 block；写入 change 目录内的 `ui-prototype/**` 与规划制品 MUST 放行

#### Scenario: 完整规划正常放行
- **WHEN** UI 声明合法、`ui-design.md` 满足对应等级，且 major change 已 approved
- **THEN** apply 与实现工具 SHALL 正常放行且不产生成功噪声事件

#### Scenario: 旧 change 不被突然卡死
- **WHEN** rollout 前创建的 change 仍绑定旧 `spec-driven` schema 且缺少 `ui-design.md`
- **THEN** 系统 SHALL 允许继续执行；仅当其触及前端时给出一次迁移提示，不作硬阻断

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
