# review-task-frontload Test Cases

complex 档测试故事账本。测试单元 = 「review 任务行从 propose 建档起就在场」完整故事：complex 缺行拦截 / 含行放行 / waived 豁免 / simple 兼容 / apply 入口提前暴露，五路全走 check-tasks-tail.sh 单一事实源。锚 Requirement：tasks review 任务行前移检查（specs/review-trace-gate delta，ADDED）。

## 主链路表

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | fixture：proposal 头 `<!-- complexity: complex -->`（无 waived），tasks 无任何 review 任务行（夹带误伤样本：正文 review 字样无勾选框、普通任务行无关键词），跑 check-tasks-tail.sh | complex 档 tasks 缺 review 任务行被拦 | 退出码 1，输出指出缺 review 任务行并给两条修复路径（补任务行 / proposal 头部 waived） | 脚本 smoke | check-tasks-tail.smoke.sh ⑦ |
| 2 | fixture：complex 档 tasks 含 `- [ ] 6.1 派子线程聚焦 review 并 review.md 建账`，尾三节 + doc-impact 齐备 | complex 档 tasks 含 review 任务行通过 | 退出码 0，尾三节等既有判定行为不变 | 脚本 smoke | check-tasks-tail.smoke.sh ⑧ |
| 3 | fixture：complex 档 proposal 头加 `<!-- review: waived -->`，tasks 无 review 任务行 | waived 豁免放行 | 退出码 0（豁免词表与检查⑤ review-trace 同源同边界） | 脚本 smoke | check-tasks-tail.smoke.sh ⑨ |
| 4 | fixture：`<!-- complexity: simple -->` 与无 complexity 声明两形态，tasks 无 review 任务行 | simple 档向后兼容 | 退出码 0，SHALL NOT 因缺行失败；正文（非头部注释区）complexity marker 同样不识别（⑪） | 脚本 smoke | check-tasks-tail.smoke.sh ⑩⑪ |
| 5 | agent 经 openspec-apply-change 进入 complex 档实现，按 skill 约定实现前先跑 check-tasks-tail.sh | apply 入口提前暴露 | 缺行在实现开始前被看见并补上，SHALL NOT 待归档才暴露 | 人工（skill 约定步，非机器门禁） | 人工：下一真实 change apply 时观察（tasks 7.3 同一观察点） |

## 继承与调整（问句⓪；delta 为 ADDED，既有 capability 资产反查留痕）

`bash scripts/harness/test-assets.sh review-trace-gate` 反查（2026-10-12 快照：主 specs 5 Requirements / 15 Scenarios；archive 历史 2026-10-11-add-review-trace-gate 含 test-cases.md；30 行 Scenario→测试映射）：

| 旧 Scenario（capability） | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| review 痕迹对账五路（建账/无发现/waived/零痕迹/空壳）+ 格式契约三条 + 豁免声明两条（review-trace-gate） | 继承（review-trace.sh 本 change 不改，判定零波及） | scripts/harness/review-trace.smoke.sh | 照跑（回归网，tasks 3.1 全绿确认） |
| 归档门禁集成三 Scenario（review-trace-gate） | 继承（spec-gate.ts 不改，③ 调同一脚本自动吃到新判定，旧断言口径不变） | .pi/extensions/tests/spec-gate.smoke.cjs | 照跑；其 fixture 无 complex marker，新判定不触发（grep 实查 2026-10-12） |
| 全绿通过 / 多项红一次性汇总 / review 痕迹红项聚合 / change 不存在 / 只读安全（archive-readiness，M） | 继承（archive-readiness.sh 不改；③ 内部判定收紧但 fixture 均 non-complex，行为不变） | scripts/harness/archive-readiness.smoke.sh | 照跑；另以本 change 真目录跑 archive-readiness.sh 活体验证新判定（tasks 7.2） |
| lifecycle 体检卡两条（review-trace-gate） | 继承（harness-retro.sh 不改） | scripts/harness/harness-retro.smoke.sh | 照跑（fixture complexity 声明为 simple/无声明，不受影响） |

## 变体走查（问句②）

| 组 | 变体 | 答案 |
| --- | --- | --- |
| 输入 | tasks.md 空文件/纯空白 | complex 档红缺行（无任何可匹配行，正解是补行不是放行）；simple 档不启用判定不受影响 |
| 输入 | 任务行勾选态 `- [x]`（已勾） | 同样命中（正则 `[x ]` 双态），已完成的 review 任务也算在场（步 2 覆盖 `[ ]`，⑧ fixture 同构） |
| 输入 | 任务行只含「审查」不含 review（中文词） | 命中（关键词二选一，不区分大小写只影响 review 拼写） |
| 输入 | `REVIEW`/`Review` 大小写混写 | 命中（grep -i） |
| 输入 | 正文提 review 但无勾选框前缀 | 不命中（误伤样本，步 1 fixture 内嵌验证） |
| 输入 | 普通任务行无 review/审查关键词 | 不命中（步 1 fixture 的 `- [ ] 1.1 干活`） |
| 输入 | 超长任务行（关键词在行尾） | 命中（grep 行匹配无长度上限，关键词位置不限） |
| 输入 | 非 review 任务行蹭到关键词（如 `- [ ] 1.1 干活（review 见第 6 节）`） | 正向宽松：命中放行（已知宽松不收紧，关键词是唯一机器可判信号；「干了没」由检查⑤痕迹对账兜底，design Risks 已登记） |
| 前置 | proposal.md 不存在/不可读 | 按「无 complexity 声明」处理，review 判定整体跳过（向后兼容；tasks.md 缺失仍走既有红） |
| 前置 | complexity 声明值非 complex/simple（如 typo `complexc`） | 不识别为 complex → 不启用判定（marker 逐行全串匹配，词表外值不模糊命中） |
| 前置 | 头注释区复杂 marker 与 waived marker 顺序颠倒 / 中间空行 | 均识别（head_markers 空行可穿插、与顺序无关） |
| 时间窗口 | marker 在正文段落后才出现 | 不识别（解析边界：开头连续注释行遇正文停，⑪ 覆盖） |
| 幂等 | check-tasks-tail.sh 重复执行 | 只读判定天然幂等，fixture 临时目录互不污染 |
| 可用性 | 红时输出可操作性 | 缺行红附带两条修复路径原文（补任务行示例 / waived 声明位置），尾三节红文案逐字保真不变 |

不适用划除：时间窗口归一化（无窗口语义）；可用性组 UI 项（无 UI，纯 CLI 输出）。

## 白盒附加（complex 档）

check-tasks-tail.sh 新增判定分支表：

| 分支 | 输入 | 出口 |
| --- | --- | --- |
| B1 头部 marker 解析 | proposal.md 开头连续 `<!-- -->` 行（空行可穿插），遇正文停 | 产出 markers 流；正文段落/代码块里的 marker 不进流（同 review-trace.sh head_markers 逐字同实现） |
| B2 complex 判定 | markers 命中 `^<!--\s*complexity\s*:\s*complex\s*-->$`（逐行全串） | 命中 → 启用；未命中（simple/无声明/typo/正文 marker）→ 判定不启用 |
| B3 豁免判定 | markers 命中 `^<!--\s*review\s*:\s*waived\s*-->$`（同检查⑤词表） | 命中 → 放行（优先于 B4，不解析理由） |
| B4 任务行扫描 | tasks.md 全文找 `^- \[[x ]\].*(review\|审查)`（-i） | ≥1 命中 → 放行；0 命中 → 红 + 两条修复指引 |

边界值与误伤防线：任务行匹配锚定行首勾选框（`- [ ] `/`- [x] `），正文/表格/引用块里的 review 字样无此前缀不命中；勾选框后紧跟关键词不要求相邻（行内任意位置）；`<!-- doc-impact:` 既有判定与 B4 互不干扰（missing 数组与 review_missing 独立累加，同红时两段输出都在）；marker 正则与 review-trace.sh 逐字同源——waived 语义两脚本一致，不会出现「③判豁免、⑤判不豁免」的口径漂移。

豁免优先级（B3 先于 B4）：complex + waived + 无任务行 → 放行（⑨）；complex + 无 waived + 有任务行 → 放行（⑧）；complex + 无 waived + 无任务行 → 红（⑦）。

## 效果核对（问句④）

效果断言依赖落地后真实使用（下一个 complex change 的 tasks 是否天生带 Review 节），本 change 交付判定与约定注入本身。落地后回检双通道：① 下一真实 change 走 propose 流程时观察 tasks 是否自带 Review 节（tasks 7.3 人工观察点，与主链路步 5 同点）；② 归档门禁侧本 change 自吃狗粮——`bash scripts/harness/archive-readiness.sh review-task-frontload` ③ 项应展示新判定生效（tasks 7.2 活体验证）。
