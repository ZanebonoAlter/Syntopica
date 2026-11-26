<!-- stage: implement -->

# Tasks

## 1. 审计（先行，决定做不做）

- [x] 1.1 派 explore 审计 `workshop-board-console`（1120 行 / 33 req）→ 有效 26 / 过期 6 / 缺写 8
- [x] 1.2 派 explore 审计 `lane-correction-surface`（728 行 / 19 req）→ 有效 15 / 过期 4 / 缺写 5
- [x] 1.3 复核两条最严重的（不自抄审计）：`MEMBER_BASE_RADIUS=7.5` vs spec 5.5；`@media(max-width:1279.98px)` 240/280 vs spec 只写 380/300 —— **两条都成立**
- [x] 1.4 判定：先修过期、后拆巨型（理由见 design D1）

## 2. 修正（9 条 MODIFIED）

- [x] 2.1 `workshop-board-console` / 关键事件视图切换与行集合：两视图 → **三视图**（matrix/archive/events；上一轮并入归档时未同步）
- [x] 2.2 `workshop-board-console` / 矩阵视图：图例「颜色=行内相对强度」→ 实际三态文案（`ObservationMatrix.vue:132`）
- [x] 2.3 `workshop-board-console` / 关键事件节点与名称：`24px 固定步长` → `min(24, 可用宽/(节点数-1))` 钳制（`KeyEventTimelineView.vue:689-691`）
- [x] 2.4 `workshop-board-console` / 治理右轨：「治理抽屉」→「单级宽工作面」，入口名「上下文」
- [x] 2.5 `workshop-board-console` / 归档排序：删掉不存在的「id 倒序」档
- [x] 2.6 `workshop-board-console` / 归档视图入口：时间导航块在工具行**上方**
- [x] 2.7 `lane-correction-surface` / 泳道修正工作面：三栏补窄档断点（`≤1279.98px` → 240/280）；左栏「四块」→「五块」（4 处 + 1 条验收条款）；顺带在验收条款补「MUST NOT 出现两侧同名的块」（review R-13，代码成立但原文无此约束）
- [x] 2.8 `lane-correction-surface` / 节点不得重叠：占高公式补 `LABEL_BOX_PAD`，`44px` → `52px`，半径 5.5 → 7.5（**这条最危险：44 < 实际 48，照 spec 改会复现 spec 自己警告的缺陷**）
- [x] 2.9 `lane-correction-surface` / 首义来源：不是「三态枚举」，是 `source` + `anchor_origin.resolved` 两字段
- [x] 2.10 `lane-correction-surface` 的 Purpose（原为 `TBD - created by archiving change ...`）

## 3. 不做（已判定，登记去处）

- [x] 3.1 **不拆分**两个巨型：拆界已由审计给出（关键事件 10 条 / 归档 8 条 / 画布几何 vs 工作面结构），等本 change 归档后另开，那时守恒验证才干净
- [x] 3.2 不修 7 条跨族重复条款（`高度贴合容器` / `横向吸顶标签列` / `缩放滚动外壳` / `窄屏门限` / `批内相对归一` 与 `workshop-lane-compose` 逐字重复；`首义读端点零写副作用` 与 `lane-ops` 重复）→ 跨族引用清扫批次
- [x] 3.3 不补 13 条缺写（`?view=` URL 键、行头就地改名、修正侧 365 日曲线、`↩加回` 第三写入口、质心池含更早成员、块内生效徽标等）→ 另开 change（补缺写是增量契约，与「让 spec 追上代码」不同件事）
- [x] 3.4 不删死代码 `BoardLaneEvolution.vue:215-217`（带「最密集的三天」文案，与矩阵视图的 MUST NOT 相悖）——代码删除单独 change
- [x] 3.5 **E-5 / E-6 / E-7 三处未修，登记去处**（review R-7）：
  - E-5 `新增泳道的时间视窗曲线口径` 整条讲的是**新建泳道**（属 `workshop-lane-compose`），修正模式自己的 365 日曲线（`useLaneCorrection.ts:735-780`）spec 一字未写 → 并入「补缺写」change（3.3）
  - E-6 代码里两处越界解说句（`LaneAnchorScope.vue:199`、`BoardComposeOverlay.vue:399`）与「产品界面不承载解说性长句」条款相悖 → 代码侧清理，单独登记（与 3.4 死代码同类）
  - E-7 轻量弹窗表漏「版块归档」（md + 手打名，无 `useConfirm`），与「危险操作 SHALL 用 useConfirm」自相矛盾 → **本 change 不判**：要么改条款（承认手打名为等价确认），要么改代码。归「跨族引用清扫」一并裁定
- [x] 3.6 不动「无法核实」项（`workshop-board-console` 1 条形态描述、`lane-correction-surface` R10 像素高度）——无证据不改，避免用猜测替换猜测

## 4. 验证

- [x] 4.1 `openspec validate fix-stale-canvas-specs` → valid
- [x] 4.2 重跑两条最严重的证据命令，确认新写数值与代码一致
- [x] 4.3 `grep` 代码枚举，确认 spec 列举 = 代码列举（三视图、五块、两排序档）
- [x] 4.4 `git diff --stat` 只触及 2 个 spec + 本 change 目录
- [x] 4.5 两个 spec 的 Requirement 总数不变（33 / 19）

## 5. review

- [x] 5.1 派 review（`minimax-cn/MiniMax-M3.1-Flash:max`；GLM 额度 20:20 后才恢复）
- [x] 5.2 处置 review 发现

## 6. 归档

- [x] 6.1 `archive-readiness.sh fix-stale-canvas-specs` 六项全绿
- [x] 6.2 `openspec archive fix-stale-canvas-specs --yes`
- [x] 6.3 归档后确认两个 spec 仍是 33 / 19 条 Requirement、marker 未丢
- [x] 6.4 `openspec validate --specs` → 146 全过

## 7. 文档

- [x] 7.1 `docs/research/process-arch-rework/issues.md` 回填 ISS-04（拆巨型前的过期清理已完成；拆分另开）
- [x] 7.2 记一条经验进 ledger：**spec 里的推导数值（`40+4=44`）会随常量漂移，应写约束 + 当前取值 + 唯一出处**

## 8. 测试

零代码改动，验证即「条款与代码对齐」：

| 验证点 | 命令 | 期望 |
| --- | --- | --- |
| delta 结构 | `openspec validate fix-stale-canvas-specs` | valid |
| 数值一致 | 重跑 `grep MEMBER_BASE_RADIUS` 等 | 7.5 / 240 / 280 / 52 |
| 枚举一致 | `grep "ConsoleView ="` | matrix \| archive \| events |
| 条数不变 | 归档后 `grep -c '^### Requirement:'` | 33 / 19 |
| 未外溢 | `git diff --stat` | 仅 2 spec + change 目录 |

## 9. 文档

<!-- doc-impact: research -->

- `docs/research/process-arch-rework/issues.md`：ISS-04 回填 + 经验一条（7.1 / 7.2）

## 10. 验收

- [x] A 9 条过期条款全部与代码一致（每条有 `file:line` 证据）
- [x] B 未引入新条款（只修不增），条数守恒
- [x] C 拆分 / 缺写 / 重复 / 死代码四项已明确登记为「不做」，各有去处
