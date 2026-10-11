# add-review-trace-gate Design

考古基线（2026-08-23 ~ 2026-10-11，events.db）：gate.check 19,983 次 review 相关 0；~32 个归档 change 仅 ~6 个有 review 派发（~16%）；09-29 后 review 归零而 10 月仍归档 6 个；dispatch 事件 169 条 change 归属 0 条。修复分两层：C（可观测）+ A（门禁）。

## D1: 门禁判据用文件痕迹，不用事件账本（双轨解耦）

review-trace.sh 的归档判据 = change 目录 `review.md` 建账或 proposal.md 头部 waived 声明；dispatch 事件归属只喂 retro 统计，不参与门禁判定。

理由：events.db 的 dispatch 事件 TTL 30 天且历史行无归属，归档时点账本既查不全也查不稳（change 名靠 detectActiveChange 兜底判定，非权威）；review.md 随 change 归档永久保存，是唯一能承载「审过/没审」终局事实的载体。门禁要的是确定性判据，账本要的是过程可观测——两个诉求两个载体，不混。

备选（弃）：门禁查 dispatch 事件存在 review 派发——历史无归属直接全红，且「派发过 review」≠「review 意见有处置」，判据失真。

## D2: dispatch 归属与 rollup/complete 同源 detectActiveChange，不引入第二真相源

dispatch 落账处新增 `const changeName = detectActiveChange(ctx.cwd)?.name ?? null`，一次判定两用：写 `agentChangeBindings`（既有，供 complete 回填）+ 写 dispatch 事件 change 列。

理由：session.rollup 快照（274 行）与 agentChangeBindings（394 行）已用 detectActiveChange，spec 措辞「会话绑定的 change」实指此函数；dispatch 同源即与 complete 回填天然配对一致。不引入 `mode.set.boundChange` 查询路径（那是档位绑定语义，跨 session 查询引入时序耦合，收益为零——两判定在绝大多数时刻同值）。

既有行不可变约束兼容：新增行带值，不改写历史（harness-fact-log 既有条款维持）。

## D3: review.md 建账格式仿 human-feedback（一行正则全解析）

条目标题 `### R-<N> <日期>【High|Medium|Low】<标题>` + 状态行 `- **状态**：<词表内>`（五值封闭：待处理/已修复/已采纳/已驳回/误报丢弃）；等级/处置是条目级属性（一个文件多条、每条等级不同），标签放条目不放文件头——与 FB 条目同构。零发现显式声明行 `- 审毕无发现（审查范围：<说明>）`。文件不进 events.db（随 change 归档永久保存，账本有 TTL）。

理由：feedback.sh 的 add/verify 三件套已验证该格式可被 bash 一行正则稳定解析；review 意见与人类反馈同生命周期（运行中追加、归档时对账），复用格式降低双份心智。

## D4: waived 显式声明，不做「纯文档 change」自动判定

豁免唯一形态 = proposal.md 头部机器可读注释 `<!-- review: waived -->`。

理由：自动判定「纯文档」需要检测代码改动面，归档时 diff 已 commit、判定脆（改一行代码 vs 改十行文档的混合 change 无处落界）；显式声明把豁免责任留给声明者，waived 占比进 retro 体检卡后滥用可见。代码类 change 亦可声明（逃生语义对齐 spec-gate 的 `--force`：能力在、留痕在、责任在）。

## D5: 小修不豁免；「审毕无发现」把留痕成本压到一行

规范原文「单文件小修仍须走 review 不得跳步」维持——不新增小修豁免档（开了就会全走小修，10 月考古已证明）。配合 D3 的无发现声明行，「审了没问题」的记录成本 = review.md 一行，区分审过/没审是门禁唯一目标，不为内容质量加码（内容质量靠 review agent 与 author 纪律，见规范 receiving-code-review）。

## D6: 检查项编号整理（spec-gate 与 archive-readiness 双侧）

spec-gate：block 级四项 → 五项（新增 review-trace 为第五项），注释与豁免文案「五项归档检查」→「六项归档检查」枚举更新；warn 级检查⑤（措辞扫描）名称不变，UI 验收证据（原⑤'）顺延为⑥'。archive-readiness.sh：硬检查输出 ⑤ review-trace、human-feedback 提醒层 ⑤→⑥（不阻断语义不变）。

理由：编号只是文档事实，跟随插入位置顺延；唯一硬约束是 spec-gate 豁免放行文案必须枚举全部检查项（既有 spec 条款），改注释时逐处同步。

## D7: retro 覆盖段口径与历史边界

review 类派发计数口径：`kind=subagent.dispatch AND change=<name>` 且（`type='change-review-glm'` 或 `desc` 含 review/审查关键词）——标注为参考指标（desc 关键词有误报面，如「修复 review M1」计为修复类而非 review 类，按 desc 含「修复」优先归类修复）。痕迹三态（review.md/waived/皆无）为权威口径。dispatch 归属改造落地前的时段显示「归属改造前无数据」，不输出误导性 0。

理由：统计段的职责是让缺口可见而非精确计量；权威口径（文件痕迹）与参考口径（派发计数）分层展示，避免 desc 误报污染结论。

## 风险与边界

- **spec-gate 拦截存量 in-flight change**：仿 scenario-trace-gate 先例不豁免——存量 change 归档前补建 review.md（一行业无发现即可），成本可控。
- **review agent 产出转建账**：change-review-glm 的三段式报告由主线程落成 review.md 条目（brief 的输出要求与本建账格式对齐，实现阶段在 brief 模板注明），不要求 review agent 自己写文件（只读纪律不动）。
- **detectActiveChange 多 change 并发误归属**：并发 change 共享工作树时 mtime 兜底可能归属到相邻 change——与 rollup/complete 既有语义一致，参考指标容错；门禁判据（D1）不受影响。
- **零测试缝**：review-trace.sh 直接吃 change-dir 参数（同 scenario-trace.sh），smoke 用临时目录构造 fixture，无需环境变量测试缝。
