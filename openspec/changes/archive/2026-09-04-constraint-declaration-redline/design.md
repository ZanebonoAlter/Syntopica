# constraint-declaration-redline Design

## Context

见 proposal.md Why 与 explore-findings.md（9 域约束节体积表、declaration 注入实测数据）。现状：declaration 注入拉约束节全文（spec 原文「声明域注入每回合从文件重解析」），keyword / jit-path 命中也是全节——同一节三种信号同一粒度。方向 B：约束节内做两级格式，声明注入只取红线层，细节层靠既有 jit / keyword 通道。方向 A（独立详设文档层）为长期演进，B 的红线句改写成果即 A 的概览层草稿。

## Goals / Non-Goals

**Goals**：declaration 单域注入 12.9KB → ≈1KB 级；多域 change 单回合约束块 12~16KB → 3~5KB；细节层可经 jit/keyword 到达；9 域格式统一进 doc-authoring 规范。

**Non-Goals**：不建详设文档层（A）；不动 keyword / jit 命中语义（仍全节）；不动 budget 降级分层顺序；不动只增不减粘性集合；不改 minSectionBytes 缺省值（512 复用于红线层下限）。

## 决策

### D1：红线层格式 = 约束节内顶层列表项的首个加粗块

格式规范（落 doc-authoring）：约束节内每条约束 MUST 以列表项（`N. ` 或 `- `）呈现，首词组加粗 `**...**` 为**自含红线句**（脱离上下文可独立理解、覆盖该条全部不变量的主干）；细节跟在红线句后。引用块（`>` 引导性说明）与自由段落不属红线层。

理由：topic-graph 等域现状已是 `N. **句**：细节` 半规整形态（样例见探索），改写是把不规整文档拉齐到既有形态，而非发明新格式——格式迁移成本最小，且人读体验不变差（红线句即该条的速览目录）。

### D2：提取纯函数 extractRedlines(section) → { lines, bytes } | null

提取规则：节内匹配顶层列表项行（`/^\s*(\d+\.|-)\s+/`），取该行首个 `**...**` 加粗块内容为一条红线句；无加粗块的列表项**不取首行文本凑数**（避免把细节句当红线），该条不出现在红线层——由此可能触发 D3 回退。返回 null 或 bytes < minSectionBytes（缺省 512，复用配置）时回退全节。确定性：同一节文本 → 同一输出字节序列（纯函数，smoke 直跑）。

### D3：回退全节的两个触发点与记账

① 红线层 0 条（文档未规整化 / 编辑中间态）；② 拼接低于 minSectionBytes。回退注入全节，`constraint.inject` payload 附 `layer` 标记（`redline` / `full`），bytes 如实。回退是降级而非报错——未改写的域照常全量注入，允许 9 域渐进改写（每改一个域，该域 declaration 立即瘦身，不需要一次全改完才生效）。

### D4：注入形态 = 红线句逐行 + 细节层指引尾行

红线层注入为各红线句逐行（保留原文顺序与编号），尾部一行：`细节层：read <doc 相对路径>「业务约束与不变量」节`。与既有「节尾附全文路径指引」同族，模型可自行补取。

### D5：9 域改写分三批，语义不变性为验收红线

批次按体积降序：① daily-report(12.9K) + data-enrichment(12.2K)；② discovery(7.6K) + reading(5.2K) + ai-summary(4.3K)；③ scheduler(4.0K) + content-enrichment(3.9K) + semantic-board(3.5K) + topic-graph(3.1K)。每批验收：每条约束首句加粗且自含；**只重组/提炼，MUST NOT 增删语义**（改写前 git diff 逐条对照）；引用块保留不动。改写挑 flow 文档安静窗口（无并行 change 正在动同名文档），冲突时 rebase。

### D6：效果核对（上线后 3~7 天）

`sqlite3 events.db` 按 reason=declaration 聚合：bytes 均值 6.3KB → 目标 <2KB；layer=redline 占比 → 目标 >90%（回退率 <10%）；jit-path 命中量观察是否补位（细节层需求真实存在的证据）。数据不达预期时红线句质量回炉，而非调阈值。

## 备选方案与否决理由

- **红线层取每个 `###` 子节标题**：否决——约束节内普遍无三级子节结构，强加子节是更大的文档手术。
- **无加粗列表项回退取首行整句**：否决——细节句冒充红线句比回退全节更糟（半红半黑混入注入层，模型无从分辨提炼度）。
- **红线层独立配置文件（红线条目与 flow 文档分离）**：否决——双真相源，flow 改约束红线层必漂移，正是本仓「单一真相源」原则的反面。

## Open Questions

（无——D1~D6 已定案；「自含」的判定标准以 D5 验收口径执行：脱离该文档上下文，单独读红线句能知道「什么 MUST / MUST NOT」）
