# Tasks: add-dispatch-constraint-digest

> 目标：把「派发任务文本带相关域红线摘要」从空 MUST 变成一条命令 + 固定格式 + 可量化回检。
> **不改任何既有注入行为**（matchText / JIT / allowedKeywordDocs 三处 gate 一律不动）。
> 勾选态 = 已实测通过；review 意见处置见 `review.md`。

## 1. 红线提取下沉为共用实现

- [x] 1.1 新建 `.pi/extensions/lib/constraint-redlines.ts`：把 `constraint-injection.ts` 的 `RedlineLayer` + `extractRedlines` **原样**迁入（review 已 diff 逐字节一致），导出类型与函数
- [x] 1.2 `.pi/extensions/constraint-injection.ts` 改为 import + re-export（对外导出面不变），删除本地实现；三处 gate 未碰。验证：`grep -c "function extractRedlines" .pi/extensions/constraint-injection.ts` → `0`
- [x] 1.3 回归：`bash .pi/extensions/tests/run-smoke.sh` → 本 change 无回归（941 ✅）；⚠️ 套件含 2 项**预存在红**（`flow/semantic-board.md`、`flow/reading.md` 的 `doc-impact-applies` 信号指向已删路径；HEAD 原版复现一致），已登记 test-patrol 台账 `constraint-injection:semantic-board-applies-orphan` / `constraint-injection:reading-applies-orphan`

## 2. 取数脚本

- [x] 2.1 新建 `scripts/harness/constraint-digest.sh`：
  - `--domains <d1,d2>`：域名 Must 为 `docs/reference/flow/*.md` basename（`listFlowDomains` 同源语义）；未知域名非零退出并列出可用域
  - `--paths <p1,p2>`：按 `doc-impact-applies` 标签反查（头部 15 行 / 同正则 / 纯子串 / 首匹配即止），命中 flow 文档则导出域
  - 输出：`### <域>（源 docs/reference/flow/<域>.md）` + 逐行红线 + `细节层：read ...`；单域红线 ≤8 行
  - **红线必须经 esbuild bundle `lib/constraint-redlines.ts` 调用**；bundle 失败 MUST 非零退出并提示，禁止回退自带正则
  - 未命中三态（无标签 / 非业务域 / 红线为空）各自显式输出 + 回退指引，禁止静默空
  - 阈值读同一 config 缺省值 `minSectionBytes`（缺省 512）；`digestDocThreshold` 不适用——脚本只取节内红线，不做整文速览
  - 验证：`bash scripts/harness/constraint-digest.sh --domains daily-report` → 输出含 `daily-report` 与源路径，红线行 ≤8
- [x] 2.2 `--help` 写明：只读文档头部 `doc-impact-applies`，不读 `constraint-domains`（proposal）与 `doc-impact`（tasks）两套注释；业务域唯一真相源 = flow basename；域数量动态输出不硬编码
- [x] 2.3 新建 `scripts/harness/constraint-digest.smoke.sh`：覆盖按域取红线、按路径反查、路径未命中三态、未知域名拒绝、红线与扩展实现逐字节一致（10 篇 flow 文档逐篇）、6 条真实路径命中集对账（vs extension `scanAppliesTags`）
- [x] 2.4 （review H1）新增 `--brief` 模式：输出单个 `## 约束（红线，源 …）` 标题 + 每域 `### <域>（源 …）` 子标题 + 固定兜底行，使输出可**原样粘贴**且被 `parseConstraintBrief` 识别为 `present`；smoke 增端到端断言（`--brief` 输出 → 真实 `parseConstraintBrief` → present + 多域）
- [x] 2.5 （review M1）bundle 改惰性构建：`--paths` 的标签扫描与三态判定不依赖 esbuild；bundle 失败时域块降级为「红线不可用」+ 细节层指引，未命中说明照常打印
- [x] 2.6 （review L1/L3/L8）删死列或注明 section 未消费；`--help` 域数动态；esbuild 优先本仓已装（`node_modules/.bin` / `npx --no-install`），都不可用才回落 `npx -y`
- [x] 2.7 （review L5）smoke 增 2 个 fixture：标签在第 16 行（窗口外→不命中）、头部两条标签行（只取首条），对应 design D3 两个边界维度

## 3. 派发记账（append-only）

- [x] 3.1 `.pi/extensions/harness-telemetry.ts` `tool_result(Agent)` 分支：`subagent.dispatch` payload 追加 `constraintDigest`（`present`/`absent`）与 `constraintDomains`（`string[]`）；既有 **15 个固定字段 + 可选 `failure`** 语义与类型不变
- [x] 3.2 解析实现为可导出的纯函数 `parseConstraintBrief`；段在无域 → `present` + `[]`；解析失败 MUST NOT 阻断派发
- [x] 3.3 扩展 `.pi/extensions/tests/telemetry-dispatch.smoke.cjs`：带约束段 / 无约束段 / 段在无源路径 / 多域去重 / 空输入 / dispatch 集成 六类用例；验证：`bash .pi/extensions/tests/run-harness-smoke.sh` → 退出码 0
- [x] 3.4 （review M2）收域正则收紧为 `docs/reference/flow/([a-z0-9-]+)\.md`（要求目录前缀），`present + []` 语义保留（视为格式异常，不阻断派发）

## 4. 规范引导

- [x] 4.1 `AGENTS.md` 子线程纪律条目追加一句取数指针（只追加，不重写原长句）：派发前 `constraint-digest.sh --paths <目标>` → 粘进 `## 约束（红线，源 ...）` 段
- [x] 4.2 `docs/reference/开发执行规范.md` §0.6 步骤 3 把「MUST 带相关域红线摘要」改成三步（判域 → 取红线 → 粘 brief）+ brief 模板片段 + 子线程自取兜底命令
- [x] 4.3 `docs/reference/constraints-index.md` 增补取数命令指针（域表保持现状，作为 `--paths` 未命中时的回退入口）

## 5. 复盘口径

- [x] 5.1 `scripts/harness/harness-retro.sh` 增「派发带域比例」段（present / (present + absent)），历史记录无两字段计入「未知」并剔除分母、报告标注未知条数；`--json` 同步输出。验证：`bash scripts/harness/harness-retro.sh --days 7 | grep -n "派发带域"` 有命中
- [x] 5.2 `scripts/harness/harness-retro.smoke.sh` 增用例：混合历史 / 全 present / 全 absent / 无派发 四种输入下的分母计算（P1–P6，112/112 通过）
- [x] 5.3 （review L9）SKILL ⑦G 节注明当前样本混池口径（主线程派发 + 子线程二次派发）与偏差方向

## 6. 机制文档

- [x] 6.1 `docs/reference/harness/pi-extensions.md`：补 constraint-digest 脚本定位、`subagent.dispatch` 两个新字段、与 `doc-impact-applies` 的关系；写明"域清单唯一真相源 = flow basename，doc-impact menu 是文档目录轴不是业务域轴"
- [x] 6.2 `docs/reference/harness/pi-extensions.md` 头部 `doc-impact-applies` 补 `scripts/harness/constraint-digest.sh`
- [x] 6.3 `.agents/skills/harness-retro/SKILL.md` ⑦ 段分组表补 G 组，并加「⑦G 派发带域比例子组」口径小节

## 7. 测试

- [x] 7.1 `bash scripts/harness/constraint-digest.smoke.sh` → 退出码 0（含 `--brief` 端到端与两个边界 fixture）
- [x] 7.2 `bash .pi/extensions/tests/run-smoke.sh` → 本 change 相关断言全绿（941 ✅）；套件 2 项预存在红见 1.3（已登记台账，归档前不阻断本 change）
- [x] 7.3 `bash .pi/extensions/tests/run-harness-smoke.sh` → 退出码 0（telemetry 字段用例全绿）
- [x] 7.4 `bash scripts/harness/harness-retro.smoke.sh` → 退出码 0（112/112）

## 8. 文档

<!-- doc-impact: standard(红线提取器下沉到 lib 并改指针：doc-authoring.md 提取器实现行；其余为 harness 工具链与规范文档) -->

- [x] 8.1 `docs/reference/harness/pi-extensions.md` 已更新（脚本 + 字段 + 域真相源）
- [x] 8.2 `docs/reference/开发执行规范.md` §0.6 步骤 3 已改为三步可执行（brief 模板与 `--brief` 真实输出形态一致）
- [x] 8.3 `docs/reference/constraints-index.md` 已加取数指针
- [x] 8.4 `AGENTS.md` 已加派发取数指针（仓库根文件，不在 `docs/` 下但计入本 change 文档面）
- [x] 8.5 `docs/reference/standard/shared/doc-authoring.md` 提取器实现指针改到 `.pi/extensions/lib/constraint-redlines.ts`（双引同一实现），doc-impact 声明相应改为 `standard`

## 9. 验证

- [x] 9.1 `bash scripts/harness/constraint-digest.sh --domains daily-report` → 退出码 0，输出含 `daily-report` 域标题 + 源路径 + 逐行红线（≤8 行，实测 8 条 + 截断标记）
- [x] 9.2 `bash scripts/harness/constraint-digest.sh --paths backend-go/internal/admin/scheduler/job_daily_report.go` → 退出码 0，命中 `daily-report` 与 `scheduler` 两域（原示例路径本仓不存在，已按真实信号路径修订）
- [x] 9.3 `bash scripts/harness/constraint-digest.sh --paths front/app/app.vue` → 输出「命中非业务域 + 可用域清单」，不输出空红线段（实测命中 `standard/frontend/loading-experience.md`）
- [x] 9.4 `bash scripts/harness/constraint-digest.sh --domains not-a-domain` → 退出码 2，列出 10 个可用域名
- [x] 9.5 `bash scripts/harness/constraint-digest.smoke.sh` → 退出码 0，红线逐字节对账通过
- [x] 9.6 `bash .pi/extensions/tests/run-smoke.sh && bash .pi/extensions/tests/run-harness-smoke.sh` → harness 全家桶全绿；run-smoke 仅 2 项预存在红（见 1.3/7.2）
- [x] 9.7 `bash scripts/harness/harness-retro.sh --days 7 | grep -n "派发带域"` → 有命中，输出「无可判定派发（present 0 / 可判定 0，未知 81 条）」
- [x] 9.8 `openspec validate add-dispatch-constraint-digest --strict` → 退出码 0
- [x] 9.9 `bash scripts/harness/scenario-trace.sh … && bash scripts/harness/doc-impact.sh verify … && bash scripts/harness/check-standards.sh --change …` → 三条退出码均为 0（check-standards 260 通过 / 0 失败）
- [x] 9.10 Review：派 `change-review` 子线程只读审查，产出 H1 / M1–M4 / L1–L9，已按级处置并建账 `review.md`，验证：`bash scripts/harness/review-trace.sh openspec/changes/add-dispatch-constraint-digest` 退出码 0

### Scenario → 测试文件

| Scenario | 测试文件 |
| --- | --- |
| 按域取红线 | scripts/harness/constraint-digest.smoke.sh |
| 按目标路径反查 | scripts/harness/constraint-digest.smoke.sh |
| 路径无命中显式报告 | scripts/harness/constraint-digest.smoke.sh |
| 红线提取与扩展实现一致 | scripts/harness/constraint-digest.smoke.sh |
| brief 形态可原样粘贴并被记账 | scripts/harness/constraint-digest.smoke.sh |
| 未知域名拒绝 | scripts/harness/constraint-digest.smoke.sh |
| 带约束段派发被记账 | .pi/extensions/tests/telemetry-dispatch.smoke.cjs |
| 无约束段派发被记账但不阻断 | .pi/extensions/tests/telemetry-dispatch.smoke.cjs |
| 旧记录计未知不计漏 | scripts/harness/harness-retro.smoke.sh |
