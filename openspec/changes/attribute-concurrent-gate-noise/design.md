# Design — attribute-concurrent-gate-noise

## Context

动机与数据证据见 `proposal.md`（Why）。这里只列塑造方案约束的现状事实（2026-09-18 实测）：

**代码现状**

- `scripts/harness/doc-impact.sh`：`changed_files()`（约 47 行，全树 tracked+staged+untracked）、`ownership_paths()`（约 66 行，**已存在**，查单 change 最新一条 `edit.map` 的 paths 并挂 `sqlite3` 缺失/库不存在 fail-open 守卫）、`filter_blacklist()`（约 84 行）、`cmd_suggest()`（约 155 行，**目前吃全树**）。`cmd_verify()` 已实现双轨（归属优先 → 回退全树 + 分级）。
- `.pi/extensions/quality-gate.ts`：`snapshot`（`Map<path,{mtimeMs,size}>`）在 session_start 以当时 git 脏文件初始化，但 turn_end 里 `snapshot = curr` **每回合覆盖 → 会话启动基线丢失**；触发集 `computeTriggerSet(snapshot, curr)`（`lib/trigger-set.ts`）；`stickyFailures: Set<cmd>` 让失败命令下回合重跑（含纯对话回合）；step 5 `if (failures.length > 0) → pi.sendMessage({...failures.join(...) })` 每回合重发 `tail(output, 30)`；分级前缀由 `lib/gate-sample.ts` 的 `stepGateOk()` 给（`everGreen` → `[回归]`，否则 `[中间态]`）。
- `.pi/extensions/lib/failure-classify.ts`：已有 `truncateDiag()` / `truncateDiagGate()`（单行特征行提取，`GATE_DIAG_RULES`）/ `isInteropFailure()` / `isToolNotFound()`，是纯函数分类的既有落点。
- 可复用既有机制：`edit.map`（change 归属）、`lib/policy-decision.ts` 的 `logPolicyDecision()`（action 白名单 + kebab-case reasonCode）、`PI_SESSION_ID`（pi 注入 shell 环境，实测可用）、`mode.set` 的 `boundChange`（会话↔change 绑定）。

**约束**

- 宪法：不做 git worktree 隔离（主仓库直改），因此「并发共享工作树」是既定前提，只能靠归属判定降噪。
- `harness-fact-log` 的低噪声约束：普通成功放行/未命中不得记账；策略事件只在显著裁决时写。
- 现有事件词汇（`gate.check` / `edit.map` / `mode.set` / `policy.decision` / `session.*`）不轻动——新增事件类型要连带保留期与查询接口，成本高于收益。

## Goals / Non-Goals

**Goals**

- suggest 的预勾选在并发工作树上语义正确（只有本 change 的文件参与），且**不把无法归属的文件藏起来**。
- 同一门禁失败的重复注入量降到接近零，同时不削弱「失败必须被修」的强制力。
- 并发场景下「别人造成的失败」不再以 `[回归]` 催修本会话；判定只用可机证信号，无法判定时一律退回现状。

**Non-Goals**

- 不收窄门禁命令范围（`golangci-lint run ./...` → 指定包、`eslint .` → 指定文件）。它能根治跨会话误报，但会把「跨包破坏」的发现时间推迟到归档门禁，属门禁语义变更，另立 change 评估（`proposal.md` Impact 已声明）。
- 不重构 `concurrency-status.sh` 的归属对照，也不抽共享归属脚本库。suggest 需要的新查询（全部 change 的最新归属集合）就地放 `doc-impact.sh` 内；「共享只读单源」列为后续项（见 Open Questions）。
- 不新增事件类型：会话启动基线只在 quality-gate 内存里维护，本 change 不落 `session.baseline`；归因事实由既有 `policy.decision` 承载，不扩 `gate.check` payload（避免连带改 `harness-fact-log` spec）。
- 不改 `spec-gate` 检查⑤'（归档前 `concurrent-dirty-tree` warn）的语义；本 change 是把它前移到 turn_end 的补充，不是替代。
- 不改 `constraint-injection` 注入内容与策略；不改前端/后端产品代码。

## Decisions

### D1：suggest 走「归属优先 + 三桶分列」，不静默过滤

归属轨（`edit.map`）**只会少不会多**：只有「绑定 change 的会话在纯编辑工具回合产生触发」才记账（`quality-gate.ts` step 2.5 的 `EDIT_TOOLS` 门控），bash 改文件（`sed -i`/`gofmt -w`/代码生成）与子线程编辑都可能不进归属集合。因此：

- 预勾选只吃归属桶（少报靠下方复选框在 verify 阶段兜底）；
- 另两桶**列出来**，让 agent 用自己脑内的计划认领——这比静默过滤多花几行输出，但避免了「漏声明域 → verify 反向启发式永不再报」的静默失效。

*备选（已否决）*：把三桶合起来做「交集/差集」后静默过滤——归属轨漏记时会把本会话真实改动藏掉，比噪声更糟。

### D2：suggest 的 change 名三源解析，显式参数优先

`--change <name>` → `PI_SESSION_ID` 查库最新 `mode.set.boundChange` → 空。第三源不用 `detectActiveChange()` 的目录 mtime 启发式——正是它把今天的门禁事件挂到了无关 change 名下（`proposal.md` 表格末行）。三源全空即回退全树并标注。

### D3：失败指纹 = `(cmd, truncateDiagGate(output))`，状态只存内存

指纹直接复用 `lib/failure-classify.ts` 的 `truncateDiagGate()`（已按 `GATE_DIAG_RULES` 提取特征行，且是 gate.check 的 diag 同源函数，账本与报告口径一致）。状态 `Map<cmd, {diag, rounds}>` 挂 module，`session_start`（reason≠startup）/`session_shutdown` 与 `stickyFailures` 同点位重置。不落库：重复抑制是**会话内报告策略**，不是历史事实；历史事实已由 `gate.check` 全量失败记账承载。

*备选（已否决）*：把指纹写进 `gate.check` payload 做跨会话去重——跨会话复用会在「上会话已报过、本会话首次遇到」时压掉首次完整块，把新会话的最强信号削掉。

### D4：报告强度递变（首次全文 → 单行 → 未修标记 → 转绿收尾）

- 首次/指纹变化：完整块（分级前缀 + `tail(output, 30)`）。
- 同指纹持续：`⟳ [cmd] 同一失败第 N 回合未变化：<diag>`。
- N ≥ 3：追加「未修」标记（一句，不是新的消息）。
- 转绿：`✓ [cmd] 已转绿`（每失败段至多一次）。

强制力来源是**粘性重跑**（不变）而不是重复文本；重复同样的字节几乎不提升遵从度，只是烧上下文（实测 91% 的失败行是重复注入）。

### D5：外部判定用「会话启动基线 + 归属集合」双信号，只降级有正证据的

`mine = 本会话累计触发集 ∪ 本 change 归属集合`；`foreign = 会话启动基线 ∪ 其他 active change 归属集合`。判外部的充分条件：`P ≠ ∅ ∧ P ∩ mine = ∅ ∧ P ⊆ foreign`。

- 会话启动基线：新增 module 变量 `baselinePaths`，session_start 初始化一次后**不再覆盖**（现有 `snapshot` 仍按原语义每回合更新）。
- 累计触发集：新增 module 变量 `accumulatedTriggerPaths`（`Set<string>`），每个跑过门禁的回合将当回合 `trigger` 并入（只增不减），重置点位与 `baselinePaths` 一致。**不得**用 edit.map 归属集合顶替：`syncEditMap` 只记 `EDIT_TOOLS` 回合，bash 编辑（`sed -i`/`gofmt -w`/代码生成）的文件只进当回合 trigger、不进 edit.map——若 `mine` 只含当回合 trigger，该文件从下回合起即不在 `mine`，恰落在其他 change 归属时会被误判 `[外部]`（apply 前评审补丁 P1-1）。
- 归属集合：`lib/edit-map.ts` 增一个只读查询（`latestEditMapByChange(cwd)`：`id in (select max(id) ... group by change)`）拿全部 change 的最新归属；本会话 change 的集合进 `mine`，其余进 `foreign`。
- 无绑定 change / 无 edit.map / 库不可用 → 该信号缺席，判定退化为基线单信号（**不扩大**外部面）。

**为什么两信号都要**：只靠基线漏掉「会话中途别的会话新脏的文件」（那文件不在我的基线里，但确实不是我的）；只靠归属集合漏掉「bash 编辑或子线程改的、无归属记录的文件」。两者并集仍是**保守判据**（只用于判定「不是我的」，不用于判定「是我的」）。

*备选（已否决）*：靠 `git blame`/mtime 推断文件作者——弱信号，且会引入新的误归因通道。

### D6：路径提取做「双锚点 + 解析失败即保守」

`extractFailurePaths(output): string[]` 纯函数（落 `lib/failure-classify.ts`，与既有分类函数同族）：

- **文件锚点**：`golangci-lint` 的 `path/file.go:LINE:COL:` 形态、`eslint` 的相对/绝对路径形态。
- **包锚点**：`go vet` / `go test` 的 `# <module>/<pkg>` / `FAIL <module>/<pkg>` 形态 → 转成目录前缀（`backend-go/<pkg>/`），按**前缀**与 `mine`/`foreign` 集合比对。
- 提取结果为空 → 返回空数组，调用方视同 `P = ∅` → 维持现状（保守）。

正则按**有界白名单**实现，只匹配到「像路径/像包」的 token；不做「猜」。工具输出格式漂移的后果是退回现状（多报不少报），不是误判外部。

### D7：外部降级只改四处行为，不新增事件类型

外部失败命中后：① 不进 `stickyFailures`；② 不进 `failures[]` 的 `[回归]/[中间态]` 分级，改走一行 `[外部]` 文案——**该行同样纳入 D4 指纹状态机：首次一行，同指纹后续回合静默，指纹变化重新一行**（外部失败无催修义务，重复提示无收益；账本已有 gate.check + policy.decision 双记录，apply 前评审补丁 P1-2）；③ 追加一条 `policy.decision(action="warn", reasonCode="foreign-breakage", target=<cmd>)`（复用 `logPolicyDecision`，reasonCode 符合 kebab-case 白名单，target 只放命令名属有界短摘要）；④ 仍照常 `gate.check` 记账（失败事实真实存在、diag 可考古）。账本侧看 `gate.check`（失败事实）+ `policy.decision`（归因）两条并存，harness-retro 可直接 join 统计。

### D8：两处改动独立可回滚，同一 change 交付

suggest 侧（bash）与 quality-gate 侧（TS）无代码耦合，只共享「change 名解析」与「归属集合」这两个**数据语义**。放在一个 change 的理由：同一根因、同一份 smoke 夹具思想（归属地图）、且重复抑制不先做的话并发降级的收益会被重复注入淹没（一条外部失败每回合重刷，等于没降噪）。回滚粒度仍可按文件 revert。

## Risks / Trade-offs

- **[假阴性：真失败被判成外部]** → 三重收窄：判据要求 `P ⊆ foreign`（全落外部侧）、`P ∩ mine = ∅`、`P` 非空；任何混合/解析失败/信号缺失一律回现状。且 `[外部]` **仍输出一行**（不静默），agent 看到路径与命令可自行翻案。
- **[路径正则随工具版本漂移]** → 漂移后果是「无法解析 → 退回现状」（多报不少报），且 smoke 用真实输出样本固定形态；不追求覆盖面。
- **[重复抑制让 agent 忘记未修的债]** → 粘性重跑不变（门禁不沉默）+ 3 回合未变化附加「未修」标记 + 归档门禁全绿硬要求不变。
- **[转绿收尾一行引入新噪声]** → 上限是「每命令每失败段 1 行」，换取「债已清」的闭环信号，可接受；若实测噪声超预期，可在 apply 期去掉该项（属实现细节，不动 spec 语义边界）。
- **[归属集合被污染]** → `edit.map` 采集侧已有工具自管路径黑名单（`isToolManagedPath`），消费侧 `filter_blacklist` 再兜一层；污染的最坏后果是「外部面变大 → 更容易判外部」，故 `mine` 侧（保守方向）依赖本会话触发集与绑定 change 归属，不依赖第三方集合。
- **[suggest 输出变长]** → 三桶在脏文件多时输出变长；桶内限制显示条数（如各桶最多 20 行 + 「另有 N 个」），保证可读性。

## Migration Plan

无数据迁移。落地顺序：

1. 纯函数先行（`lib/failure-classify.ts` 的 `extractFailurePaths` / 归属判定）+ 单测（smoke）。
2. quality-gate 接线（基线、指纹、报告分级、外部降级 + 记账）。
3. suggest 取数口径（change 名三源解析 + 三桶输出）。
4. 文档（`docs/reference/harness/pi-extensions.md`）+ 归档门禁四件套。

回滚：单文件 revert（`.pi/extensions/*` 或 `scripts/harness/doc-impact.sh` 各自独立，无 schema/数据依赖）。

## Open Questions

- 「共享只读单源」（把「脏文件 → 归属」抽成 `scripts/harness/lib/` 供 `doc-impact.sh` / `concurrency-status.sh` / `spec-gate` 复用）留待后续 change：本 change 只在 `doc-impact.sh` 内新增第二个查询函数，不做跨脚本抽象。是否值得抽，等第三个消费者的真实出现频次再判。
- 三桶的显示上限（默认 20 行/桶）是否合适，按 apply 期实测输出调整（不影响 spec 语义）。
