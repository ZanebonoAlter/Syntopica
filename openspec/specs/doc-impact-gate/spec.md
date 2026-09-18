# doc-impact-gate Specification

## Purpose
TBD - created by archiving change docs-harness-consolidation. Update Purpose after archive.

## Requirements

### Requirement: 文档影响声明（apply 启动时）

每个 openspec change 在 apply 启动时 SHALL 运行 `bash scripts/harness/doc-impact.sh suggest` 获取预勾选菜单，并将确认后的文档域声明以机器可读注释写入 tasks.md「文档」节第一行：

```markdown
<!-- doc-impact: flow api configuration -->
```

文档域 SHALL 为以下固定选项之一或多个：`flow` / `api` / `database` / `architecture` / `standard` / `configuration` / `deployment`；无文档影响时 SHALL 声明 `none` 并附理由。

#### Scenario: 声明写入 tasks.md

- **WHEN** 一个 change 进入 apply 阶段
- **THEN** 其 tasks.md「文档」节第一行存在 `<!-- doc-impact: ... -->` 注释
- **AND** 声明的每个文档域在下方有对应的具体文档 checkbox

#### Scenario: 纯代码 change 声明 none

- **WHEN** 一个 change 确认无文档影响
- **THEN** tasks.md 声明 `<!-- doc-impact: none(理由) -->`

### Requirement: 业务约束上下文获取（apply 前置）

约束上下文 SHALL 由 `constraint-injection` extension 在 harness 层每 turn 自动注入 system prompt（见 `constraint-injection` capability），替代原 `bash scripts/harness/doc-impact.sh context` 一次性命令。注入保持**双源**语义：

- **业务规范（what，理解任务）**：按 change 文本/关键词命中 domain，注入相关 flow 文档**「业务约束与不变量」节**（节级提取，非全文；节尾附全文路径指引）。
- **执行规范（how，写对代码）**：按 write/edit 路径（JIT）与 change 文本命中 standard 文档头 `doc-impact-applies` 标签，注入命中文档内容（已 spec 化文档可按 `## Requirements` 节提取，未 spec 化全文注入）。

本要求是**前置必读**而非归档断言——注入内容不构成归档门禁 FAIL 条件。`doc-impact.sh` 的 `suggest`（声明预勾选）与 `verify`（归档对账）子命令职责不变。

#### Scenario: 命中执行规范

- **WHEN** implementation 档激活且 agent edit `backend-go/internal/platform/airouter/router.go`，而 `ai-logging.md` 头 `doc-impact-applies` 含 `backend-go/internal/platform/airouter/`
- **THEN** 该 standard 文档内容被 extension 会话内追加注入（只增不减），后续每 turn 均可见

#### Scenario: 命中业务规范（节级）

- **WHEN** implementation 档激活且 change 文本含 semantic-board domain 关键词（如「板块」）
- **THEN** 注入块业务规范段为 semantic-board flow 文档「业务约束与不变量」节内容，附全文路径指引，不含其余四节

#### Scenario: 无执行规范命中

- **WHEN** 改动路径与 change 文本未命中任何 standard 文档的 `doc-impact-applies` 标签
- **THEN** 注入块不含执行规范段，会话正常进行（不注入空占位）

#### Scenario: 双源分列

- **WHEN** 注入同时命中业务规范与执行规范
- **THEN** 注入块分「业务规范（what）」「执行规范（how）」两段，各自标头，agent 同时看到任务理解与代码红线

#### Scenario: context 子命令退役

- **WHEN** 开发者执行 `bash scripts/harness/doc-impact.sh context`
- **THEN** 脚本提示该子命令已由 constraint-injection extension 取代（不静默失败），suggest/verify 行为不变

### Requirement: 归档前对账（verify）

归档门禁 SHALL 运行 `bash scripts/harness/doc-impact.sh verify <change-dir>` 对每个 active change 对账，以下任一条件 SHALL 判 FAIL：

- tasks.md 缺 `doc-impact` 声明注释
- 声明的文档文件未出现在本 change 文档改动集合中
- 反向启发式命中未声明域（如 `internal/*/handler/` 改动但声明中无 `api`）——仅当命中来自**归属轨**（该 change 的 `edit.map` 累计路径集合非空）
- 声明 `none` 但任一归属轨启发式命中
- 声明的文档文件路径不存在

启发式输入规则：

1. **归属轨优先**：事实库中该 change 的 `edit.map` 累计路径集合存在且非空时，启发式仅对该集合执行；归属轨命中 SHALL 判 FAIL（会话真实编辑过该文件，证据强）。
2. **黑名单过滤（两轨统一）**：启发式输入 SHALL 剔除工具自管文件（`openspec/changes/**/.openspec.yaml`、`openspec/changes/archive/**` 下 openspec 归档移动产物）与仓库级共享文档（根/子目录 `AGENTS.md`、`docs/reference/constraints-index.md` 等自动注册索引）；被剔除路径 MUST NOT 触发"疑似遗漏"与"声明 none 但命中"。checkbox 显式声明的文档文件对账不受黑名单影响（第 2、5 条以 git 全树为准不变）。
3. **回退轨降级警告**：归属地图为空（`edit.map` 词汇上线前的存量 change、或全程无绑定档会话编辑）时，回退全树 `git diff --name-only <base>` + 未跟踪文件执行启发式，但命中 SHALL NOT 判 FAIL，SHALL 输出「无法归属，全树回退仅提示」警告（不改变退出码）；"声明了未更新""路径不存在"对账保持 FAIL 不变。

**configuration 域正则收紧**：configuration 域启发式 MUST NOT 命中 `openspec/` 前缀路径（如 `openspec/changes/configure-dsh-energy-research/.openspec.yaml` 的 "configure" 字样不得触发配置域误报），保留对真实配置文件（`docker-compose*.yml`、`config*.yaml`、`backend-go/internal/platform/config/`）的命中。

`doc-impact-excuse` 豁免机制退役：输入收窄后"其他 active change 脏文件干扰"误报源消失，不再新增 excuse 注释；既有 `doc-impact-excuse` 注释 SHALL 保持解析兼容（不报错、不判 FAIL），仅文档标注废弃。

#### Scenario: 声明了未更新

- **WHEN** tasks.md 声明 `docs/reference/api/feeds.md` 但 git 改动集合中无此文件
- **THEN** verify 输出 `声明了未更新: docs/reference/api/feeds.md` 且退出码非零

#### Scenario: 疑似遗漏

- **WHEN** 反向启发式扫描的改动集合（归属地图优先、空则回退全树 git 改动集合且仅提示）含 `backend-go/internal/reader/handler/foo.go` 且该集合为 change `foo` 的归属集合（归属轨命中）、声明中无 `api`
- **THEN** verify 输出 `疑似遗漏: 改了 handler 未声明 api` 且退出码非零

#### Scenario: 疑似遗漏按归属地图过滤

- **WHEN** change `foo` 归属集合仅含 `backend-go/internal/dataenrichment/service/a.go`，树上另有归属 change `bar` 的 `backend-go/internal/reader/handler/foo.go`，`foo` 声明中无 `api`
- **THEN** verify 的反向启发式仅扫 `foo` 归属集合，`bar` 的 handler 文件不触发 `疑似遗漏: 改了 handler 未声明 api`

#### Scenario: 工具自管文件不触发疑似遗漏

- **WHEN** 会话绑定 change `foo` 时执行了其他 change 的归档命令，树上出现 `openspec/changes/archive/<other-change>/.openspec.yaml` 变动（或被记入 `foo` 归属集合）
- **THEN** 该路径被黑名单剔除，不触发任何域的"疑似遗漏"或"声明 none 但命中"

#### Scenario: 共享文档不触发疑似遗漏

- **WHEN** change `foo` 的归属集合含 `AGENTS.md`（注册文档时顺带编辑）且 `foo` 声明 `none`
- **THEN** 共享文档被启发式黑名单剔除，不判 FAIL

#### Scenario: openspec 元数据不命中 configuration 域

- **WHEN** 启发式输入含 `openspec/changes/configure-dsh-energy-research/.openspec.yaml`
- **THEN** configuration 域正则不命中该路径（openspec/ 前缀排除），不输出 `疑似遗漏: configuration`

#### Scenario: 归属地图为空时回退全树

- **WHEN** change `foo` 在事实库中无 `edit.map` 记录（存量 change），树上存在其他 change 的未提交 `backend-go/internal/reader/handler/foo.go` 且 `foo` 声明中无 `api`
- **THEN** verify 回退全树执行启发式，命中时输出「无法归属，全树回退仅提示」警告，但退出码不因此非零

#### Scenario: 既有 excuse 注释兼容

- **WHEN** tasks.md 含历史遗留的 `<!-- doc-impact-excuse: ... -->` 注释
- **THEN** verify 解析不报错、不因该注释存在判 FAIL

#### Scenario: 历史存量豁免

- **WHEN** change 归档日期早于 doc-impact-gate 生效 cutoff
- **THEN** check-standards.sh F 段跳过该校验

### Requirement: 文档死链检查

归档门禁 SHALL 对导航层文档（`docs/README.md`、`docs/reference/*.md` 一级、`docs/reference/flow/README.md`、`docs/reference/architecture/map.md`）执行 markdown 相对链接死链检查，失效链接 SHALL 判 FAIL。

#### Scenario: README 死链

- **WHEN** `docs/README.md` 含相对链接 `](getting-started.md)` 且 `docs/getting-started.md` 不存在
- **THEN** check-standards.sh G 段输出 FAIL 并列出该链接

### Requirement: 归档命令硬门禁（spec-gate）

pi 扩展 `.pi/extensions/spec-gate.ts` SHALL 拦截 bash 工具中匹配 `openspec archive` 的命令，在放行前强制执行四项检查：① `scripts/harness/doc-impact.sh verify <change-dir>` 退出码为 0；② `scripts/harness/check-standards.sh --change <change>` 无失败；③ 该 change 的 tasks.md 含「测试/文档/验证」尾三节及 doc-impact 标记；④ `scripts/harness/scenario-trace.sh <change-dir>` 退出码为 0。`check-standards.sh --change <change>` SHALL 保留仓库级 A-E、G-H 标准检查，但 F 段只校验该目标 change 的 doc-impact，MUST NOT 因其他 active change 的 doc-impact 失败而失败。未传 `--change` 的手动 `check-standards.sh` SHALL 继续校验全部 active change。目标 change 不存在或自身 doc-impact 失败时，范围校验 MUST 失败并给出明确原因。任一失败 MUST block 并输出中文 reason（列失败项 + 修复指引）。豁免通道：命令显式带 `--force` 或环境变量 `SPEC_GATE_BYPASS=1`（MUST 记 warning，不得静默放行）。开关：`SPEC_GATE_ENABLE`（默认开启）。

#### Scenario: 门禁未过时归档被拦截
- **WHEN** agent 执行 `openspec archive <change>` 且目标 change 的 doc-impact 对账失败
- **THEN** 命令被 block，reason 列出目标 change 的失败项与修复指引

#### Scenario: 三项全过时放行
- **WHEN** 目标 change 的 doc-impact、任务结构、Scenario 映射以及 `check-standards.sh --change <change>` 的仓库级检查均通过
- **THEN** 归档命令正常执行

#### Scenario: 无关 active change 不阻断目标归档
- **WHEN** 目标 change 的 doc-impact 对账通过，而另一 active change 的对账失败
- **THEN** `check-standards.sh --change <目标change>` 的 F 段只报告目标 change 通过，spec-gate 不因另一 change 的失败阻断归档

#### Scenario: 目标 change 对账失败仍阻断归档
- **WHEN** 归档目标的 tasks.md 缺声明或其 doc-impact 对账失败
- **THEN** `check-standards.sh --change <目标change>` 返回非零，spec-gate 阻断归档并指向目标 change 的失败原因

#### Scenario: 手动全仓巡检保持全量语义
- **WHEN** 开发者不带参数执行 `bash scripts/harness/check-standards.sh`
- **THEN** F 段继续遍历全部 active change 并报告每个 change 的 doc-impact 状态

#### Scenario: 归档目标不存在
- **WHEN** 调用 `check-standards.sh --change` 时目标 change 目录不存在
- **THEN** 脚本返回非零并输出目标 change 不存在，不执行静默的全仓回退

#### Scenario: 归档门禁使用目标范围
- **WHEN** agent 执行不带 bypass 的 `openspec archive <change>`
- **THEN** spec-gate 将该 change 传入 `check-standards.sh --change`，其余归档检查与原有阻断语义不变

#### Scenario: 显式豁免留痕
- **WHEN** 命令带 `--force` 或 `SPEC_GATE_BYPASS=1`
- **THEN** 归档放行且记录一条 warning（不静默）

### Requirement: quota-gate fail-open 落盘可观测

quota-gate 在额度查询失败而放行（fail-open）时 MUST 追加一条 custom_message 落盘记录（含失败原因），使"查询失败放行"与"未触发"在会话记录中可区分。

#### Scenario: 查询失败放行留痕
- **WHEN** quota 查询接口失败且按 fail-open 策略放行派发
- **THEN** 会话记录中出现一条 custom_message 说明"quota 查询失败已放行"及原因

### Requirement: 全量测试软守卫（test-scope-guard）

pi 扩展 `.pi/extensions/test-scope-guard.ts` SHALL 仅在识别出**真·全量后端测试**时介入提醒：命令中存在以 `go` 为可执行文件、`test` 为子命令的调用段，其包参数为**裸** `./...` 或 `...`，且该调用段的**生效工作目录**解析为仓库根下的 `backend-go` 目录（取该段之前最后一个 `cd <path>`，相对会话 cwd 解析并规整；无 `cd` 时取会话 cwd）。判定 MUST NOT 依赖"命中 `...` 即全量"的单正则。

以下情形 MUST 判定为非全量并放行（零提醒、零记账）：`./internal/<域>/...`、`./internal/...`、`./cmd/...` 等子树或前缀模式；单包路径；从 `backend-go` 子目录发起的 `./...`；无 go.mod 的仓库根发起的 `./...`。

判定 MUST 先做 `bash -c` / `sh -c` 内联脚本递归：payload 抽取 MUST 基于**原始命令文本**（先于文本掩蔽，单/双引号作为 payload 定界符），递归判定时 MUST 继承 `-c` 调用点之前解析出的生效 cwd（payload 内自有 `cd` 时在递归层内正常生效）。本级文本 MUST 先做文本掩蔽：单引号/双引号内文本、行内 `#` 注释、heredoc（`<<` 至闭合标记，未闭合时保守掩蔽至命令串尾）正文与 `cat` / `cat >>` 重定向目标 MUST 掩为占位，MUST NOT 从掩蔽内容中提取测试调用；掩蔽 MUST 只作用于该部分文本，命令其余部分中的真实调用段 MUST 照常参与判定（如 `go test ./... && cat > report.txt <<EOF` 仍判全量，`go test ./... | cat > log` 不借豁免溜走）。段切分 MUST 把 `(`、`)` 与 `;`、`&&`、`||`、`|`、换行同等作为分隔符，并 MUST 剥离 `timeout` / `env` / `nice` / `command` 等前缀修饰与前置变量赋值。

扫描通道 MUST 覆盖 `bash`、`ctx_execute`（仅当 `language` 为 shell 时解析 `code`）、`ctx_batch_execute`（逐 `commands[].command`）；quality-gate 内部 `pi.exec` 执行的命令不在扫描范围（该链路只执行影响包测试）。

放行语境 MUST 包含：归档语境（命令或最近 15 条会话条目命中 `归档|§11|archive|验证节|pre-push`）、显式逃生注释（命令尾 `# archive-gate` 或等价别名 `# allow-full-test`）、以及依赖变更语境（命令或最近 15 条会话条目命中 `/依赖变更|建议全量/`——`change-scope.sh` 在 go.mod/go.sum 变更时输出的官方建议语）。放行语境与逃生注释的判定 MUST 作用于**原始命令文本**（文本掩蔽之前）。守卫自身注入的提醒/阻断文案 MUST NOT 构成放行语境（实现：文案统一携带 `[test-scope-guard]` 前缀，语境扫描过滤含该前缀的条目——防止首次阻断后的同会话自噬豁免）。命中任一放行语境时放行：零 policy.decision 记账、零 warn/block 警示（info 记账指引除外——归档与依赖变更语境放行均附测试欠账登记指引，同一分支）。

模式开关 `TEST_SCOPE_GUARD=soft|hard|off` 与默认值 soft 保持不变；soft 只提醒不阻断，hard 阻断并提供逃生注释指引；归档语境放行时附测试欠账登记指引（`scripts/harness/test-patrol.sh --register`）的行为保持不变。

（接受的能力边界）包模式来自 shell 变量、循环展开或命令替换（如 `for d in $DOMAINS; do go test ./$d/...`）时判定**可能判不出**，此类情形 MUST 按放行处理（宁可漏报不误伤）；本 Requirement MUST NOT 被解读为覆盖全部全量写法。

#### Scenario: 影响包命令不触发（现状误报回归）

- **WHEN** 会话 cwd 为仓库根，命令为 `cd backend-go && go test -short ./internal/reader/...`
- **THEN** 零 policy.decision、零 UI 提醒，命令正常执行

#### Scenario: 日常误跑全量测试触发提醒

- **WHEN** 生效 cwd 解析为 `<repoRoot>/backend-go`，命令为 `go test ./...`（含 `-short` / `-count=1` 等 flag，或 `timeout 600 go test ./...`、`X=1 go test ./...`、`bash -c '… && go test ./...'` 等形式）
- **THEN** soft 模式下发出软提醒（建议只跑影响包）并记一条 `policy.decision(warn, full-go-test)`，命令不被阻断

#### Scenario: 子树与子目录不算全量

- **WHEN** 命令为 `cd backend-go && go test -short ./internal/... ./cmd/...`，或 `cd backend-go/internal/domain/x && go test ./...`
- **THEN** 判定为非全量，零提醒零记账

#### Scenario: 写文件命令豁免

- **WHEN** 命令为 heredoc 写入 `apply-report.md`，正文中出现 `go test ./internal/...`
- **THEN** 零提醒零记账，写入不被阻断

#### Scenario: 引号内文本不构成命令

- **WHEN** 命令为 `grep -rn "go test ./..." docs/`
- **THEN** 零提醒零记账

#### Scenario: ctx 通道同样受守卫约束

- **WHEN** 真·全量命令经 `ctx_execute`（`language=shell`）或 `ctx_batch_execute` 的 `commands[].command` 发出
- **THEN** 与 bash 通道一致地提醒/记账（hard 模式下阻断）；经 `ctx_execute` 且 language 非 shell 时 MUST NOT 解析其代码文本

#### Scenario: 依赖变更语境放行

- **WHEN** 会话最近 15 条条目或命令命中 `/依赖变更|建议全量/`，且命令为真·全量
- **THEN** 放行：零 policy.decision、零 warn/block 警示，附 info 测试欠账登记指引（与归档语境同分支）

#### Scenario: 逃生注释与归档语境仍放行

- **WHEN** 真·全量命令尾带 `# archive-gate` 或 `# allow-full-test`，或命令/近期会话上下文命中归档语境
- **THEN** 放行零记账；归档语境放行时仍附测试欠账登记指引

### Requirement: 归档溯源校验宽限期

check-standards.sh E 段（flow 变更溯源校验）SHALL 对**归档日期在宽限期内**（默认 3 天，脚本内常量可调）的 archive change 免检溯源链接——§12 流程本身为"归档后补溯源"，宽限窗口内的债 SHALL NOT 阻断其他 change 的归档；超过宽限期的 archive change 仍未被 `docs/reference/flow/*.md` 变更溯源表引用时，E 段 SHALL 判 FAIL（溯源债仍由归档门禁催收）。tasks.md 声明「无 flow 影响」的既有豁免语义保持不变。

#### Scenario: 宽限期内未溯源不 FAIL

- **WHEN** change `bar` 于 2 天前归档且尚未补 flow 变更溯源链接，change `foo` 此刻归档触发 check-standards.sh E 段
- **THEN** `bar` 因在宽限期内被跳过，E 段不因 `bar` 未溯源而 FAIL

#### Scenario: 超宽限期未溯源仍 FAIL

- **WHEN** change `bar` 归档已超过 3 天且仍未被任何 flow 文档变更溯源表引用
- **THEN** check-standards.sh E 段输出 `未溯源 bar` 并判 FAIL（任何归档动作被催收该债）

#### Scenario: 无 flow 影响豁免保持

- **WHEN** archive change 的 tasks.md 含「无 flow 影响」声明
- **THEN** E 段跳过该 change 的溯源校验（宽限期不参与判断）
