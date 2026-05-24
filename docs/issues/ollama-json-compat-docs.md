# 记录 Ollama JSON 兼容性问题诊断及对策

## What to build

在 docs 中记录此问题的完整诊断和解决策略，便于后续参考和新人理解。

## Acceptance criteria

- [ ] docs/issues/ 下记录问题现象、根因分析（Ollama 无 strict mode、嵌套 schema 遵守度差）、解决策略（降级容错 + 简单 JSON 模式 + 后置校验）
- [ ] 更新相关 reference 文档（如 AI provider 配置说明中注明 Ollama 的 schema 限制）

## Blocked by

- ollama-auxiliary-labels-fallback.md
- ollama-format-json-simple.md
- ollama-post-parse-schema-validation.md
