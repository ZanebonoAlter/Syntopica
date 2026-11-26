# 合并记录：develop → master（2026-11-26）

> **用途**：本次把 `develop` 合入 `master` 的范围、内容、部署步骤、回滚方式与遗留风险。
> **生成时间**：2026-11-26
> **状态**：`develop` 已推送（`origin/develop` = `443d7f6b`），**待人工发起 PR 合入 `master`**。

---

## 1. 合并范围

| 项 | 值 |
|---|---|
| 源分支 | `develop` → `origin/develop` = `443d7f6b` |
| 目标分支 | `master` → `origin/master` = `27ca6dc6`（上一个 PR #23） |
| 提交数 | **186** 个（`origin/master..origin/develop`） |
| 时间跨度 | 2026-10-12 → 2026-11-26 |
| 代码量 | 2864 文件，**+343,114 / −32,351**（后端 489 文件 +98,916/−3,272；前端 480 文件 +110,628/−11,194） |
| master 独有 | 3 个提交（PR #20/#21/#23 的 merge commit） |

> ⚠️ **仓库没有 `main` 分支**，主线叫 `master`（`origin/HEAD → origin/master`）。本记录与下面的命令都按 `master` 写。
> ⚠️ master 上那 3 个 merge commit 不在 develop 里 → **用 merge 方式合，不要 rebase / force-push**。

---

## 2. 这批带过去的主线内容

按业务域归并（详细 change 清单见附录 A，127 个归档 change）：

| 业务域 | 主要内容 |
|---|---|
| **版块 / 泳道（semantic-board）** | 泳道画布外壳统一（新建/编辑两支）、泳道矩阵补三类可见性（窗口内未进日报的活跃泳道、「其他话题（未入泳道）」整行、行头就地改名）、泳道修正工作面对齐、泳道热度指标、雷达管线状态、事件故事线、关键事件日卡与跨泳道事件、候选时间窗、方向缺口豁免、幽灵版块误报修复 |
| **日报（daily-report）** | 日报断档回填、泳道修正生效时机澄清、页边注搜索切 DeepSeek、日报跨进程闸门、KE 行布局修复、日报线索生命周期修复 |
| **话题 / 图谱（topic-graph）** | 话题辅助标签约束（约束锚向量 + 归属准入 + 写入 API）、手动解除/加回归属、实体主体消解、话题关键事件标记、泳道手动修正、人物抽取退役 |
| **标签（tagging）** | 标签体系 spec 重组（23 个碎片 capability → 3 个族级）、辅助标签去重公告修正、辅助标签描述修复链、队列唯一约束迁移 |
| **内容 / 抓取** | 文章描述清洗、文章级 AI 生成疑似度判定（新建开关 + 四处标记 + 日报降权）、图片代理、RSSHub 快数据源、订阅源刷新抖动修复 |
| **搜索 / 检索** | 文章搜索（语义索引 + 排序模式）、pg_trgm 迁移、搜索向量列重建、embedding 维度契约、向量 halfvec 索引校正 |
| **前端体验** | SPA loading 体验、加载进度提示、文章列表面板瘦身、阅读面板重做、移动视口一阶段、设置导航重构、变更图谱规范、桌面壳工程 |
| **基础设施 / 稳定性** | 通知中心、日志 stdout 轮转修复、编辑地图部署产物污染修复、同源服务加固、灾备与运行手册 |

### 契约变更（openspec）

- **新增 31 个 main spec**：含 `board-api`、`board-diagnostics`、`board-research`、`tagging`、`auxiliary-label`、`tag-matching`、`workshop-board-console`、`workshop-lane-compose`、`workshop-radar`、`workshop-article-archive`、`lane-correction-surface`、`lane-manual-correction`、`lane-heat-metric`、`ai-generated-article-detection`、`article-search`、`article-description-sanitization`、`entity-subject-resolution`、`event-story`、`related-events`、`thread-related-events`、`radar-pipeline-status`、`topic-key-event-markers`、`spec-domain-index`、`ui-verification-trace-gate`、`frontend-interaction-contract`、`design-decision-landing`、`disaster-recovery`、`change-diagram-convention`、`desktop-shell` 等。
- **退役 26 个 main spec**（多为 spec 族合并的产物，Requirement 逐字搬运、零语义改动）。
- **改动 127 个 main spec**；**归档 127 个 change**。
- 本批含 3 次 spec 族合并：`merge-tag-spec-family`（23→3）、`merge-board-diagnostic-specs`（4→2）、`merge-board-upgrade-api-specs`（6→2）。

### API 影响

- 文档新增端点约 16 个（GET 12 / POST 2 / PUT 1 / DELETE 1），**均为新增，无破坏性变更**。
- `GET /semantic-boards/:id/lane-dynamics` 响应新增 `other_topics` 逐日数组（completed 日报日一条、含真实 0）。
  - **向后兼容**：新前端遇旧后端（无该字段）→ 整行不渲染；旧前端遇新后端 → 忽略该字段。
- `POST /semantic-boards/:id/persistent-topics/manual` 支持可选 `constraints` 字段（话题辅助标签约束），写入失败不落库。

### 数据库影响

- 新增 8 个迁移文件，**后端启动时自动执行**（`database.Connect` → `RunAutoMigrate` → `RunMigrations`）：
  `ai_detection_migration`、`auxiliary_description_repair_migration`、`daily_report_backfill_migration`、`detection_job_unique_migration`、`entity_subject_migration`、`lane_manual_correction_migration`、`tag_job_unique_migration`、`topic_lane_constraint_migration`。
- 另有索引类变更：pg_trgm 扩展、搜索向量列重建、`topic_tags` 向量 halfvec 索引按当前 embedding 维度校正。
- **迁移方向都是加列 / 加表 / 加索引 / 加唯一约束，没有删列删表** → 不需要数据回滚，也不会丢历史数据。
- ⚠️ **首次启动会比平时慢**：向量索引重建 + 多段迁移在树莓派上要跑一会儿，属正常，别中途 Ctrl-C。

---

## 3. 部署步骤（合并后需要做的）

```bash
# 1. 数据库（已在跑就跳过）
docker compose -f docker-compose.pg.yml up -d

# 2. 前端：构建静态产物 → 铺到后端同端口 → 重启后端（迁移自动跑）
bash scripts/dev/deploy-frontend.sh

# 3. 健康检查
curl --noproxy '*' -s http://localhost:5100/api/health
```

> 日常访问走静态托管（`backend-go/frontend/`），**不是 dev server**；dev server 只在临时调样式时开，用完即停。

---

## 4. 回滚

```bash
# 合并后回滚（保留 master 的历史）
git revert -m 1 <merge-commit>
# 或整段回滚
git revert <443d7f6b>^..<443d7f6b>
```

- 数据库侧**无需回滚**：迁移是加列/加表/加索引，多余的列与索引留着不影响旧代码。
- 前端回滚 = 重新构建上一版 `backend-go/frontend/`。

---

## 5. 合并前必看（遗留与已知问题）

| # | 事项 | 状态 | 谁处理 |
|---|---|---|---|
| 1 | **5 个已退役 main spec 的删除还挂在工作区未提交**（`board-article-api` / `board-editing-ui` / `board-management-api` / `board-upgrade-expand` / `upgrade-candidate-time-window`） | 由并发会话的 spec 合并操作产生，删除动作已落磁盘但没进 commit | **合并到 master 前必须确认并提交**，否则 master 会同时存在新旧两套契约（`board-api` 与被退役的 5 个） |
| 2 | 3 条既有红测试：`LaneTrendChart.test.ts`（2 条）、`BoardConsoleShellRailViewSwitch.test.ts`（1 条） | 合并前就红，**与本批改动无关**（已在 `HEAD~9` 复现），已用 `test-patrol.sh --register` 登记台账 | 台账里排期修 |
| 3 | 后端 `go test ./...` 大量失败 | **环境问题**：testcontainers 在树莓派上起 postgres 容器超时/竞争（`unexpected container status "created"`），非代码失败；本次改动涉及的包定向跑全绿 | 换台机器或给 testcontainers 放宽超时再跑全量 |
| 4 | `lane-matrix-coverage-and-others-row` 的 flow 变更溯源 | ✅ 已补（`docs/reference/flow/semantic-board.md` 业务约束节 + 变更溯源表第 645 行） | 无需处理 |

---

## 6. 门禁执行记录（2026-11-26，推送前）

| 检查 | 结果 |
|---|---|
| `golangci-lint run ./...` | ✅ 0 issues |
| `go vet ./...` | ✅ 通过 |
| `go build ./...` | ✅ 通过 |
| `go test`（本次影响包：`topicgraph/{handler,service,repository}`） | ✅ 全绿（含 `TestPutTopicLaneConstraints_AnchorFailureReturns503`、`TestCreateManualTopic_AnchorFailureReturns503`、`TestOtherTopics*`、`TestConstraint_*`） |
| `go test ./...`（全量） | ⚠️ 失败，原因是 testcontainers 环境超时（非代码问题，见 §5 第 3 条） |
| `pnpm lint` | ✅ 0 error（7 个既有 warning，与本批无关） |
| `pnpm exec nuxi typecheck` | ✅ 通过 |
| `pnpm test:unit`（全量 285 文件） | ⚠️ 4112 通过 / 4 失败 → 其中 2 条是本批「不再挂原生 title」带来的旧断言（已修为 `fix(tests)`），另 2 条是既有红测试 + 1 条 iconify 子集漏登记（已修为 `fix(icons)`，补 5 个 mdi 图标） |
| `pnpm build` | 未跑（本次推送未改构建配置；合并后由 `deploy-frontend.sh` 产出静态产物） |

---

## 附录 A：本次归档的 127 个 openspec change

<details>
<summary>展开查看完整清单（按归档日期）</summary>

**2026-08**（2 个）
- `2026-08-24-watch-keyword-and-quickadd`
- `2026-08-31-board-level-deep-analysis`

**2026-09**（21 个）
- `2026-09-07-split-board-upgrade-directions`
- `2026-09-11-overview-lane-dynamics`
- `2026-09-16-loading-progress-tips`
- `2026-09-16-spa-loading-ux`
- `2026-09-17-add-notification-center`
- `2026-09-17-declutter-article-list-panel`
- `2026-09-17-redesign-reading-pane`
- `2026-09-18-add-source-board-hit-rate`
- `2026-09-18-mobile-viewport-stage1`
- `2026-09-19-connect-dsh-energy-data-sources`
- `2026-09-19-fix-preview-dialog-iframe-height`
- `2026-09-19-improve-discovery-recommendations`
- `2026-09-19-render-mermaid-diagrams`
- `2026-09-22-add-image-proxy`
- `2026-09-22-lane-trend-overview`
- `2026-09-24-slim-article-list-payload`
- `2026-09-25-harden-go-same-origin-serving`
- `2026-09-26-board-signal-reports`
- `2026-09-26-daily-report-margin-notes`
- `2026-09-27-restructure-settings-navigation`
- `2026-09-29-ai-provider-model-overhaul`

**2026-10**（20 个）
- `2026-10-10-add-board-health-checkup`
- `2026-10-12-daily-report-lifeline-heal`
- `2026-10-12-review-task-frontload`
- `2026-10-13-fix-digest-tag-leak`
- `2026-10-14-add-article-search`
- `2026-10-14-add-fast-frequency-datasources`
- `2026-10-14-add-ui-verification-trace-gate`
- `2026-10-15-add-topic-key-event-markers`
- `2026-10-17-fix-daily-report-ke-row-layout`
- `2026-10-23-add-search-sort-modes`
- `2026-10-23-optimize-search-semantic-index`
- `2026-10-24-design-anchor-tax`
- `2026-10-24-fix-board-direction-gap-exempt`
- `2026-10-24-fix-board-ghost-false-positive`
- `2026-10-24-fix-board-health-llm-misread`
- `2026-10-24-fix-daily-report-cross-process-gate`
- `2026-10-28-add-intra-lane-theme-bucketing`
- `2026-10-28-guard-tag-exemplar-leakage`
- `2026-10-30-fix-nested-dialog-layering`
- `2026-10-31-add-radar-pipeline-status`

**2026-11**（84 个）
- `2026-11-02-daily-report-lead-direct-preview`
- `2026-11-02-fix-observation-pool-curve`
- `2026-11-02-rework-workshop-into-radar-and-desk`
- `2026-11-03-clarify-archive-match-rules`
- `2026-11-03-fix-embedding-model-column-width`
- `2026-11-04-add-desktop-shell-remote`
- `2026-11-04-add-dispatch-constraint-digest`
- `2026-11-04-add-ui-prototype-editor-agent`
- `2026-11-04-fix-daily-report-highlights-null`
- `2026-11-05-redesign-enrichment-research-workbench`
- `2026-11-05-speed-up-tag-embedding`
- `2026-11-06-add-event-story-heat`
- `2026-11-06-board-health-triage-advice`
- `2026-11-06-enable-unbound-jit-injection`
- `2026-11-06-surface-story-heat-in-report-top`
- `2026-11-07-deconflict-radar-warming-and-new-stories`
- `2026-11-07-surface-story-events-in-radar`
- `2026-11-09-fix-board-health-triage-degrade`
- `2026-11-09-matrix-clue-fallback-and-rail-merge`
- `2026-11-09-slim-lane-snapshot-trend`
- `2026-11-10-fix-board-health-magnet-signal`
- `2026-11-10-harden-ui-delivery-acceptance`
- `2026-11-10-redesign-daily-report-front-page`
- `2026-11-11-enable-story-cross-day-continuation`
- `2026-11-11-lane-compose-story-chain`
- `2026-11-11-normalize-event-subject-entities`
- `2026-11-12-anchor-topic-centroid-with-embedding`
- `2026-11-12-retire-downgraded-mount`
- `2026-11-12-retire-heuristic-tag-fallback`
- `2026-11-12-tolerate-schema-envelope-extraction-output`
- `2026-11-13-dedupe-thread-article-copies`
- `2026-11-14-add-browser-leak-guard`
- `2026-11-15-add-ui-contract-ladder`
- `2026-11-15-add-ui-stage-accounting`
- `2026-11-16-add-prototype-slop-filter`
- `2026-11-16-fix-board-console-spec-residue`
- `2026-11-16-fix-lane-metric-provenance`
- `2026-11-16-lane-manual-correction-loop`
- `2026-11-17-fix-lane-row-gap-zoom`
- `2026-11-17-unify-lane-workbench`
- `2026-11-17-unify-vector-column-dimension`
- `2026-11-18-fix-lane-label-horizontal-fit`
- `2026-11-18-make-range-controls-collapsible`
- `2026-11-18-unify-inner-scrollbar-style`
- `2026-11-18-widen-highlight-cluster-candidates`
- `2026-11-19-archive-time-chart-range`
- `2026-11-19-collapse-margin-notes-rail`
- `2026-11-19-docs-sync-audit-fixes`
- `2026-11-19-fix-duplicate-article-heat`
- `2026-11-19-replace-lane-lifeline-with-topic-events`
- `2026-11-19-stage-driven-constraint-injection`
- `2026-11-19-streamline-business-test-verification`
- `2026-11-20-add-change-diagram-convention`
- `2026-11-20-add-other-topics-to-context-rail`
- `2026-11-20-adopt-local-first-single-form`
- `2026-11-20-console-rails-live-signals`
- `2026-11-20-fix-disaster-recovery-channel`
- `2026-11-20-fix-radar-overview-false-zero`
- `2026-11-20-fix-reader-lede-blank-space`
- `2026-11-20-narrow-outbound-proxy-scope`
- `2026-11-20-retire-story-adopt-event-metrics`
- `2026-11-20-simplify-workshop-matrix-copy`
- `2026-11-20-upgrade-openspec-2`
- `2026-11-23-add-key-event-matrix-view`
- `2026-11-23-clarify-lane-correction-effect-timing`
- `2026-11-23-fix-editmap-deploy-artifact-pollution`
- `2026-11-23-fix-feed-refresh-churn`
- `2026-11-23-lane-compose-topic-event-tags`
- `2026-11-23-rework-lane-compose-swimlane-canvas`
- `2026-11-24-fix-log-stdout-rotation`
- `2026-11-24-switch-margin-note-search-to-deepseek`
- `2026-11-25-add-ai-generated-article-detection`
- `2026-11-25-add-daily-report-gap-backfill`
- `2026-11-25-add-key-event-other-lane`
- `2026-11-25-align-lane-correction-workface`
- `2026-11-25-fix-aux-label-dedup-ann`
- `2026-11-25-key-event-day-focus-cards`
- `2026-11-25-lane-matrix-coverage-and-others-row`
- `2026-11-25-retire-person-extraction`
- `2026-11-25-unify-lane-canvas-shell`
- `2026-11-26-add-spec-domain-index`
- `2026-11-26-merge-board-diagnostic-specs`
- `2026-11-26-merge-board-upgrade-api-specs`
- `2026-11-26-merge-tag-spec-family`

</details>

## 附录 B：合并命令

```bash
# 在 GitHub 上发 PR：base = master，head = develop（推荐，保留历史）
# 或本地：
git checkout master && git pull --ff-only
git merge --no-ff develop -m "Merge branch 'develop' into master（2026-11-26，186 提交）"
git push origin master
```

> 先把 §5 第 1 条（5 个 spec 删除）处理掉再合，避免 master 上新旧两套契约并存。
