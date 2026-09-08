# constraint-declaration-redline Tasks

## 1. 格式规范落位

- [x] 1.1 `docs/reference/standard/shared/doc-authoring.md` 增「约束节红线句格式」小节：约束节内每条约束以列表项呈现、首词组加粗为自含红线句、引用块不属红线层、MUST NOT 语义增删。验证：`grep -n "红线" docs/reference/standard/shared/doc-authoring.md` 命中该节

## 2. 注入器改造（constraint-injection.ts）

- [x] 2.1 实现 `extractRedlines(sectionText)` 纯函数（design D2：顶层列表项首个加粗块逐行提取；无加粗列表项不凑数；0 条或 < minSectionBytes 返回回退判定），抽出为可 smoke 直跑的独立函数。验证：单测断言规整节 / 无加粗节 / 低字节节三态
- [x] 2.2 declaration 注入路径接入红线层：命中声明域时注入红线句逐行 + 细节层取回指引尾行；回退时注全节；`constraint.inject` payload 附 `layer`（redline|full）、bytes 如实。验证：改 proposal 声明后实测注入块含红线行与指引尾行
- [x] 2.3 keyword / jit-path 命中路径保持全节注入不变（防误伤）；budget 降级与只增不减语义不动。验证：run-smoke.sh 既有断言全绿
- [x] 2.4 同步 `.pi/extensions/` 快照到 `docs/research/extensions/`

## 3. smoke 用例

- [x] 3.1 `.pi/extensions/tests/` 增红线层提取用例（规整提取 / 零提取回退 / 低字节回退 / 记账 layer 标记）。验证：`bash .pi/extensions/tests/run-smoke.sh` 退出 0

## 4. 九域约束节改写（分三批，每批独立可交付）

- [x] 4.1 批一：`flow/daily-report.md` + `flow/data-enrichment.md`（12.9K / 12.2K 两个大头）。验收：每条约束首句加粗自含、git diff 逐条对照语义不变、引用块保留
- [x] 4.2 批二：`flow/discovery.md` + `flow/reading.md` + `flow/ai-summary.md`。验收同 4.1
- [x] 4.3 批三：`flow/scheduler.md` + `flow/content-enrichment.md` + `flow/semantic-board.md` + `flow/topic-graph.md`。验收同 4.1
- [x] 4.4 改写期间并行 change 冲突检查：每批前后 `git status` 确认无其他 change 正在动同名 flow 文档，冲突则 rebase 后复验
  - 实况：data-enrichment.md / semantic-board.md 存在另一在途 change 的未提交改动（版块简报/调查链，含约束节内 6+1 条新增项与部分细节重写）；本 change 改写以工作区当前态为基，机械校验确认本 change 改动仅落在首个加粗块内，他 change 新增项一并完成红线化、细节未动，无 rebase 需要

## 5. 文档同步

- [x] 5.1 `AGENTS.md` 约束注入段补「声明域=红线层 / 关键词·JIT=全节」两级语义；`docs/reference/constraints-index.md` 对应描述同步
- [x] 5.2 `.agents/skills/harness-facts/SKILL.md` 的 `declaration` reason 说明补层级语义与 layer 标记

## 6. 测试

| Scenario（spec delta） | 落点 |
| --- | --- |
| 实现档声明注入红线层 | .pi/extensions/tests/run-smoke.sh（新增断言：声明域注入含红线行 + 指引尾行） |
| 关键词与 JIT 命中注入全节 | .pi/extensions/tests/run-smoke.sh（既有断言回归 + 补全节形态确认） |
| 红线层为空回退全节 | .pi/extensions/tests/run-smoke.sh（新增：未规整域 fixture 回退 + layer=full） |
| 红线层低于最小字节回退全节 | .pi/extensions/tests/run-smoke.sh（新增：380B fixture） |
| 声明注入层级记账 | .pi/extensions/tests/run-smoke.sh（新增：payload layer/bytes 断言） |
| 红线层提取纯函数直跑 | .pi/extensions/tests/*.smoke.cjs（2.1 抽出函数直跑） |
| 红线层零提取回退 | .pi/extensions/tests/*.smoke.cjs |
| 节提取回落 / 节低于最小字节回退全文（既有） | .pi/extensions/tests/run-smoke.sh（回归） |
| 命中只增不减 / 降级族 / 未激活档仅索引 / 关键词词边界 / change 文本不触发（既有） | .pi/extensions/tests/run-smoke.sh（回归） |
| 九域改写语义不变 | 人工：每批 git diff 逐条对照（4.1~4.3 验收） |

## 7. 文档

<!-- doc-impact: flow, standard -->
<!-- doc-impact-excuse: api=front/app/api 与 backend API 脏文件属 add-composite-labels 等在途 change，非本工具链/flow 格式 change 改动; database=database 文档与迁移测试脏文件属在途版块能力 change，非本 change 数据模型改动; architecture=architecture/map.md 脏文件属在途版块能力 change，非本 change 架构改动; configuration=configuration.md 与 backend 配置脏文件属在途版块能力 change，非本 change 配置改动 -->

- docs/reference/flow/*.md ×9（约束节红线句格式改写，4.1~4.3）
- docs/reference/standard/shared/doc-authoring.md（格式规范，1.1）
- AGENTS.md / docs/reference/constraints-index.md / .agents/skills/harness-facts/SKILL.md（5.1 / 5.2）
- docs/research/extensions/ 快照（2.4）

## 8. 验证

| Scenario | 测试文件 |
| --- | --- |
| 实现档注入生效 | .pi/extensions/tests/run-smoke.sh |
| JIT 路径细化 | .pi/extensions/tests/run-smoke.sh |
| flow 文档节级注入 | .pi/extensions/tests/run-smoke.sh |
| 节残缺时回退全文 | .pi/extensions/tests/run-smoke.sh |
| 红线层为空回退全节 | .pi/extensions/tests/run-smoke.sh |
| 红线层低于最小字节回退全节 | .pi/extensions/tests/run-smoke.sh |
| 声明注入层级记账 | .pi/extensions/tests/run-smoke.sh |
| 未激活档仅索引 | .pi/extensions/tests/run-smoke.sh |
| change 文本不触发关键词命中 | .pi/extensions/tests/run-smoke.sh |
| ASCII 关键词词边界整词匹配 | .pi/extensions/tests/run-smoke.sh |
| 命中只增不减（缓存稳定） | .pi/extensions/tests/run-smoke.sh |
| 无声明不注入并提示 | .pi/extensions/tests/run-smoke.sh |
| 未知域名宽容忽略 | .pi/extensions/tests/run-smoke.sh |
| 超预算分层降级 | .pi/extensions/tests/run-smoke.sh |
| 降级永不真丢 | .pi/extensions/tests/run-smoke.sh |
| 模型可见省略通知 | .pi/extensions/tests/run-smoke.sh |
| 降级确定性（缓存友好） | .pi/extensions/tests/run-smoke.sh |
| 降级记账 | .pi/extensions/tests/run-smoke.sh |
| smoke test 直跑 | .pi/extensions/tests/run-smoke.sh |
| 红线层提取纯函数直跑 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 红线层零提取回退 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 节提取回落 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 节低于最小字节下限回退全文 | .pi/extensions/tests/constraint-injection.smoke.cjs |

- [x] 8.1 `bash .pi/extensions/tests/run-smoke.sh` → 退出码 0
- [x] 8.2 红线格式覆盖率 100%：`python3 - <<'EOF'` 校验 9 个 flow 约束节顶层列表项首行均有加粗块（脚本输出 `9/9 OK`，任一域失败列出文档名）
- [x] 8.3 声明注入瘦身边际实测：临时绑定一个声明 3 域的 change 激活 implementation 档，`sqlite3 .pi/harness/events.db "SELECT json_extract(payload,'$.layer'), json_extract(payload,'$.bytes') FROM events WHERE kind='constraint.inject' AND json_extract(payload,'$.reason')='declaration' ORDER BY id DESC LIMIT 6"` → 出现 layer=redline 行且 bytes < 3000
- [x] 8.4 人工：上线 3~7 天后按 design D6 聚合 declaration bytes 均值（目标 <2KB）与 layer=redline 占比（目标 >90%），记录到 change 收尾验证
  - 实况（2026-09-04 归档前聚合）：上线前基线（08-23~09-01，layer 字段未存）382 条均值 5136~7643B；09-02 过渡日（改造落地中）full 16 条（均值 7382B）+ redline 4 条；09-03 00:12 起 12/12 全部 layer=redline（**占比 100%，>90% 达标**），均值 2324B / 稳态 2390B（semantic-board 单域红线层），**较基线降 ~60% 但略超 <2KB 目标 ~15%**——红线句自含性优先于压缩极限，接受现值不回改；多域 change 单回合总注入仍远低于旧 12~16KB 水平
