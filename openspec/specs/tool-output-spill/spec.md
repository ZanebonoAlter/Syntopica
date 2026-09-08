# tool-output-spill Specification

## Purpose

超大工具输出的强制保险丝：结果超过阈值时无条件将完整内容 spill 到会话维度磁盘文件，上下文仅保留有界预览与取回路径（read 可取回），不依赖 agent 自觉遵守 ctx 路由纪律，防止单个大工具输出把会话推向 compaction。

## Requirements

### Requirement: 超阈值结果强制 spill

工具执行结果合并文本超过阈值（默认 32KB，可配置）时，系统 SHALL 将完整内容写入会话维度的 spill 文件，并将进入模型上下文的结果替换为：头部预览 + 取回路径（read 工具可直接读取）+ 尾部预览 + 省略字节数说明。未超阈值的结果 SHALL 原样通过（不写文件、不记账）。

#### Scenario: 超阈值 bash 输出被替换

- **WHEN** bash 工具返回 80KB 文本结果
- **THEN** 完整 80KB 写入 `.pi/harness/spill/<sessionId>/` 下文件
- **THEN** 模型上下文收到头部预览 + 取回路径 + 尾部预览 + 省略字节数，总大小有界

#### Scenario: 未超阈值原样通过

- **WHEN** 工具结果合并文本 20KB（低于阈值）
- **THEN** 结果原样进入上下文，不产生 spill 文件与 spill.write 事件

#### Scenario: 按需取回完整内容

- **WHEN** agent 后续需要被 spill 的完整内容
- **THEN** read 取回路径指定的文件得到完整原文（内容与工具原始输出一致）

### Requirement: 只处理文本块

spill SHALL 只作用于结果的文本内容块；非文本块（图片等）SHALL 原样通过不参与阈值判定与替换。

#### Scenario: 图片块原样通过

- **WHEN** 工具结果含图片块且整体超过阈值
- **THEN** 图片块原样进入上下文，仅文本部分参与 spill 判定与替换

### Requirement: spill 失败安全降级

spill 文件写入失败（磁盘满、权限等）时，系统 SHALL 让结果原样通过并记录失败信息，MUST NOT 因 spill 机制本身失败而阻塞工具结果返回。

#### Scenario: 磁盘写入失败不阻塞

- **WHEN** spill 目录不可写导致落盘失败
- **THEN** 工具结果原样进入上下文，spill.write 事件记录失败原因

### Requirement: spill 文件保留期与记账

spill 文件 SHALL 保留 30 天（与 events.db TTL 清扫对齐，同一清理机制覆盖）；每次成功 spill MUST 记一条 `spill.write` 事件（payload 含工具名、原始字节数、spill 相对路径），供 SQL 聚合哪些工具最常 spill。

#### Scenario: 30 天清理

- **WHEN** spill 文件年龄超过 30 天
- **THEN** 下次清理周期删除该文件（events.db 事件记录不随文件删除）

#### Scenario: 记账可聚合

- **WHEN** 按月度 SQL 聚合 spill.write 事件
- **THEN** 可按 tool 分组统计 spill 次数与总字节数（ctx 纪律漏网率量化）

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
