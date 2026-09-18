# Test Cases — harden-test-scope-guard（复杂档白盒用例）

<!-- 对应 proposal.md 头部 complexity: complex；本文件是 case-first-testing（docs/reference/开发执行规范.md §2）复杂档要求的白盒用例清单，tasks.md 1.1 的产物。

用途：把判定契约机械展开为可判定的输入→期望分支，覆盖 delta spec（openspec/changes/harden-test-scope-guard/specs/doc-impact-gate/spec.md）全部 8 个 Scenario。
本文件只做机械枚举，不写实现、不写测试代码；实现阶段逐条落成 smoke 断言，命名若与建议缝不一致须回改本文件。

判据来源：主线程取证（docs/research/harness-gate-hardening/explore-findings.md pin:d30cecfe，15/15 误报还原）+ design.md D1–D7。 -->

## 0. 判据来源与落点

| 判据来源 | 取用内容 |
| --- | --- |
| pin:d30cecfe 取证 | 三类误报成因（正则层级混淆 10 / heredoc 正文 3 / 无 cwd 概念）、ctx_* 盲区 2 次真全量 |
| design.md D1 | 四步分层（文本掩蔽 → `-c` 递归 → 段切分与前缀剥离 → 包模式 + cwd 解析） |
| design.md D2/D3 | 裸 `./...` + `cwd == <repoRoot>/backend-go` 双条件；子树全量明确不拦 |
| design.md D5 | 放行语境四类（归档 / 逃生注释两别名 / 依赖变更）+ 文案改写 |
| design.md D7 | 变量展开类漏报按放行处理，不递归加固 |
| `change-scope.sh:120,158` | 影响包命令的官方形态 `go test -short ./internal/<域>/...`；go.mod/go.sum 变更时的「建议全量」提示语 |

**落点**：B1–B8 组 → `.pi/extensions/tests/policy-decision.smoke.cjs`（沿用既有 `tsgScenario` 的 esbuild `.tsg.cjs` + 真实事件形状回放 + 临时库 fixture；soft/hard/off 走既有子进程逐模式验证）。场景级映射见 tasks.md 验证节。

| 组 | 主题 | 条数 |
| --- | --- | --- |
| B1 | 调用段定位（切分/前缀/嵌套 `-c`） | 13 |
| B2 | 包模式提取 | 10 |
| B3 | 引号遮蔽、行内注释与转义 | 9 |
| B4 | 生效 cwd 解析 | 10 |
| B5 | 文本掩蔽（heredoc / 重定向） | 6 |
| B6 | 通道 × 模式矩阵 | 9 |
| B7 | 放行语境 | 7 |
| B8 | 记账与提醒行为（含低噪声与自噬防御） | 7 |

## 1. 前置实现契约（白盒用例依赖的缝）

| 缝 | 语义（固定） | 建议名 |
| --- | --- | --- |
| 判定纯函数 | 输入命令文本 + 会话 cwd + 仓库根，输出 `full` / `scoped` / `none`（`scoped` 与 `none` 行为等价，仅为可读性区分） | `classifyTestRun(command, sessionCwd, repoRoot)` |
| 仓库根解析 | `git rev-parse --show-toplevel`，按会话缓存；解析失败 → 判定退化为 `none` | `resolveRepoRoot(cwd)` |
| 命令文本提取 | 按 toolName 分派取待判定文本（bash→`input.command`；ctx_execute→仅 `language==="shell"` 取 `input.code`；ctx_batch_execute→`input.commands[].command`） | `extractCommands(toolName, input)` |
| 测试 fixture | 可构造 `{repoRoot}/backend-go` 目录树而不依赖真实仓库；**必须 `git init`**（否则 `resolveRepoRoot` 失败 → 判定退化 `none` → 所有 full 用例假红）；B4 组另需 `backend-go/internal` 子目录（会话 cwd 变体用） | smoke 内 `mkdtemp` + `git init` + `mkdir -p backend-go/internal` |
| repoRoot 解析注入 | 实现经 `pi.exec` 跑 `git rev-parse`；smoke 的 `makePi()` 只提供 `on`，需补 exec 桩（或直接注入 repoRoot 缝绕过 exec） | smoke 内 exec 桩 / `resolveRepoRoot` 可注入 |

**掩蔽契约**：文本掩蔽（引号内、`#` 注释、heredoc 正文、`cat` 重定向目标）发生在**段切分之前**，且掩蔽内容以等长占位符替换以保持位置稳定（便于错误定位）；掩蔽不整条短路，命令其余部分的调用段照常参与判定。

## 2. B1 — 调用段定位

| ID | 输入（命令，cwd 均为 `<repoRoot>`；`BW=<repoRoot>/backend-go`） | 期望 |
| --- | --- | --- |
| TC-B1-01 | `cd backend-go && go test ./...` | `full` |
| TC-B1-02 | `cd backend-go && timeout 600 go test ./...` | `full` |
| TC-B1-03 | `cd backend-go && TESTCONTAINERS_RYUK_DISABLED=true go test ./...` | `full` |
| TC-B1-04 | `cd backend-go && env FOO=1 go test -short ./...` | `full` |
| TC-B1-05 | `cd backend-go && go test ./internal/reader/... && go test ./...` | `full`（段内任一命中） |
| TC-B1-06 | `cd backend-go && timeout 1500 bash -c 'golangci-lint run ./... && go test ./...'` | `full`（递归 `-c` payload，含 `golangci-lint run ./...` 不干扰） |
| TC-B1-07 | `cd backend-go && go test -short ./internal/... \| tail -5` | `none`（管道右端不带 go test） |
| TC-B1-08 | `cd backend-go && golangci-lint run ./... \|\| go vet ./...` | `none`（无 go test 调用段） |
| TC-B1-09 | `cd backend-go && go test ./... > /tmp/x.log 2>&1; echo done` | `full` |
| TC-B1-10 | `cd backend-go && gofmt -l . && go build ./... && go test ./...` | `full` |
| TC-B1-11 | `cd backend-go && go test`（无包参数） | `none`（无模式参数，等同当前目录单包） |
| TC-B1-12 | `(cd backend-go && go test ./...)` | `full`（`(`/`)` 与 `;`/`&&` 同等切段，子 shell 不影响判定） |
| TC-B1-13 | `bash -c 'cd backend-go && go test ./...'` | `full`（cd 在 payload 内，递归层内正常解析——ctx_execute(shell) 最常见形态的白盒锚点） |

## 3. B2 — 包模式提取

| ID | 输入（`cd backend-go &&` 前缀省略） | 期望 |
| --- | --- | --- |
| TC-B2-01 | `go test ./...` | `full` |
| TC-B2-02 | `go test -short ./internal/reader/...` | `none`（**现状误报回归**） |
| TC-B2-03 | `go test -short ./internal/... ./cmd/...` | `none`（test-patrol 官方巡检形态） |
| TC-B2-04 | `go test ./internal/platform/articlerefs/... ./internal/reader/...` | `none` |
| TC-B2-05 | `go test -count=1 ./internal/admin/scheduler/` | `none`（单包） |
| TC-B2-06 | `go test -run 'TestFoo' ./...` | `full`（flag 值不影响） |
| TC-B2-07 | `go test -run 'TestDeleteFeedCascadeHandlesFeedWithoutArticles' ./internal/reader/repository/` | `none`（flag 值含无 `...`；包路径为单包） |
| TC-B2-08 | `go test -v -count=1 -short ./...` | `full` |
| TC-B2-09 | `go test ./... ./internal/...` | `full`（裸模式存在即可） |
| TC-B2-10 | `go test "$PKGS"` | `none`（变量，D7 漏报侧） |

## 4. B3 — 引号遮蔽、行内注释与转义

| ID | 输入 | 期望 |
| --- | --- | --- |
| TC-B3-01 | `grep -rn "go test ./..." docs/` | `none`（引号内不成命令） |
| TC-B3-02 | `rg 'go test ./\.\.\.' docs/reference/` | `none` |
| TC-B3-03 | `echo "run go test ./... in backend-go"` | `none` |
| TC-B3-04 | `cd backend-go && go test ./... # 全量回归` | `full`（`#` 后注释剥离，不影响判定） |
| TC-B3-05 | `cd backend-go && echo hi # go test ./...` | `none`（注释里的调用段不算） |
| TC-B3-06 | `cd backend-go && go test ./... ; echo "done # not a comment inside quotes"` | `full` |
| TC-B3-07 | `cd backend-go && echo "unclosed && go test ./..."` | `none`（未闭合引号 → 其后全部按引号内，保守放行） |
| TC-B3-08 | `cd backend-go && echo 'a\'b' && go test ./...` | `full`（转义边界不吞掉后续真实调用段） |
| TC-B3-09 | `cat > /tmp/x.md <<EOF\ncd backend-go && go test ./...`（无闭合 EOF） | `none`（未闭合 heredoc 保守掩蔽至命令串尾） |

## 5. B4 — 生效 cwd 解析

| ID | 输入 | 会话 cwd | 期望 |
| --- | --- | --- | --- |
| TC-B4-01 | `go test ./...` | `<BW>` | `full`（pi 起在 backend-go 时无 cd） |
| TC-B4-02 | `go test ./...` | `<repoRoot>` | `none`（仓库根无 go.mod） |
| TC-B4-03 | `cd backend-go && go test ./...` | `<repoRoot>` | `full` |
| TC-B4-04 | `cd /abs/path/backend-go && go test ./...` | `<repoRoot>` | `full`（绝对路径） |
| TC-B4-05 | `cd backend-go/ && go test -short ./internal/reader/...` | `<repoRoot>` | `none`（尾斜杠 + 影响包） |
| TC-B4-06 | `cd backend-go/internal/domain/x && go test ./...` | `<repoRoot>` | `none`（子目录子树） |
| TC-B4-07 | `cd backend-go && cd internal && go test ./...` | `<repoRoot>` | `none`（最后一个 cd 生效） |
| TC-B4-08 | `cd "$DIR" && go test ./...` | `<repoRoot>` | `none`（解析失败 → 放行侧） |
| TC-B4-09 | `cd backend-go && go test ./...` | `<BW>` | `none`（词法解析为 `<BW>/backend-go` ≠ 目标；真实 shell 中该 `cd` 失败短路，调用段不会执行——期望与算法、真实语义双一致） |
| TC-B4-10 | `cd .. && go test ./...` | `<BW>/internal` | `full`（`..` 规整：`<BW>/internal/..` ≡ `<BW>`） |

## 6. B5 — 文本掩蔽（heredoc / 重定向）

| ID | 输入 | 期望 |
| --- | --- | --- |
| TC-B5-01 | `cd <repoRoot> && cat >> openspec/changes/x/apply-report.md <<'EOF' … go test ./internal/reader/... … EOF` | `none`（**现状误报回归**：写文档不被拦） |
| TC-B5-02 | `cat > /tmp/x.md <<EOF\ncd backend-go && go test ./...\nEOF` | `none` |
| TC-B5-03 | `printf 'cd backend-go && go test ./...\n' > /tmp/y.sh` | `none` |
| TC-B5-04 | `cd backend-go && go test ./... && cat > report.txt <<'EOF'\nok\nEOF` | `full`（真实调用段在掩蔽内容之外，掩蔽 MUST NOT 吞掉整条命令） |
| TC-B5-05 | `cat go.mod` | `none` |
| TC-B5-06 | `cd backend-go && go test ./... \| cat > /tmp/log` | `full`（重定向目标掩蔽不提供豁免面，真全量不借豁免溜走） |

## 7. B6 — 通道 × 模式矩阵

| ID | toolName / input | 模式 | 期望 |
| --- | --- | --- | --- |
| TC-B6-01 | `bash` `{command: "cd backend-go && go test ./..."}` | soft | 提醒 1 次 + `policy.decision(warn, full-go-test)` 1 条，放行 |
| TC-B6-02 | `ctx_execute` `{language:"shell", code:"cd backend-go && go test ./..."}` | soft | 同 TC-B6-01（**现状盲区回归**） |
| TC-B6-03 | `ctx_batch_execute` `{commands:[{label:"a",command:"cd backend-go && go test ./..."},{label:"b",command:"go vet ./..."}]}` | soft | 提醒 1 次 + 记账 1 条 |
| TC-B6-04 | `ctx_execute` `{language:"javascript", code:"execSync('go test ./...', {cwd:'backend-go'})"}` | soft | 零提醒零记账（非 shell 不解析） |
| TC-B6-05 | `ctx_execute` `{language:"python", code:"subprocess.run('go test ./...')"}` | soft | 零提醒零记账 |
| TC-B6-06 | `bash` `{command:"cd backend-go && go test ./..."}` | hard | `{block:true}` + `policy.decision(block, full-go-test)`，reason 含两条逃生路径 |
| TC-B6-07 | `ctx_execute` `{language:"shell", code:"cd backend-go && go test ./..."}` | hard | `{block:true}`（通道一致） |
| TC-B6-08 | 任一通道真全量 | off | 零提醒零记账零阻断 |
| TC-B6-09 | `bash` `{command:"cd backend-go && go test -short ./internal/reader/..."}` | hard | 放行（影响包在 hard 下也不得拦） |

## 8. B7 — 放行语境

| ID | 输入 | 期望 |
| --- | --- | --- |
| TC-B7-01 | `cd backend-go && go test ./... # archive-gate` | 放行零记账 |
| TC-B7-02 | `cd backend-go && go test ./... # allow-full-test` | 放行零记账（新别名等价） |
| TC-B7-03 | 真全量 + 会话最近 15 条含 `归档` / `§11` / `pre-push` / `验证节` / `archive` | 放行零记账 + 附 test-patrol 记账指引 |
| TC-B7-04 | 真全量 + 会话最近 15 条含 `依赖变更（go.mod/go.sum）：建议全量 go test ./...（不自动执行）` | 放行零记账 + 附 info 登记指引（与归档语境同分支，design D5） |
| TC-B7-05 | 命令自身含 `# 依赖变更` 或 `建议全量` 文本 | 放行零记账 + 附 info 登记指引 |
| TC-B7-06 | 真全量 + 最近 15 条无任何放行标记 | 触发（软提醒/hard 阻断），文案同时给出归档与依赖变更两条合法路径 |
| TC-B7-07 | 影响包命令 + 最近 15 条含归档标记 | 零记账零提醒（放行语境不额外产生记账） |

## 9. B8 — 记账与提醒行为

| ID | 输入 | 期望 |
| --- | --- | --- |
| TC-B8-01 | 单次真全量（soft） | `policy.decision` 恰 1 条，payload 仅 `policy/action/reasonCode`（无命令正文、无路径） |
| TC-B8-02 | 同会话连续两次真全量 | 2 条 warn（不做同会话去重——去重属后续 hard 升级议题） |
| TC-B8-03 | UI 不可用（`ctx.ui.notify` 抛错）且 soft | 静默降级、放行、仍记 warn（既有行为不变） |
| TC-B8-04 | 无 `ctx.cwd` 或 `sessionManager.getSessionId()` 不可用 | 不记账、不抛错（既有 `auditPolicy` 旁路语义） |
| TC-B8-05 | 归档语境放行 | 1 条 info 提醒（test-patrol 登记指引），零 policy.decision |
| TC-B8-06 | 真全量且语言为 shell 的 ctx_execute 在 hard 下被拦 | 记账仅 1 条（block），不双写 warn |
| TC-B8-07 | hard block 后同会话重跑同命令，且最近 15 条含上次 block reason 文本（带 `[test-scope-guard]` 前缀） | 仍 block（守卫自身文案不构成放行语境，自噬防御） |
