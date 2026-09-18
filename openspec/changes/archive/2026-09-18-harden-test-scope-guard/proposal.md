<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

`test-scope-guard` 现在用单行正则 `/go test[^#]*\.\.\./` 判"全量"：分不清 `./...` 与 `./internal/x/...`，不看工作目录、不看引号/heredoc，且只挂 bash 通道。2026-09-18 会话日志取证（warn 时间戳与 jsonl 里 bash toolCall 时间戳对齐 ±3–11ms，逐条还原命令）：可还原的 **15 条 warn 全是被误伤的非全量命令**——10 条影响包 `./internal/<域>/...`、3 条 heredoc 写 `apply-report.md`、2 条定向单测，**真·全量 0 条**；反方向 2 次真·根级 `go test ./...` 走 `ctx_execute` 完全未被看见。守卫处于"该拦的不拦、不该拦的全拦"状态，且因 soft 无牙齿长期未被察觉；此时翻 hard 会直接拦停日常影响包测试与归档文档写入，逼出逃生口滥用。根因与取证见 `docs/research/harness-gate-hardening/explore-findings.md`（pin:d30cecfe）。

## What Changes

- 判定重写为纯函数 `classifyTestRun(command, sessionCwd, repoRoot)` → `full | scoped | none`：引号遮蔽 + 递归 `bash -c`/`sh -c` 内联脚本 → 按 `;`/`&&`/`||`/`|`/换行/`(`/`)` 切段并剥离前缀修饰（`timeout`/`env`/`nice`/`command`/变量赋值）→ 定位 executable=`go`、子命令=`test` 的调用段 → **包参数为裸 `./...` 或 `...`，且生效 cwd 解析为 `<repoRoot>/backend-go`** 才判 `full`
- 文本掩蔽：heredoc（`<<`）正文与 `cat >`/`cat >>` 重定向目标按等长占位掩蔽，掩蔽内容不参与判定——写文档、写报告不再被拦；不整条短路（`go test ./... | cat > log` 仍可命中）
- 通道扩展：新增 `ctx_execute`（仅 `language=shell` 时解析 `code`）与 `ctx_batch_execute`（逐 `commands[].command`）；quality-gate 内部 `pi.exec` 执行的命令不在扫描范围（它本就只跑影响包）
- 放行语境新增"依赖变更全量"：命令或最近 15 条会话条目命中 `/依赖变更|建议全量/`（`change-scope.sh` 在 go.mod/go.sum 变更时输出的官方建议语）→ 放行
- 软提醒文案改写：区分“归档/pre-push 场景”与“确需全量（依赖变更）”两条合法路径；逃生口新增等价别名 `# allow-full-test`（`# archive-gate` 保留兼容）；reason/notice 统一携带 `[test-scope-guard]` 前缀且自身文案不构成放行语境（防首次 block 后同会话静默豁免的自噬回路）
- **不改**默认模式（仍 soft）、**不改** hard 语义与归档语境既有规则、**不做**前端同类守卫（另议）

## Capabilities

### Modified Capabilities

- `doc-impact-gate`：Requirement「全量测试软守卫（test-scope-guard）」的判定口径（裸 `./...` + `backend-go` 根 cwd）、文本掩蔽、扫描通道（bash + ctx_*）、放行语境（新增依赖变更）与提醒文案细化；`TEST_SCOPE_GUARD` 默认值维持 soft。原 Requirement 一句话的"命中全量 `go test ./...`"无法约束实现，本 change 将其展开为可机械核对的判定契约。

## Impact

- **代码**：`.pi/extensions/test-scope-guard.ts`（判定重写 + 通道扩展，估 +120~150 行）
- **测试**：`.pi/extensions/tests/policy-decision.smoke.cjs`（tsg 相关 5 场景 8 断言需改造：期望 warn/block 的 4 条（soft×2 + hard×2）原用 `mktmp()` 当 cwd、改后须落到 fixture 的 `backend-go` 才触发；归档指引用例 t4b 命令形态同步改为真全量，否则新判定下不命中、断言假绿）+ 新增判定矩阵用例（B1–B8 组 71 条）
- **文档**：`docs/reference/harness/pi-extensions.md`（全景表 test-scope-guard 行）、`docs/reference/开发执行规范.md` §4.1（补判定口径一句）
- **事实库**：`policy.decision(full-go-test)` 的 warn 频率应大幅下降（判定收窄后只该在真全量时出现）；该信号从"噪声"变为可用的观测指标
- **风险与边界**：包模式来自 shell 变量/循环展开/命令替换时判不出 → 按放行处理（明确接受，宁可漏报不误伤）
