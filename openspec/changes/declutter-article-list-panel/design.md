# Design: declutter-article-list-panel

## Context

现状与动机见 proposal.md；UI 契约与已批准原型见 ui-design.md。本 change 纯前端，涉及 4 个既有文件 + 新增 1 个小组件，无后端/数据模型改动。关键约束：列表用 vueuse `useVirtualList`（固定 itemHeight，不支持动态行高），所有浮层必须覆盖式定位；mdi 图标仅可用 `iconify-subset.json` 既有子集。

## Goals / Non-Goals

**Goals**
- 中间栏回归单一 surface 的行式列表，消除白/咖啡拼贴
- 处理状态收敛为行尾单图标四态 + 统一「处理详情」浮层
- 头部三合一（标题+计数 | 条件chip | 📅 | ⓘ），修复虚拟列表行高错位

**Non-Goals**
- 不改右侧阅读区、侧栏、顶栏、页面级三栏布局模式
- 不动 main.css 全局 token 与其他页面
- 不新增键盘导航、不引入第三方浮层库、不展示标签名列表（数据不可得）

## Decisions

**D1 · 行高与虚拟列表**：行式布局后行高收敛为固定基准（标题两行 40 + meta 24 + padding ≈ 76px，实现时以实测为准校准），`useVirtualList` 的 `itemHeight` 对齐基准值。**备选**：换动态高度虚拟列表（自研或换库）——改动大、收益低，否决。标题两行用 `-webkit-line-clamp` 硬限，保证行高不漂移；浮层 absolute 覆盖不参与行高计算。

**D2 · 浮层实现**：不引库，原生实现一个 `RowStatusPopover.vue`（features/articles/components/）——Teleport 到列表容器外层？否：浮层需随行滚动定位，直接在行容器内 absolute 定位（`.virtual-item` 需 `position: relative`），z-index 高于相邻行，实心底 `--color-bg-base`（半透明 bg-hover 透字，原型已验证踩坑）。日期面板与订阅 popover 同理挂在 panel-header 容器内 absolute。

**D3 · 状态图标派生逻辑**：单图标四态由既有三态元数据派生，优先级 失败 > 进行中 > 排队 > 完成——任一失败即红⚠；否则任一 processing 即 ⟳；否则抓取 pending 或总结 incomplete 即琥珀⏳；否则淡灰✓。沿用 `useArticleProcessingStatus.ts` 的状态判定函数（getFirecrawlStatusMeta/getSummaryStatusMeta）做聚合，不重复实现。图标全部用子集既有名：`mdi:clock-outline` / `mdi:loading` / `mdi:alert-circle` / `mdi:check-circle`，零新增，不触发 `pnpm generate:icons`。

**D4 · 视图态来源显示**：`showFeedTitle = selectedFeed 为空`（全部文章/收藏夹/分类视图显示来源名；单 feed 视图隐藏）。分类 pill 在行内**移除**（信息与来源名重复，分类维度已由侧栏切换表达；原型即此形态）。

**D5 · surface 选型**：面板容器维持 `--color-bg-elevated`（与侧栏/阅读区一致的卡片语言），头部行、行列表、空态全部同底色，去掉头部/筛选区的 `--color-bg-hover` 白底与 paper-card 的描边阴影堆叠。选中态 accent-subtle + inset 2px accent 竖条（替换原 border-left 方案，避免行高抖动）。

**D6 · 测试策略（complexity: simple 档）**：test-cases.md 串主链路故事（scan→select→filter→状态浮层→订阅 popover），落点：ArticleCardView 行渲染状态矩阵 Vitest 组件单测（四态图标 + 浮层内容 + 浮层互斥）+ opencli 主交互链路一处（选中/浮层开合）+ 1440×900 / 1920×1080 双视口视觉子代理检查（§11 major 双层验收）。

## Risks / Trade-offs

- **行高校准偏差**：实测行高与 itemHeight 不一致会再现滚动错位——任务里显式含「实测校准 + 滚动回归」验证步。
- **浮层被虚拟列表裁剪**：`.articles-list`/`virtual-list` 容器有 overflow-y:auto，浮层超出可视区会被裁——浮层定位需 clamp 在列表可视区内（底边不足时向上翻）。实现时验证最后一行展开场景。
- **移除分类 pill 属轻微信息削减**：与「全部文章」视图来源显示互补，可接受；若后续需要可从 meta 行 hover 或详情浮层回归。
- **完成态淡灰✓仍占位**：为保「任意态可点开详情」的入口一致性，接受这 1 个低对比图标的常驻。
