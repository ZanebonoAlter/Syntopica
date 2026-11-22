## 1. stage 解析纯函数（lib/stage.ts）

- [x] 1.1 新建 `.pi/extensions/lib/stage.ts`：导出 `STAGES`（6 档常量：`explore`/`propose`/`design`/`implement`/`verify`/`archive`）、`StageFamily`（`planning`/`implementation`）、`parseStageMarker(text)` → `{kind:"missing"}` | `{kind:"invalid"}` | `{kind:"ok", stage}`（三态严格对齐 `.pi/extensions/lib/ui-design-gate.ts:167` 的 `parseUiStage` 语义：marker 重复或值不在取值域 → invalid；缺失 → missing），验证：纯函数无 IO，同输入同输出
- [x] 1.2 同文件导出 `stageFamilyOf(stage)`（`explore`/`propose`/`design` → `planning`；`implement`/`verify`/`archive` → `implementation`）与 `isLegacyStage(parseResult)`（missing/invalid → true），验证：6 档映射全覆盖，非法输入返回 null 而非抛错
- [x] 1.3 同文件导出 `readStageFile(changeDir)`：读 `<changeDir>/stage.md`，返回 `{kind, stage?, raw}`；文件不存在 → missing（**不抛错、不记错误日志**），验证：不存在目录与空文件均返回 missing
- [x] 1.4 同文件导出 `buildStageRouting(stage, options)`：按 stage 装配路由清单（文档路径 + 节名 + 取数方式），清单项 SHALL 只含路径与节名、**不内联任何文档正文**；`options` 形如 `{domainDocs: string[], stack: "backend"|"frontend"|null}`，验证：planning 族清单不含代码规范文档、`implement` 族含按栈命中的代码规范文档

## 2. 事实库事件：stage.set 与 constraint.read

- [x] 2.1 `.pi/extensions/lib/harness-log.ts`：事件 kind 枚举新增 `constraint.read`（保留期 30 天）与人名替换 `mode.set` → `stage.set`（保留期 30 天）；`constraint.read` 的 payload 约定含文档仓库根相对路径与 boundChange，验证：`node .pi/extensions/tests/harness-log.smoke.cjs` 退出码 0
- [x] 2.2 同文件导出 `queryLatestStageSet(sessionId)`：先查 `stage.set`，无结果时查 `mode.set` 并按 `requirements`→`planning`、`implementation`→`implementation` 映射回族返回（**双读过渡**，spec Scenario「旧 mode.set 行仍可读」），验证：仅有旧 `mode.set` 行时返回 `planning` 且不报错
- [x] 2.3 同文件导出 `logConstraintRead(cwd, sessionId, docPath, boundChange)` 与 `queryConstraintReads(sessionId)` / `queryConstraintReadsByChange(changeName)`，写入失败 MUST NOT 阻断 agent 动作（fail-loud 记账、流程照常），验证：`node .pi/extensions/tests/harness-log.smoke.cjs` 退出码 0

## 3. 注入管线重排（constraint-injection.ts）

- [x] 3.1 `.pi/extensions/constraint-injection.ts`：删除 keyword 通道的整节正文注入（`matchKeywordDocs` 命中后不再送达整节）与 jit-path 通道的整节正文注入（`scanAppliesTags` 命中后改为**只加入路由清单**，不送达正文），验证：grep 断言 `keywordDocs` 配置项不再用于正文注入路径
- [x] 3.2 同文件：实现阶段首推逻辑——某 stage 首次成为当前阶段时，以增量 steer 消息送达该 stage 的红线层（复用 `.pi/extensions/lib/constraint-redlines.ts:32` 的 `extractRedlines`，**禁止重写第二份正则**）+ 路由清单；同 stage 内后续回合零重复注入（按内容指纹去重），验证：连续 5 回合同 stage 只产生 1 条注入记账
- [x] 3.3 同文件：稳定层改为「索引文档 + 路由清单（路径与节名）」，**SHALL NOT 内联任何业务域约束正文**；`stableSnapshotKey` 第一段由 `mode` 改为 `stageFamily`（同族阶段切换不重建稳定层），验证：snapshot key 在 explore→propose→design 间保持不变，planning→implementation 时变化
- [x] 3.4 同文件：退役 mode 状态机（6 个激活来源：command/skill/edit-dir/recover/inherit/fallback），改为 stage 驱动；change 绑定语义保留（输入提及 / 写 change 目录兜底 / resume-reload-startup 恢复 / 父子继承 / mtime 兜底），事实库改记 `stage.set`（payload：`stage`/`stageFamily`/`boundChange`/`source`），验证：`.pi/extensions/tests/constraint-injection.smoke.cjs` 退出码 0
- [x] 3.5 同文件：read 读取记账（review R-4 后定稿：`tool_execution_start` 登记 toolCallId→path，`tool_execution_end` 且非 error 时记 `constraint.read`）——仅当被读路径命中约束文档白名单（`docs/reference/flow/*.md` 与 `docs/reference/standard/**`）时记账，其余 read 不记（控事实库体量；失败/未回执的读取不记），验证：读 flow 文档产生一条 `constraint.read`，读普通源码不产生
- [x] 3.6 同文件：跨进程子线程继承改走 `stage.set`（读父会话最新 `stage.set`，取 `stage`/`stageFamily`/`boundChange`；stage 值随父会话快照写死，不在子线程重算），验证：smoke 覆盖 `source=inherit` 场景断言通过
- [x] 3.7 同文件：业务域声明位置支持两处——proposal.md 头部的 `<!-- constraint-domains: -->`（存量 change 既有位置）与 stage.md 头部的同名 marker（实现期活口径，用于声明实施中新发现的域）；生效域集合为两者**并集**（stage.md SHALL NOT 收窄 proposal.md 已声明的域），验证：smoke 断言「proposal 声明 reading + stage.md 声明 reading,scheduler → 生效域 {reading,scheduler}」与「stage.md 无声明 → 沿用 proposal 的域」两例通过

## 4. 消费点切换（mode → stage）

- [x] 4.1 `.pi/extensions/entry-gate.ts`：读方从「最新 `mode.set` 的 mode==implementation」改为「最新 `stage.set` 的 stageFamily==implementation」，验证：`node .pi/extensions/tests/quality-gate.smoke.cjs` 与 `node .pi/extensions/tests/constraint-injection.smoke.cjs` 退出码 0
- [x] 4.2 `.pi/extensions/ui-design-gate.ts` + `.pi/extensions/lib/ui-design-gate.ts`：`readLatestMode` 改名 `readLatestStage`，判定条件改为 stageFamily==implementation，验证：`node .pi/extensions/tests/ui-design-gate.smoke.cjs` 退出码 0
- [x] 4.3 `.pi/extensions/quality-gate.ts` 与 `.pi/extensions/lib/edit-map.ts`：`parseModeSetPayload` 改为读 `stage.set`（本消费点只用 boundChange 名，改造面最小），验证：`node .pi/extensions/tests/quality-gate.smoke.cjs` 与 `node .pi/extensions/tests/quality-gate.behavior.smoke.cjs` 退出码 0
- [x] 4.4 `scripts/harness/doc-impact.sh`：`--change` 的 boundChange 取数源从 `kind='mode.set'` 改为 `kind='stage.set'`（**双读**：先 stage.set，无则回退 mode.set，保证改造期不失效），验证：`bash scripts/harness/doc-impact.sh suggest` 退出码 0 且能解析出 change 名
- [x] 4.5 `scripts/harness/harness-retro.sh`：档位统计口径改为阶段统计（读 `stage.set` 的 `stage`/`stageFamily` 字段），历史 `mode.set` 行计入「未知」并从分母剔除（沿用既有「历史记录计未知不计漏」口径），验证：`bash scripts/harness/harness-retro.sh` 退出码 0（裸调用即出报告；`--report` 非合法参数）

## 5. 归档对账脚本

- [x] 5.1 新建 `scripts/harness/constraint-read-trace.sh <change-dir>`：解析 change 的 `<!-- constraint-domains: -->` 声明（proposal.md ∪ stage.md **并集**），逐域检查对应 `docs/reference/flow/<域>.md` 是否被**归属本 change 的成功 read**读过（R-2/R-4 口径：change 列 = 本 change + 实现期窗——首条 implementation 族 `stage.set` 之后，无记录回退生命周期起点）；未读 → 退出码 1，输出含域名、文档路径与 `bash scripts/harness/constraint-digest.sh --brief --domains <域>` 取数指引；全部读过 → 退出码 0；change 无 `stage.md`（legacy）→ 退出码 0 且输出明示豁免，验证：`bash scripts/harness/constraint-read-trace.sh openspec/changes/stage-driven-constraint-injection` 退出码 0
- [x] 5.2 `scripts/harness/archive-readiness.sh`：把 `constraint-read-trace.sh` 纳入聚合检查项（与既有 ①②③④⑤⑦ 同款聚合方式，零口径差异），验证：`bash scripts/harness/archive-readiness.sh openspec/changes/stage-driven-constraint-injection` 退出码 0（未读项如实报红）
- [x] 5.3 `.pi/extensions/spec-gate.ts`：归档拦截新增对账项（调用 `constraint-read-trace.sh`，失败即 block，reason 含未读域清单与取数指引），验证：`node .pi/extensions/tests/spec-gate.smoke.cjs` 退出码 0

## 6. 配置与标记

- [x] 6.1 `.pi/constraint-injection.json`：`modes` 二分桶改为 `stages` 段（6 档，每档含必读文档集合 + 按栈条件文档），删除死配置 `modes.*.baseDocs`（两档同值）与从未配置的 `modes.implementation.conditionalDocs`，保留 `indexDoc`/`budgetBytes`/`keywordDocs`（改为仅供路由清单的域限定，不用于正文注入）/`stackSignals`/`findingsFile` 等，验证：`node -e "JSON.parse(require('fs').readFileSync('.pi/constraint-injection.json','utf8'))"` 退出码 0
- [x] 6.2 为本 change 建 `openspec/changes/stage-driven-constraint-injection/stage.md`（`<!-- stage: implement -->` + `<!-- constraint-domains: -->` + 阶段路由表），作为本仓库首个 stage 载体样例，验证：文件存在且 marker 可被 `parseStageMarker` 解析为 ok
- [x] 6.3 `.pi/extensions/tests/constraint-injection.smoke.cjs`：补 stage 相关断言——stage marker 三态（missing/invalid/ok）、阶段族派生、路由清单装配、读取路径归一化、`stage.set` 记账、`source=inherit` 跨进程继承，验证：`node .pi/extensions/tests/constraint-injection.smoke.cjs` 退出码 0

## 7. Review

- [x] 7.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账

## 8. 测试

- [x] 8.1 影响范围测试（只跑受影响包，不跑全量），验证：`bash .pi/extensions/tests/run-harness-smoke.sh` 退出码 0
- [x] 8.2 对账脚本自测（读过→绿 / 未读→红 / legacy→豁免三态），验证：`bash scripts/harness/constraint-read-trace.sh openspec/changes/stage-driven-constraint-injection` 退出码 0
- [x] 8.3 存量 change 零阻断回归（仓库现有 11 个活跃 change 无 `stage.md`，应全部按 legacy 处理），验证：`for d in openspec/changes/*/; do bash scripts/harness/constraint-read-trace.sh "$d" || echo "FAIL: $d"; done` 无 FAIL 输出
- [x] 8.4 前端与后端门禁（本 change 不改业务代码，仅确认未破坏），验证：`cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...` 退出码 0；`cd front && pnpm lint && pnpm exec nuxi typecheck` 退出码 0

## 9. 验证

> Scenario→测试文件映射表（机器对账由 `scripts/harness/scenario-trace.sh` 执行）。
> 本 change 的行为验证主体是 harness 扩展的 smoke test（node 直跑，不启 pi）；
> 归档对账脚本另有 shell 自测（tasks.md 8.2/8.3）。

| Scenario | 测试文件 |
| --- | --- |
| stage marker 生效 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 缺失 marker 视为 legacy 不阻断 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 非法 stage 值降级不阻断 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| marker 重复视为非法 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| stage.md 可追加业务域声明 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| stage.md 无域声明时沿用 proposal | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 事实库双读兼容旧 mode.set | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 阶段首推红线层与清单 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 同阶段不重复注入 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 路由清单含取数命令 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 不再注入整节正文 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 编辑路径命中不再注入正文 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 读过则对账通过 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 未读过则红并给取数指引 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 他会话的读取不算证据 | scripts/harness/archive-readiness.smoke.sh |
| 规划期的读取不算证据 | scripts/harness/archive-readiness.smoke.sh |
| 失败读取不记账 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| legacy change 豁免对账 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 归档门禁接入对账 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 稳定层档位生命周期内恒定 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 档位切换一次性变化 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 动态层事件驱动增量发送 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| turn 中途切档后同 turn 已投递内容不被延迟重建重投 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 绑定切换后首轮仍全量重投 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| fork 继承的指纹不被陈旧快照 key 重投 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| findings 更新触发重发 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| compact 后快照重发 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 关键词域限定 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 声明域内关键词正常命中 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 稳定层差异走 steer 通知 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 未激活档仅索引 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 未激活档关键词通道保持关闭 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 实现档注入生效 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| JIT 路径细化 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 节残缺时回退全文 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 红线层为空回退全节 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 红线层低于最小字节回退全节 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| flow 文档节级注入 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 声明注入层级记账 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 命中只增不减（缓存稳定） | .pi/extensions/tests/constraint-injection.smoke.cjs |
| change 文本不触发关键词命中 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| ASCII 关键词词边界整词匹配 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 无声明不注入并提示 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 未知域名宽容忽略 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 超预算分层降级 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 降级永不真丢 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 模型可见省略通知 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 降级确定性（缓存友好） | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 降级记账 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| steer 消息 UI 受控 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 两会话交叉绑定互不污染 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 他会话绑定变化不刷新本会话稳定层 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 无自身绑定且无父子关系时不借用他会话 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 子线程显式继承父会话 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 子线程显式继承父会话（pi-web 通道） | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 命中集按会话隔离 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| turn 绑定锁按会话隔离 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 会话条目有界淘汰 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 无 sessionId 语境行为等价 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| smoke test 直跑 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 抢绑条件纯函数直跑 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 动态层 diff 指纹纯函数直跑 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 红线层提取纯函数直跑 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 红线层零提取回退 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 节低于最小字节下限回退全文 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 节提取回落 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 实现阶段自动注入发现 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| research 语境不落 change | .pi/extensions/tests/constraint-injection.smoke.cjs |
| research 语境无 topic 落通用池 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| TTL 分级清扫 | .pi/extensions/tests/harness-log.smoke.cjs |
| 事件追加不可变 | .pi/extensions/tests/harness-log.smoke.cjs |
| spill.write 事件随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| subagent.complete 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| policy.decision 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| edit.map 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| session.rollup 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| change.archive 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| dispatch 携带 change 归属 | .pi/extensions/tests/harness-log.smoke.cjs |
| 无活跃 change 时 dispatch 归属可空 | .pi/extensions/tests/harness-log.smoke.cjs |
| stage.set 事件入库 | .pi/extensions/tests/harness-log.smoke.cjs |
| 旧 mode.set 行仍可读 | .pi/extensions/tests/harness-log.smoke.cjs |
| constraint.read 事件入库 | .pi/extensions/tests/harness-log.smoke.cjs |
| 档位激活记账 | .pi/extensions/tests/harness-log.smoke.cjs |
| 绑定修正追记 | .pi/extensions/tests/harness-log.smoke.cjs |
| TTL 与既有事件兼容 | .pi/extensions/tests/harness-log.smoke.cjs |
| 恢复路径记账 | .pi/extensions/tests/harness-log.smoke.cjs |
| 记账失败不阻断激活 | .pi/extensions/tests/harness-log.smoke.cjs |


- [x] 9.1 单阶段行为链人工验证（用本 change 自身做载体）：在 `openspec/changes/stage-driven-constraint-injection/stage.md` 把 stage 依次改为 `propose` / `design` / `implement`，每次改完开启新回合，确认注入块头部显示的阶段随之变化、路由清单随之切换（planning 族无代码规范文档 → implement 族出现按栈代码规范文档），验证：三次阶段切换的注入内容差异肉眼可辨（实际验证：本会话多回合观察到 implement 档注入块〔头部阶段行 + implement 档路由清单〕随 stage.md 生效；smoke stage-routing 组覆盖六档清单差异 + 任务组 6 配置等价性实测六档 SET-IDENTICAL；用户 2026-11-18 拍板归档）
- [x] 9.2 自主导入行为验证：确认路由清单给出 `constraint-digest.sh --brief --domains <域>` 命令且执行后能取到该域红线（红线层与扩展注入逐字节一致），验证：`bash scripts/harness/constraint-digest.sh --brief --domains daily-report` 输出含该域红线（当前 8 条顶层加粗红线句）且退出码 0
- [x] 9.3 注入体量对比验证（改造前后）：改造后单次注入字节数 SHALL 显著低于改造前最坏值 27831，验证：改造后 smoke 输出的注入体量断言：declaration（红线层/取数指引）与 routing 条目均 ≤ 4000 字节/条（回退取数指引后上限对全部域恒成立）
- [x] 9.4 legacy 兜底验证：仓库现有 11 个活跃 change 均无 `stage.md`，各自注入 SHALL 按 legacy 退化形态（索引 + 声明域红线层，无阶段路由清单；keyword/jit 正文退役对全部 change 生效）且零阻断不报错，验证：`.pi/extensions/tests/constraint-injection.smoke.cjs` 的 legacy 场景断言通过

## 10. 文档

<!-- doc-impact: none -->

> 本 change 为 harness 工具链内部改造：只动 `.pi/extensions/**` 与 `scripts/harness/**`，
> 不触及 `docs/reference/flow/`（业务域文档）、`docs/reference/api/`、`docs/reference/database/`
> 等任何 doc-impact 域轴覆盖的文档。规范类文档（`docs/reference/standard/**`）不涉业务域红线，
> 本 change 不修改其内容。故声明 `none`（纯内部重构）。
>
> **无 flow 影响**：本 change 未改任何业务域流程，故不补 flow 变更溯源行（§12.2 豁免）。

- [x] 10.1 `docs/reference/harness/pi-extensions.md`：更新 constraint-injection 章节——注入形态改「红线层 + 路由清单」、stage 标识机制、`stage.set`/`constraint.read` 事件、keyword/jit-path 通道退役说明
- [x] 10.2 `docs/reference/constraints-index.md`：更新注入文案口径（「档位：未激活」→「阶段：<stage>」），补 stage.md 载体说明与路由清单取数方式
- [x] 10.3 `AGENTS.md`：更新「pi harness 扩展」小节——约束注入改为阶段驱动 + 自主导入的模型行为约定（模型需自行 read 路由清单中的文档）
- [x] 10.4 `docs/reference/开发执行规范.md` §0.6：更新 openspec 编排流程——制品链补 `stage.md`（非 schema 制品）与阶段推进约定
- [x] 10.5 `openspec/specs/constraint-injection/spec.md` 与 `openspec/specs/harness-fact-log/spec.md` 的归档同步由 `openspec archive` 完成；归档后按 §12 补 flow 变更溯源（本 change 不涉业务域 flow，`docs/reference/flow/` 无需变更——tasks.md 文档节已声明）
