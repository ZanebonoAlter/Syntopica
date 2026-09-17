我现在已经拥有了所需的一切。崩溃路径已通过三种独立方式得到充分验证（JS 可选链语义、TS strict 模式对 `possibly-null` 的拒绝，以及作者自己的 `ExtCtx` 类型声明了 `| null` 加上 null stubs）。让我整理一下审查产物。

由于我这里没有 shell 工具，我无法自己运行冒烟测试（smoke suite）——预测的失败和所需的命令已记录在报告中，供主管（supervisor）执行。

---

# review-report-round4 — per-session-constraint-binding 轮次 4 增量（跨进程子线程继承）

- 审查方式：只读静态审查（read/grep）。**本会话无 shell 工具，未能实跑 `run-smoke.sh`**——需 supervisor 执行的命令见文末「必须复跑」。
- 审查对象：`.pi/extensions/constraint-injection.ts`（round 4 新增：`sessionIdFromSessionFile`/`parentSessionIdFromForkFile`/`parentSessionId` 双来源/`inheritFromParentHistory`、startup :1772 与 fork :1804 双路继承）、`.pi/extensions/tests/constraint-injection.smoke.cjs`（19.10-19.12 + helpers）、`apply-report-round4.md`。

## Review

### Correct（已验证正确的部分，带证据）

1. **事实库回退语义正确**（核查点 1）：`inheritFromParentHistory`（constraint-injection.ts:344）复用 `recoverMode`（:1419-1427）= `queryBySession(cwd, pid, ["mode.set"])`（ORDER BY id 升序，harness-log.ts:268-271）→ 取 `rows[rows.length-1]`（父会话**最近一条** mode.set）→ `recoverFromRow`（:1398-1411）：payload 损坏 → null；`boundChange` 非空但 `openspec/changes/<boundChange>` 目录已消失（已归档/删除）→ **null**。最近一条不可恢复时**不回捞更早记录**，与 resume/reload「同 id 单段」既有语义完全一致 → 子会话落到「未激活」，不会借父会话更早的档位。✔
2. **`||` 短路顺序正确**（核查点 2）：`inheritFromParent(state, ctx) || inheritFromParentHistory(state, ctx)`（:1772 startup、:1804 fork）——内存优先。内存继承成功即 `return`，事实库不再执行 → **不可能用历史 mode.set 覆盖内存现状**（内存态独有 channel 快照/指纹/命中集）。若顺序反了：同进程 fork 会被父会话最后一条 DB 记录覆盖内存态并丢失快照/指纹（动态层可能重发）。另：内存父态 `mode===null`（如刚 /new）时 `inheritFromParent` 也返回 true → 有意跳过历史陈旧绑定（apply-report 残留风险 3 自述一致）。✔
3. **记账归属正确**（核查点 3）：两处均为 `logModeSet(ctx.cwd, realSessionId(state), …, "inherit")`（:1776、:1806）——`realSessionId(state)` 即**子会话**自身 id（`sessionKey` 来自 `getSessionId()`，:225-228），不会误记父会话名下。19.10 断言显式核对 `session_id === 'ci-smoke-child-xproc'`（smoke:874）。history 路径 `rec.mode` 恒非空（`recoverFromRow` 校验后才返回）→ 必记账；内存路径父态未激活时「空继承」不记账——合理：`logModeSet` 签名要求 `Mode` 非空，「继承到未激活」不是绑定事件，子会话后续靠自身输入绑定会有 command/edit-dir 记录，「隐性绑定不存在」不受影响。✔
4. **失败方向安全（除 P1 外）**（核查点 4）：父 id 无（header 无 + 非 forks 路径）→ `parentSessionId` null → 两函数 false → 未激活；`forksIdx < 1`（`forks` 为路径首段）→ null（:293）；父目录名无 `_` → null（:295-296）；事实库无记录/查询异常 → `queryBySession` 内部 try/catch 返回 `[]`（harness-log.ts:271-277）→ rec null → false；`recoverMode` 抛错（如 openDb 异常）→ `inheritFromParentHistory` 的 try/catch 兜底 console.error + false（:348-356）→ 不阻断 session_start；兜底槽（无 sessionId）→ `logModeSet` 直接跳过（:1357）。全部退化为「未激活」，绝不借用他人。✔
5. **smoke helpers 实现正确**（核查点 5 一部分）：`seedModeSet`（smoke:667-674）写 `path.join(dir, '.pi', 'harness', 'events.db')`，被测代码 `openDb` 读 `join(cwd, ".pi/harness", "events.db")`（harness-log.ts:82,161）——**cwd 一致**（dir = tmp = 各 ctx.cwd）；events 表在 17.x 已由扩展建好；seed 行 source='skill' 且插入于 `beforeXproc` 计数**之前** → 不污染 `.slice(beforeXproc)` 窗口。fixture `smoke-fixture-budget` 目录在 tmp 存在（smoke:87-90）→ `recoverFromRow` 的 existsSync 校验通过。✔
6. **19.10-19.12 设计上的非空性/判别力**（核查点 5 静态部分）：父 id `ci-smoke-parent-xproc` 全文件从不作为任何 ctx 的 sessionId → 内存路径必 miss；子会话 id 全新 → 自身无 mode.set 行 → 回退被短路时必未激活 → 19.10 断言 1 必红、断言 2（依赖档位激活）必红、断言 3（`length===1`）必红；19.11 同理必红。19.12 负向断言对「回退短路」探针恒真（预期），但对「借全局最新 mode.set」类错误**有判别力**（tmp 库已有 ci-smoke-A 的 implementation 行，错借即红）。与报告 §3 探针自述（恰 4 红、19.12 保持绿）形态吻合——**但见 P1-1：该探针结果与当前源码不可调和**。✔（设计层面）
7. **19.1-19.9 未被破坏**（核查点 6，静态）：旧 stub 无 `getHeader` → 可选链短路 → `sessionIdFromSessionFile(undefined)` → null；无 `getSessionFile` → `parentSessionIdFromForkFile(undefined)` → null。19.4 走内存继承路径不变；19.4b orphan（'no-such-parent'）内存 miss → 新增历史查询无行 → 仍未激活，断言不变红。19.5-19.9 的计数断言全是前后差值/绝对 id 集合，19.10-19.12 新增 2 条 inherit 行不在其窗口（19.6 的 `msBeforeLock` 在其后重取）。`mode.set source` 全集断言（smoke:731-734）已含 'inherit'。✔

### Fixed

无——本轮只读，未改任何文件。

### Finding

**P1-1【High】`parentSessionId` 在 `getHeader()` 返回 null 时抛 TypeError——round 4 核心场景（header 未落盘 → 路径兜底）必崩**
- 位置：`.pi/extensions/constraint-injection.ts:306`
- 证据链：
  1. 源码（已逐字节复核两次）：`ctx?.sessionManager?.getHeader?.()?.parentSession`——`?.()` 只防 **callee** 为 nullish，不防**调用返回值**；返回 null 后的 `.parentSession` 是普通成员访问 → `null.parentSession` → `TypeError: Cannot read properties of null (reading 'parentSession')`。TypeScript strict 下该表达式本身报 TS18047（"Object is possibly 'null'"）——类型系统直接拒绝这行代码（esbuild bundle 不做类型检查所以漏过）。
  2. 作者自己的契约：`ExtCtx` 类型声明 `getHeader?: () => { parentSession?: string } | null`（:1604）——**null 是声明的合法返回**。
  3. 触达路径：session_start startup（:1772）/ fork（:1804）→ `inheritFromParent`（:318）或 `inheritFromParentHistory`（:345）→ `parentSessionId` → **全链路无 try/catch**；smoke harness 的 `emit` 也不 catch（smoke:148-152）→ 异常直冲顶层 catch。
  4. **round 4 自己的两个新 stub 恰好构造了这个输入**：`getHeader: () => null`（smoke:884 19.11、:904 19.12）→ 实跑 `run-smoke.sh` 必在 19.11 的 emit（smoke:887）处崩：由于所有 check 在套件末尾才打印（smoke:~1040），实测表现会是**零 ✅ 行 + 一行 `FAIL TypeError …` + exit 1**，而非报告所称的 189 项全绿。
  5. 这正是本特性的**设计主场景**：「header 未落盘/契约变化时仍可从 forks 路径兜底」（:301-302 注释、19.11 用例意图）——当前代码使该场景无法工作（且在生产里会让子线程 session_start 抛错）。
- 最小修复（一行，改实现不改测试）：:305-307 改为先取 header 再可选访问，例如：
  ```ts
  const header = ctx?.sessionManager?.getHeader?.();
  const fromHeader = sessionIdFromSessionFile(header?.parentSession);
  ```
  （或等价地把末段 `.parentSession` 改为 `?.parentSession`。）`sessionIdFromSessionFile` 本就接受 `string | undefined | null`（:279-285），undefined 会正确落到 forks 路径兜底。修复后静态推演：19.11（header null → forks 路径 → 父 xproc → DB rec）与 19.12（header null + 非 fork 路径 → 未激活）均按设计走通。

**P1-2【High】`apply-report-round4.md` §2/§3 的命令结果与当前源码不可调和**
- 位置：`openspec/changes/per-session-constraint-binding/apply-report-round4.md` §2「SMOKE OK，189 项检查」+ §3 探针「恰 4 红」。
- 证据：同上 P1-1——当前源码下套件在 19.11 处 abort，既不可能打出报告所列的 5 条新 ✅，也不可能完成「短路探针 → 恰好 4 红、其余绿」的完整跑（探针 run 同样会先崩在 19.11）。说明报告的命令结果不是对当前这份源码跑出来的（最大嫌疑：:306 的末段 `.`/`?.` 在探针/跑测之后被后续编辑改坏，未复跑）。
- 最小修复：应用 P1-1 修复后**重跑** §2 两条命令并**重做** §3 探针（含 sha256 前后一致复验），按实际结果修订报告；如实跑确认了本审查预测（见下「必须复跑」），在报告里补记本次偏差。

**P2-1【Medium】`ExtCtx` 类型缺 `getSessionFile` 声明（潜伏，当前无门禁会拦）**
- 位置：constraint-injection.ts:1598-1606（类型）vs :309（使用 `ctx?.sessionManager?.getSessionFile?.()`）。
- 证据：strict tsc 会报 TS2339；当前 extensions 无 typecheck 门禁（run-smoke.sh 仅 esbuild bundle），属潜伏债——建议随 P1-1 一并补 `getSessionFile?: () => string | undefined | null;`。这也反证了 :306 的漏防：同函数内 :309 把调用结果**整体当参数传**（安全），:306 对调用结果做**成员访问**（不安全）。

**P2-2【Low，report-only】`getHeader`/`getSessionFile` 回调本身若抛错无防护**
- 位置：constraint-injection.ts:304-310。可选链只防 nullish，不防回调 throw。理论性（pi 的 accessor 预期不抛），round 1-2 已存在同一面、非本轮恶化；不要求本轮处理，记录备查。

### 重点核查 1-6 结论速览

| # | 结论 |
| --- | --- |
| 1 事实库回退正确性 | ✔ 复用 `recoverMode` 单段语义；父记录不可恢复（payload 损坏/绑定 change 已归档）→ null → 子会话未激活，不借更早档位 |
| 2 `\|\|` 短路顺序 | ✔ 内存优先，历史不可能覆盖内存现状；顺序反了才会用陈旧 DB 记录覆盖并丢快照/指纹 |
| 3 记账归属 | ✔ 记在子会话（`realSessionId(state)`）；空继承不记账合理 |
| 4 失败方向安全 | ✔ 除 :306 外全部退化「未激活」；查询异常有 try/catch；**:306 例外即 P1-1** |
| 5 smoke 非空性 | ✔ 设计上 19.10×3+19.11×1 对回退短路必红、19.12 对「借全局」有判别力；seedModeSet cwd/时序一致。**实际运行被 P1-1 挡死，须复跑证实** |
| 6 回归 19.1-19.9 | ✔ 静态无破坏；但套件因 P1-1 在 19.11 abort，修复后需全量复跑确认 |

### 已验证假设（供主线程复核）

1. `queryBySession` 的 `ORDER BY id` = 插入顺序（rowid 单调），`rows[last]` 即最近一条（harness-log.ts:262-271）。
2. 本会话无 shell 工具，未能实跑 smoke；「实跑必崩」是基于源码语义的确定性推演（JS 可选链短路语义 + TS strict 拒绝该表达式 + 作者类型契约/stub 三方互证），最终以实跑为准。
3. 采信报告自述的「2026-09-17 实测 pi header 带 `parentSession`、fork 文件布局、子线程独立进程」为已验证事实（本轮未能在真实链路复测）。
4. 报告 §5 残留风险 5（轮次 3 探针污染真库 31 行 `session_id='GLOBAL'`）未核验，交主线程处理。

### Merge verdict: **BLOCK**

阻塞项仅 P1-1（一行修复）+ P1-2（修复后重跑并修订报告）。修复后静态推演全部用例应绿（含 19.1-19.12 与两套 harness smoke）；无需扩大改动面。

### 必须复跑（supervisor 执行）

1. 修复前（可选，用于证实本预测）：`bash .pi/extensions/tests/run-smoke.sh` → 预期 `FAIL TypeError: Cannot read properties of null (reading 'parentSession')`，无任何 ✅ 行，exit 1。
2. 应用 P1-1 修复后：
   - `bash .pi/extensions/tests/run-smoke.sh` → 预期 SMOKE OK（189 项，含 19.10×3 / 19.11×1 / 19.12×1）；
   - 按 apply-report §3 重做探针（`inheritFromParentHistory` 首行短路）→ 预期恰 4 红（19.10×3 + 19.11×1），19.12 保持绿；恢复后 sha256 与修复版一致；
   - `bash .pi/extensions/tests/run-harness-smoke.sh` → 预期全绿；
   - 按 P1-2 修订 apply-report-round4.md 的命令结果段。

---

以上即完整审查产物（对应持久化目标 `openspec/changes/per-session-constraint-binding/review-report-round4.md`）。一句话总结：**轮次 4 的设计方向与绝大多数实现是对的（回退语义、短路顺序、记账归属、失败方向、测试判别力全部经静态验证成立），但 `parentSessionId` 里一个 `.` 应为 `?.` 的守卫缺失，恰好击穿了本特性最想保护的「header 缺失走路径兜底」场景，并使 smoke 套件当下必红、报告的绿结果失效——一行修复加全量复跑后即可解除 BLOCK。**