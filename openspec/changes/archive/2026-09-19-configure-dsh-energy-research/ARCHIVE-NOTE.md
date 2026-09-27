# 归档说明（2026-09-19，非正式归档）

按用户指示轻量移入 archive，**未走正式归档流程（未同步主 specs、未跑 §11 归档门禁）**。

## 对账事实

本 change 的 spec delta 中「无 MCP 行 / persona 声明专业数据源未接入」场景，已被后续
`connect-dsh-energy-data-sources`（已实现并 live 部署）推翻：energy-mcp 已接入预设、
persona 已改为「先调用工具取官方数据」。**两 change 若日后补正式归档（主 spec 同步），
`dsh-research-preset` capability 以 connect 的新事实为准。**

## 后续去向

数据源集成方向已转向 Syntopica 产品本体（研究对话助手 + Go 数据源域），
见 `docs/research/research-assistant-data-sources/explore-findings.md`。
本 change 的 live 部署（`~/.dsh/.agent-presets/energy-research/`）继续有效，无人维护不影响现状。
