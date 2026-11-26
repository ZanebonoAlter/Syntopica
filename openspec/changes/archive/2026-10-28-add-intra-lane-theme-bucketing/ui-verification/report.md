# UI 验收报告：add-intra-lane-theme-bucketing（ui-impact: minor）

- 执行日期：2026-10-24 20:30
- 环境：静态托管 `http://localhost:5100/`（`deploy-frontend.sh` 构建铺盘 + 后端重启）；真实数据报告 `report 1234`（board 3030「开发者工具与开源生态」，2026-10-24，`generation_prompt_version=6.0`），该期 cluster 0 被拆成 6 个方向桶。
- 数据安全：**未点击「重新分桶」**——真实日报的重分桶会销毁/重建 section 行并改变 ID（不可原路还原），交互语义由组件/组合式用例兜底（A-2/A-3）；视觉验收只做只读量测与截图。
- 方法：`getBoundingClientRect` / `scrollWidth` 机械量测（非目测）+ agent-browser 截图；数据侧核对见 A-6。

## 逐场景结论

### A-1 子卡方向徽标与条件标题【pass】
- 依据：`front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts` 新增 3 例——`bucket_title` 非空才渲染 `.drm-section-bucket`（含 `title` 全文）、组内单条但带桶名时仍渲染 `.drm-section-card__title`、单条未分桶时不渲染标题（历史行为不变）。

### A-2 「重新分桶」动作可见性 / loading / 二次确认【pass】
- 依据：同文件新增 4 例——算法桶显示按钮、`watch_keyword`/`watch_sentence` 隐藏、`rebucketSectionId` 非空时 `disabled` + `aria-busy=true`、点击先弹全局 `useConfirm`（取消不发事件，确认后 `emit('rebucket', 100)` 且文案含「替换该天该泳道的主题分桶」）。

### A-3 成功后局部替换 / 兄弟桶仍同组 / 深链回退【pass】
- 依据：`front/app/features/tags/composables/useDailyReportReader.test.ts` 新增 5 例——只替换该桶组（其余分区与顺序不变）、旧桶行删除后 `sectionAliases` 指向组头、失败回 idle 且原数据引用不变、`buckets=1` 不改本地数据、请求进行中重复点击被忽略；`groupSectionsByTopic` 断言替换后兄弟桶仍按同一 `persistent_topic` 成组。另有 `DailyReportTopicSection.test.ts` 的 `sectionAliases` 回退用例（旧锚点 100 → 组头 201 滚动定位）与 `dailyReportMagazine.test.ts` 的 `bucket_index` 升序稳定排序 3 例。

### A-4 双视口视觉检查（1440×900 / 1920×1080）【pass】
- 依据（1440×900，`/tags?board=3030&report=1234`）：泳道「AGENT编程工具相关」下渲染 6 张子卡，6 个方向徽标全部完整可见（`clientWidth == scrollWidth`，宽 102–134px）、6 个「重新分桶」按钮；`document.documentElement.scrollWidth == 1440`（无横向溢出）；卡片头行高统一 22px（徽标+标题+按钮单行不折行）。截图 `evidence/desktop-1440-buckets.png`、`evidence/desktop-1440-lane-buckets.png`。
- 依据（1920×1080）：唯一差异是容器留白，`scrollWidth == 1920`、徽标 0 截断、子卡 6 / 按钮 6、头行高仍 22px。截图 `evidence/desktop-1920-buckets.png`。
- **本条首测（2026-10-24 21:40）即抓到一个实现缺陷**：徽标原写 `max-width: 12ch`，`ch` 是数字宽（≈汉字一半），实测 6/6 徽标被截断成「Agent循…」（clientWidth 83 / scrollWidth 102–123），方向名完全读不出。已改为 `max-width: 14em`（≈14 汉字，覆盖 prompt 的 12 字上限）并加源码守卫用例；重新构建部署后两档视口复测全部不截断（见上）。记 `review.md` R-24。

### A-5 真实数据形态核对（只读 SQL）【pass】
- 依据：report 1234 的 cluster 0 → 6 行 section，`bucket_title` 全非空且互不相同，`bucket_index` 0–5 连续；31 条 thread 全部挂到 section（0 条 `section_id is null`、无重复行）；6 桶 tag 集两两**零重叠**；兄弟行 `lane_tier=l1_direct` / `persistent_topic_id=502` / `topic_match_confidence=anchor_hit` 一致，`topic_match_distance` 各自独立（0.0658–0.1204）；6 行 embedding 均非空；`article_count` 等于桶内 thread `related_article_ids` 去重并集（3/2/2/3/2/3）。
- 反向样本：同批其余 3 个版块（report 1236/1237/1238）无 `count(*)>1` 的组——其最大 section 仅 5 条线索，未达 `intra_bucket_min_threads=6`，与门禁设计一致（非静默失效）。

## 终态

- 组件层验收全绿：`DailyReportTopicSection.test.ts` 37/37、`dailyReportMagazine.test.ts` 24/24、`useDailyReportReader.test.ts` 11/11、`dailyReports.test.ts` 8/8。
- 视觉验收：A-4 两档视口 pass（截图 3 张在 `evidence/`）；未覆盖 0 项。
- 未实点项：A-2/A-3 的「重新分桶」点击路径仅由用例覆盖，真实数据上不做破坏性实点（见「数据安全」）。
