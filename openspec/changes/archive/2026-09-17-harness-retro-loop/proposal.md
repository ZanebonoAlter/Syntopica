<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

harness 层的规则（门禁阈值 / 注入策略 / 派发约束 / 提醒话术）目前只有「加」没有「回检」：改完一条规则，没人能回答「它到底有没有起作用」。而 `.pi/harness/events.db` 自 2026-08-23 起已积累 **26717 条**事实（`gate.check` 20521 / `constraint.inject` 3462 / `edit.map` 452 / `policy.decision` 103 / `subagent.*` 297），却只有 `harness-facts` skill 做**事后人工考古**（「当时为什么注入了这条约束」），缺一个**从数据反推改进项**的固定流程。

已躺在数据里、肉眼可见却无人处置的信号（2026-09-16 采样）：

- `test-scope-guard / full-go-test` 软提醒重复 **41 次** —— 提醒显然无效，是规则形态的问题，不是 agent 不听话的问题；
- `go test -short ./internal/dataenrichment/...` 失败 **307 次**（admin 161 / topicgraph 95 / tagmanagement 82）—— 失败热点包常年不收敛；
- `quality-gate / interop-down` 的 fail-open **42 次** —— **门禁自身故障被混进「失败统计」**，与 agent 犯错无法区分，改进方向会被带偏；
- `flip=1`（上回合尚绿、本回合转红）**651 次** —— 回归信号真实高频，但从没被汇总过。

## What Changes

- 新增只读消费脚本 `scripts/harness-retro.sh`：从 events.db 生成失败聚类报告。**分母口径 MUST 遵循 `harness-fact-log` 已定义的采样协议**（失败条计 1、翻转锚点计 1、采样成功条按 payload 的 `n` 加权），而非 naive 的 `ok=0 / 总条数` —— 后者会因成功侧采样而系统性高估失败率。
- 报告分段（每段自带可回检指标）：
  1. **门禁失败聚类**：按 cmd / 包 / diag 特征归并，出 top-N；
  2. **回归翻转率**：同一 `(session, cmd)` 由绿转红的次数与占比；
  3. **harness 自身故障单列**：`policy.decision` 中 `action=fail-open`（如 `interop-down` / `quota-query-failed` / `ui-gate-check-failed`），**不计入 agent 失败分母**；
  4. **软提醒失效**：同 `policy + reasonCode` 的 `warn` 重复次数 ≥ 阈值（可配）→ 提示「提醒无效，考虑改硬或改形态」；
  5. **重复失败热点**：同 session 同 cmd 同 diag 连续重复 → doom-loop 信号（为后续「反死循环提醒」提供数据源）；
  6. **注入面健康**：`constraint.inject` 的文档命中次数 / 字节分布，暴露「零命中的死约束」与注入膨胀。
- **基线快照与比对**：`--save-baseline` 落 JSON，其后运行输出「较基线 ±」，使**一条 harness 规则上线前后的同类事件计数变化**成为准 A/B 回检指标。
- **窗口安全**：请求窗口超过对应 kind 的保留期（`gate.check` / `policy.decision` / `edit.map` / `constraint.inject` 均为 30 天）时 MUST 显式提示「数据可能已被 TTL 清扫」，避免把「被清扫」误读成「已改善」。
- 新增 skill `.agents/skills/harness-retro/SKILL.md`：何时跑（归档后 / 定期 / 怀疑某规则无效时）、如何把报告读成改进项、**反 overfit 判据**（一条改进项必须绑定可回检指标 + 观察窗口；单 session 偶发不算），以及与 `harness-facts`（考古归因）的分工。
- 配套 `scripts/harness-retro.smoke.sh`：fixture events.db 逐段断言（含加权分母还原、阈值边界、窗口超期提示、空库/缺库降级）。
- 报告**只以命令输出形态出现，MUST NOT 注入 system prompt**（时变内容破坏前缀缓存 —— 与既有「态劳数据不进 system prompt」一致）。

## Capabilities

### New Capabilities

- `harness-retro-loop`: 从事实账本生成失败聚类报告的能力契约 —— 分母口径（采样加权还原）、harness 自身故障与 agent 失败的分离、基线与回检语义、窗口/TTL 安全提示。

### Modified Capabilities

（无。`harness-fact-log` 只被只读消费，其事件词汇、payload 契约与采样口径均不变。）

## Impact

- **新增文件**：`scripts/harness-retro.sh`、`scripts/harness-retro.smoke.sh`、`.agents/skills/harness-retro/SKILL.md`
- **只读 events.db**：不新增表 / 列，不改写入方 `.pi/extensions/lib/harness-log.ts` 与任何现存扩展的行为
- **文档**：`AGENTS.md`（skill 注册 + harness 行为规则）、`docs/reference/开发执行规范.md`（retro 在编排流程里的位置）、`.agents/skills/harness-facts/SKILL.md`（补分工指引）
- **无** API / 数据库 schema / 前端变更（`ui-impact: none`）
- **依赖**：`sqlite3` CLI（本机已装，`scripts/*.sh` 已有同类依赖惯例）

## Non-Goals

- 不自动注入报告，不修改任何现存 extension（`quality-gate` / `constraint-injection` 等）的运行时行为
- 不新增事件 kind、不改 events.db schema
- 首版**不做 LLM 分析、不派并行分析 agent**（纯 SQL 聚合，零 token）
- 不调整门禁阈值（发现的问题以改进项形式另行开 change）
- 不含**在线**反死循环提醒（本 change 只提供数据源；在线 steer + `edit.map` 逐文件计数扩展另开 change）
- 不做 dashboard / 图表 / 定时任务自动化
