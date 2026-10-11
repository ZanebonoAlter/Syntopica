<!-- ui-impact: major -->
<!-- ui-approval: pending -->
<!-- ui-prototype: ui-prototype/index.html -->

## User Journey

FB-1选择长轴关键节点高亮，拒绝列表；FB-2进一步要求基于现有版块泳道，而非所有话题放一起，并参考现有双主题。FB-3要求AI建议只在日报展示/挑选，总览不带每期未定结果。FB-4实看运行中日报：标记宿主须是现有全屏报纸阅读器的线索行（thread），同一章可多条；旧扁平原型没有体现真实形状。本轮只读核实/澄清、不改原型代码，下一版须对齐后再请求批准。当前仍pending，选方向不等于批准。

左侧选版块 → 日报查看本期建议/新闻并确认、不采纳或自己标记 → 话题总览回看本版块普通动态与已确认高亮 → 点泳道聚焦/节点读详情 → 可导航到来源日报挑选，不在总览呈现未确认结果。已确认的取消是纠错维护（含失效快照），不开放总览新挑选。没有全局关键事件中心、全局泳道、独立路由或新弹窗。

## Information Architecture

总览复用TagsPage「叙事工坊」：紧凑顶栏、版块侧栏、右侧内容tabs。日报不是在这个tab下另铺事件列表：沿用BoardDailyReportTimeline的期刊卡片→drm-reader全屏报纸阅读层、封面/头条/highlights、目录/话题正文/页边注。main的BoardThreadBrowser绑定selectedBoardId（现有board-id）；左侧全部保持文章池含义，不为其新增总泳道。总览是当前版块的lanes，聚焦是当前版块的focus，不新增产品viewMode。

- 原型两个演示版块：AI与大模型含LLM模型/Agent工具两条泳道；住房与信贷含房贷期限/住房供应两条泳道。任何一次轴只出现其中一个版块的内容。
- 本版块总览保留多条横向泳道与固定左标签列；所有行共享来源日报日期标尺。点泳道名聚焦，返回保留版块范围。
- 泳道内保留真实section节点、原标题/状态色与关系来源；关键标记是独立注记层。快照失效是覆盖层，不占真实section ID，不制造identity/similarity边。
- 点节点在下方既有inline区域读详情，详情持续显示版块名/泳道名/来源日期；必要时滚动并聚焦标题，可返回选中节点。
- 「突出关键节点」在总览只作用本版块当前窗口；focus只作用当前泳道。普通点/标题仍可达，不改时间位置，不投影成确认列表。

### 展示字段来源

| 字段 | 来源/语义 |
| --- | --- |
| 版块名、侧栏数字 | 既有版块label/tag_count；原型18/12为明确演示标签数，不能以关键数替代 |
| 泳道身份 | 既有PersistentTopic，隶属当前selectedBoardId；不自动建题/激活 |
| 原节点标题/颜色 | section.cluster_label及statusFill五态语义Token；不回写成关键文案或重要性状态 |
| AI建议与理由 | 仅日报的源thread及校验后推荐；总览不渲染、不请求suggested集合，缓存有也不泄漏 |
| 日报所选日期 | 复用既有日报日期选择，定位本版块该日thread；原型用fixture.date与原生选择器，不新增产品日期字段 |
| 人工关键文字 | confirmed的来源/确认快照作为独立注记；AI不写confirmed |
| 来源日报日期 | section/report的period_date，不声称实际事发时间 |
| 动态数/关键数 | 分别统计真实section/注记，不能混计；原型真实来源fixture与失效快照也分开计数 |

字段语义锚：persistent-topic的版块内泳道边界/主题、生命线API、节点可读性；标记快照与人工动作锚topic-key-event-markers。真实一section可有多thread，TopicGroup也可有多个section；thread.title/summary才是标记快照源，父章cluster_label/规范topic label不能替代。旧fixture一section=一thread、两标题同值的模拟不足以证明此层级；下版须两条不同thread共用一section且标题不同。

## Interaction Contract

### 泳道总览与聚焦

- 保留当前版块上下文、原lanes/focus操作与返回。总览只含该版块；focus只含所选泳道。侧栏切版块重置局部选择/强调/错误与读取generation；迟到结果不抢占新owner。
- 合法在途人工保存仍按发起时owner完成；用户切版块后，不把旧版块成功/失败提示写到新内容。重新回原版块才显示已保存状态。
- 日期从左到右；原型160px每日刻度、桌面180px标签列（存量一致），手机140px标签列。长跨度用轴容器滚动，不强行缩成读不清小点；空日只留刻度，不造内容。
- 同日多项标题分行、独立点击，常规换行，超长省略并能从title/可访问名/详情读全。固定左列不遮住选中节点：返回节点定位到日期可视区，不把节点滚到左标签后面。
- 上下分行表示同日排布，不额外声称关系或因果。原型导轨为时间参照；产品真实边继续按response，不因关键标记重算。
- 点/Enter/Space选择只打开inline详情，不直接确认或启动研究；标题获焦，详情可见。「返回选中节点」回轴并恢复焦点，使用稳定定位避免移动控件脱靶。
- 突出开关默认关，普通点弱化但标题不整体淡到不可读。坐标、section集合、原字段和计数不变；无已确认时仍保留背景和关闭强调入口。
- 总览只有confirmed注记；unconfirmed对应来源作为普通section，不渲染建议星点/虚线/理由/待确认aria或确认、不采纳、标为关键按钮。详情提供来源日报导航，仅有效来源可定位；已确认快照无来源时不造日报锚。日报确认或维护取消后不移动真实点，不重新排日期。失效快照取消后不保留伪造普通点，焦点返回强调开关。
- 查看最近时改宽窄视口仍保持最近；主动向前翻不强制追到最近。日期选择复用7/14/30/全部，不新增产品日历管理。

### 真实日报线索行接入（FB-4）

TopicGroup → 当前section → article.drm-thread → thread的依据文章。人工新选择单位是thread.id；同章A/B独立，标A不改变B的徽标/选择状态、父标题/状态/任何原计数或图关系。多篇文章只是这条线索的依据，不逐篇标；未归属有效持久话题的线索给说明，不因标记自动建题。

徽标/理由/确认或手标追加在该article内，动作与现有button.drm-thread__header同级，不套入button；原header保持标题+摘要+篇数/展开，summary的data-mn-anchor=t:thread.id及fit_distance警告保留、不重写正文。原header无文章disabled不连带禁有效marker动作。

总览注记身份为marker/thread：同一真实section可挂多条confirmed线索注记，不用sectionHasKey把整章染成一个关键事件；不制造多个真实section。此前总览保持的版块/confirmed-only分工仍有效。

### 标记动作

| 状态 | 日报挑选 | 总览/focus回看 |
| --- | --- | --- |
| 普通来源 | 标为关键，不需要AI许可 | 原状态/原标题，仅新闻与来源日报导航，无新手标 |
| suggested未处理 | AI建议/理由，确认或不采纳 | 仅普通来源，无建议徽标、理由、虚线候选或挑选按钮 |
| confirmed | 已标记，可取消 | accent外环/星标，中心原色；可取消纠错，不显示AI理由 |
| 不采纳/取消，有来源 | 普通来源，可再手标 | 原新闻保留，不能在此重选 |
| 来源替换、confirmed快照 | 不造不存在的thread入口 | 独立星形快照、原文失效提示，可取消，无假日报链接 |

无有效话题归属显示说明，不自动建题。标记动作与现有文章展开button同级，不能嵌套或继承无文章header的disabled。写操作条目互斥、失败回滚；结果未知先重读再幂等重试。保留跑题警告、页边注文本锚、文章缓存和其他可用证据。不改topic生命周期，不自动信号发现/研究。

## State Matrix

| 状态 | 呈现/允许性 | 恢复 |
| --- | --- | --- |
| 未选版块/全部 | 文章池上下文，不展示全局泳道 | 选版块进入该版块原总览 |
| 读取loading | 当前owner缓存/正文可读，局部提示，未知状态禁写 | 以owner+generation校验；旧读取不结束新读取 |
| 日报无AI建议/总览非confirmed | 日报可手标；总览只新闻，未确认不产生注记 | 不凑数、不因打开页面调AI，挑选去日报 |
| 窗口无confirmed | 普通背景、日期与无已确认提示 | 可关强调/换范围，不是空确认列表 |
| 真正无section/注记 | 本版块局部空态，版块名/日期保留 | 换窗口，不造假节点 |
| 读取error | 原阅读保留、局部重试、未知状态禁写 | 先重读真实owner状态 |
| saving | 原点/坐标保留，详情正在保存/同条动作禁用 | 成功只改注记；失败恢复选择 |
| 写error/未知 | 局部错误，不丢原标题/正文，不多层toast | 重读对账后幂等重试 |
| 来源替换/失效证据 | 快照/其他可用证据保留，不冒充原文 | 不将确认迁到相似事件；可取消 |
| section空但快照有 | 独立注记仍可显示，不能被focusMeta.empty提前隐藏 | 不捏造section/关系 |
| 超长/同日多项 | 分行、可选、完整文字在详情/可访问名可达 | 键盘/触摸可达，不互盖 |
| 切版块/泳道 | 清局部选择/强调，不显示旧owner内容/提示 | 合法旧人工写仍保存原owner |
| 旧字段缺失 | 既有节点照常，独立读取恢复confirmed | 不批量回刷建议、不抹人工数据 |
| success | 持久人选一致，日报挑选/总览confirmed投影各守模式，刷新恢复 | 全程限定版块/泳道，真实计数不改 |

## Layout Contract

总览为workspace模式，沿用TagsPage现有容器；存量没有AppPageShell，不在本change迁移整页shell。桌面左260px版块列表、右流式内容；顶栏/tabs参照现有样式，不再做大字封面页。总览180px泳道标签列；原型每日160px便于读标题，实际基线以原型确认后约束。

1440×900/1920×1080检查版块上下文、同版块多行、日期/标题/命中区、详情；轴局部横向滚动，不能全页横溢。375×667/390×844版块选择到上方、tabs换行，轴保留固定140px标签与局部滑动，选中节点完整定位在标签右侧；详情单栏、按钮换行。新增断点统一767.98px，跨767/768不改人工状态。

点击命中区桌面至少36×36、窄屏44×44（可见小点外有真实按钮）。无新增dialog、无自定义宽度弹窗、无完整新页。日报标记复用现有reader全屏层/drm-layout/PeelTransition，保留原目录、左右正文/页边注布局（原1100px右注栏抽屉断点不改）；新增标记的小屏规则不得替代这套reader适配。详情长文沿reader约760px，不移植原型整页到Nuxt。

## Component Reuse

TagsPage/BoardListSidebar/ThemeToggle/BoardThreadBrowser的版块边界、lanes/focus与inline载体；DailyReportTopicSection原日报入口。原时间线/泳道切换、侦探墙/回刷/合并/新建泳道及版块治理工具保留，本change不删这些功能；原型只演示泳道优化，不实现其他工具，不给予按mock删存量入口的许可。AppButton sm/ghost/secondary、现有composables/API/normalizer/缓存，标记action组件无自行API副作用。

唯一主题源front/app/assets/css/main.css，data-theme=editorial|dark、产品继续useTheme。原型用generate-theme.cjs复制:root及两主题变量块到theme-tokens.css，附theme-source.json来源hash；style.css只引用语义Token，不手配第三套品牌色。

实测editorial背景#faf7f2、elevated#f5f0e6、accent#d94a4a；dark背景#080c12、elevated rgba(17,27,38,.98)、accent rgba(240,138,75,.85)。原statusFill新兴/持续/分化/合并/结束使用--color-thread-status-*；人工关键叠外环，不把这些状态色染成关键色。

## Prototype

ui-prototype/index.html默认AI版块的两条泳道，不展示住房内容。住房选择仅显示房贷/供应两条；全部只说明文章池不是全局泳道。支持聚焦/返回、confirmed注记、新闻详情、来源日报导航，日报按所选来源日期显示建议/完整挑选；总览只维护已确认取消。支持loading/失败、同日错行、长文、双主题及跨版块在途演示。

静态fixture，localStorage命名空间v3并以board:lane:id隔离；无远程请求/数据库/AI。其他tab为既有壳层的静态占位，不声称实现。主题仅复制现有变量、不修改产品main.css。

已存在的FB-3原型验证日报挑选/总览confirmed分工，但FB-4确认其日报扁平结构/一thread一section不符真实接入载体，暂不可作为新版日报批准基线；下版须现有reader层级及一章多线索。FB-1/FB-2/FB-3/FB-4处理不代表用户批准；major/pending，用户看完明确确认前不得apply。机制/截图/实际结果见README与evidence/report.md。

## Acceptance

| 验收 | 可执行/机械落点 |
| --- | --- |
| 版块隔离与来源 | 原型+组件/opencli：AI只含llm/agents，住房只含housing/supply；详情跟随owner，标签数不变 |
| 既有总览/focus | 点击泳道进入本版块focus，返回是同一版块多行；全部不生成全局轴 |
| 主题来源 | generate-theme.cjs后输出与main.css变量块一致；浏览器两主题computed style核对背景/文字/accent/五态 |
| 注记不改原字段 | 确认前后section ID/clusterLabel/status与状态色不变，快照没有section ID；原动态数与侧栏tag_count不变 |
| 强调不删背景 | 开关前后节点集/坐标一致，普通仍可点，AI不写confirmed |
| 在途切版块 | 旧读/写迟到不串新owner，重新回旧版块可见合法保存，局部提示隔离 |
| 详情/键盘 | click/Enter后到匹配详情，聚焦标题、回轴恢复焦点，选择不标记/研究 |
| 真实线索粒度 | 现有全屏reader内一section的A/B不同thread、A两篇依据；只A改变、B和父标题/状态/计数不变，按钮同级/文章独立/t:thread.id锚不变，总览按marker身份而非sectionHasKey |
| 入口分工/动作 | 总览无未确认徽标/理由/待确认aria/新挑选；来源日报定位正确日期/thread，可确认/不采纳/手标；回总览只confirmed高亮且原新闻保留。取消维护/刷新/读写失败逐状态，缓存/迟到推荐不泄漏 |
| 宽窄与长标题 | 1440/1920/375/390/767/768，两主题：无全页横溢、同日/标题不遮挡、局部滑动可定位，选中节点不被固定标签盖住 |

原型检查不能代替test-cases.md完整故事、产品静态:5100 opencli、PG testcontainer和真实AI效果评估。本次只改change制品/原型，正式产品/旧数据/部署不变，无用户配置或清理操作。
