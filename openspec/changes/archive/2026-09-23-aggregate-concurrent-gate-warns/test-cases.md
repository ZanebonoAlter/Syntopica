# test-cases: aggregate-concurrent-gate-warns

> 复杂档白盒账本（complexity: complex——指纹状态机 ≥3 态 + 包锚点解析算法 + 账本记账协议）。落点均为**拟定**，随 tasks 逐步落地；断言判据（什么算对）主线程定于本文件，机械枚举不外包。机器对账以 tasks.md 验证节的 `| Scenario | 测试文件 |` 表为准，本文件主链路表与之指向同一批落点。

## 故事 S1：状态没变就闭嘴，状态一变就说话（锚 Requirement: 失败指纹与边沿触发注入）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1.1 | 会话内某门禁命令首次失败（windows native 双链路任一） | 首次失败输出完整块 | steer 含完整失败块（分级前缀 + exit + 尾部 ≤30 行），会话内仅此一次 | harness behavior smoke | `.pi/extensions/tests/quality-gate.behavior.smoke.cjs`（场景 L 改造） |
| 1.2 | 同指纹连续第 2、3 回合失败（纯对话回合，触发集空） | 同指纹持续零注入 | 这两回合零 quality-gate-failure steer（无单行摘要、无「未修」字样），直到指纹变化或转绿 | harness behavior smoke | 同上（L 场景：L2/L4 断言由旧行为改零注入） |
| 1.3 | 同指纹持续期间统计门禁命令执行次数 | 粘性重跑语义不变 | 每回合 golangci-lint 照常重跑（3 回合失败 = 3 次执行），执行次数与注入收敛无关 | harness behavior smoke | 同上（lintCalls 断言不减） |
| 1.4 | 失败特征行变化（diag 改变） | 首次失败输出完整块（指纹变化半） | 重新输出完整失败块（视同首次），新问题不被静默吞掉 | harness behavior smoke | 同上（新增指纹变化变体） |
| 1.5 | 上回合失败的命令本回合成功（失败段中） | 失败转绿输出收尾一行 | 输出一行 ✓ 已转绿（每失败段至多一次），随后清指纹 | harness behavior smoke | 同上（L5/L6b 继承） |
| 1.6 | 用户/agent 想确认红态（已注入过一次后） | 红状态可经查询入口获知 | 运行 `bash scripts/harness/gate-status.sh` 读到该命令最新红态 + diag 摘要 + 时间 | 脚本 smoke | `scripts/harness/gate-status.smoke.sh`（红态用例） |
| 1.7 | 新会话（session_start 边界）同 diag 再失败 | 指纹状态 MUST 会话绑定 | 仍输出完整块（跨会话不复用指纹） | harness behavior smoke | 同上（场景 M 继承） |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| V1 | 输入：失败输出全空（diag 截断为空串） | 指纹 "" 精确相等仍成立 → 第 2 回合零注入；首回合完整块照出 | 白盒推演 | 附录 A 分支表（边界值 2），不单独落测试（truncateDiagGate 同源双用，无分叉面） |
| V2 | 输入：diag 超 512B 截断（尾部被截） | 双侧同源（gate.check 与指纹同用 truncateDiagGate）→ 相等性不受截断影响 | 白盒推演 | 附录 A 边界值 3 |
| V3 | 前置：会话首回合即全绿（无失败段） | 零 steer、无空的「已转绿」行 | behavior smoke | 现有 A3/L6b 继承覆盖，不新增 |
| V4 | 前置：无 sessionId（stub 语境） | 状态机照走（零注入/完整块/收尾不变），仅记账跳过 | 白盒推演 | 附录 A 分支表门控列 |
| V5 | 幂等：同输入连续两回合 | 第 1 回合完整块、第 2 回合零注入（正是本 change 主诉求） | behavior smoke | 主链路 1.1+1.2 |
| V6 | 幂等：转绿收尾后再无失败 | ✓ 行只出现一次（条目已清，第 2 次成功零消息） | behavior smoke | L6b 继承 |
| V7 | 时间窗口变体 | 不适用——指纹状态无时间语义（会话边界清零，无窗口计算） | — | 划除留痕 |
| V8 | 可用性变体 | 不适用——纯 harness 注入策略，无用户界面/表单/空态 UI | — | 划除留痕（查询脚本空态见 S4-V4） |

### 继承与调整（REMOVED Requirement: 失败指纹与重复抑制 → 问句⓪ 必答）

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 首次失败输出完整块 | 继承 | `quality-gate.behavior.smoke.cjs`（L1） | 照跑（回归网），另加指纹变化变体（1.4） |
| 同指纹持续只输出单行摘要 | 废止 | 同上（L2） | 改断言：同指纹第 2 回合零注入（不再出现 ⟳ 单行）；旧行为断言删除留痕 |
| 连续三回合未变化附加未修标记 | 废止 | 同上（L4） | 改断言：第 3 回合仍零注入、全文无「未修」字样 |
| 失败转绿输出收尾一行 | 继承 | 同上（L5） | 照跑 |
| 粘性重跑语义不变 | 继承 | 同上（L3） | 照跑（次数断言不减） |
| 同指纹外部失败不重复提示（gate-failure-reporting 现状节拍） | 继承 | 同上（N6/O4） | 照跑——外部侧本就是边沿触发，本 change 不动 |

### 白盒附加（复杂档：指纹状态机）

**附录 A——`gateLog` 失败报告状态机分支表**（状态 = `failureReports.get(cmd)`，输入 = `(ok, ownership, diag)`；判据主线程定）：

| # | 前态 | 本回合输入 | 分支 | 旧行为 | 本 change 期望（断言判据） |
| --- | --- | --- | --- | --- | --- |
| A1 | 无条目 | 失败, mine, 任意 diag | 首次 | 完整块 | **完整块**（不变；L1 照跑） |
| A2 | mine{diag=d} | 失败, mine, diag=d | 同指纹持续 | ⟳ 单行（rounds≥3 加「未修」） | **零注入**：无任何 steer，rounds 照增（仅内部计数），sticky 照 add |
| A3 | mine{diag=d} | 失败, mine, diag≠d | 指纹变化 | 完整块 | **完整块**（不变） |
| A4 | 任意条目 | 成功 | 转绿 | ✓ 单行 + 清条目 | **不变** |
| A5 | 无条目 | 成功 | 静默 | 零消息 | **不变**（无空收尾） |
| A6 | 无条目 / mine | 失败, foreign, 新 diag | 外部首现 | [外部] 一行 | **不变**（含 foreign-breakage 记账每回合照记——本 change 不动外部侧记账） |
| A7 | foreign{d} | 失败, foreign, diag=d | 同指纹持续 | 零注入 | **不变** |
| A8 | foreign{d} | 失败, foreign, diag≠d | 指纹变化 | 重新一行 | **不变** |
| A9 | 无条目 / mine / foreign | 失败, mixed, 新 diag（或判性翻转至 mixed） | mixed 首现 | [并发] 完整块 + 记账 1 条 | **完整块 + 恰 1 条 `policy.decision(concurrent-mixed)`**（记账边沿 = 此处） |
| A10 | mixed{d} | 失败, mixed, diag=d | 同指纹持续 | ⟳ 单行 + **每回合记账 1 条** | **零注入 + 零新增记账**（1.2 红→绿主战场）；sticky 照 add |
| A11 | mixed{d} | 失败, mixed, diag≠d；或判性翻转 mixed→mine | 指纹变化/翻转 | 完整块（mine 侧）+ 记账照旧每回合 | mine 侧：完整块 + 不在持续分支记账；mixed 侧：完整块 + **再记 1 条** |
| A12 | 转绿清条目后 | 再次失败 mixed | 新失败段重建 | 同 A9 | **完整块 + 再记 1 条**（转绿后再现允许再记） |
| A13 | 任意 | session_start 边界 | 清零 | failureReports/sticky 双清 | **不变**（M1 照跑） |

**边界值清单（S1）**：

1. rounds 计数仅内部用：`rounds>=3` 的「未修」文案 MUST NOT 出现于任何 steer（负向断言）。
2. diag 空串（`"" === ""`）视为同指纹 → 走 A2 零注入，不因 falsy 误判首现。
3. diag 相等 = 字符串精确相等（truncateDiagGate ≤512B 截断后比对，两侧同源）。
4. 粘性重跑次数 = 回合数（与 rounds 解耦）：3 回合失败 → lint 执行 3 次。
5. 记账条数与 rounds 解耦：同指纹 N 回合 → concurrent-mixed 恰 1 条（N≥1）。
6. 注入清空后 failures 列表为空 → 本回合不发 quality-gate-failure steer（而非发空消息）。

## 故事 S2：包级失败找对主人（锚 Requirement: 包锚点参与归属判定）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 2.1 | golangci-lint 对某包报 `# <pkg> [build failed]`，输出仅包锚点；包目录下被点名文件全部归属其他 change（edit.map/基线），本会话未触发该目录 | 包级编译失败且目录全属外部时判外部 | 判 [外部]：不进粘性、不打 [回归]，foreign-breakage 记账照记（**现行实现下此用例为红 → 4.1 后转绿**） | harness behavior smoke | `quality-gate.behavior.smoke.cjs`（新增包锚点场景） |
| 2.2 | 同上，但包锚点目录 ∩ 已知归属集含本会话文件 | 包锚点目录含本会话文件时维持分级 | 维持 [回归]/[中间态] 分级 + 粘性语义 | harness behavior smoke | 同上 |
| 2.3 | 包锚点目录在 mine 与 foreign 均无精确归属（归属地图缺失） | 包目录无归属匹配时保守回退 | 视同本会话失败（[中间态]/[回归]），不误降级（**现行已绿 → 防误降级锚点，锁住**） | harness behavior smoke | 同上 |
| 2.4 | 纯函数：`# pkg` / `FAIL pkg [` / `vet: file:line` 三形态提取 | （白盒，spec 解析规则） | 形态各归其位：包 → 目录前缀映射；vet → 文件路径直并；空交 → 不并入 P | 函数单测 | `.pi/extensions/tests/failure-classify.smoke.cjs`（B3/B4 扩展） |
| 2.5 | 纯函数：解析不出路径的输出 | 路径不可解析时保守按本会话失败（继承） | P=[] → mine | 函数单测 | 同上（B6 继承） |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| V1 | 输入：`FAIL command-line-arguments [build failed]`（包路径无 `/`） | 映射 null → 不并入 P → P 空 → mine 保守 | 函数单测 | failure-classify.smoke（新增） |
| V2 | 输入：未登记 module 的包（`other-module/pkg`） | 映射 null → 同 V1 保守 | 函数单测 | 同上 |
| V3 | 输入：反斜杠 Windows 形态 / 同路径重复出现 | 归一 `/` + 去重保序 | 函数单测 | B2/B7 继承 |
| V4 | 输入：包锚点行 + 包内文件锚点行混合 | P 双入，判定按 P 整体三态（任一 mine 侧命中即不降级 [外部]） | 白盒推演 | 附录 B 判定矩阵 |
| V5 | 前置：库不可用（归属信号缺席） | 判定整体旁路 → 按本会话失败（P 场景继承） | behavior smoke | P1–P3 继承 |
| V6 | 幂等：包锚点 [外部] 连续两回合 | 第 2 回合同指纹静默（外部侧既有边沿），第 3 条 foreign-breakage 记账照记 | behavior smoke | N6/O4 语义继承 |
| V7 | 时间窗口 | 不适用——归属判定无时间语义（基线一次性快照，turn_end 不回填） | — | 划除留痕 |
| V8 | 可用性 | 不适用——无界面 | — | 划除留痕 |

### 白盒附加（复杂档：包锚点解析算法）

**附录 B——提取分支表**（`extractFailurePaths`，判据主线程定）：

| # | 输出行形态 | 提取 | 映射 | 并入 P |
| --- | --- | --- | --- | --- |
| B1 | `# syntopica-backend/internal/x [build failed]` | pkg 锚点 | `backend-go/internal/x/` 目录前缀 | 前缀（后续按目录∩已知集语义参与判定） |
| B2 | `FAIL syntopica-backend/internal/x [build failed]`（空格/Tab 分隔同效） | pkg 锚点 | 同 B1 | 同 B1 |
| B3 | `vet: internal/x/a.go:3:1: …` / `./internal/x/a.go:3:1: …` / eslint `path:line:col` | 文件锚点 | 无映射（本就是文件路径） | 文件路径直并 |
| B4 | `FAIL command-line-arguments …` / 未登记 module 包 | pkg 锚点但映射 null | — | 不并入（保守） |
| B5 | eslint bare path 行（整行一路径） | 行锚点 | 无 | 直并 |
| B6 | 散文行 / `0 issues.` / 空输出 | 不命中 | — | P=[] → mine |

**附录 B——判定矩阵**（`classifyFailureOwnership(P, mine, foreign)`，降级门槛不变）：

| P∩mine | P∩foreign | P⊆foreign | 判定 | 分级 |
| --- | --- | --- | --- | --- |
| ∅ | 非空 | 是 | foreign | [外部]（不进粘性） |
| 非空 | 非空 | — | mixed | [并发]（进粘性，不取 [回归]） |
| ∅ | 非空 | 否 | mine | 保守 [回归]/[中间态]（fail-open，防误降级） |
| ∅ | ∅ | — | mine | 保守（含 P 空、解析异常、非字符串成员） |

**边界值清单（S2）**：

1. 目录前缀精确映射：MUST NOT 递归上溯（`backend-go/internal/x/` 不得命中 `backend-go/internal/` 下无关文件之外的集合语义变化）、MUST NOT 模糊匹配。
2. 「∩已知归属集全部为 foreign → [外部]」与「∩为空 → mine」是两个边界端点，中间态（交集含 mine）落 mixed/mine，见矩阵。
3. 包目录 ∩ 已知集求交为空 → 该包锚点对 P 零贡献（不虚构路径）。
4. 4.1 实现只允许**增加**命中面（原 fail-open 案例转正确判定），MUST NOT 把现绿的 B4/V1/V2/矩阵第 3、4 行改红。

## 故事 S3：并发混合失败只记一笔账（锚 Requirement: 混合归属失败降级 [MODIFIED]）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 3.1 | go build/lint 失败，路径混合（mine ∧ foreign） | 混合归属失败标 [并发] 不标 [回归] | [并发] 前缀 + 双方路径分列 + 「可能非本会话所致，归档前仍需全绿」，无 [回归] 必须修措辞 | behavior smoke | `quality-gate.behavior.smoke.cjs` 场景 B4（B4-1 继承） |
| 3.2 | mixed 首现回合 | （记账边沿，新增约束） | 恰 1 条 `policy.decision(quality-gate/warn/concurrent-mixed, target=cmd)` | behavior smoke | B4-6（断言不变） |
| 3.3 | 同指纹连续 5 回合失败 | 同指纹多回合至多一条记账 | 事实库仍只 1 条 concurrent-mixed（**现行每回合 1 条 → 红**）；后续回合同时零 steer 注入 | behavior smoke | B4-6b 改断言（1.2 主战场） |
| 3.4 | 同指纹期间 | 混合归属照进粘性重跑 | 每回合门禁照跑（次数不减） | behavior smoke | B4-2（lintCalls 断言继承） |
| 3.5 | 失败特征变化（mixed 再现，diag 变） | 指纹变化后允许再记 | 作为新失败段**再记 1 条**（累计 2），完整块重出 | behavior smoke | 新增断言（1.2 后半段） |
| 3.6 | 失败段转绿后再次出现 mixed | 指纹变化后允许再记 | 清条目后重建 → **再记 1 条**，✓ 收尾先行 | behavior smoke | 新增断言 |
| 3.7 | 判性翻转 mixed→mine | （继承：判性翻转视同新指纹） | mine 侧完整块 + 正常分级；mine 侧本就无 concurrent-mixed 记账 | behavior smoke | B4-5 继承 |
| 3.8 | 纯外部 / 纯本会话两极 | 纯归属两极不变 | [外部]/[回归] 语义与本需求引入前一致 | behavior smoke | B4-8 继承 |
| 3.9 | 归因信号缺席（无绑定 change / 库不可用 / 解析异常） | 归因信号缺席时保守回退 | 视同本会话失败，不降级不漏催修 | behavior smoke | P1–P3 + N8 继承 |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| V1 | 前置：无 sessionId | 注入行为照走（完整块/零注入），记账跳过（0 条，不报错） | 白盒推演 | 附录 A 门控列（logPolicyDecision 调用处 sessionId 守卫既有） |
| V2 | 前置：库损坏 | gate.check/policy.decision 旁路，门禁与注入照旧 | behavior smoke | P 场景继承 |
| V3 | 输入：同回合 mixed 与 mine 命令并存 | 同一 failure 消息内 [并发]/[回归] 分列，各命令独立走各自分支 | behavior smoke | B4-7 继承 |
| V4 | 幂等：同 diag 连续回合 | 记账 1 条、注入 0 条（3.3） | behavior smoke | 3.3 |
| V5 | 时间窗口 | 不适用——记账边沿按指纹/会话，无时间窗 | — | 划除留痕 |
| V6 | 可用性 | 不适用——无界面 | — | 划除留痕 |

## 故事 S4：门禁红绿随时可查（锚 Requirement: 状态查询只读展示 [NEW]）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 4.1 | 账本含某命令最近 ok=false、另一命令最近 ok=true | 展示各命令最新红绿态 | 前者红态 + diag 首行摘要 + 时间 + 会话；后者绿态 + 时间；退出码 0 | 脚本 smoke | `scripts/harness/gate-status.smoke.sh` |
| 4.2 | `.pi/harness/events.db` 不存在 / 非 SQLite 字节 | 库缺失时报错退出 | stderr 说明、退出码 1、无误导性展示 | 脚本 smoke | 同上（缺库+坏库两用例） |
| 4.3 | 运行查询全程 | 查询全程只读 | 库文件字节前后一致（md5 不变）、零新文件、不执行任何门禁命令 | 脚本 smoke | 同上（只读断言） |
| 4.4 | 可读库但零 gate.check 记账 | 无任何门禁记账时的空态 | 「暂无门禁记账」类提示，退出码 0 | 脚本 smoke | 同上（空库用例） |
| 4.5 | 真库运行 | （tasks 5.1 验证） | 退出码 0，输出含各命令红/绿态与时间戳 | 人工+命令 | `bash scripts/harness/gate-status.sh`（V5） |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| V1 | 前置：同一 cmd 多条历史（红→绿→红） | 窗口函数取 id 最大者 = 最新红态（「最新条目=当前态」依赖记账协议） | 脚本 smoke | 4.1 用例构造多条 |
| V2 | 输入：多 cmd 混合状态 | 每 cmd 各一行独立展示 | 脚本 smoke | 4.1 |
| V3 | 绿态 diag 为 NULL | 展示不崩、无垃圾摘要 | 脚本 smoke | 4.1 绿态断言 |
| V4 | 空态 | 明确空态文案 exit 0（可用性：查了知道「没记过」而非报错） | 脚本 smoke | 4.4 |
| V5 | 时间窗口变体 | 部分适用：展示**最近**时间戳，无窗口过滤需求（全量最新即可） | 脚本 smoke | 4.1 时间列断言 |
| V6 | 幂等：连续跑两次 | 输出一致、库字节不变 | 脚本 smoke | 4.3 只读断言覆盖 |

### 白盒附加（gate-status.sh 分支/边界表）

| # | 库状态 | 分支 | 输出 | 退出码 |
| --- | --- | --- | --- | --- |
| S4-1 | 不存在 | 报错 | stderr 说明 | 1 |
| S4-2 | 存在非 SQLite / 损坏 | 报错 | stderr 说明 | 1 |
| S4-3 | 可读、零 gate.check | 空态 | 「暂无门禁记账」 | 0 |
| S4-4 | 可读、有记账 | 每 cmd 最新一条（`ROW_NUMBER() OVER (PARTITION BY json_extract(payload,'$.cmd') ORDER BY id DESC)`） | 红/绿态 + diag 首行截断 + ts + session | 0 |
| S4-5 | 红态展示 | 不改变退出码（红态是查询结果不是查询失败） | 同上 | 0 |

## 与 tasks 落点的对账索引

| tasks 条目 | 本文件落点 |
| --- | --- |
| 1.1 | S1 主链路 1.1–1.3（L 场景改造） |
| 1.2 | S3 主链路 3.3/3.5/3.6（B4 记账断言改造） |
| 1.3 | S2 主链路 2.1（红）/2.3（绿锚点） |
| 1.4 | S1 主链路 1.4/1.5（防回归锚点） |
| 5.1/5.2 | S4 全部 |
