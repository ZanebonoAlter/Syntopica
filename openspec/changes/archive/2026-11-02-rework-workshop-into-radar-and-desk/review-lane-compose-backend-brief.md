# 后端审核 brief — 新建泳道编排（后端部分）

> 审核对象：`rework-workshop-into-radar-and-desk` change 的**后端实现**（候选链唯一口径 + compose-candidates 端点扩展 + 日期窗校验 + 测试）。
> 审核方式：**只读**。不要修改任何文件；报告由主线程转写到 `review-lane-compose-implementation.md`。
> 审核模型：`zai-coding-cn/glm-5.3:max`（用户 2026-10-31 当场指定）。

## 1. 必读

1. `openspec/changes/rework-workshop-into-radar-and-desk/lane-compose-implementation-contract.md` —— **唯一口径来源**（API 契约 §1、候选链口径 §2.1、红线 §5）。
2. `openspec/changes/rework-workshop-into-radar-and-desk/specs/workshop-lane-compose/spec.md` —— delta spec。
3. `openspec/changes/rework-workshop-into-radar-and-desk/test-cases-lane-compose-implementation.md` —— §3.1 候选链分支表 C1–C15、§3.2 日期窗校验表 D1–D11（断言判据已定）。
4. `docs/reference/flow/topic-graph.md` 「业务约束与不变量」节 —— **红线权威源**。
5. `docs/reference/flow/daily-report.md` 「业务约束与不变量」节 —— 红线权威源。
6. `docs/reference/standard/backend/testing.md` —— 后端测试与数据安全红线。
7. `AGENTS.md`、`backend-go/AGENTS.md`。

## 2. 审核范围（怎么找到 diff）

后端实现由另一子线程落地，审核时用：

```bash
cd /home/zanebono/software/Syntopica
git status --porcelain backend-go/            # 本轮后端改动的文件
git diff -- backend-go/                        # 未提交的改动
```

预期落在这些位置（以实际为准）：

- `backend-go/internal/topicgraph/repository/compose_chain.go`（**新**，候选链唯一实现）
- `backend-go/internal/topicgraph/repository/compose_chain_test.go`（新，候选链分支表用例）
- `backend-go/internal/topicgraph/repository/daily_report_manual_topic.go`（`GetComposeCandidates` 扩展）
- `backend-go/internal/topicgraph/handler/daily_report_handler.go`（`composeCandidates` 参数校验）
- `backend-go/internal/topicgraph/handler/*_test.go`（日期窗 D1–D11 用例）

**只审本轮后端改动**；同工作树里其它 change 的脏文件、前端改动、原型文件一律不看、不评。

## 3. 必查项（逐条给结论：合规 / 问题 + 证据行号）

### 3.1 口径正确性

| # | 检查点 | 依据 |
| --- | --- | --- |
| A1 | 候选链只取 `status=candidate` 且有 topic id 的 section | contract §2.1 / C3 |
| A2 | 按 topic_id 分组，**不同 topic_id 不因相似边合并** | §2.1 / C9 |
| A3 | 组内沿原 relations（identity 与 similarity 都算）取无向连通分量 | §2.1 / C8 |
| A4 | 合格条件 = 节点≥2 **且** 首末 UTC 日差≥3；断档允许 | §2.1 / C5–C7 |
| A5 | 同 topic 不连通分量各自成链、不混选 | §2.1 / C10 |
| A6 | `key` 稳定、`span_days` 按 UTC 日差（跨月边界正确） | §2.1 / C11–C12 |
| A7 | 排序 `last_date` 倒序 → `span` 倒序 → `topic_id` 升序 | §2.1 / C13 |
| A8 | relation 端点缺失/窗口外/重复边不 panic、不改判定 | §2.1 / C14–C15 |
| A9 | **没有新增任何语义边**：只消费既有 relations，不写库、不造关系 | contract §1/§2.1、daily-report 约束「不生成事件关系边」 |
| A10 | `relations` 只含**窗口内两端都可见**的原边，`relation_type` 原样带出 | contract §1.1 |

### 3.2 日期窗与参数校验

| # | 检查点 | 依据 |
| --- | --- | --- |
| B1 | from/to 只给其一 → 400 | D1–D2 |
| B2 | from>to → 400 | D3 |
| B3 | to−from>89 天 → 400，**不得静默截断** | D4 / contract §1.1 |
| B4 | from 早于取数窗 → 400 | D5 |
| B5 | `to` 晚于最新已完成日报 → 允许 | D6 |
| B6 | 日期格式非法 → 400 | D7 |
| B7 | `days` 档末日 = 最新已完成日报、含首末报告日；`days<=0` 近似全历史；`days>90` 截 90 —— 与现状一致 | D9–D10 |
| B8 | 无已完成日报 → 200 + 空数组，**不 404** | D11 |
| B9 | 日期窗计算在 Go 侧完成（无 PG 专有 INTERVAL），跨库可移植 | 现有 `GetComposeCandidates` 注释的既定纪律 |
| B10 | `sections` / `match_threshold` **向后兼容**（旧前端调用不受影响） | contract §1.1 |

### 3.3 红线（topic-graph / daily-report）

| # | 检查点 |
| --- | --- |
| C1 | **未**改动 `planLifecycle`（只更新命中计数，不自动改 status） |
| C2 | **未**改动管理名单可见门槛（`FilterVisibleTopics` 仍按累计命中 `can_activate`） |
| C3 | **未**改 `candidate_decay_window` 语义（只影响 prompt 注入） |
| C4 | **未**触碰历史 section 快照（`topic_status_at_report` 等不回填） |
| C5 | **未**改 `POST .../persistent-topics/manual` 语义（active/manual + 均值初始向量 + 单事务） |
| C6 | **未**新增/迁移表结构、**未**改 `ai_settings` 默认阈值（0.3） |
| C7 | 端点**只读**：compose-candidates 不写库、不改任何状态 |
| C8 | 无并发写风险（本轮只读端点） |

### 3.4 代码质量与安全

| # | 检查点 |
| --- | --- |
| D1 | raw SQL **参数化**（`?` 占位符），无字符串拼接用户输入 |
| D2 | 无被忽略的 error（`err != nil` 都有处理或显式日志） |
| D3 | 无 nil 解引用风险（指针字段、`repoParsePgVector` 失败路径） |
| D4 | 新代码符合 `backend-go/AGENTS.md` 风格（命名、注释语言、文件布局） |
| D5 | `golangci-lint run ./...` / `go vet ./...` 干净（可自行跑一次确认） |
| D6 | 测试**不碰真库**：用 testcontainer/临时库或纯函数单测；无 TRUNCATE/DELETE/DROP 真库执行 |
| D7 | 测试覆盖是否真的覆盖了 C1–C15 / D1–D11（**逐条对照用例断言**，指出"有用例但没断到点"的假覆盖） |

## 4. 报告格式（中文，写进会话结果即可）

```
## 结论
阻断项 N 条 / 建议项 N 条 / 观察项 N 条（一句话结论）

## 阻断项（High）
- [H1] 标题：文件:行 —— 问题 —— 证据 —— 建议改法
（无则写「无」）

## 建议项（Medium）
- [M1] …

## 观察项（Low）
- [L1] …

## 逐条检查结论表
| 检查点 | 结论 | 证据（文件:行） |
（把 §3 的 A1–A10 / B1–B10 / C1–C8 / D1–D7 全部列出，无证据的写「未找到证据」）

## 未覆盖 / 无法验证
（哪些必须真机验证才能定论，例如真实 POST 保存与事务回滚）
```

**判定纪律**：

- 找不到证据的检查点写「未找到证据」，**不要**默认合规。
- 阻断项只给「会导致错误数据 / 违反红线 / 契约不符且必然被触发」三类；风格问题一律降为建议或观察。
- 不要提与本轮后端无关的重构建议。
