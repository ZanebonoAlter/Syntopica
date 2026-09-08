<!-- complexity: simple -->
<!-- ui-impact: minor -->

## Why

用户在本地 dsh（DeepSeek Shell，v0.1.2-rc.1，端口 3080）上做原油供需研究，需要一个与研究场景匹配的 agent 预设：证据优先、只保留研究所需工具（网页检索/抓取、提问、压缩），不带编码 Agent 的 shell/文件编辑/委派等能力；当前 dsh 只有内置 `standard` 预设（全功能编码 Agent），没有面向研究的受限预设。同时该预设的配置源需要进仓库可追踪、可复现部署。

## What Changes

- 新增仓库配置源 `config/dsh/presets/energy-research/`（`preset.yml` + `agent.cordis.yml`），正向组装、不 include/继承 `standard`：
  - persona：中文能源研究员身份与研究纪律（先查事实再解释、统计期/地域/单位/来源清楚、区分观测/估算/预测、缺证据明确停止、不荐股不预测价格、不假装 EIA/JODI/MCP 已接入、不编造数值、缺可靠计算工具时如实说明）。
  - tool-web：`fetch: true` + `searchTimeoutMs: 60000`，复用 host 已配置的 DeepSeek 搜索，不改 endpoint/provider/key。
  - compaction 组：`cordis:group` + `isolate {compaction, toolResultPruner}`，内含 compaction-basic、tool-result-pruner（8192/4096/1024，照本机 standard）、command-compact。
  - tool-ask-user。
  - 明确不含：agent-instructions、bash/pwsh、fs/fs-search、str_replace_editor、jobs、skills 发现加载、goal/plan、delegation/workflow/ralph、todo；不含任何 MCP 行（EIA/JODI/STEO 服务未实现，禁止虚假端口/URL）。
- 部署为 dsh 用户预设：`C:/Users/Admin/.dsh/.agent-presets/energy-research/` 两文件，与仓库源逐字一致（记录 SHA256）；不改全局默认 preset/模型/权限/`.credentials.yaml`/`settings.yaml`/`profiles/web/**`。
- `docs/reference/configuration.md` 增补一节：路径、选用操作、旧会话与默认值保留、删除用户预设的回退方式、目前无专业数据源与整场硬预算、版本前提。

无 Syntopica 应用代码/接口/数据模型改动（**不改 front/ 与 backend-go/**）。

## Capabilities

### New Capabilities

- `dsh-research-preset`: 本地 dsh「能源研究」用户预设的行为契约——预设可发现且不影响默认、工具面只含预期研究工具、persona 不虚报专业数据源、仓库源与 live 部署逐字一致且部署不触碰受保护配置、文档含回退方式。

### Modified Capabilities

（无——不触及任何现有 openspec spec 的需求。）

## Impact

- 受影响文件：`config/dsh/presets/energy-research/**`（新增）、`docs/reference/configuration.md`（增补一节）、外部 `~/.dsh/.agent-presets/energy-research/**`（新增两个 live 文件）。
- 用户可见影响：dsh 预设选择器多出「能源研究」选项（在 dsh 自有 UI 的现有选择器结构内加一项，无 Syntopica 前端改动）；`standard` 预设与部署默认值不变；既有会话不受影响（预设只在新建/空会话可选）。
- 无数据迁移、无应用部署影响。
