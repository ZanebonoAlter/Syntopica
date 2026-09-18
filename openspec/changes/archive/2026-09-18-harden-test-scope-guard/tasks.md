# Tasks — harden-test-scope-guard

## 1. 判定纯函数（用例先行）

- [x] 1.1 白盒用例文档已定稿：`test-cases.md`（B1–B8 组 71 条 + 前置契约 §1，含评审补充：B1-13/B3-09/B8-07），复杂度声明 complex 已对齐（proposal 头）
- [x] 1.2 `classifyTestRun(command, sessionCwd, repoRoot) → "full"|"scoped"|"none"`：四步分层（`-c` 原文抽取 → 文本掩蔽 → 段切分与前缀剥离 → 包模式 + cwd 解析），纯函数、无 IO、无事件依赖
- [x] 1.3 `resolveRepoRoot(cwd)`：`git rev-parse --show-toplevel` + 会话内缓存；失败 → 判定退化 `none`（不抛错）
- [x] 1.4 掩蔽实现：等长占位替换（引号内、`#` 注释、heredoc 正文、`cat` 重定向目标；掩蔽不整条短路），未闭合引号与未闭合 heredoc 均保守处理（掩至串尾）（B3/B5 组钉住）
- [x] 1.5 段切分与前缀剥离：`;`/`&&`/`||`/`|`/换行/`(`/`)` 分隔；剥离 `timeout <n>`、`env`、`nice`、`command`、`X=Y` 赋值
- [x] 1.6 `-c` 递归：在**原始命令文本**（掩蔽前）抽取 `bash|sh|zsh -c <payload>`（单/双引号定界、支持 `\` 转义），payload 递归走全部分层，并**继承 `-c` 调用点之前解析出的 cwd**（payload 内自有 `cd` 时在递归层内正常生效，B1-06/B1-13 钉住）
- [x] 1.7 包模式提取：跳过 flag 取值，识别裸 `./...`/`...`；子树/单包/无参 → `none`
- [x] 1.8 cwd 解析：取段前最后一个字面 `cd`，相对本层基准 cwd（顶层=会话 cwd，`-c` 递归层=继承 cwd）规整（含 `..`、尾斜杠、绝对路径）；解析失败 → `none`
- [x] 1.9 判定为 `full` 时保留既有归档语境放行与逃生注释短路（`# archive-gate` / 新增 `# allow-full-test`），顺序：豁免语料判定在 `full` 之后（避免影响包命令被语境逻辑二次处理）；豁免判定 MUST 作用于**原始命令文本**（掩蔽前，注释里的 tag/关键词不能被剥掉）；reason/notice 统一携带 `[test-scope-guard]` 前缀，`hasArchiveContext` 过滤含该前缀的条目（防自噬，B8-07 钉住）

## 2. 通道扩展

- [x] 2.1 `extractCommands(toolName, input)`：`bash`→`input.command`；`ctx_execute`→仅 `language==="shell"`（或语言缺省）取 `input.code`；`ctx_batch_execute`→`input.commands[].command`；其余 toolName 直接返回空（零开销放行）
- [x] 2.2 `tool_call` 钩子改写：`toolName` 白名单从 `bash` 扩为三者；多命令（ctx_batch_execute）逐个判定，命中即按既有 soft/hard 逻辑处理
- [x] 2.3 确认 `quality-gate`/`entry-gate` 等其他扩展的 `pi.exec` 链路不受影响（不在 tool_call 通道，天然边界）

## 3. 放行语境与文案

- [x] 3.1 `ARCHIVE_CONTEXT_RE` 扩展为含 `依赖变更|建议全量`（沿用"命令文本或最近 15 条会话条目"机制，不新增代码路径）
- [x] 3.2 `SOFT_NOTICE` 改写：列出两条合法路径（归档/pre-push 场景；go.mod/go.sum 依赖变更全量），并给出两个逃生注释标签；reason/notice 统一携带 `[test-scope-guard]` 前缀（reason 现缺，补上）
- [x] 3.3 hard 模式 `reason` 文案同步（两个标签 + 两条合法路径）
- [x] 3.4 文件头设计决策注释同步（判定口径、通道、豁免、边界），保持与实现一致

## 4. 测试

- [x] T1 `test-cases.md` B1–B8 组落成 `.pi/extensions/tests/policy-decision.smoke.cjs` 断言（沿用 `tsgScenario` 的 esbuild `.tsg.cjs` + 真实事件形状回放；新增 fixture：`mkdtemp` + **`git init`** + `backend-go`/`backend-go/internal` 子目录，`makePi()` 补 exec 桩或注入 repoRoot 缝）
- [x] T2 改造既有 4 个用例：`go test ./...` 场景 cwd 由 `mktmp()` 改为 fixture 的 `backend-go`（否则新判定下不再触发，用例会假绿）；归档指引用例（t4b）命令形态同步改为真全量——新判定下 `go test ./internal/...` 不再命中，原「记账指引提示」断言必失败；归档注释用例改为同时覆盖 `# archive-gate` 与 `# allow-full-test`
- [x] T3 现状误报回归用例显式存在：影响包命令（B2-02/03/04）与 heredoc 写文档（B5-01）零记账零提醒——对应本 change 的取证结论
- [x] T4 通道盲区回归用例显式存在：`ctx_execute`(shell) 与 `ctx_batch_execute` 真全量触发；非 shell language 不触发（B6-02/03/04/05）
- [x] T5 `bash .pi/extensions/tests/run-harness-smoke.sh` 全绿（exit 0），无既有断言回归
- [x] T6 静态一致性：smoke 中断言名与 `test-cases.md` 用例 ID 可对账（grep 计数 ≥ 覆盖组数）

## 5. 文档

<!-- doc-impact: none(harness 机制文档不在 doc-impact 七域；pi-extensions.md 属 harness 扩展机制全景文档，随本 change 就地同步) -->
<!-- §12.2：无 flow 影响——本 change 仅改 pi harness 守卫扩展及其 smoke/机制文档，不触及任何业务 flow -->
<!-- doc-impact-excuse: flow=edit.map 归属集合被并行 change 脏文件污染（flow/reading.md 与 front/app/features/shell/AppHeaderView.vue 属 mobile-viewport-stage1，本 change 未编辑任何 flow 文件）; standard=standard/frontend/layout.md 属并行 change，本 change 实改的 开发执行规范.md 在 docs/reference/ 根不在 standard/ 七域路径 -->

- [x] D1 `docs/reference/harness/pi-extensions.md`：全景表 test-scope-guard 行更新——判定口径（裸 `./...` + `backend-go` 根 cwd）、扫描通道（bash + `ctx_execute`(shell) + `ctx_batch_execute`）、放行语境（归档 + 依赖变更 + 双逃生标签）、默认 soft 不变
- [x] D2 `docs/reference/开发执行规范.md` §4.1「全量测试软守卫」段：补判定口径与扫描通道一句话，保留「默认 soft（hard 才 block）」
- [x] D3 `openspec/specs/doc-impact-gate/spec.md`：由本 change 归档时同步 delta（判定口径/豁免/通道/放行语境）
- [x] D4 research 回链：`docs/research/harness-gate-hardening/explore-findings.md`（pin:d30cecfe）标注"判定与通道已由本 change 修复"，并保留取证数据
- [x] D5 文档内如需示例命令，MUST NOT 写成本仓库会真实执行的形态（文档里出现 `go test ./...` 属正常表述，不属守卫管辖）

## 6. 验证

- [x] V1 `openspec validate harden-test-scope-guard` → 输出 valid
- [x] V2 `bash .pi/extensions/tests/run-harness-smoke.sh` → 全部断言通过（exit 0；含 B1–B8 新增用例与 T2 改造后的既有用例）
- [x] V3 判定对账（人工，一次性）：用固定命令集合跑判定纯函数打印分类，逐条核对 `test-cases.md` 期望（影响包 3 条全非 full、写文档 1 条非 full、真全量 5 条全 full；非全量=行为放行，scoped/none 等价）
- [x] V4 取证复核（数据侧）：`sqlite3 .pi/harness/events.db "SELECT COUNT(*) FROM events WHERE kind='policy.decision' AND json_extract(payload,'\$.policy')='test-scope-guard'"` → 修复后新产生的 warn 只应源于真全量（若样本期内仍出现影响包形态，说明判定仍有漏 —— 需定位后再归档；存量 54 条均为旧判定历史记录，30 天保留期自然滚动）
- [x] V5 `cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...` → 无告警、构建成功（确认扩展改动未波及后端）
- [x] V6 `git status --short` → 本 change 变更仅含 `.pi/extensions/test-scope-guard.ts`、`.pi/extensions/tests/policy-decision.smoke.cjs`、`docs/reference/harness/pi-extensions.md`、`docs/reference/开发执行规范.md`、`openspec/changes/harden-test-scope-guard/`（docs/research/ 按 .gitignore:65 本地留档不入库；树上其余脏文件属并行 change，不碰）
- [x] V7 Scenario→测试映射对账（§11）：8 个 Scenario 逐条注明落点用例 ID，交由 `scenario-trace.sh` 校验；未映射 Scenario 视为未覆盖，不得归档

| Scenario | 测试文件 |
| --- | --- |
| 影响包命令不触发（现状误报回归） | .pi/extensions/tests/policy-decision.smoke.cjs |
| 日常误跑全量测试触发提醒 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 子树与子目录不算全量 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 写文件命令豁免 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 引号内文本不构成命令 | .pi/extensions/tests/policy-decision.smoke.cjs |
| ctx 通道同样受守卫约束 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 依赖变更语境放行 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 逃生注释与归档语境仍放行 | .pi/extensions/tests/policy-decision.smoke.cjs |

各 Scenario 落点用例 ID（B1–B8 白盒矩阵，均落 policy-decision.smoke.cjs）：影响包不触发 → TC-B2-02/B2-04/B4-05/B6-09/B7-07；真全量触发 → TC-B1-01–06/12/13、B2-01/08、B4-01/03/04/10、B5-04/06、B6-01/02/03、B7-06、B8-01/02；子树与子目录 → TC-B2-03、B4-06/07/09；写文件豁免 → TC-B5-01/02/03/05；引号内文本 → TC-B3-01/02/03/05/07；ctx 通道 → TC-B6-02/03/04/05/07、B8-06；依赖变更 → TC-B7-04/05；逃生注释与归档 → TC-B7-01/02/03、B8-05/07。
