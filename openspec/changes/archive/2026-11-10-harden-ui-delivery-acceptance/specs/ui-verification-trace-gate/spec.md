## MODIFIED Requirements

### Requirement: report.md 建账格式契约

`ui-verification/report.md` SHALL 支持标题式 `### A-<N> <场景>【pass|fail|未覆盖】` 加紧随依据，以及现有表格式场景/判定/依据；A数字与 A-数字 SHALL 规范化为同一稳定场景锚点。首次新场景使用新编号，复验 MUST 保留旧记录并以同编号追加。默认存在性模式仍统计所有合法记录，交付模式以同源 parser 得到的历史/最新记录及非法形似条目执行 v1；不得在扩展中复制 parser。未覆盖 MUST 含原因。v1 条目域仅为根级场景编号标题行和首列A编号位置的表格候选行；表头/分隔行、blockquote、列表、散文与围栏代码中的A-ID提及不算条目。默认模式可继续按合法记录对账；v1 对条目域内非法记录、必验缺失、复验标识与新证据的更严格处理见 ui-delivery-acceptance。

#### Scenario: 标准条目解析
- **WHEN** 标题式或表格式包含合法场景编号、封闭判定与非空依据
- **THEN** parser SHALL 记录规范化编号、判定、依据、行序和本条截图引用，并计入证据对账

#### Scenario: 判定词表外不合法
- **WHEN** 条目使用搞定/基本可用等词表外判定或缺依据
- **THEN** 默认模式不将其计入合法记录，v1 交付模式 MUST 明确阻断而非静默忽略后取旧 pass

#### Scenario: 格式对齐实战形态
- **WHEN** report 使用现有表格或 A1/A-1 标题形态
- **THEN** 两种形态 SHALL 用同一解析路径，不需重写历史账本才可采用 v1

#### Scenario: 复验沿用场景锚点保留失败
- **WHEN** 表格与标题混排，同一 A-ID 后追加复验 pass
- **THEN** parser SHALL 保留历史并按文件行序提供最新记录，默认存在性计数不因历史保留而失效

### Requirement: 判定逻辑单一事实源

归档门禁与 archive-readiness MUST 调用 `scripts/harness/ui-verification-trace.sh <change-dir> --delivery`，共享证据与适用交付结果的同一解析/退出码；扩展内不得保留平行字样匹配或重新解析 report。无参数命令仍是独立的证据对账入口，不可将其成功冒充交付合格。脚本缺席或 spawn 环境异常仍按既有 fail-open 留痕；脚本明确返回未就绪 MUST 阻断，不可当作环境异常。适用/存量边界、视口和复验判定见 ui-delivery-acceptance。

#### Scenario: 门禁与脚本同源判定
- **WHEN** 同一 v1 change 分别由 delivery、自查和硬门禁检查
- **THEN** 三者对 UI 是否未就绪 SHALL 一致，默认存在性成功不抵消结果失败

#### Scenario: 扩展内旧字样检查退役
- **WHEN** tasks 含 agent-browser/1440/1920 等字样但无实物或结果未通过
- **THEN** 归档仍按同源脚本判定，不让关键词构成平行放行条件

#### Scenario: 环境异常与明确未就绪分开
- **WHEN** 脚本正常返回 1，或脚本不可执行/缺席导致 spawn 异常
- **THEN** 前者 MUST 按 ui-verification-missing 阻断；后者 SHALL 保持既有 fail-open 告知和显著裁决记账，不互相混淆
