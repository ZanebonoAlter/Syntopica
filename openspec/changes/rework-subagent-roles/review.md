# 审查报告：rework-subagent-roles

- **审查者**：（你的模型名）（只读）
- **结论**：需改后进
- **放行判断**：不可进。R-1 是对主 spec `ui-design-workflow` 一条 MUST 的未声明反转（实测复现），R-2 是降级链落地缺口，R-3/R-4 是 spec 承诺与文档/档位不一致。修完这四条可进；R-5–R-9 可随同或归档前处理。

> 实测口径：harness smoke 795 项断言，77 项全绿（唯一红项 `block 事件 target 只含失败检查名` 即 brief 所述已登记台账的 `block-target-only-names`，非本 change）；`discoverAgents(cwd,'both',provider,{})` 五家 provider 下 explore/ui-design/impl-heavy/impl-light/change-review 均解析到 model；链与 settings.json 中全部 model id 经 `pi --list-models` 逐条比对存在（126 条目录，按 `provider<空白>modelId` 解析）；`.pi/agents/stage-*` 与 `stage-agent*.sh` 无残留。门禁行为不是读代码推断：取 smoke 自带 fixture 调真 `decideUiToolGate`，B10/B12 对照行结果与 smoke 一致后再看 impl 档结果。

### R-1 2026-11-27【High】impl-heavy / impl-light 进 UI 派发白名单，反转主 spec 的 MUST 且未声明
- **状态**：已修复
- **处置**：已修复（回退）：`impl-heavy`/`impl-light` 从 `UI_DISPATCH_ALLOWED_AGENTS` 移除——它们是写能力档，`ui-design-workflow` spec:101 明确「MUST 阻止具备写能力的实现子线程派发」。白名单恢复为 4 项（explore/plan/change-review/ui-design）。harness smoke 795 绿 / 1 红（台账项）。
- **位置**：`.pi/extensions/lib/ui-design-gate.ts:652-661`；`openspec/specs/ui-design-workflow/spec.md:101-103`
- **问题**：主 spec 写明 major pending 时门禁「MUST 阻止……**具备写能力的实现子线程**派发；MUST NOT 阻断只读档与原型编辑档」。`impl-heavy` / `impl-light` 是带 `write`/`edit`/`bash` 的实现档，却被加进 `UI_DISPATCH_ALLOWED_AGENTS`。常量注释「不产生 UI 合同外的界面改动」是声称而非约束：`impl-light.md:11` 自己的职责就含「样式」。且本 change 的 `specs/` 只有 `subagent-roles` 的 ADDED，没有对 `ui-design-workflow` 的 MODIFIED；spec 最后一条 Requirement 写的是「本次改动只调整提示词内容，不改文件名」，proposal/design/tasks(5.2) 全文无「白名单」字样。即行为变了，契约没跟。
- **证据**：用 smoke fixture（major + `ui-approval: pending`）调真函数，输出：
  ```
  dispatch general-purpose -> block ui-approval-pending      （B12 不变式仍成立）
  dispatch impl-light      -> dispatch-allowed ui-dispatch-allowed
  dispatch impl-heavy      -> dispatch-allowed ui-dispatch-allowed
  mutation front/app/pages/x.vue -> block ui-approval-pending
  mutation backend-go/x.go       -> block ui-approval-pending
  ```
  效果：实现子线程能被派出，但它在 `front/`、`backend-go/` 的每次写入都被拦——与 B12「派发时即拦」相比，是烧一轮派发后才撞墙。另：E5 只查「白名单项有同名文件」，E6 只查「无写工具的档在白名单内」，两条都不覆盖 `impl-*`，这个漂移没有任何用例守。
- **建议**：二选一并落字。(a) 默认应与主 spec 一致：白名单去掉 `impl-heavy`/`impl-light`，门禁注释删掉；或 (b) 若有意放行（理由写成可反驳的，如「用户已在原型循环确认过」），补 `specs/ui-design-workflow/spec.md` 的 MODIFIED Requirement + scenario，并在 design 记一条「推翻 add-ui-prototype-editor-agent D2」。无论哪种，smoke 补一行断言把 `impl-*` 的预期行为钉死。

### R-2 2026-11-27【High】降级链只有散文在执行：无代码读取，且省略 model 派发时主线程无从知道当前在链上第几位
- **状态**：已修复
- **处置**：已修复：`AGENTS.md` 与 `开发执行规范.md` 都写死「**走链的派发 MUST 显式传 `model` 参数**」+ 理由（不传则事后不知用的是链上第几个，429 会原样重派再撞墙），并加降级判定边界表（成功 / 429·401 换下一个 / 其他错误立刻返回 / 链耗尽停止且不许静默用链外模型）。
- **位置**：`.pi/model-chains.json:1`；`docs/reference/开发执行规范.md:133`；`AGENTS.md:143`；`.pi/harness/events.db`（`subagent.dispatch` 行 id=173374）
- **问题**：全仓无任何 `.ts`/`.sh` 读 `model-chains.json`（grep 只命中 md）。链完全靠主线程「读错误 → 换下一个重派」。但 tasks 11.1 记为已过的唯一一次 impl-light 实测，是省略 `model` 派发的：账本 `model=null`，实际模型由 `agentOverridesByProvider[当前 provider]` 解析。这种派发失败时，主线程手里没有「刚才用的是哪个模型」，无法判断该跳到链上第几个；更常见的是直接重派同一档、同一 provider override，等于原地重试。
- **证据**：`events.db` id=173374：`type=impl-light, model=None, promptChars=137, status=completed`。近 7 天约 190 条 `subagent.dispatch`：凡显式传 model 的都记了全称，`NULL` 的均为主线程未传。`selectProviderOverrides()`（pi-subagents bundle）在省略 model 时取 provider 层覆盖。
- **建议**：文档把动作写死成可执行的：「走链时派发**必须显式传** `model=<链首全称>`，省略 model = 不走链」。再加一条：账本 `subagent.dispatch` 行的 `model` 为空即视为未走链（可做成 retro 的一个口径）。spec「链可自行调整」场景也应补「且主线程每次派发前重读该文件」。

### R-3 2026-11-27【Medium】「429/401 才换、其他错误不换」「链耗尽如实报错」只写进了 design，没进主线程会读的文档
- **状态**：已修复
- **处置**：已修复：同 R-2 的边界表，三份主线程文档（AGENTS.md / 开发执行规范 / design.md）口径统一。
- **位置**：`AGENTS.md:143`；`docs/reference/开发执行规范.md:133`；`docs/reference/harness/pi-extensions.md:187`
- **问题**：design.md 有完整分支（非额度错误立刻返回、链耗尽不静默换链外模型），但三份主线程文档各只写了「429/401 换下一个」。「其他错误不换」三处为 0 命中；「链耗尽」`pi-extensions.md` 为 0。spec:62「向用户说明降级原因」三份文档也都没有。「忘了换」风险 proposal 风险 2 承认了（「派发直接失败，失败是显式的」），但与同文件 AGENTS.md:127「额度耗尽手工换，不做自动 fallback」并列存在，读者需要自己判断两者是不是一回事。
- **证据**：grep 计数：`AGENTS.md` 429/401=1、其他错误不换=0、链耗尽=1、忘了换=0；`开发执行规范.md` 同；`pi-extensions.md` 429/401=1、链耗尽=0。
- **建议**：三处统一成四句：①仅 429/401 换链；②5xx/超时/内容问题原样返回不重试；③链耗尽报错并停，不换链外模型；④降级后向用户说明。AGENTS.md:127 改成引用链段，避免两条规则并存。

### R-4 2026-11-27【Medium】「不读 AGENTS.md」只覆盖四档半：explore 与 general-purpose 仍指向相反方向
- **状态**：已修复
- **处置**：已修复：`explore.md` 与 `general-purpose.md` 的「必须自读 AGENTS.md」改成「不要读全文」，按 brief 最小集工作；`plan.md` 同步。`general-purpose` 保留唯一例外——brief 明确要求做流程判断时才读相关章节。现七个档全部写明不读全文。
- **位置**：`.pi/agents/explore.md:15`、`.pi/agents/plan.md:15`、`.pi/agents/general-purpose.md:21`；`AGENTS.md:145`
- **问题**：spec「子线程默认不注入 AGENTS.md 全文」的「最小集」只列了 review/impl/ui-design 三类，explore 没列。而 `explore.md:15` 写的是「需要项目规范上下文时自行用 read 读仓库根 AGENTS.md」，`general-purpose.md:21` 更是「动手前**必须**用 read 读 AGENTS.md」。用户原话是「验证子线程」，explore 是五角色之一；general-purpose 仍在 AGENTS.md:145 被列为实现/验证/通用档，与 impl-* 职责重叠且指令相反。
- **证据**：`grep -c '不要读' .pi/agents/explore.md` → 0；三处原文见位置行。tasks 11.1「不读 AGENTS.md」的实测只有 impl-light 一条（137 字符），explore / general-purpose / change-review / ui-design 未测。
- **建议**：`explore.md` / `plan.md` 的尾注改成与其余档一致（brief 给最小集，不读全文）；`general-purpose.md` 二选一——退役（AGENTS.md:145、`开发执行规范.md:169` 同步删）或改成与 impl-* 同向。验收项 D 补一条按档的实测清单。

### R-5 2026-11-27【Medium】三份文档与档位现状矛盾：change-review「只读 / 不绑模型」、ui-design「不绑模型」
- **状态**：已修复
- **处置**：已修复：`AGENTS.md` 通用档段、`开发执行规范.md` 档位选择行、`pi-extensions.md` 原型档说明，三处「不绑模型/模型派发时指定」全部改为「钉死 glm-5.4 / 走降级链」。
- **位置**：`AGENTS.md:145`；`docs/reference/开发执行规范.md:169`；`docs/reference/harness/pi-extensions.md:202`
- **问题**：本 change 给 `change-review` 加了 `write` 并绑 `zai-coding-cn/glm-5.4`，给 `ui-design` 绑了同一模型。但 AGENTS.md:145 仍写「change-review（只读通用档……**不绑模型**）」「ui-design……**不绑模型**」，`开发执行规范.md:169` 与 `pi-extensions.md:202` 同。E6 的 `WRITE_CAPABLE_EXEMPT=['ui-design']` 也落后一档：change-review 现在带 `write`，被 E6 当成「无写工具档」检查——恰好因为其在白名单内才没红。
- **证据**：`grep '只读通用档' AGENTS.md 开发执行规范.md` 命中两处；`grep '不绑模型' pi-extensions.md:202`；`.pi/agents/change-review.md:8` `tools: …, write`，`:15` `model: zai-coding-cn/glm-5.4`。
- **建议**：三处改为「绑 glm-5.4，走 review 链」；change-review 描述里「只读」改成「只审查，write 仅限 brief 指定的报告文件」；E6 豁免表补 `change-review`。

### R-6 2026-11-27【Low】proposal 的链写成 `gpt-sol-6.2`，目录里不存在这个 id
- **状态**：已修复
- **处置**：已修复：proposal 里 `gpt-sol-6.2` 改为全称 `openai-codex/gpt-6.2-sol`，并注明用户口头是简写。
- **位置**：`openspec/changes/rework-subagent-roles/proposal.md:49`
- **问题**：proposal 原样抄了用户口头的简写 `gpt-sol-6.2`。真实 id 是 `openai-codex/gpt-6.2-sol`（链文件与 tasks 4.2 都对）。AGENTS.md 对模型名的硬规则是全称、禁 fuzzy；制品里出现一个不存在的名字，后续照 proposal 手派会报错。
- **证据**：`grep -qxF 'openai-codex/gpt-sol-6.2'` 目录 → 不存在；`model-chains.json` review[1] = `openai-codex/gpt-6.2-sol`。
- **建议**：proposal 改为全称。

### R-7 2026-11-27【Low】settings.json 的 provider 层只覆盖三档，ui-design / change-review 五家均缺
- **状态**：已修复
- **处置**：核实后不成立：五套 provider 层各配 5 档（explore/plan/general-purpose/impl-heavy/impl-light）+ 2 个钉死档（change-review/ui-design 在 `agentOverrides`），脚本核对「缺口 无」。可能是审查时读到的是改动中途状态。
- **位置**：`.pi/settings.json:17-127`；`openspec/changes/rework-subagent-roles/tasks.md:20`（5.1「五套 provider」）
- **问题**：`agentOverridesByProvider` 五家 provider 都只有 general-purpose/explore/plan/impl-light/impl-heavy；ui-design 与 change-review 仅在顶层 `agentOverrides` 固定 glm-5.4。功能上没错（实测两家都解析到 glm-5.4），但 5.1 的「覆盖全部五个角色档」字面不成立，且意味着 glm 额度耗尽时这两个档在 provider 层没有第二选择——与「review 走降级链」靠省略 model 兜底的直觉相反。
- **证据**：脚本输出五行 `missing: ['ui-design','change-review']`；`agentOverrides keys: ['change-review','ui-design']`。
- **建议**：要么在每家 provider 补这两档，要么把 5.1 的表述改成「实现/探路三档按 provider，审查/原型档固定 glm 并走链」。

### R-8 2026-11-27【Low】档位与用户流程的贴合度：主干正确，两处小偏差
- **状态**：已修复
- **处置**：已修复：`impl-heavy.md` 补对称条款（发现其实不复杂就停下报告，别为配得上档位而过度设计）；tasks 验收补 E 项「implement→review 循环实走一次」与 F 项「impl-* 不在白名单」。
- **位置**：`.pi/agents/impl-light.md:17`；`.pi/agents/change-review.md:60`；`openspec/changes/rework-subagent-roles/design.md:112`
- **问题**：逐条对用户原话——step 3（整理需求、沟通）和 step 4（datamap）都没有被做成档 ✅；implement↔review 循环：`change-review.md:60`「一次可以审多个模块，逐模块给结论」+ spec 场景「一次审多个实现」✅；review 多层级：用户说「可以多设置几个子代理层级」，现实现是一档靠 brief，proposal 关键决定 1 记录了这是用户选的 ✅。偏差两处：① `impl-light`「发现是复杂逻辑就停下报告」只写在档位正文，降级方向（heavy 发现任务简单）没有对称条款，也无 tasks 条目对应验收；② 「每个模块干完就 review」的循环没有验收项——tasks §12 A–D 没有一条是「impl 产出一个模块 → review → 修 → 再 review」跑通一遍。
- **证据**：`impl-heavy.md` 无「发现是简单任务」字样；tasks §12 四条验收均为档位存在性/静态属性。
- **建议**：补 heavy→light 的对称条款；§12 加一条循环验收。

### R-9 2026-11-27【Low】权限边界：review 的 write 限定与 ui-design 读 front/ 均为散文约束，可接受但应写明
- **状态**：已修复
- **处置**：已修复：`ui-design.md` 边界补一句「读 `front/` 是允许的（看组件/样式变量让原型贴近真实观感），只读；改它一律越界」。
- **位置**：`.pi/agents/change-review.md:76`；`.pi/agents/ui-design.md:49`；`.pi/agents/impl-heavy.md:43`、`impl-light.md:42`
- **问题**：`change-review`「write 只许写 brief 指定的报告文件」、impl 两档「不改规划制品」、ui-design「不许改 front/」都已写明 ✅，但都只是提示词，无机械门禁（ui-design 的 edit/write 过 ui-design-gate，另两档不过）。ui-design 读 `front/` 做视觉参考：正文只禁「改」，读是允许的，合理，但没明说，容易被子线程理解为禁读后凭空设计。
- **证据**：`ui-design.md:49` 原文为「**不许改** `front/` 下的产品代码」；E5c 只断言 ui-design 的 `load_extensions`+tools，对 impl-*/change-review 无同类断言。
- **建议**：`ui-design.md` 加一句「读 `front/` 对照既有组件是预期行为，只是不许改」；R-1 处置后补 impl-* 的 profile 前提断言（`load_extensions: true`）。

- 观察（非问题）：impl 两档的验收信息是齐的——brief 给红线摘要 + tasks 条目 + 验收命令，档位正文指明 `change-scope.sh` / `test-patrol.sh`（两脚本存在），不存在「不知道跑哪些测试」。
- 审毕无发现（审查范围：链中模型 id 有效性、五档注册、阶段档与脚本删除、smoke 全量）。
