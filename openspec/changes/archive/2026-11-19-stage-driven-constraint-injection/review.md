# stage-driven-constraint-injection 审查记录

审查模型：`zai-coding-cn/glm-5.3:max`。只读审查制品及相关 diff；结论：1 High、3 Medium、7 Low。修复 High 前不建议进入 verify 或归档。

### R-1 2026-11-18【High】切到 legacy change 后读取到旧阶段和旧 change
- **状态**：已修复（stageSetInfoFromRow 保留合法 stage=null 行，仅跳过损坏/矛盾行；双读/resume/跨进程继承 6 条新断言全绿）
- `.pi/extensions/lib/harness-log.ts:428–479` 跳过合法的 `stage:null` 行。GLM 用临时库复现：先绑定 A（implement），再绑定 B（legacy），查询错误返回 A。影响 UI/用例门禁、恢复会话和跨进程继承；edit-map 当前绑定归属不受影响。应保留这条空阶段状态，只跳过损坏记录，并补回归断言。

### R-2 2026-11-18【Medium】归档读取证据未限定当前 change 的实现期
- **状态**：已修复（对账加 change 归属过滤 + 实现期窗起点；archive-readiness.smoke ⑩c–⑩e 绿/红反例）
- `scripts/harness/constraint-read-trace.sh:218–260` 按路径和生命周期时间窗查询。其他 change 或计划期的读取也可能通过对账，与 delta spec 不一致。

### R-3 2026-11-18【Medium】整节回退与注入体量目标冲突
- **状态**：已修复（回退改为取数指引行，零正文；spec/proposal/constraints-index/pi-extensions 同步；≤4000B 断言落地）
- `.pi/extensions/constraint-injection.ts:1124–1164` 在红线缺失或不足 512B 时回退整节；这符合部分 delta 条款，却与自主读取目标及 `tasks.md` 9.3 的 4000 字节上限冲突。

### R-4 2026-11-18【Medium】read 发起即被算作已读
- **状态**：已修复（记账挪到 tool_execution_end 且非 error，经 toolCallId→path 在途登记；失败/未回执断言绿）
- `.pi/extensions/constraint-injection.ts:2194–2199` 在工具执行前记账，失败读取也会计入；局部读取的合格条件未定义。

### R-5 2026-11-18【Low】任务清单与实际完成度未同步
- **状态**：已修复（逐项核对后勾选 37/39；余 9.1 人工链路、10.5 归档时项；过时措辞 3.5/4.5/5.1/9.2/9.4 一并修正）
- 按 `tasks.md` 逐项核对实测结果后再勾选；不要将场景映射存在等同于验证通过。

### R-6 2026-11-18【Low】事件词表漏列既有巡检事件
- **状态**：已修复（词表改十七类，补 mode.set legacy 与 patrol.check；stale Scenario 正文同步改 stage.set）
- `specs/harness-fact-log/spec.md` 的事件词表没有 `patrol.check`；核对计数和 legacy 行的口径。

### R-7 2026-11-18【Low】记账 Requirement 标题仍引用退役事件
- **状态**：已修复（经 ## RENAMED Requirements 改名「阶段记账（stage.set）」，openspec validate --strict 通过）
- `specs/harness-fact-log/spec.md`「档位记账（mode.set）」正文已改为 `stage.set`，标题需同步。

### R-8 2026-11-18【Low】pin_finding 可见说明仍使用旧档位术语
- **状态**：已修复（description 改「有活跃 change / 无活跃 change」措辞）
- `.pi/extensions/constraint-injection.ts` 中 `registerTool` 的 description/promptSnippet 仍用旧术语。

### R-9 2026-11-18【Low】路由渲染含未使用参数
- **状态**：已修复（删除未使用的 stage 参数，调用点同步）
- `.pi/extensions/constraint-injection.ts:1103–1121` 的 `renderRoutingList` 不使用 `stage` 参数。

### R-10 2026-11-18【Low】压缩后重发与 delta 措辞不一致
- **状态**：已修复（spec 措辞对齐实现：compact 重发=动态层当前有效集合，稳定层常驻 system prompt 不受影响）
- `.pi/extensions/constraint-injection.ts:2437–2446` 只重发动态层；稳定层仍常驻，但未另外重发 spec 所说的稳定层摘要。

### R-11 2026-11-18【Low】阶段标记实际允许出现在正文
- **状态**：已采纳（保持全文恰好一条的严格解析——比仅头部更严，防正文示例伪造；spec 补澄清句）
- `.pi/extensions/lib/stage.ts:100–107` 和 `scripts/harness/constraint-read-trace.sh:118–140` 全文搜索阶段标记，与文件头约定不一致。

## 审查验证边界

OpenSpec 严格校验、harness-log / spec-gate / quality-gate / quality-gate.behavior / ui-design-gate smoke 通过；无声明和 legacy 两条归档对账路径通过。真实 Pi 会话切阶段、上线后事实库样本、体量上限未验证；只读 reviewer 未运行会在工作树创建临时夹具的主注入 smoke。
