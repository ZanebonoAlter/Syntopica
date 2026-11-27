# 约束索引

> constraint-injection extension 常驻注入本索引（**每个阶段都注入**）。本文件是**路由表，不是约束正文**——具体约束在各自文档里，需要时自行 `read`。
> 机制说明（阶段 marker / 域声明 / 路由清单）见 `AGENTS.md`「阶段驱动 + 自主导入」与 `开发执行规范.md` §13，**不在此重复**。

## 执行规范（how，写对代码）

| 领域 | 文档 |
| --- | --- |
| 后端（AI 日志 / 测试红线 / 代码风格 / lint / 包布局） | [`standard/backend/`](standard/backend/) |
| 前端（代码风格 / 交互约定 / 测试 / 主题） | [`standard/frontend/`](standard/frontend/) |
| 页面布局 · 交互模式 · 原型内容质量 | [`layout.md`](standard/frontend/layout.md) · [`interaction.md`](standard/frontend/interaction.md) · [`prototype-content.md`](standard/frontend/prototype-content.md) |
| 共享（测试准入 / 提交 PR / 文档规范与汇报叙事） | [`standard/shared/`](standard/shared/) |
| harness 扩展机制（注入通道 / 门禁分层 / 记账口径） | [`harness/pi-extensions.md`](harness/pi-extensions.md) |

## 业务规范（what，理解任务）

各业务域红线在 flow 文档的「业务约束与不变量」节：

| 域 | 文档 |
| --- | --- |
| 版块 / 标签 | [`semantic-board`](flow/semantic-board.md) |
| 话题 / 图谱 | [`topic-graph`](flow/topic-graph.md) |
| 日报 | [`daily-report`](flow/daily-report.md) |
| 抓取 / 正文 | [`content-enrichment`](flow/content-enrichment.md) |
| 数据增强 | [`data-enrichment`](flow/data-enrichment.md) |
| AI / 摘要 | [`ai-summary`](flow/ai-summary.md) |
| 偏好 / 订阅发现 | [`discovery`](flow/discovery.md) |
| 阅读页 | [`reading`](flow/reading.md) |
| 定时任务 | [`scheduler`](flow/scheduler.md) |
| 数据源 | [`research-data-sources`](flow/research-data-sources.md) |

**域声明**：change 头部写 `<!-- constraint-domains: 域名, ... -->`（`proposal.md` 或 `stage.md`，两处并集、只增不减）。**取红线**：`bash scripts/harness/constraint-digest.sh --brief --domains <域>`（派发子线程前跑，`--brief` 输出原样粘进 brief）。格式细节见 `开发执行规范.md` §13。

## 流程规范

openspec 编排（§0.6 六步）、门禁分层（§4）、归档纪律（§11/§12）：[`开发执行规范.md`](开发执行规范.md)。

**UI 设计工作流**（ui-impact 三档 / ui-design.md 制品 / major 原型审批 / apply 与归档门禁 / legacy）：[`开发执行规范.md`](开发执行规范.md) §0.6「UI 设计工作流」+ §3.1 + §5.3 + §11.1-⑤；行为契约为 [`ui-design-workflow`](../../openspec/specs/ui-design-workflow/spec.md)。
