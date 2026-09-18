## Purpose

`test-scope-guard` 的职责是"拦住非归档语境下的全量 `go test ./...`"，但 2026-09-18 取证证明它的判定与职责不符：单行正则把 `./internal/<域>/...`（AGENTS.md 与 `change-scope.sh` 要求的正规动作）当成全量，15/15 条 warn 全是误伤；真·全量反而经 `ctx_execute` 通道溜走。本 delta 把"全量"从字符串巧合收紧为可机械核对的判定契约（裸 `./...` + `backend-go` 根 cwd），并补齐扫描通道与文本掩蔽，使该守卫的记账重新具备观测价值。

## MODIFIED Requirements

### Requirement: 全量测试软守卫（test-scope-guard）

pi 扩展 `.pi/extensions/test-scope-guard.ts` SHALL 仅在识别出**真·全量后端测试**时介入提醒：命令中存在以 `go` 为可执行文件、`test` 为子命令的调用段，其包参数为**裸** `./...` 或 `...`，且该调用段的**生效工作目录**解析为仓库根下的 `backend-go` 目录（取该段之前最后一个 `cd <path>`，相对会话 cwd 解析并规整；无 `cd` 时取会话 cwd）。判定 MUST NOT 依赖"命中 `...` 即全量"的单正则。

以下情形 MUST 判定为非全量并放行（零提醒、零记账）：`./internal/<域>/...`、`./internal/...`、`./cmd/...` 等子树或前缀模式；单包路径；从 `backend-go` 子目录发起的 `./...`；无 go.mod 的仓库根发起的 `./...`。

判定 MUST 先做 `bash -c` / `sh -c` 内联脚本递归：payload 抽取 MUST 基于**原始命令文本**（先于文本掩蔽，单/双引号作为 payload 定界符），递归判定时 MUST 继承 `-c` 调用点之前解析出的生效 cwd（payload 内自有 `cd` 时在递归层内正常生效）。本级文本 MUST 先做文本掩蔽：单引号/双引号内文本、行内 `#` 注释、heredoc（`<<` 至闭合标记，未闭合时保守掩蔽至命令串尾）正文与 `cat` / `cat >>` 重定向目标 MUST 掩为占位，MUST NOT 从掩蔽内容中提取测试调用；掩蔽 MUST 只作用于该部分文本，命令其余部分中的真实调用段 MUST 照常参与判定（如 `go test ./... && cat > report.txt <<EOF` 仍判全量，`go test ./... | cat > log` 不借豁免溜走）。段切分 MUST 把 `(`、`)` 与 `;`、`&&`、`||`、`|`、换行同等作为分隔符，并 MUST 剥离 `timeout` / `env` / `nice` / `command` 等前缀修饰与前置变量赋值。

扫描通道 MUST 覆盖 `bash`、`ctx_execute`（仅当 `language` 为 shell 时解析 `code`）、`ctx_batch_execute`（逐 `commands[].command`）；quality-gate 内部 `pi.exec` 执行的命令不在扫描范围（该链路只执行影响包测试）。

放行语境 MUST 包含：归档语境（命令或最近 15 条会话条目命中 `归档|§11|archive|验证节|pre-push`）、显式逃生注释（命令尾 `# archive-gate` 或等价别名 `# allow-full-test`）、以及依赖变更语境（命令或最近 15 条会话条目命中 `/依赖变更|建议全量/`——`change-scope.sh` 在 go.mod/go.sum 变更时输出的官方建议语）。放行语境与逃生注释的判定 MUST 作用于**原始命令文本**（文本掩蔽之前）。守卫自身注入的提醒/阻断文案 MUST NOT 构成放行语境（实现：文案统一携带 `[test-scope-guard]` 前缀，语境扫描过滤含该前缀的条目——防止首次阻断后的同会话自噬豁免）。命中任一放行语境时放行：零 policy.decision 记账、零 warn/block 警示（info 记账指引除外——归档与依赖变更语境放行均附测试欠账登记指引，同一分支）。

模式开关 `TEST_SCOPE_GUARD=soft|hard|off` 与默认值 soft 保持不变；soft 只提醒不阻断，hard 阻断并提供逃生注释指引；归档语境放行时附测试欠账登记指引（`scripts/harness/test-patrol.sh --register`）的行为保持不变。

（接受的能力边界）包模式来自 shell 变量、循环展开或命令替换（如 `for d in $DOMAINS; do go test ./$d/...`）时判定**可能判不出**，此类情形 MUST 按放行处理（宁可漏报不误伤）；本 Requirement MUST NOT 被解读为覆盖全部全量写法。

#### Scenario: 影响包命令不触发（现状误报回归）

- **WHEN** 会话 cwd 为仓库根，命令为 `cd backend-go && go test -short ./internal/reader/...`
- **THEN** 零 policy.decision、零 UI 提醒，命令正常执行

#### Scenario: 日常误跑全量测试触发提醒

- **WHEN** 生效 cwd 解析为 `<repoRoot>/backend-go`，命令为 `go test ./...`（含 `-short` / `-count=1` 等 flag，或 `timeout 600 go test ./...`、`X=1 go test ./...`、`bash -c '… && go test ./...'` 等形式）
- **THEN** soft 模式下发出软提醒（建议只跑影响包）并记一条 `policy.decision(warn, full-go-test)`，命令不被阻断

#### Scenario: 子树与子目录不算全量

- **WHEN** 命令为 `cd backend-go && go test -short ./internal/... ./cmd/...`，或 `cd backend-go/internal/domain/x && go test ./...`
- **THEN** 判定为非全量，零提醒零记账

#### Scenario: 写文件命令豁免

- **WHEN** 命令为 heredoc 写入 `apply-report.md`，正文中出现 `go test ./internal/...`
- **THEN** 零提醒零记账，写入不被阻断

#### Scenario: 引号内文本不构成命令

- **WHEN** 命令为 `grep -rn "go test ./..." docs/`
- **THEN** 零提醒零记账

#### Scenario: ctx 通道同样受守卫约束

- **WHEN** 真·全量命令经 `ctx_execute`（`language=shell`）或 `ctx_batch_execute` 的 `commands[].command` 发出
- **THEN** 与 bash 通道一致地提醒/记账（hard 模式下阻断）；经 `ctx_execute` 且 language 非 shell 时 MUST NOT 解析其代码文本

#### Scenario: 依赖变更语境放行

- **WHEN** 会话最近 15 条条目或命令命中 `/依赖变更|建议全量/`，且命令为真·全量
- **THEN** 放行：零 policy.decision、零 warn/block 警示，附 info 测试欠账登记指引（与归档语境同分支）

#### Scenario: 逃生注释与归档语境仍放行

- **WHEN** 真·全量命令尾带 `# archive-gate` 或 `# allow-full-test`，或命令/近期会话上下文命中归档语境
- **THEN** 放行零记账；归档语境放行时仍附测试欠账登记指引
