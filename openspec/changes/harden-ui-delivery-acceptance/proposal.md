<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

工坊改版第二轮复盘发现：证据脚本在 53 pass / 1 fail / 2 未覆盖时仍返回通过，因为它只负责留痕齐备，却被当作交付合格；已有展示/迁移规范也未约束动作作用域和复用组件的宿主责任。需要以本次改版为试点，分清“设计职责、证据存在、验收结果”三层，防止再次用补按钮和截图文件自证完成。

## What Changes

- 在现有 UI 设计八节内补动作作用域、主入口/快捷入口理由、组件宿主与滚动/返回契约；视图替换须对账信息语义和用户决策，不只对账组件名称。规范仍由既有 reference 文件承接，不另建平行规范体系。
- 增加可机械判定的 UI 交付结果检查：必验故事必须有最新有效结论；已知失败和核心未覆盖不能因证据存在而放行；修复须指回原失败并给复验依据，保留失败历史。
- 对设计合同明确承诺的视口建立可核对的证据映射，检查实际图片尺寸和具体场景关联，不只匹配文件名；产品定位、美感和语义等价仍由人工/独立评审负责，不交给脚本猜。
- 保留 `ui-verification-trace.sh` 的证据存在性职责；交付结果判定由归档自查与硬门禁共享单一事实源，不在扩展中复制解析逻辑。
- 以 `rework-workshop-into-radar-and-desk` 显式接入新结果合同，验证现有已知失败会被阻断、修复复验后可解除。采用有版本的合同逐步迁移：新 major 规划写入新合同，其他未迁移存量不因旧格式突然卡死；旧 `spec-driven` 保持兼容。具体合同形态在 design/specs 明确，不允许执行方自行写“用户已豁免”绕过核心问题。
- 补定向 smoke：两种账本形态、失败/未覆盖、重复/非法条目、复验关系、视口/路径边界、存量兼容与自查/硬门禁一致性。

## Capabilities

### New Capabilities
- `ui-delivery-acceptance`: 结果合同、必验故事与失败复验、目标视口证据和试点迁移的只读交付检查。

### Modified Capabilities
- `ui-design-workflow`: UI 规划补产品职责与宿主契约；完成验收区分证据齐备与结果合格；规范新规划合同与存量迁移边界。
- `ui-verification-trace-gate`: 澄清痕迹检查仅判证据齐备，与交付结果检查组合而非替代；归档自查与硬门禁同源集成。

## Impact

- 脚本/扩展：`scripts/harness/` 的 UI 验收和归档自查、`.pi/extensions/spec-gate.ts` 及相关定向 smoke；必要解析逻辑采用单一实现。
- 模板：`openspec/schemas/syntopica-ui/` 的 UI 规划指令/模板（按真实结构定位）。
- 权威文档：`docs/reference/开发执行规范.md`、`standard/frontend/layout.md`、`standard/shared/test-design.md`（含 JIT 摘要）、`standard/shared/doc-authoring.md`、`harness/pi-extensions.md` 按实际影响同步。
- 试点：工坊变更的交付合同与验收账本；产品原型修订和产品代码修复仍归原工坊 change，不把业务变更混入本流程 change。
- 不新增第三方工具，不改业务数据库或 API，不重新生成业务数据，不自动归档或部署。发布后开发者在归档试点/新合同 major 时会受到结果检查；用户页面行为由原工坊返工另行交付。
