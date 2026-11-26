# 设计 — UI 契约阶梯

## 1. 问题定义

三处缺口共用一个根因：**`ui-design.md` 是散文契约，散文没有上限、不要求数据血缘、不约束交互模式**。于是 agent 的每一次「做得更完整」都在合法地增加体积，而正确性（数据有没有来源、交互是不是用户要的那个模式）没有任何一处被要求声明。

因此本 change 不引入新工具、不增加门禁，只做一件事：**把散文契约换成有结构的契约，并给结构设上限**。

## 2. 决策

### D1 — 交互模式走固定词表，不开放自由描述

**决定**：`interaction.md` 定义 7 个模式枚举：`page` / `panel` / `drawer` / `dialog` / `inline-edit` / `canvas` / `command`，每个模式必须写清「何时该用 / 何时禁止 / 本项目实例」。新增模式需要改这份规范，不许在 change 里临时发明。

**理由**：agent 的模式选择受**可用词汇 + 规训强度**支配。`layout.md` 里 dialog 有 4 档尺寸 + 3 个 Scenario，canvas 只有渲染保真条款，于是 dialog 成为「有据可依」的默认。补上「何时该用 canvas」比写「请优先考虑画布」有效——后者是偏好，前者是判据。

**备选（否决）**：在 `layout.md` 里加一句「优先使用直接操作」。否决理由：偏好句无法被 review 判定，也无法解释为什么弹窗不行。

### D2 — 界面文字只承担三类职责

**决定**：产品界面文字只允许「动作标签 / 状态结论 / 必要确认与反馈」；禁止四类：操作口径的并列解释、派生值机制科普、画布图例长句、节点卡里的教学语。

**理由**：这条判据已经在 `lane-manual-correction-loop/ui-design.md`（352 行）里被写出来并生效过——「不得出现 / 例子 / 由什么承担」三列表。问题是它埋在单个 change 里不可复用，每个新 change 都要重新打一遍这场仗。本 change 把它提为项目级规范。

**不采纳「禁止界面出现任何说明文字」**：空态、错误态、破坏性确认必须有文字，否则用户无从判断。

### D3 — 交互决策台账带 pinned / forbidden 语义

**决定**：`ui-design.md` 新增 `## Interaction Decisions` 节，每条决策一行：

| ID | 决策 | 模式 | forbidden | 状态 |
|---|---|---|---|---|
| D1 | 泳道内容的增删改入口 | `canvas` | `dialog`, `drawer` | `pinned` |

- `pinned` = 用户已定的方向，**实现与原型不得替换，只能回问**；
- `forbidden` = 显式排除的模式，原型里出现即视为偏离（本次由 review 人工核，脚本化见 D6）。

**理由**：用户的交互决策现在以散文散落在对话与 `ui-design.md` 里，没有否决语义。`lane-manual-correction-loop` 需要专门写一句「改名的唯一路径是就地编辑…SHALL NOT 弹对话框」才能表达否决——这说明散文形态表达不了「不许」，必须结构化。

### D4 — 数据 provenance 成为必填节

**决定**：`ui-design.md` 新增 `## Data Provenance` 表：每个可见数据项 → `source`（`api <path>` / `表.字段` / `派生 <公式>` / `none`）。`source: none` 必须标 `needs-approval` 并在对话里取得用户确认。

**理由**：没有血缘字段，agent 造出的「丰富」在实现阶段才发现无数据支撑——这是 `add-home` 的 163 处 `fake` 的成因。provenance 同时把「这个功能有没有数据支撑」提前到设计阶段暴露，早死早超生。

**不采纳「要求填真实 API 响应样例」**：会把 change 变成接口勘察，成本过高；`api <path>` 指到端点即可，字段校验留给实现阶段。

### D5 — 原型受预算约束，且画布类不强制可点 HTML

**决定**：
- `ui-prototype/` 目录：设计说明类 md **≤1 份**（即决策表），≤300 行；禁止 `node-spec.md` / `parts-table.md` / 散装 `*-verification.md` 长期堆积；
- 原型界面可见文字：每屏 ≤12 个短语，禁止 >40 字的完整句子；
- 原型数据 MUST 来自 `fixture.json`，禁止在 HTML 里内联 >50KB 的 `<script>` 数据集；
- 只保留当前版本，禁止 `revision-*/` 目录与 `v49`/`v50` 版本快照堆积；
- 画布 / 直接操作类交互：原型形态为 `states/` 三张状态图（空态 / 编辑中 / 已确认）+ `interaction-table.md`（触发 / 前置 / 结果 / 反馈 / 失败），**不要求**可点 HTML。

**理由**：静态 HTML 里"装得像能交互"最便宜的手段是弹窗，所以强制可点 HTML 会系统性地把画布设计推向弹窗——这正好是 `lane-manual-correction-loop` 的症状。状态图 + 操作表既便宜（不需写 JS）又不诱导模式偏移。

### D6 — 本次不写脚本，但留钩子

**决定**：四条机械判据（文字预算 / forbidden 模式 / provenance 完整性 / 原型体积）本次只落规范文本，由 `change-review` 按清单人工核。`design.md` 记录后续 change 的落点。

**理由**：实测 `ui-design-gate` 26 条裁决、34 次 `ui-design` 派发 100% 在门禁视野外（`requirements`/无档位）——挂在这个扩展上等于没写。脚本必须挂 apply 入口 + 归档门禁，照 `ui-verification-trace.sh` 的位置，这是一次独立的门禁改造，塞进本次会拖长工期且无法验证。

**后续 change 钩子**（不在本 change 实现）：
- `scripts/harness/ui-contract-check.sh`：读 `ui-design.md` 的三个新节 + `ui-prototype/`，输出 `pass|block` 与命中项；
- 挂载点：apply 入口（`mode=implementation` 首次写入前）+ 归档门禁（与 `ui-verification-trace.sh` 并列）；
- 判据：可见散文密度、`forbidden` 模式 class 是否出现、provenance 覆盖率、原型目录 md 数与行数。

## 3. 四层制品阶梯（写入 `开发执行规范.md` §3.1）

| 层 | 谁做 | 产出 | 硬约束 |
|---|---|---|---|
| **L0 契约** | 用户 + 强模型，1 轮 | `ui-design.md` 三个新节 + 区域骨架 | **唯一需要用户批准的东西** |
| L1 骨架 | 便宜模型 | 空壳截图 | 只能出现 L0 的区域，**禁止业务文案** |
| L2 交互 | 强模型 | 空态可点或状态图 | 不得新增 L0 之外的控件 |
| L3 数据 | 便宜模型 | `fixture.json` → 原型 import | 字段 ⊆ provenance 表，禁止手写数据 |

评审阶梯：

| 轮 | 给什么 | 用户只裁什么 |
|---|---|---|
| R0 | 3 个交互模式选项 + 区域骨架 | 选哪个模式 |
| R1 | 骨架截图 | 区域与层级对不对 |
| R2 | 空态可点 / 状态图 | 这条操作路径顺不顺 |
| R3 | `fixture.json` 摘要（≤1 屏） | 这些数我会看到吗 |

**R0–R2 禁止业务文案**——没文字可堆时，agent 只能用结构表达。

铁律：L1–L3 不允许新增 L0 之外的任何元素，越界即回 L0 问用户。

## 4. 风险

| 风险 | 处理 |
|---|---|
| 规范变长本身就是新的文字膨胀 | `interaction.md` 目标 ≤180 行，以表为主，不写论证段；本 design.md 的论证不复制进规范 |
| 无脚本 = 规矩会漂 | 已在 D6 登记后续 change；本次由 `change-review` 按清单人工核，并在 review brief 里显式点名四条判据 |
| `pinned` 被 agent 解释成"可优化" | 规范措辞用「不得替换，只能回问」，并在 `interaction.md` 给一条反例 |
| 旧 change 的 `ui-design.md` 不合新结构 | 不做追溯校验，仅在 review 时提示；新结构只对 rollout 后创建的 change 强制 |
| 画布用状态图会漏掉微交互（拖拽惯性、吸附） | `interaction-table.md` 的「反馈」列必须写清动效与吸附规则；实现阶段由 agent-browser 验收兜底 |
