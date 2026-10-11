# Human Feedback — add-topic-key-event-markers

> **定位**：记录用户在 change 生命周期内对产物（proposal/design/ui-design/原型/实现/文档）的反馈，作为本 change 修订依据；也是 harness-retro `--lifecycle` 体检卡「人类反馈段」的计量输入。人写给人的修订账本，不进 events.db（文件随 change 归档永久保存）。
> **消费方**：① 本 change 修订（条目 → 对应制品增量）；② harness-retro --lifecycle 计量（条数/归因分布/未处置数）；③ 归档对账（archive-readiness 顺带 feedback.sh verify——未处置条目提醒处置，不许静默丢）。
> **格式**（条目级契约，一行正则全解析）：条目标题行 `### FB-<N> <日期>【<类型>|<归因>】<标题>`；条目含状态行 `- **状态**：<状态>`。词表见 `bash scripts/harness/feedback.sh menu`。

## 反馈清单

### FB-1 2026-10-11【产品|frontend】用户不接受列表式关键事件回看，选择在话题泳道长时间轴上高亮关键节点；现有泳道看不清，要求先优化原型的日期定位、标题可读性与点击反馈，不进入产品实现。

- **状态**：已消化
- **原文要点**：用户不接受列表式关键事件回看，选择在话题泳道长时间轴上高亮关键节点；现有泳道看不清，要求先优化原型的日期定位、标题可读性与点击反馈，不进入产品实现。
- **已核实事实**：当前focus列宽96px、标签最大92px且先截断，原型只看关键时还投影为列表，不能表达长时间脉络。
- **影响面**：proposal/UI契约升级major/pending，persistent-topic高亮与可读性规格、设计/用例/任务同步；只修改change制品与静态原型。
- **后续动作**：长轴、同日错行、可读背景、确认/建议分层与就地详情已修订；92条原型断言通过、9张新截图，见ui-prototype/evidence/report.md。新原型等待用户明确确认（major/pending）；本条消化表示已按反馈修订，不代表审批，不进入产品实现。

### FB-2 2026-10-11【产品|frontend】原型须基于现有叙事工坊的版块内泳道优化：先选版块，只展示该版块的话题泳道，不做全局话题汇总；浅深主题必须参考现有main.css Token，不能沿用自配的绿色主题。

- **状态**：已消化
- **原文要点**：原型须基于现有叙事工坊的版块内泳道优化：先选版块，只展示该版块的话题泳道，不做全局话题汇总；浅深主题必须参考现有main.css Token，不能沿用自配的绿色主题。
- **已核实事实**：TagsPage按selectedBoardId挂载BoardThreadBrowser；全部是文章池；lanes/focus是版块内模式。main.css浅色为暖纸/红、深色为深蓝黑/橙，statusFill五态是独立语义，不能被关键色覆盖。已只读观察静态:5100原页面，参考浏览器关闭。
- **影响面**：原型恢复叙事工坊壳、版块侧栏与本版块多泳道/聚焦；canonical主题变量复制，独立关键外环；proposal/spec/design/tasks/test-cases/UI合同同步，不修改产品主题源或业务代码。
- **后续动作**：原型与制品已按FB-2修订；116条原型机械断言通过，9张按版块/双主题的新截图，见ui-prototype/evidence/report.md。明确核对canonical十二项Token、原section字段/状态色/计数、当前owner范围、focus/全部/迟到结果。已关闭参考与验证浏览器；当前仍major/pending，处理完成不代表用户批准，不apply。

### FB-3 2026-10-11【产品|frontend】AI建议只在日报展示并挑选；话题总览不展示每期未确认推荐，只回看已确认注记与原动态背景

- **状态**：已消化
- **原文要点**：AI建议只在日报展示并挑选；话题总览不展示每期未确认推荐，只回看已确认注记与原动态背景
- **已核实事实**：原型nodeLabelTitle/renderAxis/renderEvent直接使用同一state(item)，将suggested样式/理由/动作同时带到总览与日报。既有设计D2的话题读取已定位为confirmed，日报读端负责当期推荐状态；应明确两种宿主投影而不是增加推荐频次控制。
- **影响面**：topic-key-event-markers/persistent-topic、proposal/design/UI/tasks/test-cases及静态原型；仅日报可确认/不采纳/新手标，总览未确认来源仍为普通新闻、只confirmed注记，已确认取消保留纠错（尤其失效快照）。
- **后续动作**：原型/契约已修订；148条原型断言与12截图，见ui-prototype/evidence/report.md。总览未确认仅普通来源，日报按来源日期定位挑选→回总览confirmed，失效确认取消仍可用；缓存/迟到读写不串期/板块。验证浏览器已关闭；仍major/pending，待用户明确批准，不动产品。

### FB-4 2026-10-11【产品|fact-check】实际看真实日报再设计：标记应在线索级；当前原型未忠实呈现全屏阅读器与同section多thread

- **状态**：已消化（已核实并澄清；真实形状原型尚待修订，不代表获批）
- **原文要点**：实际看真实日报再设计：标记应在线索级；当前原型未忠实呈现全屏阅读器与同section多thread
- **已核实事实**：本轮实际只读打开:5100 AI版块10/9日报全屏reader；首TopicGroup有1个section-card和5条thread，文章篇数2/1/2/1/1，点击第一thread的header只展开2篇依据。现有代码article循环section.threads，header是button，summary有t:thread.id页边注锚；Go/TS thread都有独立id/report_id/section_id/title/summary/related_article_ids。既定设计本是thread粒度，误在原型一thread一section且拍平真实reader。
- **影响面**：proposal/design/UI/spec/tasks/test-cases澄清线索粒度，一章A/B独立、单线索多篇依据只有一marker；总览确认注记也按marker/thread而非sectionHasKey。旧148检查只属简化模型，README/证据已标适用边界，49场景计划待实现；未修改应用/原型代码。
- **后续动作**：事实已pin，参考浏览器keyevents-real-daily-lineage已关闭，真实截图仅/tmp不提交新闻内容。下版原型应直接复用全屏阅读器/章节多线索载体后再验/请用户批准；当前仍major/pending，不能拿旧扁平原型请求当前UI批准或开始apply。
