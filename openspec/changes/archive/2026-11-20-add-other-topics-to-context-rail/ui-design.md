<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计：右轨「当日其他话题」子块

> 档位 `minor`：在既有页面结构与交互模式内增加一个子块，不新增页面/弹窗/导航，不改四档布局档枚举，不做多步流程。minor 档不要求原型（`ui-prototype: none`），人工审批仅对 major 强制（`ui-approval: not-required`）。

## 入口

- **页面入口**：L1 版块态势页 `/tags/:boardId`（顶部面包屑「叙事工坊 / <版块名>」），**治理右轨**（页面最右栏，≥1200px 常驻；1440px 起为满高可滚栏体）。
- **落位**：右轨「当日日报」块内部，紧随泳道层列表之下，同属 `rail-block-now` 这一顶层区块。
- **显示门槛**：仅 ≥1440px 渲染（该宽度段才有 `--grow` 撑开产生的留白可填）。1200–1439.98px 右轨随页面流动，不出该子块。
- **用户主任务**：在宽屏下**一眼看到当天除关注泳道之外还发生了什么**，并可直接点进某个分区读正文。

## 状态

| 状态 | 触发 | 右轨表现 |
| --- | --- | --- |
| success（有分区） | 选中日有日报且含未入正式泳道的分区 | 标题行「当日其他话题」+ 计数徽标「N 条线索」；逐行：分区标题（两行截断）+ 该分区当日线索数 + 归属状态词 |
| empty（0 分区） | 当日分区全部已在泳道内可读 | 复用既有诚实空态文案「当日分区都已在泳道内可读」（非 0、非失败） |
| empty（无日报） | 选中日该版块没有日报 | 复用既有文案「所选日没有日报（不是 0 条其他话题）」 |
| loading | 报日索引或当日报告读取中 | 复用既有文案「读取报日…」/「正在读取当日报告…」 |
| error | 报日索引或当日报告读取失败 | 复用既有文案 + 重试入口（与右轨既有降级一致，不新造提示条） |
| degraded | 部分泳道当日内容未取到 | 组照常列出 + 既有附注「部分泳道当日内容未取到，其他话题按当日报告分区列出」 |

> 状态文案与状态机**全部复用** `dayOtherTopics.ts` 的既有常量，右轨不得另写一套空态/错误文案。

## 组件复用与布局

- **交互模式**：`panel`——右轨本身是页面内的并列分区，右轨子块继续是只读并列呈现；点行只在主区既有阅读槽承接，不开浮层。
- **布局档**：沿用 L1 既有登记例外（`docs/reference/standard/frontend/layout.md` §已登记例外，`rework-workshop-into-radar-and-desk` 原始登记），本 change 在该条目内标注增补来源，不另起一行；四档枚举不变、页面仍为三栏常驻、`overflow: hidden` 不变。
- **组件复用**：
  - 数据模型：`dayOtherTopics.ts` 的 `buildOtherTopics` / `OtherTopicsView`（**单一实现点**，右轨不重算排除口径与计数）
  - 状态词与空态常量：`OTHER_TOPIC_STATUS_TEXT` / `OTHER_TOPICS_*_TEXT`
  - 右轨块骨架与标题行样式：`BoardContextRail.vue` 的 `.bcr-block` / `.bcr-head` / `.bcr-title` / `.bcr-count`
  - 行内点击选分区：既有 `selectOther` 选行链（`otherTopicSelection.ts`），右轨只发事件
- **不复用**：`DayOtherTopicsGroup.vue`——它是矩阵网格里的 `display: contents` 折叠组，依赖 `observationMatrixRows.css` 的列对齐语义，搬进右轨会破网格语义；右轨另做受控展示组件。
- **口径分家**：泳道层「N 条」= lane-dynamics 该泳道当日合计（与矩阵日格同源同值）；本子块「N 条线索」= 该分区非综合稿 thread 数。两者分属不同子块，各带标题与单位后缀，不得并排成同一列。
- **响应式范围**：≥1440px 显示；1200–1439.98px 不显示（页面会变高，守「≥768px 视口布局与交互零变化」红线）；<1200px 右轨整体不渲染（窄屏走 `BoardNarrowSummary`）。

## 验收

- **组件测试**：`front/app/features/tags/components/console/RailOtherTopicsBrief.test.ts`（新建）——覆盖 success / empty / loading / error 四态渲染、行内容与计数、点行发事件。
- **接线测试**：`front/app/features/tags/components/console/BoardContextRail.test.ts`（既有）——补一条：新子块在 ≥1440px 挂进 `rail-block-now`、五块顶层顺序不变。
- **门禁**：`pnpm lint` / `pnpm exec nuxi typecheck` / 受影响组件测试 / `pnpm build`。
- **人工/浏览器验证**：在 `ui-verification/report.md` 建账——2560×1440 视口下右轨「当日日报」块不再留白、无关注泳道时仍列出当日其他话题分区；1440×900 下同样成立；1200–1439.98px 与 375px 下无变化。

## Interaction Decisions

| ID | 决策 | 模式 | forbidden | 状态 |
| --- | --- | --- | --- | --- |
| D1 | 右轨「当日其他话题」子块以只读并列列表呈现（不折叠、不弹窗、不抽屉） | `panel` | `dialog`, `drawer`, `canvas` | `pinned` |
| D2 | 点某行后的承接位置 = 主区既有阅读槽（舞台底部其他话题阅读区） | `page` | `dialog`, `drawer`, `panel` | `pinned` |

> 判据依据：D1 命中 P5（多对象并列比较，禁 `dialog`）与 P4；D2 命中「操作对象在主区内已可见」，禁开新浮层。
> `pinned` 语义：实现阶段不得替换、简化、合并这两行的模式或形态；如认为必须偏离，先回 requirements 档向用户提出。

## Data Provenance

| 数据项 | source | 说明 |
| --- | --- | --- |
| 当日其他话题分组成员与行序 | 派生 `buildOtherTopics(selectedDate, reportDay, allLanes, resolveLaneDay)` | 前端派生；唯一实现点 `front/app/features/tags/components/console/dayOtherTopics.ts`；右轨只消费结果 |
| 每分区当日线索数 | 派生 该分区非综合稿 thread 数（`OtherTopicRow.threadCount`） | 与矩阵「当日其他话题」组同源同值（口径不含综合稿） |
| 分区归属状态词 | 派生 `topic_status_at_report` → `OTHER_TOPIC_STATUS_TEXT` 映射 | 生成日报当时的状态，不读当前话题状态 |
| 子块计数徽标 | 派生 组内非综合稿线索数合计（`OtherTopicsView.clueCount`） | 与矩阵该组 summary 徽标同值 |
| 当日日报详情（分区来源） | `api GET /daily-reports/:id` | 既有端点，右轨不发新请求（复用壳层已取的单 store） |
| 泳道层当日线索数（既有，不改） | `api GET /semantic-boards/:id/lane-dynamics?days=N` | 与矩阵日格同源同值，本 change 不动 |

## 明确不做

- 不在右轨出现任何日期选择器或日期切换按钮。
- 不在右轨出现「当日其他话题」以外的第三套计数口径（如综合稿数、事件线数）。
- 不把矩阵里的「当日其他话题」折叠组从矩阵移除、折叠或改成由右轨独占。
- 不在右轨出现日报全文、正文阅读区或日报详情弹窗（正文只在主区既有阅读槽）。
- 不在右轨出现构成候选、话题追踪、来源构成之外的新治理动作入口（如「转正」「收编」按钮）。
- 不在右轨出现矩阵的泳道热度格、日期轴或演进节点。
- 不在 <1440px 视口出现该子块。
