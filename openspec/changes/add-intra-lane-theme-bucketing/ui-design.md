<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

**入口**：TagsPage → 日报阅读层 →「关心的话题」分区 → 话题泳道卡（`DailyReportTopicSection.vue` 的 `group`）→ 其下的 section 子卡（`.drm-section-card`）。

**变更**（全部落在既有页面结构与交互模式内）：

1. 子卡头部新增**主题方向徽标**：显示 `bucket_title`（如「工具调用与编排」），仅在 `bucket_title` 非空时渲染。泳道卡标题仍是泳道名（topic label），不重复展示。
2. 子卡标题渲染条件从「`group.sections.length > 1` 才显示 `cluster_label`」改为「组内多于 1 条 section **或** 该 section 有 `bucket_title` 时显示」，保证同话题多条时每张子卡可区分。
3. 子卡头部新增**「重新分桶」**次要动作（icon + 文案），仅对**非 `watch_*`** 的 section 开放（物化组由关注轨自管，服务端同口径返回 400；历史 `lane_tier` 为空的 section 同样可见，服务端会做阈值/状态门禁）；时间线/历史期同样可见（历史期重跑只影响该期）。

**不改变的**：泳道视图（`BoardThreadBrowser.vue` lanes 布局、节点堆叠、focus 视图）、阅读层分区结构、排序分区（active / briefs）、泳道趋势区、页边注与深链定位入口（重分桶后旧锚点的回退见上表「深链失效」）。

## 受影响状态

| 状态 | 表现 | 说明 |
| --- | --- | --- |
| loading | 「重新分桶」按钮置灰 + 原位 spinner；旧子卡内容保持不动 | 不做骨架屏、不清空内容，避免读者丢失阅读位置 |
| success | 该组子卡按新的桶顺序重排（`bucket_index` 升序），桶徽标与子卡标题更新；顶部一过性 toast「已重新分桶：N 个方向」 | 滚动位置尽量保持在所属泳道卡内 |
| empty（拆不动） | toast「没有发现可拆分的主题方向」，子卡保持原样 | 服务端校验后仍为 1 桶时返回 200 + `buckets=1`，前端不报错 |
| error（LLM/网络失败） | toast 失败原因 + 子卡保持原样；按钮恢复可点 | 服务端失败已回滚，前端无需额外清理 |
| 长文本 | 徽标宽度上限 **14em**（≈14 个汉字，覆盖 prompt 的 ≤12 字上限；**不可用 `ch`**——`ch` 是数字宽，12ch 只够 6 个汉字，2026-10-24 真机实测整条被截断），超出仍以 `…` 截断且完整文本进 `title`/可访问名称 | 复用现有 `drm-*` 文本溢出约定 |
| 误操作 | 「重新分桶」写入前需二次确认（复用全局 `useConfirm`，底层 `AppDialog size="sm"`），确认文案说明"该天该泳道的分桶会被替换" | 破坏性弱但不可逆（旧桶分组不保留） |
| 深链失效 | 重分桶会删建 section 行，旧锚点 `#report-section-<id>` 可能指向已删除的行 | 命中不到时前端回退到该组首条 section 并定位；组本身消失时回退到泳道卡 |

## 复用组件与布局模式

- **布局模式**：不改动，沿用阅读层现有 `reader` 列宽（正文中栏 40rem 级），子卡仍在泳道卡内部纵向排列，无新的横向分栏。
- **复用组件**：
  - `DailyReportTopicSection.vue`：`drm-section-card__head` 插槽位（徽标与标题）
  - 既有徽标族：`SectionTierBadge` / `SectionWatchBadge` / `SectionAnchorBadge`（新徽标沿用同一尺寸与配色语言，不自造排版）
  - `Icon`（`@iconify/vue`，`mdi:...`）用于「重新分桶」图标
  - `AppDialog`（`size="sm"`，420）承载二次确认；**优先复用全局 `front/app/composables/useConfirm.ts`**（已封装 AppDialog sm + 确认流程，2026-09-23 change 沉淀），不另写一套确认样板
  - toast：沿用项目既有全局提示能力（不新增弹层体系）
- **不新增**：页面、路由、主视图模式、批量操作条。

## 验收映射

- 组件测试（真实文件路径）：
  - `front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts` —— 子卡渲染桶徽标、同话题多子卡按 `bucket_index` 排序、徽标为空时不渲染、重分桶按钮可见性、loading 期间按钮置灰
  - `front/app/features/tags/components/daily-report/dailyReportMagazine.test.ts` —— `groupSectionsByTopic` 组内按 `bucket_index` 排序、缺省 `bucket_index` 时保持稳定顺序
  - `front/app/features/tags/composables/useDailyReportReader.test.ts` —— rebucket 成功后局部替换组内 sections、失败时状态回到 idle 且原数据不变
- 端到端主链路（agent-browser，实施后补 `ui-verification/report.md` 建账）：打开某板块日报 → 展开「关心的话题」某泳道 → 看到 ≥2 张带桶徽标的子卡 → 点「重新分桶」→ 子卡重排且 toast 出现。
- 视口：桌面 1440×900 与 1920×1080 两档做视觉检查（`ui-verification/report.md` 记录依据）。
