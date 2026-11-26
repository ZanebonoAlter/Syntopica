## Context
逐 Scenario 检查文件路径存在或接受“人工”前缀，不能证明测试运行或断言有效。

## Decisions
### D1. 按用户故事和已知缺陷维护测试
落点：docs/reference/standard/shared/test-design.md。
规格仍先定可观察行为。真实缺陷先复现，在最稳定的一层留最小测试并追溯来源。其余白盒代码只做临时探针，不提交。拒绝改成更短的白盒模板，因为仍会把过程推成永久资产。

### D2. 验证先跑，归档不再逐条登记
落点：.pi/extensions/spec-gate.ts、scripts/harness/archive-readiness.sh、.pi/extensions/tests/run-harness-smoke.sh。退役 entry-gate.ts 与 lib/test-case-gate.ts；保留分层措辞提示，不删除其他阶段读方。
验证节记录适用的命令与结果；纯 UI 使用现有验收证据，不新增表或证据格式。旧映射允许留下供历史检索。选择直接移除运行入口而非降为 warn，避免保留重复催办；不改变 review、UI、约束读取或其他门禁。

## Risks / Trade-offs
无法靠路径存在性证明实际执行，仍由 verify 阶段直接确认，不声称归档脚本执行过测试。现有业务测试和已归档制品不重写。后续用 4–6 个同类 change 对比归档重试和测试净增量。
