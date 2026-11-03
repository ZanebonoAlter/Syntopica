<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board -->

## Why

归档「标签卡」popover（`ArchiveMatchPopover.vue` 标签卡模式）把五条判定规则压成了一句流水线长句（「判定：按 ⓪→①→②→③→④→⑤ 顺序判定，本事件 第①步即命中 · 直接命中 0.70（交集 3 个 ≥ 2 → 满足；score=0.70）完整流程与阈值见工具行「匹配流程」」）。用户走查结论：比旧版 `MatchDetailPanel` 的「①精确匹配 / ②算余弦相似度 / ③命中率 / ④最高相似度 / ⑤综合加权」逐条列更难读——规则名与本次结果分居两个弹窗，理解前提变成跳转。

同时「辅助标签 ×5 · 版块收编 5/5」与「①交集 3 个 ≥ 2」并排出现，看着像自相矛盾：收编数的是「相似度 ≥ 0.72 的命中对数」（后端 `hits`），交集数的是「辅助标签 ID 完全相同」（`direct_hit_auxiliaries`），两个口径在一张表里没有区分标签。

## What Changes

- **标签卡 popover 重排**（不改组件、锚定形态、窄屏底部档、开关交互）：
  - 头区：标签名 · 类别 · `第①步 精确匹配 命中` · 得分（规则色），并恢复旧版指标行 `相似达标 x/N · R · Smax`（口径=相似 ≥ 阈值的标签对数，与 ① 交集「完全相同」分家），让分数有参照；
  - 新增「判定过程」块：按 ⓪→⑤ 顺序，**只列出「试过但没过」的步骤 + 命中的那一步**，每行给编号 + 规则名 + 旧版问法原话 + 本次结果（✓命中 / ✗差多少）；命中步之后合并成一排「③④⑤ 未走到（①已命中）」；
  - 公式从悬空折叠块改为挂在命中步下的就地展开（按钮改人话「这一步怎么算的」）；
  - 删掉长判定句与「完整流程见工具行」这一理解前提（入口保留，不再是前提）；
  - 口径注释（实时计算 vs 冻结 score）由两处收敛为一处。
- **两个口径显式分家**：收编表标题标「相似 ≥0.72，非完全相同」；判定过程 ① 行标「交集 N 个（完全相同）」。
- **工具行「匹配流程」弹窗与 popover 共用同源文案**：规则名 / 问法 / 编号抽到 `archiveMatchFlow.ts` 单一常量源；弹窗仍只讲规则与阈值、不给单次判定结果。
- **入板过程 popover**（article 模式）只跟随统一措辞，五阶段结构与「命中即停」语义不变。
- **BREAKING（口径反转，取代用户 2026-10-31 修订）**：现行 `workshop-article-archive` 主 spec 明确要求「标签私有判定行 … 指向通用流程入口」且「MUST NOT 在每个标签 popover 内重复完整流程」「规则代入式 SHALL 藏在展开里，默认收起」。本 change 用 MODIFIED requirement 取代这两条：popover SHALL 就地列出判定过程、公式 SHALL 在命中步下就地展开。反转理由：当日动机是防 popover 变长啰嗦；实测反馈是「一句判定 + 跳转」比三行列表更难读，且长句里混了编号体系解释、结果、括号证明、交叉引用四层信息。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `workshop-article-archive`: 「事件卡 popover（事件维度层级与收编）」中 ⑤ 私有判定行与 ⑥ 规则代入式两条改为就地判定过程 + 就地公式，并新增「两个口径分家」「指标行」「未走到合并」的可观测要求；「入板过程 popover（各阶段得分）」以 REMOVED+ADDED 重建为「入板过程 popover（五阶段得分）」——openspec MODIFIED 语义不允许 scenario 级删除，三个历史错位 Scenario（事件私有判定行 / 计算明细就地展开 / 历史计算缺失如实标注，挂在入板 requirement 下、内容却是标签卡行为）只能随重建移除，其行为锚点并入事件卡 popover 的新 Scenario；入板 requirement 正文语义不变，仅阶段命名同源化。

## Impact

- 前端：`front/app/features/tags/components/console/ArchiveMatchPopover.vue`（标签卡模式重排）、`archiveMatchFlow.ts`（抽 RULE_COPY 常量源 + 供「判定过程」逐条使用）、`archiveMatchPopover.css`、`BoardArchiveView.vue`（匹配流程弹窗引用同源文案）。
- 测试：`ArchiveMatchPopover.test.ts`（「私有判定行」「代入式默认收起」两条用例重写为判定过程各步状态 + 就地公式）、`BoardArchiveView.test.ts`（弹窗文案引用同源后断言同步）。
- 后端 / API / 数据库：零改动（`match-detail` 响应字段与 `hits` / `direct_hit_auxiliaries` 口径不变，只是前端分别标注）。
- 文档：`docs/reference/flow/semantic-board.md`（若前端呈现口径段落需同步「判定过程 / 收编 vs 交集」表述）。
- 无数据迁移、无状态机变更、无旧数据降级需求；部署后用户可见差异仅限该 popover 的呈现。
