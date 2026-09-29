## Context

以 change 为生命周期回顾 harness 流程执行现状（探索结论存档：docs/research/change-lifecycle/explore-findings.md）：数据 80% 已在 events.db（session.rollup×change.archive 每任务成本、ui-design-gate 五种卡点 reasonCode、gate.check 失败聚类、pin 复用），retro 已有 `--change` 过滤但只过滤全局七段，无单 change 组织视图。手拼体检卡过程暴露三个数据坑：催修时距统一用 gate.check 转绿判解除对非 gate 裁决错配（实测产出 89 分钟假慢）、UI 审批通过零记录只能推断、无终态事件导致 40+ 活跃 change 仅 25 个有下文。2026-09-28 双面板趋势基线已产出（docs/research/change-lifecycle/retro-cost-trend-2026-09.html），关键 SQL 配方均已在探索中验证可行。

依赖关系：本 change 与 add-foreign-gate-steer-attribution 并发修改同一 Requirement（策略显著裁决统一记账），本 design 的 D4 固化归档顺序。

## Goals / Non-Goals

**Goals:**
- `--lifecycle <change>` 单命令输出体检卡四段（流程规范/UI 工作流/成本卡/健康度）+ proposal 声明元数据联合展示。
- 三个数据坑修复：解除信号按 policy 分派、UI 放行显式记账、change.abandon 终态登记。
- human-feedback 反馈建账制度化：格式契约 + feedback.sh 三件套 + 体检卡第五段计量（FB-4 转挂诉求）。

**Non-Goals:**
- 不动七段全局报告的既有段结构（--lifecycle 是并列新模式）。
- 不做自动终态判定（静默推断"多久无活动算放弃"引入假阳性，终态必须显式登记）。
- 不改 events.db schema（新事件 kind 沿用单表 JSON payload 模式）。
- 不做反馈采集的 extension 自动化（先规范+脚本两档起步，观察漏采程度再议）。
- 不算消化率/消化时滞等流程效率指标（用户拍板：归因是主线，状态仅用于归档对账）。

## Decisions

### D1 体检卡数据源：全部 SQL 已在探索中验证，只读单脚本聚合

四段查询沿用探索验证过的配方：终值 rollup 按 `(session_id, change) GROUP BY MAX(id)`；成本归因含 parentSessionId 归父（对齐 ⑦段 B' 口径）；卡点按 `policy='ui-design-gate'` 的 reasonCode 计数；催修时距分派见 D3。proposal 头注释解析：未归档读 `openspec/changes/<name>/proposal.md` 头三行 marker，已归档读 `openspec/changes/archive/` 目录（日期前缀剥离开后拼 proposal 路径）。只读约束：脚本全程 `sqlite3 file:...?mode=ro` 打开（对齐 harness-retro 既有只读模式），归档前后账本计数不变为验收断言。

### D2 UI 放行词汇：action 扩 `pass`，两个 reasonCode

`policy.decision` 的 action 枚举从 `block|warn|bypass|fail-open` 扩展加 `pass`。ui-design-gate 记两个放行场景：
- `ui-approval-approved`：用户在对话中明确批准原型（major 档 approval pending→approved 的时刻）。
- `ui-design-satisfied`：block（ui-design-missing / ui-prototype-missing / ui-impact-mismatch / ui-verification-missing）后补齐文档/修正声明，门禁放行的时刻。

*替代方案否决*：复用 action=warn 表达放行——语义错位（warn 是催促动作），且催修时距计算会把放行吸入④段 warn 聚类误报。扩 pass 一次性解决语义与统计吸入两个问题。词汇进 harness-facts SKILL.md 词汇表 + smoke 断言稳定集；harness-retro ③④段与催修时距对 pass 的处理：pass 不入 fail-open 分桶、不入失效 warn 聚类、作为 ui-design-gate 催修时距的解除信号。

### D3 催修时距解除信号分派表（retro ⑦段 A 组实现）

| policy | 解除信号 | 剔除口径 |
| --- | --- | --- |
| quality-gate | 同 `(session, cmd)` gate.check ok=true（转绿锚点） | 既有口径不变 |
| ui-design-gate | 该 change 下该 reasonCode 的首条 pass 事件 | 引入前历史无 pass → 按末 block 后无新同 reasonCode block 推断并标注；推断不出剔除 |
| quota-gate | 同 session 后续同 policy 的 pass/放行（换 provider 后成功派发） | 无后续 → 剔除并披露剔除数 |
| spec-gate | change.archive 成功事件 | 归档失败重试计入复发次数 |

*替代方案否决*：统一用"末次裁决后 N 分钟无复发"估时距——把统计口径从事件事实降级为猜测，且 quota 的 89 分钟假慢正是这类错配的实证。

### D4 归档顺序约束：steer-attribution 先行

两 change 的 harness-fact-log delta 都 MODIFIED「策略显著裁决统一记账」。本 change 的 delta 文本已按合并后状态书写（含归因降级词汇）。执行顺序：add-foreign-gate-steer-attribution 先归档 sync；本 change 归档时 openspec 冲突校验若报 delta 过期，按其归档后主 spec 重放本 delta 的差异部分（pass 枚举 + ui 放行场景 + 正常放行例外句）。tasks 里显式排此依赖。

### D5 change.abandon 登记脚本：对齐 test-patrol 直写先例

`scripts/harness/change-abandon.sh <change-name>`：校验参数 change 存在于 `openspec/changes/`（未归档）→ 直写 `change.abandon` 事件（kind=change.abandon，payload `{name}`，change 列同名，session_id 取 PI_SESSION_ID 或 `change-abandon.sh`，对齐 test-patrol.sh 的 patrol.check 直写模式）。已归档 change（archive 目录存在）拒绝登记（提示用既有归档事实）；参数不存在非零退出。

*替代方案否决*：harness-telemetry 拦截 bash 命令配对（change.archive 模式）——openspec CLI 无 abandon 原生命令可配对，为配对而发明伪命令形态复杂度更高；直写脚本先例已在 test-patrol 验证。

### D6 体检卡 UI 工作流段放行态双口径

引入前的历史 change 无 pass 事件：放行态按「末次 block 后该 reasonCode 无新 block」推断，输出标注「推断（无放行事件）」；引入后直接呈现 pass 事件时间。两口径并存于同一报告，逐 change 标注，不混算。

### D7 human-feedback 数据源=文件，格式契约三要素

数据源拍板：human-feedback.md 是人写给人的修订账本，不要求存储友好格式，不进 events.db——双写一致性无解（采集断链必失真）且账本有 TTL（rollup 90 天）而文件随 change 归档永久保存。计量消费方（--lifecycle 第五段）直接解析文件，路径规则同声明元数据解析（D1：未归档读 changes/、已归档读 archive/ 剥日期前缀）。

格式契约（参考 doc-impact.sh 白名单模式：选择题+封闭词表）：
- 条目标题行 `### FB-<N> <日期>【<类型>|<归因>】<标题>`——一行正则全解析，标签放条目级而非文件头（doc-impact 是文件头标签，因它是整文档一个域；human-feedback 一个文件多条、每条归因不同）。
- 状态行五值封闭：待处理/处理中/已消化/已转挂/已放弃。
- 归因五类封闭：frontend/backend/test-gap/fact-check/process，语义与规范整理去向映射：frontend→standard/frontend/、backend→standard/backend/+flow/、test-gap→test-design.md、fact-check→探索纪律、process→harness/执行规范。同一问题多因按根因优先归 fact-check（FB-8 既未核实又是测试盲区→fact-check，根因在前）。

存量补标：ai-provider-model-overhaul 10 条手工补归因（实测分布 frontend×5 / fact-check×3 / backend×1 / process×1），状态词六种写法归一到五值、条目状态与处置表双源不一致处以处置记录为准修正（FB-1 条目仍「待处理」但处置表已完成）。存量小（10 条）一次补齐，不留推断口径（与 D6 双口径相反的决策——此处样本量支持补齐）。

*替代方案否决*：事件侧记账（feedback.record 事件）——采集靠 agent 自觉双写必断链、TTL 与归档持久性矛盾；复用【类型】字段不设归因——类型（改什么）与归因（哪层的责）正交（FB-4 是流程类型+process 归因），合并丢维度。

### D8 feedback.sh 三件套与第五段：对齐 doc-impact 脚本模式

- `menu`：打印归因词表+说明（把"归因是什么"从回忆题变选择题）。
- `add <change> --layer <归因> --type <产品|流程> "<要点>"`：定位 change 目录（已归档拒绝建账，提示走归档后修订）；文件不存在按模板创建（头部含定位/消费方/格式说明，对齐 ai-provider-model-overhaul 首例文件结构）；编号取现有最大 FB-N 自增；状态初始化待处理；词表外值非零退出零写入。
- `verify <change-dir>`：待处理/处理中存在→处置提醒+退出码 1（语义对齐 doc-impact verify）；全部处置或无文件→0。挂接点：archive-readiness.sh 顺带调用透传结果，不进 spec-gate 硬门禁（轻量起步，与 test-patrol 登记、doc-impact verify 同层的脚本级对账）。
- 体检卡第五段：条数（产品/流程）、归因分布、未处置数；文件缺席→「无反馈记录（或未启用）」降级标注；单条解析失败计「格式异常」单列披露不炸整段（宽容降级，手改漂移不致报告失效）。

采集完整性两档起步：①规范清单（doc-authoring + 执行规范 §0.6 制品清单，add 命令降低记录成本）；②观察期——若后续体检卡发现高发漏采（如多轮返工但零反馈记录的 change），再议 extension 钩子，本期不做。

## Risks / Trade-offs

- [action 枚举扩 pass 引发未知消费方假设破坏] → 全库消费方盘点已完成（harness-retro③④段、催修时距、harness-facts 词汇表、gate-status-query 不消费 policy.decision 的 action），smoke 断言稳定集同步扩；retro 消费侧按 D2 显式排除 pass。
- [abandon 登记滥用导致终态污染] → 脚本校验 change 存在且未归档；archive 优先冲突规则（spec 已定）兜底误操作。
- [体检卡 SQL 随事件口径漂移（如 ownership 字段引入）] → 查询配方集中在脚本单文件头部注释区，口径变更走 change 流程（与 harness-retro 同纪律）。
- [采集完整性靠自觉，系统性漏采低估计量] → 起步规范+脚本两档（D8），观察期后评估 extension 钩子；第五段对高返工零反馈的 change 天然暴露漏采信号（返工波次×反馈条数交叉）。
- [human-feedback.md 手改漂移（词表外值/缺状态行）] → 解析器宽容降级：单条计「格式异常」单列披露不炸整段；verify 输出待修正提示。

## Migration Plan

1. ui-design-gate pass 记账 + 词汇表/smoke 同步（行为变化点，独立提交）。
2. change-abandon.sh 脚本 + 只读校验（纯新增）。
3. retro --lifecycle 模式（纯新增只读消费）。
4. 催修时距分派改造（⑦段 A 组，独立提交）。
5. feedback.sh 三件套 + 存量补标（纯新增 + 存量文件一次性修正，独立提交）。
6. 第五段计量 + 制品清单制度化（文档侧）。
回滚：各步独立 revert；pass 事件为 append-only 新增，无需数据回滚；abandon 脚本删除即回滚；feedback.sh 删除即回滚（human-feedback.md 是 change 制品文件，不受脚本存废影响）。

## Open Questions

- 体检卡输出格式（纯文本段 vs markdown 表格）——实现时取 retro 七段现有风格（文本+缩进表），不足虑。
- 观察期后是否需要反馈采集 extension 钩子（D8 观察条件：返工波次×反馈条数交叉暴露的漏采率）——归后续 change。
