# Design — add-spec-domain-index

## 背景

`openspec/specs/` 177 个 capability 无域归属。项目里两套文档体系只有一套有索引：`docs/reference/flow/` 的 10 个业务域被 `constraints-index.md` 索引并被约束注入消费；`specs/` 的 177 个 capability 谁都不认识。本 change 只补索引，不动正文。

## D1 域 marker 的格式与位置

- **结论**：`<!-- domain: a, b -->` 写在文件第一行（首个 `#` 标题之前），多域逗号分隔，上限 3 个。
- **落点**：`openspec/specs/spec-domain-index/spec.md`「main spec 的域声明」；`docs/reference/standard/shared/doc-authoring.md` 注册点速查表新增一行。
- **取舍**：不用 YAML frontmatter —— 项目既有头部注释惯例全是 HTML 注释（`constraint-domains` / `doc-impact-applies` / `ui-impact`），YAML 会引入第二套语法且可能干扰 openspec 解析。不放文末 —— 检索时先读头部，放文末要多读全文才能拿到。

## D2 词表封闭为 15 个（10 业务 + 5 工具）

- **结论**：业务域取 `docs/reference/flow/*.md` 的 basename（10 个）；工具域 5 个（`harness`/`ui`/`dev`/`api`/`db`）。
- **落点**：spec-domain-index spec 的词表；`domain-history.sh` 的 `DOMAINS` 常量。
- **取舍**：不自造一套命名（如把 `semantic-board` 简写成 `board`）——flow 域已是唯一真相源，两套名字会漂移。

## D3 历史归属走两跳推导，不回填 283 个 archive change

- **结论**：`archive change → delta capability → main spec domain`，一个 change 的域 = 全部 delta capability 的并集。
- **落点**：`scripts/harness/domain-history.sh`。
- **取舍**：直接回填 283 个 change 要读 283 份 proposal 判断归属，成本高且易错。实测 151/283 = 53% 已有 `constraint-domains`，但**含孤儿的 24 个 change 里只有 5 个有声明**——恰恰是最需要的那批最缺（R-3 更正，原写 8% 为误计）。两跳实测覆盖 488/526 = 93%，剩 38 个孤儿 delta（23 个已退役 capability）由第 6 段显式列出。验收方式：`domain-history.sh <域>` 第 6 段 + 人工决定是否补标。

## D4 脚本输出五段，退出码恒 0

- **结论**：本域 spec 清单 / 历史时间线 / flow 红线 / 可能冲突（历史 REMOVED）/ 必须沿用（历史 MODIFIED）。
- **落点**：`domain-history.sh` + `domain-history.smoke.sh`。
- **取舍**：第 4/5 段只罗列不断言 —— 机器判「是否冲突」需要理解语义，误判成本高于漏判；标注「提示性罗列」把判断权留给人/review。退出码恒 0 —— 嵌进自动化链路时不因「该域无历史」中断。

## D5 归类方法：模型初判 + 人工只复核低置信

- **结论**：177 个 capability 由子线程按「标题 + Requirement 列表」判定，输出 `h/m/l/?` 四级；人工只复核 `l` 与 `?`。
- **落点**：`spec-domains-draft.md`（本 change 内，随 change 归档）。
- **取舍**：纯关键词脚本实测不可用 —— `daily-report-system` 被判成 `semantic-board`（616 分 vs 533）、`board-signal-reports` 判成 `data-enrichment`、`api-docs` 判成 `scheduler`，全错。关键词对「出现频次」敏感而非对「职责」敏感。

## D6 门禁只 warn，不 block

- **结论**：新建 capability 时若合并后的 main spec 缺 domain marker → 归档门禁 warn 一次，不阻断。
- **落点**：`scripts/harness/check-standards.sh` 新增一段（或并入现有 F 段）。
- **取舍**：block 会卡住手上 4 个活跃 change（它们新建 capability 时还不知道这条规矩）；warn 累计两三周后按观察期结果决定是否转 block，与 `ui-stage-scope-out` 同一套节奏。

## 归类结果

见 `spec-domains-draft.md`（177 行全表 + 需人判清单）。**待人工复核 `l`/`?` 后定稿。**

## 风险

| 风险 | 影响 | 缓解 |
| --- | --- | --- |
| 177 个 marker 与 4 个活跃 change 的 delta 撞文件 | merge 冲突 | 只加一行在文件头，冲突可一键取两边 |
| 归类判错传播到后续 change 的 delta 定向 | 改错 spec | `l`/`?` 人工复核；marker 是提示不是门禁，改错可随时纠 |
| 孤儿 delta（38 个）长期无人认领 | 历史沿革缺口 | 首版就在 orphan 清单里显式列出，不静默吞 |
| 词表随业务演进需要新域 | 硬编码 15 个会过时 | 词表常量为脚本内单点，加域名是改一处；域清单本身以 `flow/*.md` 为准 |
