### 总评

本 change 的核心机制（按写入路径触发、不依赖 mode、只记 warn 不阻断）在实现层是成立的：实测「无 mode.set 记录」「requirements 档」「approved 档」三种情形下记账行为均符合 delta spec，`decideUiToolGate` 的返回值与 `recordReleasePass` 的记账判定未被触碰（零裁决污染核实通过）。但存在 1 条 High 与 5 条 Medium：design D3 的写域表与实现/delta spec 相反且 design 未同步修订；三个新 warn 码未登记进事实库两处词汇表，会被当作纠正型 warn 并在「门禁催修时距」里按 block 计算；change 身份纯由路径正则得出（可归到不存在的 change）；「补 marker」动作本身产生假阳性；去重键含 sessionId 使低噪声目标在派发子线程主场景落空；marker 未进模板/生成链路使 D5 第 1 条判据不可达。建议先处置 R-1（制品一致性）与 R-2/R-3/R-4（观测有效性），再进观察期。

### R-1 2026-11-15【High】design D3 的写域白名单与实现/delta spec 的「除 ui-prototype/ 外一律放行」相反，design.md 未同步修订
- **状态**：已修复
- **位置**：`openspec/changes/add-ui-stage-accounting/design.md`（D3 决策表）；`.pi/extensions/lib/ui-design-gate.ts:223-243`（`checkStageScope`）；`openspec/changes/add-ui-stage-accounting/specs/ui-design-workflow/spec.md`（阶段语义表）；`openspec/changes/add-ui-stage-accounting/tasks.md:1.3`
- **问题**：design D3 明确选「允许前缀白名单」，写域为 `l0`＝`ui-design.md`/`proposal.md`/`design.md`/`tasks.md`/`test-cases.md`、`interaction`＝上述＋`ui-prototype/`、`data`＝上述＋`ui-prototype/fixture.json`，并写明理由「黑名单会漏……白名单漏了只会多 warn 一条，不会放过越域」。实现却是被 D3 论证否掉的黑名单口径：`l0`＝除 `ui-prototype/` 外任意路径放行、`interaction`/`data`/`approved`＝全部放行（`checkStageScope` 里 `if (stage !== "l0" && stage !== "skeleton") return null;`）。delta spec 的阶段表与实现一致、与 D3 相反，而 design.md 未标注任何修订。后果有三：① 同一 change 内 design 与 delta spec 自相矛盾，归档时无法判断哪份是契约；② D5 观察期测的规则不是设计承诺的规则（design 风险表与 D3 都按白名单口径描述）；③ `data` 档在实现中与 `interaction` 完全同域，永远不可能产生 `ui-stage-scope-out`，阶梯里实际只有 `l0`/`skeleton` 两档有约束力；④ tasks 1.3 声明的 `STAGE_SCOPE` 映射根本不存在。
- **依据**：design.md「D3 — 越域判定用「允许前缀」白名单，不用黑名单」表 vs `lib/ui-design-gate.ts:223-243` 实现 vs delta spec「阶段语义为单调阶梯」表；实测 `checkStageScope('data','ui-prototype/index.html') === null`、`checkStageScope('l0','specs/ui-design-workflow/spec.md') === null`——两者都与 D3 表相反。
- **建议**：二选一并把三处（design.md、delta spec、tasks 1.3/1.4）改到一致：(a) 认定实现口径正确 → 重写 design D3，写明为何放弃白名单、为何 `l0` 放行 `specs/**` 等制品、`data` 与 `interaction` 同域的代价与 `data` 档的用途，tasks 1.3 的 `STAGE_SCOPE` 措辞同步；(b) 认定 D3 正确 → 重建 `STAGE_SCOPE` 表并让 `checkStageScope` 按白名单判定（`l0` 只放行五个规划文件、`data` 只放行 `ui-prototype/fixture.json`），delta spec 表改回。归档前 design.md 与 delta spec 必须逐档一致。

### R-2 2026-11-15【Medium】三个新 warn 码未登记进事实库两处词汇表，默认按纠正型聚类，且在「门禁催修时距」里被当 block 计算
- **状态**：已修复
- **位置**：`scripts/harness/harness-retro.sh:45-56`（`INFORMATIVE_REASONS`）、`:729-731` 与 `:1326-1328`（`pb` CTE）、`:1480-1483`（warn 聚类）；`.agents/skills/harness-facts/SKILL.md:52`、`:66`（reasonCode 词汇表）
- **问题**：harness-retro.sh 自己的注释写明「白名单之外的 reasonCode 一律缺省按纠正型处理（最强监督口径：新情报型词汇须显式登记入此白名单，防静默漏进失效名单）」，而 `ui-stage-missing`/`ui-stage-invalid`/`ui-stage-scope-out` 未登记 → 累计 ≥ `--warn-threshold`（缺省 5）后即进「纠正型软提醒失效」清单，与真纠正型（`acceptance-wording` 等）混排。同文件 `pb` CTE 用 `action IN ('block','warn')` 取「红」样本，而 ui-design-gate 的解除信号是同 change 的 `action=pass` → stage warn 绝大多数落进「无解除信号→剔除」，放大披露的剔除数；少数排在真 pass 之前的 stage warn 会被算成「催修时长」，把延迟分布拉偏。`.agents/skills/harness-facts/SKILL.md:66` 的 ui-design-gate reasonCode 词汇表（该文件声明「词汇表与实现保持一致」）同样未登记三个新码。
- **依据**：`harness-retro.sh:45-56` 注释 + `:1480`（`warn_total_corrective`）+ `:1483`（`items`：`n >= 阈值` 且不在情报型白名单）；`:729-731`、`:1326-1328` 的 `pb` CTE（`action IN ('block','warn')`）；实测当前 `.pi/harness/events.db` 内 ui-design-gate 的 block+warn 池共 16 条，本 change 上线后每次写 `ui-prototype/**` 的会话都可能追加一条。
- **建议**：三个码登记进 `harness-retro.sh` 的 `INFORMATIVE_REASONS`（纯观测、无 steer 无纠正动作 → 情报型）与 harness-facts skill 的 reasonCode 词汇表；同时在 `pb` CTE 的 ui-design-gate 分支排除 stage 码（催修时距只吃真 block），或在 change 的 D5 回检口径里写明催修时距/④段对 stage 码的处置与基线快照。

### R-3 2026-11-15【Medium】change 身份纯由路径正则得出，未锚定仓库根、未校验目录存在，事件可归到不存在的 change
- **状态**：已修复
- **位置**：`.pi/extensions/lib/ui-design-gate.ts:188-215`（`changeNameFromPath` / `relInsideChange`）；`.pi/extensions/ui-design-gate.ts:349-360`
- **问题**：`changeNameFromPath` 用 `(?:^|\/)openspec\/changes\/([^/]+)\/` 全串匹配，任何位置的 `openspec/changes` 段都被当作 changes 根；name 只做字符集校验 `/^[A-Za-z0-9._-]+$/`，不校验该 change 目录是否真实存在。于是：① 仓库外绝对路径（如 `/tmp/x/openspec/changes/ghost/ui-prototype/i.html`）会被记成 `change=ghost`；② `docs/openspec/changes/foo/…` 这类同名目录同样误判；③ `x-backup` 之类非 change 目录被当作 change；④ `..` 通过字符集校验可成为 change 名。`relInsideChange` 不做归一化：rel 可为 `ui-prototype/../../front/x.ts`，落在 change 之外的路径仍被当作 change 内（`interaction`/`data`/`approved` 档下这类越界写入完全无事件）；大小写敏感（`openspec/Changes/…` 解析失败，而在大小写不敏感文件系统上却是同一目录）。
- **依据**：实测 `changeNameFromPath('/tmp/other-repo/openspec/changes/foo/ui-prototype/i.html') === 'foo'`、`changeNameFromPath('docs/openspec/changes/foo/ui-prototype/i.html') === 'foo'`、`changeNameFromPath('openspec/changes/../evil/ui-prototype/i.html') === '..'`；集成实测（tmp 仓库＋真 events.db）写仓库外 `openspec/changes/ghost/ui-prototype/i.html` → 产生一条 `change='ghost'` 的 warn，不存在的 change 进了事实库。
- **建议**：解析前用 `ctx.cwd` 锚定仓库根，只认 `<repoRoot>/openspec/changes/<name>/` 前缀，且 name 段不得为 `.`/`..`；解析后校验 `openspec/changes/<name>` 目录真实存在，否则不记账；rel 计算前先 `path.posix.normalize` 并要求归一结果仍以 change 根为前缀，穿越出 change 根的写入按「不在 change 内」处理、不归到该 change。

### R-4 2026-11-15【Medium】写 ui-design.md 补 marker 这一唯一补救动作本身会记一条 ui-stage-missing 假阳性
- **状态**：已修复
- **位置**：`.pi/extensions/ui-design-gate.ts:349-378`（`accountStageScope` 内的 `safeRead` 与 `rel === 'ui-design.md'` 分支）
- **问题**：`accountStageScope` 对 `rel === 'ui-design.md'` 的写入也记账，而 marker 是从磁盘现读（`tool_call` 在写入前触发 → 读到的是写前状态）。于是「给 ui-design.md 补上 `<!-- ui-stage: ... -->`」这一次写入必然读到旧内容（无 marker）→ 记 `ui-stage-missing`；legacy change 第一次补标记时还会因去重键含 reasonCode 而在同会话后续写入再记一次 missing。也就是说：唯一能把 missing 消掉的动作本身就产生 missing 证据，agent 无法靠「补标记」在账面上自证。
- **依据**：实测（tmp 仓库＋真 events.db）写 `ui-design.md`（磁盘上原无 marker）→ 产生 `['warn','ui-stage-missing','c2']`；对照写 `ui-prototype/index.html`（磁盘已有 marker）→ 零事件。
- **建议**：`ui-design.md` 自身的写入改为判「写后内容」而非写前内容（从 tool input 的 `content` 解析 stage，或写入后再读一次）；最简做法是 `ui-design.md` 写入只做 invalid 判定、不做 missing 判定。

### R-5 2026-11-15【Medium】去重键含 sessionId，在「派发子线程做原型」这一主场景几乎不去重，低噪声目标落空
- **状态**：已修复
- **位置**：`.pi/extensions/ui-design-gate.ts:65`（`stageWarned`）、`:372-373`（去重键）
- **问题**：design 风险表第一行把「每 session × change × reasonCode 去重一次」当作噪声控制手段，但原型生产的实际形态是主会话派发 ui-design 子线程去写 `ui-prototype/**`，每个子线程持有独立的 session id（child-session 模块即以父/子 session id 区分），于是同一 change 在每个子线程会话各记一条。design D1 举证的「34 次 ui-design 派发」正是这一形态 → 同一 change 可能产生数十条同码事件，D5 的「`ui-stage-missing` 占比」分母被重复样本放大，design 自己列的「事实库噪声变大」风险未被真正处理。
- **依据**：`.pi/extensions/lib/child-session.ts` 头部注释（派发子会话 header 带 `parentSession` → 子会话有自己的 session id）；`.pi/extensions/ui-design-gate.ts:373` `const key = \`${sessionId}|${change}|${reasonCode}\``；design.md「3. 风险」表第一行。
- **建议**：去重键改为 change 维度（`${change}|${reasonCode}`，跨会话只记一次）或按父会话归并；若坚持「每会话一次」，须在 D5 回检口径里显式声明事件按 (change, session) 计重，并写明派发场景下的噪声量级。

### R-6 2026-11-15【Medium】marker 未进 ui-design.md 模板与生成链路，missing 占比必然接近 100%，D5 第 1 条判据不可达
- **状态**：已修复
- **位置**：`openspec/schemas/syntopica-ui/templates/ui-design.md`（模板头部只有 ui-impact/ui-approval/ui-prototype 三条）；`openspec/schemas/syntopica-ui/schema.yaml`；`docs/reference/开发执行规范.md:221`
- **问题**：`ui-design.md` 的模板没有 `ui-stage` 行，规范里只写成「可写」（可选），schema.yaml 未登记该 marker。agent 复制模板生成 ui-design.md 时不会带 `ui-stage` → 每个新 change 都落 `ui-stage-missing`；而 D5 转 block 的前置条件之一正是「`ui-stage-missing` 占比 < 30%（说明 marker 真的在被写）」。按当前链路 missing 率必然接近 100%，观察期一开始就得出「不达标」，该指标失去区分度。
- **依据**：全仓 grep `ui-stage`（排除本 change 目录）：模板 0 处、schema.yaml 0 处，仅 `docs/reference/开发执行规范.md:221` 与 `docs/reference/harness/pi-extensions.md:20,176` 提及；模板头部实测只有第 1-3 行三条 marker。
- **建议**：把 `ui-stage` 写进 ui-design.md 模板与 `schema.yaml`（与三条 marker 同层同形），规范里改为「major/minor 档必写、取值与当前生产阶段一致」；若坚持可选，则 D5 判据要改成「已知写了 marker 的 change 内部占比」，否则观察期不可用。

### R-7 2026-11-15【Low】§F/§G 只覆盖主干，spec 点名的「无档位」分支与多处路径边界无用例
- **状态**：已修复
- **位置**：`.pi/extensions/tests/ui-design-gate.smoke.cjs:502-614`（§G）、`:616-670`（§F）
- **问题**：delta spec 的 Scenario 明写「会话处于 `requirements` 档（或无档位）」，但 §G 只种了 `mode=requirements`，「无 mode.set 记录」分支无用例（实测该分支可用，只是没被测住）；§F/§G 也未覆盖 archive 目录写入、仓库外绝对路径、路径穿越（`../`）、change 名大小写、跨会话去重；`data` 档在 §F 只有 `fixture.json` 一条（在白名单与黑名单两种口径下都通过，无法暴露 R-1 的分歧）。tasks 1.2/1.3 要求的四组路径解析与表驱动覆盖只在纯函数层做了部分。
- **依据**：smoke 全文：§G 六个用例均 `seedModeSet(..., 'requirements', ...)`；§F `scopeCases` 中 `data` 仅一条 `['data','ui-prototype/fixture.json',true]`，无 `data`+`ui-prototype/index.html`。
- **建议**：补三条集成用例——无 mode.set 记录仍记账、archive 目录写入零记账、仓库外 `openspec/changes/ghost` 路径不记账（配合 R-3 修复后断言）；§F 补 `data` 档两条把写域语义钉死。已核实：本次 §F/§G 全部通过，且未发现 `UI_MAJOR_FULL` 加 `ui-stage: interaction` 导致 §A–§E 少覆盖原分支——§A–§E 不读 ui-stage，legacy「无 marker」分支仍由 §G G4 覆盖（且 G4 若剥离失败会显式红，不会静默失覆盖）。

### R-8 2026-11-15【Low】tasks 1.1/1.3 声明的接口形状与实现不符（已勾选但未按声明实现）
- **状态**：已修复
- **位置**：`openspec/changes/add-ui-stage-accounting/tasks.md` 1.1、1.3；`.pi/extensions/lib/ui-design-gate.ts:167-186`（`parseUiStage`）、`:223-243`（`checkStageScope`）
- **问题**：tasks 1.1 写「返回 `{stage: UiStage} | {invalid: true} | null` 三态以区分 legacy 与非法」，实现返回 `{kind:'ok'|'missing'|'invalid'}`；tasks 1.3 写「新增 `STAGE_SCOPE` 映射与 `checkStageScope(...)`：返回 `null`（合规）或 `{reasonCode, detail}`」，实现既没有 `STAGE_SCOPE` 映射，返回值也是人类可读字符串（detail 含目标路径、不入库是对的，但形状与声明不同）。两处 tasks 已勾选完成，制品与实现不一致。
- **依据**：tasks.md 1.1/1.3 原文 vs `lib/ui-design-gate.ts:167-186`、`:223-243`。
- **建议**：要么把 tasks.md 措辞与实际形状对齐（并在 delta spec 固定返回契约），要么把实现改成声明形状（`checkStageScope` 返回 `{reasonCode, detail}`，detail 只进日志不入库）；无论哪种，tasks 勾选态须与实现一致。

### R-9 2026-11-15【Low】stageWarned 只增不清，且在记账成功前就置位
- **状态**：已修复
- **位置**：`.pi/extensions/ui-design-gate.ts:65`、`:372-374`；对照 `legacyWarned` 在 session_start 清零（`:78`）
- **问题**：`stageWarned` 从不清理（注释称「键含 session 故无需清空」——对正确性成立、对内存不成立）：长驻 pi-web 进程内随 (session × change) 组合线性增长，无上限。另 `stageWarned.add(key)` 在 `auditPolicy` 之前，若记账失败（DB 写失败、无 cwd），该 session 内不再重试，这条事实永久丢失。
- **依据**：`legacyWarned` 有 `pi.on('session_start', … legacyWarned.clear())`，`stageWarned` 无任何清理点；`:373-374` 的 add 先于 `auditPolicy` 调用。
- **建议**：加 `session_start` 清理（与 `legacyWarned` 一致），或改用按会话分桶的 Map（会话结束即释放）；把 `add(key)` 移到 `auditPolicy` 成功之后，或记账失败时回滚 key。

### R-10 2026-11-15【Low】stage 记账通道挂在门禁总开关下，观察期数据可无声断流
- **状态**：已修复
- **位置**：`.pi/extensions/ui-design-gate.ts:82`（`if (!ENABLED) return;`）、`:136`（`if (!sessionId) return;`）
- **问题**：`tool_call` 处理器第一行 `if (!ENABLED) return;`（`UI_DESIGN_GATE_ENABLE=0/false/off`）让整个扩展直接返回，stage 记账通道一并停用；`accountStageScope` 又位于 `if (!sessionId) return;` 之后（无 sessionId 时不记账，实测如此）。stage 通道是纯观测、不阻断，却与「阻断门禁」的开关绑定：D5 观察期内若有人关掉门禁或以无 session 的方式跑工具，账本会无声断流，而 D5 的判据（missing 占比、scope-out 误报率）无法察觉这种断流，零事件会被误读为「零越域」。BYPASS 只影响 block、不影响记账，这一点是对的。
- **依据**：`:82` 与 `:136` 的早退顺序；实测无 sessionId 时写 `ui-prototype/**` → 零记账。
- **建议**：把 stage 记账的早退条件与门禁开关解耦（ENABLED=0 时仍记账，或至少在观察期内显式提示）；在 D5 回检口径里补一条「观察期内 events.db 是否有 stage 事件写入」的活性断言。

### 核实记录

- 只读范围：`AGENTS.md`、本 change 的 proposal/design/ui-design/tasks/delta spec、`.pi/extensions/lib/ui-design-gate.ts`、`.pi/extensions/ui-design-gate.ts`、`.pi/extensions/tests/ui-design-gate.smoke.cjs`、`scripts/harness/harness-retro.sh`、`scripts/harness/archive-readiness.sh`、`.agents/skills/harness-facts/SKILL.md`、`docs/reference/harness/pi-extensions.md`、`.pi/harness/events.db`（只读查询）。除本文件外未改动任何仓库文件；构建产物 `.pi/extensions/tests/.uig.cjs`/`.uige.cjs`（gitignored）用完即删。
- 触发条件（审查重点 1）：实测「无 mode.set 记录」「requirements 档」「approved 档」「无 sessionId」四种情形 → 前三种记账符合 delta spec，第四种零记账（`ui-design-gate.ts:136` 早退，已在 R-10 记录）。`accountStageScope` 位于 mode 早退之前这一点属实，无更早的 return 会让它在有 session 的情况下跑不到。
- 零裁决污染（审查重点 3）：核实通过。`accountStageScope` 无返回值、不读写 `decideUiToolGate` 的输入输出；`recordReleasePass` 只识别 `action==='block'`/`'pass'`，warn 行不参与解除判定；`stageWarned` 为进程内 Set，仅增长不参与裁决（增长问题见 R-9）。
- 路径解析（审查重点 2）：实测 archive → 不记账（正确）；`x-backup` 会被当作 change（并入 R-3）；`../` 穿越、`..` 作为 change 名、仓库外绝对路径、路径大小写问题（并入 R-3）。
- 事实库消费侧（审查重点 4、7）：`archive-readiness.sh` 不读 `policy.decision` → 归档就绪不受影响；`harness-retro.sh` 有两处 `action IN ('block','warn')`（`:729-731`、`:1326-1328`）→ stage warn 会被当「红」计入催修时距，且未登记情报型白名单 → 计入纠正型 warn 聚类（R-2）。当前 `.pi/harness/events.db` 实测 ui-design-gate 的 block+warn 池共 16 条作为量级参照。
- 测试（审查重点 5）：`node .pi/extensions/tests/ui-design-gate.smoke.cjs` 单跑通过——123 项全绿、退出码 0（§F 13 组表驱动、§G 6 条集成全绿）；`openspec validate add-ui-stage-accounting --strict` → `is valid`（退出码 0）。覆盖缺口见 R-7。
- 规范一致性（审查重点 6）：delta spec 表 == 实现 ≠ design D3（R-1）；`data` 与 `interaction` 同写域在 delta spec 写清了（「全部（与 interaction 写域相同；区别只在记账意图）」），但 design D3 仍写 data 只许 `fixture.json`，两处未对齐即 R-1。
- 未核实（超出只读范围）：未跑全量 smoke、未跑 build、未跑 archive-readiness（其归档路径会写事件账本，避免改变状态）。
