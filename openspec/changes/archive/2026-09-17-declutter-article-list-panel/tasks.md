<!-- complexity: simple -->
<!-- ui-impact: major -->
<!-- constraint-domains: reading -->

# Tasks: declutter-article-list-panel

> 测试用例先行（§2 用例先行）：test-cases.md 已随本 change 落盘（主链路故事 + 变体走查），实现任务对着它写。

## 1. 行式列表与 surface 统一

- [x] 1.1 重构 `ArticleCardView.vue` 为行式结构（标题≤2行 + meta 行 + 行尾状态图标位 + ☆），移除 paper-card 卡片框/投影/分类 pill/错误行/三 chip 行；单 feed 视图隐藏来源名（`selectedFeed` 为空才显示）。验证：`cd front && pnpm exec vitest run tests/unit/ArticleCardView` 类组件测试绿（见 2.4 落点后回填）
- [x] 1.2 重写 `ArticleCard.css` + `ArticleListPanel.css`：面板单一 surface（头部/列表/空态同 `--color-bg-elevated` 底），行间 1px `--color-border-subtle` 分隔，选中行 accent-subtle 底 + inset 2px accent 竖条，移除头部/筛选区白底。验证：`pnpm lint` 绿 + 浏览器目检无白/咖啡拼贴
- [x] 1.3 虚拟列表行高校准：实测行高后更新 `ArticleListPanelView.vue` 的 `itemHeight`，标题两行 `-webkit-line-clamp` 硬限。验证：滚动列表无重叠/错位（opencli 截图或人工目检留痕）

## 2. 头部三合一与浮层

- [x] 2.1 重构 `ArticleListPanelView.vue` 头部：标题+计数常驻、日期筛选 📅 图标按钮（下拉面板含快速选项/起止日期/应用清除）、激活条件 chip 可一键清除；移除独立 filter-bar 行。验证：opencli 点 📅 开合 + 应用筛选后 chip 出现/可清除
- [x] 2.2 订阅源状态 popover：移除常驻状态大卡，标题栏 ⓘ 图标（仅单 feed 视图显示）+ 只读 popover（刷新/总结/抓取三项 + 设置指引）。验证：opencli 点 ⓘ 开合；全部文章视图无 ⓘ
- [x] 2.3 新增 `RowStatusPopover.vue`（features/articles/components/）：覆盖式浮层，内容=抓取/总结/标签三行状态 + 失败错误文案（默认完整可见、超长滚动）；同一时刻仅一个行浮层，点外部/再点图标关闭。验证：组件单测覆盖三态内容与互斥/关闭
- [x] 2.4 状态图标四态派生：基于 `useArticleProcessingStatus.ts` 聚合 失败>进行中>排队>完成，图标 mdi:clock-outline/mdi:loading(转圈)/mdi:alert-circle/mdi:check-circle，颜色 secondary/info/error/text-muted。验证：`pnpm exec vitest run` 状态矩阵单测绿（四态图标 + 空数据/极端错误文案变体）
- [x] 2.5 ArticleCardView 集成状态图标与浮层（替换旧三 chip 渲染与 `useArticleProcessingStatus` 的 chip 用法），错误文案行移入浮层。验证：组件测试绿 + `pnpm exec nuxi typecheck` 绿

## 3. 测试

主链路测试用例见 test-cases.md（含变体走查与继承处置）。

- [x] 3.1 test-cases.md 主链路 opencli 落点执行：进入阅读页→点行选中→点状态图标开浮层→再点收起→📅 筛选→清除→ⓘ popover，全程无控制台报错。验证：opencli 断言输出留痕
- [x] 3.2 双视口视觉子代理检查：1440×900 与 1920×1080 截图对照 ui-design.md §5 与已批准原型——单一 surface、无横向溢出、浮层定位正确（含列表最后一行展开不裁剪）。验证：视觉检查结论回写 ui-design.md §8
- [x] 3.3 实现与原型差异说明回写 ui-design.md §8 Acceptance（如有重大差异需重新审批）

## 4. 文档

<!-- doc-impact: none(纯前端展示层重构，无接口/数据模型/业务行为变更；归档后按 §12 补 flow/reading.md 变更溯源行) -->
<!-- doc-impact-excuse: flow=纯前端 UI 展示层重构，front/app/features/** 命中属启发式命中面，无业务链路/约束变更，溯源行按 §12 归档后补; api=零后端文件改动，proposal 中仅以否定句提及 API; database=零 models/AutoMigrate 改动，工作树 reader/database 脏文件属并行 change add-notification-center -->

- [x] 4.1 `docs/reference/flow/reading.md` 变更溯源表补一行（已完成，archive 后补）（change 名、摘要「阅读页列表面板行式改版与状态浮层契约」，链接 change 目录）
- [x] 4.2 `docs/reference/architecture/map.md` — N/A：组件路径与入口未变（经 grep 无 ArticleList/列表面板条目需更新）

## 5. 验证

- [x] 5.1 `cd front && pnpm lint` — 期望 0 error
- [x] 5.2 `cd front && pnpm exec nuxi typecheck` — 期望 0 error
- [x] 5.3 `cd front && pnpm test:unit` — 期望全绿（含新增 ArticleCardView/RowStatusPopover 测试）
- [x] 5.4 `cd front && pnpm build` — 期望构建成功
- [x] 5.5 `cd backend-go && go build ./...` — 期望成功（确认零后端影响）
- [x] 5.6 人工验证：`bash scripts/start-dev.sh status` 确认前后端在跑，浏览器打开阅读页对照已批准原型过一遍主链路（选中/筛选/浮层/popover），确认无回归 → 实测通过（截图留痕见 ui-design.md §8）

### Scenario → 测试映射表

| Scenario | 测试文件 |
| --- | --- |
| 面板单一 surface | 人工：浏览器双视口目检（ui-design.md §8 留痕） |
| 行内元数据去重 | front/app/features/articles/components/ArticleCardView.test.ts |
| 排队中文章可见 | front/app/features/articles/components/ArticleCardView.test.ts |
| 常态静默 | front/app/features/articles/components/ArticleCardView.test.ts |
| 失败可辨识 | front/app/features/articles/components/ArticleCardView.test.ts |
| 查看处理详情 | front/app/features/articles/components/ArticleCardView.test.ts |
| 查看失败原因 | front/app/features/articles/components/ArticleCardView.test.ts |
| 浮层互斥 | front/app/features/articles/components/ArticleCardView.test.ts |
| 单 feed 视图查看订阅状态 | 人工：浏览器实测（ui-design.md §8 留痕） |
| 非 feed 视图隐藏入口 | 人工：浏览器实测（全部文章视图无 ⓘ） |
| 筛选激活反馈 | 人工：浏览器实测（chip 出现/清除留痕） |
| 面板开合 | 人工：浏览器实测（📅 开合留痕） |
