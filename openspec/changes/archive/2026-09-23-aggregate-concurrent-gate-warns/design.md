## Context

quality-gate 的报告组装沿用 `gate-failure-reporting` 现行契约（首次全文 / 持续单行 / ≥3 回合「未修」/ 转绿一次），指纹状态机（`prev.diag / prev.rounds / prev.mixed`）同时驱动三件事：报告强度分级、粘性重跑集合、`concurrent-mixed` 记账时机。本 change 只动「注入与记账」两件事，不碰「是否重跑」。动机与数据见 proposal.md（retro ④段 41/31 次软提醒失效 + 2026-09-22 现场 5 连击样本）。事实库记账协议（转红全量、转绿必记翻转锚点、成功侧采样）见 skill `harness-facts`，是 gate-status 查询「最新条目=当前态」的正确性依据。

## Goals / Non-Goals

**Goals:**

- 状态未变的回合对 agent 零注入（含单行摘要与「未修」标记），状态变化（首现/指纹变/转绿）仍即时可见。
- `concurrent-mixed` 记账按指纹边沿，7 天计数 41 → ≤10（回检指标）。
- 提供只读状态查询入口，替代持续提醒的「查状态」需求。
- 包锚点型失败能参与归属判定，纯外部的包级失败不再 fail-open 成 [回归] 催错人。

**Non-Goals:**

- 不改触发集、粘性重跑语义、gate.lock 互斥、限核参数、spec-gate 归档硬门禁。
- 不改 `policy.decision` 词汇表（reasonCode/action 零增删）。
- 不改 `spec-gate/concurrent-dirty-tree`（本就是按归档尝试边沿）。
- 不治理并发本身（并行会话数、错峰属使用习惯，方向 C/D 已被否）。
- 归属地图对 `??` 未跟踪新文件的覆盖缺口**只诊断不扩 scope**（见 Migration Plan 末条）。

## Decisions

**D1 报告强度收敛落点：指纹状态机保留，只在 steer 组装处剔除「指纹未变」项。**
现有 `prev.rounds += 1` 的持续分支不再 push 任何失败文案（单行与「未修」一并移除）；首次/指纹变化分支保持 push 完整块；转绿分支保持单行收尾。剔空后本轮 failures 列表为空 → 不发 steer。
备选：连指纹状态机一起删——否决，它还驱动粘性重跑与记账时机，spec 明令重跑语义不变。

**D2 `concurrent-mixed` 记账边沿化：只在新指纹写入（或失败段重建）时调用 `logPolicyDecision`。**
复用 D1 的指纹判定点（`prev == null || prev.diag !== diag`），同一判定点同时决定「注入完整块」与「记 warn」，保证注入与记账两条边沿严格同相。转绿清除指纹后再次 mixed 视同新失败段，允许再记（spec scenario 已覆盖）。

**D3 包锚点解析：机械映射 + 与已知归属集合求交，不做目录内全文件枚举判定。**
提取规则覆盖 `# <pkg>` / `FAIL <pkg> [` / `vet: <file>:<line>` 三形态；`<pkg>`（module 相对路径）映射到 `backend-go/<pkg>` 目录前缀，P 加入「该目录 ∩（git 脏文件 ∪ 各 edit.map 归属集）」的路径。这样仍满足 spec「目录前缀精确映射、P⊆foreign 才降级」；映射结果为空 → 维持保守 fail-open。`vet: <file>:<line>` 本就是文件路径，直接并入。
备选：把整个包目录下所有文件塞进 P——否决，会把目录里与本次失败无关的本会话文件也算进来，造成误降级（错放 [外部]）。

**D4 `gate-status.sh`：sqlite3 只读 URI 打开 + 窗口函数取每 cmd 最新一条。**
`ROW_NUMBER() OVER (PARTITION BY json_extract(payload,'$.cmd') ORDER BY id DESC)` 取每命令最新 `gate.check`，按 `ok` 着色红/绿、diag 首行截断摘要；`file:events.db?mode=ro` 保证只读。退出码 0=查询成功（红态也算），1=库缺失/损坏（stderr 说明）。风格对齐 `concurrency-status.sh`（人读三段 + 明确空态）。
备选：读门禁进程内存状态——否决，扩展状态不跨进程，重启即失忆，而账本天然持久。

**D5 注入收敛不减信息总量的三路兜底**：首次全文（会话内已见过）→ `gate-status.sh`（随时主动查）→ spec-gate 检查①归档前全绿（硬门禁，红状态漏不掉）。AGENTS/pi-extensions 文档同步说明新姿势。

## Risks / Trade-offs

- [提醒变稀后 agent 忘红，聊着聊着以为全绿] → 转绿收尾行 + 归档硬门禁兜底 + 状态可查；tasks 验证节含「同指纹连续回合零注入」与「查询入口可读红态」用例。
- [指纹只取失败特征首行，尾部变化不触发重注] → 现行协议既有局限，本 change 不收紧也不放宽；指纹变化仍会重注（D1 保留该分支）。
- [gate-status 依赖 sqlite3 CLI] → 本机已有（harness-facts 基础假设）；缺失时按 D4 走 exit 1 报错，不静默。
- [包锚点映射后 P 变大，可能把本该 [回归] 的失败错放 [外部]] → 仅在「映射路径全部 P⊆foreign」时降级，且映射结果与已知归属集合求交（D3），无归属即保守回退，spec 三 scenario 锚定边界。
- [`??` 新文件不在 edit.map 时包锚点修复也救不了今天的场景] → 归属集合含 git 脏文件基线（含 untracked），会话启动后新建的 `??` 文件若未被对方 edit.map 记录则仍 fail-open——已列为诊断任务，超出 scope 则另行立项，不在本 change 硬修。

## Migration Plan

无数据迁移。部署=提交 `.pi/extensions/quality-gate.ts` 改动后重启 pi 会话（扩展热加载不保证）+ 新脚本入库。回滚=revert 该文件、删除 `gate-status.sh`、还原两份文档即可，账本无不可逆变更。

诊断项（不阻塞归档）：apply 阶段顺手用 harness-facts 配方验证「board-signal-reports 的 edit.map 是否覆盖其 `??` 新文件」；若确认缺口，把证据写进本 change 的 explore-findings.md 并在汇报中建议另行立项（不改本 change 的 specs/tasks）。

## Open Questions

（无——`??` 归属缺口按上文 Migration Plan 处置，不阻塞本 change 的规格与任务拆分。）
