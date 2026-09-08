# explore-findings — make-ui-design-first-class 实现基线

> 任务 1.3 盘点产物。以下事实直接决定 §2/§3/§4 实现判据；实现前不必重探。

## 1. OpenSpec schema 体系现状

- 本仓库当前**只有包级 schema** `spec-driven`（source=package，`openspec schemas --json` 仅一条）；`openspec/schemas/` 目录尚不存在。fork 落点为 `openspec/schemas/syntopica-ui/`（`openspec schema fork spec-driven syntopica-ui`）。
- 包级 schema 结构：`schema.yaml`（artifacts 列表：id/generates/template/instruction/requires + 顶层 `apply:` 节 requires/tracks/instruction）+ `templates/`（proposal.md/spec.md/design.md/tasks.md 四模板）。
  - 依赖图现状：proposal(requires:[]) → specs(requires:[proposal])、design(requires:[proposal]) → tasks(requires:[specs,design]) → apply(requires:[tasks], tracks tasks.md)。
  - 参考路径：`/root/.nvm/versions/node/v24.14.0/lib/node_modules/@fission-ai/openspec/schemas/spec-driven/schema.yaml`。
- change 绑定：`<changeDir>/.openspec.yaml`（如 `openspec/changes/make-ui-design-first-class/.openspec.yaml: schema: spec-driven, created: 2026-09-03`）；默认 schema 在 `openspec/config.yaml` 的 `schema: spec-driven` + `context:` 块（apply instruction 的 `context` 字段即来自此处）。
- schema 命令支持：`fork <source> <name> --force`、`validate <name> --verbose`、`which`、`init`。

## 2. pi 扩展体系（新 gate 的挂点与记账）

- 扩展自动加载：`.pi/extensions/*.ts` 全部生效，无显式注册表；快照同步 `docs/research/extensions/`（ts 文件）与 `docs/research/lib/`（lib 文件）。
- **spec-gate**（`.pi/extensions/spec-gate.ts:gateArchive`）：挂 `tool_call`（bash 命中 `openspec archive`）；四项独立检查（doc-impact verify / check-standards --change / tasks.md 尾三节+doc-impact 标记 / scenario-trace）+ 检查⑤措辞扫描（warn 级，`scanAcceptanceWording` 纯函数 export）；豁免 `--force`/`SPEC_GATE_BYPASS=1` 记 `policy=spec-gate action=bypass reasonCode=explicit-bypass`；健康放行零记账。任务 3.5 的 UI 证据检查应作为新检查项挂进 `gateArchive`（block 级），新增 reasonCode 需进 spec 白名单。
- **policy-decision**（`.pi/extensions/lib/policy-decision.ts:logPolicyDecision/normalizeDecision`）：action 白名单 `block|warn|bypass|fail-open`；reasonCode 必须 kebab-case ≤64 字符否则归一 `unknown`；target 截断 120 字符；change 传 `undefined`=自动检测（`lib/active-change.ts:detectActiveChange` mtime 最新 change）、`null`=明确不绑定；写入失败返回 false 不影响裁决。
- **entry-gate**（`.pi/extensions/entry-gate.ts`）：挂 `turn_end` 的 steer 软提示模式；档位从事实库 `queryBySession(cwd, sessionId, ["mode.set"])` 取最新行（payload.mode/payload.boundChange）；`session_start` 非 startup 清去重 Set（多窗口防御）；判定逻辑在纯函数 lib。**ui-design-gate（任务 3.2）拦截 tool_call 需要同一 mode.set 来源判断 implementation 档**。
- constraint-injection 写 mode.set（`.pi/extensions/constraint-injection.ts` L1065 附近，payload 含 mode 与 boundChange）。
- **smoke 体系**（`.pi/extensions/tests/run-harness-smoke.sh`）：esbuild 把每个 ts bundle 成 `.xxx.cjs` → `node *.smoke.cjs` 逐个跑 → 清理。新 ui-design-gate 需 bundle + smoke 文件 + 接入该脚本。`run-smoke.sh` 跑 constraint-injection 回归。

## 3. AppDialog 与宽度基线（任务 4.2/4.3 判据）

- **AppDialog**（`front/app/components/ui/AppDialog.vue`）：props 无 size API，`width?: string` 默认 `'480px'`，经 `:style="{ maxWidth: width }"` 注入 `.app-dialog`；基类 CSS `width: 90vw` + `max-height: 85vh`。任务 4.3 = 新增 `size?: 'sm'|'md'|'lg'|'xl'`（420/560/760/1040）+ 统一 92vw 上限，保留旧 width 兼容。
- **AppDialog 自由宽度调用点**（新代码禁用的模式，存量不迁移）：EditFeedDialog `width="672px"`、AnalysisMethodPanel `width="680px"`、AddSemanticBoardDialog/BoardEditDialog `width="520px"`、ArticlePreviewModal `width="90vw"`、TopicManageDialog 多处；组件内自带 Teleport/overlay 的反例：CompositeLabelEditDialog（design.md 证据）。
- **页面宽度基线**（分散现状）：无统一 page shell；DiscoveryPanel `max-width: 860px`、AiHealthBanner `min(92vw,640px)`、各页面自由散落；`front/app/pages/` 仅 4 页（index/tags/settings/discovery）。
- **组件测试约定**：co-located `*.test.ts`（如 `front/app/components/ai/AiHealthBanner.test.ts`）；`AppDialog.test.ts` 与 `AppPageShell.test.ts` 均不存在，需新建；跑法 `cmd.exe /C pnpm test:unit`（Windows native binding）。

## 4. 杂项

- `openspec validate <change> --strict` 现在就要求 delta spec 规范；本 change 有一个新 capability `ui-design-workflow`。
- scenario-trace.sh 要求 tasks.md 验证节有 Scenario→测试映射表（本 change 已预置，行尾三份 smoke/组件测试文件须真实存在）。
- doc-impact 域启发式：`openspec/config.yaml` 会误命中 `configuration` 域（`config.*\.ya?ml` 正则），但 `docs/reference/configuration.md` 只覆盖运行时配置——本 change 声明 `standard` 域即可，误报可在归档时以 doc-impact-excuse 豁免（如 verify 报 configuration）。
