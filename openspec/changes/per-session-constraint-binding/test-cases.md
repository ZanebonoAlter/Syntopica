# Test Cases — per-session-constraint-binding

> 单元 = 一个 Requirement 的用户故事。主线故事：**用户开两个窗口干活（窗口 A 做 change X、窗口 B 做 change Y），A 的注入块永远是 X 的（含 X 声明的域约束），B 的是 Y 的；A 派子线程时子线程仍带 X 的完整约束块，而 A 的档位/命中集不受 B 与子线程影响。**

## 主链路（节拍表）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 会话 A 绑 X、会话 B 绑 Y（同进程交替 edit-dir） | 两会话交叉绑定互不污染 | A 注入归属恒 X（含 X 域）、B 恒 Y；无交叉 | 行为 smoke | `.pi/extensions/tests/constraint-injection.smoke.cjs` |
| 2 | B 切换绑定（A 不变） | 他会话绑定变化不刷新本会话稳定层 | A 稳定层字节不变、零新增稳定层记账、指纹不变 | 行为 smoke | 同上 |
| 3 | 会话 C 无任何绑定，D 已绑 Y | 无自身绑定且无父子关系时不借用他会话 | C 仅索引、归因未绑定、无 Y 的档位基础块/域约束 | 行为 smoke | 同上 |
| 4 | 主会话绑 X 后派子线程（**独立进程**，header 带 parentSession） | 子线程显式继承父会话 | 子线程带 X 完整块（内存继承或**事实库回退**）；事实库 `mode.set source=inherit boundChange=X`；主会话档位/命中集不被改写 | 行为 smoke + 事实库核对 | `.pi/extensions/tests/constraint-injection.smoke.cjs`（19.4 内存继承 / 19.10-19.11 跨进程回退） |
| 5 | A 命中 JIT/关键词，B 无命中 | 命中集按会话隔离 | B 注入不含 A 的命中文档；A 命中集本会话内只增不减 | 行为 smoke | 同上 |
| 6 | A 同 turn 已绑定，B 发生工具调用 | turn 绑定锁按会话隔离 | A 本 turn 绑定不切换 | 行为 smoke | 同上 |
| 7 | 会话数超上限 | 会话条目有界淘汰 | 最久未用被淘汰，存活会话状态不受影响 | 单测（纯函数/容器） | 同上（容器用例） |
| 8 | 无 sessionId 语境 | 无 sessionId 语境行为等价 | 单兜底槽位，行为与隔离前一致 | 行为 smoke | 同上 |

## 继承与调整（改契约的旧资产反查）

本 change 为 ADDED（新增 Requirement「会话作用域状态隔离」），未 MODIFIED/REMOVED 既有 Requirement 文本 → 旧资产无需逐行处置；但 `constraint-injection` 的既有 smoke 用例**全部必须继续绿**（尤其下列与本次语义相邻者）：

| 既有资产 | 处置 | 理由 |
| --- | --- | --- |
| `constraint-injection.smoke.cjs`：全新 pi 会话启动不继承其他会话档位 / 无自身档位历史的会话 reload/resume 不继承他窗口档位 | 跑（回归） | 恢复路径的隔离语义不得退化 |
| 同上：子线程派发不清零主会话档位 | 跑（回归，且升级为「显式继承 + 记账」） | D5 红线 3 |
| 同上：同 turn 绑定锁定 / read 不抢绑 / 绑定健康时不抢绑 | 跑（回归） | 绑定修正条件化语义不变 |
| 同上：稳定层档位生命周期内恒定 / 动态层事件驱动增量 / compact 后快照重发 | 跑（回归） | 快照按会话隔离后字节恒定语义不变 |
| `run-smoke.sh` 其余 11 个套件 | 跑（回归） | 共享 `lib/` 无改动，防止意外连带 |

## 变体走查

### 输入
| 变体 | 答案 | 落点 |
| --- | --- | --- |
| `sessionId` 缺失（undefined / 空串） | 落 `FALLBACK_KEY` 单槽位 | smoke 用例 8 |
| `sessionId` 相同（多 pane 看同一会话） | 视为同一会话、共享 state（期望语义），不隔离 | smoke 用例（显式断言） |
| `boundChange` 为 `null`（仅档位无绑定） | 不借他人；`declaration` 不注入任何域 | smoke 用例 3 |
| 输入超长（>500 字符） | 入窗截断 500，仍按会话隔离 | smoke（入窗隔离断言） |

### 前提
| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 空集（进程内只有一个会话） | 与隔离前行为等价（无可见差异） | 全量回归 smoke |
| 单元素 / 两会话 / N 会话 | 各自独立；N 超上限走 LRU | smoke 用例 7 + 单测 |
| 重复（同会话重复绑定同一 change） | 不重复记账（无变化不记 `mode.set`）——**保持既有语义**（若现状即记，则本 change 不改该行为，只断言 per-session 归属） | smoke 回归 |
| 越界引用（绑定 change 目录已归档/删除） | 既有回落语义：档位回落未激活（本会话内判定，不查他人） | smoke 回归 |
| 部分满足（会话 A 有 mode 无绑定、B 有绑定无 mode） | 各自独立判定；`declaration` 只按有绑定的那个会话走 | smoke 用例 1/3 |
| 父子关系可证 / 不可证 | 可证 → 继承 + `source=inherit`（内存命中或事实库回退）；不可证 → 不借用 | smoke 19.4 / 19.10 / 19.11 / 19.12 |
| 子线程跨进程（真实链路） | 内存无父状态 → 必须走事实库回退，否则拿不到约束块 | smoke 19.10 / 19.11 + 人工：派真实子线程后查 `mode.set source=inherit` |

### 时间窗口
| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 两会话事件交错（A 绑 → B 绑 → A 注入） | A 的注入不受 B 影响（本次 bug 的精确复现形态） | smoke 用例 1（顺序化交错） |
| 同 turn 内两会话事件 | turn 锁按会话独立 | smoke 用例 6 |
| 长会话（条目存活跨多小时） | `lastUsedAt` 刷新、不被误淘汰 | 单测（LRU 时间戳） |

### 幂等
| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 重复执行 `session_start`（同 reason 多次） | 只重置/继承本会话条目，多次执行收敛一致 | smoke（幂等断言） |
| 部分失败重试（父会话识别失败） | 退回未绑定（仅索引），不借用他人；不阻断 | smoke 用例 3/4 变体 |
| 并发（同进程多会话并发事件） | 单线程事件循环内按 sessionId 分桶，无共享写；不声称跨进程并发安全 | 说明 + smoke |

### 可用性（UI）
不适用（`ui-impact: none`，不改前端）。用户可见效果：注入块不再串味——由 smoke 用例 1/3 与事实库口径核对覆盖。

## 效果核对

| 核对项 | 触发原因 | 方法 | 预期 | 结论（实现后填） |
| --- | --- | --- | --- | --- |
| 部署后真实多会话无串味 | smoke 是桩驱动，真实链路仍有差异 | 观察窗口内查事实库：`constraint.inject.change` 必须等于该会话自身最近一条 `mode.set.boundChange`（无 mode.set 的会话必须为 `null`） | 0 例不符 | 待回填 |
| 子线程继承在真实链路成立 | 父会话识别依赖 pi 内部字段 | 真实会话派子线程后查：子线程 `mode.set source=inherit` 存在且 `boundChange` = 主会话 | 1 例符合 | 待回填 |

## 白盒附加（复杂档：状态机分支 + 边界）

### 会话状态机分支表
| 分支 | 条件 | 期望 |
| --- | --- | --- |
| B1 | 有自身 state，事件带 sessionId | 读写自身 state |
| B2 | 无自身 state，能证父子（fork/子线程） | 继承父 state + `mode.set source=inherit` |
| B3 | 无自身 state，不能证父子 | 新建空 state（未绑定）；不读他人 |
| B4 | `session_start reason=startup` 且本会话有 mode.set 历史 | 恢复本会话档位 + `source=recover` |
| B5 | `session_start reason=startup` 且本会话无历史（冷启动/reload 恢复失败） | 未绑定（不借用他人） |
| B6 | `reason=new` | 清本会话条目后新建空 state |
| B7 | `reason=resume` / `reload` | 清本会话条目 → 同 sessionId 恢复 |
| B8 | 无 sessionId | `FALLBACK_KEY` 单槽位 |
| B9 | 条目数 > 上限 | 淘汰 `lastUsedAt` 最小者 |

### 边界值
| 边界 | 期望 |
| --- | --- |
| 上限 = 1（极端配置） | 最新会话存活，其余淘汰；不 panic |
| 同 sessionId 两会话视图交替事件 | 共享状态、不互相重置 |
| `boundChange` 指向已归档 change | 本会话回落未激活；MUST NOT 影响他会话绑定 |
| 稳定层快照 key | 含会话维度 → 他会话绑定变化不产生本会话重发 |

### 关键不变量（grep / 结构验证）
- `.pi/extensions/constraint-injection.ts` 内**不存在**模块级 `let currentMode` / `let modeBoundChange` / `let turnBindLocked` / `let recentInputs` / `let jitDocHits` / `let keywordDocHits`（应全部收进会话 state 容器或容器字段）。
- 所有 `mode.set` 写入点都带 `source`，新增取值 `inherit` 与 spec 枚举一致。
- `bash .pi/extensions/tests/run-harness-smoke.sh` 全绿（含新增跨会话用例）。
