## Context

见 `proposal.md` — Why。设计需要额外知道的现状约束：

- 事实账本已是**单表 append-only**（`.pi/harness/events.db`，`events(id, ts, session_id, kind, change, payload)`），`payload` 是 JSON 文本；写入方唯一（`.pi/extensions/lib/harness-log.ts`），既有 spec `harness-fact-log` 已声明「使用统计 SHALL 通过对既有事件的 SQL 聚合表达，MUST NOT 引入新表或物化列」——本 change 是该原则的第二个消费者。
- `gate.check` 成功侧**采样记账**：会话内首成功与失败转绿锚点必记，其后每 N 次连续成功记 1 条（缺省 N=5，payload 带 `sampled`、`n`、`flip`）。因此**落库条数 ≠ 执行次数**，报告必须还原分母，否则失败率被系统性高估。
- 各 kind 有 TTL（多数 30 天，`session.start` 90 天），过期行在开库时被清扫 → 长窗口统计会静默少数据。
- 既有相关能力：`harness-facts` skill（人工考古归因）、`scripts/*.sh` + `scripts/*.smoke.sh` 的双文件惯例（fixture 断言退出码与关键字）、`concurrent-change-coordination`（并发 change 共享工作树，重命令需错峰）。

## Goals / Non-Goals

**Goals**

- 一份**确定性、零 token、可复算**的失败聚类报告：同样输入 → 同样输出，任何数值可被独立 SQL 复算。
- 把「改了一条 harness 规则，它到底有没有用」变成可回答的问题（基线 ± 差值）。
- 明确区分「agent 犯错」与「harness 自身故障」，避免改进方向被环境故障带偏。

**Non-Goals（设计层边界）**

- 不做判定（报告不返回「门禁该不该改」的结论，退出码不表达质量好坏）。
- 不做自动修复、不自动开 change、不 auto-tune 阈值。
- 不纳入 pi 扩展运行时（不注册任何 hook，因而不进入「不回检 → 无 hooks」的正/负反馈链）。

## Decisions

### D1 形态：独立 bash 脚本 + `sqlite3` CLI（而非 pi 扩展 / Node 模块）

选择 `scripts/harness-retro.sh`，与 `scripts/` 既有惯例（`scenario-trace.sh` / `concurrency-status.sh` / `doc-impact.sh`）一致：`set -u`、中文输出、类型化退出码、配对 `.smoke.sh`。

- **备选 A：放进 `.pi/extensions/` 作为 TS 模块 + hook。** 否决——报告一旦进 hook 就有进入注入通道的诱惑，且扩展只在 pi 会话内可用，终端/CI 跑不了；也与 spec「输出形态不进入注入通道」冲突。
- **备选 B：复用 `lib/harness-log.ts` 的 `queryBySession/queryByChange`。** 否决——该接口按 session/change 取行，而报告需要 `json_each` 展开、跨会话窗口聚合与加权求和；且脚本要能在 pi 之外运行。

### D2 读取：只读打开 + 库身份校验，SQL 一次算好

`sqlite3 -readonly` 打开，先 `PRAGMA application_id` 校验魔数 `0x53594E54`，再做查询。加权逻辑集中在 SQL（`SUM(CASE WHEN ... THEN n ELSE 1 END)`）而不是把行拉到 bash 循环——bash 无浮点，聚合下推到 SQLite 可避免截断与本地化差异，并让「同一数值可被独立复算」成立。

### D3 分母还原：按 `gate.check` 采样协议加权

还原规则与 `harness-fact-log` 的 `gate.check` 记账规范一一对应：

| 落库行 | 计入执行次数 |
| --- | --- |
| `ok=false` | 1 |
| `ok=true`，无 `sampled` 标记（会话内首成功 / 翻转锚点） | 1 |
| `ok=true`，`sampled=true` | payload 的 `n`；缺 `n` 时回退缺省 N=5 |
| `ok=true`，`sampled=true`，`n` 非法（0 / 负数 / 非数字） | **1**（保守：宁可高估失败率，也不放大分母掩盖失败）+ 降级警告 |
| payload 不可解析 / 缺判定字段 | 0（跳过该行）+ 一次性降级警告 |

缺省 N 取 5，与记账侧缺省一致（减少记忆负担，且记账侧记录 `n` 时以记录值为准）。

### D4 输出：默认人读中文报告，`--json` 供基线与机器消费

默认输出按六段组织的中文文本（终端一眼可读）；`--json` 输出同结构的机器可读形态。基线快照 = `--json` 的指标子集 + 元信息（窗口、生成时间、脚本版本），因为基线必须**稳定可解析**（否则差值比对会静默失效）。

### D5 基线位置：`.pi/harness/retro-baseline.json`（git 忽略区）

与 events.db 同目录，天然与账本同生命周期、不污染 git 工作树（避免每次运行制造脏文件，与 `concurrent-change-coordination` 的「不制造混合脏树」一致）。备选「入库做历史曲线」否决：收益是可选可视化，代价是每次运行产生脏文件。

### D6 参数与缺省

| 参数 | 缺省 | 说明 |
| --- | --- | --- |
| `--db <path>` | `.pi/harness/events.db` | 目标账本路径；供 fixture 库（冒烟测试）与换机场景指向其他账本 |
| `--days N` | 7 | 窗口；超过被消费 kind 的最短保留期时输出超期提示 |
| `--change <name>` | 空（全局） | 按 `change` 列限定归属 |
| `--warn-threshold N` | 5 | 软提醒失效阈值（同 `policy+reasonCode` 的 warn 计数） |
| `--json` | 关 | 机器可读输出 |
| `--save-baseline [path]` | `.pi/harness/retro-baseline.json` | 落基线快照 |
| `--baseline [path]` | 同上 | 指定比对基线；不可解析时降级 |

7 天窗口的理由：`gate.check` 保留 30 天，7 天既够形成统计信号，又不至于被历史噪声淹没。

### D7 故障段与失败段的分离口径

「harness 自身故障」段只收 `policy.decision` 中 `action=fail-open` 的事件；`block` / `warn` / `bypass` 不进该段（它们是策略**正常工作**的结果，`block` 与 `warn` 另有归宿：warn 进第④段，block 作为独立指标计数但不参与 agent 失败数）。agent 失败数只由 `gate.check ok=false` 构成。

### D8 不接 hook，只在流程文档里留「可选一步」

不改任何 extension。在 `docs/reference/开发执行规范.md` 的归档流转一节补一句「归档后可选跑 retro，观察该类事件是否下降」——把闭环挂在**人的习惯**上，而不是新增运行时耦合。

## Risks / Trade-offs

- **[payload 字段漂移导致静默错算]** → smoke fixture 固定字段契约（缺关键字段的库作为独立 case，报告须输出降级警告而非算 0）；spec 侧声明字段为稳定契约。
- **[窗口超 TTL 读出「虚假改善」]** → 超期提示（spec requirement），并在报告头部固定打印窗口与最早事件时间。
- **[WAL 下只读打开可能触碰 sidecar]** → `sqlite3 -readonly` 优先；断言口径定为「库文件与已存在 sidecar 的内容字节不变」，而非「目录无新文件」；若实测发现只读打开在 WAL 库上失败，回退 `immutable=1`（可能少最后几条未 checkpoint 事件，属可接受取舍，须在报告头标注）。
- **[报告本身变成无人跑的新噪声]** → 唯一可证伪产出是「基线 ± 差值」；skill 把「怀疑某规则无效」列为明确触发场景，且差值口径与后续任何 harness 改进 change 的验收指标复用。
- **[加权口径实现错]** → 白盒用例（`test-cases.md`）覆盖分支表与边界（全采样、无锚点、缺 `n`、混合），且 smoke 断言分母字样出现。

## Migration Plan

纯新增，无数据迁移、无 schema 变更、无配置项。落地即用（`bash scripts/harness-retro.sh`）。回滚 = 删除新增文件，无残留状态（baseline 文件为可丢弃快照）。

## Open Questions

- 是否把「归档前跑一次 retro 并把差值写进 change 的验证节」升级为归档门禁的一部分（`spec-gate` warn 级）？—— 待积累使用数据后再定，不影响本次 specs / 设计与任务拆解。
- `--json` 是否作为对外稳定契约长期维护（供未来的 dashboard）？—— 首版只承诺服务基线比对。
