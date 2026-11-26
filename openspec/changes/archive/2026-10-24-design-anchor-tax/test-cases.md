# Test Cases — design-anchor-tax

> 工具链/文档 change（无产品代码改动）。主链路是「design 模板 → design 制品落点 → review 复核 → retro 指标」的闭合；落点解析器是唯一分支密集处，白盒附加节给分支表与边界值。解析器实现落 `scripts/harness/harness-retro.sh`（design D4），断言落 `scripts/harness/harness-retro.smoke.sh` N 系列（N1~N4）。

## 用户故事

新开一个 change 写 design.md：每条 Decision 的「落点」能指到仓库里真实存在的代码/任务/Scenario，或写明「无代码落点 + 承接制品 + 理由」，让 review 能把只有结论没有承接的决策标成「可删项」；跑 `harness-retro` 能看到落点率、每决策字数、取舍率，在模板改动前后做准 A/B。

## 主链路

| 步 | 动作 | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | Decision 落点写仓库文件 + 可检索符号（`scripts/harness/harness-retro.sh:resolve_file`） | 判定 resolved，计入落点率 | 脚本 smoke（fixture） | `harness-retro.smoke.sh` N2（n-mixed D1） |
| 2 | 落点写当前 change tasks.md 中存在的任务 id（`1.1`） | resolved | 脚本 smoke | N2（n-mixed D2） |
| 3 | 落点写 delta spec 中存在的 Scenario 标题 | resolved | 脚本 smoke | N2（n-mixed D3） |
| 4 | 无代码决策写「无代码落点：<承接制品>，因为 <理由>」 | resolved，不误伤流程决策 | 脚本 smoke | N2（n-mixed D4） |
| 5 | 只写「无代码落点」、没承接制品/理由 | 视为缺失，不计入落点率 | 脚本 smoke | N2（n-vague D4） |
| 6 | 落点写「见 tasks」「待定」等空泛表述 | 视为缺失，不计入落点率 | 脚本 smoke | N2（n-vague D1/D2） |
| 7 | 落点写仓库内不唯一的 basename（`design.md`） | 歧义按未解析，不计入 | 脚本 smoke | N2（n-vague D3） |
| 8 | 文件存在但 `:symbol` 在文件中检索不到 | 未解析，不计入 | 脚本 smoke | N2（n-vague D5） |
| 8b | 绝对路径指向仓库外 / 唯一无扩展名 basename | 仓外不计；`Dockerfile` 唯一 basename 计入 | 脚本 smoke | N2（n-vague D6 / n-mixed D7） |
| 9 | 旧格式 Decision 无「落点」行，但正文含真实文件路径 | 正文兜底解析（兼容存量），resolved | 脚本 smoke | N2（n-mixed D6） |
| 10 | 取舍：显式取舍行 / 「无备选：约束」/ 正文「替代方案否决」 | 三种都计入取舍率 | 脚本 smoke | N2（n-mixed D1~D4、D6） |
| 11 | 窗口内 design 无 Decisions 段 | decisions=0，落点率/字数/取舍率输出 null，不伪造 0% | 脚本 smoke | N3/N4（n-empty） |
| 12 | review 读到一条有结论无承接的 Decision | `review.md` 记 Medium，标题含「可删项：D1..Dn（无下游承接）」 | 人工 | `docs/reference/开发执行规范.md` 第 0 条 + 本 change review.md |
| 13 | 决策是无代码落点且写明承接制品与理由 | review 不记条目（流程决策不误伤） | 人工 + 脚本 smoke | 规范第 0 条 + N2（n-mixed D4） |
| 14 | 真库跑 `bash scripts/harness/harness-retro.sh --days 30` | ⑦ 段出现 F 子组与 `m7.design_*`；JSON 含 `effectiveness.design` | 命令验收 | tasks 8.6 |
| 15 | 对已归档 change 跑任何检查 | 不因 design 缺落点/取舍被阻断或要求补写（本 change 不新增门禁） | 人工 | 归档 change 抽查（spec Scenario「存量 change 不回溯」） |

## 变体走查（五组固定清单）

- **输入**：空串/纯空白 → 落点行内容为空，按未解析（无 anchor）；纯分隔符/标点 → token 剥壳后无 anchor，未解析；单 token → 仅当是仓库内唯一 basename/存在的路径才 resolved；大小写 → 路径在 Linux 下大小写敏感，按文件系统真实存在判定；特殊字符（反引号/引号/中文括号/句号）→ `strip` + 分词剥壳，fixture D1~D6 覆盖；超长 → 不做长度上限，字数照计（指标不是门禁，不截断）。
- **前置**：空集（0 decisions）→ N3/N4 断言 null 降级；单元素（1 条 Decision）→ 公式同 N，分母 1；重复（同一落点写两次）→ 只算这条 Decision 一次，不重复计；越界引用（不存在的任务 id/Scenario/文件）→ 未解析（N2 的 D5 用坏符号覆盖）；部分满足（一行多个候选锚点，任一 resolved）→ resolved（n-mixed D1 同时含路径与符号）。
- **时间窗口**：窗口边界 = archive 目录日期前缀 `>= since`（与 B'/E 段同口径，单测由 M 系列既有断言覆盖）；空窗口（无活跃/无窗口内归档）→ designs=0、decisions=0、指标 null；跨窗口（归档在窗口外）→ 不进入样本。
- **幂等**：报告只读（不写账本/不改 change 目录），连跑两次除窗口时间戳外输出一致（M7 既有断言覆盖全报告，F 段确定性由纯函数解析保证）；并发不适用（本地单实例 CLI，无并发安全主张）。
- **可用性**：不适用——CLI 无 UI，无空态/加载态/重复提交；错误态由既有 C4/C5/C6（库缺失/非本应用库/空库）覆盖，F 段在空目录下降级不报错。

## 效果核对

- 不依赖外部数据覆盖率或 LLM 行为：解析对象是仓库内 `design.md` + `tasks.md` + delta spec，结果完全确定。
- 真库量化核对（tasks 8.6）：`bash scripts/harness/harness-retro.sh --days 30 | grep -E "design_landing_rate|design_words_per_decision"` 命中；F 段样本数 > 0 且数值可复算。
- 口径差异已披露（design Context）：探索期 42% 是宽松计数（路径字符串/任务编号出现即算），retro 用严格可解析判定（同窗口约 20% 量级（随仓库状态浮动））。准 A/B 只比较同一解析器的改前/改后，不与 42% 直接对减。

## 白盒附加（分支表 / 边界值）

解析器 `landing_resolves` + `resolve_file` 分支：

| 分支 | 条件 | 期望 | 覆盖 |
| --- | --- | --- | --- |
| B1 | Decision 有「落点」行 | 只解析落点行，不看正文 | N2（n-mixed D1~D4） |
| B2 | Decision 无「落点」行 | 回退扫描整个 Decision 正文 | N2（n-mixed D6） |
| B3 | 落点为空 / 只有空泛词 | unresolved | N2（n-vague D1/D2） |
| B4 | 仓库相对路径存在（绝对路径只认仓库内） | resolved（带符号时再查符号） | N2（n-mixed D1） |
| B5 | 绝对路径指向仓库外（如临时目录文件） | unresolved（spec 限定仓库内，review Medium-1 修复回归） | N2（n-vague D6） |
| B6 | 唯一 basename 命中（含无扩展名如 `Dockerfile`） | resolved | N2（n-mixed D7） |
| B7 | 同名 basename 多处命中 | unresolved（歧义不猜） | N2（n-vague D3） |
| B8 | `:symbol` 不在文件内容中 | unresolved | N2（n-vague D5） |
| B9 | 任务 id 命中当前 change tasks.md | resolved | N2（n-mixed D2） |
| B10 | 任务 id 不在 tasks.md | 不构成 anchor；若别无 anchor 则 unresolved | 同 B3 语义（N2 总数反证） |
| B11 | Scenario 标题命中 delta spec | resolved | N2（n-mixed D3） |
| B12 | Scenario 标题不命中 | 不构成 anchor | 同 B3 语义 |
| B13 | 「无代码落点」+ 承接制品 + 因为/理由 + 理由非空 | resolved | N2（n-mixed D4） |
| B14 | 「无代码落点」缺承接/理由（空壳） | unresolved | N2（n-vague D4） |
| B15 | 取舍行有内容（含「无备选：约束」） | 计入取舍率 | N2（n-mixed D2） |
| B16 | 无取舍行但正文含替代方案表述 | 计入取舍率（兼容存量） | N2（n-mixed D6） |
| B17 | 无 Decisions 段 / 空 design | decisions=0，不产出比率 | N3/N4 |

边界值清单：decisions=0（N3/N4）、decisions=1（公式分母 1，人工口算）、中文路径（`docs/reference/开发执行规范.md`，N2 D4/D6）、带符号路径（N2 D1）、无扩展名唯一 basename（`Dockerfile`，N2 D7）、仓外绝对路径（N2 n-vague D6）、引号/反引号/中文括号包裹（N2 全部 fixture）、同名 basename（N2 D3）、找不到的符号（N2 D5）、窗口边界 `>= since`（M 系列既有）、空 changes 目录（M8 既有 + F 段空样本降级）。

不适用划除：**性能**（<200 份 design 线性扫描，真库实测报告总耗时 ~3.8s，无 SLA）；**并发**（单实例只读 CLI）；**权限**（沿用既有库只读打开路径，C 系列覆盖）。

## 继承与调整（⓪ 改契约了吗：是，ADDED capability `design-decision-landing`）

`bash scripts/harness/test-assets.sh design-decision-landing` 反查：主 specs 与 archive delta specs 均无该 capability（全新能力），无旧测试资产需要处置。既有 `harness-retro.smoke.sh` A~M 系列断言的行为（六段 / 故障分离 / 基线 / 任务成本 / 生命周期）本 change 不改，只新增 N 系列；N 系列断言已全绿。
