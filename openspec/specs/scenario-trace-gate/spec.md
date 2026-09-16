# scenario-trace-gate Specification

## Purpose
把 case-first-testing 已成文的「Scenario→测试映射」归档对账从自觉约定升级为机器门禁：scenario-trace.sh 对 change 的 delta specs 做只判不跑的对账（映射齐全 + 测试文件存在），spec-gate 归档时强制执行。

## Requirements

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

### Requirement: 映射表机器可读格式

tasks.md 的「`## <数字>. 验证`」节（与 spec-gate 尾三节同款锚定）SHALL 包含表头为 `| Scenario | 测试文件 |` 的 markdown 表格。每个待对账 Scenario 至少一行：Scenario 单元格为标题逐字匹配（仅裁剪首尾空白）；测试文件单元格为仓库根相对路径（多个路径以空白或逗号分隔）或以「人工」开头的说明。标题在 change 内跨 spec 重名时，一行映射覆盖全部同名实例。

#### Scenario: 定位不到验证节

- **WHEN** 待对账 Scenario 数量大于 0，而 tasks.md 不存在、无 `## <数字>. 验证` 节、或该节内无规定表头的映射表
- **THEN** 脚本退出码 1，输出指明验证节或映射表缺失

#### Scenario: 多文件单元格

- **WHEN** 一个 Scenario 由多个测试文件共同保障，单元格含多个空白分隔的仓库相对路径
- **THEN** 每个路径均做存在性校验，任一不存在即失败

### Requirement: 归档门禁集成（spec-gate 检查④）

`openspec archive` 前置门禁 SHALL 在既有三项检查（doc-impact verify / check-standards / tasks.md 尾三节）之外新增第四项：`bash scripts/scenario-trace.sh <changeDir>` 退出码 0。该项与前三项各自独立判定、任一失败即阻断归档，失败输出与修复指引并入既有 reason 格式。逃生口沿用 `--force` 与 `SPEC_GATE_BYPASS=1`（放行且 warning 留痕），不新增豁免机制；存量 in-flight change 不豁免。

#### Scenario: 归档时自动对账

- **WHEN** agent 执行 `openspec archive <change>` 且前三项检查通过而 scenario-trace.sh 退出码非 0
- **THEN** 归档被阻断，reason 含未映射 Scenario 清单与补映射表指引

#### Scenario: 逃生口留痕

- **WHEN** 归档命令带 `--force` 或环境变量 `SPEC_GATE_BYPASS=1`
- **THEN** 归档放行，warning 记录「Scenario 映射对账未通过，留痕备查」
