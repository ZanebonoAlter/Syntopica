# test-cases — add-change-lifecycle-retro

主链路故事：用户对任一 change 运行 `--lifecycle` → 读到声明元数据 + 四段体检卡 → 放行态与时距口径可信 → 放弃的 change 有终态。测试层：smoke fixture（harness-retro.smoke 扩组 + change-abandon.smoke 新建）+ 真库回检。

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

## 变体走查

- 输入：change 名带日期前缀（误传归档目录名）→ 剥前缀处理或明确报错（实现取一，smoke 断言）；空参数 → usage 退出。
- 前置：账本无该 change 任何事件 → L1 步 3 降级；proposal 头注释缺 marker → 「声明缺席」标注不报错。
- 幂等：--lifecycle 重复运行 → 输出恒等、账本零写入；abandon 重复登记 → 消费幂等。
- 时间窗口：B'/成本卡数据起点 09-17（rollup 上线）前 → 覆盖率降级标注（既有口径复用）。
- 不适用划除：并发（脚本单进程只读）；UI 可用性（无 UI）。
