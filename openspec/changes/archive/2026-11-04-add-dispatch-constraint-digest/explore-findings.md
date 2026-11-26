
## 红线提取规则（extractRedlines 纯函数语义）

## 定位
`.pi/extensions/constraint-injection.ts:718-737`（`export function extractRedlines`），调用点 `:1132-1141`。
规范侧镜像：`docs/reference/standard/shared/doc-authoring.md:74-83`「约束节红线句格式（declaration 注入层）」。

## 实现（逐行语义）
```ts
export function extractRedlines(sectionText: string): RedlineLayer | null {
	const lines: string[] = [];
	for (const line of sectionText.split(/\r?\n/)) {
		const m = line.match(/^(\d+\.|-)\s+(.*)$/);
		if (!m) continue;                       // 非顶层列表项（嵌套/引用块/自由段落/标题）跳过
		const bold = m[2].match(/\*\*(.+?)\*\*/);
		if (!bold) continue;                   // 无加粗块：不凑数，该条不进红线层
		lines.push(`${m[1]} **${bold[1]}**`);
	}
	if (!lines.length) return null;
	return { lines, bytes: Buffer.byteLength(lines.join("\n"), "utf8") };
}
```
- 正则 `^(\d+\.|-)\s+(.*)$` **行首无缩进** → 只有顶层列表项（`N. ` / `- `）入选；嵌套项、引用块（`>`）、自由段落、标题全部跳过。
- 加粗块取**首个** `**...**`（非贪婪 `\*\*(.+?)\*\*`）；bold 文本内的原始分隔标点（如 `：`）被丢弃，输出格式固定为 `` `${编号或-} **${加粗内容}` ``。
- 返回结构 `{ lines: string[], bytes: number }`；`bytes` = `lines.join("\n")` 的 UTF-8 字节数（**不含**尾行分隔符）。
- **纯函数、无 I/O、无随机/时间依赖** → 可被任意脚本逐字节复现。

## 调用点的关键阈值（易漏）
`:1132-1141`（仅当 `entry.reason === "declaration"`）：
```ts
const red = extractRedlines(sec);
if (red && red.bytes >= minSectionBytes) {          // minSectionBytes = config.minSectionBytes ?? 512
	injected = `${red.lines.join("\n")}\n\n细节层：read \`${rel}\`「${entry.section}」节`;
	injectMode = "section"; layer = "redline"; docLabel = `${display}(红线层)`;
}
```
- 阈值比的是 **红线层字节数**（不是节字节数）vs `minSectionBytes`（config 里**没有**这个键 → 缺省 **512**）。
- 0 条提取 或 bytes < 512 → 落 `injected == null` 分支 → 回退**全节**注入（`layer="full"`）。
- 失败时**不静默跳过**：回退全节。

## 能否被 bash / 独立 node 脚本复现？
**能，但不应新写。** 正则极简、纯函数、无依赖 → 复制即等价。风险在**漂移**：`doc-authoring.md:83` 明写「格式规范与提取器语义同步演进，改一处必改另一处」。仓库既有复用模式 = esbuild 打包 + node 直跑：`.pi/extensions/tests/run-smoke.sh:13-15`
```bash
npx -y esbuild ../constraint-injection.ts --bundle --platform=node --format=cjs \
  --alias:typebox="$PWD/.typebox-stub.cjs" --outfile=./.bundle.cjs >/dev/null
```
注意 `constraint-injection.ts:80-93` 顶部 import 了 `@earendil-works/pi-coding-agent`（type-only）、`typebox`、`./lib/harness-log`、`./lib/child-session` → 直接 bundle 需 typebox stub（run-smoke.sh 已造模板），且 `import type` 的 pi 包在 bundle 时会被擦除。
**建议**：新脚本复用「esbuild bundle → node 取 `ext.extractRedlines`」这条既有通路，或把 `extractRedlines` 下沉到 `.pi/extensions/lib/` 纯模块后由 extension 与脚本**双引同一实现**；绝不在 constraint-digest.sh 里手写第二份正则。

**引用**：.pi/extensions/constraint-injection.ts:extractRedlines、.pi/extensions/constraint-injection.ts:1132-1141、docs/reference/standard/shared/doc-authoring.md:74-83、.pi/extensions/tests/run-smoke.sh:13-15、.pi/extensions/tests/constraint-injection.smoke.cjs:513-515

<!-- pinned 2026-11-04T03:57:57Z -->

## doc-impact-applies 标签扫描（scanAppliesTags 语义）

## 定位
`.pi/extensions/constraint-injection.ts`：
- `APPLIES_TAG_RE`（约 L581-583）
- `parseAppliesLine(raw)`（约 L585-601）
- `walkDocs(cwd, relDir, depth)`（约 L603-618）
- `export function scanAppliesTags(cwd)`（L624-670）
- 缓存声明：`appliesTagCache`（约 L384-390，「design D3」）

## 标签语法（正则 + 解析）
```ts
const APPLIES_TAG_RE = /^\s*(?:<!--[\s]*)?doc-impact-applies:\s*(.+?)(?:\s*-->)?\s*$/;
```
- 允许行首空白；`<!--` 可选；尾部 `-->` 可选 → 同时兼容**裸行**（ai-logging 的块注释内）与**单行 HTML 注释**两种形态。
- 行内格式：`doc-impact-applies: <信号1>, <信号2>, ... | section=<节名>`
```ts
const parts = raw.split("|");
const signals = parts[0].split(",").map((s) => s.trim()).filter(Boolean);
if (!signals.length) return null;
for (const p of parts.slice(1)) { const m = p.trim().match(/^section=(.+)$/); if (m) section = m[1].trim(); }
```
- 信号 = **逗号分隔的路径片段**（子串匹配用），`|` 之后为可选 `section=`（**节名怎么给**：`| section=节名`，非 `section=` 形式无效；多段 `|` 只有 `section=` 生效）。

## 扫描范围
```ts
const files = [
  ...walkDocs(cwd, "docs/reference/flow", 1),
  ...walkDocs(cwd, "docs/reference/standard", 2),
];
```
- flow 目录深度 **1**（`docs/reference/flow/*.md`，不含子目录 `diagrams/`），standard 深度 **2**（`standard/backend/*.md`、`standard/shared/*.md` 等）。
- 只收 `.md`、排除 `README.md`。
- **只读文件头部 15 行**：`readFileSync(abs,"utf8").split(/\r?\n/).slice(0,15)`，逐行匹配，**首个匹配行即 `break`**。

## 缓存
- 模块级 `appliesTagCache: Map<rel, { mtime: number, entry: {...} | null }>`，**按文件 mtime 逐文件失效重读**（未变文件直接复用 entry，含「该文件无标签」的 null 记录）。
- 缓存是**进程内**的：新进程/新脚本一律全量重扫 → 独立脚本天然无陈旧问题。
- 注意：`scanAppliesTags` 每次调用都**重新遍历目录**（readdirSync），只有「读文件」被 mtime 缓存。

## 命中语义（消费侧）
`:1970-1979`（JIT 注入）：`tag.signals.some((s) => p.includes(s))` —— **纯子串匹配**，无路径边界、无 glob。命中即把该文档加入会话粘性集合 `state.jitDocHits`（只增不减）。

## 关键约束
- **一文档只支持一组 `{signals, section}`**（首个匹配行 break，后续标签行被忽略）→ 新脚本不能假设多标签行。
- 标签里的 `section` 是**节名**（精确匹配 `## <节名>`，见 `extractSection` L680-700：`lines.findIndex((l) => l.trimEnd() === "## " + sectionName)`，找不到 → null → 回退全文）。

## 建议
constraint-digest.sh 若要「路径 → 受影响文档/域」反查，**应复用本标签扫描语义**（头部 15 行 + 同正则 + 逗号信号），而不是自写一套 grep；但因该函数在 TS extension 内且带 pi 依赖，落地形态应为「把这 3 个纯逻辑（正则 + parse + walk）下沉到 `.pi/extensions/lib/` 共享模块，extension 与脚本同源引入」。

**引用**：.pi/extensions/constraint-injection.ts:scanAppliesTags、.pi/extensions/constraint-injection.ts:APPLIES_TAG_RE、.pi/extensions/constraint-injection.ts:1970-1979、.pi/extensions/constraint-injection.ts:extractSection

<!-- pinned 2026-11-04T03:58:13Z -->

## 业务域清单来源（listFlowDomains）与三套域口径

## 定位
`.pi/extensions/constraint-injection.ts:501-518`：
```ts
/** 合法业务域名单 = docs/reference/flow/*.md 的 basename（README 除外），目录 mtime 缓存 */
function listFlowDomains(cwd: string): string[] {
	const dir = join(cwd, "docs/reference/flow");
	try {
		const mtime = statSync(dir).mtimeMs;
		if (flowDomainsCache && flowDomainsCache.mtime === mtime) return flowDomainsCache.names;
		const names = readdirSync(dir)
			.filter((f) => f.endsWith(".md") && f !== "README.md")
			.map((f) => f.slice(0, -3));
		flowDomainsCache = { mtime, names };
		return names;
	} catch { return []; }
}
```
- **扫目录文件名**（非内容）：`docs/reference/flow/*.md` 顶层、`.md` 结尾、排除 `README.md` → 取 basename 去 `.md`。子目录 `diagrams/` 不递归。
- 缓存 = **目录 mtime**（`flowDomainsCache`）。新增/删除 flow 文档即触发失效；改文档内容不影响。
- 用途：`planInjection` L1010-1016 用它校验 proposal 的 `constraint-domains` 声明 → 合法域名白名单；未知域名进 `declUnknown` 并被**宽容忽略**（提示但不阻断）。

## 实际域清单（实测 2026-09）
`docs/reference/flow/` 文件：README.md, ai-summary.md, content-enrichment.md, daily-report.md, data-enrichment.md, discovery.md, reading.md, **research-data-sources.md**, scheduler.md, semantic-board.md, topic-graph.md（+ 子目录 diagrams/）
→ `listFlowDomains` 返回 **10** 个域：ai-summary, content-enrichment, daily-report, data-enrichment, discovery, reading, **research-data-sources**, scheduler, semantic-board, topic-graph。

## ⚠️ 三套「域清单」并存（口径漂移高发）
| 来源 | 数量 | 内容 | 位置 |
|---|---|---|---|
| A. `listFlowDomains`（文件名） | **10** | 含 `research-data-sources` | `constraint-injection.ts:501` |
| B. `.pi/constraint-injection.json` `keywordDocs` | **9** | 无 `research-data-sources` | `.pi/constraint-injection.json` |
| C. `docs/reference/constraints-index.md` 业务规范行 | **9** | 无 `research-data-sources` | `constraints-index.md:21` |
| D. `scripts/harness/doc-impact.sh` `cmd_menu` | **8** | flow/api/database/architecture/standard/configuration/deployment/none | `doc-impact.sh:167-179` |

- A ⊃ B = C；`research-data-sources.md` **只出现在 A**：它可被 `<!-- constraint-domains: research-data-sources -->` 声明并注入红线层，但**不在** keywordDocs（不会被关键词命中）、**不在** constraints-index 业务规范行（模型看不到该域清单）。
- B 内部还有二次收窄：`planInjection` L1085-1100 的 `allowedKeywordDocs` = 索引 ∪ mode-baseDocs ∪ 声明域 ∪ 栈条件文档；**集合外的关键词命中直接丢弃**（"跨域词不再误拉无关域全节"）。即「声明域」与「关键词域」是两个不同集合，脚本必须区分。
- D 的 8 域是**「哪类文档要更新」的分类学**（flow/api/database/…），与 A/B/C 的**业务域**分类学**完全不同**；D 的 `flow` 一格 = 整个 `docs/reference/flow/`（10 个业务域被压成 1 个）。两者**不可混用**。
- `doc-authoring.md:95-104` 的「新增 flow 域」checklist 把 B/C/D 列为**三条独立登记义务**（keywordDocs 不在 checklist 里，但 `constraints-index.md` 登记 + `doc-impact: flow` 声明在）。

## 建议
constraint-digest.sh 的「域 → 文档」映射**必须复用 A（`listFlowDomains` 扫文件名）**作为域白名单，与 B（keywordDocs）区分开：
- 声明域集合 → A（10 域）
- 关键词域集合 → B（9 域）
两者取并集时仍以 A 为合法性判据。

**引用**：.pi/extensions/constraint-injection.ts:listFlowDomains、.pi/extensions/constraint-injection.ts:1010-1016、.pi/extensions/constraint-injection.ts:1085-1100、.pi/constraint-injection.json:keywordDocs、docs/reference/constraints-index.md:21

<!-- pinned 2026-11-04T03:58:23Z -->

## doc-impact 路径反查能力（无反向映射）

## 定位
`scripts/harness/doc-impact.sh`（461 行，POSIX bash，只读，退出码 0 除非 verify 失败）。子命令：`menu` / `suggest` / `verify`（`context` 已退役，见 `:445-449`）。

## 结论：**没有「路径 → 文档/域」反查**
脚本里唯一与路径相关的是 `heuristic_hit()`（L126-162），方向是 **域 → 路径**（正向）：给一个域 key + 改动文件列表，echo 命中的文件。
```bash
heuristic_hit() {
	local domain="$1" files="$2"
	case "$domain" in
	flow)  echo "$files" | grep -E 'backend-go/internal/(admin|reader|tagmanagement|topicgraph|dataenrichment)/service/' ||
	        echo "$files" | grep -E '^front/app/features/' ;;
	api)   echo "$files" | grep -E 'backend-go/internal/[^/]+/handler/' || ... ;;
	database) ... models/ ... AutoMigrate|CREATE TABLE|ALTER TABLE ...
	architecture) ... internal/app/ ... platform/tracing/ ...
	standard)    .golangci.yml|eslint.config.js | ^docs/reference/standard/
	configuration) config.*\.ya?ml（排除 ^openspec/） | platform/config/
	deployment)  Dockerfile|docker-compose | init.ps1|init.sh
	none) ;;
	esac
}
```
- 映射数据来源 = **脚本内硬编码 grep 正则**，**不是** git diff、**不是**文档头部的 `doc-impact-applies` 标签、也不是任何配置文件。
- `cmd_suggest`（L189-285）= 改动文件集合（归属轨 edit.map 优先，回退全树 git diff）→ 逐域 `heuristic_hit` → 预勾选 checkbox + 输出声明模板 `<!-- doc-impact:flow,standard -->`。
- `cmd_verify`（L292-427）= 解析 tasks.md 的 `<!-- doc-impact: ... -->` 注释（域级）+ 「## N. 文档」节的 checkbox 里的 `docs/*.md` 路径（文件级）→ 对账 git diff。**只读域声明与文件路径，仍无「路径反查域」**。

## menu 的 8 个固定域（L167-179）
```
flow          业务链路文档 docs/reference/flow/
api           API 参考 docs/reference/api/
database      数据库 docs/reference/database/
architecture  架构 docs/reference/architecture/
standard      代码规约 docs/reference/standard/
configuration 配置 docs/reference/configuration.md
deployment    部署 docs/reference/deployment.md
none          纯内部重构（声明须附理由）
```
（`heuristic_hit` 只处理前 7 个 + `none`，共 8。）

## ⚠️ 与 `.pi/constraint-injection.json` keywordDocs 的域清单**不一致**（两套口径）
- doc-impact 的 8 域 = **文档类型分类学**（哪类文档需要更新）；keywordDocs 的 9 域 = **业务域分类学**（哪个业务流的约束）。语义轴完全不同，**只有 `flow` / `standard` 两个粗粒度词面重合**。
- 粒度差：doc-impact 的 `flow` 一格覆盖 `docs/reference/flow/` 全部 10 个业务域；constraint-injection 的 `daily-report` 只覆盖 `flow/daily-report.md`。
- 后果：一个只改 `flow/daily-report.md` 的 change，doc-impact 记 `flow`（二值），constraint-injection 需要知道是 `daily-report` 域。**从 doc-impact 的域声明无法推出业务域**。
- `doc-impact.sh` 注释 `:30` 指向「backend domain 白名单（与 check-standards.sh B 段一致，权威：standard/backend/package-layout.md）」——即 doc-impact 的域表只与 check-standards 对齐，**从未与 constraint-injection 对齐**。

## 建议
constraint-digest.sh 的「路径 → 受影响文档」应走 **`doc-impact-applies` 标签**（唯一已存在的路径↔文档映射，JIT 注入单一真相源），**不要**复用 `heuristic_hit`（那是域级粗筛，且口径与业务域无关）；**不要**复用 `cmd_menu` 的 8 域清单当业务域。
若必须与 doc-impact 门禁对齐：应做「路径 → 标签信号命中 → flow/standard 文档 → 文档 basename 即业务域名」这条链（flow 文档 basename 与域名一字不差，是现成的天然映射）。

**引用**：scripts/harness/doc-impact.sh:heuristic_hit、scripts/harness/doc-impact.sh:cmd_menu、scripts/harness/doc-impact.sh:cmd_verify、scripts/harness/doc-impact.sh:30、.pi/constraint-injection.json:keywordDocs

<!-- pinned 2026-11-04T03:58:37Z -->

## subagent.dispatch payload 字段与组装位置

## 定位
`.pi/extensions/harness-telemetry.ts`：
- handler：`pi.on("tool_result", async (event, ctx) => {...})` 起于 **L313**；Agent 分支起于 **L361**（`if (event.toolName !== "Agent") return;`）。
- 记账：`guardedLog(ctx.cwd, ctx, { kind: "subagent.dispatch", ... })` 起于 **L409**，payload 块 **L412-431**。
- 起点暂存：`pi.on("tool_call")` **L297-301**（`event.toolName === "Agent"` → `agentStarts.set(event.toolCallId, Date.now())`）。
- 存储：`.pi/harness/events.db` 单表 `events`（ts, session_id, kind, change, payload TEXT=JSON）→ `lib/harness-log.ts:150-160, 246-260`。

## payload 完整字段清单（L412-431）
```ts
payload: {
	parent_session_id: sessionId,
	type: input.subagent_type ?? null,
	model: input.model ?? null,
	desc: truncate(input.description, 160),
	promptChars: input.prompt?.length ?? 0,
	background: input.run_in_background === true,
	ms: details?.durationMs ?? (started ? Date.now() - started : null),
	tokens,                                  // usage?.totalTokens ?? parseTokenCount(details.tokens)
	tokenLabel,                              // details.tokens 原样（实测形如 "37.4k"）
	cost: usage?.cost?.total ?? null,
	status: details?.status ?? null,
	agentId: details?.agentId ?? null,
	toolUses: details?.toolUses ?? null,
	turnCount: details?.turnCount ?? null,
	isError: event.isError === true,
	...(failure ? { failure } : {}),          // 仅 isError 时有，见 classifyFailure
}
```
事件级字段（非 payload）：`kind="subagent.dispatch"`、`sessionId`、`change`（= `detectActiveChange(ctx.cwd)?.name ?? null`，L395）。

## ⚠️ **任务文本未被落库**
- `input.prompt`（子线程的 task 正文）**只留长度**：`promptChars: input.prompt?.length ?? 0`（L419）。全仓 grep 确认 `.pi/extensions/` 内**没有任何地方持久化 prompt 文本**。
- `input.description` 只留 **160 字符截断**（`truncate` L80-83，超长追加 `…`）。
- ⇒ 若「dispatch 时按 task 文本派生约束摘要」，**必须扩 telemetry payload**（新增字段，如 `taskDigest`/`desc` 提额/`promptHead`），否则子线程任务正文不可得。这是本 change 必然要动 `harness-telemetry.ts` 的硬事实。

## 可复用的现成解析函数（都在 harness-telemetry.ts，纯函数，已 export 供 smoke 直测）
| 函数 | 位置 | 用途 |
|---|---|---|
| `truncate(s, max)` | L80-83 | 字符串截断（非 string → ""） |
| `parseTokenCount(value)` | L85-94 | "37.4k"/"2.0M" → 整数 token |
| `errorTextOf(content)` | L97-107 | tool_result.content 取 text 段拼接 |
| `parseSubagentSummary(text)` | L122-141（export） | 解析 `get_subagent_result` 摘要：`Agent: <id>` / `Status:` / `Tool uses:` / `N.M token` / `Duration: Ns`；`running` 或解析不出 → null（fail-safe 不伪造） |
| `classifyFailure(...)` | L402-407 调用 | A4 失败分类（仅 isError 时产出 `failure`） |

## 相关：change 归属解析
`detectActiveChange(ctx.cwd)` 在 `.pi/extensions/lib/active-change.ts`（`listChangeDirs` L14、`detectActiveChange` L31）——dispatch 落账与 session.rollup 共用同一判定，**同源**。

## 建议
constraint-digest 的「子线程任务 → 应注入约束」判定，最省事且不漏的路径：
1. 复用 `subagent.dispatch` 事件里的 `desc`（已有 ≤160 字符任务摘要）+ `type`；
2. 若需更完整的 task 文本，**在 telemetry payload 增字段**（append-only，向后兼容：旧行缺字段读为 null）；
3. 域判定复用 constraint-injection 既有三段（声明域 / 关键词 / JIT），**不要**在 digest 侧另造一套关键词表。

**引用**：.pi/extensions/harness-telemetry.ts:313、.pi/extensions/harness-telemetry.ts:361、.pi/extensions/harness-telemetry.ts:409-431、.pi/extensions/harness-telemetry.ts:parseSubagentSummary、.pi/extensions/lib/harness-log.ts:246-260、.pi/extensions/lib/active-change.ts:31

<!-- pinned 2026-11-04T03:58:50Z -->

## doc-impact-applies 实际写法与覆盖缺口

## 三个实际写法样例
1. **flow 单行 HTML 注释**（最常见，10/10 flow 文档都是）——`docs/reference/flow/daily-report.md:3`：
```markdown
# 日报 / Digest 流程（Daily Report）

<!-- doc-impact-applies: backend-go/internal/topicgraph/, backend-go/internal/admin/scheduler/ | section=业务约束与不变量 -->
> 大功能：每日日报生成（后端）+ Digest 预览/查看（前端）+ Section 可视化。
```
2. **多信号 + 前端路径**——`docs/reference/flow/semantic-board.md:3`：
```markdown
<!-- doc-impact-applies: backend-go/internal/tagmanagement/, backend-go/internal/topicgraph/, front/app/api/semanticBoards.ts, front/app/api/normalizers/boardRadar, front/app/features/tags/components/console/BoardRadar, front/app/features/tags/components/console/boardRadar, front/app/features/tags/composables/useBoardRadar | section=业务约束与不变量 -->
```
3. **standard 文档的裸行形态**（块注释内，无 `<!--`）——`docs/reference/standard/backend/ai-logging.md:1-5`：
```markdown
# AI 调用记录规范（AI Call Logging）

<!--
doc-impact-applies: backend-go/internal/platform/airouter/, backend-go/internal/dataenrichment/ | section=Requirements
-->
```
（另一种特殊值：`docs/reference/standard/frontend/loading-experience.md` 用 `section=全部`——表示"整篇"。）

## 覆盖实测（flow + standard，depth 1/2，排除 README.md，共 25 篇）
- **flow：10/10 全有标签**（ai-summary, content-enrichment, daily-report, data-enrichment, discovery, reading, research-data-sources, scheduler, semantic-board, topic-graph）。
- **standard：7/15 有标签**（shared/test-design.md `section=JIT 注入摘要`、shared/doc-authoring.md `section=注册点速查`、backend/testing.md `section=🛑 DSN 安全红线（事故教训 — 不可违反）`、backend/ai-logging.md `section=Requirements`、frontend/loading-experience.md `section=全部`、frontend/layout.md `section=Requirements`、frontend/testing.md `section=单元测试（Vitest）`）。
- **standard 缺标签 8 篇**：`shared/commit-pr.md`、`backend/lint.md`、`backend/package-layout.md`、`backend/code-style.md`、`frontend/interaction-conventions.md`、`frontend/lint.md`、`frontend/theming.md`、`frontend/code-style.md`。
  → 这 8 篇**永远不会**被 JIT 路径命中（静默失效，无门禁拦——`doc-authoring.md:99` 明写「最容易漏」「无门禁拦」）。

## ⚠️ section 名不一致（脚本必须处理）
- **flow 文档一律** `section=业务约束与不变量`（10/10 一致）→ 红线提取的节名固定。
- **standard 文档节名五花八门**：`Requirements` / `全部` / `JIT 注入摘要` / `注册点速查` / `🛑 DSN 安全红线（事故教训 — 不可违反）` / `单元测试（Vitest）`。
- `section=全部` 不是真节名（无 `## 全部` 标题）→ `extractSection` 返回 null → 回退全文（`extractSection` L680-700 / 消费 L1120-1150）。
- 节名匹配是**精确匹配** `## <节名>`（`l.trimEnd() === "## " + sectionName`），`###` 小节不算。

## 与红线格式的关系
红线层提取**只对声明域（declaration）生效**，节名固定为「业务约束与不变量」（`planInjection` L1050-1060 注入 `{ section: "业务约束与不变量" }`），**JIT / 关键词命中注的是全节**（`entry.section` 来自标签，可能是任意节名）。
`docs/reference/flow/daily-report.md:214-238` 的节内容是标准样例：18 条顶层编号项，**其中第 17 条 `17. **section 展示标题必须由当天实际内容派生，不得默认复述所挂话题 label**` 之后的条目没有加粗** → 该条被 `extractRedlines` 跳过（红线层不含它，细节层含）。这正是"无加粗块不凑数"的实证。

**引用**：docs/reference/flow/daily-report.md:3、docs/reference/flow/daily-report.md:214-238、docs/reference/flow/semantic-board.md:3、docs/reference/standard/backend/ai-logging.md:3-5、docs/reference/standard/frontend/loading-experience.md:3、docs/reference/standard/shared/doc-authoring.md:99

<!-- pinned 2026-11-04T03:59:00Z -->

## 两套口径漂移风险清单（实现期红线）

以下每条都会让「constraint-digest.sh 的输出」与「constraint-injection extension 的实际注入」产生分歧。实现期必须逐条处置。

## R1 红线提取规则双实现（最高危）
- 真源：`.pi/extensions/constraint-injection.ts:718-737` `extractRedlines`（顶层列表项 + 首个 `**...**`，无加粗跳过，0 条 null）。
- `docs/reference/standard/shared/doc-authoring.md:83` 明写「格式规范与提取器语义同步演进，改一处必改另一处」→ 复制正则 = 承诺永久同步。
- **处置**：不手写第二份。要么脚本用 esbuild bundle（`.pi/extensions/tests/run-smoke.sh:13-15` 既有模式，需 typebox stub）调用同一个 `extractRedlines`；要么把纯函数下沉 `.pi/extensions/lib/constraint-redline.ts` 由 extension 与脚本**双引**。

## R2 域清单 4 处来源（见「业务域清单来源」pin）
- A `listFlowDomains`（flow 文件名，**10** 域，含 research-data-sources）
- B `.pi/constraint-injection.json` keywordDocs（**9** 域）
- C `docs/reference/constraints-index.md:21` 业务规范行（**9** 域）
- D `doc-impact.sh cmd_menu`（**8** 域，文档类型分类学）
- A ⊃ B = C；D 与 A/B/C **语义轴不同**。
- **处置**：digest 侧域白名单 = A；声明域与关键词域分开处理（关键词域另受 `allowedKeywordDocs` 收窄，`planInjection` L1085-1100）；D 只能当"文档要更新"清单，**不能**当业务域。

## R3 阈值缺省值漂移
`minSectionBytes = config.minSectionBytes ?? 512`（`:1113`）——**config 里没有这个键**（`.pi/constraint-injection.json` 只有 budgetBytes / digestDocThreshold / findingsDigestThreshold / recentInputWindow）。脚本若写死别的数 → 同一节在脚本与 extension 里一个回退全节一个注红线层。
另：`digestDocThreshold` 缺省 **6144**（`:1074` 的 `?? 6144`，config 里也没这个键，实际 json 有 `"digestDocThreshold": 6144`）。

## R4 一文档只支持一组标签
`scanAppliesTags` 首个匹配行即 `break` → 多标签行后者被忽略。脚本若"合并多条标签"会得出 extension 永远不会注入的文档集合。

## R5 信号匹配语义
命中 = `p.includes(signal)` **纯子串**（`:1972`），不是路径前缀/glob。脚本若用 `grep -F` 精确行匹配或路径边界判断 → 命中集合不同（例：信号 `backend-go/internal/reader/` 需匹配**子串**，`backend-go/internal/reader` 不带斜杠也会命中）。

## R6 节名/节内容
- 红线层只对 `reason === "declaration"` 生效，节名**恒为**「业务约束与不变量」；JIT/关键词命中注**全节**（节名来自标签，可能不存在 → 回退全文）。
- `extractSection` 精确匹配 `## <节名>`（`###` 不算），`section=全部` 之类伪节名 → null → 回退全文。

## R7 dispatch 任务文本不可得
`subagent.dispatch` payload 只有 `desc`（≤160 字符截断）+ `promptChars`（长度），**无 task 正文**（见「subagent.dispatch payload」pin）。若 digest 依赖 task 文本，必须扩 telemetry payload 字段（append-only，旧行读 null），否则与真实约束上下文脱节。

## R8 「域声明」是文件态、不是会话态
`planInjection` L1005-1020：**每回合从 proposal.md 重解析**（不进粘性集合，编辑立即生效）；而关键词/JIT 命中集合是**会话粘性只增不减**。digest 侧若把两者混成一套缓存，会与 extension 实际行为漂移。

## R9 gate 口径与注入口径是两个体系
`doc-impact.sh` 的 8 域 + `<!-- doc-impact: ... -->` 是**归档门禁**（`cmd_verify` 五规则，FAIL 会卡归档）；constraint-injection 的域是**运行时注入**。同一 change 会同时写 `<!-- constraint-domains: daily-report -->`（proposal.md）与 `<!-- doc-impact: flow -->`（tasks.md）——两个注释语义完全不同，digest 脚本别把 `doc-impact:` 当域声明读。

**引用**：.pi/extensions/constraint-injection.ts:718-737、.pi/extensions/constraint-injection.ts:1113、.pi/extensions/constraint-injection.ts:1972、.pi/extensions/constraint-injection.ts:1005-1020、.pi/constraint-injection.json、docs/reference/standard/shared/doc-authoring.md:83、scripts/harness/doc-impact.sh:167-179

<!-- pinned 2026-11-04T03:59:18Z -->
