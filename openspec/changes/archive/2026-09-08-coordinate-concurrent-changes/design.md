# Design — coordinate-concurrent-changes

## Context

现状：共享工作树上多 change 并发是常态（harness 类 change 需挂树上等其他 change 执行中实战验证），归档验证对混合树全量执行，会话/change 间无存在感知与文件归属信息。已有基础设施：quality-gate（turn_end 会话内增量路径检测）、events.db（跨会话共享 append-only 事实库，kind 为 TEXT 列可无迁移扩词汇）、spec-gate（归档命令拦截，已有调用 bash 脚本的先例）、steer 消息通道（quality-gate 失败回喂先例）。

## Goals / Non-Goals

**Goals**
- 会话能以拉模式获知并发态势（活跃 change、脏文件归属、近期验证）
- 归档验证不再被其他 change 的脏文件干扰（spec-gate warn + doc-impact verify 收窄）
- commit 与归档解耦，脏树窗口收窄到"实现进行中"
- system prompt 注入通道零改动（前缀缓存零影响）

**Non-Goals**
- 不做 git worktree 隔离（宪法已否决）
- 不做验证结果硬缓存/自动复用判定（树指纹方案已否决，重）
- 不做主体 commit 资格的机器强制 block（流程约定 + 归档 warn 兜底即可）
- 不改 constraint-injection 注入内容与策略

## Decisions

### D1: 态势走拉模式，不进 system prompt

**选择**：concurrency-status.sh 单一入口 + spec-gate 检查复用；时变数据零注入。
**备选**：constraint-injection 追加注入（否决）——"只增不减"策略下每次追加导致该 turn 整段对话历史重 prefill；态势分钟级变化意味着每 turn 重算，缓存等于没有。拉模式在价值时刻（commit 拆分前/归档前/派发前）取到的是查询时刻最新态，比注入快照新鲜。
**成本**：Agent 需要记得跑（靠编排六步流程步骤 + spec-gate 挂点提醒，不靠自觉）。

### D2: 归属地图挂 quality-gate 增量检测，append-only 累计快照

**选择**：quality-gate turn_end 已做会话内增量路径 diff（基于 git，天然覆盖 edit 工具与 bash sed 等一切修改方式）。检出新增/变化路径且会话有绑定 change 时，追加一条 `edit.map` 事件：change 列 = 本会话最近一条 `mode.set` 的 boundChange（同 session 查库，无则跳过不落），payload = 该 change 名下累计路径全量集合（每次追加为完整快照，非增量 diff）。
**理由**：append-only 库禁止改写行，"每回合一条全量快照"是唯一无锁聚合形态；查询侧（concurrency-status / spec-gate / doc-impact）取该 change 最新一条即得完整集合，跨 session 先后绑同一 change（reload 场景）也自然合并。payload 量级：典型 change <100 文件 × <100B 路径 ≈ 10KB，SQLite TEXT 列无压力；40KB spill 阈值不适用（events.db 非工具输出）。
**备选**：独立 edit hook（否决——与增量检测重复造轮子）；地图存独立文件（否决——绕开事实库 TTL/审计/会话隔离机制）。

### D3: spec-gate 复用脚本子命令，逻辑单一实现源

**选择**：`concurrency-status.sh --check <change>` 输出机器可读结果（exit code + stdout JSON 行），spec-gate 归档检查⑤'调它；人读模式（默认）输出三段文本。SQLite 只读连接（`file:...?mode=ro&immutable=0`），WAL 下与写入方无锁竞争。
**理由**：归属判定逻辑（最新快照、冲突标记、冷启动跳过）只写一份，脚本与门禁不漂移。
**备选**：spec-gate 内嵌 TS 查询（否决——与脚本双实现，漂移后 warn 与人读结果对不上）。

### D4: 归档并发检查 warn 不 block，冷启动跳过

**选择**：树上存在归属其他 active change 的未 commit 文件 → warn（steer 消息，含文件与 change 名）；全部脏文件无归属记录（edit.map 上线前存量/纯 bash 会话）→ 跳过零输出。
**理由**：归属是启发式聚合（回滚/还原后地图陈旧必现），误 block 卡死归档不可接受；warn 级符合 fail-open 门禁分层。

### D5: doc-impact verify 双轨输入，excuse 退役

**选择**：归属集合非空 → 反向启发式只扫该集合；为空 → 回退现状全树 git diff。"声明了未更新"对账保持全树（文档文件以 git 为准，与归属无关）。既有 `doc-impact-excuse` 注释解析兼容不判 FAIL，新增不再需要。
**理由**：消除跨 change 误报源的同时，存量 change（无地图）行为零变化。

### D6: commit 解耦为流程约定，机器侧只做兜底提醒

**选择**：开发执行规范修订（§0.6 编排六步"派发前拉态势"步骤 + 主体 commit 约定 + §11 归档门禁⑤'说明）；无新增强制 gate。
**理由**："主体完成"本身有主观判断（Agent 自查），机器无法可靠判定资格；spec-gate warn 已兜底"忘了 commit 就归档"的主要漏网形态。

## Risks / Trade-offs

- [归属地图陈旧：文件被改回/还原后地图仍含该路径] → spec-gate 仅 warn 不 block，输出文件清单供 Agent 现场核对；下回合增量检测会再落新快照自然收敛
- [Agent 忘跑 concurrency-status] → 编排六步流程步骤（派发前）+ spec-gate 归档检查双挂点提醒，不依赖自觉；漏跑的后果上限 = 现状（无协调），非劣化
- [edit.map 事件量随回合线性增长] → 30 天 TTL 兜底；仅"检出变化路径"的回合才落（纯对话回合零事件），与 gate.check 同量级，量可控
- [多 change 共用 files（冲突文件）时主体 commit 拆分仍需人工判断] → 冲突标记醒目输出；§0.6 红线本就禁同文件并行，属长尾保险丝
- [脚本 sqlite3 版本差异（JSON1 扩展）] → 本机 sqlite3 已验证支持 json_extract（事实库既有查询配方依赖）；脚本内不依赖更冷门特性

## Migration Plan

1. **部署顺序**：extension 与脚本同批落地（edit.map 词汇 + 聚合 + 脚本 + spec-gate 检查），无 DB 迁移（kind TEXT 列）。
2. **存量大扫除**（一次性人工操作，部署后、下一轮并发前）：当前 30+ 脏文件按 change 归属人工分批 commit；无法归属的单独 stash 审视。归属地图仅从部署时刻积累，大扫除前的文件无归属记录（冷启动路径覆盖）。
3. **回滚策略**：extension 逐个可独立禁用（既有 fail-open 语义）；脚本无状态可删；events.db 新 kind 行对旧代码无害（未知 kind 被忽略）。excuse 注释保留在存量 tasks.md 中无副作用。

## Open Questions

（无——态势通道、归属数据源、检查级别、verify 双轨均在探索阶段与用户收敛）
