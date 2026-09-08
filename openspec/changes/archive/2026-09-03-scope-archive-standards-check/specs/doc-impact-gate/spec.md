## MODIFIED Requirements

### Requirement: 归档命令硬门禁（spec-gate）

pi 扩展 `.pi/extensions/spec-gate.ts` SHALL 拦截 bash 工具中匹配 `openspec archive` 的命令，在放行前强制执行四项检查：① `scripts/doc-impact.sh verify <change-dir>` 退出码为 0；② `scripts/check-standards.sh --change <change>` 无失败；③ 该 change 的 tasks.md 含「测试/文档/验证」尾三节及 doc-impact 标记；④ `scripts/scenario-trace.sh <change-dir>` 退出码为 0。`check-standards.sh --change <change>` SHALL 保留仓库级 A-E、G-H 标准检查，但 F 段只校验该目标 change 的 doc-impact，MUST NOT 因其他 active change 的 doc-impact 失败而失败。未传 `--change` 的手动 `check-standards.sh` SHALL 继续校验全部 active change。目标 change 不存在或自身 doc-impact 失败时，范围校验 MUST 失败并给出明确原因。任一失败 MUST block 并输出中文 reason（列失败项 + 修复指引）。豁免通道：命令显式带 `--force` 或环境变量 `SPEC_GATE_BYPASS=1`（MUST 记 warning，不得静默放行）。开关：`SPEC_GATE_ENABLE`（默认开启）。

#### Scenario: 门禁未过时归档被拦截
- **WHEN** agent 执行 `openspec archive <change>` 且目标 change 的 doc-impact 对账失败
- **THEN** 命令被 block，reason 列出目标 change 的失败项与修复指引

#### Scenario: 三项全过时放行
- **WHEN** 目标 change 的 doc-impact、任务结构、Scenario 映射以及 `check-standards.sh --change <change>` 的仓库级检查均通过
- **THEN** 归档命令正常放行

#### Scenario: 无关 active change 不阻断目标归档
- **WHEN** 目标 change 的 doc-impact 对账通过，而另一 active change 的对账失败
- **THEN** `check-standards.sh --change <目标change>` 的 F 段只报告目标 change 通过，spec-gate 不因另一 change 的失败阻断归档

#### Scenario: 目标 change 对账失败仍阻断归档
- **WHEN** 归档目标的 tasks.md 缺声明或其 doc-impact 对账失败
- **THEN** `check-standards.sh --change <目标change>` 返回非零，spec-gate 阻断归档并指向目标 change 的失败原因

#### Scenario: 手动全仓巡检保持全量语义
- **WHEN** 开发者不带参数执行 `bash scripts/check-standards.sh`
- **THEN** F 段继续遍历全部 active change 并报告每个 change 的 doc-impact 状态

#### Scenario: 归档目标不存在
- **WHEN** 调用 `check-standards.sh --change` 时目标 change 目录不存在
- **THEN** 脚本返回非零并输出目标 change 不存在，不执行静默的全仓回退

#### Scenario: 归档门禁使用目标范围
- **WHEN** agent 执行不带 bypass 的 `openspec archive <change>`
- **THEN** spec-gate 将该 change 传入 `check-standards.sh --change`，其余归档检查与原有阻断语义不变

#### Scenario: 显式豁免留痕
- **WHEN** 命令带 `--force` 或 `SPEC_GATE_BYPASS=1`
- **THEN** 归档放行且记录一条 warning（不静默）
