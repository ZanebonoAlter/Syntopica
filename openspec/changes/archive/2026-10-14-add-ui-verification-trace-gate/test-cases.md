# add-ui-verification-trace-gate test-cases

测试单元：**UI 验收证据实物对账门禁**（一个 Requirement 的用户故事：UI change 归档时，验收证据由独立脚本只读对账——建账真实、截图在盘、映射指名道姓；字样不再放行）。由下列 fixture 场景串成完整故事，落点全部在冒烟脚本层（纯逻辑判定，无业务库/前端产物依赖）。

## 1. 主链路表（fixture 场景 → smoke 断言）

| # | 节拍（动作） | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | major 档：建账 2 条（pass+未覆盖带原因）、截图在盘 | major 档实物齐全通过 | rc=0，输出条目计数 + 截图对账 | 脚本冒烟 | ui-verification-trace.smoke.sh ① |
| 2 | major 档：无 ui-verification/ 目录 | major 档零痕迹阻断 | rc=1，两条修复路径（建账/显式降档） | 脚本冒烟 | ui-verification-trace.smoke.sh ② |
| 3 | major 档：report.md 全是散文零条目 | major 档空壳建账阻断 | rc=1，给出建账格式契约 | 脚本冒烟 | ui-verification-trace.smoke.sh ③ |
| 4 | major 档：条目点名 ke-not-taken-1440.png 不在盘 | major 档点名截图缺失阻断 | rc=1，列缺失清单 | 脚本冒烟 | ui-verification-trace.smoke.sh ④ |
| 5 | major 档：唯一条目【未覆盖】依据「待补」 | 未覆盖场景无原因阻断 | 不计入合法 → 零合法空壳红 | 脚本冒烟 | ui-verification-trace.smoke.sh ⑤（C locale 回归钉 ⑰） |
| 6 | minor 档：验收节写真实存在的测试路径（seam 注入伪仓库根） | minor 档指名道姓通过 | rc=0，输出「文件在盘」 | 脚本冒烟 | ui-verification-trace.smoke.sh ⑥ |
| 7 | minor 档：验收节仅「组件测试覆盖；人工验证过」 | minor 档裸关键词阻断 | rc=1，指名道姓指引 | 脚本冒烟 | ui-verification-trace.smoke.sh ⑦/⑦b |
| 8 | minor 档：agent-browser/人工映射 + report.md 一行表格式条目 | minor 档指名道姓通过 | rc=0（不要求截图） | 脚本冒烟 | ui-verification-trace.smoke.sh ⑧ |
| 9 | none 档：N/A Reason 节 + marker 一致 | none 档一致性维持 | rc=0 | 脚本冒烟 | ui-verification-trace.smoke.sh ⑨ |
| 10 | .openspec.yaml schema: spec-driven | 旧 schema change 不硬阻断 | rc=0 fail-open 零检查 | 脚本冒烟 | ui-verification-trace.smoke.sh ⑩ |
| 11 | 实战形态回归：表格式条目 + `X.png / -Y.png` 速记 | 格式对齐实战形态 | rc=0，速记截图兜底命中 | 脚本冒烟 | ui-verification-trace.smoke.sh ⑪ |
| 12 | 门禁接线：major 实物齐（tasks 零字样）走完整归档 tool_call | 门禁与脚本同源判定 | 放行零 policy.decision | 扩展冒烟（真跑真脚本） | spec-gate.smoke.cjs 8a |
| 13 | 门禁接线：opencli/1440/1920 字样齐备但无实物 | 字样齐备但无实物不得通过 | block + 透传脚本零痕迹结论 + ui-verification-missing 记账 | 扩展冒烟（真跑真脚本） | spec-gate.smoke.cjs 8b |
| 14 | 脚本绿 + 审批 pending（规划制品红） | 扩展内规划制品检查保留 | block 且 reason 含 approved | 扩展冒烟 | spec-gate.smoke.cjs 8c |
| 15 | minor 建账在案 / 测试路径不存在 / 无映射 | minor 指名映射通过 + 裸关键词不再放行 | 放行 / block（路径不存在）/ block（无映射） | 扩展冒烟（真跑真脚本） | spec-gate.smoke.cjs 8d/8d2/8e |
| 16 | block → 补齐实物 → 再归档 | 门禁与脚本同源判定（解除记账） | pass ui-design-satisfied 恰一条，再归档零新增 | 扩展冒烟 | spec-gate.smoke.cjs 8g2 |
| 17 | spawn 失败（bash 不可用） | 门禁与脚本同源判定（fail-open） | 检查⑥' 放行该项 + 零 ui-verification-missing 记账 | 扩展冒烟 | spec-gate.smoke.cjs 8i |
| 18 | legacy schema 走完整归档 | 旧 schema change 不硬阻断 | 放行零事件（脚本 fail-open + lib legacy 空） | 扩展冒烟 | spec-gate.smoke.cjs 8g |
| 19 | major+新 schema tasks 缺 `ui-verification/report.md` 任务行 | 三道防线任务行缺失阻断 | rc=1 报缺行（无勾选框引用行不救场） | 脚本冒烟 | check-tasks-tail.smoke.sh ⑫ |
| 20 | major 含任务行 / 旧 schema / none / 无 .openspec.yaml | 三道防线边界 | 均不受影响 rc=0 | 脚本冒烟 | check-tasks-tail.smoke.sh ⑬-⑯ |
| 21 | 聚合自查：stub 六脚本全绿 / major 零痕迹真跑红 | 自查脚本同口径聚合 | 6×✓ exit 0 / ✗⑦ 透传修复路径 exit 1 | 脚本冒烟 | archive-readiness.smoke.sh ①⑨ |

## 2. 变体走查（五组固定清单）

- **输入**：空 report.md（③ 空壳覆盖）｜纯空白验收节（⑦b「待定。」覆盖）｜单 token 条目行（⑬ 单条目覆盖）｜判定大小写（词表小写 pass，实战形态即小写，⑪ 覆盖；大写 PASS 不识别=词表封闭，白盒附加 4）｜超长依据行（正则仅锚定行首结构，超长无害——grep -oE 行内扫描，无回溯爆炸面）。特殊字符变体：判定标注含全角括号（实战 A1「未覆盖（本期无建议…）」⑪ 覆盖）。
- **前置**：空集（② 无目录）｜单元素（⑧ 仅 report.md 无截图）｜越界引用（④ 点名不存在的图）｜部分满足（13 字样齐但无实物）。
- **时间窗口**：不适用（脚本无时间语义；report.md 执行日期字段不参与判定）——划除留痕。
- **幂等**：重复执行（脚本只读，重复跑结论不变——archive-readiness.smoke ④ 只读安全 case + spec-gate 8g2c 二次归档零新增）｜部分失败重试（16 先 block 后补齐转绿）｜并发：不适用（单文件只读无共享态）——划除留痕。
- **可用性**：不适用（CLI 门禁无 UI）——划除留痕；脚本自身可用性=中文修复指引可执行（②③⑦ 各含 FIXEOF 指引，冒烟断言关键字）。

## 3. ⓪ 继承与调整（`bash scripts/harness/test-assets.sh ui-design-workflow` 反查）

改动契约：MODIFIED「UI 完成验收必须对照批准基线」（opencli→agent-browser、字样→实物对账、minor 指名道姓）。旧资产逐行处置如下，**不静默删**：

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| major UI 双层验收通过 | 继承改写 | spec-gate.smoke.cjs 旧 8a（tasks 含 opencli/1440/1920 字样即放行） | 改写为新 8a：**tasks 零字样** + 实物齐全（report.md 建账+截图）才放行——断言新契约「字样不再是必要条件」 |
| 只有单元测试不得完成 major UI | 继承改写 | spec-gate.smoke.cjs 旧 8b（缺字样 → block） | 改写为新 8b：**字样齐备无实物 → block**（断言旧放行通道退役）；「只有单测」形态由 8b 同 fixture 承接（TASKS_NO_EVIDENCE 无实物） |
| 字样齐备但无实物不得通过（delta 新增） | 新增 | spec-gate.smoke.cjs 8b | 新断言：旧关键字样（opencli/1440/1920 全写上）不再构成放行条件 |
| minor 指名映射通过 / minor 裸关键词不再放行（delta 新增） | 新增 | spec-gate.smoke.cjs 8d/8d2/8e + ui-verification-trace.smoke.sh ⑥⑦⑧ | lib 旧 8d（关键词正则放行）随 missingArchiveUiEvidence 瘦身退役（ui-design-gate.smoke D8b 断言 minor 恒空=判定下放脚本） |
| 其余 18 个 Scenario（声明/规划制品/布局契约/记账） | 不触及 | ui-design-gate.smoke.cjs / 组件测试 | 契约未动，原样继承（D8 两条速测随接口瘦身微调：去掉 tasksMd 入参、加「无差异字样也放行」回归锚） |

## 4. 白盒附加（复杂档：分支表 + 边界值）

| # | 分支/边界 | 输入形态 | 期望 | 落点 |
| --- | --- | --- | --- | --- |
| W1 | 判定优先级 | proposal 无 ui-impact / 旧 schema | fail-open rc=0（声明完整性归 ui-design-gate） | smoke ⑩⑫ |
| W2 | 头部注释区边界 | `<!-- ui-impact: major -->` 只出现在正文段落 | 不识别 → fail-open（与 review-trace/check-tasks-tail 同款 head_markers 边界） | smoke ⑫ + check-tasks-tail.smoke ⑪ |
| W3 | 依据行空行穿插 | 标题式条目与依据行间 1-2 个空行 | 合法条目（markdown 惯例空行容忍） | smoke ⑬ |
| W4 | 词表外判定 | `### A-1 x【搞定】` | 不计入合法条目 → 零合法空壳红 | smoke ⑭ |
| W5 | 条目缺依据行 | 标题式条目后紧跟另一条目/文件尾 | ⚠ 不计入（不致命，除非零合法） | 脚本 DROPPED_NOTES 路径（review 期人工核对） |
| W6 | 未覆盖原因阈值 | 依据+标注合并去空白 <6 字（如「待补」） | 不计入合法；另有合法条目时仅 ⚠ 不阻断（对应 spec「若全文件无其他合法条目则对账失败」） | smoke ⑤ |
| W7 | 截图速记兜底 | token 以 `-` 开头 → 尾匹配任意前邻；无杠 token 仅前邻非字母数字才命中（`final.png` 不命中 `myfinal.png`） | 实战速记命中、子串误放行被拒 | smoke ⑪ + 实战文本预演（11 条/19 张全过） |
| W8 | 表格式判定格 | cell2 `**pass**`/`**未覆盖（…）**` 词表外（`**搞定**`）或 cell1 非 `A-?[0-9]+` | 不计入合法条目 | 脚本 is_table_row 分支（W4 同通道，标题式 fixture 代言） |
| W9 | minor 组件测试路径解析 | 节内 `(front\|backend-go)/…(.test\|.spec).(ts…)` 或 `*_test.go` 首个 token | 路径不存在 → 红；无路径纯关键词 → 红 | smoke ⑥⑦ + spec-gate 8d2 |
| W10 | spawn 失败语义 | pi.exec 抛异常（code -1） | 检查⑥' fail-open warn，**不**推 failures（其余检查 code -1 仍 block——差异语义见 spec-gate 注释） | spec-gate.smoke 8i |
| W11 | 原因阈值 locale 无关（review R-1） | LC_ALL=C 下「待补」依据（bash 字节计 6） | 仍判无原因 → 不计入合法（utf8_charlen 按码点计数） | smoke ⑰ |
| W12 | 表格式未覆盖无原因（review R-5） | `\| A1 x \| **未覆盖** \| 待补 \|` | 不计入合法 → 零合法空壳红 | smoke ⑱ |
| W13 | none marker 不一致（review R-5） | proposal=none、ui-design 头部 marker=minor | rc=1 报不一致 | smoke ⑲ |
| W14 | 子串兜底负例（review R-5） | 点名 shot.png、盘上仅 screenshot.png | 仍红（前邻字母数字不命中，不放松到任意子串） | smoke ⑳ |
| W15 | 路径提及不入对账（review R-2） | 正文提 ui-prototype/evidence/x.png（两层目录） | 不算点名证据（单层速记 `A/-Y.png` 不受影响） | 实战文本预演（19 张复验） |
| W16 | 依据行吞标题防御（review R-4） | 标题式条目后紧跟 `## 结论` 或词表外 `### A-2 x【搞定】` | 前者按缺依据丢弃，后者显式 DROPPED 不静默消失 | 脚本 DROPPED_NOTES 路径（review 期人工核对） |

## 5. 效果核对

不适用——本 change 为 harness 门禁机制，判定结果即脚本退出码，无「依赖断言外因素（数据覆盖率/LLM 行为）」的效果维度；归档后可用 `bash scripts/harness/harness-retro.sh` 观察 waived/空壳占比（design Risks 节回检口径）替代量化核对。
