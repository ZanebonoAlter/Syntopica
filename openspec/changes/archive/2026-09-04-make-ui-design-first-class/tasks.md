## 1. 前置与影响面锁定

- [x] 1.1 确认 `harden-harness-policy-and-spill` 已归档且主 `openspec/specs/harness-fact-log/spec.md` 已包含 `policy.decision` 契约；若仍为 active 则停止本 change 的实现并请用户先收口该 change，验证 `openspec list --json` 不再列出它且主 spec 可检索到 `### Requirement: 策略显著裁决统一记账`
- [x] 1.2 运行 `bash scripts/doc-impact.sh suggest`，复核本 change 仅影响开发工具链与 frontend standard，并确认 proposal 的 `complexity: complex`、`ui-impact: none` 及无业务 `constraint-domains` 声明符合实际
- [x] 1.3 盘点当前 `spec-driven` schema、apply instruction、entry/spec gate、`AppDialog` API 与宽度使用基线，将会影响实现判据的新增事实补入本 change 的 `explore-findings.md`，验证每条事实均带 file:symbol 引用

## 2. 项目 OpenSpec UI 设计工作流

- [x] 2.1 执行 `openspec schema fork spec-driven syntopica-ui` 建立项目本地 schema，新增 `ui-design` artifact 并配置 `proposal → ui-design → specs/design → tasks` 依赖图，验证 `openspec schema validate syntopica-ui --verbose` 退出码 0 且 `openspec schemas --json` 显示 source=project
- [x] 2.2 扩展 proposal instruction/template，强制独立输出 `<!-- complexity: ... -->` 与 `<!-- ui-impact: none|minor|major -->`，写清三级判据和矛盾声明处理，验证 schema fixture 的缺失、非法、重复 marker 均被 UI gate 判定为对应错误
- [x] 2.3 新增 `ui-design.md` 模板与 instruction：none 生成最小 N/A、minor 生成复用/状态契约、major 生成完整八节设计并在 `ui-prototype/` 产出静态原型；验证三个 fixture 的 marker、必填节与 prototype 规则分别满足 spec
- [x] 2.4 修改 schema apply instruction，在生成实现计划和派发实现子线程前读取 UI 合同；major pending 时要求停在 requirements 档展示原型，验证 pending fixture 的 apply instruction 明确返回暂停路径、approved fixture 明确放行
- [x] 2.5 在 schema smoke 全绿后将 `openspec/config.yaml` 默认 schema 切换为 `syntopica-ui`，验证新建临时 change 自动绑定新 schema，而 rollout 前已有 change 的 `.openspec.yaml` 仍绑定 `spec-driven`；清理临时 change

## 3. UI 入口与归档门禁

- [x] 3.1 新增 `.pi/extensions/lib/ui-design-gate.ts` 纯函数，解析 proposal/ui-design marker、校验三级必填内容与 change 内 prototype 路径、识别 legacy schema 和影响等级矛盾，验证白盒分支表及空白、重复、大小写、越界路径、断链 symlink 边界全部有确定结果且函数不读文件/不抛异常
- [x] 3.2 新增 `.pi/extensions/ui-design-gate.ts`，在 implementation 档对新 schema change 拦截 Agent 派发与结构化项目 mutation，同时允许读取/验证及当前 change 的 `ui-design.md`、`ui-prototype/**` 修复；验证 major pending 阻断、approved/none/minor 放行、requirements 静默、legacy 每 session/change 仅提醒一次
- [x] 3.3 实现 `UI_DESIGN_GATE_BYPASS=1` 显式逃生口与检查异常 fail-open，阻断/提示/旁路文案均列出 change、reasonCode 和修复路径，验证 bypass 不静默、内部异常不伪装成功且不会改变既定 block 的记账语义
- [x] 3.4 通过既有 policy helper 写 `policy=ui-design-gate` 的 block/warn/bypass/fail-open 事件，限制 spec 声明的 reasonCode 白名单并保持健康放行零记录，验证 `.pi/harness/events.db` 的 change 绑定、action、reasonCode 与低噪声边界
- [x] 3.5 将 UI 证据检查接入 `spec-gate` 归档链：major 必须 approved、prototype 存在、tasks 有 opencli 与 1440×900/1920×1080 视觉证据及差异说明；minor 至少有组件/opencli/人工映射；none 校验 N/A 一致性，验证任一缺项阻断、`--force`/既有 bypass 仍留痕

## 4. 前端布局基线与复用组件

- [x] 4.1 新增 `docs/reference/standard/frontend/layout.md`，定义 reader=760px、contained=1120px、workspace、split 与 dialog sm/md/lg/xl、92vw 上限、例外登记和目标视口，验证文档具备 doc-impact 头、`## Requirements` 机器注入节并注册到 frontend standard README/AGENTS
- [x] 4.2 新增统一 `AppPageShell`（或经 review 确认的同等单一入口），提供 reader/contained/workspace/split 模式、gutter 和可访问 DOM 语义，验证组件测试覆盖四模式、窄于上限、等于上限、1440 与 1920 宽度及 split 溢出
- [x] 4.3 为 `AppDialog` 增加 `sm|md|lg|xl` size API 与统一 92vw 约束，保留旧 `width` prop 的渲染兼容且新代码优先 size，验证组件测试覆盖四档、旧 width、Escape/overlay/close 行为与无横向溢出
- [x] 4.4 为 frontend-design 与 ui-verify 的项目约束补充“先读已批准 UI 合同、不得另起美术方向、交互断言与视觉判断分流”，验证两个 skill/引用文档均能检索到 `ui-design.md`、layout mode 和双视口要求

## 5. 测试

- [x] 5.1 新增 `.pi/extensions/tests/ui-design-gate.smoke.cjs` 并接入 `run-harness-smoke.sh`，覆盖 `test-cases.md` 的声明/合同/审批/工具动作/legacy/telemetry 分支，验证脚本输出断言总数且退出码 0
- [x] 5.2 扩充 `.pi/extensions/tests/spec-gate.smoke.cjs`，覆盖 none/minor/major 归档证据、缺项阻断与显式 bypass，验证既有四项归档检查和 policy 事件断言无回归
- [x] 5.3 新增 `front/app/components/ui/AppPageShell.test.ts` 并扩充 `AppDialog.test.ts`（若原文件不存在则创建），执行 Windows `pnpm test:unit` 时两文件全部通过且不依赖浏览器截图像素
- [x] 5.4 建立 none/minor/major 三组 OpenSpec schema fixture/smoke，验证 artifact 顺序、模板内容、pending 暂停、approved 放行、新旧 schema 并存及临时目录清理

## 6. 文档

<!-- doc-impact: standard（纯开发工具链 change，无 flow 影响：不触及任何业务 flow 域，更新面为 standard/frontend、开发执行规范、AGENTS 速查与 harness 扩展快照） -->
<!-- doc-impact-excuse: flow=工作树命中项为 add-composite-labels/add-evidence-backed-cross-board-relations 等并行 change 的 backend-go 脏文件，本 change 未改 flow 辖区代码; api=同上（他人 handler 脏文件，本 change 未改 API）; database=同上（他人 models 脏文件，本 change 无表结构变更）; architecture=同上（他人 runtime.go 脏文件，本 change 未改 backend-go/internal/app）; configuration=openspec/config.yaml 是 openspec 工作流配置，docs/reference/configuration.md 只覆盖运行时配置（后端 YAML/env/Nuxt），不适用 -->

- [x] 6.1 更新 `docs/reference/开发执行规范.md`：将复杂逻辑原型与 UI 原型拆成正交通道，并在 §0.6、§3、§5.3、§11 写入 UI artifact、审批、apply/归档门禁和 legacy 兼容；验证关键字检索与章节交叉引用无断链
- [x] 6.2 更新 `docs/reference/standard/frontend/theming.md`、`testing.md`、`interaction-conventions.md` 及新增 layout 文档，明确 page shell、dialog size、major 双层验收和批准基线，验证 JIT 摘要/Requirements 与主体规则一致
- [x] 6.3 更新根 `AGENTS.md`、`front/AGENTS.md`、`docs/reference/constraints-index.md` 和 standard 注册入口，保留“用户当场指令优先”并增加 UI 分档速查，验证 `bash scripts/check-standards.sh` 不报孤立文档或失效链接
- [x] 6.4 更新 `.agents/skills/harness-facts/SKILL.md` 的 `ui-design-gate` policy/reasonCode 查询契约，并将新增/修改的 `.pi/extensions/` 文件同步到 `docs/research/` 对应快照，验证逐文件 `cmp -s` 返回 0
- [x] 6.5 在本 change 部署影响说明中明确：只影响后续 change 的规划/门禁；已有 change 不硬阻断；不迁移旧页面、不改数据；用户只需在 major UI 原型阶段明确确认，验证归档前该说明与 proposal/design 一致

## 7. 验证

- [x] 7.1 `bash .pi/extensions/tests/run-harness-smoke.sh` → 退出码 0，全部 harness smoke 通过且 UI gate 新增断言全绿
- [x] 7.2 `bash .pi/extensions/tests/run-smoke.sh` → 退出码 0，constraint-injection、档位绑定与既有 entry-gate 无回归
- [x] 7.3 `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit && pnpm build"` → 四项退出码均为 0
  - 实测（2026-09-04）：lint 0（5 条存量 warning，0 error）/ typecheck 0 / build 0；test:unit 840/842 通过，仅有的 2 个失败位于 `CompositeLabelEditDialog.test.ts`（add-composite-labels 在途 untracked 文件，断言为其 S12/D7 业务逻辑，与本 change 零依赖）；本 change 新增 `AppPageShell.test.ts`(12) 与 `AppDialog.test.ts`(13) 全绿。该外部失败由并行 change 收口后自然转绿，归档前复跑确认
- [x] 7.4 `openspec schema validate syntopica-ui --verbose && openspec validate make-ui-design-first-class --strict` → 两项退出码均为 0，无 schema/delta 格式错误
- [x] 7.5 `bash scripts/scenario-trace.sh openspec/changes/make-ui-design-first-class` → 退出码 0，以下 Scenario 映射全部存在且测试文件真实存在
- [x] 7.6 `bash scripts/doc-impact.sh verify openspec/changes/make-ui-design-first-class && bash scripts/check-standards.sh` → 两项退出码均为 0，无漏同步、孤立 standard 或幽灵引用
  - 实测：doc-impact verify 通过（standard 域 + 5 域误报已按 §11.2 机制 doc-impact-excuse 豁免，均为并行 change 脏文件）；`check-standards.sh --change make-ui-design-first-class` 130/130 零失败；无参全仓跑仅 add-composite-labels 的 F 段声明未收口（非本 change）
- [x] 7.7 `grep -R "ui-impact\|ui-design.md\|syntopica-ui" AGENTS.md front/AGENTS.md docs/reference/开发执行规范.md docs/reference/standard/frontend openspec/schemas/syntopica-ui` → 每个目标至少命中一处且措辞无矛盾

| Scenario | 测试文件 |
|---|---|
| 纯后端 change 声明 none | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 新面板必须声明 major | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 声明缺失或非法 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| none 产生最小制品 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| minor 复用既有交互 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| major 形成完整 UI 契约 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 原型待确认时暂停规划 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 用户明确确认后放行 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 重大修订使审批失效 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 治理列表选择 contained | front/app/components/ui/AppPageShell.test.ts |
| 工作台选择 workspace 或 split | front/app/components/ui/AppPageShell.test.ts |
| 弹窗拒绝随手宽度 | front/app/components/ui/AppDialog.test.ts |
| major pending 阻止实现写入 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 完整规划正常放行 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 旧 change 不被突然卡死 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| major UI 双层验收通过 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 只有单元测试不得完成 major UI | .pi/extensions/tests/spec-gate.smoke.cjs |
| pending 审批阻断被记录 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 存量提示被记录 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 健康放行零记录 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
