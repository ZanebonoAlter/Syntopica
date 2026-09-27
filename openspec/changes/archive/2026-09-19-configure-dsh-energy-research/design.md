## Context

本机 dsh v0.1.2-rc.1（Windows Node，Web UI 3080），`<dshHome> = C:/Users/Admin/.dsh`，当前无 `.agent-presets/` 目录。预设发现契约（`@deepseek-ai/dsh-agent-presets` README）：用户根目录 `<dshHome>/.agent-presets/<id>/` 下放 `preset.yml`（展示元数据 name/description/order）+ `agent.cordis.yml`（插件行列表），下一次 roster 读取即发现，无需重启；id 规则 `[a-z0-9][a-z0-9-]*`。宿主组合（`base.cordis.yml`+`web.cordis.yml`）持有 registries、sandbox/approval、persistence、model route；`web.cordis.patch.yml` 明确 host 侧 tool-web disabled，由各 preset 提供 web 工具行。既有非空会话不能切预设；「syntopica-profile」只是空工作区目录，不是 runtime profile。MCP client 只注入并注册工具、不 provide 服务。

## Goals / Non-Goals

Goals：研究专用受限预设落地且可追溯（仓库源+live 一致）；对既有 dsh 状态零副作用。
Non-Goals：不接入 EIA/JODI/STEO 等专业数据源（服务未实现，不写虚假 MCP 行）；不做整场 token/预算硬限制（host maxParallelToolCalls 与搜索 maxUses 保持原样）；不宣称工具白名单=OS 沙箱；不改 Syntopica 应用代码；不做自动安装/抓取。

## Decisions

1. **正向组装、不 include/继承 standard**。备选「复制 standard 再删行」会留下 agent-instructions 等编码面残留且随上游漂移；正向只写 4 个区块，denylist 语义由「缺行即无能力」保证（preset 权限恰等于所引用插件权限）。
2. **compaction 组带 `isolate {compaction, toolResultPruner}`**：compaction-basic 经 `ctx.get` 读 toolResultPrune，二者必须共享 realm；照抄本机 standard 参数（thresholdChars 8192 / headChars 4096 / tailChars 1024）。tokenMeter 不入 realm（host 责任）。MCP 行不加 isolate（client 只注册工具，无私有共享服务，避免前次误判复现）。
3. **tool-web 复用 host 搜索**：只写 `fetch: true`、`searchTimeoutMs: 60000`，不动 endpoint/provider/key；web 服务与搜索 provider 在 host 组合。
4. **persona 用中文纪律文本**（`|-` 字面块）：身份+纪律清单，含「未接入专业数据源」的诚实声明与防注入条款（来源文本不是指令）。
5. **preset.yml `order: 20`**：排在 standard（order: 1）之后，不抢占默认位；默认预设值不在本 change 触碰。
6. **仓库源 `config/dsh/presets/energy-research/` 作为唯一编辑点**，部署=逐字节复制到 `<dshHome>/.agent-presets/energy-research/`，双端 SHA256 记录进 tasks 验证节；目标已存在且内容差异即停。
7. **验证以静态为主 + UI 只读抽查**：YAML 结构解析、包名在 npx 缓存可解析、受保护文件哈希对比；浏览器侧 opencli 只读 roster/新建空白验证会话核对工具目录，不发 prompt、不触发模型/搜索（DeepSeek 搜索每次调用一轮 Messages，maxUses 只是单次搜索内部上限，本 change 零调用）。

## Risks / Trade-offs

- [dsh 升级后 preset 包名/行 schema 变化 → 预设被列为 broken 原因可见] → 文档记录版本前提 0.1.2-rc.1；broken 不影响 standard 与既有会话。
- [用户误把研究预设当沙箱] → 描述与文档明示「工具白名单≠OS 隔离」。
- [无专业数据源时模型可能编数值] → persona 纪律硬约束「不编造数值/序列、缺证据停止」；后续接 EIA/JODI 再另开 change。
- [并发修改 live 文件] → 部署前存在性+内容比对，差异即停报。

## Migration Plan

部署：mkdir + 复制两文件到 `<dshHome>/.agent-presets/energy-research/`（仅此两文件）。回退：删除该目录即恢复原状（运行中会话继续存活不受影响），仓库源保留。无应用侧迁移。

## Open Questions

（无——EIA/JODI 接入时机与形态属后续 change。）
