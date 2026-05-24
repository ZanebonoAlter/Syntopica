# 为 Ollama 输出增加后置 schema 校验层

## What to build

在 Ollama 不传 JSON Schema 后（改用 format: "json"），需要一层后置校验作为安全网：解析 LLM 返回的 JSON 后、进入业务逻辑前，校验关键字段是否存在且非空。不匹配则触发已有的 retry 机制。

校验逻辑按 provider 类型开关：OpenAI strict 模式下可跳过（schema 已强制），Ollama 路径必须执行。

## Acceptance criteria

- [ ] 提取结果进入业务逻辑前校验：event/person 标签必须有 label、category；keyword 标签必须有 label、category、description
- [ ] 校验失败触发已有的 retry 机制（复用 extractor_enhanced.go 的 for loop）
- [ ] 校验逻辑可按 provider 类型开关（OpenAI 跳过，Ollama 执行）
- [ ] 日志记录校验失败原因（哪个字段缺失、哪个标签）

## Blocked by

- ollama-auxiliary-labels-fallback.md（需要先确定降级容错策略，再设计校验层）
