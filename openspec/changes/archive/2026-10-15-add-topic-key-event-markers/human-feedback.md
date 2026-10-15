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

- **状态**：已消化（真实形状原型已改；图审启动阻断，不代表获批）
- **原文要点**：实际看真实日报再设计：标记应在线索级；当前原型未忠实呈现全屏阅读器与同section多thread
- **已核实事实**：本轮实际只读打开:5100 AI版块10/9日报全屏reader；首TopicGroup有1个section-card和5条thread，文章篇数2/1/2/1/1，点击第一thread的header只展开2篇依据。现有代码article循环section.threads，header是button，summary有t:thread.id页边注锚；Go/TS thread都有独立id/report_id/section_id/title/summary/related_article_ids。既定设计本是thread粒度，误在原型一thread一section且拍平真实reader。
- **影响面**：proposal/design/UI/spec/tasks/test-cases澄清线索粒度，一章A/B独立、单线索多篇依据只有一marker；总览确认注记也按marker/thread而非sectionHasKey。旧148检查只属简化模型；用户“可以，继续改吧”授权后已修静态原型：真实reader、一章三thread、父标题与子标题不同、依据2/1/0、marker同级动作；总览真实section去重和独立thread注记。49产品场景仍计划，未改正式应用。
- **后续动作**：事实已pin，参考浏览器keyevents-real-daily-lineage已关闭，真实截图仅/tmp不提交新闻内容。新版214原型检查通过，16次截图/14文件；K3截图抽检因后台host缺失未启动，失败run d012c144-f7e3-4210-9205-5f6bf2f9f9cd已记录并暂停。用户随后明确“本轮由我看图”，人工接管图审，原host问题未修且无K3输出；新版UI审批仍major/pending，不把“继续改”当批准或apply。

### FB-5 2026-10-11【产品|frontend】原型打包为all-in-one index.html；补充真实reader骨架截图；小改默认推荐且不过度测试

- **状态**：已消化
- **原文要点**：用户要求一个index.html方便查看，并给出真实reader截图；明确小改动默认按推荐，不过度测试，只是原型。
- **影响面**：仅ui-prototype/index.html内嵌现有主题/CSS/fixture/JS，交互与数据契约不变，README同步单文件说明。截图骨架为左侧索引/历史、正文话题章摘要与多条线索、下方生命线、右侧页边注，作为后续布局依据；本轮不借机扩改布局或复制真实新闻。
- **后续动作**：已生成约64KiB单文件。复制到仅有index.html的隔离临时目录，用keyevents-onefile-smoke验证加载样式、展开2篇依据、只标A不标B、双主题切换/无资源请求，共4条必要冒烟检查；浏览器已关闭，guard报告PID2045190复查已不存在。未重跑214条，不将历史全套结果算作本轮新验证。仍major/pending，无产品/数据/部署变更，无用户操作。

### FB-6 2026-10-12【产品|frontend】已标记应贴生命线；沿当前方向做高保真，不另起设计

- **状态**：已消化
- **原文要点**：用户问为何已标记与生命线不在一行；认可同轴方向，要求将现有原型美化为高保真。承接小改默认推荐、原型不过度测试和单文件要求。
- **影响面**：总览由独立低位标记槽改为源节点旁★n，点开按独立thread回看/取消；快照同轴、不伪造来源。reader沿用户截图的左索引/章小结/多线索/底部生命线/右批注骨架精修字体、分隔、留白和动作，保留原展开、依据数、警告与锚。单文件内嵌同步，主题仍canonical两套。
- **后续动作**：本轮26必要原型机械检查通过、9截图，1440/1920双主题与375短路径，无全页横溢/控件真实命中/同轴计数/独立取消；旧214留历史、不全量重跑。截图交用户人工看图，不再试失败K3入口。图样与记录见ui-prototype/evidence/hifi-report.md。正式代码/数据/部署不变，无用户操作；major/pending，方向确认不是新版原型批准，不apply。

### FB-7 2026-10-12【产品|frontend】用户明确认可FB-6高保真原型

- **状态**：已消化
- **原文要点**：“好的，我认可了”。上下文为最新高保真all-in-one原型及同轴标记方案，不再只是方向选择。
- **影响面**：ui-design.md的ui-approval更新approved，绑定当前index.html SHA-256 8eb863008f55f9f099c328a277b3be42e88c19957842cca6d7d5cb15dd7ec2fc；同轴★n、逐线索独立维护、原reader骨架成为当前获批基线。历史pending记录保留为当时事实。
- **后续动作**：审批已记录，README/tasks/当前证据说明同步。产品实现尚未启动，不修改正式代码/数据/部署，不自动apply；无需用户操作。

### FB-8 2026-10-13【产品|frontend】验收期 UI 反馈：标记按钮须与线索同行（原型分区）；总览点开详情暗色不可读且形态应对齐原型；UI 验收产物目录统一 ui-verification

- **状态**：已消化（当日修复并部署）
- **原文要点**：①「标为关键」button 独占一行太丑，与原型中「和线索一行」不一致；②深夜模式话题总览点开黑底黑字，原型中直接对点开的线索做标记处理挺不错，这里也要改（授权 glm-5.3-flash 子线程执行）；③UI 验收产物已统一规范到 `ui-verification/`（非 ui-acceptance）。
- **处置**：①KeyEventMarkerActions 改多根 fragment——按钮组=线索右列窄纵列（与标题同一视觉行，原型 thread-row 分区），AI 理由/错误/无归属说明=全宽行；窄屏 ≤720px 回退全宽横排；新增防回归断言（detail 与按钮组分离）。②子线程定位根因：popup 经 Teleport 挂 body 脱离继承链文字落 UA 纯黑、`.btb-popup-status` 硬编码 color:white 配 dark 提亮填充、引用不存在的 token 落亮色回退——修复为显式 `--color-text-primary` + 胶囊改 `--color-text-inverted`（双主题自动反转），三处注记详情行升级原型形态（Noto Serif 加粗可换行标题 + summary_snapshot 摘要行 + 动作靠右）。③后续 UI 验收产物一律 ui-verification/。
- **证据**：双主题+375px 截图 `ui-verification/ke-detail-dark-1440.png`、`ke-detail-editorial-1440.png`、`ke-focus-dark-1440.png`、`ke-detail-dark-375.png`；布局对比截图 `ke-layout-inline-1440.png`；report.md ⑤ P6 条目与时间线。
- **后续动作**：已部署 :5100 待用户看图复核；无数据/后端影响。
