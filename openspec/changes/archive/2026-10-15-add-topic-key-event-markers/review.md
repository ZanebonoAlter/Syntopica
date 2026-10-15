# add-topic-key-event-markers 实现后聚焦审查（6.6）

- 审查人：change-review-glm 子线程（2026-10-13）；审查方式：对照 4 份 delta spec、design D1–D6、tasks.md Scenario 映射逐项核查 + 抽查映射 9 条；只读验证 go build/vet 退出码 0。
- 处置原则：严重项当轮修复并补测试锚；建议项全部处置；确认无误项记录于文末。

### R-1 2026-10-13【High】总览/焦点取消注记失败完全静默（读写 error 无局部反馈与恢复入口）
- **状态**：已修复
- 发现：三处取消按钮只消费 pending 不渲染 error；markerView 对 detached 标记读不到 `-markerId` 负键错误（useTopicKeyEvents.ts:177/236），总览是失效快照取消的唯一入口，失败后 UI 无任何提示。
- 处置：① markerView 对 detached 行按 `-markerId` 键回查错误；② BoardThreadBrowser 焦点行内列表/快照详情/lanes popup 三处注记行渲染 `.btb-ke-detail__error`（role=alert），取消按钮保持可用=重试入口。
- 验证：focus 测试新增「shows a local error with retry entry when unmark fails」通过（15/15）。

### R-2 2026-10-13【Medium】重建 × 板块互斥下推荐 token 失效语义与 D3 不一致
- **状态**：已修复
- 发现：TryLock 失败注释称「rebuild already restarted the run row」不实（token 仅在新任务 Upsert 时才换）；同报告重建场景旧任务可对已替换 thread 写建议（数据安全兜底=旧 thread 已删，但 run 行终态失真）。
- 处置：新增 `invalidateKeyEventRunToken`——SaveReport 重建事务内（ReAttachKeyEventMarkers 头部）轮换 run 行 generation_token，旧任务 Save/Finish 按旧 token 匹配落空；service 注释改为描述真实机制。
- 验证：新增 `TestKeyEvent_RebuildRotatesRunToken`（token 轮换 + 迟到批次整批丢弃 + 旧终态不污染）通过。

### R-3 2026-10-13【Medium】topic 注记缓存 days 窗口切换不重拉
- **状态**：已修复
- 发现：`existing.markers.length > 0` 短路使 7→30 切换后旧窗口外 confirmed 缺失。
- 处置：entry 记录已拉 days；days=0（不限窗）覆盖一切、窗口变化或曾有错误即重拉。
- 验证：useTopicKeyEvents 既有用例回归 + typecheck 通过。

### R-4 2026-10-13【Low】API/flow 文档三处与实现不符
- **状态**：已修复
- 发现：① PATCH 示例用 `expected_revision`（实现绑定 `revision`）；② PUT 孤儿 section 写 400（实现 404）；③ flow 用 retrospective/unverifiable 与 resetLocal 措辞（实现字段 current_retrospective_attention/evidence_verified；resetLocal 非版块切换生产路径）。
- 处置：api/daily-reports.md 两处改正、flow/data-enrichment.md 按实际字段改写、flow/daily-report.md 措辞修正。
- 验证：doc-impact verify 复跑通过。

### R-5 2026-10-13【Low】突出模式零确认节点无提示 + 锚断言近恒真 + 映射表落点失真
- **状态**：已修复
- 发现：① highlight 开启且零 confirmed 无提示文案；② 「t:thread.id 锚不变」断言恒真兜底；③ tasks.md 映射三行落点应为 KeyEventMarkerActions.test.ts。
- 处置：① 焦点头下 `.btb-ke-empty-hint`（role=status）；② 改精确断言 `[data-mn-anchor="t:7"]`；③ 映射表三行校正。
- 验证：focus/section 测试通过；scenario-trace 49/49 复跑通过。

### R-6 2026-10-13【Low】GetTopicLifeline key_events 装配零测试
- **状态**：已修复
- 处置：新增 `TestKeyEvent_GetTopicLifelineCarriesConfirmedOnly`（confirmed 透出、suggested/dismissed 不进生命线）。
- 验证：通过。

### R-7 2026-10-13【Low】ListBoardConfirmedMarkers 死代码
- **状态**：已修复
- 处置：删除（前端读路径走 per-topic 端点，避免与实际路径漂移）。
- 验证：golangci-lint 0 issues。

### R-8 2026-10-15【Critical】推荐 evidence_refs 奖约不一致：候选行只渲染裸标题，AI 只能回字符串，全部建议解析失败被吞（suggested_count 恒 0）
- **状态**：已修复
- 发现：归档前用户要求查证「零建议是真数据问题还是代码问题」——DB 实查 39 个 run 全部 done 无 error、候选 1~27 条，但 suggested_count 全为 0；ai_call_logs response_snippet 显示 AI 每次都返回了 3~4 条带理由的高质量建议，但 evidence_refs 是标题字符串（如 ["报道：纽约市主计长敵促Anthropic暂停推进IPO"]）。根因链：renderKeyEventCandidateLines 候选行只要有标题就只渲染「依据：标题1；标题2」，article id 从不出现在 prompt（loadKeyEventCandidates 明明查了 id→title 映射）；system prompt 却奖约 evidence_refs:[<article id>...]（纯数字）；EvidenceRefs []uint 对字符串数组 json.Unmarshal 整体报错 → parseKeyEventSuggestions return nil → 100% 建议被丢弃。功能上线以来推荐从未真正生效，且因降级设计「失败不改日报」而无任何显性报警。
- 处置：① keyEventCandidate 新增 Evidence []KeyEventEvidence（id+title 成对），loadKeyEventCandidates 保留全部 id（无标题也裸 id 渲染）；② renderKeyEventCandidateLines 输出「依据 article id=标题：14157=xxx；14162」格式；③ system prompt 第3条明确「evidence_refs 必须填依据列出的 article id 纯数字，不要填标题文字」。
- 验证：新增 TestKeyEventRecommendation_CandidateLinesRenderEvidenceIDs 回归锚（断言 id=标题 对存在、裸 id 不丢弃、禁止退回裸标题）；service 包 27.3s 全绿；golangci-lint/vet 0 issues。真实端到端（AI 回数字 id → 落库 suggested_count>0）待下次日报生成后查 run 表复核，属 4.5 效果核对首项。

## 审毕无额外发现（审查范围：人工主权红线、指纹归一化、幂等/revision、并发与迟到批次、信号材料 attention 边界与快照冻结、前端请求锁/409 采纳/未知结果重读、组件零副作用/零嵌套按钮/突出开关仅 opacity、depguard 根包边界、表档与迁移逐项一致性——上述各项均确认无误，未列条目；R-8 为归档前用户查证「零建议」时发现，当轮修复）
