# 归档说明（2026-09-19，非正式归档）

按用户指示轻量移入 archive，**未走正式归档流程（未同步主 specs、未跑 §11 归档门禁）**。
实现与验证在移入前已全部完成（100 tests OK、源/live 逐字一致、dsh 真实会话 12 次
MCP 调用成功，详见本目录 tasks.md / evidence/）。

## 后续去向

- `tools/energy-mcp/` 保留在仓库（dsh 侧可继续使用），**停止演进**。
- 数据源集成方向转向 Syntopica 产品本体（Go 数据源域 + 研究对话助手），
  本 change 沉淀的口径知识（en-dash→null、缺失标记不转 0、单位不换算、
  JODI 404 回退一次、EIA 列漂移→SCHEMA_CHANGED、TTL 缓存策略）
  作为 Go 重写的规格书。
- 见 `docs/research/research-assistant-data-sources/explore-findings.md`。
