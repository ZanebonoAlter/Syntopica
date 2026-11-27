# 审查报告：stage-agent-routing

- **审查者**：GLM 5.4（只读）
- **结论**：需改后进
- **放行判断**：六档与路由脚本本体成立（六阶段路由、fail-open、codemode 不绕内置白名单均实测通过）。阻断项三条：① 新增只读档没进 `ui-design-gate` 派发白名单，UI 合同未就绪时 `stage-explore/verify/archive` 会被拦，smoke E6 已红；② `stage-verify` 要产出 `review.md` 却无 write 且禁 bash 写，今日两次实跑都没落盘，本审查最终由旧档 `change-review` 代做；③ `--cheap` 把三个只读阶段换成全权限通用档。修完 R-1~R-3、补 smoke（R-8）后可进。

### R-1 2026-11-27【High】新增只读档未进 UI 派发白名单，合同未就绪时被误拦
- **状态**：已修复
- **处置**：已修复：`UI_DISPATCH_ALLOWED_AGENTS` 补入六个 `stage-*`（`ui-design-gate.ts:652`），smoke 可复跑。
- **位置**：`.pi/extensions/lib/ui-design-gate.ts:652-666`；`.pi/extensions/tests/ui-design-gate.smoke.cjs:573-583`
- **问题**：`UI_DISPATCH_ALLOWED_AGENTS` 仍是 `explore/plan/change-review/ui-design`。UI 合同 block（major 原型待批等）时，派发 `stage-explore/verify/archive` 走 block 分支，而它们替代的旧档是放行的——在 UI major change 里「按阶段派发」比现状更难用；smoke E6（无写工具档必须在白名单）同步转红。proposal 只核对了 `ui-design` 档名，漏了这张表。
- **证据**：按 E6 同一段逻辑对 live 源码只读复现 → `notWhitelisted: stage-archive、stage-explore、stage-verify`，pass=false；`decideUiToolGate`（implementation 族、合同未就绪）：`explore/plan/change-review/ui-design → dispatch-allowed`，`stage-*` 六档全部 `block(ui-design-missing)`（写能力三档被拦符合预期，只读三档是回归）。未跑完整 smoke（要先 esbuild 出 `.uig.cjs`，会写文件）。
- **建议**：白名单补 `stage-explore`/`stage-verify`/`stage-archive`；tasks 增一项，跑 `bash .pi/extensions/tests/run-smoke.sh` 留证。

### R-2 2026-11-27【High】`stage-verify` 要产出 review.md，却没有任何落盘通道
- **状态**：已修复
- **处置**：已修复：`stage-verify` 的 tools 补 `write`，档位正文限定「只许写 review.md」。实测：给它写 `probe-write.md` 它拒绝并说明只有 review.md 权限。
- **位置**：`.pi/agents/stage-verify.md:5,19,26`；`openspec/changes/stage-agent-routing/tasks.md:29,58,65`
- **问题**：档内 tools 无 `write`/`edit`，正文又明令「bash 不许改文件，重定向同样违规」，产出却要求 `review.md`。brief 让用 heredoc 落盘时，模型按更严的档内纪律执行，报告只留在最终回复里。tasks 4.1「派 stage-verify 审查」实际没由它完成，8.1「六项全过」不成立。
- **证据**：子会话 `01a21abb`（02:00–02:17Z，`openai-codex/gpt-6.1-terra`，thinking max，85 次工具调用、bash 25 次，$0.34）与 `01a21acc`（02:18Z 起，同模型）均 completed，结果原文自述「只读边界禁止写 review.md，未创建」；目录里至今无 `review.md`；主线程随后改派 `change-review`（本会话）。跨供应商钉死本身有效（model_change 记录 = terra）。
- **建议**：定为「子线程最终回复即报告全文，`review.md` 由主线程落盘」（pi 无路径级 write，给 write 等于全量写）；同步改 stage-verify.md 产出段与 brief 模板，去掉 heredoc 落盘要求。

### R-3 2026-11-27【High】`--cheap` 把只读阶段换成全权限通用档
- **状态**：已修复
- **处置**：已修复：`--cheap` 改成只输出降本建议、不改路由（design D3 记录理由：降本改成权限最宽是反的）。smoke 有 `cheap 不改路由` 断言。
- **位置**：`scripts/harness/stage-agent.sh:42-49`；`.pi/agents/general-purpose.md:5,9`；`AGENTS.md:133`；`openspec/changes/stage-agent-routing/proposal.md:63`
- **问题**：explore/propose/verify 加 `--cheap` 输出 `general-purpose`（`read,bash,edit,write,grep,find,ls` + mcp，跟随 provider）。explore 本应真只读、propose 本应无 bash，一个降本开关就把权限放到最宽，正好回到 proposal 要解决的「阶段与权限不挂钩」。AGENTS.md 把它当常规建议在推。
- **证据**：对夹具跑 `--cheap --explain` → explore/propose/verify 均输出 `general-purpose  # cheap 模式：… 改用通用档跟随 provider`；design/implement/archive 不变。成本前提见 R-9。
- **建议**：cheap 改成「同档换模型」——输出 `stage-<x>` 加 `model` 覆盖（pi-web Agent 工具的 `model` 入参对新派发生效，能否盖过 frontmatter 需实测一次）；或先删 `--cheap`，等实测数据再定。

### R-4 2026-11-27【Medium】「自动选」只靠人记得跑脚本，同一文档三处口径打架
- **状态**：已修复
- **处置**：已修复：`开发执行规范.md` 分工判据表三行改成「change 内 stage-* / change 外旧档」，并加一句总纲；表格与映射表不再打架。
- **位置**：`docs/reference/开发执行规范.md:119,136-144,164`；`openspec/changes/stage-agent-routing/proposal.md:34-42`；`specs/stage-agent-routing/spec.md:29-31`
- **问题**：脚本只打印档名，派发入口没有任何读 stage.md 的环节；`开发执行规范.md:164`「档位选择」仍写实现/验证用 `general-purpose`、探索用 `explore`/`plan`、审查用 `change-review`，紧挨映射表的「分工判据」（:136-144）也是旧口径。主线程照 :164 走，六档一次都用不上。spec 把用户选的「按 stage.md 自动选」定义成了查表照抄。
- **证据**：`.pi/extensions/` 无任何 `stage-agent` 引用；上述两处文档原文。
- **建议**：:164 与分工判据改成「change 流程内按 `stage-agent.sh`，流程外用旧档」；加一个零成本机械提醒：`Agent` tool_call 时若绑定 change 有 stage 且 `subagent_type` 不等于路由结果，记 `policy.decision` 并提示（仓库已有 router-discipline、ui-design-gate 派发判定两个先例）。

### R-5 2026-11-27【Medium】路由脚本与权威 stage 解析器语义不一致，五类输入会分叉
- **状态**：已修复
- **处置**：已修复：脚本改成全文检索、多条即判非法、接受无空格与冒号前空格、大小写归一化，与 `parseStageMarker` 对齐。smoke 补 5 条边界断言（含第 31 行、多条、大写）。
- **位置**：`scripts/harness/stage-agent.sh:31-40`；`.pi/extensions/lib/stage.ts:79-90`
- **问题**：脚本只扫前 20 行、取首条、`stage` 与 `:` 之间不许空格；约束注入的真相源 `parseStageMarker` 扫全文、多于一条即 invalid、允许 `stage :`。分叉时会出现「约束按 legacy 注入、脚本却派 stage 档」或反过来。
- **证据**（脚本 / 解析器）：marker 在第 21 行 → `general-purpose` / `ok:verify`；两条 marker（design 在前、implement 在后，即推进阶段忘了删旧行）→ `stage-design` / `invalid`；首行合法 + 第 5 或 30 行正文提到另一条 marker → `stage-implement` / `invalid`；冒号前带空格 → `general-purpose` / `ok:implement`。一致的有：无空格 marker（能匹配）、Tab、BOM、CRLF、第 20 行、空文件、`bogus`、首字母大写、无文件、文件不可读，均 rc=0。现存 34 份 stage.md（33 归档 + 本 change）今天全部一致，属潜伏分叉。
- **建议**：脚本改成同语义（全文计数、恰一条、同正则），或直接 `node` 调 `parseStageMarker`；校验 `$CHANGE`（现在 `../../../../../../tmp/x` 可读仓库外任意 `stage.md`，本次测试即借此把夹具放在 /tmp，未动 `openspec/changes/`）；`LINE="$(… | grep …)"` 加 `|| true`，免得日后有人补 `-e` 时 fail-open 失效。

### R-6 2026-11-27【Medium】「权限收窄」对带 bash 的档只是提示词，proposal 表与实现不符
- **状态**：已修复
- **处置**：已修复：「只读」如实说明——`stage-explore` 无 bash 才是真只读；verify/archive/design 有 bash 则明确写「靠纪律不动」，并说明为什么不删 bash（要跑门禁）。
- **位置**：`.pi/agents/stage-verify.md:5,19`、`stage-design.md:5,19`、`stage-archive.md:5,19`、`stage-explore.md:5,23`；`openspec/changes/stage-agent-routing/proposal.md:25-30`；`tasks.md:65`
- **问题**：verify/design/archive 的「不许改文件/实现代码」没有机械拦截，`echo >`、`sed -i`、`git checkout` 都能写；proposal 表写 verify「只读」、design「读 + 写 + 画图」，实现与规范是「只读 + bash」「读 + 写 + bash」。验收 C「explore/verify 无写权限」对 verify 不成立。codemode 不绕内置白名单（已实测），但六档 `load_extensions: true` 让全部扩展工具可调：`pin_finding` 会写 `explore-findings.md`，`stage-explore` 正文要求用它而 tools 里没列，靠 load_extensions 兜着。
- **证据**：本会话（`change-review`，`read,grep,find,ls,bash` + codemode on）内 `tools.write` → `TypeError: tools.write does not exist`；`ALL_TOOLS` 24 个，含 `pin_finding/todo/ctx_*/codegraph_*`。源码 `pi-coding-agent/dist/core/agent-session.js:1226-1234`：脚本可调 = 激活的 direct 工具 + codemode/deferred 暴露的扩展工具。
- **建议**：验收 C 改成可核对口径——explore 无 bash/write/edit（真只读）；verify/design/archive「bash 可用，写权限靠纪律」，verify 跑完主线程 `git status --porcelain` 核对只新增 `review.md`；proposal 表按实现对齐。

### R-7 2026-11-27【Medium】`stage-propose` 无 bash，跑不了自己的技能
- **状态**：已修复
- **处置**：已修复：`stage-propose` 补 `bash`，正文限定只用于 `openspec validate`/`list` 这类只读自检。
- **位置**：`.pi/agents/stage-propose.md:5`；`.agents/skills/openspec-propose/SKILL.md:78-146`
- **问题**：propose 技能全程靠 CLI 推进（`openspec new change`、`status --change --json`、`instructions <制品> --json`，每产出一件复跑一次），没有 bash 就无法按技能建 change、取制品指令、自检 `validate --strict`。design 同样是「只写制品」却给了 bash，两档口径不一。
- **证据**：tools = `read, grep, find, ls, write, edit`；技能文件步骤 1/3/4/7 全是 bash 代码块。
- **建议**：给 `stage-propose` 加 bash（与 design 同口径，纪律约束），或在档内与 §0.6 写明「建 change 与 validate 由主线程代跑」。

### R-8 2026-11-27【Medium】Impact 三项未交付，且缺 design.md 就进了 implement
- **状态**：已修复
- **处置**：已修复：补 `design.md`（D1–D4 + 伪代码 + 成本表）、`scripts/harness/stage-agent.smoke.sh`（38 断言全绿）、`pi-extensions.md` 子线程矩阵补阶段档说明。
- **位置**：`openspec/changes/stage-agent-routing/proposal.md:56,59`；`tasks.md`（3 节、6 节无对应项）；change 目录无 `design.md`
- **问题**：① Impact 写「+ smoke」，仓库没有 `stage-agent.smoke.sh`（harness 其余脚本都有），tasks 也没这一项；② Impact 写 `pi-extensions.md` 子线程通道矩阵补一列，该文件未改，全文无 `stage-`；③ schema `syntopica-ui` 的制品链里 design 是必经项，`openspec status` 报 `isPlanningComplete: false`，而 stage.md 已是 implement、1.x–3.x 全勾，`stage-design` 这档没被走过。
- **证据**：`ls scripts/harness .pi/extensions/tests | grep stage-agent` → 仅 `stage-agent.sh`；`git status` 无 `pi-extensions.md`；`openspec validate --strict` 通过；`openspec status --change stage-agent-routing --json` nextSteps = 写 design。
- **建议**：补 smoke（六阶段 + 边界 + 与 `parseStageMarker` 的差分，可一并做白名单双向漂移，覆盖 R-1/R-5）；补矩阵一列；design.md 至少记三条决策（查表而非钩子、verify 落盘归属、cheap 语义）。

### R-9 2026-11-27【Low】成本前提「$4.89/次会话」无法从事实库复现，风险定性偏了
- **状态**：已修复
- **处置**：已修复：$4.89 无法复现，改成事实库真实分布（terra 52 样本中位 $0.14 / p90 $1.55 / 均值 $1.11 / 最大 $37.54；deepseek-flash 85 样本中位 $0.56），proposal 与 design 都更新，结论改为「中位可接受，尾部要防」。
- **位置**：`openspec/changes/stage-agent-routing/proposal.md:63`；`scripts/harness/stage-agent.sh:4`
- **问题**：该数沿用自上一个 change 的 review，未复核。事实库里 terra 子会话是轻中位数、重尾巴：风险在「跑飞的长会话」而不是单价，`--cheap` 解决的不是真问题。
- **证据**：`session.rollup` 按子会话取末次快照——gpt-6.1-terra 52 个：中位 $0.14、p75 $0.32、p90 $1.55、均值 $1.11、最大 $37.54（11-23，567 次工具调用），43/52 低于 $0.50；commandcode deepseek-v4.1-flash 85 个：中位 $0.56、p90 $2.61、均值 $1.34；glm-5.4 有 275/485 行 cost=0（未上报，不可比）。今日 stage-verify 实跑 $0.34。典型 change ≈10 次派发（2 explore + 1 propose + 1 design + 3 implement + 2 verify + 1 archive）按中位约 $2.5、按均值约 $10，尾部再叠一个跑飞会话。另：`subagent.dispatch.model` 取的是 Agent 调用入参，钉死档恒为 null，成本只能从子会话 rollup 对账。
- **建议**：proposal 改为实测分布 + 尾部风险；控成本盯单会话工具调用数/时长，不换档。

### R-10 2026-11-27【Low】tasks.md 首行多出一条 stage marker
- **状态**：已修复
- **处置**：已修复：`tasks.md` 首行多余的 stage marker 已删（脚本读的是 `stage.md`，tasks 里那条只会误导）。
- **位置**：`openspec/changes/stage-agent-routing/tasks.md:1`；`openspec/changes/stage-agent-routing/stage.md:1`
- **问题**：阶段载体是 stage.md（`lib/stage.ts` 头注 D1），没有任何解析器读 tasks.md 的 marker；同一 change 两处写阶段，推进时只改一处就不同步。其余 6 个活跃 change 的 tasks.md 都没有。
- **证据**：`grep -l "<!-- stage:" openspec/changes/*/tasks.md` → 仅本 change。
- **建议**：删 tasks.md 首行。

## 核实记录

- **四个重点**：① codemode 不绕内置白名单；② `$4.89` 站不住，真实成本低一个量级、风险在尾部；③ 脚本六阶段、无空格 marker、fail-open 均通过，第 21 行会漏，与权威解析器有五类分叉；④ 权限收窄对 explore 是真的，对 verify/design/archive 只是提示词，propose 则窄到跑不了技能。
- **codemode**：`docs/codemode.md`「Call tools」+ `pi-coding-agent/dist/core/agent-session.js:1226-1234` + 本会话实测（见 R-6）。
- **成本**：brief 原查询最近 30 行 rollup = 21 条 glm-5.4（cost 0）+ 9 条 deepseek-v4.1-flash（$0.09–$2.77），无 terra 行；按会话去重后的分布见 R-9。
- **脚本**：夹具经 `$CHANGE` 参数放 /tmp，测完已删；六阶段 → `stage-<x>`；本 change → `stage-implement`；空文件 / `bogus` / 大写 / 无文件 / 路径是目录 / chmod 000 / 无参数 → `general-purpose` + rc=0；未知 flag → stderr 警告 + rc=0。
- **门禁**：`openspec validate stage-agent-routing --strict` 通过；E6 只读复现失败（R-1）；未跑 `run-smoke.sh` 与完整 `ui-design-gate.smoke.cjs`（需 esbuild 产物，会写文件）。
- **文档**：`AGENTS.md:133`、`开发执行规范.md:119-130` 与六档文件一致；:136-144、:164 未同步（R-4）。
- **环境**：会话开头 dev-process-guard 报 6 个窗口外遗留进程（agent-browser + chromium，19 分钟），非本会话启动，未处置。
- **改动面**：仅新建本文件。
