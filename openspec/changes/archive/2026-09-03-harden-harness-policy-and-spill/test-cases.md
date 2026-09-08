# test-cases: harden-harness-policy-and-spill

> 复杂度声明：complex（依据：三个独立 gate 共享同一事件 payload 协议，且 spill 同时涉及并发唯一性、POSIX/Windows 权限分支与 fail-open 边界）。

## 故事 S1：事实库接受新的低噪声策略事件（锚 Requirement: 事件类型词汇与保留期）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 在既有 events 表追加 policy.decision | policy.decision 随词汇扩展落库 | 新 kind 可写可查，不升级 user_version | smoke | `.pi/extensions/tests/harness-log.smoke.cjs` |
| 2 | 重新打开包含旧事件与新事件的库 | 事件追加不可变 | 既有行不改写，新旧 kind 共存 | smoke | `.pi/extensions/tests/harness-log.smoke.cjs` |
| 3 | 开库清理 31 天前的新旧 30 天事件 | TTL 分级清扫 | policy.decision 与 constraint.inject 被清理，永久 pin.write 保留 | smoke | `.pi/extensions/tests/harness-log.smoke.cjs` |
| 4 | 回归既有 spill/subagent 事件 | spill.write 事件随词汇扩展落库 / subagent.complete 随词汇扩展落库 | 两类事件写入与 TTL 行为不变 | smoke | `.pi/extensions/tests/harness-log.smoke.cjs` + `.pi/extensions/tests/spill.smoke.cjs` |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 前置：全新库 / 已有 v1 库 | 都接受 policy.decision，不做 DDL 迁移 | smoke | `harness-log.smoke.cjs` |
| 2 | 时间：30 天边界内 / 31 天前 | 边界内保留，31 天前清理 | smoke | `harness-log.smoke.cjs` |
| 3 | 幂等：重复追加同 payload | append-only，保留两条独立事件 | smoke | `harness-log.smoke.cjs` |
| 4 | 输入/可用性 | 不适用：kind 与 payload 由类型化内部 helper 产生，无用户输入和 UI | 留痕 | 本文 |

## 故事 S2：策略干预可查询但不反向影响门禁（锚 Requirement: 策略显著裁决统一记账）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | spec-gate 因检查失败阻断归档 | spec-gate 阻断归档被记录 | 原 block 结果不变，追加 spec-gate/block 事件并绑定目标 change | smoke | `.pi/extensions/tests/spec-gate.smoke.cjs` |
| 2 | spec-gate 显式使用逃生口 | spec-gate 显式豁免被记录 | 原放行结果不变，追加 spec-gate/bypass | smoke | `.pi/extensions/tests/spec-gate.smoke.cjs` |
| 3 | quota-gate 分别遇到额度不足和查询异常 | quota-gate 阻断与 fail-open 被区分 | 分别产生 block 与 fail-open；target 只有 provider | smoke | `.pi/extensions/tests/policy-decision.smoke.cjs` |
| 4 | test-scope 以 soft/hard 处理全量测试 | test-scope 软硬模式被记录 | 分别产生 warn/block，reasonCode 均为 full-go-test | smoke | `.pi/extensions/tests/policy-decision.smoke.cjs` |
| 5 | 三个 gate 遇到正常通过或未命中 | 正常放行零记录 | policy.decision 行数不增加 | smoke | `.pi/extensions/tests/policy-decision.smoke.cjs` |
| 6 | 令事实库写入失败后重走显著裁决 | 记账故障不改变裁决 | block/warn/bypass/fail-open 返回和消息仍按原逻辑发生 | smoke | `.pi/extensions/tests/policy-decision.smoke.cjs` |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入：reasonCode 空串、含空白/大写/特殊字符 | helper 拒绝或归一到固定 unknown 代码，不把自由文本写入 | 函数 smoke | `policy-decision.smoke.cjs` |
| 2 | 输入：target 含超长文本或疑似密钥字段 | target 截断且只由 producer 传 change/provider，原文不落库 | 函数 smoke | `policy-decision.smoke.cjs` |
| 3 | 前置：无活跃 change | change 列为 null，事件仍可按 session 查询 | smoke | `policy-decision.smoke.cjs` |
| 4 | 幂等：同一工具调用同时有 warning 与 block | 按 action 追加两条显著事件，不改写既有行 | smoke | `policy-decision.smoke.cjs` |
| 5 | 时间窗口 | 不适用：策略裁决本身无业务时间窗口，保留期由 S1 覆盖 | 留痕 | 本文 |
| 6 | 可用性：记账失败 | 用户仍收到原门禁提示/阻断结果，不新增二次阻断文案 | smoke | `policy-decision.smoke.cjs` |

## 故事 S3：并行 spill 不覆盖且本地文件最小暴露（锚 Requirement: spill 文件唯一命名与最小权限）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 固定同一时间戳，两个不同 toolCallId 的同名工具同时超阈值返回 | 同毫秒同工具并行结果不覆盖 | 得到两个不同路径，各自内容完整 | smoke | `.pi/extensions/tests/spill.smoke.cjs` |
| 2 | 检查两个文件名和 spill.write.path | 文件名不暴露原始调用标识 | 文件名含确定性哈希且不含原始 toolCallId | smoke | `.pi/extensions/tests/spill.smoke.cjs` |
| 3 | 在 POSIX 新目录写 spill | POSIX 最小权限 | 目录 0700、文件 0600 | smoke | `.pi/extensions/tests/spill.smoke.cjs` |
| 4 | 将既有会话目录放宽后再次 spill | 既有目录权限收敛 | 目录恢复 0700，既有文件内容不变 | smoke | `.pi/extensions/tests/spill.smoke.cjs` |
| 5 | 模拟 chmod/排他写失败 | 权限收紧失败安全降级 | 上下文收到原始结果，失败 spill.write 可查，已有文件不覆盖 | smoke | `.pi/extensions/tests/spill.smoke.cjs` |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入：工具名含空格、斜线、Unicode 或超长文本 | 沿用安全工具名清洗，最终文件名只有跨平台安全字符 | smoke | `spill.smoke.cjs` |
| 2 | 前置：目录不存在 / 已存在且权限宽 / 文件意外同名 | 分别安全创建、权限收敛、排他写失败而不覆盖 | smoke | `spill.smoke.cjs` |
| 3 | 幂等/并发：不同 toolCallId 同毫秒；相同 toolCallId 重复信号 | 前者双文件，后者排他创建拒绝覆盖并 fail-open | smoke | `spill.smoke.cjs` |
| 4 | 平台：POSIX / Windows | POSIX 精确断言 mode；Windows 跳过 mode 断言但继续验证命名和内容 | smoke | `spill.smoke.cjs` |
| 5 | 时间窗口：30 天清理边界 | 沿用既有测试，新增命名不影响目录扫描 | smoke | `spill.smoke.cjs` |
| 6 | 可用性/UI | 不适用：无 UI；失败时模型仍收到原始工具结果 | 留痕 | 本文 |

## 效果核对

不触发问句④：结果由本地事件行、文件路径、内容和 mode 位直接断言，不依赖真实数据覆盖率、LLM 行为或外部服务。

## 继承与调整（⓪ 契约变更对账）

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| TTL 分级清扫 | 继承并扩充 policy.decision | `.pi/extensions/tests/harness-log.smoke.cjs` | 增加新 kind 的 30/31 天边界断言，既有断言照跑 |
| 事件追加不可变 | 原语义继承 | `.pi/extensions/tests/harness-log.smoke.cjs` | 加入新旧 kind 共存断言 |
| spill.write 事件随词汇扩展落库 | 原语义继承 | `.pi/extensions/tests/spill.smoke.cjs` | 原断言照跑 |
| subagent.complete 随词汇扩展落库 | 原语义继承 | `.pi/extensions/tests/harness-log.smoke.cjs` | 原断言照跑 |

## 白盒附加

### 分支表

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| 1 | policy action=block | spec/quota/test-scope 阻断 | 写 block，原返回值不变 | `policy block is audited` |
| 2 | policy action=warn | spec/quota/test-scope 提醒 | 写 warn，原消息不变 | `policy warning is audited` |
| 3 | policy action=bypass | spec 逃生口 | 写 bypass，继续放行 | `policy bypass is audited` |
| 4 | policy action=fail-open | quota 查询异常 | 写 fail-open，继续放行 | `policy fail-open is audited` |
| 5 | ordinary allow | 未命中/健康通过 | 零 policy.decision | `ordinary allow stays silent` |
| 6 | event write failure | events.db 不可写 | 原裁决不变 | `audit failure is side-effect only` |
| 7 | POSIX spill | 目录新建/复用 | 0700 + 0600 | `spill enforces posix modes` |
| 8 | Windows spill | mode 不受支持 | 命名和内容仍通过 | `spill mode is best effort on windows` |
| 9 | 路径碰撞 | 排他创建发现同名 | 不覆盖，原结果直通 | `spill collision fails open` |

### 边界值清单

| 变量 | 边界值 | 期望 | 测试用例名 |
| --- | --- | --- | --- |
| action | 4 个合法值 / 非法值 | 合法值入库；非法值不形成自由事件 | `policy action whitelist` |
| reasonCode | 最短合法值 / 空串 / 非 kebab-case | 只接受稳定有界代码 | `reason code is bounded` |
| target | null / provider / change / 超长值 | null 可省略；安全摘要有界 | `policy target is bounded` |
| toolCallId | 两个不同值 / 相同值重复 | 不同路径；重复不覆盖 | `spill call identity` |
| POSIX mode | 0777 既有目录 / 新文件 | 收敛 0700 / 0600 | `spill enforces posix modes` |
| TTL | 30 天内 / 31 天前 | 保留 / 清理 | `policy decision retention` |

### 不适用划除（留痕）

- 业务输入的空串、分隔符、大小写：不适用，三个 gate 的既有命令/额度匹配规则不在本 change 修改。
- 数据库事务与业务 SQL：不适用，events 表和写入事务语义不变。
- 前端加载态、空态、重复提交：不适用，本 change 无产品 UI。
