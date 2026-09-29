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

- 本 change 为 harness 工具链改造，交付验证 = smoke（harness-retro.smoke 扩 L 组 lifecycle 用例〔含 L12-L14 人类反馈段〕+ change-abandon.smoke 新建 + feedback.smoke 新建 + archive-readiness.smoke 扩 ⑥⑦ 反馈对账 + pass 词汇稳定集）+ 只读断言（lifecycle 前后账本计数不变、human-feedback.md md5 不变）；测试设计见 test-cases.md，用例故事编号被 smoke 引用（aggregate-concurrent-gate-warns 先例）。

## 6. 文档

<!-- doc-impact: standard -->
<!-- doc-impact-excuse: flow=front/app/features 命中为跨 change 误归属（airouter/泳道组件属 ai-provider-model-overhaul），本 change 未改 flow/ 文档 -->

- [x] 6.1 `.agents/skills/harness-retro/SKILL.md`：--lifecycle 用法、报告解读、催修时距分派口径
- [x] 6.2 `.agents/skills/harness-facts/SKILL.md`：词汇表补 pass 两个 reasonCode 与 change.abandon 事件（payload/保留期/消费侧冲突规则）
- [x] 6.3 `docs/reference/harness/pi-extensions.md`：ui-design-gate 放行记账、终态登记脚本说明
- [x] 6.4 `docs/reference/standard/shared/doc-authoring.md`：human-feedback.md 纳入 change 可选制品清单（= 8.5 同一改动）

## 7. 验证

1. `bash scripts/harness/change-abandon.smoke.sh` → 新建 smoke 全绿（登记/拒绝不存在/拒绝已归档/幂等重跑）
2. `bash scripts/harness/harness-retro.smoke.sh` → 既有全量 + 新 lifecycle 组（四段齐备/未归档标注/只读断言/放行双口径/时距分派剔除计数）
3. `bash scripts/harness/harness-retro.sh --lifecycle daily-report-margin-notes`（真库）→ 输出四段与 explore-findings 探索数据一致（成本 $28.80 量级、ui-prototype-missing ×2、19 sessions）
4. `bash scripts/harness/harness-retro.sh --lifecycle <某未归档 active change>` → 「未归档」标注 + 最后活动时间，不假定终态
5. 只读不变更：`--lifecycle` 前后 `sqlite3 .pi/harness/events.db "SELECT COUNT(*) FROM events"` 计数不变
6. pass 词汇稳定：smoke 稳定集含 ui-approval-approved / ui-design-satisfied，全库 reasonCode 枚举无漂移

Scenario→测试映射表（scenario-trace 对账，34 项）：

| Scenario | 测试文件 |
| --- | --- |
| spec-gate 阻断归档被记录 | .pi/extensions/tests/policy-decision.smoke.cjs |
| spec-gate 显式豁免被记录 | .pi/extensions/tests/policy-decision.smoke.cjs |
| quota-gate 阻断与 fail-open 被区分 | .pi/extensions/tests/policy-decision.smoke.cjs |
| test-scope 软硬模式被记录 | .pi/extensions/tests/policy-decision.smoke.cjs |
| dev-process-guard 孤儿治理事件按豁免形状记账 | .pi/extensions/tests/dev-process-guard.smoke.cjs |
| interop 探测短路被记账且不双写 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 归因降级被记账且不双写失败事实 | .pi/extensions/tests/policy-decision.smoke.cjs |
| UI 审批通过与放行被记录 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 正常放行零记录 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 记账故障不改变裁决 | .pi/extensions/tests/policy-decision.smoke.cjs |
| 放弃终态被记录 | scripts/harness/change-abandon.smoke.sh |
| 终态与归档冲突以归档为准 | scripts/harness/harness-retro.smoke.sh |
| 参数非法零记录 | scripts/harness/change-abandon.smoke.sh |
| 体检卡五段齐备 | scripts/harness/harness-retro.smoke.sh |
| 人类反馈段缺席降级 | scripts/harness/harness-retro.smoke.sh |
| 格式异常条目单列披露 | scripts/harness/harness-retro.smoke.sh |
| UI 工作流放行口径标注 | scripts/harness/harness-retro.smoke.sh |
| 未归档 change 不假定终态 | scripts/harness/harness-retro.smoke.sh |
| 只读不变更账本 | scripts/harness/harness-retro.smoke.sh |
| add 建账编号自增 | scripts/harness/feedback.smoke.sh |
| 词表外值零写入 | scripts/harness/feedback.smoke.sh |
| verify 拦截未处置条目 | scripts/harness/feedback.smoke.sh |
| 已归档 change 拒绝建账 | scripts/harness/feedback.smoke.sh |
| rollup 覆盖率显式标注 | scripts/harness/harness-retro.smoke.sh |
| 注入命中率可复算 | scripts/harness/harness-retro.smoke.sh |
| 门禁催修时距可复算 | scripts/harness/harness-retro.smoke.sh |
| 非 gate 裁决解除信号按语义分派 | scripts/harness/harness-retro.smoke.sh |
| 返工波次清单 | scripts/harness/harness-retro.smoke.sh |
| 禁止综合总分 | scripts/harness/harness-retro.smoke.sh |
| 子线程 token 占比可复算 | scripts/harness/harness-retro.smoke.sh |
| 每任务成本含子会话且可复算 | scripts/harness/harness-retro.smoke.sh |
| 模型分桶缺失降级 | scripts/harness/harness-retro.smoke.sh |
| 双锚点单侧缺失对账 | scripts/harness/harness-retro.smoke.sh |
| 不可归因成本披露 | scripts/harness/harness-retro.smoke.sh |

## 8. human-feedback 反馈建账与计量（design D7/D8）

- [x] 8.1 新增 `scripts/harness/feedback.sh` 三件套：`menu` 打印归因词表；`add <change> --layer <归因> --type <产品|流程> "<要点>"` 定位 change 目录（已归档拒绝、文件不存在按模板创建含头部定位说明）、编号取现有最大 FB-N 自增、状态初始化待处理、词表外值非零退出零写入；`verify <change-dir>` 待处理/处理中→处置提醒+退出码 1，全处置/无文件→0。验证：新建 `feedback.smoke.sh` 断言建账编号自增/词表拒绝零写入/verify 三态/已归档拒绝（design D8）
- [x] 8.2 存量补标：`openspec/changes/ai-provider-model-overhaul/human-feedback.md` 10 条补归因标注（frontend×5：FB-3/5/6/7/9，fact-check×3：FB-1/2/8，backend×1：FB-10，process×1：FB-4）+ 状态词归一五值（条目与处置表不一致处以处置记录为准，如 FB-1）。验证：`feedback.sh verify` 对该文件退出码 0（全部已处置）
- [x] 8.3 `harness-retro.sh --lifecycle` 新增第五段「人类反馈段」：解析路径规则同声明元数据（未归档 changes/、已归档 archive/ 剥日期前缀），输出条数（产品/流程）/归因分布/未处置数，文件缺席→「无反馈记录（或未启用）」降级，单条解析失败计「格式异常」单列不炸段；保持只读。验证：harness-retro.smoke 扩组（有文件真数据/无文件降级/格式异常单列/只读断言）
- [x] 8.4 `archive-readiness.sh` 归档对账顺带调用 `feedback.sh verify` 透传结果语义（有未处置反馈条目→对账输出提醒，不阻断 spec-gate 硬门禁）。验证：fixture 含未处置条目的 change 目录跑 archive-readiness 输出提醒行
- [x] 8.5 制品清单制度化：`docs/reference/standard/shared/doc-authoring.md` + `docs/reference/开发执行规范.md` §0.6 把 human-feedback.md 纳入 change 可选制品清单（字段格式契约引用本 change design D7；"收到用户对产物的反馈时记入"入清单说明）。验证：两文档 grep human-feedback 命中且格式描述与 D7 一致
- [x] 8.6 `.agents/skills/harness-retro/SKILL.md`：第五段报告解读（归因分布怎么看、高发层→规范整理去向映射）；`openspec list` 消费方说明（human-feedback.md 为仓库约定制品非 openspec 原生制品，不影响 status）。验证：SKILL 文档 grep 人类反馈段命中
