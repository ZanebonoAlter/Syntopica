# Tasks — harness-retro-sql-fixes

## 1. retro 脚本三修（scripts/harness/harness-retro.sh）

- [x] 1.1 修 `inj_dom` 域名提取 substr 差一（`-5-3` → `-4-3`）；验证：对 fixture 库输出域名无截断（无 `readin`/`ai-summar` 类畸形值），与 dom_map join 出非零命中
- [x] 1.2 `inj_dom` 通道枚举补 `jit-path`（`declaration/keyword/edit` → `declaration/keyword/jit-path`）；验证：fixture 含 `reason='jit-path'` 注入事件时计入命中率分子/分母
- [x] 1.3 A2 段加「未识别通道 N 条」显式计数（差集**限定 flow 文档注入范围**：`path LIKE '%/flow/%.md'` 的注入 reason 全集减匹配枚举；不按全库 reason 全集算，否则 `mode-base`/`change-file`/`index` 等不注入 flow 文档的合法通道会被误标）；验证：fixture 注入未知通道名 `future-chan` 的 flow 文档注入时报告出现该计数行、命中率不静默归零，且非 flow-doc 的合法通道事件不进未识别计数
- [x] 1.4 ①段家族分类前置「命令/解释器缺失」档（diag 含 `command not found` → 并发/环境冲突族，置于 `%lint%` 等关键词族之前）；验证：fixture 中 `golangci-lint: command not found` 归环境族、真实 `golangci-lint config error` 仍归 lint 族

## 2. dev-process-guard 契约豁免落档（代码零改动）

- [x] 2.1 核对 `.pi/extensions/dev-process-guard.ts` 旁路记账注释（design D6）与 delta 豁免条款一致，无需改代码；验证：grep `decision.*orphan` 注释仍在、`logEvent` 形状未变
- [x] 2.2 `.agents/skills/harness-facts/SKILL.md` 词汇表 `policy.decision` 行登记 dev-process-guard 豁免形状（`decision` 键 + 进程摘要 `cmd`/`offenders` 等；无 `action`，也不要求 `policy`/`reasonCode`；retro ③④段与催修时距不吸入）；验证：通读该行与 specs/harness-fact-log delta 措辞一致（含键形状豁免范围）

## 3. skill 文档同步

- [x] 3.1 `.agents/skills/harness-facts/SKILL.md` inject reason 枚举补 `jit-path`，并处理两个死枚举：`edit`（全库 0 条，2026-08-23 起通道名即 `jit-path`，标注更名或删除旧条目）与 `stack-conditional`（全库 0 条、从未出现，删除或标注未启用）；核对后登记全库实际 6 通道（`jit-path`/`declaration`/`keyword`/`mode-base`/`change-file`/`index`）；验证：枚举清单与 `SELECT json_extract(payload,'$.reason'), COUNT(*) FROM events WHERE kind='constraint.inject' GROUP BY 1` 一致，无 0 条死枚举残留
- [x] 3.2 `.agents/skills/harness-retro/SKILL.md` 七段读法补两句：环境噪声归并规则（`command not found` 前置）与「未识别通道计数」的读法；验证：通读无与 delta spec 矛盾的旧表述

## 4. 测试（smoke fixture 与回归断言，现路径 scripts/harness/harness-retro.smoke.sh）

- [x] 4.1 fixture 增补三类数据：截断域名 join 场景（`docs/reference/flow/reading.md` 注入 + 同域编辑 → 命中）、`jit-path` 通道注入、未知通道 `future-chan`；验证：smoke 全绿，断言 A2 命中率=fixture 复算值、未识别通道计数出现
- [x] 4.2 fixture 增补家族分类数据：`golangci-lint: command not found` 与真实 lint 失败并存；验证：smoke 断言前者在环境族、后者在 lint 族
- [x] 4.3 跑全量 smoke `bash scripts/harness/harness-retro.smoke.sh`；验证：退出码 0，七段 + 新断言齐备

## 5. 真实库验收与基线（只读）

- [x] 5.1 真实库复算：`bash scripts/harness/harness-retro.sh --days 7`，A2 命中率应 ≥30%（复盘复算值 60%）；①段 lint 族回落至 ~110 量级、环境族占比居首（窗口含 9-17 风暴）；验证：与 `docs/research/harness-effectiveness-metrics/retro-analysis-2026-09-18.md` 判定表数字方向一致
- [x] 5.2 留新基线 `bash scripts/harness/harness-retro.sh --save-baseline`；验证：`.pi/harness/retro-baseline.json` 含修正后 `m7.domain_hit_pct`
- [x] 5.3 观察项登记：rollup `turns`/`final` 疑点（观察至 2026-10-01，指标 `effectiveness.sess_dist.turns_p50`）、subagent cost（挂起待需求）补记进 retro-analysis 文档改进项清单状态列；验证：文档含两行状态更新

## 6. 验证（门禁）

- [x] 6.1 `bash scripts/harness/harness-retro.smoke.sh` 退出码 0（fixture 断言覆盖 1.1-1.4 全部修正点）
- [x] 6.2 `bash -n scripts/harness/harness-retro.sh && bash -n scripts/harness/harness-retro.smoke.sh` 语法通过
- [x] 6.3 真实库 `--days 7 --json` 输出可被 `jq '.metrics'` 正常读取且 `m7.domain_hit_pct` ≥30
- [x] 6.4 `git diff --stat` 确认改动仅涉及：scripts/harness/harness-retro.sh、scripts/harness/harness-retro.smoke.sh、两个 SKILL.md、retro-analysis 文档（零产品代码）

归档对账映射表（Scenario→测试，2026-09-18 归档前补；两套 smoke 归档前实测全绿）：

| Scenario | 测试文件 |
| --- | --- |
| spec-gate 阻断归档被记录 | .pi/extensions/tests/spec-gate.smoke.cjs |
| spec-gate 显式豁免被记录 | .pi/extensions/tests/spec-gate.smoke.cjs |
| quota-gate 阻断与 fail-open 被区分 | .pi/extensions/tests/policy-decision.smoke.cjs |
| test-scope 软硬模式被记录 | .pi/extensions/tests/policy-decision.smoke.cjs |
| dev-process-guard 孤儿治理事件按豁免形状记账 | .pi/extensions/tests/dev-process-guard.smoke.cjs |
| interop 探测短路被记账且不双写 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 正常放行零记录 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 记账故障不改变裁决 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 六段齐备且各带指标 | scripts/harness/harness-retro.smoke.sh |
| 失败特征归并 | scripts/harness/harness-retro.smoke.sh |
| 命令缺失类失败归入环境族 | scripts/harness/harness-retro.smoke.sh |
| 按 change 归属限定 | scripts/harness/harness-retro.smoke.sh |
| 注入通道词汇漂移不产生假零 | scripts/harness/harness-retro.smoke.sh |

## 7. 文档

<!-- doc-impact: none(harness 工具链纠错，不触及 reference 文档域；改动仅限脚本/skill 文档/research 文档) -->

- [x] 7.1 `docs/research/harness-effectiveness-metrics/retro-analysis-2026-09-18.md` 改进项清单补「修复状态」标注（#1/#2/#3 → 本 change；#4 → 契约豁免方案落地）；验证：grep 修复状态标记存在
- [x] 7.2 归档前 `bash scripts/harness/doc-impact.sh verify harness-retro-sql-fixes` 与 `bash scripts/harness/check-standards.sh --change harness-retro-sql-fixes` 通过（harness 域无 flow 文档，预期无 flow 溯源要求）；验证：两命令退出码 0（2026-09-18 实测：verify 通过声明 none；check-standards 168/168）
- [x] 7.3 归档协调：in-flight change `harness-effectiveness-metrics` 的 delta 也 MODIFIED 了 harness-retro-loop「报告分段与可回检指标」（六段→七段，加⑦效能看板），与本 change delta 改同一 Requirement；无论归档先后，机械整段替换都会静默抹掉一方修改（先归档它→丢⑦段描述；先归档本 change→丢本 change 新增的两条 SHALL）——归档时 MUST 手动合并该 Requirement 正文（七段描述 + 环境噪声前置与通道词汇对齐两条 SHALL + 双方全部 Scenario 并存）；验证：归档后主 spec 该 Requirement 同时含⑦段描述与两条新 SHALL（2026-09-18 实测：两 change 同批手工合并后 grep 三项均命中，6+6 Scenario 并存，openspec validate --specs 117/117）
