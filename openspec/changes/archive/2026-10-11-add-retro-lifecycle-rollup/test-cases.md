# Test Cases — add-retro-lifecycle-rollup

## 用户故事

跑一次全局 harness 体检（`--days N`），不逐个调 `--lifecycle` 也能看到人工反馈面：多少 change 有反馈账、未处置几条、归因高发在哪层、哪些多轮返工的 change 一直没人建账（漏采信号）。

## 主链路

| 步 | 动作 | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 窗口内存在有账/无账 change 混合 | E 子组输出反馈覆盖计数（有账 N / 无账 M / 未处置条数） | 脚本 smoke（fixture 落真库） | `harness-retro.smoke.sh` fixture：两 change 一有账一无账断言计数 |
| 2 | 窗口内反馈条目归因各异 | 归因分布聚合计数正确（backend×2 + test-gap×1 → 对应行） | 脚本 smoke | 同 fixture 断言 |
| 3 | 某 change 波次 ≥3 session 且零反馈 | 漏采信号名单含该 change；低波次或有账 change 不进名单 | 脚本 smoke | 同 fixture 断言（三态：进名单/不进/不进） |
| 4 | 全部 change 无反馈文件 | E 子组输出「有账 0 + 无反馈记录」降级，不静默省略 | 脚本 smoke | 空 fixture 断言 |
| 5 | 反馈文件有词表外归因条目 | 该条目计「格式异常」单列，子组不崩 | 脚本 smoke | 坏格式 fixture 断言 |
| 6 | 真库跑 `--days 7` | ⑦ 段后出现 E 生命周期面子组，数值与手工 `--lifecycle` 逐 change 汇总一致（抽查 1 个 change） | 命令验收 | tasks T4 |

## 变体走查（五组固定清单）

- 输入：~~空串/空白/分隔符/单token/大小写/特殊字符/超长~~——不适用：输入是目录名与固定格式文件，无自由文本入参，划除留痕。
- 前置：空集（窗口内零 change → E 子组输出零计数降级不省略，主链路步 4 覆盖）；单元素（仅 1 个 change）；重复（同 change 双登记不产生——文件唯一）。活跃无归档/归档无活跃/混合三态由清单口径覆盖（smoke fixture 至少含活跃态）。
- 时间窗口：边界两端——窗口内归档的边界（archive 日期前缀 = 窗口 since 当日 → 计入；前一日 → 不计入）在清单扫描用 `>= since` 日期字符串比较，smoke 以两目录跨边界断言；空窗口同前置空集。
- 幂等：重复执行——报告只读，连跑两次输出一致（smoke 断言两次输出 diff 为空）；并发不适用（本地单实例工具，无并发安全主张）。
- 可用性：~~误输入/空态/错误态/加载态/超长文本/重复提交~~——不适用：CLI 无 UI，空态即前置空集降级路径，划除留痕。

## 继承与调整（⓪ 改契约了吗：是，ADDED requirement「生命周期面汇总」）

无既有测试资产反查需求（该 requirement 为全新能力面，`bash scripts/harness/test-assets.sh harness-retro-loop` 反查旧资产：lifecycle 卡第五段相关断言在 harness-retro.smoke.sh 既有 K/L 系列——本 change 不动其行为，仅新增全局模式断言，无需处置旧测试）。
