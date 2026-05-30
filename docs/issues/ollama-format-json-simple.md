# Ollama provider 不传 JSON Schema，仅用 format: "json"

## What to build

当前 Ollama 路径将完整 JSON Schema 对象传入 `format` 字段（openai_compatible.go:109-110），但 Ollama 模型对复杂嵌套 schema 的遵守度不足，反而可能干扰输出结构。

改为：Ollama provider 路径始终使用 `format: "json"`（简单 JSON 模式），不传 schema 对象。JSON 结构约束完全由 system prompt + user prompt 保证。解析端的 SanitizeLLMJSON 兜底处理格式问题。

OpenAI 路径不受影响，仍用 `response_format` + `strict: true` schema。

## Acceptance criteria

- [ ] Ollama 路径：`format: "json"` 而非 `format: <JSONSchema>`
- [ ] OpenAI 路径不变：`response_format` + strict schema 照常
- [ ] 所有 capability（tag_extraction、tag_description、narrative、person_metadata 等）统一走简单 JSON 模式
- [ ] 现有 SanitizeLLMJSON 能处理非 schema 约束的输出（markdown fence、截断、引号转义等）
