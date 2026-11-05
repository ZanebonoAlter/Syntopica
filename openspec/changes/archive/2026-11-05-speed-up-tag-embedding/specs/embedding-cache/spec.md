# Embedding Cache — Delta

## ADDED Requirements

### Requirement: 批量 embedding 缓存 key 按单条拆分

批量调用 embedding 时，缓存查/写 SHALL 按批内每条文本独立计算 cache key（单条文本口径），不得以整个 input 数组拼接为 key。单条路径的缓存 key 生成方式不变。

#### Scenario: 批量部分命中
- **WHEN** 批量请求 [A, B, C]，其中 B 已有缓存
- **THEN** 仅 A、C 计为 miss 并发送给 provider；A、C 的结果按各自单条 key 写入缓存，B 不重复写入

#### Scenario: 单条路径 key 兼容
- **WHEN** 单条调用 embedding（既有 inline 打标路径）
- **THEN** 缓存 key 与批量化改造前完全一致（历史缓存仍可命中）
