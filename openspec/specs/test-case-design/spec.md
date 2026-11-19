# test-case-design Specification

## Purpose

把测试用例的「设计层」规范从零散经验升级为单一权威源：定义 Scenario 最低集、边界枚举、可用性用例、验收措辞的标准，并保证这些规范在 agent 写 specs / tasks / 测试代码的时刻被自动注入（知道），归档时对措辞违例 warn 留痕（兜底提醒）。

## Requirements

### Requirement: 设计规范 JIT 注入

`shared/test-design.md` 的「JIT 注入摘要」节 SHALL 在 agent 编辑以下路径时被 constraint-injection 既有 JIT 机制节级注入（写/编辑路径命中 `doc-impact-applies` 信号的下一回合起，会话内粘性）：`openspec/changes/`（写 proposal/specs/tasks/design 等全部 change 制品）、`_test.go`（后端测试代码）、`.spec.ts` / `.test.ts`（前端测试代码）。本 Requirement SHALL NOT 要求修改 constraint-injection extension 代码——JIT 单一真相源是文档头部标签。

#### Scenario: 编辑 specs 时注入

- **WHEN** agent 编辑 `openspec/changes/<name>/specs/**/*.md` 写 Scenario
- **THEN** 下一回合起 system prompt SHALL 含「JIT 注入摘要」节（jit-path 命中，会话内粘性）

#### Scenario: 编辑测试代码时注入

- **WHEN** agent 编辑 `backend-go/**/*_test.go` 或 `front/**/*.{spec,test}.ts`
- **THEN** 下一回合起 SHALL 注入同一摘要节

#### Scenario: research 语境不注入

- **WHEN** 会话仅编辑 `docs/research/**`（无档位、非 change/测试路径）
- **THEN** SHALL NOT 注入摘要（信号不含该路径）

#### Scenario: 边界——注入预算超线降级

- **WHEN** 摘要注入后总注入量接近预算上限
- **THEN** SHALL 按 constraint-injection 既有分层降级规则处理（keyword 优先于 jit），SHALL NOT 为本文档新增降级规则

### Requirement: 前后端标准挂接与索引

`standard/backend/testing.md` 与 `standard/frontend/testing.md` SHALL 各含「用例设计」小节：引用 `shared/test-design.md` 为权威源，并补各自端的用例分层判据（后端：纯函数 → `*_unit_test.go` 无 DB / repository 与迁移 → testcontainer PG / handler → 轻量；前端：纯函数 → 单测 / 组件行为 → Vitest 组件测试 / 流程 → opencli）。`docs/reference/constraints-index.md` 执行规范表 SHALL 新增 test-design 行。挂接 SHALL NOT 改动两份 testing.md 的既有权威内容（怎么跑 / DSN 红线 / 禁 SQLite 等原样保留）。

#### Scenario: 后端判据可查

- **WHEN** agent 写后端测试前查分层判据
- **THEN** `standard/backend/testing.md` SHALL 含「用例设计」节并引用 shared 权威源

#### Scenario: 索引可见

- **WHEN** 未激活档位时 agent 看 constraints-index 常驻索引
- **THEN** 执行规范表 SHALL 含 test-design 行（写 specs/tasks/测试代码前可发现权威源）

#### Scenario: 既有权威内容不变

- **WHEN** 挂接完成后比对两份 testing.md 的既有章节
- **THEN** DSN 红线 / repository 禁 SQLite / 运行门禁等既有内容 SHALL 原样保留，仅新增引用节

### Requirement: 永久业务测试标准权威源

`docs/reference/standard/shared/test-design.md` SHALL 定义永久业务测试的两类准入：API 主路径与可追溯真实缺陷的最小复现。不设强制变体清单和白盒制品；工程工具自测不受业务测试准入限制。文档 SHALL 保留 `doc-impact-applies` 与不超过 3 KB 的「JIT 注入摘要」。效果依赖真实数据、模型或外部服务时 SHALL 量化核对并记录原因、方法、结果、结论；用户可见字段来源语义 SHALL 有 spec 锚，无锚先补 Requirement。

#### Scenario: 临时白盒验证

- **WHEN** 实现者用脚本或单测探针排查内部边界
- **THEN** 探针可以在验证后丢弃，不作为交付或归档完成条件

### Requirement: 归档保留测试层措辞提醒

归档 SHALL NOT 因复杂度或任务关键词而提示补白盒文档。同一任务行同时含「纯函数」和 `SQLite` 时保留软提示，判定以 `spec-gate.ts` 为准，改判定须同步测试规范。

#### Scenario: 复杂度和算法关键词不触发白盒提醒

- **WHEN** 复杂档出现算法或解析任务但没有 test-cases.md
- **THEN** 归档不催补白盒文档

#### Scenario: 无违例静默放行

- **WHEN** 归档时 tasks.md 无措辞违例
- **THEN** 检查⑤ SHALL 不产生任何 warning，归档流程与现状一致

#### Scenario: 纯函数任务提 SQLite warn 留痕

- **GIVEN** tasks.md 某任务描述含「纯函数」且验收含「SQLite 单测」
- **WHEN** 归档门禁执行
- **THEN** 归档 SHALL 放行，SHALL 落分层错配 warning 留痕

#### Scenario: 检查⑤异常不阻断归档

- **WHEN** 措辞扫描自身抛异常（如 tasks.md 编码异常）
- **THEN** 检查⑤ SHALL fail-open（console.warn + 留痕），归档 SHALL 按适用硬门禁的结果正常裁决

#### Scenario: 豁免通道兼容

- **WHEN** 命令带 `--force` 或 `SPEC_GATE_BYPASS=1`
- **THEN** 检查⑤ 随既有豁免通道放行，SHALL NOT 追加任何 block

#### Scenario: 同指纹重试不重复投递

- **WHEN** 同一会话内对同一 change 连续多次归档尝试，tasks.md 违例集合未变（指纹相同）
- **THEN** 仅首次尝试投递该批 warning 与对应 `acceptance-wording` 记账，后续尝试零重复 warning（硬门禁结果不受影响）

#### Scenario: 违例集合变化重新投递

- **WHEN** 前一次尝试已投递，本次尝试违例集合变化（如部分错配措辞已修正）
- **THEN** 本次按新指纹重新投递剩余违例的 warning 并再记账

#### Scenario: 违例清零收尾一行

- **WHEN** 前一次尝试已投递，本次扫描零违例（所有错配措辞已修）
- **THEN** 输出一行「已清零」收尾提示并清除指纹态

### Requirement: 受影响验证与历史测试资产检索

修改或移除既有契约时 SHALL 查受影响主路径和已知回归，确认旧断言不在验证已废止行为。MAY 用 test-assets.sh 查历史制品，不要求按旧 Scenario 制作处置表或补旧资产。

#### Scenario: 修改既有契约

- **WHEN** change 修改旧 Requirement
- **THEN** 验证受影响的主路径和真实缺陷回归，不要求逐旧 Scenario 填表

#### Scenario: 反向索引三段输出

- **WHEN** 对存在历史的主 capability 执行 `bash scripts/harness/test-assets.sh <capability>`
- **THEN** SHALL 输出主 specs 现状节拍 + archive 命中 change 清单 + 历史映射重建三段，退出码 0

#### Scenario: 查询不存在即报错

- **WHEN** 对主 specs 与 archive 均不存在的 capability 查询
- **THEN** SHALL 退出码非 0 且中文提示无匹配（不猜测、不出空段充数）
