# review — add-ui-verification-trace-gate

- 审查方式：派子线程（zai-coding-cn/glm-5.3 high，只读）聚焦审查实现 diff——脚本判定逻辑 / spec-gate ⑥' 接线 / 正则契约三处高风险面，对照 proposal/design/specs/tasks 制品
- 审查日期：2026-10-13
- 结论：总评一致性好（实战 fixture 零改造通过；lib 瘦身与 design D1 逐项吻合；四套 smoke 与 spec Scenario 对齐）；High ×1、Medium ×4、Low ×5，全部当轮处置（1 修复 + 4 修复 + 5 修复/采纳，见下）
- 边界探针佐证：审查者以 /tmp fixture 复现 P1 locale 敏感（UTF-8 rc=1 / LC_ALL=C rc=0）后按建议修法收敛

### R-1 2026-10-13【High】未覆盖「≥6 字」原因阈值 locale 敏感，C/POSIX 环境检查失效

- **状态**：已修复（新增 utf8_charlen：iconv 转 UTF-32 按码点计数，locale 无关、非法输入保守判无原因；smoke 补 ⑰ C locale 回归钉；实测 UTF-8 与 LC_ALL=C 双环境「待补」均判无原因）

### R-2 2026-10-13【Medium】截图对账全文扫描，误伤非证据图片路径提及

- **状态**：已修复（shot_tokens 跳过跨两层以上目录的 token（如 ui-prototype/evidence/x.png）；速记形态 `A/-Y.png` 单层斜杠不受影响，实战 19 张对账复验不变）

### R-3 2026-10-13【Medium】minor 档词表命中无否定语境，否定表述误触发通道要求

- **状态**：已采纳（轻量版保留字面命中语义、不做否定语境解析；fix_minor_bare 指引补「否定句如『无需人工验证』也会触发，措辞请避开词表词」，误拦可按指引改写措辞）

### R-4 2026-10-13【Medium】依据行可吞标题行，缺依据条目被洗白、词表外条目静默消失

- **状态**：已修复（scan_report pending 态遇标题行/表格行按缺依据丢弃并正常处理本行；词表外 A-N 编号条目补显式 DROPPED 提示不再静默消失）

### R-5 2026-10-13【Medium】冒烟白盒缺口：表格式未覆盖红 / none marker 不一致红 / 子串兜底负例

- **状态**：已修复（smoke 补 ⑱ 表格式未覆盖无原因红、⑲ none marker(=minor)≠声明红、⑳ shot.png≠screenshot.png 子串不命中仍红）

### R-6 2026-10-13【Low】none 档 marker 缺失口径脚本/lib 不对称

- **状态**：已采纳（保持分工：marker 缺失由 lib 严判，脚本只对账「存在但不一致」；脚本 none 分支补口径注释说明双门禁合并后语义闭合）

### R-7 2026-10-13【Low】minor 分支丢弃 DROPPED_NOTES，诊断信息不对称

- **状态**：已修复（minor manual 分支 scan_report 后补打印 DROPPED_NOTES，与 major 分支对称）

### R-8 2026-10-13【Low】「编号自 A-1 自增」未强制

- **状态**：已采纳（脚本头注释言明编号不校验连续性/自增，宽松执行免歧义）

### R-9 2026-10-13【Low】修复指引未写明原因字数下限，短理由用户只能试错

- **状态**：已修复（fix_shell_report 文案补「≥6 字，占位符如『待补』不算」）

### R-10 2026-10-13【Low】archive-readiness 注释计数过期 + ⑦/⑥' 两套编号无映射说明

- **状态**：已修复（注释「五个脚本」→「七个脚本」；聚合段头补编号映射注释：本脚本 ⑦ = spec-gate 检查⑥'，⑥ 已被 human-feedback 提醒项占用）
