---
name: harness-facts
description: Syntopica harness 事实库（.pi/harness/events.db）查询指南。当需要排查"当时为什么注入了某个约束"、constraint.inject / gate.check 历史记录、事件归因（某事件为何落在某 change 名下）、session 档位切换 / reload 重绑问题、pin.write 落盘审计、子线程 Agent 派发历史等"事件考古 / 归因排查"类问题时必须先查本 skill。含 schema、事件类型与保留期、payload 字段、查询配方与归因方法论。
---

# Harness 事实库（events.db）

## 是什么（先分清词撞车）

**harness 事实库** = `.pi/harness/events.db`：pi 扩展自动写入的 SQLite 事件账本（模型零参与），记录约束注入、质量门禁、档位切换、pin 落盘、子线程派发等 harness 层事实。

⚠️ 它**不是** context-mode 的 FTS5 knowledge base（`ctx_search` 那套）。用户说"查事实库/查事件记录"时，指本库，别去调 ctx_search。两者都挂时（比如 CONTEXT_MODE_DIR 报错），本库用 `sqlite3` CLI 直接查，不受影响。

- 位置：`.pi/harness/events.db`（WAL 模式，busy_timeout 5000，CLI 直接读无碍；`.pi/harness/` 已 gitignore，是本机运行数据）
- 设计文档：`docs/research/harness事实库.md`
- 写入方（全部在 `.pi/extensions/`，gitignored；入库代码快照在 `docs/research/`）：
  - `constraint-injection.ts` → `constraint.inject` / `pin.write` / `pin.read` / `mode.set`
  - `quality-gate.ts` → `gate.check`（ok=true 采样记账：会话首条与转绿锚点必记 flip、每 5 连续成功记 1 条 sampled+n；ok=false 全量含 diag；同根因短路时未执行命令零记账）+ `edit.map`（coordinate-concurrent-changes：turn_end 增量路径 × mode.set boundChange 聚合的 change→文件归属地图，落库侧并集累计快照，取最新一条即完整集合）
  - `entry-gate.ts` → `gate.check`（cmd=entry-gate，复杂档缺 test-cases 文档提醒）
  - `tool-output-spill.ts` → `spill.write`
  - `harness-telemetry.ts` → `session.start` / `subagent.dispatch` / `subagent.complete`
  - `spec-gate.ts` / `quota-gate.ts` / `test-scope-guard.ts` / `ui-design-gate.ts` → `policy.decision`（显著裁决统一记账，harden-harness-policy-and-spill 引入；普通放行零记录；ui-design-gate 为 make-ui-design-first-class 引入）

## Schema 与事件类型

单表 `events`：`id INTEGER PK, ts TEXT(ISO), session_id TEXT, kind TEXT, change TEXT NULL, payload TEXT(JSON)`。索引：`(session_id,id)`、`(change,id)`、`(kind,ts)`。append-only，除 TTL 清扫外不删。

| kind | 保留期 | payload 关键字段 |
| --- | --- | --- |
| `session.start` | 90 天 | `reason`(new/reload/resume/fork)、`cwd`、`prev`(前一 session 文件) |
| `constraint.inject` | 30 天 | `path`、`mode`(full/section)、`reason`、`bytes` |
| `gate.check` | 30 天 | `cmd`、`phase`(turn_end)、`ok`、`ms`、`diag` |
| `mode.set` | 30 天 | `mode`、`boundChange` |
| `subagent.dispatch` | 30 天 | `type`、`model`、`desc`、`ms`、`tokens`、`status`、`agentId`、`isError` |
| `subagent.complete` | 30 天 | `agentId`、`status`、`ms`、`tokens`、`toolUses`、`isError`（后台子线程完成回填） |
| `spill.write` | 30 天 | `tool`、`bytes`、`path`、`ok`（大工具结果落盘记账） |
| `pin.write` | 永久 | 见 pin_finding 写入方 |
| `edit.map` | 30 天 | `paths`（该 change 累计编辑路径全量快照，排序去重）、`n`（=paths.length）；change 列 = 会话绑定的 change（mode.set boundChange 语义）；归属地图查询首选 `bash scripts/concurrency-status.sh [change]`（人读三段）或 `--check <change>`（exit 0 干净/2 有归属其他 active change 脏文件/3 冷启动） |
| `policy.decision` | 30 天 | `policy`、`action`、`reasonCode`，按需 `target`/`durationMs`（见下） |

`constraint.inject` 的 `reason` 枚举（实测）：`index`（未激活档的常驻索引）/ `mode-base`（档位激活后的基础注入）/ `declaration`（按 proposal 头 `constraint-domains` 声明拉 flow 约束节，**声明域=红线层注入**：payload 附 `layer`（`redline`=红线层 / `full`=提取 0 条或低于 512B 回退全节），bytes 为实际注入层级字节数）/ `keyword`（对话关键词命中，全节注入）/ `edit`（编辑路径 JIT 命中，全节注入）/ `change-file`（change 文档命中）/ `stack-conditional`（栈条件注入）。

`policy.decision` 的稳定枚举（查询契约，smoke 精确断言，漂移即 bug）：

- `action` 四值：`block` / `warn` / `bypass` / `fail-open`；白名单外拒绝写入。
- `reasonCode`（kebab-case，非法归一 `unknown`）：
  - spec-gate → `archive-check-failed`(block，target=失败检查名如 `doc-impact,trace,ui-evidence`) / `explicit-bypass`(bypass) / `acceptance-wording`(warn，检查⑤)；另在 UI 验收证据缺失时代记 `ui-design-gate` policy 的 block 事件；检查⑤'归档并发（coordinate-concurrent-changes）→ `concurrent-dirty-tree`(warn，树上存在归属其他 active change 的未 commit 文件)
  - quota-gate → `quota-low` / `quota-exhausted`(block) / `quota-query-failed`(fail-open) / `fuzzy-model-resolve`(warn，裸模型名解析风险)
  - test-scope-guard → `full-go-test`(soft=warn / hard=block)
  - ui-design-gate → `ui-impact-missing` / `ui-impact-mismatch` / `ui-design-missing`(legacy 前端迁移提醒为 warn) / `ui-prototype-missing` / `ui-approval-pending`(block) / `explicit-bypass`(bypass，UI_DESIGN_GATE_BYPASS=1) / `ui-gate-check-failed`(fail-open)；归档侧 UI 缺证据 → `ui-verification-missing`(block，由 spec-gate 检查④'代记，target=archive)。白名单共八值（与主 spec ui-design-workflow 对齐）；健康放行/requirements 档/legacy 非前端操作零记录
- 低噪声约束：普通成功放行/未命中/健康额度**零记录**；quality-gate/entry-gate 继续只用 `gate.check`，同一裁决不双写。`target` 仅 change/provider 等短摘要（截断 120 字符），禁止命令、密钥、响应正文。记账失败仅旁路（fail-loud console.error + 返回 false），绝不改变原门禁裁决。

## 查询配方

```bash
cd .pi/harness
# 1. 全局账目：各 change 名下有哪些事件、多少条（归因排查第一步）
sqlite3 events.db "SELECT COALESCE(change,'-'), kind, COUNT(*) FROM events GROUP BY change, kind ORDER BY change;"

# 2. 按 change 查时间线（payload 截断看头部）
sqlite3 events.db "SELECT ts, kind, substr(payload,1,200) FROM events WHERE change='X' ORDER BY id;"

# 3. 单条 payload 全文（-line 竖排易读）
sqlite3 -line events.db "SELECT * FROM events WHERE id=1680;"

# 4. 某 session 内多 change 交织（分钟桶，看切换节奏）
sqlite3 events.db "SELECT substr(ts,1,16), COALESCE(change,'-'), COUNT(*) FROM events WHERE session_id='S' AND kind='gate.check' GROUP BY 1,2 ORDER BY 1;"

# 5. session 锚点（reload 清档 / 重绑时点判定）
sqlite3 -line events.db "SELECT ts, payload FROM events WHERE kind='session.start' AND session_id='S';"

# 6. 档位切换史
sqlite3 events.db "SELECT ts, session_id, payload FROM events WHERE kind='mode.set' ORDER BY id;"

# 7. 策略干预复盘（harden-harness-policy-and-spill）：近 30 天各 gate 的 block/warn/bypass/fail-open 频次
sqlite3 events.db "SELECT json_extract(payload,'$.policy'), json_extract(payload,'$.action'), COUNT(*) FROM events WHERE kind='policy.decision' GROUP BY 1,2 ORDER BY 1,2;"

# 8. 策略降级时间线（fail-open/block 细节，看 reasonCode 与 target）
sqlite3 events.db "SELECT ts, COALESCE(change,'-'), payload FROM events WHERE kind='policy.decision' AND json_extract(payload,'$.action') IN ('block','fail-open') ORDER BY id;"
```

## 附：并发态势查询（coordinate-concurrent-changes）

"现在还有谁在跑 / 树上脏文件归属谁"类问题，**先跑脚本再查库**（脚本一次聚合了活跃清单、归属对照、近期验证流水三段）：

```bash
bash scripts/concurrency-status.sh <change>      # 人读三段（活跃清单/脏文件归属/近 6h gate.check）
bash scripts/concurrency-status.sh --check <change>  # 机器可读：exit 0 干净 / 2 有归属其他 active change 脏文件 / 3 冷启动
# 归属原始数据（脚本的底层源）：每 change 最新一条 edit.map 快照
sqlite3 events.db "SELECT change, payload FROM events WHERE kind='edit.map' AND id IN (SELECT MAX(id) FROM events WHERE kind='edit.map' GROUP BY change);"
```

## 归因方法论（实战教训）

**事件的 `change` 字段 = 写入那一刻绑定的档位**，不是"这个 session 主做哪个 change"。踩坑三件套：

1. **同一 session 交替做多个 change**：gate.check / inject 会按当时档位分散落在不同 change 名下，账目"看着错"其实没错。
2. **reload 清零档位后重绑**：`session.start(reason=reload)` 后重新触发 apply 时，档位可能绑回**上一个 change**（不是当前在做的），此后注入全记在旧 change 头上——恰是 constraint-injection-tier-b 要修的 resume 恢复问题的现场证据形态。
3. **注入内容看 reason 就懂来源**：`declaration` = 机械按 proposal 头 `constraint-domains` 声明拉节（红线层，`layer` 字段标记层级），与"当时正在实现什么"无关；`keyword`/`edit` 命中 = 全节注入（细节层通道）。

排查顺序：① 全局账目定异常（如某 change 名下 0 条 inject 但同期别处有 declaration）→ ② 涉事 session 分钟桶看交织 → ③ session.start 锚点定位 reload/重绑时点 → ④ 对照归档 proposal 头部声明收口。
