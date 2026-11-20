# 用例 · unify-inner-scrollbar-style

> 故事锚点：**用户打开叙事工坊雷达首页，看到的是站内统一的一套暗底细滚动条**，而不是一条系统原生浅色条；阅读页文章列表的滚动条也从"几乎看不见"变成看得见。
> 纯呈现层 change：自动化落点为「样式单一事实源 + 类挂对元素 + 无裸声明残留」的结构断言，真实配色由 computed style 实测（agent-browser）+ 人工观感兜底。

## 主链路

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 打开 `/tags`（dark），左栏板块列表溢出 | dark 主题下的内部滚动区 | `.rd-stage` computed `scrollbar-width: thin`、`scrollbar-color: rgba(255,255,255,0.18)` + 透明轨 | agent-browser 实测 + 结构断言 | 6.6 + `scrollbar-style.test.ts` |
| 2 | 同页看右栏「当天线索最多的泳道」 | dark 主题下的内部滚动区 | `.rd-rail-scroll` 与内层 `.rd-rail` 同档，**不再出现浅色原生条**（用户反馈点） | agent-browser 实测 | 6.6 |
| 3 | 切 editorial 主题刷新 | editorial 主题下的内部滚动区 | 同位置变 `rgba(26,26,26,0.25)`，粗细不变 | agent-browser 实测 | 6.6 |
| 4 | 看阅读页文章列表与全局侧栏（dark） | dark 主题下的内部滚动区 | `.virtual-list` / `.sidebar-content` 同档，不再是硬编码浅色（dark 下原先几乎看不见） | agent-browser 实测 + 结构断言 | 6.6 + 测试 |
| 5 | 抽查设置分段 / 窄屏日期条 / 日报阅读台（侧栏、阅读区、边注栏） | 单一事实源与「类挂在滚动容器本身」 | 全部走同一工具类，文件内无裸 `scrollbar-*` 残留 | 结构断言 | `scrollbar-style.test.ts` |
| 6 | 回到首页整体滚动 | 页面级滚动条不受影响 | 仍为 10px 带轨杂志条；`html` computed `scrollbar-color` = `auto` | 实测 + 结构断言 | 6.6 + 测试 |

## 变体走查

| 组 | 变体 | 结论 |
| --- | --- | --- |
| 输入 | 内容不溢出（无滚动条） | 不适用——滚动条不出现，无绘制差异；`overflow` 语义未改 |
| 输入 | 极长内容（拇指很短） | 覆盖：`thin` 宽度与内容长度无关，拇指最小尺寸由 UA 决定，本次不改 |
| 前置 | 空列表（雷达无版块 / 日报无侧栏项） | 覆盖：无滚动区即无条，空态样式不受影响 |
| 前置 | 横向溢出（设置 tab 条 / 日期条 / 生命线） | 覆盖：`.scroll-thin` 同时声明 `height`，横向条同为 6px（标准通道 `thin`）；生命线豁免不动 |
| 前置 | 组件未挂载（`DailyReportDeskIndex` 的 `.ddi`） | 覆盖：样式已收敛但现网不可见，验收清单已注明，不当作可见影响 |
| 时间窗口 | 不适用 | 无时间语义 |
| 幂等 | 切主题多次 | 覆盖：纯 CSS token 派生，无状态，重复切换结果一致 |
| 可用性 | 深浅两主题可辨识度 | 覆盖：dark `rgba(255,255,255,0.18)` / editorial `rgba(26,26,26,0.25)`；实测见 6.6 |
| 可用性 | hover 态 | **不作验收点**：标准 `scrollbar-color` 没有 hover 语法，现代 Chromium 不走伪元素；hover 变亮只对老 Chromium 生效 |
| 可用性 | 键盘 / 触屏滚动 | 覆盖：不改 `overflow`、不改 `touch-action`，滚动行为零变化 |

## 效果核对

- 本 change **不依赖**数据覆盖率或 LLM 行为，无需真库量化核对。
- 唯一"效果因外部因素而变"的是浏览器引擎：Chromium ≥121 / Firefox 走标准属性，老 Chromium 走伪元素兜底。色值同源（token），一致性由结构断言 + Chromium 双主题 computed style 实测覆盖；Firefox 不做自动化，列入人工豁免（色值同源，风险低）。
- headless Chromium 使用 overlay 滚动条（`offsetWidth - clientWidth = 0`），**像素级截图看不到条**，故验收口径定为 computed style 数值 + 结构断言，不依赖"截图里有没有一条线"。
