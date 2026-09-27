<!-- complexity: complex -->
<!-- ui-impact: none -->

## Context

spec-gate 的检查⑤（措辞扫描）与⑤'（并发脏树）是 warn 级投递点，当前每次 `openspec archive` 尝试全量重发。归档流程天然是「block → 修一项 → 重试」多尝试循环，warn 内容在循环内通常不变。参照系（同构先例）：

- `constraint-injection.ts`：per-sessionId 内存态（`sessionStates`，LRU 有界 + 兜底槽）、内容指纹 diff 驱动投递（指纹未变零投递）、`session_compact` 后重发一次。
- `quality-gate.ts`（aggregate-concurrent-gate-warns 刚落）：失败指纹状态机 `{ diag, rounds, foreign }`，边沿触发（首见全文 / 指纹变化重注 / 转绿单行收尾 / 同指纹静默），会话边界清零，记账与注入同边沿。
- 事实库证据：同会话同 change 的 `concurrent-dirty-tree` warn 多次成对出现（relax-lane-snapshot-length-caps 等 5 组）；retro 判 31 次/周为软提醒失效组。

## Goals / Non-Goals

- **Goals**：同会话内同指纹 warn 只说一次；指纹变化 / 新会话 / compact 后重发；转绿单行收尾；warn 记账与投递同边沿。
- **Non-Goals**：不改检查①-④' 的 block 判定与记账；不去重 bypass / fail-open / 无名 fail-open 的 warning（审计留痕）；不做跨会话或磁盘持久化；不改 `policy.decision` 词汇表；不动 concurrency-status.sh / gate-status.sh 脚本。

## Decisions

### D1 状态模型：per-sessionId 二级 Map，LRU 有界，兜底槽

```
warnStates: Map<sessionKey, Map<cacheKey, { fp: string }>>
sessionKey = ctx.sessionManager?.getSessionId?.() ?? "__no-session__"
cacheKey   = `${changeName}:${kind}`   // kind ∈ "wording" | "concurrent"
```

- 会话隔离对齐 constraint-injection 的事故教训（2026-09-17 会话 A 状态漂移）：**键必须含 sessionId**，不用模块级单值状态。
- 无 sessionId 的烟测 stub 语境落单一兜底槽（`__no-session__`），行为与隔离前等价（每次进程内首个 warn 视为首见）。
- 有界：会话条目上限 32，超限淘汰最久未写会话（对齐 constraint-injection `sessionStateLimit` 先例；spec-gate 触发频率低，32 足够，不设环境变量）。

### D2 指纹取值：内容 sha256，输入先规范化

- ⑤'：`conc.stdout` 按行拆分 → trim → 过滤空行 → **排序去重** → join("\n") → sha256。排序消除脚本输出顺序抖动造成的假 diff；归属 change 名与文件路径都在行内，自然参与指纹。
- ⑤：`scanAcceptanceWording(...)` 返回的文案数组 join("\n") → sha256（扫描顺序已由常量词表序决定，确定性够，不再排序——保留「新增关键词排前」的语义差异进指纹）。
- 哈希用 `node:crypto` `createHash("sha256")`，只存 16 字节 hex 前缀，不存原文（防止状态表膨胀与敏感内容落内存副本）。

### D3 投递边沿：四态与 quality-gate 家族对齐

对每次归档尝试的每个 kind：

| 状态 | 判定 | 动作 |
| --- | --- | --- |
| 首见 | 无条目 或 fp 变化 | 全量 warn 投递 + `policy.decision(warn)` 记账，写 `{ fp }` |
| 同指纹 | fp 相同 | 零 sendMessage；`console.log` 一行 `[spec-gate] 检查⑤/⑤' 同指纹静默` 留运行痕；**零记账** |
| 转绿 | 上次有条目 且 本次干净（⑤ 零违例 / ⑤' exit 0/3） | 单行 ✓ 收尾投递，删条目；零 warn 记账 |
| 天然首见（转绿后再犯） | 条目已删 | 回到首见 |

- 检查⑤' 的 exit 3（冷启动）不产生 warn 也不产生收尾（维持现状零输出）。
- **记账与展示严格同边沿**：同指纹会话内至多一条 `concurrent-dirty-tree` / `acceptance-wording`（对齐 aggregate-concurrent-gate-warns D2 口径）；`change.archive` 消费侧按名聚合天然幂等，少掉的重复 warn 记账不影响任何现有查询。
- 指纹计算、边沿判定抽成导出纯函数（输入上次 fp + 本次 fp/干净态，输出动作枚举 `deliver|silent|close`），供冒烟测试（⑤ 措辞红线：纯函数判定不涉 DB）。

### D4 compact 与会话边界：清指纹重发一次

- `pi.on("session_compact")`：删除该 sessionId 的整个会话条目 → 下一次归档尝试所有 warn 重发一次（compact 会把早先 warning 摘要掉，重发是正确性要求，对齐 constraint-injection D4）。
- 会话边界：Map 以 sessionId 为键天然隔离，不跨会话复用；进程重启即全清（新进程 = 新会话集）。

### D5 不去重范围（审计边界）

`--force` / `SPEC_GATE_BYPASS=1` 豁免放行、门禁自身异常 fail-open、提取不到 change 名的 warning **保持每尝试一条**——它们各自是独立动作的留痕，静默会削弱审计；且它们天然低频。豁免路径与检查⑤/⑤' 的短路关系不变（豁免时⑤/⑤'本就不执行）。

### D6 block 语义零改动

检查①-④' 的失败收集、block reason 文案、`policy.decision(action=block, reasonCode=archive-check-failed)` 每尝试一条、`ui-design-gate` 代记 block——全部不动。本 change 只触碰两个 warn 投递点与其记账时机。

### D7 异常 fail-open

指纹计算 / Map 操作 / compact 回调抛异常时：按现状全量投递（等价于无状态），`console.warn` 一行，绝不阻断归档、绝不改变 block 裁决。

## Risks / Trade-offs

- **静默后内容不可回看** → compact 重发 + `concurrency-status.sh <change>` / `gate-status.sh` 拉模式查询兜底（上一 change 刚落的入口）；归档 warn 本就短平快，丢上下文的代价低。
- **指纹假阳（清单微变但语义同）** → 多一次 warn 而非漏一次，方向安全；排序去重已消掉最常见的顺序抖动。
- **⑤ 的「同指纹静默」可能盖住「同一关键词、不同任务行」** → ⑤a 未声明档的文案含关键词与任务行摘录（clip 60 字），任务行变化会进入指纹 → 视为变化重投，符合预期。

## Migration Plan

单文件改造，无数据迁移、无配置项新增；行为差异仅「重复 warn 消失」。回滚 = revert 单个文件。

## Open Questions

（无——方案在 propose 阶段已与用户对齐：会话内指纹 diff，不做磁盘持久化。）
