<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

用户新装了 `antislop` 技能族（core + ui / human / copywriting / layoutmobile）。本 change 判定它**能**接进原型驱动开发，但**不能整包引入**——整包照抄会反噬 2026-11-15 刚建立的两条纪律。

三条实测契合点（问题都能在历史 change 里找到实例）：

1. **编造的"真实感"数据是比视觉更深的一层 slop。** `add-home-pipeline-status-page` 原型里 `fake` 命中 163 处、`示例` 249 处。现有 `Data Provenance` 只要求「没来源就标 `none` + `needs-approval`」，**没说清 `none` 之后占位符该长什么样**，于是 agent 用 `John Doe` 式假数据填满界面，评审误以为有数据支撑。antislop 给出的界线可直接用：真实值 / 诚实占位（`Your Name`、`email@example.com`、`[REAL DATA]`）/ 伪造内容（`John Doe`、无对照期的 `+12%`、编造动态流）。
2. **状态"存在"不等于"合格"。** 现有契约要求状态矩阵四态齐全，但 antislop R-27 指出 `No data available` 这种空态虽然技术上有，仍然什么都告诉不了用户。首次 / 筛选为空 / 无权限是三种不同空态，共用一个就是 slop。
3. **视觉与动效的否决项完全空白。** 现有 `interaction.md` 145 行里没有任何一条关于对比度、焦点环、`prefers-reduced-motion`、glass/glow 剂量。这些决定**在原型阶段就被固定**，到实现阶段改要重做整张配色。

同时三条**不吸收**（写进 Non-Goals，防止 agent 误用）：整套 Delivery Gate 的 20 项 PASS/FAIL 报告（会重新制造文字膨胀，与 R0–R2「禁止业务文案」直接冲突）、英文的 em dash 禁令（中文破折号 `——` 是合法标点，硬搬会误伤）、`DESIGN.md` / dials 体系（本项目已有双主题 token 系统 `theming.md`）。

## What Changes

1. **新增 `docs/reference/standard/frontend/prototype-content.md`**（三节，117 行）：
   - 占位数据与伪造内容的界线（判定依据 + 封闭白名单五字面 + 标注载体与剂量 + 审批衔接）；
   - 空态/加载/错误态必须给「原因 + 下一步」，三类空态不得共用（SPA 加载以 `loading-experience.md` 为准）；
   - 视觉与动效的最低否决项 V1–V6（对比度、焦点可见、键盘可达、动效与 `prefers-reduced-motion`、剂量与装饰），含**画布类形态的载体分流**（四行可访问声明落 `interaction-table.md`）。
2. **`interaction.md` 保持 ≤180 行不变**——初版把三节塞进去实测涨到 216 行（只剩 4 行余量），改为拆文件，146 行。
3. `layout.md`「交互模式选择判据」节、`theming.md` 首部、`loading-experience.md` 首部各补一条互指；`standard/README.md` 与 `constraints-index.md` 登记新文件。

## Non-Goals

- **不引入 antislop 的 Delivery Gate 报告形态。** 它要求交付前输出 20+ 项 PASS/FAIL 报告——这与本仓库 R0–R2「禁止业务文案」、原型预算「≤12 短语/屏」直接冲突。判据本体吸收，载体改用既有的 `ui-verification/report.md` 判定条目。
- **不引入 em dash（`—`）禁令。** 初版拟「仅限英文整句文案」，review 实测发现产品代码已有 `direct_hit — 精确重叠` 这类写法，规则一生效即判存量违规，且本项目无英文营销文案——收益接近零、误伤面确定存在，故删除。`—` 只保留「诚实占位」语义。
- **不引入 `DESIGN.md` / ENERGY-RHYTHM-MOTION dials 体系。** 本项目是内部工具，视觉基线已在 `theming.md` 的双主题 token 里；再引一套方向声明体系等于双权威。
- **不引入 antislop 的 codemode/代码注释类规则**（`antislop-code`）——与原型阶段无关，交给实现阶段的既有规范。
- **不在项目文档里引用 `.pi/agent/skills/` 路径作为权威源。** 那是用户级安装，换机器即断；规则本体内联进本仓库文档，只在 `design.md` 记 provenance。
- 不改产品代码、不改 `.pi/extensions/`。

## Impact

- 受影响制品：`ui-design.md` 的状态节与 `Data Provenance` 节的填充质量；`ui-prototype/` 里的占位数据与视觉基线。
- 产品运行时行为：**零变化**（`front/` 与 `backend-go/` 零 diff）。
- 旧 change：不做追溯校验；新规则只对 rollout 后创建的 change 生效。

## 回检口径

下一个 `ui-impact: major|minor` 的 change 归档时，检查 `ui-prototype/` 里是否还出现 `John Doe` / `johndoe@example.com` / 无对照期的百分比 delta 这类伪造内容；若仍出现，说明仅靠规范文本无效，须并入 `add-ui-stage-accounting` 后续的 B 阶段脚本化。
