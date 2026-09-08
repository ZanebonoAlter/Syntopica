# harness-quick-wins Tasks

## 1. quality-gate：lint 缓存路径（D1）

- [x] 1.1 quality-gate.ts 前端门禁改为 `pnpm exec eslint . --cache --cache-location node_modules/.cache/eslint/.eslintcache`；会话内 git diff 命中 `front/eslint.config.*` 时该会话去 cache 全量。验证：改 front/ 下任一文件触发门禁，第二次运行时长显著低于第一次（<10s），且 eslint.config 变更后自动全量
- [x] 1.2 确认 `node_modules/.cache/eslint/` 未被 git 跟踪（`git status --porcelain front/node_modules/.cache` 为空）

## 2. quality-gate：同根因短路 + 并行（D2）

- [x] 2.1 lint 先行执行；输出命中 typechecking error / build failed 特征（复用 truncateDiagGate 特征表同源匹配器）时短路返回，vet / domain go test 不执行、不记账。验证：制造一个编译错误，回合末事件仅 1 条 golangci-lint ok=false
- [x] 2.2 无短路时 go vet / go build / domain go test 改 Promise.all 并行执行，各自 gate.check 记账与 diag 逻辑不变。验证：正常改动回合门禁总耗时 ≈ lint + max(vet, build, test) 而非串行和

## 3. quality-gate：成功采样记账（D3）

- [x] 3.1 会话内 per-cmd 状态加 `{ consecOk, lastWasFail, everGreen }`；ok=true 按「翻转锚点必记 / 每 5 次连续成功记 1 条」落库，payload 附 `flip` / `sampled` + `n` 字段；ok=false 全量记账并清零计数。验证：连续 6 次成功场景落库恰 2 条（锚点 1 + 采样 1）
- [x] 3.2 `.pi/extensions/tests/quality-gate.smoke.cjs` 补采样/短路/everGreen 状态机断言（纯函数抽出后直跑）

## 4. quality-gate：steer 语气分级（D4）

- [x] 4.1 失败 steer 按 everGreen 分级文案：`[回归]`（曾绿变红，强语气必须修）/ `[中间态]`（从未绿，轻提示可继续）。验证：同一会话先造一次成功再失败 → steer 文案含「回归」；全新失败 → 含「中间态」

## 5. 快照与扩展测试同步

- [x] 5.1 同步 `.pi/extensions/quality-gate.ts` 快照到 `docs/research/`（diff 后 cp）
- [x] 5.2 `bash .pi/extensions/tests/run-harness-smoke.sh` 退出码 0（既有断言不回归）

## 6. 文档

- [x] 6.1 AGENTS.md 增「pi 扩展全景」表：8 扩展 ×（挂点 / 触发条件 / 软硬门禁 / fail 策略 / 记账 kind），替换现有散落三处描述中过时的部分
- [x] 6.2 AGENTS.md「pi 增量门禁」段落补 steer 分级语义（回归=必须修 / 中间态=可继续）；注明归档前全绿硬要求不变
- [x] 6.3 `.agents/skills/harness-facts/SKILL.md` 写入方清单补全（+ tool-output-spill 的 spill.write），reason 枚举说明同步

## 7. 测试

| Scenario（spec delta） | 落点 |
| --- | --- |
| 门禁命令失败记账（既有） | .pi/extensions/tests/failure-classify.smoke.cjs（回归） |
| 失败全量记账不采样 | .pi/extensions/tests/quality-gate.smoke.cjs（新增断言） |
| 转绿翻转锚点必记 | .pi/extensions/tests/quality-gate.smoke.cjs（新增断言） |
| 分母按采样口径还原 | 人工：sqlite3 按锚点计 1、采样条 ×N 聚合复算与全量期对照 |
| 同根因短路未执行不记账 | .pi/extensions/tests/quality-gate.smoke.cjs（新增断言）+ 人工：编译错误回合 sqlite3 仅 1 条事件 |
| 未运行不记账（既有） | 人工：纯 docs 编辑回合零事件（回归） |
| diag 失败特征优先（既有） | .pi/extensions/tests/failure-classify.smoke.cjs（回归） |
| go test 记账不丢 FAIL 行（既有） | .pi/extensions/tests/failure-classify.smoke.cjs（回归） |
| lint 缓存命中（D1） | 人工：同一文件二次门禁 <10s；eslint.config 变更后全量 |
| steer 分级文案（D4） | 人工：回归/中间态两文案各出现一次 |

## 8. 文档

<!-- doc-impact: 无业务域（代码在 .pi/ gitignored 与 front/package.json lint 调用方式；AGENTS.md 与 skill 为 agent 文档，非 reference 活文档域；apply 时以 doc-impact.sh suggest 校准） -->

- AGENTS.md：扩展全景表 + 门禁分级描述（6.1 / 6.2）
- .agents/skills/harness-facts/SKILL.md：写入方清单（6.3）
- docs/research/：quality-gate.ts 快照（5.1）

## 9. 验证

- [x] 9.1 `bash .pi/extensions/tests/run-harness-smoke.sh` → 退出码 0
- [x] 9.2 `cd front && time (pnpm exec eslint . --cache --cache-location node_modules/.cache/eslint/.eslintcache)` 连跑两次 → 第二次显著快于第一次（<10s，首次冷缓存）
- [x] 9.3 `sqlite3 .pi/harness/events.db "SELECT json_extract(payload,'$.sampled'), COUNT(*) FROM events WHERE kind='gate.check' AND ts > date('now','-1 day') GROUP BY 1"` → 实施后出现 sampled=true 采样行且 ok=1 总行数明显少于实施前同规模日
- [x] 9.4 `cd backend-go && go build ./...` → 退出码 0（quality-gate 不影响 go 代码，防御性确认）
- [x] 9.5 `grep -c "扩展全景" AGENTS.md` → ≥1（6.1 落盘确认）
