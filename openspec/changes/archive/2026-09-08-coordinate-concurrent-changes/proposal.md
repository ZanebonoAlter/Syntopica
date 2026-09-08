<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

并发 change 是本仓库工作流的常态（事实库证据：2026-09-03/09-04 均有两 session 分别绑定不同 change 重叠活跃 17 分钟以上；harness 类 change 需挂在树上等其他 change 执行过程中实战验证，脏树是流程固有属性而非纪律问题）。共享工作树上做归档验证因此产生三类问题：**重复执行**（多个会话对同一棵混合树各跑一遍全量 lint/typecheck/build/test）、**重复探索**（两个 change 涉及同批代码时 Agent 各自 read/grep 一遍）、**交叉污染**（别人的中间态让本 change 验证挂掉、doc-impact verify 把别人的改动误算进自己的域声明——现行 `doc-impact-excuse` 豁免机制正是对该问题的手工补丁）。根因是**会话/change 互相不知道对方的存在与文件归属**。git worktree 能根治但成本过高（Windows cmd 编译路径、node_modules 副本、Docker DB 共享资源、AGENTS.md 宪法已否决），需要轻量协调机制。

## What Changes

- **commit 与归档解耦**（流程约定，修订开发执行规范）：change 主体实现完成且增量门禁绿即做主体 commit（不等归档）；归档仍走原流程另起 commit。harness 类 change 挂树上等实战验证不受影响——commit 不改变磁盘文件，运行时行为照旧。实战验证发现问题的修复以 fixup commit 追加（可 revert，比树上脏文件丢弃可控）。
- **归属地图落库**（事实库新事件）：quality-gate 已有"会话内增量路径"检测，与 mode.set 的 boundChange 绑定关系聚合，将"本 change 的会话累计编辑过哪些文件"以新事件类型（`edit.map`）落 events.db。同文件被多个 change 触过时标记冲突。
- **态势拉取脚本** `scripts/concurrency-status.sh`（新增）：聚合输出活跃 change 清单 × 绑定 session × 最近活动、脏文件归属地图（含冲突标记）、近期 gate.check 验证流水摘要。拉模式（Agent 在 commit 拆分前/归档验证前/编排派发前主动跑），**不进 system prompt**——时变数据走注入通道会破坏前缀缓存（system prompt 尾部一变，整段对话历史缓存失效）。
- **spec-gate 归档并发检查**（新增检查⑤'）：归档时若树上存在归属其他 active change 的未 commit 文件，`warn` 提示（steer 消息通道，quality-gate 先例），不 block（归属是启发式，误 block 会卡死正常归档）。
- **doc-impact verify 按归属过滤**：verify 的"疑似遗漏"启发式输入从全树脏文件收窄为本 change 归属文件，消除"别的 active change 脏文件干扰"误报源；`doc-impact-excuse` 豁免机制随之退役（保留解析兼容，文档标注废弃）。

## Capabilities

### New Capabilities

- `concurrent-change-coordination`: 并发 change 协调机制——文件归属地图（change → 编辑文件集合的事实库聚合）、并发态势拉取（concurrency-status.sh 单一入口）、归档前并发检查（spec-gate warn）、commit 解耦流程约定（门禁绿即主体 commit）。

### Modified Capabilities

- `harness-fact-log`: 事件词汇表新增 `edit.map`（30 天保留期），payload 含 change 归属的文件路径集合与冲突标记；遵循既有"词汇扩展无 schema 迁移"先例（spill.write / policy.decision 同款）。
- `doc-impact-gate`: `verify` 子命令的"疑似遗漏"检查输入按归属地图收窄为本 change 文件；`doc-impact-excuse` 豁免机制退役。

## Impact

- **Extension 改动**（`.pi/extensions/`，gitignored，入库代码快照同步 `docs/research/`）：
  - `quality-gate.ts`：turn_end 增量路径检测处追加归属聚合落库（`edit.map` 事件）
  - `spec-gate.ts`：新增归档并发检查（⑤'，warn 级）
  - `lib/harness-log.ts`：事件词汇与保留期表扩展
- **脚本**：`scripts/concurrency-status.sh`（新增，bash + sqlite3 只读查询）、`scripts/doc-impact.sh`（verify 收窄输入）
- **文档**：`docs/reference/开发执行规范.md`（§0.6 编排六步加"派发前态势拉取"、§11 归档门禁说明、commit 解耦约定）、`.agents/skills/harness-facts/SKILL.md`（新事件类型与查询配方）
- **不涉及**：前端零改动；system prompt 注入通道零改动（concurrency 数据明确不走 constraint-injection）
- **部署影响**：无数据库迁移（events.db kind 为 TEXT 列，词汇扩展无迁移）；存量混合脏树需一次性人工大扫除（按 change 归属分批 commit），归属地图仅从启用时刻开始积累
