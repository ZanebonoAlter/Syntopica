# Test Cases: harden-constraint-injection-channel

> complexity: complex（档位×绑定×turn 锁状态机 + diff 指纹算法 + 通道投递协议）。
> 故事锚点：**一个实现档会话从档位激活到约束送达，历经中途命中更新、多 change 并行干扰、compaction，约束始终在场、缓存始终稳定、归因始终可查**。
> 权威规范：`openspec/changes/harden-constraint-injection-channel/specs/{constraint-injection,harness-fact-log}/spec.md`；行为契约变更依据 `design.md` D1~D8。

## 0. 继承与调整（⓪ 改契约，MODIFIED Requirements）

本次 MODIFIED 了 `constraint-injection` 三个 Requirement（`档位识别与 change 绑定` / `每 turn system prompt 强制注入` → RENAMED `混合注入通道…` / `smoke test 覆盖纯函数`）与 `harness-fact-log` 两个（`注入与 pin 记账` / `档位记账`）。旧测试仍会跑绿但可能断言旧契约 —— 逐行处置：

| 旧 Scenario | 处置 | 旧测试资产 | 动作 |
|---|---|---|---|
| 实现档注入生效 | 契约改（红线层仍 systemPrompt，不变） | `.pi/extensions/tests/constraint-injection.smoke.cjs`（decl 系列） | 保留；新增断言：稳定层含红线层且**不含** keyword/JIT 全节 |
| 未激活档仅索引 | 契约不变 | smoke（未激活档仅注索引） | 保留 |
| JIT 路径细化 | 契约改（投递通道）；⓪旧测试可能断言 systemPrompt 含 JIT 节 | smoke（JIT: edit airouter 后追加 ai-logging） | 改断言：JIT 节出现在**动态层消息**，systemPrompt 不含 |
| flow 文档节级注入 | 契约改（声明域走 systemPrompt 红线层；关键词/JIT 走消息） | smoke（节级注入系列） | 拆分两路断言 |
| 节残缺时回退全文 | 契约不变 | smoke（残缺节回退全文） | 保留 |
| 红线层为空/低于下限回退全节 | 契约不变 | smoke（decl 回退系列） | 保留 |
| 声明注入层级记账 | 契约部分改（送达时记账，payload 仍含 layer） | smoke（decl: 记账 layer 标记） | 保留 layer 断言，新增「非重复记账」断言 |
| change 文本不触发关键词命中 | 契约不变 | smoke（撞车词系列） | 保留 |
| ASCII 关键词词边界整词匹配 | 契约不变 | smoke（词边界系列） | 保留 |
| 命中只增不减（缓存稳定） | 契约改（粘性集合保留，但**投递 diff 化**：稳态零重发） | smoke（关键词命中粘性） | 改：粘性集合仍只增不减 + 新增「指纹不变→零消息」断言 |
| 无声明不注入并提示 / 未知域名宽容忽略 | 契约不变 | smoke（无声明/未知域系列） | 保留 |
| 超预算分层降级 / 降级永不真丢 / 模型可见省略通知 / 降级确定性 / 降级记账 | 契约不变（通道无关，作用于送达内容） | smoke（预算系列） | 保留；确认作用对象含动态层消息正文 |
| 斜杠命令/skill 激活档位、change 归档后回落、explore 不绑定、requirements 提及绑定 | 契约不变 + 新增 source 字段记账 | smoke（档位系列 + mode.set 记账存在） | 保留；新增 source 断言 |
| resume/reload/startup 恢复系列、子线程不清零、新会话不继承 | 契约不变 + 恢复路径新增 code=recover 记账 | smoke（resume/startup 系列） | 保留；断言文案改「混合通道」表述处同步 |
| pin_finding 落点解析（三条） | 契约不变 | smoke（pin 系列） | 保留 |
| smoke test 覆盖纯函数（直跑/红线层提取/零提取回退/节提取回落/节低于下限） | 契约不变 + 新增五类纯函数 | smoke | 保留；新增用例见白盒节 |
| （旧）「agent 写 change 目录 SHALL 修正绑定」 | **契约改**：条件化（read 不抢、健康不抢、仅兜底绑、turn 锁定） | smoke 无对应用例（旧契约在主 spec 文本） | 新增反向断言：健康绑定时 read/write 其他 change **零绑定变化** |
| pin.read 会话内去重 | 契约不变 | smoke（pin.read 系列） | 保留 |
| constraint.inject 记录命中原因 | 契约改（送达时记） | smoke（constraint.inject 记账含 change-file） | 保留 payload 断言 + 新增「稳态不重复记账」 |
| 门禁记账 / gate.check | 不涉 | — | 不适用（划除） |

## 1. 主链路表（故事节拍）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 输入 `/opsx-apply hardening-x` | 斜杠命令激活档位 | mode=implementation、bound=hardening-x、记 mode.set(source=command) | 行为 smoke | `.pi/extensions/tests/constraint-injection.behavior.smoke.cjs` |
| 2 | 首轮 before_agent_start | 实现档注入生效 / 未激活档仅索引 | systemPrompt 追加稳定层（索引+baseDocs+声明域红线层）；动态层 findings 走 message | 行为 smoke | 同上 |
| 3 | 连续 20 turn 无新命中 | 稳定层档位生命周期内恒定 | 每轮 systemPrompt 字节一致（快照 key 未变则复用） | 行为 smoke | 同上 |
| 4 | 输入命中声明域关键词「airouter」 | 声明域内关键词正常命中 | 动态层一条消息含 ai-summary.md 全节；systemPrompt 不变 | 行为 smoke | 同上 |
| 5 | 同 turn 内并行 read 其他 change 目录 | read 不抢绑 | bound 不变、零 mode.set、注入归因不漂 | 行为 smoke | 同上 |
| 6 | 同 turn 内 write 其他 change 目录 | 绑定健康时写其他 change 目录不抢绑 | bound 不变、零 mode.set | 行为 smoke | 同上 |
| 7 | agent edit 命中 `doc-impact-applies` 路径 | JIT 路径细化 | `pi.sendMessage` steer 投递（triggerTurn=false），含该域全节 | 行为 smoke | 同上 |
| 8 | pin_finding 追加新条目 | findings 更新触发重发 | 下轮动态层一条含更新后 findings 的消息；稳态零重发 | 行为 smoke | 同上 |
| 9 | 发生 compaction | compact 后快照重发 / compact 重发快照记账标记 | 一条快照消息（稳定层摘要+动态层有效集合），记账 source=compact-resend | 行为 smoke | 同上 |
| 10 | 再次输入命中同一关键词 | 动态层事件驱动增量发送 | 零消息（指纹未变） | 行为 smoke | 同上 |
| 11 | change 目录被归档 | change 归档后回落 | 下一轮回落未激活（仅索引稳定层） | 行为 smoke | 同上 |
| 12 | 另开全新会话 startup | 全新 pi 会话启动不继承其他会话档位 | 未激活、零注入、零 mode.set | 行为 smoke | 同上 |
| 13 | 事后查事实库 | 恢复路径记账 / 绑定修正追记 | 该会话 mode.set 序列完整、每条带 source、无隐性绑定 | 行为 smoke + 真实库 | 同上 + 9.2 验证 |

## 2. 变体走查（五组固定清单）

### 2.1 输入

| 变体 | 答案 | 依据/落点 |
|---|---|---|
| 空串 | 不写入 recentInputs、不触发关键词命中 | 现有 smoke（input 分支） |
| 纯空白（全角空格/tab） | `.trim()` 后为空 → 同上 | 纯函数/behavior smoke 新增 |
| 纯分隔符（`,`、`、`、`/`） | 不命中任何关键词（词边界整词匹配） | 词边界系列扩展 |
| 单 token（`agi`） | 按词边界匹配，命中则进粘性集合 | 纯函数 smoke |
| 大小写（`Cron` vs `cron`） | ASCII 关键词统一小写比对（沿用既有实现） | 纯函数 smoke 断言 |
| 特殊字符（`cron*`、`cron?`） | 不视为词边界命中（仅 `\b` 边界内的独立词） | 纯函数 smoke 新增 |
| 超长（>500 字符） | 截断至 500 后入窗（既有语义） | 纯函数 smoke 断言截断 |

### 2.2 前置

| 变体 | 答案 | 依据/落点 |
|---|---|---|
| 空集（无命中文档） | 稳定层仅索引/baseDocs；动态层零投递 | behavior smoke |
| 单元素 | 正常投递单条 | behavior smoke |
| 重复（同一文档多次命中） | 同 itemKey 去重，指纹不变→零重发 | 纯函数 smoke（computeDynamicDiff） |
| 越界引用（声明域不存在 / 绑定 change 目录消失） | 未知域宽容忽略；绑定回落未激活 | smoke（未知域/归档系列） |
| 部分满足（声明 3 域，1 域解析失败） | 其余域正常注入、失败域忽略并提示 | behavior smoke 新增 |

### 2.3 时间窗口

| 变体 | 答案 | 依据/落点 |
|---|---|---|
| 边界两端（窗口 N=5，恰好第 5 条命中） | 命中保留；第 6 条入窗后粘性集合仍保留（只增不减） | 纯函数 smoke |
| 空窗口（recentInputs 为空） | 关键词命中源为空 → 零意外命中 | behavior smoke |
| 跨窗口（词滚出窗口） | 粘性集合保留该项；投递因指纹不变而零重发 | 纯函数 + behavior smoke |
| 归一化 | 输入 trim + 500 截断；关键词小写比对 | 纯函数 smoke |

### 2.4 幂等

| 变体 | 答案 | 依据/落点 |
|---|---|---|
| 重复执行（同 turn 两次注入计算） | 指纹/快照 key 相同 → 第二次零投递、systemPrompt 字节一致 | behavior smoke |
| 部分失败重试（记账写库失败） | fail-loud，注入与绑定照常（不阻断） | behavior smoke（记账失败不阻断激活） |
| 并发（同 turn 并行工具触碰多 change） | turn 锁定：仅首个生效，mode.set ≤1 | behavior smoke（同 turn 绑定锁定） |

### 2.5 可用性（TUI）

| 变体 | 答案 | 依据/落点 |
|---|---|---|
| 误输入反馈 | 命令不命中 → 不进档位，仅索引（不报错） | behavior smoke |
| 空态 | 无命中时 widget 显示「无命中」，零消息 | behavior smoke |
| 错误态 | 文档读失败/解析异常 → fail-open（跳过该文档，其余照常） | behavior smoke |
| 加载态 | 不适用（无异步加载 UI）——划除留痕：扩展为同步注入，无 loading 态 | — |
| 超长文本 | 动态层消息按 dynamicDisplay 精简；budgetBytes 超限走分层降级 | behavior smoke |
| 重复提交 | `triggerTurn:false` 不触发额外 turn；稳态零投递 | behavior smoke |

## 3. 效果核对（依赖断言外因素 → 真库/真实会话量化）

| 效果 | 触发原因 | 方法 | 量化结果 | 结论 |
|---|---|---|---|---|
| system prompt 缓存稳定 | 通道分层后新内容走追加 | 真实会话连续 turn 观察注入块 widget / 事件流 | 待实现后实测（9.3 人工确认） | 待填 |
| `constraint.inject` 频次下降 | 每 turn 重复记 → 送达时记 | `python3` 查 `.pi/harness/events.db` 会话内同 path 条数 | 基线见 task 1.2（现状每 turn 重复） | 待填 |
| 跨域关键词不再误拉 | 关键词域过滤 | 事实库查 keyword reason 的 discovery.md 注入是否消失 | 2026-09-16 现场：s=01a0aabd/01a0aabf 均有 `keyword: discovery.md` | 待填 |
| 绑定不再横跳 | 抢绑条件化 + turn 锁定 | 事实库查单会话 mode.set 条数与 source 分布 | 基线：s=01a0aabf 一毫秒 4 条 mode.set | 待填 |

> 效果核对是**交付必做项**：数据覆盖率/行为依赖断言外因素，必须真库量化并在归档时回填。

## 4. 白盒附加（complex：状态机 / 算法 / 协议）

### 4.1 档位×绑定×turn 锁 状态机分支表

| 档位 | 绑定 | turn 锁 | 触发事件 | 期望转移 | 落点 |
|---|---|---|---|---|---|
| 未激活 | null | unlocked | input `/opsx-apply X` | implementation / X / locked，记 source=command | behavior smoke |
| 未激活 | null | unlocked | read skill 文件 | 对应档位 / mtime 兜底（仅 impl）/ locked，记 source=skill | behavior smoke |
| requirements | null | unlocked | input 提及 X | requirements / X / locked，记 source=command | behavior smoke |
| requirements | null | unlocked | 写 change 目录 | 兜底绑 X，记 source=edit-dir | behavior smoke |
| implementation | 健康 | unlocked | read 其他 change | 不变，零记账 | behavior smoke |
| implementation | 健康 | unlocked | write 其他 change | 不变，零记账 | behavior smoke |
| implementation | 健康 | **locked** | write 其他 change / 再绑请求 | 不变，零记账（锁定拦截） | behavior smoke |
| implementation | null | unlocked | write change 目录 | 兜底绑，记 source=edit-dir | behavior smoke |
| implementation | 目录消失 | unlocked | before_agent_start | 回落未激活 | behavior smoke |
| 任意 | 任意 | 任意 | before_agent_start | 锁重置 unlocked | behavior smoke |
| 未激活 | null | unlocked | session_start(new/fork) | 清零（不恢复） | behavior smoke |
| 任意 | 任意 | 任意 | session_start(startup, 档位非空=子线程) | 保持不动、零记账 | behavior smoke |
| 未激活 | null | unlocked | session_start(startup, 同 sessionId 有记录) | 恢复 + 记 source=recover | behavior smoke |

### 4.2 diff 指纹算法分支表

| 输入 | 期望 | 落点 |
|---|---|---|
| current == sent（全等） | 增量空、零投递、指纹不变 | 纯函数 smoke |
| 新增 1 item | 增量=[该 item]、投递只含该 item | 纯函数 smoke |
| 已有 item 内容变化（hash 变） | 增量=[该 item]、投递只含该 item | 纯函数 smoke |
| 已有 item 内容不变 | 非增量 | 纯函数 smoke |
| 档位切换（集合重置） | 清空 sent、全量集合为增量（首轮） | 纯函数 smoke |
| compact 重发（pendingSnapshot） | 无视指纹投递快照、重建 sent | 纯函数 + behavior smoke |
| 稳定层 key 未变 | 复用 block、不读文件 | behavior smoke（读计数/记账条数） |
| 稳定层 key 变（mode/绑定/声明域/文档 hash） | 重算 block、systemPrompt 变化一次 | behavior smoke |

### 4.3 边界值清单

| 边界 | 取值 | 期望 | 落点 |
|---|---|---|---|
| `minSectionBytes` | 511 / 512 / 513 | <512 回退全文；≥512 注节 | 纯函数 smoke |
| 红线层条数 | 0 / 1 / N | 0 条回退全节；≥1 且 ≥512B 注红线层 | 纯函数 smoke |
| 红线层字节 | 511 / 512 | <512 回退全节 | 纯函数 smoke |
| `budgetBytes` | 0（关闭预算）/ 2048（极小）/ 32768（缺省） | 0=不降级；极小=全占位但永不真丢 | 纯函数 smoke |
| 声明域数 | 0 / 1 / 多 / 含未知 | 0=提示无声明；未知忽略并提示 | behavior smoke |
| `recentInputs` 窗口 | 0 / 1 / 5 / 6 | 只保留最近 N；粘性集合不受影响 | 纯函数 smoke |
| 单条输入长度 | 499 / 500 / 501 | >500 截断至 500 | 纯函数 smoke |
| turn 内绑定请求数 | 1 / 3（不同 change） | 仅首个生效 | behavior smoke |
| findings 标题数 | 0 / 1 / 多 | 0 不注入；多时逐条记 pin.read（去重） | 纯函数 + behavior smoke |

### 4.4 不适用划除留痕

- **协议重试/超时**：不适用——扩展内无网络调用（差异：quota-gate 有；本 change 无）。
- **数据库事务回滚**：不适用——事实库 append-only、单条 insert，无事务边界。
- **并发写冲突（多进程）**：不适用——模块状态为单进程内；跨进程隔离由 pi 进程边界保证。
- **视觉回归（截图）**：不适用——ui-impact: none，无前端界面。

## 5. 层选择说明

| 层 | 覆盖 | 理由 |
|---|---|---|
| 纯函数 smoke（node 直跑 .cjs） | 抢绑判定、turn 锁定组合、diff 指纹、关键词域过滤、稳定层 key、渲染、快照计划、边界值 | 最便宜层；纯逻辑无 IO |
| 行为 smoke（mock pi 驱动真实 handler + 临时 events.db） | 主链路 13 节拍 + 状态机分支表 + 记账语义 + compact | 需真实事件时序与状态迁移 |
| 真实会话观测 | system prompt 字节恒定性、widget/消息展示 | 效果依赖真实 provider 与 TUI（见 §3） |
| opencli 端到端 | 不适用（无前端交互 change；ui-impact: none） | — |
