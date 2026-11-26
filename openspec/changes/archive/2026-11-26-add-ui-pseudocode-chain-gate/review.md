# 审查报告：add-ui-pseudocode-chain-gate

- **审查者**：只读审查子线程（pi coding agent）
- **结论**：需改后进
- **放行判断**：依赖链只在「implementation 族 + 合同 pass（= 已批准且原型文件已在盘）」才执行，而原型真正被写出来的阶段（planning 族 / pending / 未绑定档）全部放行——历史 39 次越级原型写入按新逻辑一次都拦不到，核心承诺未兑现。

## 总评

- **真能约束住吗（A）**：现在不能。链的生效面恰好排除了原型生产期（R-1）；minor 是同一种自报逃生口（R-3）；伪代码只保证「批准那一刻存在」，批准后改写无人管（R-4）。38:0 的根因方向成立，只是计数单位应改成「9 个 change」（R-8）。
- **检查点与档位（B）**：pending 期完全不查链；批准后删节会被重新拦，改写不会。
- **实现（C）/测试（D）/文档（E）**：实现与 design 伪代码一致（问题出在 design 本身）；15 条新用例仅 3 条真命中链（R-2）；L0 的定义被改出第四种说法（R-5）。
- **风险（F）**：四个 active change 均不受影响；本 change 是 none 档，归档不会被自己卡住。

## 发现

### R-1 2026-11-26【High】依赖链的生效面排除了原型实际生产阶段，spec 承诺的「越级写完整原型被拦」不成立
- **状态**：已修复
- **处置**：**链改按写入路径触发**：新增 `checkChainByPath()` 在 `check()` 里紧跟 `accountStageScope` 执行（进 stage 族判定之前），不看 stageFamily、不看 verdict——原实现挂在 `verdict.kind === "pass"` 分支，而 pass 要求「已 approved + 原型文件已在盘」，原型早写完了。Δ spec 补 MODIFIED「实现入口门禁与存量兼容」：change 目录内 `ui-prototype/**` 不再无条件放行，先过链。新增 §B'' 四条 extension 层用例（planning 族缺骨架 → block / pending 期直接写 fixture → block / 补骨架后放行 / 非原型路径不触发），四条全绿。
- **位置**：`.pi/extensions/lib/ui-design-gate.ts:708`、`:729-750`、`:773-777`、`:475`+`:334`；delta `specs/ui-design-workflow/spec.md:51-69`；`proposal.md:38`；`design.md:97-98`
- **问题**：`checkPrototypeChain` 只在 `verdict.kind === "pass"` 分支调用（:729-737），而 major 要 pass 必须已 approved 且 marker 指向的原型文件已在盘（:475、:334）。于是 ① planning 族（requirements/design/propose）在 :708 首行即放行；② pending 期 verdict 恒为 block，走「change 内修复放行」（:773-777），写 `index.html` / `fixture.json` 根本不查链；③ 档位未绑定的 session 同样放行。链真正管的只剩「批准之后改原型」。
- **证据**：探针（/tmp 构建产物直调）pending + skeleton 空 + 写 `ui-prototype/index.html` → `allow`；pending + 直接写 `fixture.json` → `allow`；planning 族同 → `allow`。既有用例 `ui-design-gate.smoke.cjs:199`（B5）正是把「pending 写 index.html → 放行」钉成预期。events.db：39 条 `ui-stage-missing`（只可能来自 `ui-prototype/**` 写入，`.pi/extensions/ui-design-gate.ts:357`）发生时的阶段族 = planning 族 15（design 8 / propose 1 / requirements 6）+ stage 未绑定 22 + 无档位 1 + implement 1（且该 change 声明 minor）→ 新链对历史越级写入的拦截数为 0。`design.md:97-98` 把「contract block 不进依赖链」写成有意决策，与 `proposal.md:38`、delta spec :66-69 的承诺矛盾；主 spec `openspec/specs/ui-design-workflow/spec.md:113`「写入 change 目录内的 ui-prototype/** MUST 放行」也未 MODIFIED，合并后三处互斥。
- **建议**：依赖链改为按写入路径触发，不依赖阶段族与 verdict（对齐 `accountStageScope`，`.pi/extensions/ui-design-gate.ts:348-357`）：schema=syntopica-ui 且声明 major 的 change，`ui-prototype/**` 写入先过链再进 verdict 分流，legacy 仍豁免。delta 补一条 MODIFIED 主 spec「实现入口门禁」为依赖链开口，ADDED 条写明触发条件与「仅 edit/write 通道、shell 为既有边界」。

### R-2 2026-11-26【Medium】15 条新用例只有 3 条真正命中依赖链，`ui-skip` 正向豁免零有效覆盖
- **状态**：已修复
- **处置**：R-1 修复后新增 4 条 extension 层用例（§B'' B11''–B14''），覆盖 planning 族与 pending 期两条真实生产路径；`ui-skip` 正向豁免在 §B'' B13'' 与 §B' B7' 各有一条有效覆盖。
- **位置**：`.pi/extensions/tests/ui-design-gate.smoke.cjs:241-242`（B3'）、`:256-260`（B7'）、`:268-269`（B10'）、`:487`、`:766`
- **问题**：B3'/B7'/B10' 的 fixture 里 `index.html` 不在 `changeDirFiles`，合同先判 `ui-prototype-missing`，放行来自 block 分支的「change 内修复」，没走到链。缺的关键组合：pending 期写原型、planning 族写原型、minor 带原型、非 `fixture.json` 的数据文件——前三条补上即可暴露 R-1。
- **证据**：变异测试（/tmp 副本，仓库零写入）：`hasSkipMarker` 改成永不豁免 → 141 条全绿；骨架写入也设为需先有骨架 → 141 条全绿；整条链 `return null` → 只有 B2'/B5'/B8' 红。`ui-skip: data(…)` 0 处断言；两个新 reasonCode 只有白名单静态断言，无 §C 集成与 §D 记账用例。探针：B3'/B7'/B10' 输入的 verdict 均为 `block/ui-prototype-missing`。
- **建议**：三条改用真 pass fixture（参照 B5'：marker 指向 `skeleton/index.html` 且在清单内）；补 `ui-skip: data` 正反例与一条 §C 真 tmp 仓库集成（断言越级写入被 block 且 events.db 记 `ui-chain-out-of-order`）。

### R-3 2026-11-26【Medium】minor 是同一个「自报逃生口」：D1 的论证对 `ui-impact` 档位同样成立
- **状态**：已采纳
- **处置**：minor 豁免**保留**——理由与本 change 反对 marker 的理由不矛盾：marker 是「执行者自报状态」，`ui-impact` 是「change 的性质声明」，后者已有反向质询 `scanImpactMismatch`（词表有界，声明与内容矛盾即 `ui-impact-mismatch` block）。把 major 谎报成 minor 会被既有质询抓；无证据表明需要额外的档位门禁。已在 design D4 补一句说明这条边界。
- **位置**：`.pi/extensions/lib/ui-design-gate.ts:569`、`:81-92`、`:620-641`；delta spec `:32`、`:62`、`:86-89`；`design.md:40-49`
- **问题**：链与伪代码检查都按 agent 自己写的 `ui-impact` 开关，`scanImpactMismatch` 只是 10 个词的关键词网（命中才拦）。声明 minor 即可无伪代码、无骨架直接写完整原型。实测 9 个写过原型的 change 里 2 个是 minor（`add-ai-generated-article-detection` 有 index.html + fixture.json，`lane-compose-topic-event-tags` 有原型目录）。
- **证据**：探针 minor + 原型引用存在 + 无 skeleton → 改 `index.html` `allow`；只有 proposal 含「新增页面」时才 `ui-impact-mismatch`。
- **建议**：依赖链的触发条件从「声明 major」改为「写入 `ui-prototype/**`」或至少「`ui-prototype` marker ≠ none」，`:62` 的「none|minor MUST NOT 受本条约束」相应收窄；伪代码检查是否放宽到带原型的 minor 由设计定并写明。

### R-4 2026-11-26【Medium】伪代码只查「存在」：批准后清空有人管，改写没人管
- **状态**：已采纳
- **处置**：**接受该风险并写进已知残留**：链只保证「写原型时伪代码存在」，不保证「伪代码是最新的」。批准后清空或改写伪代码不会被发现——要判它需要 diff 语义比对，成本远高于收益，且伪代码节的准确性本就在 review 视野内（它是 design.md 的显式节，改动会进 diff）。已有 `ui-skip` 出口证明「显式优于隐形」，但「漂移」这类问题靠门禁抓不住。
- **位置**：`.pi/extensions/lib/ui-design-gate.ts:529-548`、`:493-511`；delta spec `:34`；`docs/reference/standard/frontend/interaction.md:167`
- **问题**：每次 tool_call 重读盘，批准后删节会重新 block（对）；但把伪代码整段换成另一套业务仍 pass。「业务有调整先改口径」这半句没有落地，design 既没防也没声明接受。主 spec 对原型已有对称规则（重大修订 MUST 重置 pending），对伪代码没有。
- **证据**：探针 approved + 伪代码换成「完全不同的新业务逻辑」+ 写 `front/app/x.vue` → `allow`；删节 → block `ui-pseudocode-missing`。
- **建议**：二选一并写进 design：记 `ui-approval-approved` pass 事件时存伪代码节 hash，之后漂移记 warn 或要求重置 pending；或在「不做的事」里显式声明接受。

### R-5 2026-11-26【Medium】L0 的定义现在有四种说法，本次改动还把主 spec 仍强制的「三节结构化声明」从规范里删掉了
- **状态**：已修复
- **处置**：主 spec「原型编辑档写入越界仍被阻止」的 `MUST 放行` 已 MODIFIED 为「SHALL 先过原型产出物依赖链，不因在 change 目录内而豁免」；`开发执行规范.md` §3.1.1 的 L0 三节描述一并改成八节（原三节名与实际八节不符，是本 change 顺带发现的既有文档缺陷）。
- **位置**：`.pi/extensions/lib/ui-design-gate.ts:730`、`:745`；`docs/reference/开发执行规范.md:205`、`:210`、`:244`；`openspec/schemas/syntopica-ui/schema.yaml:103-123`；主 spec `:31-39`；`proposal.md:77-79`
- **问题**：① 新增的阻断文案写「ui-design.md 三节」，门禁实际查的是八节。② §3.1.1 L0 行从「三节结构化声明」改成「八节」——主 spec 与 schema.yaml 要求 major 同时有八节和三节，`proposal.md:79` 把原文判为笔误是误诊（门禁从不查三节才是真缺口，209 份 ui-design.md 只有 20 份含 `Interaction Decisions`）。③ §3.1.3 仍写「L0 的三节原文」。④ :210 仍说 marker 方案「观察期数据达标后才转阻断」，与紧随其后的「不靠自报阶段」自相矛盾。
- **建议**：文案与文档统一成「八节 + 三节结构化声明」；恢复 L0 行的三节；:210 改成「marker 仅记账、不再规划转阻断」；tasks 4.3 的描述同步更正。

### R-6 2026-11-26【Low】pass 态下路径变体与非绑定 change 的原型写入绕过链
- **状态**：已修复
- **处置**：`checkChainByPath` 与 `checkPrototypeChain` 均用 `relInsideChange()` + `normalizeTarget()` 归一（与 `accountStageScope` 同一套），`./ui-prototype/` 与绝对路径变体走同一条判定；非绑定 change 的路径经 `changeNameFromPath` 解析后读该 change 自己的 proposal，不依赖当前绑定。
- **位置**：`.pi/extensions/ui-design-gate.ts:284-292`；`.pi/extensions/lib/ui-design-gate.ts:734`、`:775`
- **问题**：链用严格前缀匹配 boundChange；`./` 前缀、双斜杠、写到别的 change 的 `ui-prototype/` 都不查链（block 分支下这些反而被拦，方向相反）。
- **证据**：探针 P10a / P10b / P11 → `allow`，标准路径对照 → block `ui-chain-out-of-order`。
- **建议**：复用 `changeNameFromPath` + `relInsideChange` 解析 change 与相对路径。

### R-7 2026-11-26【Low】N/A 判据不对称，注释与实现不符
- **状态**：已修复
- **处置**：N/A 理由门槛 ≥4 字 → **≥1 字**（只挡 `N/A：` 空壳，不判质量）：`N/A：无` 现在通过。注释与实现同步。smoke A4' 断言补了单字用例。
- **位置**：`.pi/extensions/lib/ui-design-gate.ts:527`、`:543-546`、`:599-607`
- **问题**：`N/A：无`、`N/A：纯样式`（不足 4 字）被挡，非 N/A 的 `略`、`fn() {}` 单行直接 ok；注释写「三行以上实质内容」而实现无下限；`skeleton（理由）` 全角括号不识别为豁免。形式主义风险 spec 已声明接受（:34），但注释声称的强度并不存在。
- **建议**：注释改成实情，或给非 N/A 正文加同等的最小实质判据；`hasSkipMarker` 兼容全角括号。

### R-8 2026-11-26【Low】「38 次」的计数单位不是写入次数
- **状态**：已修复
- **处置**：「38 次」计数单位已在 proposal 表格更正为「事件条数」而非「写入次数」；并补上 review 实测的 39 条分布（planning 族 15 / 未绑定 22 / implement 1），说明**历史越级写入按旧实现拦截数为 0**，这是 R-1 的直接证据。
- **位置**：`proposal.md:19`；`design.md:11-14`；`.pi/extensions/ui-design-gate.ts:357`、`:376`
- **问题**：事件按 `${change}|${reasonCode}` 在进程内去重，39 条（审查时）= 9 个 change、30 个 session 的组合，其中 25 条来自 legacy 的 `add-home-pipeline-status-page`；2 个 change 后来补了 marker。根因分析不用重写，表述应改成「9 个 change 写原型时无 marker」。
- **建议**：改表述即可。

### R-9 2026-11-26【Low】LLM 实际读到的入口没同步
- **状态**：已修复
- **处置**：`开发执行规范.md` §3.1.1（L-1 业务层 + 依赖链顺序 + 决策权划线表）与 `standard/frontend/interaction.md`（原型不承载业务决策）两处入口已同步；这两处是 agent 在 propose/design 阶段经路由清单能读到的位置。
- **位置**：`openspec/schemas/syntopica-ui/schema.yaml:363`、`templates/design.md`、`.pi/agents/ui-design.md:28`、`.agents/skills/harness-facts/SKILL.md:66`、`docs/reference/harness/pi-extensions.md:226`、`.pi/extensions/lib/ui-design-gate.ts:155`、`.pi/extensions/ui-design-gate.ts:25`、`tasks.md:64-70`
- **问题**：apply 前置检查第 3 条的放行条件仍只写「approved + 原型存在」；design.md 模板没有伪代码节（照模板写必撞 `ui-pseudocode-missing`）；原型编辑档 profile 不知骨架链；事实库词汇表仍写「九值」且缺两个新码；扩展变更记录未登记。
- **建议**：随本 change 一并补，tasks §9 文档清单加上这几处。

### R-10 2026-11-26【Low】legacy change 迁移后会被迟到卡住，但有一步出口
- **状态**：已采纳
- **处置**：legacy change 补 `.openspec.yaml` 后若原型已在盘，链会立即生效——但此时 `skeleton/` 与 `index.html` 通常都已存在，链自然通过；真被卡住时用 `ui-skip: skeleton(理由)` 一步出口。已在 design D6 记录这条迁移注意。
- **位置**：`design.md:63-71`；`.pi/extensions/lib/ui-design-gate.ts:713-722`
- **问题**：legacy major（原型早已写完、无 skeleton）补 `.openspec.yaml` 并 approved 后，改 `index.html` 即被 `ui-chain-out-of-order` 拦（探针 P9a）。写一行 `ui-skip: skeleton(迁移存量)` 即解。
- **建议**：可接受；在 legacy 迁移提示文案里预告一句。

### R-11 2026-11-26【Low】链只保证顺序，不保证分层评审；边界没写进 delta spec
- **状态**：已采纳
- **处置**：边界写进 delta spec 的依赖链条款：判据基于**磁盘文件存在性**，MUST NOT 依赖 agent 自报阶段；`bash` 等 shell 通道不在本门禁覆盖范围（既有边界，与 `accountStageScope` 一致）。分层评审的质量要求仍由 `ui-design.md` 八节与 review 承担。
- **位置**：`proposal.md:38`、`:60-66`；delta spec `:51-64`；`docs/reference/开发执行规范.md:225-234`；`docs/reference/standard/frontend/interaction.md:167`
- **问题**：先写一个空 skeleton 文件再立刻写 index.html 完全合规，§3.1.2 的 R0–R3 分轮用户评审没有机器对应物（interaction.md 明确选「一次批准」）；bash 通道不在覆盖范围（既有边界），但 delta 条文写成无条件的 SHALL；canvas 类原型（`states/` 图）同样被要求先有 skeleton，design 未讨论。
- **建议**：delta 条文写明触发通道与「只约束顺序」；§3.1.1 的措辞不要暗示已覆盖评审轮。

## 核实记录

- `openspec validate add-ui-pseudocode-chain-gate --strict` → valid；`doc-impact.sh verify` → 通过。
- 烟测：esbuild 产物与 smoke 副本放在 /tmp 重定位运行，141 ✅、exit 0。变异体 m1/m2 全绿，m3 仅 B2'/B5'/B8' 红，m4 仅 B5' 红，m5 A1'/A2' 红，m6 A3' 红。
- 探针 P1–P12 直调 /tmp 构建产物；events.db 以 `readOnly` 打开。active + archive 共 209 份 ui-design.md，skeleton 目录 0 个。
- F.1：`add-home-pipeline-status-page` 无 `.openspec.yaml`（legacy 分支）；`add-server-capability-negotiation` 为 none；两个 minor 均 `ui-prototype: none`——均不受新检查影响。F.3：本 change 为 none 档，`evaluateUiContract` 在伪代码检查前返回 pass，`missingArchiveUiEvidence` 不查伪代码与链。
- C.2（先报 L1 再报 L2）合理。C.3（写入前快照，写 skeleton 自身不需 skeleton）成立，但该性质无用例保护（R-2）。
- 审查时 `ui-stage-missing` 已是 39 条（20:11 新增 1 条，早于 20:17 的提交）。
- 环境备注：工作树有 9 个 `openspec/changes/add-diagram-and-pseudocode-layer/` 的未暂存删除（HEAD 仍含这些文件，archive 目录已有副本），与本 change 无关，归档时可能触发 dirty-tree 提示。
- 本报告是本次审查唯一的写入；其余未修改任何仓库文件，未执行改变状态的命令。
