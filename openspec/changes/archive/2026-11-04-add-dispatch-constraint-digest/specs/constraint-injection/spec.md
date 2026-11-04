## ADDED Requirements

### Requirement: 派发期约束摘要取数（dispatch constraint digest）

系统 SHALL 提供取数入口 `scripts/harness/constraint-digest.sh`，供主线程在派发子线程前把业务域红线以短文本形态取到手，无需读整个 flow 文档。入口 MUST 支持两种输入形态：

- `--domains <域名>[,<域名>...]`：域名 MUST 为业务域唯一真相源（`docs/reference/flow/*.md` 的 basename，与 `listFlowDomains` 同源）；域名不存在时 MUST 以非零退出并列出可用域名，MUST NOT 静默产出空结果。
- `--paths <路径>[,<路径>...]`：按 `doc-impact-applies` 标签反查受影响文档，再据文档归属导出域清单。路径反查语义 MUST 与 constraint-injection 的 `scanAppliesTags` 一致（仅扫文档头部 15 行、同一正则、纯子串匹配、单文档首个匹配行即止）。

输出的每个域 MUST 为「域标题 + 源文档相对路径 + 逐行红线句 + 细节层 read 指引」，且单域红线正文 SHALL 控制在 8 行以内。入口 MUST 提供 `--brief` 模式：输出**单个** `## 约束（红线，源 <源路径…>）` 标题行（多域源路径逗号分隔）+ 每域 `### <域>（源 …）` 子标题 + 红线行 + 固定兜底行，使结果可被派发任务文本**原样粘贴**且被 `parseConstraintBrief` 识别为 `present`；默认（无 `--brief`）MUST 保持人读形态。

**红线提取 MUST 复用 constraint-injection 的同一实现**（`extractRedlines`），MUST NOT 在脚本内重写第二份正则或判定逻辑；提取结果与扩展实际注入的红线层 MUST 逐字节一致。红线层提取为空或低于 `minSectionBytes` 时 MUST 回退为「该域无结构化红线，见 §业务约束与不变量」的显式提示，MUST NOT 输出残缺集合冒充完整。

`--paths` 在以下情形 MUST 显式报告「未命中」而不是空输出：路径不落在任何 `doc-impact-applies` 信号内、命中的文档无对应 flow 域（如 `standard/` 文档）、文档头部无标签。未命中时 MUST 附一条回退指引（按任务关键词查 `constraints-index.md` 域表后改用 `--domains`）。

#### Scenario: 按域取红线

- **WHEN** 主线程执行 `constraint-digest.sh --domains daily-report`
- **THEN** 输出含 `daily-report` 域标题、源路径 `docs/reference/flow/daily-report.md`、逐行红线句与细节层 read 指引，红线正文不超过 8 行

#### Scenario: 按目标路径反查

- **WHEN** 主线程执行 `constraint-digest.sh --paths backend-go/internal/admin/scheduler/job_daily_report.go`
- **THEN** 按 `doc-impact-applies` 标签反查命中 `flow/daily-report.md` 与 `flow/scheduler.md`，输出这两域红线

#### Scenario: 路径无命中显式报告

- **WHEN** 执行 `constraint-digest.sh --paths front/app/components/Foo.vue` 且无任何 `doc-impact-applies` 信号命中该路径
- **THEN** 退出码保持可判定，输出「未命中」说明与 `constraints-index.md` 域表回退指引，不输出空的红线段落

#### Scenario: 红线提取与扩展实现一致

- **WHEN** smoke 对同一个 flow 文档分别调用脚本与 constraint-injection 的 `extractRedlines`
- **THEN** 两者产出的红线行逐字节一致（含 0 条时的回退形态判定一致）

#### Scenario: brief 形态可原样粘贴并被记账

- **WHEN** 主线程执行 `constraint-digest.sh --brief --domains daily-report,scheduler` 并把输出原样粘进派发任务文本
- **THEN** 输出含单个 `## 约束（红线，源 …）` 标题行与两域 `### <域>（源 …）` 子标题，`parseConstraintBrief` 解析得 `present` 且域清单含 `daily-report` 与 `scheduler`

#### Scenario: 未知域名拒绝

- **WHEN** 执行 `constraint-digest.sh --domains not-a-domain`
- **THEN** 脚本非零退出，列出可用域名，不静默返回空

### Requirement: 派发期约束摘要的规范引导与可观测

开发规范 MUST 把「派发任务文本带相关域红线摘要」从无方法的 MUST 改写为可执行三步：**判域（按目标路径反查）→ 取红线（`constraint-digest.sh`）→ 粘进派发任务文本的 `## 约束（红线，源 <路径>）` 段**。规范 MUST 同时给出 brief 模板片段与子线程自取兜底命令（派出后子线程按目标路径自行补取），使注入不单靠主线程记性。

事实库的 `subagent.dispatch` 事件 payload SHALL **append-only** 追加两个字段：`constraintDomains`（从派发任务文本 `## 约束` 段解析出的域名数组）与 `constraintDigest`（`present` / `absent`）。既有字段 MUST NOT 变更语义或类型。派发任务文本无法解析出约束段时 SHALL 记 `absent`，MUST NOT 因解析失败而阻断派发。

`harness-retro` SHALL 提供「派发带域比例」口径（present 派发数 / 可判定派发数）。历史记录缺少两个新字段时 MUST 计入「未知」并从分母剔除，MUST NOT 把历史数据回溯记为 absent（避免把改造前数据算成遗漏）。

#### Scenario: 带约束段派发被记账

- **WHEN** 主线程派发任务文本含 `## 约束（红线，源 docs/reference/flow/daily-report.md）` 段
- **THEN** `subagent.dispatch` payload 记 `constraintDigest=present` 与 `constraintDomains=["daily-report"]`，派发行为本身不受影响

#### Scenario: 无约束段派发被记账但不阻断

- **WHEN** 派发任务文本无 `## 约束` 段
- **THEN** payload 记 `constraintDigest=absent`、`constraintDomains=[]`，派发照常进行

#### Scenario: 旧记录计未知不计漏

- **WHEN** `harness-retro` 统计窗口内混有改造前的 `subagent.dispatch` 记录（无两个新字段）
- **THEN** 这些记录计入「未知」并从带域比例分母剔除，报告显式标注未知条数
