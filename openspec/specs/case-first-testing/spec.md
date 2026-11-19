# case-first-testing Specification

## Purpose
TBD - created by archiving change amend-dev-workflow. Update Purpose after archive.

## Requirements

### Requirement: 实现前明确可观察契约

实现前 SHALL 明确 Scenario 所描述的可观察行为；复杂度声明继续服务评审，不触发白盒制品义务。白盒分析与探针 MAY 在开发期间临时使用，MUST NOT 因复杂度进入长期业务测试或成为归档前置。实现入口不再因缺白盒文档提醒。

#### Scenario: 复杂实现无需白盒制品

- **WHEN** 一个 complex change 不提交临时白盒代码或 test-cases.md
- **THEN** 实现入口不提醒补件，复杂度声明的其他用途不变

### Requirement: 长期业务测试准入与实际验证

新增长期业务自动化 SHALL 只覆盖主用户故事的 API 主线或可追溯的真实缺陷回归。缺陷修复 MUST 先确认复现，长期测试在最适宜一层保留最小复现；新增功能测试与实现顺序自由并同 change 落地。验证阶段 SHALL 实际执行本次适用的验证并在 tasks 验证节记录命令及观察结果；纯 UI 使用现有 UI 验收机制。归档 MUST NOT 要求逐 Scenario 映射。

#### Scenario: 同一主路径不做分层重复

- **WHEN** 新业务的用户可观察主路径已由 API 测试覆盖
- **THEN** 不因内部实现分支或 Scenario 数量新增重复业务测试

#### Scenario: 修复有记录的缺陷

- **WHEN** 可在组件层稳定复现真实缺陷
- **THEN** 先复现，再保留可追溯的最小组件测试，无需增加等价 API 测试
