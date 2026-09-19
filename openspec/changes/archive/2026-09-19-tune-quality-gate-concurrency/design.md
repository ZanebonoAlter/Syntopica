## Context

quality-gate（`.pi/extensions/quality-gate.ts`，turn_end 挂钩）现状：触发判定与增量路由已有（git 快照 trigger set → backend/frontend 侧），命令执行链为 lint 哨兵 → vet+build `Promise.all` 并行 → 域测试串行（5min 预算）→ 前端 eslint。失败处理已有三分支（外部归因 / 同指纹抑制 / 首次分级 [回归]/[中间态]）+ 粘性重跑 + steer 注入。动机与数据见 proposal.md Why 与 `docs/research/quality-gate-concurrency/explore-findings.md`。

关键既有事实（设计输入）：
- pi 扩展是每会话独立 Node 进程，无共享内存；跨会话协调只能走文件系统（锁文件）或 events.db（既有 `lib/harness-log.ts` 同步 SQLite 写）。
- mine/foreign 集合已在 step 3.7 预取（gateLog 执行前），三态归因只改判定分支，不动集合构建。
- 既有逃生口惯例：interop-down / toolchain-down 短路都走 `logPolicyDecision` + fail-open，锁跳过沿用同模式。
- 单条门禁命令 timeout 上限 120s（锁 TTL 须大于它）。

## Goals / Non-Goals

**Goals:**
- 同一仓库同一时刻至多一个会话执行门禁命令；抢不到锁者本轮零成本跳过（不等待）。
- 单会话门禁 CPU 峰值 ≤ ~2 核（4 核机上留出 2 核给其他会话/前台任务）。
- 混合归属失败不再以 [回归] 措辞催修，agent 可辨识串扰；粘性与归档全绿语义不破。

**Non-Goals:**
- 不做门禁命令本身的增量收窄（`./...` → 影响包）——真增量命令（方案 C）留作后续 change。
- 不做排队等待：跳过即跳过，覆盖靠下回合复检 + 粘性兜底。
- 不做锁公平性调度（防饿死）——先观察账本 skip 率，出现长期饿死再加优先级。
- 不改前端 eslint（单进程单核，非叠加源）。
- 不动 interop / toolchain 健康探测与归因（gate-interop-health / gate-toolchain-health 语义原样）。

## Decisions

### D1 锁实现：O_EXCL 原子创建 + mtime TTL，非 flock

用 `fs.openSync(lockPath, "wx")`（O_EXCL）原子创建，失败即视为被持有；TTL 判定用锁文件 mtime（`fs.statSync().mtimeMs`）。**不用 flock**：pi 扩展无法保证关闭 fd 的时机（异常路径多），flock 释放依赖进程退出，Node 进程存活期间锁不会掉；O_EXCL + finally unlink + TTL 三层兜底与仓库既有 fail-open 风格一致，且锁文件内容可考古。

- 锁路径：`<repoRoot>/.pi/harness/gate.lock`（与 events.db 同目录，天然 per-repo）。
- 锁内容：单行 JSON `{sessionId, ts, cmd}`（释放前更新为最后执行的命令，供人工/账本排查"谁在跑"）。
- 抢锁时机：在 step 3.5（interop/toolchain 探测）**之后**、step 3.7（归因预取）**之前**——探测短路者不占锁（不跑命令就不该挡别人）；归因预取是纯 SQLite 读，放抢锁后可保证 mine/foreign 快照与命令执行时刻更近，代价是持锁期间一次本地读，可接受。两侧均短路（`!isBackend && !isFrontend` return）时同样不抢锁。
- 跳过路径：记 `logPolicyDecision(action:"fail-open", reasonCode:"gate-lock-held", target:"gate")` 后 return——零命令、零 gate.check、零 steer；粘性失败集合原样保留（内存态天然保留）。（修订 2026-09-19：原设计写 action:"skip"，但主 spec harness-fact-log 固定 action 枚举为 block|warn|bypass|fail-open，锁跳过与 interop-down/toolchain-down 同构——本轮整体跳过+零记账+后续自动恢复——故用 fail-open，delta spec 只约束 policy/reasonCode 不受影响。）
- 释放：`try { ...命令执行与 steer 段... } finally { unlock() }`，unlock 吞异常；释放前比对锁内容 sessionId，非本会话（已被 stale 覆盖）则不删（防误删他方锁）。
- TTL=180s：> 单条命令 120s 上限，取"大概率持有方已死"的保守阈值；万一活性锁被误判 stale（命令真跑了 3 分钟+），后果只是两把锁并存一回合，下一回合自愈（门禁只读不写仓库状态，双持锁无正确性损害）。

### D2 限核：GOMAXPROCS=2 前缀 + lint --concurrency=2 + vet/build 串行

- `runBackend` 的 cmdline 前缀统一加 `GOMAXPROCS=2 `（native bash -c 语境生效）；windows 分支 cmd.exe 形态为 `set GOMAXPROCS=2 && `（历史遗留路径保持同等限核语义）。
- golangci-lint 追加 `--concurrency=2`（与 `--allow-parallel-runners` 正交）。
- vet/build 从 `Promise.all` 改顺序 await；域测试循环本就串行，前缀生效即可。
- **不碰** turn_end 之外的手动命令（用户自己在 bash 跑的全量检查不受影响）。

### D3 三态归因：classifyFailureOwnership → 'foreign' | 'mixed' | 'mine'

- 判定（P = 失败路径集合；空/解析失败维持现状视同 mine）：
  - P∩mine=∅ ∧ P⊆foreign → `foreign`（既有语义不变）
  - P∩mine≠∅ ∧ P∩foreign≠∅ → `mixed`（新）
  - 其余 → `mine`（既有语义不变）
- `mixed` 处理：进粘性（`stickyFailures.add`）但**不取** `d.failPrefix`（[回归]/[中间态]），改 `[并发]` 前缀；正文列双方路径（他人 ≤3 条 + 本会话 ≤3 条，超出计数）+「可能非本会话所致，归档前仍需全绿」；失败指纹状态机复用（同指纹 ⟳ 单行抑制、转绿 ✓ 收尾），`failureReports` 条目加 `mixed: true` 标记，判性翻转（mixed→mine）时恢复完整块与正常分级。
- 记账：`gate.check` 照记（ok=false，diag 同源）；每命中回合记 `logPolicyDecision(action:"warn", reasonCode:"concurrent-mixed", target:cmd)`。
- steer 消息：mixed 失败行并入既有 failures 段（同一条 quality-gate-failure 消息），不新开消息类型——mixed 与 mine 可能同回合共存，分列即可。

### D4 记账词汇扩展（events.db payload）

新增两个 reasonCode：`gate-lock-held`（action=fail-open，见 D1 修订）、`concurrent-mixed`（action=warn）。`harness-facts` skill 的 schema 文档与 `docs/reference/harness/pi-extensions.md` 记账口径表同步补两行。无需 DB 迁移（payload 自由字段）。

## Risks / Trade-offs

- **锁跳过导致复检延迟**：极端下 B 会话连续撞锁多回合才跑上门禁。缓解：turn_end 之间天然有 LLM 思考间隙（数十秒），A 持锁只占命令执行段；账本可按 `gate-lock-held` 频率回检饿死率（harness-retro 指标），出现再治。
- **真回归被 [并发] 降级**：混合归属里若真回归来自本会话文件，降级只改措辞不改粘性——回合末复检与归档全绿仍会逼出修复。残留风险是 agent 优先级误判（先修别人的），靠提示中双方路径分列缓解。
- **限核拖慢单会话门禁**：冷缓存 vet/build 串行 + 2 核，最坏 ~2× 现时长，仍在 120s 预算内（现状 max 34.8s 是 4 核全速值；热缓存均值 2s 级几乎无感）。
- **TTL 误判活性锁**：见 D1，后果是短暂双持锁自愈，无正确性损害。
- **`.pi/harness/` 目录竞争**：events.db 多会话并发写已验证可行，锁文件无额外风险。
