### R-1 2026-11-20【Medium】样板图与原文事实对账

- **状态**：已修复
- **发现**：初稿把「count 校验不通过」画成了流程中断，与 `design.md` D6/风险节不符——原文明确「缺口不阻断退出码，只进报告」。同时「迁移状态对账」也只在报告层，不阻断。
- **处置**：流程图改为 `G -- 不匹配 --> G1[报告差异，人工判读]`，并在图后「图不表达的约束」写明「缺口不阻断退出码」。恢复链路六步（新建库 → 预装扩展 → TOC 过滤 → restore → count → 迁移对账）逐条对照 proposal 的 L3 描述，无遗漏。

### R-2 2026-11-20【Medium】规范落点改到开发执行规范而非 doc-authoring

- **状态**：已采纳
- **发现**：`docs/reference/standard/shared/doc-authoring.md` 的职责是「docs/reference 文档编写标准 + 三种头部注释机器接口」，而 `design-diagram.md` 是 change 制品且**不设门禁**。放进 doc-authoring 会让读者误以为它是机器接口，且要进「注册点速查」表，语义错位。
- **处置**：规范正文改放 `docs/reference/开发执行规范.md` 新增 §13（流程约定的家），§0.6 步骤 1 与 §9 产出物清单各加一句入口指引。proposal / tasks 同步改写。

### R-3 2026-11-20【Low】mermaid 语法校验不引入新依赖

- **状态**：已采纳
- **发现**：`npx @mermaid-js/mermaid-cli` 会拉 puppeteer/chromium（数百 MB），违反「不主动加工具」约束。
- **处置**：用前端已有 `front/node_modules/mermaid@12` + `happy-dom`（已有测试依赖）临时脚本解析，样板 3/3、§13 骨架 3/3 通过。脚本只放 `/tmp`，不进仓库、不进 CI。

### R-4 2026-11-20【Medium】防止图替代决策文字

- **状态**：已采纳
- **发现**：图表达最容易被滥用的场景，是拿一张流程图替代 `design.md` 的 Decision 三行，尤其是「取舍」——被否方案与不选理由几乎画不成图。
- **处置**：spec 的「图与文字分工」Scenario 明确 `design.md` 三行 MUST NOT 被图替代；§13 写作约束同步写入。归档 review 时把「图与实现是否一致」列为检查项（风险节已记）。
