# 任务 — add-ui-contract-ladder

> 纯文档 change：不改产品代码，测试节按 `openspec/config.yaml` 的「纯文档 change 豁免代码测试，但必须附 grep 一致性校验」执行。

## 1. 交互契约规范

- [x] 1.1 新建 `docs/reference/standard/frontend/interaction.md`：7 个模式词表（`page`/`panel`/`drawer`/`dialog`/`inline-edit`/`canvas`/`command`），每个模式给「何时该用 / 何时禁止 / 本项目实例」三行；含文档头 `doc-impact-applies` 标签；目标 ≤180 行；验证：`test -f docs/reference/standard/frontend/interaction.md && wc -l docs/reference/standard/frontend/interaction.md` → 期望 ≤180
- [x] 1.2 同文件写入界面文字纪律四条禁令（操作口径并列解释 / 机制科普 / 图例长句 / 教学语）+ 三类允许文字；内容从 `openspec/changes/lane-manual-correction-loop/ui-design.md:134`「界面文字纪律」提炼，保留「不得出现」清单的原始例证；验证：`grep -c '动作标签\|状态结论\|确认与反馈' docs/reference/standard/frontend/interaction.md` → 期望 ≥3
- [x] 1.3 同文件写入交互决策台账形态（`ID`/`决策`/`模式`/`forbidden`/`状态`）+ `pinned` 语义（「不得替换，只能回问」）+ 一条反例；验证：`grep -n 'pinned' docs/reference/standard/frontend/interaction.md` 有命中且含「只能回问」
- [x] 1.4 同文件写入数据 provenance 表形态（`api <path>` / `表.字段` / `派生 <公式>` / `none`，`none` 须 `needs-approval`）+ 原型预算五条 + 画布「状态图 + 操作表」形态；验证：`grep -c 'needs-approval\|fixture.json\|interaction-table' docs/reference/standard/frontend/interaction.md` → 期望 ≥3

## 2. 布局契约接入

- [x] 2.1 `docs/reference/standard/frontend/layout.md` 新增「交互模式选择判据」小节（放在「画布类工作面必须按容器真实像素 1:1 绘制」之前），写明「本节的 1:1 是渲染保真，不是模式选择」并交叉引用 `interaction.md`；验证：`grep -n '交互模式选择判据' docs/reference/standard/frontend/layout.md` 有命中
- [x] 2.2 在 `layout.md`「弹窗必须使用统一尺寸档」节首句补一句边界：「尺寸档解决的是尺寸，不构成选弹窗的理由；是否用弹窗见 `interaction.md` 判据」；验证：`grep -n '不构成选弹窗的理由' docs/reference/standard/frontend/layout.md` 有命中

## 3. 编排纪律

- [x] 3.1 `docs/reference/开发执行规范.md` §3.1 写入 L0–L3 四层制品阶梯表（L0 契约 / L1 骨架 / L2 交互 / L3 数据）与 L1–L3「不得新增 L0 之外元素」铁律；验证：`grep -n 'L0' docs/reference/开发执行规范.md` 有命中
- [x] 3.2 同节写入 R0–R3 评审阶梯（每轮人只裁一个决策）+「R0–R2 禁止业务文案」；验证：`grep -n 'R0–R2 禁止业务文案\|R0-R2 禁止业务文案' docs/reference/开发执行规范.md` 有命中
- [x] 3.3 同节写入「按契约切批，不按模块切批」一条，引 `add-home-pipeline-status-page` 的 FB-2 与产出形态作为反例；验证：`grep -n '按契约切批' docs/reference/开发执行规范.md` 有命中

## 4. spec 基线

- [x] 4.1 `openspec/changes/add-ui-contract-ladder/specs/ui-design-workflow/spec.md` 的 MODIFIED delta 与主 spec 无冲突项；验证：`grep -n 'ui-design 作为必经规划制品\|major UI 原型必须获得用户确认' openspec/specs/ui-design-workflow/spec.md` 两条 Requirement 标题在主 spec 存在（MODIFIED 前置条件）
- [x] 4.2 归档时运行 `openspec archive add-ui-contract-ladder` 把 delta 并入主 spec（由归档动作本身执行）；验证：归档后 `grep -n 'Interaction Decisions' openspec/specs/ui-design-workflow/spec.md` 有命中、`openspec/specs/frontend-interaction-contract/spec.md` 存在

## 5. 并行轨道留痕（非本 change 范围，不参与归档判定）

- [x] 5.1 存量瘦身（备份 / 63 份 md 合并 / 删除零引用件）的完整执行记录见 `add-home-pipeline-status-page/human-feedback.md` 的 **FB-3**，任务本体归 `add-home-pipeline-status-page`；本 change 只留此指针。结果：176MB → 42MB、796 → 407 文件、md 10,608 → 375 行；备份 `~/backups/ui-prototype-add-home-20261115{,-incremental}.tar.gz`；验证：`grep -c 'FB-3' openspec/changes/add-home-pipeline-status-page/human-feedback.md` → 1

## 6. 审查

- [x] 6.1 派 `change-review` 档子线程审查本 change 的 openspec 制品与四份规范文本，产出 `review.md`（实得 11 条：1 High / 6 Medium / 4 Low）；验证：`bash scripts/harness/review-trace.sh openspec/changes/add-ui-contract-ladder` 通过
- [x] 6.2 处置 review 意见：11 条全部处置为「已修复」（1 High / 6 Medium / 4 Low）：R-1 同步 schema + 模板 + §3.1 表；R-2 画布类审批/验收降级口径；R-3 补三条判据的计数口径与 `api <path>` 可核要求；R-4 画布类 md 上限改 ≤2 且决策表只落 `ui-design.md`；R-5 pinned 四种禁用偏离 + 唯一合法变更路径 + 回问最小形式；R-6 四条任务自带验证命令修正（2.1/8.1/8.4/9.1/9.5）；R-7 登记 `standard/README.md` 与 `constraints-index.md`；R-8 存量瘦身改为指针；R-9 `interaction.md` 两处 MUST 补 none 豁免 / minor 两节；R-10 与 `interaction-conventions.md` 划界；R-11 `doc-impact-applies` 补 `components/common/` 与 `composables/`；验证：`bash scripts/harness/review-trace.sh openspec/changes/add-ui-contract-ladder` 零 High 待处理

## 7. 测试

- [x] 7.1 文档一致性 grep 校验（纯文档 change 替代测试）：6 处引用均为相对路径且目标文件存在（layout.md ×4、开发执行规范.md ×1、constraints-index.md ×1），无死链
- [x] 7.2 词表与既有浮层形态一致性：`interaction.md` 末节「与 layout.md 的边界」已显式区分 layout mode（`reader`/`contained`/`workspace`/`split`，管宽度与栏数）与本文件的 7 个交互模式（管操作形态）；与 `interaction-conventions.md` 的分工已在两文件首部互链写明
- [x] 7.3 无新增死链：`bash scripts/harness/check-standards.sh` → 通过 298 / 失败 0

## 8. 文档

<!-- doc-impact: standard -->

- [x] 8.1 新建 `docs/reference/standard/frontend/interaction.md` 并在 `docs/reference/standard/README.md`「这层装什么」表 + 目录树、`docs/reference/constraints-index.md` 执行规范表三处登记（doc-authoring checklist 第 3/4 步，r-7）
- [x] 8.2 更新 `docs/reference/standard/frontend/layout.md`（见 2.1 / 2.2）
- [x] 8.3 更新 `docs/reference/开发执行规范.md` §3.1（见 3.1–3.3）
- [x] 8.4 更新 `docs/reference/constraints-index.md` 执行规范表（新增 `standard/frontend/interaction.md` 一行）；验证：`grep -c 'frontend/interaction.md' docs/reference/constraints-index.md` → 1
- [x] 8.6 同步 `openspec/schemas/syntopica-ui/schema.yaml`（major 段八节 → 八节 + 三节；Prototype 按主模式分流；审批流/验收的画布措辞）与 `openspec/schemas/syntopica-ui/templates/ui-design.md`（补三节骨架 + Prototype 变体）；验证：`grep -c 'Interaction Decisions' openspec/schemas/syntopica-ui/schema.yaml openspec/schemas/syntopica-ui/templates/ui-design.md` → 两者均 ≥1
- [x] 8.7 同步 `docs/reference/开发执行规范.md` §3.1 表（八节合同 → 八节 + 三节结构化声明）与 §5.3 ③（画布类差异说明改为状态图逐态对账）；验证：`grep -c '三节结构化声明' docs/reference/开发执行规范.md` → ≥1
- [x] 8.5 运行 `bash scripts/harness/doc-impact.sh verify openspec/changes/add-ui-contract-ladder` → 通过（声明: standard，5 个文件）与 `bash scripts/harness/check-standards.sh` → 298/0

## 9. 验证

- [x] 9.1 制品完整性：`ls openspec/changes/add-ui-contract-ladder/{proposal.md,design.md,ui-design.md,tasks.md,review.md}` 与两份 delta spec 均存在 → 通过（共 7 个）
- [x] 9.2 openspec 校验：`openspec validate add-ui-contract-ladder` → `Change 'add-ui-contract-ladder' is valid`
- [x] 9.3 门禁对账：`bash scripts/harness/archive-readiness.sh add-ui-contract-ladder` → 六项全绿
- [x] 9.4 规范可读性：`interaction.md` 145 行（≤180），以表为主
- [x] 9.5 反向检查（防误伤）：本 change 改动集仅限 `docs/reference/standard/frontend/{interaction,layout}.md`、`docs/reference/standard/README.md`、`docs/reference/constraints-index.md`、`docs/reference/开发执行规范.md`、`openspec/schemas/syntopica-ui/` 与 change 目录；`front/` 与 `backend-go/` 零 diff（工作树上的 26 个前端脏文件属其他 change）
> **归档后回检锚点（观察项，非本 change 的完成任务）**：本 change 归档后，下一个 `ui-impact: major` 的 change 应能一次产出「决策台账 + provenance + 明确不做」三节；若仍出现 `node-spec.md` 散装文档或多个 `revision-*` 目录，则判定纯文档规范无效，必须转 D6 的脚本化改造（stage 标记 + 按路径触发 + 路径域限制，见后续 change）。回检方式：`bash scripts/harness/harness-retro.sh --lifecycle <该 change 名>`。

### Scenario → 落地对账

> 纯文档 change：本表将 delta spec 的 18 个 Scenario 映射到「承载它的文档」或人工对读方式（无自动化测试，按 `test-design.md`「无自动化→人工…留痕」）。

| Scenario | 测试文件 |
| --- | --- |
| 并列比较不得塞进弹窗 | docs/reference/standard/frontend/interaction.md |
| 单字段修改不得弹窗 | docs/reference/standard/frontend/layout.md |
| 自创模式名被拒 | docs/reference/standard/frontend/interaction.md |
| 节点卡不得承载教学语 | docs/reference/standard/frontend/interaction.md |
| 机制解释压缩为结论徽标 | 人工：对读 interaction.md「界面文字只承担三类职责」表第 2 行，确认「派生值机制科普」与「结论徽标」对应关系存在 |
| 按模块分批不得乘出散装文档 | 人工：对读 interaction.md 原型预算表首行与开发执行规范 §3.1.3 反例段，确认两处口径一致 |
| 版本快照堆积被拒 | docs/reference/standard/frontend/interaction.md |
| 画布交互用弹窗模拟被拒 | docs/reference/standard/frontend/interaction.md |
| 状态图缺已确认态 | 人工：对读 interaction.md「画布类交互的原型形态」节，确认「至少三张」句列举了三态 |
| none 产生最小制品 | docs/reference/开发执行规范.md |
| minor 复用既有交互 | openspec/changes/add-ui-contract-ladder/specs/ui-design-workflow/spec.md |
| major 形成完整 UI 契约 | openspec/changes/add-ui-contract-ladder/specs/ui-design-workflow/spec.md |
| 决策台账缺模式词表取值 | docs/reference/standard/frontend/interaction.md |
| pinned 决策被实现阶段改写 | docs/reference/standard/frontend/interaction.md |
| 原型待确认时暂停规划 | docs/reference/开发执行规范.md |
| 用户明确确认后放行 | docs/reference/开发执行规范.md |
| 重大修订使审批失效 | openspec/changes/add-ui-contract-ladder/specs/ui-design-workflow/spec.md |
| 画布类界面强行用可点 HTML 模拟 | openspec/changes/add-ui-contract-ladder/specs/ui-design-workflow/spec.md |
