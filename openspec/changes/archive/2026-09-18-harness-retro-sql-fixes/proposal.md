<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

2026-09-18 凌晨的无人值守复盘（`docs/research/harness-effectiveness-metrics/retro-analysis-2026-09-18.md`）实锤了 harness-retro 报告的两处脚本假信号：⑦段 A2「注入命中率 0%」是 `inj_dom` SQL 两个 bug（域名提取 substr 差一 + JIT 通道枚举漂移）叠加的产物，修正复算真实值 60%（18/30）；①段失败家族把 315 条 `command not found` 环境风暴误归「lint 规则」，真实环境噪声占比 ~64% 被呈现为 lint 25% 居首。假信号直接踩中 skill 解读红线（「命中率持续 0 = 注了白注」），会误导 harness 规则决策。同批并入 retro 报告指出的数据源侧修复项：dev-process-guard 写入的 5 条契约外 `policy.decision` payload 稀释了词汇表契约。

## What Changes

- **`scripts/harness-retro.sh` 三处修正**：
  1. `inj_dom` 域名提取 substr 差一（`-5-3` → `-4-3`），修正后域名可与 dom_map 完整域名 join；
  2. `inj_dom` 通道枚举补 `jit-path`（现枚举 `declaration/keyword/edit` 中 `edit` 全库 0 条，实际通道名自 2026-08-23 起为 `jit-path`）；
  3. ①段失败家族分类：`command not found`（命令/解释器缺失）前置归入「并发/环境冲突」族，置于 `%lint%` 等关键词家族判断之前。
- **dev-process-guard 契约豁免登记（代码零改动）**：其直写 `events.db` 的 `policy.decision`（`decision` 键旁路形状，design D6 有意权衡）保持不变，在 `harness-fact-log` 契约中登记为唯一显式豁免 + skill 词汇表同步——治愈「暗豁免」（实现了但契约没写）导致的词汇表与实现不一致（账本已积累 5 条契约外形状事件：orphan-warn 4 + orphan-killed 1）。
- **文档同步**：`.agents/skills/harness-facts/SKILL.md` 的 inject reason 枚举补 `jit-path`（并核对其他通道名）；`harness-retro` skill 若引用家族分类示例同步。
- **观察项立账（不动代码）**：rollup `turns` 口径与 `final` 标记失效疑点、subagent cost 全空——按 retro 报告改进项 #5/#7 登记观察窗口与回检指标，到期复看再定是否立 change。
- **sqlite 容量结论留档**：2026-09-18 讨论结论（TTL 分级 + 100MB 保险丝 + 稳态 ~20MB ≪ 上限，无需动）记入 design 备忘与 research 文档，防重复讨论。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `harness-retro-loop`: 报告口径修正——「失败特征归并」要求补充环境噪声（command not found）优先归类规则；⑦效能看板 A2 注入命中率的域名/通道匹配口径修正（修正后命中率预期从假 0% 回到真实量级，报告中「命中率持续 0」红线判定恢复可信）。
- `harness-fact-log`: `policy.decision` 契约显式豁免——dev-process-guard 孤儿治理事件登记为「策略显著裁决统一记账」的唯一显式豁免形状（此前 5 条契约外 payload 证明该路径是暗豁免，契约与实现不一致）。

## Impact

- `scripts/harness-retro.sh`（SQL 与家族分类）、`scripts/harness-retro.smoke.sh`（fixture 断言扩展）
- `.pi/extensions/dev-process-guard.ts` 仅核对旁路记账注释与豁免条款一致（**代码零改动**）
- `.agents/skills/harness-facts/SKILL.md`、`.agents/skills/harness-retro/SKILL.md`（文档枚举同步）
- 不触碰产品代码（front/backend-go 零影响）、不触碰注入通道配置；retro 只读契约不变
- 验收锚点：修复后同窗口复算 `m7.domain_hit_pct` 应 ≥30%（复算值 60%）；fixture 覆盖三类回归（substr/jit-path/家族分类）；修复前后 `--save-baseline` → `--baseline` 回检
