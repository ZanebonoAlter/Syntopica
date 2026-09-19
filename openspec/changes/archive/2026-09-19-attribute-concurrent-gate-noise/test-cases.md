# 白盒用例 — attribute-concurrent-gate-noise

> 声明 `<!-- complexity: complex -->` 对应的白盒用例文档（分支表 + 边界值清单）。
> 断言判据由主线程给定（下表「预期」列即判据）；子线程只做机械枚举，不得改判据。
> 黑盒行为用例见 change 内 specs/ 的 Scenario（WHEN/THEN），本文件是它们的实现级展开。

## A. 失败指纹状态机（`.pi/extensions/quality-gate.ts` step 5）

状态：`failureReports: Map<cmd, { diag: string; rounds: number }>`（会话边界清零，与 `stickyFailures` 同点位）。

| # | 输入（上回合 → 本回合） | 预期报告 | 预期状态迁移 | 预期粘性 |
| --- | --- | --- | --- | --- |
| A1 | 无 → 失败，diag = D1 | 完整块（分级前缀 + `tail(output,30)`） | `{D1, rounds:1}` | 进 sticky |
| A2 | `{D1,1}` → 失败，diag = D1 | 单行 `⟳ [cmd] 同一失败第 2 回合未变化：D1` | `{D1,2}` | 留 sticky |
| A3 | `{D1,2}` → 失败，diag = D1 | 单行 + 「未修」标记 | `{D1,3}` | 留 sticky |
| A4 | `{D1,4}` → 失败，diag = D1 | 单行 + 「未修」标记（不重复升级文案） | `{D1,5}` | 留 sticky |
| A5 | `{D1,1}` → 失败，diag = D2（特征行变化） | 完整块（视同首次） | `{D2,1}` | 留 sticky |
| A6 | `{D1,2}` → 成功 | 单行 `✓ [cmd] 已转绿` | 条目删除 | 出 sticky |
| A7 | 无条目 → 成功 | 无输出 | 无条目 | 不在 sticky |
| A8 | `{D1,2}` → 成功（上回合本会话从未失败过该命令，即首次跑就绿） | 无输出（不产生空的「已转绿」） | 无条目 | 不在 sticky |
| A9 | 会话 A `{D1,3}` → 会话边界重置 → 会话 B 首次失败 diag = D1 | **完整块**（跨会话不复用指纹） | `{D1,1}`（会话 B） | 进 sticky |
| A10 | `{D1,1}` → 纯对话回合（触发集空，sticky 非空） | 命令重跑；若仍失败 → 单行（A2 语义） | rounds 递增 | 粘性重跑生效 |

边界与不变量：

- **B1**：单回合多命令同时失败（如 lint + vet + build）→ 每条命令各自独立一行/一块，互不影响 rounds 计数。
- **B2**：`diag` 取 `truncateDiagGate(output)`（≤512 字节单行），指纹比较用该字符串的**精确相等**，不做模糊匹配。
- **B3**：完整块总数上限不因抑制而消失——本 change 不改变「同时失败多条命令时 steer 消息含 N 块」的既有行为（首次回合）。
- **B4**：`envFailures`（interop / toolchain 归因）路径**不参与**指纹状态机（它们本就不进 sticky，语义不变）。

## B. 路径提取 `extractFailurePaths(output)`（`.pi/extensions/lib/failure-classify.ts`）

| # | 输入片段 | 预期输出 | 说明 |
| --- | --- | --- | --- |
| B1 | `internal/tagmanagement/service/sourcestats/sourcestats.go:63:1: unused` | `["internal/tagmanagement/service/sourcestats/sourcestats.go"]` | golangci-lint 正斜杠形态 |
| B2 | `internal\admin\wire.go:97:1: File is not properly formatted (gofmt)` | `["internal/admin/wire.go"]` | **Windows 反斜杠形态（实测存在）→ 归一化为 `/`** |
| B3 | `# syntopica-backend/internal/topicgraph/service` | `["backend-go/internal/topicgraph/service/"]` | go vet 包锚点 → 目录前缀（module 名 `syntopica-backend` 映射到 `backend-go/`） |
| B4 | `FAIL syntopica-backend/internal/admin/service [build failed]` | `["backend-go/internal/admin/service/"]` | go test 包锚点 |
| B5 | `/home/u/repo/front/app/x.vue` + `front/app/y.vue` | `["/home/u/repo/front/app/x.vue","front/app/y.vue"]` | eslint 绝对/相对双形态 |
| B6 | `0 issues.` / `bash: line 1: go: command not found` / 空串 | `[]` | 无路径可提取 → 调用方保守处理 |
| B7 | 同一路径在同一输出出现 3 次 | 去重为 1 项 | 集合语义 |
| B8 | `openspec/changes/foo/tasks.md:12:1: ...`（非代码路径） | 原样返回该路径（是否外部由归属集判定，提取层不做业务过滤） | 提取层只负责解析 |

不变量：**不做任何「猜测式」推断**（不把任意 token 当路径）；解析失败一律返回空数组，后果是多报不少报。

## C. 归属判定真值表

输入：`P`（提取路径集合，第 B 组输出，可能为空）、`mine`（本会话触发集 ∪ 本 change 归属）、`foreign`（会话启动基线 ∪ 其他 active change 归属）。

| # | P | P ∩ mine | P ⊆ foreign | 预期判定 | 后果 |
| --- | --- | --- | --- | --- | --- |
| C1 | `{f.go}` 其中 f.go 在本会话启动基线中 | ∅ | 是 | **外部** | 不进 sticky、无分级前缀、一行 `[外部]` 文案、记 `foreign-breakage` |
| C2 | `{f.go}` 其中 f.go 是其他 active change 归属 | ∅ | 是 | **外部** | 同上 |
| C3 | `{f.go}`，f.go 本会话触发过 | 非空 | 真/假 | 本会话失败 | 现状（sticky + 分级 + 完整块） |
| C4 | `{a.go,b.go}`，a.go ∈ mine，b.go ∈ foreign | 非空 | 真 | 本会话失败 | 现状（混合即保守） |
| C5 | `{}`（解析不出） | ∅ | 形式真 | 本会话失败 | 现状（`P = ∅` 不判外部） |
| C6 | `{c.go}`，c.go 既不在 mine 也不在 foreign（**新增未归属脏文件**） | ∅ | 假 | 本会话失败 | 现状（宁可多报） |
| C7 | `{f.go}`，标志库/`edit.map` 不可用（foreign 仅剩会话基线） | ∅ | 是 | 外部（单信号仍成立） | 同上；信号缺席不扩大外部面 |
| C8 | `{f.go}`，cwd 无 git / 无绑定 change（mine 仅本会话触发集） | ∅ | 是 | 外部 | 同上 |
| C9 | 外部判定命中后，下一回合该命令仍在 sticky？ | — | — | **不在** | 纯对话回合不重跑该命令 |
| C10 | 外部判定命中：`gate.check` 是否仍记账？ | — | — | **仍然记**（ok=false 全量） | 并追加一条 `policy.decision(warn, foreign-breakage)` |
| C11 | 外部判定命中后，下一**编辑**回合门禁重跑、同指纹外部失败仍存在 | — | — | **不重发提示行**（同指纹会话内至多一行；指纹变化重新输出；gate.check 照记） | 外部提示与内部完整块共用指纹状态机的「首现一次」语义 |

## D. suggest 取数口径（`scripts/harness/doc-impact.sh suggest`）

| # | 输入条件 | 预期输出 | 退出码 |
| --- | --- | --- | --- |
| D1 | `--change foo` 且库中 foo 有 edit.map（含 `backend-go/internal/reader/service/a.go`） | 预勾选只由该集合推导（命中 `flow`）；标注「归属轨（change=foo）」 | 0 |
| D2 | 无 `--change`，`PI_SESSION_ID` 对应会话最新 mode.set 的 boundChange = `foo` | 同 D1（用查库得到的 foo） | 0 |
| D3 | `--change bar` 且同会话 boundChange = `foo` | 以 `bar` 为查询目标；输出标注 change=bar | 0 |
| D4 | 三源都解析不到 / foo 无 edit.map 记录 | 回退全树 diff；输出含「回退轨：全树 diff，可能含其他会话/其他 change 的改动」 | 0 |
| D5 | 树上有 `front/app/x.vue` 归属别人、`backend-go/.../a.go` 归属本 change | 三桶：本 change=〔a.go〕参与预勾选；其他 change=〔x.vue〕列出不参与；无归属=〔…〕列出不参与 | 0 |
| D6 | 无 `sqlite3` 命令 / 库文件缺失 | 回退全树轨道 + 标注；**不打印错误堆栈、不非零退出** | 0 |
| D7 | 任一桶超过 20 条 | 该桶最多显示 20 行 + 「另有 N 个」汇总行（不改变桶语义） | 0 |
| D8 | change 名含单引号等特殊字符（如 `--change "o'brien"`） | SQL 转义安全（沿用 `ownership_paths` 现有 `\'` 转义），不产生语法错误 | 0 |
