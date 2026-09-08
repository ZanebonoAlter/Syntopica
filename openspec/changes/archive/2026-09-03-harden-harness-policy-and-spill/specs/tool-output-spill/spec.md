## ADDED Requirements

### Requirement: spill 文件唯一命名与最小权限

每个 spill 文件名 SHALL 包含由该次工具调用 `toolCallId` 确定性派生的短哈希，并保留时间戳与清洗后的工具名，使同一毫秒完成的同名并行工具调用仍写入不同路径。哈希 MUST NOT 暴露原始 toolCallId；相同输入 MUST 产生相同哈希，文件名 MUST 仅含跨平台安全字符。

在支持 POSIX 权限的平台，新建或复用的会话 spill 目录 SHALL 收敛为 `0700`，新写 spill 文件 SHALL 使用 `0600`；权限收紧失败视为 spill 写入失败，按既有 fail-open 语义原样返回工具结果。在 Windows 等不完整支持 POSIX mode 的平台，系统 SHALL best-effort 设置权限且 MUST NOT 仅因平台不支持 mode 而阻断 spill。既有 spill 文件不迁移，继续按 30 天保留期清理。

#### Scenario: 同毫秒同工具并行结果不覆盖

- **WHEN** 两个工具名相同、时间戳相同但 toolCallId 不同的超阈值结果完成
- **THEN** 两次 spill 写入不同文件，且两个文件分别保存各自完整原文

#### Scenario: 文件名不暴露原始调用标识

- **WHEN** 超阈值工具结果携带可识别的 toolCallId
- **THEN** spill 文件名含该标识的短哈希但不含原始 toolCallId，且 spill.write 记录最终相对路径

#### Scenario: POSIX 最小权限

- **WHEN** 在支持 POSIX mode 的平台成功 spill
- **THEN** 会话目录权限为 `0700`，新写文件权限为 `0600`

#### Scenario: 既有目录权限收敛

- **WHEN** 会话 spill 目录已存在且权限宽于 `0700`，随后写入新的 spill 文件
- **THEN** 在 POSIX 平台目录权限被收敛为 `0700`，新文件以 `0600` 创建，既有文件内容不迁移不改写

#### Scenario: 权限收紧失败安全降级

- **WHEN** POSIX 平台无法将目录或新文件收紧到规定权限
- **THEN** 本次 spill 按写入失败处理，工具结果原样进入上下文并记录失败信息
