# Tasks：右轨「当日其他话题」子块

> 承接：`proposal.md`（why/what）、`ui-design.md`（交互与数据契约）、`design.md`（D1–D9 技术决策）、`specs/workshop-board-console/spec.md`（条款 delta）。
> 后端零改动；数据全部来自前端既有派生层（`buildOtherTopics` + 壳层单 store）。

## 1. 实现

- [x] T1 新建 `front/app/features/tags/components/console/RailOtherTopicsBrief.vue`：受控展示 `OtherTopicsView`——标题行「当日其他话题」+ 计数徽标「N 条线索」+ 每行（分区标题两行截断 / 该分区当日线索数「N 条线索」/ 归属状态词）；空态、加载中、失败文案与重试入口全部复用 `dayOtherTopics.ts` 既有常量，不新写一套。
- [x] T2 `front/app/features/tags/components/console/BoardContextRail.vue`：在 `rail-block-now` 块内、泳道层之后挂新子块（子块自带标题行与分隔，视觉上读作两块）；子块默认隐藏、仅 `@media (min-width: 1440px)` 显示；点行发 `select-other` 事件。
- [x] T3 `front/app/features/tags/components/console/railContext.ts` + `useConsoleRail.ts`：`RailContext` / `RailBinding` / `ConsoleRailDeps` 加 `otherTopics` 字段并透传（与 `now` 同款路径）。
- [x] T4 `front/app/features/tags/components/console/BoardConsoleShell.vue`：把 `wiring.otherTopics` 接进右轨装配（`useConsoleShellRail` 的 deps）。
- [x] T5 选行定位链路：`BoardContextRail` → `BoardConsoleBody` → `BoardConsoleShell` → 既有 `selectOther({ sectionId, date, topicId })`，由主区既有阅读槽承接；右轨不自己改选择状态、不另开浮层。
- [x] T6 口径与视觉分家：泳道层计数单位「条」、其他话题层「条线索」；两套计数不并排成同一列；子块标题与泳道层标题层级可区分。

## 2. 测试

- [x] T7 新建 `front/app/features/tags/components/console/RailOtherTopicsBrief.test.ts`：success（有分区：行内容 + 计数徽标 + 状态词）/ empty（0 分区）/ loading / error 四态；点行发 `select-other` 事件。
- [x] T8 补 `front/app/features/tags/components/console/BoardContextRail.test.ts`：新子块挂进 `rail-block-now`；右轨五个顶层区块顺序不变（既有顺序断言不破）。
- [x] T9 视口门槛机械锚：断言子块样式默认隐藏且存在 `@media (min-width: 1440px)` 显示规则（1200–1439.98px 与 <768px 不出现该子块）。

## Review

- [x] T10 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账

## 3. 文档

<!-- doc-impact: flow standard -->

- [x] T11 `docs/reference/flow/semantic-board.md`：右轨顺序描述改写；「candidate 分区 MUST NOT 进右轨」约束句按 delta 改写；补变更溯源行（§12.2 归档后补链接）。
- [x] T12 `docs/reference/standard/frontend/layout.md`：「已登记例外」表在同一单元格内标注本 change 名 + 表下追加增补说明（不另起一行）。

## 4. 验证

> 实测结果（2026-11-20，本机 linux-arm64）：下列命令全部实跑通过；浏览器几何实测见 `ui-verification/report.md`（A-1–A-6 全 pass）。

- `cd front && pnpm lint` → **实测 exit 0**，0 error / 7 warning（warning 均在既有文件）
- `cd front && pnpm exec nuxi typecheck` → **实测 exit 0**，无 TS 错误
- `cd front && pnpm test:unit <受影响 8 文件> --maxWorkers=2` → **实测 8 文件 / 106 测试全绿**（RailOtherTopicsBrief / BoardContextRail / BoardWideWorkface / BoardConsoleShell×4 / nowBrief）
- `cd front && NODE_OPTIONS=--max-old-space-size=4096 pnpm build` → **实测 exit 0**（本机 7GB 内存下默认堆会 OOM，需抬堆）
- `bash scripts/harness/doc-impact.sh verify openspec/changes/add-other-topics-to-context-rail` → **实测通过**（声明 flow standard，2 文件）
- `bash scripts/harness/check-standards.sh --change add-other-topics-to-context-rail` → **实测 317 通过 / 0 失败**（含 F/G/I 段）
- `bash scripts/harness/ui-verification-trace.sh openspec/changes/add-other-topics-to-context-rail` → **实测通过**（组件测试在盘 + 6 条合法判定条目 pass 6 / fail 0 / 未覆盖 0）
- `bash scripts/harness/review-trace.sh openspec/changes/add-other-topics-to-context-rail` → **实测 exit 0**（review.md 建账 R-1–R-6）
- `openspec archive add-other-topics-to-context-rail` 后 `git diff --stat openspec/specs/workshop-board-console/spec.md` → **预期净 diff 大于单 change 增量**：当前基线是 `f908cc53` 回退后的「四块旧版本」，本 change 的 ADDED 条款以「五块 + 当日日报三层」为基底，归档会把 `console-rails-live-signals` 被覆盖的增量一併正畸回来（详见 delta 的「基线说明」段与 review R-2）。核对点：归档后该条款应含「当日日报」三层表达 + 五个顶层区块顺序 + 其他话题层
- `bash scripts/dev/deploy-frontend.sh` → **实测完成**（构建 + 铺 `backend-go/frontend/` + 重启后端；脚本 60s 健康等待超时但后端实际启动完成，`/health` 200、`/tags/2197` 200、静态产物时间 17:25:55），静态产物形态经 agent-browser 复验见 `ui-verification/report.md` A-6
- 域外红记录：本 change 实现期间撞见两类**不属于本 change** 的门禁红，均未修改他人文件——① `front/app/utils/htmlSanitize.ts:35 no-control-regex`（属并发 change `fix-reader-lede-blank-space`，后被其属主修好）；② `internal/reader/repository/repository.go:322 commentFormatting`（属并发 change `add-ai-generated-article-detection`，已转绿）。归档时两者均已消失
