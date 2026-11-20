# Review 建账（add-other-topics-to-context-rail）

> 审查方式：`change-review` 子线程（只读：read / git diff / grep / `openspec validate`），审查范围 = 本 change 制品（proposal / ui-design / design / tasks / specs delta）+ 本 change 代码 diff。
> 工作树内其他并发会话的脏文件（`utils/htmlSanitize.ts`、`features/articles/**`、`ObservationMatrix*` 等）已由审查方逐类核对并排除，未计入本 change 评审。

### R-1 【Medium】工作面 context 页签在 ≥1440px 下子块可见，但点行 / 重试无响应（次路径断链）

- **状态**：已修复
- **事实**：`BoardWideWorkface` 的 `BoardContextRail variant="full"` 未绑定 `@select-other` / `@retry-now`；而 `.bcr-other` 的显示门按**视口**（`min-width: 1440px`）而非 variant 判定，且工作面并非只在 <1200px 出现（≥1200px 右轨的体检详情 / 候选 / 来源入口同样打开它，页签可切到 context）。结果：同块内泳道行 `locate-lane` 可点、其他话题行与重试按钮是死键，违反 delta spec「点该层任一行 SHALL 选中该分区」。
- **修复**：`BoardWideWorkface.vue` 补 `retryNow` / `selectOther` 两个 emit 并转发（:`**`），`BoardConsoleShell.vue` 在 `<BoardWideWorkface>` 上绑 `@select-other="handleSelectOther"` 与 `@retry-now="handleRetryNow"`（与 `@locate-lane` 对齐）；`BoardWideWorkface.test.ts` 补一条机械锚测试（点行 → 外抛 `selectOther` 全量 payload；点重试 → 外抛 `retryNow` 日期）。
- **同时**：`design.md` D3 原写「工作面本身只在 <1200px 出现，实际不构成重复」是与现实相反的事实断言，已改写为「工作面在 ≥1440px 也会渲染该子块，故必须接线」。

### R-2 【Medium】主 spec 基线处于被回退的旧版本，delta 的 Reason/Migration 与实际基线不符

- **状态**：已修复（表述与审计链修正；基线回退本身由归档净效果一并正畸）
- **事实**：`openspec/specs/workshop-board-console/spec.md` 的「治理右轨与治理抽屉」现为**四块旧基线**——`console-rails-live-signals` 于同日 13:29（`5284af47`）同步进去的五块两层增量，被其后 `f908cc53`（15:03 归档 `simplify-workshop-matrix-copy`，基于旧基线写 MODIFIED）覆盖。delta 的 REMOVED 按条款名整条命中，与文本无关，因此归档净结果仍正确（ADDED 新条款以五块三层为基底，等于顺手把回退正畸回来），但原 Reason 描述的「原条款」与实际被删文本不一致，审计链会失真。
- **修复**：delta 的 REMOVED 段补「**基线说明（归档时必须知情）**」，写明回退来源提交、ADDED 的撰写基底、归档净效果（四块旧基线 → 五块三层 + 其他话题层，净 diff 大于单 change 增量属预期）；`stage.md` 的并发注意段同步改写为已核实的版本；`tasks.md` 验证节末项改为可执行、与事实一致的判据。

### R-3 【Low】`openspec validate --strict` 报 ADDED 条款正文 >500 字符（WARNING）

- **状态**：已驳回（沿袭现有先例）
- **理由**：非 strict 校验通过（`openspec archive` 默认校验可过）；主 spec 既有同类条款（含被本 change 替换的原条款）本就同长度。为满足 500 字符上限而拆分条款会把「右轨顺序 + 当日日报 + 其他话题层 + 治理内容」这条同源契约切碎，损失可读性。

### R-4 【Low】`docs/reference/flow/semantic-board.md` 溯源行提前写入，链接暂时指向不存在的 archive 目录

- **状态**：已采纳（归档时即生效）
- **处置**：保留该行；归档本 change 时目录即落到 `openspec/changes/archive/2026-11-20-add-other-topics-to-context-rail`，链接随之成立。归档后须确认未重复追加同一行。

### R-5 【Low】`BoardWideWorkface.test.ts` fixture 里空态文案写死中文字符串而非引用常量

- **状态**：误报丢弃
- **理由**：该处是**测试期望值**（stub 渲染的固定文案），不是生产代码里的文案复制；生产侧四态文案本身全部直出 `OtherTopicsView.statusText`（源自 `dayOtherTopics.ts` 常量）。测试期望值写死恰是对「文案不得被静默改写」的护栏。

### R-6 【Low】tasks.md 复选框未随实现同步

- **状态**：已修复
- **处置**：T1–T12 按实际完成状态勾选；`review.md`（本文件）即 T10 的产出。

- 审毕无发现（审查范围：单一实现点 / 两套量纲分家 / 视口门槛 / 选行链路 / 状态文案复用 / 顶层顺序 / 测试质量 —— 详见下方逐项）

## 审查方逐项核查结论（A–I）

| 项 | 结论 | 证据摘要 |
| --- | --- | --- |
| A 单一实现点 | ✅ | 运行时 `buildOtherTopics` 唯一调用点 = `otherTopicSelection.ts:102`；`RailOtherTopicsBrief.vue` 只消费 `props.view`，无第二套排除口径/自算计数 |
| B 两套量纲分家 | ✅ | 泳道层「N 条」vs 子块「N 条线索」；子块自带 `.rob-head` 标题 + `.rob` 上分隔线，无并排同列 |
| C 视口门槛 | ✅（含 R-1 修复） | `.bcr-other{display:none}` + `@media(min-width:1440px){display:block}`；实测三档 `display:none` 且占高 0 |
| D 选行定位链路 | ✅ | 三层 payload 类型一致；`now.date` 经 `nowBrief` 日期闸门 + `selectOther` 强校验双重兜底，不会误选邻日；实测 A-4 通过 |
| E 状态与文案复用 | ✅ | 四态 + degraded 文案全部直出 `view.statusText/note`；`countText` 仅 ready 给数字；`no-report` 正确不给重试 |
| F 顶层顺序 | ✅ | 五 section 顺序未动，既有 DOM 顺序断言原样保留（diff 纯新增） |
| G 测试质量 | ✅ | 9 条新组件用例 + RS-9 接缝断言；视口门槛为 `readFileSync` 源码正则机械锚，非 class 存在空壳 |
| H 文档一致性 | ✅（R-4 待归档生效） | flow 三处一致、旧禁令句无残留；layout.md 按「同单元格标注 + 表下说明」增补 |
| I 并发风险 | ✅（R-2 已处置） | 归档净结果正确；审计链表述已修正 |
