# 语义板块文章匹配严重跨域误判

## 症状

选中"中东冲突事件"板块（ID 2191），相关文章列表中出现了完全不相关的文章，例如 "mini-cc：一个轻量级 AI 编程助手的诞生"。

## 诊断

该文章有 5 个 AI 编程标签，经过 topic_tag_board_labels 全部以 hit_rate 理由匹配到了中东板块（score: 0.75~1.0）。

直接查库确认余弦相似度：

| 辅助标签 A | 辅助标签 B | 余弦相似度 |
|-----------|-----------|-----------|
| Claude Code | 伊朗 | 0.6503 |
| Anthropic | 伊朗 | 0.6649 |
| Anthropic | 以色列 | 0.6532 |
| 开源 | 中东 | 0.6676 |
| OpenAI | 以色列 | 0.6512 |

全部超过默认 SimThreshold=0.6。使用中的 embedding 模型 Qwen3-Embedding:4b 区分度不足。

## 临时缓解

当前默认 SimThreshold 已从 0.6 提升至 0.72（在 semantic_board_matching.go 的 loadConfig 中）。

## 根治方向

1. **替换 embedding 模型** — 实测 bge-m3 / gte-Qwen2 等更大参数模型，验证跨域相似度能否降到 0.3 以下
2. **匹配算法增加领域隔离** — 给板块打 domain 标签，匹配时规避跨域匹配
3. **匹配过程可视化** — 在 UI 中展示每个标签-板块匹配的相似度明细，便于调试
