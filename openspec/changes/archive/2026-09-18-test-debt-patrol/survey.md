# test-debt-patrol 受控存量摸底报告（2026-09-17）

> 对应 tasks 1.1–1.3。原始日志：`/tmp/tdp/survey/*.log`（本机临时，结论已在本文件与台账中固化）。
> 摸底前置：清掉 agent-browser/chromium 泄漏栈（load 7.2 → 3.5）；后端 dev server 按用户要求保留（实测 idle 0.7% CPU，不干扰）。

## 1. 结论速览

| 项 | 实测结果 |
| ---- | ---- |
| 后端 `go test -short -count=1 ./internal/... ./cmd/...` 全量 | **42s**，exit 0，39 包 ok / 11 包无测试文件 / **0 失败** |
| 后端逐片（5 domain + skeleton） | admin 5s、dataenrichment 10s、reader 12s、tagmanagement 4s、topicgraph 3s、skeleton 33s——**全绿** |
| 前端全量 100 个测试文件 | 4 worker 下 202–295s；`--maxWorkers=2` 分片实测合计 ≈ 20 min |
| 前端存量红 | **2 个文件 / 18 个用例**（见 §3） |
| 结论 | 存量红**全在前端**（后端 `-short` 干净）；分片粒度按 §4 |

> ⚠️ 摸底口径 = `-short`（跳过 DB 集成测试）。`go test ./...`（testcontainer PG）**未测**，仍是巡检盲区，作为已知边界登记（见 §5）。

## 2. 后端分片实测（`-short -count=1`，warm build cache，load ≈3.5）

| 分片 | 命令 | 耗时 | exit | 失败用例 |
| ---- | ---- | ---- | ---- | ---- |
| admin | `go test -short ./internal/admin/...` | 5s | 0 | 0 |
| dataenrichment | `go test -short ./internal/dataenrichment/...` | 10s | 0 | 0 |
| reader | `go test -short ./internal/reader/...` | 12s | 0 | 0 |
| tagmanagement | `go test -short ./internal/tagmanagement/...` | 4s | 0 | 0 |
| topicgraph | `go test -short ./internal/topicgraph/...` | 3s | 0 | 0 |
| skeleton | `go test -short ./internal/platform/... ./internal/models/... ./internal/app/... ./cmd/...` | 33s | 0 | 0 |
| （整片） | `go test -short ./internal/... ./cmd/...` | 42s | 0 | 0 |

**判据落地（design 决策 2「骨架片与 domain 片可合并跑」的定夺）**：全后端 42s ≪ 3 分钟阈值 → **后端整体一片（`backend-all`）成立**；同时保留 6 个 domain 分片静态枚举供细粒度巡检/定点复跑。脚本同时提供 `--shard backend-all` 与单片两种粒度。

cold cache 说明：42s 为 warm build cache；冷缓存首跑需额外编译测试二进制（树莓派上约 2–4 分钟量级），巡检连续跑时属一次性成本。

## 3. 前端存量红清单（本 change 初始欠账）

| test_id | 规模 | 现象 | 归因（初判） |
| ---- | ---- | ---- | ---- |
| `front/app/plugins/chunk-error-fallback.test.ts::<file-level>` | 整文件（collect 失败） | `Failed to resolve import "#imports" from "app/plugins/chunk-error-fallback.ts"` | vitest 配置无 Nuxt 别名映射（`#imports` 由 Nuxt 注入），测试文件与插件同源；**测试基础设施问题**，非产品缺陷 |
| `front/app/composables/useOnboarding.test.ts::<17 cases>` | 17 用例 | `Cannot read properties of undefined (reading 'clear')` / `(reading 'mockRestore')`；启动期 warning `localStorage is not available because --localstorage-file was not provided` | Node 26 实验性 `localStorage` 全局与 happy-dom 交互，测试内 `localStorage.clear()`/`reloadSpy` 拿不到实例；**测试环境问题** |

两者均**非本 change 影响范围**，按 spec「存量摸底先行」登记台账作初始欠账（发现语境 = 存量摸底），并在 6.1 按「小修」处置（存量 ≤5 处 → 本 change 内直接修，顺带验证 open→fixed 闭环）。

## 4. 前端分片实测与分片粒度定夺（`--maxWorkers=2`，正确调用形式）

### 4.1 ⚠️ 摸底同时发现的调用陷阱（必须修）

`pnpm test:unit -- <filter>` 里的 `--` 会把 filter **连同 `--maxWorkers=2` 一起吞掉**，静默跑成全量 100 文件：

```
> vitest run -- --maxWorkers=2 --reporter=basic app/features/tags/components
Test Files  2 failed | 98 passed (100)   ← 期望 43 个文件，实际跑了全量
```

实测对照（3 次）：带 `--` → 全量；不带 `--`（`pnpm test:unit <filter> --maxWorkers=2`）→ 精确命中 1 个文件。
`docs/reference/standard/frontend/testing.md` L49-53 与 `AGENTS.md` pre-push 行写的正是带 `--` 的坏形式 → **巡检分片必须用不带 `--` 的形式**，文档同 change 内改正（本 change 9.2）。

### 4.2 分片耗时

| 分片 | 目录组 | 文件数 | 耗时 | exit | 失败文件 |
| ---- | ---- | ---- | ---- | ---- | ---- |
| FE1 | `app/features/tags` | 43 | 197s | 0 | — |
| FE2 | `app/features/{discovery,articles,settings,ai,shell}` | 22 | 565s | 0 | — |
| FE3 | `app/{api,utils,stores,plugins,assets}` | 20 | 13s | 1 | chunk-error-fallback |
| FE4 | `app/{composables,components}` + 根级 2 文件 | 15 | 415s | 1 | useOnboarding |
| FE2a | `app/features/discovery` | 9 | 12s | 0 | — |
| FE2b | `app/features/{settings,articles,ai,shell}` | 13 | 143s | 0 | — |
| FE4a | `app/composables` | 6 | 4s | 1 | useOnboarding |
| FE4b | `app/components` + 根级 2 文件 | 9 | 15s | 0 | — |

**最终 6 片（已按实测定夺，供脚本静态枚举）**

| 分片 | 目录组 | 文件数 | 实测耗时 |
| ---- | ---- | ---- | ---- |
| fe-tags | `app/features/tags` | 43 | 197s |
| fe-discovery | `app/features/discovery` | 9 | 12s |
| fe-features | `app/features/settings app/features/articles app/features/ai app/features/shell` | 13 | 143s |
| fe-core | `app/api app/utils app/stores app/plugins app/assets` | 20 | 13s |
| fe-composables | `app/composables` | 6 | 4s |
| fe-components | `app/components app/error.test.ts app/spa-loading-template.test.ts` | 9 | 15s |
| （全量一圈合计） | — | 100 | **≈ 384s（6.4 min）** |

**耗时失真源（重要）**：FE2/FE4 的单文件用例耗时合计仅 5–7s，但整片 wall time 达 400–570s——日志里大量 `connect ETIMEDOUT <公网 IP>:443`（每次 TCP SYN 重试 ≈130s）的游离 fetch，挂在 worker teardown 上。即分片 wall time 被**真实外网超时**主导，与时序/网络环境强相关（同一 RPi，重跑可能快很多）。FE3 快是因为那组没有外网访问型测试。

### 4.3 判据（design 决策 2 回填）

- 分片粒度取实测目标为单片 2–4 分钟量级，按目录静态分组，落地 **6 片前端**（`fe-tags` 197s 最重、`fe-features` 143s，其余四片 ≤15s）——全量一圈 ≈6.4 min。
- 拆分后单片上界仍受外网超时噪声影响（无法靠分组消除）；脚本以 `--maxWorkers=2` 硬约束 CPU 侧并发（外网等待不占 CPU），符合「资源占用有上界」要求。
- 噪声实证：初版 4 片分组里 FE2（22 文件）实测 565s，拆半后 FE2a 12s + FE2b 143s = 155s——差值全在游离 fetch 的 TCP 超时等待，与分组无关，重跑波动大。降噪候选（不在本 change 范围，登记）：mock 掉外网 fetch / 加短超时。

## 5. 已知盲区（登记，不在本 change 出圈）

1. **DB 集成测试（`go test ./...`，需 Docker/testcontainer）**：巡检只用 `-short`，集成测试红仍不可见——与 test-scope-guard 劝退全量的既有事实同源。
2. **前端 typecheck / build**：巡检只跑单测，typecheck/build 仍归归档门禁与人工。
3. **外网依赖测试**：网络超时噪声会污染分片耗时（见 §4.2），不影响通过/失败判定。
