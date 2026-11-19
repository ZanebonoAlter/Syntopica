## Context

`constraint-injection` 扩展（`.pi/extensions/constraint-injection.ts`，2354 行）当前按「档位（mode）」+「关键词/JIT 路径命中」两条通道主动注入约束正文。实测基线（事实库 2862 条 `constraint.inject`）：

| reason | 条数 | 平均字节 | 最大字节 |
| --- | --- | --- | --- |
| declaration（红线层） | 878 | 3.6–5.1K | 9 040 |
| jit-path（**整节正文**） | 918 | 2.0–4.2K | **27 831** |
| index | 443 | 3.9–4.3K | 4 551 |
| mode-base | 348 | 3.4–4.3K | 4 551 |
| change-file | 139 | 2.6–4.5K | 9 353 |
| keyword（**整节正文**） | 136 | 0.1–3.9K | 23 103 |

本次改造的技术约束：

- **前缀缓存**：system prompt 任何字节变化 → 全部历史缓存失效（`archive/2026-09-16-harden-constraint-injection-channel/proposal.md:8`）。稳定层必须字节恒定，快照键粒度直接影响命中率。
- **红线提取单一实现**：`lib/constraint-redlines.ts` 的 `extractRedlines` 被扩展与 `scripts/harness/constraint-digest.sh`（esbuild bundle 后）双引，**禁止重写第二份正则**（`constraint-digest.sh` 注释明文）。
- **域清单单一真相源**：`docs/reference/flow/*.md` basename = `listFlowDomains` = `constraint-digest.sh` 的 `flow_domains`，三处同源。
- **marker 三态先例**：`lib/ui-design-gate.ts:167` 的 `parseUiStage`（missing / invalid / ok，缺失=legacy 不阻断）是仓库内既有的阶段 marker 范式，本设计沿用。
- **存量 change 零阻断**：仓库现有 11 个活跃 change 均无 `stage.md`，改造 MUST NOT 让它们集体卡死。

## Goals / Non-Goals

**Goals:**

- G1：openspec 进度有**唯一机器可读真相源**（`stage.md` 的 `<!-- stage: -->` marker），取代档位二值。
- G2：注入体量从「最坏 27.8KB 整节」降到「红线层 ≤3.1KB + 路由清单（十行内）」，正文由模型自主 read。
- G3：「该读的文档读没读过」可核验（`constraint.read` 记账 + 归档对账），不靠自觉。
- G4：存量 change 与存量事实库行**零阻断**平滑过渡。

**Non-Goals:**

- NG1：不改造 openspec CLI 本体（`stage` 不是 schema 制品，不进 DAG）。
- NG2：不引入 yaml 解析依赖（`.openspec.yaml` 走 `stage:` 字段的方案因此被否）。
- NG3：不改前端 / 后端业务代码（`ui-impact: none`，不动 `front/` 与 `backend-go/`）。
- NG4：不做「约束文档全文的向量化检索」——路由清单是显式契约，不是搜索引擎。

## Decisions

### D1. stage 标识载体：change 目录内独立 `stage.md` 文件

- **结论**：每个活跃 change 目录新增 `openspec/changes/<name>/stage.md`，头部一条 `<!-- stage: <值> -->` marker，取值域 6 档单调阶梯（`explore` → `propose` → `design` → `implement` → `verify` → `archive`）。缺失 = legacy（不阻断），非法/重复 = invalid（降级不阻断 + warn 记账）。
- **落点**：`openspec/changes/stage-driven-constraint-injection/specs/constraint-injection/spec.md`（Requirement「stage 进度标识（替代档位机制）」）；载体契约同文件；目录职责参考同仓库的 `explore-findings.md` / `human-feedback.md`（非 schema 制品）。
- **取舍**：**被否方案 A**：`tasks.md` 头部 marker —— `tasks.md` 在 design/tasks 阶段才生成，探索期与 proposal 期无载体可写，且 tasks.md 是会被反复改写的执行计划，进度标记混在里面会被覆盖。**被否方案 B**：`.openspec.yaml` 加 `stage:` 字段 —— YAML 与本仓「HTML 注释 marker」约定异构，且扩展无 yaml 依赖（`yaml` 解析需引入第三方包，违反 NG2）。**被否方案 C**：直接读 `openspec status --json` 的 `artifacts[].status` —— 每次 turn 开子进程（实测 200ms+/turn），且该 JSON 契约是 CLI 内部实现细节，跨版本不稳定；且它表达不了「planning 与 implementation 的分界」（`artifacts[].status` 只到 tasks 为止，apply 之后无 artifact 变化）。

### D2. 阶段族作为派生值，不作为独立可写状态

- **结论**：`stage` 是唯一可写状态；`stageFamily`（`planning` = explore/propose/design，`implementation` = implement/verify/archive）由 stage 映射**派生**。族只服务三处纯布尔判定（entry-gate 白盒用例提醒、ui-design-gate 实现期门禁、mtime 兜底绑定）。
- **落点**：`openspec/changes/stage-driven-constraint-injection/specs/constraint-injection/spec.md`（Requirement「stage 进度标识（替代档位机制）」场景「stage marker 生效」）；消费点在 `.pi/extensions/entry-gate.ts:53`、`.pi/extensions/ui-design-gate.ts:231`、`.pi/extensions/constraint-injection.ts:985`。
- **取舍**：**被否方案**：保留 mode 作为独立状态 + stage 作为第二个状态 —— 两套进度概念并存，正是本次要消灭的问题（proposal.md「Why」第 3 段）。族作为派生而非第二真相源，避免两者不一致。

### D3. 快照键用阶段族而非完整 stage

- **结论**：`stableSnapshotKey` 的第一段由 `mode` 改为 `stageFamily`（仍二值）。同族内阶段切换（`explore`→`propose`→`design`）不重建稳定层。
- **落点**：`openspec/changes/stage-driven-constraint-injection/specs/constraint-injection/spec.md`（Requirement「混合注入通道（稳定层 system prompt + 动态层 steer 消息）」场景「快照键用阶段族」）；实现点在 `.pi/extensions/constraint-injection.ts:1509` 的 `stableSnapshotKey`。
- **取舍**：**被否方案**：直接用 6 档 stage 值入键 —— 6 档让稳定层重建频率上升，而重建即破坏前缀缓存，代价与收益不成比例（planning 族内部切换不改变「该读哪些文档」的实质）。**无备选的部分**：族切换（planning↔implementation）仍需重建，因为路由清单内容确实变了。

### D4. 注入形态：红线层 + 路由清单，正文自主 read

- **结论**：退役 keyword 与 jit-path 两条整节正文通道。改为：① **阶段首推**——某 stage 首次成为当前阶段时，以增量 steer 消息送达该 stage 的红线层（声明域 flow 文档「业务约束与不变量」节的顶层加粗句逐行）+ 路由清单（文档路径 + 节名 + 取数方式）；② 同阶段内零重复注入；③ 路由清单同时给出 `bash scripts/harness/constraint-digest.sh --brief --domains <域>` 命令形式。
- **落点**：`openspec/changes/stage-driven-constraint-injection/specs/constraint-injection/spec.md`（Requirement「阶段驱动的约束路由与自主导入」全部 5 个场景）；红线提取实现在 `.pi/extensions/lib/constraint-redlines.ts:32`；清单装配新增于 `.pi/extensions/lib/stage.ts`。
- **取舍**：**被否方案**：保留整节注入但降低频率 —— 27.8KB 的问题不在频率在体量，一次整节就把模型上下文预算吃掉一大块，且「模型自己 read 一节 12KB 文档」比「harness 塞 12KB 字节」更省（模型只读需要的段落）。**被否方案**：完全不给清单、纯靠模型自觉 —— 无路由即无约束，退化成 2026-11-06 已证伪的「规范引导路线」（harness-retro ⑦G 实测派发带域比例 29% < 70% 判据）。**被否方案**：read 触发式自动补齐全文 —— 实质仍是 harness 塞正文，且与 read 事件耦合，复杂度不换收益。

**退役的全局性**：keyword 与 jit-path 两条整节正文通道的退役对**全部** change 生效（含无 `stage.md` 的 legacy change）—— 退役的是「主动塞正文」这一形态本身，不是某个阶段的特性。legacy change 的退化形态是「索引 + 声明域红线层」（无阶段路由清单），不阻断任何流程。

### D5. 路由清单按 stage 派生，配置与 stage 绑定

- **结论**：清单构成由 `.pi/constraint-injection.json` 的 `stages` 段配置（替代现 `modes` 二分桶）：每 stage 声明「必读文档集合 + 按栈条件文档」。planning 族给业务红线 + 索引；`design` 加 UI 契约规范（`standard/frontend/layout.md`、`interaction.md`）；`implement` 加代码与测试规范（按栈命中 `standard/backend/` 或 `standard/frontend/`）；`verify`/`archive` 加验收与归档规范（`standard/shared/test-design.md`、`commit-pr.md`、`doc-authoring.md`）。
- **落点**：`openspec/changes/stage-driven-constraint-injection/specs/constraint-injection/spec.md`（Requirement「阶段驱动的约束路由与自主导入」表格）；配置改造点在 `.pi/constraint-injection.json`。
- **取舍**：**被否方案**：清单写死在代码里 —— 阶段与文档的对应关系是「仓库知识」不是「程序逻辑」，放配置才能随规范演进调整而不动扩展代码。**被否方案**：清单由 agent 自己写在 `stage.md` 里 —— 那样每次 change 都要人肉维护一份清单，且清单与实际注入可能不一致（无法机器对账）。

### D6. 读取记账 + 归档对账（兜底防裸奔）

- **结论**：`read` 工具事件拿到路径 → 记 `constraint.read` 事实库事件（payload：文档相对路径 + boundChange）。归档对账脚本 `scripts/harness/constraint-read-trace.sh <change-dir>` 检查：该 change 声明的**每个**业务域，其 flow 文档 MUST 在实现期内被 `read` 过；未读 → 红项 + 取数指引。legacy change（无 `stage.md`）豁免。
- **落点**：`openspec/changes/stage-driven-constraint-injection/specs/constraint-injection/spec.md`（Requirement「约束文档读取记账与归档对账」全部 4 个场景）；脚本接入 `scripts/harness/archive-readiness.sh` 与 `.pi/extensions/spec-gate.ts`。
- **取舍**：**被否方案**：只记账不阻断 —— 退化无法回检，等于回到「纯靠自觉」。**被否方案**：要求读过**特定节**而非整文档 —— read 工具是整文件读（无节粒度），要判定节级需解析文件内容，代价高收益低；整文档读即算读。**取舍点**：对账只查「声明域的 flow 文档」，不查 standard 规范 —— standard 规范按栈条件命中，若某 change 恰好不碰后端却要求读后端测试规范，属误报；声明域是 change 自己签字承诺的范围，对账它最扎实。

### D7. 事实库事件改名与双读过渡

- **结论**：`mode.set` 停写，改写 `stage.set`（payload：`stage` / `stageFamily` / `boundChange` / `source`）。读方过渡期双读：先查 `stage.set`，无则查 `mode.set` 并按 `requirements`→`planning`、`implementation`→`implementation` 映射回族。既有行不改写（30 天保留期自然消退）。
- **落点**：`openspec/changes/stage-driven-constraint-injection/specs/harness-fact-log/spec.md`（Requirement「事件类型词汇与保留期」+「档位记账（mode.set）」）；事件 kind 枚举在 `.pi/extensions/lib/harness-log.ts:97`。
- **取舍**：**被否方案**：直接改名不做双读 —— 改造当天所有 resume/reload 会话与跨进程子线程继承全断，且断链症状隐蔽（不报错，只是约束变少）。**被否方案**：保留 `mode.set` 名字只改 payload —— 名字与语义不符会让 `harness-retro.sh` 的档位统计口径继续误导。

### D8. 跨进程子线程继承保留 boundChange 通道

- **结论**：`stage.set` 保留 `boundChange` 字段，子线程读**父会话**最新 `stage.set` 完成继承（复用既有 `inheritFromParentHistory` 结构，只把 `mode` 换成 `stage`）。stage 值随父会话快照写死，不在子线程重算。
- **落点**：`openspec/changes/stage-driven-constraint-injection/specs/constraint-injection/spec.md`（Requirement「会话作用域状态隔离」子线程场景）；实现点在 `.pi/extensions/constraint-injection.ts:352` 的 `inheritFromParentHistory` 与 `.pi/extensions/lib/child-session.ts:60`。
- **取舍**：**被否方案**：子线程自行 `detectActiveChange`（mtime 启发式）—— `.pi/extensions/lib/edit-map.ts:11-12` 已明说 mtime 启发式「不适合归属判定」，多 change 并行会绑错。**被否方案**：子线程从自身任务文本解析 change 名 —— 与事实库通道割裂，派发文本格式一变就断。

## Risks / Trade-offs

| # | 风险 | 症状 | 缓解 |
| --- | --- | --- | --- |
| R1 | 阶段 marker 与实际制品状态脱节（agent 改了代码但没更新 stage.md） | 该阶段该读的文档没被路由，读了无关文档 | 路由清单按 stage 派生（不依赖 marker 之外的信号），模型仍可按需 read；归档对账兜底声明域（D6） |
| R2 | 存量 change 无 stage.md → 按 legacy 处理，注入面收窄（无阶段路由清单，且不再有 keyword/JIT 整节正文） | 老 change 体验变薄，但不会裸奔（仍有索引 + 声明域红线层） | 预期行为（D4 末段「退役的全局性」）；迁移可后置：给存量 change 补 `stage.md` 即获得阶段路由，不补也不阻断 |
| R3 | `mode.set` → `stage.set` 改名当天读方漏改 → 子线程退化为未绑定 | 约束块缺失且**不报错**（隐蔽） | D7 双读过渡 + smoke test 覆盖 `source=inherit` 路径（`.pi/extensions/tests/constraint-injection.smoke.cjs:860`） |
| R4 | 路由清单随 stage 增长导致稳定层变大 | 前缀缓存收益下降 | 清单只含路径与节名（十行内），不含正文；快照键按族（D3） |
| R5 | `constraint.read` 记账量随 read 调用频次增长 | 事实库体积上升 | 只在命中「约束文档路径白名单」时记账（不是所有 read）；30 天保留期；100MB 保险丝兜底 |
| R6 | 对账口径只认 `read` 工具事件，模型用 bash `cat` 读了文档不被计入 | 误报未读 → 归档被拦 | 记账白名单覆盖 `read` 工具；bash 读取**不计**（无法可靠归因路径）—— 取数指引让模型用 `constraint-digest.sh` 或 read；误报成本为「补一次 read」，可接受 |
| R7 | 6 档 stage 与 openspec 制品 DAG 的对应关系需人工维护 | marker 与 `openspec status` 不一致 | `stage.md` 由 change 作者维护，与 `ui-approval` 同性质（作者负责）；D1 已论证不绑 CLI status 的理由 |

## Migration Order

1. `lib/stage.ts`（纯函数）+ smoke test —— 无行为变更，可独立验证。
2. `lib/harness-log.ts` 事件 kind 增删（`stage.set` / `constraint.read` 写入，`mode.set` 停写）+ 双读查询。
3. `constraint-injection.ts` 注入管线重排（删 keyword / jit-path 整节通道，加 stage 路由）+ `constraint.read` 记账。
4. 消费点切换（entry-gate / ui-design-gate / quality-gate / edit-map / doc-impact.sh / harness-retro.sh）。
5. `constraint-read-trace.sh` + 接入 archive-readiness.sh 与 spec-gate.ts。
6. 配置与文档同步。
