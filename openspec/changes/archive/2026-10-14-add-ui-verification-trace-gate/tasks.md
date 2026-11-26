# add-ui-verification-trace-gate Tasks

## 1. 脚本层

- [x] 1.1 新增 `scripts/harness/ui-verification-trace.sh`：只读对账 `<change-dir>`——读 `.openspec.yaml` schema 行，非 syntopica-ui 退出码 0（fail-open）；none 档判 ui-design.md `## N/A Reason` 节存在；minor 档判 ui-design.md 验收节映射指名（组件测试→测试文件路径且仓库根相对路径存在；agent-browser/人工→`ui-verification/report.md` ≥1 合法条目）；major 档判目录存在 + `report.md` ≥1 条合法条目（`### A-<N> <场景标题>【pass|fail|未覆盖】`+紧随依据行；未覆盖依据行须含原因）+ 条目/清单点名截图文件真实存在于目录 + 未覆盖带原因；任一失败退出码 1 且中文输出修复路径指引（验证：`bash -n` 语法通过；以 `openspec/changes/add-topic-key-event-markers` 现存报告文本为输入解析出全部合法条目）
- [x] 1.2 新增 `scripts/harness/ui-verification-trace.smoke.sh`：临时目录 fixture 十例——major 实物齐全绿（条目计数+截图对账输出）/ major 零痕迹红 / major 空壳 report.md 红 / major 点名截图缺失红（列缺失清单）/ 未覆盖无原因红 / minor 组件测试路径存在绿 / minor 裸关键词红 / minor agent-browser 建账绿 / none N/A 绿 / 旧 schema 零检查绿（验证：`bash scripts/harness/ui-verification-trace.smoke.sh` 退出码 0）
- [x] 1.3 存量迁移：`git mv openspec/changes/add-topic-key-event-markers/ui-acceptance openspec/changes/add-topic-key-event-markers/ui-verification`；tasks.md 4.4 行与 report.md 文内 `ui-acceptance` 引用同步为 `ui-verification`（验证：`grep -rn 'ui-acceptance' openspec/changes/` 仅允许本 change 迁移说明与 archive/ 历史档案命中——归档即永久家不改写历史，与 6.2 opencli 白名单同口径；迁移后 `bash scripts/harness/ui-verification-trace.sh openspec/changes/add-topic-key-event-markers` 退出码 0——首个实战 change 走通即机制预演）

## 2. 扩展层

- [x] 2.1 `.pi/extensions/lib/ui-design-gate.ts` `missingArchiveUiEvidence` 瘦身：删除 tasks 尾节 `opencli`/`1440`/`1920` 字样匹配与 minor 四关键词正则两段判定；保留 ui-approval marker、原型文件存在、none N/A 节三项规划制品检查；`.pi/extensions/tests/` 邻近 smoke 同步改写（验证：字样齐备无实物的 fixture 从放行变为不再由 lib 判定）
- [x] 2.2 `.pi/extensions/spec-gate.ts` 检查⑥' 接线：`checkUiEvidence` 改为 spawn `bash scripts/harness/ui-verification-trace.sh <changeDir>` 取退出码，失败推入 failures（reasonCode 沿用 `ui-verification-missing`）；spawn 失败沿既有 fail-open 语义记 warning 不阻断；注释与豁免文案「六项检查」枚举同步；`.pi/extensions/tests/spec-gate.smoke.cjs` 补断言——字样齐备无实物 fixture 阻断 / 实物齐全 fixture 放行（验证：`node .pi/extensions/tests/spec-gate.smoke.cjs` 退出码 0）
- [x] 2.3 `scripts/harness/check-tasks-tail.sh` 增第三道防线规则：proposal 头部 `ui-impact: major` 且非旧 schema 时，tasks.md 须含指向 `ui-verification/report.md` 的 `- [ ]`/`- [x]` 任务行，缺则退出码 1 + 修复指引（实现期若误报率高按 design Open Question 降级为归档单防线，不改变 spec 判定）；smoke 补断言（验证：`bash scripts/harness/check-tasks-tail.smoke.sh` 退出码 0）
- [x] 2.4 `scripts/harness/archive-readiness.sh` 聚合新增本脚本为 ⑦ UI 验收痕迹对账项（消除「UI 验收证据不在自查范围」盲区）；`archive-readiness.smoke.sh` 补断言（验证：smoke 退出码 0 且输出可见新项）

## 3. 文档与口径迁移

- [x] 3.1 `docs/reference/开发执行规范.md`：§5.3 major 双层验收段落改写（agent-browser 口径 + `ui-verification/report.md` 建账 + 脚本单一事实源）；前端验收四维度表「证据存在性」行从「归档前人工对账目录实物」改为机器对账；§11.1 检查⑥' 描述同步；全篇 opencli→agent-browser（12 处）
- [x] 3.2 `docs/reference/standard/shared/test-design.md`：主体层选择表/主链路硬规则与 JIT 注入摘要节共 3 处 opencli→agent-browser（速查版头部「改主体必同步本节」义务履行）
- [x] 3.3 `docs/reference/standard/frontend/layout.md` 验收方式引用改 agent-browser 与 report.md 建账落点
- [x] 3.4 悬空 skill 引用清理：开发执行规范中 skill `ui-verify`/`opencli-browser` 引用改为 skill `agent-browser`（存在性验证：`.agents/skills/agent-browser/SKILL.md` 在库）
- [x] 3.5 `docs/reference/harness/pi-extensions.md` 扩展全景表 spec-gate 行检查⑥' 描述同步为脚本单一事实源
- [x] 3.6 openspec 侧口径：`.pi/agent`/`.agents` 下 openspec skill 与 ui-design.md 模板注释中 opencli 字样改 agent-browser（grep 定位逐处改，skill 副本一并）

## 4. Review

- [x] 4.1 派子线程聚焦 review（审查制品 + diff：脚本判定逻辑、spec-gate 接线、正则契约三处高风险面），意见按 High/Medium/Low 处置后 `review.md` 建账（格式契约见开发执行规范「review 执行要点」）

## 5. 测试

- [x] 5.1 新建 `openspec/changes/add-ui-verification-trace-gate/test-cases.md`：主链路表（十 fixture 场景→smoke 断言）+ ⓪ 继承与调整表（`bash scripts/harness/test-assets.sh ui-design-workflow` 反查 spec-gate/lib 旧 smoke 断言逐行处置：字样放行类用例改写为脚本判定断言，不静默删）+ 白盒附加节（脚本判定优先级/正则边界：头部注释区边界、依据行空行穿插、词表外判定）
- [x] 5.2 `bash scripts/harness/ui-verification-trace.smoke.sh` → 退出码 0
- [x] 5.3 `bash scripts/harness/check-tasks-tail.smoke.sh` → 退出码 0
- [x] 5.4 `bash scripts/harness/archive-readiness.smoke.sh` → 退出码 0
- [x] 5.5 `node .pi/extensions/tests/spec-gate.smoke.cjs` → 退出码 0
- [x] 5.6 邻近无回归：`node .pi/extensions/tests/run-harness-smoke.sh` 清单内既有 smoke 全绿（ui-design-gate 邻近扩展零改动验证）

## 6. 文档一致性

- [x] 6.1 编号与口径三处一致（人工：spec-gate.ts 注释 / archive-readiness.sh 输出 / pi-extensions.md 全景表 / 开发执行规范 §11.1）：检查⑥' = 脚本单一事实源，无相互矛盾
- [x] 6.2 `grep -rn 'opencli' docs/ scripts/ .pi/ .agents/ openspec/` → 活文档零残留；白名单（实现期据实修订）：archive/、experience/ 历史档案，docs/research/ 日期快照，.pi/harness/ 运行时产物，存量 change 历史文本（D3 不改写），ui-verification-trace.sh 兼容词表（D3：opencli 按人工同通道），smoke 测试夹具（断言旧字样退役），openspec/specs/ui-design-workflow/spec.md（本 change delta MODIFIED 覆盖，归档时替换，不提前手改主 spec），openspec/specs/test-case-design/spec.md（未声明 capability 不越权改，留待后续触及该域的 change 迁移）
- [x] 6.3 `grep -rn 'ui-acceptance' openspec/ docs/ scripts/` → 仅本 change 迁移说明与 archive/ 历史档案命中（归档不改写历史，同 6.2 白名单口径）

## 7. 验证

以下是实现后门禁，不是本次已执行结果。新增测试路径须在实现期创建；scenario-trace、doc-impact、review-trace、ui-verification-trace 只在实际实现与文档更新后才应全绿。

| Scenario | 测试文件 |
| --- | --- |
| major 档实物齐全通过 | scripts/harness/ui-verification-trace.smoke.sh |
| major 档零痕迹阻断 | scripts/harness/ui-verification-trace.smoke.sh |
| major 档空壳建账阻断 | scripts/harness/ui-verification-trace.smoke.sh |
| major 档点名截图缺失阻断 | scripts/harness/ui-verification-trace.smoke.sh |
| 未覆盖场景无原因阻断 | scripts/harness/ui-verification-trace.smoke.sh |
| minor 档指名道姓通过 | scripts/harness/ui-verification-trace.smoke.sh |
| minor 档裸关键词阻断 | scripts/harness/ui-verification-trace.smoke.sh |
| none 档一致性维持 | scripts/harness/ui-verification-trace.smoke.sh |
| 旧 schema change 不硬阻断 | scripts/harness/ui-verification-trace.smoke.sh |
| 标准条目解析 | scripts/harness/ui-verification-trace.smoke.sh |
| 判定词表外不合法 | scripts/harness/ui-verification-trace.smoke.sh |
| 格式对齐实战形态 | scripts/harness/ui-verification-trace.smoke.sh |
| 门禁与脚本同源判定 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 扩展内旧字样检查退役 | .pi/extensions/tests/spec-gate.smoke.cjs |
| major UI 双层验收通过 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 只有单元测试不得完成 major UI | .pi/extensions/tests/spec-gate.smoke.cjs |
| 字样齐备但无实物不得通过 | .pi/extensions/tests/spec-gate.smoke.cjs |
| minor 指名映射通过 | scripts/harness/ui-verification-trace.smoke.sh |
| minor 裸关键词不再放行 | scripts/harness/ui-verification-trace.smoke.sh |
| 三道防线任务行缺失阻断 | scripts/harness/check-tasks-tail.smoke.sh |
| 自查脚本同口径聚合 | scripts/harness/archive-readiness.smoke.sh |

<!-- doc-impact: standard, harness（业务域白名单零触碰；docs/reference 更新：test-design.md/layout.md 属 standard 域、pi-extensions.md 属 harness 域、开发执行规范.md 属执行规范本体随流程段落更新） -->

- [x] 7.1 `bash scripts/harness/ui-verification-trace.sh openspec/changes/add-ui-verification-trace-gate` → 本 change 为 ui-impact: none 档，退出码 0（N/A 一致性吃自家狗粮）
- [x] 7.2 `openspec validate add-ui-verification-trace-gate` → `is valid`
- [x] 7.3 `bash scripts/harness/archive-readiness.sh openspec/changes/add-topic-key-event-markers` → 新增⑦项绿（存量迁移后按新门禁走通 = 首个实战验证），其余项判定逻辑零改动
- [x] 7.4 既有门禁无回归：`bash scripts/harness/check-tasks-tail.sh openspec/changes/add-topic-key-event-markers` → 退出码 0（其 tasks 4.4 含 ui-verification/report.md 引用，第三道防线规则不误伤存量）
- [x] 7.5 pi 扩展热载生效（人工：改 spec-gate 后新会话跑一次 `openspec archive --dry-run` 类探查或观察 turn 事件无报错，确认 spawn bash 路径在 pi 沙箱内可用）
