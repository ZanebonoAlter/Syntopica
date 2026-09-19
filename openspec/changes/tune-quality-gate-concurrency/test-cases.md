# Test Cases — tune-quality-gate-concurrency（复杂档白盒用例）

<!-- 对应 proposal.md 头部 complexity: complex；case-first-testing（docs/reference/开发执行规范.md §2）复杂档要求，tasks.md 1.x 的产物。

用途：把 delta spec（specs/gate-concurrency-control/spec.md，3 Requirement / 11 Scenario）机械展开为可判定输入→期望分支。只枚举不实现；实现阶段逐条落 smoke 断言，命名与建议缝不一致须回改本文件。

判据来源：docs/research/quality-gate-concurrency/explore-findings.md（7 天账本取证）+ design.md D1–D4。 -->

## 0. 判据来源与落点

| 判据来源 | 取用内容 |
| --- | --- |
| research 取证 | 341 冲突分钟 / foreign-breakage 7 天 0 命中 / 失败集中 go 侧（lint 562 / vet 465 / build 364） |
| design D1 | O_EXCL 抢锁 + mtime TTL 180s + finally 释放；探测短路轮不抢锁 |
| design D2 | GOMAXPROCS=2 前缀（native/windows 两形态）+ lint --concurrency=2 + vet/build 串行 |
| design D3 | classify 三态判定；mixed 进粘性、[并发] 前缀、双方路径 ≤3 条、指纹状态机复用 |
| design D4 | 新增 reasonCode：gate-lock-held（skip）/ concurrent-mixed（warn） |

**落点**：B1/B3/B4 → `.pi/extensions/tests/quality-gate.behavior.smoke.cjs`（沿用既有事件形状回放 + 临时库 fixture 模式）；B2 → `.pi/extensions/tests/quality-gate.smoke.cjs`（命令形态断言）。场景级映射见 tasks.md 验证节。

## 1. 前置实现契约（白盒用例依赖的缝）

| 缝 | 语义（固定） | 建议名 |
| --- | --- | --- |
| 抢锁 | 原子创建锁文件；返回 acquired（新建或 stale 覆盖）/ held（活性锁在）；锁内容含 sessionId、ts、当前 cmd | `acquireGateLock(repoRoot, sessionId, cmd)` |
| 释放 | 删除锁文件，任何异常（含 ENOENT——已被 stale 覆盖者删）吞掉 | `releaseGateLock(repoRoot, sessionId)`（签名较原稿多 sessionId：释放前比对防误删他方锁，B1-7 语义所需） |
| 三态判定 | 纯函数：paths ∩ mine/foreign 三态分流；paths 空、mine/foreign 传入异常时返回 mine（保守） | `classifyFailureOwnership({ paths, mine, foreign })` |
| mixed 行格式化 | [并发] 前缀 + cmd + 双方路径各 ≤3 条 + 超出计数 + 归档前全绿提示 | `formatMixedFailure(cmd, myPaths, foreignPaths)` |

## B1 锁生命周期（8 条）

| # | 输入/动作 | 期望 |
| --- | --- | --- |
| B1-1 | 锁文件不存在，acquireGateLock | acquired；锁文件内容 JSON 含 sessionId/ts/cmd |
| B1-2 | 活性锁存在（mtime 距今 < 180s，他人 sessionId），acquireGateLock | held；锁文件不被改写 |
| B1-3 | stale 锁（mtime 距今 > 180s），acquireGateLock | acquired；锁内容覆盖为本会话 |
| B1-4 | held 时 turn_end 路径 | 本轮门禁命令零执行、零 gate.check、零失败 steer；记 1 条 policy.decision（fail-open/gate-lock-held；原稿 skip 不可用——主 spec action 枚举固定四值，与 interop-down 同构）；既有 stickyFailures 原样保留（下回合锁空闲照常重跑） |
| B1-5 | acquired 后门禁命令全部完成 | releaseGateLock 删除锁文件 |
| B1-6 | 门禁命令抛异常/超时中途退出 | finally 仍释放锁（文件不存在于回合结束） |
| B1-7 | releaseGateLock 时锁已被他方 stale 覆盖（内容 sessionId 非本会话） | 吞异常不报错、不误删他方锁（比对 sessionId 再删） |
| B1-8 | interop-down / toolchain-down 短路轮、两侧均短路轮 | 不抢锁（未跑命令不挡他人），既有短路记账不变 |

## B2 限核命令形态（5 条）

| # | 输入/动作 | 期望 |
| --- | --- | --- |
| B2-1 | native 模式 runBackend 组装 vet/build/域测试 cmdline | 每条前缀 `GOMAXPROCS=2 `（bash -c 语境） |
| B2-2 | windows 模式 cmd.exe 组装 | `set GOMAXPROCS=2 && ` 前缀形态；`cd /d` 链路不破坏 |
| B2-3 | golangci-lint 命令 | 同时含 `--concurrency=2` 与 `--allow-parallel-runners` |
| B2-4 | vet 与 build 执行顺序 | 串行（build 的发起不早于 vet 结算；无 Promise.all 并发窗口） |
| B2-5 | 前端 eslint 命令 | 无任何改动（不带 GOMAXPROCS，参数原样） |

## B3 三态判定（5 条）

| # | 输入 paths/mine/foreign | 期望 |
| --- | --- | --- |
| B3-1 | P={a.go}，mine 含 a.go，foreign 含 b.go | mine |
| B3-2 | P={b.go}，mine={a.go}，foreign={b.go} | foreign |
| B3-3 | P={a.go,b.go}，mine={a.go}，foreign={b.go} | mixed |
| B3-4 | P={}（extractFailurePaths 解析不出） | mine（保守回退） |
| B3-5 | mine/foreign 构建异常（库不可用信号缺席路径） | mine（保守回退） |

## B4 mixed 报告/记账/粘性（8 条）

| # | 输入/动作 | 期望 |
| --- | --- | --- |
| B4-1 | mixed 首次失败 | failures 行以 `[并发] [cmd]` 前缀呈现；含他人路径与本会话路径分列（各 ≤3 条 + 计数）与「可能非本会话所致，归档前仍需全绿」文案；**不出现** [回归]/[中间态] 前缀 |
| B4-2 | mixed 失败后 | 该 cmd 进入 stickyFailures（下回合重跑） |
| B4-3 | mixed 同指纹第二回合 | ⟳ 单行抑制（不重灌完整块），rounds 递增 |
| B4-4 | mixed 命令转绿 | ✓ 已转绿收尾行，指纹条目清除 |
| B4-5 | 判性翻转 mixed→mine（他方文件被撤销，失败只剩本会话路径） | 恢复完整块 + 正常 [回归]/[中间态] 分级 |
| B4-6 | mixed 失败回合的账本 | gate.check ok=false 照记（diag 同源）+ 1 条 policy.decision（warn/concurrent-mixed，target=cmd） |
| B4-7 | 同回合 mixed 与 mine 失败并存 | 同一条 quality-gate-failure steer 消息内分列呈现 |
| B4-8 | foreign / mine 两极行为回归 | 与引入前既有断言逐条一致（既有 smoke 不动照绿） |
