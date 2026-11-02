# UI 验收账本 — rework-workshop-into-radar-and-desk

> **2026-10-26 最新复核：不能按验收完成收口。** A-43 原 pass 已被实页点击反证，现修正为 fail；原截图保留。A-35/A-36/A-46 仅支持入口可达/功能接线，不代表信息架构定位、控件产品适配已通过。新增 A-55/A-56 记录宽屏操作区及旧关系故事的未覆盖边界。详见 [第二轮规范与产品复盘](norm-audit-2026-10-26/report.md)，问题未修复、未部署。

> 逐阶段建账。验收环境：本地静态托管 `http://localhost:5100`（same-origin），agent-browser 实测，
> 截图存本目录。`ui-verification-trace.sh` 以本文件为唯一事实源。
> 视口：1440×900 / 1920×1080 / 1024×768（中宽降级）/ 375×667（窄屏）。

## L1 版块态势（矩阵 + 治理右轨）

| 条目 | 判定 | 依据 |
| --- | --- | --- |
| A-1 雷达 → 点最热行 → L1 矩阵 | **pass** | `/tags` 渲染 12 个活跃版块：4 个关注行 + 常温 8 个折叠；5 条今日该读；无升温显示明确文案；点行 `/tags/3030` 进 L1；radar-1440.png |
| A-2 矩阵点格子 → section 抽屉 → Esc 后选中态保持 | **pass** | 1440 实测；抽屉按 section_id 联查日报线索；l1-1440.png；点 `matrix-cell-1204-2026-10-11` 开抽屉，Esc 关闭后 `?lane=` 保持 |
| A-9 L1 三区 + 固定视口（1440/1920） | **pass** | 页面无滚动；舞台内滚；右轨 18rem@1440 / 21rem@1920；l1-1440.png / l1-1920.png；`documentElement.scrollHeight=900/1080` |
| A-10 治理右轨 + 治理抽屉（32rem、五页签、来源 5 列） | **pass** | 右轨体检/候选 top3/来源 top3/入口；抽屉五页签可切；l1-gov-1440.png；抽屉 panel 512×900@1440，来源表 5 列且抽屉内横向滚动 |
| A-11 中宽 1024 降级：右轨隐藏 + 顶栏治理入口 | **pass** | 1024 舞台 799px，矩阵不横滚；治理抽屉 480×768；l1-1024.png / l1-gov-1024.png |
| A-12 窄屏 375 基线 | **pass** | `scrollWidth=375` 无横向溢出；右轨/版块栏隐藏，矩阵不渲染，改为版块摘要（6 泳道 × 14 格）；story-s1-375.png |
| A-13 归档文章抽屉 60vw（证据/日报状态/打开原文） | **pass** | 1440 下 864×900；1920 下按 64rem 封顶 1057×1080；Esc 关闭后归档上下文保留；archive-drawer-1440.png / archive-drawer-1920.png；无「补进日报/重跑」写动作 |
| A-14 归档双视口 + 窄屏 | **pass** | 1440/1920 固定视口内滚，375 无横向溢出；archive-1920.png / archive-375.png；1920 舞台内滚 3326/1023 |
| A-15 编排态画布（日期列 × 泳道 + 待确认、关联线、侧栏、Esc） | **pass** | 1440 全屏不透明、14 连续日期列、关联 4 条线；搜索「俄乌」从 32 卡收敛到 2 卡；已归属 section 在画布/抽屉均只读（B-4：抽屉移出「加入新泳道」）；1024 画布 709 + 侧栏 315；375 不渲染且提示；compose-1440.png |
| A-16 阅读台响应式 | **pass** | 1100 收索引→正文顶锚点条（3 栏）；900 隐左队列（正文+右栏）；375 单栏、`scrollWidth=375`；desk-375.png |
| A-17 无日报空态 | **pass** | `/tags/1974/2026-01-01` 显示「这一天还没有日报」+ 生成/看最近一期；未点生成（避免写库）；desk-empty-1440.png |
| A-18 阅读台文章抽屉 | **pass** | thread 文章点击打开 60vw 抽屉、URL 停留日报路由；desk-article-1440.png |
| A-20 抽屉 Esc 分层 | **pass** | 阅读台文章抽屉打开时按 Esc：抽屉关闭、`/tags/1974/2026-10-23` 路由保持不变（B-2 修复） |
| A-25 用户故事 S1 版块态势 | **pass** | 1974：5 活跃 + 1 休眠；行尾 38/27/17/16/10 条线索（title 保留 14/14/9/10/7 分区）；section 抽屉 480×900、Esc 关闭保持泳道；空格点击提示不打开；休眠展开可选中；375 摘要 6 泳道×14=84 格；story-s1-1440.png / story-s1-1920.png / story-s1-375.png / story-s1-evolution-1440.png |
| A-26 用户故事 S2 归档 | **pass** | 默认数据日 10-25：13 篇 / 12 已入 / 1 未入 / 6 弱证据；自由日期与近 7 天重新取数；平铺 13 行、行内全部标签；证据抽屉逐标签规则/分数/降级/方向；product-archive-light-1440.png / product-archive-dark-1440.png |
| A-27 用户故事 S3 编排态 | **pass** | 全屏、14 日期列、71 sections；右侧「按天选事件」18 条未归属 section 以标题+勾选框平铺，画布节点 hover 有完整标题；采纳/确认启用/搜索/空名称提示保持；375 点击提示；product-compose-light-1440.png / product-compose-dark-1440.png |
| A-28 用户故事 S4 雷达 | **pass** | 12 活跃=4 行 + 常温 8；今日该读 5 条；点 3030 进 L1；热点 0（最高 attention 34）不伪造「今天要看」；1920 双栏 1459/352 无页面滚动；375 单列无溢出、迷你矩阵隐藏；story-s4-radar-1440.png / story-s4-radar-1920.png / story-s4-radar-375.png |
| A-29 用户故事 S5 阅读台 | **pass** | 1440 两栏 225/600 居中，无最左历史栏/右侧演进；日期历史下拉并入索引顶部；1100 单栏 615 + 紧凑日期/锚点条；375 单栏无横向溢出；页边注入口改为右下抽屉；product-desk-light-1440.png / product-desk-light-1100.png / product-desk-light-375.png / product-desk-dark-1440.png |
| A-30 用户故事 S6 来源构成 | **pass** | 右轨 top3 华尔街 63/50%、凤凰 33/26%、联合早报 16/13%；治理抽屉来源页 5 列表格、7/30/90 切换后 148 篇 7 源；只读无写动作；story-s6-sources-1440.png / story-s6-drawer-1440.png |
| A-31 S5 Esc 回矩阵焦点恢复 | **pass** | `/tags/1974/2026-10-24?lane=1204` 按 Esc → `/tags/1974?lane=1204`，`document.activeElement` 为 `matrix-lane-1204`（不再落到 body）；临时 `focus=report` query 自动清理 |
| A-24 旧深链路由化 | **pass** | `/tags?board=1974&report=1229&annotation=5` → `/tags/1974/2026-10-24?annotation=5` 阅读台；`/tags?board=1974` → `/tags/1974` 矩阵 |
| A-32 雷达 3840 超宽 | **pass** | 3840 下 `.radar` 1920 居中（x=960），列表 1459 + 右栏 352，无横向溢出；radar-3840.png；4K 视觉密度仍偏松（内容 4 行） |
| A-23 雷达 1920 固定视口 | **pass** | 1920×1080 下列表 1459 + 右栏 352 双栏，页面级纵向滚动 0；radar-1920.png |
| A-21 雷达窄屏单列 | **pass** | 375 下列表单列、`scrollWidth=375`、迷你泳道矩阵隐藏、侧栏下沉；radar-375.png |
| A-22 版块摘要窄屏 | **pass** | 375 下不渲染矩阵/右轨，6 条泳道 × 14 格迷你条=84、日报入口「看 10-24 日报」；summary-375.png |
| A-19 泳道热度改用线索数 | **pass** | 演进带 meta「14 天 38 条线索 · 今日 0」，节点 aria「N 条线索」；lane-dynamics 实测美伊 14 个 section / 38 条线索（窗口 10-11~10-24），矩阵行尾 38（title 保留 14 个分区）；矩阵格值仍为 section 数（入口语义） |

## 归档 / 编排 / 阅读台（后续阶段）

| 条目 | 判定 | 依据 |
| --- | --- | --- |
| A-3 视图切归档 → 日期平铺 | **pass** | 默认 2026-10-25 单日 13 篇（12 已入 / 1 未入 / 6 弱证据），日期头下列出文章，不再按话题分组；product-archive-light-1440.png |
| A-4 归档日期/筛选 | **pass** | 「数据日 / 近 7 / 14 / 30 天」与自由起止日期均重新取数；未纳入/弱证据按单篇生效；product-archive-light-1440.png / product-archive-light-375.png |
| A-5 编排态采纳候选 → 质量条变化 | **pass** | 采纳「沙特求援美拒打击胡塞…」→ 泳道名预填；该候选 owned section 不在近 14 天池时成员可为 0，属数据边界；compose-1440.png |
| A-6 确认启用禁用提示 | **pass** | `BoardComposeOverlay.test.ts` 覆盖连续 1 天置灰 + 提示、连续 3 天可点；真实候选全为「已中断」组，仅采纳入口 |
| A-7 阅读台两栏渲染 | **pass** | 1440 下 225px 索引 + 600px 正文居中；日期下拉可切日报；无最左历史栏、无右栏演进/本期话题；正文独立滚动；product-desk-light-1440.png |
| A-8 深链 `/tags/:boardId/:date` | **pass** | 直接打开历史日期自动扩窗并渲染；Esc/返回矩阵恢复 `?lane=1204`；切日期 replace 保留 lane；1100/375 收为单栏 + 日期下拉；product-desk-light-1100.png / product-desk-light-375.png |

| A-33 暗色主题（雷达/控制台/阅读台） | **pass** | `html[data-theme=dark]` 底色 `rgb(8,12,18)`；雷达行文字 `rgba(255,255,255,.9)` 可见；控制台、阅读台、编排态均无白底白字；product-radar-dark-1440.png / product-console-dark-1440.png / product-desk-dark-1440.png / product-compose-dark-1440.png |
| A-34 返回主页入口 | **pass** | 雷达「主页」、控制台面包屑主页图标、阅读台「主页」、研究台「主页」四处均可点回 `/`；实测四条路径均为 `/`；system-regression/light-home-1440.png |
| A-35 新建版块入口 | **pass** | 雷达头部常驻「新建版块」+ 空态「新建第一个版块」；控制台左栏「新建版块」、顶部「更多 → 新建版块」、右轨「版块治理 → 新建版块」；单测覆盖冷启动创建并跳 `/tags/77`；product-radar-light-1440.png / product-console-more-light-1440.png |
| A-36 旧总览治理能力恢复 | **pass** | 顶部「更多」/右轨「版块治理」提供扩充版块、匹配回填、匹配规则、标签合并、标签池；匹配规则弹窗、升级建议面板、标签池弹窗实测可打开；标签合并修复 `merge-preview?limit=200` 的问号缺失后 188 组正常加载；product-console-more-light-1440.png |
| A-37 归档自由日期与证据 | **pass** | 默认 10-25、近 7/14/30 与自由起止均重新请求；13 行平铺、行内展示全部标签；点行抽屉展示逐标签规则/分数/降级/方向与「为什么匹配」；product-archive-light-1440.png / product-archive-light-375.png / product-archive-drawer-dark-1440.png |
| A-38 编排按天事件列表 | **pass** | 右侧「按天选事件」显示 18 条未归属 section 的完整标题与勾选框，点击同步画布；节点 `title` 为完整事件标题；product-compose-light-1440.png |
| A-39 阅读台去重 | **pass** | 最左历史栏与右栏演进/本期话题不再渲染；日期历史进索引顶部下拉；页边注为右下抽屉；1100/375 单栏；product-desk-light-1440.png / product-desk-light-1100.png / product-desk-light-375.png |
| A-40 全系统回归矩阵 | **pass** | `/`、`/search`、`/discovery`、`/settings`、`/tags`、`/tags/:boardId`、`/tags/:boardId/:date`、`/tags/:boardId/research` 在 editorial/dark × 1440×900（关键页另 375×667）均渲染、无横向溢出；system-regression/light-*-1440.png / dark-*-1440.png |

| A-41 无 active 泳道仍有日报出口 | **pass** | `/tags/50413` 无活跃泳道时显示「已有日报内容」回退条 + 顶栏「最新日报」；可直接读 2026-10-24 日报、看同日归档（3 篇），不再要求先转正话题；product-50413-fallback-1440.png |
| A-42 删除版块安全入口 | **pass** | 治理抽屉 → 设置出现「删除版块」危险区，二次确认文案含不可撤销；`BoardSettingsForm.test.ts` 覆盖确认→API→deleted 事件；product-delete-1440.png |
| A-43 归档站内阅读与计算明细 | **fail** | 2026-10-26 撤回原 pass：站内阅读入口恢复不等于计算明细可用。1440×900 点击计算明细后面板 y=937.625、position=static、父为归档列表，不在文章抽屉内且视口内不可见；norm-audit-2026-10-26-match-detail-after-click-1440.png。旧 product-archive-read-1440.png / product-match-detail-1440.png 保留，但后者只有文章证据抽屉，不能作为计算面板展示通过的依据 |
| A-44 归档来源/方向筛选 | **pass** | 归档工具栏有来源下拉与「显示方向不符」开关，分别传 `feed_id` / `show_direction_mismatch=true`；日期/筛选变化重新取数；product-archive-filters-1440.png |
| A-45 话题管理直达入口 | **pass** | 治理右轨「话题管理（重命名 / 归档 / 删除）」直接打开 TopicManageDialog，不再必须绕侦探墙；实测 50413 候选行有确认启用/重命名/合并/归档/删除；product-topics-1440.png |
| A-46 阅读台跨版块切换 | **pass** | 阅读台索引顶部新增版块下拉（1974/1980/…），可切换并保留日期；最左历史栏与右栏演进仍不渲染；product-desk-board-1440.png |
| A-47 历史格子 → 归档带日期 | **pass** | 1974 矩阵 10-12 格子 → section 抽屉「看归档」→ 归档范围显示 `2026-10-12 ~ 2026-10-12`，不再落最新一天 |
| A-48 归档同日按完整发布时间排序 | **pass** | `sortArchiveArticles` 比较完整 `pub_date`，同日才用 id 兜底；纯逻辑用例覆盖 23:20 → 20:19 → 20:19 → 09:02，不被入库 id 打乱；`useBoardArchive.test.ts` |
| A-49 全库回填范围提示 | **pass** | 「更多」菜单文案改为「匹配回填（全库）」，点击先 `confirm` 说明会重算全库、可能耗时；取消不发请求 |
| A-50 全系统/产品任务复审 | **pass** | `/`、`/search`、`/discovery`、`/settings`、`/tags`、`/tags/:boardId`、`/tags/:boardId/:date`、`/tags/:boardId/research` 的 editorial/dark × 1440（关键页 375）截图在 `system-regression/`；产品任务：无泳道读日报、归档站内读正文、历史格子带日期、话题管理、阅读台切版块均实点 |

| A-51 暗色常温入口可读性 | **pass** | dark 下展开常温 8 版块：入口文字 `rgba(255,255,255,0.6)`、字号 11.84px，不再是 0.35 的禁用观感；日期副文本同步改为 secondary；product-radar-cold-dark-1440.png |
| A-52 研究台 375 标题 | **pass** | 375 dark 下「研究」单行 35×26（不再上下拆成两字），说明文案换行到下一行；product-research-dark-375.png |
| A-53 编排事件列表可读性 | **pass** | `.bco-event-row` 字号提升至 0.76rem / 行高 1.5，选中按钮带 `aria-pressed`；画布节点保留完整 title；product-compose-light-1440.png |
| A-54 全库回填范围提示 | **pass** | 菜单文案「匹配回填（全库）」+ 点击前 confirm 说明重算全库；取消不触发请求 |

## 2026-10-26 补充核验边界

| 条目 | 判定 | 依据 |
| --- | --- | --- |
| A-55 日报切换操作区的目标宽屏产品适配 | **未覆盖** | 本轮补2560×1440 CSS等效视口测量和截图，已有根字号缩放，但不能替代用户否定后的操作区设计复审；长名称/下拉展开态及全部视口尚未完成复验，不以A-46功能切换抵消；norm-audit-2026-10-26-desk-2560.png |
| A-56 旧非泳道关系浏览与待确认引导的完整承接 | **未覆盖** | 1974编排初始71节点/0线，点第一已归属节点后1条identity线；局部画线不证明原similarity DAG/待确认关联故事完整保留。各待确认节点、全量开关和旧关系语义尚未完成重新对账与产品确认；norm-audit-2026-10-26-compose-focus-1440.png |

## 说明

- 截图均来自本轮实际构建（根字号 `clamp(15px,.86vw,30px)` 生效）。
- A-1 雷达已随 8.1–8.3 补齐；A-7/A-8 已随 6.x 补齐。8.4 旧面板清理仍待办。
- 暗色主题与 4K（3840 CSS）已在 2026-10-25 复审补齐（A-33/A-32）；全系统路由矩阵见 A-40。
