
## 跨会话注入污染的事实库证据（复现序列）

本 change 的证据基线（harness 事实库 `.pi/harness/events.db`，2026-09-17 实测）：

**会话 `01a0aaf4` 的完整时间线**（`select id,ts,kind,change,payload from events where session_id like '01a0aaf4%' order by id`）：
- 26643 16:02:56 `session.start`（reason=startup, prev=None）
- 26644/26645 16:02:56 `constraint.inject` change=**linux-native-dev-environment**（mode-base + jit-path test-design）
- 26682/26683 16:11:32 `pin.write` change=**heal-dangling-article-refs**（← 该会话真正在做的事）
- 26855 17:09:37 `session.start`（reason=startup，**同 sessionId 第二次**）
- 26859-26863 17:10:04 `constraint.inject` change=**dedupe-rss-articles**，其中 reason=declaration 的两条是 `content-enrichment.md`(layer=redline,1618B) + `topic-graph.md`(redline,1143B) —— 那是 dedupe-rss-articles 声明的域
- 27036 23:50:17 `constraint.inject` change=**harness-retro-loop**
- **该会话零 `mode.set` 记录**（`mode.set` 只属于别的会话）

**跨会话借用的直接因果**：26856（=会话 `01a0ab11`，17:09:58）`mode.set` `{mode:requirements, boundChange:dedupe-rss-articles, source:edit-dir}` ——**6 秒后** 01a0aaf4 的注入就带上了 dedupe-rss-articles 与其声明域。即「他会话最后绑定 = 本会话注入归属」。

**全库口径**：会话级「有 constraint.inject 但零 mode.set」的会话 10+ 个（`01a0a01e`/`01a0a097`/`01a09da6`/`01a09d42`/`01a09ba1`… 归属 improve-discovery-recommendations 或 offline-catchup）。

**注入事件列名**：events 表是 `id, ts, session_id, kind, change, payload`（没有 name/change_name 列，早期查询会报 no such column）。

**引用**：backend-go/internal/platform/articlerefs/articlerefs.go、.pi/extensions/constraint-injection.ts

<!-- pinned 2026-09-17T00:21:37Z -->

## extension 会话作用域状态与测试入口（实现契约）

`.pi/extensions/constraint-injection.ts`（2017 行）的会话作用域状态与读写点实测清单：

**模块级全局状态（要收进 per-session 容器）**：`:141 let currentMode`、`:143 let modeBoundChange`、`:145 let recentInputs`、`:147 let jitDocHits`、`:150 let keywordDocHits`、`:177 let turnBindLocked`；另有 `pinReadSeen`、稳定层快照/指纹/`pendingSnapshot`（由 `resetChannelState()` 整体重置）。

**读写点**：`session_start` = `:1544-1589`（startup 分支在 `:1549` 判 `currentMode === null` → `queryBySession` 恢复 → `:1557-1560`；new/resume/fork/reload 分支 `:1567-1571` 全域清零 + `:1585-1588` recover）；`input` = `:1591-1624`（入窗 `:1599-1601`、命令绑定 `:1605-1620`）；`tool_execution_start` = `:1626-1701`（skill 绑定 `:1644-1656`、JIT 命中 `:1663-1668`、绑定修正 `:1675-1690`、JIT 即时投递 `:1695-1701`）；turn 收尾 `:1704-1760`（`:1723 turnBindLocked=false`、`:1728-1733` 绑定目录消失回落、`:1747-1753` mtime fallback）；`before_agent_start` 注入计划 `:1780-1810`（`:1790 const key = stableSnapshotKey(currentMode, modeBoundChange)`）；`declaration` 域选择 `:1940-1955`（`:1946-1947` 用 `modeBoundChange` 找 change 目录）。

**关键既有语义（必须保住）**：`session_start` 的 startup 分支有 @bugfix 注释——pi-subagents 派发子线程时 `createAgentSession` 也发 `session_start{reason:"startup"}` 到**同一共享模块实例**，靠「`currentMode` 非空则不动」实现**子线程继承主会话档位**（注释原文：「子线程继承主会话档位是期望行为（快照注入能带上完整约束块）」）。所以按会话隔离时必须显式提供 fork/子线程继承路径，否则会打断 apply 场景的约束注入。

**测试手段**：`bash .pi/extensions/tests/run-harness-smoke.sh`（先 esbuild 把各 `.ts` bundle 到 `.cjs` 再跑 `*.smoke.cjs`）；constraint-injection 的用例在 `.pi/extensions/tests/constraint-injection.smoke.cjs`（既有 40+ 用例含「全新 pi 会话启动不继承其他会话档位」「无自身档位历史的会话 reload/resume 不继承他窗口档位」「子线程派发不清零主会话档位」）；另有 `run-smoke.sh` 跑记账类套件。

**引用**：.pi/extensions/constraint-injection.ts、.pi/extensions/tests/constraint-injection.smoke.cjs、.pi/extensions/tests/run-harness-smoke.sh

<!-- pinned 2026-09-17T00:21:42Z -->
