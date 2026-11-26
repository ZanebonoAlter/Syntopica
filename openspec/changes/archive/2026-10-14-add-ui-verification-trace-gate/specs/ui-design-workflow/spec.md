## MODIFIED Requirements

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
