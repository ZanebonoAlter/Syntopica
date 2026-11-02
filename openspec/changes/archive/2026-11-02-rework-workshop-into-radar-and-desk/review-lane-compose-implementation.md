# 新建泳道编排 — 实现期 review 建账（2026-10-31）

> 本文件记录本轮**产品实现**（区别于原型期的 `ui-prototype/revision-2026-10-30-lane-compose-selection/review.md`）的 review 意见与处置。
> 契约：[`lane-compose-implementation-contract.md`](lane-compose-implementation-contract.md)｜用例账本：[`test-cases-lane-compose-implementation.md`](test-cases-lane-compose-implementation.md)｜delta spec：[`specs/workshop-lane-compose/spec.md`](specs/workshop-lane-compose/spec.md)。

## 一、派发编排（§0.6 六步）

| 步 | 执行者 | 内容 | 结果 |
| --- | --- | --- | --- |
| 1 | 主线程 | 读制品 + `doc-impact.sh suggest`（预勾选 flow/api/database/standard）+ 并发态势核对 | ✅ |
| 2 | 主线程 | 写实现契约（端点/口径/落点/映射/红线）+ 白盒分支表 C1–C15 / D1–D11 / M1–M12 / S1–S10 / V1–V16 | ✅ |
| 3 | 子线程 | 后端实现（`minimax-cn/MiniMax-M3.1-Flash:max`）、前端实现（同模型）、前端返工补回丢失能力（同模型） | ✅ |
| 4 | 子线程 | **后端 review：`zai-coding-cn/glm-5.3:max`（用户 2026-10-31 当场指定）**，brief：[`review-lane-compose-backend-brief.md`](review-lane-compose-backend-brief.md) | ✅ 0 阻断 |
| 5 | 主线程 | 门禁 + 真机端到端 + 文档溯源 | 见 §四 |

## 二、后端 review 意见与处置（`zai-coding-cn/glm-5.3:max`）

结论：**阻断 0 / 建议 2 / 观察 5**。

| 编号 | 意见 | 处置 | 状态 |
| --- | --- | --- | --- |
| M1 | `window.days` 回显「生效值」与契约字面（请求值）冲突 | **采纳并定稿**：契约改为「回显生效值（>90 截 90、<=0 记 0、from/to 档为 to-from+1）」——与同对象 `from/to` 自洽；实现与 D10 断言不动 | ✅ 已改契约 §1.1 |
| M2 | 白盒表 D6 字面例自相矛盾（92 天跨度同时踩「超 90 天」） | **采纳**：用例表 D6 字面输入改为 `from=2026-10-20&to=2026-12-31`（72 天），并新增 D3b 记录「跨度 → 窗外 → 其它」的规则优先级；实现不动（判定实现正确） | ✅ 已改用例表 §3.2 |
| L1 | `window.from/to` 可为 null，契约未定义 | 契约 §1.1 补「无锚点/全历史时为 null（显式 JSON null，不是空串）」；前端类型需按 `string \| null` 归一化 | ✅ 已改契约；前端类型待收口 |
| L2 | 候选链输入隐含「有可用 embedding」前提 | 契约 §2.1 补前提说明（端点既有过滤） | ✅ 已改契约 |
| L3 | 后端 `days` 缺省仍是 0（≈全历史），契约写「默认 30」 | 契约 §1.1 改口径：**后端保持向后兼容（缺省 0）**，「默认近 30 天」由前端快捷档显式传 30 保证 | ✅ 已改契约 |
| L4 | `days` 非法字符串静默降级全历史 | 沿现状（HEAD 既有代码），记录在案、不改 | ✅ 记录 |
| L5 | 纯函数测试文件命名与 `testing.md` 分层表不一致 | 按契约指定名 `compose_chain_test.go` 保留（无 DB 依赖，`-short` 与全量都跑） | ✅ 保留 |
| D7 缺口 | 缺「from/to 与 days 同给 → days 被忽略」与「days<=0 全历史档 window 回显」两条用例 | 补 D12 / D13 两条用例（后端子线程返工） | ✅ 已补 |

**审核逐条检查结论**：A1–A10 / B1–B10 / C1–C8 / D1–D6 全部「合规」且给出文件:行证据；D7 为「基本合规 + 2 条缺口」（已补）；**未覆盖**：真实 POST 保存与事务回滚（用例账本 §5 人工豁免项）、同工作树其它 change 的后端脏文件（不在本轮范围）、全量 lint/vet 与 topicgraph 全包复跑（树莓派纪律）。

## 三、能力去向表（旧能力 → 新承接）

> 依据 AGENTS「review 落点复核」：旧入口用 `git show HEAD:<path>` 或现网旧组件核对真实调用点，不靠"新页面长得像"。

| 旧能力 | 旧入口 | 新承接 | 判定 | 证据 |
| --- | --- | --- | --- | --- |
| 历史待启用话题·**带入草稿** | 旧编排面左栏 `ComposeSidebar.vue` 候选卡 + `useInlineCompose.adopt` | 新工作面「历史待启用话题」入口 → 带入草稿（自动名绑定来源、人工名不被覆盖） | **已迁移**（首轮重写时**遗漏**，返工子线程补回） | `ComposeSidebar.vue:48/173-177`、`useInlineCompose.ts:382` |
| 历史待启用话题·**直接启用原话题** | 同上（`activate` → `updateTopic(status='active')`，资格沿服务端 `can_activate`） | 新工作面同一入口，按钮仅 `can_activate` 时可点 | **已迁移**（同上，返工补回） | `useInlineCompose.ts:405-410` |
| **合并泳道** | 旧编排面「更多操作」 | 版块治理 `BoardGovernanceRail.vue` 的合并入口（`openAdvanced: 'merge'`） | **入口迁移，未删除** | `BoardGovernanceRail.vue:47/156` |
| **历史回填** | 旧编排面「更多操作」 | 版块治理 `BoardGovernanceRail.vue` 的回填入口（`openAdvanced: 'backfill'`） | **入口迁移，未删除** | `BoardGovernanceRail.vue:47/156` |
| **散装事件列表 + 相似补选 + 20 条上限** | 旧编排面 | 三栏工作面（候选泳道整组 + 独立推荐） | **主动退役**（已批准原型如此；用户批准即视为知情退役） | 已批准原型 `ui-design-lane-compose-draft.md` §交互 |
| 旧 `useInlineCompose.adopt` 语义（相似补选 + 20 上限） | `useInlineCompose.ts:382` | 保留给 `BoardThreadBrowser.vue` 使用，编排态**不复用** | **未变** | `useInlineCompose.ts` 本轮零改动 |
| 候选区「关联事件清单 / 计算依据 / 话题泳道参照」 | 旧右栏 | 右栏瘦身为三段 + 两 tab | **主动退役**（已批准） | `ui-design-lane-compose-draft.md` §右栏 |

## 四、前端实现自报偏差与处置

| 偏差 | 处置 |
| --- | --- |
| 历史管理名单入口首轮未进新工作面 | **判为遗漏**，派返工子线程补回（见 §三 前两行） |
| 旧编排面「合并 / 历史回填」从工作面消失 | **判为入口迁移**（版块治理仍在），不重复入口；已登记 §三 |
| `cosineFor` 返回余弦**距离**（1−cos） | 保留原型语义（契约 §2.2 的 `distance` 同口径），注释已修正 |
| `placeLabels` 在产品里的落点是话题带标题的越界/撞车守卫 | 符合契约 §2.3（「全部标题」档按契约不进产品，移植时自然收敛） |
| 窄屏从旧版「自动 close」改为「只提示宽屏/横屏」 | 符合 delta spec「窄屏禁用编排」，采纳 |
| 历史带入草稿统一为「只带 owned 成员，不自动补相似、不截 20」 | 采纳（与候选整组口径一致，避免"偷偷自动勾选"）；与旧产品行为的差异已在此留痕 |

## 五、真机端到端（agent-browser）

见 [`ui-verification/lane-compose-e2e-2026-10-31/stories.md`](ui-verification/lane-compose-e2e-2026-10-31/stories.md)（E1–E14 故事 + G1–G5 几何断言 + E15 相关节点标题）。结果与截图落 `ui-verification/lane-compose-e2e-2026-10-31/`，**75 checks / 0 failed**。

## 五之二、用户补充建议的落地（2026-11-01「相关节点标题」）

用户当场提的附加建议：「选择一个话题后，其他相关的节点可以在节点下显示标题缩略」。实现期按契约落地，但**实机数据逼出了原型里没有的事实**，据此把形态定成两路（这是对用户建议的**细化**，不是打折）：

| 实机事实（2026-11-01 真库实测） | 影响 |
| --- | --- |
| 30 天窗里 1984 链的 3 个相关节点**全部属于已有 active 泳道** | 「画在节点下」在默认视图里**根本没有可画的节点** → 必须给隐藏的相关节点一条出路 |
| 只读参照区同泳道节点 **10px** 一档密排、CJK 行高实测 **13px** | 在只读梯子里逐点放标题必然**压字**（违反 layout.md「不得压字」）→ 只读相关节点改为**底部提示行点名** |
| 90 天窗里有 11 条候选链带「可画（候选）相关节点」 | 「节点下显示标题缩略」在可画区**成立**（实测 `1405:4555` → 4424/4469/4756 三条标题全部画出） |

最终口径（契约 §2.3 + delta spec 增补段落 + 2 个 Scenario）：

- **可画的相关节点**（候选/未归属）→ **节点下方**显示标题缩略（≤10 字带省略号）；撞上已放标题时**向下错行**（最多 2 行，仍冲突才宁缺勿叠跳过）——相关节点常落在相邻日期列，逐点标题本就会互相压。
- **只读的相关节点**（已有泳道）→ **不逐点画标题**，改为画布底部**提示行点名**（标题缩略 + 「在已有泳道里（默认隐藏）」说明）；**不代用户开只读参照开关**。
- 关系口径（**用户同日二次澄清后定稿**）：**相关** = ① 与已选成员**有原关系**（1 跳）的其它节点 ∪ ② **质心较近**的**可补充**推荐节点（= 右栏「可补充的事件」**同一批**，同一套距离与阈值口径）。①只认服务端透出的原关系（`identity`+`similarity`）、1 跳不做传递闭包；②**不含** active 只读参照（实测 30 天窗有 100+ 个 active 推荐，既不在右栏也不该在画布抢戏）；两条都排除已选成员自己。落点：`laneComposeLogic.ts:relatedNodeIds`（1 跳口径，可单测）+ 画布内合并 `centroidRecommendations` 的可补充组。

> 实机事实（30 天窗、1984 链选中）：1 跳相关 3 个（全在已有泳道）；质心推荐 **137** 个（114 active + **23 可补充**）→ 最终相关 = 26，其中可画 23（节点下出标题）、只读 3（提示行点名）。

建账：`human-feedback.md` FB-15（产品|frontend，已消化）｜`review.md` 未记（用户建议非 review 发现）｜用例 `laneComposeLogic.test.ts` R1–R5/T1–T2 + `BoardComposeOverlay.test.ts` 组件级 R6–R8｜E2E E15.0–E15.15。

## 五之三、同日另外两件（FB-16 tooltip / FB-17 滚轮缩放）

- **FB-16 tooltip × 压字**（已消化）：节点卡 `.lcc-card__close` 是右上角绝对定位，而 `.lcc-card__title` 没给它留位 → 文字多时标题流到 × 下面。修法：标题 `padding-right: 18px`（字号、卡片宽度、× 位置与可点面积都不动）。
- **FB-17 滚轮缩放**（已消化，子线程 `minimax-cn/MiniMax-M3.1-Flash:max` 实现）：
  - 缩放模型：**逻辑坐标不动**（`viewBox` 恒为 `0 0 vbWidth canvasHeight`），等比缩放靠改 SVG **元素尺寸** → **日期列永不重排**，且 `zoom=100%` 时 1:1 契约逐字成立（缩放只改尺寸不改 viewBox）。
  - 光标锚点：`zoomAnchorScroll` 换算新滚动位置，光标下内容尽量不动；滚轮 `preventDefault`（画布上滚轮不滚走页面）。
  - 点的像素下限：缩小时按 `max(逻辑半径, 下限/zoom)` 补偿，普通点渲染半径仍 ≥3px、只读 ≥2.5px。
  - 节点卡跟随：HTML 浮层定位改用**画布像素**（`point.x * zoom`、屏幕半径 `radius * zoom`），缩放后仍贴在节点旁（E16.5 量测 <200px）。
  - 控件：右下角读数 + 「重置」（只用 `--color-*` token、**没写 z-index**、没新开浮层档位）。
  - **已知取舍（如实留痕）**：滚轮被画布吃掉 → 纵向滚动改用容器滚动条；`scrollbar-gutter: stable` 常驻预留 ~10–15px 滚动条槽；缩放是组件本地状态、不进 URL。
  - 用例：`laneComposeLogic.test.ts` Z1–Z4 + 新建 `LaneComposeCanvas.test.ts` Z1–Z7 + 真机 E2E E16.1–E16.7。
- **FB-18 拖拽平移**（已消化）：按住左键拖画布即平移（内容跟手、滚动位置与指针位移反向）；**拖 ≠ 点**（位移 <4px 算点，节点点击/节点卡/快捷选入照常生效）；拖完那一下 click 不误选节点（250ms 窗口）；节点卡与缩放控件不参与平移。
  - **实现期实测踩到的坑（已修，留档）**：最初在 `pointerdown` 就 `setPointerCapture` → 浏览器把随后的 `pointerup`/`click` **改派到画布**，节点的点击收不到 → 真机 E2E 的「卡片跟随选中节点」直接红（`E7.3`）。改法：指针捕获**只在判定为拖拽的那一刻**启用（位移超阈值）。这条已写进契约 §2.3 与 delta spec，避免后人再踩。
  - 用例：`laneComposeLogic.test.ts` P1–P2 + `LaneComposeCanvas.test.ts` P1–P3 + 真机 E2E E17.1–E17.3。
- **FB-19 节点卡塌成竖条**（已消化，用户实机报障「有超长的显示 bug」）：绝对定位卡片默认 shrink-to-fit，宽度被「容器宽 − left」夹住；而 `left` 又用**渲染后量出来的宽度**算 → **反馈环**（量到 92px → 位置更靠右 → 可用宽更小 → …）把卡片塌成 92×296 的竖条、标题一个字一行。修法：宽度改用**常量** `CARD_WIDTH = 300` 参与定位、量出来的只取高度、样式固定宽度 + `max-height` 封顶 + 卡内滚动。
  - 这条是**缩放上线后才暴露**的（节点屏幕坐标变大 → 翻到左侧的分支变多 → 更容易进入反馈环），属于「加功能时把旧隐患放大」的典型，写进契约 §2.3 与 delta spec。
  - 回归锚：源码锚（Z8）+ 真机量宽（E16.8，真实浏览器 ≥240px）。

## 六、归档前仍欠的账（不洗绿）

- 真实 `POST .../persistent-topics/manual` 保存与事务回滚 → 人工豁免（用例账本 §5）。
- `bash scripts/harness/test-assets.sh workshop-lane-compose` → 该 capability 只在本 change 的 ADDED delta（主 specs 与 archive 均无），反查报「无匹配」属**新增能力的预期**，归档时以「capability 尚未归档」留痕，**不记绿**。
- 后端性能：90 天真实数据量下全量 section 扫描 + relations `IN` 查询未压测。
- 本 change 其余区域（总览/矩阵/归档视图线）的既有欠账与未覆盖项**不因本阶段清零**。
