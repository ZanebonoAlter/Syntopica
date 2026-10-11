# review-trace-gate Specification

## Purpose
把开发执行规范 §0.6 步骤 4「子线程聚焦 review」从纯文本自觉升级为机器可对账的痕迹要求：review 做了要在 change 目录留 review.md 建账，不做要显式 waived 声明，两者皆无则归档门禁阻断。配套让 subagent.dispatch 事件携带 change 归属（可观测性），retro --lifecycle 体检卡可统计 review 覆盖。考古依据：2026-08-23 ~ 2026-10-11 期间 38 次归档仅 ~6 个有 review 派发（~16%），10 月起归零，gate.check 19,983 次中 review 相关 0 次。

## Requirements

### Requirement: review 痕迹对账脚本

系统 SHALL 提供 `scripts/harness/review-trace.sh <change-dir>`（change-dir 形如 `openspec/changes/<name>`），对单个 openspec change 做归档前 review 痕迹对账：只做静态判定，不执行任何测试/编译命令、不修改仓库文件。判定按优先级：① proposal.md 头部存在机器可读豁免声明 `<!-- review: waived -->` → 通过（输出注明 waived）；② change 目录存在 `review.md` 且含至少一条合法条目（或合法的「审毕无发现」声明）→ 通过；③ 两者皆无、或 review.md 存在但零合法条目 → 退出码 1。脚本 SHALL 在 POSIX bash 环境（Linux / macOS / WSL）可运行，退出码 0=通过、1=有失败，失败输出为中文并给出两条修复路径指引（建账或声明豁免）。

#### Scenario: review.md 建账通过

- **WHEN** change 目录存在 review.md，且含至少一条符合建账格式的条目
- **THEN** 脚本退出码 0，输出条目计数

#### Scenario: 审毕无发现声明通过

- **WHEN** review.md 无问题条目，但含合法的「审毕无发现」声明行（含审查范围说明）
- **THEN** 脚本退出码 0，输出注明无发现形态

#### Scenario: waived 豁免通过

- **WHEN** proposal.md 头部机器可读注释区存在 `<!-- review: waived -->`
- **THEN** 脚本退出码 0，输出注明 waived 豁免形态（供 retro 统计区分）

#### Scenario: 零痕迹阻断

- **WHEN** review.md 与 waived 声明两者皆无
- **THEN** 脚本退出码 1，输出指明缺 review.md 与豁免声明，并给出建账格式或声明位置指引

#### Scenario: 空壳文件阻断

- **WHEN** review.md 存在但不含任何合法条目或合法无发现声明
- **THEN** 脚本退出码 1，输出指明文件为空壳（有文件不等于审过）

### Requirement: review.md 建账格式契约

review 意见建账文件 SHALL 位于 change 目录 `review.md`，随 change 归档永久保存（不进 events.db）。文件头部 SHALL 含一行定位说明；每条意见一个条目：标题行 `### R-<N> <日期>【High|Medium|Low】<标题>`（编号自 R-1 自增、日期 `YYYY-MM-DD`），下一行为状态行 `- **状态**：<处置词>`（括号说明可选）。处置词封闭五值：`待处理`、`已修复`、`已采纳`、`已驳回`、`误报丢弃`（对应规范步骤 4 的 High 必修 / Medium 选修 / Low 误报丢弃纪律）。零发现的 review SHALL 以显式声明行 `- 审毕无发现（审查范围：<说明>）` 记录（不允许只留空文件）。等级词表封闭三值 High/Medium/Low。

#### Scenario: 标准条目解析

- **WHEN** review.md 含 `### R-1 2026-10-12【High】并发窗口内双重 upsert` 与 `- **状态**：已修复`
- **THEN** 判定为一条合法 High 条目

#### Scenario: 词表外等级不合法

- **WHEN** 条目等级写成 Critical（词表外）
- **THEN** 该条目不计入合法条目，若全文件无其他合法条目则对账失败

#### Scenario: 状态词表外不合法

- **WHEN** 状态行写成「搞定」（词表外）
- **THEN** 该条目不计入合法条目，若全文件无其他合法条目则对账失败

### Requirement: waived 显式豁免声明

豁免 review 的 change SHALL 在 proposal.md 头部机器可读注释区（与 `ui-impact` / `complexity` / `constraint-domains` 同层）写 `<!-- review: waived -->`。声明不携带理由字段（理由写在 proposal 正文）；代码类 change 亦可声明（agent 自担质量责任），waived 占比由 retro --lifecycle 统计暴露滥用。SHALL NOT 提供第三种豁免形态（无声跳过即门禁红）。

#### Scenario: 纯文档 change 豁免

- **WHEN** 纯文档 change 在 proposal.md 头部声明 waived
- **THEN** review-trace 对账通过，retro 体检卡该 change 记为 waived 形态

#### Scenario: 声明位置不识别

- **WHEN** `review: waived` 只出现在 proposal 正文段落而非头部机器可读注释区
- **THEN** 豁免不被识别，对账按零痕迹处理

### Requirement: 归档门禁集成（spec-gate 第五项 block 检查）

`openspec archive` 前置门禁 SHALL 在既有四项 block 检查（doc-impact verify / check-standards / tasks.md 尾三节 / scenario-trace）之外新增第五项：`bash scripts/harness/review-trace.sh <changeDir>` 退出码 0。该项与前四项各自独立判定、任一失败即阻断归档，失败输出与修复指引并入既有 reason 格式；豁免放行文案（`--force` / `SPEC_GATE_BYPASS=1`）SHALL 同步枚举新增检查项。逃生口沿用既有机制不新增；存量 in-flight change 不豁免。

#### Scenario: 归档时零痕迹阻断

- **WHEN** agent 执行 `openspec archive <change>` 且前四项通过而 review-trace.sh 退出码非 0
- **THEN** 归档被阻断，reason 含缺痕迹说明与 review.md 建账 / waived 声明两条修复路径

#### Scenario: 豁免放行留痕

- **WHEN** review-trace.sh 退出码非 0 但以 `--force` 或 `SPEC_GATE_BYPASS=1` 归档
- **THEN** 归档放行，warning 文案枚举含 review 痕迹检查未执行的留痕说明

#### Scenario: 自查脚本同口径聚合

- **WHEN** 运行 `scripts/harness/archive-readiness.sh <change>` 自查
- **THEN** 输出含 review 痕迹检查项（硬检查语义，见 archive-readiness capability），判定实现与门禁同一脚本事实源

### Requirement: retro 体检卡 review 覆盖段

`harness-retro.sh --lifecycle <change>` 体检卡 SHALL 新增「review 覆盖」段，输出：① 痕迹三态（review.md 建账 / waived / 两者皆无——皆无且已归档记为缺口）；② review 类子线程派发计数（kind=subagent.dispatch 且 change=本 change，type 为 change-review-glm 或 desc 含 review/审查关键词，标注为参考口径）；③ review 修复派发计数参考（desc 含修复 review 意见类关键词）。dispatch 事件 change 归属改造落地前的历史时段，统计 SHALL 显示「归属改造前无数据」而非按零误报。

#### Scenario: 建账 change 覆盖段展示

- **WHEN** 对存在 review.md 的 change 运行 --lifecycle
- **THEN** review 覆盖段显示建账形态与条目计数，review 派发计数来自 dispatch 事件

#### Scenario: 归属改造前不误报

- **WHEN** change 的 review 派发发生在 dispatch change 归属改造落地前（事件 change 列为空）
- **THEN** 覆盖段展示「归属改造前无数据」标注，不输出 0 次派发的误导值
