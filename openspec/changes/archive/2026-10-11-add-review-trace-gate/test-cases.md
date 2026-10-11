# add-review-trace-gate Test Cases

complex 档测试故事账本。测试单元 = 「review 从无痕迹自觉到机器对账」完整故事：建账/豁免/阻断三路 × 脚本/门禁/账本三层。

## 主链路表

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 构造含 2 条合法条目的 review.md，跑 review-trace.sh | review.md 建账通过 | 退出码 0，输出条目计数 2 | 脚本 smoke | review-trace.smoke.sh |
| 2 | 构造仅「审毕无发现」声明的 review.md | 审毕无发现声明通过 | 退出码 0，输出无发现形态 | 脚本 smoke | review-trace.smoke.sh |
| 3 | proposal.md 头注释区写 `<!-- review: waived -->` | waived 豁免通过 | 退出码 0，输出 waived 形态 | 脚本 smoke | review-trace.smoke.sh |
| 4 | 零痕迹 change 目录跑 review-trace.sh | 零痕迹阻断 | 退出码 1，中文输出含建账/豁免两条修复路径 | 脚本 smoke | review-trace.smoke.sh |
| 5 | 有 review.md 但全词表外条目 | 空壳文件阻断 / 词表外等级 / 状态词表外 | 退出码 1，指明空壳 | 脚本 smoke | review-trace.smoke.sh |
| 6 | waived 只写正文段落不写头注释区 | 声明位置不识别 | 豁免不识别，按零痕迹处理 | 脚本 smoke | review-trace.smoke.sh |
| 7 | spec-gate 拦截 openspec archive（零痕迹 fixture） | 归档时零痕迹阻断 | block，reason 含两修复路径 | 扩展 smoke | spec-gate.smoke.cjs |
| 8 | SPEC_GATE_BYPASS=1 归档放行 | 豁免放行留痕 | warning 枚举含 review 检查未执行 | 扩展 smoke | spec-gate.smoke.cjs |
| 9 | archive-readiness.sh 聚合自查 | 同口径聚合 / 全绿（五项）/ 多项红汇总 / review 痕迹红项聚合 | 五项 ✓/✗，⑤ review-trace 红透传原因尾部 20 行 | 脚本 smoke | archive-readiness.smoke.sh |
| 10 | 派发 Agent 子线程（有活跃 change 目录） | dispatch 携带 change 归属 | events.db dispatch 行 change=目录名 | 扩展 smoke | telemetry-dispatch.smoke.cjs |
| 11 | 派发 Agent 子线程（无非 archive 目录） | 无活跃 change 时归属可空 | dispatch 行 change=null，落库不阻断 | 扩展 smoke | telemetry-dispatch.smoke.cjs |
| 12 | lifecycle 体检卡对建账 change | 建账 change 覆盖段展示 | 三态展示建账+条目数，派发计数来自 dispatch 事件 | 脚本 smoke | harness-retro.smoke.sh |
| 13 | lifecycle 对改造前 change | 归属改造前不误报 | 显示「归属改造前无数据」，不输出 0 派发误导值 | 脚本 smoke | harness-retro.smoke.sh |

## 继承与调整（问句⓪，改契约 M/R）

| 旧 Scenario（capability） | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 全绿通过 / 多项红一次性汇总 / change 不存在 / 只读安全（archive-readiness，M） | 改语义（四项→五项） | scripts/harness/archive-readiness.smoke.sh | 既有断言改五项口径，change 不存在/只读两条断言不动（语义未变） |
| block 文案含自查指引（archive-readiness） | 不变 | .pi/extensions/tests/spec-gate.smoke.cjs | 零改动，回归跑绿即可 |
| 事件追加不可变（harness-fact-log，M） | 不变（新增行带值不改旧行） | .pi/extensions/tests/harness-log.smoke.cjs | 零改动回归 + telemetry-dispatch.smoke.cjs 补「追加不改写」断言 |
| TTL 分级清扫（harness-fact-log） | 不变 | .pi/extensions/tests/harness-log.smoke.cjs | 零改动回归 |
| 词汇扩展系 6 条 Scenario（harness-fact-log） | 不变 | harness-log.smoke.cjs / 对应扩展 smoke | 零改动回归 |

## 变体走查（问句②）

| 组 | 变体 | 答案 |
| --- | --- | --- |
| 输入 | review.md 空文件 / 仅头部说明 | 空壳红（步 5 覆盖） |
| 输入 | 条目编号跳号（R-1、R-3） | 合法——编号自增供人读，脚本只计合法条目数不校验连续性 |
| 输入 | 全角【】缺失 / 等级小写 high | 不合法（词表封闭 High/Medium/Low 逐字匹配） |
| 输入 | 状态行带括号说明「已修复（fixup commit abc）」 | 合法（括号说明可选，feedback.sh 同款语义） |
| 前置 | proposal.md 不存在 | 按 waived 未声明处理，走 review.md 判定 |
| 前置 | review.md 与 waived 同时存在 | 通过，输出注明双痕迹（waived 优先级仅影响输出措辞） |
| 幂等 | review-trace.sh 重复执行 | 只读判定天然幂等 |
| 时间窗口 | 归属改造前历史 dispatch（change 列 null） | lifecycle 标注无数据不误报（步 13） |
| 可用性 | 门禁红后补建 review.md 再归档 | 复跑通过即放行（自我修复路径） |

不适用划除：UI 可用性组（无 UI）；时间窗口归一化（无窗口语义）。

## 白盒附加（complex 档）

review-trace.sh 判定分支表：

| 分支 | 输入 | 出口 |
| --- | --- | --- |
| B1 proposal 头注释 waived | 头注释区命中 `<!-- review: waived -->` | 通过（waived 形态） |
| B2 review.md 缺席 + B1 未命中 | 两文件皆无 | 红（零痕迹） |
| B3 review.md 存在 | 逐行扫描条目正则与无发现声明 | ≥1 合法 → 通过 / 0 合法 → 红（空壳） |
| B4 waived 位置错误 | 正文段落含 waived 字样 | 不识别，落 B2/B3 |

边界值：条目正则容忍标题内任意非换行字符；日期格式 `YYYY-MM-DD` 逐字（容忍 R-<N> 后多空格）；状态行容忍 `**状态**` 前后空白。dispatch 归属边界：detectActiveChange 遍历目录 mtime 的空目录/权限失败 → null 容错（既有函数语义，回归断言覆盖）。

## 效果核对（问句④）

效果断言依赖落地后真实使用，本 change 交付门禁与统计能力本身（效果指标 = 后续 change 的 review 覆盖率，由 retro 体检卡滚动可见）。落地后回检：下一会话对任一新 change 跑 `harness-retro.sh --lifecycle`，确认 review 覆盖段三态展示与 dispatch 归属生效（tasks 6.4 人工留痕）。
