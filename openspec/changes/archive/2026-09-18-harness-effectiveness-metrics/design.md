# Design — harness-effectiveness-metrics

## Context

探索发现已固化在 `docs/research/harness-effectiveness-metrics/explore-findings.md`（数据源盘点/维度设计/用户三项决策），此处只记技术决策。核心事实：events.db 有 25 天账（330 session / 56 change），但 pi session jsonl 只滚动保留 ~2 天；jsonl 中 assistant 消息 100% 带 usage（含 cost）；`events.session_id` = jsonl 文件名 UUID，可直接关联。

## Goals / Non-Goals

**Goals**
- `session.rollup` 快照事件让 token/轮次/成本/时长长期沉淀进 events.db（90 天保留）
- retro 报告新增⑦效能看板段（A 插件 ROI / B 效率基线 / C 返工信号），六段故障视角零改动
- 基线 A/B 机制覆盖新指标

**Non-Goals**
- 不做加权综合评分（spec 已禁止）
- 不改产品代码、不触碰注入通道配置、不改 retro 只读契约（`sqlite3 -readonly` + application_id 校验不动）
- 不回填历史（2 天外的 session 永久缺 rollup，靠覆盖率标注暴露，不伪造）

## D1 rollup 数据源与写入路径（telemetry 扩展）

数据源：**pi sessionManager 的 jsonl 文件**，两条路径都不需要新依赖：

- **turn_end 节流快照**：`ctx.sessionManager.getSessionFile()` 拿当前会话 jsonl 路径（`dist/core/session-manager.d.ts` 已确认存在；telemetry 现只用 `getSessionId()`，本次新增调用）。模块级记 `lastOffset`（session_start 重置），增量读新增行（每 turn 只读几 KB），聚合出 `{turns(user 消息数), steps(assistant 数), toolCalls(toolResult 数), tokens{input,output,cacheRead,cacheWrite,total}, cost, durationSec(首末消息 ts 差), model(最近 model_change), final:false}`。
- **session_start 回填 prev 终值**：`event.previousSessionFile`（telemetry 现有代码已在记）→ 全量解析 prev jsonl → 若账本中该 session 无 `final=true` 快照则补写一条 `{..., final:true}`（session_id 用 prev 文件名里的 UUID）。

**节流条件**：turn 计数 % 5 == 0 或 tokens.total 较上次快照增量 > 20% 时写库；每 turn 都增量读（便宜），只是不每次落库。终值偏差有界：最后快照最多落后 5 turn 或 20% token；pi 退出后的最后缺口由下次 session_start 的 prev 回填收口（jsonl 2 天内都在）。**最后一个 session 在 pi 不再启动时永远缺 final——可接受**（用户不跑 pi 时也不需要评价它）。

**时序容忍**：turn_end 回调触发时当前 turn 的 assistant 消息可能尚未 flush 进 jsonl——快照数值允许滞后一回合（下回合增量读自然补齐），不追求精确到当前 turn。payload 数值单调不减（累计口径），spec Scenario 已对齐。

**fail-open**：jsonl 缺失/行解析失败 → 跳过该行、零写入、console 旁路告警，绝不抛出到钩子外（对齐 parseSubagentSummary 的 fail-safe 先例）。单行解析失败的行计入 payload 可选字段 `skippedLines` 供口径排查，不影响其余行聚合。

## D2 rollup 事件契约与查询语义

- kind=`session.rollup`，保留期 90 天（session 级汇总，占空间小：每 session ≤ turns/5 + 1 条）
- change 列 = 快照写入时刻 `detectActiveChange(ctx.cwd)?.name ?? null`（对齐 telemetry 现有 subagent 事件的绑定语义）
- 查询终值：`WHERE kind='session.rollup' AND id IN (SELECT MAX(id) ... GROUP BY session_id)`——对齐 edit.map 的"取最新一条即完整集合"既有模式，harness-facts SKILL.md 同步补一行
- 数值字段全部可空容忍（model/cost 可能为 null），聚合时 `COALESCE(x,0)` 仅用于展示、分布计算排除 null 样本并标注样本数（对齐子线程 token 占比的 spec Scenario）

## D3 ⑦段指标口径（retro 脚本，SQL 下推）

沿用现有单次 SQL 聚合下推 + python3 只做渲染的架构。新增指标全部由 events.db 计算（不读 jsonl——retro 保持只读账本的纯度）：

| 指标 | 源 | 口径 |
| --- | --- | --- |
| 注入负载 | constraint.inject | 每 session Σbytes 的中位数/P75（稳态零投递改造后同 path 不重复记，直接可用）；degraded 计数单列 |
| 注入命中率 | inject(reason=declaration/keyword/edit 的域) × edit.map | 域映射见 D4；命中率 = 该 change 实际编辑路径映射域 ⊆ 被注入域 的比例（逐 change 算后取中位数） |
| pin 复用率 | pin.write / pin.read | 窗口内 write 数 vs read 数（按 change 关联可选拆分） |
| 催修时距 | gate.check | 同 `(session_id, cmd)` ok=false → 下一条 ok=true 的 ts 差中位数；policy.decision(block) → 转绿同法（target 作键时降级为 change 级） |
| block 复发 | policy.decision | 同 (change, reasonCode) 出现次数 > 1 的清单 |
| 效率分布 | session.rollup 终值 | per change / per session 的 turns、tokens.total、tokens.output、cost、durationSec 中位数+P75（PERCENT_RANK 窗口函数） |
| 子线程 token 占比 | subagent.dispatch+complete Σtokens(非 null) ÷ (同值 + rollup Σtotal) | 样本数标注 |
| 返工波次 | edit.map | `session_id × json_each(paths)` 去重 → 文件→sessions 集合大小 Top10（≥3 才列） |
| 归档重试 | policy.decision | 每 change 的 archive-check-failed block 计数 |

**change 归属**：⑦段 per-change 聚合统一用各事件自带的 change 列（快照/写入时刻绑定值），不做 session→change 插值（rollup 自带 change 后插值不再必要；混合 session 的误差已在 harness-facts 踩坑三件套备案）。

## D4 域映射（路径→业务域）提取

注入命中率需要「编辑路径 → 业务域」映射。**不硬编码**：脚本启动时解析 `docs/reference/flow/*.md` 头部 `doc-impact-applies` 标签（格式如 `doc-impact-applies: paths:internal/domain/daily_report/**`，具体以现有文档头实际格式为准，实现时核对一篇），构建 路径前缀→域 表；解析失败 → 命中率指标输出「域映射不可用」降级标注（沿用 T5/T6 白盒降级先例），其余指标不受影响。inject 侧的域从 payload.path（flow 文档 basename）取。

## D5 基线扩展

`--save-baseline` 落盘 JSON 在既有结构上新增 `metrics7` 键组（各效能指标的扁平键值，键名与报告段内指标名一致）；`--baseline` 比对逻辑复用现有差值输出。旧基线文件无 `metrics7` → 该组跳过比对并提示，不失败（向后兼容，对齐"基线损坏降级"先例）。

## D6 覆盖率白盒

⑦段头部固定输出：`rollup 覆盖率 = 有终值快照的 session ÷ 窗口内出现过的 session`。覆盖率 < 50% 时 B 组分布指标行尾追加「数据积累中」标注（数值照出，不隐藏）。C 组返工信号与 A 组不依赖 rollup，无覆盖率概念。

## D7 测试策略

- **telemetry 纯函数直测**：jsonl 增量聚合器 export（对齐 parseSubagentSummary 先例）：fixture jsonl 文本 → 期望 {turns,steps,toolCalls,tokens,cost}；坏行跳过；空文件/缺文件零写入。放 `.pi/extensions/tests/`（现有目录）。
- **retro smoke 扩展**（harness-retro.smoke.sh）：fixture 库加三类数据：① rollup 快照序列（多条中间+一条 final）② 无 rollup 的 session ③ declaration 注入 domain-A + edit.map 全落 domain-B 的 change。断言：七段齐备、终值只取最新快照（数值=final 条而非 Σ）、覆盖率数值正确、命中率=0、降级标注出现、无综合分行。
- **手动验收**：真实库跑 `--days 30`，确认⑦段在 rollup 稀疏下正常降级；跑 `--save-baseline` 两次确认 metrics7 差值。

## 风险与回滚

- **jsonl 格式漂移**（pi 升级改 usage 结构）：聚合器容错 + skippedLines 口径字段，退化为部分指标缺失而非误记账（对齐 subagent 摘要解析的 fail-safe）。
- **sessionFile 路径不可得**（getSessionFile() 返回 undefined）：该 turn 跳过 rollup，仅记 console；prev 回填路径独立不受影响。
- **回滚**：删除 telemetry 的 rollup 代码块即可停写；retro ⑦段独立成函数，revert 单段不影响六段；词汇表多一个 kind 无兼容性风险（TTL 90 天自然过期）。
