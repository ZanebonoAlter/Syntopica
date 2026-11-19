# 约束索引

> constraint-injection extension 常驻注入本索引（未激活档仅此索引；激活档后按 change 文本/关键词/编辑路径自动追加具体约束内容）。与 AGENTS.md 优先级宪法冲突时，以宪法为准。

## 执行规范（how，写对代码）

| 场景 | 文档 |
| --- | --- |
| AI 调用与日志（`internal/platform/airouter/`、`internal/dataenrichment/`） | [`standard/backend/ai-logging.md`](standard/backend/ai-logging.md) |
| 后端测试与数据安全红线 | [`standard/backend/testing.md`](standard/backend/testing.md) |
| 业务测试准入（API 主线、真实 Bug 回归、临时白盒） | [`standard/shared/test-design.md`](standard/shared/test-design.md) |
| 后端代码风格 / lint / 包布局 | [`standard/backend/`](standard/backend/) |
| 前端代码风格 / 交互约定 / 测试 / 主题 | [`standard/frontend/`](standard/frontend/) |
| 页面布局模式 / 弹窗尺寸档 / major UI 双视口验收 | [`standard/frontend/layout.md`](standard/frontend/layout.md) |
| 交互模式选择判据 / 界面文字职责 / 原型预算与数据来源声明 | [`standard/frontend/interaction.md`](standard/frontend/interaction.md) |
| 原型内容质量（占位 vs 伪造 / 四态文案 / 视觉与动效下限 V1–V6） | [`standard/frontend/prototype-content.md`](standard/frontend/prototype-content.md) |
| 执行规范（how，写对代码）之外——pi harness 扩展机制（扩展全景/注入通道/门禁分层/记账口径） | [`harness/pi-extensions.md`](harness/pi-extensions.md) |
| 提交与 PR | [`standard/shared/commit-pr.md`](standard/shared/commit-pr.md) |
| reference 文档新增/修订标准（目录职责、头部注释、注册点 checklist） | [`standard/shared/doc-authoring.md`](standard/shared/doc-authoring.md) |

## 业务规范（what，理解任务）

各业务域红线在 flow 文档「业务约束与不变量」节：版块/标签 → [semantic-board](flow/semantic-board.md)｜话题/图谱 → [topic-graph](flow/topic-graph.md)｜日报 → [daily-report](flow/daily-report.md)｜抓取/正文 → [content-enrichment](flow/content-enrichment.md)｜数据增强 → [data-enrichment](flow/data-enrichment.md)｜AI/摘要 → [ai-summary](flow/ai-summary.md)｜偏好/订阅发现 → [discovery](flow/discovery.md)｜阅读页 → [reading](flow/reading.md)｜定时任务 → [scheduler](flow/scheduler.md)。

**change 涉及哪些域，在 proposal.md 头部用 `<!-- constraint-domains: 域名, ... -->` 显式声明**（域名=上表 flow 文档 basename）；档位激活后 constraint-injection 按声明注入对应约束节（**声明域=红线层**：顶层列表项首个加粗红线句逐行，细节层经关键词/JIT 全节命中或自行 read 补取；红线层提取失败回退全节），代码编辑路径由 flow/standard 文档头部 `doc-impact-applies` 标签 JIT 兜底（**关键词/JIT 命中=全节注入**，各域辖区见标签声明）。红线句格式规范见 [`standard/shared/doc-authoring.md`](standard/shared/doc-authoring.md)「约束节红线句格式」。

> **派发前取域红线**：派发子线程前跑 `bash scripts/harness/constraint-digest.sh --brief --paths <目标路径>`（或 `--brief --domains <域>`）取红线，**`--brief` 输出原样**粘进任务文本（单个 `## 约束（红线，源 …）` 段，其下每域 `### <域>（源 …）` 子标题；不带 `--brief` 的人读形态含旁注段，不可直接粘）。取数三步与 brief 模板见 [`开发执行规范.md`](开发执行规范.md) §0.6 步骤 3。**域清单唯一真相源 = `docs/reference/flow/*.md` basename**（脚本 `--domains` 只认它）；本文件域表是面向模型的人读索引（可能少于真相源），仅作 `--paths` 未命中时的**回退入口**；`scripts/harness/doc-impact.sh menu` 的 8 项是**文档目录轴**（flow/api/database/…）不是业务域轴——三处禁止混用。

## 流程规范

openspec 编排（§0.6 六步）、门禁分层（§4）、归档纪律（§11/§12）：[`开发执行规范.md`](开发执行规范.md)。

**UI 设计工作流**（ui-impact 三档声明 / ui-design.md 制品 / major 原型审批 / apply 与归档门禁 / legacy 兼容）：[`开发执行规范.md`](开发执行规范.md) §0.6「UI 设计工作流」+ §3.1 + §5.3 + §11.1-⑤；行为契约为 openspec 主 spec [`ui-design-workflow`](../../openspec/specs/ui-design-workflow/spec.md)（2026-09-04 归档自 change make-ui-design-first-class）。
