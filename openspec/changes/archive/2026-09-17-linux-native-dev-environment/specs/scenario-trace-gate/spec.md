## MODIFIED Requirements

### Requirement: Scenario→测试映射对账脚本

系统 SHALL 提供 `scripts/scenario-trace.sh <change-dir>`（change-dir 形如 `openspec/changes/<name>`），对单个 openspec change 做归档前对账：只做静态判定，不执行任何测试或编译命令。对账范围为 change delta specs（`<change-dir>/specs/**/*.md`）中 ADDED / MODIFIED / RENAMED Requirements 节下的全部 `#### Scenario:` 标题；REMOVED Requirements 节下的 Scenario 不计入（删除的场景不再需要测试保障）。脚本 SHALL 在 POSIX bash 环境（Linux / macOS / WSL）可运行，退出码 0=通过、1=有失败，失败输出为中文并逐条列出原因。

#### Scenario: 映射齐全通过

- **WHEN** delta specs 的每个待对账 Scenario 在 tasks.md「N. 验证」节的映射表中均有对应行，且映射的测试文件在仓库内真实存在
- **THEN** 脚本退出码 0

#### Scenario: 缺映射阻断

- **WHEN** 某待对账 Scenario 标题在映射表中无对应行
- **THEN** 脚本退出码 1，输出列出全部未映射的 Scenario 标题及其所属 spec 文件

#### Scenario: 映射文件不存在阻断

- **WHEN** 映射行指向的测试文件路径在仓库中不存在
- **THEN** 脚本退出码 1，输出列出全部不存在的路径

#### Scenario: 人工映射合法

- **WHEN** 映射行的测试文件单元格以「人工」开头（如「人工（UI 目视确认）」）
- **THEN** 该 Scenario 视为已覆盖，不做文件存在性校验

#### Scenario: 无 delta Scenario 直接过

- **WHEN** change 无 specs/ 目录，或 delta specs 中不存在 ADDED / MODIFIED / RENAMED 的 Scenario（纯删除或 skip_specs change）
- **THEN** 脚本退出码 0
