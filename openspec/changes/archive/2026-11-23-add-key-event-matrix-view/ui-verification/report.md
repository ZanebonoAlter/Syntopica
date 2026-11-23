# 关键事件视图 · 实机视觉验收报告

- 变更：`add-key-event-matrix-view`（ui-impact: major）
- 验收方式：agent-browser 无头 Chromium（session `kev-rv4-1795427062`，独立命名 session；同机另有一条 `lane-star` 会话在跑，未受影响），机械断言用 `eval` 取 DOM 度量 + 页面内 hook `window.fetch` 抓请求参数
- 验收日期：2026-11-23（**第四轮 · review 修复复验**：review 查出 R-1～R-9 共 9 条，主线程处置为 **7 条已修（R-1 / R-3 / R-4 / R-5 / R-7 / R-8 及 design 制品侧 R-2）+ 2 条驳回（R-6 / R-9）**；本轮只做「修复有没有把已通过的验收弄坏 + 修复本身生效」的定点复验，不改实现代码。A-25 为产品级既有债，按主线程拍板保持 fail 不动）
- 被测版本：工作树整树部署（`bash scripts/dev/deploy-frontend.sh`，构建 + 铺盘 + 后端重启约 3.5 分钟，`/health` 200，PID 316436）
- 靶子：版块 1980「生成式 AI 与大模型厂商」`/tags/1980`；空态靶子版块 2242；右轨「话题追踪」可定位条目只在版块 3030 存在（1980 的两条 watch 均无对应泳道，不渲染为可点条目）
- 判定词表：`pass` / `fail` / `未覆盖`（封闭三值）；截图只在本目录，同名文件即为实物
- 口径备注：候选话题 2477 的 11-19 确认事件按 spec「候选话题不占行、其确认事件不呈现」不出现（非故障）；30 天窗 API 实返 23 条（31 天窗）− 1 条 candidate = 视图呈现 22 条；事件卡文章列表只出标题 + 日期（`/articles/:id` 只给 `feed_id` 不给来源名，既有 `LaneRelatedArticles` 同款限制）
- 第四轮口径备注（R-4 修复后）：关键事件读端 `days` 不再封顶 365，请求窗口按**可见轴起点 → 今天**如实取。实测各档：14 天档 → `days=15`、30 天档 → `days=31`、365 天档 → `days=366`（anchor = 该版块最新 completed 日报日 2026-11-22，today = 2026-11-23，365 档轴 = 2025-11-23 ~ 2026-11-22 共 365 列）。DB 真值：`semantic_board_id=1980 AND state='confirmed'` 共 28 条 − 1 条 candidate 话题 2477 = **27 条**（active 话题 89/90/93/94/971），与 365 档视图的 27 节点 / 27 标签逐行吻合
- 第四轮复现提示：右轨「当日其他话题」的 `data-testid="rail-other-topic-item-<sectionId>"` 挂在 **`<li>`** 上，可点的是其内层 `button.rob-row-btn`（`data-testid="rail-other-topic-row"`）；直接 `li.click()` **不会**触发选择（事件不向下冒到子按钮），本轮前几次误判即由此而来，测该路径必须点内层按钮
- 本轮附带：对 delta spec `specs/workshop-board-console/spec.md` 做了两处**措辞收紧**（仅动文字、未动任何行为条款与场景），目的是让 `openspec validate --strict` 的「requirement 正文 > 500 字」告警消失、并消除契约内部矛盾；明细见本文件末「附：规格措辞修订」。

## 三档视口机械断言总表（第二轮复测；第三轮终验抽验项见各条目依据；第四轮 review 修复复验的定点实测见各条目「复验·第四轮」行）

| 视口 | 持久化行数 | 节点数（14 天 / 30 天） | 页面级横向滚动 | 舞台内横向滚动（client→scroll） | 标签重叠对数（14 天 / 30 天） | 标签层数（row 93 / 971） | 虚线轴宽 | 选中态保持 | 拖拽平移 | 事件→文章→阅读闭合 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1440×900 | 6 | 11 / 22 | 无（scrollWidth=clientWidth=1440） | 有（845→2112，30 天档） | **0 / 0**（上轮 11 / 17） | **4 层 / 3 层**（上轮全 1 层） | **896px**（上轮 0px） | 保持 | 生效 | 闭合 |
| 1920×1080 | 6 | 11 / – | 无（1920=1920） | 无（14 天 1277=1277） | 0 | 4 层 / 3 层 | **1085px**（上轮 0px） | 保持 | – | – |
| 2560×1271 | 6 | 11 / – | 无（2560=2560） | 无（1917=1917，**首屏即均分**） | 0 | 4 层 / 3 层 | **1725px**（上轮 0px） | 保持 | – | – |

窄屏 700×900：「关键事件」按钮与视图均不渲染，回落版块摘要（`board-narrow-summary`）。第四轮补充：窄屏直开 `?view=events` 深链时同样回落摘要，且 **URL 的 `view` 被清掉、阅读槽仍在、seg 有按下项**（见 A-39）。

## 判定条目

### A-1 时间视窗与矩阵共用同一控件【pass】
- 依据（复验）：关键事件视图内 `observation-range-controls` 计数 = 1（无第二个跨度控件）；在事件视图内切 30 天，日期轴由 14 列变 30 列（2026-10-24~11-22），14 天时为 2026-11-09~11-22；切回矩阵仍是同一控件、同一档位。截图 `1440-events-14d-labels-layered.png`（1440×900）
- 依据（复验·第四轮）：`/tags/1980?view=events` 首屏实测全页 `data-testid="observation-range-controls"` 计数 = **1**、`.orc-tiers` 组 = **1**、档位 chip 恰 **4** 个（7 天 / 14 天 / 30 天 / 近 1 年），事件视图内不再自带第二套跨度控件；R-4 改请求窗口后控件与轴口径未变（14 天轴 11/09~11/22 → 30 天轴 10/24~11/22 → 365 天轴 11/23~11/22 共 365 列）。截图 `1440-events-365d-request-days-366.png`（1440×900）

### A-2 行集合只含持久化话题、候选话题不占行【pass】
- 依据（复验）：1440 视口渲染 6 行（topic 88/89/90/93/94/971，与 DB 中该版块 active 持久化话题一致）；candidate 话题 2477 不占行，其 11-19 确认事件在轴与当日阅读区都不出现（30 天窗逐日节点统计无 2026-11-19）；版块级接口 31 天窗实返 23 条 confirmed，其中 1 条属 candidate → 视图呈现 22 条，口径自洽。截图 `1440-events-14d-labels-layered.png`（1440×900）
- 依据（复验·第四轮）：14 天窗逐日节点统计 = 2026-11-10 ×3、11-11 ×1、11-12 ×1、11-14 ×1、11-16 ×1、**11-18 ×3**、11-22 ×1，合计 11 节点 / 11 标签，**无 2026-11-19**（candidate 话题 2477 事件按口径不呈现）；365 天窗节点 27 / 标签 27（= DB 真值 28 − 1 candidate），行集合仍为 6 行。截图 `1440-events-14d-labels-layered.png`（1440×900）

### A-3 11-18 列落 3 个事件节点与 3 个事件标签【pass】
- 依据（复验）：14 天窗内 2026-11-18 逐日节点计数 = 3（topic 93 ×1、topic 971 ×2），同日三条各自成标签未合并成 1 条；整窗 11 个节点 / 11 条标签一一对应。截图 `1440-events-14d-labels-layered.png`（1440×900）
- 依据（复验·第四轮）：14 天窗逐日节点统计实测 11-18 = **3**（其余日 3/1/1/1/1/1），整窗 11 节点 = 11 标签，节点与标签仍一一对应，未合并。截图 `1440-events-14d-labels-layered.png`（1440×900）

### A-4 点事件节点后下方阅读区出现同日全部事件标题【pass】
- 依据（复验）：点 `key-event-node-93-2026-11-18` 后阅读区标题「11/18 关键事件 3 条」，三条标题为「GPT-6.2 Astra与Sol双模型发布，分层定位专业与日常」「可灵Kling Image 4.0聚焦专业场景，支持原生8K与组图直出」「MiniCPM-V 4.7以MoE架构优化端侧推理」；此时仍停在关键事件视图。截图 `1440-events-inside-selection-stays.png`（1440×900）
- 依据（复验·第四轮）：`/tags/1980?view=events` 点 `key-event-node-93-2026-11-18` → 阅读区实测 `.kdp-title` = 「11/18 关键事件」、`.kdp-count` = 「3 条」、三条 `.kdp-card-title` 全文与上轮逐字一致（32 / 35 / 38 字），**未少字、未截断**；此时 `key-event-view` 仍在（未踢出时间轴），URL = `?view=events`。截图 `1440-events-inside-selection-stays.png`（1440×900）

### A-5 切换视图保留 selectedLane / selectedDate / 观察窗【pass】
- 依据（复验）：事件视图选 topic 93 + 11-18 → 切矩阵后 URL 保留 `?lane=93`，矩阵内 `matrix-lane-93`（is-selected）与 `matrix-cell-93-2026-11-18` 均处选中态 → 切回事件视图后行头 93 仍 `is-selected`、11-18 仍选中、轴仍 14 天。截图 `1440-events-inside-selection-stays.png`（1440×900）

### A-6 URL 恢复与非法值回落【pass】
- 依据（复验）：直接访问 `/tags/1980?view=events` 渲染事件视图；访问 `?view=xxx` 回落矩阵（`console-view-matrix` aria-pressed=true、事件视图不存在），非法值不写回视图参数。截图 `1440-events-14d-labels-layered.png`（1440×900）

### A-7 30 天列宽守 64px 下限、舞台内横向滚动、行头 sticky【pass】
- 依据（复验）：30 天时 `.kev-day` 实测 `--kev-col=64px`（列宽下限），舞台容器 scrollWidth 2112 > clientWidth 845（舞台内横滚成立），页面级 scrollWidth=clientWidth=1440（无页面级横滚）；舞台滚到 scrollLeft=900 时行头仍贴在舞台左沿（left=290=stage left，`position: sticky`）。截图 `1440-events-30d-selected-out-of-view.png`（1440×900）

### A-8 切到 30 天后选中日自动滚入视口【pass】
- 依据（复验，**fail → pass**）：上轮 fail 现象为「14 天切 30 天后 scrollLeft=0、选中日 11-22 实测 left=2338 / right=2402、舞台右沿 1134，`selectedInStage=false`；刷新直开 `?view=events&days=30` 同样 scrollLeft=0」。本轮实测（第三轮·终验，session `kev-final-1795424275`，1440×900）：① **切档路径** — `/tags/1980?view=events` 点「30 天」后 `.kev-scroll` 的 `scrollLeft` 落定为 **1267**（≠0）；instrument（patch `Element.prototype.scrollTo`）抓到的调用目标值确为 **1657.5**（= 192 + 29×64 + 32 − 845/2，与 30 天轴一致），浏览器按可达上限钳位（`scrollWidth 2112 − clientWidth 845 = 1267`；30 天轴内容宽 192 + 30×64 恰好铺满，1657.5 物理不可达）；选中日 `11/22` 列实测 `left=1070.59 / right=1134.59`，舞台 `left=289.59 / right=1134.41` → **`selectedInStage=true`**（右沿 0.19px 子像素舍入，肉眼全见；非上轮「差 1267px 在视野外」）。② **冷启动路径** — 直开 `?view=events&days=30` 实测 `scrollLeft=1267`（>0）、`selectedInStage=true`、选中日同在舞台右沿内。③ **反向不退化** — 点事件节点 `key-event-node-93-2026-11-10` 换日实测 `scrollLeft=890`（= 192 + 17×64 + 32 − 845/2，与任务书预期一致），选中日 `11/10` 居中（`left=679.59 / right=743.59`，舞台中心 712.00）、`selectedInStage=true`，两条路径互不打架。④ **抖动** — 切档后以 400ms 间隔连采 8 次，`scrollLeft` 序列 `0 → 1186 → 1267 → 1267×5`（平滑滚动单调推进，无来回跳），落定后再采 3 次恒为 1267。截图 `1440-events-30d-selected-in-view.png`（切档后 30 天 / 选中日在视口内）、`1440-events-30d-node-date-scrolled.png`（点节点换日 scrollLeft=890）（均 1440×900）

### A-9 同日多条事件标签互不重叠（分层避让）【pass】
- 依据（复验，**fail → pass**）：上轮 fail 现象为「row 93 的 6 个标签 top 恒 46px、两两重叠 11 对；row 971 的 4 个标签重叠 3 对；30 天窗 17 对」。本轮实测：14 天窗 row 93 的 6 个标签分层到 **4 个不同 top**（513 / 557 / 601 / 645）、**两两重叠 0 对**；row 971 的 4 个标签分 3 层、重叠 0 对；30 天窗 row 93 的 11 个标签分 4 层、重叠 0 对；行高随层数增长（无事件行 70px / 1 层 90px / 3 层 178px / 4 层 222px）。截图 `1440-events-14d-labels-layered.png`（1440×900）

### A-10 标签完整显示标题、不出省略号残句【pass】
- 依据（复验）：11 条标签的 `.kev-label-text` 全部 `white-space: nowrap` 且 `text-overflow: clip`，无截断；实测如「GPT-6.2 Astra与Sol双模型发布，分层定位专业与日常」全文可见，且与相邻标签互不压字（A-9 已修）。截图 `1440-events-inside-selection-stays.png`（1440×900）

### A-11 1440×900 与 1920×1080 无页面级横向滚动、列与行头对齐【pass】
- 依据（复验）：三档 `documentElement.scrollWidth === clientWidth`（1440=1440、1920=1920、2560=2560）；日期轴头与格位逐列对齐（头 x=482/546/610/674，格 x=482/546/610/674，逐列差值全 0），行头右沿 482 = 首列左沿 482。截图 `1920-events-14d.png`（1920×1080）

### A-12 2560 宽屏 14 天列宽均分填满、不右侧留白【pass】
- 依据（复验，**fail → pass**）：上轮 fail 现象为「首屏进入实测 `--kev-col=64px`、舞台 1917px 宽而 14 列只占 896px、右侧约 830px 留白」。本轮实测**首屏进入**（直接开 `/tags/1980?view=events`，不切矩阵再切回）：`--kev-col = 123.214px`，恰为公式值 (1917 − 192) / 14 = 123.21px（任务书写的「≈169px」与同句给出的公式不符，以公式与「均分填满、无大留白」为准），舞台 scrollWidth = clientWidth = 1917（14 列 × 123.21 = 1725 + 行头 192 = 1917，**右侧零留白**）；1920 档同口径 `--kev-col=77.5px`、1277=1277。截图 `2560-events-14d-firstscreen.png`（2560×1271）

### A-13 左键拖拽平移与 5px 阈值【pass】
- 依据（复验）：舞台空白处 pointerdown 后右移 60px，scrollLeft 由 300 变 240（按位移平移、列宽不变 64px）；同点位移 2px（<5px）时 scrollLeft 保持 300 不动（视为点选不误伤节点/标签）。截图 `1440-events-14d-labels-layered.png`（1440×900）

### A-14 事件卡就地展开关联文章【pass】
- 依据（复验）：事件 829（GPT-6.2 Astra与Sol双模型发布）点卡头就地展开摘要全文，「相关文章 · 5 篇」入口可就地展开 5 条文章行（`lane-chronicle-article-178945/178947/179203/179282/179435`，各带标题 + 日期）。截图 `1440-article-preview-modal.png`（1440×900）

### A-15 文章预览闭合回原卡、卡片保持展开、滚动位置不变【pass】
- 依据（复验）：点文章行后 `ArticlePreviewModal` 打开（标题「文章预览」，正文「V2EX - 技术 / GPT-6.2 Astra & Sol 发布了 / 2026年11月18日」）；Esc 关闭后弹层消失、`.console-stage` scrollTop 前后同为 435（未跳回顶部）、事件卡 829 仍带 `is-open`、卡片头 aria-expanded=true、文章列表仍展开（aria-expanded=true、5 条仍在）、仍停在关键事件视图。截图 `1440-article-preview-modal.png`（1440×900）

### A-16 无关联文章时不显示入口【未覆盖】
- 依据（复验，状态不变）：实机无该数据样本——库内 8 个版块全部 confirmed 注记都带非空 `article_ids_snapshot`（board 1980 28/28 条、2165 8、2128 3、3030 11、2272 10、15220 5、2197 4、1974 5），无法造出 0 篇样本；该分支由组件用例 `front/app/features/tags/components/console/KeyEventDayPanel.test.ts` 覆盖（有 articleIds 显示入口、无 articleIds 不显示）

### A-17 空态照常渲染行与日期轴并给出文案【pass】
- 依据（复验）：版块 2242（6 条持久化话题、0 条 confirmed）进入事件视图：日期轴 14 列照常渲染、2 条活跃行 + 「休眠泳道」折叠组照常渲染、提示「这段时间没有确认的关键事件 · 在日报阅读台标记」仍在，未用空白冒充有内容。截图 `1440-empty-board-2242.png`（1440×900）

### A-18 读取失败显示错误条与重试、不冒称 0 条【pass】
- 依据（复验）：用 agent-browser 网络拦截 abort 掉 `**/key-events*` 后重进：显示「关键事件读取失败（Failed to fetch）」+「重试」按钮，未出现「这段时间没有确认的关键事件」空态文案；解除拦截后点「重试」→ 错误条消失、节点与标签恢复为 11/11。截图 `1440-events-error-retry.png`（1440×900）
- 依据（复验·第四轮）：`network route "**/key-events*" --abort` 后直开 `/tags/1980?view=events` → 实测 `.kev-error` 文案 = 「关键事件读取失败（Failed to fetch）」+「重试」按钮在；页面全文 **不含**「这段时间没有确认的关键事件」（`hasEmptyClaim = false`）、节点 0 / 行 0（空态未被冒称），错误条与空态文案互斥未混。截图 `1440-events-error-retry-rv4.png`（1440×900）

### A-19 窄屏不渲染本视图与切换按钮【pass】
- 依据（复验）：视口 700×900 时 `console-view-events` 按钮不存在、`key-event-view` 不存在，页面回落 `board-narrow-summary`（版块摘要），页面级 scrollWidth=clientWidth=700。截图 `700-narrow-summary.png`（700×900）
- 依据（复验·第四轮）：视口 700×900 直开 `/tags/1980?view=events` → 实测 `board-narrow-summary` 在、`key-event-view` 不在、`console-view-events` 按钮不渲染（窄屏仍不提供切到事件视图的入口）；A-8 修复（窄屏回落）后阅读槽 `.stage-chronicle` **仍在**且有内容。截图 `700-narrow-events-deeplink-fallback.png`（700×900）

### A-20 长窗收起判据改为「堆叠层数上限」而非「观察窗天数」【pass】
- 依据（复验，**旧断言作废后按新口径判定**）：上一轮本条断言的是旧口径「切近 1 年后标签数=0、只剩 ★ 计数」，该口径已被 design D7 / tasks 4.8 取代（tasks 4.6 的「天数阈值」已作废），本轮按新口径判：切「近 1 年」（365 天轴、列宽 64px）后 6 行**全部照常显示事件名称**（row 93 = 16 条标签 / 222px、row 971 = 4 条 / 178px、row 90 = 3 条 / 134px、row 89 与 94 各 2 条 / 90px），★ 计数节点 0 个、空标签 0 个；与上轮同一 365 天窗「标签数 0、只剩 ★1/★2」的旧行为相反。截图 `1440-events-55d-window.png`（1440×900，55 天窗同形态）
- 依据（复验·第四轮）：切「近 1 年」后实测 365 天轴（2025-11-23 ~ 2026-11-22）、6 行逐行标签数 = 3 / 16 / 0 / 4 / 2 / 2（合计 27）、行高 134 / 222 / 70 / 178 / 90 / 90px；**★ 计数节点 0 个、空标签 0 个**、`.kev-label-text` 无一个 `text-overflow` 非 `clip`。R-4 修复把请求窗口从 365 改 366 后，长窗的名称展示与行序未受影响。截图 `1440-events-365d-names-shown-rv4.png`（1440×900）

### A-21 只呈现 confirmed、suggested/dismissed 不进入视图【pass】
- 依据（复验）：版块级接口 `GET /semantic-boards/1980/key-events?days=31` 实返 23 条，`state` 全部为 confirmed（库内该版块另有 137 条 suggested、3 条 dismissed，实测节点/标签总数 22/22 与「confirmed 且属持久化话题」条数完全吻合），suggested/dismissed 一条未出现。截图 `1440-events-14d-labels-layered.png`（1440×900）

### A-22 无事件话题行只渲染说明、不落节点、不伪造 0【pass】
- 依据（复验）：topic 88 / 89（14 天窗内）行头显示「无确认事件」，行内 0 节点、0 标签，未落节点也未显示数字 0 格；55 天窗内 topic 88 同样 0 节点。截图 `1440-events-14d-labels-layered.png`（1440×900）
- 依据（复验·第四轮）：14 天窗实测 row-88 / row-89 / row-90 行头 = 「无确认事件」且各 0 节点 / 0 标签；365 天窗实测 row-88 仍 0 节点 / 行高 70px（基础行高），未落伪 0 格。截图 `1440-events-365d-names-shown-rv4.png`（1440×900）

### A-23 行内虚线轴（生命线）可见【pass】
- 依据（复验，**fail → pass**）：上轮 fail 现象为「6 行 `.kev-axis-line` 实测宽 0px、高 1px，虚线轴在三档视口与空态板均不可见」。本轮实测：三档视口 6 行的 `.kev-axis-line` 宽分别 **896px / 1085px / 1725px**（= 行体显式宽 = 列宽 × 可见日数），高 1px、`border-top: 1px dashed var(--color-border-medium)`、visibility visible；空态版块 2242 的 2 条行同样 896px；截图放大可见横贯整条轴的虚线（节点骑在轴线上）。截图 `1440-events-14d-labels-layered.png`（1440×900）、`1440-empty-board-2242.png`（1440×900）
- 依据（复验·第四轮）：14 天窗 6 行 `.kev-axis-line` 实测宽均 **896px**、高 1px、`border-top-style: dashed`、`visibility: visible`（R-3 只改了卡片标题样式块，未波及轴线）。截图 `1440-events-14d-labels-layered.png`（1440×900）

### A-24 双主题逐项对比度：节点 / 标签底色 / 选中列 / 文章列表 / 预览层【pass】
- 依据（复验）：暗色（`data-theme="dark"`）实测对比度：节点文字/底 5.36、标签文字/底 6.91、选中列底/行头文字 7.28、文章列表行标题 14.19、预览层主标题 14.19 / 正文 14.19，均过 WCAG AA（正文 4.5、图形 3.0）；亮色同项为节点 3.68、标签 6.07（节点底/标签底属图形类，阈值 3.0）。截图 `1440-dark-events-14d.png`（暗色整页）、`1440-dark-article-preview.png`（暗色预览层）、`1440-article-preview-modal.png`（亮色预览层）

### A-25 次级文字（--color-text-muted）对比度【fail】
- 依据（复验，**保持 fail、本 change 不修**）：日期轴日期（0.66rem）、行头事件计数（0.64rem）、事件卡话题快照、空态提示实测亮色 3.04~3.23、暗色 3.15~3.21，仍低于小字号正文 4.5:1；归属判定复测：这些颜色取自产品级令牌 `--color-text-muted`（亮色 #8a8a8a / 暗色 #ffffff59），同页**矩阵视图**的面包屑「主页/叙事工坊」、右轨标题与计数、视窗字段「起始日/—」、背景曲线坐标轴刻度实测同为 3.04~3.23 → 属设计系统既有口径、**非本 change 引入**，本 change 不改、留待单开 change 走令牌层整改（连带动线与边框口径）。另记：标签边框与虚线轴用 `--color-border-medium`（亮 #1a1a1a @ 10% / 暗 #ffffff @ 10%），对比度约 1.33~1.36，轮廓偏淡（装饰性元素，文字本身可读）。截图 `1440-dark-events-14d.png`（1440×900）

### A-26 三档视口截图齐备且视口尺寸与声明一致【pass】
- 依据（复验）：`1920-events-14d.png` 实测 1920×1080、`2560-events-14d-firstscreen.png` 实测 2560×1271、`1440-events-14d-labels-layered.png` 实测 1440×900（PNG IHDR 实读），三档均已存档并被本报告条目点名。截图 `1920-events-14d.png`（1920×1080）

### A-27 与批准原型的差异说明已落账【pass】
- 依据（复验）：差异清单见本文件「与批准原型的差异说明」节（原型按用户 2026-11-21 口径只作功能布局与骨架参考，不是 1:1 像素基线）；本轮据 2026-11-23 用户实机反馈修订后的差异（长窗收起判据、右轨联动）已并入该节；信息架构、主流程、布局模式（workspace）均未偏离，故 `ui-approval` 不重置。截图 `1440-events-14d-labels-layered.png`（1440×900）

### A-28 长窗（观察窗 > 30 日）仍看得见事件名称【pass】
- 依据（复验，**本轮新行为首验**）：构造 55 天观察窗 `?view=events&days=30&start=2026-09-29&end=2026-11-22`（轴实测 55 天，2026-09-29 ~ 2026-11-22）：6 行中 5 行照常显示事件名称标签（row 90 = 3 条 / 2 层 / 134px、row 93 = 16 条 / 4 层 / 222px、row 971 = 4 条 / 3 层 / 178px、row 89 = 2 条 / 1 层 / 90px、row 94 = 2 条 / 1 层 / 90px），**★ 计数节点 0 个、标签重叠 0 对**，稀疏行行高 90px；未出现「整屏只有光点和 ★」。截图 `1440-events-55d-window.png`（1440×900）
- 依据（复验·第四轮）：重跑同一 55 天窗（`?view=events&days=30&start=2026-09-29&end=2026-11-22`）：轴实测 **55 列**（首 09/29、末 11/22）、6 行标签数 3 / 16 / 0 / 4 / 2 / 2（合计 27）、行高 134 / 222 / 70 / 178 / 90 / 90px，**★ 计数节点 0 个、空标签 0 个**，长窗仍看得见事件名称。截图 `1440-events-55d-window-rv4.png`（1440×900）

### A-29 超长窗不得「整视图连坐收起」【pass】
- 依据（复验，**本轮新行为首验**）：与 A-28 同口径的反面验证——365 天轴（近 1 年）下 6 行**逐行**照常显示名称（16 / 4 / 3 / 2 / 2 条标签 + 1 条无事件行），无一行退化为纯节点，也无整视图连坐收起；30 天 / 55 天 / 365 天三档均无「★N 计数」节点。截图 `1440-events-55d-window.png`（1440×900）
- 依据（复验·第四轮）：365 天档（近 1 年）实测 6 行**逐行**照常显示名称（16 / 4 / 3 / 2 / 2 + 1 条无事件行），**无一行退化为纯节点**，全页 `★` 计数节点 **0** 个；55 天档同样 0 个（两档均未因“天数长”而连坐收起）。截图 `1440-events-365d-names-shown-rv4.png`（1440×900）

### A-30 层数超限时「只收该行」退化为节点 + 当日事件计数【未覆盖】
- 依据（复验，**本轮新行为首验 · 真实数据不可达**）：单行退化需该行标签层数 > 4（上限 4 层）；实测全库最密的两个行（1980/93 共 16 条、2272/1615 共 7 条含 11-19 同日 3 条）在 7 / 14 / 30 / 55 / 365 天五档窗口下层数最多均为 **4 层**（未越限），全库无「同一话题 5 日内 ≥5 条」的样本，故「节点 + ★N」退化分支本轮实机不可触发；已实测的反面（不得整视图连坐收起、不得按天数一刀切）见 A-28 / A-29 均为 pass；该退化分支由组件用例 `front/app/features/tags/components/console/KeyEventTimelineView.test.ts`（层数超上限 → 只渲染节点 + ★计数）覆盖。

### A-31 右轨「话题追踪」条目选中 → 切回矩阵 / 追踪并高亮滚入【pass】
- 依据（复验，**本轮新行为首验**）：版块 3030（该版块 watch #10「Agent工具相关」映射到泳道 1804，1980 的两条 watch 均无对应泳道故不渲染为可点条目）进入「关键事件」视图 → 点右轨 `rail-watch-locate-10` → 主舞台**切回矩阵 / 追踪**（`key-event-view` 消失、`board-matrix` 出现、`console-view-matrix` aria-pressed=true）、目标泳道 `matrix-lane-1804` 带 `is-selected`、矩形 top=453 / bottom=474 落在 900 高视口内（已滚入视野）。截图 `1440-rail-watch-locate-back-to-matrix.png`（1440×900）
- 依据（复验·第四轮）：`/tags/3030?view=events`（事件视图在、矩阵不在）→ 点右轨 `rail-watch-locate-10` → 实测**切回矩阵**：`key-event-view` 消失、`.omx-matrix` 在、`console-view-matrix` 带 `is-on`（「关键事件」不再 pressed）；目标泳道 `matrix-lane-1804` 带 `is-selected`，矩形 top=452.94 / bottom=473.58 → 落在 900 高视口内（已滚入视野）。R-1 的「仅在 events 视图切视图」条件未影响本路径（events 视图内仍切）。截图 `1440-rail-watch-locate-back-to-matrix-rv4.png`（1440×900）

### A-32 右轨「当日其他话题」条目选中 → 切回矩阵并出现该分区阅读槽【pass】
- 依据（复验，**本轮新行为首验**）：版块 1980 与 3030 均实测——关键事件视图内点右轨「当日其他话题」任一条（1980：`rail-other-topic-item-9148`；3030：`rail-other-topic-item-9184`）→ 主舞台切回矩阵 / 追踪（`console-view-matrix` aria-pressed=true），主舞台下方出现「当日其他话题 · 2026-11-22 / Inkling 2 Mini 开放权重次日，llama.cpp 与自研引擎 GPU 实测对比」分区阅读槽；该分区不是泳道，矩阵内无泳道高亮（符合既有语义，非缺陷）。截图 `1440-rail-other-topic-back-to-matrix.png`（1440×900）
- 依据（复验·第四轮）：`/tags/1980?view=events`（`key-event-view` 在、矩阵不在）→ 点右轨「当日其他话题」首条（**必须点内层 `button.rob-row-btn`，点 `<li>` 不触发**）→ 实测**切回矩阵**：`key-event-view` 消失、`.omx-matrix` 在、`console-view-matrix` 带 `is-on`；下方分区阅读槽在（`other-topic-reading` 与 `other-reading-threads` 均在），URL = `/tags/1980`（无 `view`、无 `lane`）。R-1 的「仅在 events 视图切视图」条件未影响本路径。截图 `1440-rail-other-topic-back-to-matrix-rv4.png`（1440×900）

### A-33 视图内选中（点行头 / 点事件节点）不被踢出时间轴【pass】
- 依据（复验，**本轮新行为首验**）：关键事件视图内点行头（`key-event-row-93 .kev-row-head`）→ 仍在 events 视图（`key-event-view` 存在、`console-view-events` aria-pressed=true）、行头转 `is-selected`、URL 写 `?view=events&lane=93`；点事件节点 `key-event-node-93-2026-11-18` → 仍在 events 视图、选中日 11-18 整列高亮、下方阅读区出当天 3 条事件。截图 `1440-events-inside-selection-stays.png`（1440×900）

### A-34 右轨选中后 URL 的 view 参数与实际视图不一致【pass】
- 依据（复验，**fail → pass**）：上轮 fail 现象为「右轨『话题追踪』选中后画面已切矩阵，但 URL 仍 `?view=events&lane=1804`，刷新该 URL → 回到关键事件视图（`key-event-view` 存在、矩阵不存在），违反 design D6」。本轮实测（第三轮·终验，session `kev-final-1795424275`，1440×900）：靶子版块 3030 → `/tags/3030?view=events`（切档按钮 `console-view-events` 带 `is-on`、`key-event-view` 在、`.omx-matrix` 不在）→ 点右轨 `rail-watch-locate-10`（title「定位到泳道『Agent工具相关』」）后：**URL = `/tags/3030?lane=1804`**（`location.search = "?lane=1804"`，**实测无 `view=events` 残留**）；画面为矩阵（`console-view-matrix` 带 `is-on`、`.omx-matrix` 在 845×209、`key-event-view` 已卸载）；泳道 1804 带 `is-selected`（`[data-testid="matrix-lane-1804"]` class = `omx-lane-name-btn is-selected`，矩形 top=452.94 / bottom=473.58，落在 900 高视口内 → 已滚入视野）。**刷新该 URL**（同 query 重新导航）后**仍是矩阵视图**（`console-view-matrix` 带 `is-on`、`.omx-matrix` 在、`key-event-view` 不在、泳道 1804 仍 `is-selected`），未回到关键事件视图。**同类路径**（右轨「当日其他话题」）：`/tags/1980?view=events` 点 `rail-other-topic-item-9148` 后 URL = **`/tags/1980`**（`location.search` 为空串，无 `view`、无 `lane`），画面为矩阵（`console-view-matrix` 带 `is-on`、`.omx-matrix` 在 845×402），下方出现该分区阅读槽（`other-reading-threads` 在，含文章标题）。截图 `1440-rail-watch-locate-url-consistent.png`（右轨追踪选中后矩阵 + URL 干净）、`1440-rail-other-topic-url-clean.png`（当日其他话题后 URL 干净 + 分区阅读槽）（均 1440×900）
- 依据（复验·第四轮）：R-1 修复后重测两条右轨路径，URL 与画面仍一致：① `/tags/3030?view=events` 点 `rail-watch-locate-10` → URL = **`/tags/3030?lane=1804`**（`location.search = "?lane=1804"`，**无 `view=events` 残留**），画面为矩阵、泳道 1804 `is-selected`；② `/tags/1980?view=events` 点 `rail-other-topic-item-9148` 的内层按钮 → URL = **`/tags/1980`**（`location.search` 为空串，无 `view`、无 `lane`），画面为矩阵且分区阅读槽在。R-1 把切视图收窄到「仅 events 视图」后，**URL 清理动作没有回退**（仍走同一 `batchQueryWrite` 合并写入）。**刷新复核**：刷新 `/tags/3030?lane=1804` 后仍是矩阵视图（`console-view-matrix` 带 `is-on`、`key-event-view` 不在、泳道 1804 仍 `is-selected`），未回落事件视图。截图 `1440-rail-watch-locate-back-to-matrix-rv4.png`（1440×900）、`1440-rail-other-topic-back-to-matrix-rv4.png`（1440×900）

### A-35 关键事件读端请求窗口覆盖可见轴起点（去 365 封顶）【pass】
- 依据（复验·第四轮，R-4 修复首验）：页面内 hook `window.fetch` 捕获 `GET /api/semantic-boards/1980/key-events?days=N`：切「近 1 年」（365 档）后实测请求为 **`?days=366`**（> 365，修前被 `Math.min(365, …)` 封顶）。轴实测 **365 列**（首 11/23 = 2025-11-23、末 11/22 = 2026-11-22；anchor = 该版块最新 completed 日报日 2026-11-22，today = 2026-11-23 → 轴起点到今天跨 366 天）。**无噪声**：365 档视图节点 **27** / 标签 **27**，与 DB 真值逐行吻合（`semantic_board_id=1980 AND state='confirmed'` 共 28 条 − 1 条 candidate 话题 2477 = 27；逐行 89=2 / 90=3 / 93=16 / 94=2 / 971=4 = 27）；`days=365` 与 `days=366` 两次接口返回条数相同（均 28 条，含 candidate），即多要的那一天**不引入噪声**。**回归**：30 档请求 `days=31`、14 档 `days=15`（= 轴起点到今天的跨度，非无脑变大）；自定义 55 天窗（轴 2026-09-29 ~ 2026-11-22）请求 `days=56`（= 09-29 → today 11-23），同样不封顶；14 档节点 11 / 标签 11 与上轮一致。截图 `1440-events-365d-request-days-366.png`（1440×900）、`1440-events-365d-names-shown-rv4.png`（1440×900）

### A-36 归档视图内右轨选中不再被静默切走视图【pass】
- 依据（复验·第四轮，R-1 修复首验）：① `/tags/3030?view=archive`（版块 3030 的 watch #10「Agent工具相关」映射泳道 1804）→ 点右轨 `rail-watch-locate-10` → 实测**仍在归档视图**：`console-view-archive` 带 `is-on`（「矩阵 / 追踪」「关键事件」均不 pressed）、`.archive` 列表仍在、`.omx-matrix` 与 `key-event-view` **都不在**；URL = **`/tags/3030?view=archive&lane=1804`**（`view=archive` 保留，泳道选择照旧记录 = 旧行为），**归档返回滚动位未被消费**（`.console-stage` scrollTop 前后同为 0、`.archive` scrollTop 前后同为 0）。② `/tags/1980?view=archive` → 点右轨「当日其他话题」首条（内层按钮）→ 同样**留在归档视图**，URL 仍是 `?view=archive`。对照：同一条目在 `?view=events` 下**仍切回矩阵**（A-31 / A-32 复验），在矩阵视图下为 no-op（视图与 URL 均不变）——即修复只把「切视图」收窄到 events 视图，未改其它视图的旧行为。截图 `1440-archive-rail-watch-stays-archive.png`（1440×900）

### A-37 当日阅读区事件标题换行完整可读（无省略号截断）【pass】
- 依据（复验·第四轮，R-3 修复首验）：`/tags/1980?view=events` → 30 天档 → 点 `key-event-node-90-2026-10-31` → 阅读区 4 张卡（`.kdp-title` = 「10/31 关键事件」）。`.kdp-card-title` computed style 实测：**`text-overflow: clip`**（非 `ellipsis`）、**`white-space: normal`**、`overflow-wrap: anywhere`、`overflow: visible`；4 条标题**均不含「…」**、`scrollWidth === clientWidth`（未裁切）。长标题「DeepSeek 同日发布新 Agent 架构并开源 Personal Agent 思路」（44 字）在 1440 / 1024 视口下 1 行显示（clientW 325 / 362–386px），视口收至 **800×900** 时该标题实测 **换行 2 行**（`Range.getClientRects()` 行数 = 2、元素高 40px = 2 × 19.84px 行高），全文可见、无残句。截图 `800-events-daypanel-title-wrapped.png`（800×900）、`1024-events-daypanel-title-wrap.png`（1024×900）

### A-38 刷新失败时 stale 文案点名数据所属版块【pass】
- 依据（复验·第四轮，R-7 修复首验）：`/tags/1980?view=events` 已有 14 天数据后 `network route "**/key-events*" --abort`，切「30 天」触发请求失败 → 顶部 stale 条实测文案 **「事件刷新失败（Failed to fetch），当前显示的是版块「生成式 AI 与大模型厂商」之前的数据」**（**点名版块名**，不再是只说「之前的数据」）；旧内容保留（11 节点）且不冒称 0 条。**跨版块变体（「（不是当前版块的数据）」）本轮实机不可达**：切版块（`board-rail-item-3030`）走 `router.push('/tags/3030')` 会连 query 一起丢弃（URL 变 `/tags/3030`、视图回落矩阵），且事件视图组件随版块重挂载 → 无「旧版块内容」可保留，实测呈现的是全量错误态「关键事件读取失败（Failed to fetch）」；该分支由 `useBoardKeyEvents.test.ts` / `KeyEventTimelineView.test.ts` 组件用例覆盖。截图 `1440-events-stale-names-board.png`（1440×900）

### A-39 窄屏 + `?view=events` 深链回落矩阵、阅读槽仍在、URL 清 view【pass】
- 依据（复验·第四轮，R-8 修复首验）：视口 700×900 直开 `/tags/1980?view=events` → 实测落 **窄屏摘要形态**（`board-narrow-summary` 在、`key-event-view` 不在、「关键事件」按钮 `console-view-events` 不渲染）、**阅读槽 `.stage-chronicle` 在且有内容**（「当日线索·Agent 架构重构与标准化协作生态 2026-11-22…」）、seg 组**有按下项**（「矩阵 / 追踪」`aria-pressed=true` / `is-on`）、**URL 的 `view` 已被清掉**（`location.search = ""`、URL = `/tags/1980`）。即不再出现「空舞台 + 无按下项 + URL 仍写着 view=events」的卡死形态。截图 `700-narrow-events-deeplink-fallback.png`（700×900）

## 与批准原型的差异说明

原型 `ui-prototype/index.html` 只作功能布局与骨架参考（用户 2026-11-21 明确「不是 1:1 像素级实现基线」），实现以 ui-design.md 合同与 delta spec 为准。实装与原型的差异如下（均按合同口径落地，未改合同）：

1. **日期轴**：原型是「月份行 + 日期行」两级轴；实现只有日期行（MM/DD），无月份行。
2. **行头宽度与候选徽标**：原型行头 196px 且给候选话题画「候选」chip；实现行头 192px，候选话题按 spec 直接不渲染（无 chip）。
3. **事件标签内容**：原型标签为「日期 + 标题」；实现标签为「圆点 + 标题」（日期在轴上、tooltip 里）。
4. **节点形态**：原型按日聚合成一个空心大圆点；实现是「每条确认事件一个节点按钮」，同日多条沿轴横向散开 ±6px。
5. **行头计数文案**：原型「N 条确认 · 最近 X」；实现「N 事件 · 最近 X」；无事件时两边都写「无确认事件」。
6. **当日阅读区标题**：原型「2026-11-18 + N 条确认事件 + 跨 N 个话题」；实现「11/18 关键事件 + N 条」（话题数改为逐卡显示快照标签）。
7. **空态文案**：原型「这个跨度内没有确认的关键事件。最近一条在 10-15…」；实现「这段时间没有确认的关键事件 · 在日报阅读台标记」（按 ui-design State Matrix 口径）。
8. **关联文章入口文案**：原型按钮是「关联文章 N 篇」且展开后变「收起文章」；实现复用既有 `LaneRelatedArticles`，按钮文案为「相关文章 · N 篇」、展开后只换箭头图标不换文案（合同里的「关联文章 N 篇」措辞未逐字落地，展开行为一致）。
9. **额外控件**：实现多一个「所选日居中」按钮（原型没有）——用于 30 天宽轴下手动把选中日拉回视野（A-8 自动滚入在「轴变化」路径上仍失效，需该按钮兜底）。
10. **主题切换**：原型自带「深色」按钮；实现的主题开关在应用头部（月亮图标），本视图本身不提供主题入口。
11. **网格实现**：原型用 CSS grid 逐列铺排；实现是轴头 flex + 单元格绝对定位，列宽取 `max(64px, 可用宽/可见日数)`，超宽屏均分、窄屏守 64px 后由舞台容器横滚承接（行头 sticky）。
12. **生命线可见性**：原型的行内轨道线（实线/虚线）可见；上轮实装的虚线轴因 `.kev-row-body` 宽度为 0 而不可见（见 A-23），**本轮已修**（行体显式宽 = 轴宽），故此条不再是差异项。
13. **长窗降级判据（2026-11-23 用户反馈修订）**：原型与上轮实装按「可见日数 > 31」整体收成光点 + ★ 计数；本轮按 design D7 改为「堆叠层数 > 4」才单行退化——长窗（55 天 / 365 天）照常显示事件名称，稀疏行行高约 90px（原型无长窗形态，此为合同修订后的实装行为）。
14. **右轨联动（2026-11-23 用户反馈修订）**：原型无「右轨选中 → 切视图」联动；本轮按 design D14 实装——右轨发起的话题选中一律切回矩阵 / 追踪并把泳道滚入视野 + 高亮，本视图内部的选中仍留在本视图（原型右轨条目为纯展示）。

## 缺陷清单（按严重度）

1. **~~P1 切档（窗口/轴变化）后选中日仍不自动滚入视口~~（A-8）——已修 + 复验 pass**：上轮根因是切档走骨架态把 `.kev-scroll` 整块卸载重建、滚动位置随旧容器丢失（watch 目标 1657.5 被容器重建吞掉）。实现线程修法：滚动容器函数 ref `setScrollRef` 采纳**新元素**时（`observedSurface !== node`）排一次 `nextTick` → 补一次 `scrollSelectedDateIntoView()`（骨架视觉未改）。本轮终验实测：切 30 天后 `scrollLeft` 落定 1267（`scrollTo` 目标 1657.5，浏览器按可达上限 1267 钳位），选中日 11-22 列 `left=1070.59 / right=1134.59` 在舞台 `right=1134.41` 内（`selectedInStage=true`）；冷启动 `?view=events&days=30` 同值；点节点换日仍为 890。截图 `1440-events-30d-selected-in-view.png`、`1440-events-30d-node-date-scrolled.png`。
2. **~~P2 右轨选中后 URL 的 view 参数与实际视图不一致~~（A-34）——已修 + 复验 pass**：上轮根因是 `locateLaneFromRail` 里 `handleSetView('matrix')` 与 `selectLaneFromStage` 两次 `router.replace` 竞争，后者用未更新的 `route.query` 把 `view=events` 写回。实现线程修法：`useBoardConsole` 新增 `batchQueryWrite(fn)` + `applyQueryPatch`，把「切矩阵 + 选泳道」合并成**一次** replace。本轮终验实测：点 `rail-watch-locate-10` 后 URL = `/tags/3030?lane=1804`（无 `view`），画面为矩阵、泳道 1804 `is-selected` 且在视口内；刷新同 URL 仍是矩阵视图；「当日其他话题」同类路径 URL = `/tags/1980`（无 `view`、无 `lane`）。截图 `1440-rail-watch-locate-url-consistent.png`、`1440-rail-other-topic-url-clean.png`。
3. **P3 次级文字对比度不足（3.04~3.23，小字号 AA 未达）与标签边框/虚线轴轮廓偏淡（1.33~1.36）**：均源自产品级令牌（`--color-text-muted` / `--color-border-medium`），矩阵视图同款元素（面包屑、右轨标题、视窗字段、坐标轴刻度）实测同为 3.04~3.23 → 属设计系统既有口径、**非本 change 引入**；本 change 不改，留待单开 change 走令牌层整改。截图 `1440-dark-events-14d.png`。
4. **P3（不可达，非缺陷）「层数超限单行退化」分支实机无法验证**：全库最密行层数上限 4，未越限 → 本轮只能验证反面（不得整视图连坐收起，已 pass），退化分支无实机样本（见 A-30）。
5. **P3 结构性残留风险（R-9；主线程 2026-11-23 已驳回、本 change 不改、亦不计入 A 条目判定）**：`useObservationWindow.writeQuery` 仍是与 `useBoardConsole.batchQueryWrite` **不同源**的独立 query writer。第四轮用脚本在**同一动作**内触发「切视图 + 切档」（同步连点 `console-view-events` 与「30 天」档 chip，或两者间隔 `setTimeout(0)`）**可复现**：URL 落为 **`?days=30`**——`setView('events')` 写入的 `view=events` 被观察窗那次 `router.replace({query:{...route.query}})`（旧快照）抹掉，而**画面仍在关键事件视图**（`key-event-view` 在、「关键事件」pressed）→ **URL 与画面不一致**；刷新该 URL 回落矩阵视图。间隔拉到 **200ms** 后**不复现**（URL = `?view=events&days=30`，画面与 URL 一致），即竞态窗口 < 200ms，**人手不可复现**（与 review R-9 的判定一致）。仅作结构性风险记录，本 change 不动。
6. **观察提示（非缺陷）**：右轨「当日其他话题」的 `data-testid="rail-other-topic-item-<sectionId>"` 挂在 `<li>` 上，可点的是内层 `button.rob-row-btn`；`li.click()` 不会触发（事件不向下冒到子按钮），测该路径必须点内层按钮。本轮前几次误判「右轨当日其他话题点击无效」即由此而来（A-32 复验已给出正确复现步骤）。

## 附：规格措辞修订（本轮为过门禁所做的 delta spec 文字改动）

对象：`openspec/changes/add-key-event-matrix-view/specs/workshop-board-console/spec.md` → ADDED Requirement「关键事件标签布局与横向滚动」。改动只涉及**正文措辞**（场景与行为条款一字未动），原因：`openspec validate --strict` 对「requirement 正文 > 500 字」报 WARNING 并以退出码 1 判红（该 requirement 正文被 2026-11-23 的 D7 修订加长到 527 字）。两处改动：

1. `行内列宽 SHALL 取舒适固定值` → `行内列宽 SHALL 均分可用宽度且不低于下限（64px）`。
   依据：design D11 与本 requirement 自带的 Scenario「列宽均分填满」（每列 SHALL 均分可用宽度）都规定**均分**，原文「舒适固定值」与二者自相矛盾，且与实装（`max(64, 可用宽/可见日数)`）不符。此处是把正文对齐到**已定的设计决策与场景**，不是放宽条款。
2. D7 句压缩：`事件标签的堆叠层数未超过上限（行高上限）时 SHALL 照常显示事件名称；仅当堆叠层数超过上限时 SHALL 收起文字标签、只保留节点与当日事件计数，名称改由选中日阅读区承担。收起与否 MUST NOT 仅由观察窗天数决定——长窗（观察窗超过 30 日）MUST NOT 整体收成光点，MUST NOT 因列多而让用户看不到任何事件名称。` → `标签堆叠层数未超上限时 SHALL 照常显示事件名称；超上限时 SHALL 收起文字标签、只保留节点与当日事件计数，名称改由选中日阅读区承担。收起与否 MUST NOT 仅由观察窗天数决定：长窗 MUST NOT 整体收成光点致用户看不到任何事件名称。`（另把「写入与矩阵相同的选中日」缩为「写入相同的选中日」，语义不变）。
   依据：全部 SHALL / MUST 语义与「仅层数超限才退化」「不得因天数长整体收成光点」两条红线逐条保留；Scenario「层数超限才收起标签」「长窗仍看得见事件名称」原文未动（完整细节在场景里）。

改后该 requirement 正文 493 字（≤ 500），`openspec validate add-key-event-matrix-view --strict` → 通过（退出码 0）。
