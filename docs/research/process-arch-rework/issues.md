# 流程架构返工 — 议题台账

> 建立：2026-11-26 ｜性质：讨论台账（非 openspec 制品、不设门禁、可勾选推进）
> 来源：一次关于「开发流程架构」的讨论（用户痛点在原型返工 + 验收靠人肉翻 + 主线程上下文污染）。
> 用途：把结论与待定项逐条钉住，避免对话漂移丢失；每条可独立转 openspec change。
> 已定方向 ≠ 已实现。状态三值：`待讨论` / `已定方向` / `已落地（change 名）`。

---

## A 类 · 制品形态（用户最关心，优先）

### ISS-01 决策制品图形化：ER / 顺序图 / 状态图 / 用例图 + 伪代码层

- **状态**：`已落地`（change `2026-11-26-add-diagram-and-pseudocode-layer`，2026-11-26 归档；契约源 `change-diagram-convention`）
- **已定（2026-11-26 讨论）**：
  1. **单一 marker 管全链**，不逐层各一个（`diagram-er: skip` / `diagram-seq: skip` 这类不要）。因为**最终是人工看图，这一步省不掉**，机器不需要逐层精准判
  2. marker 二值 + 人话理由：`<!-- diagrams: full(白话理由) -->` / `<!-- diagrams: none(白话理由) -->`
  3. **数值阈值降级为参考信号**，不做门禁判据（不红不拦）；可作 agent 自检提示 + 人工看图 checklist
  4. 缺席/产出理由 MUST 按 **EIAL5（explain as I am 5）** 写（见 ISS-11）
- **层**：制品形态 / 沟通载体
- **用户原话**（需求原意，勿转述失真）：
  > 原型这块其实md我都基本不看，我最希望的是图片表达，涉及到数据库的需要er图，用户故事需要顺序图，复杂业务流转的需要状态图，大型srs的需要用例图，按架构和用户故事顺序拆解功能设计，这个就是《伪代码》沟通。
  > 原型就是基于这些基础的业务上的视觉表达。
- **现状机制**：`docs/reference/开发执行规范.md` §13 已有 `design-diagram.md`（change 根可选制品），但只有三类图（`flowchart` / `stateDiagram-v2` / `sequenceDiagram`），**可选、不设门禁**、质量在 review 里提。契约源 `openspec/specs/change-diagram-convention/spec.md`。
- **缺口**：
  1. **无 ER 图**（数据库/SRS 场景）、**无用例图**（大型 SRS 场景）——用户点名的两类恰好缺
  2. 图是「补充」不是「主载体」：判据写的是「成立才画，不凑数」，默认仍是文字 md 主导
  3. **落盘形态**：mermaid 代码块仍是文字。用户不看 md 的根因是「要读字」——不渲染成图（SVG/PNG）等于没解决
  4. 「按架构 + 用户故事顺序拆解功能设计」的**顺序性**没有载体（现在 design.md 是无序条目）
- **待定问题**：
  - Q1 四类图的**必画判据**（什么信号触发 ER / 顺序 / 状态 / 用例），以及判据能不能机器判
  - Q2 图的**生成工具链**：mermaid（已在 §13）vs 项目内 `diagram-design` skill（支持 ER/use case/sequence/state，输出 HTML/SVG/PNG/drawio/excalidraw）——选哪个，或分工
  - Q3 图**怎么进 openspec 制品链**：扩展 `design-diagram.md` 契约？还是新增 `ui-design.md` 上游一节？是否升级为 schema 制品
  - Q4 **渲染产物是否入库**（SVG 入库 vs 构建期生成）——入库则 diff 噪声大，不入库则用户看不到需要额外命令
  - Q5 「伪代码层」怎么定位：独立制品（`pseudo-code.md`）？还是 design.md 的强制节？伪代码粒度（函数签名级 / 控制流级）
  - Q6 与 `ui-design.md` L0 契约的关系：图在 L0 之前定稿？还是 L0 的一部分
- **六个待定问题已全部落定（2026-11-26，change `add-diagram-and-pseudocode-layer`）**：

  | Q | 结论 |
  | --- | --- |
  | Q1 必画判据 | **反转**：默认画，`none` 需写白话理由；五类图判据只回答「画哪几类」，不再是「该不该有图」的闸门 |
  | Q2 工具链 | **mermaid 直出，零脚本**。实测 pi-web 自带 mermaid（`^11.16.1`），对话里贴三个反引号 mermaid 直接渲染 |
  | Q3 进制品链 | `design-diagram.md` 仍是非 schema 制品；伪代码进 `design.md` 强制节 `## 核心逻辑（伪代码）` |
  | Q4 渲染产物入库 | **不入库**——对话里看，文件只留 mermaid 源 |
  | Q5 伪代码层 | `design.md` 强制节，粒度判据「看伪代码能不能发现问题」；无控制流变化写 `N/A：<理由>` |
  | Q6 与 ui-design.md | L0 引用图时写**指针**，不复制（复制会漂） |

- **一次被推翻的方案（值得记）**：原本打算 chromium headless 渲染 mermaid → PNG → PIL 裁白边 → agent `read` 图片贴进对话。技术上**跑通了**（实测两张成功），但用户看不到——**pi-web 的工具结果不渲染图片**。改成直接贴 mermaid 源码块后才通。教训：**技术可行 ≠ 达成目的**；「用户能不能看到」是独立于「能不能做出来」的判定点。
- **新增硬约束**：图写完 SHALL 在对话中贴出 mermaid 源码块，不能只落盘。这条唯一无法机器验证，靠 review 兜——因为「落盘」和「看到」是两件事。
- **关联**：ISS-02（原型作为下游表达层）、ISS-03（原型机械检查）
- **讨论顺序建议**：第一个谈。它决定 ISS-02 的上游长什么样。

### ISS-02 原型定位重排：从「第一决策现场」降为「业务图 + 伪代码的下游视觉表达」

- **状态**：`已落地`（change `2026-11-26-add-ui-pseudocode-chain-gate`，2026-11-26 归档）
- **层**：制品形态 / 流程顺序
- **症状**：原型是唯一被反复审的地方，讨论修改全往原型上堆；用户不看 md、也不想在原型上做业务逻辑决策。
- **根因（比原记录更具体）**：`ui-design.md` 八节**全是界面层**（User Journey / IA / Interaction / State / Layout / Component / Prototype / Acceptance），**没有一节讲「数据从哪来、怎么算、状态怎么变」**。业务问题没有载体，唯一出口就是原型——不是因为该在那儿讨论，而是因为别处没地方放。
- **关键洞察（用户点破）**：「靠文本不太行，我设计了原型分层，但是一点都没遵守」。数据坐实：`events.db` 里 `ui-stage-missing` **39 条**、`ui-stage-scope-out` **0 条**——分层规则从未被触发，因为 marker 从来没被写。**任何「让 LLM 自报状态再按状态检查」的方案都有同一个漏洞：它可以不申报。**
- **解法（已落地）**：
  1. **业务先定**：`ui-impact: major` 且 `ui-approval: approved` 时，`design.md` 必须有 `## 核心逻辑（伪代码）` 节（或 `N/A：<理由>`），否则 block `ui-pseudocode-missing`。pending 期不查（原型探索期允许边改边定）
  2. **产物依赖链**：`design.md` 伪代码 → `ui-design.md` 八节 → `ui-prototype/skeleton/` → `ui-prototype/index.html` → `ui-prototype/fixture.json`，缺前一层 block `ui-chain-out-of-order`。**只判文件存在性，不判自报阶段**
  3. **按写入路径触发**：链在 `check()` 里紧跟 `accountStageScope` 执行，不看 stageFamily / verdict——这是 review R-1 抓出的致命修复（原实现挂在 pass 分支，原型早写完了，拦截数 0）
  4. **显式出口**：`<!-- ui-skip: skeleton(理由) -->` / `data(理由)`，理由非空才豁免——显式留痕优于隐性跳过（marker 可以不写，ui-skip 必须写出来）
  5. **只对 major 强制**：minor/none 不背骨架链；谎报档位由既有 `scanImpactMismatch` 抓
- **不做**：不把 `ui-stage-missing` 转 block（那会让所有现有 change 卡死，且 marker 方案本身就失败）；不判「这个改动算不算业务口径」（机器判不了，靠 review 与对话里主动喊停）
- **已知残留**：链只保证「写原型时伪代码存在」，不保证「伪代码是最新的」——批准后清空或改写发现不了，这个漂移靠 review 不靠门禁
- **关联**：ISS-01（上游图 + 伪代码）、ISS-03（原型机械检查）

### ISS-03 原型机械检查

- **状态**：`已落地`（change `2026-11-26-add-ui-prototype-mechanics-gate`，2026-11-26 归档）
- **最终形态：只机器化一项——破坏性操作必须有二次确认**

立项时按「人眼可靠吗」分了堆，本想机器化四项（热区/焦点/悬停/危险操作）。两次校准推翻了：

**校准一（用户澄清）**：现存那个 504 文件 / 98M 的「原型目录」不是垃圾桶，是 **three.js 首页改版的视觉探索**——多版本实验页、渲染导出、blender 试点是那类工作的正常产物。所以规范先分两类：

| 类别 | 声明 | 机械检查 |
|---|---|---|
| 功能原型（默认） | `<!-- ui-prototype-kind: functional -->` | 适用 |
| 视觉探索原型 | `<!-- ui-prototype-kind: exploratory(理由) -->` | 不适用 |

**校准二（评审实测）**：另三项的静态判定是形式主义，整条撤回——
- 热区：任何容器有一处 `padding: 24px` 全过；`padding: 12px 24px` 反被判红
- 焦点/悬停：判「出现过 `:focus` 字样」——`outline: none` 过关，用浏览器默认焦点环的反被判红

热区 24×24 / 焦点 / 悬停 / 键盘**保留为规范里的人核设计下限**，明写「不做机器检查」。

- **门禁**：`scripts/harness/ui-prototype-check.sh`，挂 `spec-gate` 检查⑨（真拦）+ archive-readiness ⑨（自查，八项→九项）
- **致命教训（R-1）**：`set -o pipefail` 下 `printf | grep -q` 会让写端吃 SIGPIPE(141)，**同一输入 30 次跑出 5 种裁决**。全部文本判定改 here-string。这是我第二次踩「测试假绿」——smoke 21 条全绿看不见它，因为 fixture 输入太小写端不阻塞；现在 smoke 专门造 100KB 输入连跑 5 次断言一致。
- **判据的一般化**：**能判「有没有这个机制」的才机器化，只能判「字样在不在」的不要机器化**——后者会逼出应付式写法（`outline:none` 满足「有 :focus」），比不查更糟。

--- | --- | --- |
  | 热区 <44px、可点元素无 hover/focus、破坏性操作无确认 | 不可靠（100 个元素数不过来） | **留**，纯机械 |
  | 状态有没有路径到达（闭环） | 可靠（看图就能看出来） | **砍**，交给人 |
  | 状态该不该存在、流程顺不顺 | 只有人能判 | 本就不该机器管 |
  | 原型里的文字预算 | 半可靠 | 改成「文字是否在替代结构」，不是数字数 |
  | 单屏可点数 >12 | 可靠 | **砍**，一眼就知道挤不挤 |

- **保留项的实现搁置点**：DOM 卫生扫描（44px/hover/focus/确认）——用 agent-browser 还是 jsdom，待定
- **层**：门禁
- **症状**：越改越散、状态不闭环、操作反人类。根因不是注意力，是原型允许无限追加且无机械边界。
- **⚠️ 已重新评估**：「最终是人工看图，这一步省不掉」→ 改为只保留人眼数不准的项，见上表。
- **实测事实**：`openspec/changes/add-home-pipeline-status-page/ui-prototype/` = **530 文件 / 96MB**；正常 change 是 11–14 文件 / ~1MB。
- **三个候选检查**（均脚本可判）：
  1. **文字预算**：原型目录业务文案中文字数上限（如 ≤800 字/change），超则红
  2. **状态闭环**：`interaction-table.md` 每行 = (触发元素 → 目标状态)；校验每个 status 至少一条操作可达 + 每个可点元素四态文案齐全（空/加载/错/成，取自 `standard/frontend/prototype-content.md`），缺则红
  3. **操作卫生**：DOM 扫原型 HTML —— 热区 <44px、破坏性操作无确认、可点元素无 hover/focus、disabled 无解释、单屏可点 >12 个
- **待定问题**：
  - Q1 三件的阈值定多少（先观察期只 warn 还是直接 block）
  - Q2 「操作卫生」用 agent-browser 还是 jsdom 扫（前者真实渲染、后者便宜）
  - Q3 挂在哪个扩展/脚本：新脚本 `ui-state-closure.sh` + 现有 `ui-verification-trace.sh` 复用？
- **关联**：ISS-01、ISS-02

---

## A-2 类 · 沟通标准（跨制品）

### ISS-11 汇报叙事标准（EIAL5）——跨制品通用

- **状态**：`已落地`（change `2026-11-27-add-plain-report-standard`；规范在 `standard/shared/doc-authoring.md` §汇报叙事）
- **层**：沟通形态（不绑任何单一制品）
- **主张**：**所有「向用户汇报」的场合，先给一段 5 岁能懂的人话，再给结构化信息**。术语与判据编号只用于内部（agent 之间、门禁记账），不出现在给人读的第一段。
- **适用面**：图链缺席理由、UI 原型档位汇报、review 收口、派发前方案说明、change 完工的「部署影响」汇报
- **反面样例**（不要这样）：`diagrams: none(改动为配置项，无状态机/跨层调用/schema 变更)`
- **正面样例**：`diagrams: none(只把按钮文字从「确认」改成「标记」，别的都没动，画出来跟以前一模一样)`
- **价值**：治「人工注意力越改越散」——每轮汇报都从人话重建上下文，人不用从术语里倒推这次在干嘛
- **落定**：Q1 → `standard/shared/doc-authoring.md` 新节（写文档时必读，必经路径）；Q2 → **不做机检**，用户拍板「不用机械校验，靠自觉（也没法机械校验）」。理由：只能判「字样在不在」的检查有俩洞——黑名单永远不全、挡不住空话首段（同 ISS-03 撤掉三项 CSS 检查的教训）。替代是两道自检：5 秒数术语个数、30 秒读给不熟项目的人听。

---

## B 类 · 规模代价（用户认可方向，待细化）

### ISS-04 规模账单：按仓库常态对比，不设硬预阈

- **状态**：`实施中（首族已合并）`（2026-11-26 用户拍板走「路 B 现在就合并」+ 按功能族粒度 + 设计层级概念与架构图）
- **已落地（2026-11-26）**：change `2026-11-26-merge-tag-spec-family` 把标签体系 23 个 capability / 2771 行合并为 3 个族——`tagging`（L0，55 req）、`auxiliary-label`（L0 就地扩容，38 req）、`tag-matching`（L2，55 req）。全仓 spec 178 → 158。Requirement 逐字搬运零语义改动，每条带 `<!-- from: -->` 来源注释；施工记录单列末尾标 `<!-- kind: impl -->`（116 business / 29 impl）。
- **分层模型（代码依赖验证）**：L0 原子（文章/标签）→ L1 语义结构（话题/版块）→ L2 加工（匹配/增强/发现/阅读）→ L3 形态（工作台/日报）→ L4 工程支撑。证据：`tagmanagement` 被 reader/topicgraph/discovery/dataenrichment 四包 import。
- **第二族（2026-11-26）**：change `2026-11-26-merge-board-diagnostic-specs` 把「版块诊断/研究」4 个 spec 合成 2 族——`board-diagnostics`（体检 405 + 方向校验 210 = 22 req）、`board-research`（外部新闻研究 387 + 版块语义调查 382 = 36 req）；`radar-pipeline-status`（195 行 UI 状态块契约）**保留独立**，因为「名字都带 board」不等于同一族。全仓 spec 158 → 156。
- **归族判据沉淀**：按「改 A 时是否几乎必然要动 B」切，不按名字前缀切。反例：5 个 board-* 里 radar-pipeline-status 与其余 4 个零改动连带，硬并会把 UI 契约混进分析族。
- **第三族（2026-11-26）**：change `2026-11-26-merge-board-upgrade-api-specs` 把升级/接口 6 个 spec 合成 2 族——`board-upgrade`（就地扩容，23 req，升级建议管线）、`board-api`（25 req，CRUD+文章列表+升级接口面+前端对话框）。全仓 spec 156 → 152。
- **第四族（2026-11-26）**：change `2026-11-26-merge-topic-specs` 把话题/watch 4 个 spec 合成 2 族（均就地扩容）——`persistent-topic`（26 req，持久话题层 + 泳道关键事件标记）、`topic-watch`（13 req，watch 提示轨 + 物化轨）。全仓 spec 152 → 150。
- **第五族（2026-11-26）**：change `2026-11-26-split-daily-report` 把全仓最肥单 spec（daily-report-system 1417 行 / 41 req）拆 3 族——`daily-report-system`（就地扩容，23 req，生成管线+选题+分桶）、`daily-report-interface`（18 req，端点+前端+雷达状态块）、`daily-report-ops`（7 req，补档+告警）；`daily-report-match-fallback` 与 `radar-pipeline-status` 并入后退役。全仓 spec 仍 150（退役 2 + 新建 2）。
- **第六族（2026-11-26，泳道组收口）**：change `2026-11-26-merge-lane-ops-specs` 只并两组真碎片——`lane-ops`（新建，23 req，人工修正+动态视图+趋势+热度量纲）、`workshop-board-console`（就地扩容至 33 req，吸收文章档案视图）。9 个里 3 个保持独立（radar / lane-correction-surface / workshop-lane-compose），理由见 design D1。全仓 spec 150 → 146。
- **收敛完成（六轮总账）**：178 → 146 spec；13 个族级合并/拆分 change；全部 Requirement 逐字搬运零语义变更。domain-history 覆盖 15 个域。
- **剩余**：`related-events`（122 行，阅读页事件检索）；`data-enrichment`（684 行，独立域）；`workshop-board-console`（1120 行）与 `lane-correction-surface`（728 行）是下一批拆分候选（分界：矩阵+归档 vs 关键事件视图 / 画布布局 vs 修正交互）。
- **巨型 spec 的「过期」比「碎片」更危险（2026-11-26 fix-stale-canvas-specs）**：审计两个巨型（board-console 1120 行 / lane-correction 728 行）发现 10 条条款与代码不一致，其中 1 条到了「照 spec 改会复现 spec 自己警告的缺陷」的程度（写出 44px 行距，实际 52px，44 < 占高 48）。**教训：spec 里的推导数值（`40+4=44`）会随常量漂移，应写「约束 + 当前取值（非契约）+ 唯一出处常量」。** 已修 9 条（MODIFIED，条数守恒 33/19），另 13 条缺写 / 7 条跨族重复 / 死代码清理各登记去处。
- **脚本化批改 spec 的陷阱**：整行替换会连带删掉行内仍有效的契约（第一版就把 `selectedLane` / 话题行序 / 「切换视图 MUST NOT 重置」一起删了，靠 review 抓回）。**批量改 spec 必须用最小子串替换，且改完逐条 diff 核对。**
- **MODIFIED 不能改 Requirement 标题**：openspec 按标题匹配，改名会被当成新 capability（旧标题留着、新标题算 ADDED）。要改标题只能 REMOVED + ADDED，或保留原标题并在正文注明沿革。
- **归档陷阱（本轮实测）**：openspec 的 retire 只认「标题 + 首段 + scenario 子弹」，**markdown 表格行与首段后散文段**都算 unaccounted content → 拒绝删除并整次中止，而六项门禁不含 dry-run 查不出。清掉后方可归档（内容确认已在目标族里）。
- **拆分机制**：新增 `scripts/harness/split-spec-family.sh`（按标题子集切分，`--kind`/`--purpose`），支持「部分搬运 + 就地扩容」；plan 与分类清单存 change 目录可复现。
- **已登记待办（合并副产品）**：①「跨族引用清扫」change——修各族合并后残留的旧 capability 名引用（tag-matching「详见 board-direction-check」、board-diagnostics:222、board-concept-management:5,26）；②「升级建议跨族收口」change——board-api 里的生成侧描述降为指针、手动回填队列并入接口族，顺带修 `/upgrade/suggest` 与 `/upgrade-suggest` 矛盾路径、升级建议 API/DTO 重复 Scenario、红线 6/7 独立条款。
- **层**：门禁 / 观测
- **症状**：性能退化、代码膨胀、测试文件暴增——这三样**不会让任何现有门禁变红**，只能验收时肉眼发现。
- **实测基线（2026-11-26，近 120 commit，排除 archive 移动）**：

  | 指标 | 实测 |
  | --- | --- |
  | 新增文件按类别 | 文档 564、前端代码 201、后端测试 169、前端测试 150、后端代码 136 |
  | **测试 : 实现（文件数）** | **0.95**（319 / 337）—— 几乎 1:1 |
  | 测试行 : 实现行（提交行数） | 0.94；测试行占「代码+测试」中位 **49%**、p90 70% |
  | 文档 : 代码（文件数） | **1.67** |
  | 存量测试文件 | 后端 `_test.go` **514 个**（中位 187 行，72 个 >500 行，max 1716）；前端 `*.test.ts` **285 个** |

  → 按 `standard/shared/test-design.md`（永久业务测试只留 API 主线 + 真实 Bug 回归，白盒用完即弃），测试 1:1 不可能都是主线，说明白盒逾期未删。
- **方案：不叫预算，叫「规模账单」**（预算暗示硬限，账单只记账）
  1. **基线不拍数字**：由脚本从 git 历史自算「仓库常态」，无需维护的阈值文件，不会过时
  2. 只对「偏离常态」提示一句（如 >2 倍），**不 block**（呼应「数值可参考不绝对」）
  3. 输出形态 = 人话 + 数字（ISS-11 EIAL5），例：
     > 本次实现 12 文件 / +820 行，测试 11 文件 / +740 行（0.90，常态 0.95，正常），文档 +310 行（0.38，低于常态 1.67）。一个值得看的点：新增测试里 6 个是查 SQL 字符串匹配，属白盒，按规范该丢弃——能砍 400 行。
  4. **白盒嫌疑粗筛**（首版可做可缓）：测试文件里 grep `sqlmock` / 手造 httptest / 临时前缀 / 单个测试 >3 个 mock
- **复用**：文件归属直接吃现有 `edit.map`（无需新数据）
- **触发点**：change 完工汇报（AGENTS.md 强制那节）+ 归档门禁 warn
- **待定**：Q1 偏离阈值（2x？）、Q2 白盒粗筛进不进首版、Q3 文档比例要不要单列
- **文档判定已定（2026-11-26）**：不用全仓比例，改为**按功能域看体量**——flow 的 11 个域已被 `constraints-index.md` 索引，但 `openspec/specs/` **无域索引**（177 个 spec，96 个连前缀都归不进业务域）。域归属缺失问题另立 ISS-13；未补上之前文档项暂只报数不判定。

### ISS-05 测试膨胀与白盒残留

- **状态**：`已并入 ISS-04`（数据已取得：测试:实现 = 0.95，后端 514 个测试文件、前端 285 个）
- 保留编号避免后续引用断链；具体方案看上条。

### ISS-13 openspec main spec 膨胀：不进上下文，但缺域索引

- **状态**：`已落地`（change：`2026-11-26-add-spec-domain-index`，2026-11-26 归档；方案 A）
- **落地结果**：178 个 main spec 全部有合法 domain marker（177 + 本 change 自己）；`domain-history.sh` 六段输出（spec 清单/时间线/flow 红线/REMOVED/MODIFIED/孤儿 delta）；J 段 warn 观察期。用户已拍口径：报告类按作用对象（semantic-board 主域）、AI 平台 9 个挂 ai-summary、标签池 10 个挂 semantic-board。
- **实测（2026-11-26）**：

  | 层次 | 规模 | 下次 apply 会读吗 |
  | --- | --- | --- |
  | `specs/*/spec.md`（main spec） | **177 个 / 27203 行**，最大 `daily-report-system` 1414 行 | **不读** |
  | `changes/<name>/**`（活跃 change） | 4 个 | 读（`contextFiles`） |
  | `changes/archive/**` | 283 个 change、527 个 delta spec、15MB md | 不读 |

- **实测验证**：`openspec instructions apply --change <name> --json` 返回的 `contextFiles` = change 内 proposal/ui-design/specs/design/tasks **合计 341 行**，main spec **零行**。→ **main spec 膨胀 ≠ 上下文膨胀**，这是 openspec 设计对的地方（apply 只吃增量）。
- **真实代价不在 token，在另三处**：
  1. **碎片化**：177 个 spec 平均 154 行；按名称前缀分组，**96 个落 `other`**（无业务域可比对）
  2. **矛盾**：两个 in-progress change 可各自 MODIFIED 同一 Requirement 且互不包含——已发生并被 review 抓到（`2026-11-25-add-key-event-other-lane` R-High「两个 in-progress change 对同一 Requirement 的 MODIFIED 文本互斥」）
  3. **找不到**：agent 写 delta 时不知道该 MODIFIED 哪个 spec，只能赌
- **根因**：项目里有两套文档体系——`docs/reference/flow/` 11 个域（有 `constraints-index` 索引，被注入消费）与 `openspec/specs/` 177 个 capability（**无任何索引轴**）。openspec 的 spec 粒度是「capability」不是「业务域」，单人项目 177 个 capability 可能过细。
- **建议方向（待拍）**：
  - **方案 A**（低成本可逆）：每个 `spec.md` 头部加 `<!-- domain: <flow 域> -->`，与 flow 同名。收益：域体量可算（ISS-04 落地前提）+ 写 delta 时能定向；代价：177 个文件加一行
  - **方案 B**（大手术）：把碎片 spec 合并回域级 capability。收益：域内一致性有唯一真相；风险：不可逆、archive 的 delta 合并会跨文件、历史溯源断链
  - **方案 C**：不动 spec，只给 `openspec/specs/` 建一张人读索引（域 → spec 列表）
- **待定**：Q1 选 A/B/C；Q2 177 个里 96 个 other 具体是什么、能不能归；Q3 域体量上限定多少（相对 flow 文档 + 该域代码行数？）
- **实现进展（2026-11-26）**：change `add-spec-domain-index` 已建（proposal / ui-design N/A / delta spec / design 已落，`openspec validate` 过）；177 个 capability 归类已派子线程判（模型 GLM-5.3，因判错会传播到后续所有 change 的 delta 定向），产出 `spec-domains-draft.md` 待人工复核 `l`/`?`。
- **已实测不可行的方案**：纯关键词脚本归类 —— `daily-report-system` 被判成 semantic-board（616 vs 533）、`board-signal-reports` 判成 data-enrichment、`api-docs` 判成 scheduler，全错。关键词对「出现频次」敏感而非对「职责」敏感。

### ISS-14 flow 文档按 change 追加导致的堆积

- **状态**：`semantic-board 已落地，其余域待推广`（change `2026-11-26-restructure-flow-semantic-board`，2026-11-26 归档）
- **根因**：链路设计按 change 追加，只增不减。`semantic-board.md` 646 行里链路设计占 448 行、17 块标题直接带 change 名——想查「标签匹配现在什么规矩」要在 5 块里各翻一段。
- **一处判断修正**：本台账原写「链路设计装的是 ADR，spec 里没有」。抽样看 **17 块大部分是「当前规则描述」**，历史决策只占少数。所以不是「ADR 被埋」，而是**规则按 change 切片存放**——做法跟着变：先把规则按族归位，再把散落的历史降到同族的决策记录层。
- **落地形态**：6 族（与 spec 对齐）+ 每族两层
  | 族 | spec |
  |---|---|
  | 一、标签体系 | `tagging` |
  | 二、版块匹配 | `tag-matching` |
  | 三、升级建议 | `board-upgrade` |
  | 四、泳道与工作面 | `lane-ops` + `workshop-board-console` + `workshop-lane-compose` |
  | 五、版块体检 | `board-diagnostics` + `board-research` |
  | 六、版块治理 | `board-api` + `board-concept-management` + `tagging`(related-events) |
- **硬约束**：`## 业务约束与不变量` 节名与 39 条红线**逐字不动**（`constraint-injection` 按节名 + 顶层加粗句抓取）。其余四节自由重组。
- **对账口径（这是本 change 最大教训）**：判据是「**正文行零丢失**」，不是「总行数相等」——重排必然新增结构行（族标题/层标题/族说明）。我第一版 spec 写的「非空白行数 SHALL 相等」从一开始就是错的，是 review 抓出来的。
- **评审抓到的真伤（R-1）**：`> **两阶段事务边界**` 被整块下沉到决策记录，但它含 4 条**现行约束**（慢 I/O 事务外、阶段二原子性、`ConfirmSuggestion` 同模式、PG 超时）。判据应是「**换一种做法会不会错**」——会错就是现行规则。同理「watch 观察池已退役，**不得再生成**」也是现行禁令，不是历史。
- **推广判据**：链路设计 >150 行且块数 ≥10。按此 `daily-report`(322/14)、`data-enrichment`(254/11)、`reading`(168/12)、`topic-graph`(143/14) 达标；`ai-summary`(49/2)、`research-data-sources`(20/0) 不达标不动。
- **进度**：`semantic-board`（6 族）与 `daily-report`（3 族）已改；余下达标的 `data-enrichment`(254/11)、`reading`(168/12)、`topic-graph`(143/14) 待做。
- **第二轮（daily-report）新增教训**：
  - **「粗体内含 change 名」不等于「这段是历史」** —— Step5.5 泳道内主题分桶因为标题带 change 名被误下沉，而它讲的是在跑的现行规则。真正判据只有「换一种做法会不会错」，需要**人工复核每一条**，正则只能做初筛。
  - **数字要当场核**：proposal 写「14 块 / 9 块后端」实际是 13/8；「73 条红线」按编号是 73、按 `constraint-digest` 提取是 71（2 条不符合顶层加粗句格式）。两种口径要分别说清。
  - **「来源 change」是元信息不是决策记录**：放块标题正下方，别塞进决策记录层撑壳。
- **flow 覆盖缺口（本轮发现，未修）**：
  - `daily-report-margin-notes`（8 条 Requirement，报纸页边注）在 flow 里无链路块，只在阅读台布局里提一句 `MarginNotesRail.vue`
  - `event-story` 的 flow 侧只存在于「已停写」的事件线块里
- **已知残留**：过期文本原样留在当前规则（搬迁前旧债）；「相关事件」放六族偏离标签语义但移动会破坏「只搬不改」；h6 深度用满；长引用块里结论靠后——都记在归档 change 的 review.md。

--- | ---: | ---: | ---: | ---: |
  | semantic-board | 643 | 39 | 76 | **446** |
  | daily-report | 554 | 73 | 62 | 320 |
  | data-enrichment | 427 | 50 | 18 | 252 |
  | reading | 264 | 15 | 28 | 166 |
  | topic-graph | 236 | 25 | 29 | 141 |
  | content-enrichment | 183 | 16 | 17 | 113 |
  | scheduler | 152 | 8 | 27 | 80 |
  | discovery | 150 | 25 | 8 | 75 |
  | ai-summary | 118 | 19 | 15 | 47 |
  | research-data-sources | 64 | 0 | 5 | 18 |

  → `semantic-board` 与 `daily-report` 都偏胖，但**约束节本身很紧**（semantic-board 48 行装 39 条红线，只占全文 7.5%）。胖的是链路设计。
- **不是冗余（重要，别当垃圾删）**：链路设计里装的是 **ADR——为什么这么设计 + 曾经踩过什么坑**，spec 里永远没有这类内容。抽样证据：
  - 「两阶段事务边界（fix-aux-label-idle-tx-cache）」：单事务内调外部 embedding API 导致生产 `idle in transaction` 4–40s、事务持 xmin 阻塞 vacuum
  - 「L2 不会形成合并黑洞」：对比主标签路径 embedding 命中曾覆盖 label → 重生成 → 恶性循环（`v1.3.1/fix-tag-blackhole-embedding-match`）
  - 「空结果是合法结论」：旧规则词回填把分类名「新闻」与 HTML 属性子串误命的「Coding」写库
- **真正的问题在组织**：
  1. 按 change 追加而非按功能族 → 无入口、越加越长
  2. 当前链路与历史决策混在一起 → 读完分不清哪条还有效
  3. 信息密度极不均 → 9 行的块（`Event 标签延迟 embedding`）与 90 行的块（`升级建议生命周期`）并排
- **方案（重排规则）**：
  1. **按功能族分节**（与 spec 的族对齐）：一族一节 = mermaid 流程图（当前链路）+ 决策记录（标注 change 名 + 日期）
  2. **当前链路与历史决策分离**：历史决策可收进附录或折叠，不干扰"现在什么规矩"
  3. **原样保留 39 条红线**（48 行是最干净的部分），只调编号顺序
  4. 变更溯源表（76 行）保留——它是索引，有检索价值
- **硬约束**：`## 业务约束与不变量` **节名不能改**——注入系统按节名硬编码抓取红线句（`constraint-injection` 的声明域注入）。其他四节（需求说明/链路设计/代码入口/变更溯源）可自由重组。
- **前置依赖**：ISS-04 的 spec 分族。flow 的分族必须跟 spec 的分族对齐，spec 改一半就动 flow 要改两遍。
- **预估效果**：643 行 → 约 400 行（同样信息，去掉重复的 change 名标题与失效的历史描述），且能按族跳读。
- **待定**：Q1 历史决策放附录还是折叠块、Q2 是否给 10 个 flow 文档统一模板、Q3 是否把 ADR 独立成 `docs/reference/decisions/`（权衡：独立后 flow 更薄，但"为什么"与"是什么"分离，读的人要多跳一次）

### ISS-12 性能回归（规模账单治不了）

- **状态**：`待讨论（未展开）`
- **症状**：用户提到「代码实现导致的性能问题」，但性能与代码体量不等价——大可以是对的，小可以是雷的。
- **现状**：无关键路径 benchmark、无慢查询阈值告警、无 pprof 巡检
- **待定**：Q1 要先找出「哪几条路径算关键」（日报生成 / 板内去重 / 向量检索？）Q2 用 Go benchmark 还是线上慢日志采样；Q3 进不进 quality-gate（benchmark 很慢，不适合 turn_end）

---

## C 类 · 上下文路由（用户提出，待讨论判据）

### ISS-06 子线程 explore 派发判据

- **状态**：`已落地`（change `2026-11-26-add-router-discipline-nudge`，2026-11-26 归档）
- **用户意图**：主线程强制注入很多多余信息污染判断，想用子线程 explore 返回结论，但需要「什么时候调、什么时候不调」的判据。
- **判据（已落地为规范）**：不判「任务该不该派」（主观），判**分工类型**——探索 / 实现 / 验证外包给子线程，判断 / 验收留主线程。表在 `开发执行规范.md` §主线程纪律。
- **机制（不靠自觉）**：`.pi/extensions/router-discipline.ts` 观测两个行为信号——单次 `read` >200 行、同一文件连续 `edit` ≥3 次——`steer` 一句提醒（每会话每类一次，不阻断）。
- **豁免必须带理由**：`bash scripts/harness/router-discipline.sh --excuse "<理由>"`，理由非空才生效，per-session，理由进事实库（`action=bypass` / `reasonCode=router-excused` / `target=<理由>`）。与 `ui-skip` / `doc-impact-excuse` 同一纪律：显式留痕优于隐形跳过。
- **⚠️ 生效条件（重要）**：新扩展文件在会话启动时加载，**本会话内新增不会生效**。归档时实测：主线程读了多个大文件、连续 edit 同一文件多次，`router-discipline` 事件数为 0——功能代码已就绪，要**重启会话/pi-web** 才跑起来。
- **已知边界**：只覆盖 `read`/`edit`，`bash`（`cat`/`sed`）绕得过——实测本轮大量工作确实走 bash，这是明确接受的取舍。
- **立项时的一处误判（已撤回）**：初稿写「主线程 $44.96 vs 子线程 $0.015、单价差 2600 倍」，review R-3 指出 `session.rollup.models` 是**主线程自己切模型**的聚合（`subagent.dispatch` 的 cost 全为 null），两个数分属不同口径、相除无意义。已从四处文案撤回，只留可核对计数（995 次工具调用 vs 25 次派发）。
- **判据的关键转换（保留）**：判据不该是「什么时候派 subagent」，而是**「什么信息有资格进主线程」**——只收结论 / 决策点 / 路径指针（`file:line`），>200 行原文不落主线程。

### ISS-07 主线程大读记账（先观察，后决定拦不拦）

- **状态**：`已定方向`（讨论结论：不要一上来写硬拦插件）
- **层**：上下文路由 / 观测
- **做法**：先只在 quality-gate 或新扩展里记账（主线程直接大读次数、体量分布），跑两周看数据再定阈值与是否硬拦。参照 `ui-stage-scope-out` 的「观察期转阻断」规矩。
- **待定问题**：Q1 记在哪（`events.db` 新 kind vs 复用 `constraint.inject`）；Q2 什么算「大读」（read 行数 / grep 命中数 / bash 输出字节）

### ISS-08 constraints-index 稳定层瘦身

- **状态**：`待讨论`（低优先级）
- **层**：上下文路由
- **实测事实**：`constraints-index.md` 全文 **6031 字节**，每 session 进 system prompt，占中位注入量（8KB/session）的 ~75%；它本质是张路由表。
- **待定问题**：改 digest 能省 3/4 稳定层，但动稳定层会破前缀缓存（整段换一次即可）；值不值得现在做。
- **备注**：注入总量未失控（中位 8KB、p90 42KB、max 443KB），所以优先级低。

## D 类 · review 收口（用户说现状「还好」，仅记录不推）

> 用户反馈：review 倒还好，有主线程判断要不要改。以下两条降级为 P2，先记录避免丢失。

### ISS-09 review「处置通道」三分 + 收口行

- **状态**：`待讨论（降级）`
- **实测事实**：全部 change 汇总 **987 条意见**（High 135 / Medium 391 / Low 461），来自 104 份 `review.md`；单 change 最多 50 条。Low 里大量是真 bug（`stack_alive` 不比 starttime 致 PID 复用误报、`parseFloatDefault` 放行 NaN、阶段 CAS 未查 RowsAffected），**不是噪声，是分类轴错**。
- **草案**：① 每条加「处置通道」三值（机械 / 代理 / 人类），归档门禁查「人类待办 ≤N」；② review 必须收口成一行结论（放行判断 + 一句话理由 + 需人拍板项 ≤5），把 987 条压成 104 行。

### ISS-10 review 人读摘要生成器

- **状态**：`待讨论（降级）`
- **做法**：脚本把 `review.md` 按处置通道折叠成「机械已修 N / 待你拍板 M」，人只看 M。

---

## 附录 · 决策依据数据（2026-11-26 从 `.pi/harness/events.db` + 仓库制品实测）

避免下轮重查，结论引用时注明来源。

| 指标 | 数值 | 来源 |
| --- | --- | --- |
| 约束注入体量（近 60 天） | 中位 8KB/session、p90 42KB、max 443KB | `constraint.inject` payload.bytes |
| 注入构成 Top3 | declaration 1456 次 × 中位 4144B（域红线层）、index 766 次 × 4215B、jit-path 868 次 × 3223B | 同上 |
| 子线程派发 | 669 次；brief 无红线摘要 287（43%）、有 262、未记录 120 | `subagent.dispatch` |
| 派发档位分布 | general-purpose 226、ui-design 71、change-review 68、explore 66、implementer 54（已退役） | 同上 |
| review 意见量 | 987 条（H135/M391/L461），104 份 review.md，9 份零条目 | `openspec/changes/**/review.md` |
| 用户反馈归因 | 96 条：产品 77 / 流程 18；frontend 44、fact-check 26、process 16、backend 8、test-gap 2 | `human-feedback.md` ×14 |
| 原型体积失控样本 | `add-home-pipeline-status-page` 530 文件 / 96MB | `ui-prototype/` |
| 门禁失败率 | golangci-lint 62.2%（439 次）、go vet 38.9%（316）、pnpm lint 24.9%（285）、go build 26.1%（264） | `gate.check` |
| 现有 harness 脚本 | 无「原型状态闭环」检查、无「规模预算」检查、无「大读记账」 | `scripts/harness/` 清单 |

---

## 讨论顺序（建议，可改）

1. **ISS-01** 决策制品图形化 —— 它是上游，决定 ISS-02/03 的形态
2. **ISS-02 + ISS-03** 原型定位与机械检查 —— 与 ISS-01 一起成型
3. **ISS-04 + ISS-05** 规模代价 —— 用户已认可方向，需细化判据
4. **ISS-06 + ISS-07 + ISS-08** 上下文路由 —— 判据需要先有数据，可与 3 并行
5. **ISS-09 + ISS-10** review 收口 —— 现状可接受，最后做
6. **ISS-14** flow 文档堆积 —— **排在 ISS-04 之后**（分族要对齐，否则改两遍）
