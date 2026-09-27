## Context

`test-scope-guard.ts`（143 行）当前判定是单行正则 `FULL_GO_TEST_RE = /go test[^#]*\.\.\./` + 归档语境放行 + soft/hard/off 三态。取证结论（`docs/research/harness-gate-hardening/explore-findings.md` pin:d30cecfe，2026-09-18）：

- warn 事件 payload 不含命令，但 warn 时间戳与会话 jsonl 里 `bash` toolCall 时间戳对齐（±3–11ms）→ 可逐条还原。22 个 warn session 中 7 个仍有日志，还原 15 条，**15/15 为误伤**：10 条 `./internal/<域>/...` 影响包、3 条 heredoc 写 `apply-report.md`、2 条 `-run` 定向单测；真·全量 0 条。
- 反方向：语料去重后守卫可见 58 次（影响包/子树 49 + 写文件 9 + 真全量 0），而 **2 次真·根级 `go test ./...` / `go test -short ./...` 全走 `ctx_execute`**（守卫只挂 `toolName === "bash"`，:58），零拦截零记账。
- 若此时翻 hard：同 session 回放 13/15 被 block（含写报告文档）。

既有可抄先例：`quality-gate.ts:197`/`:271` 用 `git rev-parse --show-toplevel` 定位仓库根、`join(repoRoot, "backend-go")` 定位后端目录（:507）；`change-scope.sh:158` 在 go.mod/go.sum 变更时输出「建议全量 `go test ./...`(不自动执行)」--**项目自己的工具会推荐全量**,这是"合法全量"集合的一部分。仓库根无 go.mod(仅 `backend-go/go.mod`),故仓库根的 `./...` 本就秒失败,无需守卫。

## Goals / Non-Goals

**Goals:**

- 判定与职责对齐：只有"裸 `./...` + `backend-go` 根 cwd"算全量，其余（影响包/子树/单包/写文件/引号内文本）一律放行零记账
- 补齐通道：`ctx_execute`(shell) / `ctx_batch_execute` 与 bash 同等受约束
- 消灭现行 15 条误报的全部三类成因（正则层级混淆、heredoc 正文、无 cwd 概念）
- 判定逻辑可白盒断言（纯函数 + 分支表用例），使"守卫准不准"从猜测变为可回归

**Non-Goals:**

- 不改默认模式（仍 soft）、不翻 hard（先让记账证明判定准了再谈；hard 语义与逃生口不变）
- 不改 quality-gate / entry-gate / spec-gate 任何行为
- 不做前端（`pnpm test:unit`）同类守卫 —— 那是独立议题（判据不同于后端：有无过滤参数 + `--` 吞噬陷阱）
- 不追 shell 变量/循环/命令替换展开（见 D7）

## Decisions

### D1 判定分层：`-c` 原文抽取 → 文本掩蔽 → 段切分 → 包模式 + cwd 解析

每一层命令文本按以下四步判定，任一步判定为“非全量”立即返回 `none`，不再往下走：

1. **`-c` 结构抽取（基于原始文本，先于掩蔽）**：在**未掩蔽的原始命令文本**上识别 `bash|sh|zsh -c <payload>`（单/双引号作为 payload 定界符，支持 `\` 转义），payload 作为子命令**递归**走 1–4 步，并**继承 `-c` 调用点之前解析出的生效 cwd**（`cd backend-go && bash -c 'go test ./...'` 的 payload 内无 `cd` 也能判 `full`；payload 内自有 `cd` 时在递归层内正常生效）。抽取 MUST 先于第 2 步——等长掩蔽会把引号内文本换成占位符，掩蔽后就无从抽取；第 2 步的引号掩蔽只负责治 `grep -rn "go test ./..." docs/` 一类**非 `-c`** 的引号文本误伤，与本步把引号当定界符不冲突（两步读的文本不同：本步读原文，第 2 步读掩蔽产物）。
2. **文本掩蔽**：对抽取 `-c` 之后的本级文本做单次扫描，把三类文本掩为等长占位（未闭合引号按“其后全部视为引号内”处理）：① 单引号/双引号内文本（支持 `\` 转义）；② 行内 `#` 之后内容（shell 注释，治 `echo hi # go test ./...` 一类误判）；③ 未被引号掩蔽的 `<<` 开启的 heredoc 正文（掩至闭合标记行；**未闭合时保守掩蔽至命令串尾**）与 `cat >` / `cat >>`（`\bcat\b` 词边界）的重定向目标（治 3 条写 `apply-report.md` 的误报）。掩蔽内容 MUST NOT 参与调用段提取，掩蔽后无候选段 → `none`；掩蔽 MUST 只作用于该部分文本、不整条短路——`go test ./... && cat > report.txt <<EOF` 仍判 `full`，`go test ./... | cat > log` 不借豁免溜走。
3. **段切分 + 前缀剥离**：按 `;`、`&&`、`||`、`|`、换行、`(`、`)` 切段；每段剥离前置修饰（`timeout <n>`、`env A=B`、`nice`、`command`、`X=Y` 形式变量赋值）后，若首词为 `go` 且次词为 `test` 则该段是候选调用段。
4. **包模式 + cwd**：候选段中取非 flag 参数（跳过 flag 的取值，如 `-run 'X'` 的 `X`）；存在**裸** `./...` / `...` token 才继续。生效 cwd = 该段之前最后一个 `cd <path>` 相对**本层基准 cwd**（顶层为会话 cwd，`-c` 递归层为继承的 cwd）解析（无 `cd` 则取本层基准 cwd），规整后必须 **等于** `<repoRoot>/backend-go`。

`repoRoot` 用 `git rev-parse --show-toplevel`（与 quality-gate.ts:240 同源），结果按会话缓存（与 `execPlatform` 的"会话内稳定"先例同族）。

### D2 为什么用"裸 `./...`"而不是"含 `...`"或"包数量"

- 含 `...` 判定的证据已足：15/15 误报、其中 10 条是影响包——**这正是 AGENTS.md 与 `change-scope.sh` 要求的动作**（`add_target "go test -short ./internal/$d/..."`）。
- 包数量/耗时判定需要执行或解析输出，属于事后信息，无法在 `tool_call` 事前阻断，且引入输出解析复杂度；判定必须从事前可见的命令文本得出。
- 备选"更严口径"（把 `./internal/... ./cmd/...` 全量子树也拦）被否：那是 `test-patrol.sh` 官方巡检跑法（≈42s，与全量同价），拦它会常态化误伤——**代价是漏掉子树级全量，接受**。

### D3 cwd 必须参与判定（而不是只看模式）

`cd backend-go && go test ./...` 与 `cd backend-go/internal/domain/x && go test ./...` 的模式参数同为 `./...`，只有 cwd 能区分"全模块"与"单域子树"。仓库根无 go.mod 使"无 cd 的 `./...`"天然无害，无需额外特判。`cd` 目标是变量（`cd $DIR`）、或多段 `cd` 且存在条件分支时，取"最后一个字面 `cd`"并可能解析失败 → 解析失败一律按 `none`（D7 同精神）。

### D4 通道覆盖：bash + ctx_execute(shell) + ctx_batch_execute

两处真实溜走均发生在 `ctx_execute`。覆盖方式：在既有 `tool_call` 钩子里按 `toolName` 分派取命令文本——`bash` 取 `input.command`；`ctx_execute` 仅在 `input.language === "shell"` 时取 `input.code`（`language` 为该工具 schema 必填项，其余值一律不解析——宁漏报不误伤，也不做“缺省当 shell”的防御分支）；`ctx_batch_execute` 取 `input.commands[].command` 逐个判定。`quality-gate` 内部 `pi.exec` 不扫:它只执行影响包测试(`go test -short ./internal/<域>/...`),无全量可能;且它不经过 `tool_call`,属天然边界(写进 Non-Goals)。

### D5 放行语境：新增“依赖变更”，逃生口加别名

`change-scope.sh` 在 go.mod/go.sum 变更时输出「建议全量 `go test ./...`（不自动执行）」——不处理会让守卫拦掉项目自己推荐的动作。做法：把 `ARCHIVE_CONTEXT_RE` 扩展为 `/归档|§11|archive|验证节|pre-push|依赖变更|建议全量/`（同一套“命令或最近 15 条会话条目”机制，不加新代码路径）。同时：软提醒文案当前写“属归档/pre-push 场景”，对依赖变更场景是错的 → 文案改为列出两条合法路径（归档/pre-push；依赖变更全量）；逃生注释新增等价别名 `# allow-full-test`（`# archive-gate` 保留兼容，两个 tag 都进 spec）。

三个实现细节（防自噬，评审补充）：

- **豁免判定作用于原始命令文本（掩蔽之前）**：`# archive-gate` / `# 依赖变更` 都在 `#` 注释里，掩蔽后即被剥掉——语境/逃生 tag 匹配 MUST 读原始命令，不能读掩蔽产物。
- **守卫自身文案 MUST NOT 构成放行语境（自噬回路）**：hard 的 block reason 会以 tool error 身份回到会话条目，reason/软提醒文案里含「归档」「依赖变更」等关键词时，**首次 block 之后同会话的后续同命令会全部静默豁免零记账**（现行实现已存在此洞，本 change 扩关键词会放大它）。处置：reason 与 notice 统一携带 `[test-scope-guard]` 前缀（notice 现有、reason 补上），`hasArchiveContext` 扫描时过滤含该前缀的条目（TC-B8-07 钉住）。
- **依赖变更放行同样附登记指引**（与归档语境同一分支、零新代码路径）：全量跑出非本 change 的红测试同样该走 `test-patrol.sh --register`，语义自洽，不拆分支。

### D6 保持 soft：先让记账变成可用信号

判定修好后，`policy.decision(warn, full-go-test)` 才第一次具备"真·全量观测"的语义（此前是噪声）。因此本 change 不翻 hard：留一个观察窗口，用 warn 频率与内容验证判定准了（预期：真全量只出现在归档/依赖变更语境，而这两类已放行零记账 → 正常应接近零 warn），再由后续 change 决定是否 hard。反向风险（翻 hard 却仍误伤）已由 D1 的分层与 D7 的保守取向压低，但没必要在同一 change 里叠加两个行为变更。

### D7 明确接受的漏报边界（反 overfit）

不做：shell 变量/循环展开（`for d in $DOMAINS; do go test ./$d/...`）、命令替换（`go test $(cat pkgs)`）、`eval`、变量化 `cd`。理由：判定必须在**事前**、用**文本**完成；为覆盖这些形态只能靠猜或靠执行，前者制造新误报、后者改变守卫定位（事前拦截 → 事后判定）。取舍固定为**宁可漏报不误伤**，并写入 spec 的"接受的能力边界"段，防止后续把漏报当 bug 递归加固。

## Risks / Trade-offs

| 风险 | 处置 |
| --- | --- |
| 现有 smoke 4 个用例语义变更（用 `mktmp()` 当 cwd、`go test ./...` 期望 warn） | 用例改为 fixture 仓库根 + 真实 `backend-go` 目录（`mkdtemp` + `git init` 或注入 repoRoot 缝）；白盒用例文档 B5 组显式覆盖 |
| 引号遮蔽的未闭合引号 / 转义边界 | 未闭合引号按"其后全部视为引号内"处理（保守 → 少判 full，偏放行侧）；用 B3 组用例钉住 |
| `cd` 解析失败误判为 full | 解析失败一律 `none`（放行侧保守） |
| 判定收窄后被绕过（变量展开全量） | 接受并记录（D7）；该写法在 22 个 session 的语料里 0 出现 |
| 守卫自身 reason/notice 文案含豁免关键词 → 首次 block 后同会话静默豁免（自噬回路，现行实现已存在） | reason/notice 统一 `[test-scope-guard]` 前缀，`hasArchiveContext` 过滤含该前缀的条目（D5、TC-B8-07） |
| hard 模式下 ctx_* 阻断对 agent 的可用性 | 本 change 不改默认模式；hard 仍需显式设置 env，逃生注释指引随 reason 返回 |
