# test-debt-patrol Design

## Context

见 proposal.md Why。技术现状关键事实（2026-09-17 探索取证，`docs/research/test-debt-patrol/explore-findings.md`）：

- 增量门禁（quality-gate turn_end）对被碰 domain 跑 `go test -short`，[回归] 会话内必修——这条线健康；盲区在 platform/骨架档不测、前端只 lint、基线按会话重置
- 事实库证据：test-scope-guard `full-go-test` warn 30+ 条（9/2~9/14），全量兜底名存实亡
- 先例：`lint-zero-debt` spec（门禁零债务契约）与本 change 同构，可参照其措辞强度；台账软约束强度低于门禁硬约束，靠归档记账要求 + 巡检滚动补强
- 树莓派约束：前端全量 `test:unit` 96 文件叠多会话曾致 load 106 系统假死（`standard/frontend/testing.md`）；巡检必须限并发、错峰

## Goals / Non-Goals

**Goals:**

- 「发现了红」与「红被处理」之间不再有缺口：所有非本 change 的红进台账，生命周期可追溯
- 全局绿有低成本兜底：滚动巡检把全量测试摊到 N 天，单片资源占用有上界
- 摸底数据驱动后续决策：分片粒度、还债规模、是否需要 change-scope 反向依赖升级

**Non-Goals:**

- 不改 quality-gate「turn_end 不跑 go test」的门禁分层设计（巡检是独立通道）
- 不做反向依赖影响包分析（change-scope-gate 升级）——视摸底数据另立 change
- 不做巡检的扩展自动化触发（session_start 自动跑）——v1 走脚本 + AGENTS.md 纪律
- 不把欠账做成归档硬门禁（spec-gate 机器拦截需实测测试，回到硬件死结）——v1 纪律 + 提示语

## Decisions

### 决策 1：台账存储 = events.db 新表 `test_debt`（独立于 events 表，不走 TTL）

- **选型**：`.pi/harness/events.db` 内新建 `test_debt` 表，替代方案为独立 JSONL 或独立 SQLite 库。
- **理由**：① 复用既有开库安全逻辑（`lib/harness-log.ts` 的 schema 版本校验、拒绝他人/未来版本库）；② sqlite3 CLI 从 bash 脚本读写零依赖；③ 台账是**状态数据**（记录要更新、终态保留），与 events 表 append-only + 30 天 TTL 语义不同，独立表不受 TTL 清扫影响；④ 单库单文件，备份/清理策略不膨胀。
- **弃 JSONL**：状态迁移需重写文件，并发写（巡检脚本 + 归档会话同时登记）有竞态；弃独立库：跨库事务/备份多余，且 harness-fact-log 已有单库契约。
- **表结构（初稿，实现时可微调）**：`id PK, test_id UNIQUE, domain, first_seen, last_seen, context, status(open/fixed/waived), fixed_by, waived_reason, waived_at, note`。`test_id` 规范：后端 `<pkg>::<TestName>`，前端 `<相对文件路径>::<用例名>`。

### 决策 2：分片静态枚举，后端按 domain、前端按目录组（**已按摸底数据定夺**）

- **后端**：5 个业务 domain 各一片 + 骨架片（platform/models/app/cmd 合并）**共 6 片**；另设 `be-all`（整片，**不进轮转**，仅 `--shard be-all` 显式指定）。摸底实测 `go test -short -count=1 ./internal/... ./cmd/...` 全后端 **42s**（≪ 3 分钟阈值，见 `survey.md` §2）→ 合并可行；但单片只要 3~33s，保留 6 片可给出失败归属与更细的进度粒度，故不合并默认轮转，`be-all` 作便捷入口。
- **前端**：**6 片**静态目录组（`fe-tags` / `fe-discovery` / `fe-features` / `fe-core` / `fe-composables` / `fe-components`），统一 `--maxWorkers=2`。初版 4 片分组实测单片最慢 565s（超 2~4 分钟目标），按实测把 `features 其余` 与 `composables+components` 各拆二。
- **实测单片耗时（`--maxWorkers=2`）**：`fe-core` 13s、`fe-discovery` 12s、`fe-tags` 197s；`fe-features`/`fe-composables`/`fe-components` 见 `survey.md` §4.2（含外网超时噪声说明）。
- **关键调用形式（摸底新发现的硬约束）**：前端分片 **必须** 用 `pnpm test:unit <filter...> --maxWorkers=2`——带 `--` 的写法（`pnpm test:unit -- <filter>`）会被 vitest 当作位置参数丢弃，**静默跑成全量 100 文件**（三次实测复现，`survey.md` §4.1）；`standard/frontend/testing.md` 现有示例正是坏形式，本 change 内改正。
- **理由**：静态可枚举（spec 要求）→ 分片进度可持久化、「最久未巡优先」可机械判定；目录分组符合改动聚簇直觉，比文件序号轮流好排查。
- **前端耗时的真实瓶颈（登记，不在本 change 出圈）**：单片 wall time 被测试内真实外网连接（`connect ETIMEDOUT <公网IP>:443`，每次 TCP 重试 ≈130s）主导，同一组重跑可从 12s 到 9 分钟；`--maxWorkers=2` 约束的是 CPU 侧并发（外网等待不占 CPU），资源上界仍成立。降噪候选（另立 change）：mock/hang 掉外网 fetch 或加短超时。

### 决策 3：巡检脚本单入口 `scripts/test-patrol.sh`，子命令式

- `bash scripts/test-patrol.sh`（默认跑一片）/ `--shards <n>`（连续 n 片，按最久未巡优先）/ `--shard <name>`（指定片，含 `be-all`）/ `--report`（台账汇总）/ `--register` `--resolve` `--waive`（台账手工入口）/ `--init`（建表）。写台账 + `patrol.check` 事件均走 sqlite3 CLI 直写 events.db（`sqlite3 3.46.1` 已确认可用）。
- **退出码**：`0`=分片全绿（或 init/report/登记类成功）；`1`=分片跑完但检出红；`2`=用法/环境错误——供 CI/pre-push 与归档流程机械判定。
- **触发纪律**（v1，无扩展自动化）：AGENTS.md 增补一行——会话收尾若本回合无高负载操作，顺手 `test-patrol.sh` 跑一片；归档/pre-push 前必看 `--report` 是否有未还欠账。
- **资源预检**：脚本开跑前检查 load average（>4 提示延后，不强制）与 `.pi/run/` pidfile 白名单外的是否有 build/浏览器自动化进程提示；宁提醒勿阻断（巡检本来就是低峰补充）。

### 决策 4：归档记账 = 人工纪律 + 双提示语，不做机器拦截

- `开发执行规范.md` §11.4 自检清单加一条；test-scope-guard 归档语境放行时的放行提示、spec-gate 的归档指引文案各补一句「撞见非本 change 红 → `test-patrol.sh --register` 登记后继续」。
- **理由**：spec-gate 机器拦截必须实测测试命令，时长与硬件回到死结（探索已证）；纪律 + 工具使能（登记命令顺手）成本低；效果经 harness-retro 观察（台账 `context` 字段能归因到归档动作），不够再升级硬门禁。

### 决策 5：存量还债在本 change 内「小修大拆」

- 摸底后存量红 ≤ 5 处：本 change 直接修（顺带验证台账 fixed 流转）；> 5 处或单处修复涉及行为判断：登记后拆独立 fix change，本 change 只交付机制。
- **理由**：本 change 主交付是机制，还债规模不可预知；「小修」作为台账闭环的实机验证，「大拆」避免 change 失焦。

### 决策 6：`patrol.check` 事件词汇走 harness-fact-log 既有扩展模式

- bash 直写 events 表（change 列 NULL），payload `{shard, ok, ms, fails}`；30 天 TTL 与 gate.check 同级，台账持久化欠账、事件只记流水，二者不双写不互删（spec 已定）。
- session_id 取 `$PI_SESSION_ID`（pi 会话）；无则退 `patrol.sh`（手动/定时跑的语境）。

### 决策 7：摸底顺带修正 `pnpm test:unit -- <filter>` 文档陷阱（在范围内）

- **事实**：摸底实测 `pnpm test:unit -- <filter>` 的 `--` 会被 vitest 当作位置参数吞掉——filter 与 `--maxWorkers=2` 一起失效、**静默跑成全量 100 文件**（3 次复现，`survey.md` §4.1）。
- **而 `standard/frontend/testing.md` 与根 `AGENTS.md` 写的正是这个坏形式**——巡检分片（本 change 的核心交付）依赖 filter 正确生效，属「机制前置条件被现有文档破坏」，故同 change 修正文档、写明陷阱与正确形式；`AGENTS.md` 两处 `-- --maxWorkers=2` 一并改正。
- **理由**：不修则脚本正确调用与文档矛盾，后人照文档跑会重现 load-106 风险；修正成本仅几行文本，收益是消灭一个静默高危陷阱。

## Risks / Trade-offs

- **台账漂移**：测试改名/删除后 `open` 记录成为 stale——`--report` 展示 `last_seen` 超 30 天的记录供人工清理；不做自动判定（误判风险高于清理收益）。
- **巡检纪律衰减**：v1 无自动化触发，纪律可能松掉——harness-retro 用 `patrol.check` 事件频次做回检指标（如连续 7 天无巡检记录即提醒），数据在、升级路径在。
- **-short 全后端仍可能慢**（编译缓存冷 + 树莓派 IO）：摸底数据兜底；若单 domain 片 > 5 分钟，进一步按子包拆片（分片函数留参数化空间）。
- **软约束强度**：台账约束低于 lint-zero-debt 的门禁硬约束，存在「记了不修」新形态漂没——接受 v1，靠 harness-retro 效能看板观察欠账趋势，劣化再升硬。
- **前端分片 wall time 被外网超时噪声主导**（摸底新发现）：测试内真实外网连接每次 `ETIMEDOUT` 要等 ~130s，同一组重跑可从 12s 到 9 分钟（`survey.md` §4.2）。影响：单片耗时不可精确预测、进度表里的 ms 只是参考。不影响通过/失败判定与时序正确性。降噪（mock fetch / 短超时）登记为后续 change 候选。
- **混合工作树**：本 change 执行期间另有 2 个 active change 在树上（`harness-effectiveness-metrics`／`dev-process-guard`），共碰 `AGENTS.md`／`harness-retro.sh`／`harness-log.ts`；本 change 对这几个文件只做最小追加，归档前按 §0.6「拆 commit 看归属地图」对账。
