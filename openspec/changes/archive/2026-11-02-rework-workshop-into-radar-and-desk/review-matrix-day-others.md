# 8C「矩阵当日其他话题」增量独立评审（change-review-glm，2026-10-30）

> 审查对象：change `rework-workshop-into-radar-and-desk` 的 8C 增量最终态（G1→G2→G3→8C.4S→G4a/G4b→8C.6 集成批→2026-10-29 真实浏览器验收→M1/L1 文案修复后代码）。只读评审：除本报告外未改任何文件、未跑测试/构建、未派子线程；仅执行了只读验证命令（`openspec validate --strict`、`check-tasks-tail.sh`、sha256/wc/grep）。

## 一、结论先行：8C 增量可归档性判断

**结论：8C 增量的代码、测试、类型、lint、文档与真实浏览器验收证据链完整且相互印证，spec↔实现↔测试三方一致，无 High 问题；但增量尚未达到归档门禁——8C.7/8C.8 仍未勾选，且当前 `:5100` 静态产物（index sha256 `1cca5ffcb4b68848…`，mtime 10-29 23:37，与浏览器验收指纹一致）不包含其后的 M1/L1 修复源码。按 8C.8 合同「以新 index/chunk/hash 在 :5100 跑 8C.7，不得验旧产物」，还需一次：单次 `deploy-frontend.sh --no-restart` 候选发布 → 新产物上复跑 S11 关键断言（至少 S11-A 冷启动 + M1/L1 文案相关项）→ 勾 8C.7/8C.8。完成该步后，8C 增量自身无已知阻断（整 change 归档另含 8.4/9.3/9.4/9.5/11.1/11.2 等前序欠账，不在本报告判定范围）。**

tasks 8C 段勾选真实性抽查无冒称：8C.1–8C.6/8C.4S/8C.5A/8C.5B 的验收声明逐条与 `ui-verification/matrix-day-others-*.md` 各报告实跑记录对得上；8C.7/8C.8 如实保持未勾（M1 修复报告 §7 明确「父统一勾 8C.7」，浏览器报告自称「未勾 tasks」）。

## 二、问题清单

### High

无。未发现功能错误、红线违反或冒称验收。

### Medium

**M-1｜已验收静态产物落后于已修源码；M1/L1 修复未经真实浏览器复验（流程缺口，非代码缺陷）**
- 证据：`backend-go/frontend/index.html` sha256 = `1cca5ffcb4b68848…`（2026-10-29 23:37 铺盘），与 `ui-verification/matrix-day-others-2026-10-29/report.md` §1 验收指纹一致；而 M1/L1 修复（`matrix-day-others-8c7-m1-fix.md`：删 `console-no-lanes-fallback` 横幅、`BoardConsoleStage.vue` 335→299 行、`BoardNarrowSummary.vue` L196-197 文案改「0 正式泳道——」）发生在验收之后且明言「未部署、未跑浏览器」。浏览器报告 §3 A5/A6 与窄屏 L1 断言是在**旧产物**上通过的。
- 处置建议：执行 8C.8 单次候选发布后，以新 index/chunk/hash 复跑 S11-A 冷启动主链（A1–A8、A15–A18）+ M1（横幅不存在、内联说明唯一）与 L1（窄屏文案）断言，通过后再勾 8C.7/8C.8；不得把 2026-10-29 报告直接套用到新产物结论上。

### Low

**L-1｜`refreshAfterRebuild` 的上下文过滤条件恒真（冗余防御，无运行时影响）**
- 位置：`front/app/features/tags/composables/useConsoleReading.ts:150-155`（`if (date !== null && (selectedDate === null || selectedDate === date))`）。`target.date()` 与 `currentSelectedDate()` 同源于唯一绑定 `selectedDate` ref，该条件永远为真；注释与代码形态暗示存在「切日中途不读旧日」过滤，实际该防护由 G1 `reportDay` 的 seq/display gate 承担。
- 处置建议：后续小修时删除恒真分支或改写注释，避免读者误以为此层有过滤语义。不阻塞归档。

其余维度未立新项；已知已披露项核对见 §四。

## 三、逐维度核对结论

1. **spec↔实现↔测试三方一致**：核过无异常。两新 Requirement（`specs/workshop-board-console/spec.md`「日报日驱动的日期轴（含零正式泳道）」5 Scenario + 「当日其他话题折叠组与就地下读」12 Scenario）与 `specs/board-lane-dynamics/spec.md` 新增 2 Scenario（零泳道仍按报日读分区 / 报日索引不改聚合口径）合计 19 条，与 tasks §11 映射表逐一对应；抽查落点测试真实存在且断言语义相符（`ObservationMatrix.groups.test.ts:28` 组顺序、`:76` 0 休眠 summary、`:109` 只标所选日；`BoardConsoleShell.integration.test.ts:40/69/103/127/171/194/223/242/271` 九例集成；`useConsoleReturnContext.test.ts:153/162/350/355` 旧六字段与 other 载体）。选中日三级校正实现（`laneConsoleView.ts:100-131` `reconcileSelectedDate`）与 spec 优先级逐条对上（保留→内容日→最新报日→null，`null` 三态不误报无日报）；`specification` 的「成员按 sectionId∪persistentTopicId 排除、cluster_index 升序、threads.length 计数、状态词取快照」在 `dayOtherTopics.ts:126-210` 单点实现，组件/壳层无第二套口径。测试计数逐文件复核与各报告声明一致（G1 118 / G2 55 / G3 阅读 16 = 189；grid 9、group 12、dayOtherTopics 20、laneConsoleView 23、useConsoleSelection 8、normalizer 9、cache 11）。
2. **读取边界与竞态**：核过无异常。索引单份/板块（`laneDayReportIndex.ts`，切板块 `reset`）、详情 ≤14 / 文章 ≤20（`useLaneDayReading.ts:53-54` 常量 → `laneDayReadingCache.ts` Map FIFO + 失败不入成功缓存 + 在途合乘 + `clear()` 失效代）；R1 索引请求序（`runIndexRequest` seq 拒写旧响应并把最新结果转交旧消费者，不永 loading）、R2 缓存失效代 + `ensureRelatedArticle` 双道 epoch 校验（`laneDayRelatedArticles.ts:66-72`）、R3 force 有效 days = max(请求, 成功索引, 同板块在途)（`requestIndex`）均在代码中落位且有先红后绿记录；G4a 前置（`retryReport` 索引 await 后、旧日读取前 `selectionMovedAway` 复核，`useConsoleReading.ts:126-130`）与 G4b 接线（`useConsoleReadingWiring.ts` 显式 `clearOther`、成员合法性只认 G1 rows）与报告一致；晚到不回填（boardEpoch + requestSeq + display gate）在 `laneDayReportDay.ts`/`laneDayCurrentReading.ts` 均有。
3. **身份与生命周期**：核过无异常。排除口径为并集且候选池/watch/辅助标签不进入模型入参（`dayOtherTopics.ts:162-186`）；快照状态词只用 `topic_status_at_report`，未知枚举降级「当时状态未记录」、无 ID「当次未归属」（`statusKindOf`），不读当前 topic 状态；未发现任何生命周期/生成/回填写操作被本增量触碰（矩阵/组/阅读槽全只读，浏览器报告 §8 网络统计 GET 3616 / 业务 POST 0）。
4. **返回契约**：核过无异常。`other:<sectionId>` 阅读 key 与 lane 实体分离，不写 `laneId`/URL `lane`（`consoleReturnContext.ts:44-47`、`useConsoleReturnContext.ts:81-87/147-156`）；旧六字段形态在无 other 时逐键不变（`useConsoleReturnContext.test.ts:350-355`）；URL 显式 lane 不同→不恢复 other、相同→恢复（S11-20 真实往返）；`openReport` 只写路径 `/tags/:boardId/:date` + 可选 `lane` query，全文件无 `date=` 写入；0 正式泳道返回恢复（allLanes 空）有 G4a 用例、浏览器 C7/C8/C10 实证。
5. **测试资产真实性**：核过无异常。分批计数不跨轮相加、各报告口径自洽；8C.4S 前后 189 例 `fullName+status` 多重集 SHA 一致且父方独立重派生（`f990f5e7…`）；G4b 旧 38 例拆分多重集一致 + 集成 9 例断线反证（断开接线 9/9 红）；S11-A/B/C 浏览器证据为 32 张 PNG + 逐断言实测值（报日集合以现场索引为准、70 格对 API 逐格、双主题像素采样核对），未把快照样本当现场数量；未发现 skip 掩盖（各目标批次 0 failed/0 skipped）。
6. **规范符合**：核过无异常。抽样 27 个源文件/组件全部 ≤500 行且 ≤15360B（最紧 `ObservationMatrixGrid.vue` 447 行/15224B，风险已在 structure 报告自曝）；抽样目录 grep `as any`/`@ts-ignore`/`@ts-expect-error` 零命中；snake_case→camelCase、数字 ID 字符串化只在 API 边界（`api/normalizers/laneDayReading.ts`、`api/otherTopicObservation.ts`），组件/状态层无散落 `String(id)` 映射。
7. **已知披露项核对（不重复立新）**：①auxrepair 404/Go lint 事件属 `board-health-triage-advice` 他线（g1-review.md「并发门禁补充」，本增量未触碰 Go）；②`SignalReportView.test.ts` 7 条他域旧债原样保留（8c6 报告 §4.3 有 SHA/mtime 佐证，typecheck 终态 `components/console/` 0 命中）；③`reading-behavior/track-batch` 自动埋点（浏览器 L2）为既有链路且 spec 明文豁免用户状态动作；④留账项实际为 L2（埋点口径）、L3（线程展开态不跨返回保留，合同允许）与 C4（真库无 `persistent_topic_id=null` 样本，组件层覆盖），未见「L4」条目——按 L2/L3/C4 核对一致留账；⑤M1/L1 已修：`BoardConsoleStage.vue` 299 行无 `console-no-lanes-fallback`/`.stage-fallback`/`shortDate` 残留，`BoardNarrowSummary.vue:196-197` 文案已改「0 正式泳道——」，两处负向断言与 `matrix-empty-no-lanes`/`other-topics-group` 正向断言在 `BoardConsoleShell.test.ts:45-47/99-100` 落位，61 例回归绿记录在案。

## 四、实际读过的文件清单

**openspec 文档**：`proposal.md`、`tasks.md`（§8C 全段 + §11 映射表）、`specs/workshop-board-console/spec.md`、`specs/board-lane-dynamics/spec.md`、`specs/daily-report-system/spec.md`（头部，确认无 day-others delta）、`implementation-matrix-day-others.md`、`test-cases-matrix-day-others.md`。
**ui-verification**：`matrix-day-others-2026-10-29/report.md`、`matrix-day-others-8c7-m1-fix.md`、`matrix-day-others-8c6-typecheck-fix.md`、`matrix-day-others-g1.md`、`matrix-day-others-g1-race-fixes.md`、`matrix-day-others-g1-review.md`、`matrix-day-others-g2.md`、`matrix-day-others-g3.md`、`matrix-day-others-g3-review.md`、`matrix-day-others-g3-closure.md`、`matrix-day-others-structure.md`、`matrix-day-others-g4a.md`、`matrix-day-others-g4a-review.md`、`matrix-day-others-g4b.md`（另 `ls` 核对 shots/ 32 张 PNG 在场）。
**产品代码（全文或关键段）**：`composables/useLaneDayReading.ts`、`laneDayReportIndex.ts`、`laneDayReportDay.ts`、`laneDayCurrentReading.ts`、`laneDayRelatedArticles.ts`、`laneDayReadingCache.ts`、`useOtherTopicObservation.ts`（关键段）；`console/otherTopicSelection.ts`、`useConsoleReading.ts`、`useConsoleReadingWiring.ts`、`useConsoleSelection.ts`、`laneConsoleView.ts`、`dayOtherTopics.ts`、`consoleReturnContext.ts`、`useConsoleReturnContext.ts`、`consoleStageTypes.ts`、`useConsoleShellActions.ts`（头部）、`useConsoleRefresh.ts`、`DayOtherTopicsGroup.vue`、`OtherTopicReading.vue`、`ObservationMatrix.vue`、`ObservationMatrixGrid.vue`（script+模板+结构）、`BoardConsoleShell.vue`、`BoardConsoleStage.vue`、`BoardNarrowSummary.vue`（关键段）；`api/laneDayReading.ts`、`api/otherTopicObservation.ts`、`api/normalizers/laneDayReading.ts`。
**测试（存在性/命名/计数抽查）**：`ObservationMatrix{.test,.dayHeaders.test,.groups.test}.test.ts`、`ObservationMatrixGrid.test.ts`、`DayOtherTopicsGroup.test.ts`、`OtherTopicReading{.test,.observation.test,.signals.test}.test.ts`、`BoardConsoleShell{.test,.integration.test}.test.ts`、`BoardConsoleShellNavigation.test.ts`、`useConsoleReturnContext.test.ts`、`consoleReturnContext.test.ts`、`dayOtherTopics.test.ts`、`laneConsoleView.test.ts`、`useConsoleSelection.test.ts`、`useLaneDayReading*.test.ts`×5、`laneDayReadingCache.test.ts`、`api/normalizers/laneDayReading.test.ts`。
**红线参照**：`docs/reference/flow/semantic-board.md`（红线 19-26 体检只读等）、`docs/reference/flow/daily-report.md`（相关段）、`docs/reference/standard/shared/test-design.md`（机械断言/继承与调整节）。
**只读验证命令实跑**：`openspec validate rework-workshop-into-radar-and-desk --strict` → valid（exit 0）；`bash scripts/harness/check-tasks-tail.sh …` → ✓（exit 0）；`sha256sum backend-go/frontend/index.html`；抽样 27 文件 `wc -l/-c`；目录级 grep（`as any`/`@ts-ignore`/`date=`/`暂无活跃泳道`/`console-no-lanes-fallback`）。

## 五、给父的裁决点

1. **8C.8 发布与 8C.7 复验（对应 M-1，唯一实质待办）**：单次 `bash scripts/dev/deploy-frontend.sh --no-restart` 后以新 index/chunk/hash 复跑 S11-A 主链 + M1/L1 文案断言（范围可收窄到受影响断言：横幅不存在、窄屏「0 正式泳道」、A5/A6/A15-18），通过才勾 8C.7/8C.8；失败按合同回滚备份。
2. **类型表达收尾（已披露，二选一）**：`otherTopicSelection.ts` 的 `reportDates` 声明收宽为 `ComputedRef<ReadonlySet<string> | null>` + `selectDayHeader` 空态守卫，或维持 8C.6 的接线侧直传 G1 原始集合（两者已论证运行时等价，见 `matrix-day-others-8c6-typecheck-fix.md` §2.1）；建议归档前定案并留一行记录。
3. **产品级留账裁决（不阻塞归档）**：L2 埋点是否计入「严格零写」口径；L3 线索展开态是否扩返回载体（会破旧六字段 `Object.keys` 断言）；`othersOpen` 纯折叠偏好是否持久化（G4a §8.5 保守未做）。
4. **低优先清理**：L-1 恒真条件（`useConsoleReading.ts:150-155`）可在下一次触及该文件时顺带清理。

## 六、总体判定

8C 增量工程质量与账目纪律高于本仓库平均水平（先红后绿、多重集对账、断线反证、诚实留账均有实据）；唯一门禁缺口是 M-1 的「修复后产物未上盘未复验」。完成 §五.1 后，8C 增量可进入整 change 归档流程（整 change 归档仍受前序阶段与根账 53/1/2、他域类型债等既有欠账约束，不因本报告改变状态）。
