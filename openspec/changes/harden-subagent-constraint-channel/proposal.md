<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

仓库 harness 约束体系（constraint-injection 注入、quality-gate 增量门禁等扩展）对子线程不生效：pi-web Agent 工具的 agent profile 写死 `loadExtensions:false`（子会话 `noExtensions:true`，连 AGENTS.md 都不自动带），pi-subagents 前台子线程按设计不加载 ambient 扩展。2026-09-18 实测：后台子线程（「实现后端聚合服务与端点」）零 `constraint.inject` 事件、转录零注入痕迹，其 WIP 代码欠账（lint 中间态、测试红）堆在共享工作树上由主会话 turn_end 门禁兜出。子线程承担 §0.6 步骤3 的实现重活却是约束盲区——不知道红线、没有门禁、欠账归因混乱。

## What Changes

- **配置层（A）**：让需要的 agent profile 带上扩展
  - pi-web 通道：利用其 profile frontmatter 的 `load_extensions` 开关（解析白名单已含该字段），新增/修改项目侧 agent profile（`.pi/agents/`），供实现类任务派发使用；只读探索类档（explore 等）保持无扩展轻量
  - pi-subagents 通道：按需在 agent frontmatter 显式 `extensions:` 挂载（前台子线程亦生效）；后台子线程默认已加载，仅需确认无 `extensions: []` 覆盖
  - 不改动 pi-web 包本体（npm 编译产物，改了升级即丢）；配置全部落在仓库侧
- **扩展层（C）**：扩展感知子线程并按角色裁决行为
  - constraint-injection：子线程检测复用既有 parentSession 判定（`parentSessionId()`）；pi-web 通道子线程（开 `load_extensions` 后）与 pi-subagents 子线程统一走既有「显式继承父会话 + `mode.set source=inherit` 记账」链路；注入指纹随继承拷贝，防跨通道重发
  - quality-gate：子线程 turn_end 门禁降载（跳过重命令或采样执行），避免多子线程并发叠加 lint/test 压垮 4 核宿主；子线程欠账经事实库（edit.map / gate.check）仍可归因
  - 其余扩展（spec-gate / quota-gate / ui-design-gate / dev-process-guard 等）在子线程的行为逐一裁决并在设计文档记录，默认维持现状
- **文档沉淀**：机制结论入 `docs/reference/harness/pi-extensions.md`（扩展全景补子线程通道行）与 `docs/reference/开发执行规范.md` §0.6（派发要点：实现类任务用带扩展 profile）；研究底稿已存 `docs/research/subagent-constraint-gap/`

## Capabilities

### New Capabilities

- `subagent-harness-coverage`: 子线程 harness 覆盖——两条派发通道（pi-web / pi-subagents）的扩展加载配置约定、子线程门禁降载策略、通道行为矩阵（前台/后台 × 扩展开关 × 继承链路）

### Modified Capabilities

- `constraint-injection`: 子线程检测与继承语义从「pi-subagents 后台通道（独立进程 runner）」扩展到「pi-web 通道子线程（profile 开启扩展加载时）」；显式继承、`mode.set source=inherit` 记账、指纹拷贝防重发的要求保持不变，子线程检测来源增加 pi-web 子线程会话特征

## Impact

- **代码**：`.pi/extensions/constraint-injection.ts`（子线程检测/继承路径复用与补齐）、`.pi/extensions/quality-gate.ts`（子线程降载开关）+ `.pi/extensions/tests/` 对应 smoke 用例
- **配置**：`.pi/agents/` 下新增或修改 agent profile（带 `load_extensions` / `extensions` 声明）
- **文档**：`docs/reference/harness/pi-extensions.md`、`docs/reference/开发执行规范.md` §0.6、`docs/research/subagent-constraint-gap/`（已存在，设计阶段引用）
- **风险与边界**：
  - 子线程加载扩展后 events.db 转为多进程并发写（WAL + busy_timeout 5000ms 既有机制覆盖）
  - 多子线程并发 turn_end 的负载叠加由降载策略约束，设计文档定档
  - pi-web 升级若变更 profile frontmatter 白名单，配置层可能失效——依赖点在设计文档登记，并保留「编排层任务文本带红线摘要」作为不依赖配置的兜底纪律
