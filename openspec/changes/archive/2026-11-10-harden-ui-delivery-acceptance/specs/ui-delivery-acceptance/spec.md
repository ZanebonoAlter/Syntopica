## Purpose

在证据存在性对账之外，用同一账本解析源检查 UI 必验故事、失败复验和逐场景视口证据；不把脚本当作产品定位、美感或截图真实性的裁判。

## ADDED Requirements

### Requirement: 交付模式与分阶段兼容

`scripts/harness/ui-verification-trace.sh <change-dir> --delivery` SHALL 先执行原有证据对账，再对 `syntopica-ui` 的 major change 执行适用的交付合同检查，保持只读且不运行测试/构建。无参数的默认模式 SHALL 仅检查证据存在性，不因账本包含 fail 或有原因的未覆盖而改变该职责。none、minor 与旧 spec-driven 不进入 v1 结果判定。

major 的 `ui-design.md` 文件开头连续单行 HTML 注释区（可穿插空行，遇首个非注释/非空白行即结束，与既有 head_markers 同域）内，唯一 `<!-- ui-acceptance-contract: v1 -->` SHALL 显式启用 v1。未知、非法、重复或位于该头部区之外的实际合同 marker MUST 阻断交付，包括存量 major，不得因位置错误降级为“未迁移”；围栏代码或行内代码中展示的 marker 示例不是实际声明。自 `.openspec.yaml` 的 `created >= 2026-10-27` 的新 major MUST 使用 v1，缺 marker MUST 阻断。此前的 major 未迁移时 SHALL 保持证据层兼容，并明确输出“仅证据齐备，未迁移交付合同”，不得宣称结果合格；无法证明创建日期的 major 缺合同 MUST 给出缺前置错误。不得新增环境变量改变此边界或跳过结果检查；沿用已有显式归档绕过并留痕的纪律。

#### Scenario: 默认模式仍只检查证据
- **WHEN** major 账本有合法 fail 和带原因的未覆盖，但点名实物齐备
- **THEN** 默认模式 SHALL 按证据存在性返回 0，并明确不将其解释为交付结果合格

#### Scenario: 试点显式启用合同
- **WHEN** 创建日期早于边界的 major 显式声明 v1
- **THEN** delivery 模式 MUST 执行全部 v1 规则，不因其是存量而跳过

#### Scenario: 新 major 缺合同阻断
- **WHEN** major 的创建日期不早于 2026-10-27 且缺 v1 marker，或缺少可判定创建日期
- **THEN** delivery MUST 返回 1，指明补合同的路径，不猜测它属于存量

#### Scenario: 存量未迁移明确降级
- **WHEN** major 早于边界且没有 marker
- **THEN** delivery SHALL 在证据对账成功后返回 0，明确告知只有证据齐备而无结果判定，不新增普通放行 policy 事件

#### Scenario: 非法合同不作存量放行
- **WHEN** major 声明未知版本、非法值、重复 marker，或把实际合同注释放在头部连续注释区之外
- **THEN** delivery MUST 返回 1，即使创建日期较早也不得回退为未迁移

#### Scenario: 非 major 与旧 schema 兼容
- **WHEN** change 为 none、minor 或旧 spec-driven
- **THEN** delivery SHALL 保持相应原有证据规则，不要求 v1 的必验故事或 PNG 视口映射

### Requirement: 必验故事合同只有一个来源

v1 MUST 在 `ui-design.md` 的 `## Acceptance` 下包含唯一 `### Delivery Contract` 表，固定列为 `A-ID | 必验故事与打开状态 | 必验视口`。每行 MUST 有一个规范化场景编号（A-数字，兼容 A数字）、非空故事说明，以及以逗号分隔的正整数 `宽x高` 视口列表。场景 MUST 唯一，表 MUST 非空，未知/非法行、重复场景或畸形视口 MUST 阻断。视口只由该表承诺，不再增加平行全局视口 marker。修改必验故事范围、交互或布局 SHALL 按既有 major 审批纪律重新批准，不得仅从表中删除问题以制造全绿。

#### Scenario: 必验故事从未验过
- **WHEN** 合同要求 A-43，但账本没有该场景的合法条目
- **THEN** delivery MUST 返回 1 并点名缺失的 A-43

#### Scenario: 合同空壳或行不合法
- **WHEN** v1 缺唯一合同表、表为空、A-ID 重复、故事为空或任一视口格式非法
- **THEN** delivery MUST 返回 1 并给出具体合同缺口，不把非法行静默忽略

### Requirement: 最新结论与失败复验不得洗绿

账本中的 A-ID SHALL 是稳定场景锚点，表格/标题形态共享同一解析与编号规范化；按文件出现顺序保存历次合法记录，最后记录为该场景最新结论。追加复验 SHALL 保留旧记录。v1 中任意场景最新为 fail MUST 阻断，不限于必验表；任何必验场景最新不是 pass（含未覆盖）MUST 阻断。非必验未覆盖可以有原因地保留，但 SHALL 逐条输出编号与原因。不得因“转挂某个存在的 change”或自填豁免而放行已知失败/核心未覆盖。

存在先前 fail/未覆盖的场景若追加 pass，其依据 MUST 以“复验”开头，说明检查后的状态并点名至少一份未出现在该场景最近一次非 pass 依据中的新截图。仅追加 pass、改掉原 fail、复用旧失败图或只有转挂说明不构成复验。v1 的条目判定域 MUST 限于两个语法锚点：根级 `### A…` 的场景编号标题行，以及首列处于 A 编号位置的表格行（含编号畸形但明显占用该位置的候选行）。表头/分隔行、blockquote、列表、普通散文和围栏代码块均不属于条目域，提及 A-ID 不计数也不阻断。在条目域内判定/依据/编号等格式非法的记录 MUST 阻断，不得由于 parser 丢弃而以更早 pass 放行。

#### Scenario: 任意最新失败阻断
- **WHEN** 必验或非必验场景的最新记录为 fail
- **THEN** delivery MUST 返回 1，即使依据写有转挂且承接 change 存在

#### Scenario: 核心未覆盖阻断而非核心明确提醒
- **WHEN** 必验场景最新为未覆盖，同时另有非必验未覆盖及原因
- **THEN** delivery MUST 因必验场景阻断，并保留非必验项的编号/原因提醒

#### Scenario: 追加有效复验后解除
- **WHEN** 同一编号的旧 fail 或未覆盖保留在前，末尾追加 pass，依据以复验开头、有状态说明和新的点名截图，且其余合同要求满足
- **THEN** delivery SHALL 按最新复验判定通过，不因保留历史失败永久阻断

#### Scenario: 无复验或旧失败截图不能解除
- **WHEN** 旧非 pass 后追加 pass，但无复验标识、无状态依据或未点名该场景的新截图
- **THEN** delivery MUST 返回 1，指出复验缺口

#### Scenario: 非法条目不得被忽略洗绿
- **WHEN** 账本曾有 A-43 pass，后又在条目语法锚点出现 A-43 的非法判定/缺依据，或其他条目域候选存在格式错误
- **THEN** v1 delivery MUST 返回 1，而不是忽略错误后继续使用早前 pass；说明/引用/列表/代码样例中仅提及 A-ID 不属于此条件

### Requirement: 视口证据必须属于该场景最新记录

每个必验场景最新 pass 的依据 MUST 点名该场景自己的截图，逐一覆盖本行全部必验视口。只有该条最新记录中的引用可参与尺寸匹配；全文件截图清单、其他 A-ID 的截图和旧失败图不得借用。尺寸 SHALL 读取合法 PNG 的真实 IHDR 宽高，不信文件名；非 PNG 可以留作其他证据，但不计入 v1 视口覆盖。截图路径 MUST 相对 `ui-verification/` 且在其目录内真实存在，不得通过绝对路径、越界或符号链接逃逸参与结果检查。v1 引用 SHALL 支持无空格裸相对路径与 Markdown 链接；带空格路径使用 `[说明](<子目录/有 空格.png>)` 形态，中文文件名/子目录同样有效，不按同名图片猜测目标。图片伪损坏/截断或无法读取尺寸时 MUST 报不满足视口，不得猜尺寸。

脚本只能检查引用归属、路径和尺寸；截图内容是否呈现打开态、点击命中/无重叠断言是否真实、语义等价和产品合理性 MUST 由评审核验，不能由尺寸通过替代。

#### Scenario: 逐场景视口齐全通过
- **WHEN** 必验 A-55 的最新 pass 依据点名实际 1440x900 与 2560x1440 的合法 PNG，覆盖其合同视口
- **THEN** 视口检查 SHALL 通过，不要求以文件名编码尺寸

#### Scenario: 别的页面宽屏图不能借用
- **WHEN** A-55 要求 2560x1440，但该尺寸图只在 A-57 或全文件清单点名，A-55 自己只有 1440x900
- **THEN** delivery MUST 返回 1 并点名 A-55 缺少的视口

#### Scenario: 真实尺寸而非文件名判定
- **WHEN** 名称含 1440 的 PNG 实际为 1280x633，或文件不是可读合法 PNG
- **THEN** 它 MUST NOT 满足 1440x900，delivery 在覆盖不足时阻断；不把文件名直接当证据

#### Scenario: 截图路径不得越界
- **WHEN** 最新条目点名的截图不存在、越出证据目录或经链接解析到目录外
- **THEN** delivery MUST 返回 1，不从仓库其他原型或全局文件系统寻找同名图兜底

### Requirement: 归档结果与自查同源

归档硬门禁与 archive-readiness MUST 对相同 change 调用同一个 `--delivery` 入口，取得证据与结果两层结论，不复制解析或平行放行条件。结果未就绪 SHALL 沿用 `ui-verification-missing` 归档桶，不归为环境失败；现有显式 bypass/fail-open 事件纪律保持，不新增静默绕过。普通存量降级说明不增加成功 policy 噪声。

#### Scenario: 自查与硬门禁同一结果
- **WHEN** 试点有 A-43 最新 fail 或必验未覆盖
- **THEN** delivery、自查和硬门禁 MUST 对 UI 未就绪给出一致结论；默认 trace 证据成功不能抵消它
