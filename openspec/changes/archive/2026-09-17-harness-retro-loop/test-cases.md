# 测试用例（harness-retro-loop）

> 单元 = Requirement 的用户故事；spec Scenario 是断言片段，本文档串成完整故事。
> 复杂度声明：`complex`（proposal.md 头部）→ 白盒附加节为强义务。
> 层选择：脚本行为 = `scripts/harness-retro.smoke.sh`（fixture events.db，断言 stdout 关键字 + 退出码 + 只读性）；文档注册 = 人工 grep；可复算性 = 人工独立 SQL 复算。

## 1. 主链路（故事节拍）

**故事 A：从账本到改进项，改完能回检**（来源：`harness-retro-loop` 报告分段 / 分母口径 / 基线比对）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| A1 | 对 fixture 库跑 `bash scripts/harness-retro.sh --days 7` | 六段齐备且各带指标 | 输出含六段标题，每段至少一个「指标名 + 数值」对 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |
| A2 | fixture 含 6 失败 + 1 锚点 + 2 采样（`n=5`） | 按锚点与采样权重还原分母 | 还原执行次数 17、失败率 6/17，报告中口径被显式标注 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |
| A3 | `--save-baseline` 后追加同类失败再跑 | 生成基线与差值 | 对应指标输出正向差值；基线文件内容不被后续运行改写 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |
| A4 | `--change <name>` | 按 change 归属限定 | 各段数值只由该 change 的事件构成（跨 change 事件不计入） | 脚本烟测 | `scripts/harness-retro.smoke.sh` |
| A5 | fixture 含多个包的失败 diag（编译失败族 / 测试失败族） | 失败特征归并 | 按簇归并展示，非平铺清单 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |

**故事 B：不把 harness 故障读成代码质量下降**（来源：故障分离 / 软提醒失效）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| B1 | fixture 只有 3 条 `action=fail-open`、零门禁失败 | fail-open 不计入 agent 失败 | 故障段为 3、门禁失败段为 0，且报告标注排除 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |
| B2 | fixture 含 `block` 与 `bypass` 裁决 | 阻断类裁决不混入故障段 | 二者不出现在故障段 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |
| B3 | 某 `policy+reasonCode` 的 warn 计数达阈值 | 超阈值提醒纳入 | 出现在「软提醒失效」段并显示次数 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |
| B4 | 另一组 warn 计数低于阈值 | 低于阈值不产生噪声 | 不出现在任何分段 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |

**故事 C：库异常与窗口边界下不误导**（来源：只读消费 / 窗口 TTL / 空库语义 / 输出形态）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| C1 | 对 fixture 库跑脚本前后比对 | 只读打开不产生任何写入 | 库文件与已存在 sidecar 字节不变、事件行数不变 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |
| C2 | `--days 45` | 超期窗口给出提示 | 出现窗口超期提示，报告仍按请求窗口计算 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |
| C3 | `--days 7` | 常规窗口无提示 | 无超期提示 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |
| C4 | 指向不存在的库 | 库不存在时 fail-loud | 中文原因 + 非 0 退出码，无看似正常的报告 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |
| C5 | 指向 `application_id` 非魔数的库 | 拒绝非本应用的库 | 拒绝读取 + 非 0 退出码，且不执行写操作 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |
| C6 | 窗口内零事件的库 | 空库不产出伪指标 | 退出码 0、输出「窗口内无数据」、无失败率数值 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |
| C7 | 大量失败的库 | 有失败但退出码仍为 0 | 退出码 0 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |
| C8 | 运行（含保存基线）后查库 | 运行后账本零新增 | 事件行数与运行前一致 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |
| C9 | 运行后比对工作树 | 不触碰注入配置 | `.pi/constraint-injection.json` 与 `.pi/extensions/` 未变 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |
| C10 | 基线文件被写入乱码后运行 | 基线损坏时降级 | 报告照常生成（退出码 0）+「基线不可用」提示 | 脚本烟测 | `scripts/harness-retro.smoke.sh` |

**故事 D：方法论可被加载且能落地**（来源：复盘方法论随脚本交付）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| D1 | 检查 skill 文档与注册点 | 方法论文档存在且被注册引用 | 文档存在，且根 `AGENTS.md` 等注册点引用该 skill | 人工（grep） | tasks 4.2 |
| D2 | 按 skill 跑一遍流程产出改进项 | 判据可与报告指标对齐 | 改进项能落到报告某段指标名上（可回检） | 人工（评审） | tasks 4.3 |

## 2. 变体走查（五组固定清单，逐项给答案）

| 组 | 变体 | 答案 |
|---|---|---|
| 输入 | 空串 / 纯空白 | 覆盖：`--change ""` 视为全局（空值等价缺省），不报错 |
| 输入 | 纯分隔符 / 单 token / 特殊字符 | 覆盖：`--change` 值含特殊字符时按 SQL 参数/转义处理，不得拼接注入（白盒 T8）；`--days` 非数字 / 0 / 负数 → 参数非法、非 0 退出 |
| 输入 | 大小写 | 不适用：kind / policy / reasonCode 为固定小写标识，无大小写折叠需求 |
| 输入 | 超长 | 覆盖：diag 超长按簇归并时截断展示（≤120 字符），不整段回显（白盒 T9） |
| 前置 | 空集（窗口内无事件） | 覆盖：故事 C6（空库不产出伪指标） |
| 前置 | 单元素 | 覆盖：只有一条失败事件 → 失败率 1/1，聚类段只有一项 |
| 前置 | 重复 | 覆盖：同一 `(session, cmd)` 反复失败 → 计数正确、进「重复失败热点」段（白盒 T10） |
| 前置 | 越界引用 | 覆盖：`change` 列指向不存在的 change 名 → 按字面聚合，不报错 |
| 前置 | 部分满足 | 覆盖：一条 payload 不可解析 → 跳过该行 + 降级警告，其余段照常（白盒 T11） |
| 时间窗口 | 边界两端 | 覆盖：`--days 30` 不提示（等于保留期）、`--days 31` 提示（白盒 T12） |
| 时间窗口 | 空窗口 / 跨窗口 / 归一化 | 覆盖：`ts` 为 ISO 8601 UTC，窗口按 UTC 计算；跨 TTL 边界行不存在即不计（白盒 T13） |
| 幂等 | 重复执行 | 覆盖：同参数重复运行输出一致（除「生成时间」）；`--save-baseline` 覆盖同路径快照（幂等写） |
| 幂等 | 部分失败重试 | 不适用：无网络/外部服务调用，失败即确定性失败 |
| 幂等 | 并发 | 不适用：只读查询；与门禁重命令的并发纪律无关（无写锁） |
| 可用性 | 误输入反馈 | 覆盖：未知参数 → 用法提示 + 非 0 退出（白盒 T14） |
| 可用性 | 空态 / 错误态 / 加载态 | 覆盖：空态=故事 C6；错误态=故事 C4/C5/C10 |
| 可用性 | 超长文本 / 重复提交 | 不适用（非 UI） |

## 3. 效果核对

| 项 | 触发原因 | 方法 | 量化结果 | 结论 |
|---|---|---|---|---|
| 报告数据与真实账本一致 | 需确认脚本不是「自洽但错」 | 对真实 `.pi/harness/events.db` 跑 `--days 7`，再用独立 SQL 复算同一指标 | 失败条数 583、还原执行次数 3750、fail-open 42、full-go-test warn 26 —— 四项与独立 SQL 完全一致 | ✓ 通过（可复算） |
| 分母口径是否真的还原 | 记账侧成功采样会让 naive 口径低估分母 | 真实库上对比「naive 条数口径」与「加权还原口径」的失败率 | naive 583/2402 = **24.3%** vs 加权 583/3750 = **15.5%**，naive 高估 **8.7 个百分点** | ✓ 口径必要（不还原就误读） |
| 阈值触发是否符合预期 | 26 次 warn 应显著高于阈值 5 | 对真实库跑报告，核对 `test-scope-guard/full-go-test` 是否进「软提醒失效」段并显示 26 | 已入段并显示 `26 次`（组数 1/2，warn 共 27 条） | ✓ 通过 |
| 故障分离是否成立 | interop-down 曾污染失败统计 | 核对故障段计数与门禁失败段是否互斥 | 故障段 42 条全为 `quality-gate/interop-down`，未计入 agent 失败（另有 block 5 条不入段） | ✓ 通过 |
| 脚本耗时是否可接受 | 每次问数若过慢则不会被用 | `time bash scripts/harness-retro.sh --days 7` | **0.19s**（纯 SQL 聚合，2.6 万行账本） | ✓ 通过（远低于 1s 预期） |
| skill 是否真能被加载 | 注册点缺失则成孤立文档 | 检查注册点 + 参数表与 `--help` 一致性 | `AGENTS.md`（两处）、`开发执行规范.md` §12.5、`harness-facts` 交叉引用均命中；8 个参数在 `--help` 与 skill 参数表双现 | ✓ 通过 |

## 4. 白盒附加（分母还原分支表 + 边界值清单）

**分母还原分支表**（`gate.check` 行 → 计入执行次数）：

| # | ok | sampled | n | 计入 | 说明 |
|---|---|---|---|---|---|
| T1 | false | — | — | 1 | 失败全量记账，一条即一次 |
| T2 | true | 无 / false | — | 1 | 会话内首成功或失败转绿锚点（`flip=1`） |
| T3 | true | true | 5 | 5 | 采样条按其记录权重还原 |
| T4 | true | true | 缺失 | 5 | 缺 `n` 回退记账侧缺省 N=5 |
| T5 | true | true | 非法（0 / 负数 / 非数字） | **1 + 降级警告** | 保守取 1：宁可高估失败率，也不掩盖失败（不放大分母） |
| T6 | 任意 | — | — | 0（跳过）+ 降级警告 | payload 不可解析、无 `ok` 字段：该行不参与任何段，报告须提示降级 |
| T7 | false | — | — | 1 | `diag` 缺失不影响计数（diag 仅用于归并展示） |

**聚合与展示边界**：

- 某 `(session, cmd)` 窗口内失败数为 0 → 不输出失败率（避免除零），仅输出执行次数。
- 失败率以整数百分比展示，由 `awk` 计算；不得用 bash 整数除法（会截断成 0）。
- 全为采样条、无锚点：分母仍按 `n` 权重还原（与有锚点的差值在结果中不体现，但日志须可核对）。
- 阈值边界：warn 计数 **等于** 阈值即命中（`≥`）；阈值参数为 0 或非数字 → 参数非法、非 0 退出（不允许「阈值 0 = 全部命中」的静默语义）。
- 窗口边界：`--days 30` 与保留期相等 → 不提示；`--days 31` → 提示（判定式按「严格大于保留期」）。
- 时间基准：全部按 UTC 计算，避免宿主时区（CST）导致的日界偏移。

**输入安全边界**：

| # | 场景 | 期望 |
|---|---|---|
| T8 | `--change "x'; drop table events; --"` | 无注入：查询不执行破坏性语句；库行数不变（参数化或严格转义） |
| T9 | 单条 diag 10KB | 归并展示时截断（≤120 字符），报告不被冲刷 |
| T10 | 同 session 同 cmd 同 diag 连续重复 ≥ 阈值 | 进「重复失败热点」段并给出连续次数（死循环信号） |
| T11 | 单行 payload 为非法 JSON | 跳过该行 + 一次性降级警告；其余指标不受影响 |
| T12 | `--days 31` / `--days 30` | 分别提示 / 不提示 |
| T13 | 事件 `ts` 跨 UTC 日界与窗口边界 | 落在窗口外的行不计入（含恰好等于边界的时间戳：按 `>=` 含左边界） |
| T14 | 未知参数 / 位置参数 | 用法提示 + 非 0 退出，不静默忽略 |

**只读性断言**（故事 C1 / C8 / C9 的机械化）：

- 运行前后 `sha256sum` 比对：库文件、已存在的 `-wal` / `-shm`；
- 运行前后 `select count(*) from events` 一致；
- 运行前后 `git status --porcelain .pi/` 零变化（含 `constraint-injection.json` 与 `extensions/`）。

## 5. 继承与调整（问句⓪：旧契约的既有测试如何处置）

本 change 为**新增能力**（`harness-retro-loop` 全新 capability），不修改任何既有 capability 的 Requirement，因此不涉及「旧测试断言是否仍测新契约」的逐行核对。

| 旧契约 | 处置 | 动作 |
|---|---|---|
| `harness-fact-log`（全部 Requirement） | 保留，零改动 | 断言不变（`.pi/extensions/tests/harness-log.smoke.cjs` 等原样跑绿） |
| `gate.check` 采样记账契约 | 保留，被本 change **只读消费** | 分母还原的白盒分支表 T1-T7 必须与既有记账规范逐条对齐（尤其 T4 的缺省 N=5 与 T5 的非法取值处置） |
| `concurrent-change-coordination` | 不受影响 | 本脚本只读、不制造脏文件（基线落 git 忽略区） |
| 既有 `scripts/*.smoke.sh` 惯例 | 沿用 | 新 smoke 按同一断言风格（rc + 关键字）编写 |
