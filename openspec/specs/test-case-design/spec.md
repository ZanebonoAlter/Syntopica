# test-case-design Specification

## Purpose

把测试用例的「设计层」规范从零散经验升级为单一权威源：定义 Scenario 最低集、边界枚举、可用性用例、验收措辞的标准，并保证这些规范在 agent 写 specs / tasks / 测试代码的时刻被自动注入（知道），归档时对措辞违例 warn 留痕（兜底提醒）。

## Requirements

### Requirement: 用例设计标准权威源

仓库 SHALL 维护 `docs/reference/standard/shared/test-design.md` 作为用例设计标准的唯一权威源（跨前后端），采用**故事锚点单元模型 + 四问句**结构：

- **单元模型**：测试单元 SHALL 为用户故事，载体为 spec 的一个 Requirement（能力切片）——但 Scenario 是断言片段，SHALL NOT 直接充当完整故事；涉及行为的 change SHALL 产出测试用例文档（change 目录 `test-cases.md`，纯文档 / 工具链 change 豁免）把 Scenario 节拍串成完整故事（主链路表：步 / 动作 / 来源 Scenario / 期望 / 层 / 落点 + 变体走查 + 效果核对 + 复杂档白盒附加）。双轨原则：方法级单测允许且鼓励（纯函数快反馈），但交付账本 SHALL 在用例文档的故事落点。
- **问句①节拍完备**：每个 Scenario SHALL 有测试落点，无自动化测试的显式映射「人工」留痕（沿 scenario-trace 既有约定）。
- **问句②变体走查**：五组固定条目清单——输入（空串/纯空白/纯分隔符/单token/大小写/特殊字符/超长）、前置数据状态（空集/单元素/重复/越界引用/部分满足）、时间窗口（边界两端含当天归属/空窗口/跨窗口/归一化）、幂等重复（重复执行/部分失败重试/并发仅当声称线程安全）、可用性（误输入反馈/空态/错误态/加载态/超长文本/重复提交）；不适用维度 SHALL 划除留痕。
- **问句③层选择**：每个测试落点 SHALL 选「能讲完这个故事的最便宜层」：纯逻辑→函数单测；涉 SQL/迁移/约束→testcontainer PG；HTTP 契约→handler 测试；组件行为→组件测试；**前端交互故事的完整主链路→opencli 端到端**（涉及 UI 交互流程的 change SHALL 至少一个 opencli 落点，或「人工」留痕豁免）。
- **问句④效果核对**：故事效果依赖测试断言之外因素（数据覆盖率 / LLM 实际行为 / 外部服务）时，SHALL 做真实库量化核对（触发原因/核对方法/量化结果/结论四要素）。
- **验收措辞规范**：验收 SHALL 可机械判定（命令+期望 / 文件存在性 / 「人工」显式留痕），含禁用词表（spec-gate 检查⑤词表的权威源）。
- **白盒附加**：复杂档（状态机/算法/多模块协议）在用例文档的白盒附加节补分支表/边界值清单/不适用划除留痕（沿 case-first-testing 既有要求，断言判据主线程定）——附加件非主角。

文档正文 SHALL 含「JIT 注入摘要」节：上述结构的速查版，体积 SHALL ≤ 3KB；文档头部 SHALL 带 `doc-impact-applies` JIT 标签（见下一条 Requirement）。

#### Scenario: 权威源结构齐全

- **WHEN** 任何人需要查「测什么 / 测到什么程度 / 验收怎么写」类规范
- **THEN** `docs/reference/standard/shared/test-design.md` SHALL 是唯一权威源，含单元模型 + 四问句 + 验收措辞规范 + 白盒补充件 + 「JIT 注入摘要」节

#### Scenario: 故事锚点与对账同锚

- **WHEN** agent 为某 Requirement 设计测试落点
- **THEN** 单元 SHALL 为该 Requirement 的完整故事（由 test-cases.md 串节拍），机器对账仍锚 tasks 验证节 Scenario→测试文件映射表，两者指向同一批落点

#### Scenario: 用例文档串故事

- **GIVEN** 涉及行为的 change，其 spec 含多个 Scenario 断言片段
- **WHEN** 编写测试用例文档
- **THEN** test-cases.md SHALL 把节拍串成主链路（每步引用来源 Scenario），变体走查与层选择落点显式成表，跨节拍的完整用户旅程可从文档读出

#### Scenario: 前端交互故事 opencli 落点

- **GIVEN** change 涉及前端 UI 交互流程（多步导航/表单提交/状态切换）
- **WHEN** 设计测试落点
- **THEN** 主用户故事 SHALL 至少有一个 opencli 端到端落点（驱动真实浏览器走完整主链路），或以「人工」留痕豁免

#### Scenario: 摘要节体积受控

- **WHEN** 维护者扩充「JIT 注入摘要」节
- **THEN** 该节 SHALL ≤ 3KB，注入不挤占既有 keyword 层约束（超线走 constraint-injection 既有分层降级，不因本文档破坏既有注入秩序）

#### Scenario: 与既有规范的关系

- **WHEN** test-design.md 与 `standard/*/testing.md`（怎么跑）/ `case-first-testing`（Scenario 即用例）内容交叠
- **THEN** test-design.md SHALL 只管「测什么 / 测到什么程度 / 措辞可判定」，SHALL NOT 复写「怎么跑」（框架 / 分层运行 / DSN 红线仍在 testing.md），SHALL 引用而非重复

### Requirement: 设计规范 JIT 注入

`shared/test-design.md` 的「JIT 注入摘要」节 SHALL 在 agent 编辑以下路径时被 constraint-injection 既有 JIT 机制节级注入（写/编辑路径命中 `doc-impact-applies` 信号的下一回合起，会话内粘性）：`openspec/changes/`（写 proposal/specs/tasks/design 等全部 change 制品）、`_test.go`（后端测试代码）、`.spec.ts` / `.test.ts`（前端测试代码）。本 Requirement SHALL NOT 要求修改 constraint-injection extension 代码——JIT 单一真相源是文档头部标签。

#### Scenario: 编辑 specs 时注入

- **WHEN** agent 编辑 `openspec/changes/<name>/specs/**/*.md` 写 Scenario
- **THEN** 下一回合起 system prompt SHALL 含「JIT 注入摘要」节（jit-path 命中，会话内粘性）

#### Scenario: 编辑测试代码时注入

- **WHEN** agent 编辑 `backend-go/**/*_test.go` 或 `front/**/*.{spec,test}.ts`
- **THEN** 下一回合起 SHALL 注入同一摘要节

#### Scenario: research 语境不注入

- **WHEN** 会话仅编辑 `docs/research/**`（无档位、非 change/测试路径）
- **THEN** SHALL NOT 注入摘要（信号不含该路径）

#### Scenario: 边界——注入预算超线降级

- **WHEN** 摘要注入后总注入量接近预算上限
- **THEN** SHALL 按 constraint-injection 既有分层降级规则处理（keyword 优先于 jit），SHALL NOT 为本文档新增降级规则

### Requirement: 归档措辞 warn 检查（spec-gate 检查⑤）

`openspec archive` 归档门禁 SHALL 新增第五项检查（**warn 级，SHALL NOT block 归档**），对 `<changeDir>/tasks.md` 扫描两类措辞违例，命中时以 `spec-gate-warning` custom_message 留痕（display: true）：

- **⑤a 白盒用例文档缺失提醒**：tasks.md 任务描述文本命中复杂档关键词（算法 / 状态机 / 解析 / 协议等）且 change 目录不存在白盒用例文档（`test-cases*.md` 或 `*-test-cases.md`）→ warn「case-first-testing 要求复杂档产出白盒用例文档（分支表 / 边界值清单），当前 change 目录未检出；确非复杂档可忽略」
- **⑤b 分层错配措辞**：任务描述含「纯函数」且其验收措辞含「SQLite」→ warn「纯函数用例按 testing.md 分层应落 `*_unit_test.go` 无 DB；若任务确需 DB 请修正任务描述」

扫描 SHALL 抽成无副作用的纯函数（输入 tasks.md 文本与 change 目录文件列表，输出违例列表）供冒烟测试；违例列表与警告文案 SHALL NOT 影响检查①-④ 的既有 block 语义；检查⑤自身异常 SHALL fail-open（沿用本扩展既有异常策略，console.warn + 留痕，不阻断归档）。

禁用词表与关键词表以 `shared/test-design.md` 为权威源，spec-gate 内置同表常量并注明同步义务（表内容稳定，双源漂移风险可忽略）。

#### Scenario: 无违例静默放行

- **WHEN** 归档时 tasks.md 无措辞违例
- **THEN** 检查⑤ SHALL 不产生任何 warning，归档流程与现状一致

#### Scenario: 白盒用例缺失 warn 留痕

- **GIVEN** change 的 tasks.md 任务描述含「解析」关键词，change 目录无 `test-cases*.md`
- **WHEN** `openspec archive` 触发归档门禁
- **THEN** 归档 SHALL 放行（不被 block），且 SHALL 落一条含修复指引的 warning 留痕

#### Scenario: 纯函数任务提 SQLite warn 留痕

- **GIVEN** tasks.md 某任务描述含「纯函数」且验收含「SQLite 单测」
- **WHEN** 归档门禁执行
- **THEN** 归档 SHALL 放行，SHALL 落分层错配 warning 留痕

#### Scenario: 检查⑤异常不阻断归档

- **WHEN** 措辞扫描自身抛异常（如 tasks.md 编码异常）
- **THEN** 检查⑤ SHALL fail-open（console.warn + 留痕），归档 SHALL 按检查①-④ 的结果正常裁决

#### Scenario: 豁免通道兼容

- **WHEN** 命令带 `--force` 或 `SPEC_GATE_BYPASS=1`
- **THEN** 检查⑤ 随既有豁免通道放行，SHALL NOT 追加任何 block

### Requirement: 前后端标准挂接与索引

`standard/backend/testing.md` 与 `standard/frontend/testing.md` SHALL 各含「用例设计」小节：引用 `shared/test-design.md` 为权威源，并补各自端的用例分层判据（后端：纯函数 → `*_unit_test.go` 无 DB / repository 与迁移 → testcontainer PG / handler → 轻量；前端：纯函数 → 单测 / 组件行为 → Vitest 组件测试 / 流程 → opencli）。`docs/reference/constraints-index.md` 执行规范表 SHALL 新增 test-design 行。挂接 SHALL NOT 改动两份 testing.md 的既有权威内容（怎么跑 / DSN 红线 / 禁 SQLite 等原样保留）。

#### Scenario: 后端判据可查

- **WHEN** agent 写后端测试前查分层判据
- **THEN** `standard/backend/testing.md` SHALL 含「用例设计」节并引用 shared 权威源

#### Scenario: 索引可见

- **WHEN** 未激活档位时 agent 看 constraints-index 常驻索引
- **THEN** 执行规范表 SHALL 含 test-design 行（写 specs/tasks/测试代码前可发现权威源）

#### Scenario: 既有权威内容不变

- **WHEN** 挂接完成后比对两份 testing.md 的既有章节
- **THEN** DSN 红线 / repository 禁 SQLite / 运行门禁等既有内容 SHALL 原样保留，仅新增引用节

### Requirement: 回归走查与旧资产可见性

涉及 MODIFIED / REMOVED Requirements 的 change（改契约）SHALL 在其 test-cases.md 含「继承与调整」表：每行 = 旧 Scenario × 处置（继承照跑 / 改语义改断言 / 废止删除留痕）× 旧测试文件 × 动作；验收锚点为表逐行有处置（跑绿 ≠ 对，旧测试可能仍在断言旧契约）。纯新增 capability 的 change SHALL 豁免（无旧节拍可继承，不报缺失）。

仓库 SHALL 提供 `scripts/harness/test-assets.sh <capability>` 反向索引（只读只判不猜）：①主 specs 该 capability 的 Requirement/Scenario 现状清单；②archive 中含该 capability delta 且含 test-cases*.md 的历史 change；③这些 change 的 tasks.md 验证节 Scenario→测试文件映射重建。查询不存在的 capability SHALL 退出码非 0 且如实提示（不猜测）。

「JIT 注入摘要」节 SHALL 含回归走查提醒（触发条件 + 继承表 + test-assets.sh 指引）。

#### Scenario: 改契约必答继承表

- **GIVEN** change 的 delta specs 含 MODIFIED Requirements
- **WHEN** 编写 test-cases.md
- **THEN** SHALL 含「继承与调整」表，每个受影响旧 Scenario 一行且处置/动作可判定

#### Scenario: 纯新增豁免

- **WHEN** change 仅含 ADDED Requirements（纯新增 capability）
- **THEN** SHALL 无需继承表，且不因缺失被报违例

#### Scenario: 反向索引三段输出

- **WHEN** 对存在历史的主 capability 执行 `bash scripts/harness/test-assets.sh <capability>`
- **THEN** SHALL 输出主 specs 现状节拍 + archive 命中 change 清单 + 历史映射重建三段，退出码 0

#### Scenario: 查询不存在即报错

- **WHEN** 对主 specs 与 archive 均不存在的 capability 查询
- **THEN** SHALL 退出码非 0 且中文提示无匹配（不猜测、不出空段充数）

#### Scenario: 摘要含回归提醒

- **WHEN** agent 写 specs 时被 JIT 注入摘要
- **THEN** 摘要 SHALL 含问句⓪触发条件与 test-assets.sh 指引一句
