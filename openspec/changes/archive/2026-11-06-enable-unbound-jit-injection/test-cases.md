# Test Cases: enable-unbound-jit-injection

> 复杂度 complex（design.md 已声明）。测试单元 = 「未激活档会话一改业务代码，对应域红线就自己到手」用户故事。主链路表串节拍，变体走查 + 效果核对 + 白盒附加见后。
> 扩展层落点在 `.pi/extensions/tests/constraint-injection.smoke.cjs`（stub typebox + esbuild 打包扩展 + 真实事件形状回放，事件为 `tool_execution_start`、路径取 `args.path`）；shell 落点见步 13-14。

## 主链路表（故事：无档会话与无档子线程按编辑路径自愈）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 未激活档 `tool_execution_start`（toolName=edit，args.path=`backend-go/internal/platform/airouter/xxx.go`） | 未激活档编辑命中业务路径 | 动态层送达 ai-summary.md 完整节；`constraint.inject` 记 `reason=jit-path`、`tier="none"` | 扩展 smoke | `unbound-jit-1` |
| 2 | 未激活档 edit `front/app/features/articles/...`（只命中 `standard/frontend/layout.md`） | 未激活档编辑命中业务路径 | 送达 layout.md 节（standard 文档同样走 JIT，不限 flow） | 同上 | `unbound-jit-2` |
| 3 | 未激活档 edit `scripts/db/xxx.sh`（无任何标签） | 未激活档编辑未映射路径 | 零动态层发送、零 `constraint.inject` | 同上 | `unbound-jit-3` |
| 4 | 未激活档连续 edit 同域两个文件 | 未激活档同域多次编辑只注一次 | 该文档只追加一次，第二条消息零增量（指纹去重） | 同上 | `unbound-jit-4` |
| 5 | 未激活档用户输入含「日报」「定时任务」 | 未激活档关键词通道保持关闭 | 不注入 daily-report / scheduler 节 | 同上 | `unbound-jit-5` |
| 6 | 未激活档 edit change 临时目录文件，该路径由夹具临时标签命中 | 未激活档 JIT 不绑定 change | 追加约束投递；`boundChange` 仍空、事实库无 `mode.set` | 同上（临时夹具 + 事实库断言） | `unbound-jit-6` |
| 7 | 未激活档 JIT 注入前后比对 `before_agent_start` 的 systemPrompt | 稳定层档位生命周期内恒定（无档变体） | 稳定层字节完全一致（仅索引，快照键 `none|none`） | 同上 | `unbound-jit-7` |
| 8 | 未激活档命中 daily-report（节 46.3KB）、`budgetBytes=32768` | 未激活档 JIT 受预算降级 | 该条降为占位行；消息头列降级路径；文档仍有「标题 + read 路径」 | 同上（改配置夹具） | `unbound-jit-8` |
| 9 | 子线程（`parentSession` 可证、父 `mode=null`）edit 命中 ai-summary 文件 | 子线程未继承档位时按自身路径自愈 | 子线程收到 ai-summary 节 steer 消息（不依赖主线程 brief） | 同上（子会话 ctx 夹具） | `unbound-jit-9` |
| 10 | 任一未激活档 JIT 注入 | 记账可归因 | `constraint.inject` 行：`reason=jit-path`、`tier="none"`、`mode` 仍为文档形态（`section`/`full`），无 `mode.set` | 同上（事实库 sqlite 夹具） | `unbound-jit-10` |
| 11 | 未激活档 + `channel=legacy` edit 命中 | legacy 通道不即时投递 | 本 turn 零 steer；下一 turn `before_agent_start` 的 systemPrompt 含该文档 | 同上（配置夹具） | `unbound-jit-11` |
| 12 | 生命周组合：无档主会话 compact / 无 `before_agent_start` 子线程 compact / 同进程 fork / reload / 同文档重复编辑 | 五条生命周期 Scenario | 主会话重发；子线程不重发且**同文档重复编辑不重投**；fork 不重投父已送达节；reload 后 `jitDocHits` 清零 | 同上（事件回放） | `unbound-jit-12` |
| 13 | `bash scripts/harness/constraint-digest.sh --paths <5 条真实路径>`（清单见 tasks 7.3） | 路径标签补全验收 | 按输入顺序命中：reading / semantic-board / discovery / semantic-board / ai-summary；改前前三条为「未命中」 | shell | `constraint-digest.smoke.sh` 增段 |
| 14 | `bash scripts/harness/harness-retro.sh` | 可观测口径 | 输出含「⑦H 无档 JIT」段：条数/会话数/平均域数/送达字节/降级占比（字段必须出现，值允许 0） | shell（sqlite 夹具） | `harness-retro` 探针段 |

## 继承与调整（⓪ 契约盘点）

改动契约：`constraint-injection` 的「未激活档仅索引」→「稳定层仅索引 + 编辑路径 JIT 追加」；`constraint.inject` payload 追加 `tier`（append-only）。

| 旧 Scenario / 旧断言 | 处置 | 旧测试 | 动作 |
|---|---|---|---|
| Scenario「未激活档仅索引」 | **MODIFIED**（拆为「稳定层仅索引」+「关键词通道保持关闭」） | smoke 第 8 号「JIT 门控: 未激活时写 airouter 不注入 ai-summary / ai-logging」 | **反转断言**：改为「写入后注 ai-summary」，并新增未映射路径零注入对照 |
| Scenario「未激活档不注 flow 约束节」（input 关键词） | 不变 | smoke L199-200 | 保留，补步 5 断言 |
| 稳定层快照 / 指纹 regime 语义 | 不变 | smoke regime 用例组 | 保留全跑 + 步 7/12 增无档变体 |
| `constraint.inject` payload 结构 | 追加 `tier`（不改 `mode`） | smoke L355-367 的 sqlite 断言 | 断言改为同时校验 `mode`（文档形态）与 `tier` |
| doc-authoring.md「仅档位激活时生效」描述 | 文档同步 | 无自动化 | 文档任务 6.2 |

`bash scripts/harness/test-assets.sh constraint-injection` 反查旧资产清单（已跑，输出主 spec 全部 Requirement/Scenario），归档前逐行核对。

## 变体走查（五组）

- **输入**：路径带尾斜杠 / 文件全名 / 扩展名碰撞（标签 `X.ts` 会命中路径 `X.tsx`，步 13 矩阵断言）/ 空路径 / Windows 反斜杠 / 相对 vs 绝对路径 → 均断言命中集等于预期或显式不命中。
- **前置**：空 `jitDocHits`；集合已含同文档（去重）；一次 edit 同时命中多域（topicgraph 命中 3 域是合法结果，条数 = 域数）；带标签文档共 17 份（注入条数上界，非 10）。
- **时间窗口**：本 change 不涉及时段/当天口径，划除。
- **幂等**：同文件连续 edit 2 次；同 turn 多次 edit；fork 继承父会话 `jitDocHits`（步 12 断言不重投）；reload 后清零。
- **可用性**：无 UI，划除；改为断言 steer 消息头部受控（折叠形态、含路径与域标签）。

## 效果核对（问题 4）

- 触发原因：`harness-retro` ⑦G 派发带域比例 29%（31/107）+ 无档会话裸奔 + 路径反查 21/36 一级目录无覆盖。
- 方法：
  - `constraint-coverage.sh` 改前/改后各跑一次，比「样本清单中未被任何域覆盖的条目数」与 5 条探针的命中域集合；
  - 单次注入分别记录**原始字节 / 送达字节 / 是否降级**（实测节字节 3.4–46.3KB，见 design D4 表；daily-report 46.3KB 必降级）；探针统计只用送达字节，不统计原始字节（payload 无该字段）；
  - smoke 断言计入（步 1-12）。
- 量化结果（目标）：样本未覆盖条目数下降且新增命中逐条确认；5 条探针中改前未命中的至少 3 条转命中；降级占比记录为基线（不设上限）。
- 结论写入归档前 verify 报告（本文件不预填结果，避免自证）。

## 白盒附加（分支表 + 边界值）

| 分支 | 边界值 | 用例落点 |
|---|---|---|
| 档位判定 | `mode = null` / implementation / requirements | 步 1 / 5（对照） |
| 通道 | `mixed` / `legacy`（legacy 本 turn 不投、下一 turn 进 system prompt） | 步 11 |
| 预算 | 0（禁用）/ 2048 / 32768 / 单域 46.3KB 超线 | 步 8 |
| 标签窗口 | 第 15 行命中 / 第 16 行不命中 / 头部两条标签只取首条 | 复用既有 fixture 用例 |
| 去重 | 同文档 1 / 2 / N 次 edit | 步 4 |
| 路径匹配 | 目录尾斜杠 / 文件全名 / `.ts` 标签命中 `.tsx` 路径（碰撞面）/ 多域重叠（3 域） | 步 13 矩阵 |
| 生命周期 | 主会话 compact / 子线程 compact / fork / reload | 步 12 |

不适用划除：时间窗口与 UI 状态走查（无 UI、无时段逻辑）。
