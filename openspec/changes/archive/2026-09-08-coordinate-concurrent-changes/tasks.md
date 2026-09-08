# Tasks — coordinate-concurrent-changes

## 1. 前置与用例先行

- [x] 1.1 锁定实现依赖事实并落 `explore-findings.md`：quality-gate turn_end 增量路径检测的具体位置与数据结构、mode.set boundChange 的会话内可查性、spec-gate 调用 bash 脚本的先例签名、doc-impact.sh verify 反向启发式的输入构造点、harness-log 词汇表注册结构（每条带 file:symbol 引用），验证 explore-findings.md 存在且每条事实可按引用定位
- [x] 1.2 编写 `test-cases.md`（complex 档白盒用例）：多回合累计聚合、无档会话跳过、跨 session 同 change 合并、冲突文件标记、冷启动跳过、verify 双轨输入（地图优先/空回退全树）、既有 excuse 兼容、spec-gate warn 不 block；验证每个 spec Scenario 至少映射一条用例
- [x] 1.3 运行 `bash scripts/doc-impact.sh suggest` 复核文档域声明（预期 `standard`，复核 scripts/.pi 改动不触发其他域启发式），验证 tasks.md 文档节声明与 suggest 输出一致

## 2. 事实库词汇扩展（harness-log）

- [x] 2.1 `lib/harness-log.ts` 词汇与保留期表新增 `edit.map`（30 天 TTL），扩展 `.pi/extensions/tests/harness-log.smoke.cjs` 断言：TTL 清扫含 edit.map、31 天过期、无 schema 迁移，验证 `bash .pi/extensions/tests/run-harness-smoke.sh harness-log` 退出码 0

## 3. 归属地图聚合（quality-gate）

- [x] 3.1 quality-gate turn_end 增量路径检出后：查同 session 最近一条 `mode.set` 取 boundChange（无则跳过），将该 change 名下累计路径全量集合追加为 `edit.map` 事件（change 列 = boundChange），扩展 `.pi/extensions/tests/quality-gate.smoke.cjs` 断言：多回合聚合为累计快照、无档会话零事件、纯对话回合零事件，验证 `bash .pi/extensions/tests/run-harness-smoke.sh quality-gate` 退出码 0
- [x] 3.2 跨 session 合并语义验证：fixture 中两个 session 先后绑定同一 change 各自编辑，验证按 change 取最新一条 edit.map 时集合为两 session 并集（查询侧合并，落库不改写）

## 4. 并发态势脚本（concurrency-status.sh）

- [x] 4.1 新增 `scripts/concurrency-status.sh`：默认人读三段输出（活跃 change × 绑定 session × 最近活动 / git 脏文件 × 归属对照三类分列含冲突标记 / 近 6h gate.check 流水），`--check <change>` 机器可读模式（stdout JSON + exit code：0=无并发脏文件 2=有 warn 3=冷启动跳过）；SQLite 只读连接（`mode=ro`）；新建 `scripts/concurrency-status.smoke.sh` 以 fixtures 临时库断言三段输出、冲突标记、冷启动跳过与只读不锁，验证 `bash scripts/concurrency-status.smoke.sh` 退出码 0

## 5. spec-gate 归档并发检查（⑤'）

- [x] 5.1 spec-gate 归档拦截流程增加检查⑤'：调用 `concurrency-status.sh --check <change>`，exit 2 时输出 warn 级提示（steer 通道，含文件清单与归属 change 名）不 block 归档命令，exit 3 时零输出放行；warn 记账 `policy.decision`（policy=spec-gate、action=warn、reasonCode=concurrent-dirty-tree，target=change 摘要），扩展 `.pi/extensions/tests/spec-gate.smoke.cjs` 断言 warn 语义与记账 payload，验证 `bash .pi/extensions/tests/run-harness-smoke.sh spec-gate` 退出码 0

## 6. doc-impact verify 归属收窄

- [x] 6.1 `scripts/doc-impact.sh` verify 的反向启发式（"疑似遗漏"/"声明 none 但命中"）改为双轨输入：事实库该 change 最新 `edit.map` 非空时仅扫归属集合，为空回退全树 git diff + 未跟踪（现状）；"声明了未更新"保持全树对账；既有 `doc-impact-excuse` 注释解析兼容不判 FAIL，新建 `scripts/doc-impact.smoke.sh` 以双 change fixture 断言：他人 handler 脏文件不再触发疑似遗漏、空地图回退全树仍触发、excuse 注释零报错、声明了未更新仍全树命中，验证 `bash scripts/doc-impact.smoke.sh` 退出码 0

## 7. 文档
（纯工具链 change：extension 落库 / bash 脚本 / 门禁检查 / 规范文档，无 flow 影响）

<!-- doc-impact: none(文档更新均为流程/工具链文档：开发执行规范.md、AGENTS.md、harness-facts skill、docs/research 快照，不属七域辖区；无 flow/api/database/architecture/standard/configuration/deployment 行为变化；suggest 预勾选 none 与实现路径盘点一致) -->
<!-- doc-impact-excuse: database=backend-go 迁移/模型脏文件属 watch-materialize-llm-adjudication 的进行中改动，本 change 未触碰 backend-go; flow=docs/reference/flow/daily-report.md 脏文件属 watch-materialize-llm-adjudication 的进行中改动，本 change 未触碰 flow 文档; api=front/app/api/dailyReports.ts 脏文件属 watch-materialize-llm-adjudication 的进行中改动，本 change 未触碰 api 文档 -->

- [x] 7.1 更新 `docs/reference/开发执行规范.md`：§0.6 编排六步表后新增「commit 与归档解耦」节（门禁绿即主体 commit / 归档另起 commit / fixup 语义 / harness 类挂验证不受影响 / 态势拉取挂点）、子线程并行派发节补派发前态势拉取、§11 归档硬门禁段写入双轨输入与检查⑤'说明，验证章节交叉引用无断链且 scenario-trace 不受影响
- [x] 7.2 更新 `.agents/skills/harness-facts/SKILL.md`：事件词汇表加 edit.map（含聚合语义与 payload 结构）、policy.decision reasonCode 枚举加 concurrent-dirty-tree、新增 concurrency-status.sh 查询配方（"现在还有谁在跑"标准查询），验证与 spec 契约措辞一致
- [x] 7.3 更新根 `AGENTS.md` pi 扩展全景表：quality-gate 行补 edit.map 落库记账、spec-gate 行补检查⑤'并发 warn，验证 `bash scripts/check-standards.sh` 不报孤立文档
- [x] 7.4 将新增/修改的 `.pi/extensions/` 文件同步到 `docs/research/` 对应快照，验证逐文件 `cmp -s` 返回 0
- [x] 7.5 部署影响说明：无 DB 迁移、extension 同批部署、存量混合脏树需一次性人工大扫除（按 change 归属分批 commit，无法归属的 stash 审视）、edit.map 仅从部署时刻积累、doc-impact-excuse 停止新增（存量注释兼容），验证归档前该说明与 proposal Impact 节一致

## 8. 测试

- [x] 8.1 `bash .pi/extensions/tests/run-harness-smoke.sh` → 退出码 0，全部 harness smoke（含 harness-log/quality-gate/spec-gate 新增断言）通过（实测 325 项断言全绿）
- [x] 8.2 `bash scripts/concurrency-status.smoke.sh && bash scripts/doc-impact.smoke.sh && bash scripts/check-standards.smoke.sh && bash scripts/scenario-trace.smoke.sh` → 四项退出码均为 0（实测 13+7 项断言全绿，既有两 smoke 无回归）
## 9. 验证

- [x] 8.3 `bash scripts/doc-impact.sh verify openspec/changes/coordinate-concurrent-changes && bash scripts/check-standards.sh --change coordinate-concurrent-changes` → 两项退出码均为 0（实测 verify 通过启发式:fallback——本会话加载的旧版扩展未落 edit.map，预期冷启动回退；check-standards 133/133）
- [x] 8.4 `openspec schema validate syntopica-ui --verbose && openspec validate coordinate-concurrent-changes --strict` → 两项退出码均为 0（实测 schema valid + change valid；另 `scenario-trace.sh` 对账 21/21 映射齐全）
- [x] 8.5 端到端自证：模拟第二会话视角对当前树上"归属其他 active change 的脏文件"运行 `bash scripts/concurrency-status.sh --check coordinate-concurrent-changes`，验证输出 exit 2 + warn 清单与当前 git status 归属一致（本 change 的并发检查在自己归档时生效，吃自己的狗粮）

| Scenario | 测试文件 |
|---|---|
| 编辑路径按绑定 change 聚合 | .pi/extensions/tests/quality-gate.smoke.cjs |
| 同文件双 change 触碰标记冲突 | scripts/concurrency-status.smoke.sh |
| 无档会话不计入归属 | .pi/extensions/tests/quality-gate.smoke.cjs |
| 归档验证前拉取态势 | scripts/concurrency-status.smoke.sh |
| 脚本只读安全 | scripts/concurrency-status.smoke.sh |
| 树上有其他 change 归属文件时 warn | .pi/extensions/tests/spec-gate.smoke.cjs |
| 冷启动跳过 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 门禁绿即主体 commit | 人工（流程约定：docs/reference/开发执行规范.md §0.6 修订存在性，grep 关键字命中） |
| harness change 挂验证不受 commit 影响 | 人工（git 语义约定：开发执行规范 commit 解耦节存在，grep 关键字命中） |
| edit.map 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| TTL 分级清扫 | .pi/extensions/tests/harness-log.smoke.cjs |
| 事件追加不可变 | .pi/extensions/tests/harness-log.smoke.cjs |
| spill.write 事件随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| subagent.complete 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| policy.decision 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| 疑似遗漏 | scripts/doc-impact.smoke.sh |
| 疑似遗漏按归属地图过滤 | scripts/doc-impact.smoke.sh |
| 归属地图为空时回退全树 | scripts/doc-impact.smoke.sh |
| 既有 excuse 注释兼容 | scripts/doc-impact.smoke.sh |
| 声明了未更新 | scripts/doc-impact.smoke.sh |
| 历史存量豁免 | scripts/doc-impact.smoke.sh |
