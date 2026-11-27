# reference 文档编写标准（doc-authoring）

<!--
doc-impact-applies: docs/reference/ | section=注册点速查
-->
> **权威源**：本文件是「新增/修订 `docs/reference/` 文档」的唯一标准源——目录职责、头部注释语法、flow 域模板、注册点 checklist 及各门禁拦截点。流程编排（何时做什么）见《开发执行规范》§0.6，两文互引不重复。
> 本文件自身按以下标准注册（自举）：头部 `doc-impact-applies` 标签 + `standard/README.md` 表 + `constraints-index.md` 登记——编辑命中本辖区（`docs/reference/`）时本文件被列进阶段路由清单（正文不自动注入，模型按清单自主 read）。

## 目录职责表

| 位置 | 装什么 | 不装什么 |
| --- | --- | --- |
| `flow/<域>.md` | 大功能五位一体活文档（需求说明/链路设计/业务约束/代码入口/变更溯源），承接原 user-guide「系统能做什么」 | 代码写法约束（→ standard/）、跨功能耦合（→ architecture/coupling-map.md） |
| `standard/` | 代码规范唯一权威源：frontend/ backend/ shared/ 三子目录（README 有细目表） | 业务流程链路（→ flow/）、执行流程与门禁（→ 开发执行规范.md） |
| `architecture/` | 架构定位与骨架；`map.md` 是业务域→流程→代码入口索引地图；`coupling-map.md` 跨功能传导耦合 | 具体业务约束（→ flow 对应节） |
| `api/` | API 参考（按路由前缀拆分） | — |
| `database/` | 数据库字段参考 | — |
| `开发执行规范.md` | 任务拆解/用例先行/门禁分层/归档纪律（how 与流程） | 代码风格细节（→ standard/） |
| `harness/` | pi harness 扩展机制参考（扩展全景表/注入通道/门禁分层/事件账本入口） | 业务约束（→ flow/）、代码风格（→ standard/）、流程编排（→ 开发执行规范.md）、agent 日常行为红线（→ AGENTS.md） |
| `constraints-index.md` | 常驻约束索引（constraint-injection 数据源）；业务域→flow 文档映射表 | 手工堆放约束正文（正文在 flow/standard，这里只索引） |
| `configuration.md` / `deployment.md` | 配置 / 部署 | — |
| `development.md` / `testing.md` | 仅存构建/运行参考（规范内容已迁 standard/） | 新的规范条款（写这里=错误位置） |
| `docs/research/`（reference 之外，仓库顶层） | 调研自动落盘区（pin_finding 无档语境按 topic 分目录）；有长期价值的调研可整理晋升 reference。**无注册点、无门禁校验**；`docs/reference/research/` 已废弃删除，research 材料归置由用户统一处理中 | — |

## 注册点速查

三种头部注释是 harness（constraint-injection / doc-impact / spec-gate）的机器接口，**写错不报错、静默失效**：

| 注释 | 放哪 | 语法 | 仓库实例 | 写错的后果 |
| --- | --- | --- | --- | --- |
| `doc-impact-applies` | flow/standard 文档**头部 15 行内** | `doc-impact-applies: 路径前缀, ... \| section=节名`（裸行或 `<!-- -->` 包裹皆可；section 可选） | `flow/daily-report.md`：`doc-impact-applies: backend-go/internal/topicgraph/, ... \| section=业务约束与不变量`；`standard/backend/ai-logging.md`：块注释裸行 + `section=Requirements` | 超出 15 行=扫不到；路径不匹配编辑路径（前缀包含判定）=JIT 不命中；无任何报错。漏写 `section=` 则清单少节名（取数指引不精确）。**编辑路径命中只把文档列进阶段路由清单**（零正文；正文注入通道已随 stage 化退役） |
| `constraint-domains` | change 的 **proposal.md 头部 ∪ stage.md 头部**（两处并集，只增不减） | `<!-- constraint-domains: 域名, ... -->`（域名=flow 文档 basename） | `<!-- constraint-domains: daily-report, topic-graph -->` | 域名≠flow basename=域声明红线层不注入（widget 显示「无域声明」）；纯工具链 change 可不写。每回合重解析，改 proposal/stage.md 即时生效 |
| `<!-- domain: ... -->` | `openspec/specs/*/spec.md` **头部 8 行内**（首个 `#` 标题前） | `<!-- domain: 域名[, 域名] -->`，域名封闭 15 选：10 个业务域（= flow basename）+ 5 个工具域（harness / ui / dev / api / db），多域上限 3 个 | `openspec/specs/daily-report-system/spec.md`：`<!-- domain: daily-report, semantic-board -->`（契约源：spec-domain-index） | 缺 marker → 该 capability 不进任何域反查，其历史 delta 落第 6 段「孤儿 delta」；词表外/空值/超 3 域 → `check-standards.sh` J 段 WARN 并指明非法值（不阻断）；`domain-history.sh` 对词表外查询名拒绝并打印词表 |
| `<!-- doc-impact: ... -->` | change 的 **tasks.md 文档节第一行** | `<!-- doc-impact: 域列表 -->`，域固定 8 选：flow / api / database / architecture / standard / configuration / deployment / none(附理由) | `<!-- doc-impact: standard -->` | 域不在 8 选项=verify 失败；「声明了未更新文档」→ verify FAIL 被 spec-gate ①拦；启发式疑似遗漏误报可加 `<!-- doc-impact-excuse: domain=理由 -->` 豁免（只豁免疑似遗漏，不豁免真没改） |
| `<!-- ui-front-path-excuse: 理由 -->` | change 的 **ui-design.md** | `<!-- ui-front-path-excuse: 前端路径为他人 change 脏文件示例，与本 change 无关 -->`（存在即豁免，理由不解析；fix-doc-impact-misattribution 引入） | `archive/2026-09-08-coordinate-concurrent-changes/ui-design.md`：豁免 tasks 历史豁免声明中的 `front/app/api/dailyReports.ts` 他人脏文件路径 | ui-design-gate 的 ui-impact-mismatch block 照常（该注释放行 none 档纯工具链 change 的前端路径误报；不写注释无任何报错） |
| human-feedback 条目格式 | change 的 **human-feedback.md**（可选制品；收到用户对产物的反馈时记入） | 标题行 `### FB-<N> <日期>【<类型>\|<归因>】<标题>` + 状态行 `- **状态**：<状态>`；类型二值 产品\|流程，归因五类封闭 frontend\|backend\|test-gap\|fact-check\|process（多因按根因优先归 fact-check），状态五值封闭 待处理\|处理中\|已消化\|已转挂\|已放弃（契约源：add-change-lifecycle-retro design D7；词表 `feedback.sh menu`） | `openspec/changes/ai-provider-model-overhaul/human-feedback.md`：10 条含归因标注与状态，verify 全处置 | 词表外归因/缺状态行 → `harness-retro --lifecycle` 第五段计「格式异常」单列、`feedback.sh verify` 状态不明视同未处置提醒（归档对账不阻断）；不进 events.db（文件随 change 归档永久保存） |
| review 建账与豁免 marker | change 的 **review.md**（步骤 4 review 意见建账）+ **proposal.md 头部**豁免声明 | 条目标题行 `### R-<N> <日期>【High\|Medium\|Low】<标题>`（编号自 R-1 自增）+ 状态行 `- **状态**：<处置词>`（五值封闭：待处理/已修复/已采纳/已驳回/误报丢弃，括号说明可选）；零发现写显式声明行 `- 审毕无发现（审查范围：<说明>）`；不做 review 的 change 在 proposal.md 头部机器可读注释区（与 ui-impact 同层）写 `<!-- review: waived -->`（理由写在正文，单文件小修不豁免；契约源：add-review-trace-gate design D3/D4） | `openspec/changes/add-review-trace-gate/review.md`（本 change 自吃狗粮） | 词表外等级/状态不计入合法条目、空壳文件同零痕迹 → `review-trace.sh` 退出码 1，spec-gate 检查⑤阻断归档；marker 写在正文段落不识别；waived 占比进 `harness-retro --lifecycle` review 覆盖统计；review.md 不进 events.db（随 change 归档永久保存） |

**flow 节名红线**：flow 文档的注入按「业务约束与不变量」节名**硬编码抓取**——节名写成「业务红线」「约束」等别名，域声明注入静默取不到节（A 段五段式校验也会 FAIL）。

## diagrams marker 与伪代码节

**marker**（`proposal.md` 头部，与 `ui-impact` 同层）：

```
<!-- diagrams: full(白话理由) -->     有图；design-diagram.md 必须存在且含 mermaid 块
<!-- diagrams: none(白话理由) -->     无图；理由必填，写清为什么不需要
```

机器只查「有没有 marker / 理由空不空 / 图文件在不在」，**不判理由是否成立**——那是 review 与看图人的事（详见 `开发执行规范.md` §13）。

**伪代码节**（`design.md`）：必须有 `## 核心逻辑（伪代码）`，覆盖数据结构、主控制流、失败与边界分支；粒度到「看伪代码能发现问题」为止。无控制流变化时写 `N/A：<理由>`。

## 族级 capability 与来源注释

capability **按功能族划，不按 change 划**。每来一个 change 就新建一个 capability，会让同一套规矩碎成十几个文件（实测：标签体系曾碎成 23 个 spec / 2771 行，改一次要开 21 个文件）。

| 场景 | 做法 |
| --- | --- |
| 新 change 改一个已存在的功能 | **MODIFY 现有族级 spec**，不新建 capability |
| 功能确实自成一族（与现有族无改动连带关系） | 新建 capability，命名用功能名而非 change 名 |
| 族内碎片已多（>1500 行 或 改一次要读 >3 个 spec） | 开一个合并 change，用 `scripts/harness/merge-spec-family.sh` 生成初稿 |
| 合并时搬迁 Requirement | 每条标题下加 `<!-- from: <旧capability> (<归档日期> <归档change>) -->`；来源只认 `## ADDED Requirements` 的 change，查不到就只写 capability 名，**不编造** |
| 合并时发现施工记录（包结构/命名/一次性迁移/纯测试要求） | 排到族 spec 末尾并加 `<!-- kind: impl -->`，**不删**——删契约要单独提 change |

**族名与旧 capability 重名时就地扩容**（不搞改名新建）：旧 capability 保留，只把兄弟成员的 Requirement 以 `## ADDED Requirements` 追加进去；代价是它原有条目不重排，可接受。

**族级 domain marker 按「改这族必须读哪些 flow 红线」取舍，不按「成员碰过哪些域」**：J 段上限 3 域装不下成员并集，选主业务域 + 会改变契约含义的次域（如 `tagging` 选 `semantic-board, ui`，因队列芯片是用户可见组件）。已知代价：被舍弃的次域查询不到该族。

**归档必须声明 `retire_capabilities: true`**：capability 的 Requirement 被 REMOVED 清空后，openspec 默认「写入重建稿 → 校验」，空 spec 校验失败会整次中止；声明后归档直接删除 spec 文件并修剪空目录，**不会留空壳**（先例：`archive/2026-11-20-retire-story-adopt-event-metrics/.openspec.yaml`）。

**三个注释各是干嘛的、为什么这么设计**：

| 注释 | 干嘛的 | 为什么这么设计 |
| --- | --- | --- |
| `doc-impact-applies` | 声明文档的**代码辖区**：编辑命中这些路径前缀时，本文档进本阶段路由清单（路径 + section 节名） | 辖区声明放文档头部而非中心配置——文档自描述，新增文档零中心注册；头部 15 行限制 + mtime 缓存让扫描开销固定；**编辑路径命中（文件系统事实、零歧义）只把文档列进本阶段路由清单**，引导模型自主 read——正文注入通道已退役（stage-driven-constraint-injection） |
| `constraint-domains` | change **声明涉哪些业务域**，阶段激活后注入对应 flow 约束节的**红线层**（关键词/编辑路径命中只进路由清单，不送正文） | 9 个 flow 约束节全量 43K 塞不下，域声明让注入范围显式可审计（widget 可见「无域声明」）；两处并集=proposal（存量）+ stage.md（实现期活口径，只增不减）；每回合重解析＝改了即时生效且不进粘性集合（防旧域残留）；域名＝flow basename 免维护映射表 |
| `<!-- doc-impact: -->` | 声明本 change **动了哪些文档域/文件**，归档前 verify 对账 | 文档更新靠自觉必漏（本标准存在的理由）——机器可读声明把「别忘了更新文档」变成机械校验；excuse 只豁免「疑似遗漏」（多 change 并行脏树误报），不豁免「声明了未更新」（真没改文档，必须真改） |

## flow 域文档模板

新 flow 域文档骨架（五段式节名固定，A 段逐节校验）：

```markdown
# <功能名>（<英文名>）

<!-- doc-impact-applies: <辖区代码路径, ...> | section=业务约束与不变量 -->
> 大功能：一句话。
> 跨端。互补：`flow/<相关域>.md`（相关性一句话）。

## 需求说明
## 链路设计
（mermaid 流程图 + 状态流转）
## 业务约束与不变量
（状态机/幂等/去重/限额等红线——本节是 constraint-injection 注入单元，每条约束按下方「约束节红线句格式」书写）
## 代码入口
（后端 handler/service + 前端 feature 入口）
## 变更溯源
| 日期 | 归档位置 | 说明 |
| --- | --- | --- |
```

「变更溯源」初始为空表头即可；每次归档后按《开发执行规范》§12.2 追加行（含 `<date>-<change>` 全名，E 段校验依赖）。

## 约束节红线句格式（declaration 注入层）

「业务约束与不变量」节是 constraint-injection 的注入单元，且**声明域注入（proposal `constraint-domains` 命中）只取红线层**——节内顶层列表项的首个加粗块。细节层由模型按**阶段路由清单**自行 `read` 到达（关键词/JIT 两条正文通道已随 stage 化退役，`@constraint-declaration-redline`）。

- 每条约束 MUST 以顶层列表项（`N. ` 或 `- `，行首无缩进）呈现，**首词组加粗 `**...**` 为自含红线句**：脱离本文档上下文单独读该句，即知道「什么 MUST / MUST NOT」——含主语与边界（对象、触发时机、例外），不再是「TriggerNow 互斥」这类需上下文才能解码的主题短语。
- 红线句是既有约束内容的提炼重组，MUST NOT 新造语义、MUST NOT 增删或弱化不变量；细节跟在红线句后（`：` 分隔或紧随），细节不进红线层。
- 无加粗块的列表项不进红线层（提取器跳过该条，不取首行文本凑数）；引用块（`>`）与自由段落不属红线层；嵌套列表项属细节层。
- 红线层提取失败（0 条）、拼接低于 `minSectionBytes`（缺省 512B）或约束节缺失时，声明域注入回退**取数指引行**（路径+节名+digest 命令，零正文）——不遵循本格式 = 该域红线不进注入，模型只能自己 read 全文，红线「必达」降级为「靠自觉」。

提取器实现：`.pi/extensions/lib/constraint-redlines.ts` 的 `extractRedlines()`（顶层列表项行取首个 `**...**` 块，保留原文顺序与编号）——constraint-injection 扩展与 `scripts/harness/constraint-digest.sh` **双引同一实现**，红线提取只有这一份，脚本侧禁止另写正则；格式规范与提取器语义同步演进，改一处必改另一处。

> **marker 理由括号怎么写才够大白话**（`diagrams: none(...)` 那类）→ 见下方 §汇报叙事。

## 汇报叙事：先给人话（EIAL5，2026-11-27）

> 来源：台账 ISS-11，用户原话「用 explain as i am 5 的程度进行解答是有必要的」。
> 这条**只做规范，不做门禁**——理由见本节末尾。

### 规则

**任何向用户汇报的场合，第一段用大白话，结构化信息放后面。**

第一段要能回答两个问题：
1. **发生了什么**（对用户而言，不是对代码而言）
2. **对你有什么影响**（要你做什么 / 你会看到什么不同）

术语、条款编号、`file:line`、门禁项名、脚本命令——**都可以有，但放在第二段之后**。

### 四种最常翻车的场合

| 场合 | 反例 | 正例 |
| --- | --- | --- |
| change 完工的部署影响 | 「`agentOverridesByProvider` 三层配置已生效」 | 「以后换订阅不用改文件了，切一下主会话就行」 |
| review 收口 | 「R-1 High：参数顺序错导致记账为零」 | 「有个 bug 让统计一直没记上，因为传参传错了地方」 |
| 制品 marker 的理由 | `diagrams: none(无状态机/跨层调用/schema 变更)` | `diagrams: none(只把「确认」两个字改成「标记」，别处没动)` |
| 派发前方案说明 | 「按 §0.6 步骤 3 取域红线」 | 「我先把这片的红线抄给你看，免得派下去做错」 |

### 和门禁机检格式怎么共存

`review.md` 要求 `### R-<N> <日期>【级别】<标题>` 这种机器可解析的格式——这跟「第一段说人话」不冲突：**顶部的自由人话段不计入门禁解析，`### R-N` 块从第二段起算**。实测 `review-trace.sh` 只扫 `###` 开头的块，人话段放着不影响。

### 判据（自检，不是机检）

**5 秒自检**：数一遍第一段里的内部术语（`reasonCode` / `fail-open` / `幂等` / `R-1` / 模块名），**>0 个就挪到后面**。

**30 秒自检**：把第一段读给**不熟悉本项目的人**听，他能不能复述出「这次改了什么范围」。复述不出来就是没写够——多半是术语太多，或者省略了主语（「已生效」→ 什么生效了？）。

两道都过不了，别急着发。

### 为什么不做机械校验

用户明确：「不用机械校验，靠自觉（也没法机械校验）」。

**只能判「字样在不在」的检查会催生应付式写法**。具体两个洞：

1. **黑名单永远不全**——规定「首段不得出现 `R-1` / `reasonCode` / `MUST`」，那我换个没被列进去的术语（`fail-open`、`幂等`、`白名单`）照样能过
2. **挡不住空话首段**——「这次改动优化了流程」满足所有格式要求，但什么也没说

规则满足了，可读性没改善，还多一道维护成本。

这跟 `ui-prototype-mechanics` 那轮**撤掉三项 CSS 检查**是同一个教训：`outline:none` 也能满足「有 :focus」，`padding:24px` 也能满足「热区够大」。**判不了的就不判，写清规范让人自己负责。**

## 最佳实践案例（照这些抄）

| 要写什么 | 照谁抄 | 好在哪 |
| --- | --- | --- |
| flow 域文档 | `flow/daily-report.md` | 头部标签辖区路径写全 + `section=业务约束与不变量`；互补引用行点明与 topic-graph / semantic-board 的分工；五段式齐全，「业务约束与不变量」节按条列红线——注入后模型可直接执行 |
| standard 文档 | `standard/backend/ai-logging.md` | 块注释裸行形态标签 + `section=Requirements`（节级注入比整文档省）；头部「权威源」声明 + 🛛 红线标记醒目 |
| proposal 域声明 | `openspec/changes/watch-keyword-and-quickadd/proposal.md`：`<!-- constraint-domains: topic-graph, daily-report -->` | 多域逗号分隔，域名＝flow basename 一字不差；归档例：`archive/2026-08-23-fix-quality-audit-p0` |
| tasks 文档节 + 豁免 | `archive/2026-08-23-constraint-injection-tier-b/tasks.md` | `none(理由)` 写满理由 + excuse 逐域写原因——多 change 并行脏工作树误报的标准处理法 |
| 反面教材 | `archive/2026-08-23-scenario-test-mapping-gate` | 纯工具链 change 归档时漏写「无 flow 影响」豁免词 → E 段持续 FAIL 卡住**所有后续归档**（2026-08-23 补手续才清）——豁免词一字不差，归档前自查一遍 |

## checklist：新增 flow 域

1. [ ] 建 `flow/<域名>.md`，五段式节名一字不差 → 漏节：**check-standards A 段**
2. [ ] 「业务约束与不变量」节每条约束首词加粗自含红线句（上方格式节）→ 不遵循：声明域回退取数指引行（红线不进注入；无门禁拦）
2. [ ] 头部 15 行内写 `doc-impact-applies`（辖区路径 + `| section=业务约束与不变量`）→ 漏写：JIT 静默失效，**无门禁拦**（最容易漏）
3. [ ] `constraints-index.md` 业务规范节登记域名 →flow 文档 → 漏登：constraint-injection 域声明不识别，**无门禁拦**
4. [ ] proposal.md 写 `<!-- constraint-domains: 域名 -->` → 漏写：域注入不触发（widget 有提示）
5. [ ] tasks.md 文档节 `<!-- doc-impact: flow -->` + checkbox → 漏声明/声明未更新：**spec-gate ① / F 段**
6. [ ] 归档后在 flow 文档「变更溯源」表补行 → 漏补：**E 段**（归档后 check-standards）

## checklist：新增 standard 文档

1. [ ] 放对子目录：frontend/ backend/ shared/（代码 vs 跨端约定）→ 放错位置：无硬门禁，但读者找不到
2. [ ] 头部 15 行内写 `doc-impact-applies`（辖区代码路径 + `| section=<规范节名>`）→ 漏写：JIT 静默失效（同上）
3. [ ] `standard/README.md`「这层装什么」表加行，且文件名被 开发执行规范/AGENTS.md/子目录 AGENTS 之一引用 → 漏引：**check-standards D 段「孤立文档」**
4. [ ] `constraints-index.md` 执行规范表登记（导航可见性）→ 漏登：无门禁拦，但脱离注入体系
5. [ ] tasks.md 文档节 `<!-- doc-impact: standard -->` → 同 flow checklist 5
6. [ ] 新文档内引用其他 .md 用相对路径正确（若被导航层引用，死链进 **G 段**）

## 门禁索引（哪里拦你）

| 门禁 | 校验内容 | 拦截时机 |
| --- | --- | --- |
| check-standards A | 文档完整性（standard/flow/architecture/map.md 关键文件存在） | 归档前（spec-gate ②） |
| check-standards B/C/H | 后端结构/domain 白名单/前端结构/model tag（与文档新增无关，改代码时注意） | 同上 |
| check-standards D | standard/**（含 shared）**/*.md 被引用（文件名 grep：front·backend-go·根 AGENTS / standard README / 开发执行规范） | 同上 |
| check-standards E | archive change 被 flow「变更溯源」表引用（2026-06-29 后） | 归档后 |
| check-standards F | active change 的 doc-impact 声明对账（doc-impact.sh verify） | 归档前 |
| check-standards G | 导航层（docs/README、docs/reference/*.md 一级、flow/README、architecture/map.md）相对 .md 链接死链 | 归档前 |
| check-standards J | main spec 的 `<!-- domain: -->` 存在性 + 值合法性（15 词表 / 非空 / ≤3 域）——**只 WARN 不阻断**（观察期） | 归档前 |
| spec-gate 文档与任务结构 | doc-impact、check-standards、tasks 尾三节和声明标记；不再对账 Scenario 映射。其余检查见开发执行规范 §11 | `openspec archive` 时 tool_call 拦截 |

## 维护提醒

改 harness 注册机制的 change（动 constraint-injection.ts / check-standards.sh / doc-impact.sh / spec-gate.ts 中注释语法或段语义），**必须把本文件纳入 doc-impact 影响面**（域=standard）并同步「注册点速查」/「门禁索引」——标准文档与实现漂移比没有标准更糟。
