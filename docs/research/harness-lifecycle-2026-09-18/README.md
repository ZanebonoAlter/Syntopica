# Syntopica harness 约束体系：一次开发生命周期的完整走读

> 写给想理解「这个仓库里 pi harness 扩展到底怎么管着开发过程」的新读者。大白话，源码为准（`.pi/extensions/` 已入库），参考文档为辅（[`docs/reference/harness/pi-extensions.md`](../../reference/harness/pi-extensions.md)）。
>
> 产出日期：2026-09-18（同日增补图 4）· 四张配图在本目录 `diagrams/` 下，用浏览器打开即可（自包含 HTML，只依赖 Google Fonts）。

---

## 一句话总览

这套体系干两件事：**开发前让你"知道"规矩**（constraint-injection 自动把业务红线注入上下文），**干完活让你"做到"规矩**（turn_end 增量门禁 + 归档硬门禁 + 派发额度门禁），中间发生的每件事都**记进事实账本**（`.pi/harness/events.db`），账本反过来养出一个**复盘闭环**（harness-retro 报告 → 改规则 → 基线回检）。

## 四张图

| 图 | 讲什么 | 文件 |
| --- | --- | --- |
| 图 1 全景 | 生命周期 10 个阶段 × 4 个角色（用户/主线程/子线程/扩展层），谁在哪一步介入 | [`diagrams/lifecycle-panorama.html`](diagrams/lifecycle-panorama.html) |
| 图 2 单回合 | 一个回合内：回合开始注入约束 → 工具改代码 → turn_end 增量门禁 → steer 喂回/记账 | [`diagrams/turn-gate-sequence.html`](diagrams/turn-gate-sequence.html) |
| 图 3 闭环 | 记账 → 报告 → 改进项 → 改规则 → 基线回检，回到记账的飞轮 | [`diagrams/retro-loop.html`](diagrams/retro-loop.html) |
| 图 4 角色顺序图 | 10 个扩展全员登台的四幕连续剧（每幕一张顺序图）：开工注入 → 派发双门 → 写码护卫 → 归档闭环 | [`diagrams/lifecycle-story-sequence.html`](diagrams/lifecycle-story-sequence.html) |

每图两三句导读放在下文对应小节开头。

> **图 4 导读**：图 1 把扩展层画成一整条泳道，图 4 把那条泳道拆开演——一个自包含 HTML 里四张独立顺序图纵向排成连续剧，谁挂哪个钩子、说了什么话（消息标签即台词）、软提醒还是硬拦截（coral = 硬 block / 关键 steer），一目了然。第 1 幕开工（constraint-injection 注入规矩、harness-telemetry 记账）→ 第 2 幕派发（quota-gate / ui-design-gate 两道硬门）→ 第 3 幕写码（test-scope-guard / tool-output-spill / entry-gate / quality-gate 四护卫轮值）→ 第 4 幕归档收尾（spec-gate 五项检查、dev-process-guard 清孤儿、harness-retro 闭环回第 1 幕）。出场顺序与速查表对应；账本记什么、retro 读什么详见 §5.1 / §5.2。

## 出场角色速查（10 个扩展 + 2 个脚本）

源码在 `.pi/extensions/`，下表逐个核对过源码头部注释与注册的挂点：

| 角色 | 挂点 | 干什么 | 软/硬 | 你能看到什么 |
| --- | --- | --- | --- | --- |
| constraint-injection | `before_agent_start`（稳定层）+ `input` / `tool_execution_start` / `session_compact` / `session_start`（状态管理、动态层） | 把业务约束注入上下文（详见 §3） | 软（fail-open） | 界面 widget 显示 `[约束] 档位 \| change \| 命中文档`；对话里出现注入文本/steer |
| quality-gate | `turn_end` | 增量跑 lint/vet/build/影响包测试 + eslint（详见 §4） | 软 steer 催修 | 失败消息带 **[回归]** / **[中间态]** 前缀 |
| entry-gate | `turn_end` | implementation 档 + complex change 缺 test-cases 文档 → 提醒 | 软 steer（每会话一次） | 一条补白盒用例文档的提醒 |
| test-scope-guard | `tool_call` | 检测全量 `go test ./...` → 提醒只跑影响包（归档语境放行） | 软（默认 soft；hard 才 block） | 一条「别跑全量」提醒 |
| quota-gate | `tool_call`（仅 `Agent` 工具） | 子线程派发前查供应商额度（GLM/Kimi 查 5h/周窗口、DeepSeek 查余额；<10% 或 <¥1 拦） | **硬 block** | 中文 block reason：剩余额度 + 建议换 provider 全称重试 |
| spec-gate | `tool_call`（bash 命中 `openspec archive`） | 归档门禁：doc-impact verify / check-standards / tasks.md 尾三节 / scenario-trace 映射 / UI 验收证据，外加并发脏树 warn | **硬 block** | block reason 列失败项 + 每项尾部输出 + 修复指引 |
| ui-design-gate | `tool_call`（Agent 派发 + edit/write） | implementation 档绑定 syntopica-ui change 时：major 原型未批准（`ui-approval: pending`）→ 拦实现派发与项目代码写入；当前 change 目录内的 ui-design.md / 原型修复不受限 | **硬 block**（legacy schema 仅 warn 一次） | block reason：ui-impact 缺失/不符、ui-design.md 缺、原型缺、审批 pending |
| dev-process-guard | `session_start` / `turn_end` / `session_shutdown` | 会话结束自动清「窗口内泄漏的 dev 进程组」（go run / pnpm dev / nuxt dev / agent-browser 无头 chromium，五条件合取；pidfile 白名单豁免）；历史遗留孤儿软提醒 | 自动清 + 软提醒 | 关会话时进程被清；遗留孤儿每会话提醒一次 |
| harness-telemetry | `session_start` / `turn_end` / `tool_call` / `tool_result` | 通用事实采集，不干预；`session.rollup` 效能快照（每 5 turn 或 token 增量 >20% 节流写） | —（fail-safe） | 无感 |
| tool-output-spill | `tool_result` | 工具输出 >32KB（`.pi/harness.json` 可调）→ 落盘 `.pi/harness/spill/` + 有界预览替换 | —（fail-open） | 看到预览 + spill 文件路径 |
| test-patrol.sh（脚本） | 按纪律手动跑 | 全量测试拆 12 分片滚动巡检，`patrol.check` 记账 + `test_debt` 台账 | — | 巡检报告 / 台账 |
| harness-retro.sh（脚本） | 按纪律手动跑 | 只读消费账本，六段失败聚类报告 + 基线对比（详见 §6） | — | 报告文本 |

所有写账本的事件统一进 `.pi/harness/events.db`（SQLite WAL，append-only）：`constraint.inject` / `mode.set` / `gate.check` / `patrol.check` / `policy.decision` / `edit.map` / `spill.write` / `session.start` / `session.rollup` / `subagent.*` / `pin.*`。保留期：多数 30 天，`session.start`/`session.rollup` 90 天，`pin.write` 永久（源码 `lib/harness-log.ts` 的 `RETENTION_DAYS`）；库超 100MB 还有删最老一半的保险丝。

---

## 1. 生命周期全景（图 1）

> **图 1 导读**：四条泳道是四个角色——用户、主线程（只调度不写码）、子线程（派出去干活的 Agent）、扩展层（harness 扩展，蓝框标出全部自动介入点）。十个列从左到右是 change 的一生；橙色聚焦格是「实现」，扩展层在派发、迭代、归档三处设卡。

按阶段走一遍（括号里是扩展行为，均已对源码核实）：

**① 需求探索** — 主线程脑暴/调研（openspec-explore）。此时尚未绑定 change，constraint-injection 处于「未激活档」：只有常驻索引（`docs/reference/constraints-index.md`）注入 system prompt，告诉你规矩查哪里。

**② 提案（proposal）** — proposal.md 头部要写两个机器可读声明：`<!-- ui-impact: none|minor|major -->`（UI 影响档位）和 `<!-- constraint-domains: 域名, ... -->`（业务域声明，域名 = flow 文档 basename）。**这两个声明直接决定后面注入什么约束**：constraint-domains 决定激活后注入哪些「业务约束与不变量」节（红线层），ui-impact 决定 ui-design.md 要写多重、归档时要交什么验收证据。major 档此时产出 `ui-design.md` + `ui-prototype/` 原型，等用户批准（`ui-approval: approved` 只有用户对话确认后才能标）。

**③ 编排计划（apply 六步）** — 主线程按《开发执行规范》§0.6 编排：读制品建上下文、跑 `doc-impact.sh suggest` 预勾选文档域、写实现计划（含 Scenario→测试文件映射表）、把 `<!-- doc-impact: ... -->` 写进 tasks.md。文档域预勾选对应归档时要更新的 `docs/reference/` 文件。

**④ 派发** — 主线程把实现任务派给子线程。这一步有两道自动拦截：
- **quota-gate**：派发前查目标 provider 额度，见底就 block，reason 会告诉你换哪个 provider（必须用 `provider/modelId` 全称）；
- **ui-design-gate**：major 原型还没批准就想派实现？block。用户批准后（图 1 用户泳道的「原型批准」格）拦截自动解除——门禁每次检查都重读磁盘，不用清缓存。

**⑤ 实现（焦点）** — 子线程写代码、写测试。从切入 implementation 档这一刻起（constraint-injection 记 `mode.set`，带 source：command / skill / edit-dir / recover / inherit / fallback，**隐性绑定不存在**），每回合开头自动注入已声明业务域的红线约束 + 编辑路径 JIT 命中的 standard 文档。

**⑥ 迭代** — 写码回合的每个 turn_end，quality-gate 自动跑增量检查（详见 §4）。红了你会在下回合开头看到 [回归]/[中间态] 消息。

**⑦ review** — 子线程聚焦 review（精读高风险文件 + grep 验证不变量 + High/Med/Low 分级）；author 按 receiving-code-review 纪律处理意见。

**⑧ 验收·归档** — 主线程按 §11.4 自检（重跑验证节命令、域外红登记台账、doc-impact verify、check-standards），然后执行 `openspec archive` —— 被 **spec-gate** 拦下做最后五项检查（详见 §5）。通过后归档进 `openspec/changes/archive/<date>-<change>/`。

**⑨ 溯源·retro** — 归档不是终点：受影响的 `docs/reference/flow/<功能>.md` 必须补「变更溯源」链接回 archive 目录（check-standards E 段催收，3 天宽限期）；改了 harness 规则的 change 还可以跑一次 retro 回检（详见 §6）。

**agent 与用户各感知到什么**：agent 在上下文里看到注入的约束文本、widget 状态行、steer 消息和 block reason；用户在对话里看到的是——major 原型等待批准、额度 block 的换供应商提示、归档被拦的修复清单，以及会话结束时偶发的「孤儿进程已清理」提醒。正常情况下用户什么都不用做，体系静默运转。

---

## 2. 约束是怎么"进脑子"的（constraint-injection，管"知道"）

混合通道，两路并行：

- **稳定层 → system prompt**：常驻索引 + 档位基础说明 + 已声明域的红线层。关键设计是**快照 key = 档位 | 绑定 change**——档位和绑定不变，这段字节就恒定，不会每回合刷新 system prompt 打断模型的前缀缓存。切档/换绑定时快照重建，按指纹 regime 判定是否全量重投（fix-injection-transition-fingerprint-wipe）。
- **动态层 → steer 消息**：关键词命中全节、编辑路径 JIT 命中（flow/standard 文档头部 `doc-impact-applies` 标签对上你正在改的文件路径）、change 级文件（explore-findings、词汇表）、稳定层差异补偿。**指纹 diff 驱动，稳态零投递**——已经送过的内容不重复送；`session_compact` 压缩后会重发一次快照补偿。

命中范围有域限定：只有「声明域 ∪ 栈相关 ∪ 索引内」的关键词命中才生效，防止跨域误拉。声明域注入的是**红线层**（约束节里顶层列表项的首个加粗红线句逐行），提取不到才回退全节。

配置在 `.pi/constraint-injection.json`；应急回退 `channel: "legacy"` 可变回每 turn 全量进 system prompt。

**你看到什么**：界面 widget 一行 `[约束] implementation | <change名> | 通道:split | 命中文档列表`；动态层命中时对话里出现对应约束文本。

## 3. 一个回合的流水账（图 2）

> **图 2 导读**：四个生命线——Agent 回合、constraint-injection、quality-gate（焦点，橙框）、events.db。上半场是注入（before_agent_start：稳定层快照 diff → system prompt），下半场是门禁（turn_end：增量路由 → 跑命令 → alt 分支：有失败 steer 喂回 / 纯文档静默放行），无论分支如何都落一条 gate.check 记账。

quality-gate 的增量逻辑（源码 `.pi/extensions/quality-gate.ts`）：

1. **触发条件**：本回合用过 edit/write/bash/apply_patch 才跑；纯对话回合零成本放行。例外：上回合有失败未转绿的命令，纯对话也重跑（**粘性催修**，防"口头修复"）。
2. **增量路由**：session_start 时把当时的 git 脏文件拍快照进基线；turn_end 对比快照，只有**本回合新增/变化**的路径命中后端（`backend-go/**.go`）才跑后端四件套、命中前端（`front/` 非 .md）才跑 eslint。会话开始前就存在的脏文件不触发——不替别人背锅。
3. **跑什么**：后端 = `golangci-lint` 先行（编译失败即同根因短路，vet/build/test 必红同因就不浪费了）→ `go vet` + `go build` 并行 → `change-scope.sh` 映射的影响包 `go test -short`（DB 集成测试自动 skip，总预算 5 分钟）。前端 = `pnpm exec eslint --cache`（秒级）。**不跑** typecheck/build/完整集成测试——这是门禁分层设计，留给手动 + 归档门禁兜底。
4. **执行链路按宿主分流**：cmd.exe 可达（Windows/WSL）走 interop 并每回合健康探测，探测挂了整轮短路 fail-open；不可达（本仓库现在的 Linux 树莓派宿主）走 native 模式，本机工具链直接跑，不探测。
5. **steer 分级**：失败喂回分两档——**[回归]**（上回合还绿、这回合红了：必须修，不得忽略）和 **[中间态]**（从未绿过的新代码：正在推进可继续，回合末复检）。归档前全绿的硬要求不受分级影响。
6. **记账口径**：失败全量记，成功采样记（会话首条与转绿锚点必记，之后每 5 连续成功记 1 条）——账本不膨胀，retro 报告按采样口径还原分母。

**用户感知**：绝大多数回合什么都没有（门禁静默全绿）；红了的回合，下回合 agent 会收到带 [回归]/[中间态] 的消息并自己去修。

## 4. 什么时候会被真的拦下来（硬门禁 + 逃生口）

| 门禁 | 拦什么 | 逃生口（显式留痕，不静默） |
| --- | --- | --- |
| quota-gate | 子线程派发时 provider 额度见底 | 无（换 provider 全称重试或等重置；查询失败 fail-open 放行） |
| ui-design-gate | major 原型未批准时的实现派发/项目代码写入 | `UI_DESIGN_GATE_BYPASS=1` |
| spec-gate | `openspec archive` 五项检查任一失败 | `--force` 或 `SPEC_GATE_BYPASS=1` |

三个门禁共同的设计哲学：**fail-open 不装死**——门禁自身异常时放行，但必须 console.warn + steer 告警 + 记 `policy.decision(fail-open)`，绝不静默假装通过。所有 block/warn/bypass/fail-open 裁决都记账（保留 30 天），普通成功放行零记录。

spec-gate 的五项检查对应 §11.1 归档前置条件中可机器判定的部分：① `doc-impact.sh verify`（声明域 vs 实际改动对账，有 edit.map 归属记录时只扫归属集合）② `check-standards.sh --change`（代码规范/死链/F 段对账）③ tasks.md 尾三节（测试/文档/验证）+ doc-impact 声明标记 ④ scenario-trace（delta Scenario ↔ 测试文件映射对账）④' UI 验收证据（major：approved + 原型 + opencli 主链路 + 双视口证据 + 差异说明）。另有检查⑤'（warn 级不 block）：树上存在归属其他 active change 的未 commit 文件时提醒拆 commit。

dev-process-guard 是另一种"硬"：会话结束时**自动清理**本会话窗口内泄漏的 dev 进程组（TERM → 2s → KILL）。红线是 dev 服务起停必须走 `scripts/start-dev.sh`（写 pidfile 白名单），手搓 `nohup setsid` 的会被当孤儿清掉。

## 5. 事实闭环：账本 → 报告 → 改进 → 回检（图 3）

> **图 3 导读**：中央深色枢纽是事实账本（events.db，多数事件 30 天 TTL）；外圈五个站顺时针转：扩展记账 → harness-retro 报告（只读消费）→ 改进项（必须绑定可回检指标 + 观察窗口）→ 走 openspec change 改规则 → 落基线快照做前后对比，然后进入下一轮记账。橙色「基线回检」是这个飞轮区别于普通循环的关键一步。

- **记账**：各扩展按统一口径追加事件（§速查表）。记账失败严格旁路，绝不影响裁决本身。
- **报告**：`bash scripts/harness-retro.sh --days 7` 产出六段聚类：① 门禁失败聚类 ② 回归翻转 ③ harness 自身故障（fail-open 单列）④ 软提醒失效（同 policy+reasonCode warn ≥5 次）⑤ 重复失败热点 ⑥ 注入面健康，外加⑦效能看板。分母按采样口径还原。
- **改进项**：判据在 skill `harness-retro`——每条必须绑定**可回检指标 + 观察窗口**；单 session 偶发不升格为规则；防 overfit。报告不做判定、不自动开 change，改进项仍走正常编排。
- **改规则**：改扩展/阈值/注入规则照旧开 openspec change（本仓库的 harness 扩展源码也在 git 里，改动可溯源）。
- **基线回检**：改规则前 `--save-baseline`，改完 `--baseline` 对比同类事件计数。**没降 = 改动无效或问题不在那**——这是准 A/B，防止"改了感觉良好"。注意窗口 >30 天会因 TTL 清扫失真（报告头部会提示）。

### 5.1 事实库记了哪些东西（events.db 事件清单）

速查表提过一句事件类型，这里展开成对照（写入者/语义/保留期均与源码 `lib/harness-log.ts` 的 `RETENTION_DAYS` 及各扩展源码核对）：

| 事件 | 谁写 | 记什么 | 保留期 |
| --- | --- | --- | --- |
| `constraint.inject` | constraint-injection | 每次注入的命中与通道（稳定层快照 / 动态层关键词·JIT·差异补偿） | 30 天 |
| `mode.set` | constraint-injection | 档位与绑定切换，含 source（command/skill/edit-dir/recover/inherit/fallback） | 30 天 |
| `gate.check` | quality-gate 及各门禁 | 门禁执行结果——**失败全记、成功采样**（会话首条 + 转绿锚点 + 每 5 连续成功 1 条） | 30 天 |
| `policy.decision` | 各门禁 | block / warn / bypass / fail-open 裁决（普通成功放行零记录） | 30 天 |
| `patrol.check` | test-patrol.sh | 分片巡检结果与 test_debt 台账（open/fixed/waived） | 30 天 |
| `edit.map` | 编辑归属链路 | 编辑路径 → change 归属；spec-gate 归档对账时只扫归属集合 | 30 天 |
| `spill.write` | tool-output-spill | 超 32KB 输出的落盘记录（spill 文件路径 + 预览） | 30 天 |
| `session.start` | harness-telemetry | 会话启动与档位继承/恢复（内存 → 事实库回退链路） | 90 天 |
| `session.rollup` | harness-telemetry | 效能快照（每 5 turn 或 token 增量 >20% 节流写） | 90 天 |
| `subagent.*` | 派发链路 | 子线程 Agent 派发历史 | 30 天 |
| `pin.write` | pin 落盘 | 探索发现持久化审计 | **永久** |

保险丝：库超 100MB 删最老一半；记账失败严格旁路，绝不影响裁决本身。

### 5.2 harness-retro 分析什么数据（报告七段 ↔ 数据源）

| 报告段 | 吃什么数据 | 回答什么问题 |
| --- | --- | --- |
| ① 门禁失败聚类 | `gate.check` 失败按 policy + reasonCode 聚类 | 哪条门禁老红、卡在哪一步 |
| ② 回归翻转 | 相邻 `gate.check` 绿→红序列 | 什么改动把绿的搞红了 |
| ③ harness 自身故障 | `policy.decision(fail-open)` 单列 | 插件自己挂了几次、fail-open 放行多少 |
| ④ 软提醒失效 | 同 policy + reasonCode 的 warn ≥5 次 | 哪条软提醒提醒了也没人听 |
| ⑤ 重复失败热点 | 失败事件按文件/命令聚合 | 同一个坑摔了几次 |
| ⑥ 注入面健康 | `constraint.inject` 命中统计 | 哪些约束文档从没被注入过 |
| ⑦ 效能看板 | `session.rollup` 快照 | 注入命中率 / token 消耗 / 返工，插件值不值 |

口径与边界：分母按 `gate.check` 采样口径还原；窗口 >30 天会因 TTL 清扫失真（报告头部提示）；报告只读不判定，改进项必须绑可回检指标 + 观察窗口，走正常 openspec change。

配套消费：事件考古查 skill `harness-facts`（从账本查"为什么注入了/为什么被拦"）；测试欠账查 `bash scripts/test-patrol.sh --report`（分片进度 + test_debt 台账 open/fixed/waived）。

---

## 6. 源码核对备注（与 reference 文档的差异）

写作时逐个核对了 `.pi/extensions/` 源码与 `docs/reference/harness/pi-extensions.md`，行为一致，发现两处文档未展开的细节（以源码为准，本文已按源码表述）：

1. **constraint-injection 还有 `session_start` 挂点**——reference 全景表只列了 `before_agent_start` + 动态层三事件。源码里 `session_start` 负责会话状态管理：startup 时子线程/fork 从父会话继承档位（内存 → 事实库回退）、resume/reload 恢复本会话档位并显式记 `mode.set(source=recover)`、new/fork 清零。注入主链路行为与文档一致。
2. **事件保留期有 90 天档**——reference 只提了 gate.check/patrol.check 30 天；源码 `RETENTION_DAYS` 里 `session.start` / `session.rollup` 是 90 天，`pin.write` 永久。

另：quality-gate 的平台分流、quota-gate 阈值（窗口 <10%、余额 <¥1、缓存 TTL 3 分钟）、spec-gate 逃生口、spill 阈值 32KB 均与文档一致，无出入。

## 7. 画图选型、假设与删减取舍

按任务书既定决策执行（用户不可达，无法中途确认）：diagram-design 技能、**默认 editorial 皮肤**（显式默认选择，跳过 first-time setup 的 onboarding 询问，未定制 style guide）、四张自包含静态 HTML（内联 CSS + 内联 SVG，外部依赖仅 Google Fonts，无动画无脚本；图 4 为单文件四张内联 SVG）。四张图均通过技能自带 `self_check.py`（可访问 SVG 契约 + 单文件安全）。另：图 4 用 skill 仓库的 `verify-geometry.py` 整文件校验时报 7 条「标签遮蔽」，逐条核实均为跨 SVG 假阳性——该脚本用正则全文件抓 rect、不区分 `<svg>` 边界，四幕 viewBox 同从 0,0 起，幕 N 的 actor tag 与幕 N+1 的 actor 盒坐标天然重叠；把四段 SVG 拆开逐幕校验全部 0 finding（同幕内 tag 画在盒之后，不存在真实遮蔽）。此为该脚本对单文件多 SVG 形态的已知限制，留痕在此。

**选型理由**：

1. **图 1 用 Process**（type-process，泳道 × 阶段格）：生命周期全景的本质是"多角色顺序流程 + 每步谁介入"，Process 的泳道（角色）× 列（阶段）× 空格留白恰好表达"harness 只在某些格子介入"；Swimlane 更适合纯人员分工，Data flow 强调载荷不合适。
2. **图 2 用 Sequence**（type-sequence）：单回合内是严格的时间序消息交换（挂点回调 → 工具调用 → 回调 → steer/记账），alt 分支（有失败/纯文档）是 UML combined fragment 的标准场景。
3. **图 3 用 Loop**（type-loop，飞轮）：retro 闭环的“最后一站喂回第一站 + 中央枢纽累积状态（账本）”正是 Loop 的定义性特征——虚线辐条写回枢纽是它的识别信号。
4. **图 4 用 Sequence ×4 幕**（type-sequence，单文件四张）：用户诉求是“把 10 个角色在各阶段做的事直观体现”，而 sequence 单图硬预算是 ≤5 生命线 / ≤12 消息 / ≤1 组合片段，10 扩展 + 用户 + 主/子线程不可能进一张——按 skill 的 split 指引拆成四幕连续剧（开工/派发/写码/归档收尾），每幕独立守预算，全部 10 扩展有生命线或明确动作；呈现形式经用户选定“单文件四幕纵向”（与每图一文件的旧三图不同，README 表格只加一行）。中文标签沿用图 2 的适配（消息标签 12px Noto Sans SC、Latin 技术子标签 Geist Mono 8px）。

**尺寸与预算假设**：四图均取 doc-inline 档（标准字号坡道）。图 1:4 泳道(≤6)、10 阶段(≤12)、11 节点(Process 类型自身预算不设节点上限,官方示例同为 11 节点;超出 SKILL.md 通用 9 节点建议,判定为类型预算优先)、11 箭头(≤12)、焦点 = 实现阶段 + 实现节点(≤2 coral)+ 3 个自定义蓝节点(≤3 上限,语义=扩展关注点)。图 2:4 生命线(≤5)、11 消息(≤12)、1 个 alt 双区 fragment(≤1)、1 处 coral(steer 喂回)+ quality-gate 焦点框。图 3:5 站(5-8)+ 1 枢纽、3 条虚线辐条(报告/改规则两站不写账,故无辐条)、焦点 = 基线回检。图 4（单文件四张 SVG,每张独立守预算）：幕1 4 生命线/10 消息/无片段,记账消息下附事件类型 mono 行（完整清单见 §5.1）；幕2 4 生命线/11 消息/1 alt 双区；幕3 5 生命线/11 消息/1 alt 双区；幕4 5 生命线/10 消息/1 alt 双区；每幕 coral 消息各 1 条（quota block / steer 喂回 / spec-gate block；幕1 无 coral 消息,焦点 actor 描边不计入消息预算）；硬门禁 actor（quota/ui/spec/quality）用 accent 描边标记，retro 脚本用虚线框（按纪律手动跑，非常驻）。布局：五盒均分画布（中心 100/320/540/760/980,盒间隙全 80,不重叠）；幕 3 把焦点 quality-gate 排在子线程旁，ALT 框只跨参与的两条生命线（符合 type-sequence「spans only the participating lifelines」规则）；所有消息标签逐一核算避开生命线。

**中文标签的字号适配**（对 Process 类型参数几何的刻意偏离）：技能规定 CJK 文字下限 12px，而 Process 类型原生节点标题仅 9px、步骤标签 6px——已将节点放大为 120×72（原 100×64）、列距 132（原 112）、页眉抬高至 48（原 36）以容纳 12px 中文标签；标题/中文标签用 Geist + Noto Sans SC，技术子标签（扩展名、事件名、命令）保持 Latin mono。字体链接在默认基础上补充 `Noto Sans SC` / `Noto Serif SC`（技能默认链接不含简体中文字体）。

**内容删减取舍**（保密度预算，细节由本文正文与 reference 文档承接）：

- 图 1 未画 events.db 泳道——记账是所有扩展的横切行为，放图 3 的枢纽更准确；entry-gate / test-scope-guard / tool-output-spill / dev-process-guard / telemetry 五个低戏剧性扩展未入图（速查表有全量），避免超预算。
- 图 2 未画 session_compact 快照补偿、JIT 工具命中即时 steer、interop 探测短路三个支线（正文 §2/§3 覆盖）；工具调用以 Agent 自环消息代替独立生命线，换出 events.db 位置——记账是本图必须交代的落点。
- 图 3 的「报告」「改规则」两站无辐条：harness-retro 只读消费、规则本体走 openspec change 而非写账本，只有真正写回账本/基线的动作才画辐条（Loop 语法的方向性要求）。
- 图 4 四幕均未给 events.db 独立生命线——记账是横切行为，幕 1 由 harness-telemetry 生命线代理、其余幕以标签「记账」/旁白承接（图 2/图 3 已详演落点）；test-patrol.sh 在幕 4 以主线程自环「欠账台账」动作出现而非独立生命线；幕 3 未画 constraint-injection 的动态层注入（图 2 主角，避免重演）；幕 4 幕尾「改进项走新 change」用旁白点出闭环而非第 12 条消息，保消息预算。
