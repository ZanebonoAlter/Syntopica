# constraint-declaration-redline

> 工具链 + 文档格式 change。数据依据：2026-08-23 → 09-01 事实库复盘（declaration 注入实测均值 6.3KB/次、多域 change 单回合 12~16KB）。

## Why

declaration 注入（按 proposal 头 `constraint-domains` 声明拉 flow「业务约束与不变量」节全文）是多域 change 上下文开销的大头：9 个 flow 域约束节实测 3.0~12.9KB（daily-report 12.9KB、data-enrichment 12.2KB 为最重），声明 3~4 域的 change 单回合注入达 12~16KB 且常驻 system prompt。declaration 是机械全量注入（spec 自认"与当时正在实现什么无关"），而同仓的 JIT 路径命中（jit-path）早已实现按需拉节——同一注入体系内两套粒度并存，声明侧粒度失衡。方案取轻量两级：约束节内做"红线句层 + 细节层"格式分层，声明注入只取红线层，细节靠既有 jit-path / keyword 机制按需补全（方向 B；"再造详细设计文档层"的方向 A 作为长期演进，B 的改写成果直接是 A 拆分时的概览层草稿，不浪费）。

## What Changes

- **格式约定**：flow 文档「业务约束与不变量」节内每条约束整理为固定格式——首行为自含红线句（一句话、可独立理解、不改语义的提炼），细节跟后。红线句是既有约束内容的提炼，MUST NOT 新造或改变语义。
- **declaration 注入降为红线层**：constraint-injection 的声明域注入从"拉约束节全文"改为"拉约束节红线句层"（预计每域 12.9KB → ≈1KB 量级，多域 change 单回合 12~16KB → 3~5KB）。
- **细节层按需**：jit-path（编辑路径命中）与 keyword 命中仍注全节（细节层经既有机制到达模型），minSectionBytes 残缺节回退逻辑对红线层适配（红线层提取失败回退全节，不注入残缺内容）。
- **9 个 flow 文档约束节改写**：semantic-board / topic-graph / daily-report / content-enrichment / data-enrichment / ai-summary / discovery / reading / scheduler。
- **规范同步**：`standard/shared/doc-authoring.md` 补红线句格式规范（新文档/修订文档的约束节必循）；AGENTS.md 约束注入描述、harness-facts skill 的 reason 枚举说明同步。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `constraint-injection`: 「每 turn system prompt 强制注入」requirement 中声明域注入语义从"注入约束节全文"改为"注入约束节红线层（首行红线句），细节层经 jit-path / keyword 按需"；预算降级顺序（域声明节层）与 minSectionBytes 回退语义相应适配；`constraint.inject` 记账 bytes 按实际注入层级如实记录。

## Impact

- `.pi/extensions/constraint-injection.ts`（声明域节解析 → 红线层提取；gitignored 本机运行，快照同步 `docs/research/extensions/`）
- `docs/reference/flow/*.md` ×9（约束节格式改写，语义不变）
- `docs/reference/standard/shared/doc-authoring.md`（格式规范新增）
- `AGENTS.md`（约束注入段落）、`.agents/skills/harness-facts/SKILL.md`（declaration 说明）
- `openspec/specs/constraint-injection/spec.md`（delta 同步）
- `.pi/extensions/tests/` smoke（节解析纯函数用例）
- 风险：① 红线句提炼质量决定效果——提炼不当会把关键细节挡在模型视野外，需在改写时保证红线句自含；② 约束节改写是一次性 9 文档批量操作，与并行业务 change 冲突面大（改写期间归档的 change 若也动 flow 约束节需 rebase）；③ jit-path 是否足以承接细节层需求，上线后需用 constraint.inject 事件核对（declaration bytes 下降 + jit-path 命中是否补位）。
