# Review 意见账（add-ui-prototype-editor-agent，2026-11-04）

> 审查方式：`change-review` 档子线程只读审查（制品 + 实现 diff + 门禁实跑），2026-11-04；审查报告全文见子线程会话存档。结论：High 0 / Medium 5 / Low 6；总评「核心命题独立验证成立，需修 5 项 Medium 方可归档」。
>
> 处置结果：2026-11-04 本轮全修（11/11 已修复，无驳回项）——Medium 5 条与 Low 6 条均已落地；涉及规范措辞的改动同步到 delta spec、AGENTS.md、开发执行规范.md 与 harness 机制文档。

### R-1 2026-11-04【Medium】规范与文档把「写侧边界」写成无条件保证，但 bash 是门禁未覆盖的写入通道

- **状态**：已修复
- **问题**：delta spec 与三处用户可见文档称「子线程的每次写入仍逐个过门禁」「其写入 MUST NOT 越出当前 change 目录」「越出 change 目录照常阻断」——该表述在 `bash`/`ctx_execute(shell)` 通道上不成立（`classifyTool` 只认 `Agent`/`edit`/`write`，bash 归 `other` 放行）。白名单里的 `ui-design` 档自身带 `bash`。
- **位置**：`specs/ui-design-workflow/spec.md`（放行条款 + 原型编辑档写入越界场景）、`specs/subagent-harness-coverage/spec.md`（原型编辑类条款）、`AGENTS.md` 档位/纪律段、`docs/reference/开发执行规范.md` §0.6 档位选择行
- **依据**：`classifyTool`（`.pi/extensions/ui-design-gate.ts`）对 bash 一律 `return null` 不介入；`decideUiToolGate` 的 `other` 分支 `return { action: "allow" }`。父会话的 bash 同样不过门禁——属 design D4 已披露的既有边界，本 change 不新增能力类别。
- **建议**：口径限定到结构化写工具——spec 与文档改为「经 `edit`/`write` 工具的写入 MUST NOT 越出当前 change 目录；shell 通道不在本门禁覆盖范围（既有边界，见 design D4）」。

### R-2 2026-11-04【Medium】放行理由「只读档无写工具」对 change-review 不成立

- **状态**：已修复
- **问题**：delta spec 与 lib 注释把 `change-review` 归入「只读档（无写工具）」，但其 `tools` 含 `bash`——物理上并非「无写工具」。该前提一旦被后续维护者引用会误判放行边界。
- **位置**：`specs/ui-design-workflow/spec.md`（放行条款正文）、`.pi/extensions/lib/ui-design-gate.ts`（`UI_DISPATCH_ALLOWED_AGENTS` 注释与 block 分支注释）、`design.md` D2
- **依据**：`grep -n "^tools:" .pi/agents/*.md` → `change-review.md` 为 `read, grep, find, ls, bash`；`explore.md`/`plan.md` 才是纯只读四件套。
- **建议**：改为可核验的事实陈述——「只读档（explore/plan：无写工具；change-review：只读工具 + shell，shell 通道为既有边界）」；design D2 补「白名单划分依据是**档位语义**而非工具集」。

### R-3 2026-11-04【Medium】本 change 自身的 ui-design.md 缺 `<!-- ui-prototype: none -->`，不满足它所修改的主 spec

- **状态**：已修复
- **问题**：主 spec 的 none 档场景要求 `ui-design.md` 记录 `ui-impact` / `ui-approval` / `ui-prototype` 三条 marker，本 change 只写了两条。实测按 `syntopica-ui` schema 送进 `evaluateUiContract` → `block / ui-design-missing`——一旦本 change 按迁移建议补上 schema，会被自己修改的门禁锁死。
- **位置**：`openspec/changes/add-ui-prototype-editor-agent/ui-design.md` 头部
- **依据**：主 spec「ui-design 作为必经规划制品」none 档条款；同仓库 `add-desktop-shell-remote/ui-design.md`、`add-ghost-mount-unmount/ui-design.md` 均含该 marker。
- **建议**：头部补一行 `<!-- ui-prototype: none -->`。

### R-4 2026-11-04【Medium】tasks.md 验证节记录两条不实结果，当前 archive-readiness 实为红

- **状态**：已修复
- **问题**：tasks.md 记 `review-trace.sh → 退出码 0（review.md 已建账）` 与 `constraint-injection.smoke.cjs → PASS`，均与实测不符（review.md 当时未建账；constraint-injection 有 2 项红）。
- **位置**：`openspec/changes/add-ui-prototype-editor-agent/tasks.md`「## 8. 验证」节
- **依据**：`review-trace.sh` → `✗ 未通过：缺 review.md 建账与豁免声明`，退出码 1；`run-smoke.sh` → constraint-injection 段 2 项红。
- **建议**：review.md 建账后 review-trace 转绿；constraint-injection 那条改为如实表述「2 项红，均为 flow 文档 `doc-impact-applies` 标签路径漂移（既存，与本 change 无关），已登记 test-patrol 台账」。

### R-5 2026-11-04【Medium】delta scenario「原型编辑档带扩展派发后写入受门禁校验」的前提无任何测试断言

- **状态**：已修复
- **问题**：该 scenario 的前提（原型编辑档 profile 声明 `load_extensions: true`）无断言——删掉该字段则全部测试仍绿，但子线程不再加载 UI 门禁扩展，写侧零执行，核心安全命题静默失效。与该 delta requirement 自称的「字段名漂移导致配置失效 SHALL 以实测暴露而非静默」冲突。
- **位置**：`specs/subagent-harness-coverage/spec.md`（原型编辑档场景）、`.pi/extensions/tests/ui-design-gate.smoke.cjs`（E5 仅查文件名存在）
- **依据**：`grep -rn "load_extensions|\.pi/agents" .pi/extensions/tests/*.cjs` → 全仓 smoke 无一条断言 `.pi/agents/*.md` 的 frontmatter。
- **建议**：补一条断言——`.pi/agents/ui-design.md` frontmatter 含 `load_extensions: true` 且 `tools` 含 `edit`/`write`。

### R-6 2026-11-04【Low】design D8 把 `skills: true` 当作「开 skills」的依据，该键在 pi-web 侧被忽略

- **状态**：已修复
- **问题**：pi-web 解析里 `skills` 只接受数组或非枚举字符串，布尔 `true` 落 `undefined`；真正生效的是 `load_skills: true`。行为无影响，但设计文档把机制指错了。
- **位置**：`design.md` D8
- **依据**：pi-web profile 解析 `loadSkills: v(e?.load_skills ?? e?.skills, false)`。
- **建议**：D8 改为「`load_skills: true`（生效键）；`skills: true` 为与既有档同形的惰性键」。

### R-7 2026-11-04【Low】tasks.md 3.3 引用的 runner 路径不存在

- **状态**：已修复
- **问题**：`bash scripts/harness/run-harness-smoke.sh` 路径不存在，实际在 `.pi/extensions/tests/run-harness-smoke.sh`。
- **位置**：`tasks.md` §3.3
- **依据**：`ls scripts/harness/` 无该文件。
- **建议**：改正路径。

### R-8 2026-11-04【Low】tasks.md §6 列的单文件命令不可独立复现

- **状态**：已修复
- **问题**：`node .pi/extensions/tests/*.smoke.cjs` 依赖 esbuild 临时 bundle（`.uig.cjs`/`.pdec.cjs`），单独跑会 `MODULE_NOT_FOUND`，须先跑 runner。
- **位置**：`tasks.md` §6「## 6. 测试」节
- **依据**：直跑 `node .pi/extensions/tests/policy-decision.smoke.cjs` → `Cannot find module './.pdec.cjs'`。
- **建议**：命令补注「（需先跑 `.pi/extensions/tests/run-harness-smoke.sh` 产出 bundle）」。

### R-9 2026-11-04【Low】worktree 隔离下门禁退化为 legacy，是本 change 放行派发新增的可达组合

- **状态**：已修复
- **问题**：`Agent` 工具支持 `isolation: "worktree"`，worktree 从 HEAD 建（未提交 change 目录不存在）→ `schema=null` → 合同判 legacy → 落回「front/ mutation 每会话 warn 一次、不阻断」。本 change 又放行了 `ui-design` 档派发，形成「派发放行 + worktree 隔离 → 写侧零执行」的新可达组合。
- **位置**：`.pi/extensions/ui-design-gate.ts`（快照按 `ctx.cwd` 拼接）、`lib/ui-design-gate.ts` legacy 分支
- **依据**：`evaluateUiContract` 首行 `if (input.schema !== UI_SCHEMA) return { kind: "legacy" }`。
- **建议**：在 design.md「已知边界」补记这一条（与 D4 的 bash 边界并列），本 change 不扩权处理。

### R-10 2026-11-04【Low】pi-extensions.md 通道矩阵的 ui-design 行未写 turn_end 降载行为

- **状态**：已修复
- **问题**：矩阵「门禁行为」列其它行都写了「turn_end 降载：零重命令放行 + `policy.decision(bypass, child-session)` 记账」，新增的 ui-design 行只写了写侧逐个过 ui-design-gate，漏了降载。该档 `load_extensions: true` → quality-gate 加载 → 降载语义适用。
- **位置**：`docs/reference/harness/pi-extensions.md` 子线程通道矩阵新增行
- **依据**：矩阵其它行均含该表述；`lib/child-session.ts` 注释说明 quality-gate 与 constraint-injection 共用同一父子判定面。
- **建议**：补「turn_end 降载：同项目覆盖档（child-session bypass）」。

### R-11 2026-11-04【Low】白名单漂移防护是单向的，新增只读档不会被自动纳入

- **状态**：已修复
- **问题**：E5 只断言「白名单每一项在 `.pi/agents/` 有同名文件」，不反向断言「`.pi/agents/` 中不具写工具的 profile 都在白名单」。后续新增只读档忘记同步白名单会被误拦（fail-closed 方向，但与本 change 要解决的「误伤」同类）。
- **位置**：`.pi/extensions/tests/ui-design-gate.smoke.cjs`（E5）、`.pi/extensions/lib/ui-design-gate.ts`（白名单常量）
- **依据**：E5 实现为 `UI_DISPATCH_ALLOWED_AGENTS.every((a) => agentFiles.includes(`${a}.md`))`，无反向遍历。
- **建议**：补一条断言——`.pi/agents/` 下 `tools` 不含 `edit`/`write` 的 profile 必须都在白名单内（frontmatter 文本粗判即可），失败方向为「报出待同步档名」。
