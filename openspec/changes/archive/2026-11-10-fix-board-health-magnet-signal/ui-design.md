<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI Design Contract（minor：复用/状态契约）

## 入口与入口变更

入口不变：版块控制台 → 右轨/治理面板体检卡（`BoardHealthWorkspace.vue` 组合层）→ 「体验核查」页签（`BoardHealthIssueIndex.vue` 索引 + `BoardHealthIssueDetail.vue` 详情 + `BoardHealthRepairEditor.vue` 修复入口）。

本 change 不新增页面 / 面板 / 弹窗 / 导航 / 布局模式。三处**信息级**变化，全部落在既有展示位：

| # | 变化 | 展示位 | 性质 |
| --- | --- | --- | --- |
| 1 | 磁铁证据对比口径 | `BoardHealthIssueDetail.vue` 磁铁证据行（原「failed=X vs all=Y」） | 数值语义替换：改为「失败子集 X vs 健康子集 Y」；旧快照无 `good_mean` 时回退旧口径并显式标注「旧口径快照」 |
| 2 | 核心主体徽标 | `BoardHealthIssueIndex.vue` 索引行 + `BoardHealthIssueDetail.vue` 详情头部 | 新增一枚只读徽标「核心主体」，不改变行结构与排序 |
| 3 | 闸门提示条 | `BoardHealthIssueIndex.vue` 顶部 | 新增一行警示条（可执行占比超限才出现）：说明本轮信号可能不可用、不得按此批量操作 |

降级项（闸门把可执行项降为 `insufficient`）**不新增 UI**：既有 `splitTriageItems` 已按 `remove|rewrite` 分流成「待核对」，`insufficient` 落在「其余标签」折叠区、既无摘除入口也无重写入口——本 change 复用该既有契约，不动修复入口的可见性逻辑。

## 受影响状态

| 状态 | 受影响 | 说明 |
| --- | --- | --- |
| 有待核对项（remove/rewrite 占比正常） | 否 | 索引与详情保持现状；磁铁对比行改为新口径文案 |
| 有待核对项且含核心主体 | **是** | 核心主体带徽标；其分诊结论被规则层改判为 keep → 落入「其余标签」区，不再出现在待核对索引 |
| 闸门降级轮（可执行占比超限） | **是** | 顶部出现警示条；全部可执行项落入「其余标签」区（无修复入口）；用户不会被「47 项待核对」误导 |
| 旧快照（无 `good_mean` / 无 `core_subject` / 无 `triage_guard`） | **是** | 磁铁对比回退旧口径 + 标注「旧口径快照」；核心主体徽标不显示；无警示条 |
| 降级轮（`triage=null`） | 否 | 既有空态展示不变 |
| 无快照 | 否 | 既有空态不变 |

## 复用组件与布局模式

- **零新组件、零新布局**：全部复用 `BoardHealthWorkspace.vue` / `BoardHealthIssueIndex.vue` / `BoardHealthIssueDetail.vue` / `boardHealthWorkspace.ts`（纯派生）。
- 布局模式不变：体检面板在既有版块控制台结构内，无 layout mode 变更、无 dialog、无自由宽度；徽标沿用面板内既有 chip/徽章样式，不引入新视觉语言。
- **前端不重算任何口径纪律不变**：`good_mean` / `good_samples` / `core_subject` / `triage_guard` 全部由服务端提供，前端只搬运与展示；占比、阈值、降级原因由后端算好（`triage_guard.ratio` / `.threshold` / `.reason`），前端不自己除法。
- 展示字段来源语义锚：`core_subject` / `good_mean` / `triage_guard` 的来源语义在本 change 的 specs delta「分诊合理性闸门」「标签分诊信号计算」「分诊以问题处理为主线」Requirement 内锚定（⓪ 改契约 → MODIFIED + 展示字段盘点表，见 `test-cases.md`）。

## 验收映射

- **派生层**：`front/app/features/tags/components/console/board-health/boardHealthWorkspace.model.test.ts` 新增用例——闸门降级时索引不含被降级项；旧快照缺 `good_mean` 时回退旧口径文案。
- **归一化层**：`front/app/api/normalizers/boardHealthTriage.ts` 相关测试新增用例——`good_mean` / `good_samples` / `core_subject` / `triage_guard` 缺失或非法时容错为 null / false，不报错、不臆造。
- **组件层**：`front/app/features/tags/components/console/board-health/BoardHealthWorkspace.test.ts` 新增用例——核心主体徽标渲染、`triage_guard` 警示条渲染与不渲染两种情形。
- **真实页面核对**（静态 `:5100`，agent-browser 或人工，落点在 tasks 验证节定）：重跑体检后打开板块「中东地缘政治与美伊关系」体检抽屉——「体验核查」索引首项不再是「伊朗」，全板待核对条数回落到个位/十位数级，警示条按闸门结论出现或消失。
