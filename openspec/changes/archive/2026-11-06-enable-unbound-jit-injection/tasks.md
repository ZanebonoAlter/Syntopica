<!-- doc-impact: flow, standard -->

# Tasks: enable-unbound-jit-injection

> 目标：按 `add-dispatch-constraint-digest` D7 判据（29% < 70%）放开「无档编辑路径 JIT」最小口子，并补齐路径标签覆盖，让无档会话与无档子线程能按编辑路径自愈。
> 勾选态 = 已实测通过；review 意见处置见 `review.md`。

## 1. 机制改动（扩展）

- [x] 1.1 `.pi/extensions/constraint-injection.ts` 的 `tool_execution_start` handler 内 JIT **写入** gate 去掉 `state.mode &&`（~L1949，路径取 `event.args.path`）：未激活档 write/edit 也扫描 `scanAppliesTags` 并写 `jitDocHits`
- [x] 1.2 同 handler JIT **投递** gate 去掉 `state.mode &&`（~L1983）：`if (jitAdded && cfg.channel !== "legacy")`
- [x] 1.3 同步改注释：两处 gate 附近 + `planInjection` 头部「未激活档仅注入索引」表述改为「稳定层仅索引 + 编辑路径 JIT 动态追加」
- [x] 1.4 只读核对「不必改」清单并记录：`planInjection` 的 `jitDocHits` 追加（~L1075）、`splitPlanItems` 归 dynamic、`applyBudget` 顺位、`bindAction`（无档不绑）、`stableSnapshotKey(null,null) = "none|none"`、legacy 落 system prompt 路径（~L2050）——结论写 `review.md`

## 2. 路径标签补全

- [x] 2.1 新增 `scripts/harness/constraint-coverage.sh`（只读）：样本清单**固定在脚本内**（`backend-go/internal/**` 业务包 + `front/app/features/**` 功能目录，显式列出），输出「路径 → 命中业务域 / 未覆盖」矩阵与覆盖率；分母 = 样本清单长度；跑基线并把改前数字与矩阵贴 `review.md`
- [x] 2.2 按矩阵给 `docs/reference/flow/*.md` 补头部 `doc-impact-applies` 标签：只补「该域业务约束管得着」的路径；目录信号带尾斜杠、单文件信号写完整名（注意 `X.ts` 会误中 `X.tsx`）；保持「前 15 行、一文档首条生效」；不追求目录数指标
- [x] 2.3 顺带修复两项预存在红（孤儿标签）：`标签: semantic-board.md 路径真实存在（7 条）`、`标签: reading.md 路径真实存在（3 条）`；修好后 `bash scripts/harness/test-patrol.sh --resolve constraint-injection:semantic-board-applies-orphan --by enable-unbound-jit-injection`（reading 同款），不留「先改后豁免」
- [x] 2.4 补标签后跑 `constraint-digest.sh --paths` 对 5 条真实路径断言命中集合（见 7.3 清单），输出贴 `review.md`

## 3. 记账与探针

- [x] 3.1 `constraint.inject` payload 追加 append-only `tier`（`none`/`requirements`/`implementation`），由注入器自报；**不改** 既有 `mode`（文档形态字段）语义
- [x] 3.2 `scripts/harness/harness-retro.sh` 增「⑦H 无档 JIT」子组：按日统计注入条数 / 会话数 / 平均域数 / **送达字节** / 降级占比；compact 重发计入条数不重计会话；域名只算 `path` 以 `docs/reference/flow/` 开头的条目；口径写进段头（历史无 `tier` 计未知并剔除分母）

## 4. 测试

- [x] 4.1 `test-cases.md` 14 步落点全部实现（含 ⓪ 继承与调整表逐行处置）
- [x] 4.2 `constraint-injection.smoke.cjs` 新增 `unbound-jit-1..10` 用例组；**反转**旧第 8 号「JIT 门控: 未激活时写 airouter 不注入」为「注入 ai-summary」，并补未映射路径零注入对照；用例间用 `session_start(new)` + `markMessages()` 重置，避免状态污染
- [x] 4.3 生命周期用例：无档主会话 compact 重发、子线程（无 `before_agent_start`）compact 不重发、同进程 fork 继承不重投、legacy 下一 turn 落 system prompt、reload 命中集清零
- [x] 4.4 `constraint-digest.smoke.sh` 增补标签后路径命中断言；`constraint-coverage.sh` 增 smoke（临时夹具矩阵输出）

## 5. Review

- [x] 5.1 派子线程聚焦 review（审查制品 + diff：gate 最小性、标签是否过宽、扩展名碰撞、预算/降级实测、生命周期边界、验收可执行性），意见按 High/Medium/Low 处置后 `review.md` 建账

## 6. 文档

- [x] 6.1 `docs/reference/harness/pi-extensions.md`：JIT 通道小节补「无档可注入」语义、`tier` 归因口径、生命周期边界（compact/fork/legacy）、一行级回滚
- [x] 6.2 `docs/reference/standard/shared/doc-authoring.md`：`doc-impact-applies` 说明里「仅档位激活时生效」改为「档位激活全量 + 未激活按编辑路径 JIT 追加」
- [x] 6.3 `AGENTS.md` + `docs/reference/开发执行规范.md` 核对表述：派发取红线三步不变；补一句「未激活档也会按编辑路径自动注入，brief 仍用于覆盖无编辑场景」
- [x] 6.4 本 change 只改 flow 文档头部标签（不动业务约束正文），`constraint-domains` 不声明（纯工具链 change，合法），`doc-impact` 声明 `flow, standard` 并在归档前对账

## 7. 验证

- [x] 7.1 `bash .pi/extensions/tests/run-smoke.sh` → **退出码 0**，且 stdout 含 `SMOKE OK`（两项预存在红已在 2.3 修掉；不使用管道后接 grep/tail 判定，避免掩盖退出码）
- [x] 7.2 `bash scripts/harness/constraint-coverage.sh` → 业务样本未覆盖条目数较基线下降，且矩阵中新增命中逐条等于预期域集合
- [x] 7.3 `bash scripts/harness/constraint-digest.sh --paths front/app/features/articles/components/ArticleCardView.vue,front/app/pages/tags/,front/app/features/discovery/,front/app/features/tags/components/,backend-go/internal/platform/airouter/embedding.go` → 命中集合按输入顺序逐条等于：reading / semantic-board / discovery / semantic-board / ai-summary，且无「未命中任何」字样
- [x] 7.4 `bash scripts/harness/harness-retro.sh 2>&1 | grep -A6 '无档 JIT'` → 段头含口径说明，且输出「注入条数 / 会话数 / 平均域数 / 送达字节 / 降级占比」五项数值（允许为 0，但字段必须出现）
- [x] 7.5 `bash scripts/harness/check-tasks-tail.sh openspec/changes/enable-unbound-jit-injection` → 退出码 0
- [x] 7.6 `bash scripts/harness/doc-impact.sh verify openspec/changes/enable-unbound-jit-injection` → 通过（声明 ↔ diff 对账）
- [x] 7.7 `bash scripts/harness/review-trace.sh openspec/changes/enable-unbound-jit-injection` → 通过（无零痕迹/空壳）
- [x] 7.8 `openspec validate enable-unbound-jit-injection --strict` → valid

### Scenario → 测试文件映射

| Scenario | 测试文件 |
|---|---|
| 稳定层档位生命周期内恒定 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 档位切换一次性变化 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 动态层事件驱动增量发送 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| turn 中途切档后同 turn 已投递内容不被延迟重建重投 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 绑定切换后首轮仍全量重投 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| fork 继承的指纹不被陈旧快照 key 重投 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| findings 更新触发重发 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| compact 后快照重发 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 关键词域限定 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 声明域内关键词正常命中 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 稳定层差异走 steer 通知 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 未激活档仅索引 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 未激活档关键词通道保持关闭 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 实现档注入生效 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| JIT 路径细化 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 节残缺时回退全文 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 红线层为空回退全节 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 红线层低于最小字节回退全节 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| flow 文档节级注入 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 声明注入层级记账 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 命中只增不减（缓存稳定） | .pi/extensions/tests/constraint-injection.smoke.cjs |
| change 文本不触发关键词命中 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| ASCII 关键词词边界整词匹配 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 无声明不注入并提示 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 未知域名宽容忽略 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 超预算分层降级 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 降级永不真丢 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 模型可见省略通知 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 降级确定性（缓存友好） | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 降级记账 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| steer 消息 UI 受控 | 人工：TUI 中观察动态层消息默认折叠（display=false），只显示类型+摘要，不整块刷屏 |
| 未激活档编辑命中业务路径 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 未激活档编辑未映射路径 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 未激活档同域多次编辑只注一次 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 未激活档 JIT 不绑定 change | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 未激活档 JIT 受预算降级 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 子线程未继承档位时按自身路径自愈 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| legacy 通道不即时投递 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 未激活档 compact 后重发 JIT 集合 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 无 before_agent_start 的子线程 compact 不重发 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 同进程 fork 继承命中集不重投 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| reload 后命中集清零 | .pi/extensions/tests/constraint-injection.smoke.cjs |
