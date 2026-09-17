
## 改版事实与决策全案（现状乱源/交互决策/虚拟列表约束/原型）

【改版对象与现状】阅读页中间栏 = front/app/features/shell/components/ArticleListPanelView.vue（350行）+ ArticleCardView.vue（front/app/features/articles/components/，161行）+ useArticleProcessingStatus.ts + components/layout/ArticleListPanel.css + components/article/ArticleCard.css。入口链：pages/index.vue → FeedLayoutShell → ArticleListPanelShell → ArticleListPanelView。ArticleCardView 仅被 ArticleListPanelView 引用（features/articles/public.ts 导出），改动不外溢。

【现状乱源】①订阅源状态大卡（feedStatusExpanded，刷新/总结/抓取三 pill）常驻列表顶部，折叠态只剩无字ⓘ按钮；②每卡 3 个流程 chip，shouldShowFirecrawlStatus/shouldShowSummaryStatus 在 feed 开启时全量显示，AI 未连通满屏 amber pending；③头部三段 chrome 用 --color-bg-hover(85%白) 而列表区 #f5f0e6，卡片(paper-card=bg-elevated)与栏底同色全靠描边——白/咖啡拼贴；④日期筛选独占一行；⑤单 feed 视图每卡重复 feed 名；⑥虚拟列表 itemHeight:120 与实际行高(约140-160)不符，.virtual-item 无固定高度，滚动错位风险。

【已定决策】行式列表（去卡片框、1px border-subtle 分隔、整栏 bg-elevated 单一 surface、选中=accent-subtle底+左2px accent 竖条）；处理状态成功/排队(pending)不显示、进行中=⟳(info色 spin)、失败=红⚠(--color-error)点开就地浮层；标签状态默认隐藏，行内 🏷(mdi:tag/tag-outline) 点开就地浮层；订阅状态卡→标题栏ⓘ popover(只读)；日期筛选→标题栏📅按钮+激活条件chip可清除；单feed视图隐藏行内feed名。

【关键技术事实】浮层（⚠/🏷 就地展开、日期面板、订阅popover）必须 absolute 定位覆盖、不改行高——useVirtualList(vueuse) 固定 itemHeight 不支持动态高度；浮层底色 MUST 实心 --color-bg-base（原型实测半透明 bg-hover 透出下层文字）；行高收敛后 itemHeight 对齐基准值；mdi 图标全部复用现有子集名（无新增，不触发 pnpm generate:icons）；颜色全走 main.css 语义 token 双主题适配，全局 token 不动。

【原型】openspec/changes/declutter-article-list-panel/ui-prototype/index.html（独立HTML可交互，Exhibit A主视图/B行状态6样张/C头部浮层2样张），已浏览器渲染验证。ui-design.md 八节齐备，ui-approval 初始 pending。

<!-- pinned 2026-09-17T03:32:13Z -->
