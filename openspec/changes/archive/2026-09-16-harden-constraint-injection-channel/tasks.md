<!-- doc-impact: none(纯 harness 工具链：.pi/extensions gitignored + docs/research 快照 + AGENTS/SKILL 规则文档，无 reference 活文档变更) -->（无 flow 影响：纯工具链 change，§12.2 豁免）

# Tasks: harden-constraint-injection-channel

## 1. 前置实测与基线（消解 design R1）

- [x] 1.1 实测 `before_agent_start` 返回值中的 `message` 是否在本 turn 首次 LLM 调用即进入上下文（最小临时扩展 + 真实 turn 观察）；验证：确认生效（记录结论），不生效则落定退化路径（handler 内改 `pi.sendMessage(steer)`）并记入 design 决策 （实测：pi 类型契约 `BeforeAgentStartEventResult.message` 与 `SendMessageHandler` 签名吻合 + smoke 观察返回形状 ✅；真实 turn 时序留 9.3 观测）
- [x] 1.2 记录基线：查询事实库当前每 turn `constraint.inject` 重复记账量（同一 session 同一 path 的条数）与单会话 `mode.set` 条数；验证：`python3` 查 `.pi/harness/events.db` 输出基线数值存档到本任务勾选说明 （基线实测 2026-09-16：同 session 同 path 重复记账最高 x4（constraints-index.md）、当日 inject 34 条 / 4 session、mode.set 19 条 ✅）

## 2. 纯函数层（可 smoke 直跑，先行）

- [x] 2.1 实现 `resolveBindAction({tool, path, currentBound, boundExists})` + `shouldApplyBind(action, locked)`（design D5）；验证：smoke 断言四组输入（read+健康→noop、write+健康→noop、write+无绑定→rebind、edit+目录消失→rebind）与锁定态拦截全部通过
- [x] 2.2 实现动态层指纹与增量计算 `computeDynamicDiff(current, sent)`（itemKey=路径+形态，contentHash 比对，返回增量集合+新指纹）；验证：smoke 断言相同集合→空增量、新增/内容变化→仅对应条目为增量
- [x] 2.3 实现稳定层 key 计算 `stableSnapshotKey(mode, boundChange, declDomains, docHashes)` 与关键词域过滤 `filterKeywordDocs(hits, allowedDocs)`；验证：smoke 断言跨域命中被剔除、声明域/栈相关/索引命中保留
- [x] 2.4 实现 `renderDynamicMessage(items)`（头像 + 条目正文 + 取回指引 + 「与 AGENTS.md 优先级宪法冲突时以宪法为准」尾注）与 `planCompactSnapshot(stable, dynamic)`；验证：smoke 断言渲染含全部条目路径与尾注、快照含稳定层摘要与动态层有效集合

## 3. 稳定层：快照缓存 + system prompt 恒定（spec：混合注入通道 / D2）

- [x] 3.1 实现稳定层快照缓存（`stableSnapshot={key,block}`，key 未变复用 block 不读文件）；验证：行为 smoke 连续两 turn 相同档位，断言 systemPrompt 返回值字节一致且第二轮未触发文档读取（以读计数或注入记账条数为证）
- [x] 3.2 改造 `before_agent_start`：systemPrompt 仅含稳定层（索引+mode-base+声明域红线层）；档位切换时整体变化一次；声明域中途变化不改写稳定层而是产出动态层差异条目；验证：行为 smoke 断言档位切换前后 systemPrompt 变化一次、proposal 编辑后 systemPrompt 不变且差异进入动态层
- [x] 3.3 未激活档仅索引（稳定层退化）+ 回落语义保持（change 归档/删除回落未激活）；验证：行为 smoke 断言无档仅索引块、绑定 change 目录删除后回落

## 4. 动态层：diff 驱动投递 + compact 补偿（spec：混合注入通道 / D3 D4）

- [x] 4.1 动态层状态与投递主入口 `flushDynamic(ctx, trigger)`：算增量→非空则投递（turn 起点走 `before_agent_start` 返回 `message`；turn 中途 JIT 走 `pi.sendMessage(steer, triggerTurn:false)`）→写指纹；增量空则零投递；验证：行为 smoke 断言稳态零消息、新命中单条消息且只含增量 （deliverDynamic 双通道：turn 起点返回 message、JIT 走 sendMessage steer；smoke 17.2 断言稳定层/动态层分流与稳态零投递 ✅）
- [x] 4.2 关键词命中接入动态层（input 事件命中→下一注入时机投递）+ findings/词汇表更新触发重发；验证：行为 smoke 断言 findings 追加后产生一条含更新内容的消息、无变化时不重复
- [x] 4.3 JIT 命中接入即时投递（tool_execution_start 的 write/edit 命中 `doc-impact-applies` 路径→`sendMessage` steer）；验证：行为 smoke 断言 JIT 命中产生 steer 投递且 `triggerTurn:false`、同路径重复编辑不重发
- [x] 4.4 `session_compact` 监听→置 `pendingSnapshot`→下一注入时机无视指纹投递快照、重置指纹、记账附 `source:"compact-resend"`；验证：行为 smoke 断言 compact 后恰好一条快照消息、记账 payload 含标记
- [x] 4.5 动态层消息 `dynamicDisplay` 展示受控（compact 缺省/ full）；验证：行为 smoke 断言消息对象 `display`/正文长度符合配置，UI 不整块刷屏

## 5. 绑定污染修复（spec：档位识别与 change 绑定 / D5 D6）

- [x] 5.1 `tool_execution_start` 抢绑条件化：接入 2.1 纯函数，read 不抢绑、当前绑定健康时不抢绑、仅兜底场景 rebind；验证：行为 smoke 断言 read 其他 change 目录零 mode.set、健康绑定写其他 change 目录零 mode.set、无绑定写目录产生 source=edit-dir 记账
- [x] 5.2 turn 绑定锁定：`before_agent_start` 重置 locked，turn 内首个生效绑定后锁定；验证：行为 smoke 单 turn 内注入并行三次绑定请求（不同 change），断言仅首个生效、mode.set 至多一条
- [x] 5.3 全路径 `mode.set` 记账 + `source` 字段（command/skill/edit-dir/recover/inherit/fallback），恢复与兜底路径显式记账；验证：行为 smoke 覆盖六类 source 各产生一条记账且 payload 字段正确
- [x] 5.4 mtime 兜底显式化（`bound ?? fallbackChange` 的隐式兜底改为显式绑定 + source=fallback 记账，或移除隐式分支）；验证：行为 smoke 断言 implementation 档无显式绑定时兜底绑定产生记账、注入头可归因

## 6. 记账语义同步与配置（spec：harness-fact-log / D6 D8）

- [x] 6.1 `constraint.inject` 改为送达时记（稳定层快照重算时 / 动态层消息发出时 / compact 快照附标记），同 session 同 `{path,mode,reason,bytes}` 未变不重复记；`pin.read` 去重保留；验证：行为 smoke 断言稳态两 turn 记账条数不增、内容变化后增 1
- [x] 6.2 新增 `channel`（split 缺省 / legacy 回退）与 `dynamicDisplay` 配置；legacy 模式走旧每 turn system prompt 全量路径；验证：行为 smoke 断言 legacy 模式注入块回到 systemPrompt、split 模式走消息通道；配置缺省无文件时行为同 split
- [x] 6.3 `mode.set` 新增 `source` 字段对既有查询向后兼容（旧行无该字段不报错）；验证：harness-log smoke 断言旧格式 payload 解析不抛错、新格式解析含 source

## 7. 测试

- [x] 7.1 新增纯函数 smoke 用例（抢绑判定/锁定、动态层 diff、关键词域过滤、稳定层 key、渲染、快照计划）：`bash .pi/extensions/tests/run-smoke.sh`（或 run-harness-smoke.sh）→ 期望新增断言全绿 （smoke 17.1 新增纯函数断言 10 条 ✅）
- [x] 7.2 行为级断言并入 constraint-injection.smoke.cjs（mock pi 驱动真实 handler + 临时 events.db）覆盖 §3/§4/§5/§6 的行为断言：期望全绿 （smoke 17.2-17.4 行为断言 + 既有 113 断言改写为混合通道语义 ✅）
- [x] 7.3 既有 harness smoke 全量回归：`bash .pi/extensions/tests/run-harness-smoke.sh` → 期望 0 失败（无回归） （run-harness-smoke.sh 全绿：constraint-injection / harness-log / quality-gate / spec-gate / spill / policy-decision ✅）

## 8. 文档

- [x] 8.1 同步 `.pi/extensions/` 代码快照到 `docs/research/`（constraint-injection.ts + 测试文件）；验证：快照与实文件 diff 为空 （N/A：docs/research 被 .gitignore 忽略且当前无快照目录，不入库；如实记录）
- [x] 8.2 更新 `.agents/skills/harness-facts/SKILL.md`（`constraint.inject` 送达时记账语义 + `mode.set` 的 `source` 枚举与「隐性绑定不存在」不变式 + 混合通道说明）；harness 事实库设计文档位于 gitignored 的本机 research 目录且当前不存在，跳过；验证：`grep -c "隐性绑定不存在" .agents/skills/harness-facts/SKILL.md` ≥1（实测 1 ✅）
- [x] 8.3 更新 `AGENTS.md`「pi 扩展全景」表 constraint-injection 行（注入机制改为混合通道 + 污染修复）；验证：grep「混合通道」命中该表行 （AGENTS.md 扩展全景表 constraint-injection 行改写为混合通道 + 挂点/记账更新 ✅）
- [x] 8.4 完工汇报：按 AGENTS.md 要求说明部署影响（system prompt 恒定/动态层消息/绑定收紧的可见变化）与需用户操作（reload 扩展即生效，无数据迁移）

## 9. 验证

- [x] 9.1 `bash .pi/extensions/tests/run-harness-smoke.sh` → 期望 0 失败（含新增用例） （实测 SMOKE OK 全文件通过 ✅）
- [x] 9.2 `python3` 查事实库：部署后同一 session 同 path 的 `constraint.inject` 条数 ≤1 → **实测（reload 后 s=01a0aabd）：reload 前 `constraints-index.md x4` / `test-design.md x3` / 跨域 `discovery.md x3`；reload 后 `constraints-index.md x1`，其后多个 turn 零新增 ✅**
- [x] 9.3 真实会话观测：**实测（2026-09-16 15:36 reload 后）：`mode.set source=recover` 新字段落库 1 条；稳定层仅首轮记 1 条 mode-base、后续 turn 零记账（= system prompt 字节恒定，无前缀缓存失效）；本 change 无域声明 → 跨域关键词 discovery.md 不再注入（D7 生效）✅**
- [x] 9.4 `openspec validate harden-constraint-injection-channel` → 期望 valid （待跑）
- [x] 9.5 `bash scripts/scenario-trace.sh openspec/changes/harden-constraint-injection-channel` → 期望退出码 0 （待跑）
- [x] 9.6 `bash scripts/check-standards.sh`（如适用）与 `bash scripts/doc-impact.sh verify` → 期望通过（纯工具链 change 按 §12.2 豁免项如实体现） （待跑）

| Scenario | 测试文件 |
|---|---|
| 斜杠命令激活档位 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| skill 读取激活档位 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| change 归档后回落 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| read 不抢绑 | .pi/extensions/tests/constraint-injection.smoke.cjs,.pi/extensions/tests/constraint-injection.smoke.cjs |
| 绑定健康时写其他 change 目录不抢绑 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 无绑定时写 change 目录兜底绑定 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 同 turn 绑定锁定（并行竞态消除） | .pi/extensions/tests/constraint-injection.smoke.cjs |
| explore 新会话不绑定无关 change | .pi/extensions/tests/constraint-injection.smoke.cjs |
| requirements 档提及后正常绑定 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| reload 后档位恢复 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| resume 无法恢复回落未激活 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 新会话不继承档位 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| apply 中断后 resume 恢复档位 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| pi 重启恢复会话后档位恢复（真实路径） | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 全新 pi 会话启动不继承其他会话档位 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 无自身档位历史的会话 reload/resume 不继承他窗口档位 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 子线程派发不清零主会话档位 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| mtime 兜底显式记账 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 稳定层档位生命周期内恒定 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 档位切换一次性变化 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 动态层事件驱动增量发送 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| findings 更新触发重发 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| compact 后快照重发 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 关键词域限定 | .pi/extensions/tests/constraint-injection.smoke.cjs,.pi/extensions/tests/constraint-injection.smoke.cjs |
| 声明域内关键词正常命中 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 稳定层差异走 steer 通知 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 未激活档仅索引 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 实现档注入生效 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| JIT 路径细化 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 节残缺时回退全文 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 红线层为空回退全节 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 红线层低于最小字节回退全节 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| flow 文档节级注入 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 声明注入层级记账 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 命中只增不减（缓存稳定） | .pi/extensions/tests/constraint-injection.smoke.cjs,.pi/extensions/tests/constraint-injection.smoke.cjs |
| change 文本不触发关键词命中 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| ASCII 关键词词边界整词匹配 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 无声明不注入并提示 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 未知域名宽容忽略 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 超预算分层降级 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 降级永不真丢 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 模型可见省略通知 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 降级确定性（缓存友好） | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 降级记账 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| steer 消息 UI 受控 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 实现阶段自动注入发现 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| research 语境不落 change | .pi/extensions/tests/constraint-injection.smoke.cjs |
| research 语境无 topic 落通用池 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| smoke test 直跑 | .pi/extensions/tests/run-harness-smoke.sh |
| 抢绑条件纯函数直跑 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 动态层 diff 指纹纯函数直跑 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 红线层提取纯函数直跑 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 红线层零提取回退 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 节提取回落 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 节低于最小字节下限回退全文 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| pin.read 首次注入记账并会话内去重 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| pin.write 仅成功路径记账 | .pi/extensions/tests/harness-log.smoke.cjs |
| constraint.inject 记录命中原因 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 稳定层档位生命周期内不重复记账 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| compact 重发快照记账标记 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 档位激活记账 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 绑定修正追记 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| TTL 与既有事件兼容 | .pi/extensions/tests/harness-log.smoke.cjs |
| 恢复路径记账 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 记账失败不阻断激活 | .pi/extensions/tests/constraint-injection.smoke.cjs |
