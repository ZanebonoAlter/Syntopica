# Explore Findings：右轨留白成因与「当日其他话题」数据流

> 探索期（2026-11-20）只读摸底结论。实现期照此动手，不必重探。

## 1. 右轨留白的精确成因

链路（全部在 `front/app/features/tags/components/console/`）：

| 层 | 位置 | 事实 |
| --- | --- | --- |
| ① 壳层锁视口高 | `BoardConsoleBody.vue:210-219` | `.console { height: 100dvh; overflow: hidden }` → 右轨加内容**不会撑高页面** |
| ② 三栏栅格 | `BoardConsoleBody.vue:221-226` | `17rem / minmax(0,1fr) / 21rem`（1440–1535.98px 收成 18rem，`:233-237`） |
| ③ 右轨容器 | `BoardConsoleBody.vue:246-256` | `.console-gov { overflow: hidden; display: flex; flex-direction: column }` |
| ④ 右轨满高可滚 | `BoardContextRail.vue:227-235` | `@media (min-width: 1440px) .bcr.is-rail { flex: 1 1 auto; height: 100%; overflow-y: auto }` |
| ⑤ 五块默认不撑开 | `BoardContextRail.vue:237-245` | `.bcr-block { flex: 0 0 auto }` |
| ⑥ 当日日报块独吞剩余高度 | `BoardContextRail.vue:113` + `:247-252` | `<section class="bcr-block bcr-block--grow" data-testid="rail-block-now">`；`@media (min-width: 1440px) .bcr.is-rail .bcr-block--grow { flex: 1 1 auto }` |
| ⑦ 块内内容不撑开 | `RailNowBrief.vue:129-134` | `.rnb { display: flex; flex-direction: column; gap: .45rem }` 无 `flex-grow` → 高度=内容高、顶部对齐 |

**结论**：留白 = ⑥ 分配的剩余高度 − ⑦ 的内容高，落在 `rail-block-now` 块内部底部（泳道层列表下方、话题追踪块上方）。不是 bug。

## 2. 截图里「2 条 / 1 条 / 1 条」是泳道层（不是别的组件）

- 组件：`RailNowBrief.vue:97-122`（小标题「各泳道」在 `:96`），计数在 `:114-118`：`{{ lane.clueCount === null ? '—' : `${lane.clueCount} 条` }}`。
- 模型：`nowBrief.ts:37-58`（`NowBriefModel.lanes`）；组装 `buildLanes()`（`nowBrief.ts:144-177`）。
- 入选三重条件：`section.persistentTopicId` 非空 **且** `topicStatusAtReport === 'active'` **且** topicId ∈ lane-dynamics 正式泳道名单（`nowBrief.ts:154-160`）。
- 口径：`clueCountAt()`（`nowBrief.ts:127-132`）= lane-dynamics 该泳道该日线索合计（与矩阵日格同源同值），**不是**日报分区 thread 数；取不到显示 `—`，不补 0。
- 排序：线索数降序，并列保日报原序（`nowBrief.ts:135-141`）。

## 3. 两套量纲（最易被读错的地方）

| 位置 | 数字含义 | 口径来源 |
| --- | --- | --- |
| 右轨泳道层「N 条」 | 该泳道当日线索合计 | `nowBrief.ts:127-132`（lane-dynamics） |
| 矩阵「当日其他话题」组「N 条线索」 | 该分区非综合稿 thread 数 | `dayOtherTopics.ts:124-130` |

spec 已有红线「泳道层不并排两套量纲」→ 新层必须分块 + 分单位后缀。

## 4. 「当日其他话题」数据流（零新增请求）

```
buildOtherTopics()            dayOtherTopics.ts:170-222   ← 纯函数、零网络、零状态
  ↑ useOtherTopicSelection    otherTopicSelection.ts:102-110
  ↑ useConsoleReadingWiring   useConsoleReadingWiring.ts:189（stageProps.otherTopics）/ :214（返回 otherTopics）
  ↑ BoardConsoleShell         BoardConsoleShell.vue:118-129 / :299-309
```

- 模型：`OtherTopicsView { state, rows, clueCount, statusText, note, retryable }`；行 `OtherTopicRow { sectionId, label, clusterIndex, threadCount, compositeCount, persistentTopicId, statusKind, statusText }`（`dayOtherTopics.ts:49-87`）。
- 状态机文案常量：`OTHER_TOPIC_STATUS_TEXT` + `OTHER_TOPICS_*_TEXT`（`dayOtherTopics.ts:29-47`）。
- 排除口径单一实现点（`dayOtherTopics.ts:17-18` 头注）：成员 = 当日报告 sections −（正式/休眠泳道在所选日解析为 content 的 sectionId）∪（persistentTopicId ∈ A 名单）。
- 报告详情读取：`useConsoleReading.ts:79-102`（报日索引 + 选中日绑定即读当日详情，**组折叠与否都读**）。
- 选行入口：`otherTopicSelection.ts:142`（`selectOther`，自带选中日/成员合法性校验）。

## 5. 断点与降级

| 区间 | 右轨 | `--grow` 留白 |
| --- | --- | --- |
| <768px | 不渲染（`BoardConsoleBody.vue:289-303`，走 `BoardNarrowSummary`） | 无 |
| 768–1199.98px | 不渲染（收进顶栏「本板块上下文」入口 → 工作面抽屉） | 无 |
| 1200–1439.98px | 可见（18rem），页面滚动、右轨随页面流 | 无 |
| ≥1440px | 可见（18–21rem），满高 + 内部滚动 | **有** |

## 6. 现状约束（实现时别踩）

- `openspec/specs/workshop-board-console/spec.md:188` + Scenario「候选分区不进右轨」：本 change 改的就是它（delta 已写）。
- `nowBrief.ts:19` 头注同样写死「候选分区不进右轨」→ 实现时同步改注释。
- `BoardContextRail.test.ts:167-176` 有 DOM 顺序断言 → 顶层顺序不能动。
- `docs/reference/standard/frontend/layout.md:214-224` 已登记 L1 `/tags/:boardId` 的固定视口栅格例外 → 按增补口径同单元格标注，不另起一行。
