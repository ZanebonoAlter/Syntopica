# test-cases: stage-driven-constraint-injection

> 复杂度 complex（proposal.md 头部声明）。测试单元 = 「change 走到哪一步，模型就知道该读哪几篇约束文档、并且被查得出读没读过」用户故事。主链路表串节拍，变体走查 + 效果核对 + 白盒附加见后。
> 扩展层落点在 `.pi/extensions/tests/constraint-injection.smoke.cjs`（stub typebox + esbuild 打包扩展 + 真实事件形状回放）；事实库落点在 `.pi/extensions/tests/harness-log.smoke.cjs`；归档门禁落点在 `.pi/extensions/tests/spec-gate.smoke.cjs`；shell 落点见步 12-14。

## 故事 S1: 阶段标识驱动约束路由（锚 Requirement: stage 进度标识（替代档位机制） / 阶段驱动的约束路由与自主导入）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试文件或人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | `stage.md` 头部写 `<!-- stage: implement -->`，扩展解析 | stage marker 生效 | 解析为 ok，stage=implement，阶段族=implementation | 扩展 smoke | `constraint-injection.smoke.cjs`（stage-parse 组） |
| 2 | `stage.md` 不存在 | 缺失 marker 视为 legacy 不阻断 | 解析为 missing，行为与改造前一致，不阻断 | 扩展 smoke | 同上（stage-parse 组） |
| 3 | `stage.md` 写 `<!-- stage: coding -->`（非法值） | 非法 stage 值降级不阻断 | 解析为 invalid，降级不阻断，记 warn 事件 | 扩展 smoke | 同上（stage-parse 组） |
| 4 | `stage.md` 写两条 `<!-- stage: -->` | marker 重复视为非法 | 解析为 invalid，MUST NOT 取其中任一条生效 | 扩展 smoke | 同上（stage-parse 组） |
| 5 | 事实库仅有旧 `mode.set`（mode=requirements），查询阶段族 | 事实库双读兼容旧 mode.set | 返回 `planning`，不报错 | 事实库 smoke | `harness-log.smoke.cjs`（stage-set 双读组） |
| 6 | 阶段从 `propose` 切到 `implement`，声明域 `daily-report` | 阶段首推红线层与清单 | 一条增量 steer 送达 daily-report 红线层 + implement 阶段路由清单；**不含整节正文** | 扩展 smoke | `constraint-injection.smoke.cjs`（stage-routing 组） |
| 7 | 连续 5 回合同 stage、声明域不变 | 同阶段不重复注入 | 只在首回合注入 1 次，后 4 回合零注入 | 扩展 smoke | 同上（stage-routing 组） |
| 8 | 检查 implement 阶段路由清单内容 | 路由清单含取数命令 | 含 daily-report 路径与节名 + `constraint-digest.sh --brief --domains daily-report` 命令 | 扩展 smoke | 同上（stage-routing 组） |
| 9 | 用户输入含「discovery」关键词 | 不再注入整节正文 | **零正文注入**（改造前会误拉 ~8KB discovery.md 全节） | 扩展 smoke | 同上（keyword-retired 组） |
| 10 | agent edit 命中某 flow 文档 `doc-impact-applies` 标签的路径 | 编辑路径命中不再注入正文 | **零正文注入**；该域出现在本阶段路由清单中 | 扩展 smoke | 同上（keyword-retired 组） |
| 11 | 稳定层在 explore→propose→design 间切换 | 快照键用阶段族 | `stableSnapshotKey` 不变，稳定层不重建 | 扩展 smoke | 同上（snapshot-key 组） |
| 12 | 稳定层与动态层全文扫描 | 稳定层档位生命周期内恒定 / 稳定层不含约束正文 | 注入文本中不含任何业务域约束节正文 | 扩展 smoke | 同上（no-body 组） |
| 13 | 阶段/绑定切换后的动态层 diff | 动态层事件驱动增量发送 / turn 中途切档后同 turn 已投递内容不被延迟重建重投 | 指纹 regime 与新快照 key 一致时零重发 | 扩展 smoke | 同上（diff-fingerprint 组） |
| 14 | 父会话绑定 change X 后派发子线程（跨进程） | 子线程显式继承父会话 | 子线程 `stage.set`（source=inherit）带 boundChange=X，注入带 X 的路由清单 | 扩展 smoke | 同上（inherit 组） |
| 15 | 子线程跨进程继承时事实库只有旧 `mode.set` | 旧 mode.set 行仍可读 | 子线程仍能恢复阶段族（双读兜底不断链） | 事实库 smoke | `harness-log.smoke.cjs`（inherit 双读组） |

## 故事 S2: 读取记账与归档对账（锚 Requirement: 约束文档读取记账与归档对账）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试文件或人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | agent `read` `docs/reference/flow/daily-report.md` | constraint.read 事件入库 | 事实库一条 `constraint.read`，payload 含该路径 | 事实库 smoke | `harness-log.smoke.cjs`（constraint-read 组） |
| 2 | agent `read` 普通源码文件 | （记账范围约束） | **零** `constraint.read`（白名单外不记，控事实库体量） | 事实库 smoke | 同上 |
| 3 | 某 change 声明域 `daily-report`，事实库已有该文档读取记录 | 读过则对账通过 | `constraint-read-trace.sh` 退出码 0 | shell | `scripts/harness/constraint-read-trace.sh` + tasks 8.2 |
| 4 | 某 change 声明域 `scheduler`，事实库无该文档读取记录 | 未读过则红并给取数指引 | 退出码 1，输出含域名、路径、`constraint-digest.sh --brief --domains scheduler` | shell | 同上 |
| 5 | 某 change 无 `stage.md`（legacy） | legacy change 豁免对账 | 退出码 0 且明示豁免 | shell | 同上 |
| 6 | 对账报红时执行 `openspec archive` | 归档门禁接入对账 | 归档被 block，reason 含未读域清单 | 扩展 smoke | `spec-gate.smoke.cjs`（archive 对账组） |
| 7 | 仓库现有 11 个活跃 change（均无 `stage.md`）逐个跑对账 | legacy change 豁免对账 | 全部退出码 0，无 FAIL | shell | tasks 8.3 |

## 继承与调整（⓪ 契约盘点）

`bash scripts/harness/test-assets.sh constraint-injection` 反查结果：主 spec 8 Requirements / 88 Scenarios；archive 历史 12 个相关 change（其中 4 个含 test-cases*.md）；历史 Scenario→测试文件映射全部指向 `.pi/extensions/tests/*.smoke.cjs`。本 change 含 MODIFIED + REMOVED Requirements，逐条处置如下。

| 旧 Scenario / 旧断言 | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| Requirement「档位识别与 change 绑定」全 19 Scenario | **REMOVED**（档位机制退役，契约迁到 stage） | `.pi/extensions/tests/constraint-injection.smoke.cjs` 的 mode 激活组 | **废止**：改为 stage 组（步 1-4、15）；「斜杠命令激活档位」「skill 读取激活档位」两条断言删除——命令/skill 不再是档位激活源；「reload 后档位恢复」「resume 无法恢复回落未激活」「apply 中断后 resume 恢复档位」三条**改造为 stage 恢复语义**后保留（回归网） |
| Requirement「无档位编辑路径 JIT 追加注入」全 11 Scenario | **REMOVED**（整节正文注入退役） | 同上 `unbound-jit-*` 组 | **反转断言**：步 9-10——原「编辑命中业务路径 → 送达完整节」改为「编辑命中业务路径 → **零正文注入 + 出现在路由清单**」；「未激活档 JIT 不绑定 change」一条**继承**（改为「编辑命中 → 仍不绑定 change」） |
| Scenario「未激活档仅索引」 | 继承 | 同上 | 保留断言（未绑定会话仍只注索引）；措辞由「未激活档」改为「未绑定」 |
| Scenario「实现档注入生效」 | 继承（语义调整） | 同上 | 保留断言，档位值改为 stage=implement |
| Scenario「JIT 路径细化」 | **反转** | 同上 | 原「编辑命中 → 注入该节」改为「编辑命中 → 加入路由清单」（步 10） |
| Scenario「声明域内关键词正常命中」 | **废止** | 同上 | 关键词通道整体退役（步 9）；改为断言「零正文注入」 |
| Scenario「关键词域限定」「change 文本不触发关键词命中」「ASCII 关键词词边界整词匹配」 | 废止 | 同上 | 关键词命中源整体退役，无对应新契约 |
| Scenario「稳定层档位生命周期内恒定」「档位切换一次性变化」 | **继承**（场景名不改，正文语义更新） | 同上 | 保留断言；步 11-12 补充「稳定层不含约束正文」「快照键用阶段族」 |
| Scenario「动态层事件驱动增量发送」 | 继承（语义调整） | 同上 | 原「第 3 turn 命中关键词发全节」改为「阶段首推发一次红线层 + 清单，同阶段零重发」（步 6-7） |
| Scenario「turn 中途切档后同 turn 已投递内容不被延迟重建重投」 | 继承（语义调整） | 同上 | 保留断言，档位切换改阶段切换（步 13） |
| Scenario「fork 继承的指纹不被陈旧快照 key 重投」 | 继承（语义调整） | 同上 | 保留断言 |
| Scenario「命中只增不减（缓存稳定）」 | 继承 | 同上 | 保留断言（改由 change-file 与路由清单承载） |
| Scenario「超预算分层降级」「降级永不真丢」「模型可见省略通知」「降级确定性」「降级记账」 | 继承（降级顺位调整） | 同上 | 保留断言；降级顺位由「关键词→JIT→…」改为「红线层→路由清单→change-file」（正文仍不真丢） |
| Scenario「子线程显式继承父会话」×2 | 继承（机制改名） | 同上 `inherit` 组 | 保留断言；`mode.set source=inherit` 改 `stage.set`（步 14） |
| Scenario「无自身绑定且无父子关系时不借用他会话」 | 继承 | 同上 | 保留断言；「档位基础块」改「路由清单」 |
| Scenario「命中集按会话隔离」 | **改语义** | 同上 | 原对比对象是「JIT/关键词命中集」，现改为「阶段 + 绑定」（步 12-13） |
| Scenario「红线层提取纯函数直跑」「红线层零提取回退」「节低于最小字节下限回退全文」「节提取回落」 | 继承 | 同上 | **全跑**（红线层是本次注入形态的核心载体，不得回归） |
| Scenario「抢绑条件纯函数直跑」「动态层 diff 指纹纯函数直跑」「smoke test 直跑」 | 继承 | 同上 | 全跑 |
| Scenario「实现阶段自动注入发现」「research 语境不落 change」「research 语境无 topic 落通用池」 | 继承（措辞调整） | 同上 | 保留断言，「档位」措辞改「阶段」 |
| harness-fact-log「档位记账（mode.set）」全 5 Scenario | **改名继承** | `.pi/extensions/tests/harness-log.smoke.cjs` | 断言由 `mode.set` 改 `stage.set`（payload `stage`/`stageFamily`）；「TTL 与既有事件兼容」保留并补「旧 `mode.set` 行仍在且可读」（步 5） |
| harness-fact-log「事件类型词汇与保留期」全 10 Scenario | 继承 + 扩展 | 同上 | 全跑；新增 `constraint.read` / `stage.set` 入库断言（步 1-2） |
| doc-impact.sh `--change` 取数源（`mode.set`） | 继承（双读） | `scripts/harness/doc-impact.smoke.sh` | 保留断言并补双读回退 |
| harness-retro.sh 档位统计口径 | 继承（改阶段统计） | `scripts/harness/harness-retro.smoke.sh` | 保留断言（字段名改 stage/stageFamily），历史行计「未知」 |

## 变体走查（五组）

- **输入**：stage marker 值大小写（`Implement` / `IMPLEMENT` → invalid，非大小写不敏感）、marker 前后多余空格、marker 嵌在正文中间（不在头部）、`stage.md` 为空文件、`stage.md` 含 BOM、marker 值带尾随逗号（`implement,` → invalid）、两个 marker 之间有空行 → 全部断言落入 {missing, invalid, ok} 三态之一，无未捕获异常。
- **前置**：`stage.md` 存在但 change 目录已归档（目录消失）→ 回落未绑定；`stage.md` 存在但 `proposal.md` 无 `constraint-domains` 声明 → 零域红线注入、清单仍给索引；声明域含未知域名（宽容忽略，沿用既有语义）。
- **时间窗口**：本 change 不涉业务时段口径，划除；替代口径为「实现期内读过」——读取事件早于 change 创建时间不计入（判据：读取事件时间戳落在该 change 的生命周期窗内）。
- **幂等**：同一文档连续 read 3 次 → 只记 1 条 `constraint.read`（避免事实库膨胀）；同一阶段连续 5 回合 → 只注入 1 次；阶段来回切换 `propose`→`implement`→`propose` → 每次切换各推一次（回到旧阶段不抑制）。
- **可用性**：无 UI（`ui-impact: none`），划除；改为断言 steer 消息头部受控（折叠形态、含阶段名与路由清单路径）。

## 效果核对（问句④：效果依赖断言外因素）

- 触发原因：本次改造的核心效果是**注入体量下降**与**不再跨域误伤**，两者都不是断言外因素，而是可量化的注入行为本身；但「模型是否真的去读路由清单里的文档」依赖模型行为，属断言外因素 → 需量化核对。
- 方法：
  1. 注入体量：改造前后各跑一次 smoke 的注入体量统计，比「单条注入平均字节 / 最大字节」；改造前基线取事实库 2862 条 `constraint.inject`（jit-path 最大 27 831、keyword 最大 23 103）。
  2. 跨域误伤：构造输入「harness 机制怎么实现」（含 "discovery" 词），比改造前后的注入域集合——改造前命中 discovery 全节，改造后应为零正文注入。
  3. 读取覆盖：跑完一个真实 change 的实现期后，查 `constraint.read` 事件覆盖率 = 已读文档数 / 路由清单文档数；归档对账脚本的通过率即为该指标。
- 量化结果（目标）：单条注入最大字节从 27 831 降至 ≤ 4 000（红线层 ≤ 3 105 + 清单余量）；误伤探针的注入域集合从 {discovery} 降为 {}；真实 change 的读取覆盖率 ≥ 声明域数（归档对账通过即达标）。
- 结论写入归档前 verify 报告（本文件不预填结果，避免自证）。

## 白盒附加（复杂档：分支表 + 边界值）

| 分支 | 边界值 | 用例落点 |
| --- | --- | --- |
| stage marker 解析 | missing / invalid（重复）/ invalid（越界值）/ invalid（大小写错）/ ok（6 档各一） | `constraint-injection.smoke.cjs` stage-parse 组 |
| 阶段族派生 | explore/propose/design→planning；implement/verify/archive→implementation；非法 stage→null | 同上 |
| 快照键 | 同族切换（值不变）/ 跨族切换（值变）/ boundChange 变（值变） | 同上 snapshot-key 组 |
| 路由清单装配 | planning 族 / design 族 / implement 族（backend 栈）/ implement 族（frontend 栈）/ 无栈命中 / 声明域为空 | 同上 stage-routing 组 |
| 红线层回退 | 提取 0 条 / 低于 `minSectionBytes`（512）/ 正常（daily-report 55 条 / semantic-board 75 条上界） | 同上 redline 组（既有断言继承） |
| 读取路径归一化 | 相对路径 / 绝对路径 / Windows 反斜杠 / 大小写差异 / 路径含 `..` | 同上 read-path 组 |
| 事实库双读 | 有 `stage.set` / 仅有 `mode.set` / 两者皆无 / `mode.set` 在 `stage.set` 之后 | `harness-log.smoke.cjs` stage-set 组 |

**不适用划除**：无状态机（stage 是单调阶梯非状态机）、无并发写冲突（事实库追加式）、无算法复杂度分界。
