# Design: add-dispatch-constraint-digest

## 0. 问题边界（代码事实）

约束注入以 `mode`（requirements / implementation）为总开关，三处闸门全部 gated：

| 闸门 | 定位 | 无档位时行为 |
| --- | --- | --- |
| 关键词命中 | `.pi/extensions/constraint-injection.ts` `planInjection`：`matchText = mode ? state.recentInputs.join("\n") : ""` | 不扫描任何域关键词 |
| 编辑路径 JIT | 同函数：JIT 收集侧 `if (state.mode && (write\|\|edit))` | 改代码不追加路径映射文档 |
| 域限定 D7 | 同函数 `allowedKeywordDocs`：无 mode 时集合只含 `indexDoc` | 即使命中也被丢弃 |

子线程侧：`session_start` 从事实库继承父会话最近一条可恢复 `mode.set`（只取 `mode + boundChange`），随后 `deliverChildInitStable` 一次性投递稳定层并置 `childInitDelivered = true`；pi-web 子线程 SDK 无 `before_agent_start` 事件，动态层命中计算同样 gated on mode → **父为 null 时子线程拿到索引且不可自愈**。

本 change **不动上述任何一处**，只在「派发」这一外部动作上补一条主动通道。

## D1 为什么是脚本 + 规范，而不是改插件门槛

| 方案 | 覆盖 | 成本/风险 | 结论 |
| --- | --- | --- | --- |
| 放开门槛（无档也跑关键词/JIT） | 全自动 | 动注入链路，引入误命中与稳态注入量变化；与"未激活=仅索引"不变量冲突 | 本 change 不做，留观察窗口后决策 |
| 派发时插件改 task 参数 | 精确 | `tool_call` 改不了参数，只能 block 重派；且只看主线程写的文本 | 不做 |
| **主线程主动取数 + 规范引导** | 覆盖有明确目标路径的派发 | 零注入行为变更；摩擦靠一条命令压到最低 | **采用** |

关键理由：主线程**比插件更懂任务真实意图**（不会因为聊到「日报」就误判域），且派发任务文本是唯一保证能到子线程的通道（子线程进程隔离 + 无 before_agent_start）。缺的只是"取数方法"，补一条命令即可。

## D2 红线提取：单一实现（下沉 lib + bundle）

`extractRedlines` 现为 `.pi/extensions/constraint-injection.ts:718-737` 的无依赖纯函数：`/^(\d+\.|-)\s+(.*)$/` 取行首无缩进顶层项 → 取首个 `**...**` → 无加粗跳过 → 0 条返回 null；`bytes` = 拼接后 UTF-8 长度。

**决策：下沉到 `.pi/extensions/lib/constraint-redlines.ts`（与 `lib/child-session.ts` 同款共享先例），扩展改为 import，脚本用 esbuild bundle 该 lib 后调用。**

```bash
npx -y esbuild .pi/extensions/lib/constraint-redlines.ts --bundle --platform=node \
  --format=cjs --outfile="$TMP/redlines.cjs"
node -e 'const {extractRedlines}=require(process.argv[1]); ...' "$TMP/redlines.cjs"
```

理由：`docs/reference/standard/shared/doc-authoring.md` 明令红线句格式"改一处必改另一处"；脚本内重写第二份正则是最高的漂移风险（R1）。esbuild bundle 是 `.pi/extensions/tests/run-smoke.sh:13-15` 既有模式，无新依赖。

失败方向：esbuild 不可用 / bundle 失败 → 脚本 MUST 非零退出并提示"无法加载红线提取实现"，**MUST NOT** 回退到自带的简化正则（避免静默降级成两套口径）。

## D3 路径反查：同语义两份实现 + 矩阵对账

`scanAppliesTags` 的解析语义（作为契约）：

- 正则 `/^\s*(?:<!--[\s]*)?doc-impact-applies:\s*(.+?)(?:\s*-->)?\s*$/`
- 只扫文档**头部 15 行**；flow 深度 1、standard 深度 2
- 语法 `信号1, 信号2 | section=节名`；**单文档首个匹配行即止**（一文档仅一组标签）
- 信号匹配是**纯子串** `path.includes(signal)`，非路径前缀/glob
- 存在模块级 mtime 缓存（进程内）

脚本不直接复用 `scanAppliesTags`：它带目录递归 + 进程内 mtime 缓存，而脚本是一次性进程，缓存无收益、下沉会把文件系统生命周期耦合进 lib。**因此脚本内实现同语义读取，但一致性由 smoke 的路径矩阵对账钉死**（命中 / 未命中 / 多域 / 标签在 15 行外 / 无标签文档）。

与 D2 的区别理由（为什么这里能接受两份）：红线提取是**内容生成**（逐字节一致性要求高、规则极易漂移）；标签扫描是**文件解析**（输入输出可完全枚举，可用矩阵测试覆盖）。前者共用实现，后者对账测试。

未命中 MUST 三态显式报告，禁止静默空输出：

| 情形 | 输出 |
| --- | --- |
| 路径不匹配任何 `doc-impact-applies` 信号 | 「未命中任何文档标签」+ 回退 `constraints-index.md` 域表 |
| 命中文档但非 flow 域（如 `standard/backend/...`） | 「命中 <doc>，该文档不属业务域」+ 列出可用域 |
| 命中 flow 文档但红线层为空/过短 | 「该域无结构化红线」+ 细节层 read 指引 |

## D4 域清单：唯一真相源与四源对照

实测存在 4 处域名集合，语义轴不同：

| 来源 | 数量 | 语义 | 本 change 定位 |
| --- | --- | --- | --- |
| `docs/reference/flow/*.md` basename（`listFlowDomains`） | 10 | **业务域唯一真相源** | 脚本 `--domains` 只认这个 |
| `.pi/constraint-injection.json` `keywordDocs` | 9 | 关键词→文档映射（省 `research-data-sources`） | 别名/子集，不参与取数 |
| `docs/reference/constraints-index.md` 域表 | 9 | 面向模型的人读索引 | 回退指引目标 |
| `scripts/harness/doc-impact.sh menu` | 8 | **文档目录轴**（flow/api/database/...） | 不同轴，禁止混用 |

脚本 MUST 在域名不存在时列出「唯一真相源」的可用域名并非零退出。本次**不统一**其余三处（收敛会牵动 `/opsx-*` 注入语义与归档门禁，超出本 change 范围），只在该脚本 `--help` 与机制文档里声明这个关系。

## D5 事实库字段：append-only 与三态解析

组装点在 `.pi/extensions/harness-telemetry.ts` `tool_result(Agent)` 分支（payload 组装约 L409）。`event.input.prompt` 在该事件中**完整可得**（当前只落 `promptChars`，未落正文）。

追加两个字段（不改既有字段语义）：

- `constraintDigest`: `"present"` | `"absent"`
- `constraintDomains`: `string[]`（解析出的域名，可为空数组）

解析规则（与 D6 的 brief 模板构成契约）：

1. 在任务文本中定位 `^## 约束` 标题行；无 → `absent`。
2. 段内用 `/flow\/([a-z0-9-]+)\.md/` 全局匹配收域（模板固定写 `源 docs/reference/flow/<域>.md`）。
3. 段存在但收不到域名 → 记 `present` + `[]`（不猜域、不阻断）。

历史兼容：改造前的 `subagent.dispatch` 记录无这两个字段 → retro 计入「未知」并从比例分母剔除，**MUST NOT** 回溯记 `absent`（否则把历史数据算成遗漏）。

## D6 规范引导：三步 + brief 模板 + 子线程自取

**AGENTS.md**（子线程纪律那一条，只追加指针，不重写长句）：

> 派发前按目标路径取域红线：`bash scripts/harness/constraint-digest.sh --paths <目标>` → 粘进任务文本 `## 约束（红线，源 docs/reference/flow/<域>.md）` 段；取数方法见 `开发执行规范.md` §0.6 步骤 3。

**开发执行规范 §0.6 步骤 3**（补三步，替换原来只有"MUST 带"的空约束）：

1. **判域**：按目标路径反查（`--paths`），路径未定时才按任务词查 `constraints-index.md` 域表。
2. **取红线**：`constraint-digest.sh --paths <目标>` 或 `--domains <域>`，输出 5–10 行/域，**不得**让主线程读整篇 flow 文档后自行摘要。
3. **粘进 brief**：固定段落格式（下面模板），源路径必写（子线程要能复核）。

**brief 模板片段**（规范里直接给，禁止自创格式——telemetry 解析依赖它）：

```markdown
## 约束（红线，源 docs/reference/flow/daily-report.md）
- 日报生成必须幂等：同日重复触发不得产生重复记录
- 生成失败必须落失败态并保留原因，禁止静默吞错
> 目标路径与本清单冲突时以 `--paths <你的目标>` 输出为准
```

最后一行是**子线程自取兜底**：brief 漏了域，子线程按自己真实目标路径补取，不靠主线程记性。

## D7 观察窗口与回检判据

- 窗口：≥30 次可判定派发 或 2 周（先到为准）。
- 指标：`harness-retro` 的「派发带域比例」= present / (present + absent)。
- 判据：≥70% → 规范引导路线成立，**不**上插件兜底；<70% → 另开 change 只放开"无档编辑路径 JIT"（最小口子），并复测。
- 反 overfit：本 change **不改注入行为**，观察窗口内只有"规范 + 取数"一个变量在动，比例变化可归因。

## D8 明确不做

- 不放宽 `matchText` / JIT / `allowedKeywordDocs` 任一 gate。
- 不改 `deliverChildInitStable`、快照 key、`mode.set` 语义。
- 不统一 4 处域清单。
- 不落库任务正文（只落两字段派生值，隐私/体积考量不变）。

## 风险与处置

| # | 风险 | 处置 |
| --- | --- | --- |
| R1 | 红线提取双实现漂移 | 下沉 lib 共用 + smoke 逐字节对账；bundle 失败即非零退出，不回退简化正则 |
| R2 | 域清单四源不一致 | 声明唯一真相源=flow basename；脚本只认它；本次不收敛但写入文档 |
| R3 | 阈值 `minSectionBytes`/`digestDocThreshold` 在 config 缺省 | 脚本读取同一 config 缺省值（512/6144），不写死别的数 |
| R4 | 一文档仅一组 `doc-impact-applies`（首个匹配即止） | 沿用同语义并在 smoke 覆盖多行标签用例 |
| R5 | 信号为纯子串匹配 | 输出附命中信号原文，供人工判断误命中 |
| R6 | standard 15 篇中 8 篇缺标签 | 未命中态显式提示回退域表，不静默 |
| R7 | 历史 dispatch 无新字段 | 计「未知」并剔除分母，报告标注条数 |
| R8 | 声明域（文件态、每回合重解析）与关键词/JIT（会话粘性）语义不同 | 脚本一次进程内不做粘性；每次调用现读文件 |
| R9 | `constraint-domains`（proposal 注入）与 `doc-impact`（归档门禁）两套注释易混 | `--help` 与机制文档写明：本脚本只读文档头部 `doc-impact-applies`，不读二者 |
