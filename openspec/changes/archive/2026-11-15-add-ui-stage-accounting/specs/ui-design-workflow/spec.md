# ui-design-workflow — 原型生产阶段可识别 delta

## ADDED Requirements

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
