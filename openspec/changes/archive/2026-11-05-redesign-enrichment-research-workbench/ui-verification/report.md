# UI 走查验证报告 — redesign-enrichment-research-workbench

> 走查环境：树莓派本机静态托管 `http://localhost:5100/`（2026-10-16 部署，含 review 修复 R-1/R-2/R-16/R-17 与 loadCandidates 竞态守卫）；agent-browser 会话，editorial/dark 双主题。
> 证据目录：`ui-verification/evidence/`（1440×900 为主视口，另附 1920/375/390 与 dark）。
> 真实数据：板块 1974「中东地缘政治与美伊关系」2026-10 周期 8 条候选；研究样例为用户授权的一次真实成文（§B）。

## A. 场景走查

### A1 板块列表首屏【pass】
`/tags` 打开，叙事工坊 shell（顶栏/语义板块侧栏 12 板块）正常。证据 `A1-tags-editorial-1440-initial.png`。

### A2 研究 tab 工作台入口【pass】
「研究」tab 进入：发现信号按钮、筛选 tab（全部/未研究/可阅读/已忽略）、历史研究（只读）入口齐备，无退役区块（聚焦分析/辩论/新闻背景折叠区）。证据 `A2-research-tab-editorial-1440.png`。

### A3 候选索引主从 split【pass】
板块 1974 → 8 条候选索引 + 详情栏主从（AppPageShell split）。行内状态徽章（待研究/已有报告 报告×1）、上次研究进展（轮次/取数次）正确；空板块（1980）显示「本周期还没有候选信号」空态。证据 `A3-board1974-index-split-1440.png`。

### A4 历史研究（只读）弹层【pass】
「历史研究（只读）」→ AppDialog xl 内嵌旧历史列表：调查 #13/#12、简报 #11、旧版分析 #5/#4/#3、话题分析 #1，四旧 kind 齐全，行带日期与 session 前缀（data_enr）。证据 `A4-legacy-history-dialog-1440.png`、`A4b-legacy-history-open.png`。

### A5 旧版分析只读阅读【pass】
旧版分析 #5 以「旧版 · 只读」reader 呈现；DOM 断言无 深入调查/追问/沉淀/采纳 等写控件，QAPanel 不复用（静态正文）。证据 `A5-legacy-analysis-reader-1440.png`。

### A6 相关话题「背景预览」（只读入口）【pass】
候选详情「相关话题」→ 背景预览按钮 → AppDialog md 弹层：标题带话题名、周/月/年 粒度选择、空态「该周周期暂无新闻背景。」如实标注、无任何生成/写按钮（只读预览）。注：AppDialog 未透传 data-testid 到 DOM，机械锚以视觉+文案断言替代。证据 `A6-related-topic-background-preview-md.png`。

### A7 话题总览 focus → 新闻背景（管理入口）【pass】
话题泳道 → 点泳道行进 focus 视图 → 头部「新闻背景」按钮 → 管理弹层（新闻背景 · 新闻记忆）：粒度切换 + 显式「补生成」按钮（空态不自动生成，符合手动主权约束）。证据 `A7-topic-background-mgmt-lg.png`。

### A8 schema3 报告阅读视图【pass】
候选 07 研究完成后阅读：分源时效行（`eia_wpsr_table1 2026-10-09 · fred_series 2026-10-14 · gdelt_doc 无观测 · jodi_oil_primary 2026-07 · 研究时点 / 报告生成`，无观测如实标注）+ 引用值渲染（114.82→135.51 等）+ 关键观测证据表。证据 `A8-schema3-report-top-1440.png`。

### A9 忽略 → 已忽略视图 → 恢复【pass】
忽略候选 02：visible 索引 8→7；已忽略 tab 服务端重拉返回 1 行（网络断言 `visibility=ignored`）+ 行级「恢复」按钮；详情按钮双态切「恢复」；恢复后全部 tab 回到 8 条。证据 `A9-ignored-view-restore-1440.png`。
（走查中发现并修复 R-17：select-view 监听器曾因处理函数嵌套被 prod 静默丢弃，已忽略 tab 因此从不重拉——见 review.md R-17 与接线锚测试。）

### A10 恢复后工作台全景【pass】
全部 tab 8 条候选、任务态清空、无悬挂按钮。证据 `A10-workbench-restored-1440.png`。

### A11 宽视口 1920×1080【pass】
split 主从保持、无横向溢出（`scrollWidth ≤ innerWidth` 断言 false）。证据 `A11-workbench-1920.png`。

### A12 窄视口 375×667【pass】
单栏降级、无横向溢出（375：`sw=375=iw`）。顶栏内容 tab 竖排换行为 TagsPage 存量样式（非本 change 辖区），功能可达。证据 `A12-workbench-375.png`。

### A13 窄视口 390×844【pass】
单栏降级、无横向溢出（390：`sw=390=iw`）。顶栏内容 tab 竖排换行为 TagsPage 存量样式（非本 change 辖区），功能可达。证据 `A13-workbench-390.png`。

### A14 dark 主题 1440×900【pass】
深色模式下 split 主从/详情区块/只读 chips/强调色渲染完好。证据 `A14-workbench-dark-1440.png`。

### A15 返回任务入口（D6 任务可找回）【未覆盖】
未覆盖原因：运行中正例的 E2E 不重复消耗研究预算，以单测为准。恢复链路（刷新/重进后从 analysis-status 恢复进度 + 任务周期快照）在 useSignalWorkbench 单测覆盖（含 R-16 修复的 job 自带周期传递）；端点接线已在浏览器断言：`/api/enrichment/analysis-status?scope=board&id=1974` → 200 `running=false`（旧实现拼错路径恒 404）。

### A16 忽略与研究互斥（按钮可用性）【pass】
真实研究运行期间：详情 忽略/开始研究 均禁用，title「发现/研究进行中，忽略与新增研究暂不可用」；发现入口同步禁用。（截图存于走查会话记录，互斥逻辑另有 handler 单测锁两个竞争顺序。）

## B. 真实成文样例核对（7.3，用户授权一次）

样本：候选 08「麦加共同防务协议」（3 条切片），走生产入口 `PUT /signals/8/research`，2026-10-16 09:52 触发，全程约 12 分钟（research 13 轮 + compose ~70s）。

| 核对项 | 结果 |
| --- | --- |
| schema_version | **3**（result_id=21） |
| attempts / retries | **1 / 0**（一次成文，零重试） |
| 决策轮次 / stop_reason | 14/40 · finished |
| 块放置合规 | thesis=[paragraph]、facts=[paragraph, evidence_table, chart]、causal=[paragraph]、implication=[paragraph] ✓ |
| 坏块/HTML 注入 | 无（三类平面块之外零出现；正文无标签注入） |
| data_freshness | 分源 per-tool latest + cutoff≠generated_at；`gdelt_doc 无观测` 如实声明 |
| 复制账本数值 | 表 label 零命中（附录 8 条取数 / 5 条计算观测值均未出现在表/正文标签） |
| raw 保持 | appendix calls=8、calculations=5 全量可重建 |
| 图表 | 1 张，挂载 facts 节，单周期合法 |

### A17 宽屏双栏定高内滚（2026-10-16 用户反馈追加）【pass】
用户反馈：左右栏高度基本固定（视口高）各自滚动，免整页来回滚；右栏铺满余宽。落地：≥768px 工作台 `calc(100dvh - 190px)` 定高，左索引/右详情各自 `overflow-y:auto`，`.workbench-candidate` 去 760px 帽铺满主栏。断言：1440×900 shellH=710/左栏内滚/详情 780=780；2560×1215 shellH=1025/详情 1900 铺满/无横向溢出；375×667 高度样式不生效（tags-content 内滚原行为零变化）。证据 `A17-workbench-2560-fixedpanes.png`。

### A19 话题总览点击与相关话题 chip（2026-10-16 用户反馈追加）【pass】
反馈两症状：①话题总览左键点击被拖拽吞掉（泳道进不了聚焦）；②候选详情相关话题 chip 点击无反应。修复后实证：真实指针序列（pointerdown→pointerup→click）点泳道「美伊博弈升级」进入聚焦视图、新闻背景按钮在位；拖拽（>3px 阈值）后 click 仍被正确抑制；研究工作台 chip 点击直开「背景预览 · 美伊博弈升级…」弹层。证据 `A18-related-topic-chip-preview.png`。

## C. 结论

16 项场景 pass 14、未覆盖 E2E 1（单测兜底）、pass（附存量观察）1；真实成文样例九项核对全过。三处走查发现的缺陷（R-16 恢复端点 404、R-17 监听器静默丢失、loadCandidates 竞态）均已修复并以测试/浏览器断言锁死，详见 review.md。
