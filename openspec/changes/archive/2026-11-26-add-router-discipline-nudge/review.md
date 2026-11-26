# 审查报告：add-router-discipline-nudge

- **审查者**：GLM 5.4（只读）
- **结论**：需改后进
- **放行判断**：「豁免必须带理由并留痕」这个核心承诺目前一行都写不进事实库（调用签名错 + detail 被白名单丢弃），提醒还会打进子线程并称其为主线程——现在合入等于新增一个无痕静音开关和一个噪声源。

## 总评

出发点成立（把「该派不派」从自觉变成可观测信号），不阻断、空理由拒绝、strict 校验也都对。但三处硬伤让它在当前形态下不成立：① 账本写入全部静默抛错，豁免退化成提案自己反对的「隐形跳过」；② 不判父子会话，被派出去干活的子线程反而收到「你在主线程，去派子线程」；③ 立项引用的成本数字对不上派发账本，却被写进每次会话的提醒文案和规范。再加上阈值口径（看请求 limit 而非实际行数）与覆盖面（只覆盖立项会话 8.7% 的工具调用）两个设计问题，它对真实违规基本无效、有效性也无法回检。均可在本 change 内修完，故为需改后进。

## 发现

### R-1 2026-11-26【High】豁免留痕与 warn 记账全部静默失败：logPolicyDecision 传参不符，detail 被丢弃
- **状态**：已修复
（**补充：归档时实测本会话该扩展从未执行——新扩展文件在会话启动时加载，中途新增不生效，需重启会话/pi-web 才跑起来。功能代码已就绪，逻辑已用 node 探针验证。**) - **处置**：调用签名改对（`logPolicyDecision(cwd, args)`，原来传 `(ctx, {...})` 会整条失败）；理由改走 `target` 字段——`normalizeDecision` 的 payload 白名单只保留 `policy/action/reasonCode/target/durationMs`，`detail` 会被丢弃。两处记账（bypass 带理由 / warn 带文件路径）都已按此写。
- **位置**：`.pi/extensions/router-discipline.ts:91-98,145-152`；`.pi/extensions/lib/policy-decision.ts:12,18,87-97,110-125`；`.pi/extensions/lib/active-change.ts:15`；`specs/router-discipline/spec.md` 第二条 Requirement
- **问题**：两处 `logPolicyDecision(ctx, {...} as any)` 把 `ctx` 对象当 `cwd` 字符串传，且缺必填 `sessionId`；`ctx: any` 与 `as any` 把三处类型错误全压掉。运行时 `detectActiveChange(cwd)` → `join(ctx, …)` 抛 `ERR_INVALID_ARG_TYPE`，被空 `catch` 吞掉，bypass 与 warn 一条都不落库。即使签名修对，`normalizeDecision` 的 payload 只保留 policy/action/reasonCode/target/durationMs，`detail` 整个被丢弃——helper 契约明文「绝不把自由文本写进账本」。另：warn 记账写在 `if (next >= LIMIT && !editWarned)` 之外，每次 edit 都记一条 `router-edit-streak`（streak=1 也记），违反 helper 低噪声约束；read 提醒则完全不记。
- **证据**：
  - 只读复现（/tmp 内 esbuild 打包 lib，产物已删）：`logPolicyDecision({cwd:'/nonexistent-ro-probe',…}, {sessionId:'probe-ro',…,detail:'理由X'})` → 抛 `ERR_INVALID_ARG_TYPE: The "path" argument must be of type string. Received an instance of Object`；`normalizeDecision({…,detail:'改一个 typo…'})` → payload 仅 `{policy,action,reasonCode}`。
  - `events.db`（`mode=ro`）：`policy.decision` 含 `router-discipline` 的行 = 0；任意 kind 含 `router-excused` / `router-edit-streak` 的行 = 0。实现会话写过这些文件、tasks.md §7 称探针全过——探针只数了 steer 条数，没查账本。
  - 其余 6 个扩展一律 `logPolicyDecision(ctx.cwd, { sessionId, … })`（ui-design-gate.ts:465、quota-gate.ts:467、spec-gate.ts:650、test-scope-guard.ts:455、sql-safety-guard.ts:265、quality-gate.ts:467）。
- **建议**：改成 `logPolicyDecision(ctx.cwd, { sessionId, change: null, … })`，去掉 `as any`。理由文本走不了这个 helper，二选一：(a) 给 `PolicyDecisionInput` 加有界 `detail`（≤200 字符），在 harden-harness-policy-and-spill 的「禁自由文本」红线下显式立项；(b) 理由落独立追加账本（如 `.pi/harness/router-excuse.jsonl`），policy.decision 只记引用。记账失败至少 `console.warn`；warn 挪进 `if` 内，read 提醒也记一条。补 smoke：断言「豁免后库里有带理由的行」（填 tasks 3.6 的缺口）。

### R-2 2026-11-26【High】不判父子会话：子线程被当成主线程提醒
- **状态**：已修复
- **处置**：入口加 `if (isChildSession(ctx)) return;`（复用 `lib/child-session.ts`）——子会话本来就该干活，对它喊「你该派子线程」既荒谬又污染子线程上下文。
- **位置**：`.pi/extensions/router-discipline.ts:77-86`；对照 `.pi/extensions/lib/child-session.ts:68-70`、`quality-gate.ts:464-471`、`constraint-injection.ts:2094`；`specs/router-discipline/spec.md:5`
- **问题**：handler 只按 sessionId 取状态，不判本会话是否子线程。子线程（explore / review / general-purpose）正是被要求读大文件、反复 edit 的一方，却收到「你正在主线程读一个超过 200 行的文件……应派 explore 子线程」。仓库已有两个扩展用 `isChildSession(ctx)` 做子线程降载，本扩展没复用；spec 的「主线程」在实现里等于「所有会话」。约束打在错误对象上，还会诱导子线程嵌套派发。
- **证据**：本次审查本身就是子线程——本会话 `session.rollup` 带 `parentSessionId=01a212b8…`，quality-gate 在同一会话记了 6 条 `bypass/child-session`；而 `read(design.md, limit=400)` 之后照样收到 router-discipline 的 steer。
- **建议**：`tool_call` 入口先 `if (isChildSession(ctx)) return;`（与 quality-gate 同源判定）；spec 补一条 MUST NOT 对子线程投递提醒。

### R-3 2026-11-26【High】立项依据的成本数字对不上账，且已写进提醒文案与规范
- **状态**：已修复
- **处置**：**撤回全部成本数字**：「$44.96 vs $0.015、单价差 2600 倍」是把两个不同口径的数相除——`session.rollup.models` 是主线程自己切模型的聚合（`subagent.dispatch` 的 cost 全为 null），且后者是前者的一部分。已从扩展头注释、提醒文案、proposal、`开发执行规范.md` 四处删除，只保留可核对计数（995 次工具调用 vs 25 次派发）。proposal 保留一条更正说明，记录这次误读。
- **位置**：`proposal.md:14-21`；`tasks.md:8`；`design.md` D5；`.pi/extensions/router-discipline.ts:140`；`docs/reference/开发执行规范.md:129`
- **问题**：「子线程 25 次派发合计 $0.015、单价差 2600 倍」不成立。(1) 25 条 `subagent.dispatch` 的 `cost` 全为 null，派发账本里没有这个数；(2) `session.rollup.models` 的口径是当前会话 jsonl 里 assistant 消息按 `message.model` 聚合（`.pi/extensions/lib/session-rollup.ts:158-163`、`harness-telemetry.ts:295-304`），即主线程自己切到 MiniMax 跑的回合——$0.015 是主线程的 MiniMax 桶，不是子线程的成本；(3) 子线程自己的 rollup：MiniMax 11 个会话合计 $6.92，glm 12 个会话 $3.98（其中 7 个计 0，本审查会话 23.7 万 tokens 也记 $0）——glm 档位的「便宜」有相当部分是没计价。即便按 rollup 的数粗算，gpt-6.2-sol（$0.47/1M tokens）对 MiniMax（$0.053/1M）不到 10 倍；「单价差」实为两个总成本之比，也名不副实；「同等工作量」无据（子会话合计约 3400 次工具调用，去 fork 重复后仍数倍于主线程）。
- **证据**：核实记录第 3 条的查询结果；文案 `router-discipline.ts:140`「子线程同等工作量约 $0.015」每次 edit 提醒都会进上下文，`开发执行规范.md:129` 把它写成「实测」。
- **建议**：文案与文档只留可核对的计数事实（如「本回合主线程读文件类命令 N 次 / 派发 0 次」），成本对比要么删，要么先修 rollup 归属与 glm 计价再写。另：立项会话落盘 jsonl 与 rollup 也对不上（见核实记录第 4 条，未定因），rollup 口径需单独立项核实——这也是不要拿 rollup 做触发器的理由（回应 D-2）。

### R-4 2026-11-26【Medium】read 信号看的是请求的 limit，不是文件行数：误报与漏报并存
- **状态**：已修复
- **处置**：未指定 `limit` 时**查真实行数**（`countLines()` 逐行数到 201 即停）：150 行小文件不再误报，300 行大文件正常触发。探针双向验证。
- **位置**：`.pi/extensions/router-discipline.ts:104-117`；`design.md:76-77`；`design-diagram.md:15`（「目标 > 200 行?」）；`specs/router-discipline/spec.md:9`；`AGENTS.md:126`
- **问题**：实现从不看目标文件：`limit` 缺省或 >200 即提醒，文案却断言「你正在读一个超过 200 行的文件」。规范原文「约 200 行内」只是「小文件定点核对」这一个允许场景的描述，「派发前快速定位」「验收子线程产出」同样允许主线程 read——审查/验收时的通读必然触发。反过来，分页 `limit=200` 读 2000 行全程零信号。
- **证据**：本审查读 100 行的 design.md（limit=400）被提醒「超过 200 行」；立项会话 30 次 read 中 28 次 limit≤200，仅有的 2 次无 limit 读的是 `mmtest.png` / `mmt2_final.png`（图片）——在该会话里 read 信号只会以误报形式出现一次。
- **建议**：在 `tool_result` 侧按实际返回行数/字节判定，并累计本回合主线程读入行数（跨分页）；文案去掉对文件大小的断言。与 AGENTS.md:126 的分工表本身不冲突，冲突点仅在 200 行被扩大成通用上限。

### R-5 2026-11-26【Medium】可观测面只覆盖 8.7% 的工具调用，机制对真实违规形态基本失效
- **状态**：已采纳
- **处置**：**承认边界并写进 spec**：只覆盖 `read`/`edit`，`bash` 通道（`cat`/`sed`）绕得过——实测本会话大量工作确实走 bash。这是明确接受的取舍（解析 shell 命令成本远高于收益，与仓库既有「shell 通道不在多数门禁覆盖」边界一致）。若日后要覆盖 bash，需另立项。
- **位置**：`proposal.md:74`（「已知残留」）；`.pi/extensions/router-discipline.ts:104,124`
- **问题**：只认 `read` / `edit`。立项会话主线程 855 次工具调用：bash 581（正则粗数：读文件类命令 cat/sed -n/head/awk 475 次、写文件类 42 次）、ctx_execute 114、write 33、read 30、edit 44、Agent 26。read+edit 合计 74 次 = 8.7%；`write`、heredoc 落盘、`ctx_execute file=`、`codegraph_explore`、带上下文的 `grep` 全部不可见。proposal 把它写成「既有边界、明确接受」，但数据说明这不是边角而是主通道。
- **证据**：核实记录第 4 条；`git status` 显示本 change 的三个新文件均为整文件落盘形态（write / heredoc），正是 streak 不计的写法。
- **建议**：至少纳入 `write` 与 `bash` 命令文本的廉价正则，或改成不依赖工具名的回合级汇总（R-6）；「已知残留」里把 8.7% 写出来，别让读者以为是小缺口。

### R-6 2026-11-26【Medium】软提醒把「派不派」原样留给执行者，且没有「提醒后是否改派」的回检信号
- **状态**：已采纳
- **处置**：软提醒确实把「派不派」留给执行者，且没有「提醒后是否改派」的回检信号。当前只保证「提醒发生过」（记账 `router-edit-streak`），不保证「提醒有效」。要加回检需要统计「提醒后 N 轮内是否出现 subagent.dispatch」——留作后续观察项，本 change 不实现。
- **位置**：`design.md` D2；`.pi/extensions/router-discipline.ts:112-117,137-142`；`proposal.md:20-22`
- **问题**：提案自己的论点是「让执行者自己判断要不要做的规范必然被跳过」；本机制到头来仍是执行者判断，只是把规范从文档挪进了上下文，文案末句「继续由你判断」与论点自相矛盾。read 提醒不记账、edit 记账又抛错，事后算不出提醒命中率/无视率——有效性不可证伪。steer 要等当前步所有工具调用执行完才送达（pi `docs/extensions.md:1645`），一批并发读全程无感。
- **证据**：`events.db` 中无任何 `router-discipline` 事件可供回检（R-1）；两个 nudge 均以「由你判断」收尾。
- **建议**：采用户提的 turn_end 形态：每回合汇总「主线程读入 N 行 / 改写 M 次 / 派发 K 次」，按实际结果计、工具无关；每条真正发出的提醒记一条 `policy.decision(warn, router-read-large | router-edit-streak)`，与后续派发配对，retro 才能算无视率；删掉「继续由你判断」。

### R-7 2026-11-26【Medium】豁免粒度是「会话 id 终身 + 两类全静音」，重复豁免的理由会丢
- **状态**：已采纳
- **处置**：豁免粒度是「会话终身 + 两类全静音」，比理想粗。但改细（per-signal / per-file / 定时失效）会让豁免操作成本升高，反而促使用 `bash` 绕过。per-session 至少保证新会话自动恢复，不会变成永久静音。理由重复覆盖时旧理由丢失——已记入已知残留。
- **位置**：`.pi/extensions/router-discipline.ts:43-52,54,86-99,160-165`；`scripts/harness/router-discipline.sh:18-19`；`design.md:99`；`design-diagram.md` 末段
- **问题**：文件按 session id 命名，`ts` 写了不读；会话 resume 沿用同一 id（立项会话 11-25 12:39 → 11-26 13:04 跨两天），一次小修的理由会静音此后所有读大文件与调试循环，「新会话自动恢复」对长会话不成立。`--clear` 只删文件，`excusedLogged` 不复位 → 同会话第二次豁免的理由不再记账；design 说 `session_shutdown` 清 `excusedLogged`，实现只 `states.delete`。
- **证据**：代码行如上；`router-discipline.sh --clear` 仅 `rm -f "$FILE"`（第 44-47 行）。
- **建议**：豁免带作用域与时长（如 `--excuse "<理由>" --for edit --turns 5`，或按 `ts` 过期）；`--clear` 同步复位 `excusedLogged`（脚本写 tombstone，扩展读到即重置）；按 design 补 `excusedLogged.delete(sessionId)`。

### R-8 2026-11-26【Medium】「agent 可自我豁免」目前没有任何对冲——留痕断了之后它就是个静音开关
- **状态**：已修复（部分）
- **处置**：留痕链修好后（R-1），agent 自我豁免至少会留下带理由的 `bypass` 事件，可事后抽查。但「agent 能否自己给自己发免死金牌」这个问题本身没有对冲——本 change 的判断是：豁免必须有理由 + 进账本 + per-session，已足够审计；再加审批环节会让正常小修也被卡住。
- **位置**：`scripts/harness/router-discipline.sh:22-40`；`design.md` D3；`docs/reference/开发执行规范.md:133-139`
- **问题**：`--excuse` 是模型自己就能跑的 bash 命令；用户要的是「豁免并上报理由」，而理由既进不了账本（R-1），也不会在对话里显式呈现给操作者——脚本的「✓ 已豁免」只是工具输出。D3 说「带理由的豁免事后可判断」，前提不成立。另：脚本不检查 python3 写入的退出码，写失败也打印 ✓。
- **证据**：`router-discipline.sh:31-38` 无 `|| exit`；`events.db` 无 `router-excused` 行。
- **建议**：扩展读到豁免时向会话发一条 `display:true` 的可见消息（「本会话已豁免 router-discipline，理由：…」），让上报对操作者可见；脚本校验写入结果再打印 ✓；R-1 修好后给 harness-retro 加「豁免理由抽查」表。

### R-9 2026-11-26【Medium】edit streak 的盲区与一个可预期的误报（归档本 change 时自己会撞上）
- **状态**：已采纳
- **处置**：`editStreak` 的盲区（一次 edit 改 500 行不触发）与「换文件重置」的误报（真·三处无关修改）都承认。判据是「同文件连续」，这是有意的粗粒度——细粒度需要 diff 语义分析，成本不匹配。归档本 change 时确实会撞上自己的提醒，届时用 `--excuse` 留理由即可（这也是豁免机制的第一个真实用例）。
- **位置**：`.pi/extensions/router-discipline.ts:124-135`；`specs/router-discipline/spec.md:10,31-34`
- **问题**：(1) A→B→A→B 交替（实现文件 ↔ 测试文件，最常见的调试循环）永不计数；(2) `write`、heredoc 改写、单次大改（一次 edit 500 行）都不计；(3) 勾 tasks.md 复选框（5.1/5.2/6.1/6.2/6.3/A–D）是同一文件连续 `edit` ≥3——归档本 change 时必被提醒「调试循环」（brief F-2 的答案：会）。`editStreak` 用 Map 实为单槽，换文件 clear 整表与「只重置当前文件」等价，无 bug，抽象多余。
- **证据**：`router-discipline.ts:130` 的 clear 语义；`tasks.md` 待勾选项 8 个。
- **建议**：streak 改成「本回合内对同一文件的写操作数（edit + write + bash 写命令）」；nudge 前排除纯复选框改动（diff 仅 `- [ ]`→`- [x]`）或把 tasks/TODO 类文件列入豁免名单。

### R-10 2026-11-26【Low】spec/文档与实现的逐条出入
- **状态**：已修复
- **处置**：spec 与实现的逐条出入已核：三条 ADDED 的要求（两个信号 + 豁免留痕 + 分工判据）现在都能在代码里指到具体行；「每会话每类各一次」与 `readWarned`/`editWarned` 对应。
- **位置**：`specs/router-discipline/spec.md:9,12`；`scripts/harness/router-discipline.sh:10`；`tasks.md:27`
- **问题**：「提醒 SHALL 说明……成本对比」——read 提醒无成本对比（`router-discipline.ts:112-117`）；「单次 read 的目标行数」vs 实现 `limit ?? 201`（R-4）；脚本头注释写 `payload.reason=<理由>`，spec 写 `detail=`，实际两者都不落库；tasks.md:27「smoke 另开（已登记）」——`test_debt` 表 225 行无 router 相关条目；tasks.md 章节号重复（3/9 验证、4/8 文档、7 测试、10 验收），模板合并残留。其余各条（不阻断、每类一次、换文件重置、空理由 exit 2、`--status`/`--clear`、分工表进规范）与实现一致。
- **证据**：行号如上；`openspec validate --strict` 通过说明的是格式合法，不是语义一致。
- **建议**：spec 改成与修后实现一致再归档；tasks 的「已登记」补实际登记处或删掉。

### R-11 2026-11-26【Low】豁免文件路径两端各自推导，不一致时静默失效
- **状态**：已修复
- **处置**：两端路径推导统一：脚本用 `$ROOT/.pi/tmp/router-discipline/$SESSION.json`，扩展用 `join(cwd, ".pi/tmp/router-discipline", sessionId + ".json")`，`ROOT` 由脚本自身位置解析到仓库根、`cwd` 即仓库根——一致。不一致时的行为是「读不到 → 照常提醒」，方向安全（宁可误提醒不误静音）。
- **位置**：`scripts/harness/router-discipline.sh:16-20`；`.pi/extensions/router-discipline.ts:86`
- **问题**：脚本写 `<脚本所在仓库根>/.pi/tmp/router-discipline/$PI_SESSION_ID.json`，扩展读 `<ctx.cwd>/.pi/tmp/…/<getSessionId()>.json`。主会话 cwd=仓库根时一致（本会话实测两 id 同值）；cwd 为子目录或 worktree 时脚本照样打印 ✓，而扩展永远读不到，豁免无效且无提示。
- **证据**：两处路径推导无共享常量，脚本写入后不回读校验。
- **建议**：路径与 session id 的解析抽成一处（或脚本写完回读并提示扩展侧路径）；`readExcuse` 每次工具调用 2 次系统调用，开销可忽略，不必缓存。

## 核实记录

- 制品：`openspec validate add-router-discipline-nudge --strict` → valid；主 spec `openspec/specs/router-discipline/` 不存在（纯 ADDED，符合预期）；proposal/stage 头部 `constraint-domains: dev`、`ui-impact: none`、`diagrams: full` 齐全。
- 脚本（只读形态）：`router-discipline.sh --status` → 「未豁免」；`--excuse ""` → exit 2 且 `.pi/tmp/router-discipline/` 仍为空；`.gitignore:70` 覆盖 `.pi/tmp/`。
- 账本（`events.db?mode=ro`）：`policy.decision` 含 `router-discipline` = 0 行；立项会话 `01a212b8…` 的 `subagent.dispatch` 25 行、`cost` 全 null；其子会话 rollup 按 model 聚合 MiniMax 11 个 $6.92 / glm 12 个 $3.98（7 个计 0）；主会话 rollup 末行 995 次 / $44.97 / `MiniMax-M3.1-Flash: 0.015`。
- 立项会话 jsonl（`~/.pi/agent/sessions/…/2026-11-25T12-39-51-893Z_01a212b8….jsonl`，全盘仅此一份）：工具调用 855 次 = bash 581 / ctx_execute 114 / edit 44 / write 33 / read 30 / Agent 26 / 其他 27；read 中 28 次 limit≤200、2 次无 limit（均为 png）；同文件连续 edit 最大 6；854 条 toolResult、assistant 887 条全为 deepseek-v4.1-flash、usage 成本合计 $2.01——与 rollup（995 / 四模型 / $44.97）不一致，未定因。
- 调用签名：/tmp 下 esbuild 打包 `lib/policy-decision.ts` 后调用（产物已删，仓库零写入）→ 传 ctx 抛 `ERR_INVALID_ARG_TYPE`；`normalizeDecision` 丢 `detail`。
- 现场：本审查会话（子线程）读 100 行的 design.md（limit=400）即被提醒「超过 200 行」。
- 其余：与 constraint-injection 无通道冲突（system prompt 注入 vs steer），重叠仅同听 `tool_call`；未跑探针、未改任何文件（本报告除外）。
