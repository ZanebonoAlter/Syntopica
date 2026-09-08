# reference 文档编写标准（doc-authoring）

<!--
doc-impact-applies: docs/reference/ | section=注册点速查
-->
> **权威源**：本文件是「新增/修订 `docs/reference/` 文档」的唯一标准源——目录职责、头部注释语法、flow 域模板、注册点 checklist 及各门禁拦截点。流程编排（何时做什么）见《开发执行规范》§0.6，两文互引不重复。
> 本文件自身按以下标准注册（自举）：头部 `doc-impact-applies` 标签 + `standard/README.md` 表 + `constraints-index.md` 登记——编辑 `docs/reference/` 下任意文档时本文件「注册点速查」节被 JIT 注入。

## 目录职责表

| 位置 | 装什么 | 不装什么 |
| --- | --- | --- |
| `flow/<域>.md` | 大功能五位一体活文档（需求说明/链路设计/业务约束/代码入口/变更溯源），承接原 user-guide「系统能做什么」 | 代码写法约束（→ standard/）、跨功能耦合（→ architecture/coupling-map.md） |
| `standard/` | 代码规范唯一权威源：frontend/ backend/ shared/ 三子目录（README 有细目表） | 业务流程链路（→ flow/）、执行流程与门禁（→ 开发执行规范.md） |
| `architecture/` | 架构定位与骨架；`map.md` 是业务域→流程→代码入口索引地图；`coupling-map.md` 跨功能传导耦合 | 具体业务约束（→ flow 对应节） |
| `api/` | API 参考（按路由前缀拆分） | — |
| `database/` | 数据库字段参考 | — |
| `开发执行规范.md` | 任务拆解/用例先行/门禁分层/归档纪律（how 与流程） | 代码风格细节（→ standard/） |
| `constraints-index.md` | 常驻约束索引（constraint-injection 数据源）；业务域→flow 文档映射表 | 手工堆放约束正文（正文在 flow/standard，这里只索引） |
| `configuration.md` / `deployment.md` | 配置 / 部署 | — |
| `development.md` / `testing.md` | 仅存构建/运行参考（规范内容已迁 standard/） | 新的规范条款（写这里=错误位置） |
| `docs/research/`（reference 之外，仓库顶层） | 调研自动落盘区（pin_finding 无档语境按 topic 分目录）；有长期价值的调研可整理晋升 reference。**无注册点、无门禁校验**；`docs/reference/research/` 已废弃删除，research 材料归置由用户统一处理中 | — |

## 注册点速查

三种头部注释是 harness（constraint-injection / doc-impact / spec-gate）的机器接口，**写错不报错、静默失效**：

| 注释 | 放哪 | 语法 | 仓库实例 | 写错的后果 |
| --- | --- | --- | --- | --- |
| `doc-impact-applies` | flow/standard 文档**头部 15 行内** | `doc-impact-applies: 路径前缀, ... \| section=节名`（裸行或 `<!-- -->` 包裹皆可；section 可选） | `flow/daily-report.md`：`doc-impact-applies: backend-go/internal/topicgraph/, ... \| section=业务约束与不变量`；`standard/backend/ai-logging.md`：块注释裸行 + `section=Requirements` | 超出 15 行=扫不到；路径不匹配编辑路径（前缀包含判定）=JIT 不命中；无任何报错。漏写 `section=` 则注入整文档（更大更贵）。仅档位激活时生效（有意设计） |
| `constraint-domains` | change 的 **proposal.md 头部** | `<!-- constraint-domains: 域名, ... -->`（域名=flow 文档 basename） | `<!-- constraint-domains: daily-report, topic-graph -->` | 域名≠flow basename=域声明注入不命中（widget 显示「无域声明」）；纯工具链 change 可不写。每回合重解析，改 proposal 即时生效 |
| `<!-- doc-impact: ... -->` | change 的 **tasks.md 文档节第一行** | `<!-- doc-impact: 域列表 -->`，域固定 8 选：flow / api / database / architecture / standard / configuration / deployment / none(附理由) | `<!-- doc-impact: standard -->` | 域不在 8 选项=verify 失败；「声明了未更新文档」→ verify FAIL 被 spec-gate ①拦；启发式疑似遗漏误报可加 `<!-- doc-impact-excuse: domain=理由 -->` 豁免（只豁免疑似遗漏，不豁免真没改） |
| `<!-- ui-front-path-excuse: 理由 -->` | change 的 **ui-design.md** | `<!-- ui-front-path-excuse: 前端路径为他人 change 脏文件示例，与本 change 无关 -->`（存在即豁免，理由不解析；fix-doc-impact-misattribution 引入） | `archive/2026-09-08-coordinate-concurrent-changes/ui-design.md`：豁免 tasks 历史豁免声明中的 `front/app/api/dailyReports.ts` 他人脏文件路径 | ui-design-gate 的 ui-impact-mismatch block 照常（该注释放行 none 档纯工具链 change 的前端路径误报；不写注释无任何报错） |

**flow 节名红线**：flow 文档的注入按「业务约束与不变量」节名**硬编码抓取**——节名写成「业务红线」「约束」等别名，域声明注入静默取不到节（A 段五段式校验也会 FAIL）。

**三个注释各是干嘛的、为什么这么设计**：

| 注释 | 干嘛的 | 为什么这么设计 |
| --- | --- | --- |
| `doc-impact-applies` | 声明文档的**代码辖区**：编辑命中这些路径前缀时，本文档（section 指定的节）被 JIT 注入 | 辖区声明放文档头部而非中心配置——文档自描述，新增文档零中心注册；头部 15 行限制 + mtime 缓存让扫描开销固定；「仅档位激活时生效」保「未激活=仅索引」不变量（未激活会话不硬塞，模型按需自读） |
| `constraint-domains` | change **声明涉哪些业务域**，档位激活后注入对应 flow 约束节（声明为主，关键词/编辑路径命中兑底） | 9 个 flow 约束节全量 43K 塞不下，域声明让注入范围显式可审计（widget 可见「无域声明」）；每回合重解析＝proposal 改了即时生效且不进粘性集合（防旧域残留）；域名＝flow basename 免维护映射表 |
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

「业务约束与不变量」节是 constraint-injection 的注入单元，且**声明域注入（proposal `constraint-domains` 命中）只取红线层**——节内顶层列表项的首个加粗块。细节层经关键词 / JIT 路径命中的全节注入、或模型自行 `read` 到达（`@constraint-declaration-redline`）。

- 每条约束 MUST 以顶层列表项（`N. ` 或 `- `，行首无缩进）呈现，**首词组加粗 `**...**` 为自含红线句**：脱离本文档上下文单独读该句，即知道「什么 MUST / MUST NOT」——含主语与边界（对象、触发时机、例外），不再是「TriggerNow 互斥」这类需上下文才能解码的主题短语。
- 红线句是既有约束内容的提炼重组，MUST NOT 新造语义、MUST NOT 增删或弱化不变量；细节跟在红线句后（`：` 分隔或紧随），细节不进红线层。
- 无加粗块的列表项不进红线层（提取器跳过该条，不取首行文本凑数）；引用块（`>`）与自由段落不属红线层；嵌套列表项属细节层。
- 红线层提取失败（0 条）或拼接低于 `minSectionBytes`（缺省 512B）时，声明域注入回退全节（fail-safe）——不遵循本格式 = 该域恒定全量注入，丢失瘦身收益。

提取器实现：`.pi/extensions/constraint-injection.ts` 的 `extractRedlines()`（顶层列表项行取首个 `**...**` 块，保留原文顺序与编号）；格式规范与提取器语义同步演进，改一处必改另一处。

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
2. [ ] 「业务约束与不变量」节每条约束首词加粗自含红线句（上方格式节）→ 不遵循：声明域注入恒回退全节（无门禁拦，注入字节不降）
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
| spec-gate ①-④ | ① doc-impact verify 退出码 0 ② check-standards 退出码 0 ③ tasks 尾三节+声明标记 ④ scenario-trace（验证节 `\| Scenario \| 测试文件 \|` 映射表，「人工」开头合法） | `openspec archive` 时 tool_call 拦截 |

## 维护提醒

改 harness 注册机制的 change（动 constraint-injection.ts / check-standards.sh / doc-impact.sh / spec-gate.ts 中注释语法或段语义），**必须把本文件纳入 doc-impact 影响面**（域=standard）并同步「注册点速查」/「门禁索引」——标准文档与实现漂移比没有标准更糟。
