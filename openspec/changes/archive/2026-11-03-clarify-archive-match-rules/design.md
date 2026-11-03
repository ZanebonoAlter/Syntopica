## Context

归档「标签卡」popover 现由 `ArchiveMatchPopover.vue`（tag 模式）渲染，规则相关数据全部来自两处既有来源：

- `front/app/features/tags/components/console/archiveMatchFlow.ts`：`buildTagFlowSteps()` 已能产出「①精确匹配命中 / ①交集 1<2 不够 → ②算相似度 R=… / ③命中不够 …」这种逐条结果，但组件目前只取其中 `state === 'matched'` 的那一条拼进一句判定行；`buildGenericFlowSteps()` 是工具行弹窗用的通用规则说明。
- `GET /api/semantic-boards/:id/match-detail/:tagId`：`hits`（相似度 ≥ 阈值的命中对数）与 `direct_hit_auxiliaries`（ID 完全相同的交集）是两个不同口径的字段，现状在收编表与判定行里并排出现却不标口径。

约束：popover 的锚定形态、`Teleport` 层级、窄屏底部档、请求竞态保护均为既有契约，本次不动；后端与 API 零改动（见 proposal.md - What Changes）。

## Goals / Non-Goals

**Goals:**

- 判定过程在 popover 内自带解释力：规则名 / 问法 / 本次数值同屏，无需先跳「匹配流程」弹窗。
- 两个口径（收编 = 相似 ≥ 阈值；交集 = ID 完全相同）在文字上不可混淆。
- 规则文案单一来源：popover 判定过程与工具行弹窗用同一份规则名、问法、编号。

**Non-Goals:**

- 不改匹配算法、阈值、`match-detail` 响应字段与后端口径。
- 不改入板过程 popover 的五阶段结构（只跟随命名同源）。
- 不重开文章级证据抽屉，不新增页面/面板/弹窗。

## Decisions

### D1. 判定过程复用 `buildTagFlowSteps`，组件只做展示

- **结论**：把「逐条判定」作为 `archiveMatchFlow.ts` 的纯函数输出（补一个供列表用的返回：每步含编号、规则名、问法、本次结果文案、状态与是否已走过），`ArchiveMatchPopover.vue` 只负责渲染与分组，不在组件里重算交集 / 命中率判断。
- **落点**：front/app/features/tags/components/console/archiveMatchFlow.ts:buildTagFlowSteps
- **取舍**：在组件内按 `detail` 重新推一遍判定条件——与工具弹窗和将来任何消费方会分叉出两套口径，现有「只取 matched 一条」的写法本就是这种分叉的温床，不选。

### D2. 未走到的步骤合并成一排（A2），只展开「已试过 + 命中」的步

- **结论**：判定过程只逐条渲染已试过但未通过的步、命中的那一步，以及 ② 中间计算参照行（凡判定用到相似度数据的场景都给——含 direct_hit 命中时供指标行参照，现状函数在 ① 后提前 return 需补；composite_hit 命中不产 ②，组合交集即命中、免相似度计算）；命中步之后合并成一行（direct_hit →「③④⑤ 未走到（①已命中即停）」；composite_hit →「①②③④⑤ 未走到（⓪已命中即停）」），弱化样式。
- **落点**：Scenario「标签卡判定过程逐条呈现」
- **取舍**：五步全列（含空壳「未走到」行）规则全貌更完整，但多 3 行无信息量条目把 popover 拉长；用户 2026-11-02 明确选 A2。

### D3. 公式从悬空折叠块改为命中步就地展开

- **结论**：`primaryFormula` / `substitutionFormula` / `rateFormula` 的渲染挂到命中步那一行下面，默认收起，按钮文案「这一步怎么算的」；`match-detail` 缺失时不渲染公式入口。
- **落点**：front/app/features/tags/components/console/ArchiveMatchPopover.vue
- **取舍**：保留独立「规则代入式」折叠块、只在标题里注明对应规则——读者仍需自己把公式和规则编号对上，且与现行 spec「独立块 + 默认收起」正是用户抱怨的形态，不选。

### D4. 收编与交集两个口径显式分家

- **结论**：收编区标题写「相似 ≥ 0.72，非完全相同」（阈值取真实配置值），判定过程 ① 行写「交集 N 个（完全相同）」；两处数字各自带口径词，文案不得复用「命中」一个词同时指代两者。
- **落点**：Scenario「收编与交集口径分家」
- **取舍**：把两个数字合成一个「命中 N」——会丢掉后端已经分别给出的两个真实字段语义，且 5/5 与 3 并排的误读无法消除，不选。

### D5. 规则文案抽单一常量源，popover 与工具弹窗共用

- **结论**：在 `archiveMatchFlow.ts` 导出规则元数据常量（编号 / 规则名 / 人话问法 / 短名），`buildGenericFlowSteps`、`buildTagFlowSteps`、入板过程阶段名与 popover 判定过程全部从这里取；顺序为 ⓪组合命中 → ①精确匹配 → ②算余弦相似度（中间计算步）→ ③命中率 → ④最高相似度 → ⑤综合加权。
- **落点**：front/app/features/tags/components/console/archiveMatchFlow.ts:buildGenericFlowSteps
- **取舍**：三处各自维护文案——现状已出现「①→⑤ 无 ⓪」与「⓪→⑤」的漂移，以及工具弹窗叫「直接命中」、列表 chip 叫「精确」的命名不一致，不选。

## Risks / Trade-offs

- [REMOVED+ADDED 重建「入板过程 popover」requirement（改名「各阶段得分」→「五阶段得分」）] → openspec 1.13.2 的 MODIFIED 语义要求同名 Scenario 全保留，scenario 级删除只能走整条重建；正文语义不变，三个错位 Scenario 锚点并入事件卡 popover 新 Scenario；归档后该 requirement 位置移至主 spec 末尾（ADDED 追加），无行为影响。
- [反转 2026-10-31 用户口径，未来复核可能按旧 spec 判违规] → 在 `workshop-article-archive` MODIFIED requirement 内文写明「2026-11-02 修订：取代该日口径」并指向本 change，归档后主 spec 即为新口径。
- [popover 变长，遮住更多归档列表] → 用 A2 合并未走到步骤；命中步公式默认收起；窄屏走既有底部全宽档滚动，不新增高度上限。
- [`match-detail` 不可得时判定过程退化成「只有阈值没有数字」，读起来像没算] → 命中步结果显式写「需实时拉取」，并在头区沿用 `missingParts` 的「未取到：…（如实标注，不伪造）」，不渲染公式。
- [测试断言整句文案易碎] → 断言用 `data-testid` + 关键片段（规则名、口径词、状态词），不断言完整长句。
- [工具弹窗与 popover 共用一个常量后，弹窗文案改动会连带红掉 popover 测试] → 这是期望行为（同源），断言取常量值而非硬编码字符串。

## Migration Plan

纯前端呈现改版：无数据库迁移、无 API 变更、无旧数据降级。部署走既有静态托管流程（前端构建 → `bash scripts/dev/deploy-frontend.sh`），用户可见差异仅限标签卡 popover 的信息结构与文案；回滚 = revert 本次前端改动并重新部署。
