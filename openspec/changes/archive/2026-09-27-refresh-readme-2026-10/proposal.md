<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

README（521 行）已明显落后于仓库实际状态：默认端口仍写 5000（实际 5100）、文档列表挂 2 处死链（`architecture/data-flow.md`、`api/topic-graph.md`）、数据增强域描述停留在「三角色 Agent 因果分析报告」而实际主视图已是「板块信号解读报告」且新增了简报/调查、跨版块关系、研究数据源等能力。同时理念/用户旅程/功能全景三层对同一内容重复叙述，显得冗长。

## What Changes

**事实性修正（硬伤）：**

- 端口：`http://localhost:5000` / `ws://localhost:5000/ws` ×3 处 → 5100（compose 默认 `${PORT:-5100}`）。
- 死链清理：移除 `architecture/data-flow.md`、`api/topic-graph.md` 引用；文档列表按 `docs/reference/` 现状重列（补 board-signals / dataenrichment / datasources / reading / system 等核心入口，收敛到少量导航入口而非全量罗列）。
- 错别字：「Firecrawl 作为可选兑底」→「兜底」；`expand-board.png` 的错误 alt（复制残留）。
- 目录树中 `deploy/same-origin/` 描述更新：受支持同源形态已收敛为 **Go 单进程同域**（`:5100` 托管前端静态产物 + API + WS），快速开始节补充该部署形态。

**结构重组（方案 B「分层去重」，521 → 约 320 行）：**

- 内容唯一归属：理念（压缩）→ 用户旅程（砍到 5-6 节、每节 1 图）→ 功能全景（**唯一全量事实清单**）→ 快速开始/AI 配置 → 对比与适合谁（合并一节）。
- llama.cpp 完整命令行示例与显存参考表压缩或外链 `docs/reference/deployment.md`。
- 「与相邻产品」表与「与 TrendRadar 的区别」合并为一节。

**功能描述补齐（对照 map.md 与归档 change）：**

- 数据增强域重写：板块信号解读报告（工作台主视图，两阶段人工：发现候选 → 逐条预算研究）、板块简报 + 问题调查、跨版块关系发现、报告追问；保留循环 A/B 双循环叙事。
- 剔除 FinGenius 辩论（废案，不写入）。
- 研究数据源四源（EIA/JODI/WDI/Comtrade）一句话带过，标注 alpha（测试调研用）。
- 日报与话题节补：泳道动态视图（lane-dynamics）、日报旁注。
- 订阅与阅读节补：文章归档（代替删除）、图片代理、订阅源来源质量（入板块率）。

**截图更新：**

- `img/` 全量可更新：数据增强节换板块信号解读工作台截图（替换现 causal-report 双图）；旅程节图片随结构精简同步裁减；旧版本目录（1.3.3/1.4.0）可收敛为单目录。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

（无——纯文档 change，无 spec 级行为变化，`.openspec.yaml` 设 `skip_specs: true`）

## Impact

- `README.md` 全文重组；`img/` 截图资产更新。
- 无代码 / 接口 / 数据模型改动；无 flow 文档影响（README 不属任何业务域）。
- 部署行为不变，仅文档对齐现状。
