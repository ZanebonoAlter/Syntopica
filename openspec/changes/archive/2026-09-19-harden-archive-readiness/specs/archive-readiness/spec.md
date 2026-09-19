## Purpose

归档就绪自查：提供与 spec-gate 归档门禁同口径的四项聚合只读自查入口，让 agent 在 `openspec archive` 前一次看清全部缺口并补齐，消除「被拦 → 补一格 → 再试」的试探式归档重试（2026-09-19 体检：`m7.block_recur_groups=5`，单 change 83 分钟 7 连试）。

## ADDED Requirements

### Requirement: 归档就绪聚合自查脚本

系统 SHALL 提供 `scripts/harness/archive-readiness.sh <change>`（change 为 openspec change 名，kebab-case），执行与 spec-gate 归档门禁完全相同口径的四项检查：doc-impact verify、check-standards、tasks.md 尾三节与 doc-impact 声明标记、scenario-trace 对账。脚本 SHALL 逐项输出绿/红与失败原因摘要，任一项红则退出码 1，全绿退出码 0。四项判定 SHALL 复用与门禁相同的脚本与判定实现（同一事实源），不得另立第二份判定逻辑；脚本 SHALL 只读判定，不修改仓库文件、不执行测试或编译命令。

#### Scenario: 全绿通过

- **WHEN** 四项检查全部通过
- **THEN** 脚本退出码 0，输出逐项 ✓ 与总通过语

#### Scenario: 多项红一次性汇总

- **WHEN** 四项中存在至少一项红
- **THEN** 脚本退出码 1，一次性列出全部红项（非仅首个）及各自的失败原因摘要

#### Scenario: change 不存在

- **WHEN** 给定的 change 目录不存在
- **THEN** 脚本退出码非 0，输出明确指明 change 目录不存在

#### Scenario: 只读安全

- **WHEN** 脚本对任意 change 执行
- **THEN** 仓库文件零修改，且全程不执行测试或编译命令

### Requirement: 门禁 block 指引联动

spec-gate 归档阻断文案 SHALL 包含归档就绪自查指引：提示先运行 `bash scripts/harness/archive-readiness.sh <change>` 自查全绿后再归档，勿以 block 消息为增量清单逐项试探重试。指引为文案增强，不改变门禁判定逻辑与豁免机制。

#### Scenario: block 文案含自查指引

- **WHEN** 任一归档检查失败导致 openspec archive 被阻断
- **THEN** block reason 文案中包含 archive-readiness.sh 自查指引
