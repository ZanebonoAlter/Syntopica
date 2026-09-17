---
name: harness-retro
description: Syntopica harness 失败聚类复盘指南（.pi/harness/events.db 的只读消费方）。当需要回答"最近门禁为什么老红"、"这条软提醒到底有没有用"、"改了 harness 规则有没有效果"、"哪些约束文档从没被注入过"、"失败里有多少是环境噪声"这类**从数据找改进项**的问题时用本 skill。含报告六段的解读方式、改进项产出判据（可回检指标 + 观察窗口 + 反 overfit）与基线回检流程。与 harness-facts 的分工见文末（那边管"当时为什么这样"，本 skill 管"接下来该改什么"）。
---

# harness-retro — 从事实账本产出改进项

**一句话**：`bash scripts/harness-retro.sh` 把 `.pi/harness/events.db` 里的事件聚合成一份失败聚类报告；本 skill 讲怎么把报告读成**可回检的改进项**，而不是读成一堆数字。

前置知识（schema / payload 字段 / TTL / 采样记账协议）在 skill `harness-facts`，本 skill 不重复。

## 何时跑

| 触发场景 | 跑法 |
| --- | --- |
| change 归档后（看这次改动有没有留下新噪声） | `bash scripts/harness-retro.sh --days 7` |
| 定期体检（如每周一次） | 同上；配合 `--save-baseline` 留快照 |
| 怀疑某条软提醒 / 某条规则无效（"这事提醒了八百遍还在犯"） | `--days 7`，重点看第 ④ 段 |
| 要改 harness 门禁或约束注入规则（准 A/B） | 改前 `--save-baseline`，改后 `--baseline`，对比同类指标 |
| 排查"为啥最近老是红" | `--days 3`，重点看第 ①③⑤ 段 |

## 用法（与 `bash scripts/harness-retro.sh --help` 一致）

```bash
bash scripts/harness-retro.sh [--db PATH] [--days N] [--change NAME] \
    [--warn-threshold N] [--json] [--save-baseline [PATH]] [--baseline [PATH]] [--help]
```

| 参数 | 缺省 | 说明 |
| --- | --- | --- |
| `--db PATH` | `.pi/harness/events.db` | 目标账本（换机 / 指向快照副本时用） |
| `--days N` | 7 | 统计窗口（UTC，左闭）；> 30 天会提示 TTL 清扫风险 |
| `--change NAME` | 全局 | 只看某个 change 名下的事件 |
| `--warn-threshold N` | 5 | 第 ④ 段阈值（同 policy+reasonCode 的 warn 计数 ≥ 阈值才列出） |
| `--json` | 关 | 机器可读（基线比对与脚本消费用） |
| `--save-baseline [PATH]` | `.pi/harness/retro-baseline.json` | 落基线快照 |
| `--baseline [PATH]` | 同上 | 输出「较基线 ± 变化量」 |

退出码：`0` 报告成功（**发现大量失败也是 0**，报告不做判定）；`1` 参数或环境错误（库缺失 / 非本应用库 / 损坏）。

## 六段报告怎么读

**读之前先记住分母口径**：`gate.check` 成功侧是**采样记账**（会话首成功与转绿锚点各记 1 条，其后每 5 次连续成功记 1 条带 `sampled`+`n`）。所以「落库条数」不是执行次数，报告里的**还原执行次数（分母）**才是。别用 `ok=0 条数 / 总条数` 自己算——那样会系统性高估失败率。

| 段 | 看什么 | 什么算信号 | 改进方向 |
| --- | --- | --- | --- |
| ① 门禁失败聚类 | 失败率（用还原分母）+ **失败特征归并** | 「并发/环境冲突」「测试环境未就绪」占比高 → 失败噪声来自环境；「编译/类型」高 → 真实代码问题 | 环境族高 → 修门禁/环境稳定性，**别催 agent 改代码**；代码族高 → 看 Top 命令与 ⑤ |
| ② 回归翻转 | 回归失败数（曾绿变红）占比 + 转绿锚点 | 占比高 = 改动反复打破既有绿灯（改前没跑影响包 / 改错方向） | 强化「改前先跑影响包」；锚点数 = 修复次数（越高说明反复修） |
| ③ harness 自身故障 | `action=fail-open` 条数（如 `interop-down`） | 条数高 = **门禁自己不可用**，不是代码质量下降 | 先修 harness（环境链路），**别据此放宽阈值**——放宽会把真问题一起放掉 |
| ④ 软提醒失效 | 同 `policy+reasonCode` 的 warn 计数 ≥ 阈值 | 提醒被反复无视（同一件事触发很多次仍没改） | 三选一：升级为 block（须用户确认）/ 改提示文案与触发条件 / 承认它不该是提醒 |
| ⑤ 重复失败热点 | 同会话同命令、归一化 diag 连续重复 ≥ 阈值 | 连续同 diag 重复 = 死循环或试探式改错 | 这类会话需要更强干预（而不是再提醒一次） |
| ⑥ 注入面健康 | 各文档注入次数/字节 + 窗口内零命中文档 | 零命中 = 死约束（索引声明了却从没被注入）；单文档字节很高 = 注入膨胀 | 死约束：清理或修触发关键词；膨胀：走红线层提取（见 `standard/shared/doc-authoring.md`） |

## 改进项产出判据（硬要求）

1. **必须绑定可回检指标**：每条改进项写清「哪段指标名 + 当前值 + 期望值 + 观察窗口（如未来 7 天）」。指标名取自报告或 `--json` 的 `metrics`（`gate.failures` / `harness.fail_open` / `soft.stale_groups` / `inject.zero_hit_docs` …）。落不到指标上的主观判断不算改进项。
2. **单次偶发不得升格为规则**：只在一个 session 出现一次的失败，**不许**直接写成新规则；至少跨 2 个 session 或累计 ≥3 次再动规则。
3. **改完必须回检**：`--save-baseline` → 改 → `--baseline` 对比同类指标；差值不降就说明改动无效（或问题不在那）。
4. **反 overfit 警告**：针对单一任务、单一 diag 文本堆特例的规则会伤害泛化。优先调**口径、阈值、触发条件**，其次才是加规则；加规则前先问「这条规则对第二个场景也成立吗」。
5. **报告不判定、不自动开 change**：它只呈现。改进项要走本仓库默认流程（openspec change），并在 tasks 的验证节绑定同一个指标做验收。

## 边界（别做这些）

- 报告**不进注入通道**：不写 system prompt、不改 `.pi/constraint-injection.json`、不写任何事件——时变内容进 system prompt 会破坏前缀缓存（见 `docs/reference/开发执行规范.md` §4.1 与 AGENTS.md 约束注入说明）。
- 不 auto-tune 阈值、不自动开 change、不自动改门禁。
- 窗口 > 30 天时与基线比会失真（TTL 清扫掉的算「改善」）——报告头部会提示，看到提示就别下结论。
- 跨 change 并行时（共享工作树），报告里 `--days` 短窗口的数字会混入他 change 的脏文件影响；要归因到具体 change 用 `--change`，归因到具体文件用 skill `harness-facts` 的 `edit.map`。

## 与 harness-facts 的分工

| 你要回答的问题 | 用哪个 |
| --- | --- |
| 该改什么？指标趋势如何？改动有没有效果？ | **harness-retro**（本 skill，从数据找改进项） |
| 当时为什么注入了这条约束？这条事件为什么算在这个 change 名下？pin 落盘审计？ | **harness-facts**（事件考古/归因排查） |
| schema、payload 字段、TTL、采样协议细节 | **harness-facts** |
