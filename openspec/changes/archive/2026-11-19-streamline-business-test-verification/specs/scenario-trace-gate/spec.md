## REMOVED Requirements

### Requirement: Scenario→测试映射对账脚本
**Reason**: 文件路径存在无法证明实际覆盖或执行。
**Migration**: 旧脚本可作历史对账，不再用于新流程。

### Requirement: 映射表机器可读格式
**Reason**: 把重复填表变成归档成本。
**Migration**: verify 阶段记录适用命令和实际结果；旧表不追溯删除。

### Requirement: 归档门禁集成（spec-gate 检查④）
**Reason**: 产品契约不再逐场景绑定永久测试或人工占位。
**Migration**: 归档及自查均不调用此检查。
