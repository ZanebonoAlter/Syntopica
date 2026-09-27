const TASK = `无人值守分析任务：Syntopica 仓库（cwd /home/zanebono/software/Syntopica）harness 执行情况复盘，回答核心问题——**效能分析这条路，是分析脚本（scripts/harness-retro.sh）要优化，还是数据源（.pi/harness/events.db 事件账本/埋点）要优化？**

步骤：
1. 先读方法论文档 /home/zanebono/software/Syntopica/.agents/skills/harness-retro/SKILL.md（七段读法、改进项硬要求、边界），需要 schema/payload 细节再读 /home/zanebono/software/Syntopica/.agents/skills/harness-facts/SKILL.md。
2. 跑 bash scripts/harness-retro.sh --days 7 看七段报告；再跑 --days 7 --json 拿结构化指标。背景：session.rollup 埋点 2026-09-17 刚部署（openspec change harness-effectiveness-metrics），⑦效能看板 B 组「数据积累中」是预期，不要当成 bug。
3. 判定脚本侧：读 scripts/harness-retro.sh，检查聚合口径（还原分母/采样记账）、降级标注、SQL 是否有缺陷或值得改进；对照 docs/research/harness-effectiveness-metrics/explore-findings.md 的设计意图。
4. 判定数据源侧：用 sqlite3 -readonly 查 .pi/harness/events.db，看事件种类/字段完整性、rollup 快照覆盖率、TTL 保留期是否够支撑效能分析（注意：只读查询，绝不写库）。
5. 归类判据：数据没积累→不动等积累或建议加埋点；埋点字段/事件缺失→数据源优化；数据在但聚合错/呈现糟→脚本优化。
6. 产出改进项清单，每条按 skill 硬要求绑定可回检指标（指标名+当前值+期望值+观察窗口），标明归属（脚本/数据源/无需动）；单次偶发不得升格为规则；不 auto-tune 阈值、不开 change、不改任何脚本代码。

报告写到 /home/zanebono/software/Syntopica/docs/research/harness-effectiveness-metrics/retro-analysis-2026-09-18.md（含：七段报告关键数字摘录、脚本 vs 数据源结论与依据、改进项清单）。除该报告文件外不得写/改任何文件。最终回复=报告路径 + 3 句话内结论。`

const run = await runs.run("analysis", { agent: "delegate", task: TASK })

return { output: run.output }
