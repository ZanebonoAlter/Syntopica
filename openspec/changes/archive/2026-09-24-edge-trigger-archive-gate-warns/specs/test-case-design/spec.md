## MODIFIED Requirements

### Requirement: 归档措辞 warn 检查（spec-gate 检查⑤）

`openspec archive` 归档门禁 SHALL 新增第五项检查（**warn 级，SHALL NOT block 归档**），对 `<changeDir>/tasks.md` 扫描两类措辞违例，命中时以 `spec-gate-warning` custom_message 留痕（display: true）：

- **⑤a 白盒用例文档缺失提醒**：tasks.md 任务描述文本命中复杂档关键词（算法 / 状态机 / 解析 / 协议等）且 change 目录不存在白盒用例文档（`test-cases*.md` 或 `*-test-cases.md`）→ warn「case-first-testing 要求复杂档产出白盒用例文档（分支表 / 边界值清单），当前 change 目录未检出；确非复杂档可忽略」
- **⑤b 分层错配措辞**：任务描述含「纯函数」且其验收措辞含「SQLite」→ warn「纯函数用例按 testing.md 分层应落 `*_unit_test.go` 无 DB；若任务确需 DB 请修正任务描述」

扫描 SHALL 抽成无副作用的纯函数（输入 tasks.md 文本与 change 目录文件列表，输出违例列表）供冒烟测试；违例列表与警告文案 SHALL NOT 影响检查①-④ 的既有 block 语义；检查⑤自身异常 SHALL fail-open（沿用本扩展既有异常策略，console.warn + 留痕，不阻断归档）。

违例 warn 的投递 SHALL 为**边沿触发**（同会话同指纹至多一轮）：指纹取自本次违例文案列表的内容摘要，同会话内违例集合未变的重试归档尝试 MUST NOT 重复投递同一批 warning；违例集合变化（新增/消失关键词、补齐或删除用例文档）时视同首见重新投递；上一次投递过、本次扫描零违例时 SHALL 输出一行「已清零」收尾并清除指纹态。指纹态 MUST 与会话生命周期绑定（会话边界清零、session compact 后重发一次），MUST NOT 跨会话复用。`policy.decision(action=warn, reasonCode=acceptance-wording)` 记账与投递走同一边沿（同指纹会话内至多一轮）。

禁用词表与关键词表以 `shared/test-design.md` 为权威源，spec-gate 内置同表常量并注明同步义务（表内容稳定，双源漂移风险可忽略）。

#### Scenario: 无违例静默放行

- **WHEN** 归档时 tasks.md 无措辞违例
- **THEN** 检查⑤ SHALL 不产生任何 warning，归档流程与现状一致

#### Scenario: 白盒用例缺失 warn 留痕

- **GIVEN** change 的 tasks.md 任务描述含「解析」关键词，change 目录无 `test-cases*.md`
- **WHEN** `openspec archive` 触发归档门禁
- **THEN** 归档 SHALL 放行（不被 block），且 SHALL 落一条含修复指引的 warning 留痕

#### Scenario: 纯函数任务提 SQLite warn 留痕

- **GIVEN** tasks.md 某任务描述含「纯函数」且验收含「SQLite 单测」
- **WHEN** 归档门禁执行
- **THEN** 归档 SHALL 放行，SHALL 落分层错配 warning 留痕

#### Scenario: 检查⑤异常不阻断归档

- **WHEN** 措辞扫描自身抛异常（如 tasks.md 编码异常）
- **THEN** 检查⑤ SHALL fail-open（console.warn + 留痕），归档 SHALL 按检查①-④ 的结果正常裁决

#### Scenario: 豁免通道兼容

- **WHEN** 命令带 `--force` 或 `SPEC_GATE_BYPASS=1`
- **THEN** 检查⑤ 随既有豁免通道放行，SHALL NOT 追加任何 block

#### Scenario: 同指纹重试不重复投递

- **WHEN** 同一会话内对同一 change 连续多次归档尝试，tasks.md 违例集合未变（指纹相同）
- **THEN** 仅首次尝试投递该批 warning 与对应 `acceptance-wording` 记账，后续尝试零重复 warning（检查①-④ 结果不受影响）

#### Scenario: 违例集合变化重新投递

- **WHEN** 前一次尝试已投递，本次尝试违例集合变化（如新增关键词命中或已补 `test-cases*.md` 使 ⑤a 消失）
- **THEN** 本次按新指纹重新投递剩余违例的 warning 并再记账

#### Scenario: 违例清零收尾一行

- **WHEN** 前一次尝试已投递，本次扫描零违例（如已补白盒用例文档且措辞已修）
- **THEN** 输出一行「已清零」收尾提示并清除指纹态
