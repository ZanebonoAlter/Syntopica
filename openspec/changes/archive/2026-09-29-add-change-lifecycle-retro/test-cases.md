# test-cases — add-change-lifecycle-retro

主链路故事：用户对任一 change 运行 `--lifecycle` → 读到声明元数据 + 五段体检卡（含人类反馈段）→ 放行态与时距口径可信 → 放弃的 change 有终态 → 反馈建账有工具、归档有对账。测试层：smoke fixture（harness-retro.smoke 扩组 + change-abandon.smoke 新建 + feedback.smoke 新建）+ 真库回检。

## 主链路表（故事 L1：体检卡输出）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 对已归档、事件完整的 change 跑 `--lifecycle` | 体检卡四段齐备 | 四段输出 + 头部 ui-impact/complexity/constraint-domains 声明 | smoke | harness-retro.sh |
| 2 | 对未归档 active change 跑 | 未归档 change 不假定终态 | 「未归档」标注 + 最后活动时间，成本卡照出 | smoke | harness-retro.sh |
| 3 | 对无事件 change 跑 | （数据缺席降级） | 各段降级标注而非静默省略，退出码 0 | smoke | harness-retro.sh |
| 4 | 运行前后账本计数 | 只读不变更账本 | COUNT 不变 | smoke | mode=ro 打开 |

## 主链路表（故事 L2：放行记账与时距分派）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | major 档原型获用户批准 | UI 审批通过与放行被记录 | policy.decision action=pass、reasonCode=ui-approval-approved、change 绑定 | fixture | ui-design-gate |
| 2 | block 后补齐文档放行 | 同上 | pass、ui-design-satisfied | fixture | ui-design-gate |
| 3 | ui-design-gate 样本时距 | 非 gate 裁决解除信号按语义分派 | 以 pass 事件计时；无 pass 的历史按推断口径标注；quota block 后无 gate.check 变化（89 分钟假慢反例）不入时距、计剔除披露 | smoke | retro ⑦段 A 组 |
| 4 | pass 事件入③④段 | （消费侧排除） | 不入 fail-open 分桶、不入失效 warn 聚类 | smoke | retro 消费侧 |

## 主链路表（故事 L3：终态登记）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 对存在且未归档的 change 登记放弃 | 放弃终态被记录 | change.abandon 落库（payload.name、change 同名） | smoke | change-abandon.sh |
| 2 | 对不存在的 change 登记 | 参数非法零记录 | 零记录、非零退出 | smoke | change-abandon.sh |
| 3 | 对已归档 change 登记 | （拒绝已归档） | 零记录、非零退出、提示 | smoke | change-abandon.sh |
| 4 | 重复登记同一 change | （幂等语义） | append-only 多条，消费侧按名聚合 | smoke | 消费侧 |
| 5 | abandon 与 archive 并存 | 终态与归档冲突以归档为准 | 消费侧判已归档 + 冲突提示行 | smoke | lifecycle/B' 统计 |

## 主链路表（故事 L4：human-feedback 建账与计量）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 对存在 human-feedback.md（FB-1~10）的 change 执行 `feedback.sh add --layer frontend --type 产品 "原话"` | add 建账编号自增 | 新条目 FB-11，标题行含【产品|frontend】，状态行「待处理」，其余内容不变 | smoke | feedback.sh |
| 2 | 对无 human-feedback.md 的 change 执行 add | 同上 | 按模板创建（含头部定位/消费方/格式说明）后建账 | smoke | feedback.sh |
| 3 | add 传词表外归因（如 --layer ui） | 词表外值零写入 | 非零退出，文件零修改 | smoke | feedback.sh |
| 4 | 对含待处理条目的目录执行 `feedback.sh verify` | verify 拦截未处置条目 | 处置提醒列出未处置条目 + 退出码 1；全处置/无文件→退出码 0 | smoke | feedback.sh |
| 5 | 对归档目录中的 change 执行 add | 已归档 change 拒绝建账 | 拒绝写入 + 提示走归档后修订流程 | smoke | feedback.sh |
| 6 | 对含 human-feedback.md 的 change 跑 `--lifecycle` | 体检卡五段齐备 | 第五段输出条数（产品/流程）/归因分布/未处置数 | smoke | harness-retro.sh |
| 7 | 对无 human-feedback.md 的 change 跑 `--lifecycle` | 人类反馈段缺席降级 | 第五段输出「无反馈记录（或未启用）」，其余四段正常，不静默省略 | smoke | harness-retro.sh |
| 8 | human-feedback.md 含词表外归因或缺状态行条目 | 格式异常条目单列披露 | 该条计「格式异常」单列，可解析条目照常统计，段输出不中断 | smoke | harness-retro.sh |
| 9 | archive-readiness 对含未处置条目的 change 跑 | （归档对账顺带调用） | 输出反馈处置提醒行，不阻断硬门禁 | smoke | archive-readiness.sh |

## 变体走查

- 输入：change 名带日期前缀（误传归档目录名）→ 剥前缀处理或明确报错（实现取一，smoke 断言）；空参数 → usage 退出。
- 前置：账本无该 change 任何事件 → L1 步 3 降级；proposal 头注释缺 marker → 「声明缺席」标注不报错。
- 幂等：--lifecycle 重复运行 → 输出恒等、账本零写入；abandon 重复登记 → 消费幂等。
- 时间窗口：B'/成本卡数据起点 09-17（rollup 上线）前 → 覆盖率降级标注（既有口径复用）。
- 输入（L4）：反馈要点含引号/竖线/换行 → 转义处理不破坏标题行格式；FB 编号非连续（如已有 FB-1/FB-5）→ 取最大值自增不受影响。
- 前置（L4）：human-feedback.md 存在但零条目（仅头部说明）→ 同文件缺席降级口径；条目缺归因标签但状态合法 → 计「格式异常」不丢条目计数。
- 幂等（L4）：add 中断重试 → 不产生重复编号（编号取写入时最大值）。
- 不适用划除：并发（脚本单进程）；UI 可用性（无 UI）；可用性五检（非 UI 交付）。
