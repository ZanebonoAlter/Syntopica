## ADDED Requirements

### Requirement: 浮层层级契约

系统 SHALL 为浮层提供**唯一档位表**（`front/app/components/ui/layout-contract.ts` 的 `Z_INDEX`，权威源 `docs/reference/standard/frontend/layout.md`「浮层层级契约」节），组件 SHALL NOT 自行手写层级数字（存量例外见下）。

档位与规则：

- 基础弹窗遮罩 = `Z_INDEX.dialog`（1000）。
- 弹窗内再开的浮层（嵌套浮层）层级 SHALL **严格高于其宿主弹窗遮罩**——否则被宿主遮罩整体盖住，用户点了入口看不到浮层。
- 嵌套浮层默认取 `Z_INDEX.dialogNested`（1100）；宿主弹窗处于更高档时，宿主 SHALL 用 `nestedOverlayZ(hostZ)` 抬升并显式传入——返回值 SHALL 严格大于 hostZ，SHALL **不落在任何已占档位上**（档位表是唯一占用清单；裸抬升撞上「弹窗内全屏层」9200 时 SHALL 继续抬升跳过该档），且 SHALL 不越过全局提示档 `Z_INDEX.toast`。
- 全屏层 SHALL 高于其宿主弹窗遮罩，且低于全局提示层（`Z_INDEX.toast`）。
- **存量例外**：已入库组件的历史手写层级数字保持现状（登记表见 `standard/frontend/layout.md`「存量迁移」节），不随本契约强制迁移；主动重构该组件时 SHALL 一并收敛到档位表。

#### Scenario: 弹窗内嵌套浮层高于宿主遮罩

- **WHEN** 用户在弹窗内（版块治理工作面等）打开一个嵌套浮层（组合标签「创建并挂载」弹窗）
- **THEN** 嵌套浮层的 z-index SHALL 严格大于宿主弹窗遮罩的 z-index，渲染在宿主之上，不被宿主遮罩遮挡

#### Scenario: 嵌套浮层档位可按宿主抬升

- **WHEN** 宿主弹窗使用高于基础档的层级（如阅读层弹窗 9100）
- **THEN** `nestedOverlayZ(hostZ)` SHALL 返回严格大于 hostZ 的层级，供宿主显式传给嵌套浮层
- **AND** 返回值 SHALL 不等于任何其它已占档位（如 `readingFullscreen` 9200），且 SHALL 不超过 `Z_INDEX.toast`

#### Scenario: 层级档位单一事实源

- **WHEN** 新增或修改任一浮层组件
- **THEN** 其层级 SHALL 取自 `Z_INDEX` 档位表（组件内不得手写层级数字），且 `AppDialog` 默认层级 SHALL 等于 `Z_INDEX.dialog`

## MODIFIED Requirements

### Requirement: 统一对话框外壳 AppDialog

新弹窗 SHALL 复用统一外壳 `AppDialog`，尺寸经 `size` 属性四档选择，SHALL 受可视区宽度约束（92vw 上限），SHALL NOT 传自由 width。遮罩层级 SHALL 取 `Z_INDEX.dialog` 档位（不接受组件内手写数字）；需要高于基础档（阅读层等）时经 `z-index` prop 显式指定，并 SHALL 高于宿主浮层。

#### Scenario: 新弹窗使用四档尺寸

- **WHEN** 需要新增一个弹窗
- **THEN** 实现 MUST 复用 `<AppDialog size="...">` 四档之一，不传自由 width（与 `standard/frontend/layout.md` dialog 契约一致）

#### Scenario: 主题自动适配

- **WHEN** 在 editorial/dark 任一主题下打开 AppDialog
- **THEN** overlay 与内容 SHALL 经 Layer 2 语义 token 自动适配当前主题

#### Scenario: 默认遮罩层级取基础档

- **WHEN** 使用 `AppDialog` 未显式传 `z-index`
- **THEN** 遮罩层级 SHALL 等于 `Z_INDEX.dialog`（1000），与层级档位表一致
