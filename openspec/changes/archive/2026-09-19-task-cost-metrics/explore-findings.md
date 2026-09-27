
## task-cost-metrics 实现关键事实

实现落点（2026-09-19）：

**写入侧**
- `lib/session-rollup.ts`：`SessionUsageSummary.models: Record<string, {tokens 五值, cost}>`，`consumeLines` assistant 分支按消息级 `message.model` 归属（实测字段在 `d.message.model`，与 model_change 行顶层 `d.modelId` 不同层），缺字段计 `unknown` 桶；`sessionFileIdOf`/`readParentSessionId`（读首行 session 头 `parentSession` 完整路径→basename UUID）/`findPrevSessionFile`（mtime 最新、排除当前会话与提不出 UUID 条目、并列 mtime 取文件名字典序大者保确定性）。
- `harness-telemetry.ts`：turn_end payload 附 `models` + `cachedParentSessionId`（undefined=未解析/null=主会话，省略键 fail-open）；session_start 回填自扫 `path.dirname(getSessionFile())` 目录下 `*.jsonl`（彻底不用 `event.previousSessionFile`）；回填 payload 同样附 models+parentSessionId；tool_call(bash) 命中 `openspec\s+archive` 暂存 `archiveCalls: Map<toolCallId, command>`，tool_result(bash) 配对非 isError → `decideArchiveEvent` → `logEvent(change.archive, change=name, payload={name})`；`sessionIdFromSessionFile` 改为 re-export `sessionFileIdOf`（同源唯一实现，smoke 入口不变）。
- `lib/archive-accounting.ts`（新）：`isArchiveCommand`/`extractArchiveChangeName`/`decideArchiveEvent` 纯函数，正则语义同 spec-gate `extractChangeName`——注意带引号形态（`bash -c 'openspec archive foo'`）提取名含尾引号不合法 → 零记录，这是 spec-gate 语义本身，非 bug。
- `lib/harness-log.ts`：`change.archive` 入 kind 联合类型 + RETENTION_DAYS 30 天。

**读侧（harness-retro.sh）**
- bash 侧装配 `ARCH_JSON`：扫 `${HARNESS_RETRO_ARCHIVE_DIR:-openspec/changes/archive}`，目录名 `YYYY-MM-DD-name` 且日期 >= 窗口起点，同时读 proposal.md 头 complexity 注释；smoke 用该环境变量注入 fixture（smoke 全程 export 指空目录防误读真实 archive）。
- SQL CTE 链：`ru_px`（ru 终值 + cost/parent/models_json）→ `ru_attr` 三路归因（chg 直归 / parent 归父一层不递归 / NULL=unattributed）→ `tc`（归因 ∩ arch_all 双锚点并集 GROUP BY Σcost）；`tm_flat`（json_each(models_json) 展开）+ `tm_nom`（models_json IS NULL → 未分模型桶）；`tcx`（LEFT JOIN cx_map，缺 → 未声明）；`arch_evo`/`arch_diro` 锚点不对称计数。
- json `effectiveness.task_cost` 段 + metrics 键 `m7.task_cost_p50/p75/avg/count/by_model/by_complexity/unattributed_cost_pct/active_unarchived`（by_model/by_complexity 为 dict，基线 delta 自动 None 安全）。
- 真实库基线（2026-09-19，7 天窗）：15 个归档任务 P50 ¥3.92 / P75 ¥11.56 / 均值 ¥5.17；未分模型 100%（存量无 models，随换代衰减）；活跃未归档 7；仅目录侧 35（change.archive 事件自本 change 起新记）。

**测试锚**
- session-rollup.smoke 23 组、telemetry-archive.smoke 7 组（mock pi + 临时库集成，现场 esbuild bundle 因 harness-telemetry 相对导入无扩展名不可直载）、harness-retro.smoke 63 断言（fixture K1~K6 四场景全数值复算）。

**遗留**：任务 19（新开 pi 会话触发回填、final=true 计数增长）须本会话结束后人工验证。

<!-- pinned 2026-09-19T05:53:07Z -->
