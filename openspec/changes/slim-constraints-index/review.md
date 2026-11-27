# 审查报告：slim-constraints-index

- **审查者**：（模型名：GLM，change-review 只读档）（只读）
- **结论**：需改后进
- **放行判断**：无代码缺陷、信息无丢失、路由链路实测接通；需改 3 处制品文字（R-1 测试项指错脚本、R-2 spec 断言与事实不符、R-3 风险节说准）后可进归档门禁

这次改动没丢东西，路也接通了。索引瘦身后，原来那些「怎么写标记」的说明都能在别处找到，我拿旧版逐条对过；真正给模型看的路由清单在探索和提案两个阶段确实会把新搬过去的那一节指出来，节名一字不差。要改的是三处文字：测试表里写的那个烟测脚本其实根本不跑这次改的代码（我跑了该跑的那个，是绿的）；规范里说「说明只在 AGENTS.md 一处」，但开发执行规范里现在有两份一模一样的；提案说「没有门禁兜底」也不准确——一半有，一半没有，该把没有的那一半说准，顺手给它一个能看见的信号。

## 实测记录

| 项 | 结果 |
| --- | --- |
| `wc -c docs/reference/constraints-index.md` | 2718（原 6032，省 55%） |
| `grep -c '<!-- stage:' constraints-index.md` | 0 |
| 索引相对链接 | 20 条，0 死链（旧版 26 条 0 死链） |
| 域清单 vs `flow/` 含「业务约束与不变量」 | 10/10（`README.md` 只是提及，不是域）；`constraint-digest.sh` 可用域清单与索引完全一致 |
| `ROUTING_DOC_CATALOG` vs 文档标题 | 14 条 13 OK；`test-design.md` MISS（R-6，既存） |
| `buildStageRouting` 实跑（配置 select） | explore/propose 含 `开发执行规范.md`「13. change 头部声明与图表达（stage / 域 / diagrams marker）」；design/implement/verify 不含（符合 `.pi/constraint-injection.json`） |
| `bash .pi/extensions/tests/run-harness-smoke.sh` | 795 ✅ / 1 ❌（spec-gate 台账项 `block 事件 target 只含失败检查名`） |
| `bash .pi/extensions/tests/run-smoke.sh`（真正跑 constraint-injection 的） | 1078 ✅ / 1 ❌（同一条台账项）；stage-routing-1~5 全绿 |
| `bash scripts/harness/check-standards.sh` | 通过 360 / 失败 0 / 警告 0；J 段 153/153 domain marker 合法 |
| `openspec validate slim-constraints-index` | valid |
| `constraint-digest.sh --brief --domains research-data-sources` | 识别为合法域，但「该域无结构化红线（218 < 512）」（R-7） |

## 旧版信息归宿逐条核对

| 旧版（`/tmp/constraints-index.before.md`） | 新归宿 | 判定 |
| --- | --- | --- |
| `stage.md` 载体 + 样例路径（:27） | §13:585 + AGENTS.md:146 | 有 |
| `<!-- stage: -->` 六取值（:27） | §13 表格第 1 行（六值全列）+ AGENTS.md:146 | 有；explore/propose 路由可达 |
| 阶段族由 stage 派生、不可单独写（:27） | 索引/§13/AGENTS.md 都没有；在 `harness/pi-extensions.md:63` + `stage.ts:37-38` | 一跳可达（索引执行规范表 → pi-extensions.md） |
| `constraint-domains` 两处并集 + 只增不减（:25） | 索引:33 + §13:591 + §0.6:60 + AGENTS.md:146 | 有，且过多（R-2） |
| `--brief --paths/--domains` + 原样粘（:29,:31） | 索引:33 + §13:596 + §0.6 步骤 3（:186-210 三步 + brief 模板） | 有 |
| `--brief` 输出形态（`## 约束（红线…）`/`### <域>`）（:31） | §0.6:194 brief 模板 | 有；§13 未交叉引用（可补一句） |
| 域清单唯一真相源 = flow basename（:31） | §13:588 + §0.6:210 | 有 |
| 红线层回退规则（0 条/<512B/节缺失 → 指引行）（:25） | `pi-extensions.md:70` | 有（扩展内部约定，归 harness 文档合理） |
| keywordDocs / 索引域表 / doc-impact menu 三轴禁止混用（:31） | `pi-extensions.md:36` + `constraint-digest.sh:21-22,106-107` | 有 |
| 「与 AGENTS.md 优先级宪法冲突以宪法为准」（:3） | 扩展渲染 `constraint-injection.ts:1337`，smoke 断言 | 正确去重（本就重复） |
| `ai-logging.md` 直链 + 触发路径（:9） | 仅 `standard/backend/` 目录链接；文档自带 `doc-impact-applies` 覆盖 airouter/dataenrichment 路径 | 路径命中可补；不在任何阶段清单 |
| `test-design.md` / `commit-pr.md` / `doc-authoring.md` 直链（:11,:18,:19） | `standard/shared/` 目录链接；verify/archive 路由清单必含 | 有 |
| 执行规范表各行的场景描述（弹窗尺寸档 / V1–V6 等） | 删 | 可接受：文档名自述，路由清单给节名 |
| UI 工作流「legacy 兼容」+ 归档出处（:37） | 删 | 琐碎 |

没有「索引、§13、AGENTS.md 三处都没有」的信息。

### R-1 2026-11-27【Medium】tasks §6「smoke 不破」指向的 runner 根本不跑本 change 改的代码
- **状态**：已修复
- **处置**：已修复：tasks §6 的 smoke 命令改成 `run-smoke.sh`（这个才 esbuild 构建并跑 constraint-injection），实测 1078 绿 / 1 红（台账项）。原写的 `run-harness-smoke.sh` 不覆盖 stage.ts 改动。
- **位置**：openspec/changes/slim-constraints-index/tasks.md:70（§6 表「smoke 不破」行）
- **问题**：`run-harness-smoke.sh` 只构建 harness-log/failure-classify/spec-gate 等 19 个 bundle，不构建、不运行 `constraint-injection.smoke.cjs`（它自己的头注释第 7 行写着「constraint-injection 的记账回归在 run-smoke.sh」）。`stage.ts` 的 `ROUTING_DOC_CATALOG` 改动被这条测试项完全绕过——写了个检查，但没验证到改动物。
- **证据**：`.pi/extensions/tests/run-harness-smoke.sh:7` 注释 + `:13-42` 构建清单无 constraint-injection；`grep -n "bundle\|esbuild" run-harness-smoke.sh` 无 `constraint-injection.ts`。实跑 `run-smoke.sh`：1078 ✅ / 1 ❌，`stage-routing-1`~`5` 全绿。
- **建议**：§6 该行改为 `bash .pi/extensions/tests/run-smoke.sh`，期望「1078 绿 / 1 红（spec-gate 台账项）」。

### R-2 2026-11-27【Medium】spec「完整说明只在 AGENTS.md 一处」不成立——开发执行规范内部新增了一对重复
- **状态**：已修复
- **处置**：已修复：`开发执行规范.md` §0.6 里那两段（stage.md 载体 + 域声明）压成一句指针指向 §13，删掉重复的域声明段。现在三处各司其职：AGENTS.md 讲机制背景、§13 讲格式表格、索引只给一句路标——不再有三份完整副本。
- **位置**：openspec/changes/slim-constraints-index/specs/constraints-index-layering/spec.md:47-50；docs/reference/开发执行规范.md:60（§0.6，既存）vs :583-596（§13，新增）
- **问题**：§0.6:60 自 2026-11-19 起就完整写着 marker 格式 + 两处并集 + 只增不减 + 红线层描述（带 `@constraint-domain-declaration` 锚）；§13 又把同样内容写一遍。proposal 的立论是「同一协议两处各写一遍，改一处忘一处就不一致」，结果在同一个文件里新造了一对漂移面。scenario 的 THEN「完整说明只在 AGENTS.md 一处」与事实不符，下一个照 spec 核的人要么判红、要么把 §0.6:60 删了。
- **证据**：`git blame -L 60,60 docs/reference/开发执行规范.md` → `1371f215d 2026-11-19`；两处文本均含「两处并集」「只增不减」「红线层」。
- **建议**：定一个权威：§13 作格式权威，§0.6:60 缩成一句引用（保留锚点）；scenario 改为「完整说明只在 AGENTS.md 与 §13，§0.6 引用 §13」。

### R-3 2026-11-27【Medium】机制失传风险节说得不准：「声明了没读」已有门禁，真正不可见的只有两支
- **状态**：已修复
- **处置**：已修复：proposal 的风险段改成「哪些失败可见、哪些不可见」对照表——「声明了但没读」有归档门禁 ⑥ 兜（会红）、「完全没声明域」与「stage.md 缺失」确实不可见（静默 legacy）。另记了后续可加的可见化信号（`doc-impact-applies` 命中但域未声明 → spec-gate warn），本 change 不做。
- **位置**：openspec/changes/slim-constraints-index/proposal.md:62-66；scripts/harness/constraint-read-trace.sh:9-14, :213
- **问题**：proposal 说「没有门禁兜底」。实际 archive ⑥ / spec-gate ⑦'（`constraint-read-trace.sh`）已管住「声明了域但实现期没 read flow 文档」。真正不可见的是：①没声明域（脚本 ③「无声明即无义务」exit 0）；②`stage.md` 缺失/非法（① legacy 豁免）。改后索引只剩一行域声明提醒，这两支的触发概率不比改前低，但没有任何可回检指标。
- **证据**：`constraint-read-trace.sh:9-14` 判定表；`:213` 「无域声明 → 无义务」；`archive-readiness.smoke.sh:169` 只断言「声明域未读」分支。
- **建议**：(a) 风险节改为「哪两支可见、哪两支不可见」；(b) 给不可见支一个 warn 级信号——change diff 触及某 flow 文档 `doc-impact-applies` 所列路径、但该域未声明 → archive-readiness warn（数据已有，不新造）；(c) tasks §8 加一条 retro 回检：归档两周后 `harness-retro.sh` 看带 `stage.md` 的 change 占比与无域声明占比，对比改前基线。

### R-4 2026-11-27【Low】§13 结构：「三种头部声明」无编号，且 diagrams 前言被它吞掉
- **状态**：已修复
- **处置**：已修复：§13 的「三种头部声明」加编号 13.1；前言改成「前两种决定注入什么，第三种决定门禁查什么」，diagrams 不会再被读成第四种。
- **位置**：docs/reference/开发执行规范.md:578-600
- **问题**：「### 三种头部声明」之后直接接「**性质**：`design-diagram.md` 是 change 根的非 schema 制品」「**默认要画，`none` 才需理由」「#### 原型机械检查」，中间没有小标题——读者会把 `design-diagram.md` 当成第四种头部声明。其余小节是 13.1–13.5，这一节没编号。标题去掉了「伪代码层」，但 13.4 伪代码层还在。
- **证据**：`git diff` 显示新内容插在旧「## 13」标题与旧首段「**性质**」之间，无分隔标题。
- **建议**：在「**性质**」前加「### 13.1 图表达（diagrams marker）」并顺延编号（或把「三种头部声明」编为 13.0）；标题补回「/ 伪代码层」。

### R-5 2026-11-27【Low】spec Requirement 1 的 SHALL NOT 比索引实际保留的宽
- **状态**：已修复
- **处置**：已修复：spec 放宽为「SHALL NOT 包含操作性说明（载体样例/多步流程/完整枚举）」，并显式允许保留 `<!-- constraint-domains: ... -->` 的单行写法骨架——它是「往哪写」而不是「怎么用」。
- **位置**：specs/constraints-index-layering/spec.md:9 vs docs/reference/constraints-index.md:33
- **问题**：Requirement 写「SHALL NOT 包含操作手册类内容（marker 格式、载体样例、取数步骤详解）」，但索引保留了 `<!-- constraint-domains: 域名, ... -->` 格式 + 两处并集 + 只增不减 + digest 命令。Scenario 只搜 `<!-- stage:` 所以过了。保留这行是对的（它是常驻注入里唯一的机制钩子），但 spec 与实现不一致，下次有人「按 spec 修索引」就会把钩子删掉。
- **证据**：`constraints-index.md:33` 原文；spec:22-25 只测 `<!-- stage:`。
- **建议**：Requirement 改为「SHALL NOT 包含 stage marker 格式与载体样例；允许保留一行域声明提醒 + 一行取数命令」，并写明理由。

### R-6 2026-11-27【Low】ROUTING_DOC_CATALOG 既存漂移：`test-design.md` 节名不存在
- **状态**：已修复
- **处置**：已修复：`stage.ts` catalog 里 `test-design.md` 的节名「单元模型：一个能力一个故事…」文档里根本不存在，改成实际存在的「永久业务测试只留两类」。另加了一次全 catalog 校验（逐条比对 path 存在 + section 命中），现 0 漂移。
- **位置**：.pi/extensions/lib/stage.ts:253-256 「单元模型：一个能力一个故事，用例文档串起来」 vs docs/reference/standard/shared/test-design.md（实际标题：永久业务测试只留两类 / 白盒是临时验证 / 验证和归档 / UI 验收机械断言 / JIT 注入摘要）
- **问题**：verify/archive 路由清单让模型去读一个不存在的节。非本 change 引入，但本 change 动了同一张表，tasks §6「路由节名对齐」只手工 grep 了一条。
- **证据**：程序化比对 catalog 全部 14 条：13 OK，1 MISS。
- **建议**：顺手改成真实节名（如「永久业务测试只留两类」）；把「节名对齐」做成 smoke 断言（读文档标题比对 catalog，十行代码），别靠 grep。

### R-7 2026-11-27【Low】`research-data-sources` 红线层 218B < 512B 阈值——补进域表是对的，但声明了也只拿到取数指引行
- **状态**：已修复
- **处置**：已记录（不改）：`research-data-sources` 的红线约 218 B，低于 512 B 阈值，声明了也只拿到指引行。列进域表仍然必要（否则连名字都不知道）；已在 proposal 的「顺手修的」节写明这个已知局限与替代做法（自己跑 `constraint-digest --brief --domains`）。
- **位置**：`bash scripts/harness/constraint-digest.sh --brief --domains research-data-sources` 输出第 3 行；docs/reference/flow/research-data-sources.md「业务约束与不变量」
- **问题**：它是业务域（flow 文档自述「第 10 个业务域」，脚本可用域清单含它），补进索引正确。但其约束节没按 `doc-authoring.md`「约束节红线句格式」写顶层加粗红线句，提取仅 218B，低于阈值回退。另：不在 `constraint-injection.json` `keywordDocs`（9/10），关键词命中不会标注——既有的「keywordDocs 是子集」设计，不算错，但与索引「10 域」观感不一致。
- **证据**：脚本输出「该域无结构化红线（节字节 3525，红线层字节 218 < 阈值 512）」；`grep -c research-data-sources .pi/constraint-injection.json` = 0。
- **建议**：不在本 change 范围；记一条 follow-up 给该 flow 文档补红线句格式。

## 观察（非问题）

- 4 个 main spec 的 `<!-- domain: harness -->` 已随 `1cf777c9`（configure-subagent-model-routing）提交，工作区 `git diff openspec/specs` 为空；tasks 3.1 勾选与终态一致，只是 provenance 混在别的 commit。
- implement 阶段路由清单不含 `开发执行规范.md`，模型靠索引头部指针（常驻注入）+ 索引:33 一行域声明提醒拿格式；子线程无 AGENTS.md，但子线程不写 `stage.md`，足够。
- `openspec validate` 通过；ui-design.md `none` 理由是人话。

## 自指检验

- proposal 读起来是人话（起因 / 为什么是污染 / 改成什么 / 风险），合 `doc-authoring.md` §汇报叙事。两处不精确：「AGENTS.md:146 已经有一份，而且更完整」漏了 §0.6:60（R-2）；「没有门禁兜底」其实有一半（R-3）。
- tasks §6 七项：六项可跑且已复核；「smoke 不破」指错 runner（R-1）。
