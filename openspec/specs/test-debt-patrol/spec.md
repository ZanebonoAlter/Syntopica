# test-debt-patrol Specification

## Purpose
TBD - created by archiving change test-debt-patrol. Update Purpose after archive.

## Requirements

### Requirement: 测试欠账台账

系统 SHALL 在 `.pi/harness/` 下维护一份测试欠账台账（本机运行数据，gitignored），登记「非本 change 改坏、且不在本次修复范围内的红测试」。每条记录 SHALL 包含：测试标识（后端包路径+用例名 / 前端测试文件+用例名）、首见时间、发现语境（巡检分片名 / 归档 change 名 / 手动全量）、当前状态、备注。

台账记录的状态机 SHALL 为：`open`（红，未处理）→ `fixed`（已修复，附修复 change 名）或 `waived`（豁免，附理由与豁免日期）。`fixed`/`waived` 为终态，记录保留供复盘统计，不得物理删除。

#### Scenario: 巡检发现新红自动登记

- **WHEN** 巡检某分片发现失败测试，且该测试不在台账中
- **THEN** 新增一条 `open` 记录，发现语境记为该分片名与时间

#### Scenario: 已有记录不重复登记

- **WHEN** 巡检发现失败测试，且台账中已存在同标识的 `open` 记录
- **THEN** 更新该记录的最近复现时间，不新增记录

#### Scenario: 状态迁移留痕

- **WHEN** 某测试被修复（对应分片转绿）或被显式豁免
- **THEN** 记录迁移到 `fixed` 或 `waived`，附修复 change 名或豁免理由，原 `open` 时期的字段保留

#### Scenario: 台账可查询

- **WHEN** 以查询模式运行巡检脚本（如 `--report`）
- **THEN** 按 domain/状态/首见时间汇总输出当前欠账清单，供还债排期与 harness-retro 复盘消费

### Requirement: 滚动分片巡检

系统 SHALL 提供巡检脚本（`scripts/harness/test-patrol.sh`）将全量测试拆为有界分片滚动执行：后端按 `backend-go/internal/` domain 目录分片（`go test -short`，跳过 DB 集成）；前端按测试文件组分片（`pnpm test:unit <filter...>`，**不带 `--`**——实测 `pnpm test:unit -- <filter>` 会吞掉 filter 静默跑全量，见 `standard/frontend/testing.md`；统一 `--maxWorkers=2`）。分片划分 SHALL 在脚本内静态可枚举（不依赖运行时发现），每片资源占用 SHALL 有上界。

巡检进度 SHALL 持久化（最近完成分片、时间、结果摘要），跨会话可续：连续巡检按「最久未巡的分片优先」推进，一轮全部完成后重新开始。支持一次跑一片（默认）与一次跑多片（显式参数）。

#### Scenario: 单分片巡检落账

- **WHEN** 执行 `bash scripts/harness/test-patrol.sh`（无参数，跑一片）
- **THEN** 选取最久未巡的分片执行，输出通过/失败摘要，失败测试自动登记台账，进度与时间落账

#### Scenario: 资源红线——低并发执行

- **WHEN** 前端分片执行
- **THEN** 以 `--maxWorkers=2` 运行；脚本 SHALL 检测其它 pi 会话高负载或并发 build/浏览器自动化时提示延后（提醒，不强制）

#### Scenario: 分片内测试全绿

- **WHEN** 某分片全部通过
- **THEN** 不产生任何台账记录变更，仅更新该分片的巡检进度

### Requirement: 归档语境欠账记账

在归档语境（§11 验证节实测、pre-push、归档门禁）执行测试撞见**非本 change 引起**的失败测试时，agent SHALL 将其登记台账（或确认已在台账）后方可继续归档流程——禁止以「不是本 change 改坏的」为由不记录不处理。本 change 范围内的失败仍按既有 §11 门禁要求必须修复。

#### Scenario: 归档撞见域外红

- **WHEN** 归档 change X 时，验证节实测发现失败测试属于 X 影响包之外
- **THEN** 该失败登记台账（发现语境记为 change X 归档），X 的验证节记录该失败已记账，归档继续

#### Scenario: 本 change 红不适用记账豁免

- **WHEN** 失败测试属于本 change 影响包
- **THEN** 不走台账记账通道，必须修复后归档（§11 既有要求不变）

### Requirement: 存量摸底先行

巡检机制落地前 SHALL 先完成一次受控存量摸底：分片执行前后端全部测试（停其它 pi 会话、限并发、不与 build 并行），产出 ① 当前存量红清单（直接登记台账作为初始欠账）② `go test -short ./internal/...` 全后端实测耗时 ③ 前端分片粒度与单片耗时数据。摸底数据 SHALL 记入本 change tasks.md 验证节，作为巡检分片粒度与还债排期的依据。

#### Scenario: 摸底产出初始欠账

- **WHEN** 摸底执行完成
- **THEN** 所有存量红测试已登记台账（发现语境记为「存量摸底」），清单规模记入 tasks.md

#### Scenario: 摸底数据决定分片粒度

- **WHEN** 全后端 `-short` 实测耗时确认
- **THEN** 后端是否整体一片（而非按 domain 拆片）以此数据定夺，结论记入 design.md
