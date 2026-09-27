# Test Cases — fix-injection-transition-fingerprint-wipe

> 单元 = 一个 Requirement 的用户故事。主线故事：**agent 在一个 turn 中途读了 explore skill（档位从未激活变 requirements），随后同 turn 编辑命中 test-design 的 doc-impact-applies 标签，该约束节经 steer 消息送达一次；下一个 turn 开始时稳定层快照按新档位重建，但这条已经送达的约束节不再来第二条——除非档位/绑定真的换了语境（regime 变化）才全量重投。**

## 主链路（节拍表）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 未激活会话跑一 turn（`before_agent_start`） | 未激活档仅索引 | 建快照 key=`none\|none`，稳定层含索引，零动态投递 | 行为 smoke | `.pi/extensions/tests/constraint-injection.smoke.cjs` |
| 2 | 同 turn：agent read requirements skill 文件（`tool_execution_start`） | skill 读取激活档位 | mode=requirements、记 `mode.set source=skill`；快照不重建（turn 中途无重建点） | 行为 smoke | 同上 |
| 3 | 同 turn：agent edit 命中 fixture 文档 doc-impact-applies 标签 | JIT 路径细化 | 1 条 steer 消息送达该节（JIT 即时投递），指纹写入且 regime=`requirements\|none`；markMessages 消费 | 行为 smoke | 同上 |
| 4 | 下一 turn `before_agent_start` | **turn 中途切档后同 turn 已投递内容不被延迟重建重投**（新增） | 稳定层重建（system prompt 含新档位 header/mode-base）；`dynamicText()` 为空——**第二次投递零发生** | 行为 smoke | 同上 |
| 5 | 接 4：agent edit `openspec/changes/<fixture>/`（edit-dir 兜底绑定） | 无绑定时写 change 目录兜底绑定 | 绑定切换、记 `mode.set source=edit-dir` | 行为 smoke | 同上 |
| 6 | 再下一 turn `before_agent_start` | **绑定切换后首轮仍全量重投**（新增） | 指纹 regime≠新 key → 清空 → 动态层全量重投（含步骤 3 已投过的节）——反向护栏，防修复过宽 | 行为 smoke | 同上 |

## 继承与调整（改契约的旧资产反查）

`bash scripts/test-assets.sh constraint-injection` 反查：主 specs 5 Requirements / 63 Scenarios；archive 含 constraint-injection delta 的 change 8 个，其中 `2026-09-16-harden-constraint-injection-channel`（混合通道引入者，含 test-cases）与 `2026-09-17-per-session-constraint-binding`（隔离不变式，含 test-cases）与本 change 语义最近。本 change MODIFIED「混合注入通道」一个 Requirement（正文新增「指纹重置按 regime 判定」段 + 3 个新 Scenario，其余 Scenario 原样保留）：

| 既有资产 | 处置 | 理由 |
| --- | --- | --- |
| smoke：动态层事件驱动增量发送 / 命中只增不减（缓存稳定） | 跑（回归） | 稳态零投递语义不变，且是本修复的目标行为 |
| smoke：档位切换一次性变化 / 稳定层档位生命周期内恒定 | 跑（回归） | 稳定层重建与字节恒定语义不变（修复不动 isTransition 的快照重建） |
| smoke：compact 后快照重发 | 跑（回归） | force 路径不看 regime，行为不变 |
| smoke：子线程显式继承父会话 / 他会话绑定变化不刷新本会话稳定层（隔离族） | 跑（回归） | per-session 隔离不变式不得破坏（头注释 160-166）；`inheritFromParent` 新增拷贝一个 string 字段，不改继承判定 |
| smoke：JIT 路径细化 / 节级注入 / 红线层三态 / 预算降级族 | 跑（回归） | planInjection / 降级路径零改动 |
| smoke：动态层 diff 指纹纯函数直跑 | 跑（回归） | `computeDynamicDiff` 签名与语义不变（regime 在调用方维护） |
| `run-smoke.sh` 其余 11 个套件 | 跑（回归） | 共享 `lib/` 无改动 |

## 变体走查

### 输入
| 变体 | 答案 | 落点 |
| --- | --- | --- |
| mid-turn 切档后**无**同 turn JIT 投递（regime 未写入，仍为旧值/null） | 下一 turn 重建时 regime≠新 key → 清空（空 Map 或旧指纹）→ 按 diff 正常投递 | smoke 主链路步骤 2 后直接跳 4 的变体 |
| mid-turn 切档 → JIT 投递 → **同 turn 再次切档**（skill 激活后又 edit-dir 绑定） | JIT 投递时 regime=当时键；下一 turn 新 key 不同 → 清空 → 全量重投（正确：语境确实变了） | smoke 白盒分支表覆盖 |
| 指纹 regime=null（从未有动态投递的会话切档） | 清空空 Map 无副作用；稳定层照常重建 | smoke 回归（未激活→激活既有用例） |

### 前提
| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 会话初始即激活档（input 命令路径，同 turn 起点重建） | 不存在延迟重建窗口，行为与现状完全一致 | smoke 回归：斜杠命令激活档位 |
| fork 继承：父快照 key 陈旧 + 指纹 regime=新 key | 继承方首 turn 重建时 regime=新 key → 不清空 → 父已投递不重发（fork 语义恢复） | smoke 场景 C |
| fork 继承：父 regime 与子新 key 不一致（父切档后无投递） | 清空 → 子按自身 diff 投递 | smoke 场景 C 反向 |
| channel="legacy" | 混合通道整体旁路，本修复不可达 | 既有 legacy 用例回归 |
| LRU 淘汰后重建 state | 全新 state（mode=null），本修复不可达 | 既有会话条目有界淘汰用例回归 |

### 时间窗口
| 变体 | 答案 | 落点 |
| --- | --- | --- |
| JIT 投递与下一 turn 重建之间隔 N 个 turn（内容/命中集无变化） | 指纹在、regime 在 → 一直零重投（不只第一个 turn） | smoke 主链路步骤 4 后追加空 turn 断言 |
| compact 发生在 mid-turn 切档与重建之间 | `pendingSnapshot` force 重发优先，不看 regime（重发后指纹重写为新 regime） | 既有 compact 用例回归 + 白盒分支表 |
| 同 turn 多次 JIT 投递（多个文档先后命中） | 各自写指纹与同一 regime；重建后全部零重投 | smoke 场景 A 扩展（2 个 fixture 文档） |

### 幂等
| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 步骤 4 的 `before_agent_start` 重放两次 | 第一次重建后快照 key 已一致 → 第二次非 transition，零投递 | smoke 幂等断言 |
| 同 regime 下文件内容未变重复 diff | `computeDynamicDiff` 零增量（既有纯函数用例） | 既有纯函数直跑用例 |

### 可用性（UI）
| 变体 | 答案 | 落点 |
| --- | --- | --- |
| widget / steer 消息展示 | 不改（仅消息条数减少）；`display` 策略不变 | 既有 steer 消息 UI 受控用例回归 |

## 效果核对

- `constraint.inject` 记账：同会话代内（代边界 = session.start + mode.set）不再出现「同 path + 同 bytes + 同 reason」重复行——人工 SQL 抽查（design.md §5 回检指标）。
- 稳定层记账（mode-base 重建时的一次性重记）保留——那是稳定层内容真实变化的正确记账，不在消灭范围。

## 白盒附加（复杂档：状态机分支 + 边界）

### 快照×指纹状态机分支表

| # | 前置状态（snapshot.key / fingerprintRegime / sentFingerprint） | 事件 | 迁移后 | 断言要点 |
| --- | --- | --- | --- | --- |
| 1 | `none\|none` / null / 空 | before_agent_start（mode=null） | 快照建、regime 仍 null | 稳定层注入，零动态 |
| 2 | `none\|none` / null / 空 | tool skill read → mode=req | 仅 mode 变；快照/指纹不动 | mode.set 记账 |
| 3 | `none\|none` / null / 空 | tool edit JIT 命中（mode=req） | 指纹={doc→h}、regime=`req\|none` | 1 条 steer |
| 4 | `none\|none` / `req\|none` / {doc} | before_agent_start（key=`req\|none`） | 快照重建；**指纹保留**（regime=key） | 零 steer（本案修复点） |
| 5 | `req\|none` / `req\|none` / {doc} | tool edit-dir 绑 changeY | 仅 boundChange 变 | mode.set 记账 |
| 6 | `req\|none` / `req\|none` / {doc} | before_agent_start（key=`req\|Y`） | 快照重建 + **指纹清空**（regime≠key） | 全量重投（护栏） |
| 7 | 任意 / 任意 / 任意 | session_compact → 下一注入时机 | force 重发（不看 regime） | 快照消息 1 条 |
| 8 | 父：`none\|none` / `req\|none` / {doc} | fork 继承 → 子首 turn（key=`req\|none`） | 快照重建；指纹保留 | 零 steer（fork 变体） |
| 9 | `req\|X` / `req\|X` / {doc} | 归档 X → before_agent_start（mode 回落 null） | key=`none\|none`、指纹清空（regime≠key） | 回未激活索引态 |

### 边界值
- `fingerprintRegime` 为 null 且 sentFingerprint 非空（理论不可达，防御）：清空条件 `null !== key` 为真 → 清空——退化方向安全（多投一次，不崩不漏状态）。
- mode/boundChange 均为 null 的 regime 字符串 `none|none` 与快照 key 判等——纯字符串判等，无类型混淆面。
- `inheritFromParent` 拷贝后 regime 与指纹一致性：两者同源同时写入（deliverDynamic 单点），拷贝原子性由对象字面量一次性构造保证。

### 关键不变式（grep / 结构验证）
- `sentFingerprint` 赋值点仅 3 处（newSessionState / resetSessionState / deliverDynamic）+ isTransition 清空 1 处——grep 确认无第 4 写点。
- `fingerprintRegime` 写点仅 deliverDynamic（+ 构造器初始化 + inherit 拷贝）；清空判定仅 before_agent_start 一处。
- per-session 隔离：`fingerprintRegime` 只出现在 `state.channel.*` 读写，无模块级变量（`grep -n "fingerprintRegime"` 全部形如 `state.channel.fingerprintRegime`）。
