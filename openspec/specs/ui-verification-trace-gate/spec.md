# ui-verification-trace-gate Specification

## Purpose
UI 验收证据实物对账门禁：major/minor UI change 归档时，验收证据（`ui-verification/` 目录、`report.md` 建账、截图实物、minor 指名道姓映射）由独立脚本只读对账，取代 spec-gate 内的关键词字样匹配，堵住「tasks 写一句含 opencli/1440 字样即过」的空转通道。同构 review-trace-gate 的脚本—契约—豁免三段式。

## Requirements

### Requirement: UI 验收痕迹对账脚本

`scripts/harness/ui-verification-trace.sh <change-dir>` MUST 只读对账（不执行测试/构建、不修改仓库文件），按 change 的 `ui-impact` 声明分档判定，退出码 0（通过）/ 1（有失败，中文输出修复路径指引）。判定优先级：none 一致性 → minor 指名映射 → major 实物对账。旧 schema（spec-driven）change MUST 零检查不硬阻断。

#### Scenario: major 档实物齐全通过

- **WHEN** major change 的 `ui-verification/` 目录存在，`report.md` 含 ≥1 条合法判定条目，条目点名引用的截图文件均真实存在，且所有「未覆盖」条目显式写明原因
- **THEN** 脚本退出码 0 并输出条目计数与截图对账结果

#### Scenario: major 档零痕迹阻断

- **WHEN** major change 无 `ui-verification/` 目录或无 `report.md`
- **THEN** 脚本退出码 1 并输出两条修复路径（补做验收建账 / 显式降档理由）

#### Scenario: major 档空壳建账阻断

- **WHEN** `report.md` 存在但全文件零条合法判定条目（空壳文件）
- **THEN** 脚本退出码 1 并指出建账格式契约

#### Scenario: major 档点名截图缺失阻断

- **WHEN** 判定条目点名引用 `ke-xxx.png` 等截图文件，但目录内无此文件
- **THEN** 脚本退出码 1 并列出缺失文件清单

#### Scenario: 未覆盖场景无原因阻断

- **WHEN** 判定条目为「未覆盖」但其依据行未写明原因（如数据条件不满足）
- **THEN** 该条目不计入合法条目，若全文件无其他合法条目则对账失败

#### Scenario: minor 档指名道姓通过

- **WHEN** minor change 的 ui-design.md 验收节映射到组件测试且写出测试文件路径、该文件真实存在；或映射到 agent-browser/人工验证且 `ui-verification/report.md` 含 ≥1 条合法判定条目
- **THEN** 脚本退出码 0

#### Scenario: minor 档裸关键词阻断

- **WHEN** 验收节仅出现「组件测试/opencli/人工」等关键词字样，未挂任何实物（无测试文件路径或无 report.md 记录）
- **THEN** 脚本退出码 1 并给出指名道姓格式指引

#### Scenario: none 档一致性维持

- **WHEN** none change 的 ui-design.md 含 `## N/A Reason` 节且无影响等级不一致
- **THEN** 脚本退出码 0（不要求 `ui-verification/` 目录）

#### Scenario: 旧 schema change 不硬阻断

- **WHEN** change 绑定旧 `spec-driven` schema
- **THEN** 脚本零检查退出码 0（fail-open 兼容，不因本机制卡死存量）

### Requirement: report.md 建账格式契约

`ui-verification/report.md` 判定条目 MUST 采用一行标题 + 紧随依据行的结构：`### A-<N> <场景标题>【pass|fail|未覆盖】`（编号自 A-1 自增，判定词表封闭三值），下一行依据行说明断言依据；`未覆盖` 条目的依据行 MUST 含原因说明；截图证据以文件名形式出现在条目或截图清单中并参与实物对账。词表外判定/状态 → 该条目不计入合法条目。

#### Scenario: 标准条目解析

- **WHEN** 条目写作 `### A-6 原位详情【pass】` 且下一行为依据说明（含截图文件名）
- **THEN** 计入合法条目，截图文件名进入实物对账清单

#### Scenario: 判定词表外不合法

- **WHEN** 条目写成「搞定」「基本可用」等词表外判定
- **THEN** 该条目不计入合法条目，若全文件无其他合法条目则对账失败

#### Scenario: 格式对齐实战形态

- **WHEN** report.md 采用 add-topic-key-event-markers 4.4 验收报告的条目形态（A-N 场景 + 判定 + 依据 + 截图清单）
- **THEN** 无需改造即通过解析（契约自实战形态归纳，不自创结构）

### Requirement: 判定逻辑单一事实源

归档门禁（spec-gate 检查⑥'）与聚合自查（archive-readiness）的 UI 验收判定结果 MUST 与本脚本输出一致；扩展内不得保留平行的字样匹配判定。脚本缺席或执行失败时按既有 fail-open 语义处理（legacy/异常不硬阻断），且 MUST NOT 改变脚本自身的对账结论。

#### Scenario: 门禁与脚本同源判定

- **WHEN** 同一 change 目录分别由脚本与归档门禁判定
- **THEN** 两者对「UI 验收证据是否齐全」的结论一致（门禁不再因 tasks 含特定字样而放行脚本判失败的 change）

#### Scenario: 扩展内旧字样检查退役

- **WHEN** 实现后 tasks.md 含 `opencli`/`1440`/`1920` 字样但无实物证据
- **THEN** 归档门禁仍按脚本结论阻断（旧的关键词字样匹配逻辑不再单独构成放行条件）
