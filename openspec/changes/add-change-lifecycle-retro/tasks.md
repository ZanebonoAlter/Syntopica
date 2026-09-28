## 1. ui-design-gate 放行记账（design D2）

- [x] 1.1 `.pi/extensions/lib/ui-design-gate.ts`（及 gate 接线处）action 枚举扩 `pass`，两个放行场景记 `policy.decision(action=pass)`：`ui-approval-approved`（用户明确批准原型时刻）、`ui-design-satisfied`（block 后补齐放行时刻），change 绑定该 change，验证 fixture 两 reasonCode 各落一条且不与既有 block 记账双写
- [x] 1.2 harness-retro 消费侧适配：③④段与催修时距对 pass 显式排除（不入 fail-open 分桶、不入失效 warn 聚类），验证 smoke 分桶断言
- [x] 1.3 词汇表与 smoke 稳定集同步：harness-facts SKILL.md policy.decision 词汇表 + action 枚举更新（block|warn|bypass|fail-open|pass）

## 2. change.abandon 终态登记（design D5）

- [x] 2.1 新增 `scripts/harness/change-abandon.sh <change-name>`：校验 change 存在于 openspec/changes/ 且未归档（archive 目录存在则拒绝）→ 直写 change.abandon 事件（payload 含 name、change 列同名、session_id 取 PI_SESSION_ID 或脚本名，对齐 test-patrol.sh 直写先例）；参数非法非零退出零记录，验证脚本对存在/不存在/已归档三态行为
- [x] 2.2 消费侧终态判定：`--lifecycle` 与 ⑦段 B' 幸存者偏差统计将 abandon change 区分为「已放弃」；同 change 名下 archive 与 abandon 并存以 archive 为准并输出冲突提示，验证 fixture 冲突提示行

## 3. retro --lifecycle 体检卡（design D1/D6）

- [x] 3.1 `scripts/harness/harness-retro.sh` 新增 `--lifecycle <change>` 模式：四段输出（流程规范/UI 工作流/成本卡/健康度）+ proposal 头注释元数据联合（未归档读 changes/、已归档读 archive/ 目录日期前缀剥离），全程 `mode=ro` 只读打开，验证对已归档/未归档/无事件 change 三态输出与降级标注
- [x] 3.2 成本卡按 models map 分桶与 rollup 覆盖降级标注（对齐 ⑦段 B' 口径），UI 工作流段放行态双口径（pass 事件直读 / 无事件按末 block 推断并标注），验证 fixture 双口径输出
- [x] 3.3 归档顺序依赖执行（design D4）：确认 add-foreign-gate-steer-attribution 已归档 sync 后再动 harness-fact-log 主 spec；若本 change 先行，按其归档后主 spec 手动重放 pass 枚举差异（apply 时验证：add-foreign-gate-steer-attribution 已于 2026-09-29 归档 sync，主 spec 已含归因降级词汇与 scenario；本 delta 已补入该 scenario 为合并后状态，openspec validate 绿）

## 4. 催修时距分派（design D3）

- [x] 4.1 ⑦段 A 组催修效率按 policy 分派解除信号（分派表：quality-gate→gate.check 转绿；ui-design-gate→pass 事件；quota-gate→后续 pass/放行；spec-gate→change.archive），信号缺席样本剔除并披露剔除数，验证 fixture 含 89 分钟假慢反例（quota block 后无 gate.check 变化不计入时距）

## 5. 测试

- 本 change 为 harness 工具链改造，交付验证 = smoke（harness-retro.smoke 扩 L 组 lifecycle 用例 + change-abandon.smoke 新建 + pass 词汇稳定集）+ 只读断言（lifecycle 前后账本计数不变）；测试设计见 test-cases.md，用例故事编号被 smoke 引用（aggregate-concurrent-gate-warns 先例）。

## 6. 文档

<!-- doc-impact: none（harness 层，无业务域） -->

- [x] 6.1 `.agents/skills/harness-retro/SKILL.md`：--lifecycle 用法、报告解读、催修时距分派口径
- [x] 6.2 `.agents/skills/harness-facts/SKILL.md`：词汇表补 pass 两个 reasonCode 与 change.abandon 事件（payload/保留期/消费侧冲突规则）
- [x] 6.3 `docs/reference/harness/pi-extensions.md`：ui-design-gate 放行记账、终态登记脚本说明

## 7. 验证

1. `bash scripts/harness/change-abandon.smoke.sh` → 新建 smoke 全绿（登记/拒绝不存在/拒绝已归档/幂等重跑）
2. `bash scripts/harness/harness-retro.smoke.sh` → 既有全量 + 新 lifecycle 组（四段齐备/未归档标注/只读断言/放行双口径/时距分派剔除计数）
3. `bash scripts/harness/harness-retro.sh --lifecycle daily-report-margin-notes`（真库）→ 输出四段与 explore-findings 探索数据一致（成本 $28.80 量级、ui-prototype-missing ×2、19 sessions）
4. `bash scripts/harness/harness-retro.sh --lifecycle <某未归档 active change>` → 「未归档」标注 + 最后活动时间，不假定终态
5. 只读不变更：`--lifecycle` 前后 `sqlite3 .pi/harness/events.db "SELECT COUNT(*) FROM events"` 计数不变
6. pass 词汇稳定：smoke 稳定集含 ui-approval-approved / ui-design-satisfied，全库 reasonCode 枚举无漂移
