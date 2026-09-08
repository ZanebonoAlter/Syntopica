# doc-impact-gate Delta — fix-doc-impact-misattribution

## MODIFIED Requirements

### Requirement: 归档前对账（verify）

归档门禁 SHALL 运行 `bash scripts/doc-impact.sh verify <change-dir>` 对每个 active change 对账，以下任一条件 SHALL 判 FAIL：

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

## ADDED Requirements

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
