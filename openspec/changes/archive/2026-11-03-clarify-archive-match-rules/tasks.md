## 1. 规则文案与逐条判定（纯函数层）

- [x] 1.1 `archiveMatchFlow.ts` 抽出规则元数据常量源（编号 / 规则名 / 人话问法 / 短名，顺序 ⓪→⑤），`buildGenericFlowSteps` 改为从常量渲染；**新建** `archiveMatchFlow.test.ts`（现状不存在），验证：`pnpm test:unit app/features/tags/components/console/archiveMatchFlow.test.ts --maxWorkers=2` 覆盖「五条规则编号与问法来自常量」断言通过
- [x] 1.2 `buildTagFlowSteps` 补「供列表消费」的输出（每步：编号、规则名、问法、本次结果文案、状态 `matched|failed|compute|skipped`、是否已走过），并在 `match-detail` 缺失时对命中步给「需实时拉取」；**direct_hit 命中时仍补产 ② compute 参照步**（相似达标 x/N · R · Smax，spec Scenario「标签卡判定过程逐条呈现」的 ② 行要求，现状函数在 ① 后提前 return 不产），composite_hit 命中时不产 ②（组合交集即命中、免相似度计算），验证：单测覆盖 direct_hit 命中（含 ② 参照步）、hit_rate 命中且 ① 未通过、weighted 全落空、composite_hit 命中（仅 ⓪ 步）四条分支
- [x] 1.3 入板过程阶段名改为从同一常量源取（⓪组合命中 / ①精确匹配 / ③命中率 / ④最高相似度 / ⑤综合加权），验证：`grep -n "ruleOf\|RULE_COPY" front/app/features/tags/components/console/ArchiveMatchPopover.vue` 有命中，且组件测试中原阶段文案断言改后仍绿

## 2. 标签卡 popover 重排

- [x] 2.1 头区补「第 N 步 · 规则名 命中」与指标行 `相似达标 x/N · R · Smax`（口径=相似度 ≥ 阈值的标签对数，与 ①「完全相同」分家；数值缺失给 `—`，不显示 `0.00`），验证：`ArchiveMatchPopover.test.ts` 新增断言通过
- [x] 2.2 新增「判定过程」块：只逐条列已试过未通过的步 + 命中步（+② 参照行）；命中步之后合并一排（direct_hit 命中 →「③④⑤ 未走到（①已命中即停）」；composite_hit 命中 →「①②③④⑤ 未走到（⓪已命中即停）」，无 ② 参照行），验证：组件测试断言 direct_hit 命中场景下 ①② 行存在、③④⑤ 未走到行存在且不带各步空壳行；composite_hit 场景 ⓪ 行命中 + ①②③④⑤ 合并排
- [x] 2.3 删掉长判定句与「完整流程见工具行」措辞，改为一行结论（含「本文 N 个辅助标签里 M 个与版块完全相同 → ①精确匹配命中，得分 x.xx」；无数值时写「该步命中数值需实时拉取」），验证：`grep -n "判定：按" front/app/features/tags/components/console/ArchiveMatchPopover.vue` 无命中；组件测试断言结论行存在且含规则名 + 得分
- [x] 2.4 公式从独立折叠块移入命中步下就地展开（按钮文案「这一步怎么算的」），`match-detail` 缺失时不渲染入口，验证：组件测试断言默认收起、点开后 KaTeX 出现在命中步容器内、缺数据场景无公式节点
- [x] 2.5 口径分家：收编标题带「相似 ≥ x.xx，非完全相同」、① 行带「（完全相同）」，口径注释（实时计算 vs 冻结 score）收敛为一处小字，验证：组件测试断言两处口径词同时存在且注释节点只有 1 个
- [x] 2.6 顺序调整为「结论 → 判定过程 → 辅助标签证据 → {类别}描述 → 本文标签 → 参数」，窄屏（<768px）判定过程两行折行不横向滚动，验证：`archiveMatchPopover.css` 内新增窄屏规则；agent-browser 375×667 实页无横向溢出

## 3. 工具弹窗同源与入板模式对齐

- [x] 3.1 `BoardArchiveView.vue` 的「匹配流程」弹窗文案改从 `archiveMatchFlow.ts` 常量源取（规则名/问法/编号与 popover 一致），弹窗仍只讲规则与阈值、不给单次结果，验证：`BoardArchiveView.test.ts` 断言弹窗步骤名与常量源一致
- [x] 3.2 入板过程 popover 只做命名同源替换（结构、五阶段、「未走到」语义不变），验证：`ArchiveMatchPopover.test.ts` 入板模式用例（强证据/各阶段分组/命中即停/弱证据/无标签）全绿

## 4. 测试

- [x] 4.1 按 `test-cases.md` 主链路表逐行执行，人工项结果记入 `ui-verification/report.md`，验证：报告内每行状态为 pass 或有原因标注
- [x] 4.2 组件用例落地：`pnpm test:unit app/features/tags/components/console/ArchiveMatchPopover.test.ts app/features/tags/components/console/BoardArchiveView.test.ts --maxWorkers=2` → 全绿
- [x] 4.3 纯函数用例落地：`pnpm test:unit app/features/tags/components/console/archiveMatchFlow.test.ts --maxWorkers=2` → 全绿
- [x] 4.4 契约变更对账：按 `test-cases.md`「继承与调整」表逐行处置旧 Scenario 与旧测试资产（改写/保留/删除各有留痕），验证：表中无空白处置行

<!-- doc-impact: flow -->
<!-- doc-impact-excuse: database=backend-go/internal/database 等脏文件为他人并行会话改动，本 change 为纯前端呈现改版（front/ + flow 文档），无任何数据库改动 -->

## 5. 文档

- [x] 5.1 `docs/reference/flow/semantic-board.md` 前端入口段落补「标签卡 popover 判定过程（①→⑤ 逐条 + 收编 vs 交集口径）」，验证：`grep -n "判定过程" docs/reference/flow/semantic-board.md` 有命中
- [x] 5.2 `docs/reference/flow/semantic-board.md` 变更溯源表追加本 change 一行，验证：`grep -n "clarify-archive-match-rules" docs/reference/flow/semantic-board.md` 有命中

## 6. 验证

- [x] 6.1 `cd front && pnpm lint` → 退出码 0
- [x] 6.2 `cd front && pnpm exec nuxi typecheck` → 退出码 0
- [x] 6.3 `cd front && pnpm test:unit app/features/tags/components/console/ArchiveMatchPopover.test.ts app/features/tags/components/console/BoardArchiveView.test.ts app/features/tags/components/console/archiveMatchFlow.test.ts --maxWorkers=2` → 全绿
- [x] 6.4 `cd front && pnpm build` → 退出码 0（不与其它 pi 会话/浏览器自动化并行）
- [x] 6.5 `openspec validate clarify-archive-match-rules --strict` → 退出码 0
- [x] 6.6 `bash scripts/harness/scenario-trace.sh openspec/changes/clarify-archive-match-rules && bash scripts/harness/doc-impact.sh verify openspec/changes/clarify-archive-match-rules && bash scripts/harness/check-standards.sh --change clarify-archive-match-rules` → 三条命令退出码均为 0
- [x] 6.7 静态托管冒烟：`bash scripts/dev/deploy-frontend.sh` 后打开 `http://localhost:5100/tags` → 版块 → 归档 → 点标签 chip，判定过程可读、公式就地展开、收编与交集口径各自标注；窄屏 375×667 折行正常（结果记 `ui-verification/report.md`）

### Scenario → 测试文件

| Scenario | 测试文件 |
| --- | --- |
| 解释弱证据 | `front/app/features/tags/components/console/ArchiveMatchPopover.test.ts` |
| 方向校验标记 | `front/app/features/tags/components/console/ArchiveMatchPopover.test.ts` |
| 标签描述展开 | `front/app/features/tags/components/console/ArchiveMatchPopover.test.ts` |
| 事件辅助×版块收编层级 | `front/app/features/tags/components/console/ArchiveMatchPopover.test.ts` |
| 标签卡判定过程逐条呈现 | `front/app/features/tags/components/console/ArchiveMatchPopover.test.ts` |
| 判定过程前置失败步给差距 | `front/app/features/tags/components/console/ArchiveMatchPopover.test.ts` |
| 收编与交集口径分家 | `front/app/features/tags/components/console/ArchiveMatchPopover.test.ts` |
| 判定数值缺失 | `front/app/features/tags/components/console/ArchiveMatchPopover.test.ts` |
| 命中步就地展开公式 | `front/app/features/tags/components/console/ArchiveMatchPopover.test.ts` |
| 强证据文章判定 | `front/app/features/tags/components/console/ArchiveMatchPopover.test.ts` |
| 各阶段得分分组 | `front/app/features/tags/components/console/ArchiveMatchPopover.test.ts` |
| 命中即停的阶段语义 | `front/app/features/tags/components/console/ArchiveMatchPopover.test.ts` |
| 通用匹配流程说明入口 | `front/app/features/tags/components/console/BoardArchiveView.test.ts` |
| 仅弱证据文章判定 | `front/app/features/tags/components/console/ArchiveMatchPopover.test.ts` |
| 无标签 | `front/app/features/tags/components/console/ArchiveMatchPopover.test.ts` |

## 7. Review

- [x] 7.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置并在 `review.md` 建账，验证：`bash scripts/harness/review-trace.sh openspec/changes/clarify-archive-match-rules` 退出码 0
