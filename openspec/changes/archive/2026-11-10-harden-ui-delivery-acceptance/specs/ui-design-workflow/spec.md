## ADDED Requirements

### Requirement: UI 设计说明作用域与复用宿主

major 的既有八节 UI 合同 MUST 在 Information Architecture / Interaction Contract 中说明动作实际作用域、主入口、必要快捷入口及理由与操作后去向；不得把全局动作无说明地伪装成当前资源的治理。Component Reuse MUST 说明复用组件的宿主形态、宽度/滚动归属、关闭/返回/焦点契约，不得把内联组件名称当作已完成浮层迁移。图、矩阵、列表等视图替换 MUST 对账原信息语义和用户决策，新形态存在不证明旧能力等价。基础控件可以采用原生语义，但主题、密度、焦点与打开态须符合产品合同，不一刀切禁止原生 HTML。

#### Scenario: 全局动作与重复入口有职责
- **WHEN** major 重新安排全局动作或给同一能力设置多个入口
- **THEN** 合同 MUST 列明真实作用域、主入口及快捷入口理由；独立 review MUST 按真实用户任务核验，不以多个按钮可点判定定位正确

#### Scenario: 内联组件复用到新工作面
- **WHEN** 旧内联组件被迁入抽屉、弹层或新页面壳
- **THEN** 合同 MUST 指明其宿主/滚动/返回责任，验收 MUST 检查打开后当前视口可见可点，不能只验证组件或 API 存在

#### Scenario: 视图替换保留信息语义
- **WHEN** 新矩阵或列表替换原图关系浏览
- **THEN** 设计与 review MUST 分别对账不同关系语义与原用户故事，不能用一种连续性展示替代全部关系而未说明承接或重新批准退役

## MODIFIED Requirements

### Requirement: UI 完成验收必须对照批准基线

major UI change 的完成验收 MUST 同时包含：至少一条 agent-browser 主交互链路机械断言、1440×900 与 1920×1080 两档截图视觉检查，以及实现与已批准 ui-design/原型的差异说明；合同另行承诺的宽屏/窄屏及打开态 MUST 有对应场景证据。上述证据 MUST 以 `ui-verification/report.md` 建账，点名截图在 `ui-verification/` 内真实存在，未覆盖 MUST 有原因。minor 的组件测试/人工映射仍须指名真实文件或合法条目。

归档检查 MUST 区分证据齐备与交付结果：`ui-verification-trace.sh` 默认模式只判证据存在；归档自查与硬门禁 MUST 调用同一 `--delivery` 入口。适用 v1 的 major MUST 依 ui-delivery-acceptance 对必验故事、最新结论、失败复验及逐场景视口做结果判定；已知失败或核心未覆盖不得被证据存在抵消。新规划模板 MUST 引导 major 声明版本合同和 Acceptance 的唯一必验表；存量迁移边界按 ui-delivery-acceptance 执行。图片尺寸/引用通过不替代评审对截图内容、操作合理性和真实故事的核验。

#### Scenario: major UI 双层验收通过
- **WHEN** 主链路断言、所需视口视觉证据与原型差异说明齐备，且适用的交付合同全部通过
- **THEN** UI 验收 SHALL 可纳入 Scenario→测试映射，不能仅凭默认 trace 成功宣称交付完成

#### Scenario: 只有单元测试不得完成 major UI
- **WHEN** major 只有组件单测与编译结果，没有交互链路或视觉证据
- **THEN** 归档门禁 MUST 判定 UI 验收不完整并阻止归档

#### Scenario: 字样齐备但无实物不得通过
- **WHEN** tasks 只有 agent-browser 和视口字样，没有 report 或点名截图
- **THEN** 归档门禁 MUST 按脚本结论阻断

#### Scenario: minor 指名映射通过
- **WHEN** minor 映射的组件测试文件真实存在，或人工验证有合法判定条目
- **THEN** minor 证据验收 SHALL 通过，不要求 major 的 v1 合同

#### Scenario: minor 裸关键词不再放行
- **WHEN** minor 验收只有组件测试/人工等关键词而没有指名实物
- **THEN** 归档门禁 MUST 阻断并给格式指引

#### Scenario: 有截图但故事失败仍不得完成
- **WHEN** v1 major 的图片全部在盘，但任一最新 fail 或必验未覆盖尚未复验
- **THEN** 归档门禁 MUST 阻断，独立 review 不得用符合原型或面板能打开抵消用户任务失败
